package runtime

import (
	"github.com/mnemon-dev/mnemon/harness/core/config"
	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/kernel"
	"github.com/mnemon-dev/mnemon/harness/core/projection"
	"github.com/mnemon-dev/mnemon/harness/core/reconcile"
)

const dispatchCursor = "dispatch"

type Runtime struct {
	store      *kernel.Store
	reconciler *reconcile.Reconciler
	resolved   config.Resolved
	bridge     *Bridge
}

func New(s *kernel.Store, resolved config.Resolved, newID, now func() string) *Runtime {
	k := kernel.NewKernel(s, kernel.DefaultSchemaGuard(), resolved.Rules)
	return &Runtime{store: s, reconciler: reconcile.NewReconciler(s, k), resolved: resolved, bridge: NewBridge(newID, now)}
}

// Tick runs one deterministic, restart-safe cycle:
//  1. DISPATCH every not-yet-dispatched event through matching bindings (per-binding-actor projection),
//     bridging each in-scope intent into a TRUSTED *.proposed event. The append of an observed event's
//     proposed events AND the advance of the durable dispatch cursor are ONE atomic DispatchTx
//     (Invariant R6 / finding #1 — a crash can never leave a half-dispatched observation that re-fires).
//     A *.proposed event matches no observed binding, so it is never re-dispatched.
//  2. RECONCILE: the reconciler decides the pending *.proposed events (its own cursor from the decision
//     log). The kernel is the sole writer; callbacks only proposed.
func (rt *Runtime) Tick() ([]contract.Decision, error) {
	cur := rt.store.GetCursor(dispatchCursor)
	evs, err := rt.store.PendingEvents(cur)
	if err != nil {
		return nil, err // fail-stop on a corrupt log (consistent with RunOnce)
	}
	for _, ev := range evs {
		var stamped []contract.Event
		for _, b := range rt.resolved.Bindings {
			if b.EventType != ev.Type {
				continue
			}
			view := projection.Build(rt.store, rt.resolved.Scopes[b.Actor], b.Actor)
			for _, p := range DispatchBindings([]config.ResolvedBinding{b}, ev, view) {
				e, serr := rt.bridge.Stamp(p.Binding, view, ev, p.Intent)
				if serr != nil {
					continue // out-of-scope write (R11): dropped, never becomes an event
				}
				stamped = append(stamped, e)
			}
		}
		// ATOMIC: append this observed event's proposed events + advance the dispatch cursor past it.
		// (Empty `stamped` just advances the cursor — an observation with no/blocked proposals is still
		// consumed exactly once.)
		if err := rt.store.DispatchTx(stamped, dispatchCursor, ev.IngestSeq); err != nil {
			return nil, err
		}
	}
	return rt.reconciler.RunOnce(rt.resolved.Modes), nil
}
