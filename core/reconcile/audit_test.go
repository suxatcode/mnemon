package reconcile

import (
	"testing"

	"github.com/mnemon-dev/mnemon/core/contract"
)

// #4: a decision must carry the triggering event's IngestSeq (event<->decision audit link).
func TestDecisionCarriesEventIngestSeq(t *testing.T) {
	s, k := newRecon(t)
	X := contract.ResourceRef{Kind: "memory", ID: "X"}
	seedCreate(t, k, X, map[string]any{"content": "v0"}) // X@1 (direct Apply, ingest_seq 0)
	appendProposal(t, s, updateProposal("e1", "a1", "c1", X, 1, map[string]any{"content": "v1"}, nil))
	ds := NewReconciler(s, k).RunOnce(casModes())
	if len(ds) != 1 || ds[0].Status != contract.Accepted {
		t.Fatalf("want 1 Accepted, got %+v", ds)
	}
	if ds[0].IngestSeq != 1 {
		t.Fatalf("decision must carry triggering event IngestSeq=1, got %d", ds[0].IngestSeq)
	}
}

// #4: same-actor deferred feedback is ordered by IngestSeq (stable pull-feedback order).
func TestPullFeedbackOrderedByIngestSeq(t *testing.T) {
	s, k := newRecon(t)
	X := contract.ResourceRef{Kind: "memory", ID: "X"}
	seedCreate(t, k, X, map[string]any{"content": "v0"})    // X@1
	seedUpdate(t, k, X, 1, map[string]any{"content": "v1"}) // X@2 -> base 1 is stale
	// two stale proposals from codex -> two Deferred decisions (seq 1, seq 2)
	appendProposal(t, s, updateProposal("e1", "codex", "c", X, 1, map[string]any{"content": "a"}, nil))
	appendProposal(t, s, updateProposal("e2", "codex", "c", X, 1, map[string]any{"content": "b"}, nil))
	_ = NewReconciler(s, k).RunOnce(casModes())
	fb, err := s.DecisionsForActor("codex")
	if err != nil {
		t.Fatalf("feedback: %v", err)
	}
	if len(fb) != 2 {
		t.Fatalf("want 2 deferred feedback decisions, got %d", len(fb))
	}
	if fb[0].IngestSeq != 1 || fb[1].IngestSeq != 2 {
		t.Fatalf("feedback must be ordered by IngestSeq [1,2], got [%d,%d]", fb[0].IngestSeq, fb[1].IngestSeq)
	}
}
