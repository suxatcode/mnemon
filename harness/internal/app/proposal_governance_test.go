package app

import "testing"

// TestGovernMemoryEntryPersistsCanonical proves the memory route is lowered to the kernel as
// the single PERSISTENT writer (P2.2/D1): a first governed apply creates the canonical
// resource, and a SECOND distinct apply targeting the same canonical id is refused by the
// kernel rule pre-gate — which is only possible if the first write persisted in the core
// store across apply calls. The duplicate guard now lives at the governed gate, not the file.
func TestGovernMemoryEntryPersistsCanonical(t *testing.T) {
	h := New(t.TempDir())
	spec := memoryProfileEntrySpec{
		ProfileID: "personal-default",
		EntryID:   "entry-1",
		EntryType: "preference",
		Summary:   "summary",
		Content:   "content",
	}
	if err := h.governMemoryEntry("apply-1", spec); err != nil {
		t.Fatalf("first governed apply must be accepted by the kernel: %v", err)
	}
	if err := h.governMemoryEntry("apply-2", spec); err == nil {
		t.Fatalf("a distinct apply of an already-canonical entry id must be denied by the kernel")
	}

	// A genuinely different entry id is still accepted (the gate governs, it does not jam).
	other := spec
	other.EntryID = "entry-2"
	if err := h.governMemoryEntry("apply-3", other); err != nil {
		t.Fatalf("a distinct entry id must still be accepted: %v", err)
	}
}
