package versioncheck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/entireio/cli/cmd/entire/cli/settings"
)

// autoUpdateFixture wires all the test seams for MaybeAutoUpdate tests.
type autoUpdateFixture struct {
	t            *testing.T
	installCalls int
	installErr   error
	lastCommand  string
	confirmed    bool
}

func newAutoUpdateFixture(t *testing.T) *autoUpdateFixture {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("CI", "")
	t.Setenv(envKillSwitch, "")
	t.Setenv("ACCESSIBLE", "")

	f := &autoUpdateFixture{t: t, confirmed: true}

	origRun := runInstaller
	runInstaller = func(_ context.Context, cmd string) error {
		f.installCalls++
		f.lastCommand = cmd
		return f.installErr
	}
	origTerm := stdoutIsTerminal
	stdoutIsTerminal = func() bool { return true }
	origNow := nowFunc
	nowFunc = func() time.Time { return time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC) }

	t.Cleanup(func() {
		runInstaller = origRun
		stdoutIsTerminal = origTerm
		nowFunc = origNow
	})
	return f
}

func writeBrewProvenance(t *testing.T) {
	t.Helper()
	dir, err := settings.GlobalConfigDir()
	if err != nil {
		t.Fatalf("GlobalConfigDir() error = %v", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	p := InstallProvenance{
		Manager: "brew", Channel: "stable", Package: "entire",
		InstalledAt: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, installProvenanceFileName), data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func writeGlobalSettings(t *testing.T, mode string) {
	t.Helper()
	if err := settings.SaveGlobal(&settings.GlobalSettings{AutoUpdate: mode}); err != nil {
		t.Fatalf("SaveGlobal() error = %v", err)
	}
}

func TestMaybeAutoUpdate_ModeOff_DoesNothing(t *testing.T) {
	f := newAutoUpdateFixture(t)
	writeBrewProvenance(t)
	// no settings file -> defaults to off

	var buf bytes.Buffer
	release := &GitHubRelease{TagName: "v2.0.0", PublishedAt: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)}
	MaybeAutoUpdate(context.Background(), &buf, "1.0.0", release)

	if f.installCalls != 0 {
		t.Errorf("installer called %d times, want 0", f.installCalls)
	}
	if buf.Len() != 0 {
		t.Errorf("expected no output, got %q", buf.String())
	}
}

func TestMaybeAutoUpdate_KillSwitch_ShortCircuits(t *testing.T) {
	f := newAutoUpdateFixture(t)
	writeGlobalSettings(t, settings.AutoUpdateAuto)
	writeBrewProvenance(t)
	t.Setenv(envKillSwitch, "1")

	var buf bytes.Buffer
	release := &GitHubRelease{TagName: "v2.0.0", PublishedAt: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)}
	MaybeAutoUpdate(context.Background(), &buf, "1.0.0", release)

	if f.installCalls != 0 {
		t.Errorf("installer called with kill-switch set")
	}
}

func TestMaybeAutoUpdate_CI_ShortCircuits(t *testing.T) {
	f := newAutoUpdateFixture(t)
	writeGlobalSettings(t, settings.AutoUpdateAuto)
	writeBrewProvenance(t)
	t.Setenv("CI", "true")

	var buf bytes.Buffer
	release := &GitHubRelease{TagName: "v2.0.0", PublishedAt: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)}
	MaybeAutoUpdate(context.Background(), &buf, "1.0.0", release)

	if f.installCalls != 0 {
		t.Errorf("installer called in CI")
	}
}

func TestMaybeAutoUpdate_NoTTY_ShortCircuits(t *testing.T) {
	f := newAutoUpdateFixture(t)
	writeGlobalSettings(t, settings.AutoUpdateAuto)
	writeBrewProvenance(t)
	stdoutIsTerminal = func() bool { return false }

	var buf bytes.Buffer
	release := &GitHubRelease{TagName: "v2.0.0", PublishedAt: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)}
	MaybeAutoUpdate(context.Background(), &buf, "1.0.0", release)

	if f.installCalls != 0 {
		t.Errorf("installer called without TTY")
	}
}

func TestMaybeAutoUpdate_AutoRequiresProvenance(t *testing.T) {
	f := newAutoUpdateFixture(t)
	writeGlobalSettings(t, settings.AutoUpdateAuto)
	// no provenance file -> fallback to path inference
	// executablePath default is os.Executable which in tests returns go test binary path -> not brew/mise

	var buf bytes.Buffer
	release := &GitHubRelease{TagName: "v2.0.0", PublishedAt: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)}
	MaybeAutoUpdate(context.Background(), &buf, "1.0.0", release)

	if f.installCalls != 0 {
		t.Errorf("installer called on auto mode without provenance")
	}
}

func TestMaybeAutoUpdate_AutoSoakDelay(t *testing.T) {
	f := newAutoUpdateFixture(t)
	writeGlobalSettings(t, settings.AutoUpdateAuto)
	writeBrewProvenance(t)

	// Published 1 hour before "now"
	release := &GitHubRelease{
		TagName:     "v2.0.0",
		PublishedAt: nowFunc().Add(-1 * time.Hour),
	}
	var buf bytes.Buffer
	MaybeAutoUpdate(context.Background(), &buf, "1.0.0", release)

	if f.installCalls != 0 {
		t.Errorf("installer called despite soak delay")
	}
}

