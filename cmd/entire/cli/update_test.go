package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUpdateCmd_CheckOnlyPrintsCommand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Seed install provenance so the command resolves deterministically.
	dir := filepath.Join(home, ".config", "entire")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	prov := map[string]any{
		"manager":      "brew",
		"channel":      "stable",
		"package":      "entire",
		"installed_at": time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.Marshal(prov)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "install.json"), data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"--check-only"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute error = %v", err)
	}
	if !strings.Contains(out.String(), "brew upgrade entire") {
		t.Errorf("output missing command: %q", out.String())
	}
}
