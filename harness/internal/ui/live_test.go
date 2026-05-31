package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func writeEventLog(t *testing.T, root string, lines ...string) {
	t.Helper()
	mnemon := filepath.Join(root, ".mnemon")
	if err := os.MkdirAll(mnemon, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mnemon, "events.jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func event(id, ts, typ, summary string) string {
	return `{"schema_version":1,"id":"` + id + `","ts":"` + ts + `","type":"` + typ +
		`","loop":null,"host":null,"actor":"user","source":"test","correlation_id":"c","caused_by":null,"payload":{"summary":"` + summary + `"}}`
}

// TestLiveEvidencePoll proves an appended event becomes visible in Evidence via
// the poll path, without a keypress (the U3 live gate).
func TestLiveEvidencePoll(t *testing.T) {
	root := t.TempDir()
	writeEventLog(t, root, event("evt_1", "2026-05-30T10:00:00Z", "session.started", "first"))

	m := loadModel(t, root)
	m.active = pageEvidence
	if got := len(m.filteredEvidence()); got != 1 {
		t.Fatalf("expected 1 event at load, got %d", got)
	}
	if m.eventLogChanged() {
		t.Fatal("event log should match the load baseline")
	}

	// Append a new event out-of-band (as `lifecycle event append` would).
	writeEventLog(t, root,
		event("evt_1", "2026-05-30T10:00:00Z", "session.started", "first"),
		event("evt_2", "2026-05-30T10:05:00Z", "goal.planned", "second appeared"),
	)
	if !m.eventLogChanged() {
		t.Fatal("poll should detect the appended event")
	}

	// A poll tick (no keypress) triggers a reload; drive its reload cmd.
	cmd := m.handlePoll()
	if cmd == nil {
		t.Fatal("poll should schedule work")
	}
	m = drain(m, m.loadCmd())
	if got := len(m.filteredEvidence()); got != 2 {
		t.Fatalf("appended event should be visible after poll, got %d", got)
	}
	if out := m.View(); !strings.Contains(out, "second appeared") {
		t.Errorf("evidence should render the appended event; got:\n%s", out)
	}
}

// TestEvidenceFilter proves the Evidence filter narrows the stream by type.
func TestEvidenceFilter(t *testing.T) {
	root := t.TempDir()
	writeEventLog(t, root,
		event("e1", "2026-05-30T10:00:00Z", "goal.planned", "plan A"),
		event("e2", "2026-05-30T10:01:00Z", "session.started", "boot"),
		event("e3", "2026-05-30T10:02:00Z", "goal.completed", "done"),
	)
	m := loadModel(t, root)
	m.active = pageEvidence
	if got := len(m.filteredEvidence()); got != 3 {
		t.Fatalf("unfiltered should be 3, got %d", got)
	}
	m.evFilter = "goal."
	got := m.filteredEvidence()
	if len(got) != 2 {
		t.Fatalf("filter goal. should match 2, got %d", len(got))
	}
	for _, it := range got {
		if !strings.HasPrefix(it.title, "goal.") {
			t.Errorf("filtered item %q should start with goal.", it.title)
		}
	}
}

// TestFilterInputFlow proves typing a filter via the input commits to the active
// page filter.
func TestFilterInputFlow(t *testing.T) {
	root := t.TempDir()
	writeEventLog(t, root, event("e1", "2026-05-30T10:00:00Z", "goal.planned", "x"))
	m := loadModel(t, root)
	m.active = pageEvidence

	m = send(m, "/")
	if !m.filtering {
		t.Fatal("/ should enter filter mode")
	}
	for _, r := range "goal" {
		m = send(m, string(r))
	}
	m = send(m, "enter")
	if m.filtering {
		t.Error("enter should exit filter mode")
	}
	if m.evFilter != "goal" {
		t.Errorf("committed filter should be %q, got %q", "goal", m.evFilter)
	}
}

// TestColdStartRendersAllPages proves a fresh project (empty event log) renders
// all four pages without error.
func TestColdStartRendersAllPages(t *testing.T) {
	root := t.TempDir()
	// Fresh project: empty event log + the harness goals dir.
	mnemon := filepath.Join(root, ".mnemon")
	if err := os.MkdirAll(filepath.Join(mnemon, "harness", "goals"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mnemon, "events.jsonl"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	m := loadModel(t, root)
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = nm.(model)

	for _, p := range []pageID{pageScope, pageEvidence, pageProposals, pageProfile} {
		m.active = p
		out := m.View() // must not panic and must produce a frame
		if strings.TrimSpace(out) == "" {
			t.Errorf("page %s rendered empty on cold start", pageNames[p])
		}
	}
	// Spot-check the cold-start guidance.
	m.active = pageProposals
	if !strings.Contains(m.View(), "no proposals yet") {
		t.Error("proposals cold start should guide the operator")
	}
	m.active = pageEvidence
	if !strings.Contains(m.View(), "no evidence yet") {
		t.Error("evidence cold start should guide the operator")
	}
}
