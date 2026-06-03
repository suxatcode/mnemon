package standard

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/mnemon-dev/mnemon/core/contract"
	"github.com/mnemon-dev/mnemon/core/kernel"
	"github.com/mnemon-dev/mnemon/core/reconcile"
)

func goListDeps(t *testing.T, pkg string) []string {
	t.Helper()
	out, err := exec.Command("go", "list", "-deps", pkg).Output()
	if err != nil {
		t.Fatalf("go list -deps %s: %v", pkg, err)
	}
	return strings.Split(strings.TrimSpace(string(out)), "\n")
}
func contains(deps []string, sub string) bool {
	for _, d := range deps {
		if strings.Contains(d, sub) {
			return true
		}
	}
	return false
}

// Falsifiable smallness (Invariant #18): the second adapter imports ONLY core/contract (+ stdlib).
// If it ever reaches core/kernel or core/reconcile, the contract surface is too big — shrink it.
func TestSecondAdapterImportsOnlyContract(t *testing.T) {
	deps := goListDeps(t, "github.com/mnemon-dev/mnemon/core/standard")
	for _, bad := range []string{"core/kernel", "core/reconcile", "core/projection", "core/callback"} {
		if contains(deps, bad) {
			t.Fatalf("adapter reached %s — contract is too big, shrink it", bad)
		}
	}
	if !contains(deps, "core/contract") {
		t.Fatal("adapter must import core/contract")
	}
}

// The contract-only adapter participates: it emits a *.proposed event carrying its read-set (based_on),
// and a full RunOnce (wired here in the TEST, which may import kernel/reconcile) produces a Decision.
func TestSecondAdapterParticipatesInReconcile(t *testing.T) {
	s, err := kernel.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	k := kernel.NewKernel(s, kernel.DefaultSchemaGuard(),
		kernel.AuthorityRules{Allow: map[contract.ActorID][]contract.ResourceKind{"ext": {"memory"}}})
	ref := contract.ResourceRef{Kind: "memory", ID: "m1"}
	seed := k.Apply(contract.KernelOp{OpID: "seed", Actor: "ext", Writes: []contract.ResourceWrite{
		{Ref: ref, Kind: contract.OpCreate, Fields: map[string]any{"content": "v0"}}}},
		contract.Modes{Conflict: contract.ConflictRebase, Isolation: contract.IsolationWriteCAS, Authz: contract.AuthzStrict})
	if seed.Status != contract.Accepted {
		t.Fatalf("seed: %s", seed.Reason)
	}

	// the contract-only adapter builds a *.proposed event from what it read (read-set = based_on)
	view := ProjectionView{Resources: []contract.ResourceVersion{{Ref: ref, Version: 1}}, Digest: "d"}
	ev := Propose("ext", "task1", view, ref, 1, map[string]any{"content": "v1"})
	if ev.CorrelationID == "" {
		t.Fatal("adapter must carry a non-empty CorrelationID (escalation grouping key)")
	}
	if ev.Type != "memory.write.proposed" {
		t.Fatalf("adapter must emit a *.proposed event, got %q", ev.Type)
	}
	if _, err := s.AppendEvent(ev); err != nil {
		t.Fatalf("append: %v", err)
	}

	ds := reconcile.NewReconciler(s, k).RunOnce(
		contract.Modes{Conflict: contract.ConflictRebase, Isolation: contract.IsolationProjectionReadSet, Authz: contract.AuthzStrict})
	if len(ds) != 1 || ds[0].Status != contract.Accepted {
		t.Fatalf("adapter proposal must reconcile to an Accepted Decision, got %+v", ds)
	}
}
