package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/entireio/cli/cmd/entire/cli/settings"
)

const autoUpdateLong = `Control whether Entire checks for and installs new versions automatically.

Modes:
  off     Show update notifications only (default).
  prompt  Notify and ask Y/N to install when an update is detected.
  auto    Install new releases silently after a 24h soak delay.

Auto-update requires install provenance (written by install.sh, brew, or
scoop). Users who built from source or installed via mise will continue to
see the notification hint.

Settings live at ~/.config/entire/settings.json (machine-wide).
Override at runtime with ENTIRE_NO_AUTO_UPDATE=1.`

func newAutoUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auto-update",
		Short: "Configure automatic CLI updates",
		Long:  autoUpdateLong,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAutoUpdateStatus(cmd.OutOrStdout())
		},
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Print current auto-update mode",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAutoUpdateStatus(cmd.OutOrStdout())
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "enable",
		Short: "Set auto-update mode to prompt",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAutoUpdateSet(cmd.OutOrStdout(), settings.AutoUpdatePrompt)
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "disable",
		Short: "Set auto-update mode to off",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAutoUpdateSet(cmd.OutOrStdout(), settings.AutoUpdateOff)
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "set <off|prompt|auto>",
		Short: "Set auto-update mode explicitly",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mode := args[0]
			if !settings.IsValidAutoUpdate(mode) {
				fmt.Fprintf(cmd.ErrOrStderr(),
					"invalid mode %q: must be %q, %q, or %q\n",
					mode, settings.AutoUpdateOff, settings.AutoUpdatePrompt, settings.AutoUpdateAuto)
				return NewSilentError(fmt.Errorf("invalid mode: %s", mode))
			}
			return runAutoUpdateSet(cmd.OutOrStdout(), mode)
		},
	})

	return cmd
}

func runAutoUpdateStatus(w io.Writer) error {
	g, err := settings.LoadGlobal()
	if err != nil {
		return fmt.Errorf("loading global settings: %w", err)
	}
	path, err := settings.GlobalSettingsPath()
	if err != nil {
		return fmt.Errorf("resolving settings path: %w", err)
	}
	fmt.Fprintf(w, "auto_update: %s\n", g.GetAutoUpdate())
	fmt.Fprintf(w, "config: %s\n", path)
	return nil
}

func runAutoUpdateSet(w io.Writer, mode string) error {
	g, err := settings.LoadGlobal()
	if err != nil {
		return fmt.Errorf("loading global settings: %w", err)
	}
	g.AutoUpdate = mode
	if err := settings.SaveGlobal(g); err != nil {
		return fmt.Errorf("saving global settings: %w", err)
	}
	fmt.Fprintf(w, "auto_update set to %s (updated %s)\n", mode, time.Now().Format(time.RFC3339))
	return nil
}
