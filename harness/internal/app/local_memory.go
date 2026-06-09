package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/mnemon-dev/mnemon/harness/internal/assembler"
	"github.com/mnemon-dev/mnemon/harness/internal/capability"
	"github.com/mnemon-dev/mnemon/harness/internal/channel"
	"github.com/mnemon-dev/mnemon/harness/internal/config"
	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/driver"
	"github.com/mnemon-dev/mnemon/harness/internal/hostsurface"
	"github.com/mnemon-dev/mnemon/harness/internal/kernel"
	"github.com/mnemon-dev/mnemon/harness/internal/rule"
	"github.com/mnemon-dev/mnemon/harness/internal/runtime"
)

// OpenLocalRuntime boots Local Mnemon over the select-only assembler: loops (from the setup-written
// localConfig) enable capabilities; bindings stay the source of truth for observe/pull/status scope.
// An empty loops list (the hidden `local run --bindings` path, which has no localConfig) derives
// enablement from the binding scope kinds ∩ capability.Builtins.
func OpenLocalRuntime(storePath string, loaded channel.LoadedBindings, loops []string) (*runtime.Runtime, error) {
	if len(loops) == 0 {
		loops = loopsFromBindings(loaded.Bindings)
	}
	rc, err := assembler.Assemble(capabilityFileFromLoops(loops), loaded.Bindings)
	if err != nil {
		return nil, err
	}
	return runtime.OpenRuntime(storePath, rc)
}

// LocalRuntimeConfigFromBindings derives Local Mnemon's policy from the installed Agent Integration
// bindings alone (enablement = binding scope kinds ∩ Builtins). It is the bindings-only convenience
// over the same select-only assembly OpenLocalRuntime uses.
func LocalRuntimeConfigFromBindings(bindings []channel.ChannelBinding) (runtime.RuntimeConfig, error) {
	return assembler.Assemble(capabilityFileFromLoops(loopsFromBindings(bindings)), bindings)
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

// loopsFromBindings derives capability enablement from binding scope kinds ∩ Builtins.
func loopsFromBindings(bindings []channel.ChannelBinding) []string {
	seen := map[string]bool{}
	var loops []string
	for _, b := range bindings {
		for _, ref := range b.SubscriptionScope {
			id := string(ref.Kind)
			if _, ok := capability.Builtins[id]; ok && !seen[id] {
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
	Loops       []string
	Hosts       map[string][]string
	ProjectRoot string
}

// RunLocalHTTPServerWithBindings serves Local Mnemon from a binding manifest. It is the product boot
// path used by `mnemon-harness local run`. When opts.Hosts is non-empty it co-hosts the Background
// Driver (plan 3.4): one goroutine in the SAME process — never a second store opener — driving
// Tick + DrainOutbox and re-projecting each recorded host's managed definition files when an
// invalidation drained. A driver error stops the driver (logged to stderr); the hot path serves on.
func RunLocalHTTPServerWithBindings(ctx context.Context, addr, storePath string, loaded channel.LoadedBindings, opts ServeOptions, out io.Writer) error {
	rt, err := OpenLocalRuntime(storePath, loaded, opts.Loops)
	if err != nil {
		return err
	}
	defer rt.Close()
	if reproject := reprojectForHosts(opts.Hosts, opts.ProjectRoot); reproject != nil {
		d := driver.New(rt, reproject, 0)
		go func() {
			if err := d.Run(ctx); err != nil && ctx.Err() == nil {
				fmt.Fprintf(os.Stderr, "mnemon-harness: background driver stopped: %v\n", err)
			}
		}()
	}
	return runtime.ServeRuntime(ctx, addr, rt, channel.NewBindingAuthenticator(loaded), out)
}

// reprojectForHosts builds the driver's re-projection callback over every recorded host surface
// (deterministic host order). nil when no hosts are recorded — old installs get no background
// re-projection until a setup rerun records the hosts map.
func reprojectForHosts(hosts map[string][]string, projectRoot string) func() error {
	if len(hosts) == 0 {
		return nil
	}
	names := make([]string, 0, len(hosts))
	for h := range hosts {
		names = append(names, h)
	}
	sort.Strings(names)
	return func() error {
		for _, host := range names {
			if len(hosts[host]) == 0 {
				continue
			}
			if _, err := hostsurface.ReProject(hostsurface.ProjectContext{
				Host:        host,
				ProjectRoot: projectRoot,
				Loops:       hosts[host],
			}, nil); err != nil {
				return fmt.Errorf("re-project %s: %w", host, err)
			}
		}
		return nil
	}
}

func OpenSyncImportRuntime(storePath string, refs []contract.ResourceRef) (*runtime.Runtime, error) {
	return runtime.OpenRuntime(storePath, SyncImportRuntimeConfig(refs))
}

// SyncImportRuntimeConfig is the sync-import policy. Remote import is memory/skill-only by design:
// the two import rules carry genuinely different merge semantics and are NOT derived from the
// capability descriptors — revisit when a third capability gains a remote producer.
func SyncImportRuntimeConfig(refs []contract.ResourceRef) runtime.RuntimeConfig {
	return runtime.RuntimeConfig{
		Subs: map[contract.ActorID]contract.Subscription{
			contract.SyncImportActor: {Actor: contract.SyncImportActor, Refs: refs},
		},
		Rules: rule.NewRuleSet(capability.RemoteMemoryImportRule(contract.SyncImportActor), capability.RemoteSkillImportRule(contract.SyncImportActor)),
		Authority: kernel.AuthorityRules{Allow: map[contract.ActorID][]contract.ResourceKind{
			contract.SyncImportActor: {"memory", "skill"},
		}},
	}
}
