package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/entireio/cli/cmd/entire/cli/logging"
)

// corruptedUserName is the literal string that #456 sets user.name to.
const corruptedUserName = "user.email"

// configSnapshot captures user.name and user.email from the LOCAL .git/config only.
type configSnapshot struct {
	name  string
	email string
}

// snapshotLocalGitConfig reads user.name and user.email from LOCAL .git/config.
// Uses `git config --local --get` to avoid picking up global/system config.
// Returns empty strings for fields that are not set locally (which is the normal case).
func snapshotLocalGitConfig() configSnapshot {
	return configSnapshot{
		name:  getLocalGitConfigValue("user.name"),
		email: getLocalGitConfigValue("user.email"),
	}
}

// getLocalGitConfigValue retrieves a git config value from local scope only.
// Returns empty string if the value is not set locally or on error.
func getLocalGitConfigValue(key string) string {
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "git", "config", "--local", "--get", key)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// checkConfigIntegrity compares before/after snapshots of local git config
// and warns if user.name or user.email changed during an operation.
// Specifically detects the #456 pattern: user.name set to literal "user.email".
func checkConfigIntegrity(ctx context.Context, operation string, before, after configSnapshot) {
	nameChanged := before.name != after.name
	emailChanged := before.email != after.email

	if !nameChanged && !emailChanged {
		return
	}

	// Detect #456 pattern first: user.name changed to the literal string "user.email"
	if nameChanged && after.name == corruptedUserName {
		fmt.Fprintf(os.Stderr,
			"WARNING: .git/config user.name was set to the literal string \"user.email\" during %s. "+
				"This is a known corruption pattern (see https://github.com/entireio/cli/issues/456). "+
				"Please report this with your session logs.\n",
			operation,
		)
		logging.Error(ctx, "detected #456 git config corruption: user.name = literal \"user.email\"",
			slog.String("operation", operation),
		)
		return
	}

	// Generic warning for any unexpected change (log booleans, not PII)
	logging.Warn(ctx, "local git config changed during operation",
		slog.String("operation", operation),
		slog.Bool("user_name_changed", nameChanged),
		slog.Bool("user_email_changed", emailChanged),
	)

	fmt.Fprintf(os.Stderr,
		"WARNING: local .git/config was modified during %s (user.name: %q→%q, user.email: %q→%q). "+
			"If unexpected, please report at https://github.com/entireio/cli/issues/456\n",
		operation, before.name, after.name, before.email, after.email,
	)
}

// validateConfigNotCorrupted checks at session start whether local git config
// already has the #456 corruption pattern (user.name = literal "user.email").
// This catches corruption that happened between sessions (e.g., an AI agent
// running `git config user.name user.email` as a standalone command).
func validateConfigNotCorrupted(ctx context.Context) {
	snap := snapshotLocalGitConfig()
	if snap.name == corruptedUserName {
		fmt.Fprintf(os.Stderr,
			"WARNING: .git/config user.name is the literal string \"user.email\" — this is a known "+
				"corruption pattern (see https://github.com/entireio/cli/issues/456). "+
				"Fix with: git config --local user.name \"<your actual name>\"\n",
		)
		logging.Warn(ctx, "detected #456 git config corruption at session start: user.name = literal \"user.email\"")
	}
}
