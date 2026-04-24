// Package main is the entry point for the entireio/cli fork.
// This CLI tool provides a unified interface for interacting with Entire.io services.
//
// Personal fork: customized for local development and experimentation.
// Upstream: https://github.com/entireio/cli
//
// Note: Using exit code 2 for usage errors to follow POSIX convention
// (exit code 1 = general errors, exit code 2 = misuse of shell builtins/usage errors).
package main

import (
	"fmt"
	"os"

	"github.com/entireio/cli/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		// Exit code 1 is more appropriate here for general runtime errors;
		// exit code 2 is reserved for usage/argument errors handled inside cmd.
		os.Exit(1)
	}
}
