package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/runtime"
	"github.com/spf13/cobra"
)

// Background sync must NOT silently fail every pass while a co-hosted Local Mnemon holds the
// single-writer lock; it refuses cleanly up front with an actionable message.
func TestSyncBackgroundRefusesWhenLocalMnemonHoldsStore(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "governed.db")
	rt, err := runtime.OpenRuntime(storePath, runtime.RuntimeConfig{}) // holds the single-writer lock
	if err != nil {
		t.Fatalf("open runtime (hold lock): %v", err)
	}
	defer rt.Close()

	prevPath, prevBg, prevInt := syncStorePath, syncBackground, syncInterval
	syncStorePath, syncBackground, syncInterval = storePath, true, time.Second
	t.Cleanup(func() { syncStorePath, syncBackground, syncInterval = prevPath, prevBg, prevInt })

	err = runSyncBackground(&cobra.Command{}, nil)
	if err == nil || !strings.Contains(err.Error(), "sync worker") {
		t.Fatalf("background sync must refuse while Local Mnemon holds the store and point at the in-process sync worker; got %v", err)
	}
}
