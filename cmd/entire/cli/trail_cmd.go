package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/entireio/cli/cmd/entire/cli/paths"
	"github.com/entireio/cli/cmd/entire/cli/strategy"
	"github.com/entireio/cli/cmd/entire/cli/stringutil"
	"github.com/entireio/cli/cmd/entire/cli/trail"

	"github.com/charmbracelet/huh"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/spf13/cobra"
)

func newTrailCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "trail",
		Short:  "Manage trails for your branches",
		Hidden: true,
		Long: `Trails are branch-centric work tracking abstractions. They describe the
"why" and "what" of your work, while checkpoints capture the "how" and "when".

Running 'entire trail' without a subcommand shows the trail for the current
branch, or lists all trails if no trail exists for the current branch.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTrailShow(cmd.OutOrStdout())
		},
	}

	cmd.AddCommand(newTrailListCmd())
	cmd.AddCommand(newTrailCreateCmd())
	cmd.AddCommand(newTrailUpdateCmd())
	cmd.AddCommand(newTrailBranchCmd())

	return cmd
}

// runTrailShow shows the trail for the current branch, or falls through to list.
func runTrailShow(w io.Writer) error {
	branch, err := GetCurrentBranch(context.Background())
	if err != nil {
		return runTrailListAll(w, "", false, false)
	}

	repo, err := strategy.OpenRepository(context.Background())
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	store := trail.NewStore(repo)
	metadata, _, err := store.FindByBranch(branch)
	if err != nil || metadata == nil {
		return runTrailListAll(w, "", false, false)
	}

	printTrailDetails(w, metadata)
	return nil
}

func printTrailDetails(w io.Writer, m *trail.Metadata) {
	fmt.Fprintf(w, "Trail: %s\n", m.Title)
	fmt.Fprintf(w, "  ID:      %s\n", m.TrailID)
	fmt.Fprintf(w, "  Status:  %s\n", m.Status)
	fmt.Fprintf(w, "  Author:  %s\n", m.Author)

	if m.Intent != nil {
		fmt.Fprintf(w, "  Intent:  %s (%s)\n", m.Intent.Value, m.Intent.Kind)
	}

	if m.Body != "" {
		fmt.Fprintf(w, "  Body:    %s\n", m.Body)
	}

	// Show branches (new multi-branch format)
	if len(m.Branches) > 0 {
		fmt.Fprintf(w, "  Branches:\n")
		for _, b := range m.Branches {
			statusMarker := " "
			switch b.Status {
			case trail.BranchOpen:
				statusMarker = " "
			case trail.BranchMerged:
				statusMarker = "+"
			case trail.BranchDiscarded:
				statusMarker = "x"
			}
			prInfo := ""
			if b.PR != nil {
				prInfo = fmt.Sprintf(" PR #%d", b.PR.Number)
			}
			fmt.Fprintf(w, "    %s %s -> %s [%s]%s\n", statusMarker, b.Name, b.BaseBranch, b.Status, prInfo)
		}
	} else if m.Branch != "" {
		// Legacy single-branch display
		fmt.Fprintf(w, "  Branch:  %s\n", m.Branch)
		fmt.Fprintf(w, "  Base:    %s\n", m.Base)
	}

	if len(m.Labels) > 0 {
		fmt.Fprintf(w, "  Labels:  %s\n", strings.Join(m.Labels, ", "))
	}
	if len(m.Assignees) > 0 {
		fmt.Fprintf(w, "  Assignees: %s\n", strings.Join(m.Assignees, ", "))
	}
	fmt.Fprintf(w, "  Created: %s\n", m.CreatedAt.Format(time.RFC3339))
	fmt.Fprintf(w, "  Updated: %s\n", m.UpdatedAt.Format(time.RFC3339))
}

func newTrailListCmd() *cobra.Command {
	var statusFilter string
	var jsonOutput bool
	var showAll bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all trails",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTrailListAll(cmd.OutOrStdout(), statusFilter, jsonOutput, showAll)
		},
	}

	cmd.Flags().StringVar(&statusFilter, "status", "", "Filter by status (draft, open, in_progress, in_review, merged, closed)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON")
	cmd.Flags().BoolVarP(&showAll, "all", "a", false, "Include merged and closed trails")

	return cmd
}

func runTrailListAll(w io.Writer, statusFilter string, jsonOutput, showAll bool) error {
	// Fetch remote trails branch so we see trails from collaborators
	fetchTrailsBranch()

	repo, err := strategy.OpenRepository(context.Background())
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	store := trail.NewStore(repo)
	trails, err := store.List()
	if err != nil {
		return fmt.Errorf("failed to list trails: %w", err)
	}

	if trails == nil {
		trails = []*trail.Metadata{}
	}

	totalCount := len(trails)

	// Apply status filter
	if statusFilter != "" {
		status := trail.Status(statusFilter)
		if !status.IsValid() {
			return fmt.Errorf("invalid status %q: valid values are %s", statusFilter, formatValidStatuses())
		}
		var filtered []*trail.Metadata
		for _, t := range trails {
			if t.Status == status {
				filtered = append(filtered, t)
			}
		}
		trails = filtered
	} else if !showAll {
		// By default, hide done and abandoned trails
		var filtered []*trail.Metadata
		for _, t := range trails {
			if t.Status != trail.StatusDone && t.Status != trail.StatusAbandoned {
				filtered = append(filtered, t)
			}
		}
		trails = filtered
	}

	// Sort by updated_at descending
	sort.Slice(trails, func(i, j int) bool {
		return trails[i].UpdatedAt.After(trails[j].UpdatedAt)
	})

	if jsonOutput {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(trails); err != nil {
			return fmt.Errorf("failed to encode JSON: %w", err)
		}
		return nil
	}

	if len(trails) == 0 {
		hiddenCount := totalCount - len(trails)
		if hiddenCount > 0 {
			fmt.Fprintf(w, "No active trails found. %d merged/closed trail(s) hidden — use --all to show.\n", hiddenCount)
		} else {
			fmt.Fprintln(w, "No trails found.")
			fmt.Fprintln(w)
			fmt.Fprintln(w, "Commands:")
			fmt.Fprintln(w, "  entire trail create   Create a trail for the current branch")
			fmt.Fprintln(w, "  entire trail list     List all trails")
			fmt.Fprintln(w, "  entire trail update   Update trail metadata")
		}
		return nil
	}

	// Table output
	fmt.Fprintf(w, "%-30s %-40s %-13s %-15s %s\n", "BRANCH", "TITLE", "STATUS", "AUTHOR", "UPDATED")
	for _, t := range trails {
		branch := stringutil.TruncateRunes(t.ActiveBranchName(), 30, "...")
		title := stringutil.TruncateRunes(t.Title, 40, "...")
		fmt.Fprintf(w, "%-30s %-40s %-13s %-15s %s\n",
			branch, title, t.Status, stringutil.TruncateRunes(t.Author, 15, "..."), timeAgo(t.UpdatedAt))
	}

	return nil
}

func newTrailCreateCmd() *cobra.Command {
	var title, body, base, branch, status string
	var intentFile, intentIssue, intentInline string
	var checkout bool

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a trail for the current or a new branch",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTrailCreate(cmd, title, body, base, branch, status, intentFile, intentIssue, intentInline, checkout)
		},
	}

	cmd.Flags().StringVar(&title, "title", "", "Trail title")
	cmd.Flags().StringVar(&body, "body", "", "Trail body")
	cmd.Flags().StringVar(&base, "base", "", "Base branch (defaults to detected default branch)")
	cmd.Flags().StringVar(&branch, "branch", "", "Branch for the trail (defaults to current branch)")
	cmd.Flags().StringVar(&status, "status", "", "Initial status (defaults to draft)")
	cmd.Flags().BoolVar(&checkout, "checkout", false, "Check out the branch after creating it")
	cmd.Flags().StringVar(&intentFile, "intent-file", "", "Path to spec/intent file")
	cmd.Flags().StringVar(&intentIssue, "intent-issue", "", "Issue ID (e.g., LIN-123)")
	cmd.Flags().StringVar(&intentInline, "intent", "", "Inline intent description")

	return cmd
}

//nolint:cyclop // sequential steps for creating a trail — splitting would obscure the flow
func runTrailCreate(cmd *cobra.Command, title, body, base, branch, statusStr, intentFile, intentIssue, intentInline string, checkout bool) error {
	w := cmd.OutOrStdout()
	errW := cmd.ErrOrStderr()

	repo, err := strategy.OpenRepository(context.Background())
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	// Determine base branch
	if base == "" {
		base = strategy.GetDefaultBranchName(repo)
		if base == "" {
			base = defaultBaseBranch
		}
	}

	_, currentBranch, _ := IsOnDefaultBranch(context.Background()) //nolint:errcheck // best-effort detection
	interactive := !cmd.Flags().Changed("title") && !cmd.Flags().Changed("branch")

	if interactive {
		// Interactive flow: title → body → branch (derived) → status
		if err := runTrailCreateInteractive(&title, &body, &branch, &statusStr); err != nil {
			return err
		}
	} else {
		// Non-interactive: derive missing values from provided flags
		if branch == "" {
			if cmd.Flags().Changed("title") {
				branch = slugifyTitle(title)
			} else {
				branch = currentBranch
			}
		}
		if title == "" {
			title = trail.HumanizeBranchName(branch)
		}
	}
	if branch == "" {
		return errors.New("branch name is required")
	}

	// Create the branch if it doesn't exist
	needsCreation := branchNeedsCreation(repo, branch)
	if needsCreation {
		if err := createBranch(repo, branch); err != nil {
			return fmt.Errorf("failed to create branch %q: %w", branch, err)
		}
		fmt.Fprintf(w, "Created branch %s\n", branch)
	} else if currentBranch != branch {
		fmt.Fprintf(errW, "Note: trail will be created for branch %q (not the current branch)\n", branch)
	}

	// Check if trail already exists for this branch
	store := trail.NewStore(repo)
	existing, _, err := store.FindByBranch(branch)
	if err == nil && existing != nil {
		fmt.Fprintf(w, "Trail already exists for branch %q (ID: %s)\n", branch, existing.TrailID)
		return nil
	}

	// Determine status
	var trailStatus trail.Status
	if statusStr != "" {
		trailStatus = trail.Status(statusStr)
		if !trailStatus.IsValid() {
			return fmt.Errorf("invalid status %q: valid values are %s", statusStr, formatValidStatuses())
		}
	} else {
		trailStatus = trail.StatusDraft
	}

	// Generate trail ID
	trailID, err := trail.GenerateID()
	if err != nil {
		return fmt.Errorf("failed to generate trail ID: %w", err)
	}

	// Get author (GitHub username, falls back to git user.name)
	authorName := getTrailAuthor(repo)

	// Build intent from flags
	var intent *trail.Intent
	switch {
	case intentFile != "":
		cleanPath := filepath.Clean(intentFile)
		content, readErr := os.ReadFile(cleanPath)
		if readErr != nil {
			return fmt.Errorf("failed to read intent file: %w", readErr)
		}
		intent = &trail.Intent{Kind: "file", Value: intentFile, Content: string(content)}
	case intentIssue != "":
		intent = &trail.Intent{Kind: "issue", Value: intentIssue}
	case intentInline != "":
		intent = &trail.Intent{Kind: "inline", Value: intentInline, Content: intentInline}
	}

	// Generate branch entry ID
	entryID, err := trail.GenerateID()
	if err != nil {
		return fmt.Errorf("failed to generate branch entry ID: %w", err)
	}

	now := time.Now()
	metadata := &trail.Metadata{
		TrailID:   trailID,
		Title:     title,
		Body:      body,
		Status:    trailStatus,
		Author:    authorName,
		Assignees: []string{},
		Labels:    []string{},
		CreatedAt: now,
		UpdatedAt: now,
		Intent:    intent,
		Branches: []trail.BranchEntry{
			{
				ID:         entryID.String(),
				Name:       branch,
				BaseBranch: base,
				Status:     trail.BranchOpen,
				AddedAt:    now,
			},
		},
		// Legacy fields for backward compat
		Branch: branch,
		Base:   base,
	}

	if err := store.Write(metadata, nil, nil, nil); err != nil {
		return fmt.Errorf("failed to create trail: %w", err)
	}

	fmt.Fprintf(w, "Created trail %q for branch %s (ID: %s)\n", title, branch, trailID)

	// Push the branch and trail data to origin
	if needsCreation {
		if err := pushBranchToOrigin(branch); err != nil {
			fmt.Fprintf(errW, "Warning: failed to push branch: %v\n", err)
		} else {
			fmt.Fprintf(w, "Pushed branch %s to origin\n", branch)
		}
	}
	if err := strategy.PushTrailsBranch(context.Background(), "origin"); err != nil {
		fmt.Fprintf(errW, "Warning: failed to push trail data: %v\n", err)
	}

	// Checkout the branch if requested or prompted
	if needsCreation && currentBranch != branch {
		shouldCheckout := checkout
		if !shouldCheckout && !cmd.Flags().Changed("checkout") {
			// Interactive: ask whether to checkout
			form := NewAccessibleForm(
				huh.NewGroup(
					huh.NewConfirm().
						Title(fmt.Sprintf("Check out branch %s?", branch)).
						Value(&shouldCheckout),
				),
			)
			if formErr := form.Run(); formErr == nil && shouldCheckout {
				checkout = true
			}
		}
		if checkout {
			if err := CheckoutBranch(context.Background(), branch); err != nil {
				return fmt.Errorf("failed to checkout branch %q: %w", branch, err)
			}
			fmt.Fprintf(w, "Switched to branch %s\n", branch)
		}
	}

	return nil
}

func newTrailUpdateCmd() *cobra.Command {
	var statusStr, title, body, branch string
	var labelAdd, labelRemove []string

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update trail metadata",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runTrailUpdate(cmd.OutOrStdout(), statusStr, title, body, branch, labelAdd, labelRemove)
		},
	}

	cmd.Flags().StringVar(&statusStr, "status", "", "Update status")
	cmd.Flags().StringVar(&title, "title", "", "Update title")
	cmd.Flags().StringVar(&body, "body", "", "Update body")
	cmd.Flags().StringVar(&branch, "branch", "", "Branch to update trail for (defaults to current)")
	cmd.Flags().StringSliceVar(&labelAdd, "add-label", nil, "Add label(s)")
	cmd.Flags().StringSliceVar(&labelRemove, "remove-label", nil, "Remove label(s)")

	return cmd
}

func runTrailUpdate(w io.Writer, statusStr, title, body, branch string, labelAdd, labelRemove []string) error {
	repo, err := strategy.OpenRepository(context.Background())
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	// Determine branch
	if branch == "" {
		branch, err = GetCurrentBranch(context.Background())
		if err != nil {
			return fmt.Errorf("failed to determine current branch: %w", err)
		}
	}

	store := trail.NewStore(repo)
	metadata, _, err := store.FindByBranch(branch)
	if err != nil {
		return fmt.Errorf("failed to find trail: %w", err)
	}
	if metadata == nil {
		return fmt.Errorf("no trail found for branch %q", branch)
	}

	// Interactive mode when no flags are provided
	noFlags := statusStr == "" && title == "" && body == "" && labelAdd == nil && labelRemove == nil
	if noFlags {
		// Build status options with current value as default.
		// Exclude "done" and "abandoned" unless the trail is already in that status
		// (otherwise the select would silently reset to the first option).
		var statusOptions []huh.Option[string]
		for _, s := range trail.ValidStatuses() {
			if (s == trail.StatusDone || s == trail.StatusAbandoned) && s != metadata.Status {
				continue
			}
			label := string(s)
			if s == metadata.Status {
				label += " (current)"
			}
			statusOptions = append(statusOptions, huh.NewOption(label, string(s)))
		}
		statusStr = string(metadata.Status)
		title = metadata.Title
		body = metadata.Body

		form := NewAccessibleForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Status").
					Options(statusOptions...).
					Value(&statusStr),
				huh.NewInput().
					Title("Title").
					Value(&title),
				huh.NewText().
					Title("Body").
					Value(&body),
			),
		)
		if formErr := form.Run(); formErr != nil {
			return fmt.Errorf("form cancelled: %w", formErr)
		}
	}

	// Validate status if provided
	if statusStr != "" {
		status := trail.Status(statusStr)
		if !status.IsValid() {
			return fmt.Errorf("invalid status %q: valid values are %s", statusStr, formatValidStatuses())
		}
	}

	err = store.Update(metadata.TrailID, func(m *trail.Metadata) {
		if statusStr != "" {
			m.Status = trail.Status(statusStr)
		}
		if title != "" {
			m.Title = title
		}
		if body != "" {
			m.Body = body
		}
		for _, l := range labelAdd {
			if !slices.Contains(m.Labels, l) {
				m.Labels = append(m.Labels, l)
			}
		}
		for _, l := range labelRemove {
			m.Labels = slices.DeleteFunc(m.Labels, func(v string) bool { return v == l })
		}
	})
	if err != nil {
		return fmt.Errorf("failed to update trail: %w", err)
	}

	fmt.Fprintf(w, "Updated trail for branch %s\n", branch)
	return nil
}

// defaultBaseBranch is the fallback base branch name when it cannot be determined.
const defaultBaseBranch = "main"

func formatValidStatuses() string {
	statuses := trail.ValidStatuses()
	names := make([]string, len(statuses))
	for i, s := range statuses {
		names[i] = string(s)
	}
	return strings.Join(names, ", ")
}

// runTrailCreateInteractive runs the interactive form for trail creation.
// Prompts for title, body, branch (derived from title), and status.
func runTrailCreateInteractive(title, body, branch, statusStr *string) error {
	// Step 1: Title and body
	form := NewAccessibleForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Trail title").
				Placeholder("What are you working on?").
				Value(title),
			huh.NewText().
				Title("Body (optional)").
				Value(body),
		),
	)
	if err := form.Run(); err != nil {
		return fmt.Errorf("form cancelled: %w", err)
	}
	*title = strings.TrimSpace(*title)
	if *title == "" {
		return errors.New("trail title is required")
	}

	// Step 2: Branch (derived from title) and status
	suggested := slugifyTitle(*title)
	*branch = suggested

	// Build status options, excluding done/abandoned
	var statusOptions []huh.Option[string]
	for _, s := range trail.ValidStatuses() {
		if s == trail.StatusDone || s == trail.StatusAbandoned {
			continue
		}
		statusOptions = append(statusOptions, huh.NewOption(string(s), string(s)))
	}
	if *statusStr == "" {
		*statusStr = string(trail.StatusDraft)
	}

	form = NewAccessibleForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Branch name").
				Placeholder(suggested).
				Value(branch),
			huh.NewSelect[string]().
				Title("Status").
				Options(statusOptions...).
				Value(statusStr),
		),
	)
	if err := form.Run(); err != nil {
		return fmt.Errorf("form cancelled: %w", err)
	}
	*branch = strings.TrimSpace(*branch)
	if *branch == "" {
		*branch = suggested
	}
	return nil
}

// fetchTrailsBranch fetches the remote trails branch so we see trails from collaborators.
// Best-effort: silently ignores errors (e.g., no remote, no network).
func fetchTrailsBranch() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	branchName := paths.TrailsBranchName
	refSpec := fmt.Sprintf("+refs/heads/%s:refs/remotes/origin/%s", branchName, branchName)

	cmd := exec.CommandContext(ctx, "git", "fetch", "origin", refSpec)
	// Ensure non-interactive fetch in hook/agent contexts
	cmd.Stdin = nil
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	_ = cmd.Run() //nolint:errcheck // best-effort fetch
}

// getTrailAuthor returns the GitHub username for the trail author.
// Falls back to git user.name if gh CLI is unavailable or not authenticated.
func getTrailAuthor(repo *git.Repository) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", "api", "user", "-q", ".login")
	if output, err := cmd.Output(); err == nil {
		if login := strings.TrimSpace(string(output)); login != "" {
			return login
		}
	}
	name, _ := strategy.GetGitAuthorFromRepo(repo)
	return name
}

// slugifyTitle converts a title string into a branch-friendly slug.
// Example: "Add user authentication" -> "add-user-authentication"
func slugifyTitle(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	// Replace spaces and underscores with hyphens
	s = strings.NewReplacer(" ", "-", "_", "-").Replace(s)
	// Remove anything that's not alphanumeric, hyphen, or slash
	var b strings.Builder
	prevHyphen := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '/' {
			b.WriteRune(r)
			prevHyphen = false
		} else if r == '-' && !prevHyphen {
			b.WriteRune('-')
			prevHyphen = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// branchNeedsCreation checks if a branch exists locally.
func branchNeedsCreation(repo *git.Repository, branchName string) bool {
	_, err := repo.Reference(plumbing.NewBranchReferenceName(branchName), true)
	return err != nil
}

// createBranch creates a new local branch pointing at HEAD without checking it out.
func createBranch(repo *git.Repository, branchName string) error {
	head, err := repo.Head()
	if err != nil {
		return fmt.Errorf("failed to get HEAD: %w", err)
	}
	ref := plumbing.NewHashReference(plumbing.NewBranchReferenceName(branchName), head.Hash())
	if err := repo.Storer.SetReference(ref); err != nil {
		return fmt.Errorf("failed to create branch ref: %w", err)
	}
	return nil
}

func newTrailBranchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "branch",
		Short: "Manage branches within a trail",
	}

	cmd.AddCommand(newTrailBranchAddCmd())
	cmd.AddCommand(newTrailBranchSetPRCmd())
	cmd.AddCommand(newTrailBranchDiscardCmd())
	return cmd
}

//nolint:cyclop // sequential steps for adding a branch — splitting would obscure the flow
func newTrailBranchAddCmd() *cobra.Command {
	var baseBranch string
	var trailFlag string

	cmd := &cobra.Command{
		Use:   "add [branch-name]",
		Short: "Add a branch to a trail",
		Long:  "Adds the current branch (or a named branch) to a trail. Uses stateless lookup to find the trail from the current branch.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repo, err := strategy.OpenRepository(context.Background())
			if err != nil {
				return fmt.Errorf("failed to open repository: %w", err)
			}

			store := trail.NewStore(repo)

			// Determine branch name to add
			var branchName string
			if len(args) > 0 {
				branchName = args[0]
			} else {
				branchName, err = GetCurrentBranch(context.Background())
				if err != nil {
					return fmt.Errorf("could not determine current branch: %w", err)
				}
			}

			// Check if branch is already in a trail
			existing, existingEntry, findErr := store.FindByBranch(branchName)
			if findErr != nil {
				return fmt.Errorf("failed to look up branch: %w", findErr)
			}
			if existing != nil && existingEntry != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Branch %q is already in trail %q\n", branchName, existing.Title)
				return nil
			}

			// Resolve which trail to add to
			var targetTrail *trail.Metadata
			if trailFlag != "" {
				if err := trail.ValidateID(trailFlag); err != nil {
					return fmt.Errorf("invalid trail ID: %w", err)
				}
				targetTrail, _, _, _, err = store.Read(trail.ID(trailFlag))
				if err != nil {
					return fmt.Errorf("trail %s not found: %w", trailFlag, err)
				}
			} else {
				// Try to find trail from current branch (if adding a different branch)
				currentBranch, branchErr := GetCurrentBranch(context.Background())
				if branchErr == nil && currentBranch != branchName {
					targetTrail, _, _ = store.FindByBranch(currentBranch) //nolint:errcheck // best-effort lookup; nil targetTrail handled below
				}
				if targetTrail == nil {
					return errors.New("cannot determine trail; use --trail <id> to specify")
				}
			}

			// Determine base branch
			if baseBranch == "" {
				baseBranch = getDefaultBranch(repo)
			}

			// Generate branch entry ID
			entryID, err := trail.GenerateID()
			if err != nil {
				return fmt.Errorf("failed to generate branch ID: %w", err)
			}

			entry := trail.BranchEntry{
				ID:         entryID.String(),
				Name:       branchName,
				BaseBranch: baseBranch,
				Status:     trail.BranchOpen,
				AddedAt:    time.Now().UTC(),
			}

			if err := store.Update(targetTrail.TrailID, func(m *trail.Metadata) {
				m.Branches = append(m.Branches, entry)
			}); err != nil {
				return fmt.Errorf("failed to add branch: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Added branch %q to trail %q\n", branchName, targetTrail.Title)
			return nil
		},
	}

	cmd.Flags().StringVar(&baseBranch, "base", "", "Base branch for stacking (default: repository default branch)")
	cmd.Flags().StringVar(&trailFlag, "trail", "", "Trail ID to add branch to")

	return cmd
}

func newTrailBranchSetPRCmd() *cobra.Command {
	var branchFlag string

	cmd := &cobra.Command{
		Use:   "set-pr <number>",
		Short: "Set or replace the PR reference for a branch",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			prNumber, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid PR number: %w", err)
			}

			repo, err := strategy.OpenRepository(context.Background())
			if err != nil {
				return fmt.Errorf("failed to open repository: %w", err)
			}
			store := trail.NewStore(repo)

			branchName := branchFlag
			if branchName == "" {
				branchName, err = GetCurrentBranch(context.Background())
				if err != nil {
					return fmt.Errorf("could not determine current branch: %w", err)
				}
			}

			found, entry, err := store.FindByBranch(branchName)
			if err != nil {
				return fmt.Errorf("failed to find branch: %w", err)
			}
			if found == nil || entry == nil {
				return fmt.Errorf("branch %q not found in any trail", branchName)
			}

			if err := store.Update(found.TrailID, func(m *trail.Metadata) {
				for i := range m.Branches {
					if m.Branches[i].ID == entry.ID {
						m.Branches[i].PR = &trail.PRRef{Number: prNumber}
						break
					}
				}
			}); err != nil {
				return fmt.Errorf("failed to update trail: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Set PR #%d on branch %q in trail %q\n", prNumber, branchName, found.Title)
			return nil
		},
	}

	cmd.Flags().StringVar(&branchFlag, "branch", "", "Branch name (default: current branch)")
	return cmd
}

func newTrailBranchDiscardCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "discard",
		Short: "Mark the current branch as discarded",
		RunE: func(cmd *cobra.Command, _ []string) error {
			repo, err := strategy.OpenRepository(context.Background())
			if err != nil {
				return fmt.Errorf("failed to open repository: %w", err)
			}
			store := trail.NewStore(repo)

			branchName, err := GetCurrentBranch(context.Background())
			if err != nil {
				return fmt.Errorf("could not determine current branch: %w", err)
			}

			found, entry, err := store.FindByBranch(branchName)
			if err != nil {
				return fmt.Errorf("failed to find branch: %w", err)
			}
			if found == nil || entry == nil {
				return fmt.Errorf("branch %q not found in any trail", branchName)
			}

			if err := store.Update(found.TrailID, func(m *trail.Metadata) {
				for i := range m.Branches {
					if m.Branches[i].ID == entry.ID {
						m.Branches[i].Status = trail.BranchDiscarded
						break
					}
				}
			}); err != nil {
				return fmt.Errorf("failed to update trail: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Marked branch %q as discarded in trail %q\n", branchName, found.Title)
			return nil
		},
	}
	return cmd
}

// getDefaultBranch returns the default branch name for the repository.
func getDefaultBranch(repo *git.Repository) string {
	name := strategy.GetDefaultBranchName(repo)
	if name == "" {
		return defaultBaseBranch
	}
	return name
}

// pushBranchToOrigin pushes a branch to the origin remote.
func pushBranchToOrigin(branchName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "push", "--no-verify", "-u", "origin", branchName)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}
