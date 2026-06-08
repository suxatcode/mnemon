// Package server is the governed control loop: a ControlServer ingests observations exactly-once, runs them
// through the rule pre-gate, bridges proposals into trusted *.proposed events, reconciles them through the
// single-writer kernel, and emits outbox invalidations + durable diagnostics. The kernel stays minimal; the
// rich admission semantics live here (D4). The edge<->server contract is the channel.ServerAPI interface (D5).
package server

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/channel"
	"github.com/mnemon-dev/mnemon/harness/internal/config"
	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/job"
	"github.com/mnemon-dev/mnemon/harness/internal/kernel"
	"github.com/mnemon-dev/mnemon/harness/internal/projection"
	"github.com/mnemon-dev/mnemon/harness/internal/reconcile"
	"github.com/mnemon-dev/mnemon/harness/internal/rule"
	"github.com/mnemon-dev/mnemon/harness/internal/runtime"
	"github.com/mnemon-dev/mnemon/harness/internal/store"
)

const (
	serverDispatchCursor = "server_dispatch"
	decisionSinkCursor   = "decision_sink" // tracks decisions whose S2/S7 side-effects are produced (recoverable)
)

var syncableResourceKinds = map[contract.ResourceKind]bool{
	"memory": true,
	"skill":  true,
}

var _ channel.ServerAPI = (*ControlServer)(nil)

// ControlServer is the one single-writer governed loop. Tick is its deterministic, restart-safe driver.
type ControlServer struct {
	tickMu     sync.Mutex // serializes Tick: closes the GetCursor->dispatch TOCTOU + the reconciler-cursor race
	store      *store.Store
	kernel     *kernel.Kernel
	reconciler *reconcile.Reconciler
	bridge     *runtime.Bridge
	rules      rule.RuleSet
	subs       map[contract.ActorID]contract.Subscription
	modes      contract.Modes
	newID      func() string
	now        func() string

	// effectful job lane (S4/S5): nil runner = no lane (P0–P2). Configured via WithLane.
	runner    job.Runner
	laneOwner contract.ActorID
	laneTTL   int64
	nowUnix   func() int64
}

