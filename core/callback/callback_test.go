package callback

import (
	"errors"
	"testing"

	"github.com/mnemon-dev/mnemon/core/contract"
	"github.com/mnemon-dev/mnemon/core/kernel"
	"github.com/mnemon-dev/mnemon/core/projection"
)

func newStoreKernel(t *testing.T) (*kernel.Store, *kernel.Kernel) {
	t.Helper()
	s, err := kernel.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	k := kernel.NewKernel(s, kernel.DefaultSchemaGuard(),
		kernel.AuthorityRules{Allow: map[contract.ActorID][]contract.ResourceKind{"user": {"memory", "goal", "skill"}}})
	return s, k
}
func observedEvent() contract.Event { return contract.Event{Type: "memory.hot_write_observed"} }

func TestCallbackProducesIntentNotFact(t *testing.T) {
	s, _ := newStoreKernel(t)
	reg := NewRegistry()
	reg.On("memory.hot_write_observed", BuiltinFunc(func(ev contract.Event, _ projection.Projection) ([]contract.ProposedEvent, error) {
		return []contract.ProposedEvent{{Type: "memory.write.proposed", Payload: map[string]any{"content": "derived"}}}, nil
	}))
	before := s.DecisionCount()
	intents := reg.Dispatch(observedEvent(), projection.Projection{})
	if s.DecisionCount() != before {
		t.Fatal("callback mutated state directly — must only propose") // Invariant #13
	}
	if len(intents) != 1 {
		t.Fatal("expected one intent")
	}
}
func TestDispatchDropsAllIntentsFromErroringCallback(t *testing.T) {
	reg := NewRegistry()
	reg.On("x.observed", BuiltinFunc(func(contract.Event, projection.Projection) ([]contract.ProposedEvent, error) {
		return []contract.ProposedEvent{{Type: "y.proposed"}}, errors.New("boom") // intent AND error
	}))
	if n := len(reg.Dispatch(contract.Event{Type: "x.observed"}, projection.Projection{})); n != 0 {
		t.Fatalf("erroring callback must contribute ZERO intents, got %d", n) // trust-boundary fix
	}
}
