package job

import (
	"testing"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/kernel"
)

func seedBudget(t *testing.T, k *kernel.Kernel, id string, limit, spent float64) {
	t.Helper()
	d := k.Apply(contract.KernelOp{OpID: "seed_budget_" + id, Actor: "agent", Writes: []contract.ResourceWrite{
		{Ref: contract.ResourceRef{Kind: "budget", ID: contract.ResourceID(id)}, Kind: contract.OpCreate,
			Fields: map[string]any{"limit_usd": limit, "spent_usd": spent}}}},
		contract.Modes{Conflict: contract.ConflictReject, Isolation: contract.IsolationWriteCAS, Authz: contract.AuthzStrict})
	if d.Status != contract.Accepted {
		t.Fatalf("seed budget: %s", d.Reason)
	}
}

// S6: the budget reserve and the data write are ONE multi-write op carrying budget@v in the read-set. Two
// concurrent reserves that each fit locally but together exceed limit_usd -> the second op's read-set is
// stale -> rejected (no overshoot, no partial write).
func TestBudgetReserveIsAtomicWithWrite(t *testing.T) {
	k := newJobKernel(t, "agent")
	seedBudget(t, k, "global", 10, 0)
	bref := contract.ResourceRef{Kind: "budget", ID: "global"}
	v0, _, _ := k.Store().GetResource(bref) // both reservers read v0
	modes := contract.Modes{Conflict: contract.ConflictReject, Isolation: contract.IsolationProjectionReadSet, Authz: contract.AuthzStrict}
	mk := func(opid, memID string) contract.KernelOp {
		return contract.KernelOp{OpID: opid, Actor: "agent",
			Writes: []contract.ResourceWrite{
				{Ref: bref, Kind: contract.OpUpdate, BasedOn: v0, Fields: map[string]any{"limit_usd": float64(10), "spent_usd": float64(6)}},
				{Ref: contract.ResourceRef{Kind: "memory", ID: contract.ResourceID(memID)}, Kind: contract.OpCreate, Fields: map[string]any{"content": "x"}},
			},
			ReadSet: []contract.ResourceVersion{{Ref: bref, Version: v0}}}
	}
	dA := k.Apply(mk("rA", "mA"), modes)
	dB := k.Apply(mk("rB", "mB"), modes) // same based_on v0 -> stale -> rejected
	if dA.Status != contract.Accepted {
		t.Fatalf("first reserve must accept; got %+v", dA)
	}
	if dB.Status == contract.Accepted {
		t.Fatalf("second concurrent reserve (stale budget@v) must be rejected; got %+v", dB)
	}
	_, fields, _ := k.Store().GetResource(bref)
	if asFloat(fields["spent_usd"]) != 6 {
		t.Fatalf("no overshoot: spent must be 6 (one reserve); got %v", fields["spent_usd"])
	}
	if v, _, _ := k.Store().GetResource(contract.ResourceRef{Kind: "memory", ID: "mB"}); v != 0 {
		t.Fatalf("no partial write: rejected reserve's data must be absent; mB at v%d", v)
	}
	if v, _, _ := k.Store().GetResource(contract.ResourceRef{Kind: "memory", ID: "mA"}); v != 1 {
		t.Fatalf("accepted reserve must write its data; mA at v%d", v)
	}
}

// adversarial #1: a negative cost must be refused — else it decreases spent_usd and launders an overshoot
// of limit_usd (cumulative real spend > limit while stored spent stays low).
func TestReserveRefusesNegativeCost(t *testing.T) {
	k := newJobKernel(t, "agent")
	seedBudget(t, k, "global", 10, 10) // fully spent
	dw := contract.ResourceWrite{Ref: contract.ResourceRef{Kind: "memory", ID: "m1"}, Kind: contract.OpCreate, Fields: map[string]any{"content": "x"}}
	if _, err := Reserve(k, "global", "agent", -10, dw); err == nil {
		t.Fatal("a negative cost must be refused (it would launder a spend-ceiling refund)")
	}
	// spent must be unchanged (no laundering).
	_, fields, _ := k.Store().GetResource(contract.ResourceRef{Kind: "budget", ID: "global"})
	if asFloat(fields["spent_usd"]) != 10 {
		t.Fatalf("spent must stay 10 after a refused negative reserve; got %v", fields["spent_usd"])
	}
}

func TestReserveRefusesOverBudget(t *testing.T) {
	k := newJobKernel(t, "agent")
	seedBudget(t, k, "global", 10, 7)
	dw := contract.ResourceWrite{Ref: contract.ResourceRef{Kind: "memory", ID: "m1"}, Kind: contract.OpCreate, Fields: map[string]any{"content": "x"}}
	if _, err := Reserve(k, "global", "agent", 5, dw); err == nil { // 7+5=12 > 10
		t.Fatal("a reserve exceeding limit_usd must be refused")
	}
	d, err := Reserve(k, "global", "agent", 2, dw) // 7+2=9 <= 10
	if err != nil {
		t.Fatalf("in-budget reserve: %v", err)
	}
	if d.Status != contract.Accepted {
		t.Fatalf("in-budget reserve must accept; got %+v", d)
	}
	_, fields, _ := k.Store().GetResource(contract.ResourceRef{Kind: "budget", ID: "global"})
	if asFloat(fields["spent_usd"]) != 9 {
		t.Fatalf("spent must be 9 after reserve; got %v", fields["spent_usd"])
	}
	if v, _, _ := k.Store().GetResource(contract.ResourceRef{Kind: "memory", ID: "m1"}); v != 1 {
		t.Fatalf("reserve must write its data atomically; m1 at v%d", v)
	}
}