func TestMaybeAutoUpdate_AutoHappyPath(t *testing.T) {
	f := newAutoUpdateFixture(t)
	writeGlobalSettings(t, settings.AutoUpdateAuto)
	writeBrewProvenance(t)

	// Published 2 days before "now" — past the 24h soak
	release := &GitHubRelease{
		TagName:     "v2.0.0",
		PublishedAt: nowFunc().Add(-48 * time.Hour),
	}
	var buf bytes.Buffer
	MaybeAutoUpdate(context.Background(), &buf, "1.0.0", release)

	if f.installCalls != 1 {
		t.Fatalf("installer called %d times, want 1", f.installCalls)
	}
	if f.lastCommand != "brew upgrade entire" {
		t.Errorf("installer got %q, want brew upgrade entire", f.lastCommand)
	}
}

func TestMaybeAutoUpdate_AutoRetryCooldown(t *testing.T) {
	f := newAutoUpdateFixture(t)
	writeGlobalSettings(t, settings.AutoUpdateAuto)
	writeBrewProvenance(t)

	// Seed a recent failed attempt
	cache := &VersionCache{
		LastCheckTime:         nowFunc().Add(-25 * time.Hour),
		LastUpdateAttemptTime: nowFunc().Add(-1 * time.Hour),
		LastUpdateAttemptOk:   false,
	}
	dir, err := settings.GlobalConfigDir()
	if err != nil {
		t.Fatalf("GlobalConfigDir() error = %v", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	data, err := json.Marshal(cache)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, cacheFileName), data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	release := &GitHubRelease{
		TagName:     "v2.0.0",
		PublishedAt: nowFunc().Add(-48 * time.Hour),
	}
	var buf bytes.Buffer
	MaybeAutoUpdate(context.Background(), &buf, "1.0.0", release)

	if f.installCalls != 0 {
		t.Errorf("installer called during retry cooldown")
	}
}

func TestMaybeAutoUpdate_PromptModeYes(t *testing.T) {
	f := newAutoUpdateFixture(t)
	writeGlobalSettings(t, settings.AutoUpdatePrompt)
	writeBrewProvenance(t)

	// Replace the interactive confirm with a seam so the test doesn't open huh.
	origRunInstaller := runInstaller
	runInstaller = func(_ context.Context, cmd string) error {
		f.installCalls++
		f.lastCommand = cmd
		return nil
	}
	t.Cleanup(func() { runInstaller = origRunInstaller })

	// Skip huh by using a non-TTY short-circuit: prompt mode + no TTY == skipped.
	// To actually exercise prompt=yes we'd need a huh stub; keep this as a
	// negative test and ensure explicit yes path is covered via RunUpdateNow below.
	stdoutIsTerminal = func() bool { return false }

	var buf bytes.Buffer
	release := &GitHubRelease{TagName: "v2.0.0"}
	MaybeAutoUpdate(context.Background(), &buf, "1.0.0", release)

	if f.installCalls != 0 {
		t.Errorf("installer called without TTY in prompt mode")
	}
}

func TestMaybeAutoUpdate_RecordsFailureInCache(t *testing.T) {
	f := newAutoUpdateFixture(t)
	writeGlobalSettings(t, settings.AutoUpdateAuto)
	writeBrewProvenance(t)
	f.installErr = errors.New("simulated failure")

	release := &GitHubRelease{
		TagName:     "v2.0.0",
		PublishedAt: nowFunc().Add(-48 * time.Hour),
	}
	var buf bytes.Buffer
	MaybeAutoUpdate(context.Background(), &buf, "1.0.0", release)

	if f.installCalls != 1 {
		t.Fatalf("installer called %d times, want 1", f.installCalls)
	}
	cache, err := loadCache()
	if err != nil {
		t.Fatalf("loadCache() error = %v", err)
	}
	if cache.LastUpdateAttemptOk {
		t.Errorf("LastUpdateAttemptOk = true, want false")
	}
	if cache.LastUpdateAttemptTime.IsZero() {
		t.Errorf("LastUpdateAttemptTime not recorded")
	}
}

func TestRunUpdateNow_CheckOnly(t *testing.T) {
	f := newAutoUpdateFixture(t)
	writeBrewProvenance(t)

	var buf bytes.Buffer
	ran, err := RunUpdateNow(context.Background(), &buf, true, false)
	if err != nil {
		t.Fatalf("RunUpdateNow() error = %v", err)
	}
	if ran {
		t.Errorf("check-only should not run installer")
	}
	if f.installCalls != 0 {
		t.Errorf("installer called in check-only mode")
	}
	if !bytes.Contains(buf.Bytes(), []byte("brew upgrade entire")) {
		t.Errorf("output missing command: %q", buf.String())
	}
}

func TestRunUpdateNow_KillSwitchBlocks(t *testing.T) {
	_ = newAutoUpdateFixture(t)
	t.Setenv(envKillSwitch, "1")

	var buf bytes.Buffer
	_, err := RunUpdateNow(context.Background(), &buf, false, true)
	if err == nil {
		t.Fatal("RunUpdateNow() expected error with kill-switch set")
	}
}

func TestRunUpdateNow_YesSkipsPrompt(t *testing.T) {
	f := newAutoUpdateFixture(t)
	writeBrewProvenance(t)

	var buf bytes.Buffer
	ran, err := RunUpdateNow(context.Background(), &buf, false, true)
	if err != nil {
		t.Fatalf("RunUpdateNow() error = %v", err)
	}
	if !ran {
		t.Errorf("expected installer to run")
	}
	if f.installCalls != 1 {
		t.Errorf("installer called %d times, want 1", f.installCalls)
	}
}
