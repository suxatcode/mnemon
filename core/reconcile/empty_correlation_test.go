package reconcile

import (
	"testing"

	"github.com/mnemon-dev/mnemon/core/contract"
)

// R2#1: events without a CorrelationID must NOT all share one escalation bucket. Three UNRELATED stale
// proposals with empty correlation (but distinct event IDs) must each stay on rebase — none should be
// escalated to human_review just because they share the empty-string key.
func TestEmptyCorrelationDoesNotShareEscalationBucket(t *testing.T) {
	s, k := newRecon(t)
	X := contract.ResourceRef{Kind: "memory", ID: "X"}
	seedCreate(t, k, X, map[string]any{"content": "v0"})    // X@1
	seedUpdate(t, k, X, 1, map[string]any{"content": "v1"}) // X@2 -> base 1 stale
	for _, id := range []string{"u1", "u2", "u3"} {
		appendProposal(t, s, updateProposal(id, "codex", "" /* empty correlation */, X, 1, map[string]any{"content": "r"}, nil))
	}
	ds := NewReconciler(s, k).RunOnce(casModes())
	if len(ds) != 3 {
		t.Fatalf("want 3 decisions, got %d", len(ds))
	}
	for i, d := range ds {
		if d.NextAction != "rebase" {
			t.Fatalf("event %d (%s): unrelated empty-correlation proposals must not escalate, got %q", i, d.OpID, d.NextAction)
		}
	}
}
