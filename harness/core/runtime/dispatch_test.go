package runtime

import (
	"errors"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/core/callback"
	"github.com/mnemon-dev/mnemon/harness/core/config"
	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/projection"
)

func TestDispatchPairsIntentsWithTheirBinding(t *testing.T) {
	emit := func(actor contract.ActorID) config.ResolvedBinding {
		return config.ResolvedBinding{EventType: "memory.observed", Actor: actor, Emits: "memory.write.proposed",
			Callback: callback.BuiltinFunc(func(contract.Event, projection.Projection) ([]contract.ProposedEvent, error) {
				return []contract.ProposedEvent{{Type: "memory.write.proposed", Payload: map[string]any{"by": string(actor)}}}, nil
			})}
	}
	// a third binding that emits the WRONG type must be dropped (R8)
	wrong := config.ResolvedBinding{EventType: "memory.observed", Actor: "agent", Emits: "memory.write.proposed",
		Callback: callback.BuiltinFunc(func(contract.Event, projection.Projection) ([]contract.ProposedEvent, error) {
			return []contract.ProposedEvent{{Type: "goal.update.proposed"}}, nil
		})}
	bindings := []config.ResolvedBinding{emit("a1"), emit("a2"), wrong}
	pairs := DispatchBindings(bindings, contract.Event{Type: "memory.observed"}, projection.Projection{})
	if len(pairs) != 2 {
		t.Fatalf("want 2 pairs (wrong-type intent dropped), got %d", len(pairs))
	}
	if pairs[0].Binding.Actor != "a1" || pairs[1].Binding.Actor != "a2" {
		t.Fatalf("each intent must carry ITS binding's actor; got %q,%q", pairs[0].Binding.Actor, pairs[1].Binding.Actor)
	}
}

// P2 Gate surface (Invariant #13): a callback that errors contributes ZERO intents.
func TestDispatchErroringCallbackYieldsZeroIntents(t *testing.T) {
	boom := config.ResolvedBinding{EventType: "memory.observed", Actor: "agent", Emits: "memory.write.proposed",
		Callback: callback.BuiltinFunc(func(contract.Event, projection.Projection) ([]contract.ProposedEvent, error) {
			return []contract.ProposedEvent{{Type: "memory.write.proposed"}}, errors.New("boom")
		})}
	pairs := DispatchBindings([]config.ResolvedBinding{boom}, contract.Event{Type: "memory.observed"}, projection.Projection{})
	if len(pairs) != 0 {
		t.Fatalf("erroring callback must contribute zero intents, got %d", len(pairs))
	}
}
