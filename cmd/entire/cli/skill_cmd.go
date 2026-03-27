package cli

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/entireio/cli/cmd/entire/cli/paths"
	"github.com/entireio/cli/cmd/entire/cli/skilltui"
)

func newSkillCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "skill",
		Short: "Skill analytics and improvement dashboard",
		Long:  "Interactive TUI that discovers skill files, tracks their usage from session data, and generates AI-powered improvement suggestions.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()

			if IsAccessibleMode() {
				cmd.SilenceUsage = true
				fmt.Fprintln(cmd.ErrOrStderr(), "The skill dashboard requires an interactive terminal. Set ACCESSIBLE= to disable accessible mode.")
				return NewSilentError(errors.New("skill TUI requires interactive terminal"))
			}

			worktreeRoot, err := paths.WorktreeRoot(ctx)
			if err != nil {
				cmd.SilenceUsage = true
				fmt.Fprintln(cmd.ErrOrStderr(), "Not a git repository. Please run from within a git repository.")
				return NewSilentError(fmt.Errorf("not a git repository: %w", err))
			}

			skillDBPath := filepath.Join(worktreeRoot, paths.EntireDir, "skill-analytics.db")
			insightsDBPath := filepath.Join(worktreeRoot, paths.EntireDir, "insights.db")

			return skilltui.Run(ctx, skillDBPath, insightsDBPath, worktreeRoot)
		},
	}
}
