// Package server is the governed control loop: a ControlServer ingests observations exactly-once, runs them
// through the rule pre-gate, bridges proposals into trusted *.proposed events, reconciles them through the
// single-writer kernel, and emits outbox invalidations + durable diagnostics. The kernel stays minimal; the
// rich admission semantics live here (D4). The edge<->server contract is the ServerAPI interface (D5).
package server

import (
	"encoding/json"
	"strings"

	"github.com/mnemon-dev/mnemon/harness/core/config"
	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/kernel"
	"github.com/mnemon-dev/mnemon/harness/core/projection"
	"github.com/mnemon-dev/mnemon/harness/core/reconcile"
	"github.com/mnemon-dev/mnemon/harness/core/rule"
	"github.com/mnemon-dev/mnemon/harness/core/runtime"
)

const serverDispatchCursor = "server_dispatch"

// ServerAPI is the edge<->server boundary (D5). Production HTTP/gRPC+mTLS is a thin adapter over it
// (httpapi.go); the in-process implementation is *ControlServer. It grows by phase: Ingest (P0),
// PullProjection (P2), ClaimJob/FinishJob (P3).
type ServerAPI interface {
	Ingest(principal contract.ActorID, env contract.ObservationEnvelope) (seq int64, dup bool, err error)
}

var _ ServerAPI = (*ControlServer)(nil)

// ControlServer is the one single-writer governed loop. Tick is its deterministic, restart-safe driver.
type ControlServer struct {
	store      *kernel.Store
	kernel     *kernel.Kernel
	reconciler *reconcile.Reconciler
	bridge     *runtime.Bridge
	rules      rule.RuleSet
	subs       map[contract.ActorID]contract.Subscription
	modes      contract.Modes
	newID      func() string
	now        func() string
}

func New(s *kernel.Store, k *kernel.Kernel, rules rule.RuleSet, subs map[contract.ActorID]contract.Subscription, modes contract.Modes, newID, now func() string) *ControlServer {
	return &ControlServer{
		store:      s,
		kernel:     k,
		reconciler: reconcile.NewReconciler(s, k),
		bridge:     runtime.NewBridge(newID, now),
		rules:      rules,
		subs:       subs,
		modes:      modes,
		newID:      newID,
		now:        now,
	}
}

// Ingest records an observation exactly-once (S1). Source and Event.Actor are stamped from the AUTHENTICATED
// principal — the client's payload claim is overwritten, never trusted (D7/S9).
func (cs *ControlServer) Ingest(principal contract.ActorID, env contract.ObservationEnvelope) (int64, bool, error) {
	env.Source = principal
	env.Event.Actor = principal
	return cs.store.IngestObservation(env)
}

// Tick runs one governed cycle:
//  1. DISPATCH: scan events past the durable dispatch cursor; for each OBSERVED event, build its actor's
//     scoped view, run the rule pre-gate, turn the verdict into trusted events — a propose -> bridged
//     *.proposed event; a deny / rule-error -> a *.diagnostic event (S7, no silent drop). The proposed +
//     diagnostic events AND the cursor advance are ONE atomic DispatchTx (S2).
//  2. RECONCILE: the kernel decides the pending *.proposed events (the kernel is the only writer).
//  3. INVALIDATE: each Accepted decision enqueues an outbox invalidation (downstream projections are stale).
func (cs *ControlServer) Tick() ([]contract.Decision, error) {
	cur := cs.store.GetCursor(serverDispatchCursor)
	evs, err := cs.store.PendingEvents(cur)
	if err != nil {
		return nil, err // fail-stop on a corrupt log (consistent with RunOnce)
	}
	for _, ev := range evs {
		stamped, derr := cs.dispatchOne(ev)
		if derr != nil {
			return nil, derr
		}
		if err := cs.store.DispatchTx(stamped, serverDispatchCursor, ev.IngestSeq); err != nil {
			return nil, err
		}
	}
	decisions := cs.reconciler.RunOnce(cs.modes)
	if err := cs.enqueueInvalidations(decisions); err != nil {
		return nil, err
	}
	return decisions, nil
}

