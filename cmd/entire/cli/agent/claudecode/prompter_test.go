package claudecode

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/entireio/cli/cmd/entire/cli/agent"
)

func TestCLICommand(t *testing.T) {
	t.Parallel()
	ag := &ClaudeCodeAgent{}
	if ag.CLICommand() != "claude" {
		t.Errorf("CLICommand() = %q, want %q", ag.CLICommand(), "claude")
	}
}

func TestPrompt_ValidJSONResponse(t *testing.T) {
	t.Parallel()

	response := `{"result":"Hello from Claude"}`

	ag := &ClaudeCodeAgent{
		CommandRunner: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "sh", "-c", "printf '%s' '"+response+"'")
		},
	}

	result, err := ag.Prompt(context.Background(), "Say hello", agent.PromptOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "Hello from Claude" {
		t.Errorf("Text = %q, want %q", result.Text, "Hello from Claude")
	}
}

func TestPrompt_TextOutputFormat(t *testing.T) {
	t.Parallel()

	ag := &ClaudeCodeAgent{
		CommandRunner: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "sh", "-c", "printf 'plain text response'")
		},
	}

	result, err := ag.Prompt(context.Background(), "Say hello", agent.PromptOptions{
		OutputFormat: "text",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "plain text response" {
		t.Errorf("Text = %q, want %q", result.Text, "plain text response")
	}
}

func TestPrompt_CommandArgs(t *testing.T) {
	t.Parallel()

	var capturedArgs []string

	ag := &ClaudeCodeAgent{
		CommandRunner: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			capturedArgs = append([]string{name}, args...)
			return exec.CommandContext(ctx, "sh", "-c", `printf '{"result":"ok"}'`)
		},
	}

	_, err := ag.Prompt(context.Background(), "test prompt", agent.PromptOptions{
		Model:          "opus",
		AllowedTools:   "Read,Glob,Grep",
		PermissionMode: "bypassPermissions",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	argsStr := strings.Join(capturedArgs, " ")
	for _, expected := range []string{
		"claude",
		"--print",
		"--setting-sources",
		"--output-format json",
		"--model opus",
		"--allowedTools Read,Glob,Grep",
		"--permission-mode bypassPermissions",
	} {
		if !strings.Contains(argsStr, expected) {
			t.Errorf("expected args to contain %q, got: %s", expected, argsStr)
		}
	}
}

func TestPrompt_WorkDir_Custom(t *testing.T) {
	t.Parallel()

	var capturedCmd *exec.Cmd
	ag := &ClaudeCodeAgent{
		CommandRunner: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
			cmd := exec.CommandContext(ctx, "sh", "-c", `printf '{"result":"ok"}'`)
			capturedCmd = cmd
			return cmd
		},
	}

	customDir := t.TempDir()
	_, err := ag.Prompt(context.Background(), "test", agent.PromptOptions{
		WorkDir: customDir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedCmd.Dir != customDir {
		t.Errorf("Dir = %q, want %q", capturedCmd.Dir, customDir)
	}
}

func TestPrompt_WorkDir_Default(t *testing.T) {
	t.Parallel()

	var capturedCmd *exec.Cmd
	ag := &ClaudeCodeAgent{
		CommandRunner: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
			cmd := exec.CommandContext(ctx, "sh", "-c", `printf '{"result":"ok"}'`)
			capturedCmd = cmd
			return cmd
		},
	}

	_, err := ag.Prompt(context.Background(), "test", agent.PromptOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedCmd.Dir != os.TempDir() {
		t.Errorf("Dir = %q, want %q", capturedCmd.Dir, os.TempDir())
	}
}

func TestPrompt_GitIsolation(t *testing.T) {
	var capturedCmd *exec.Cmd

	ag := &ClaudeCodeAgent{
		CommandRunner: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
			cmd := exec.CommandContext(ctx, "sh", "-c", `printf '{"result":"ok"}'`)
			capturedCmd = cmd
			return cmd
		},
	}

	// Set GIT_* vars that would normally be inherited from a git hook
	t.Setenv("GIT_DIR", "/some/repo/.git")
	t.Setenv("GIT_WORK_TREE", "/some/repo")
	t.Setenv("CLAUDECODE", "1")

	_, err := ag.Prompt(context.Background(), "test", agent.PromptOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, env := range capturedCmd.Env {
		if strings.HasPrefix(env, "GIT_") {
			t.Errorf("found GIT_* env var in subprocess: %s", env)
		}
		if strings.HasPrefix(env, "CLAUDECODE=") {
			t.Errorf("found CLAUDECODE env var in subprocess: %s", env)
		}
	}
}

func TestPrompt_ExtraEnv(t *testing.T) {
	var capturedCmd *exec.Cmd

	ag := &ClaudeCodeAgent{
		CommandRunner: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
			cmd := exec.CommandContext(ctx, "sh", "-c", `printf '{"result":"ok"}'`)
			capturedCmd = cmd
			return cmd
		},
	}

	t.Setenv("GIT_DIR", "")

	_, err := ag.Prompt(context.Background(), "test", agent.PromptOptions{
		ExtraEnv: []string{"ENTIRE_WINGMAN_APPLY=1"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, env := range capturedCmd.Env {
		if env == "ENTIRE_WINGMAN_APPLY=1" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected ENTIRE_WINGMAN_APPLY=1 in subprocess env")
	}
}

func TestPrompt_CommandNotFound(t *testing.T) {
	t.Parallel()

	ag := &ClaudeCodeAgent{
		CommandRunner: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "nonexistent-command-that-should-not-exist-12345")
		},
	}

	_, err := ag.Prompt(context.Background(), "test", agent.PromptOptions{})
	if err == nil {
		t.Fatal("expected error when command not found")
	}
	if !strings.Contains(err.Error(), "not found") && !strings.Contains(err.Error(), "executable file not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestPrompt_NonZeroExit(t *testing.T) {
	t.Parallel()

	ag := &ClaudeCodeAgent{
		CommandRunner: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "sh", "-c", "echo 'error message' >&2; exit 1")
		},
	}

	_, err := ag.Prompt(context.Background(), "test", agent.PromptOptions{})
	if err == nil {
		t.Fatal("expected error on non-zero exit")
	}
	if !strings.Contains(err.Error(), "exit 1") {
		t.Errorf("expected exit code in error, got: %v", err)
	}
}

func TestPrompt_InvalidJSONResponse(t *testing.T) {
	t.Parallel()

	ag := &ClaudeCodeAgent{
		CommandRunner: func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "sh", "-c", "printf 'not valid json'")
		},
	}

	_, err := ag.Prompt(context.Background(), "test", agent.PromptOptions{})
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "failed to parse claude CLI response") {
		t.Errorf("expected parse error, got: %v", err)
	}
}
