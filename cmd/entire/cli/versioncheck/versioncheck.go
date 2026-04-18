package versioncheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/entireio/cli/cmd/entire/cli/logging"
	"github.com/entireio/cli/cmd/entire/cli/versioninfo"
	"golang.org/x/mod/semver"
)

// CheckAndNotify performs a version check and notifies the user if a newer version is available.
// This is the main entry point for the version check system.
// The function is silent on all errors to avoid interrupting CLI operations.
func CheckAndNotify(ctx context.Context, w io.Writer, currentVersion string) {
	// Skip checks for dev builds
	if currentVersion == "dev" || currentVersion == "" {
		return
	}

	// Ensure the global config directory exists
	if err := ensureGlobalConfigDir(); err != nil {
		// Silent failure - don't block CLI operations
		return
	}

	// Load the cache to check when we last fetched
	cache, err := loadCache()
	if err != nil {
		cache = &VersionCache{}
	}

	// Skip if we checked recently (within 24 hours)
	if time.Since(cache.LastCheckTime) < checkInterval {
		return
	}

	// Fetch the latest release from the appropriate channel
	var release *GitHubRelease
	if isNightly(currentVersion) {
		release, err = fetchLatestNightlyRelease(ctx)
	} else {
		release, err = fetchLatestRelease(ctx)
	}

	// Always update cache to avoid retrying on every CLI invocation
	cache.LastCheckTime = time.Now()
	if saveErr := saveCache(cache); saveErr != nil {
		logging.Debug(ctx, "version check: failed to save cache",
			"error", saveErr.Error())
	}

	if err != nil {
		logging.Debug(ctx, "version check: failed to fetch latest version",
			"error", err.Error())
		return
	}

	// Show notification and (if configured) offer/run an auto-update when outdated
	if isOutdated(currentVersion, release.TagName) {
		printNotification(w, currentVersion, release.TagName)
		MaybeAutoUpdate(ctx, w, currentVersion, release)
	}
}

// globalConfigDirPath returns the expanded path to the global config directory (~/.config/entire).
func globalConfigDirPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	return filepath.Join(home, globalConfigDirName), nil
}

// ensureGlobalConfigDir creates the global config directory if it doesn't exist.
func ensureGlobalConfigDir() error {
	configDir, err := globalConfigDirPath()
	if err != nil {
		return err
	}

	//nolint:gosec // ~/.config/entire is user home directory, 0o755 is appropriate
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	return nil
}

// cacheFilePath returns the full path to the version check cache file.
func cacheFilePath() (string, error) {
	configDir, err := globalConfigDirPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, cacheFileName), nil
}

// loadCache loads the version check cache from disk.
// Returns an error if the file doesn't exist or is corrupted.
func loadCache() (*VersionCache, error) {
	filePath, err := cacheFilePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(filePath) //nolint:gosec // cacheFilePath is safe
	if err != nil {
		return nil, fmt.Errorf("reading cache file: %w", err)
	}

	var cache VersionCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, fmt.Errorf("parsing cache: %w", err)
	}

	return &cache, nil
}

// saveCache saves the version check cache to disk.
// Uses atomic write semantics (write to temp file, then rename).
func saveCache(cache *VersionCache) error {
	filePath, err := cacheFilePath()
	if err != nil {
		return err
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling cache: %w", err)
	}

	// Write to temp file first (atomic write)
	dir := filepath.Dir(filePath)
	tmpFile, err := os.CreateTemp(dir, ".version_check_tmp_")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close() // cleanup on error path
		return fmt.Errorf("writing cache: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}

	// Rename temp file to final location

	if err := os.Rename(tmpFile.Name(), filePath); err != nil {
		return fmt.Errorf("renaming cache file: %w", err)
	}

	return nil
}

// fetchLatestRelease fetches the latest stable release from the GitHub API.
func fetchLatestRelease(ctx context.Context) (*GitHubRelease, error) {
	ctx, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubAPIURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "entire-cli")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching release info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	release, err := parseGitHubRelease(body)
	if err != nil {
		return nil, fmt.Errorf("parsing release: %w", err)
	}

	return release, nil
}

// isNightly returns true if the version string is a nightly build.
func isNightly(version string) bool {
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}
	return strings.Contains(semver.Prerelease(version), "nightly")
}

// fetchLatestNightlyRelease fetches the latest nightly release from the GitHub releases list.
func fetchLatestNightlyRelease(ctx context.Context) (*GitHubRelease, error) {
	ctx, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubReleasesURL+"?per_page=20", nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "entire-cli/"+versioninfo.Version)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	var releases []GitHubRelease
	if err := json.Unmarshal(body, &releases); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}

	for i, r := range releases {
		if r.Prerelease && strings.Contains(r.TagName, "-nightly.") {
			return &releases[i], nil
		}
	}

	return nil, errors.New("no nightly release found")
}

