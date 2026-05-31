package profile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/eventlog"
)

func TestStoreAddEntryWritesEvidenceBackedProfileAndEvent(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)

	prof, entry, err := store.AddEntry(AddEntryOptions{
		ProfileID: "personal-default",
		EntryID:   "focused-commits",
		Type:      "work_style",
		Summary:   "Prefer focused harness-only commits",
		Content:   "Keep harness changes staged and avoid stable mnemon release paths.",
		Evidence: []EvidenceRef{{
			Type:    "manual",
			Ref:     "plan:E2",
			Summary: "User boundary instruction",
		}},
		ProjectionTargets: []ProjectionTarget{{Host: "codex", Loop: "memory"}},
		Now:               now,
	})
	if err != nil {
		t.Fatalf("AddEntry returned error: %v", err)
	}
	if prof.ID != "personal-default" || entry.ID != "focused-commits" {
		t.Fatalf("unexpected profile/entry ids: %s %s", prof.ID, entry.ID)
	}
	path := filepath.Join(root, ".mnemon", "harness", "profiles", "personal-default", "profile.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read profile: %v", err)
	}
	for _, want := range []string{
		`"schema_version": "mnemon.profile.v1"`,
		`"scope_type": "personal"`,
		`"evidence"`,
		`"projection_targets"`,
		`"host": "codex"`,
		`"loop": "memory"`,
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("expected %s in profile:\n%s", want, string(data))
		}
	}

	events, err := eventlog.New(root)
	if err != nil {
		t.Fatalf("eventlog.New returned error: %v", err)
	}
	allEvents, err := events.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	if len(allEvents) != 1 {
		t.Fatalf("expected one profile event, got %d", len(allEvents))
	}
	event := allEvents[0]
	if event.Type != EventEntryRecord {
		t.Fatalf("unexpected event type %s", event.Type)
	}
	if event.Scope["profile_ref"] != ProfileRef("personal-default") || event.Scope["binding_scope"] != "project" {
		t.Fatalf("unexpected event scope: %#v", event.Scope)
	}
	if event.Payload["entry_id"] != "focused-commits" {
		t.Fatalf("unexpected event payload: %#v", event.Payload)
	}
}

func TestStoreAddEntryRequiresEvidence(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	_, _, err = store.AddEntry(AddEntryOptions{
		Type:    "preference",
		Summary: "Needs evidence",
		Content: "This should not be recorded without evidence.",
		Now:     time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC),
	})
	if err == nil || !strings.Contains(err.Error(), "entry evidence is required") {
		t.Fatalf("expected evidence error, got %v", err)
	}
}

func TestStoreRejectsDuplicateEntryID(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	opts := AddEntryOptions{
		EntryID: "duplicate",
		Type:    "preference",
		Summary: "No duplicates",
		Content: "Duplicate entry ids should be explicit failures.",
		Evidence: []EvidenceRef{{
			Type: "manual",
			Ref:  "note:1",
		}},
		Now: time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC),
	}
	if _, _, err := store.AddEntry(opts); err != nil {
		t.Fatalf("first AddEntry returned error: %v", err)
	}
	opts.Now = opts.Now.Add(time.Second)
	if _, _, err := store.AddEntry(opts); !errors.Is(err, ErrDuplicateEntryID) {
		t.Fatalf("expected duplicate entry error, got %v", err)
	}
}

func TestFilterEntriesUsesExplicitProjectionTargets(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	prof := Profile{
		SchemaVersion: SchemaVersion,
		Kind:          Kind,
		ID:            "personal-default",
		ScopeType:     ScopePersonal,
		CreatedAt:     "2026-05-29T12:00:00Z",
		UpdatedAt:     "2026-05-29T12:00:00Z",
		Entries: []Entry{
			profileEntry("codex-memory", []ProjectionTarget{{Host: "codex", Loop: "memory"}}),
			profileEntry("claude-skill", []ProjectionTarget{{Host: "claude", Loop: "skill"}}),
			profileEntry("stored-only", nil),
		},
	}

	filtered := store.FilterEntries(prof, "codex", "memory")
	if len(filtered.Entries) != 1 || filtered.Entries[0].ID != "codex-memory" {
		t.Fatalf("unexpected filtered entries: %#v", filtered.Entries)
	}
	unfiltered := store.FilterEntries(prof, "", "")
	if len(unfiltered.Entries) != 3 {
		t.Fatalf("expected all entries without projection filter, got %d", len(unfiltered.Entries))
	}
}

func profileEntry(id string, targets []ProjectionTarget) Entry {
	return Entry{
		ID:                id,
		Type:              "preference",
		Summary:           id,
		Content:           "content",
		Evidence:          []EvidenceRef{{Type: "manual", Ref: "note"}},
		ProjectionTargets: targets,
		CreatedAt:         "2026-05-29T12:00:00Z",
		UpdatedAt:         "2026-05-29T12:00:00Z",
	}
}
