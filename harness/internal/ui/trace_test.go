package ui

import (
	"strings"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/ui/read"
)

// appliedChainSnapshot builds a snapshot whose single proposal P1 was applied:
// evidence → proposal → review → apply audit → emitted event → projected entry.
func appliedChainSnapshot() read.Snapshot {
	return read.Snapshot{
		Proposals: []read.Proposal{{
			ID: "P1", Route: "memory", Status: "applied", Risk: "low", Title: "add tabs pref",
			Summary:      "prefer tabs",
			Evidence:     []read.EvidenceRef{{Type: "observation", Ref: "ev-e7"}},
			Review:       read.ReviewPolicy{Required: true, RequiredScope: "project", Reviewers: []string{"operator"}},
			DecisionRefs: []string{"decision-1"},
			AuditRefs:    []string{"audit://x/apply-1"},
			UpdatedAt:    "2026-05-30T10:00:00Z",
		}},
		Audits: []read.AuditRecord{{
			Audit: read.AuditDoc{Metadata: read.AuditMetadata{Name: "proposal-P1-apply-20260530T100000"},
				Spec: map[string]any{"audit_kind": "proposal.apply", "proposal_id": "P1"}},
			Path: "/x/apply-1.json",
			Ref:  map[string]any{"uri": "audit://x/apply-1"},
		}},
		Events: []read.Event{{
			ID: "evt-apply-1", TS: "2026-05-30T10:00:01Z", Type: "audit.recorded",
			Actor: "mnemon-manual", Source: "proposal.apply", CorrelationID: "proposal:P1",
			Payload: map[string]any{"outcome": "applied", "entry_id": "E1", "proposal_id": "P1"}, Raw: "{}",
		}},
		Profile: read.Profile{Entries: []read.ProfileEntry{{
			ID: "E1", Type: "preference", Summary: "tabs", Content: "use tabs",
			ProjectionTargets: []read.ProjectionTarget{{Host: "codex", Loop: "memory"}},
		}}},
	}
}

// TestTraceShowsAppliedChain proves the Trace page renders the full accountability
// chain for an applied proposal: evidence, the apply audit, and — the Band 0 gate
// requirement — the projection target the next run pulls.
func TestTraceShowsAppliedChain(t *testing.T) {
	m := withSnapshot(appliedChainSnapshot())
	m = send(m, "t") // trace the focal (only) proposal
	if m.active != pageTrace {
		t.Fatalf("t should open the Trace page, active=%d", m.active)
	}
	out := m.View()
	for _, want := range []string{"TRACE", "add tabs pref", "evidence", "apply audit", "audit://x/apply-1", "projection", "codex/memory"} {
		if !strings.Contains(out, want) {
			t.Errorf("trace view missing %q:\n%s", want, out)
		}
	}
}

// TestTraceJumpsToApplyAudit proves a navigable trace step jumps to the underlying
// record: selecting the apply-audit step and pressing enter lands on the Evidence
// page focused on that audit (the chain is navigable, not just visible).
func TestTraceJumpsToApplyAudit(t *testing.T) {
	m := withSnapshot(appliedChainSnapshot())
	m = send(m, "t") // open trace (sel = proposal node)
	m = send(m, "j") // move to the apply-audit step
	m = send(m, "enter")
	if m.active != pageEvidence {
		t.Fatalf("following the audit step should land on Evidence, active=%d", m.active)
	}
	if !m.evDetail {
		t.Error("the audit record should open in detail after the jump")
	}
}

// TestTraceEmptyWithoutFocus proves the page degrades gracefully with no proposal.
func TestTraceEmptyWithoutFocus(t *testing.T) {
	m := withSnapshot(read.Snapshot{})
	m = send(m, "5") // jump to Trace page with nothing to focus
	if !strings.Contains(m.View(), "no proposal selected") {
		t.Errorf("empty trace should explain how to focus a proposal:\n%s", m.View())
	}
}

// TestTraceClosesCoordinationLoop proves P4.3: the trace navigates a
// route=coordination applied proposal end to end — evidence, apply audit, the
// emitted topology event, and the coordination projection (hosts pull
// COORDINATION.json), same as the memory/eval routes.
func TestTraceClosesCoordinationLoop(t *testing.T) {
	snap := read.Snapshot{
		Proposals: []read.Proposal{{
			ID: "CP1", Route: "coordination", Status: "applied", Risk: "medium", Title: "Merge duplicate work: T1, T2",
			Evidence:  []read.EvidenceRef{{Type: "coordination", Ref: "E7"}},
			AuditRefs: []string{"audit://x/coord-apply-1"},
			UpdatedAt: "2026-05-30T10:00:00Z",
		}},
		Audits: []read.AuditRecord{{
			Audit: read.AuditDoc{Metadata: read.AuditMetadata{Name: "proposal-CP1-coordination-apply-20260530T100000"},
				Spec: map[string]any{"audit_kind": "proposal.apply", "proposal_id": "CP1"}},
			Path: "/x/coord-apply-1.json",
			Ref:  map[string]any{"uri": "audit://x/coord-apply-1"},
		}},
		Events: []read.Event{{
			ID: "evt-join", TS: "2026-05-30T10:00:01Z", Type: "task.joined",
			Actor: "mnemon-manual", Source: "proposal.apply", CorrelationID: "proposal:CP1",
			Payload: map[string]any{"task_id": "T2", "joined_into": "T1"}, Raw: "{}",
		}},
	}
	m := withSnapshot(snap)
	m = send(m, "t") // trace the focal coordination proposal
	out := m.View()
	for _, want := range []string{"TRACE", "Merge duplicate work", "apply audit", "audit://x/coord-apply-1", "task.joined", "coordination topology", "COORDINATION.json"} {
		if !strings.Contains(out, want) {
			t.Errorf("coordination trace missing %q:\n%s", want, out)
		}
	}
}

// TestScopeHealthRendersInHeader proves audit + anti-pattern health surface in the
// scope header beside projection health.
func TestScopeHealthRendersInHeader(t *testing.T) {
	snap := read.Snapshot{Scope: read.Scope{
		ProjectRoot: "/x", ProjectionHealth: "ok", AuditHealth: "ok", AntipatternHealth: "2 finding(s)",
	}}
	m := withSnapshot(snap)
	out := m.View()
	for _, want := range []string{"projection", "audit", "patterns", "2 finding(s)"} {
		if !strings.Contains(out, want) {
			t.Errorf("scope header missing health field %q:\n%s", want, out)
		}
	}
}
