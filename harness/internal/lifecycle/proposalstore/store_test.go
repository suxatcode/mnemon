package proposalstore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/eventlog"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/proposal"
)

func TestStoreCreateLoadListAndTransition(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	now := time.Date(2026, 5, 27, 8, 30, 0, 0, time.UTC)
	item, err := store.Create(fixtureCreateOptions(now))
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if item.Status != proposal.StatusDraft {
		t.Fatalf("unexpected status: %s", item.Status)
	}
	if item.Scope["loop"] != "memory" || item.Scope["profile_ref"] != "profile:personal/default" {
		t.Fatalf("unexpected proposal scope: %#v", item.Scope)
	}
	assertExists(t, filepath.Join(root, ".mnemon", "harness", "proposals", "draft", item.ID, "proposal.json"))

	loaded, err := store.Load(item.ID)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if loaded.ID != item.ID || loaded.Route != proposal.RouteMemory {
		t.Fatalf("loaded mismatch: %#v", loaded)
	}
	draftItems, err := store.List(proposal.StatusDraft)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(draftItems) != 1 || draftItems[0].ID != item.ID {
		t.Fatalf("unexpected draft list: %#v", draftItems)
	}

	opened, err := store.Transition(TransitionOptions{
		ID:     item.ID,
		Status: proposal.StatusOpen,
		Now:    now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("Transition returned error: %v", err)
	}
	if opened.Status != proposal.StatusOpen {
		t.Fatalf("unexpected transitioned status: %s", opened.Status)
	}
	assertMissing(t, filepath.Join(root, ".mnemon", "harness", "proposals", "draft", item.ID))
	assertExists(t, filepath.Join(root, ".mnemon", "harness", "proposals", "open", item.ID, "proposal.json"))

	events, err := eventlog.New(root)
	if err != nil {
		t.Fatalf("eventlog New returned error: %v", err)
	}
	allEvents, err := events.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	if len(allEvents) != 2 || allEvents[0].Type != "proposal.created" || allEvents[1].Type != "proposal.opened" {
		t.Fatalf("unexpected events: %#v", allEvents)
	}
	for _, event := range allEvents {
		if event.Scope["loop"] != "memory" || event.Scope["profile_ref"] != "profile:personal/default" {
			t.Fatalf("event %s missing proposal scope: %#v", event.Type, event.Scope)
		}
	}
}

func TestStoreUpdate(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	now := time.Date(2026, 5, 27, 8, 30, 0, 0, time.UTC)
	item, err := store.Create(fixtureCreateOptions(now))
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	updated, err := store.Update(UpdateOptions{
		ID:                item.ID,
		Summary:           "Updated proposal summary.",
		ValidationSummary: "Run updated validation.",
		Evidence: []proposal.EvidenceRef{{
			Type: "audit",
			Ref:  "audit:proposal-update",
		}},
		Now: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if updated.Summary != "Updated proposal summary." || len(updated.Evidence) != 2 {
		t.Fatalf("unexpected updated proposal: %#v", updated)
	}

	events, err := eventlog.New(root)
	if err != nil {
		t.Fatalf("eventlog New returned error: %v", err)
	}
	allEvents, err := events.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	if len(allEvents) != 2 || allEvents[1].Type != "proposal.updated" {
		t.Fatalf("unexpected events: %#v", allEvents)
	}
}

func TestStoreUpdateAllowsMultipleEventsInSameSecond(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	now := time.Date(2026, 5, 27, 8, 30, 0, 0, time.UTC)
	item, err := store.Create(fixtureCreateOptions(now))
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	sameSecond := now.Add(time.Minute)
	if _, err := store.Update(UpdateOptions{
		ID:      item.ID,
		Summary: "First same-second update.",
		Now:     sameSecond,
	}); err != nil {
		t.Fatalf("first Update returned error: %v", err)
	}
	if _, err := store.Update(UpdateOptions{
		ID:      item.ID,
		Summary: "Second same-second update.",
		Now:     sameSecond,
	}); err != nil {
		t.Fatalf("second Update returned error: %v", err)
	}

	events, err := eventlog.New(root)
	if err != nil {
		t.Fatalf("eventlog New returned error: %v", err)
	}
	allEvents, err := events.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	if len(allEvents) != 3 {
		t.Fatalf("expected created event plus two updates, got %#v", allEvents)
	}
	if allEvents[1].Type != "proposal.updated" || allEvents[2].Type != "proposal.updated" {
		t.Fatalf("expected two proposal.updated events, got %#v", allEvents)
	}
	if allEvents[1].ID == allEvents[2].ID {
		t.Fatalf("expected unique same-second update event ids, got %#v", allEvents)
	}
}

func TestStoreRejectsDuplicateAndInvalidTransition(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	now := time.Date(2026, 5, 27, 8, 30, 0, 0, time.UTC)
	opts := fixtureCreateOptions(now)
	if _, err := store.Create(opts); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if _, err := store.Create(opts); err == nil {
		t.Fatal("expected duplicate proposal error")
	}
	if _, err := store.Transition(TransitionOptions{
		ID:     opts.ID,
		Status: proposal.StatusApplied,
		Now:    now.Add(time.Minute),
	}); err == nil {
		t.Fatal("expected invalid transition error")
	}
}

func TestStoreAppendAuditRef(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	now := time.Date(2026, 5, 27, 8, 30, 0, 0, time.UTC)
	opts := fixtureCreateOptions(now)
	if _, err := store.Create(opts); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	updated, err := store.AppendAuditRef(AppendRefOptions{
		ID:       opts.ID,
		AuditRef: ".mnemon/harness/audit/records/apply.json",
		Now:      now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("AppendAuditRef returned error: %v", err)
	}
	if len(updated.AuditRefs) != 1 || updated.AuditRefs[0] != ".mnemon/harness/audit/records/apply.json" {
		t.Fatalf("unexpected audit refs: %#v", updated.AuditRefs)
	}
	again, err := store.AppendAuditRef(AppendRefOptions{
		ID:       opts.ID,
		AuditRef: ".mnemon/harness/audit/records/apply.json",
		Now:      now.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("duplicate AppendAuditRef returned error: %v", err)
	}
	if len(again.AuditRefs) != 1 {
		t.Fatalf("duplicate audit ref was appended: %#v", again.AuditRefs)
	}

	events, err := eventlog.New(root)
	if err != nil {
		t.Fatalf("eventlog New returned error: %v", err)
	}
	allEvents, err := events.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	if len(allEvents) != 2 || allEvents[1].Type != "proposal.updated" {
		t.Fatalf("expected create plus audit-ref update event, got %#v", allEvents)
	}
}

func TestStoreLoadMissing(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	_, err = store.Load("missing")
	if !errors.Is(err, ErrProposalNotFound) {
		t.Fatalf("expected ErrProposalNotFound, got %v", err)
	}
}

func fixtureCreateOptions(now time.Time) CreateOptions {
	return CreateOptions{
		ID:      "prop_memory_hot_write",
		Route:   proposal.RouteMemory,
		Risk:    proposal.RiskMedium,
		Title:   "Review memory write",
		Summary: "Review a durable memory write.",
		Change: proposal.ChangeRequest{
			Summary: "Write durable project preference memory.",
			Targets: []proposal.TargetRef{{
				Type: "memory",
				URI:  "mnemon://memory/project/preferences",
			}},
		},
		Evidence: []proposal.EvidenceRef{{
			Type: "memory",
			Ref:  "memory:recall-001",
		}},
		ValidationPlan: proposal.ValidationPlan{
			Summary:  "Run memory recall.",
			Commands: []string{"mnemon recall project preference"},
		},
		Review: proposal.ReviewPolicy{
			Required:        true,
			RequiredScope:   "exact",
			RequiredReviews: 1,
		},
		Scope: map[string]any{
			"id":            "project",
			"type":          "project",
			"project_root":  ".",
			"loop":          "memory",
			"profile_ref":   "profile:personal/default",
			"binding_scope": "project",
		},
		Now: now,
	}
}

func assertExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}

func assertMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be missing, got %v", path, err)
	}
}
