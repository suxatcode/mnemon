package runtime

import (
	"github.com/mnemon-dev/mnemon/harness/core/config"
	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/projection"
)

type Pair struct {
	Binding config.ResolvedBinding
	Intent  contract.ProposedEvent
}

// DispatchBindings runs every binding whose EventType matches the trigger and pairs each returned intent
// with the binding that produced it (so the bridge stamps each with ITS binding's actor — Invariant R8).
// An erroring callback contributes ZERO intents (Invariant #13); an intent whose Type != binding.Emits is
// dropped (a callback may not emit a type it is not bound to). The caller builds the per-binding-actor
// projection and passes it as view.
func DispatchBindings(bindings []config.ResolvedBinding, trigger contract.Event, view projection.Projection) []Pair {
	var out []Pair
	for _, b := range bindings {
		if b.EventType != trigger.Type {
			continue
		}
		intents, err := b.Callback.OnEvent(trigger, view)
		if err != nil {
			continue
		}
		for _, it := range intents {
			if it.Type != b.Emits {
				continue
			}
			out = append(out, Pair{Binding: b, Intent: it})
		}
	}
	return out
}
