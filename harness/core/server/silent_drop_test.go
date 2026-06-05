package server

import (
	"encoding/json"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/job"
	"github.com/mnemon-dev/mnemon/harness/core/kernel"
	"github.com/mnemon-dev/mnemon/harness/core/rule"
)

// MED#8: a Propose verdict carrying a nil Proposal must emit a diagnostic, never a silent drop. The deny and
// enqueue_job/request_evidence nil-payload branches already diagnose (S7); the Propose branch must be
// symmetric — otherwise a rule emitting {Verdict:Propose, Proposal:nil} produces zero durable evidence.
func TestProposeNilProposalEmitsDiagnostic(t *testing.T) {
	nilProp := rule.NewNativeRule("np", "agent", "memory.write.proposed", []string{"memory.observed"},
		func(rule.RuleInput) (contract.RuleDecision, error) {
			return contract.RuleDecision{Verdict: contract.VerdictPropose, Proposal: nil}, nil
		})
	s, _, cs := newServerWith(t, rule.NewRuleSet(nilProp))
	if _, _, err := cs.Ingest("agent", contract.ObservationEnvelope{ExternalID: "e1", Event: contract.Event{Type: "memory.observed", CorrelationID: "c1"}}); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if _, err := cs.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(diagEvents(t, s)) == 0 {
		t.Fatal("a propose verdict with a nil Proposal must emit a diagnostic (no silent drop, S7)")
	}
}

// MED#9: the job-lane crash-recovery re-mint must mirror the live path's no-silent-drop guarantee. If the
// proposal recorded in a completed effect's receipt is OUT OF SCOPE, recovery re-mints it, the bridge rejects
// it, and that reject must emit a stage:bridge diagnostic — not be swallowed while the row is acked (S7).
func TestLaneRecoveryOutOfScopeProposalEmitsDiagnostic(t *testing.T) {
	runner := job.NewFakeRunner(laneProposal())
	s, cs := newServerWithLane(t, rule.NewRuleSet(requestEvidenceRule()), runner)
	// Stage a receipt for effect key "ev-job" (the key requestEvidenceRule uses) whose recorded proposal writes
	// an OUT-OF-SCOPE ref — agent scope is {memory/m1}, this writes memory/m-evil. The lane will see the receipt
	// already exists (effect ran pre-crash) and take the recovery path.
	evilProp := &contract.ProposedEvent{Type: "memory.write.proposed", Payload: map[string]any{
		"writes": []contract.ResourceWrite{{Ref: contract.ResourceRef{Kind: "memory", ID: "m-evil"}, Kind: contract.OpCreate, Fields: map[string]any{"content": "x"}}}}}
	propJSON, _ := json.Marshal(evilProp)
	lk := kernel.NewKernel(s, kernel.DefaultSchemaGuard(), kernel.AuthorityRules{Allow: map[contract.ActorID][]contract.ResourceKind{"lane": {"receipt"}}})
	if d := lk.Apply(contract.KernelOp{OpID: "pre", Actor: "lane", Writes: []contract.ResourceWrite{
		{Ref: contract.ResourceRef{Kind: "receipt", ID: "ev-job"}, Kind: contract.OpCreate, Fields: map[string]any{"job_id": "job_k_ev-job", "effect_id": "ev-job", "outcome": "ok", "proposal": string(propJSON)}}}},
		contract.Modes{Conflict: contract.ConflictReject, Isolation: contract.IsolationWriteCAS, Authz: contract.AuthzStrict}); d.Status != contract.Accepted {
		t.Fatalf("pre-write receipt+proposal: %s", d.Reason)
	}
	if _, _, err := cs.Ingest("agent", contract.ObservationEnvelope{ExternalID: "e1", Event: contract.Event{Type: "memory.observed", CorrelationID: "c1"}}); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if _, err := cs.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if runner.Calls() != 0 {
		t.Fatalf("recovery must NOT re-run the effect; got %d", runner.Calls())
	}
	if v, _ := s.GetVersion(contract.ResourceRef{Kind: "memory", ID: "m-evil"}); v != 0 {
		t.Fatalf("out-of-scope recovery proposal must not be written; m-evil@%d", v)
	}
	if !hasDiagStage(t, s, "bridge") {
		t.Fatal("an out-of-scope re-minted proposal must emit a stage:bridge diagnostic (no silent drop, S7)")
	}
}