// parseGitHubRelease parses the GitHub API response and returns the latest stable release.
// Filters out prerelease versions.
func parseGitHubRelease(body []byte) (*GitHubRelease, error) {
	var release GitHubRelease
	if err := json.Unmarshal(body, &release); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}

	if release.Prerelease {
		return nil, errors.New("only prerelease versions available")
	}

	if release.TagName == "" {
		return nil, errors.New("empty tag name")
	}

	return &release, nil
}

// isOutdated compares current and latest versions using semantic versioning.
// Returns true if current < latest.
func isOutdated(current, latest string) bool {
	// Ensure versions have "v" prefix for semver package
	if !strings.HasPrefix(current, "v") {
		current = "v" + current
	}
	if !strings.HasPrefix(latest, "v") {
		latest = "v" + latest
	}

	// Skip notification for dev builds (e.g., "1.0.0-dev-xxx").
	// These are local development builds and shouldn't trigger update notifications.
	// Normal prereleases (e.g., "1.0.0-rc1") should still be compared normally.
	if strings.Contains(semver.Prerelease(current), "dev") {
		return false
	}

	// semver.Compare returns -1 if current < latest
	return semver.Compare(current, latest) < 0
}

// executablePath is the function used to get the current executable path.
// It's a variable so tests can override it.
var executablePath = os.Executable

type updateHint struct {
	Command string
}

func installProvenanceFilePath() (string, error) {
	configDir, err := globalConfigDirPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, installProvenanceFileName), nil
}

func loadInstallProvenance() (*InstallProvenance, error) {
	filePath, err := installProvenanceFilePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(filePath) //nolint:gosec // filePath is inside the user's config dir
	if err != nil {
		return nil, fmt.Errorf("reading install provenance: %w", err)
	}

	var provenance InstallProvenance
	if err := json.Unmarshal(data, &provenance); err != nil {
		return nil, fmt.Errorf("parsing install provenance: %w", err)
	}

	return &provenance, nil
}

const (
	channelStable  = "stable"
	channelNightly = "nightly"
)

func resolveUpdateHintFromProvenance(provenance *InstallProvenance) (updateHint, bool) {
	if provenance == nil {
		return updateHint{}, false
	}

	channel := strings.TrimSpace(provenance.Channel)
	if channel == "" {
		channel = channelStable
	}

	switch strings.TrimSpace(provenance.Manager) {
	case "install.sh":
		if channel != channelStable {
			return updateHint{}, false
		}
		return updateHint{Command: "curl -fsSL https://entire.io/install.sh | bash"}, true
	case "brew":
		if channel != channelStable && channel != channelNightly {
			return updateHint{}, false
		}
		pkg := strings.TrimSpace(provenance.Package)
		if pkg == "" {
			pkg = "entire"
		}
		return updateHint{Command: "brew upgrade " + pkg}, true
	case "scoop":
		if channel != channelStable && channel != channelNightly {
			return updateHint{}, false
		}
		pkg := strings.TrimSpace(provenance.Package)
		if pkg == "" {
			pkg = "entire/cli"
		}
		return updateHint{Command: "scoop update " + pkg}, true
	default:
		return updateHint{}, false
	}
}

func inferUpdateHintFromExecutablePath() updateHint {
	execPath, err := executablePath()
	if err != nil {
		return updateHint{Command: "curl -fsSL https://entire.io/install.sh | bash"}
	}

	// Resolve symlinks to find the real path (Homebrew symlinks from bin/ to Cellar/)
	realPath, err := filepath.EvalSymlinks(execPath)
	if err != nil {
		realPath = execPath
	}

	if strings.Contains(realPath, "/Cellar/") || strings.Contains(realPath, "/opt/homebrew/") || strings.Contains(realPath, "/linuxbrew/") {
		return updateHint{Command: "brew upgrade entire"}
	}

	if strings.Contains(realPath, "/mise/installs/") {
		return updateHint{Command: "mise upgrade entire"}
	}

	return updateHint{Command: "curl -fsSL https://entire.io/install.sh | bash"}
}

// resolveUpdateCommand returns the update instruction and whether it came from
// install provenance (true) or the executable-path fallback (false).
// "auto" mode refuses to execute the command when fromProvenance is false.
func resolveUpdateCommand() (cmd string, fromProvenance bool) {
	provenance, err := loadInstallProvenance()
	if err == nil {
		if hint, ok := resolveUpdateHintFromProvenance(provenance); ok {
			return hint.Command, true
		}
	}
	return inferUpdateHintFromExecutablePath().Command, false
}

// updateCommand returns the appropriate update instruction string.
// Preserved as a thin wrapper for the notification message.
func updateCommand() string {
	cmd, _ := resolveUpdateCommand()
	return cmd
}

// printNotification prints the version update notification to the user.
func printNotification(w io.Writer, current, latest string) {
	msg := fmt.Sprintf("\nA newer version of Entire CLI is available: %s (current: %s)\nRun '%s' to update.\n",
		latest, current, updateCommand())
	fmt.Fprint(w, msg)
}
