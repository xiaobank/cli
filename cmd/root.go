// Package cmd provides the root command and CLI framework setup for the entireio CLI.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	// Version is set at build time via ldflags.
	Version = "dev"
	// Commit is the git commit hash set at build time.
	Commit = "none"
	// Date is the build date set at build time.
	Date = "unknown"

	// verbose enables verbose output across all subcommands.
	verbose bool
	// outputFormat controls the output format (json, text, yaml).
	outputFormat string
)

// rootCmd is the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "entire",
	Short: "entireio CLI — manage and interact with entireio services",
	Long: `entire is a command-line interface for interacting with entireio services.

It provides commands for managing resources, querying data, and automating
workflows against the entireio platform.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main() and only needs to happen once.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func init() {
	// Persistent flags are available to all subcommands.
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output")
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "text", "output format: text, json, yaml")

	// Register subcommands.
	rootCmd.AddCommand(newVersionCmd())
}

// newVersionCmd returns the version subcommand.
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version information",
		Run: func(cmd *cobra.Command, args []string) {
			switch outputFormat {
			case "json":
				fmt.Printf(`{"version":%q,"commit":%q,"date":%q}\n`, Version, Commit, Date)
			default:
				fmt.Printf("entire version %s (commit: %s, built: %s)\n", Version, Commit, Date)
			}
		},
	}
}
