// Package coreengine is the host-lifecycle layer's handle to the core kernel as the ONE
// canonical writer (D1). A governed lifecycle write (today: a memory profile entry, on
// proposal approval) is lowered to a core observation that flows through the channel:
// ServerAPI.Ingest -> rule pre-gate -> bridge (write-scope, R11) -> Kernel.Apply. The
// kernel is the single writer of the canonical resource; the caller materializes the host
// file (the .mnemon profile) only AFTER the kernel accepts, so the file is a mirror of the
// canonical state, never an independent writer (P2.1 shim (a) / P2.2 lowering).
//
// A persistent kernel store under the harness dir holds the canonical resources across
// invocations. The store is opened per operation (the kernel's single-writer lock makes
// that safe for the sequential CLI) so no long-lived handle leaks across facade calls.
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

const (
	// memoryActor is the trusted write identity for governed memory entry writes. It is
	// authorized for the "memory" kind only (kernel AuthorityRules), so a forged write to any
	// other kind is rejected at the kernel.
	memoryActor contract.ActorID = "host-memory"
	// observedType is the observation the host-lifecycle layer pushes in; the memory rule
	// turns it into a memory.write.proposed the bridge stamps and the kernel applies.
	observedType = "memory.entry.observed"
)

// MemoryEngine governs memory profile-entry writes through the kernel.
type MemoryEngine struct {
	storePath string
	newID     func() string
	now       func() string
}

// NewMemoryEngine binds an engine to a persistent kernel store under harnessDir. newID/now
// feed the bridge's id/clock; pass deterministic generators in tests, uuid/time in prod.
func NewMemoryEngine(harnessDir string, newID, now func() string) *MemoryEngine {
	return &MemoryEngine{
		storePath: filepath.Join(harnessDir, "control", "memory.db"),
		newID:     newID,
		now:       now,
	}
}

// Result is the outcome of lowering one entry write through the kernel.
type Result struct {
	Accepted bool
	Version  int64
	Reason   string // populated when !Accepted (the rule/bridge/kernel refusal)
}

// AdmitEntry lowers a memory profile entry (identified canonically by entryID, carrying the
// entry's fields) to a governed kernel create. applyID is the idempotency key (the approving
// proposal's id): re-applying the same proposal is a kernel inbox dedup (idempotent), while a
// DIFFERENT proposal targeting an already-canonical entryID is denied by the rule pre-gate.
func (e *MemoryEngine) AdmitEntry(applyID, entryID string, fields map[string]any) (Result, error) {
	if err := os.MkdirAll(filepath.Dir(e.storePath), 0o755); err != nil {
		return Result{}, fmt.Errorf("coreengine: create store dir: %w", err)
	}
	store, err := kernel.OpenStore(e.storePath)
	if err != nil {
		return Result{}, fmt.Errorf("coreengine: open kernel store: %w", err)
	}
	defer store.Close()

	ref := contract.ResourceRef{Kind: "memory", ID: contract.ResourceID(entryID)}
	k := kernel.NewKernel(store, kernel.DefaultSchemaGuard(), kernel.AuthorityRules{
		Allow: map[contract.ActorID][]contract.ResourceKind{memoryActor: {"memory"}},
	})
	subs := map[contract.ActorID]contract.Subscription{
		memoryActor: {Actor: memoryActor, Refs: []contract.ResourceRef{ref}},
	}
	modes := contract.Modes{Conflict: contract.ConflictReject, Isolation: contract.IsolationProjectionReadSet, Authz: contract.AuthzStrict}
	cs := server.New(store, k, rule.NewRuleSet(memoryEntryRule()), subs, modes, e.newID, e.now)

	correlation := "memory:" + applyID
	_, dup, err := cs.Ingest(memoryActor, contract.ObservationEnvelope{
		ExternalID: applyID,
		Event: contract.Event{
			Type:          observedType,
			CorrelationID: correlation,
			Payload:       map[string]any{"entry_id": entryID, "fields": fields},
		},
	})
	if err != nil {
		return Result{}, fmt.Errorf("coreengine: ingest entry: %w", err)
	}
	if dup {
		// Idempotent re-apply: the observation was already recorded (and applied) on a prior
		// call. Report the entry's current canonical version rather than re-deciding.
		v, _, gerr := store.GetResource(ref)
		if gerr != nil {
			return Result{}, fmt.Errorf("coreengine: read deduped entry: %w", gerr)
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
	return Result{Reason: denialReason(store, correlation)}, nil
}

// memoryEntryRule admits a memory.entry.observed into a memory.write.proposed create, or
// denies it when the entry id already exists in the actor's canonical view (duplicate) — the
// duplicate check now lives at the governed rule pre-gate, not the app facade.
func memoryEntryRule() rule.Rule {
	return rule.NewNativeRule("host-memory-entry", memoryActor, "memory.write.proposed", []string{observedType},
		func(in rule.RuleInput) (contract.RuleDecision, error) {
			entryID, _ := in.Event.Payload["entry_id"].(string)
			if entryID == "" {
				return contract.RuleDecision{Verdict: contract.VerdictDeny, Reasons: []string{"memory.entry.observed missing entry_id"}}, nil
			}
			ref := contract.ResourceRef{Kind: "memory", ID: contract.ResourceID(entryID)}
			var cur contract.Version
			for _, rv := range in.View.Resources {
				if rv.Ref == ref {
					cur = rv.Version
				}
			}
			if cur > 0 {
				return contract.RuleDecision{Verdict: contract.VerdictDeny, Reasons: []string{"memory entry " + entryID + " already exists (version " + fmt.Sprint(cur) + ")"}}, nil
			}
			fields, _ := in.Event.Payload["fields"].(map[string]any)
			if fields == nil {
				fields = map[string]any{}
			}
			return contract.RuleDecision{Verdict: contract.VerdictPropose, Proposal: &contract.ProposedEvent{
				Type: "memory.write.proposed",
				Payload: map[string]any{"writes": []contract.ResourceWrite{
					{Ref: ref, Kind: contract.OpCreate, BasedOn: cur, Fields: fields}}},
			}}, nil
		})
}

// denialReason recovers the rule/bridge refusal reason from the durable memory.diagnostic the
// server emitted for this correlation (S7: every refusal is a diagnostic).
func denialReason(store *kernel.Store, correlation string) string {
	events, err := store.PendingEvents(0)
	if err != nil {
		return "kernel refused the write"
	}
	reason := "kernel refused the write"
	for _, ev := range events {
		if ev.Type == "memory.diagnostic" && ev.CorrelationID == correlation {
			if r, ok := ev.Payload["reason"].(string); ok && r != "" {
				reason = r
			}
		}
	}
	return reason
}
