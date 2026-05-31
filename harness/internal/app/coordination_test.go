package app

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/coordination"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/eventlog"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/proposal"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/proposalstore"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
)

func coordEvent(id, typ, host string, payload map[string]any) schema.Event {
	h := host
	loop := "coordination"
	return schema.Event{
		SchemaVersion: schema.Version,
		ID:            id,
		TS:            "2026-05-30T10:00:00Z",
		Type:          typ,
		Loop:          &loop,
		Host:          &h,
		Actor:         "host-agent",
		Source:        "test",
		CorrelationID: "c",
		Payload:       payload,
	}
}

// TestSupervisorProposesWithZeroDirectMutation is the Band 3 automated gate: a
// test stand-in supervisor reads the coordination topology and lands a
// route=coordination proposal in the review queue with ZERO direct mutation —
// the topology is unchanged and the only new events are proposal lifecycle
// events (no coordination event, no audit.recorded).
func TestSupervisorProposesWithZeroDirectMutation(t *testing.T) {
	root := t.TempDir()
	store, err := eventlog.New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	// Two tasks share evidence E7 -> a merge candidate the supervisor will flag.
	for _, ev := range []schema.Event{
		coordEvent("c1", coordination.EventTaskClaimed, "codex", map[string]any{coordination.FieldTaskID: "T1"}),
		coordEvent("c2", coordination.EventTaskClaimed, "claude-code", map[string]any{coordination.FieldTaskID: "T2"}),
		coordEvent("c3", coordination.EventEvidenceLinked, "codex", map[string]any{coordination.FieldTaskID: "T1", coordination.FieldEvidenceRef: "E7"}),
		coordEvent("c4", coordination.EventEvidenceLinked, "claude-code", map[string]any{coordination.FieldTaskID: "T2", coordination.FieldEvidenceRef: "E7"}),
	} {
		if err := store.Append(ev); err != nil {
			t.Fatalf("append %s: %v", ev.ID, err)
		}
	}

	before, err := store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	topoBefore := coordination.DeriveView(before)

	var out bytes.Buffer
	if err := New(root).SupervisorPropose(&out, "rule-standin"); err != nil {
		t.Fatalf("SupervisorPropose: %v", err)
	}

	// A route=coordination proposal landed in the review queue (a draft awaiting review).
	pstore, err := proposalstore.New(root)
	if err != nil {
		t.Fatalf("proposalstore.New: %v", err)
	}
	props, err := pstore.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var coord []proposal.Proposal
	for _, p := range props {
		if p.Route == proposal.RouteCoordination {
			coord = append(coord, p)
		}
	}
	if len(coord) != 1 {
		t.Fatalf("want 1 route=coordination proposal, got %d: %#v", len(coord), coord)
	}
	if coord[0].Status != proposal.StatusDraft {
		t.Errorf("supervisor proposal should be a draft for review, got %s", coord[0].Status)
	}
	if len(coord[0].Change.Operations) == 0 || coord[0].Change.Operations[0].Type != "coordination.merge" {
		t.Errorf("proposal missing the merge operation: %#v", coord[0].Change)
	}

	// ZERO direct mutation: the topology is unchanged. New events are proposal
	// lifecycle + the authorship audit (accountability, not mutation) — never a
	// coordination topology event.
	after, err := store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll after: %v", err)
	}
	topoAfter := coordination.DeriveView(after)
	if len(topoAfter.Tasks) != len(topoBefore.Tasks) || len(topoAfter.Conflicts) != len(topoBefore.Conflicts) {
		t.Errorf("supervisor mutated the topology: tasks %d->%d, conflicts %d->%d",
			len(topoBefore.Tasks), len(topoAfter.Tasks), len(topoBefore.Conflicts), len(topoAfter.Conflicts))
	}
	for _, ev := range after[len(before):] {
		if coordination.IsCoordinationType(ev.Type) {
			t.Errorf("supervisor emitted a coordination topology event %q — not zero direct mutation", ev.Type)
		}
	}
}

