package cmd

import (
	"fmt"
	"os"

	"github.com/mnemon-dev/mnemon/internal/store"
	"github.com/spf13/cobra"
)

// version is set at build time via ldflags.
var version = "dev"

var dataDir string

var rootCmd = &cobra.Command{
	Use:     "mnemon",
	Version: version,
	Short:   "Memory daemon for LLM agents",
	Long:    "Mnemon is a standalone memory daemon based on MAGMA's four-graph architecture.",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&dataDir, "data-dir", store.DefaultDataDir(), "data directory for mnemon database")
}

// openDB is a helper used by subcommands.
func openDB() (*store.DB, error) {
	return store.Open(dataDir)
}
