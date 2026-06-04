package server

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/job"
	"github.com/mnemon-dev/mnemon/harness/core/kernel"
	"github.com/mnemon-dev/mnemon/harness/core/rule"
)

type erroringRunner struct{}

func (erroringRunner) Run(contract.JobSpec) (job.Result, error) { return job.Result{}, errors.New("runner boom") }

// #9: PullProjection must serve only the actor's CONFIGURED scope; client-named out-of-scope refs are denied.
func TestPullProjectionEnforcesConfiguredScope(t *testing.T) {
	_, k, cs := newServerWith(t, rule.NewRuleSet(proposeRule()))
	// a resource the agent is NOT configured to see (agentSubs scope = {m1}).
	if d := k.Apply(contract.KernelOp{OpID: "secret", Actor: "agent", Writes: []contract.ResourceWrite{
		{Ref: contract.ResourceRef{Kind: "memory", ID: "secret"}, Kind: contract.OpCreate, Fields: map[string]any{"content": "top"}}}}, p0Modes()); d.Status != contract.Accepted {
		t.Fatalf("seed secret: %s", d.Reason)
	}
	proj, err := cs.PullProjection("agent", contract.Subscription{Actor: "agent", Refs: []contract.ResourceRef{{Kind: "memory", ID: "secret"}}})
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	for _, rv := range proj.Resources {
		if rv.Ref.ID == "secret" {
			t.Fatal("a client-named out-of-scope ref must NOT be served (S9: server enforces the configured scope)")
		}
	}
}

// #3: a job with an EMPTY idempotency key must not collide on the outbox id PK and poison the dispatch loop.
func TestEmptyIdempotencyKeyJobDoesNotPoison(t *testing.T) {
	emptyKeyRule := rule.NewNativeRule("ek", "agent", "memory.write.proposed", []string{"memory.observed"},
		func(rule.RuleInput) (contract.RuleDecision, error) {
			return contract.RuleDecision{Verdict: contract.VerdictEnqueueJob, Job: &contract.JobSpec{Kind: "gather", IdempotencyKey: ""}}, nil
		})
	s, cs := newServerWithLane(t, rule.NewRuleSet(emptyKeyRule), job.NewFakeRunner(nil))
	for _, id := range []string{"e1", "e2"} {
		if _, _, err := cs.Ingest("agent", contract.ObservationEnvelope{ExternalID: id, Event: contract.Event{Type: "memory.observed", CorrelationID: "c-" + id}}); err != nil {
			t.Fatalf("ingest %s: %v", id, err)
		}
	}
	if _, err := cs.Tick(); err != nil {
		t.Fatalf("two empty-key jobs must not poison the dispatch loop; got %v", err)
	}
	// both observations consumed (no memory.observed remains past the dispatch cursor — the lane may have
	// appended its own diagnostics, which is fine).
	evs, _ := s.PendingEvents(s.GetCursor("server_dispatch"))
	for _, ev := range evs {
		if ev.Type == "memory.observed" {
			t.Fatal("an observed event was not dispatched (poison loop)")
		}
	}
}

// #2/#4: a job whose receipt already exists (e.g. effect ran before a crash pre-ack) must NOT re-run; the
// outbox row drains (idempotent recovery, no infinite re-run wedge).
func TestLaneSkipsJobWithExistingReceipt(t *testing.T) {
	runner := job.NewFakeRunner(laneProposal())
	s, cs := newServerWithLane(t, rule.NewRuleSet(requestEvidenceRule()), runner)
	// pre-write the receipt keyed by the idempotency key (the deterministic dedup identity).
	lk := kernel.NewKernel(s, kernel.DefaultSchemaGuard(), kernel.AuthorityRules{Allow: map[contract.ActorID][]contract.ResourceKind{"lane": {"receipt"}}})
	if d := lk.Apply(contract.KernelOp{OpID: "pre", Actor: "lane", Writes: []contract.ResourceWrite{
		{Ref: contract.ResourceRef{Kind: "receipt", ID: "ev-job"}, Kind: contract.OpCreate, Fields: map[string]any{"job_id": "job_ev-job", "effect_id": "ev-job", "outcome": "ok"}}}},
		contract.Modes{Conflict: contract.ConflictReject, Isolation: contract.IsolationWriteCAS, Authz: contract.AuthzStrict}); d.Status != contract.Accepted {
		t.Fatalf("pre-write receipt: %s", d.Reason)
	}
	if _, _, err := cs.Ingest("agent", contract.ObservationEnvelope{ExternalID: "e1", Event: contract.Event{Type: "memory.observed", CorrelationID: "c1"}}); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if _, err := cs.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if runner.Calls() != 0 {
		t.Fatalf("a job whose receipt already exists must NOT re-run; got %d runs", runner.Calls())
	}
	claimed, _ := s.ClaimOutbox("probe", time.Minute)
	for _, r := range claimed {
		if r.Kind == "job" {
			t.Fatal("the job outbox row must be acked (drained) after an idempotent skip, not re-claimable")
		}
	}
}

