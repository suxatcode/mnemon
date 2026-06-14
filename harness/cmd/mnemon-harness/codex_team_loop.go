package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/channel"
	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/projection"
)

// ============================================================================
// Governed self-continuation loop.
//
// This is the "use a cluster like a single agent" engine. It is deliberately a
// DUMB, content-blind nudge executor: it makes ZERO routing decisions. Each pass it
// pulls every agent's server-scoped projection, and when an agent's projection Digest
// has changed (its governed scope moved) it NUDGES that agent — handing it the fresh
// projection as a turn packet. Whatever observations the agent's brain returns are
// ingested and governed (Ingest + Tick) through the same channel any report uses.
//
// The "who acts next / what to do" decision never lives here. It lives in a POC brain's
// GOVERNED assignment emissions. The engine cannot tell a worker report from a routing
// assignment from a review — it only sees "this agent's scope changed, nudge it". That
// is the line that keeps this a governed cluster, not an orchestrator with a nicer UI.
// ============================================================================

// turnPacket is what a nudged agent receives: its scoped projection and why it was woken.
// Reason is always "scope-changed" — the only nudge cause (content-blind).
type turnPacket struct {
	Principal  contract.ActorID
	Reason     string
	Projection projection.Projection
}

// agentBrain turns a nudge into observations. The brain owns ALL understanding and routing;
// the engine owns none. A scripted brain (deterministic) and a real-Codex brain (an LLM in
// the seat) are both just agentBrains — swapping one for the other is a brain change, never
// an engine change.
type agentBrain interface {
	Principal() contract.ActorID
	// Act is called when the agent's governed scope changed. It returns the observations the
	// agent chooses to emit. Emissions MUST be idempotent via ExternalID so re-nudges on an
	// unrelated scope change re-emit harmlessly (the channel dedupes), letting the loop reach
	// quiescence. Empty return = nothing to do this turn.
	Act(pkt turnPacket) []contract.ObservationEnvelope
}

// scriptedBrain is a deterministic agentBrain: its understanding/routing is a Go closure
// instead of an LLM. It proves the PLUMBING of governed self-continuation without burning a
// real Codex turn; the real-Codex brain is a drop-in with the same interface.
type scriptedBrain struct {
	principal contract.ActorID
	act       func(pkt turnPacket) []contract.ObservationEnvelope
}

func (b scriptedBrain) Principal() contract.ActorID { return b.principal }
func (b scriptedBrain) Act(pkt turnPacket) []contract.ObservationEnvelope {
	if b.act == nil {
		return nil
	}
	return b.act(pkt)
}

// nudgeEvent records one nudge for the human-facing UI: which agent woke, on what digest,
// how much it emitted, and how many governed decisions that produced. This is the
// observability surface that makes the self-continuation legible.
type nudgeEvent struct {
	Step      int
	Principal contract.ActorID
	Digest    string
	Emitted   int
	Accepted  int
}

// governedLoop drives the cluster to quiescence by nudging on scope change. It holds the
// runtime handle (for in-process pull/submit), the brains, and each principal's scope.
type governedLoop struct {
	handle *codexTeamRuntimeHandle
	brains []agentBrain
	subs   map[contract.ActorID]contract.Subscription

	// Delay, when > 0, paces the loop one step at a time so a human can watch the cluster
	// self-continue in the UI. Zero (the test/CI default) runs at full speed.
	Delay time.Duration

	mu     sync.Mutex
	seen   map[contract.ActorID]string
	nudges []nudgeEvent
	done   bool
}

// newGovernedLoop builds the loop. Each principal's subscription scope comes straight from
// its channel binding (the auditable ceiling); the engine never widens or narrows it — scope
// is the communication graph, configured at binding time, not in code.
func newGovernedLoop(h *codexTeamRuntimeHandle, bindings []channel.ChannelBinding, brains ...agentBrain) *governedLoop {
	subs := make(map[contract.ActorID]contract.Subscription, len(bindings))
	for _, b := range bindings {
		subs[b.Principal] = contract.Subscription{Actor: b.Principal, Refs: b.SubscriptionScope}
	}
	return &governedLoop{
		handle: h,
		brains: brains,
		subs:   subs,
		seen:   make(map[contract.ActorID]string),
	}
}

// Run drives passes until quiescence (a full pass that produces no new accepted decision) or
// maxSteps (a runaway guard). It returns the total number of accepted decisions the loop
// produced. Quiescence — not a fixed round count — is what "the cluster finished" means.
func (g *governedLoop) Run(maxSteps int) (int, error) {
	return g.RunContext(context.Background(), maxSteps)
}

