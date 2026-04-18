package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/entireio/cli/cmd/entire/cli/settings"
)

func TestRunAutoUpdateStatus_DefaultOff(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var buf bytes.Buffer
	if err := runAutoUpdateStatus(&buf); err != nil {
		t.Fatalf("runAutoUpdateStatus() error = %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "auto_update: off") {
		t.Errorf("status missing default mode: %q", out)
	}
	if !strings.Contains(out, "settings.json") {
		t.Errorf("status missing config path: %q", out)
	}
}

func TestRunAutoUpdateSet_Persists(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var buf bytes.Buffer
	if err := runAutoUpdateSet(&buf, settings.AutoUpdatePrompt); err != nil {
		t.Fatalf("runAutoUpdateSet() error = %v", err)
	}

	g, err := settings.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal() error = %v", err)
	}
	if g.AutoUpdate != settings.AutoUpdatePrompt {
		t.Errorf("AutoUpdate = %q, want %q", g.AutoUpdate, settings.AutoUpdatePrompt)
	}
}

func TestAutoUpdateCmd_SetRejectsInvalid(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cmd := newAutoUpdateCmd()
	cmd.SetArgs([]string{"set", "bogus"})
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid mode")
	}
	if !strings.Contains(errBuf.String(), "invalid mode") {
		t.Errorf("stderr missing error message: %q", errBuf.String())
	}
}

func TestAutoUpdateCmd_EnableDisable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cmd := newAutoUpdateCmd()
	cmd.SetArgs([]string{"enable"})
	cmd.SetOut(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("enable error = %v", err)
	}
	g, err := settings.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal() error = %v", err)
	}
	if g.AutoUpdate != settings.AutoUpdatePrompt {
		t.Errorf("after enable: AutoUpdate = %q, want %q", g.AutoUpdate, settings.AutoUpdatePrompt)
	}

	cmd2 := newAutoUpdateCmd()
	cmd2.SetArgs([]string{"disable"})
	cmd2.SetOut(&bytes.Buffer{})
	if err := cmd2.Execute(); err != nil {
		t.Fatalf("disable error = %v", err)
	}
	g, err = settings.LoadGlobal()
	if err != nil {
		t.Fatalf("LoadGlobal() error = %v", err)
	}
	if g.AutoUpdate != settings.AutoUpdateOff {
		t.Errorf("after disable: AutoUpdate = %q, want %q", g.AutoUpdate, settings.AutoUpdateOff)
	}
}
