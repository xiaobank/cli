package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/entireio/cli/cmd/entire/cli/versioncheck"
)

func newUpdateCmd() *cobra.Command {
	var checkOnly, skipPrompt bool

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update Entire CLI to the latest version",
		Long: `Run the installer for the current install method (brew, scoop, or install.sh).

The installer is resolved from ~/.config/entire/install.json, with a fallback
to the executable path when provenance is missing.

Flags:
  --check-only  Print the installer command without running it.
  --yes         Skip the confirmation prompt.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if _, err := versioncheck.RunUpdateNow(cmd.Context(), cmd.OutOrStdout(), checkOnly, skipPrompt); err != nil {
				return fmt.Errorf("update: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&checkOnly, "check-only", false, "Print the installer command without running it")
	cmd.Flags().BoolVar(&skipPrompt, "yes", false, "Skip the confirmation prompt")
	return cmd
}
