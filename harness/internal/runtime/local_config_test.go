package runtime

import (
	"github.com/mnemon-dev/mnemon/harness/internal/capability"
	"github.com/mnemon-dev/mnemon/harness/internal/channel"
	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/kernel"
	"github.com/mnemon-dev/mnemon/harness/internal/rule"
)

// localRuntimeConfigT mirrors app.LocalRuntimeConfigFromBindings for the runtime-level integration
// tests, which exercise the capability rules end-to-end through the runtime (and assert on runtime
// internals). The production derivation lives in app; this keeps the test in package runtime without
// importing app (which would cycle).
func localRuntimeConfigT(bindings []channel.ChannelBinding) RuntimeConfig {
	var rules []rule.Rule
	allow := map[contract.ActorID][]contract.ResourceKind{}
	for _, b := range bindings {
		if b.Allows(channel.VerbObserve) && allowsAnyT(b, capability.ObservedTypeAndAliases(capability.MemoryWriteCandidateObserved)) {
			if ref, ok := scopeRefT(b, "memory"); ok {
				rules = append(rules, capability.MemoryAdmissionRule(b.Principal, ref))
			}
		}
		if b.Allows(channel.VerbObserve) && allowsAnyT(b, capability.ObservedTypeAndAliases(capability.SkillWriteCandidateObserved)) {
			if ref, ok := scopeRefT(b, "skill"); ok {
				rules = append(rules, capability.SkillAdmissionRule(b.Principal, ref))
			}
		}
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
	return RuntimeConfig{
		Bindings:  bindings,
		Subs:      channel.SubsFromBindings(bindings),
		Rules:     rule.NewRuleSet(rules...),
		Authority: kernel.AuthorityRules{Allow: allow},
	}
}

func allowsAnyT(b channel.ChannelBinding, types []string) bool {
	for _, t := range types {
		if b.AllowsObservedType(t) {
			return true
		}
	}
	return false
}

func scopeRefT(b channel.ChannelBinding, kind contract.ResourceKind) (contract.ResourceRef, bool) {
	for _, ref := range b.SubscriptionScope {
		if ref.Kind == kind {
			return ref, true
		}
	}
	return contract.ResourceRef{}, false
}
