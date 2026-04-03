package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/entireio/cli/cmd/entire/cli/logging"
	"github.com/entireio/cli/cmd/entire/cli/paths"
	"github.com/entireio/cli/cmd/entire/cli/strategy"
	"github.com/spf13/cobra"
)

// Git remote protocol identifiers used for auth detection.
const (
	fetchProtocolSSH   = "ssh"
	fetchProtocolHTTPS = "https"
)

func newFetchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "fetch",
		Hidden: true,
		Short:  "Fetch the remote checkpoint tip",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()

			if _, err := paths.WorktreeRoot(ctx); err != nil {
				return errors.New("not a git repository")
			}

			logging.SetLogLevelGetter(GetLogLevel)
			if err := logging.Init(ctx, ""); err == nil {
				defer logging.Close()
			}

			return runFetch(ctx, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}

	return cmd
}

func runFetch(ctx context.Context, w, errW io.Writer) error {
	// Resolve the checkpoint remote URL from settings.
	checkpointURL, configured, err := strategy.ResolveCheckpointRemoteURL(ctx)
	if err != nil {
		return fmt.Errorf("failed to resolve checkpoint remote: %w", err)
	}

	// Determine the fetch target: checkpoint URL if configured, otherwise "origin".
	target := "origin"
	if configured && checkpointURL != "" {
		target = checkpointURL
	}

	protocol := resolveProtocolForTarget(ctx, target)
	authMethod := detectAuthMethod(ctx, protocol)
	remoteInfo := resolveRemoteInfo(ctx, target)

	logging.Debug(ctx, "fetch: auth detection",
		slog.String("auth", authMethod),
		slog.String("domain", remoteInfo.domain),
		slog.String("repo", remoteInfo.ownerRepo),
		slog.String("protocol", protocol),
	)

	if isDebugMode() {
		fmt.Fprintf(errW, "[entire] fetch: remote=%s/%s protocol=%s auth=%s\n",
			remoteInfo.domain, remoteInfo.ownerRepo, protocol, authMethod)
	}

	// Fetch the metadata branch.
	branchName := paths.MetadataBranchName
	logging.Debug(ctx, "fetch: fetching checkpoint tip",
		slog.String("branch", branchName),
		slog.String("domain", remoteInfo.domain),
		slog.String("repo", remoteInfo.ownerRepo),
		slog.String("protocol", protocol),
	)

	if err := strategy.FetchMetadataBranch(ctx, target); err != nil {
		fmt.Fprintf(errW, "[entire] Failed to fetch %s: %v\n", branchName, err)
		return NewSilentError(err)
	}

	fmt.Fprintf(w, "[entire] Fetched %s\n", branchName)
	return nil
}

// detectAuthMethod determines how git will authenticate for the given target.
// Returns a human-readable description for debug logging.
func detectAuthMethod(ctx context.Context, protocol string) string {
	// Check if ENTIRE_CHECKPOINT_TOKEN overrides auth.
	if token := strings.TrimSpace(os.Getenv(strategy.CheckpointTokenEnvVar)); token != "" {
		if protocol == fetchProtocolSSH {
			return "ENTIRE_CHECKPOINT_TOKEN set (ignored: remote uses SSH)"
		}
		return "ENTIRE_CHECKPOINT_TOKEN"
	}

	switch protocol {
	case fetchProtocolSSH:
		if hasSSHAgent() {
			return "SSH agent"
		}
		return "SSH (no agent detected)"
	case fetchProtocolHTTPS:
		if helper := gitCredentialHelper(ctx); helper != "" {
			return fmt.Sprintf("Git credential helper (%s)", helper)
		}
		return "no auth detected"
	default:
		return "no auth detected"
	}
}

// resolveProtocolForTarget returns "ssh", "https", or "" for the given target.
func resolveProtocolForTarget(ctx context.Context, target string) string {
	if isFetchURL(target) {
		if strings.HasPrefix(target, "https://") {
			return fetchProtocolHTTPS
		}
		if strings.Contains(target, "@") {
			return fetchProtocolSSH
		}
		return ""
	}
	// Remote name — resolve URL.
	rawURL, err := getRemoteURLForFetch(ctx, target)
	if err != nil {
		return ""
	}
	if strings.HasPrefix(rawURL, "https://") || strings.HasPrefix(rawURL, "http://") {
		return fetchProtocolHTTPS
	}
	// SCP-style SSH: git@host:path or contains "@"
	if strings.Contains(rawURL, "@") {
		return fetchProtocolSSH
	}
	return ""
}

// hasSSHAgent checks whether an SSH agent is available via SSH_AUTH_SOCK.
func hasSSHAgent() bool {
	return os.Getenv("SSH_AUTH_SOCK") != ""
}

// gitCredentialHelper returns the configured git credential helper, if any.
func gitCredentialHelper(ctx context.Context) string {
	cmd := exec.CommandContext(ctx, "git", "config", "credential.helper")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// getRemoteURLForFetch resolves a remote name to its URL.
func getRemoteURLForFetch(ctx context.Context, remoteName string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "remote", "get-url", remoteName)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("remote %q not found", remoteName)
	}
	return strings.TrimSpace(string(output)), nil
}

// fetchRemoteInfo holds parsed remote components for logging.
type fetchRemoteInfo struct {
	domain    string // e.g., "github.com"
	ownerRepo string // e.g., "org/my-repo"
}

// resolveRemoteInfo parses the target into domain and owner/repo for display.
// If the target is a remote name, resolves it to a URL first.
func resolveRemoteInfo(ctx context.Context, target string) fetchRemoteInfo {
	rawURL := target
	if !isFetchURL(target) {
		var err error
		rawURL, err = getRemoteURLForFetch(ctx, target)
		if err != nil {
			return fetchRemoteInfo{domain: target}
		}
	}

	host, owner, repo, err := strategy.ParseRemoteURL(rawURL)
	if err != nil {
		return fetchRemoteInfo{domain: target}
	}
	return fetchRemoteInfo{domain: host, ownerRepo: owner + "/" + repo}
}

// isDebugMode returns true if debug-level logging is enabled.
func isDebugMode() bool {
	level := strings.ToUpper(os.Getenv(logging.LogLevelEnvVar))
	if level != "" {
		return level == "DEBUG"
	}
	return strings.EqualFold(GetLogLevel(), "DEBUG")
}

// isFetchURL returns true if the target looks like a URL rather than a git remote name.
func isFetchURL(target string) bool {
	return strings.Contains(target, "://") || strings.Contains(target, "@")
}
