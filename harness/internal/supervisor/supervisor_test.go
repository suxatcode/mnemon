package supervisor

import (
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/coordination"
)

func mergeCandidateContext() Context {
	return Context{Topology: coordination.View{
		MergeCandidates: []coordination.MergeCandidate{{EvidenceRef: "E7", Tasks: []string{"T1", "T2"}}},
	}}
}

func TestRuleStandinProposesMerge(t *testing.T) {
	sug := RuleStandin{}.Propose(mergeCandidateContext())
	if len(sug) != 1 {
		t.Fatalf("want 1 suggestion, got %d: %#v", len(sug), sug)
	}
	if sug[0].Operation != OpMerge {
		t.Errorf("operation = %q, want %q", sug[0].Operation, OpMerge)
	}
	if sug[0].TargetURI != "coordination:merge/T1+T2" {
		t.Errorf("target = %q", sug[0].TargetURI)
	}
	if len(sug[0].EvidenceRefs) != 1 || sug[0].EvidenceRefs[0] != "E7" {
		t.Errorf("evidence = %#v", sug[0].EvidenceRefs)
	}
}

// TestRuleStandinDedupsAgainstOpenProposals proves the supervisor does not
// re-propose a change already awaiting review.
func TestRuleStandinDedupsAgainstOpenProposals(t *testing.T) {
	ctx := mergeCandidateContext()
	ctx.OpenProposals = []OpenProposal{{ID: "p1", Route: "coordination", Status: "open", TargetURI: "coordination:merge/T1+T2"}}
	got := RuleStandin{}.Propose(ctx)
	if len(got) != 0 {
		t.Errorf("should not duplicate an open proposal, got %d: %#v", len(got), got)
	}
}

func TestRuleStandinNoCandidatesNoSuggestions(t *testing.T) {
	got := RuleStandin{}.Propose(Context{})
	if len(got) != 0 {
		t.Errorf("no merge candidates should yield no suggestions, got %d", len(got))
	}
}

// TestFromConfigSwappableByKind proves the brain is selected by config, not code.
func TestFromConfigSwappableByKind(t *testing.T) {
	s, err := FromConfig(Config{Kind: KindRule})
	if err != nil || s.Name() != KindRule {
		t.Fatalf("rule kind: %v %v", s, err)
	}
	if s, err := FromConfig(Config{}); err != nil || s.Name() != KindRule {
		t.Errorf("empty kind should default to the rule stand-in: %v %v", s, err)
	}
	if _, err := FromConfig(Config{Kind: KindHostAgent}); err == nil {
		t.Error("host-agent kind runs externally; in-core selection should error (real-host follow-up)")
	}
	if _, err := FromConfig(Config{Kind: "bogus"}); err == nil {
		t.Error("unknown kind should error")
	}
}