// TestSupervisorStampsAuthorship is the C2 / P3.4 gate: a supervisor-authored
// proposal carries its origin (kind + run correlation) on the proposal, and an
// authorship audit records the same origin + the context it read — so "which
// supervisor proposed this" survives a config swap.
func TestSupervisorStampsAuthorship(t *testing.T) {
	root := t.TempDir()
	store, err := eventlog.New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	for _, ev := range []schema.Event{
		coordEvent("c1", coordination.EventTaskClaimed, "codex", map[string]any{coordination.FieldTaskID: "T1"}),
		coordEvent("c2", coordination.EventTaskClaimed, "claude-code", map[string]any{coordination.FieldTaskID: "T2"}),
		coordEvent("c3", coordination.EventEvidenceLinked, "codex", map[string]any{coordination.FieldTaskID: "T1", coordination.FieldEvidenceRef: "E7"}),
		coordEvent("c4", coordination.EventEvidenceLinked, "claude-code", map[string]any{coordination.FieldTaskID: "T2", coordination.FieldEvidenceRef: "E7"}),
	} {
		if err := store.Append(ev); err != nil {
			t.Fatalf("append %s: %v", ev.ID, err)
		}
	}

	var out bytes.Buffer
	if err := New(root).SupervisorPropose(&out, "rule-standin"); err != nil {
		t.Fatalf("SupervisorPropose: %v", err)
	}

	// 1. The proposal carries the authorship origin.
	pstore, err := proposalstore.New(root)
	if err != nil {
		t.Fatalf("proposalstore.New: %v", err)
	}
	props, err := pstore.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var p *proposal.Proposal
	for i := range props {
		if props[i].Route == proposal.RouteCoordination {
			p = &props[i]
		}
	}
	if p == nil {
		t.Fatal("no coordination proposal created")
	}
	authorship, _ := p.Metadata["authorship"].(map[string]any)
	if authorship == nil {
		t.Fatalf("proposal missing authorship origin: %#v", p.Metadata)
	}
	if authorship["supervisor_kind"] != "rule-standin" {
		t.Errorf("authorship kind = %v, want rule-standin", authorship["supervisor_kind"])
	}
	run, _ := authorship["supervisor_run"].(string)
	if run == "" {
		t.Error("authorship missing supervisor_run correlation")
	}

	// 2. An authorship audit records the same origin + the context read.
	var buf bytes.Buffer
	if err := New(root).AuditList(&buf, "", "json"); err != nil {
		t.Fatalf("AuditList: %v", err)
	}
	if !strings.Contains(buf.String(), "supervisor.proposed") || !strings.Contains(buf.String(), "rule-standin") {
		t.Errorf("authorship audit missing supervisor origin:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), run) {
		t.Errorf("authorship audit missing the run correlation %q", run)
	}
}

// TestSupervisorPluggableByConfig proves swapping the supervisor is a config
// change: an unknown/external kind is rejected at config selection.
func TestSupervisorPluggableByConfig(t *testing.T) {
	var out bytes.Buffer
	if err := New(t.TempDir()).SupervisorPropose(&out, "bogus"); err == nil {
		t.Error("unknown supervisor kind should error at config selection")
	}
}

// TestCoordinationApplyClosesLoop is the Band 4 final-form gate (apply half): a
// supervisor-proposed merge, approved and applied via the facade path exactly as
// the U2 tests do, mutates the topology narrowly (T2 joined into T1), writes an
// audit, and back-links the audit ref — the coordination loop closes accountably.
func TestCoordinationApplyClosesLoop(t *testing.T) {
	root := t.TempDir()
	store, err := eventlog.New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	for _, ev := range []schema.Event{
		coordEvent("c1", coordination.EventTaskClaimed, "codex", map[string]any{coordination.FieldTaskID: "T1"}),
		coordEvent("c2", coordination.EventTaskClaimed, "claude-code", map[string]any{coordination.FieldTaskID: "T2"}),
		coordEvent("c3", coordination.EventEvidenceLinked, "codex", map[string]any{coordination.FieldTaskID: "T1", coordination.FieldEvidenceRef: "E7"}),
		coordEvent("c4", coordination.EventEvidenceLinked, "claude-code", map[string]any{coordination.FieldTaskID: "T2", coordination.FieldEvidenceRef: "E7"}),
	} {
		if err := store.Append(ev); err != nil {
			t.Fatalf("append %s: %v", ev.ID, err)
		}
	}

	h := New(root)
	var buf bytes.Buffer
	if err := h.SupervisorPropose(&buf, "rule-standin"); err != nil {
		t.Fatalf("SupervisorPropose: %v", err)
	}
	pstore, err := proposalstore.New(root)
	if err != nil {
		t.Fatalf("proposalstore.New: %v", err)
	}
	props, err := pstore.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	id := ""
	for _, p := range props {
		if p.Route == proposal.RouteCoordination {
			id = p.ID
		}
	}
	if id == "" {
		t.Fatal("supervisor did not create a coordination proposal")
	}

	// Approve through the facade path, exactly as the U2 governed tests do.
	for _, st := range []string{"open", "in_review", "approved"} {
		if err := h.ProposalTransition(&buf, id, st); err != nil {
			t.Fatalf("transition %s: %v", st, err)
		}
	}
	if err := h.ProposalApply(&buf, id); err != nil {
		t.Fatalf("apply: %v", err)
	}

	// 1. Topology mutated narrowly: T2 joined into T1.
	after, err := store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	view := coordination.DeriveView(after)
	var t2 *coordination.Task
	for i := range view.Tasks {
		if view.Tasks[i].ID == "T2" {
			t2 = &view.Tasks[i]
		}
	}
	if t2 == nil || t2.Status != "joined" || t2.JoinedInto != "T1" {
		t.Fatalf("expected T2 joined into T1, got %#v", t2)
	}

	// 2. Audit written + back-linked; proposal applied.
	applied, err := pstore.Load(id)
	if err != nil {
		t.Fatalf("Load applied: %v", err)
	}
	if applied.Status != proposal.StatusApplied {
		t.Errorf("status = %s, want applied", applied.Status)
	}
	if len(applied.AuditRefs) == 0 {
		t.Error("applied coordination proposal missing audit_refs")
	}

	// 3. The apply emitted a governed coordination event correlated to the proposal.
	foundJoin := false
	for _, ev := range after {
		if ev.Type == coordination.EventTaskJoined && ev.CorrelationID == "proposal:"+id {
			foundJoin = true
		}
	}
	if !foundJoin {
		t.Error("no task.joined topology event correlated to the proposal")
	}
}

// createApprovedCoord creates + approves a route=coordination proposal carrying
// one operation + payload (the governed manual path), but does not apply it.
func createApprovedCoord(t *testing.T, h *Harness, id, op, target string, payload map[string]any) {
	t.Helper()
	pj, _ := json.Marshal(payload)
	content := ProposalContent{
		Title:             op,
		Summary:           op,
		ChangeSummary:     op,
		Targets:           []string{"coordination=" + target},
		Operations:        []string{op + "=" + target + "=" + op + "=" + string(pj)},
		Evidence:          []string{"coordination=ev-" + id + "=evidence"},
		ValidationSummary: "human review before apply",
	}
	var buf bytes.Buffer
	if err := h.ProposalCreate(&buf, id, "coordination", "low", content); err != nil {
		t.Fatalf("create %s: %v", id, err)
	}
	for _, st := range []string{"open", "in_review", "approved"} {
		if err := h.ProposalTransition(&buf, id, st); err != nil {
			t.Fatalf("transition %s %s: %v", id, st, err)
		}
	}
}

// createApproveApplyCoord creates, approves, and applies a coordination proposal.
func createApproveApplyCoord(t *testing.T, h *Harness, id, op, target string, payload map[string]any) {
	t.Helper()
	createApprovedCoord(t, h, id, op, target, payload)
	var buf bytes.Buffer
	if err := h.ProposalApply(&buf, id); err != nil {
		t.Fatalf("apply %s: %v", id, err)
	}
}

// TestCoordinationApplyRejectsStale is a C4 gate: a coordination proposal whose op
// no longer applies (the topology moved between approval and apply) is rejected
// with a clear reason + a boundary audit, and is not applied.
func TestCoordinationApplyRejectsStale(t *testing.T) {
	root := t.TempDir()
	h := New(root)
	store, err := eventlog.New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for _, id := range []string{"T1", "T2", "T3"} {
		if err := store.Append(coordEvent("c-"+id, coordination.EventTaskClaimed, "codex", map[string]any{coordination.FieldTaskID: id})); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	// Proposal A (approved, not yet applied): merge T2 into T1.
	createApprovedCoord(t, h, "A", "coordination.merge", "coordination:merge/T2+T1", map[string]any{"tasks": []any{"T2"}, "into": "T1"})
	// Proposal B applies first and joins T2 into T3 — now A is stale.
	createApproveApplyCoord(t, h, "B", "coordination.merge", "coordination:merge/T2+T3", map[string]any{"tasks": []any{"T2"}, "into": "T3"})

	var buf bytes.Buffer
	if err := h.ProposalApply(&buf, "A"); err == nil {
		t.Fatal("a stale coordination apply must be rejected")
	} else if !strings.Contains(err.Error(), "already joined into T3") {
		t.Errorf("rejection should explain the conflict, got: %v", err)
	}
	pstore, _ := proposalstore.New(root)
	a, _ := pstore.Load("A")
	if a.Status != proposal.StatusApproved {
		t.Errorf("stale-rejected proposal should stay approved (not applied), got %s", a.Status)
	}
	var ab bytes.Buffer
	if err := New(root).AuditList(&ab, "", "json"); err != nil {
		t.Fatalf("AuditList: %v", err)
	}
	if !strings.Contains(ab.String(), "proposal.apply_rejected") {
		t.Errorf("stale reject should write a boundary audit:\n%s", ab.String())
	}
}

// TestCoordinationApplyIdempotent is a C4 gate: applying an already-satisfied op
// emits no new topology event (idempotent), while still recording the apply.
func TestCoordinationApplyIdempotent(t *testing.T) {
	root := t.TempDir()
	h := New(root)
	store, err := eventlog.New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := store.Append(coordEvent("c1", coordination.EventTaskClaimed, "codex", map[string]any{coordination.FieldTaskID: "T1"})); err != nil {
		t.Fatalf("seed: %v", err)
	}
	createApproveApplyCoord(t, h, "link1", "coordination.link", "coordination:link/T1+E1", map[string]any{"task_id": "T1", "evidence_ref": "E1"})
	linkedBefore := countEventType(coordReadAll(t, root), "evidence.linked")

	// A second proposal re-asserts the same link; applying it is idempotent.
	createApproveApplyCoord(t, h, "link2", "coordination.link", "coordination:link/T1+E1-again", map[string]any{"task_id": "T1", "evidence_ref": "E1"})
	after := coordReadAll(t, root)
	if got := countEventType(after, "evidence.linked"); got != linkedBefore {
		t.Errorf("idempotent re-link must emit no new evidence.linked event: %d -> %d", linkedBefore, got)
	}
	pstore, _ := proposalstore.New(root)
	p2, _ := pstore.Load("link2")
	if p2.Status != proposal.StatusApplied {
		t.Errorf("idempotent apply should still mark the proposal applied, got %s", p2.Status)
	}
	v := coordination.DeriveView(after)
	cnt := 0
	for _, tk := range v.Tasks {
		if tk.ID == "T1" {
			for _, e := range tk.EvidenceRefs {
				if e == "E1" {
					cnt++
				}
			}
		}
	}
	if cnt != 1 {
		t.Errorf("E1 should appear exactly once on T1 after idempotent re-link, got %d", cnt)
	}
}

func coordReadAll(t *testing.T, root string) []schema.Event {
	t.Helper()
	store, err := eventlog.New(root)
	if err != nil {
		t.Fatalf("eventlog.New: %v", err)
	}
	events, err := store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	return events
}

func taskHasEvidence(v coordination.View, taskID, ref string) bool {
	for _, tk := range v.Tasks {
		if tk.ID != taskID {
			continue
		}
		for _, e := range tk.EvidenceRefs {
			if e == ref {
				return true
			}
		}
	}
	return false
}

func viewGroupHasMember(v coordination.View, groupID, member string) bool {
	for _, g := range v.Groups {
		if g.ID != groupID {
			continue
		}
		for _, m := range g.Members {
			if m == member {
				return true
			}
		}
	}
	return false
}

func countEventType(events []schema.Event, typ string) int {
	n := 0
	for _, ev := range events {
		if ev.Type == typ {
			n++
		}
	}
	return n
}

// TestCoordinationCompensationRoundTrip is the C3 gate: link/unlink and member
// add/remove each round-trip through the governed apply path with audit, and the
// undo is a new compensating event — no event is ever deleted (the log only grows).
func TestCoordinationCompensationRoundTrip(t *testing.T) {
	root := t.TempDir()
	h := New(root)
	store, err := eventlog.New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := store.Append(coordEvent("c1", coordination.EventTaskClaimed, "codex", map[string]any{coordination.FieldTaskID: "T1"})); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := store.Append(coordEvent("g0", coordination.EventGroupCreated, "codex", map[string]any{coordination.FieldGroupID: "G1"})); err != nil {
		t.Fatalf("seed group: %v", err)
	}

	// link -> view has it
	createApproveApplyCoord(t, h, "link1", "coordination.link", "coordination:link/T1+E1", map[string]any{"task_id": "T1", "evidence_ref": "E1"})
	if !taskHasEvidence(coordination.DeriveView(coordReadAll(t, root)), "T1", "E1") {
		t.Fatal("link should attach E1 to T1")
	}
	n1 := len(coordReadAll(t, root))

	// unlink (compensation) -> view no longer has it; log only grew
	createApproveApplyCoord(t, h, "unlink1", "coordination.unlink", "coordination:unlink/T1+E1", map[string]any{"task_id": "T1", "evidence_ref": "E1"})
	after := coordReadAll(t, root)
	if taskHasEvidence(coordination.DeriveView(after), "T1", "E1") {
		t.Fatal("unlink should detach E1 from T1")
	}
	if len(after) <= n1 {
		t.Fatal("compensation must append a new event, never delete")
	}
	if countEventType(after, "evidence.linked") != 1 || countEventType(after, "evidence.unlinked") != 1 {
		t.Fatalf("both link + unlink events must remain in the log (linked=%d unlinked=%d)",
			countEventType(after, "evidence.linked"), countEventType(after, "evidence.unlinked"))
	}

	// member add -> view has it; member remove (compensation) -> view drops it
	createApproveApplyCoord(t, h, "madd", "coordination.member_add", "coordination:group/G1+claude", map[string]any{"group_id": "G1", "member": "claude-code"})
	if !viewGroupHasMember(coordination.DeriveView(coordReadAll(t, root)), "G1", "claude-code") {
		t.Fatal("member_add should add claude-code to G1")
	}
	createApproveApplyCoord(t, h, "mrem", "coordination.member_remove", "coordination:group/G1-claude", map[string]any{"group_id": "G1", "member": "claude-code"})
	if viewGroupHasMember(coordination.DeriveView(coordReadAll(t, root)), "G1", "claude-code") {
		t.Fatal("member_remove should drop claude-code from G1")
	}

	// Every compensation applied through the governed path: applied + audit_refs.
	pstore, err := proposalstore.New(root)
	if err != nil {
		t.Fatalf("proposalstore.New: %v", err)
	}
	for _, id := range []string{"link1", "unlink1", "madd", "mrem"} {
		p, err := pstore.Load(id)
		if err != nil {
			t.Fatalf("load %s: %v", id, err)
		}
		if p.Status != proposal.StatusApplied {
			t.Errorf("%s should be applied, got %s", id, p.Status)
		}
		if len(p.AuditRefs) == 0 {
			t.Errorf("%s missing audit_refs", id)
		}
	}
}
