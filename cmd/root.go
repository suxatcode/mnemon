package cmd

import (
	"fmt"
	"os"

	"github.com/mnemon-dev/mnemon/internal/store"
	"github.com/spf13/cobra"
)

// version is set at build time via ldflags.
var version = "dev"

var (
	dataDir   string
	storeName string
)

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
	rootCmd.PersistentFlags().StringVar(&dataDir, "data-dir", store.DefaultDataDir(), "base data directory")
	rootCmd.PersistentFlags().StringVar(&storeName, "store", "", "named memory store (overrides MNEMON_STORE and active file)")
}

// resolveStoreName returns the effective store name.
// Priority: --store flag > MNEMON_STORE env > active file > "default".
func resolveStoreName() string {
	if storeName != "" {
		return storeName
	}
	if env := os.Getenv("MNEMON_STORE"); env != "" {
		return env
	}
	return store.ReadActive(dataDir)
}

// openDB is a helper used by subcommands.
func openDB() (*store.DB, error) {
	if err := store.MigrateIfNeeded(dataDir); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	name := resolveStoreName()
	dir := store.StoreDir(dataDir, name)
	return store.Open(dir)
}
