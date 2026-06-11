package hostsurface

import (
	"regexp"
	"strings"
	"testing"
)

// bareMnemonCLI matches an invocation of the legacy `mnemon` binary (a space-delimited command), but
// NOT `mnemon-harness` (hyphen) — the host hooks must reach Local Mnemon only through the channel.
var bareMnemonCLI = regexp.MustCompile(`\bmnemon `)

// Every memory/skill prime hook must reach Local Mnemon ONLY through the channel: no bare `mnemon`
// CLI, and a `mnemon-harness control` (observe/pull/status) routing — never a direct read of the
// governed store. (Catting the derived MEMORY.md mirror is intended and not checked here.)
func TestHostPrimesRouteThroughChannel(t *testing.T) {
	primes := []struct{ host, loop string }{
		{"codex", "memory"}, {"codex", "skill"},
		{"claude-code", "memory"}, {"claude-code", "skill"},
	}
	for _, pr := range primes {
		p := pr.host + "/" + pr.loop + "/prime"
		content, err := RenderHook(pr.loop, pr.host, "prime")
		if err != nil {
			t.Fatalf("render %s: %v", p, err)
		}
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
