package ui

import (
	"strings"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/ui/read"
)

func sp(s string) *string { return &s }

// twoHostSnapshot mirrors two host identities writing back to one ledger (events
// newest-first): claude-code most recently in the skill loop, codex earlier in
// the memory loop (twice).
func twoHostSnapshot() read.Snapshot {
	return read.Snapshot{
		Events: []read.Event{
			{ID: "e3", TS: "2026-05-30T12:00:00Z", Type: "skill.usage_observed", Host: sp("claude-code"), Loop: sp("skill"), Raw: "{}"},
			{ID: "e2", TS: "2026-05-30T11:00:00Z", Type: "memory.hot_write_observed", Host: sp("codex"), Loop: sp("memory"), Raw: "{}"},
			{ID: "e1", TS: "2026-05-30T10:00:00Z", Type: "memory.hot_write_observed", Host: sp("codex"), Loop: sp("memory"), Raw: "{}"},
		},
	}
}

// TestHostsViewShowsBothHosts is the Band 1 "TUI shows both" proof: the Hosts
// page, derived purely from the event stream, lists both host identities with
// their current loop and writeback activity.
func TestHostsViewShowsBothHosts(t *testing.T) {
	m := withSnapshot(twoHostSnapshot())
	m = send(m, "6")
	if m.active != pageHosts {
		t.Fatalf("6 should open the Hosts page, active=%d", m.active)
	}
	out := m.View()
	for _, want := range []string{"HOSTS (2)", "codex", "claude-code", "skill", "memory", "2 events"} {
		if !strings.Contains(out, want) {
			t.Errorf("hosts view missing %q:\n%s", want, out)
		}
	}
}

// TestHostsViewJumpsToLatestEvent proves the page is navigable: enter on a host
// lands on the Evidence page focused on that host's latest event.
func TestHostsViewJumpsToLatestEvent(t *testing.T) {
	m := withSnapshot(twoHostSnapshot())
	m = send(m, "6")     // Hosts page; selection 0 = most-recent host (claude-code)
	m = send(m, "enter") // follow to its latest event
	if m.active != pageEvidence {
		t.Fatalf("enter on a host should land on Evidence, active=%d", m.active)
	}
	if !m.evDetail {
		t.Error("the host's latest event should open in detail")
	}
}

// TestHostsViewShowsReadback proves the writeback-verifier state surfaces per host
// on the Hosts page (observed / acted-but-unattributed).
func TestHostsViewShowsReadback(t *testing.T) {
	snap := twoHostSnapshot()
	snap.Readback = []read.HostReadback{
		{Host: "claude-code", State: "observed", LiveDigest: "sha256:D1"},
		{Host: "codex", State: "acted-but-unattributed"},
	}
	m := withSnapshot(snap)
	m = send(m, "6")
	out := m.View()
	for _, want := range []string{"readback", "observed", "acted-but-unattributed"} {
		if !strings.Contains(out, want) {
			t.Errorf("hosts view should surface readback %q:\n%s", want, out)
		}
	}
}

// TestHostsViewEmpty proves graceful degradation when no host has written back.
func TestHostsViewEmpty(t *testing.T) {
	m := withSnapshot(read.Snapshot{})
	m = send(m, "6")
	if !strings.Contains(m.View(), "no host has written back yet") {
		t.Errorf("empty hosts view should explain the empty state:\n%s", m.View())
	}
}
