// Package coreengine is the host-lifecycle layer's handle to the core kernel as the ONE
// canonical writer (D1). A governed lifecycle write (a memory profile entry or an eval asset
// promotion, on proposal approval) is lowered to a core observation that flows through the
// channel: ServerAPI.Ingest -> rule pre-gate -> bridge (write-scope, R11) -> Kernel.Apply.
// The kernel is the single writer of the canonical resource; the caller materializes the host
// file only AFTER the kernel accepts, so the file is a mirror of the canonical state, never an
// independent writer (P2.2 lowering; the file is the P2.1 transitional mirror shim).
//
// The canonical resources live in the ONE harness control store (server.DefaultStorePath under the
// project root). Embedded mode: each AdmitCreate opens a server.Runtime over that store, ingests +
// ticks one operation through the channel, and closes it — so the runtime (not coreengine) owns the
// store/kernel/ControlServer/Tick, and the kernel's single-writer lock keeps a per-op opener and a
// live `mnemon-harness server` from owning the store at once (S11). coreengine is a thin lowering
// client over the runtime's ServerAPI, never a second writer.
package coreengine

import (
	"fmt"
	"path/filepath"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/kernel"
	"github.com/mnemon-dev/mnemon/harness/core/rule"
	"github.com/mnemon-dev/mnemon/harness/core/server"
)

// Engine governs lifecycle resource creates through the kernel.
type Engine struct {
	storePath string
	newID     func() string
	now       func() string
}

// New binds an engine to the ONE canonical harness control store, resolved as
// server.DefaultStorePath under the project root — the same path-resolution `mnemon-harness server`
// uses, so a governed lifecycle write is readable by a host-agent pull through the channel (no store
// split). newID/now feed the bridge's id/clock; pass deterministic generators in tests, uuid/time in
// prod.
func New(root string, newID, now func() string) *Engine {
	return &Engine{
		storePath: filepath.Join(root, server.DefaultStorePath),
		newID:     newID,
		now:       now,
	}
}

// Result is the outcome of lowering one create through the kernel.
type Result struct {
	Accepted bool
	Version  int64
	Reason   string // populated when !Accepted (the rule/bridge/kernel refusal)
}

// AdmitCreate lowers a governed resource create to the kernel. kind is a core resource kind
// (memory/skill/goal/...); id is the canonical resource id; fields must include the kind's
// schema-required fields (memory:content, skill:name, goal:statement). applyID is the
// idempotency key (the approving proposal's id): re-applying the same proposal is a kernel
// inbox dedup (idempotent), while a DIFFERENT proposal targeting an already-canonical id is
// denied by the rule pre-gate.
func (e *Engine) AdmitCreate(applyID string, kind contract.ResourceKind, id string, fields map[string]any) (Result, error) {
	actor := contract.ActorID("host-" + string(kind))
	observed := string(kind) + ".governed.observed"
	ref := contract.ResourceRef{Kind: kind, ID: contract.ResourceID(id)}

	// Embedded mode: open the one server-owned runtime over the canonical store, lower this create
	// through its channel, then close it (S11 single-writer — no long-lived server owns the store
	// concurrently). The runtime owns the store/kernel/ControlServer/Tick; coreengine only drives it.
	rt, err := server.OpenRuntime(e.storePath, server.RuntimeConfig{
		Rules:     rule.NewRuleSet(governedCreateRule(kind, actor, observed)),
		Authority: kernel.AuthorityRules{Allow: map[contract.ActorID][]contract.ResourceKind{actor: {kind}}},
		Subs:      map[contract.ActorID]contract.Subscription{actor: {Actor: actor, Refs: []contract.ResourceRef{ref}}},
		Modes:     contract.Modes{Conflict: contract.ConflictReject, Isolation: contract.IsolationProjectionReadSet, Authz: contract.AuthzStrict},
		NewID:     e.newID,
		Now:       e.now,
	})
	if err != nil {
		return Result{}, fmt.Errorf("coreengine: open runtime: %w", err)
	}
	defer rt.Close()

	correlation := string(kind) + ":" + applyID
	_, dup, err := rt.API().Ingest(actor, contract.ObservationEnvelope{
		ExternalID: applyID,
		Event: contract.Event{
			Type:          observed,
			CorrelationID: correlation,
			Payload:       map[string]any{"entry_id": id, "fields": fields},
		},
	})
	if err != nil {
		return Result{}, fmt.Errorf("coreengine: ingest %s create: %w", kind, err)
	}
	if dup {
		// Idempotent re-apply: the observation was already recorded (and applied) on a prior
		// call. Report the resource's current canonical version rather than re-deciding.
		v, _, gerr := rt.Resource(ref)
		if gerr != nil {
			return Result{}, fmt.Errorf("coreengine: read deduped resource: %w", gerr)
		}
		if v > 0 {
			return Result{Accepted: true, Version: int64(v)}, nil
		}
		return Result{Reason: "idempotent re-apply produced no canonical write"}, nil
	}

	decisions, err := rt.Tick()
	if err != nil {
		return Result{}, fmt.Errorf("coreengine: tick: %w", err)
	}
	for _, d := range decisions {
		if d.Status == contract.Accepted {
			v, _, _ := rt.Resource(ref)
			return Result{Accepted: true, Version: int64(v)}, nil
		}
	}
	return Result{Reason: denialReason(rt, string(kind)+".diagnostic", correlation)}, nil
}

