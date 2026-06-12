package main

import (
	"fmt"

	"github.com/mnemon-dev/mnemon/harness/internal/app"
	"github.com/spf13/cobra"
)

var (
	refreshRoot        string
	refreshProjectRoot string
	refreshHost        string
	refreshLoops       []string
)

// refresh re-projects the managed definition files (GUIDE, hooks, skill defs) for a host loop without
// clobbering user edits, and without touching the channel (bindings, token, config). It is a sibling
// of setup, not a subcommand, so it carries its own flags. Every integration is a loop — memory and
// skill are `--loop memory` / `--loop skill` (PD7: no privileged flags).
var refreshCmd = &cobra.Command{
	Use:   "refresh --host HOST --loop LOOP [--loop LOOP ...]",
	Short: "Re-project managed definition files, preserving user edits",
	RunE: func(cmd *cobra.Command, args []string) error {
		conflicts, err := app.New(refreshRoot).Refresh(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(),
			refreshProjectRoot, refreshHost, append([]string(nil), refreshLoops...), nil)
		if err != nil {
			return err
		}
		for _, c := range conflicts {
			fmt.Fprintf(cmd.OutOrStdout(), "preserved user-modified %s\n", c)
		}
		return nil
	},
}

func init() {
	refreshCmd.Flags().StringVar(&refreshRoot, "root", ".", "repository root containing harness declarations")
	refreshCmd.Flags().StringVar(&refreshProjectRoot, "project-root", "", "project root for Agent Integration artifacts (defaults to root)")
	refreshCmd.Flags().StringVar(&refreshHost, "host", "", "Agent Integration host id")
	refreshCmd.Flags().StringArrayVar(&refreshLoops, "loop", nil, "loop id to refresh (e.g. memory, skill, or an external package); may be repeated")
	refreshCmd.GroupID = groupSpine
	rootCmd.AddCommand(refreshCmd)
}
