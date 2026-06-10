package hostsurface

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/projection"
)

func mirrorProj(tag string) projection.Projection {
	return projection.Projection{Content: []projection.ResourceContent{{
		Ref:    contract.ResourceRef{Kind: "memory", ID: "project"},
		Fields: map[string]any{"content": "# Local Memory\n- " + strings.Repeat(tag, 4096)},
	}}}
}

// The prime hook (another process) and the background driver regenerate the mirror concurrently.
// Each writer must use its OWN temp file: with a fixed temp name, writer B truncates the inode
// writer A renames into place, exposing torn bytes through the target path. Pin: after racing
// writers, the mirror is ONE complete body (never a mix), and no temp files are left behind.
func TestWriteMemoryMirrorConcurrentWritersNeverTear(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "MEMORY.md")
	projA, projB := mirrorProj("A"), mirrorProj("B")

	for round := 0; round < 30; round++ {
		errs := make(chan error, 2)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); errs <- WriteMemoryMirror(path, projA) }()
		go func() { defer wg.Done(); errs <- WriteMemoryMirror(path, projB) }()
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("round %d: concurrent writer failed: %v", round, err)
			}
		}

		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
		hasA, hasB := strings.Contains(string(body), "AAAA"), strings.Contains(string(body), "BBBB")
		if hasA == hasB { // both or neither = torn/mixed mirror
			t.Fatalf("round %d: mirror is torn (A=%v B=%v)", round, hasA, hasB)
		}
		if len(body) < 4096 {
			t.Fatalf("round %d: mirror truncated to %d bytes", round, len(body))
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}
