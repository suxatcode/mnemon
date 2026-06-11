package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/assembler"
	"github.com/mnemon-dev/mnemon/harness/internal/assets"
	"github.com/mnemon-dev/mnemon/harness/internal/capability"
	"github.com/mnemon-dev/mnemon/harness/internal/channel"
	"github.com/mnemon-dev/mnemon/harness/internal/config"
	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/driver"
	"github.com/mnemon-dev/mnemon/harness/internal/hostsurface"
	"path/filepath"

	"github.com/mnemon-dev/mnemon/harness/internal/kernel"
	"github.com/mnemon-dev/mnemon/harness/internal/manifest"
	"github.com/mnemon-dev/mnemon/harness/internal/rule"
	"github.com/mnemon-dev/mnemon/harness/internal/runtime"
)

// OpenLocalRuntime boots Local Mnemon over the select-only assembler: loops (from the setup-written
// localConfig) enable capabilities; bindings stay the source of truth for observe/pull/status scope.
// An empty loops list (the hidden `local run --bindings` path, which has no localConfig) derives
// enablement from the binding scope kinds ∩ catalog. catalog selects the capability universe
// (nil = capability.Builtins); the serve path passes the boot-resolved external-merged catalog.
// The assembled policy is then merged with the sync-import half (withSyncImport), so the SERVING
// runtime can import pulled commits in-process (v1.1 #2) without a second runtime boot.
func OpenLocalRuntime(storePath string, loaded channel.LoadedBindings, loops []string, catalog map[string]capability.Capability) (*runtime.Runtime, error) {
	if len(loops) == 0 {
		loops = loopsFromBindings(loaded.Bindings, catalog)
	}
	rc, err := assembler.Assemble(capabilityFileFromLoops(loops), loaded.Bindings, catalog)
	if err != nil {
		return nil, err
	}
	return runtime.OpenRuntime(storePath, withSyncImport(rc, loaded.Bindings))
}

// withSyncImport merges the sync-import half into an assembled runtime policy (v1.1 #2): sync@local
// gets the two import rules + the skipped-kind deny rule, kernel authority for the syncable kinds,
// and a subscription covering the binding scope's syncable refs (the import rules read the current
// resource through this view to merge against). Co-existence is by construction: the added rules
// Handle only the remote.* / sync.* observation types AND gate on the sync principal, so host-agent
// events never match them and host rules never see the import events — pinned by a test.
func withSyncImport(rc runtime.RuntimeConfig, bindings []channel.ChannelBinding) runtime.RuntimeConfig {
	rules := append(append([]rule.Rule(nil), rc.Rules.Rules()...),
		capability.RemoteMemoryImportRule(contract.SyncImportActor),
		capability.RemoteSkillImportRule(contract.SyncImportActor),
		capability.SyncImportSkippedRule(contract.SyncImportActor))
	rc.Rules = rule.NewRuleSet(rules...)
	if rc.Subs == nil {
		rc.Subs = map[contract.ActorID]contract.Subscription{}
	}
	rc.Subs[contract.SyncImportActor] = contract.Subscription{Actor: contract.SyncImportActor, Refs: syncableScopeRefs(bindings)}
	if rc.Authority.Allow == nil {
		rc.Authority.Allow = map[contract.ActorID][]contract.ResourceKind{}
	}
	rc.Authority.Allow[contract.SyncImportActor] = []contract.ResourceKind{"memory", "skill"}
	return rc
}

// syncableScopeRefs collects the deduped binding-scope refs of syncable kinds — the resources a
// pulled commit may target on this replica (the same canonical refs the host loops govern).
func syncableScopeRefs(bindings []channel.ChannelBinding) []contract.ResourceRef {
	seen := map[contract.ResourceRef]bool{}
	var refs []contract.ResourceRef
	for _, b := range bindings {
		for _, ref := range b.SubscriptionScope {
			if contract.SyncableResourceKinds[ref.Kind] && !seen[ref] {
				seen[ref] = true
				refs = append(refs, ref)
			}
		}
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Kind != refs[j].Kind {
			return refs[i].Kind < refs[j].Kind
		}
		return refs[i].ID < refs[j].ID
	})
	return refs
}

// LocalRuntimeConfigFromBindings derives Local Mnemon's policy from the installed Agent Integration
// bindings alone (enablement = binding scope kinds ∩ catalog; nil = Builtins). It is the
// bindings-only convenience over the same select-only assembly OpenLocalRuntime uses.
func LocalRuntimeConfigFromBindings(bindings []channel.ChannelBinding, catalog map[string]capability.Capability) (runtime.RuntimeConfig, error) {
	return assembler.Assemble(capabilityFileFromLoops(loopsFromBindings(bindings, catalog)), bindings, catalog)
}

