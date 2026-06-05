package server

import (
	"context"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/job"
	"github.com/mnemon-dev/mnemon/harness/core/kernel"
	"github.com/mnemon-dev/mnemon/harness/core/replay"
	"github.com/mnemon-dev/mnemon/harness/core/rule"
	wasmrule "github.com/mnemon-dev/mnemon/harness/core/rule/wasm"
)

// TestFullChainWithWasmRule drives the WHOLE control plane through a real wazero WASM rule, in one test:
// two edges over httptest -> wasm deny (no evidence) + wasm propose (evidence) -> CAS Accept + a cross-edge
// CONFLICT on m1 -> scoped projection -> a request_evidence job lane -> FakeRunner -> receipt -> proposal ->
// CAS Accept of m2 -> a content-tampered readback caught -> Replay reproduces the decisions masked.
func TestFullChainWithWasmRule(t *testing.T) {
	wasmBytes, err := os.ReadFile("../rule/wasm/testdata/rule_allow_if_evidence.wasm")
	if err != nil {
		t.Fatalf("read wasm: %v", err)
	}
	wr, err := wasmrule.New(context.Background(), wasmBytes, wasmrule.Limits{Timeout: 100 * time.Millisecond, MemPages: 16})
	if err != nil {
		t.Fatalf("wasm new: %v", err)
	}
	gatherRule := rule.NewNativeRule("gather", "agent", "memory.write.proposed", []string{"gather.observed"},
		func(rule.RuleInput) (contract.RuleDecision, error) {
			return contract.RuleDecision{Verdict: contract.VerdictRequestEvidence, Job: &contract.JobSpec{Kind: "gather", IdempotencyKey: "gather-1"}}, nil
		})

	s, err := kernel.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	k := kernel.NewKernel(s, kernel.DefaultSchemaGuard(), kernel.AuthorityRules{Allow: map[contract.ActorID][]contract.ResourceKind{
		"agent": {"memory"}, "lane": {"lease", "receipt"},
	}})
	subs := map[contract.ActorID]contract.Subscription{
		"agent": {Actor: "agent", Refs: []contract.ResourceRef{{Kind: "memory", ID: "m1"}, {Kind: "memory", ID: "m2"}}},
	}
	runner := job.NewFakeRunner(&contract.ProposedEvent{Type: "memory.write.proposed", Payload: map[string]any{
		"writes": []contract.ResourceWrite{{Ref: contract.ResourceRef{Kind: "memory", ID: "m2"}, Kind: contract.OpCreate, Fields: map[string]any{"content": "from-runner"}}}}})
	cs := New(s, k, rule.NewRuleSet(wr, gatherRule), subs, p0Modes(), seqGen(), fixedNow())
	n := int64(1000)
	cs.WithLane(runner, "lane", func() int64 { n++; return n }, 60)
	// bootstrap m1 via a trusted *.proposed event so the canonical log FULLY describes the state — Replay can
	// then reproduce the whole chain from zero (a direct Apply seed would be invisible to the event log).
	if _, err := s.AppendEvent(contract.Event{ID: "boot", Type: "memory.write.proposed", Actor: "agent",
		Payload: map[string]any{"writes": []contract.ResourceWrite{{Ref: contract.ResourceRef{Kind: "memory", ID: "m1"}, Kind: contract.OpCreate, Fields: map[string]any{"content": "v0"}}}}}); err != nil {
		t.Fatalf("boot: %v", err)
	}
	if _, err := cs.Tick(); err != nil { // reconcile the bootstrap so m1@1 exists before the edges propose
		t.Fatalf("boot tick: %v", err)
	}

	srv := httptest.NewServer(NewHTTPHandler(cs))
	defer srv.Close()
	edgeA := NewClient(srv.URL, "agent")
	edgeB := NewClient(srv.URL, "agent")

	obs := func(c *Client, ext, typ, corr string, payload map[string]any) {
		if _, _, err := c.Ingest("agent", contract.ObservationEnvelope{ExternalID: ext, Event: contract.Event{Type: typ, CorrelationID: corr, Payload: payload}}); err != nil {
			t.Fatalf("ingest %s: %v", ext, err)
		}
	}
	obs(edgeA, "a1", "memory.observed", "ca", nil)                              // no evidence -> wasm DENY
	obs(edgeB, "b1", "memory.observed", "cb", map[string]any{"evidence": "x"}) // evidence -> wasm PROPOSE m1
	obs(edgeA, "b2", "memory.observed", "cc", map[string]any{"evidence": "y"}) // evidence -> wasm PROPOSE m1 (conflicts)
	obs(edgeB, "g1", "gather.observed", "cg", nil)                             // -> request_evidence -> job lane

	ds, err := cs.Tick()
	if err != nil {
		t.Fatalf("tick: %v", err)
	}

	if v, _ := s.GetVersion(contract.ResourceRef{Kind: "memory", ID: "m1"}); v != 2 {
		t.Fatalf("wasm propose must advance m1 to @2 via CAS; got %d", v)
	}
	if v, _ := s.GetVersion(contract.ResourceRef{Kind: "memory", ID: "m2"}); v != 1 {
		t.Fatalf("job lane (FakeRunner) must create m2@1; got %d", v)
	}
	if v, f, _ := s.GetResource(contract.ResourceRef{Kind: "receipt", ID: "job_k_gather-1"}); v != 1 || f["outcome"] != "ok" {
		t.Fatalf("the job must write a receipt; got v%d %v", v, f)
	}
	var accepted, deferred int
	for _, d := range ds {
		switch d.Status {
		case contract.Accepted:
			accepted++
		case contract.Deferred:
			deferred++
		}
	}
	if accepted < 2 || deferred < 1 {
		t.Fatalf("chain must Accept (m1 propose + m2 lane) and Defer (the conflicting m1 propose); got %d accept, %d defer", accepted, deferred)
	}
	hasDiag := func(pred func(reason string) bool) bool {
		for _, dg := range diagEvents(t, s) {
			if r, _ := dg.Payload["reason"].(string); pred(r) {
				return true
			}
		}
		return false
	}
	if !hasDiagStage(t, s, "rule") {
		t.Fatal("the wasm deny must leave a stage:rule diagnostic")
	}
	if !hasDiag(func(r string) bool { return strings.Contains(r, "memory/m1") && strings.Contains(r, "actual v2") }) {
		t.Fatal("the cross-edge conflict must leave a diagnostic naming the raced version")
	}

	// content-tampered readback: pull the scoped projection, then observe with a tampered digest -> blocked.
	proj, err := cs.PullProjection("agent", subs["agent"])
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	obs(edgeB, "tamper", "memory.observed", "ct", map[string]any{"evidence": "x"})
	// re-stamp the tampered digest by ingesting an envelope whose event carries a bad ContextDigest:
	if _, _, err := edgeB.Ingest("agent", contract.ObservationEnvelope{ExternalID: "tamper2", Event: contract.Event{Type: "memory.observed", CorrelationID: "ct2", ContextDigest: "tampered-" + proj.Digest, Payload: map[string]any{"evidence": "x"}}}); err != nil {
		t.Fatalf("ingest tamper: %v", err)
	}
	if _, err := cs.Tick(); err != nil {
		t.Fatalf("tick2: %v", err)
	}
	if !hasDiag(func(r string) bool { return strings.Contains(r, "echoed digest") }) {
		t.Fatal("a content-tampered readback must be caught with a diagnostic")
	}

	// Replay the canonical log -> reproduces decisions deterministically (masked).
	evs, _ := s.PendingEvents(0)
	rep := replay.Replay(evs, rule.RuleSet{})
	if len(rep) == 0 {
		t.Fatal("Replay must reproduce decisions from the canonical log")
	}
	repAccept := 0
	for _, d := range rep {
		if d.Status == contract.Accepted {
			repAccept++
		}
	}
	if repAccept == 0 {
		t.Fatal("Replay must reproduce the accepted writes")
	}
}