// #6: a job-lane runner failure must emit a durable diagnostic (no silent drop, S7).
func TestLaneRunnerFailureEmitsDiagnostic(t *testing.T) {
	s, cs := newServerWithLane(t, rule.NewRuleSet(requestEvidenceRule()), erroringRunner{})
	if _, _, err := cs.Ingest("agent", contract.ObservationEnvelope{ExternalID: "e1", Event: contract.Event{Type: "memory.observed", CorrelationID: "c1"}}); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if _, err := cs.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if !hasDiagStage(t, s, "runner") {
		t.Fatal("a runner failure must emit a stage:runner diagnostic (no silent drop)")
	}
}

// #7: an out-of-scope lane-minted proposal dropped by the bridge must emit a diagnostic (no silent drop).
func TestLaneOutOfScopeProposalEmitsDiagnostic(t *testing.T) {
	evil := job.NewFakeRunner(&contract.ProposedEvent{Type: "memory.write.proposed", Payload: map[string]any{
		"writes": []contract.ResourceWrite{{Ref: contract.ResourceRef{Kind: "memory", ID: "m-evil"}, Kind: contract.OpCreate, Fields: map[string]any{"content": "x"}}}}})
	s, cs := newServerWithLane(t, rule.NewRuleSet(requestEvidenceRule()), evil)
	if _, _, err := cs.Ingest("agent", contract.ObservationEnvelope{ExternalID: "e1", Event: contract.Event{Type: "memory.observed", CorrelationID: "c1"}}); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if _, err := cs.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if !hasDiagStage(t, s, "bridge") {
		t.Fatal("an out-of-scope lane proposal must emit a stage:bridge diagnostic")
	}
	if v, _ := s.GetVersion(contract.ResourceRef{Kind: "memory", ID: "m-evil"}); v != 0 {
		t.Fatalf("out-of-scope lane write must not be created; got v%d", v)
	}
}

// #8: an enqueue_job/request_evidence verdict carrying a nil Job must emit a diagnostic (no silent drop).
func TestNilJobVerdictEmitsDiagnostic(t *testing.T) {
	nilJob := rule.NewNativeRule("nj", "agent", "memory.write.proposed", []string{"memory.observed"},
		func(rule.RuleInput) (contract.RuleDecision, error) {
			return contract.RuleDecision{Verdict: contract.VerdictRequestEvidence}, nil // no Job
		})
	s, _, cs := newServerWith(t, rule.NewRuleSet(nilJob))
	if _, _, err := cs.Ingest("agent", contract.ObservationEnvelope{ExternalID: "e1", Event: contract.Event{Type: "memory.observed", CorrelationID: "c1"}}); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if _, err := cs.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(diagEvents(t, s)) == 0 {
		t.Fatal("a request_evidence/enqueue_job verdict with a nil Job must emit a diagnostic (no silent drop)")
	}
}

// #5: concurrent Tick must be data-race-free and never double-dispatch (run under -race).
func TestConcurrentTickIsSafe(t *testing.T) {
	s, _, cs := newServerWith(t, rule.NewRuleSet(proposeRule()))
	if _, _, err := cs.Ingest("agent", contract.ObservationEnvelope{ExternalID: "e1", Event: contract.Event{Type: "memory.observed", CorrelationID: "c1"}}); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = cs.Tick() }()
	}
	wg.Wait()
	if v, _ := s.GetVersion(contract.ResourceRef{Kind: "memory", ID: "m1"}); v != 2 {
		t.Fatalf("concurrent Tick must dispatch the observation exactly once (m1 @2); got %d", v)
	}
}