// RunContext is Run with cancellation and optional per-step pacing (Delay) for the live UI.
func (g *governedLoop) RunContext(ctx context.Context, maxSteps int) (int, error) {
	defer g.markDone()
	total := 0
	for step := 1; step <= maxSteps; step++ {
		if g.Delay > 0 {
			select {
			case <-ctx.Done():
				return total, ctx.Err()
			case <-time.After(g.Delay):
			}
		} else if ctx.Err() != nil {
			return total, ctx.Err()
		}
		n, err := g.step(step)
		if err != nil {
			return total, err
		}
		total += n
		if n == 0 {
			return total, nil
		}
	}
	return total, nil
}

func (g *governedLoop) markDone() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.done = true
}

// Done reports whether the loop has reached quiescence (or stopped).
func (g *governedLoop) Done() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.done
}

// step is one pass over the agents: nudge each whose scope changed, ingest+govern its output.
// Returns the number of NEW accepted decisions produced this pass.
func (g *governedLoop) step(step int) (int, error) {
	accepted := 0
	for _, brain := range g.brains {
		p := brain.Principal()
		proj, err := g.handle.PullProjection(p, g.subs[p])
		if err != nil {
			return accepted, fmt.Errorf("pull projection for %s: %w", p, err)
		}
		if proj.Digest == g.lastDigest(p) {
			continue // scope unchanged for this agent — no nudge (content-blind trigger)
		}
		g.setDigest(p, proj.Digest)

		emitted := brain.Act(turnPacket{Principal: p, Reason: "scope-changed", Projection: proj})
		nudgeAccepted := 0
		for _, env := range emitted {
			_, dup, decisions, serr := g.handle.Submit(p, env)
			if serr != nil {
				return accepted, fmt.Errorf("submit %s observation for %s: %w", env.Event.Type, p, serr)
			}
			if dup {
				continue
			}
			for _, d := range decisions {
				if d.Status == contract.Accepted {
					nudgeAccepted++
				}
			}
		}
		accepted += nudgeAccepted
		g.recordNudge(nudgeEvent{Step: step, Principal: p, Digest: proj.Digest, Emitted: len(emitted), Accepted: nudgeAccepted})
	}
	return accepted, nil
}

func (g *governedLoop) lastDigest(p contract.ActorID) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.seen[p]
}

func (g *governedLoop) setDigest(p contract.ActorID, digest string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.seen[p] = digest
}

func (g *governedLoop) recordNudge(ev nudgeEvent) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.nudges = append(g.nudges, ev)
}

// Nudges returns a copy of the nudge timeline for the UI/observability surface.
func (g *governedLoop) Nudges() []nudgeEvent {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]nudgeEvent(nil), g.nudges...)
}

// ---- observation + projection helpers ----

// codexLoopObs builds an observation envelope. Source is left empty: the server stamps the
// authenticated principal as Event.Actor on Ingest — a client never names its own identity.
func codexLoopObs(eventType, externalID string, payload map[string]any) contract.ObservationEnvelope {
	return contract.ObservationEnvelope{
		ExternalID: externalID,
		Event:      contract.Event{Type: eventType, Payload: payload},
	}
}

// projectionHasKind reports whether a resource of kind is present (materialized) in the view.
func projectionHasKind(proj projection.Projection, kind contract.ResourceKind) bool {
	for _, c := range proj.Content {
		if c.Ref.Kind == kind {
			return true
		}
	}
	return false
}

// projectionItems returns the item list of the first resource of kind in the view. Coordination
// kinds (assignment, progress_digest, project_intent) carry their records under the "items" field.
func projectionItems(proj projection.Projection, kind contract.ResourceKind) []map[string]any {
	for _, c := range proj.Content {
		if c.Ref.Kind != kind {
			continue
		}
		raw, ok := c.Fields["items"].([]any)
		if !ok {
			return nil
		}
		out := make([]map[string]any, 0, len(raw))
		for _, r := range raw {
			if m, ok := r.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}
	return nil
}

// itemStr reads a string field from a coordination item.
func itemStr(item map[string]any, key string) string {
	if s, ok := item[key].(string); ok {
		return s
	}
	return ""
}

// ============================================================================
// Runtime-handle methods (cmd-layer wrappers over already-exported framework surface;
// no harness/internal edits). PullProjection/DecisionLedger are read-only; Submit is the
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

// DecisionLedger returns the full accepted/rejected decision history — the replay surface
// that the acceptance test reconstructs the self-continuation chain from.
func (h *codexTeamRuntimeHandle) DecisionLedger() ([]contract.Decision, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.rt == nil {
		return nil, fmt.Errorf("runtime unavailable")
	}
	return h.rt.DecisionLedger()
}
