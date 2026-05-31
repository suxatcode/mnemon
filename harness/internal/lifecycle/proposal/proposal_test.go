package proposal

import (
	"strings"
	"testing"
	"time"
)

func TestValidateAcceptsCompleteProposal(t *testing.T) {
	item := fixtureProposal(t)
	if err := Validate(item); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
}

func TestValidateRejectsMissingGovernanceFields(t *testing.T) {
	item := fixtureProposal(t)
	item.Change.Targets = nil
	item.ValidationPlan = ValidationPlan{}
	item.Review.Required = false

	err := Validate(item)
	if err == nil {
		t.Fatal("expected validation error")
	}
	for _, want := range []string{
		"at least one target",
		"validation_plan",
		"review is required",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected error to contain %q, got %v", want, err)
		}
	}
}

func TestTransitionRules(t *testing.T) {
	valid := []struct {
		from Status
		to   Status
	}{
		{StatusDraft, StatusOpen},
		{StatusOpen, StatusInReview},
		{StatusInReview, StatusApproved},
		{StatusApproved, StatusApplied},
		{StatusOpen, StatusRequestChanges},
		{StatusRequestChanges, StatusDraft},
		{StatusBlocked, StatusRejected},
	}
	for _, tc := range valid {
		if err := ValidateTransition(tc.from, tc.to); err != nil {
			t.Fatalf("expected %s -> %s to be valid: %v", tc.from, tc.to, err)
		}
	}

	invalid := []struct {
		from Status
		to   Status
	}{
		{StatusDraft, StatusApplied},
		{StatusRejected, StatusOpen},
		{StatusApplied, StatusSuperseded},
	}
	for _, tc := range invalid {
		if err := ValidateTransition(tc.from, tc.to); err == nil {
			t.Fatalf("expected %s -> %s to be invalid", tc.from, tc.to)
		}
	}
}

func TestTransitionSetsTimestamps(t *testing.T) {
	item := fixtureProposal(t)
	item.Status = StatusApproved
	nextTime := time.Date(2026, 5, 27, 9, 0, 1, 900, time.UTC)

	updated, err := Transition(item, StatusApplied, nextTime)
	if err != nil {
		t.Fatalf("Transition returned error: %v", err)
	}
	if updated.Status != StatusApplied {
		t.Fatalf("status mismatch: %s", updated.Status)
	}
	if updated.UpdatedAt != "2026-05-27T09:00:01Z" || updated.ClosedAt != "2026-05-27T09:00:01Z" {
		t.Fatalf("unexpected timestamps: updated=%s closed=%s", updated.UpdatedAt, updated.ClosedAt)
	}
}

func TestTerminalStatusRequiresClosedAt(t *testing.T) {
	item := fixtureProposal(t)
	item.Status = StatusRejected
	item.ClosedAt = ""

	err := Validate(item)
	if err == nil || !strings.Contains(err.Error(), "closed_at is required") {
		t.Fatalf("expected closed_at error, got %v", err)
	}
}

func fixtureProposal(t *testing.T) Proposal {
	t.Helper()
	now := time.Date(2026, 5, 27, 8, 30, 0, 0, time.UTC)
	item := New("prop_memory_hot_write", RouteMemory, RiskMedium, "Review memory write", "Review a durable memory write.", now)
	item.Change = ChangeRequest{
		Summary: "Write durable project preference memory.",
		Targets: []TargetRef{{
			Type: "memory",
			URI:  "mnemon://memory/project/preferences",
		}},
		Operations: []Operation{{
			Type:    "write",
			Target:  "mnemon://memory/project/preferences",
			Summary: "Persist the preference.",
		}},
	}
	item.Evidence = []EvidenceRef{{
		Type:    "memory",
		Ref:     "memory:recall-001",
		Summary: "User confirmed preference.",
	}}
	item.ValidationPlan = ValidationPlan{
		Summary:  "Run memory recall and verify the new fact is retrievable.",
		Commands: []string{"mnemon recall project preference"},
	}
	return item
}
