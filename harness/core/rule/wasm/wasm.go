// Package wasm is the wazero WASM backend behind the rule seat (D2/D10/S12). A committed .wasm rule is a PURE
// function of typed JSON input: it imports ONLY env.read_state_view (no WASI, no fs/net/clock/random — those
// host funcs are never registered, so they are structurally unavailable), it is bounded by a per-call
// deadline (WithCloseOnContextDone + context.WithTimeout — wazero has no fuel/epoch) and a memory page cap
// (WithMemoryLimitPages), and it is RETURN-ONLY: it never holds a Store/Kernel, so it can describe a decision
// but never perform a write. The same module satisfies the rule.Rule seat as the native backend.
package wasm

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/rule"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// Limits bounds a wasm rule call: a per-call Timeout (wazero has NO fuel/epoch — bounding is the
// context deadline + WithCloseOnContextDone) and a memory page cap (WithMemoryLimitPages).
type Limits struct {
	Timeout  time.Duration
	MemPages uint32
}

type wasmRule struct {
	ctx      context.Context
	runtime  wazero.Runtime
	mod      api.Module
	alloc    api.Function
	evaluate api.Function
	limits   Limits
	// metadata for the rule seat (fixed to the committed module's purpose; the manifest governs promotion).
	id, emits string
	actor     contract.ActorID
	handles   map[string]bool
}

// New instantiates a wasm rule from module bytes. It registers ONLY the env.read_state_view host import (no
// WASI), caps memory, and enables context-deadline interruption. Returns an error if the module fails to
// validate/instantiate (e.g. it imports something other than env.read_state_view, or needs WASI).
func New(ctx context.Context, wasmBytes []byte, limits Limits) (rule.Rule, error) {
	rc := wazero.NewRuntimeConfig().WithCloseOnContextDone(true)
	if limits.MemPages > 0 {
		rc = rc.WithMemoryLimitPages(limits.MemPages)
	}
	rt := wazero.NewRuntimeWithConfig(ctx, rc)
	// the ONLY host import: read_state_view. No WASI, no fs/net/clock/random are ever registered.
	if _, err := rt.NewHostModuleBuilder("env").
		NewFunctionBuilder().
		WithFunc(func(ptr, length uint32) uint32 { return 0 }).
		Export("read_state_view").
		Instantiate(ctx); err != nil {
		rt.Close(ctx)
		return nil, err
	}
	mod, err := rt.InstantiateWithConfig(ctx, wasmBytes, wazero.NewModuleConfig()) // no WASI module config
	if err != nil {
		rt.Close(ctx)
		return nil, err
	}
	alloc, evaluate := mod.ExportedFunction("alloc"), mod.ExportedFunction("evaluate")
	if alloc == nil || evaluate == nil || mod.Memory() == nil {
		rt.Close(ctx)
		return nil, fmt.Errorf("wasm rule must export memory, alloc, and evaluate")
	}
	return &wasmRule{
		ctx: ctx, runtime: rt, mod: mod, alloc: alloc, evaluate: evaluate, limits: limits,
		id: "wasm-allow-if-evidence", actor: "agent", emits: "memory.write.proposed",
		handles: map[string]bool{"memory.observed": true},
	}, nil
}

func (r *wasmRule) ID() string              { return r.id }
func (r *wasmRule) Actor() contract.ActorID { return r.actor }
func (r *wasmRule) Emits() string           { return r.emits }
func (r *wasmRule) Handles(t string) bool   { return r.handles[t] }

// Evaluate marshals the typed input to JSON, hands it to the module via alloc+memory, runs evaluate under a
// per-call deadline, and decodes the returned JSON RuleDecision. On a runaway module the deadline expires and
// wazero returns a sys.ExitError-wrapped error (never a hang). The module can only RETURN a decision.
func (r *wasmRule) Evaluate(in rule.RuleInput) (contract.RuleDecision, error) {
	inJSON, err := json.Marshal(in)
	if err != nil {
		return contract.RuleDecision{}, err
	}
	cctx, cancel := context.WithTimeout(r.ctx, r.limits.Timeout)
	defer cancel()
	allocRes, err := r.alloc.Call(cctx, uint64(len(inJSON)))
	if err != nil {
		return contract.RuleDecision{}, err
	}
	ptr := uint32(allocRes[0])
	if !r.mod.Memory().Write(ptr, inJSON) {
		return contract.RuleDecision{}, fmt.Errorf("wasm rule: input write out of bounds")
	}
	packed, err := r.evaluate.Call(cctx, uint64(ptr), uint64(len(inJSON)))
	if err != nil {
		return contract.RuleDecision{}, err // deadline (sys.ExitError) or trap — surfaced, never a hang
	}
	outPtr, outLen := uint32(packed[0]>>32), uint32(packed[0])
	out, ok := r.mod.Memory().Read(outPtr, outLen)
	if !ok {
		return contract.RuleDecision{}, fmt.Errorf("wasm rule: output read out of bounds")
	}
	var dec contract.RuleDecision
	if err := json.Unmarshal(out, &dec); err != nil {
		return contract.RuleDecision{}, fmt.Errorf("wasm rule: decode decision: %w", err)
	}
	return dec, nil
}

// Close releases the wazero runtime.
func (r *wasmRule) Close() error { return r.runtime.Close(r.ctx) }