// capabilityFileFromLoops constructs the in-memory config.File for the enabled loops. The on-disk
// localConfig (schema_version 1) stays the enablement authority; config.Load parses the FUTURE
// on-disk form and is not yet the boot reader (do not migrate until a capability needs a knob the
// loops list cannot express).
func capabilityFileFromLoops(loops []string) config.File {
	caps := make(map[string]config.CapabilityConfig, len(loops))
	for _, loop := range loops {
		caps[loop] = config.CapabilityConfig{Enabled: true, ResourceRef: loop + "/project", RuleRef: "native:" + loop}
	}
	return config.File{Capabilities: caps}
}

// loopsFromBindings derives capability enablement from binding scope kinds ∩ catalog (nil =
// Builtins). config.loops stays the product-path authority — this derivation only runs when the
// loops list is empty (the hidden bindings-only path).
func loopsFromBindings(bindings []channel.ChannelBinding, catalog map[string]capability.Capability) []string {
	if catalog == nil {
		catalog = capability.Builtins
	}
	seen := map[string]bool{}
	var loops []string
	for _, b := range bindings {
		for _, ref := range b.SubscriptionScope {
			id := string(ref.Kind)
			if _, ok := catalog[id]; ok && !seen[id] {
				seen[id] = true
				loops = append(loops, id)
			}
		}
	}
	sort.Strings(loops)
	return loops
}

// ServeOptions carries the boot-config state the serve path needs beyond bindings: capability
// enablement (Loops), the per-host projected loops (Hosts — the background driver's re-projection
// authority), and the project root the host surfaces live under.
type ServeOptions struct {
	Loops          []string
	Hosts          map[string][]string
	ProjectRoot    string
	MirrorMode     string // "manual" | "prime-refresh" (driver-side mirror regeneration gate)
	IgnoreExternal bool   // boot the embedded-only catalog, naming each ignored external package on stderr
	// AllowInsecureRemote is the sync worker's T2 downgrade override (v1.1 #3): permit a plaintext
	// non-loopback remote endpoint. Default false — fail closed.
	AllowInsecureRemote bool
	SyncInterval        time.Duration // sync worker cadence; <= 0 = default (30s)
}

// RunLocalHTTPServerWithBindings serves Local Mnemon from a binding manifest. It is the product boot
// path used by `mnemon-harness local run`. When opts.Hosts is non-empty it co-hosts the Background
// Driver (plan 3.4): one goroutine in the SAME process — never a second store opener — driving
// Tick + DrainOutbox and re-projecting each recorded host's managed definition files when an
// invalidation drained. A driver error stops the driver (logged to stderr); the hot path serves on.
func RunLocalHTTPServerWithBindings(ctx context.Context, addr, storePath string, loaded channel.LoadedBindings, opts ServeOptions, out io.Writer) error {
	catalog, ignored, err := resolveBootCatalog(opts.ProjectRoot, opts.IgnoreExternal, os.Stderr)
	if err != nil {
		return err
	}
	rt, err := OpenLocalRuntime(storePath, loaded, disableIgnoredLoops(opts.Loops, ignored, os.Stderr), catalog)
	if err != nil {
		return err
	}
	// Shutdown ordering (MED-5): the background driver and sync worker write through rt's open store
	// on their own goroutines. rt.Close() must not race a mid-flight worker store write, so JOIN both
	// goroutines (they exit promptly on ctx cancel) BEFORE closing the store. Defers run LIFO, so the
	// later-registered wg.Wait() runs FIRST — after ServeRuntime returns (ctx cancelled), then the
	// store closes on a quiesced runtime.
	defer rt.Close()
	var wg sync.WaitGroup
	defer wg.Wait()
	if reproject := serveReproject(rt, loaded, opts.Hosts, opts.ProjectRoot, opts.MirrorMode); reproject != nil {
		d := driver.New(rt, swallowReprojectErrors(reproject, os.Stderr), 0)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := d.Run(ctx); err != nil && ctx.Err() == nil {
				fmt.Fprintf(os.Stderr, "mnemon-harness: background driver stopped: %v\n", err)
			}
		}()
	}
	// The sync worker runs on its OWN goroutine/cadence (never inside driver.Tick — a slow remote
	// must not stall the governed loop; the client is timeout-bounded regardless, v1.1 #2/#10). It
	// self-gates on remotes.json presence: no remote configured = zero sync activity (I13).
	wg.Add(1)
	go func() {
		defer wg.Done()
		RunSyncWorker(ctx, rt, SyncWorkerOptions{
			ProjectRoot:         opts.ProjectRoot,
			AllowInsecureRemote: opts.AllowInsecureRemote,
			Interval:            opts.SyncInterval,
		}, os.Stderr)
	}()
	return runtime.ServeRuntime(ctx, addr, rt, channel.NewBindingAuthenticator(loaded), out)
}

