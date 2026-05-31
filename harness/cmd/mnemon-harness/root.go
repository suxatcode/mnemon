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

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
