package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/mnemon-dev/mnemon/harness/internal/channel"
	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/job"
	"github.com/mnemon-dev/mnemon/harness/internal/kernel"
	"github.com/mnemon-dev/mnemon/harness/internal/rule"
	"github.com/mnemon-dev/mnemon/harness/internal/store"
)

// Runtime is the server-owned governed runtime: it owns the canonical kernel
// store, the kernel, the ControlServer channel boundary, the single Tick driver,
// and shutdown. Host surfaces reach the engine through this runtime over
// server.Client, rather than opening the store directly.
//
// At any instant there is exactly ONE store owner and ONE dispatch-cursor driver (S11 single-writer):
// the runtime holds the kernel store's single-writer lock for its lifetime, so an embedded opener and
// a live server can never own the same store at once.
type Runtime struct {
	store     *store.Store
	cs        *ControlServer
	api       channel.ServerAPI // cs, or an authorizedAPI wrapping cs when Bindings are configured
	storePath string
	bindings  *channel.BindingSet // nil when unbound (embedded/trusted owner)
}

// RuntimeConfig selects the runtime's policy: the rule pre-gate set, the kernel authority, the
// per-principal subscription scopes, the reconcile modes, and the id/clock generators. The zero
// config boots a bare channel endpoint with no rules or preconfigured actors. NewID/Now default to
// uuid/RFC3339; Modes defaults to reject + projection-read-set + strict authz.
type RuntimeConfig struct {
	Rules     rule.RuleSet
	Authority kernel.AuthorityRules
	Subs      map[contract.ActorID]contract.Subscription
	Modes     contract.Modes
	NewID     func() string
	Now       func() string

	// Bindings, when non-empty, gates the runtime's channel API with a channel.BindingSet authorizer (P2.1):
	// every principal must have a binding granting the verb / observed type / pull scope it uses. The
	// zero (nil) leaves the API unbound — correct for a trusted in-process owner (embedded coreengine).
	Bindings []channel.ChannelBinding

	// Runner, when non-nil, enables the effectful job lane (S4/S5): jobs the rule pre-gate enqueues are
	// run by Runner under leases owned by LaneOwner, fenced for LaneTTL seconds. A nil Runner leaves the
	// lane OFF — a job verdict is inert. (The assembler sets Runner only when the rule set emits a job
	// verdict; the P0 builtins never do, so it stays nil.)
	Runner    job.Runner
	LaneOwner contract.ActorID
	LaneTTL   int64
}

func (cfg RuntimeConfig) withDefaults() RuntimeConfig {
	if cfg.NewID == nil {
		cfg.NewID = func() string { return uuid.NewString() }
	}
	if cfg.Now == nil {
		cfg.Now = func() string { return time.Now().UTC().Format(time.RFC3339) }
	}
	if cfg.Modes == (contract.Modes{}) {
		cfg.Modes = contract.DefaultModes() // single source with replay.canonicalModes (I6)
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
	// Absolutize so the store ref + the single-writer lockfile are keyed on the CANONICAL path: a
	// relative and an absolute form of the same store must not be treated as two disjoint owners
	// (otherwise the S11 lock cannot catch a split).
	if abs, err := filepath.Abs(storePath); err == nil {
		storePath = abs
	}
	if dir := filepath.Dir(storePath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create control store dir: %w", err)
		}
	}
	store, err := store.OpenStore(storePath)
	if err != nil {
		return nil, fmt.Errorf("open kernel store: %w", err)
	}
	cfg = cfg.withDefaults()
	k := kernel.NewKernel(store, kernel.DefaultSchemaGuard(), cfg.Authority)
	cs := New(store, k, cfg.Rules, cfg.Subs, cfg.Modes, cfg.NewID, cfg.Now)
	if cfg.Runner != nil { // gated lane: configured ONLY when a runner is supplied (a nil runner = no lane)
		cs.WithLane(cfg.Runner, cfg.LaneOwner, func() int64 { return time.Now().Unix() }, cfg.LaneTTL)
	}
	rt := &Runtime{store: store, cs: cs, api: cs, storePath: storePath}
	if len(cfg.Bindings) > 0 {
		bindings, err := channel.NewBindingSet(cfg.Bindings...)
		if err != nil {
			_ = store.Close()
			return nil, fmt.Errorf("channel bindings: %w", err)
		}
		rt.bindings = bindings
		rt.api = channel.NewAuthorizedAPI(cs, bindings)
	}
	return rt, nil
}

// API returns the channel boundary (channel.ServerAPI: observe via Ingest, pull via PullProjection) every
// surface speaks to: the bare ControlServer, or — when bindings are configured — a channel.BindingSet
// authorizer wrapping it (P2.1). The Tick driver and read helpers stay on the unwrapped runtime.
func (r *Runtime) API() channel.ServerAPI { return r.api }