// dispatchOne runs the rule pre-gate for one event and returns the trusted events to append (proposals +
// diagnostics). Events no rule handles (proposals, diagnostics, other domains) produce nothing — the cursor
// still advances past them, so each event is consumed exactly once.
func (cs *ControlServer) dispatchOne(ev contract.Event) ([]contract.Event, error) {
	view := cs.scopedView(ev.Actor)
	dec, diags := cs.rules.Evaluate(rule.RuleInput{Event: ev, View: view})
	var stamped []contract.Event
	for _, dg := range diags { // S7: every rule error is a durable diagnostic.
		stamped = append(stamped, cs.diagnosticEvent(ev, dg))
	}
	switch dec.Verdict {
	case contract.VerdictPropose:
		if dec.Proposal == nil {
			break
		}
		b, ok := cs.proposerBinding(ev, dec)
		if !ok {
			stamped = append(stamped, cs.diagnosticEvent(ev, contract.Diagnostic{Stage: "bridge", Reason: "no rule owns the proposal type", Ref: dec.Proposal.Type}))
			break
		}
		e, serr := cs.bridge.Stamp(b, view, ev, *dec.Proposal)
		if serr != nil {
			stamped = append(stamped, cs.diagnosticEvent(ev, contract.Diagnostic{Stage: "bridge", Reason: serr.Error(), Ref: string(b.Actor)}))
			break
		}
		stamped = append(stamped, e)
	case contract.VerdictDeny:
		stamped = append(stamped, cs.diagnosticEvent(ev, contract.Diagnostic{Stage: "rule", Reason: strings.Join(dec.Reasons, "; "), Ref: ev.Type}))
	case contract.VerdictEnqueueJob, contract.VerdictRequestEvidence:
		// the effectful job lane is wired in P3; for now these verdicts produce no proposal.
	}
	return stamped, nil
}

// scopedView builds the actor's scoped projection. (P2 strengthens the scoping + digest behind this seam;
// the call site stays stable.)
func (cs *ControlServer) scopedView(actor contract.ActorID) projection.Projection {
	sub := cs.subs[actor]
	return projection.Build(cs.store, sub.Refs, actor)
}

// proposerBinding finds the rule that produced a proposal (deterministic, by rule order) so the bridge stamps
// the trusted write identity (Actor) + authorized type (Emits) from the RULE, never the payload.
func (cs *ControlServer) proposerBinding(ev contract.Event, dec contract.RuleDecision) (config.ResolvedBinding, bool) {
	if dec.Proposal == nil {
		return config.ResolvedBinding{}, false
	}
	for _, r := range cs.rules.Rules() {
		if r.Handles(ev.Type) && r.Emits() == dec.Proposal.Type {
			return config.ResolvedBinding{EventType: ev.Type, Actor: r.Actor(), Emits: r.Emits()}, true
		}
	}
	return config.ResolvedBinding{}, false
}

// diagnosticEvent builds a durable "*.diagnostic" event in the trigger's domain (S7). Domain = the prefix of
// the trigger type before the first dot (memory.observed -> memory.diagnostic).
func (cs *ControlServer) diagnosticEvent(trigger contract.Event, dg contract.Diagnostic) contract.Event {
	domain := trigger.Type
	if i := strings.IndexByte(domain, '.'); i >= 0 {
		domain = domain[:i]
	}
	return contract.Event{
		SchemaVersion: 1,
		ID:            cs.newID(),
		TS:            cs.now(),
		Type:          domain + ".diagnostic",
		Actor:         trigger.Actor,
		CorrelationID: trigger.CorrelationID,
		CausedBy:      trigger.ID,
		Payload:       map[string]any{"stage": dg.Stage, "reason": dg.Reason, "ref": dg.Ref},
	}
}

// enqueueInvalidations records an outbox invalidation per Accepted decision (S2 downstream propagation). The
// DecisionID is the idempotency key, so a replayed decision never double-enqueues.
func (cs *ControlServer) enqueueInvalidations(decisions []contract.Decision) error {
	for _, d := range decisions {
		if d.Status != contract.Accepted {
			continue
		}
		payload, _ := json.Marshal(d.NewVersions)
		key := "inv_" + d.DecisionID
		if err := cs.store.WithTx(func(tx *kernel.Tx) error {
			return tx.EnqueueOutbox(kernel.OutboxRow{
				ID: key, Kind: "invalidation", EventSeq: d.IngestSeq,
				Target: "projection", Payload: string(payload), IdempotencyKey: key,
			})
		}); err != nil {
			return err
		}
	}
	return nil
}
