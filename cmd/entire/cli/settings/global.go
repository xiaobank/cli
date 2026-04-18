package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Auto-update mode values for GlobalSettings.AutoUpdate.
const (
	AutoUpdateOff    = "off"
	AutoUpdatePrompt = "prompt"
	AutoUpdateAuto   = "auto"
)

const (
	// globalConfigDirName mirrors the path used by versioncheck for ~/.config/entire.
	globalConfigDirName = ".config/entire"

	// globalSettingsFileName is the machine-wide settings file.
	globalSettingsFileName = "settings.json"
)

// GlobalSettings represents machine-wide settings stored at
// ~/.config/entire/settings.json. Unlike the project-scoped EntireSettings,
// this file holds preferences that apply to every repository on the machine.
type GlobalSettings struct {
	// AutoUpdate is one of "off" (default), "prompt", or "auto".
	// "off"    = notification only (historical behavior).
	// "prompt" = after the notification, ask Y/N and run the installer on yes.
	// "auto"   = run the installer silently when provenance resolves and the
	//            release has aged past the soak delay.
	AutoUpdate string `json:"auto_update,omitempty"`
}

// IsValidAutoUpdate reports whether mode is a recognised auto-update value.
func IsValidAutoUpdate(mode string) bool {
	switch mode {
	case AutoUpdateOff, AutoUpdatePrompt, AutoUpdateAuto:
		return true
	}
	return false
}

// GetAutoUpdate returns the effective mode, defaulting to "off" when empty.
func (g *GlobalSettings) GetAutoUpdate() string {
	if g == nil || g.AutoUpdate == "" {
		return AutoUpdateOff
	}
	return g.AutoUpdate
}

// GlobalConfigDir returns ~/.config/entire. Callers that need to create the
// directory should use EnsureGlobalConfigDir.
func GlobalConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	return filepath.Join(home, globalConfigDirName), nil
}

// EnsureGlobalConfigDir creates ~/.config/entire if it does not exist.
func EnsureGlobalConfigDir() error {
	dir, err := GlobalConfigDir()
	if err != nil {
		return err
	}
	//nolint:gosec // ~/.config/entire is user-owned, 0o755 is appropriate
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating global config directory: %w", err)
	}
	return nil
}

// GlobalSettingsPath returns the absolute path to ~/.config/entire/settings.json.
func GlobalSettingsPath() (string, error) {
	dir, err := GlobalConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, globalSettingsFileName), nil
}

// LoadGlobal reads ~/.config/entire/settings.json. A missing file returns
// default settings (AutoUpdate=""). Malformed JSON returns an error.
func LoadGlobal() (*GlobalSettings, error) {
	path, err := GlobalSettingsPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path) //nolint:gosec // path is inside user's config dir
	if err != nil {
		if os.IsNotExist(err) {
			return &GlobalSettings{}, nil
		}
		return nil, fmt.Errorf("reading global settings: %w", err)
	}

	var g GlobalSettings
	if err := json.Unmarshal(data, &g); err != nil {
		return nil, fmt.Errorf("parsing global settings: %w", err)
	}
	return &g, nil
}

// SaveGlobal writes settings to ~/.config/entire/settings.json using an
// atomic temp-file + rename to avoid partial writes.
func SaveGlobal(g *GlobalSettings) error {
	if g == nil {
		return errors.New("global settings: nil")
	}
	if g.AutoUpdate != "" && !IsValidAutoUpdate(g.AutoUpdate) {
		return fmt.Errorf("invalid auto_update value %q: must be %q, %q, or %q",
			g.AutoUpdate, AutoUpdateOff, AutoUpdatePrompt, AutoUpdateAuto)
	}

	if err := EnsureGlobalConfigDir(); err != nil {
		return err
	}

	path, err := GlobalSettingsPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling global settings: %w", err)
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".settings_tmp_")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing global settings: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("renaming global settings file: %w", err)
	}
	return nil
}
