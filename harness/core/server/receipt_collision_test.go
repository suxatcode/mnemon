package server

import (
	"testing"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/job"
	"github.com/mnemon-dev/mnemon/harness/core/rule"
)

// MED#7 (S4): the outbox-id namespaces are disjoint ("job_k_"+key vs "job_s_"+seq), but keying the receipt by
// the RAW idempotency key reopened the collision one layer down: a keyless job keys its receipt by its row id
// "job_s_<seq>", and a keyed job whose literal IdempotencyKey == "job_s_<seq>" (payload-derivable) forged that
// same receipt identity — so only one effect ran and the other was silently skipped as a "duplicate", its
// governed proposal lost. Keying the receipt by the (already-disjoint) outbox ROW ID closes it: two distinct
// jobs get two distinct receipts and both effects run.
func TestReceiptKeyCrossJobNoCollision(t *testing.T) {
	keyFromPayload := rule.NewNativeRule("k", "agent", "memory.write.proposed", []string{"memory.observed"},
		func(in rule.RuleInput) (contract.RuleDecision, error) {
			key, _ := in.Event.Payload["key"].(string)
			return contract.RuleDecision{Verdict: contract.VerdictEnqueueJob, Job: &contract.JobSpec{Kind: "g", IdempotencyKey: key}}, nil
		})
	runner := job.NewFakeRunner(nil)
	s, cs := newServerWithLane(t, rule.NewRuleSet(keyFromPayload), runner)
	// e1 (IngestSeq 1) -> keyless job; outbox id "job_s_1"; its receipt identity must stay "job_s_1"-scoped.
	if _, _, err := cs.Ingest("agent", contract.ObservationEnvelope{ExternalID: "e1", Event: contract.Event{Type: "memory.observed", CorrelationID: "c1"}}); err != nil {
		t.Fatalf("ingest e1: %v", err)
	}
	// e2 keyed with the literal "job_s_1" -> outbox id "job_k_job_s_1"; pre-fix it forged the receipt "job_s_1".
	if _, _, err := cs.Ingest("agent", contract.ObservationEnvelope{ExternalID: "e2", Event: contract.Event{Type: "memory.observed", CorrelationID: "c2", Payload: map[string]any{"key": "job_s_1"}}}); err != nil {
		t.Fatalf("ingest e2: %v", err)
	}
	if _, err := cs.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if runner.Calls() != 2 {
		t.Fatalf("two distinct jobs must each run a distinct effect; got %d (receipt-key collision dropped one)", runner.Calls())
	}
	// each owns a distinct receipt: the keyless one under "job_s_1", the keyed one under "job_k_job_s_1".
	for _, id := range []contract.ResourceID{"job_s_1", "job_k_job_s_1"} {
		if v, _, _ := s.GetResource(contract.ResourceRef{Kind: "receipt", ID: id}); v != 1 {
			t.Fatalf("expected a distinct receipt %q; got v%d", id, v)
		}
	}
}
