package main

import (
	"fmt"

	"github.com/mattn/go-isatty"
	"github.com/mnemon-dev/mnemon/harness/internal/ui"
	"github.com/spf13/cobra"
)

var uiRoot string

var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Open the Mnemon cognition harness console (TUI)",
	Long: "Open the terminal cognition console: a bubbletea UI layered on the\n" +
		"harness facade. The screen is the governed improvement loop — scope,\n" +
		"evidence, proposals (review + apply), audit, next run. All writes route\n" +
		"through the same facade the CLI uses; the console never bypasses audit.",
	RunE: runUI,
}

func init() {
	uiCmd.Flags().StringVar(&uiRoot, "root", ".", "project root for the harness console")
	rootCmd.AddCommand(uiCmd)
}

func runUI(cmd *cobra.Command, args []string) error {
	// The console is a full-screen interactive program; it requires a TTY on
	// both ends. In a non-TTY context (pipe, CI, redirect) exit cleanly with a
	// message rather than hanging on an input stream that never produces keys.
	in, ok := cmd.InOrStdin().(interface{ Fd() uintptr })
	out, okOut := cmd.OutOrStdout().(interface{ Fd() uintptr })
	if !ok || !okOut || !isatty.IsTerminal(in.Fd()) || !isatty.IsTerminal(out.Fd()) {
		fmt.Fprintln(cmd.ErrOrStderr(), "mnemon-harness ui requires an interactive terminal (TTY).")
		return nil
	}
	return ui.Run(uiRoot)
}
