package capability

import (
	"strings"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/rule"
)

// The skipped-kind rule is a pure deny descriptor (v1.1 #4): it handles only the skipped
// observation type, denies with a reason NAMING the kind for the sync principal, and passes a
// foreign principal's event through (co-existence gate).
func TestSyncImportSkippedRuleDeniesNamingKind(t *testing.T) {
	r := SyncImportSkippedRule(contract.SyncImportActor)
	if r.Handles(MemoryWriteCandidateObserved) || !r.Handles(SyncImportSkippedObserved) {
		t.Fatal("rule must handle exactly the skipped observation type")
	}
	dec, err := r.Evaluate(rule.RuleInput{Event: contract.Event{
		Type: SyncImportSkippedObserved, Actor: contract.SyncImportActor,
		Payload: map[string]any{"kind": "goal", "origin_replica_id": "r1", "local_decision_id": "d1", "remote_id": "hub"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if dec.Verdict != contract.VerdictDeny || len(dec.Reasons) != 1 || !strings.Contains(dec.Reasons[0], `"goal"`) {
		t.Fatalf("skip must deny naming the kind, got %+v", dec)
	}
	foreign, err := r.Evaluate(rule.RuleInput{Event: contract.Event{Type: SyncImportSkippedObserved, Actor: "someone@else"}})
	if err != nil || foreign.Verdict != contract.VerdictAllow {
		t.Fatalf("a foreign principal's event must pass through, got %+v err=%v", foreign, err)
	}
}
