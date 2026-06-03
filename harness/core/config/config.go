package config

import (
	"fmt"
	"strings"

	"github.com/mnemon-dev/mnemon/harness/core/callback"
	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/kernel"
	"github.com/mnemon-dev/mnemon/harness/core/reconcile"
)

type ModeConfig struct{ Conflict, Isolation, Authz string }

// BindingConfig binds an OBSERVED event type to a builtin callback that may emit ONE *.proposed type AS
// a declared actor. Callback is a CATALOG KEY into a trusted in-process builtin map — never a path.
type BindingConfig struct {
	EventType string
	Callback  string
	Actor     contract.ActorID
	Emits     string
}

type RuntimeConfig struct {
	SchemaVersion int
	Modes         ModeConfig
	Actors        map[contract.ActorID][]contract.ResourceKind
	Bindings      []BindingConfig
	Scopes        map[contract.ActorID][]contract.ResourceRef
}

type ResolvedBinding struct {
	EventType string
	Actor     contract.ActorID
	Emits     string
	Callback  callback.Callback
}

type Resolved struct {
	Modes    contract.Modes
	Rules    kernel.AuthorityRules
	Bindings []ResolvedBinding
	Scopes   map[contract.ActorID][]contract.ResourceRef
}

// Resolve SELECTS from the trusted catalogs and executes nothing. Every field is checked against a fixed
// Go-side catalog (mode catalogs in contract, KindCatalog, the provided builtin map, the declared actor
// set). It can compose existing trusted pieces but cannot introduce new conflict semantics, new authz
// teeth, a new resource kind, or executable callback code (Invariant R4/R5/C7).
func Resolve(cfg RuntimeConfig, builtins map[string]callback.Callback) (Resolved, error) {
	if cfg.SchemaVersion != 1 {
		return Resolved{}, fmt.Errorf("unsupported config schema_version %d (want 1)", cfg.SchemaVersion)
	}
	modes, err := reconcile.ResolveModes(reconcile.Config{Conflict: cfg.Modes.Conflict, Isolation: cfg.Modes.Isolation, Authz: cfg.Modes.Authz})
	if err != nil {
		return Resolved{}, err
	}
	for actor, kinds := range cfg.Actors {
		for _, k := range kinds {
			if !contract.KindCatalog[k] {
				return Resolved{}, fmt.Errorf("actor %q: unknown resource kind %q", actor, k)
			}
		}
	}
	for actor, refs := range cfg.Scopes {
		if _, ok := cfg.Actors[actor]; !ok {
			return Resolved{}, fmt.Errorf("scope actor %q is not a declared actor", actor)
		}
		for _, r := range refs {
			if !contract.KindCatalog[r.Kind] {
				return Resolved{}, fmt.Errorf("scope %q: unknown resource kind %q", actor, r.Kind)
			}
		}
	}
	var rbs []ResolvedBinding
	for _, b := range cfg.Bindings {
		// EventType must be a non-empty OBSERVED type. A *.proposed EventType would make a callback fire on
		// a proposal and emit another proposal — a self-amplifying loop (review finding #4).
		if b.EventType == "" || strings.HasSuffix(b.EventType, ".proposed") {
			return Resolved{}, fmt.Errorf("binding EventType %q must be a non-empty observed type, not a .proposed type", b.EventType)
		}
		cb, ok := builtins[b.Callback]
		if !ok || cb == nil {
			return Resolved{}, fmt.Errorf("binding callback %q is not a registered builtin (paths are forbidden; nil is rejected)", b.Callback)
		}
		if _, ok := cfg.Actors[b.Actor]; !ok {
			return Resolved{}, fmt.Errorf("binding actor %q is not a declared actor", b.Actor)
		}
		if !strings.HasSuffix(b.Emits, ".proposed") {
			return Resolved{}, fmt.Errorf("binding emits %q must end in .proposed", b.Emits)
		}
		rbs = append(rbs, ResolvedBinding{EventType: b.EventType, Actor: b.Actor, Emits: b.Emits, Callback: cb})
	}
	return Resolved{Modes: modes, Rules: kernel.AuthorityRules{Allow: cfg.Actors}, Bindings: rbs, Scopes: cfg.Scopes}, nil
}
