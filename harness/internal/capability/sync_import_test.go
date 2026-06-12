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

// The first-party importable set is descriptor-derived (PD6, replacing the former hardcoded
// contract.SyncableResourceKinds): the embedded catalog opts exactly memory + skill into Remote
// Workspace import, each under its declared closed-set merge strategy. This is the pin the deleted
// contract.clamp_test invariant moved to — its home is now the catalog that declares it.
func TestEmbeddedImportableKindsAreMemoryAndSkill(t *testing.T) {
	kinds := ImportableKinds(EmbeddedCatalog())
	if len(kinds) != 2 || kinds[0] != "memory" || kinds[1] != "skill" {
		t.Fatalf("embedded importable kinds must be exactly [memory skill] (sorted): %v", kinds)
	}
	cat := EmbeddedCatalog()
	if cat["memory"].Sync.Merge != "entry-dedup" || cat["skill"].Sync.Merge != "declaration-dedup" {
		t.Fatalf("merge strategies must match the descriptors: memory=%q skill=%q", cat["memory"].Sync.Merge, cat["skill"].Sync.Merge)
	}
	if got := cat["memory"].RemoteCommitObserved(); got != "memory.remote_commit.observed" {
		t.Fatalf("remote-commit observation must be the system-derived form, got %q", got)
	}
	if _, ok := RemoteImportRule(cat["memory"], contract.SyncImportActor); !ok {
		t.Fatal("an importable capability must yield a remote-import rule")
	}
	if r, ok := RemoteImportRule(cat["memory"], contract.SyncImportActor); !ok || !r.Handles("memory.remote_commit.observed") {
		t.Fatalf("the memory import rule must handle its derived observation type, ok=%v", ok)
	}
}
