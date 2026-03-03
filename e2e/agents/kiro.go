//go:build e2e

package agents

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func init() {
	if env := os.Getenv("E2E_AGENT"); env != "" && env != "kiro" {
		return
	}
	if _, err := exec.LookPath("kiro-cli"); err != nil {
		return
	}
	Register(&Kiro{})
	// Kiro uses Amazon Q API which may have rate limits
	RegisterGate("kiro", 1)
}

// Kiro implements the Agent interface for Amazon's Kiro CLI.
type Kiro struct{}

func (k *Kiro) Name() string               { return "kiro" }
func (k *Kiro) Binary() string             { return "kiro-cli" }
func (k *Kiro) EntireAgent() string        { return "kiro" }
func (k *Kiro) PromptPattern() string      { return `!>` }
func (k *Kiro) TimeoutMultiplier() float64 { return 1.5 }

func (k *Kiro) IsTransientError(out Output, _ error) bool {
	combined := out.Stdout + out.Stderr
	for _, p := range []string{"overloaded", "rate limit", "503", "529", "throttl"} {
		if strings.Contains(strings.ToLower(combined), p) {
			return true
		}
	}
	return false
}

func (k *Kiro) Bootstrap() error {
	// kiro-cli uses Amazon Q / Builder ID auth.
	// On CI, ensure the user is logged in; locally, auth is handled by the desktop app.
	if os.Getenv("CI") == "" {
		return nil
	}
	// Verify login status — fail fast if not authenticated.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "kiro-cli", "whoami")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("kiro-cli auth check failed (run `kiro-cli login`): %s", out)
	}
	return nil
}

func (k *Kiro) RunPrompt(ctx context.Context, dir string, prompt string, opts ...Option) (Output, error) {
	cfg := &runConfig{}
	for _, o := range opts {
		o(cfg)
	}

	// kiro-cli --no-interactive mode does not fire agent hooks, so we must
	// use interactive (tmux) mode and send the prompt through the TUI.
	timeout := 2 * time.Minute
	if cfg.PromptTimeout > 0 {
		timeout = cfg.PromptTimeout
	}

	// --agent entire activates the agent profile that contains our hooks
	args := []string{"chat", "-a", "--agent", "entire"}
	if cfg.Model != "" {
		args = append(args, "--model", cfg.Model)
	}

	name := fmt.Sprintf("kiro-run-%d", time.Now().UnixNano())
	s, err := NewTmuxSession(name, dir, []string{"ENTIRE_TEST_TTY"}, k.Binary(), args...)
	if err != nil {
		return Output{}, fmt.Errorf("starting kiro session: %w", err)
	}
	defer func() { _ = s.Close() }()

	// Wait for initial prompt — kiro-cli TUI shows "!>" in trust-all mode
	if _, err := s.WaitFor(k.PromptPattern(), 30*time.Second); err != nil {
		content := s.Capture()
		return Output{
			Command: k.Binary() + " " + strings.Join(args, " "),
			Stderr:  content,
		}, fmt.Errorf("waiting for kiro startup: %w", err)
	}
	s.stableAtSend = ""

	// Send the prompt
	if err := s.Send(prompt); err != nil {
		return Output{}, fmt.Errorf("sending prompt to kiro: %w", err)
	}

	// Wait for "Credits:" which only appears after the agent finishes a response.
	// More reliable than PromptPattern for single-prompt mode since it uniquely
	// identifies response completion without needing to distinguish echoed input.
	content, err := s.WaitFor(`Credits:`, timeout)
	exitCode := 0
	if err != nil {
		exitCode = -1
	}

	return Output{
		Command:  k.Binary() + " " + strings.Join(args, " ") + " " + fmt.Sprintf("%q", prompt),
		Stdout:   content,
		ExitCode: exitCode,
	}, err
}

func (k *Kiro) StartSession(ctx context.Context, dir string) (Session, error) {
	name := fmt.Sprintf("kiro-test-%d", time.Now().UnixNano())
	s, err := NewTmuxSession(name, dir, []string{"ENTIRE_TEST_TTY"}, k.Binary(), "chat", "-a", "--agent", "entire")
	if err != nil {
		return nil, err
	}

	// Wait for the prompt indicator — kiro-cli TUI shows "!>" in trust-all mode
	if _, err := s.WaitFor(k.PromptPattern(), 15*time.Second); err != nil {
		_ = s.Close()
		return nil, fmt.Errorf("waiting for kiro startup prompt: %w", err)
	}
	s.stableAtSend = ""

	return &KiroSession{TmuxSession: s}, nil
}

// KiroSession wraps TmuxSession for Kiro's interactive sessions.
// After Send, WaitFor uses an end-of-line pattern to avoid matching
// the echoed "!>" prompt in the input line.
type KiroSession struct {
	*TmuxSession
}

func (s *KiroSession) WaitFor(pattern string, timeout time.Duration) (string, error) {
	// For initial waits (before any Send), use the original pattern ("!>").
	if s.stableAtSend == "" {
		return s.TmuxSession.WaitFor(pattern, timeout)
	}
	// After a Send, the echoed input line contains "!>" with text after it
	// (e.g., "[entire] 3% !> now commit it"). The real prompt has "!>" at
	// end-of-line (e.g., "[entire] 3% !>"). Match only the latter.
	return s.TmuxSession.WaitFor(`(?m)!>\s*$`, timeout)
}
