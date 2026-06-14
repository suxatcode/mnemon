package main

import (
	"fmt"

	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/projection"
)

// ============================================================================
// codexTeamRuntimeHandle satisfies autopilot.Runtime — the cmd-layer adapter that lets the
// (optional) autopilot drive this in-process runtime over already-exported framework surface
// (no harness/internal edits). PullProjection/DecisionLedger are read-only; Submit is the
// in-process Ingest+Tick that closes the governed loop without an HTTP round trip.
// ============================================================================

// PullProjection returns the principal's server-scoped projection — the trigger packet.
func (h *codexTeamRuntimeHandle) PullProjection(principal contract.ActorID, sub contract.Subscription) (projection.Projection, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.rt == nil {
		return projection.Projection{}, fmt.Errorf("runtime unavailable")
	}
	return h.rt.API().PullProjection(principal, sub)
}

// Submit ingests one observation under principal and drives one governed Tick (the same
// synchronous local mode the HTTP /ingest handler uses). It returns the ingest seq, whether
// the observation was a duplicate, and the decisions the Tick produced.
func (h *codexTeamRuntimeHandle) Submit(principal contract.ActorID, env contract.ObservationEnvelope) (int64, bool, []contract.Decision, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.rt == nil {
		return 0, false, nil, fmt.Errorf("runtime unavailable")
	}
	seq, dup, err := h.rt.API().Ingest(principal, env)
	if err != nil || dup {
		return seq, dup, nil, err
	}
	decisions, terr := h.rt.Tick()
	return seq, dup, decisions, terr
}

// DecisionLedger returns the full accepted/rejected decision history — the replay surface the
// autopilot's acceptance tests reconstruct the self-continuation chain from.
func (h *codexTeamRuntimeHandle) DecisionLedger() ([]contract.Decision, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.rt == nil {
		return nil, fmt.Errorf("runtime unavailable")
	}
	return h.rt.DecisionLedger()
}
