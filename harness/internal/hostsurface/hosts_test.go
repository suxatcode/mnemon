package hostsurface

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/assets"
)

// bareMnemonCLI matches an invocation of the legacy `mnemon` binary (a space-delimited command), but
// NOT `mnemon-harness` (hyphen) — the host hooks must reach Local Mnemon only through the channel.
var bareMnemonCLI = regexp.MustCompile(`\bmnemon `)

// Every memory/skill prime hook must reach Local Mnemon ONLY through the channel: no bare `mnemon`
// CLI, and a `mnemon-harness control` (observe/pull/status) routing — never a direct read of the
// governed store. (Catting the derived MEMORY.md mirror is intended and not checked here.)
func TestHostPrimesRouteThroughChannel(t *testing.T) {
	primes := []string{
		"hosts/codex/memory/hooks/prime.sh",
		"hosts/codex/skill/hooks/prime.sh",
		"hosts/claude-code/memory/hooks/prime.sh",
		"hosts/claude-code/skill/hooks/prime.sh",
	}
	for _, p := range primes {
		data, err := fs.ReadFile(assets.FS, p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		content := string(data)
		if bareMnemonCLI.MatchString(content) {
			t.Errorf("%s calls the bare `mnemon` CLI; route through mnemon-harness control instead", p)
		}
		if !strings.Contains(content, "control observe") &&
			!strings.Contains(content, "control pull") &&
			!strings.Contains(content, "control status") {
			t.Errorf("%s must route through mnemon-harness control (observe/pull/status)", p)
		}
	}
}
