package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "dev"

var rootCmd = &cobra.Command{
	Use:     "mnemon-harness",
	Version: version,
	Short:   "Experimental Mnemon lifecycle harness",
	Long:    "Experimental Mnemon lifecycle, profile, daemon, HostAgent runner, and goal governance commands.",
}

// Command groups: the everyday spine (loop install, ui, proposal review, goal
// governance) is surfaced first; the rest is an advanced tail. Grouping is help-only
// — it changes how `--help` lists verbs, never a verb path or behavior.
const (
	groupSpine    = "spine"
	groupAdvanced = "advanced"
)

func init() {
	rootCmd.AddGroup(
		&cobra.Group{ID: groupSpine, Title: "Spine commands (the everyday path):"},
		&cobra.Group{ID: groupAdvanced, Title: "Advanced commands:"},
	)
	rootCmd.SetHelpCommandGroupID(groupAdvanced)
	rootCmd.SetCompletionCommandGroupID(groupAdvanced)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
