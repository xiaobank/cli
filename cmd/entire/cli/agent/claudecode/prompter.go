package claudecode

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/entireio/cli/cmd/entire/cli/agent"
)

// Compile-time assertion that ClaudeCodeAgent implements agent.Prompter.
var _ agent.Prompter = (*ClaudeCodeAgent)(nil)

// CLICommand returns the CLI executable name for Claude Code.
func (c *ClaudeCodeAgent) CLICommand() string {
	return "claude"
}

// Prompt sends a prompt to the Claude CLI and returns the text response.
func (c *ClaudeCodeAgent) Prompt(ctx context.Context, prompt string, opts agent.PromptOptions) (*agent.PromptResult, error) {
	args := []string{"--print", "--setting-sources", ""}

	outputFormat := opts.OutputFormat
	if outputFormat == "" {
		outputFormat = "json"
	}
	args = append(args, "--output-format", outputFormat)

	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.AllowedTools != "" {
		args = append(args, "--allowedTools", opts.AllowedTools)
	}
	if opts.PermissionMode != "" {
		args = append(args, "--permission-mode", opts.PermissionMode)
	}

	runner := c.CommandRunner
	if runner == nil {
		runner = exec.CommandContext
	}

	cmd := runner(ctx, c.CLICommand(), args...)

	// Working directory
	if opts.WorkDir != "" {
		cmd.Dir = opts.WorkDir
	} else {
		cmd.Dir = os.TempDir()
	}

	// Environment isolation
	isolate := true
	if opts.IsolateFromGit != nil {
		isolate = *opts.IsolateFromGit
	}
	if isolate {
		cmd.Env = agent.StripGitEnv(os.Environ())
	} else {
		cmd.Env = os.Environ()
	}
	cmd.Env = append(cmd.Env, opts.ExtraEnv...)

	cmd.Stdin = strings.NewReader(prompt)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("claude prompt failed: %w", agent.FormatExecError(err, "claude", stderr.String()))
	}

	// Parse response based on output format
	if outputFormat == "json" {
		var cliResp agent.CLIResponse
		if err := json.Unmarshal(stdout.Bytes(), &cliResp); err != nil {
			return nil, fmt.Errorf("failed to parse claude CLI response: %w", err)
		}
		return &agent.PromptResult{Text: cliResp.Result}, nil
	}

	return &agent.PromptResult{Text: stdout.String()}, nil
}
