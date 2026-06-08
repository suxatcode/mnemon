package app

import (
	"github.com/mnemon-dev/mnemon/harness/internal/capability"
	"github.com/mnemon-dev/mnemon/harness/internal/channel"
	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/rule"
)

var localProjectSkillRef = contract.ResourceRef{Kind: "skill", ID: "project"}

func LocalSkillRules(bindings []channel.ChannelBinding) []rule.Rule {
	var rules []rule.Rule
	for _, b := range bindings {
		if !b.Allows(channel.VerbObserve) || !allowsAnyObservedType(b, capability.ObservedTypeAndAliases(capability.SkillWriteCandidateObserved)) {
			continue
		}
		ref, ok := skillRefForBinding(b)
		if !ok {
			continue
		}
		rules = append(rules, capability.SkillAdmissionRule(b.Principal, ref))
	}
	return rules
}

func skillRefForBinding(b channel.ChannelBinding) (contract.ResourceRef, bool) {
	for _, ref := range b.SubscriptionScope {
		if ref == localProjectSkillRef {
			return ref, true
		}
	}
	for _, ref := range b.SubscriptionScope {
		if ref.Kind == "skill" {
			return ref, true
		}
	}
	return contract.ResourceRef{}, false
}
