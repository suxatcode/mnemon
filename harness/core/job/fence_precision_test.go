package job

import (
	"testing"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
)

// MED#5 (S5): fence_until must survive the resource JSON round-trip EXACTLY. Stored as a float64, at
// UnixNano magnitude (~1.78e18, where the float64 ULP is 256ns) the intended integer fence is mis-rounded,
// so the active/expired boundary S5 rests on is corrupted — an ACTIVE lease becomes foreign-stealable.
// now+ttl = 1780000000000000300 rounds to 1780000000000000256 as a float64; a steal at now+280 then looks
// "expired" (280 > 256) and succeeds, though now+280 is genuinely inside the [now, now+300) active window.
func TestActiveLeaseNotStealableAtNanoMagnitude(t *testing.T) {
	k := newJobKernel(t, "w1", "w2")
	const now = int64(1780000000000000000) // UnixNano magnitude; exactly representable (multiple of 256)
	const ttl = int64(300)
	if _, err := Claim(k, "job1", "w1", now, ttl); err != nil { // intended fence_until = now+300
		t.Fatalf("w1 claim: %v", err)
	}
	if _, err := Claim(k, "job1", "w2", now+280, ttl); err == nil {
		t.Fatal("an active lease must not be stealable; a lossy float64 fence_until let w2 steal it (S5)")
	}
}

// The stored fence_until read back through the kernel store must equal the exact integer that was claimed
// (no precision loss), at a magnitude well beyond float64's 2^53 exact-integer range.
func TestFenceUntilRoundTripsExactly(t *testing.T) {
	k := newJobKernel(t, "w1")
	const now = int64(1780000000000000000)
	const ttl = int64(123)
	if _, err := Claim(k, "job1", "w1", now, ttl); err != nil {
		t.Fatalf("claim: %v", err)
	}
	_, fields, _ := k.Store().GetResource(contract.ResourceRef{Kind: "lease", ID: "job1"})
	if got := asInt64(fields["fence_until"]); got != now+ttl {
		t.Fatalf("fence_until must round-trip exactly; got %d want %d (lost %d)", got, now+ttl, now+ttl-got)
	}
}
