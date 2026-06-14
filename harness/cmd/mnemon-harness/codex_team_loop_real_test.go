package main

import (
	"strings"
	"testing"
)

// TestSandboxGuidance guards the bug a real run exposed: a hardcoded "read-only" instruction
// under a writable sandbox silently blocks all file work. The guidance must match the policy.
func TestSandboxGuidance(t *testing.T) {
	if g := sandboxGuidance("readOnly"); !strings.Contains(g, "do not modify") {
		t.Fatalf("readOnly should forbid writes: %q", g)
	}
	for _, sb := range []string{"workspaceWrite", "dangerFullAccess"} {
		if g := sandboxGuidance(sb); !strings.Contains(g, "create") {
			t.Fatalf("%s should permit writes: %q", sb, g)
		}
	}
}

// These tests exercise the real-Codex brain's output parsing and role wiring WITHOUT spending a
// real Codex turn — the model's text is supplied directly.

func TestParseRealReport(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"tagged", "I broke the goal into lanes.\nMNEMON_REPORT: planned build and review lanes", "planned build and review lanes"},
		{"case-insensitive tag", "done\nmnemon_report:  shipped it ", "shipped it"},
		{"last tag wins", "MNEMON_REPORT: first\nMNEMON_REPORT: final", "final"},
		{"fallback to one-liner", "just a sentence with no tag", "just a sentence with no tag"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseRealReport(c.in); got != c.want {
				t.Fatalf("parseRealReport(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestParseRealAssign(t *testing.T) {
	assignee, scope, ok := parseRealAssign("Reviewer should look at it.\nMNEMON_ASSIGN: codex-05@appserver\nMNEMON_SCOPE: review the build for risk")
	if !ok || assignee != "codex-05@appserver" || scope != "review the build for risk" {
		t.Fatalf("parse routing: ok=%v assignee=%q scope=%q", ok, assignee, scope)
	}

	if _, _, ok := parseRealAssign("Nothing to route right now.\nMNEMON_ASSIGN: none"); ok {
		t.Fatalf("'none' should yield ok=false")
	}
	if _, _, ok := parseRealAssign("no contract line at all"); ok {
		t.Fatalf("missing tag should yield ok=false")
	}

	// scope is optional; a present assignee with no scope still routes (with a default scope).
	a, s, ok := parseRealAssign("MNEMON_ASSIGN: codex-03@appserver")
	if !ok || a != "codex-03@appserver" || s == "" {
		t.Fatalf("assignee-only: ok=%v a=%q s=%q (scope should default non-empty)", ok, a, s)
	}
}

func TestParseLoopRealRoles(t *testing.T) {
	got, err := parseLoopRealRoles(" planner , poc-build ")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !got["planner"] || !got["poc-build"] || len(got) != 2 {
		t.Fatalf("got %+v", got)
	}
	if _, err := parseLoopRealRoles("planner,bogus"); err == nil {
		t.Fatalf("expected error for unknown role")
	}
	if got, _ := parseLoopRealRoles(""); len(got) != 0 {
		t.Fatalf("empty should be no real roles, got %+v", got)
	}
}

// TestCodexLoopBrainsSubstitution verifies a named role gets a real brain (same agentBrain
// interface) while the rest stay scripted — no turn is run because Act is never called here.
func TestCodexLoopBrainsSubstitution(t *testing.T) {
	cfg := defaultLoopDemoConfig()
	brains, reals := codexLoopBrains(cfg, map[string]bool{"planner": true}, "/tmp", "codex", "readOnly", 0, nil)
	if len(brains) != 5 {
		t.Fatalf("want 5 brains, got %d", len(brains))
	}
	if len(reals) != 1 {
		t.Fatalf("want 1 real brain (planner), got %d", len(reals))
	}
	if reals[0].Principal() != cfg.Planner {
		t.Fatalf("real brain principal = %q, want planner %q", reals[0].Principal(), cfg.Planner)
	}
	// The planner slot (index 0) must be the real brain; the rest scripted.
	if _, ok := brains[0].(*realCodexBrain); !ok {
		t.Fatalf("brain[0] should be *realCodexBrain")
	}
	if _, ok := brains[1].(scriptedBrain); !ok {
		t.Fatalf("brain[1] (poc-build) should be scriptedBrain")
	}
}
