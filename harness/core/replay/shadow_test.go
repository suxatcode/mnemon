package replay

import (
	"testing"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/rule"
)

func mref(id string) contract.ResourceRef {
	return contract.ResourceRef{Kind: "memory", ID: contract.ResourceID(id)}
}

// observedLog is a log a real server would produce: a bootstrap proposal that creates m1 (giving the rules
// realistic canonical state) plus an OBSERVED event that drives the rule pre-gate.
func observedLog() ([]contract.Event, map[contract.ActorID]contract.Subscription) {
	events := []contract.Event{
		proposeWrite("boot", contract.ResourceWrite{Ref: mref("m1"), Kind: contract.OpCreate, Fields: map[string]any{"content": "v0"}}),
		{ID: "o1", Type: "memory.observed", Actor: "agent", CorrelationID: "c1"},
	}
	subs := map[contract.ActorID]contract.Subscription{"agent": {Actor: "agent", Refs: []contract.ResourceRef{mref("m1")}}}
	return events, subs
}

func proposeOnObserved(id string, actor contract.ActorID, content string) rule.Rule {
	return rule.NewNativeRule(id, actor, "memory.write.proposed", []string{"memory.observed"},
		func(in rule.RuleInput) (contract.RuleDecision, error) {
			rv := in.View.Resources[0]
			return contract.RuleDecision{Verdict: contract.VerdictPropose, Proposal: &contract.ProposedEvent{Type: "memory.write.proposed",
				Payload: map[string]any{"writes": []contract.ResourceWrite{{Ref: rv.Ref, Kind: contract.OpUpdate, BasedOn: rv.Version, Fields: map[string]any{"content": content}}}}}}, nil
		})
}

func denyOnObserved(id string, actor contract.ActorID) rule.Rule {
	return rule.NewNativeRule(id, actor, "memory.write.proposed", []string{"memory.observed"},
		func(rule.RuleInput) (contract.RuleDecision, error) { return contract.RuleDecision{Verdict: contract.VerdictDeny}, nil })
}

// S8: Shadow EXERCISES the candidate's rules over the OBSERVED events (the prior model only re-reconciled the
// already-minted .proposed events, never running the candidate's observed-handling rules — false-clean for
// every real rule change). A candidate that changes observed->decision behavior (propose -> deny) is non-clean;
// an identical policy is clean; Shadow never mutates a live store.
func TestShadowExercisesCandidateOverObservedEvents(t *testing.T) {
	events, subs := observedLog()
	live := rule.NewRuleSet(proposeOnObserved("p", "agent", "x"))
	candidate := rule.NewRuleSet(denyOnObserved("d", "agent"))

	liveStore, _ := liveDecisions(t, events)
	before := liveStore.DecisionCount()

	rep := Shadow(events, subs, live, candidate)
	if rep.Clean || rep.Diffs == 0 {
		t.Fatalf("a candidate that changes observed->decision behavior must be non-clean; got %+v", rep)
	}
	if liveStore.DecisionCount() != before {
		t.Fatalf("Shadow must not mutate a live store; decisions %d -> %d", before, liveStore.DecisionCount())
	}
	if c := Shadow(events, subs, live, live); !c.Clean || c.Diffs != 0 {
		t.Fatalf("an identical policy must be clean; got %+v", c)
	}
}

// S8: the comparison covers proposal CONTENT, not just the verdict — two policies that both propose but write
// different content diverge (the old kernel-diff false-clean class cannot recur: full payload is compared).
func TestShadowCatchesProposalContentDifference(t *testing.T) {
	events, subs := observedLog()
	rep := Shadow(events, subs, rule.NewRuleSet(proposeOnObserved("p", "agent", "alpha")), rule.NewRuleSet(proposeOnObserved("p", "agent", "beta")))
	if rep.Clean || rep.Diffs == 0 {
		t.Fatalf("a candidate proposing different CONTENT must be non-clean; got %+v", rep)
	}
}

// S8: a candidate that changes the proposal's trusted ORIGIN actor (same verdict + content, different write
// identity) diverges — the origin is part of the behavior (and the R2 misattribution fix carries it).
func TestShadowCatchesOriginActorDifference(t *testing.T) {
	events, subs := observedLog()
	rep := Shadow(events, subs, rule.NewRuleSet(proposeOnObserved("p", "agent", "x")), rule.NewRuleSet(proposeOnObserved("p", "other", "x")))
	if rep.Clean || rep.Diffs == 0 {
		t.Fatalf("a candidate stamping a different origin actor must be non-clean; got %+v", rep)
	}
}
