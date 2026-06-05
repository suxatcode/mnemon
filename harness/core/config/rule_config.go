package config

import (
	"fmt"
	"strings"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/rule"
)

// RuleBinding binds an OBSERVED event type to an admission rule selected by KEY from a trusted in-process map.
// The key is never a path: a path string becoming executable behavior is the exact anti-pattern the resolver
// forbids (C7, mirroring callback selection).
type RuleBinding struct {
	EventType string
	Rule      string
}

// RuleConfig is the select-only rule-pre-gate config: the admission analog of the callback BindingConfig.
type RuleConfig struct {
	Bindings []RuleBinding
}

// ResolveRules SELECTS rules from a trusted registry and validates every binding against the fixed catalogs:
// the EventType must be a non-empty OBSERVED type (a .proposed EventType would make a rule fire on a proposal
// and emit another — a self-amplifying loop, R4); the key must resolve to a non-nil registered rule (paths /
// nil rejected, C7); the selected rule must actually Handle that EventType; its Actor must be declared; and it
// may only Emit a .proposed type. It composes existing trusted rules but introduces no new behavior.
func ResolveRules(rc RuleConfig, registry map[string]rule.Rule, actors map[contract.ActorID][]contract.ResourceKind) (rule.RuleSet, error) {
	var rules []rule.Rule
	for _, b := range rc.Bindings {
		if b.EventType == "" || strings.HasSuffix(b.EventType, ".proposed") {
			return rule.RuleSet{}, fmt.Errorf("rule binding EventType %q must be a non-empty observed type, not a .proposed type", b.EventType)
		}
		r, ok := registry[b.Rule]
		if !ok || r == nil {
			return rule.RuleSet{}, fmt.Errorf("rule %q is not a registered rule (paths forbidden; nil rejected)", b.Rule)
		}
		if !r.Handles(b.EventType) {
			return rule.RuleSet{}, fmt.Errorf("rule %q does not handle event type %q", b.Rule, b.EventType)
		}
		if _, ok := actors[r.Actor()]; !ok {
			return rule.RuleSet{}, fmt.Errorf("rule %q actor %q is not a declared actor", b.Rule, r.Actor())
		}
		if !strings.HasSuffix(r.Emits(), ".proposed") {
			return rule.RuleSet{}, fmt.Errorf("rule %q emits %q must end in .proposed", b.Rule, r.Emits())
		}
		// SELECT-ONLY scoping: a binding selects ONE event type, but a registry rule may Handle several. Append
		// a wrapper whose Handles is restricted to exactly b.EventType, so the rule fires only on the bound type
		// — never on the others it happens to handle (a rule handling memory.observed AND goal.observed, bound
		// only to memory.observed, must not fire on goal.observed; define≠select).
		rules = append(rules, boundRule{inner: r, eventType: b.EventType})
	}
	return rule.NewRuleSet(rules...), nil
}

// boundRule restricts a selected rule's Handles to exactly its bound EventType. All other behavior (identity,
// emit type, evaluation) delegates to the inner rule unchanged.
type boundRule struct {
	inner     rule.Rule
	eventType string
}

func (b boundRule) ID() string                                            { return b.inner.ID() }
func (b boundRule) Actor() contract.ActorID                               { return b.inner.Actor() }
func (b boundRule) Emits() string                                         { return b.inner.Emits() }
func (b boundRule) Handles(t string) bool                                 { return t == b.eventType }
func (b boundRule) Evaluate(in rule.RuleInput) (contract.RuleDecision, error) { return b.inner.Evaluate(in) }