// governedCreateRule admits a <kind>.governed.observed into a <kind>.write.proposed create, or
// denies it when the id already exists in the actor's canonical view (duplicate) — the
// duplicate check lives at the governed rule pre-gate, not the app facade.
func governedCreateRule(kind contract.ResourceKind, actor contract.ActorID, observed string) rule.Rule {
	return rule.NewNativeRule("host-"+string(kind)+"-create", actor, string(kind)+".write.proposed", []string{observed},
		func(in rule.RuleInput) (contract.RuleDecision, error) {
			id, _ := in.Event.Payload["entry_id"].(string)
			if id == "" {
				return contract.RuleDecision{Verdict: contract.VerdictDeny, Reasons: []string{string(observed) + " missing entry_id"}}, nil
			}
			ref := contract.ResourceRef{Kind: kind, ID: contract.ResourceID(id)}
			var cur contract.Version
			for _, rv := range in.View.Resources {
				if rv.Ref == ref {
					cur = rv.Version
				}
			}
			if cur > 0 {
				return contract.RuleDecision{Verdict: contract.VerdictDeny, Reasons: []string{string(kind) + " " + id + " already exists (version " + fmt.Sprint(cur) + ")"}}, nil
			}
			fields, _ := in.Event.Payload["fields"].(map[string]any)
			if fields == nil {
				fields = map[string]any{}
			}
			return contract.RuleDecision{Verdict: contract.VerdictPropose, Proposal: &contract.ProposedEvent{
				Type: string(kind) + ".write.proposed",
				Payload: map[string]any{"writes": []contract.ResourceWrite{
					{Ref: ref, Kind: contract.OpCreate, BasedOn: cur, Fields: fields}}},
			}}, nil
		})
}

// denialReason recovers the rule/bridge refusal reason from the durable diagnostic the server
// emitted for this correlation (S7: every refusal is a diagnostic). It reads the runtime's event
// log (read-only — coreengine is never a second writer).
func denialReason(rt *server.Runtime, diagnosticType, correlation string) string {
	events, err := rt.PendingEvents(0)
	if err != nil {
		return "kernel refused the write"
	}
	reason := "kernel refused the write"
	for _, ev := range events {
		if ev.Type == diagnosticType && ev.CorrelationID == correlation {
			if r, ok := ev.Payload["reason"].(string); ok && r != "" {
				reason = r
			}
		}
	}
	return reason
}
