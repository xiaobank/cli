package agent

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestStripGitEnv(t *testing.T) {
	t.Parallel()

	env := []string{
		"HOME=/Users/test",
		"GIT_DIR=/repo/.git",
		"PATH=/usr/bin",
		"GIT_WORK_TREE=/repo",
		"GIT_INDEX_FILE=/repo/.git/index",
		"SHELL=/bin/zsh",
		"CLAUDECODE=1",
	}

	filtered := StripGitEnv(env)

	expected := []string{
		"HOME=/Users/test",
		"PATH=/usr/bin",
		"SHELL=/bin/zsh",
	}

	if len(filtered) != len(expected) {
		t.Fatalf("got %d entries, want %d", len(filtered), len(expected))
	}

	for i, e := range filtered {
		if e != expected[i] {
			t.Errorf("filtered[%d] = %q, want %q", i, e, expected[i])
		}
	}
}

func TestStripGitEnv_NoGitVars(t *testing.T) {
	t.Parallel()

	env := []string{"HOME=/Users/test", "PATH=/usr/bin"}
	filtered := StripGitEnv(env)

	if len(filtered) != 2 {
		t.Errorf("expected 2 entries, got %d", len(filtered))
	}
}

func TestStripGitEnv_Empty(t *testing.T) {
	t.Parallel()

	filtered := StripGitEnv(nil)
	if len(filtered) != 0 {
		t.Errorf("expected 0 entries, got %d", len(filtered))
	}
}

func TestFormatExecError_NotFound(t *testing.T) {
	t.Parallel()

	err := &exec.Error{Name: "claude", Err: errors.New("not found")}
	result := FormatExecError(err, "claude", "")

	if !strings.Contains(result.Error(), "claude CLI not found") {
		t.Errorf("expected 'claude CLI not found', got: %v", result)
	}
}

func TestFormatExecError_ExitError(t *testing.T) {
	t.Parallel()

	// Use a real command that will fail to get an ExitError
	cmd := exec.CommandContext(context.Background(), "sh", "-c", "exit 42")
	err := cmd.Run()
	result := FormatExecError(err, "claude", "some stderr output")

	if !strings.Contains(result.Error(), "claude CLI failed") {
		t.Errorf("expected 'claude CLI failed', got: %v", result)
	}
	if !strings.Contains(result.Error(), "exit 42") {
		t.Errorf("expected exit code 42, got: %v", result)
	}
	if !strings.Contains(result.Error(), "some stderr output") {
		t.Errorf("expected stderr content, got: %v", result)
	}
}

func TestFormatExecError_GenericError(t *testing.T) {
	t.Parallel()

	err := errors.New("connection refused")
	result := FormatExecError(err, "claude", "")

	if !strings.Contains(result.Error(), "failed to run claude CLI") {
		t.Errorf("expected 'failed to run claude CLI', got: %v", result)
	}
}
