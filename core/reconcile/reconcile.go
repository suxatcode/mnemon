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
}

// NewReconciler seeds its cursor from the durable decision log (Store.MaxDecidedSeq), so a process
// restart resumes after the last consumed event instead of re-reading the log from 0 and re-deciding
// already-accepted events (which would pollute pull feedback). The decision log is the cursor.
//
// The liveness-escalation counter (Invariant #10) is NOT kept in memory either — it is derived per event
// from the durable log (Store.DeferralCount), so escalation survives restart exactly as the cursor does.
func NewReconciler(s *kernel.Store, k *kernel.Kernel) *Reconciler {
	return &Reconciler{store: s, kernel: k, cursor: s.MaxDecidedSeq()}
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
		if err := json.Unmarshal(b, &writes); err != nil {
			writes = nil // malformed payload -> no writes -> kernel rejects it (never a phantom Accepted no-op, #3)
		}
	}
	return contract.KernelOp{OpID: ev.ID, Actor: ev.Actor, Writes: writes, ReadSet: ev.BasedOn, IngestSeq: ev.IngestSeq, CorrelationID: ev.CorrelationID}
}

func (r *Reconciler) RunOnce(modes contract.Modes) []contract.Decision {
	var out []contract.Decision
	evs, err := r.store.PendingEvents(r.cursor)
	if err != nil {
		// A corrupt ingest log is fail-stop: do not advance the cursor or manufacture decisions from a
		// partial/garbage read — it needs operator attention. (Surfacing it to the caller would require a
		// RunOnce signature change; the store-level error is the durable signal.)
		return out
	}
	for _, ev := range evs { // strictly IngestSeq order (Invariant #9)
		call := modes
		// Escalate BEFORE Apply (so the persisted decision is terminal, #10). The deferral count is read
		// from the durable log, not in-memory, so a restart cannot silently reset the escalation clock.
		if modes.Conflict == contract.ConflictRebase && r.store.DeferralCount(ev.CorrelationID) >= 2 {
			call.Conflict = contract.ConflictDeferToHuman
		}
		d := r.kernel.Apply(opFromEvent(ev), call) // kernel is the serializer, not us (Invariant #2)
		out = append(out, d)
		r.cursor = ev.IngestSeq
	}
	return out
}