// resolveBootCatalog resolves the capability catalog ONCE at boot. Default: embedded Builtins +
// every external package under <projectRoot>/.mnemon/loops via capability.ResolveCatalog
// (requiredFields = kernel.DefaultSchemaGuard().Required — app owns the kernel import; capability
// stays a contract-level leaf), fail-closed: a bad external package REFUSES to start Local Mnemon
// — the directory's presence is a contract, not a hint. ignoreExternal is the operator escape
// hatch (`local run --ignore-external`): boot the embedded-only catalog and name each ignored
// package on errw, one line per package, so what is offline is visible, never silent. The second
// return is those ignored package names — the serve path must drop them from the enabled loops
// too (disableIgnoredLoops), or an enabled-then-corrupted package would still sink the boot on
// `unknown rule_ref`.
func resolveBootCatalog(projectRoot string, ignoreExternal bool, errw io.Writer) (map[string]capability.Capability, []string, error) {
	if !ignoreExternal {
		catalog, err := capability.ResolveCatalog(projectRoot, kernel.DefaultSchemaGuard().Required)
		return catalog, nil, err
	}
	entries, err := os.ReadDir(filepath.Join(projectRoot, ".mnemon", "loops"))
	if err != nil {
		return capability.Builtins, nil, nil // absent (or unreadable) external root: nothing to ignore
	}
	var ignored []string
	for _, e := range entries {
		if e.IsDir() || e.Type()&os.ModeSymlink != 0 {
			ignored = append(ignored, e.Name())
			fmt.Fprintf(errw, "mnemon-harness: --ignore-external: ignoring external package .mnemon/loops/%s\n", e.Name())
		}
	}
	return capability.Builtins, ignored, nil
}

// disableIgnoredLoops is the loop-list half of --ignore-external: the PRIMARY ignore scenario is
// an external package the operator already ENABLED (config.loops carries its name) that has since
// gone bad. Ignoring only the catalog would still sink boot — the assembler would fail on
// `unknown rule_ref "native:<name>"` — so the ignored package names are dropped from the enabled
// loops too, one stderr line per disabled loop, visible, never silent. Names that match no
// ignored package pass through untouched (a typo in config.loops keeps its diagnostic).
func disableIgnoredLoops(loops, ignored []string, errw io.Writer) []string {
	if len(ignored) == 0 {
		return loops
	}
	skip := map[string]bool{}
	for _, name := range ignored {
		skip[name] = true
	}
	kept := make([]string, 0, len(loops))
	for _, loop := range loops {
		if skip[loop] {
			fmt.Fprintf(errw, "mnemon-harness: --ignore-external: disabling loop %s\n", loop)
			continue
		}
		kept = append(kept, loop)
	}
	return kept
}

// serveReproject builds the driver's reproject callback: (a) re-project every recorded host's
// managed DEFINITION files under no-clobber (cheap no-op when unchanged), and (b) when the
// drained refs touch the memory kind and MirrorMode permits, regenerate each host's derived
// MEMORY.md mirror from a fresh scoped projection (I11: derived, freely regenerated — never
// routed through conflict-preserve). nil when no hosts are recorded — old installs get no
// background re-projection until a setup rerun records the hosts map.
//
// Mirror scope reconciliation: only the memory loop carries a runtime mirror today; the
// loop-declared generic version replaces this helper when loop packages carry mirror
// declarations (stage 3 final form / stage 5 external packages — the stage-2 render catalog
// is the building block, not the trigger).
func serveReproject(rt *runtime.Runtime, loaded channel.LoadedBindings, hosts map[string][]string, projectRoot, mirrorMode string) func(refs []contract.ResourceRef) error {
	if len(hosts) == 0 {
		return nil
	}
	names := make([]string, 0, len(hosts))
	for h := range hosts {
		names = append(names, h)
	}
	sort.Strings(names)
	return func(refs []contract.ResourceRef) error {
		for _, host := range names {
			if len(hosts[host]) == 0 {
				continue
			}
			if _, err := hostsurface.ReProject(hostsurface.ProjectContext{
				Host:        host,
				ProjectRoot: projectRoot,
				Loops:       hosts[host],
			}, refs); err != nil {
				return fmt.Errorf("re-project %s: %w", host, err)
			}
		}
		if mirrorMode == "manual" || !refsTouchKind(refs, "memory") {
			return nil
		}
		principal, ok := mirrorPrincipal(loaded.Bindings)
		if !ok {
			return nil // no memory-scoped host-agent binding: nothing to mirror
		}
		proj, err := rt.API().PullProjection(principal, contract.Subscription{Actor: principal})
		if err != nil {
			return fmt.Errorf("mirror projection: %w", err)
		}
		for _, host := range names {
			if !containsLoop(hosts[host], "memory") {
				continue
			}
			binding, err := manifest.LoadBinding(assets.FS, host, "memory")
			if err != nil {
				return fmt.Errorf("mirror binding %s: %w", host, err)
			}
			path := filepath.Join(projectRoot, filepath.FromSlash(binding.RuntimeSurface), "MEMORY.md")
			if err := hostsurface.WriteMemoryMirror(path, proj); err != nil {
				return fmt.Errorf("mirror %s: %w", host, err)
			}
		}
		return nil
	}
}

