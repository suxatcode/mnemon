package read

import "testing"

// TestClassifyProposalDeterministic proves the review badge is code-computed and
// deterministic — high-blast/hard-to-reverse → review, narrow/reversible → safe —
// never a model verdict and never an apply decision.
func TestClassifyProposalDeterministic(t *testing.T) {
	coord := func(op string) Proposal {
		return Proposal{Route: "coordination", Change: ChangeRequest{Operations: []Operation{{Type: op}}}}
	}
	cases := []struct {
		name     string
		p        Proposal
		wantSafe bool
	}{
		{"coordination merge", coord("coordination.merge"), false},
		{"coordination reassign", coord("coordination.reassign"), false},
		{"coordination link", coord("coordination.link"), true},
		{"coordination unlink", coord("coordination.unlink"), true},
		{"group member_removed", coord("coordination.group.member_removed"), true},
		{"memory route", Proposal{Route: "memory", Risk: "low"}, false},
		{"skill route", Proposal{Route: "skill", Risk: "low"}, false},
		{"low-risk eval", Proposal{Route: "eval", Risk: "low"}, true},
		{"high-risk eval", Proposal{Route: "eval", Risk: "high"}, false},
	}
	for _, c := range cases {
		got := ClassifyProposal(c.p)
		if got.Safe != c.wantSafe {
			t.Errorf("%s: Safe=%v want %v (label %q reason %q)", c.name, got.Safe, c.wantSafe, got.Label, got.Reason)
		}
		if got.Label == "" || got.Reason == "" {
			t.Errorf("%s: badge must carry a label + reason, got %#v", c.name, got)
		}
	}
	// Determinism: identical input yields identical output.
	p := coord("coordination.merge")
	if ClassifyProposal(p) != ClassifyProposal(p) {
		t.Error("ClassifyProposal must be deterministic")
	}
}
