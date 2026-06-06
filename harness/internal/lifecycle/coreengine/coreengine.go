// Package coreengine is the host-lifecycle layer's handle to the core kernel as the ONE
// canonical writer (D1). A governed lifecycle write (a memory profile entry or an eval asset
// promotion, on proposal approval) is lowered to a core observation that flows through the
// channel: ServerAPI.Ingest -> rule pre-gate -> bridge (write-scope, R11) -> Kernel.Apply.
// The kernel is the single writer of the canonical resource; the caller materializes the host
// file only AFTER the kernel accepts, so the file is a mirror of the canonical state, never an
// independent writer (P2.2 lowering; the file is the P2.1 transitional mirror shim).
//
// A persistent kernel store under the harness dir holds the canonical resources across
// invocations. The store is opened per operation (the kernel's single-writer lock makes that
// safe for the sequential CLI) so no long-lived handle leaks across facade calls.
package coreengine

import (
	"fmt"
	"os"
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
	if err := os.MkdirAll(filepath.Dir(e.storePath), 0o755); err != nil {
		return Result{}, fmt.Errorf("coreengine: create store dir: %w", err)
	}
	store, err := kernel.OpenStore(e.storePath)
	if err != nil {
		return Result{}, fmt.Errorf("coreengine: open kernel store: %w", err)
	}
	defer store.Close()

	actor := contract.ActorID("host-" + string(kind))
	observed := string(kind) + ".governed.observed"
	ref := contract.ResourceRef{Kind: kind, ID: contract.ResourceID(id)}
	k := kernel.NewKernel(store, kernel.DefaultSchemaGuard(), kernel.AuthorityRules{
		Allow: map[contract.ActorID][]contract.ResourceKind{actor: {kind}},
	})
	subs := map[contract.ActorID]contract.Subscription{
		actor: {Actor: actor, Refs: []contract.ResourceRef{ref}},
	}
	modes := contract.Modes{Conflict: contract.ConflictReject, Isolation: contract.IsolationProjectionReadSet, Authz: contract.AuthzStrict}
	cs := server.New(store, k, rule.NewRuleSet(governedCreateRule(kind, actor, observed)), subs, modes, e.newID, e.now)

	correlation := string(kind) + ":" + applyID
	_, dup, err := cs.Ingest(actor, contract.ObservationEnvelope{
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
		v, _, gerr := store.GetResource(ref)
		if gerr != nil {
			return Result{}, fmt.Errorf("coreengine: read deduped resource: %w", gerr)
		}
		if v > 0 {
			return Result{Accepted: true, Version: int64(v)}, nil
		}
		return Result{Reason: "idempotent re-apply produced no canonical write"}, nil
	}

	decisions, err := cs.Tick()
	if err != nil {
		return Result{}, fmt.Errorf("coreengine: tick: %w", err)
	}
	for _, d := range decisions {
		if d.Status == contract.Accepted {
			v, _, _ := store.GetResource(ref)
			return Result{Accepted: true, Version: int64(v)}, nil
		}
	}
	return Result{Reason: denialReason(store, string(kind)+".diagnostic", correlation)}, nil
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
// emitted for this correlation (S7: every refusal is a diagnostic).
func denialReason(store *kernel.Store, diagnosticType, correlation string) string {
	events, err := store.PendingEvents(0)
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
