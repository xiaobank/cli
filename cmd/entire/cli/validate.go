package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/entireio/cli/cmd/entire/cli/paths"
	"github.com/spf13/cobra"
)

// ProofConfig defines a single proof (validation check) to run.
type ProofConfig struct {
	// Type is the proof identifier (e.g., "test", "lint", "typecheck")
	Type string `json:"type"`
	// Command is the shell command to execute
	Command string `json:"command"`
	// Required indicates if this proof must pass for validation to succeed
	Required bool `json:"required"`
}

// ValidationConfig holds the validation configuration from settings.
type ValidationConfig struct {
	// Enabled controls whether validation runs automatically
	Enabled bool `json:"enabled"`
	// Proofs is the list of proof checks to run
	Proofs []ProofConfig `json:"proofs"`
}

// ProofResult holds the result of running a single proof.
type ProofResult struct {
	Type     string        `json:"type"`
	Command  string        `json:"command"`
	Passed   bool          `json:"passed"`
	Required bool          `json:"required"`
	Output   string        `json:"output,omitempty"`
	Error    string        `json:"error,omitempty"`
	Duration time.Duration `json:"duration"`
}

// ValidationResult holds the complete validation results.
type ValidationResult struct {
	Passed    bool          `json:"passed"`
	Proofs    []ProofResult `json:"proofs"`
	Duration  time.Duration `json:"duration"`
	Timestamp time.Time     `json:"timestamp"`
}

// DefaultValidationConfig returns the default validation configuration.
func DefaultValidationConfig() *ValidationConfig {
	return &ValidationConfig{
		Enabled: false,
		Proofs: []ProofConfig{
			{Type: "test", Command: "mise run test", Required: true},
			{Type: "lint", Command: "mise run lint", Required: true},
		},
	}
}