// swallowReprojectErrors keeps the background driver alive across reproject failures: the driver
// stops on the FIRST Tick error, and a transient mirror/file failure must never permanently kill
// outbox draining (and with it, pruning) for the process lifetime. Reproject is best-effort —
// log and continue; store-level Tick errors still stop the driver.
func swallowReprojectErrors(reproject func(refs []contract.ResourceRef) error, errw io.Writer) func(refs []contract.ResourceRef) error {
	return func(refs []contract.ResourceRef) error {
		if err := reproject(refs); err != nil {
			fmt.Fprintf(errw, "mnemon-harness: background re-projection: %v\n", err)
		}
		return nil
	}
}

// refsTouchKind reports whether any drained ref is of kind (selective refresh: a skill-only
// write does not regenerate the memory mirror).
func refsTouchKind(refs []contract.ResourceRef, kind contract.ResourceKind) bool {
	for _, r := range refs {
		if r.Kind == kind {
			return true
		}
	}
	return false
}

// mirrorPrincipal picks the projection identity for mirror regeneration: the first (by
// principal, deterministic) host-agent binding whose scope covers the memory kind. The memory
// resource is shared, so any in-scope principal projects identical content.
func mirrorPrincipal(bindings []channel.ChannelBinding) (contract.ActorID, bool) {
	var candidates []channel.ChannelBinding
	for _, b := range bindings {
		if b.ActorKind != contract.KindHostAgent {
			continue
		}
		for _, ref := range b.SubscriptionScope {
			if ref.Kind == "memory" {
				candidates = append(candidates, b)
				break
			}
		}
	}
	if len(candidates) == 0 {
		return "", false
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Principal < candidates[j].Principal })
	return candidates[0].Principal, true
}

func containsLoop(loops []string, name string) bool {
	for _, l := range loops {
		if l == name {
			return true
		}
	}
	return false
}

func OpenSyncImportRuntime(storePath string, refs []contract.ResourceRef) (*runtime.Runtime, error) {
	return runtime.OpenRuntime(storePath, SyncImportRuntimeConfig(refs))
}

// SyncImportRuntimeConfig is the sync-import policy. Remote import is memory/skill-only by design:
// the two import rules carry genuinely different merge semantics and are NOT derived from the
// capability descriptors — revisit when a third capability gains a remote producer. The skipped-kind
// deny rule (v1.1 #4) keeps any OTHER pulled kind a durable diagnostic instead of a silent drop —
// the same three-rule set withSyncImport merges into the serving runtime, so the offline and
// in-process import paths share one policy.
func SyncImportRuntimeConfig(refs []contract.ResourceRef) runtime.RuntimeConfig {
	return runtime.RuntimeConfig{
		Subs: map[contract.ActorID]contract.Subscription{
			contract.SyncImportActor: {Actor: contract.SyncImportActor, Refs: refs},
		},
		Rules: rule.NewRuleSet(
			capability.RemoteMemoryImportRule(contract.SyncImportActor),
			capability.RemoteSkillImportRule(contract.SyncImportActor),
			capability.SyncImportSkippedRule(contract.SyncImportActor)),
		Authority: kernel.AuthorityRules{Allow: map[contract.ActorID][]contract.ResourceKind{
			contract.SyncImportActor: {"memory", "skill"},
		}},
	}
}
