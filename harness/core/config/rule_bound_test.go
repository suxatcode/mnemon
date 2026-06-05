package config

import (
	"testing"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/rule"
)

// A binding SELECTS one event type. A registry rule may Handle several, but the select-only model means the
// resolved rule fires ONLY on the bound type — never on the others it happens to handle (R4 / define≠select).
func TestResolveRulesScopesHandlesToBoundEventType(t *testing.T) {
	denyBoth := rule.NewNativeRule("d", "agent", "memory.write.proposed", []string{"memory.observed", "goal.observed"},
		func(rule.RuleInput) (contract.RuleDecision, error) {
			return contract.RuleDecision{Verdict: contract.VerdictDeny, Reasons: []string{"x"}}, nil
		})
	rc := RuleConfig{Bindings: []RuleBinding{{EventType: "memory.observed", Rule: "d"}}}
	reg := map[string]rule.Rule{"d": denyBoth}
	actors := map[contract.ActorID][]contract.ResourceKind{"agent": {"memory"}}
	rs, err := ResolveRules(rc, reg, actors)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	// the BOUND type fires.
	if d, _ := rs.Evaluate(rule.RuleInput{Event: contract.Event{Type: "memory.observed"}}); d.Verdict != contract.VerdictDeny {
		t.Fatalf("rule must fire on the bound type; got %q", d.Verdict)
	}
	// an unbound type the rule ALSO handles must NOT fire.
	if d, _ := rs.Evaluate(rule.RuleInput{Event: contract.Event{Type: "goal.observed"}}); d.Verdict != contract.VerdictAllow {
		t.Fatalf("a rule bound only to memory.observed must NOT fire on goal.observed; got %q", d.Verdict)
	}
}