func New(s *store.Store, k *kernel.Kernel, rules rule.RuleSet, subs map[contract.ActorID]contract.Subscription, modes contract.Modes, newID, now func() string) *ControlServer {
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

// NewFromConfig is the documented boot front door over the select-only resolvers: it
// composes config.ResolveRules (rule pre-gate selection, validated against the declared
// actors) + reconcile.ResolveModes (mode selection) and wires the result into New. The
// caller still owns the kernel (built with the matching AuthorityRules) — NewFromConfig
// selects policy, it does not introduce engine wiring New lacks.
//
// newID/now are REQUIRED, not optional: New feeds them to runtime.NewBridge and the
// exactly-once id/clock, so a caller (and the server tests) can inject deterministic
// generators. A resolver error (unknown rule key, undeclared actor, bad mode) is
// returned, never panicked.
func NewFromConfig(s *store.Store, k *kernel.Kernel, rc config.RuleConfig, registry map[string]rule.Rule, actors map[contract.ActorID][]contract.ResourceKind, subs map[contract.ActorID]contract.Subscription, modes reconcile.Config, newID, now func() string) (*ControlServer, error) {
	rules, err := config.ResolveRules(rc, registry, actors)
	if err != nil {
		return nil, err
	}
	resolvedModes, err := reconcile.ResolveModes(modes)
	if err != nil {
		return nil, err
	}
	return New(s, k, rules, subs, resolvedModes, newID, now), nil
}

// WithLane enables the effectful job lane: jobs the rule pre-gate enqueues are run by runner under leases
// owned by owner (fenced for ttl seconds; nowUnix is the injectable clock). Returns the server for chaining.
func (cs *ControlServer) WithLane(runner job.Runner, owner contract.ActorID, nowUnix func() int64, ttl int64) *ControlServer {
	cs.runner, cs.laneOwner, cs.nowUnix, cs.laneTTL = runner, owner, nowUnix, ttl
	return cs
}

// jobPayload is the outbox payload for an enqueued job: the spec plus the trusted lineage (originating actor,
// trigger, correlation) the lane uses to mint a trusted proposal candidate.
type jobPayload struct {
	Spec        contract.JobSpec
	Actor       contract.ActorID
	TriggerID   string
	Correlation string
}

// Ingest records an observation exactly-once (S1). Source and Event.Actor are stamped from the AUTHENTICATED
// principal — the client's payload claim is overwritten, never trusted (D7/S9).
//
// Trust boundary (R11/S9/S10): the wire admits ONLY observations. A *.proposed / *.diagnostic is an INTERNAL,
// trusted event class — a *.proposed is minted EXCLUSIVELY by the bridge after the rule pre-gate + write-scope
// check, a *.diagnostic only by the server. The reconciler trusts every *.proposed in the log, and dispatchOne
// SKIPS reserved types (so they bypass the rule pre-gate, bridge write-scope, and readback). Admitting a
// client-supplied one would let an edge write any resource of an authorized KIND outside its dispatched scope
// (a within-kind cross-resource / cross-principal escalation) and dodge the S10 content digest. Reject it at
// the door, before it can enter the canonical log.
func (cs *ControlServer) Ingest(principal contract.ActorID, env contract.ObservationEnvelope) (int64, bool, error) {
	if t := env.Event.Type; strings.HasSuffix(t, ".proposed") || strings.HasSuffix(t, ".diagnostic") {
		return 0, false, fmt.Errorf("ingest: event type %q is internal-only; the wire admits observations, never proposals/diagnostics (R11/S9)", t)
	}
	env.Source = principal
	env.Event.Actor = principal
	return cs.store.IngestObservation(env)
}

// PullProjection serves an actor's scoped, server-built view. The subscription's actor MUST equal the
// authenticated principal (S9/D7): a client can never name another actor's scope on the wire.
func (cs *ControlServer) PullProjection(principal contract.ActorID, sub contract.Subscription) (projection.Projection, error) {
	if sub.Actor != principal {
		return projection.Projection{}, fmt.Errorf("subscription actor %q does not match authenticated principal %q", sub.Actor, principal)
	}
	// S9: serve ONLY the actor's server-CONFIGURED scope. The client may NARROW (request a subset) but never
	// widen — requested refs are intersected with the configured scope, so a client-named out-of-scope ref is
	// never materialized. An empty request defaults to the whole configured scope.
	configured := cs.subs[principal]
	allowed := make(map[contract.ResourceRef]bool, len(configured.Refs))
	for _, r := range configured.Refs {
		allowed[r] = true
	}
	want := sub.Refs
	if len(want) == 0 {
		want = configured.Refs
	}
	var refs []contract.ResourceRef
	for _, r := range want {
		if allowed[r] {
			refs = append(refs, r)
		}
	}
	return projection.ScopedView(cs.store, contract.Subscription{Actor: principal, Refs: refs, PrivacyTier: configured.PrivacyTier}), nil
}

// Tick runs one governed cycle:
//  1. DISPATCH: scan events past the durable dispatch cursor; for each OBSERVED event, build its actor's
//     scoped view, run the rule pre-gate, turn the verdict into trusted events — a propose -> bridged
//     *.proposed event; a deny / rule-error -> a *.diagnostic event (S7, no silent drop). The proposed +
//     diagnostic events AND the cursor advance are ONE atomic DispatchTx (S2).
//  2. RECONCILE: the kernel decides the pending *.proposed events (the kernel is the only writer).
//  3. INVALIDATE: each Accepted decision enqueues an outbox invalidation (downstream projections are stale).
func (cs *ControlServer) Tick() ([]contract.Decision, error) {
	cs.tickMu.Lock() // single-writer: Tick is serialized (the in-memory reconciler cursor is not concurrency-safe)
	defer cs.tickMu.Unlock()
	cur := cs.store.GetCursor(serverDispatchCursor)
	evs, err := cs.store.PendingEvents(cur)
	if err != nil {
		return nil, err // fail-stop on a corrupt log (consistent with RunOnce)
	}
	for _, ev := range evs {
		stamped, jobs, derr := cs.dispatchOne(ev)
		if derr != nil {
			return nil, derr
		}
		// S2: this observed event's produced events + enqueued jobs + the cursor advance are ONE tx.
		if err := cs.store.WithTx(func(tx *store.Tx) error {
			for _, e := range stamped {
				if err := tx.AppendEvent(e); err != nil {
					return err
				}
			}
			for _, j := range jobs {
				if err := tx.EnqueueOutbox(j); err != nil {
					return err
				}
			}
			return tx.SetCursor(serverDispatchCursor, ev.IngestSeq)
		}); err != nil {
			return nil, err
		}
	}
	// 1) decide the rule proposals.
	decisions := cs.reconciler.RunOnce(cs.modes)
	// 2) run the effectful job lane (no-op without a runner); it mints proposal candidates as *.proposed.
	if err := cs.runJobLane(); err != nil {
		return nil, err
	}
	// 3) decide the lane-minted proposals so the full chain closes in one Tick.
	laneDecisions := cs.reconciler.RunOnce(cs.modes)
	// 4) produce each decision's side-effects (S2 invalidation / S7 diagnostic) from the durable decision log,
	//    advancing a sink cursor — so a crash between a decision commit and its side-effects is RECOVERABLE on
	//    the next Tick (the reconciler cursor alone would skip past the un-effected decision).
	if err := cs.processDecisionSideEffects(); err != nil {
		return nil, err
	}
	return append(decisions, laneDecisions...), nil
}

// dispatchOne runs the rule pre-gate for one event and returns the trusted events to append (proposals +
// diagnostics). Events no rule handles (proposals, diagnostics, other domains) produce nothing — the cursor
// still advances past them, so each event is consumed exactly once.
func (cs *ControlServer) dispatchOne(ev contract.Event) ([]contract.Event, []store.OutboxRow, error) {
	// Only OBSERVED events go through the readback check + rule pre-gate. Internal events — a *.proposed event
	// (decided by the reconciler) carries a PROVENANCE digest, a *.diagnostic carries none — must NOT be
	// readback-checked: re-scanning a proposal on a later Tick (its stamped digest now stale vs the current
	// view) would otherwise spuriously trip the readback diagnostic.
	if strings.HasSuffix(ev.Type, ".proposed") || strings.HasSuffix(ev.Type, ".diagnostic") {
		return nil, nil, nil
	}
	view := cs.scopedView(ev.Actor)
	// S10/D8 readback: if the edge echoed the digest it claims to have read, it MUST match the current
	// canonical content digest. A mismatch means the edge acted on tampered/stale content — block the
	// dependent proposal (no write) and surface a stage:readback diagnostic.
	if ev.ContextDigest != "" && ev.ContextDigest != view.Digest {
		return []contract.Event{cs.diagnosticEvent(ev, contract.Diagnostic{
			Stage: "readback", Reason: fmt.Sprintf("echoed digest %q != current %q", ev.ContextDigest, view.Digest), Ref: string(ev.Actor)})}, nil, nil
	}
	dec, diags := cs.rules.Evaluate(rule.RuleInput{Event: ev, View: view})
	var stamped []contract.Event
	var jobs []store.OutboxRow
	for _, dg := range diags { // S7: every rule error is a durable diagnostic.
		stamped = append(stamped, cs.diagnosticEvent(ev, dg))
	}
	switch dec.Verdict {
	case contract.VerdictPropose:
		if dec.Proposal == nil {
			// S7: a propose verdict that carries no proposal is diagnosed, never silently dropped — symmetric
			// with the deny and enqueue_job/request_evidence nil-payload branches.
			stamped = append(stamped, cs.diagnosticEvent(ev, contract.Diagnostic{Stage: "rule", Reason: "verdict propose carried no proposal", Ref: ev.Type}))
			break
		}
		// The bridge write identity comes from the TRUSTED origin the reducer carried (the producing rule's
		// Actor) + the proposal's type (which the reducer enforced == that rule's Emits) — NOT a guess by
		// scanning for any rule with a matching Handles/Emits, which could stamp a different rule's actor.
		if dec.ProposalActor == "" {
			stamped = append(stamped, cs.diagnosticEvent(ev, contract.Diagnostic{Stage: "bridge", Reason: "no rule owns the proposal", Ref: dec.Proposal.Type}))
			break
		}
		b := config.ResolvedBinding{EventType: ev.Type, Actor: dec.ProposalActor, Emits: dec.Proposal.Type}
		e, serr := cs.bridge.Stamp(b, view, ev, *dec.Proposal)
		if serr != nil {
			stamped = append(stamped, cs.diagnosticEvent(ev, contract.Diagnostic{Stage: "bridge", Reason: serr.Error(), Ref: string(b.Actor)}))
			break
		}
		stamped = append(stamped, e)
	case contract.VerdictWarn:
		// S7: a warn surfaces its reasons as a diagnostic — never a silent warn. (The action, if any, still
		// rides whatever verdict won; a standalone warn produces no proposal.)
		stamped = append(stamped, cs.diagnosticEvent(ev, contract.Diagnostic{Stage: "rule", Reason: "warn: " + strings.Join(dec.Reasons, "; "), Ref: ev.Type}))
	case contract.VerdictDeny:
		stamped = append(stamped, cs.diagnosticEvent(ev, contract.Diagnostic{Stage: "rule", Reason: strings.Join(dec.Reasons, "; "), Ref: ev.Type}))
	case contract.VerdictEnqueueJob, contract.VerdictRequestEvidence:
		if dec.Job == nil {
			// S7: a job verdict with no spec is diagnosed, never silently dropped.
			stamped = append(stamped, cs.diagnosticEvent(ev, contract.Diagnostic{Stage: "rule", Reason: "verdict " + string(dec.Verdict) + " carried no job spec", Ref: ev.Type}))
			break
		}
		// S4: enqueue an outbox job. The keyed and keyless id namespaces are DISJOINT ("job_k_"+key vs
		// "job_s_"+seq) so a literal key like "seq_1" can never collide with a keyless id and poison the
		// dispatch loop on the outbox id PK. A non-empty key still dedupes a retry via idempotency_key UNIQUE;
		// a keyless job gets a unique per-observation id from the durable IngestSeq.
		id := "job_k_" + dec.Job.IdempotencyKey
		if dec.Job.IdempotencyKey == "" {
			id = fmt.Sprintf("job_s_%d", ev.IngestSeq)
		}
		payload, _ := json.Marshal(jobPayload{Spec: *dec.Job, Actor: ev.Actor, TriggerID: ev.ID, Correlation: ev.CorrelationID})
		jobs = append(jobs, store.OutboxRow{
			ID: id, Kind: "job", EventSeq: ev.IngestSeq,
			Target: dec.Job.Kind, Payload: string(payload), IdempotencyKey: dec.Job.IdempotencyKey})
	}
	// S7: surface accumulated advisory reasons (e.g. a co-firing warn rule) for the verdicts whose branch does
	// not already emit them (Deny and standalone-Warn do). A warning is never silently dropped, even when a
	// higher-ranked verdict wins.
	if len(dec.Reasons) > 0 && (dec.Verdict == contract.VerdictPropose || dec.Verdict == contract.VerdictEnqueueJob || dec.Verdict == contract.VerdictRequestEvidence) {
		stamped = append(stamped, cs.diagnosticEvent(ev, contract.Diagnostic{Stage: "warn", Reason: "warn: " + strings.Join(dec.Reasons, "; "), Ref: ev.Type}))
	}
	return stamped, jobs, nil
}

// runJobLane is one pass of the effectful job lane (S4/S5). It claims pending outbox jobs, for each takes a
// FENCED lease (job.Claim), runs the injected Runner, writes a receipt + releases the lease (job.Finish),
// then mints the runner's proposal candidate into a TRUSTED *.proposed event (via the bridge, stamped as the
// originating actor) and acks the outbox. The kernel never performs the effect — it only commits the receipt
// and decides the minted proposal. No-op when no runner is configured.
func (cs *ControlServer) runJobLane() error {
	if cs.runner == nil {
		return nil
	}
	// the lane claims ONLY job rows — leasing an invalidation row it never delivers would churn that row's
	// lease/attempts every Tick and starve its real delivery worker (S2).
	claimed, err := cs.store.ClaimOutbox(string(cs.laneOwner), time.Duration(cs.laneTTL)*time.Second, "job")
	if err != nil {
		return err
	}
	for _, row := range claimed {
		if row.Kind != "job" {
			continue // defensive: ClaimOutbox already filtered to job rows
		}
		var jp jobPayload
		if err := json.Unmarshal([]byte(row.Payload), &jp); err != nil {
			continue
		}
		trigger := contract.Event{ID: jp.TriggerID, Type: "job.observed", Actor: jp.Actor, CorrelationID: jp.Correlation}
		// The receipt/dedup identity is the outbox ROW ID, whose keyed/keyless namespaces are already DISJOINT
		// ("job_k_"+key vs "job_s_"+seq). Keying the receipt by the raw idempotency key reopened the collision one
		// layer down: a keyed job whose literal key equals a keyless row id ("job_s_<seq>") forged that keyless
		// job's receipt, silently dropping one of two distinct effects (S4). The row id is disjoint by
		// construction and still stable across a keyed retry (the outbox UNIQUE(idempotency_key) yields one row
		// per key), so two distinct jobs always get two distinct receipts.
		effectKey := row.ID
		// Idempotent recovery: if the effect's receipt already exists (it ran, perhaps before a crash that
		// preceded the ack), do NOT re-run — re-mint the proposal recorded in the receipt (so a crash between
		// Finish and the mint does not lose the governed write), then ack so the row drains.
		if v, fields, _ := cs.store.GetResource(contract.ResourceRef{Kind: "receipt", ID: contract.ResourceID(effectKey)}); v != 0 {
			if err := cs.remintFromReceipt(jp, fields); err != nil {
				return err
			}
			_ = cs.store.AckOutbox(row.ID, string(cs.laneOwner))
			continue
		}
		lease, err := job.Claim(cs.kernel, row.ID, cs.laneOwner, cs.nowUnix(), cs.laneTTL)
		if err != nil {
			continue // another worker holds the fenced lease (contention, not a drop)
		}
		result, err := cs.runner.Run(jp.Spec)
		if err != nil {
			// S7: a runner failure is durable, not silent. The row stays claimed -> retried after lease expiry.
			if _, aerr := cs.store.AppendEvent(cs.diagnosticEvent(trigger, contract.Diagnostic{Stage: "runner", Reason: err.Error(), Ref: jp.Spec.Kind})); aerr != nil {
				return aerr
			}
			continue
		}
		// Key the receipt by the dedup identity (idempotency key, or the unique row id for a keyless job), not
		// the runner's effect id.
		result.EffectID = effectKey
		if err := job.Finish(cs.kernel, lease, result, cs.nowUnix()); err != nil {
			// S7: a stale-fence / duplicate-effect finish is diagnosed (and the row is not acked -> retried).
			if _, aerr := cs.store.AppendEvent(cs.diagnosticEvent(trigger, contract.Diagnostic{Stage: "finish", Reason: err.Error(), Ref: row.ID})); aerr != nil {
				return aerr
			}
			continue
		}
		if result.ProposalCandidate != nil {
			view := cs.scopedView(jp.Actor)
			b := config.ResolvedBinding{Actor: jp.Actor, Emits: result.ProposalCandidate.Type}
			if e, serr := cs.bridge.Stamp(b, view, trigger, *result.ProposalCandidate); serr != nil {
				// S7: an out-of-scope lane proposal is dropped with a diagnostic, never silently.
				if _, aerr := cs.store.AppendEvent(cs.diagnosticEvent(trigger, contract.Diagnostic{Stage: "bridge", Reason: serr.Error(), Ref: string(jp.Actor)})); aerr != nil {
					return aerr
				}
			} else if _, aerr := cs.store.AppendEvent(e); aerr != nil {
				return aerr
			}
		}
		_ = cs.store.AckOutbox(row.ID, string(cs.laneOwner))
	}
	return nil
}

// remintFromReceipt re-mints the proposal recorded in a completed effect's receipt (recovery after a crash
// between Finish and the original mint). It is idempotent at the state level: if the proposal was already
// minted+applied, the re-minted one races the same version and the kernel CAS defers it (no double-write).
// If the recorded proposal is OUT OF SCOPE, the bridge rejects it and recovery emits a stage:bridge diagnostic
// — mirroring the live lane path's no-silent-drop guarantee (S7), never swallowing the reject while acking.
func (cs *ControlServer) remintFromReceipt(jp jobPayload, receiptFields map[string]any) error {
	raw, ok := receiptFields["proposal"].(string)
	if !ok || raw == "" {
		return nil
	}
	var cand contract.ProposedEvent
	if json.Unmarshal([]byte(raw), &cand) != nil {
		return nil
	}
	view := cs.scopedView(jp.Actor)
	b := config.ResolvedBinding{Actor: jp.Actor, Emits: cand.Type}
	trigger := contract.Event{ID: jp.TriggerID, Type: "job.observed", Actor: jp.Actor, CorrelationID: jp.Correlation}
	e, serr := cs.bridge.Stamp(b, view, trigger, cand)
	if serr != nil {
		_, aerr := cs.store.AppendEvent(cs.diagnosticEvent(trigger, contract.Diagnostic{Stage: "bridge", Reason: serr.Error(), Ref: string(jp.Actor)}))
		return aerr
	}
	_, aerr := cs.store.AppendEvent(e)
	return aerr
}

// scopedView builds the actor's scoped projection. (P2 strengthens the scoping + digest behind this seam;
// the call site stays stable.)
func (cs *ControlServer) scopedView(actor contract.ActorID) projection.Projection {
	return projection.ScopedView(cs.store, cs.subs[actor])
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

// processDecisionSideEffects produces every not-yet-effected decision's side-effects from the DURABLE decision
// log: an Accepted decision enqueues an outbox invalidation (S2 downstream propagation); a non-Accepted one
// appends a diagnostic naming WHY (S7 — no silent drop). Each decision's side-effect AND the sink-cursor
// advance are ONE tx, so a crash leaves the sink exactly at what committed: on restart the gap decisions are
// re-derived (the invalidation is idempotent via its UNIQUE key; the diagnostic, being atomic with the sink
// advance, is appended exactly once). Direct (non-reconciler) applies carry IngestSeq 0 and produce no
// side-effect — the sink just advances past them.
func (cs *ControlServer) processDecisionSideEffects() error {
	cur := cs.store.GetCursor(decisionSinkCursor)
	decs, err := cs.store.DecisionsAfter(cur)
	if err != nil {
		return err
	}
	for _, dr := range decs {
		d := dr.Decision
		rid := dr.Rowid
		if e := cs.store.WithTx(func(tx *store.Tx) error {
			if d.IngestSeq > 0 {
				if d.Status == contract.Accepted {
					payload, _ := json.Marshal(d.NewVersions)
					key := "inv_" + d.DecisionID
					if err := tx.EnqueueOutbox(store.OutboxRow{ID: key, Kind: "invalidation", EventSeq: d.IngestSeq, Target: "projection", Payload: string(payload), IdempotencyKey: key}); err != nil {
						return err
					}
					if d.Actor != SyncImportActor {
						if err := tx.RecordSyncCommitsTx(d, syncableResourceKinds); err != nil {
							return err
						}
					}
				} else if err := tx.AppendEvent(cs.rejectDiagnostic(d)); err != nil {
					return err
				}
			}
			return tx.SetCursor(decisionSinkCursor, rid)
		}); e != nil {
			return e
		}
	}
	return nil
}

// rejectDiagnostic turns a kernel reject/defer into a durable "*.diagnostic" event (S7). A CAS/read-stale
// conflict names the raced ResourceVersion (kind/id@actual); a schema/authz reject carries the kernel's
// reason, which already names actor×kind/field. The domain is the conflict's resource kind when present.
func (cs *ControlServer) rejectDiagnostic(d contract.Decision) contract.Event {
	stage, reason, ref, domain := "kernel", d.Reason, string(d.Actor), "control"
	if len(d.Conflicts) > 0 {
		c := d.Conflicts[0]
		reason = fmt.Sprintf("conflict on %s/%s: expected v%d, actual v%d (%s)", c.Ref.Kind, c.Ref.ID, c.ExpectedVersion, c.ActualVersion, c.Kind)
		ref = fmt.Sprintf("%s/%s@%d", c.Ref.Kind, c.Ref.ID, c.ActualVersion)
		domain = string(c.Ref.Kind)
	}
	return contract.Event{
		SchemaVersion: 1, ID: cs.newID(), TS: cs.now(),
		Type: domain + ".diagnostic", Actor: d.Actor, CorrelationID: d.CorrelationID, CausedBy: d.OpID,
		Payload: map[string]any{"stage": stage, "reason": reason, "ref": ref, "decision": string(d.Status)},
	}
}
