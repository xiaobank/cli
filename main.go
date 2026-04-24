// Package main is the entry point for the entireio/cli fork.
// This CLI tool provides a unified interface for interacting with Entire.io services.
//
// Personal fork: customized for local development and experimentation.
// Upstream: https://github.com/entireio/cli
package main

import (
	"fmt"
	"os"

	"github.com/entireio/cli/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
