package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/entireio/cli/cmd/entire/cli/paths"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func setupValidateTestRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}

	t.Chdir(dir)
	paths.ClearRepoRootCache()

	// Create initial commit
	emptyTree := &object.Tree{Entries: []object.TreeEntry{}}
	obj := repo.Storer.NewEncodedObject()
	if err := emptyTree.Encode(obj); err != nil {
		t.Fatalf("failed to encode empty tree: %v", err)
	}
	emptyTreeHash, err := repo.Storer.SetEncodedObject(obj)
	if err != nil {
		t.Fatalf("failed to store empty tree: %v", err)
	}

	sig := object.Signature{Name: "test", Email: "test@test.com"}
	commit := &object.Commit{
		TreeHash:  emptyTreeHash,
		Author:    sig,
		Committer: sig,
		Message:   "initial commit",
	}
	commitObj := repo.Storer.NewEncodedObject()
	if err := commit.Encode(commitObj); err != nil {
		t.Fatalf("failed to encode commit: %v", err)
	}
	commitHash, err := repo.Storer.SetEncodedObject(commitObj)
	if err != nil {
		t.Fatalf("failed to store commit: %v", err)
	}

	// Create HEAD and master references
	headRef := plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName("master"))
	if err := repo.Storer.SetReference(headRef); err != nil {
		t.Fatalf("failed to set HEAD: %v", err)
	}
	masterRef := plumbing.NewHashReference(plumbing.NewBranchReferenceName("master"), commitHash)
	if err := repo.Storer.SetReference(masterRef); err != nil {
		t.Fatalf("failed to set master: %v", err)
	}

	return dir
}

func writeValidationSettings(t *testing.T, dir string, config *ValidationConfig) {
	t.Helper()

	settingsDir := filepath.Join(dir, ".entire")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatalf("failed to create .entire dir: %v", err)
	}

	settings := map[string]interface{}{
		"strategy": "manual-commit",
		"enabled":  true,
		"strategy_options": map[string]interface{}{
			"validation": config,
		},
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal settings: %v", err)
	}

	if err := os.WriteFile(filepath.Join(settingsDir, "settings.json"), data, 0o644); err != nil {
		t.Fatalf("failed to write settings: %v", err)
	}
}

