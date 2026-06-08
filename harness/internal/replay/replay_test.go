package replay

import (
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/kernel"
	"github.com/mnemon-dev/mnemon/harness/internal/reconcile"
	"github.com/mnemon-dev/mnemon/harness/internal/rule"
	"github.com/mnemon-dev/mnemon/harness/internal/store"
)

func proposeWrite(id string, w contract.ResourceWrite) contract.Event {
	return contract.Event{ID: id, Type: "memory.write.proposed", Actor: "agent",
		Payload: map[string]any{"writes": []contract.ResourceWrite{w}}}
}

// liveDecisions produces decisions the canonical way: append the proposed events to a fresh kernel and
// reconcile (the same modes Replay uses), returning the store + decisions.
func liveDecisions(t *testing.T, events []contract.Event) (*store.Store, []contract.Decision) {
	t.Helper()
	s, err := store.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	k := kernel.NewKernel(s, kernel.DefaultSchemaGuard(), permissiveAuthority(events))
	r := reconcile.NewReconciler(s, k)
	for _, ev := range events {
		if _, err := s.AppendEvent(ev); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	return s, r.RunOnce(canonicalModes)
}

var sampleEvents = []contract.Event{
	proposeWrite("p1", contract.ResourceWrite{Ref: contract.ResourceRef{Kind: "memory", ID: "m1"}, Kind: contract.OpCreate, Fields: map[string]any{"content": "v1"}}),
	proposeWrite("p2", contract.ResourceWrite{Ref: contract.ResourceRef{Kind: "memory", ID: "m1"}, Kind: contract.OpUpdate, BasedOn: 1, Fields: map[string]any{"content": "v2"}}),
	proposeWrite("p3", contract.ResourceWrite{Ref: contract.ResourceRef{Kind: "memory", ID: "m1"}, Kind: contract.OpUpdate, BasedOn: 1, Fields: map[string]any{"content": "stale"}}), // stale -> conflict
}

// S8: replaying the event log over a FRESH throwaway kernel reproduces the live decisions, identical after
// masking the dynamic fields (DecisionID/AppliedAt).
func TestReplayReproducesDecisionsMasked(t *testing.T) {
	_, live := liveDecisions(t, sampleEvents)
	replayed := Replay(sampleEvents, rule.RuleSet{})
	if len(replayed) != len(live) || len(live) == 0 {
		t.Fatalf("replay must reproduce %d decisions; got %d", len(live), len(replayed))
	}
	for i := range live {
		l, r := maskDynamic(live[i]), maskDynamic(replayed[i])
		if l.Status != r.Status || l.OpID != r.OpID || l.IngestSeq != r.IngestSeq || l.NextAction != r.NextAction {
			t.Fatalf("decision %d differs after masking:\n live=%+v\n repl=%+v", i, l, r)
		}
	}
}

// S8: replay never touches a live store/cursor and is a pure function of the events (twice -> identical).
func TestReplayIsReadOnly(t *testing.T) {
	liveStore, _ := liveDecisions(t, sampleEvents)
	before := liveStore.DecisionCount()
	_ = Replay(sampleEvents, rule.RuleSet{})
	if liveStore.DecisionCount() != before {
		t.Fatalf("Replay must not mutate any live store; decision count %d -> %d", before, liveStore.DecisionCount())
	}
	a, b := Replay(sampleEvents, rule.RuleSet{}), Replay(sampleEvents, rule.RuleSet{})
	if len(a) != len(b) {
		t.Fatalf("Replay must be deterministic; got %d vs %d", len(a), len(b))
	}
	for i := range a {
		if maskDynamic(a[i]).Status != maskDynamic(b[i]).Status {
			t.Fatalf("Replay non-deterministic at %d", i)
		}
	}
}
