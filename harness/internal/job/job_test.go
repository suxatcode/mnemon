package job

import (
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/kernel"
	"github.com/mnemon-dev/mnemon/harness/internal/store"
)

func newJobKernel(t *testing.T, owners ...contract.ActorID) *kernel.Kernel {
	t.Helper()
	s, err := store.OpenStore(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	allow := map[contract.ActorID][]contract.ResourceKind{}
	for _, o := range owners {
		allow[o] = []contract.ResourceKind{"lease", "receipt", "budget", "memory"}
	}
	return kernel.NewKernel(s, kernel.DefaultSchemaGuard(), kernel.AuthorityRules{Allow: allow})
}

// S5: the lease Version is the fencing token. A claim is a read-modify-write CAS; the resulting Version is
// the fence.
func TestClaimIsFencedByLeaseVersion(t *testing.T) {
	k := newJobKernel(t, "w1", "w2")
	l1, err := Claim(k, "job1", "w1", 100, 60)
	if err != nil {
		t.Fatalf("w1 claim: %v", err)
	}
	if l1.Fence != 1 {
		t.Fatalf("first claim fence must be lease version 1; got %d", l1.Fence)
	}
	if _, err := Claim(k, "job1", "w2", 100, 60); err == nil {
		t.Fatal("w2 must not claim a lease w1 actively holds")
	}
}

func TestActiveLeaseNotStealable(t *testing.T) {
	k := newJobKernel(t, "w1", "w2")
	if _, err := Claim(k, "job1", "w1", 100, 60); err != nil {
		t.Fatalf("w1: %v", err)
	}
	if _, err := Claim(k, "job1", "w2", 130, 60); err == nil { // 130 < 160 (still active)
		t.Fatal("an active lease must not be stealable")
	}
}

func TestExpiredLeaseReclaimable(t *testing.T) {
	k := newJobKernel(t, "w1", "w2")
	l1, _ := Claim(k, "job1", "w1", 100, 60)   // fence_until = 160
	l2, err := Claim(k, "job1", "w2", 200, 60) // 200 > 160 (expired)
	if err != nil {
		t.Fatalf("w2 reclaim after expiry: %v", err)
	}
	if l2.Fence <= l1.Fence {
		t.Fatalf("reclaim must advance the fence; got %d <= %d", l2.Fence, l1.Fence)
	}
}

func TestStaleFinishRejected(t *testing.T) {
	k := newJobKernel(t, "w1", "w2")
	l1, _ := Claim(k, "job1", "w1", 100, 60)                   // fence v1
	if _, err := Claim(k, "job1", "w2", 200, 60); err != nil { // expired -> fence v2
		t.Fatalf("w2 reclaim: %v", err)
	}
	if err := Finish(k, l1, Result{JobID: "job1", EffectID: "e1", Outcome: "ok"}, 300); err == nil {
		t.Fatal("a stale-fence finish must be rejected (the lease moved on)")
	}
}

func TestFinishWritesReceipt(t *testing.T) {
	k := newJobKernel(t, "w1")
	l1, _ := Claim(k, "job1", "w1", 100, 60)
	if err := Finish(k, l1, Result{JobID: "job1", EffectID: "e1", Outcome: "ok"}, 200); err != nil {
		t.Fatalf("finish: %v", err)
	}
	v, fields, _ := k.Store().GetResource(contract.ResourceRef{Kind: "receipt", ID: "e1"})
	if v != 1 || fields["outcome"] != "ok" {
		t.Fatalf("finish must write a receipt resource (effect_id); got v%d %v", v, fields)
	}
}

func TestFakeRunnerDeterministic(t *testing.T) {
	r := NewFakeRunner(&contract.ProposedEvent{Type: "memory.write.proposed", Payload: map[string]any{"x": 1}})
	res, err := r.Run(contract.JobSpec{Kind: "eval", IdempotencyKey: "k1"})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.ProposalCandidate == nil || res.ProposalCandidate.Type != "memory.write.proposed" {
		t.Fatalf("FakeRunner must return its fixed proposal candidate; got %+v", res.ProposalCandidate)
	}
	if res.EffectID == "" {
		t.Fatal("FakeRunner must mint an effect id")
	}
	if r.LastKey() != "k1" {
		t.Fatalf("FakeRunner must record the idempotency key; got %q", r.LastKey())
	}
}
