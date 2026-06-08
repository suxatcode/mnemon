package config

import (
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/rule"
)

func allowRule(id string, actor contract.ActorID, emits string, handles ...string) rule.Rule {
	return rule.NewNativeRule(id, actor, emits, handles,
		func(rule.RuleInput) (contract.RuleDecision, error) {
			return contract.RuleDecision{Verdict: contract.VerdictAllow}, nil
		})
}

func validRuleCfg() (RuleConfig, map[string]rule.Rule, map[contract.ActorID][]contract.ResourceKind) {
	return RuleConfig{Bindings: []RuleBinding{{EventType: "memory.observed", Rule: "writer"}}},
		map[string]rule.Rule{"writer": allowRule("writer", "agent", "memory.write.proposed", "memory.observed")},
		map[contract.ActorID][]contract.ResourceKind{"agent": {"memory"}}
}

func TestResolveRulesAcceptsValid(t *testing.T) {
	rc, reg, actors := validRuleCfg()
	rs, err := ResolveRules(rc, reg, actors)
	if err != nil {
		t.Fatalf("valid rule config rejected: %v", err)
	}
	if len(rs.Rules()) != 1 {
		t.Fatalf("expected one resolved rule, got %d", len(rs.Rules()))
	}
}

func TestResolveRulesRejectsBadInputs(t *testing.T) {
	actors := map[contract.ActorID][]contract.ResourceKind{"agent": {"memory"}}
	cases := map[string]struct {
		rc  RuleConfig
		reg map[string]rule.Rule
	}{
		"unknown rule key": {
			RuleConfig{Bindings: []RuleBinding{{EventType: "memory.observed", Rule: "ghost"}}},
			map[string]rule.Rule{"writer": allowRule("writer", "agent", "memory.write.proposed", "memory.observed")},
		},
		"nil rule (path forbidden)": {
			RuleConfig{Bindings: []RuleBinding{{EventType: "memory.observed", Rule: "./evil.so"}}},
			map[string]rule.Rule{"./evil.so": nil},
		},
		"proposed EventType (self-loop)": {
			RuleConfig{Bindings: []RuleBinding{{EventType: "memory.write.proposed", Rule: "writer"}}},
			map[string]rule.Rule{"writer": allowRule("writer", "agent", "memory.write.proposed", "memory.write.proposed")},
		},
		"empty EventType": {
			RuleConfig{Bindings: []RuleBinding{{EventType: "", Rule: "writer"}}},
			map[string]rule.Rule{"writer": allowRule("writer", "agent", "memory.write.proposed", "memory.observed")},
		},
		"undeclared actor": {
			RuleConfig{Bindings: []RuleBinding{{EventType: "memory.observed", Rule: "writer"}}},
			map[string]rule.Rule{"writer": allowRule("writer", "ghost", "memory.write.proposed", "memory.observed")},
		},
		"non-proposed emit": {
			RuleConfig{Bindings: []RuleBinding{{EventType: "memory.observed", Rule: "writer"}}},
			map[string]rule.Rule{"writer": allowRule("writer", "agent", "memory.write", "memory.observed")},
		},
		"rule does not handle EventType": {
			RuleConfig{Bindings: []RuleBinding{{EventType: "goal.observed", Rule: "writer"}}},
			map[string]rule.Rule{"writer": allowRule("writer", "agent", "memory.write.proposed", "memory.observed")},
		},
	}
	for name, c := range cases {
		if _, err := ResolveRules(c.rc, c.reg, actors); err == nil {
			t.Fatalf("%s: expected rejection (define≠select breach)", name)
		}
	}
}
