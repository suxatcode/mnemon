package reconcile

import (
	"encoding/json"

	"github.com/mnemon-dev/mnemon/core/contract"
	"github.com/mnemon-dev/mnemon/core/kernel"
)

type Reconciler struct {
	store  *kernel.Store
	kernel *kernel.Kernel
	cursor int64
	rebase map[string]int // per-CorrelationID deferral count; PERSISTS across RunOnce calls (Invariant #10)
}

func NewReconciler(s *kernel.Store, k *kernel.Kernel) *Reconciler {
	return &Reconciler{store: s, kernel: k, rebase: map[string]int{}}
}

// opFromEvent builds the KernelOp from a TRUSTED event. Actor and read-set come from the event envelope
// which the reconciler/registry stamped from trusted sources (registry binding + the dispatched projection),
// NEVER from callback-controlled payload (trust-boundary fix, Invariants #13/#15).
//
// Deviation from the plan's literal `ev.Payload["writes"].([]contract.ResourceWrite)`: that type-assert
// PANICS after the AppendEvent->PendingEvents JSON round-trip (Payload["writes"] decodes to []any, not the
// typed slice). We re-marshal+unmarshal the payload's "writes" into typed ResourceWrite — round-trip-safe
// and behaviorally identical for the typed fixtures.
func opFromEvent(ev contract.Event) contract.KernelOp {
	var writes []contract.ResourceWrite
	if raw, ok := ev.Payload["writes"]; ok {
		b, _ := json.Marshal(raw)
		_ = json.Unmarshal(b, &writes)
	}
	return contract.KernelOp{OpID: ev.ID, Actor: ev.Actor, Writes: writes, ReadSet: ev.BasedOn}
}

func (r *Reconciler) RunOnce(modes contract.Modes) []contract.Decision {
	evs, _ := r.store.PendingEvents(r.cursor)
	var out []contract.Decision
	for _, ev := range evs { // strictly IngestSeq order (Invariant #9)
		call := modes
		if modes.Conflict == contract.ConflictRebase && r.rebase[ev.CorrelationID] >= 2 {
			call.Conflict = contract.ConflictDeferToHuman // escalate BEFORE Apply -> terminal decision persisted once (#10)
		}
		d := r.kernel.Apply(opFromEvent(ev), call) // kernel is the serializer, not us (Invariant #2)
		if d.Status == contract.Deferred && d.NextAction == "rebase" {
			r.rebase[ev.CorrelationID]++
		}
		out = append(out, d)
		r.cursor = ev.IngestSeq
	}
	return out
}
