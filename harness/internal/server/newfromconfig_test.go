package server

import (
	"testing"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/config"
	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/kernel"
	"github.com/mnemon-dev/mnemon/harness/internal/reconcile"
	"github.com/mnemon-dev/mnemon/harness/internal/rule"
	"github.com/mnemon-dev/mnemon/harness/internal/store"
)

// agentActors is the declared actor->kinds catalog used both to build the kernel
// authority rules and (in NewFromConfig) to validate rule bindings.
func agentActors() map[contract.ActorID][]contract.ResourceKind {
	return map[contract.ActorID][]contract.ResourceKind{"agent": {"memory"}}
}

func p0ModesConfig() reconcile.Config {
	return reconcile.Config{Conflict: "rebase", Isolation: "projection_read_set", Authz: "strict"}
}

func bootViaConfig(t *testing.T, registry map[string]rule.Rule, bindings []config.RuleBinding) (*store.Store, *ControlServer) {
	t.Helper()
	s, err := store.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	k := kernel.NewKernel(s, kernel.DefaultSchemaGuard(), kernel.AuthorityRules{Allow: agentActors()})
	cs, err := NewFromConfig(s, k, config.RuleConfig{Bindings: bindings}, registry, agentActors(), agentSubs(), p0ModesConfig(), seqGen(), fixedNow())
	if err != nil {
		t.Fatalf("NewFromConfig: %v", err)
	}
	if d := k.Apply(contract.KernelOp{OpID: "seed", Actor: "agent", Writes: []contract.ResourceWrite{
		{Ref: contract.ResourceRef{Kind: "memory", ID: "m1"}, Kind: contract.OpCreate, Fields: map[string]any{"content": "v0"}}}}, p0Modes()); d.Status != contract.Accepted {
		t.Fatalf("seed: %s", d.Reason)
	}
	return s, cs
}

// TestNewFromConfigBootsEquivalentServer asserts a server booted through the config
// front door (config.ResolveRules + reconcile.ResolveModes) behaves identically to
// one hand-wired via New: a propose rule accepts + advances state + enqueues an
// invalidation; a deny rule changes nothing; an unregistered rule key is rejected.
func TestNewFromConfigBootsEquivalentServer(t *testing.T) {
	t.Run("propose accepts", func(t *testing.T) {
		s, cs := bootViaConfig(t,
			map[string]rule.Rule{"writer": proposeRule()},
			[]config.RuleBinding{{EventType: "memory.observed", Rule: "writer"}})
		if _, _, err := cs.Ingest("agent", contract.ObservationEnvelope{ExternalID: "e1", Event: contract.Event{Type: "memory.observed", CorrelationID: "c1"}}); err != nil {
			t.Fatalf("ingest: %v", err)
		}
		ds, err := cs.Tick()
		if err != nil {
			t.Fatalf("tick: %v", err)
		}
		if len(ds) != 1 || ds[0].Status != contract.Accepted {
			t.Fatalf("propose-rule must lead to one Accepted decision; got %+v", ds)
		}
		if v, _ := s.GetVersion(contract.ResourceRef{Kind: "memory", ID: "m1"}); v != 2 {
			t.Fatalf("m1 must advance to @2; got %d", v)
		}
		claimed, _ := s.ClaimOutbox("w", time.Minute)
		if len(claimed) != 1 || claimed[0].Kind != "invalidation" {
			t.Fatalf("accepted decision must enqueue an outbox invalidation; got %+v", claimed)
		}
	})

	t.Run("deny changes nothing", func(t *testing.T) {
		s, cs := bootViaConfig(t,
			map[string]rule.Rule{"denier": denyRule()},
			[]config.RuleBinding{{EventType: "memory.observed", Rule: "denier"}})
		if _, _, err := cs.Ingest("agent", contract.ObservationEnvelope{ExternalID: "e1", Event: contract.Event{Type: "memory.observed", CorrelationID: "c1"}}); err != nil {
			t.Fatalf("ingest: %v", err)
		}
		ds, err := cs.Tick()
		if err != nil {
			t.Fatalf("tick: %v", err)
		}
		if len(ds) != 0 {
			t.Fatalf("deny must produce no decision; got %+v", ds)
		}
		if v, _ := s.GetVersion(contract.ResourceRef{Kind: "memory", ID: "m1"}); v != 1 {
			t.Fatalf("deny must not change state; m1 must stay @1; got %d", v)
		}
	})

	t.Run("unregistered rule key rejected", func(t *testing.T) {
		s, err := store.OpenStore(":memory:")
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		defer s.Close()
		k := kernel.NewKernel(s, kernel.DefaultSchemaGuard(), kernel.AuthorityRules{Allow: agentActors()})
		if _, err := NewFromConfig(s, k,
			config.RuleConfig{Bindings: []config.RuleBinding{{EventType: "memory.observed", Rule: "ghost"}}},
			map[string]rule.Rule{"writer": proposeRule()}, agentActors(), agentSubs(), p0ModesConfig(), seqGen(), fixedNow()); err == nil {
			t.Fatal("NewFromConfig must surface the ResolveRules error for an unregistered rule key")
		}
	})
}