func TestRunValidate_NoProofsConfigured(t *testing.T) {
	dir := setupValidateTestRepo(t)

	// Write settings with empty proofs
	writeValidationSettings(t, dir, &ValidationConfig{
		Enabled: true,
		Proofs:  []ProofConfig{},
	})

	var stdout, stderr bytes.Buffer
	err := runValidate(&stdout, &stderr, false, nil)
	if err != nil {
		t.Fatalf("runValidate() error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "No validation proofs configured") {
		t.Errorf("Expected 'No validation proofs configured' message, got: %s", output)
	}
}

func TestRunValidate_PassingProof(t *testing.T) {
	dir := setupValidateTestRepo(t)

	// Configure a proof that always passes
	writeValidationSettings(t, dir, &ValidationConfig{
		Enabled: true,
		Proofs: []ProofConfig{
			{Type: "test", Command: "true", Required: true},
		},
	})

	var stdout, stderr bytes.Buffer
	err := runValidate(&stdout, &stderr, false, nil)
	if err != nil {
		t.Fatalf("runValidate() error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "PASSED") {
		t.Errorf("Expected 'PASSED' in output, got: %s", output)
	}
	if !strings.Contains(output, "All required proofs passed") {
		t.Errorf("Expected 'All required proofs passed' in output, got: %s", output)
	}
}

func TestRunValidate_FailingProof(t *testing.T) {
	dir := setupValidateTestRepo(t)

	// Configure a proof that always fails
	writeValidationSettings(t, dir, &ValidationConfig{
		Enabled: true,
		Proofs: []ProofConfig{
			{Type: "test", Command: "false", Required: true},
		},
	})

	var stdout, stderr bytes.Buffer
	err := runValidate(&stdout, &stderr, false, nil)

	// Should return a SilentError
	if err == nil {
		t.Fatal("runValidate() should return error for failing proof")
	}
	silentError := &SilentError{}
	if !errors.As(err, &silentError) {
		t.Errorf("Expected SilentError, got %T", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "FAILED") {
		t.Errorf("Expected 'FAILED' in output, got: %s", output)
	}
	if !strings.Contains(output, "Validation failed") {
		t.Errorf("Expected 'Validation failed' in output, got: %s", output)
	}
}

func TestRunValidate_OptionalProofFailure(t *testing.T) {
	dir := setupValidateTestRepo(t)

	// Configure an optional proof that fails - should not fail overall
	writeValidationSettings(t, dir, &ValidationConfig{
		Enabled: true,
		Proofs: []ProofConfig{
			{Type: "optional-lint", Command: "false", Required: false},
			{Type: "test", Command: "true", Required: true},
		},
	})

	var stdout, stderr bytes.Buffer
	err := runValidate(&stdout, &stderr, false, nil)

	// Should pass overall because the failing proof is optional
	if err != nil {
		t.Fatalf("runValidate() should pass when only optional proofs fail, got error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "All required proofs passed") {
		t.Errorf("Expected 'All required proofs passed' in output, got: %s", output)
	}
	if !strings.Contains(output, "(optional)") {
		t.Errorf("Expected '(optional)' label in output, got: %s", output)
	}
}

func TestRunValidate_MultipleProofs(t *testing.T) {
	dir := setupValidateTestRepo(t)

	// Configure multiple passing proofs
	writeValidationSettings(t, dir, &ValidationConfig{
		Enabled: true,
		Proofs: []ProofConfig{
			{Type: "test", Command: "true", Required: true},
			{Type: "lint", Command: "true", Required: true},
			{Type: "typecheck", Command: "true", Required: true},
		},
	})

	var stdout, stderr bytes.Buffer
	err := runValidate(&stdout, &stderr, false, nil)
	if err != nil {
		t.Fatalf("runValidate() error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "3 passed") {
		t.Errorf("Expected '3 passed' in output, got: %s", output)
	}
}

func TestRunValidate_JSONOutput(t *testing.T) {
	dir := setupValidateTestRepo(t)

	writeValidationSettings(t, dir, &ValidationConfig{
		Enabled: true,
		Proofs: []ProofConfig{
			{Type: "test", Command: "echo hello", Required: true},
		},
	})

	var stdout, stderr bytes.Buffer
	err := runValidate(&stdout, &stderr, true, nil)
	if err != nil {
		t.Fatalf("runValidate() error = %v", err)
	}

	var result ValidationResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("Failed to parse JSON output: %v", err)
	}

	if !result.Passed {
		t.Error("Expected Passed=true")
	}
	if len(result.Proofs) != 1 {
		t.Errorf("Expected 1 proof, got %d", len(result.Proofs))
	}
	if result.Proofs[0].Type != "test" {
		t.Errorf("Expected proof type 'test', got %q", result.Proofs[0].Type)
	}
	if !result.Proofs[0].Passed {
		t.Error("Expected proof to pass")
	}
	if !strings.Contains(result.Proofs[0].Output, "hello") {
		t.Errorf("Expected output to contain 'hello', got %q", result.Proofs[0].Output)
	}
}

func TestRunValidate_FilterProofs(t *testing.T) {
	dir := setupValidateTestRepo(t)

	writeValidationSettings(t, dir, &ValidationConfig{
		Enabled: true,
		Proofs: []ProofConfig{
			{Type: "test", Command: "true", Required: true},
			{Type: "lint", Command: "true", Required: true},
			{Type: "typecheck", Command: "true", Required: true},
		},
	})

	var stdout, stderr bytes.Buffer
	// Only run the "lint" proof
	err := runValidate(&stdout, &stderr, false, []string{"lint"})
	if err != nil {
		t.Fatalf("runValidate() error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "[1/1]") {
		t.Errorf("Expected '[1/1]' indicating single proof, got: %s", output)
	}
	if !strings.Contains(output, "lint") {
		t.Errorf("Expected 'lint' proof to run, got: %s", output)
	}
}

func TestRunValidate_FilterProofs_NoMatch(t *testing.T) {
	dir := setupValidateTestRepo(t)

	writeValidationSettings(t, dir, &ValidationConfig{
		Enabled: true,
		Proofs: []ProofConfig{
			{Type: "test", Command: "true", Required: true},
		},
	})

	var stdout, stderr bytes.Buffer
	err := runValidate(&stdout, &stderr, false, []string{"nonexistent"})

	if err == nil {
		t.Fatal("runValidate() should return error when no proofs match filter")
	}
	if !strings.Contains(err.Error(), "no matching proofs") {
		t.Errorf("Expected 'no matching proofs' error, got: %v", err)
	}
}

func TestRunValidate_NotGitRepository(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	paths.ClearRepoRootCache()

	var stdout, stderr bytes.Buffer
	err := runValidate(&stdout, &stderr, false, nil)

	if err == nil {
		t.Fatal("runValidate() should return error for non-git directory")
	}
	if !strings.Contains(err.Error(), "not in a git repository") {
		t.Errorf("Expected 'not in a git repository' error, got: %v", err)
	}
}

func TestRunValidate_CommandWithOutput(t *testing.T) {
	dir := setupValidateTestRepo(t)

	// Configure a proof that produces output before failing
	writeValidationSettings(t, dir, &ValidationConfig{
		Enabled: true,
		Proofs: []ProofConfig{
			{Type: "test", Command: "echo 'test output line 1' && echo 'test output line 2' && false", Required: true},
		},
	})

	var stdout, stderr bytes.Buffer
	err := runValidate(&stdout, &stderr, false, nil)

	// Should fail
	if err == nil {
		t.Fatal("runValidate() should return error for failing proof")
	}

	// Should capture the output
	combinedOutput := stdout.String() + stderr.String()
	if !strings.Contains(combinedOutput, "test output line 1") {
		t.Errorf("Expected output to contain test lines, got: %s", combinedOutput)
	}
}

func TestDefaultValidationConfig(t *testing.T) {
	config := DefaultValidationConfig()

	if config.Enabled {
		t.Error("Default config should have Enabled=false")
	}
	if len(config.Proofs) != 2 {
		t.Errorf("Expected 2 default proofs, got %d", len(config.Proofs))
	}

	// Check default proof types
	proofTypes := make(map[string]bool)
	for _, p := range config.Proofs {
		proofTypes[p.Type] = true
	}
	if !proofTypes["test"] {
		t.Error("Expected 'test' in default proofs")
	}
	if !proofTypes["lint"] {
		t.Error("Expected 'lint' in default proofs")
	}
}

func TestFilterProofsByType(t *testing.T) {
	proofs := []ProofConfig{
		{Type: "test", Command: "cmd1"},
		{Type: "lint", Command: "cmd2"},
		{Type: "Typecheck", Command: "cmd3"},
	}

	// Test case-insensitive matching
	filtered := filterProofsByType(proofs, []string{"LINT", "typecheck"})
	if len(filtered) != 2 {
		t.Errorf("Expected 2 filtered proofs, got %d", len(filtered))
	}

	// Test no matches
	filtered = filterProofsByType(proofs, []string{"nonexistent"})
	if len(filtered) != 0 {
		t.Errorf("Expected 0 filtered proofs, got %d", len(filtered))
	}
}

func TestRunSingleProof_Success(t *testing.T) {
	dir := t.TempDir()

	proof := ProofConfig{
		Type:     "test",
		Command:  "echo 'success'",
		Required: true,
	}

	result := runSingleProof(dir, proof)

	if !result.Passed {
		t.Error("Expected proof to pass")
	}
	if result.Error != "" {
		t.Errorf("Expected no error, got: %s", result.Error)
	}
	if !strings.Contains(result.Output, "success") {
		t.Errorf("Expected output to contain 'success', got: %s", result.Output)
	}
	if result.Duration <= 0 {
		t.Error("Expected positive duration")
	}
}

func TestRunSingleProof_Failure(t *testing.T) {
	dir := t.TempDir()

	proof := ProofConfig{
		Type:     "test",
		Command:  "exit 1",
		Required: true,
	}

	result := runSingleProof(dir, proof)

	if result.Passed {
		t.Error("Expected proof to fail")
	}
	if result.Error == "" {
		t.Error("Expected error message")
	}
}

func TestRunSingleProof_WorkingDirectory(t *testing.T) {
	dir := t.TempDir()

	// Create a file in the temp directory
	testFile := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(testFile, []byte("content"), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	proof := ProofConfig{
		Type:     "test",
		Command:  "cat test.txt",
		Required: true,
	}

	result := runSingleProof(dir, proof)

	if !result.Passed {
		t.Errorf("Expected proof to pass, error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "content") {
		t.Errorf("Expected output to contain file content, got: %s", result.Output)
	}
}

func TestNewValidateCmd_Hidden(t *testing.T) {
	cmd := newValidateCmd()

	if !cmd.Hidden {
		t.Error("validate command should be hidden")
	}
	if cmd.Use != "validate" {
		t.Errorf("Expected Use='validate', got %q", cmd.Use)
	}
}

func TestLoadValidationConfig_FromSettings(t *testing.T) {
	dir := setupValidateTestRepo(t)

	writeValidationSettings(t, dir, &ValidationConfig{
		Enabled: true,
		Proofs: []ProofConfig{
			{Type: "custom", Command: "custom-cmd", Required: true},
		},
	})

	config, err := loadValidationConfig()
	if err != nil {
		t.Fatalf("loadValidationConfig() error = %v", err)
	}

	if !config.Enabled {
		t.Error("Expected Enabled=true from settings")
	}
	if len(config.Proofs) != 1 {
		t.Errorf("Expected 1 proof, got %d", len(config.Proofs))
	}
	if config.Proofs[0].Type != "custom" {
		t.Errorf("Expected proof type 'custom', got %q", config.Proofs[0].Type)
	}
}

func TestLoadValidationConfig_NoSettings(t *testing.T) {
	dir := setupValidateTestRepo(t)

	// Create .entire directory but no settings
	if err := os.MkdirAll(filepath.Join(dir, ".entire"), 0o755); err != nil {
		t.Fatalf("failed to create .entire dir: %v", err)
	}

	config, err := loadValidationConfig()
	if err != nil {
		t.Fatalf("loadValidationConfig() error = %v", err)
	}

	// Should return defaults
	if config.Enabled {
		t.Error("Expected Enabled=false (default)")
	}
	if len(config.Proofs) != 2 {
		t.Errorf("Expected 2 default proofs, got %d", len(config.Proofs))
	}
}
