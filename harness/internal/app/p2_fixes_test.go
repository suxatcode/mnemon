package app

import (
	"io"
	"strings"
	"testing"
)

// TestEvalRePromotionNotFalseDenied pins the P2 adversarial fix (C): the eval kernel gate keys
// by the APPLY id, not the asset id, so a SECOND distinct proposal re-promoting the same asset
// is NOT false-denied (eval promotion is a repeatable host transition). A re-apply of the SAME
// proposal stays idempotent.
func TestEvalRePromotionNotFalseDenied(t *testing.T) {
	h := New(t.TempDir())
	target := evalProposalTarget{Kind: "suite", ID: "my-suite"}
	if err := h.governEvalPromotion("proposal-1", target); err != nil {
		t.Fatalf("first promotion must be accepted: %v", err)
	}
	if err := h.governEvalPromotion("proposal-2", target); err != nil {
		t.Fatalf("a distinct proposal re-promoting the same asset must not be false-denied: %v", err)
	}
	if err := h.governEvalPromotion("proposal-1", target); err != nil {
		t.Fatalf("idempotent re-apply of the same proposal must not error: %v", err)
	}
}

// TestProfileEntryAddIsGovernedByKernel pins the P2 adversarial fix (A): the direct CLI verb
// `profile entry add` routes through the kernel rule pre-gate, so a duplicate direct add is
// refused BY THE KERNEL (not a silent second canonical writer that bypasses the gate).
func TestProfileEntryAddIsGovernedByKernel(t *testing.T) {
	h := New(t.TempDir())
	in := ProfileEntryInput{ProfileID: "personal-default", EntryID: "pref-1", Type: "preference", Summary: "s", Content: "c", Evidence: []string{"observation=ref-1"}}
	if err := h.ProfileEntryAdd(io.Discard, in); err != nil {
		t.Fatalf("first direct add must be accepted: %v", err)
	}
	err := h.ProfileEntryAdd(io.Discard, in)
	if err == nil {
		t.Fatalf("a duplicate direct profile entry add must be refused")
	}
	if !strings.Contains(err.Error(), "kernel denied") {
		t.Fatalf("the refusal must come from the kernel gate, got: %v", err)
	}
}

// TestProfileEntryAddNonCanonicalIdGovernedConsistently pins fix (B): a non-canonical entry id
// is canonicalized ONCE, so the kernel key matches the host-stored id and the two duplicate
// gates never disagree — a second direct add with an id that canonicalizes to the same value is
// refused by the kernel.
func TestProfileEntryAddNonCanonicalIdGovernedConsistently(t *testing.T) {
	h := New(t.TempDir())
	if err := h.ProfileEntryAdd(io.Discard, ProfileEntryInput{ProfileID: "personal-default", EntryID: "My Pref!!", Type: "preference", Summary: "s", Content: "c", Evidence: []string{"observation=ref-1"}}); err != nil {
		t.Fatalf("first add with a non-canonical id must be accepted: %v", err)
	}
	// "my pref" canonicalizes to the same id as "My Pref!!" -> the kernel must refuse it.
	err := h.ProfileEntryAdd(io.Discard, ProfileEntryInput{ProfileID: "personal-default", EntryID: "my pref", Type: "preference", Summary: "s2", Content: "c2", Evidence: []string{"observation=ref-2"}})
	if err == nil || !strings.Contains(err.Error(), "kernel denied") {
		t.Fatalf("an id canonicalizing to an existing entry must be kernel-denied, got: %v", err)
	}
}
