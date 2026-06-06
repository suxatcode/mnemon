package server

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/kernel"
	"github.com/mnemon-dev/mnemon/harness/core/projection"
	"github.com/mnemon-dev/mnemon/harness/core/rule"
)

// Runtime is the ONE server-owned governed runtime: it owns the canonical kernel store, the kernel,
// the ControlServer (the ServerAPI channel boundary), the single Tick driver, and shutdown. Every
// Agent Surface reaches the engine through this one runtime — never by opening the store itself:
//
//   - service mode: a long-lived `mnemon-harness server` owns the runtime; HostAgent / ControlAgent
//     surfaces call it through server.Client over the channel and never touch the store directly.
//   - embedded mode: a CLI/app Agent Surface opens the runtime, ingests + processes one operation,
//     and closes it — no long-lived server owns the store concurrently.
//
// At any instant there is exactly ONE store owner and ONE dispatch-cursor driver (S11 single-writer):
// the runtime holds the kernel store's single-writer lock for its lifetime, so an embedded opener and
// a live server can never own the same store at once.
type Runtime struct {
	store     *kernel.Store
	cs        *ControlServer
	api       ServerAPI // cs, or an authorizedAPI wrapping cs when Bindings are configured
	storePath string
	bindings  *BindingSet // nil when unbound (embedded/trusted owner)
}

// RuntimeConfig selects the runtime's policy: the rule pre-gate set, the kernel authority, the
// per-principal subscription scopes, the reconcile modes, and the id/clock generators. The zero
// config boots a BARE channel endpoint (empty rules + no preconfigured actors): it records
// observations and serves scoped projections, which is what `mnemon-harness server` uses. NewID/Now
// default to uuid/RFC3339; Modes defaults to reject + projection-read-set + strict authz.
type RuntimeConfig struct {
	Rules     rule.RuleSet
	Authority kernel.AuthorityRules
	Subs      map[contract.ActorID]contract.Subscription
	Modes     contract.Modes
	NewID     func() string
	Now       func() string

	// Bindings, when non-empty, gates the runtime's channel API with a BindingSet authorizer (P2.1):
	// every principal must have a binding granting the verb / observed type / pull scope it uses. The
	// zero (nil) leaves the API unbound — correct for a trusted in-process owner (embedded coreengine).
	Bindings []ChannelBinding
}

func (cfg RuntimeConfig) withDefaults() RuntimeConfig {
	if cfg.NewID == nil {
		cfg.NewID = func() string { return uuid.NewString() }
	}
	if cfg.Now == nil {
		cfg.Now = func() string { return time.Now().UTC().Format(time.RFC3339) }
	}
	if cfg.Modes == (contract.Modes{}) {
		cfg.Modes = contract.Modes{Conflict: contract.ConflictReject, Isolation: contract.IsolationProjectionReadSet, Authz: contract.AuthzStrict}
	}
	if cfg.Subs == nil {
		cfg.Subs = map[contract.ActorID]contract.Subscription{}
	}
	return cfg
}

// OpenRuntime opens (or creates) the kernel store at storePath and wires the one ControlServer over
// it per cfg. storePath "" defaults to DefaultStorePath. The caller MUST Close the runtime; while it
// is open it is the sole owner of the store (S11). A failure to create the store dir or open the
// store is returned, never panicked.
func OpenRuntime(storePath string, cfg RuntimeConfig) (*Runtime, error) {
	if storePath == "" {
		storePath = DefaultStorePath
	}
	if dir := filepath.Dir(storePath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create control store dir: %w", err)
		}
	}
	store, err := kernel.OpenStore(storePath)
	if err != nil {
		return nil, fmt.Errorf("open kernel store: %w", err)
	}
	cfg = cfg.withDefaults()
	k := kernel.NewKernel(store, kernel.DefaultSchemaGuard(), cfg.Authority)
	cs := New(store, k, cfg.Rules, cfg.Subs, cfg.Modes, cfg.NewID, cfg.Now)
	rt := &Runtime{store: store, cs: cs, api: cs, storePath: storePath}
	if len(cfg.Bindings) > 0 {
		bindings, err := NewBindingSet(cfg.Bindings...)
		if err != nil {
			_ = store.Close()
			return nil, fmt.Errorf("channel bindings: %w", err)
		}
		rt.bindings = bindings
		rt.api = NewAuthorizedAPI(cs, bindings)
	}
	return rt, nil
}

// API returns the channel boundary (ServerAPI: observe via Ingest, pull via PullProjection) every
// surface speaks to: the bare ControlServer, or — when bindings are configured — a BindingSet
// authorizer wrapping it (P2.1). The Tick driver and read helpers stay on the unwrapped runtime.
func (r *Runtime) API() ServerAPI { return r.api }

// StorePath is the canonical store path this runtime owns (status/diagnostic evidence).
func (r *Runtime) StorePath() string { return r.storePath }

// BindingKind reports the principal's bound actor kind, when a binding is configured.
func (r *Runtime) BindingKind(principal contract.ActorID) (ActorKind, bool) {
	if r.bindings == nil {
		return "", false
	}
	b, ok := r.bindings.Binding(principal)
	if !ok {
		return "", false
	}
	return b.ActorKind, true
}

// Tick drives one governed cycle. The runtime owns the SINGLE dispatch-cursor driver — no surface
// drives Tick independently against the store.
func (r *Runtime) Tick() ([]contract.Decision, error) { return r.cs.Tick() }

// Resource reads one canonical resource's version + fields directly from the store. It is a
// read-after-decision helper for the OWNING surface (read-only — never a second writer).
func (r *Runtime) Resource(ref contract.ResourceRef) (contract.Version, map[string]any, error) {
	return r.store.GetResource(ref)
}

// Projection serves a scoped view straight from the store for the owning surface's read-after-write
// checks (the wire path is API().PullProjection, which adds the principal/scope enforcement).
func (r *Runtime) Projection(sub contract.Subscription) projection.Projection {
	return projection.ScopedView(r.store, sub)
}

// PendingEvents exposes the durable event log past seq for the owning surface (e.g. recovering a
// refusal diagnostic after a denied apply). Read-only.
func (r *Runtime) PendingEvents(afterSeq int64) ([]contract.Event, error) {
	return r.store.PendingEvents(afterSeq)
}

// Close releases the store and its single-writer lock. After Close the runtime no longer owns the
// store, so another owner (embedded or service) may take it (S11).
func (r *Runtime) Close() error { return r.store.Close() }
