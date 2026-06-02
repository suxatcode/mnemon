// Package standard is a minimal "second adapter": a host that knows ONLY core/contract. It reads a
// projection-shaped view and emits a *.proposed event carrying its read-set (based_on). It imports
// neither core/kernel nor core/reconcile — the falsifiable afternoon-adapter smallness proof for the
// standard surface (Invariant #18). The human-readable surface companion is
// .insight/core-control-plane/SURFACE.md (gitignored, not tracked).
package standard

import "github.com/mnemon-dev/mnemon/core/contract"

// ProjectionView is the contract-shaped slice of state a host sees. The adapter does NOT import
// core/projection — it reconstructs only the fields the contract exposes.
type ProjectionView struct {
	Resources []contract.ResourceVersion
	Digest    string
}

// Propose builds a *.proposed event from what the host read. based_on (the event read-set) is the set of
// versions the proposal is premised on; the write itself rides in the payload. This is the entire
// host-side surface: a Projection in, a contract.Event out.
func Propose(actor contract.ActorID, view ProjectionView, ref contract.ResourceRef, basedOn contract.Version, fields map[string]any) contract.Event {
	return contract.Event{
		ID:            "ext_" + string(actor),
		Type:          "memory.write.proposed",
		Actor:         actor,
		ResourceRefs:  []contract.ResourceRef{ref},
		BasedOn:       view.Resources, // read-set the proposal is premised on
		ContextDigest: view.Digest,    // provenance only
		Payload: map[string]any{
			"writes": []contract.ResourceWrite{{Ref: ref, Kind: contract.OpUpdate, BasedOn: basedOn, Fields: fields}},
		},
	}
}
