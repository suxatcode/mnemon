package eval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/eventlog"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/proposal"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/proposalstore"
)

func TestPromoteAssetAppendsPromotionEvents(t *testing.T) {
	root := t.TempDir()
	writePromotionFixture(t, root)
	proposalID := createPromotionProposal(t, root, "eval-promotion", proposal.RouteEval, proposal.StatusApproved)

	tests := []struct {
		name   string
		kind   EvalAssetKind
		id     string
		target EvalAssetState
		from   EvalAssetState
	}{
		{"catalog scenario", EvalAssetScenario, "scenario-smoke", EvalAssetPromoted, EvalAssetCandidate},
		{"scenario file", EvalAssetScenario, "memory/project-preference-recall", EvalAssetCandidate, EvalAssetEphemeral},
		{"suite", EvalAssetSuite, "custom", EvalAssetPromoted, EvalAssetCandidate},
		{"rubric", EvalAssetRubric, "eval-asset-quality", EvalAssetCandidate, EvalAssetEphemeral},
	}

	for index, tc := range tests {
		result, err := PromoteAsset(root, PromotionOptions{
			Kind:        tc.kind,
			ID:          tc.id,
			Target:      tc.target,
			ProposalRef: proposalID,
			AuditRef:    "audit:" + tc.name,
			EventID:     "evt_eval_promotion_" + sanitizeABID(tc.name),
			Now:         time.Date(2026, 5, 27, 12, 0, index, 0, time.UTC),
		})
		if err != nil {
			t.Fatalf("PromoteAsset(%s) returned error: %v", tc.name, err)
		}
		if result.Event.Type != EvalAssetPromotedEventType {
			t.Fatalf("unexpected event type: %#v", result.Event)
		}
		if result.FromState != tc.from || result.ToState != tc.target {
			t.Fatalf("unexpected states for %s: %#v", tc.name, result)
		}
		if result.Event.ProposalRef["id"] != proposalID {
			t.Fatalf("expected proposal ref on event: %#v", result.Event.ProposalRef)
		}
		if result.Event.Payload["asset_kind"] != string(tc.kind) || result.Event.Payload["to_state"] != string(tc.target) {
			t.Fatalf("unexpected payload: %#v", result.Event.Payload)
		}
		if result.Event.Scope["binding_scope"] != "project" || result.Event.Scope["loop"] != "eval" {
			t.Fatalf("expected project eval scope on promotion event: %#v", result.Event.Scope)
		}
	}

	store, err := eventlog.New(root)
	if err != nil {
		t.Fatalf("eventlog.New returned error: %v", err)
	}
	events, err := store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	var promotions int
	for _, event := range events {
		if event.Type == EvalAssetPromotedEventType {
			promotions++
		}
	}
	if promotions != len(tests) {
		t.Fatalf("expected %d promotion events, got %d in %#v", len(tests), promotions, events)
	}
}

func TestPromoteAssetRequiresApprovedEvalProposal(t *testing.T) {
	root := t.TempDir()
	writePromotionFixture(t, root)
	openProposalID := createPromotionProposal(t, root, "eval-open", proposal.RouteEval, proposal.StatusOpen)

	_, err := PromoteAsset(root, PromotionOptions{
		Kind:        EvalAssetRubric,
		ID:          "eval-asset-quality",
		Target:      EvalAssetCandidate,
		ProposalRef: openProposalID,
		EventID:     "evt_open_proposal",
	})
	if err == nil || !strings.Contains(err.Error(), "must be approved") {
		t.Fatalf("expected approved proposal error, got %v", err)
	}
}

func writePromotionFixture(t *testing.T, root string) {
	t.Helper()
	for _, dir := range []string{
		filepath.Join(root, "harness", "loops", "eval", "suites"),
		filepath.Join(root, "harness", "loops", "eval", "scenarios", "memory"),
		filepath.Join(root, "harness", "loops", "eval", "rubrics"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "harness", "loops", "eval", "suites", "custom.json"), []byte(`{
  "name": "custom",
  "host": "codex",
  "runner": "codex-app-server",
  "lifecycle": "candidate",
  "scenario_ids": ["scenario-smoke"]
}`), 0o644); err != nil {
		t.Fatalf("write suite: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "harness", "loops", "eval", "scenarios", "codex-app.json"), []byte(`{
  "schema_version": 1,
  "name": "codex-app",
  "scenarios": [
    {
      "id": "scenario-smoke",
      "area": "eval",
      "lifecycle": "candidate",
      "loops": ["eval"],
      "prompts": ["Run the smoke scenario."]
    }
  ]
}`), 0o644); err != nil {
		t.Fatalf("write scenario catalog: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "harness", "loops", "eval", "scenarios", "memory", "project-preference-recall.md"), []byte("# Scenario\n"), 0o644); err != nil {
		t.Fatalf("write scenario file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "harness", "loops", "eval", "rubrics", "eval-asset-quality.md"), []byte("# Rubric\n"), 0o644); err != nil {
		t.Fatalf("write rubric: %v", err)
	}
}

func createPromotionProposal(t *testing.T, root, id string, route proposal.Route, final proposal.Status) string {
	t.Helper()
	store, err := proposalstore.New(root)
	if err != nil {
		t.Fatalf("proposalstore.New returned error: %v", err)
	}
	now := time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC)
	if _, err := store.Create(proposalstore.CreateOptions{
		ID:      id,
		Route:   route,
		Risk:    proposal.RiskLow,
		Title:   "Promote eval asset",
		Summary: "Fixture proposal for eval asset promotion.",
		Change: proposal.ChangeRequest{
			Summary: "Promote an eval asset.",
			Targets: []proposal.TargetRef{{
				Type: "eval_asset",
				URI:  "harness/loops/eval",
			}},
		},
		ValidationPlan: proposal.ValidationPlan{Summary: "Run promotion tests."},
		Now:            now,
	}); err != nil {
		t.Fatalf("Create proposal returned error: %v", err)
	}
	if final == proposal.StatusDraft {
		return id
	}
	transitions := []proposal.Status{proposal.StatusOpen, proposal.StatusInReview, proposal.StatusApproved}
	for index, status := range transitions {
		if _, err := store.Transition(proposalstore.TransitionOptions{
			ID:     id,
			Status: status,
			Now:    now.Add(time.Duration(index+1) * time.Second),
		}); err != nil {
			t.Fatalf("Transition proposal to %s returned error: %v", status, err)
		}
		if status == final {
			return id
		}
	}
	return id
}