// StorePath is the canonical store path this runtime owns (status/diagnostic evidence).
func (r *Runtime) StorePath() string { return r.storePath }

// Tick drives one governed cycle. The runtime owns the SINGLE dispatch-cursor driver — no surface
// drives Tick independently against the store.
func (r *Runtime) Tick() ([]contract.Decision, error) { return r.cs.Tick() }

// Resource reads one canonical resource's version + fields directly from the store. It is a
// read-after-decision helper for the OWNING surface (read-only — never a second writer).
func (r *Runtime) Resource(ref contract.ResourceRef) (contract.Version, map[string]any, error) {
	return r.store.GetResource(ref)
}

// PendingEvents exposes the durable event log past seq for the owning surface (e.g. recovering a
// refusal diagnostic after a denied apply). Read-only.
func (r *Runtime) PendingEvents(afterSeq int64) ([]contract.Event, error) {
	return r.store.PendingEvents(afterSeq)
}

// Status builds the principal's channel status. When bindings are configured it is gated on the
// binding's channel.VerbStatus (a grant distinct from pull). The digest is the principal's server-configured
// scope, read through the kernel store directly (the server owns the runtime), so status does not
// require the pull verb.
func (r *Runtime) Status(principal contract.ActorID) (contract.ChannelStatus, error) {
	var kind contract.ActorKind
	sub := contract.Subscription{Actor: principal}
	if r.bindings != nil {
		b, ok := r.bindings.Binding(principal)
		if !ok {
			return contract.ChannelStatus{}, fmt.Errorf("no channel binding for principal %q", principal)
		}
		if !b.Allows(channel.VerbStatus) {
			return contract.ChannelStatus{}, fmt.Errorf("principal %q is not bound to status", principal)
		}
		kind = b.ActorKind
		// Clamp the status digest/count to the binding scope (the auditable ceiling), not the broader
		// engine cfg.Subs — the same ClampRefs default the empty-ref pull path uses.
		refs, err := b.ClampRefs(nil)
		if err != nil {
			return contract.ChannelStatus{}, err
		}
		sub.Refs = refs
	}
	proj, err := r.cs.PullProjection(principal, sub)
	if err != nil {
		return contract.ChannelStatus{}, err
	}
	syncCounts, err := r.store.SyncCommitCounts()
	if err != nil {
		return contract.ChannelStatus{}, err
	}
	return contract.ChannelStatus{
		Principal:     principal,
		Digest:        proj.Digest,
		Resources:     len(proj.Resources),
		ActorKind:     kind,
		StoreRef:      r.storePath,
		Mode:          "service",
		SyncPending:   syncCounts.Pending,
		SyncSynced:    syncCounts.Synced,
		SyncConflicts: syncCounts.Conflicts,
	}, nil
}

// DrainOutbox claims, acks, AND PRUNES the pending projection-invalidation outbox rows. It is the
// driver's out-of-band duty, UNCONDITIONAL of the job lane (a second ClaimOutbox caller, kind
// "invalidation", with an owner distinct from the lane). It returns the DEDUPED resource refs the
// drained rows invalidated (the producer stamps d.NewVersions into every row's payload) so the
// driver can refresh selectively, plus the drained row COUNT — re-projection triggers on the count,
// never on the refs, so an undecodable payload loses selectivity but never the trigger. Acked rows
// are pruned in the same pass — nothing re-reads them.
func (r *Runtime) DrainOutbox() ([]contract.ResourceRef, int, error) {
	const owner = "invalidation-driver"
	rows, err := r.store.ClaimOutbox(owner, 60*time.Second, "invalidation")
	if err != nil {
		return nil, 0, err
	}
	seen := map[contract.ResourceRef]bool{}
	var refs []contract.ResourceRef
	for _, row := range rows {
		if err := r.store.AckOutbox(row.ID, owner); err != nil {
			return nil, 0, err
		}
		var versions []contract.ResourceVersion
		if json.Unmarshal([]byte(row.Payload), &versions) == nil {
			for _, v := range versions {
				if !seen[v.Ref] {
					seen[v.Ref] = true
					refs = append(refs, v.Ref)
				}
			}
		}
	}
	if _, err := r.store.DeleteAckedOutbox("invalidation"); err != nil {
		return nil, 0, err
	}
	return refs, len(rows), nil
}

// Close releases the store and its single-writer lock. After Close the runtime no longer owns the
// store, so another owner (embedded or service) may take it (S11).
func (r *Runtime) Close() error { return r.store.Close() }
