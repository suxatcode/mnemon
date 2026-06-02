package callback

import (
	"github.com/mnemon-dev/mnemon/core/contract"
	"github.com/mnemon-dev/mnemon/core/projection"
)

// Registry binds event types to callbacks. Dispatch returns INTENT only; it never touches a
// Store/Kernel — commit is always the kernel's downstream (Invariant #13).
type Registry struct {
	handlers map[string][]Callback
}

func NewRegistry() *Registry { return &Registry{handlers: map[string][]Callback{}} }

func (r *Registry) On(eventType string, cb Callback) {
	r.handlers[eventType] = append(r.handlers[eventType], cb)
}

// Dispatch runs every callback bound to ev.Type and collects their intents. A callback that returns an
// error contributes ZERO intents — ALL of that callback's intents are dropped (all-or-nothing per
// callback; an erroring proposer must not half-propose — trust-boundary fix, Invariants #13/#15).
func (r *Registry) Dispatch(ev contract.Event, view projection.Projection) []contract.ProposedEvent {
	var out []contract.ProposedEvent
	for _, cb := range r.handlers[ev.Type] {
		intents, err := cb.OnEvent(ev, view)
		if err != nil {
			continue // drop ALL of this callback's intents
		}
		out = append(out, intents...)
	}
	return out
}
