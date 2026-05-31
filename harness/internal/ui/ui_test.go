package ui

import (
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mnemon-dev/mnemon/harness/internal/app"
	"github.com/mnemon-dev/mnemon/harness/internal/ui/read"
)

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve caller path")
	}
	dir := filepath.Dir(thisFile) // .../harness/internal/ui
	for i := 0; i < 3; i++ {
		dir = filepath.Dir(dir)
	}
	return dir // module root
}

func keyOf(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func send(m model, key string) model {
	nm, _ := m.Update(keyOf(key))
	return nm.(model)
}

func withSnapshot(snap read.Snapshot) model {
	m := newModel(".")
	nm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = nm.(model)
	nm, _ = m.Update(snapshotMsg{snap: snap})
	return nm.(model)
}

// TestPagesRenderRealData proves all four pages render real .mnemon data (0 mock).
func TestPagesRenderRealData(t *testing.T) {
	root := moduleRoot(t)
	snap := read.Load(root)
	if snap.Err.Events != nil || len(snap.Events) == 0 {
		t.Skipf("no real events to render: %v", snap.Err.Events)
	}
	m := withSnapshot(snap)

	// Scope shows the dogfood goal.
	m.active = pageScope
	if out := m.View(); !strings.Contains(out, "harness-ui-console") {
		t.Errorf("scope page should list the dogfood goal; got:\n%s", out)
	}

	// Evidence shows a real recorded event type.
	m.active = pageEvidence
	if out := m.View(); !strings.Contains(out, "EVIDENCE") || !strings.Contains(out, "goal.") {
		t.Errorf("evidence page should show real lifecycle events; got:\n%s", out)
	}

	// Proposals shows a real draft proposal title.
	m.active = pageProposals
	if out := m.View(); !strings.Contains(out, "Review memory eval outcome") {
		t.Errorf("proposals page should show a real proposal title; got:\n%s", out)
	}

	// Header carries the live project root.
	if out := m.View(); !strings.Contains(out, "mnemon") {
		t.Errorf("header should render scope; got:\n%s", out)
	}
}

// TestEvidenceToProposalLink proves the evidence → proposal forward link
// navigates to the linked proposal.
func TestEvidenceToProposalLink(t *testing.T) {
	snap := read.Snapshot{
		Proposals: []read.Proposal{
			{ID: "P1", Route: "memory", Status: "open", Risk: "low", Title: "First", UpdatedAt: "2026-05-30T10:00:00Z"},
		},
		Events: []read.Event{
			{
				ID: "evt_apply", TS: "2026-05-30T11:00:00Z", Type: "proposal.applied",
				Actor: "mnemon-manual", Source: "mnemon", CorrelationID: "c",
				Payload: map[string]any{"proposal_id": "P1", "summary": "applied P1"},
				Raw:     `{"id":"evt_apply"}`,
			},
		},
	}
	m := withSnapshot(snap)
	m.active = pageEvidence
	m = send(m, "enter") // open evidence detail
	if !m.evDetail {
		t.Fatal("evidence detail should open on enter")
	}
	m = send(m, "enter") // follow link
	if m.active != pageProposals {
		t.Fatalf("following the link should switch to Proposals, got page %d", m.active)
	}
	if got := m.orderedProposals()[m.prSel].ID; got != "P1" {
		t.Errorf("should focus proposal P1, focused %q", got)
	}
	if !m.prDetail {
		t.Error("linked proposal should open in detail")
	}
}

// TestProposalToAuditLink proves the proposal → audit forward link navigates to
// the matching audit record in Evidence, closing the evidence→proposal→audit
// trace.
func TestProposalToAuditLink(t *testing.T) {
	uri := ".mnemon/harness/audit/records/proposal-P1-apply-20260530T120000000000000.json"
	snap := read.Snapshot{
		Proposals: []read.Proposal{
			{ID: "P1", Route: "memory", Status: "applied", Risk: "low", Title: "First",
				UpdatedAt: "2026-05-30T12:00:00Z", AuditRefs: []string{uri}},
		},
		Audits: []read.AuditRecord{
			{
				Audit: read.AuditDoc{
					Metadata: read.AuditMetadata{Name: "proposal-P1-apply-20260530T120000000000000"},
					Spec:     map[string]any{"audit_kind": "proposal.apply", "decision": "applied"},
				},
				Path: "/abs/" + uri,
				Ref:  map[string]any{"uri": uri},
			},
		},
	}
	m := withSnapshot(snap)
	m.active = pageProposals
	m = send(m, "enter") // open proposal detail
	if !m.prDetail {
		t.Fatal("proposal detail should open")
	}
	m = send(m, "enter") // follow audit link
	if m.active != pageEvidence {
		t.Fatalf("following audit_refs should switch to Evidence, got page %d", m.active)
	}
	items := m.evidenceItems()
	if m.evSel >= len(items) || items[m.evSel].kind != "audit" {
		t.Fatalf("should focus the audit evidence item, got sel %d", m.evSel)
	}
}

// TestProfilePaneDegradesIndependently proves a failed profile section renders as
// unavailable while other panes keep rendering real content.
func TestProfilePaneDegradesIndependently(t *testing.T) {
	snap := read.Snapshot{
		Proposals: []read.Proposal{
			{ID: "P1", Route: "memory", Status: "open", Risk: "low", Title: "Visible proposal", UpdatedAt: "2026-05-30T10:00:00Z"},
		},
		Err: read.SectionErrors{Profile: errors.New("profile.json missing")},
	}
	m := withSnapshot(snap)

	m.active = pageProfile
	profOut := m.View()
	if !strings.Contains(profOut, "no profile") {
		t.Errorf("profile pane should degrade to a cold-start/unavailable message; got:\n%s", profOut)
	}

	m.active = pageProposals
	if propOut := m.View(); !strings.Contains(propOut, "Visible proposal") {
		t.Errorf("proposals pane should keep rendering despite profile failure; got:\n%s", propOut)
	}
}

// TestGoalViewUsesFacadeType is a compile-time guard that GoalView embeds the
// facade's status view (the surface uses app types directly for structured
// returns).
func TestGoalViewUsesFacadeType(t *testing.T) {
	var gv read.GoalView
	gv.GoalStatusView = app.GoalStatusView{ID: "x", Status: "active"}
	if gv.ID != "x" {
		t.Fatal("GoalView should embed app.GoalStatusView")
	}
}
