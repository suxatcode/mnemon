// Package assembler is the select-only Loop/Capability Assembler: it compiles a config.File (which
// built-in capabilities are enabled + how they are bound/limited) plus the channel bindings into a
// runtime.RuntimeConfig. It only SELECTS already-compiled built-in capabilities from
// capability.Builtins (resolved via the native:<id> rule_ref); an unknown capability id fails closed.
// Config can never define new behavior — the canonical state still flows observed -> rule -> kernel.
package assembler

import (
	"fmt"
	"strings"

	"github.com/mnemon-dev/mnemon/harness/internal/capability"
	"github.com/mnemon-dev/mnemon/harness/internal/channel"
	"github.com/mnemon-dev/mnemon/harness/internal/config"
	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/kernel"
	"github.com/mnemon-dev/mnemon/harness/internal/rule"
	"github.com/mnemon-dev/mnemon/harness/internal/runtime"
)

// Assemble derives the Local Mnemon runtime config from the enabled capabilities in cfg and the
// installed channel bindings. For each enabled capability it resolves the built-in descriptor by
// rule_ref (fail-closed on an unknown id), then builds one actor-bound rule per binding that may
// observe the capability's type, granting that principal kernel write authority for the resource kind.
//
// Divergence from the locked Assemble(cfg, loops) signature (code wins): the runtime config needs the
// channel bindings (principals/scope), which the loop manifests do not carry; bindings are the second
// argument. This is the config-driven replacement for app.LocalRuntimeConfigFromBindings.
func Assemble(cfg config.File, bindings []channel.ChannelBinding) (runtime.RuntimeConfig, error) {
	var rules []rule.Rule
	allow := map[contract.ActorID][]contract.ResourceKind{}
	for name, cc := range cfg.Capabilities {
		if !cc.Enabled {
			continue
		}
		id := strings.TrimPrefix(cc.RuleRef, "native:")
		cap, ok := capability.Builtins[id]
		if !ok {
			return runtime.RuntimeConfig{}, fmt.Errorf("capability %q: unknown rule_ref %q (fail-closed)", name, cc.RuleRef)
		}
		ref, err := parseRef(cc.ResourceRef)
		if err != nil {
			return runtime.RuntimeConfig{}, fmt.Errorf("capability %q: %w", name, err)
		}
		observed := capability.ObservedTypeAndAliases(cap.ObservedType)
		for _, b := range bindings {
			if b.ActorKind != contract.KindHostAgent {
				continue
			}
			if !b.Allows(channel.VerbObserve) || !allowsAnyObservedType(b, observed) {
				continue
			}
			rules = append(rules, cap.Rule(b.Principal, ref, capability.Limits{MaxPayloadBytes: cc.MaxPayloadBytes}))
			allow[b.Principal] = appendKind(allow[b.Principal], cap.ResourceKind)
		}
	}
	return runtime.RuntimeConfig{
		Bindings:  bindings,
		Subs:      channel.SubsFromBindings(bindings),
		Rules:     rule.NewRuleSet(rules...),
		Authority: kernel.AuthorityRules{Allow: allow},
	}, nil
}

func parseRef(s string) (contract.ResourceRef, error) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return contract.ResourceRef{}, fmt.Errorf("resource_ref %q must be \"<kind>/<id>\"", s)
	}
	return contract.ResourceRef{Kind: contract.ResourceKind(parts[0]), ID: contract.ResourceID(parts[1])}, nil
}

func allowsAnyObservedType(b channel.ChannelBinding, types []string) bool {
	for _, t := range types {
		if b.AllowsObservedType(t) {
			return true
		}
	}
	return false
}

func appendKind(kinds []contract.ResourceKind, kind contract.ResourceKind) []contract.ResourceKind {
	for _, k := range kinds {
		if k == kind {
			return kinds
		}
	}
	return append(kinds, kind)
}
