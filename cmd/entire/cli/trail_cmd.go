package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/entireio/cli/cmd/entire/cli/jsonutil"
	"github.com/entireio/cli/cmd/entire/cli/trail"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

func newTrailCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "trail",
		Short:  "Manage trails (task queue)",
		Hidden: true, // Experimental feature
		Long: `Manage trails - a git-native task queue for autonomous agent execution.

Trails are task definitions stored in the entire/trails orphan branch.
The Trail Runner discovers open trails, claims them atomically, creates worktrees,
and runs agents to complete the work.

Example workflow:
  entire trail create "Implement user authentication"
  entire trail list
  entire trail-runner run-once`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(newTrailListCmd())
	cmd.AddCommand(newTrailShowCmd())
	cmd.AddCommand(newTrailCreateCmd())
	cmd.AddCommand(newTrailDeleteCmd())
	cmd.AddCommand(newTrailResetCmd())

	return cmd
}

func newTrailListCmd() *cobra.Command {
	var (
		statusFilter string
		jsonOutput   bool
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List trails",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTrailList(cmd, statusFilter, jsonOutput)
		},
	}

	cmd.Flags().StringVar(&statusFilter, "status", "", "Filter by status: open, in_progress, completed, failed")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON")

	return cmd
}

func newTrailShowCmd() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "show <trail-id>",
		Short: "Show trail details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTrailShow(cmd, args[0], jsonOutput)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON")

	return cmd
}

func newTrailCreateCmd() *cobra.Command {
	var (
		branch     string
		baseBranch string
		labels     []string
	)

	cmd := &cobra.Command{
		Use:   "create <title>",
		Short: "Create a new trail",
		Long: `Create a new trail (task) for the trail runner to execute.

If no title is provided, an interactive form will be shown.

Examples:
  entire trail create "Implement login form"
  entire trail create "Fix bug #123" --branch fix/bug-123
  entire trail create "Add tests" --base main --labels urgent,tests`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var title string
			if len(args) > 0 {
				title = args[0]
			}
			return runTrailCreate(cmd, title, branch, baseBranch, labels)
		},
	}

	cmd.Flags().StringVar(&branch, "branch", "", "Branch name for this trail (default: trail/<id>)")
	cmd.Flags().StringVar(&baseBranch, "base", "", "Base branch to create from (default: main)")
	cmd.Flags().StringSliceVar(&labels, "labels", nil, "Labels for categorization")

	return cmd
}

func newTrailDeleteCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "delete <trail-id>",
		Short: "Delete a trail",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTrailDelete(cmd, args[0], force)
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Delete without confirmation")

	return cmd
}

func newTrailResetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reset <trail-id>",
		Short: "Reset a trail's execution state (return to open)",
		Long: `Reset a trail's execution state, allowing it to be re-executed.

This removes all state refs (claimed, completed, failed) for the trail,
returning it to the 'open' state. The trail definition is not modified.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTrailReset(cmd, args[0])
		},
	}

	return cmd
}

func runTrailList(cmd *cobra.Command, statusFilter string, jsonOutput bool) error {
	repo, err := openRepository()
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	discovery := trail.NewDiscovery(repo)
	ctx := context.Background()

	// Build filter
	var filter *trail.ListFilter
	if statusFilter != "" {
		state, err := parseTrailState(statusFilter)
		if err != nil {
			return err
		}
		filter = &trail.ListFilter{States: []trail.TrailState{state}}
	}

	trails, err := discovery.ListWithState(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to list trails: %w", err)
	}

	if jsonOutput {
		data, err := jsonutil.MarshalIndentWithNewline(trails, "", "  ")
		if err != nil {
			return err //nolint:wrapcheck // simple pass-through
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return nil
	}

	if len(trails) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No trails found.")
		fmt.Fprintln(cmd.OutOrStdout(), "\nCreate a trail with: entire trail create \"Title\"")
		return nil
	}

	// Print table
	fmt.Fprintln(cmd.OutOrStdout(), "ID            STATUS        BRANCH                    TITLE")
	fmt.Fprintln(cmd.OutOrStdout(), strings.Repeat("-", 85))

	for _, t := range trails {
		status := formatTrailState(t.State)
		branch := t.GetBranch()
		if len(branch) > 24 {
			branch = branch[:21] + "..."
		}
		title := t.Title
		if len(title) > 30 {
			title = title[:27] + "..."
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%-13s %-13s %-25s %s\n",
			t.ID, status, branch, title)
	}

	return nil
}

func runTrailShow(cmd *cobra.Command, idStr string, jsonOutput bool) error {
	id, err := trail.NewTrailID(idStr)
	if err != nil {
		return err //nolint:wrapcheck // validation error
	}

	repo, err := openRepository()
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	discovery := trail.NewDiscovery(repo)
	ctx := context.Background()

	t, err := discovery.GetWithState(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get trail: %w", err)
	}
	if t == nil {
		return fmt.Errorf("trail not found: %s", id)
	}

	if jsonOutput {
		data, err := jsonutil.MarshalIndentWithNewline(t, "", "  ")
		if err != nil {
			return err //nolint:wrapcheck // simple pass-through
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return nil
	}

	// Print details
	fmt.Fprintf(cmd.OutOrStdout(), "Trail: %s\n", t.ID)
	fmt.Fprintf(cmd.OutOrStdout(), "Title: %s\n", t.Title)
	fmt.Fprintf(cmd.OutOrStdout(), "State: %s\n", formatTrailState(t.State))
	fmt.Fprintf(cmd.OutOrStdout(), "Branch: %s\n", t.GetBranch())
	if t.BaseBranch != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Base Branch: %s\n", t.BaseBranch)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Created: %s\n", t.CreatedAt.Format(time.RFC3339))
	fmt.Fprintf(cmd.OutOrStdout(), "Updated: %s\n", t.UpdatedAt.Format(time.RFC3339))

	if len(t.Labels) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "Labels: %s\n", strings.Join(t.Labels, ", "))
	}
	if len(t.Assignees) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "Assignees: %s\n", strings.Join(t.Assignees, ", "))
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\nDescription:\n%s\n", t.Description)

	return nil
}

func runTrailCreate(cmd *cobra.Command, title, branch, baseBranch string, labels []string) error {
	var description string

	// Interactive mode if no title provided
	if title == "" {
		var err error
		title, description, branch, baseBranch, err = runTrailCreateInteractive()
		if err != nil {
			return err
		}
	} else {
		// Get description interactively
		form := NewAccessibleForm(
			huh.NewGroup(
				huh.NewText().
					Title("Description").
					Description("Describe the task for the agent").
					Value(&description).
					CharLimit(4000),
			),
		)
		if err := form.Run(); err != nil {
			return fmt.Errorf("cancelled: %w", err)
		}
	}

	if description == "" {
		return errors.New("description is required")
	}

	// Generate trail ID
	id, err := trail.GenerateTrailID()
	if err != nil {
		return fmt.Errorf("failed to generate trail ID: %w", err)
	}

	// Create trail
	t := &trail.Trail{
		ID:          id,
		Title:       title,
		Description: description,
		Branch:      branch,
		BaseBranch:  baseBranch,
		Labels:      labels,
	}

	repo, err := openRepository()
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	store := trail.NewStore(repo)
	ctx := context.Background()

	if err := store.Create(ctx, t); err != nil {
		return fmt.Errorf("failed to create trail: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Created trail: %s\n", id)
	fmt.Fprintf(cmd.OutOrStdout(), "Branch: %s\n", t.GetBranch())
	fmt.Fprintf(cmd.OutOrStdout(), "\nRun the trail with: entire trail run %s\n", id)

	return nil
}

func runTrailCreateInteractive() (title, description, branch, baseBranch string, err error) {
	form := NewAccessibleForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Title").
				Description("Brief title for the task").
				Value(&title),
			huh.NewText().
				Title("Description").
				Description("Describe the task for the agent").
				Value(&description).
				CharLimit(4000),
			huh.NewInput().
				Title("Branch").
				Description("Branch name (leave empty for default)").
				Value(&branch),
			huh.NewInput().
				Title("Base Branch").
				Description("Base branch to create from (leave empty for main)").
				Value(&baseBranch),
		),
	)

	if err := form.Run(); err != nil {
		return "", "", "", "", fmt.Errorf("cancelled: %w", err)
	}

	if title == "" {
		return "", "", "", "", errors.New("title is required")
	}

	return title, description, branch, baseBranch, nil
}

func runTrailDelete(cmd *cobra.Command, idStr string, force bool) error {
	id, err := trail.NewTrailID(idStr)
	if err != nil {
		return err //nolint:wrapcheck // validation error
	}

	repo, err := openRepository()
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	store := trail.NewStore(repo)
	ctx := context.Background()

	// Check if trail exists
	t, err := store.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get trail: %w", err)
	}
	if t == nil {
		return fmt.Errorf("trail not found: %s", id)
	}

	// Confirm deletion
	if !force {
		var confirm bool
		form := NewAccessibleForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title(fmt.Sprintf("Delete trail %s?", id)).
					Description(t.Title).
					Value(&confirm),
			),
		)
		if err := form.Run(); err != nil {
			return fmt.Errorf("cancelled: %w", err)
		}
		if !confirm {
			fmt.Fprintln(cmd.OutOrStdout(), "Cancelled.")
			return nil
		}
	}

	// Also reset state refs
	state := trail.NewStateManager(repo)
	if err := state.Reset(ctx, id); err != nil {
		// Non-fatal, continue with deletion
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to reset state refs: %v\n", err)
	}

	if err := store.Delete(ctx, id); err != nil {
		return fmt.Errorf("failed to delete trail: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Deleted trail: %s\n", id)
	return nil
}

func runTrailReset(cmd *cobra.Command, idStr string) error {
	id, err := trail.NewTrailID(idStr)
	if err != nil {
		return err //nolint:wrapcheck // validation error
	}

	repo, err := openRepository()
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	state := trail.NewStateManager(repo)
	ctx := context.Background()

	if err := state.Reset(ctx, id); err != nil {
		return fmt.Errorf("failed to reset trail state: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Reset trail: %s (now open)\n", id)
	return nil
}

func parseTrailState(s string) (trail.TrailState, error) {
	switch strings.ToLower(s) {
	case "open":
		return trail.TrailStateOpen, nil
	case "in_progress", "inprogress", "claimed":
		return trail.TrailStateInProgress, nil
	case "completed", "done":
		return trail.TrailStateCompleted, nil
	case "failed", "error":
		return trail.TrailStateFailed, nil
	default:
		return "", fmt.Errorf("invalid status: %s (valid: open, in_progress, completed, failed)", s)
	}
}

func formatTrailState(state trail.TrailState) string {
	switch state {
	case trail.TrailStateOpen:
		return "open"
	case trail.TrailStateInProgress:
		return "in_progress"
	case trail.TrailStateCompleted:
		return "completed"
	case trail.TrailStateFailed:
		return "failed"
	default:
		return string(state)
	}
}