// re-verify MED: keyed and keyless outbox id namespaces must be DISJOINT — a literal key "seq_<N>" must not
// collide with the keyless job_seq_<N> id PK and poison the dispatch loop.
func TestKeylessAndLiteralSeqKeyDoNotCollide(t *testing.T) {
	keyFromPayload := rule.NewNativeRule("k", "agent", "memory.write.proposed", []string{"memory.observed"},
		func(in rule.RuleInput) (contract.RuleDecision, error) {
			key, _ := in.Event.Payload["key"].(string)
			return contract.RuleDecision{Verdict: contract.VerdictEnqueueJob, Job: &contract.JobSpec{Kind: "g", IdempotencyKey: key}}, nil
		})
	s, cs := newServerWithLane(t, rule.NewRuleSet(keyFromPayload), job.NewFakeRunner(nil))
	// e1 (IngestSeq 1) -> keyless ; e2 -> keyed "seq_1" (would collide with keyless "job_seq_1" pre-fix).
	if _, _, err := cs.Ingest("agent", contract.ObservationEnvelope{ExternalID: "e1", Event: contract.Event{Type: "memory.observed", CorrelationID: "c1"}}); err != nil {
		t.Fatalf("ingest e1: %v", err)
	}
	if _, _, err := cs.Ingest("agent", contract.ObservationEnvelope{ExternalID: "e2", Event: contract.Event{Type: "memory.observed", CorrelationID: "c2", Payload: map[string]any{"key": "seq_1"}}}); err != nil {
		t.Fatalf("ingest e2: %v", err)
	}
	if _, err := cs.Tick(); err != nil {
		t.Fatalf("a keyless job and a literal seq_-key job must not collide on the outbox id; got %v", err)
	}
	for _, ev := range func() []contract.Event { e, _ := s.PendingEvents(s.GetCursor("server_dispatch")); return e }() {
		if ev.Type == "memory.observed" {
			t.Fatal("an observed event was not dispatched (poison loop)")
		}
	}
}

// re-verify MED: a crash between job.Finish (receipt committed) and the proposal mint must not lose the
// governed write — recovery re-mints the proposal from the receipt without re-running the effect.
func TestLaneRemintsProposalFromReceiptOnRecovery(t *testing.T) {
	runner := job.NewFakeRunner(laneProposal())
	s, cs := newServerWithLane(t, rule.NewRuleSet(requestEvidenceRule()), runner)
	propJSON, _ := json.Marshal(laneProposal())
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
	if v, _ := s.GetVersion(contract.ResourceRef{Kind: "memory", ID: "m1"}); v != 2 {
		t.Fatalf("recovery must re-mint the proposal from the receipt (m1 @2); got %d", v)
	}
}

// re-verify LOW: two distinct keyless jobs must each get a distinct receipt (keyed by the unique outbox row
// id) and both drain — neither wedges on a shared runner effect id.
func TestTwoKeylessJobsBothDrain(t *testing.T) {
	keyless := rule.NewNativeRule("kl", "agent", "memory.write.proposed", []string{"memory.observed"},
		func(rule.RuleInput) (contract.RuleDecision, error) {
			return contract.RuleDecision{Verdict: contract.VerdictEnqueueJob, Job: &contract.JobSpec{Kind: "g", IdempotencyKey: ""}}, nil
		})
	runner := job.NewFakeRunner(nil)
	s, cs := newServerWithLane(t, rule.NewRuleSet(keyless), runner)
	for _, id := range []string{"e1", "e2"} {
		if _, _, err := cs.Ingest("agent", contract.ObservationEnvelope{ExternalID: id, Event: contract.Event{Type: "memory.observed", CorrelationID: "c-" + id}}); err != nil {
			t.Fatalf("ingest %s: %v", id, err)
		}
	}
	if _, err := cs.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if runner.Calls() != 2 {
		t.Fatalf("both keyless jobs must run (distinct receipts); got %d", runner.Calls())
	}
	// each keyless job must own a DISTINCT receipt (keyed by its unique outbox row id job_s_<seq>); on the
	// broken path both collapse to the runner's shared effect id and the second wedges with no receipt.
	for _, id := range []contract.ResourceID{"job_s_1", "job_s_2"} {
		if v, _, _ := s.GetResource(contract.ResourceRef{Kind: "receipt", ID: id}); v != 1 {
			t.Fatalf("keyless job %q must own a distinct receipt; got v%d", id, v)
		}
	}
}

