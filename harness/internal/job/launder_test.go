package job

import (
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/kernel"
	"github.com/mnemon-dev/mnemon/harness/internal/store"
)

// Reserve takes a caller-supplied dataWrite. Aliasing it back to the budget ref (a second OpUpdate that
// resets spent_usd) would, without the kernel's distinct-write-ref guard, commit reserve+reset as ONE
// accepted op and launder the spend ceiling (S6): real spend accumulates while stored spent_usd stays 0.
// The kernel now rejects an op whose writes alias the same ref, so the launder op is NOT accepted and the
// budget is left untouched.
func TestReserveCannotLaunderByAliasingBudgetRef(t *testing.T) {
	s, err := store.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	rules := kernel.AuthorityRules{Allow: map[contract.ActorID][]contract.ResourceKind{"agent": {"budget"}}}
	k := kernel.NewKernel(s, kernel.DefaultSchemaGuard(), rules)
	budgetRef := contract.ResourceRef{Kind: "budget", ID: "global"}
	if d := k.Apply(contract.KernelOp{OpID: "seed", Actor: "agent", Writes: []contract.ResourceWrite{
		{Ref: budgetRef, Kind: contract.OpCreate, Fields: map[string]any{"limit_usd": 10.0, "spent_usd": 0.0}}}}, reserveModes()); d.Status != contract.Accepted {
		t.Fatalf("seed budget: %s", d.Reason)
	}
	version, _, _ := k.Store().GetResource(budgetRef) // == 1
	// dataWrite aliases the budget ref, resetting spent_usd to 0, based_on the version AFTER the reserve's own
	// OpUpdate (version+1) — the laundering move the probe demonstrated.
	launder := contract.ResourceWrite{Ref: budgetRef, Kind: contract.OpUpdate, BasedOn: version + 1, Fields: map[string]any{"limit_usd": 10.0, "spent_usd": 0.0}}
	d, err := Reserve(k, "global", "agent", 9, launder) // 0 + 9 <= 10 passes the local check
	if err != nil {
		t.Fatalf("reserve returned an error before the kernel could rule: %v", err)
	}
	if d.Status == contract.Accepted {
		t.Fatal("aliasing the data write back to the budget ref must NOT be accepted (S6 launder)")
	}
	v, fields, _ := k.Store().GetResource(budgetRef)
	if v != version {
		t.Fatalf("rejected launder op must not bump the budget version; got @%d want @%d", v, version)
	}
	if asFloat(fields["spent_usd"]) != 0 {
		t.Fatalf("budget must be untouched; spent_usd=%v", fields["spent_usd"])
	}
}

// A reserve with a genuinely DISTINCT data write still commits atomically (no false positive).
func TestReserveWithDistinctDataWriteStillCommits(t *testing.T) {
	s, err := store.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	rules := kernel.AuthorityRules{Allow: map[contract.ActorID][]contract.ResourceKind{"agent": {"budget", "memory"}}}
	k := kernel.NewKernel(s, kernel.DefaultSchemaGuard(), rules)
	budgetRef := contract.ResourceRef{Kind: "budget", ID: "global"}
	if d := k.Apply(contract.KernelOp{OpID: "seed", Actor: "agent", Writes: []contract.ResourceWrite{
		{Ref: budgetRef, Kind: contract.OpCreate, Fields: map[string]any{"limit_usd": 10.0, "spent_usd": 0.0}}}}, reserveModes()); d.Status != contract.Accepted {
		t.Fatalf("seed budget: %s", d.Reason)
	}
	dw := contract.ResourceWrite{Ref: contract.ResourceRef{Kind: "memory", ID: "m1"}, Kind: contract.OpCreate, Fields: map[string]any{"content": "x"}}
	d, err := Reserve(k, "global", "agent", 4, dw)
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if d.Status != contract.Accepted {
		t.Fatalf("a distinct-ref reserve must commit; got %s %s", d.Status, d.Reason)
	}
	_, fields, _ := k.Store().GetResource(budgetRef)
	if asFloat(fields["spent_usd"]) != 4 {
		t.Fatalf("spent_usd must advance to 4; got %v", fields["spent_usd"])
	}
}
