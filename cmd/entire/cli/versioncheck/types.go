package versioncheck

import "time"

// VersionCache represents the cached version check data.
type VersionCache struct {
	LastCheckTime         time.Time `json:"last_check_time"`
	LastUpdateAttemptTime time.Time `json:"last_update_attempt_time,omitzero"`
	LastUpdateAttemptOk   bool      `json:"last_update_attempt_ok,omitempty"`
}

// InstallProvenance describes how a released Entire binary was installed.
// The CLI reads this file but does not create or modify it.
type InstallProvenance struct {
	Manager     string    `json:"manager"`
	Channel     string    `json:"channel"`
	Package     string    `json:"package"`
	InstalledAt time.Time `json:"installed_at"`
}

// GitHubRelease represents the GitHub API response for a release.
type GitHubRelease struct {
	TagName     string    `json:"tag_name"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
}

// githubAPIURL is the GitHub API endpoint for fetching the latest stable release.
// This is a var (not const) to allow overriding in tests.
var githubAPIURL = "https://api.github.com/repos/entireio/cli/releases/latest"

// githubReleasesURL is the GitHub API endpoint for listing releases (used for nightly checks).
var githubReleasesURL = "https://api.github.com/repos/entireio/cli/releases"

const (
	// checkInterval is the duration between version checks.
	checkInterval = 24 * time.Hour

	// httpTimeout is the timeout for HTTP requests to the GitHub API.
	httpTimeout = 2 * time.Second

	// cacheFileName is the name of the cache file stored in the global config directory.
	cacheFileName = "version_check.json"

	// installProvenanceFileName is the installer-owned provenance file.
	installProvenanceFileName = "install.json"

	// globalConfigDirName is the name of the global config directory in the user's home.
	globalConfigDirName = ".config/entire"
)
