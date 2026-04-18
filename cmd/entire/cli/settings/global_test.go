package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadGlobal_MissingFileReturnsDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	g, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal() error = %v", err)
	}
	if g.GetAutoUpdate() != AutoUpdateOff {
		t.Errorf("GetAutoUpdate() = %q, want %q", g.GetAutoUpdate(), AutoUpdateOff)
	}
}

func TestSaveLoadGlobalRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := SaveGlobal(&GlobalSettings{AutoUpdate: AutoUpdatePrompt}); err != nil {
		t.Fatalf("SaveGlobal() error = %v", err)
	}

	g, err := LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal() error = %v", err)
	}
	if g.AutoUpdate != AutoUpdatePrompt {
		t.Errorf("AutoUpdate = %q, want %q", g.AutoUpdate, AutoUpdatePrompt)
	}
}

func TestSaveGlobal_RejectsInvalidMode(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	err := SaveGlobal(&GlobalSettings{AutoUpdate: "bogus"})
	if err == nil {
		t.Fatal("SaveGlobal() expected error for invalid mode, got nil")
	}
}

func TestSaveGlobal_CreatesConfigDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := SaveGlobal(&GlobalSettings{AutoUpdate: AutoUpdateAuto}); err != nil {
		t.Fatalf("SaveGlobal() error = %v", err)
	}

	path := filepath.Join(home, globalConfigDirName, globalSettingsFileName)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("settings file missing: %v", err)
	}
}

func TestIsValidAutoUpdate(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{AutoUpdateOff, AutoUpdatePrompt, AutoUpdateAuto} {
		if !IsValidAutoUpdate(mode) {
			t.Errorf("IsValidAutoUpdate(%q) = false, want true", mode)
		}
	}
	for _, mode := range []string{"", "bogus", "ON", "OFF", "Prompt"} {
		if IsValidAutoUpdate(mode) {
			t.Errorf("IsValidAutoUpdate(%q) = true, want false", mode)
		}
	}
}
