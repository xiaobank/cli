package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestUpdateCmd_CheckOnlyPrintsCommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cmd := newUpdateCmd()
	cmd.SetArgs([]string{"--check-only"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute error = %v", err)
	}
	if !strings.Contains(out.String(), "Update command") {
		t.Errorf("output missing command header: %q", out.String())
	}
}
