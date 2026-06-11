package syncserver

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The trust-domain import boundary (goal-stage6 adjudication #1): the standalone hub packages —
// syncserver and the mnemond binary — may import contract/store-level shared leaves only, NEVER
// channel / runtime / app / hostsurface. Local and remote are separate trust domains that share
// only the contract; this test walks the real dependency graph so a casual import cannot slip in.
func TestHubImportBoundaryExcludesLocalTrustDomain(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps",
		"./harness/internal/syncserver", "./harness/cmd/mnemond")
	cmd.Dir = moduleRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps: %v\n%s", err, out)
	}
	forbidden := map[string]bool{
		"github.com/mnemon-dev/mnemon/harness/internal/channel":     true,
		"github.com/mnemon-dev/mnemon/harness/internal/runtime":     true,
		"github.com/mnemon-dev/mnemon/harness/internal/app":         true,
		"github.com/mnemon-dev/mnemon/harness/internal/hostsurface": true,
	}
	for _, dep := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if forbidden[strings.TrimSpace(dep)] {
			t.Fatalf("hub dependency graph crossed the trust boundary: %s", dep)
		}
	}
}

// moduleRoot walks up to go.mod so the `./...` patterns resolve regardless of the package dir the
// test runs from (the command runs THERE via cmd.Dir — never a global chdir).
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above test dir")
		}
		dir = parent
	}
}
