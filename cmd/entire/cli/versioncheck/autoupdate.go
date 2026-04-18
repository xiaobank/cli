package versioncheck

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/charmbracelet/huh"
	"golang.org/x/term"

	"github.com/entireio/cli/cmd/entire/cli/logging"
	"github.com/entireio/cli/cmd/entire/cli/settings"
)

const (
	// autoUpdateSoakDuration is how long a release must have been published
	// before "auto" mode will install it unattended. Cushions against bad
	// releases: prompt mode still installs immediately.
	autoUpdateSoakDuration = 24 * time.Hour

	// autoUpdateRetryCooldown is how long to wait after a failed attempt
	// before retrying in "auto" mode.
	autoUpdateRetryCooldown = 24 * time.Hour

	// envKillSwitch disables all auto-update behavior regardless of setting.
	envKillSwitch = "ENTIRE_NO_AUTO_UPDATE"
)

// Test seams.
var (
	runInstaller     = realRunInstaller
	loadGlobal       = settings.LoadGlobal
	nowFunc          = time.Now
	stdoutIsTerminal = defaultStdoutIsTerminal
)

func defaultStdoutIsTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd())) //nolint:gosec // G115: uintptr->int is safe for fd
}

// MaybeAutoUpdate offers or runs an auto-update after a newer release is
// detected. The caller has already printed the standard "version available"
// notification. This function is silent on every error path — failures must
// never interrupt the CLI.
func MaybeAutoUpdate(ctx context.Context, w io.Writer, currentVersion string, release *GitHubRelease) {
	if release == nil {
		return
	}
	if os.Getenv(envKillSwitch) != "" {
		logging.Debug(ctx, "auto-update: skipped by kill-switch env")
		return
	}
	if os.Getenv("CI") != "" {
		logging.Debug(ctx, "auto-update: skipped in CI")
		return
	}

	g, err := loadGlobal()
	if err != nil {
		logging.Debug(ctx, "auto-update: load global settings failed", "error", err.Error())
		return
	}
	mode := g.GetAutoUpdate()
	if mode == settings.AutoUpdateOff {
		return
	}

	if !stdoutIsTerminal() {
		logging.Debug(ctx, "auto-update: skipped (no TTY)")
		return
	}

	cmdStr := updateCommand(currentVersion)
	manager := installManagerForCurrentBinary()

	if mode == settings.AutoUpdateAuto {
		if manager == installManagerUnknown {
			logging.Debug(ctx, "auto-update: skipped (install manager unknown)")
			return
		}
		if !release.PublishedAt.IsZero() && nowFunc().Sub(release.PublishedAt) < autoUpdateSoakDuration {
			logging.Debug(ctx, "auto-update: skipped (release under soak delay)")
			return
		}
		if cache, cerr := loadCache(); cerr == nil {
			if !cache.LastUpdateAttemptTime.IsZero() && !cache.LastUpdateAttemptOk &&
				nowFunc().Sub(cache.LastUpdateAttemptTime) < autoUpdateRetryCooldown {
				logging.Debug(ctx, "auto-update: skipped (recent failure)")
				return
			}
		}
	}

	if mode == settings.AutoUpdatePrompt {
		confirmed, perr := confirmUpdate(release.TagName)
		if perr != nil {
			logging.Debug(ctx, "auto-update: prompt failed", "error", perr.Error())
			return
		}
		if !confirmed {
			return
		}
	}

	fmt.Fprintf(w, "\nUpdating Entire CLI: %s\n", cmdStr)
	runErr := runInstaller(ctx, cmdStr)
	recordUpdateAttempt(ctx, runErr == nil)
	if runErr != nil {
		fmt.Fprintf(w, "Update failed: %v\n", runErr)
		return
	}
	fmt.Fprintf(w, "Update complete. Re-run entire to use the new version.\n")
}

// confirmUpdate prompts the user interactively. Returns false if they declined
// or aborted (Ctrl-C / timeout), true only on explicit yes.
func confirmUpdate(latest string) (bool, error) {
	var confirmed bool
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("Install Entire CLI %s now?", latest)).
				Affirmative("Yes").
				Negative("No").
				Value(&confirmed),
		),
	).WithTheme(huh.ThemeDracula())
	if os.Getenv("ACCESSIBLE") != "" {
		form = form.WithAccessible(true)
	}
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) || errors.Is(err, huh.ErrTimeout) {
			return false, nil
		}
		return false, fmt.Errorf("confirm form: %w", err)
	}
	return confirmed, nil
}

// realRunInstaller shells out to the installer command, streaming stdin/stdout/stderr
// so password prompts and progress output reach the user.
func realRunInstaller(ctx context.Context, cmdStr string) error {
	var c *exec.Cmd
	if runtime.GOOS == "windows" {
		c = exec.CommandContext(ctx, "cmd", "/C", cmdStr)
	} else {
		c = exec.CommandContext(ctx, "sh", "-c", cmdStr)
	}
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("installer exited: %w", err)
	}
	return nil
}

// recordUpdateAttempt persists the outcome of the last install attempt so
// "auto" mode can back off after failures.
func recordUpdateAttempt(ctx context.Context, ok bool) {
	cache, err := loadCache()
	if err != nil {
		cache = &VersionCache{}
	}
	cache.LastUpdateAttemptTime = nowFunc()
	cache.LastUpdateAttemptOk = ok
	if err := saveCache(cache); err != nil {
		logging.Debug(ctx, "auto-update: save cache failed", "error", err.Error())
	}
}

// RunUpdateNow is the one-shot update entry point used by `entire update`.
// It ignores the 24h check cache and the auto_update mode gate, but still
// respects the kill-switch env, TTY/CI checks (unless skipPrompt is true),
// and resolves the installer command from provenance.
// Returns (ran bool, err error). ran=false means the user declined or
// the gates blocked execution; err indicates a real failure.
func RunUpdateNow(ctx context.Context, w io.Writer, currentVersion string, checkOnly, skipPrompt bool) (bool, error) {
	if os.Getenv(envKillSwitch) != "" {
		return false, errors.New("auto-update disabled by " + envKillSwitch)
	}

	cmdStr := updateCommand(currentVersion)
	manager := installManagerForCurrentBinary()
	if checkOnly {
		if manager == installManagerUnknown {
			fmt.Fprintf(w, "Update command (fallback, install manager unknown): %s\n", cmdStr)
		} else {
			fmt.Fprintf(w, "Update command (%s): %s\n", manager, cmdStr)
		}
		return false, nil
	}

	if !skipPrompt {
		if !stdoutIsTerminal() {
			return false, errors.New("refusing to run installer on non-TTY stdout (use --yes)")
		}
		confirmed, perr := confirmUpdate("")
		if perr != nil {
			return false, fmt.Errorf("prompt: %w", perr)
		}
		if !confirmed {
			return false, nil
		}
	}

	fmt.Fprintf(w, "\nUpdating Entire CLI: %s\n", cmdStr)
	runErr := runInstaller(ctx, cmdStr)
	recordUpdateAttempt(ctx, runErr == nil)
	if runErr != nil {
		return false, fmt.Errorf("installer: %w", runErr)
	}
	fmt.Fprintf(w, "Update complete. Re-run entire to use the new version.\n")
	return true, nil
}
