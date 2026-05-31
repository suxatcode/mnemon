package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mnemon-dev/mnemon/harness/internal/ui/read"
)

// TestConfirmModalSwallowsQuit: a bare q must not abandon an open governed-write
// confirm; ctrl+c remains the hard escape.
func TestConfirmModalSwallowsQuit(t *testing.T) {
	root := t.TempDir()
	createMemoryProposal(t, root, "p-confirm")
	m := loadModel(t, root)
	m.active = pageProposals

	m = send(m, "o") // raise the open-transition confirm
	if m.confirm == nil {
		t.Fatal("action key should raise a confirm modal")
	}
	// q is swallowed by the modal — the program does not quit and the modal stays.
	nm, cmd := m.Update(keyOf("q"))
	m = nm.(model)
	if returnsQuit(cmd) {
		t.Error("q must not quit while a confirm modal is open")
	}
	if m.confirm == nil {
		t.Error("confirm modal should remain open after q")
	}
	// ctrl+c is still the hard quit.
	if _, c := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC}); !returnsQuit(c) {
		t.Error("ctrl+c should quit even with a confirm modal open")
	}
}

// TestLinkNavClosesSourceDetail: following evidence→proposal closes the evidence
// detail so returning to Evidence shows the list, not a stale detail.
func TestLinkNavClosesSourceDetail(t *testing.T) {
	snap := read.Snapshot{
		Proposals: []read.Proposal{{ID: "P1", Route: "memory", Status: "open", Risk: "low", Title: "T", UpdatedAt: "2026-05-30T10:00:00Z"}},
		Events: []read.Event{{ID: "e", TS: "2026-05-30T11:00:00Z", Type: "proposal.applied", Actor: "u", Source: "s",
			Payload: map[string]any{"proposal_id": "P1"}, Raw: "{}"}},
	}
	m := withSnapshot(snap)
	m.active = pageEvidence
	m = send(m, "enter") // open evidence detail
	m = send(m, "enter") // follow link to proposal
	if m.active != pageProposals || !m.prDetail {
		t.Fatalf("should land in proposal detail; active=%d prDetail=%v", m.active, m.prDetail)
	}
	if m.evDetail {
		t.Error("source evidence detail flag should be cleared after following the link")
	}
}

// TestSwitchPageClosesDetail: 1-4/tab lands on the list, never a stale detail.
func TestSwitchPageClosesDetail(t *testing.T) {
	root := t.TempDir()
	createMemoryProposal(t, root, "p-sw")
	m := loadModel(t, root)
	m.active = pageProposals
	m = send(m, "enter") // open proposal detail
	if !m.prDetail {
		t.Fatal("proposal detail should open")
	}
	m = send(m, "2") // switch to Evidence
	m = send(m, "3") // back to Proposals
	if m.prDetail {
		t.Error("returning to a page should show its list, not a stale detail")
	}
}

// TestExtractAuditTSTrailing: the trailing stamp wins when a name carries two.
func TestExtractAuditTSTrailing(t *testing.T) {
	name := "goal-improve-20260101T010101-completion-20260102T020202000000000"
	got := extractAuditTS(name)
	if got != "2026-01-02T02:02:02Z" {
		t.Errorf("extractAuditTS should return the trailing stamp, got %q", got)
	}
	if extractAuditTS("manual-check-no-stamp") != "" {
		t.Error("a name without a stamp should yield empty")
	}
}

// TestUndatedAuditSortsLast: an audit whose name has no parseable timestamp must
// not float to the top of the reverse-chronological stream.
func TestUndatedAuditSortsLast(t *testing.T) {
	snap := read.Snapshot{
		Events: []read.Event{{ID: "e1", TS: "2026-05-30T10:00:00Z", Type: "goal.planned", Actor: "u", Source: "s", Raw: "{}"}},
		Audits: []read.AuditRecord{{
			Audit: read.AuditDoc{Metadata: read.AuditMetadata{Name: "manual-check"}, Spec: map[string]any{"audit_kind": "manual"}},
			Ref:   map[string]any{"uri": "x"},
		}},
	}
	m := withSnapshot(snap)
	items := m.evidenceItems()
	if len(items) != 2 {
		t.Fatalf("expected 2 evidence items, got %d", len(items))
	}
	if items[0].kind != "event" {
		t.Errorf("the timestamped event should sort first, got %q", items[0].kind)
	}
	if items[1].kind != "audit" {
		t.Errorf("the undated audit should sort last, got %q", items[1].kind)
	}
}

// TestTruncPlainDisplayWidth: truncation respects terminal cell width for wide
// runes (a row never overflows its budget).
func TestTruncPlainDisplayWidth(t *testing.T) {
	s := "日本語のテストです末長く" // all double-width runes
	out := truncPlain(s, 10)
	if w := lipgloss.Width(out); w > 10 {
		t.Errorf("truncPlain should respect display width <= 10, got %d (%q)", w, out)
	}
	// ASCII still fits exactly.
	if got := truncPlain("hello", 10); got != "hello" {
		t.Errorf("short ASCII should pass through unchanged, got %q", got)
	}
}

// TestToastAutoClears: setToast schedules a clear that only fires for the toast
// it scheduled (a newer toast owns its own expiry).
func TestToastAutoClears(t *testing.T) {
	m := newModel(".")
	if cmd := (&m).setToast("hello", false); cmd == nil {
		t.Fatal("setToast should return an expiry command")
	}
	seq := m.toastSeq
	// A stale clear (older seq) must not clear the current toast.
	nm, _ := m.Update(clearToastMsg{seq: seq - 1})
	m = nm.(model)
	if m.toast == "" {
		t.Error("a stale clearToast must not clear a newer toast")
	}
	// The matching clear empties it.
	nm, _ = m.Update(clearToastMsg{seq: seq})
	m = nm.(model)
	if m.toast != "" {
		t.Errorf("matching clearToast should empty the toast, got %q", m.toast)
	}
}

// TestPollBaselineFromSnapshot: the poll baseline matches the stat the load
// observed (carried on the snapshot), so an append is never silently swallowed.
func TestPollBaselineFromSnapshot(t *testing.T) {
	root := t.TempDir()
	writeEventLog(t, root, event("e1", "2026-05-30T10:00:00Z", "session.started", "x"))
	m := loadModel(t, root)
	if m.pollSize != m.snap.EventLogSize || m.pollMod != m.snap.EventLogMod {
		t.Errorf("baseline should come from the snapshot's observed stat: base=(%d,%d) snap=(%d,%d)",
			m.pollSize, m.pollMod, m.snap.EventLogSize, m.snap.EventLogMod)
	}
	if m.snap.EventLogSize == 0 {
		t.Error("snapshot should record the observed event-log size")
	}
}
