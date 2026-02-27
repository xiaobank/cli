package agents

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type kiroAgent struct {
	timeout time.Duration
}

func init() {
	if env := os.Getenv("E2E_AGENT"); env != "" && env != "kiro" {
		return
	}
	if _, err := exec.LookPath("kiro-cli"); err != nil {
		return
	}
	Register(&kiroAgent{timeout: 2 * time.Minute})
}

func (a *kiroAgent) Name() string               { return "kiro" }
func (a *kiroAgent) Binary() string             { return "kiro-cli" }
func (a *kiroAgent) EntireAgent() string        { return "kiro" }
func (a *kiroAgent) PromptPattern() string      { return `(>|kiro)` }
func (a *kiroAgent) TimeoutMultiplier() float64 { return 1.5 }

func (a *kiroAgent) IsTransientError(out Output, _ error) bool {
	transientPatterns := []string{
		"overloaded",
		"rate limit",
		"529",
		"503",
		"ECONNRESET",
		"ETIMEDOUT",
		"throttling",
	}
	for _, p := range transientPatterns {
		if strings.Contains(out.Stderr, p) {
			return true
		}
	}
	return false
}

func (a *kiroAgent) Bootstrap() error {
	// No-op for now — add warmup once kiro-cli's startup behavior is characterized.
	return nil
}

func (a *kiroAgent) RunPrompt(ctx context.Context, dir string, prompt string, opts ...Option) (Output, error) {
	cfg := &runConfig{}
	for _, o := range opts {
		o(cfg)
	}

	args := []string{"chat", "--prompt", prompt}

	timeout := a.timeout
	if envTimeout := os.Getenv("E2E_TIMEOUT"); envTimeout != "" {
		if parsed, err := time.ParseDuration(envTimeout); err == nil {
			timeout = parsed
		}
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, a.Binary(), args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "ENTIRE_TEST_TTY=0")

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	out := Output{
		Command: a.Binary() + " " + strings.Join(args, " "),
		Stdout:  stdout.String(),
		Stderr:  stderr.String(),
	}

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			out.ExitCode = exitErr.ExitCode()
		} else {
			out.ExitCode = -1
		}
		return out, err
	}

	return out, nil
}

func (a *kiroAgent) StartSession(ctx context.Context, dir string) (Session, error) {
	name := fmt.Sprintf("kiro-test-%d", time.Now().UnixNano())
	s, err := NewTmuxSession(name, dir, nil, "env", "ENTIRE_TEST_TTY=0", a.Binary())
	if err != nil {
		return nil, err
	}

	// Wait for Kiro TUI to be ready
	if _, err := s.WaitFor(a.PromptPattern(), 15*time.Second); err != nil {
		_ = s.Close()
		return nil, fmt.Errorf("waiting for kiro startup: %w", err)
	}
	s.stableAtSend = ""
	return s, nil
}
