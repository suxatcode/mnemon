package coreengine

import (
	"strconv"
	"testing"
)

func seqGen() func() string   { n := 0; return func() string { n++; return "id-" + strconv.Itoa(n) } }
func fixedNow() func() string { return func() string { return "2026-06-06T00:00:00Z" } }

// TestMemoryEngineGovernsEntryWrites proves the kernel is the admission authority for a
// memory entry: a fresh entry is APPLIED by the kernel (canonical at v1), and a SECOND,
// distinct apply that targets an already-canonical entry id is DENIED by the kernel's rule
// pre-gate (not by the app) — surfacing a reason. The persistent store keeps the first
// entry canonical between the two calls.
func TestMemoryEngineGovernsEntryWrites(t *testing.T) {
	dir := t.TempDir()
	eng := NewMemoryEngine(dir, seqGen(), fixedNow())

	res, err := eng.AdmitEntry("apply-1", "entry-1", map[string]any{"summary": "first", "content": "c1"})
	if err != nil {
		t.Fatalf("admit entry-1: %v", err)
	}
	if !res.Accepted || res.Version != 1 {
		t.Fatalf("fresh entry must be accepted by the kernel at v1; got %+v", res)
	}

	// A different proposal (apply-2) that tries to create the SAME canonical entry id must be
	// denied by the kernel rule pre-gate, with a reason — the kernel governs, not the app.
	dup, err := eng.AdmitEntry("apply-2", "entry-1", map[string]any{"summary": "again", "content": "c-again"})
	if err != nil {
		t.Fatalf("admit duplicate: %v", err)
	}
	if dup.Accepted {
		t.Fatalf("a duplicate entry id must be denied by the kernel; got accepted %+v", dup)
	}
	if dup.Reason == "" {
		t.Fatalf("kernel denial must carry a reason")
	}

	// A genuinely new entry id is still accepted (the engine is not stuck).
	res3, err := eng.AdmitEntry("apply-3", "entry-2", map[string]any{"summary": "second", "content": "c2"})
	if err != nil {
		t.Fatalf("admit entry-2: %v", err)
	}
	if !res3.Accepted || res3.Version != 1 {
		t.Fatalf("second distinct entry must be accepted at v1; got %+v", res3)
	}
}

// TestMemoryEngineIdempotentReapply proves re-applying the SAME proposal id is idempotent:
// the kernel's inbox dedup means no second write, and the engine reports the entry as already
// canonical (accepted) rather than a spurious denial.
func TestMemoryEngineIdempotentReapply(t *testing.T) {
	dir := t.TempDir()
	eng := NewMemoryEngine(dir, seqGen(), fixedNow())
	if res, err := eng.AdmitEntry("apply-1", "entry-1", map[string]any{"summary": "x", "content": "cx"}); err != nil || !res.Accepted {
		t.Fatalf("first apply must be accepted; got %+v err=%v", res, err)
	}
	res, err := eng.AdmitEntry("apply-1", "entry-1", map[string]any{"summary": "x", "content": "cx"})
	if err != nil {
		t.Fatalf("idempotent re-apply: %v", err)
	}
	if !res.Accepted || res.Version != 1 {
		t.Fatalf("idempotent re-apply must report the entry already canonical at v1; got %+v", res)
	}
}
