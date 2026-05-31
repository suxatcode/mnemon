package ui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/app"
	"github.com/mnemon-dev/mnemon/harness/internal/ui/read"
)

// createApprovedCoordLink creates and approves a route=coordination link proposal
// (applies cleanly via the existing executor) for the bulk-apply test.
func createApprovedCoordLink(t *testing.T, root, id, taskID, ref string) {
	t.Helper()
	h := app.New(root)
	var buf bytes.Buffer
	content := app.ProposalContent{
		Title:             "Link " + ref + " to " + taskID,
		Summary:           "link evidence " + ref + " to " + taskID,
		ChangeSummary:     "link evidence",
		Targets:           []string{"coordination=coordination:link/" + taskID + "+" + ref},
		Operations:        []string{`coordination.link=coordination:link/` + taskID + `+` + ref + `=Link={"task_id":"` + taskID + `","evidence_ref":"` + ref + `"}`},
		Evidence:          []string{"coordination=" + ref + "=evidence"},
		ValidationSummary: "human review before apply",
	}
	if err := h.ProposalCreate(&buf, id, "coordination", "low", content); err != nil {
		t.Fatalf("create %s: %v", id, err)
	}
	for _, st := range []string{"open", "in_review", "approved"} {
		if err := h.ProposalTransition(&buf, id, st); err != nil {
			t.Fatalf("transition %s %s: %v", id, st, err)
		}
	}
}

func appliedCoordCount(m model) int {
	n := 0
	for _, p := range m.snap.Proposals {
		if p.Route == "coordination" && p.Status == "applied" {
			n++
		}
	}
	return n
}

// TestBulkApplyAppliesSelectedApproved is the C1 gate: a reviewer selects several
// approved proposals and applies them in one confirmed batch — each still through
// the governed apply path — and NOTHING applies until the human confirms.
func TestBulkApplyAppliesSelectedApproved(t *testing.T) {
	root := t.TempDir()
	createApprovedCoordLink(t, root, "cl1", "T1", "E1")
	createApprovedCoordLink(t, root, "cl2", "T2", "E2")

	m := loadModel(t, root)
	m.active = pageProposals
	m = step(m, " ") // select first approved proposal
	m = step(m, "j")
	m = step(m, " ") // select second
	if m.selectedCount() != 2 {
		t.Fatalf("want 2 selected, got %d", m.selectedCount())
	}

	// B opens the batch confirm — it must NOT apply anything yet.
	m = step(m, "B")
	if m.confirm == nil {
		t.Fatal("B should open a bulk-apply confirm")
	}
	if got := appliedCoordCount(m); got != 0 {
		t.Fatalf("nothing must apply before the human confirms; %d already applied", got)
	}

	// Confirm: now both apply through the governed path.
	m = step(m, "y")
	if got := appliedCoordCount(m); got != 2 {
		t.Fatalf("bulk apply should have applied both, got %d applied", got)
	}
}

// TestBulkApplyNoSelectionDoesNothing proves B with no selection opens no apply.
func TestBulkApplyNoSelectionDoesNothing(t *testing.T) {
	snap := read.Snapshot{Proposals: []read.Proposal{
		{ID: "p1", Route: "coordination", Status: "approved", Risk: "low", Title: "link evidence",
			Change: read.ChangeRequest{Operations: []read.Operation{{Type: "coordination.link"}}}, UpdatedAt: "2026-05-31T10:00:00Z"},
		{ID: "p2", Route: "coordination", Status: "approved", Risk: "medium", Title: "merge tasks",
			Change: read.ChangeRequest{Operations: []read.Operation{{Type: "coordination.merge"}}}, UpdatedAt: "2026-05-31T09:00:00Z"},
	}}
	m := withSnapshot(snap)
	m.active = pageProposals
	out := m.View()
	if !strings.Contains(out, "safe") || !strings.Contains(out, "review") {
		t.Errorf("proposals view should show the deterministic safe/review badges:\n%s", out)
	}
	m = send(m, "B")
	if m.confirm != nil {
		t.Error("B with no selection must not open an apply confirm (nothing auto-applies)")
	}
}
