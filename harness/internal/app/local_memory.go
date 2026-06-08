package app

import (
	"context"
	"io"

	"github.com/mnemon-dev/mnemon/harness/internal/capability"
	"github.com/mnemon-dev/mnemon/harness/internal/channel"
	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/kernel"
	"github.com/mnemon-dev/mnemon/harness/internal/rule"
	"github.com/mnemon-dev/mnemon/harness/internal/runtime"
)

var localProjectMemoryRef = contract.ResourceRef{Kind: "memory", ID: "project"}

// OpenLocalRuntime boots Local Mnemon policy over the server runtime: bindings define the Agent
// Integration scope, local rules admit memory candidates, and the kernel remains the single writer.
func OpenLocalRuntime(storePath string, loaded channel.LoadedBindings) (*runtime.Runtime, error) {
	return runtime.OpenRuntime(storePath, LocalRuntimeConfigFromBindings(loaded.Bindings))
}

// LocalRuntimeConfigFromBindings derives Local Mnemon's policy from the installed Agent Integration
// bindings. The binding remains the source of truth for observe/pull/status scope; this only adds the
// local admission rules and kernel authority needed to apply accepted local writes.
func LocalRuntimeConfigFromBindings(bindings []channel.ChannelBinding) runtime.RuntimeConfig {
	rules := append(LocalMemoryRules(bindings), LocalSkillRules(bindings)...)
	return runtime.RuntimeConfig{
		Bindings:  bindings,
		Subs:      channel.SubsFromBindings(bindings),
		Rules:     rule.NewRuleSet(rules...),
		Authority: LocalAuthorityFromBindings(bindings),
	}
}

// RunLocalHTTPServerWithBindings serves Local Mnemon from a binding manifest. It is the product boot
// path used by `mnemon-harness local run`.
func RunLocalHTTPServerWithBindings(ctx context.Context, addr, storePath string, loaded channel.LoadedBindings, out io.Writer) error {
	rt, err := OpenLocalRuntime(storePath, loaded)
	if err != nil {
		return err
	}
	defer rt.Close()
	var auth channel.Authenticator = channel.HeaderAuthenticator{}
	if len(loaded.Tokens) > 0 {
		auth = channel.TokenAuthenticator{Tokens: loaded.Tokens}
	}
	return runtime.ServeRuntime(ctx, addr, rt, auth, out)
}

// LocalAuthorityFromBindings grants each bound principal write authority only for resource kinds it
// can see through its Local Mnemon scope. Wire clients still cannot submit proposals directly.
func LocalAuthorityFromBindings(bindings []channel.ChannelBinding) kernel.AuthorityRules {
	allow := map[contract.ActorID][]contract.ResourceKind{}
	for _, b := range bindings {
		if b.ActorKind != contract.KindHostAgent {
			continue
		}
		seen := map[contract.ResourceKind]bool{}
		for _, ref := range b.SubscriptionScope {
			if ref.Kind == "memory" || ref.Kind == "skill" {
				seen[ref.Kind] = true
			}
		}
		for kind := range seen {
			allow[b.Principal] = append(allow[b.Principal], kind)
		}
	}
	return kernel.AuthorityRules{Allow: allow}
}

// allowsAnyObservedType reports whether the binding admits any of the observed-type aliases — the
// gate that keeps a loop from being stranded when a binding lists only the legacy underscore form
// while the canonical type has converged to dotted.
func allowsAnyObservedType(b channel.ChannelBinding, types []string) bool {
	for _, t := range types {
		if b.AllowsObservedType(t) {
			return true
		}
	}
	return false
}

// LocalMemoryRules creates one actor-bound admission rule per binding that can submit memory
// candidates. Each rule only proposes for its own authenticated principal.
func LocalMemoryRules(bindings []channel.ChannelBinding) []rule.Rule {
	var rules []rule.Rule
	for _, b := range bindings {
		if !b.Allows(channel.VerbObserve) || !allowsAnyObservedType(b, capability.ObservedTypeAndAliases(capability.MemoryWriteCandidateObserved)) {
			continue
		}
		ref, ok := memoryRefForBinding(b)
		if !ok {
			continue
		}
		rules = append(rules, capability.MemoryAdmissionRule(b.Principal, ref))
	}
	return rules
}

func OpenSyncImportRuntime(storePath string, refs []contract.ResourceRef) (*runtime.Runtime, error) {
	return runtime.OpenRuntime(storePath, SyncImportRuntimeConfig(refs))
}

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

func memoryRefForBinding(b channel.ChannelBinding) (contract.ResourceRef, bool) {
	for _, ref := range b.SubscriptionScope {
		if ref == localProjectMemoryRef {
			return ref, true
		}
	}
	for _, ref := range b.SubscriptionScope {
		if ref.Kind == "memory" {
			return ref, true
		}
	}
	return contract.ResourceRef{}, false
}