func newValidateCmd() *cobra.Command {
	var (
		jsonOutput bool
		proofTypes []string
	)

	cmd := &cobra.Command{
		Use:    "validate",
		Short:  "Run validation proofs on the codebase",
		Hidden: true, // Hidden experimental command
		Long: `Run configured validation proofs to verify code quality.

This command runs the validation proofs configured in .entire/settings.json
under the "validation" key. Each proof is a command that must exit with
status 0 to pass.

Example configuration in .entire/settings.json:
  {
    "validation": {
      "enabled": true,
      "proofs": [
        {"type": "test", "command": "mise run test", "required": true},
        {"type": "lint", "command": "mise run lint", "required": true},
        {"type": "typecheck", "command": "go build ./...", "required": true}
      ]
    }
  }

This is an experimental feature for autonomous validation workflows.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runValidate(cmd.OutOrStdout(), cmd.ErrOrStderr(), jsonOutput, proofTypes)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output results as JSON")
	cmd.Flags().StringSliceVar(&proofTypes, "proof", nil, "Run only specific proof types (can be repeated)")

	return cmd
}

func runValidate(stdout, stderr io.Writer, jsonOutput bool, filterProofs []string) error {
	// Check if we're in a git repository
	repoRoot, err := paths.RepoRoot()
	if err != nil {
		return fmt.Errorf("not in a git repository: %w", err)
	}

	// Load validation config
	config, err := loadValidationConfig()
	if err != nil {
		return fmt.Errorf("failed to load validation config: %w", err)
	}

	if len(config.Proofs) == 0 {
		if jsonOutput {
			result := &ValidationResult{
				Passed:    true,
				Proofs:    []ProofResult{},
				Timestamp: time.Now(),
			}
			return outputJSON(stdout, result)
		}
		fmt.Fprintln(stdout, "No validation proofs configured.")
		fmt.Fprintln(stdout, "\nAdd proofs to .entire/settings.json:")
		fmt.Fprintln(stdout, `  "validation": {`)
		fmt.Fprintln(stdout, `    "enabled": true,`)
		fmt.Fprintln(stdout, `    "proofs": [`)
		fmt.Fprintln(stdout, `      {"type": "test", "command": "mise run test", "required": true}`)
		fmt.Fprintln(stdout, `    ]`)
		fmt.Fprintln(stdout, `  }`)
		return nil
	}

	// Filter proofs if specific types requested
	proofsToRun := config.Proofs
	if len(filterProofs) > 0 {
		proofsToRun = filterProofsByType(config.Proofs, filterProofs)
		if len(proofsToRun) == 0 {
			return fmt.Errorf("no matching proofs found for types: %v", filterProofs)
		}
	}

	// Run validation
	result := runProofs(stdout, stderr, repoRoot, proofsToRun, jsonOutput)

	if jsonOutput {
		return outputJSON(stdout, result)
	}

	// Print summary
	printValidationSummary(stdout, result)

	if !result.Passed {
		return NewSilentError(errors.New("validation failed"))
	}
	return nil
}

func loadValidationConfig() (*ValidationConfig, error) {
	settings, err := LoadEntireSettings()
	if err != nil {
		return DefaultValidationConfig(), nil //nolint:nilerr // use defaults on error
	}

	// Check if validation is configured in strategy_options
	if settings.StrategyOptions == nil {
		return DefaultValidationConfig(), nil
	}

	validationRaw, ok := settings.StrategyOptions["validation"]
	if !ok {
		return DefaultValidationConfig(), nil
	}

	// Convert to JSON and back to parse into ValidationConfig
	data, err := json.Marshal(validationRaw)
	if err != nil {
		return nil, fmt.Errorf("marshaling validation config: %w", err)
	}

	var config ValidationConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing validation config: %w", err)
	}

	return &config, nil
}

func filterProofsByType(proofs []ProofConfig, types []string) []ProofConfig {
	typeSet := make(map[string]bool)
	for _, t := range types {
		typeSet[strings.ToLower(t)] = true
	}

	var filtered []ProofConfig
	for _, p := range proofs {
		if typeSet[strings.ToLower(p.Type)] {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

func runProofs(stdout, stderr io.Writer, repoRoot string, proofs []ProofConfig, quiet bool) *ValidationResult {
	start := time.Now()
	result := &ValidationResult{
		Passed:    true,
		Proofs:    make([]ProofResult, 0, len(proofs)),
		Timestamp: start,
	}

	for i, proof := range proofs {
		if !quiet {
			fmt.Fprintf(stdout, "\n[%d/%d] Running %s: %s\n", i+1, len(proofs), proof.Type, proof.Command)
		}

		proofResult := runSingleProof(repoRoot, proof)
		result.Proofs = append(result.Proofs, proofResult)

		if !quiet {
			if proofResult.Passed {
				fmt.Fprintf(stdout, "  ✓ PASSED (%s)\n", proofResult.Duration.Round(time.Millisecond))
			} else {
				fmt.Fprintf(stdout, "  ✗ FAILED (%s)\n", proofResult.Duration.Round(time.Millisecond))
				if proofResult.Output != "" {
					// Print last 20 lines of output
					lines := strings.Split(strings.TrimSpace(proofResult.Output), "\n")
					if len(lines) > 20 {
						fmt.Fprintf(stderr, "  ... (%d lines truncated)\n", len(lines)-20)
						lines = lines[len(lines)-20:]
					}
					for _, line := range lines {
						fmt.Fprintf(stderr, "  │ %s\n", line)
					}
				}
				if proofResult.Error != "" {
					fmt.Fprintf(stderr, "  Error: %s\n", proofResult.Error)
				}
			}
		}

		// Update overall pass status
		if !proofResult.Passed && proof.Required {
			result.Passed = false
		}
	}

	result.Duration = time.Since(start)
	return result
}

func runSingleProof(repoRoot string, proof ProofConfig) ProofResult {
	start := time.Now()
	result := ProofResult{
		Type:     proof.Type,
		Command:  proof.Command,
		Required: proof.Required,
	}

	// Create command with shell
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", proof.Command) //nolint:gosec // command is user-configured in settings.json
	cmd.Dir = repoRoot

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result.Duration = time.Since(start)

	// Combine stdout and stderr for output
	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += stderr.String()
	}
	result.Output = strings.TrimSpace(output)

	if err != nil {
		result.Passed = false
		if ctx.Err() == context.DeadlineExceeded {
			result.Error = "command timed out after 10 minutes"
		} else {
			result.Error = err.Error()
		}
	} else {
		result.Passed = true
	}

	return result
}

func printValidationSummary(w io.Writer, result *ValidationResult) {
	fmt.Fprintln(w, "\n"+strings.Repeat("─", 50))
	fmt.Fprintln(w, "Validation Summary")
	fmt.Fprintln(w, strings.Repeat("─", 50))

	passed := 0
	failed := 0
	for _, p := range result.Proofs {
		status := "✓"
		if !p.Passed {
			status = "✗"
			failed++
		} else {
			passed++
		}
		reqLabel := ""
		if !p.Required {
			reqLabel = " (optional)"
		}
		fmt.Fprintf(w, "  %s %s%s\n", status, p.Type, reqLabel)
	}

	fmt.Fprintln(w, strings.Repeat("─", 50))
	fmt.Fprintf(w, "Total: %d passed, %d failed (%.1fs)\n", passed, failed, result.Duration.Seconds())

	if result.Passed {
		fmt.Fprintln(w, "\n✓ All required proofs passed")
	} else {
		fmt.Fprintln(w, "\n✗ Validation failed - required proofs did not pass")
	}
}

func outputJSON(w io.Writer, result *ValidationResult) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		return fmt.Errorf("encoding validation result: %w", err)
	}
	return nil
}