// re-verify MED: the job lane must claim ONLY job rows — it must not lease/churn invalidation rows (which it
// never delivers), or it starves a future S2 invalidation-delivery consumer.
func TestLaneDoesNotLeaseInvalidationRows(t *testing.T) {
	s, cs := newServerWithLane(t, rule.NewRuleSet(proposeRule()), job.NewFakeRunner(nil))
	if _, _, err := cs.Ingest("agent", contract.ObservationEnvelope{ExternalID: "e1", Event: contract.Event{Type: "memory.observed", CorrelationID: "c1"}}); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if _, err := cs.Tick(); err != nil { // propose accepted -> invalidation row enqueued; lane runs (no jobs)
		t.Fatalf("tick: %v", err)
	}
	// the invalidation row must still be claimable by its own delivery worker — not held by the lane.
	claimed, _ := s.ClaimOutbox("invalidation-worker", time.Minute, "invalidation")
	if len(claimed) != 1 || claimed[0].Kind != "invalidation" {
		t.Fatalf("an invalidation row must be claimable by its delivery worker, not leased by the lane; got %+v", claimed)
	}
}

// re-verify LOW: a VerdictWarn must surface its reasons as a diagnostic, not be silently dropped.
func TestWarnVerdictEmitsDiagnostic(t *testing.T) {
	warnRule := rule.NewNativeRule("w", "agent", "memory.write.proposed", []string{"memory.observed"},
		func(rule.RuleInput) (contract.RuleDecision, error) {
			return contract.RuleDecision{Verdict: contract.VerdictWarn, Reasons: []string{"heads up"}}, nil
		})
	s, _, cs := newServerWith(t, rule.NewRuleSet(warnRule))
	if _, _, err := cs.Ingest("agent", contract.ObservationEnvelope{ExternalID: "e1", Event: contract.Event{Type: "memory.observed", CorrelationID: "c1"}}); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if _, err := cs.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}
	found := false
	for _, dg := range diagEvents(t, s) {
		if reason, _ := dg.Payload["reason"].(string); strings.Contains(reason, "heads up") {
			found = true
		}
	}
	if !found {
		t.Fatal("a warn verdict must surface its reasons as a diagnostic (no silent warn)")
	}
}

// re-verify LOW: warn reasons must surface even when a higher verdict (propose) wins — never a silent warn.
func TestWarnReasonsSurfacedWhenProposeWins(t *testing.T) {
	warn := rule.NewNativeRule("warn", "agent", "x.proposed", []string{"memory.observed"},
		func(rule.RuleInput) (contract.RuleDecision, error) {
			return contract.RuleDecision{Verdict: contract.VerdictWarn, Reasons: []string{"DANGER"}}, nil
		})
	s, _, cs := newServerWith(t, rule.NewRuleSet(warn, proposeRule()))
	if _, _, err := cs.Ingest("agent", contract.ObservationEnvelope{ExternalID: "e1", Event: contract.Event{Type: "memory.observed", CorrelationID: "c1"}}); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if _, err := cs.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if v, _ := s.GetVersion(contract.ResourceRef{Kind: "memory", ID: "m1"}); v != 2 {
		t.Fatalf("propose must still win (m1 @2); got %d", v)
	}
	found := false
	for _, dg := range diagEvents(t, s) {
		if reason, _ := dg.Payload["reason"].(string); strings.Contains(reason, "DANGER") {
			found = true
		}
	}
	if !found {
		t.Fatal("warn reasons must surface as a diagnostic even when propose wins")
	}
}

func hasDiagStage(t *testing.T, s *kernel.Store, stage string) bool {
	t.Helper()
	for _, dg := range diagEvents(t, s) {
		if dg.Payload["stage"] == stage {
			return true
		}
	}
	return false
}
