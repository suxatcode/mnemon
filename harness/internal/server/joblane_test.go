package server

import (
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/job"
	"github.com/mnemon-dev/mnemon/harness/internal/kernel"
	"github.com/mnemon-dev/mnemon/harness/internal/rule"
	"github.com/mnemon-dev/mnemon/harness/internal/store"
)

// requestEvidenceRule asks the job lane to gather evidence (a fixed idempotency key) when none is present.
func requestEvidenceRule() rule.Rule {
	return rule.NewNativeRule("evidence", "agent", "memory.write.proposed", []string{"memory.observed"},
		func(in rule.RuleInput) (contract.RuleDecision, error) {
			if _, ok := in.Event.Payload["evidence"]; !ok {
				return contract.RuleDecision{Verdict: contract.VerdictRequestEvidence,
					Job: &contract.JobSpec{Kind: "gather", IdempotencyKey: "ev-job"}}, nil
			}
			return contract.RuleDecision{Verdict: contract.VerdictAllow}, nil
		})
}

func laneProposal() *contract.ProposedEvent {
	return &contract.ProposedEvent{Type: "memory.write.proposed", Payload: map[string]any{
		"writes": []contract.ResourceWrite{{Ref: contract.ResourceRef{Kind: "memory", ID: "m1"}, Kind: contract.OpUpdate, BasedOn: 1, Fields: map[string]any{"content": "evidence-gathered"}}}}}
}

func newServerWithLane(t *testing.T, rs rule.RuleSet, runner job.Runner) (*store.Store, *ControlServer) {
	t.Helper()
	s, err := store.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	rules := kernel.AuthorityRules{Allow: map[contract.ActorID][]contract.ResourceKind{
		"agent": {"memory"},
		"lane":  {"lease", "receipt"},
	}}
	k := kernel.NewKernel(s, kernel.DefaultSchemaGuard(), rules)
	cs := New(s, k, rs, agentSubs(), p0Modes(), seqGen(), fixedNow())
	n := int64(1000)
	cs.WithLane(runner, "lane", func() int64 { n++; return n }, 60)
	if d := k.Apply(contract.KernelOp{OpID: "seed", Actor: "agent", Writes: []contract.ResourceWrite{
		{Ref: contract.ResourceRef{Kind: "memory", ID: "m1"}, Kind: contract.OpCreate, Fields: map[string]any{"content": "v0"}}}}, p0Modes()); d.Status != contract.Accepted {
		t.Fatalf("seed: %s", d.Reason)
	}
	return s, cs
}

// S4 end-to-end: request_evidence -> outbox job -> fenced claim -> FakeRunner -> receipt -> proposal candidate
// minted as *.proposed -> kernel CAS Accepted. The kernel never touches the effect; only commits its result.
func TestJobLaneEndToEnd(t *testing.T) {
	s, cs := newServerWithLane(t, rule.NewRuleSet(requestEvidenceRule()), job.NewFakeRunner(laneProposal()))
	if _, _, err := cs.Ingest("agent", contract.ObservationEnvelope{ExternalID: "e1", Event: contract.Event{Type: "memory.observed", CorrelationID: "c1"}}); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	ds, err := cs.Tick()
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	accepted := false
	for _, d := range ds {
		if d.Status == contract.Accepted {
			accepted = true
		}
	}
	if !accepted {
		t.Fatalf("job-lane proposal candidate must reach CAS Accepted; got %+v", ds)
	}
	if v, _ := s.GetVersion(contract.ResourceRef{Kind: "memory", ID: "m1"}); v != 2 {
		t.Fatalf("lane-minted proposal must advance m1 to @2; got %d", v)
	}
	// the receipt is keyed by the idempotency key (the deterministic dedup identity), not the runner effect id.
	if v, fields, _ := s.GetResource(contract.ResourceRef{Kind: "receipt", ID: "job_k_ev-job"}); v != 1 || fields["outcome"] != "ok" {
		t.Fatalf("the effect must write a receipt keyed by the idempotency key; got v%d %v", v, fields)
	}
}

// S4 idempotency: a retried job (same IdempotencyKey) runs once — the outbox.idempotency_key UNIQUE prevents
// a second enqueue, so exactly one effect/receipt.
func TestIdempotentRetryIsNoop(t *testing.T) {
	runner := job.NewFakeRunner(laneProposal())
	s, cs := newServerWithLane(t, rule.NewRuleSet(requestEvidenceRule()), runner)
	if _, _, err := cs.Ingest("agent", contract.ObservationEnvelope{ExternalID: "e1", Event: contract.Event{Type: "memory.observed", CorrelationID: "c1"}}); err != nil {
		t.Fatalf("ingest1: %v", err)
	}
	if _, err := cs.Tick(); err != nil {
		t.Fatalf("tick1: %v", err)
	}
	// a second observation requests the SAME job key.
	if _, _, err := cs.Ingest("agent", contract.ObservationEnvelope{ExternalID: "e2", Event: contract.Event{Type: "memory.observed", CorrelationID: "c2"}}); err != nil {
		t.Fatalf("ingest2: %v", err)
	}
	if _, err := cs.Tick(); err != nil {
		t.Fatalf("tick2: %v", err)
	}
	if runner.Calls() != 1 {
		t.Fatalf("a retried job (same idempotency key) must run exactly once; got %d runs", runner.Calls())
	}
	_ = s
}
