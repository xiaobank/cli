package cli

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// initTestRepo creates a minimal git repo in a temp dir and chdir into it.
func initTestRepo(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	repo, err := git.PlainInit(tmpDir, false)
	if err != nil {
		t.Fatalf("Failed to init repo: %v", err)
	}

	w, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Failed to get worktree: %v", err)
	}
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0o644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}
	if _, err := w.Add("test.txt"); err != nil {
		t.Fatalf("Failed to add test file: %v", err)
	}
	if _, err := w.Commit("initial commit", &git.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@example.com"},
	}); err != nil {
		t.Fatalf("Failed to create initial commit: %v", err)
	}

	return tmpDir
}

// captureStderr captures stderr output during fn execution.
// Restores os.Stderr before reading to avoid resource leaks.
// NOT safe for parallel tests (mutates process-global os.Stderr).
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	defer r.Close()
	os.Stderr = w

	fn()

	w.Close()
	os.Stderr = oldStderr // Restore BEFORE reading to avoid leak on panic

	data, readErr := io.ReadAll(r)
	if readErr != nil {
		t.Fatalf("Failed to read from pipe: %v", readErr)
	}
	return string(data)
}

func TestSnapshotLocalGitConfig_NoLocalConfig(t *testing.T) {
	initTestRepo(t)

	snap := snapshotLocalGitConfig()
	if snap.name != "" {
		t.Errorf("expected empty name, got %q", snap.name)
	}
	if snap.email != "" {
		t.Errorf("expected empty email, got %q", snap.email)
	}
}

func TestSnapshotLocalGitConfig_WithLocalConfig(t *testing.T) {
	dir := initTestRepo(t)

	// Set local config values
	setLocalConfig(t, dir, "user.name", "Local User")
	setLocalConfig(t, dir, "user.email", "local@example.com")

	snap := snapshotLocalGitConfig()
	if snap.name != "Local User" {
		t.Errorf("expected name %q, got %q", "Local User", snap.name)
	}
	if snap.email != "local@example.com" {
		t.Errorf("expected email %q, got %q", "local@example.com", snap.email)
	}
}

func TestCheckConfigIntegrity_NoChange(t *testing.T) {
	// NOT parallel — uses captureStderr which mutates os.Stderr
	before := configSnapshot{name: "", email: ""}
	after := configSnapshot{name: "", email: ""}

	output := captureStderr(t, func() {
		checkConfigIntegrity(context.Background(), "test-op", before, after)
	})

	if len(output) > 0 {
		t.Errorf("expected no stderr output for unchanged config, got: %s", output)
	}
}

func TestCheckConfigIntegrity_DetectsChange(t *testing.T) {
	before := configSnapshot{name: "", email: ""}
	after := configSnapshot{name: "someone", email: "someone@test.com"}

	output := captureStderr(t, func() {
		checkConfigIntegrity(context.Background(), "test-op", before, after)
	})

	if len(output) == 0 {
		t.Fatal("expected warning on stderr, got nothing")
	}
	if !strings.Contains(output, "WARNING") {
		t.Errorf("expected WARNING in stderr output, got: %s", output)
	}
}

func TestCheckConfigIntegrity_DetectsEmailOnlyChange(t *testing.T) {
	before := configSnapshot{name: "Same Name", email: ""}
	after := configSnapshot{name: "Same Name", email: "changed@test.com"}

	output := captureStderr(t, func() {
		checkConfigIntegrity(context.Background(), "test-op", before, after)
	})

	if len(output) == 0 {
		t.Fatal("expected warning on stderr for email-only change, got nothing")
	}
	if !strings.Contains(output, "WARNING") {
		t.Errorf("expected WARNING in stderr output, got: %s", output)
	}
}

func TestCheckConfigIntegrity_Detects456Pattern(t *testing.T) {
	// Use realistic before state: real name getting overwritten
	before := configSnapshot{name: "Real User", email: "real@example.com"}
	after := configSnapshot{name: "user.email", email: "real@example.com"}

	output := captureStderr(t, func() {
		checkConfigIntegrity(context.Background(), "test-op", before, after)
	})

	if len(output) == 0 {
		t.Fatal("expected warning on stderr, got nothing")
	}
	if !strings.Contains(output, "456") {
		t.Errorf("expected #456 reference in stderr output, got: %s", output)
	}
	if !strings.Contains(output, "literal string") {
		t.Errorf("expected 'literal string' in stderr output, got: %s", output)
	}
}

func TestCheckConfigIntegrity_EmailOnlyChangeWithPreExistingCorruption(t *testing.T) {
	// Cursor Bugbot catch: if user.name was already "user.email" and only email changed,
	// the #456 warning should NOT fire (name didn't change during this operation).
	before := configSnapshot{name: "user.email", email: "old@example.com"}
	after := configSnapshot{name: "user.email", email: "new@example.com"}

	output := captureStderr(t, func() {
		checkConfigIntegrity(context.Background(), "test-op", before, after)
	})

	if len(output) == 0 {
		t.Fatal("expected generic warning on stderr for email change, got nothing")
	}
	if strings.Contains(output, "literal string") {
		t.Errorf("should NOT fire #456 warning when name didn't change, got: %s", output)
	}
	if !strings.Contains(output, "WARNING") {
		t.Errorf("expected generic WARNING for email change, got: %s", output)
	}
}

func TestValidateConfigNotCorrupted_Clean(t *testing.T) {
	initTestRepo(t)

	// Verify precondition: no local user.name set (initTestRepo uses go-git commit author, not local config)
	snap := snapshotLocalGitConfig()
	if snap.name != "" {
		t.Fatalf("precondition failed: expected no local user.name, got %q", snap.name)
	}

	output := captureStderr(t, func() {
		validateConfigNotCorrupted(context.Background())
	})

	if len(output) > 0 {
		t.Errorf("expected no warning for clean config, got: %s", output)
	}
}

func TestValidateConfigNotCorrupted_Detects456(t *testing.T) {
	dir := initTestRepo(t)

	// Set user.name to the #456 corruption pattern
	setLocalConfig(t, dir, "user.name", "user.email")

	// Verify precondition
	snap := snapshotLocalGitConfig()
	if snap.name != "user.email" {
		t.Fatalf("precondition failed: expected user.name=%q, got %q", "user.email", snap.name)
	}

	output := captureStderr(t, func() {
		validateConfigNotCorrupted(context.Background())
	})

	if len(output) == 0 {
		t.Fatal("expected warning for corrupted config, got nothing")
	}
	if !strings.Contains(output, "WARNING") {
		t.Errorf("expected WARNING for corrupted config, got: %s", output)
	}
	if !strings.Contains(output, "456") {
		t.Errorf("expected #456 reference, got: %s", output)
	}
	if !strings.Contains(output, "git config --local user.name") {
		t.Errorf("expected fix instructions, got: %s", output)
	}
}

func setLocalConfig(t *testing.T, dir, key, value string) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", "config", "--local", key, value)
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to set local config %s=%s: %v", key, value, err)
	}
}
