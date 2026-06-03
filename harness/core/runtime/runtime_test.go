package runtime

import (
	"testing"

	"github.com/mnemon-dev/mnemon/harness/core/callback"
	"github.com/mnemon-dev/mnemon/harness/core/config"
	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/kernel"
	"github.com/mnemon-dev/mnemon/harness/core/projection"
)

// memoryWriter proposes updating the single resource in its read-set to a new content, based_on the
// version it saw (point-in-time read-set; projection_read_set isolation catches a stale premise).
func memoryWriter() callback.Callback {
	return callback.BuiltinFunc(func(ev contract.Event, view projection.Projection) ([]contract.ProposedEvent, error) {
		if len(view.Resources) == 0 {
			return nil, nil
		}
		rv := view.Resources[0]
		return []contract.ProposedEvent{{Type: "memory.write.proposed", Payload: map[string]any{
			"writes": []contract.ResourceWrite{{Ref: rv.Ref, Kind: contract.OpUpdate, BasedOn: rv.Version, Fields: map[string]any{"content": "derived"}}},
		}}}, nil
	})
}

func newRuntime(t *testing.T) (*kernel.Store, *Runtime) {
	t.Helper()
	s, err := kernel.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	cfg := config.RuntimeConfig{
		SchemaVersion: 1,
		Modes:         config.ModeConfig{Conflict: "rebase", Isolation: "projection_read_set", Authz: "strict"},
		Actors:        map[contract.ActorID][]contract.ResourceKind{"agent": {"memory"}},
		Bindings:      []config.BindingConfig{{EventType: "memory.observed", Callback: "memory-writer", Actor: "agent", Emits: "memory.write.proposed"}},
		Scopes:        map[contract.ActorID][]contract.ResourceRef{"agent": {{Kind: "memory", ID: "m1"}}},
	}
	resolved, err := config.Resolve(cfg, map[string]callback.Callback{"memory-writer": memoryWriter()})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	// seed m1@1 via the kernel (trusted setup)
	k := kernel.NewKernel(s, kernel.DefaultSchemaGuard(), resolved.Rules)
	if d := k.Apply(contract.KernelOp{OpID: "seed", Actor: "agent", Writes: []contract.ResourceWrite{
		{Ref: contract.ResourceRef{Kind: "memory", ID: "m1"}, Kind: contract.OpCreate, Fields: map[string]any{"content": "v0"}}}}, resolved.Modes); d.Status != contract.Accepted {
		t.Fatalf("seed: %s", d.Reason)
	}
	return s, New(s, resolved, seqGen(), fixedNow())
}

func TestMinimalMemoryLoop(t *testing.T) {
	s, rt := newRuntime(t)
	// an observed event triggers the callback
	if _, err := s.AppendEvent(contract.Event{ID: "obs1", Type: "memory.observed", CorrelationID: "c1"}); err != nil {
		t.Fatalf("append: %v", err)
	}
	ds, err := rt.Tick()
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(ds) != 1 || ds[0].Status != contract.Accepted {
		t.Fatalf("observed->callback->bridge->reconcile->kernel must Accept; got %+v", ds)
	}
	if v, _ := s.GetVersion(contract.ResourceRef{Kind: "memory", ID: "m1"}); v != 2 {
		t.Fatalf("m1 must advance to @2, got %d", v)
	}
	if projection.Build(s, []contract.ResourceRef{{Kind: "memory", ID: "m1"}}, "agent").Resources[0].Version != 2 {
		t.Fatal("next projection must show the new version")
	}
}

func TestTickIsExactlyOnceAcrossRestart(t *testing.T) {
	s, rt := newRuntime(t)
	if _, err := s.AppendEvent(contract.Event{ID: "obs1", Type: "memory.observed", CorrelationID: "c1"}); err != nil {
		t.Fatalf("append: %v", err)
	}
	_, _ = rt.Tick()
	before := s.DecisionCount()
	// "restart": a fresh Runtime over the same store must NOT re-dispatch obs1
	rt2 := New(s, rt.resolved, seqGen(), fixedNow())
	ds, _ := rt2.Tick()
	if len(ds) != 0 {
		t.Fatalf("restart re-dispatched an already-dispatched observation, got %d decisions", len(ds))
	}
	if s.DecisionCount() != before {
		t.Fatalf("restart polluted state: %d -> %d", before, s.DecisionCount())
	}
}

// R11 end-to-end: a callback that proposes a write OUTSIDE its scope yields NO decision and NO state
// change (the bridge blocks it before it can become an event).
func TestOutOfScopeProposalYieldsNoDecision(t *testing.T) {
	s, err := kernel.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	evilWriter := callback.BuiltinFunc(func(contract.Event, projection.Projection) ([]contract.ProposedEvent, error) {
		return []contract.ProposedEvent{{Type: "memory.write.proposed", Payload: map[string]any{
			"writes": []contract.ResourceWrite{{Ref: contract.ResourceRef{Kind: "memory", ID: "m-evil"}, Kind: contract.OpCreate, Fields: map[string]any{"content": "x"}}}}}}, nil
	})
	cfg := config.RuntimeConfig{SchemaVersion: 1,
		Modes:    config.ModeConfig{Conflict: "rebase", Isolation: "projection_read_set", Authz: "strict"},
		Actors:   map[contract.ActorID][]contract.ResourceKind{"agent": {"memory"}},
		Bindings: []config.BindingConfig{{EventType: "memory.observed", Callback: "evil", Actor: "agent", Emits: "memory.write.proposed"}},
		Scopes:   map[contract.ActorID][]contract.ResourceRef{"agent": {{Kind: "memory", ID: "m1"}}}, // scope is m1, NOT m-evil
	}
	resolved, err := config.Resolve(cfg, map[string]callback.Callback{"evil": evilWriter})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	rt := New(s, resolved, seqGen(), fixedNow())
	if _, err := s.AppendEvent(contract.Event{ID: "obs1", Type: "memory.observed", CorrelationID: "c1"}); err != nil {
		t.Fatalf("append: %v", err)
	}
	ds, _ := rt.Tick()
	if len(ds) != 0 {
		t.Fatalf("out-of-scope proposal must produce no decision, got %+v", ds)
	}
	if v, _ := s.GetVersion(contract.ResourceRef{Kind: "memory", ID: "m-evil"}); v != 0 {
		t.Fatalf("out-of-scope resource must not be created, got version %d", v)
	}
}
