package app

import (
	"context"
	"io"
	"sort"

	"github.com/mnemon-dev/mnemon/harness/internal/assembler"
	"github.com/mnemon-dev/mnemon/harness/internal/capability"
	"github.com/mnemon-dev/mnemon/harness/internal/channel"
	"github.com/mnemon-dev/mnemon/harness/internal/config"
	"github.com/mnemon-dev/mnemon/harness/internal/contract"
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

// RunLocalHTTPServerWithBindings serves Local Mnemon from a binding manifest. It is the product boot
// path used by `mnemon-harness local run`.
func RunLocalHTTPServerWithBindings(ctx context.Context, addr, storePath string, loaded channel.LoadedBindings, loops []string, out io.Writer) error {
	rt, err := OpenLocalRuntime(storePath, loaded, loops)
	if err != nil {
		return err
	}
	defer rt.Close()
	return runtime.ServeRuntime(ctx, addr, rt, channel.NewBindingAuthenticator(loaded), out)
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
