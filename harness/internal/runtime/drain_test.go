package runtime

import (
	"path/filepath"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/kernel"
	"github.com/mnemon-dev/mnemon/harness/internal/rule"
)

// DrainOutbox is the driver's out-of-band claim of projection invalidations: an accepted apply
// enqueues an invalidation, the driver claims + acks it (unconditional of the job lane), and a
// re-drain finds nothing.
func TestDrainOutboxClaimsInvalidations(t *testing.T) {
	rt, err := OpenRuntime(filepath.Join(t.TempDir(), "s.db"), RuntimeConfig{
		Rules:     rule.NewRuleSet(createOnObserve()),
		Authority: kernel.AuthorityRules{Allow: map[contract.ActorID][]contract.ResourceKind{"agent": {"memory"}}},
		Subs:      map[contract.ActorID]contract.Subscription{"agent": {Actor: "agent", Refs: []contract.ResourceRef{{Kind: "memory", ID: "m1"}}}},
	})
	if err != nil {
		t.Fatalf("open runtime: %v", err)
	}
	defer rt.Close()

	if _, _, err := rt.API().Ingest("agent", contract.ObservationEnvelope{
		ExternalID: "e1", Event: contract.Event{Type: "memory.observed", Payload: map[string]any{}},
	}); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if _, err := rt.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}

	n, err := rt.DrainOutbox()
	if err != nil {
		t.Fatalf("drain: %v", err)
	}
	if n != 1 {
		t.Fatalf("an accepted apply must enqueue exactly one invalidation to drain; got %d", n)
	}
	if n2, err := rt.DrainOutbox(); err != nil || n2 != 0 {
		t.Fatalf("a re-drain must find nothing; got %d (err %v)", n2, err)
	}
}
