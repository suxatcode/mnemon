package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
)

func TestValidateResultAcceptsStructuredRunnerResult(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "runner.log"), []byte("ok"), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	result := validResult()
	if err := ValidateResult(result, ValidateOptions{
		Budget:               Budget{MaxTurns: 3},
		ArtifactRoot:         root,
		RequireArtifactFiles: true,
	}); err != nil {
		t.Fatalf("ValidateResult returned error: %v", err)
	}
}

func TestValidateResultFailsClosedForInvalidEventAndArtifacts(t *testing.T) {
	result := validResult()
	result.ArtifactRefs[0].Privacy = ""
	result.RecommendedEvents = []schema.Event{{
		SchemaVersion: 1,
		ID:            "evt_bad",
		TS:            "not-a-date",
		Type:          "Bad.Event",
		Actor:         "agent",
		Source:        "fixture",
		CorrelationID: "corr",
		Payload:       map[string]any{},
	}}
	err := ValidateResult(result, ValidateOptions{Budget: Budget{MaxTurns: 3}})
	if err == nil {
		t.Fatal("expected validation error")
	}
	for _, want := range []string{"privacy", "recommended_events", "ts must be RFC3339"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected error to contain %q, got %v", want, err)
		}
	}
}

func TestValidateResultRejectsTurnBudgetExceeded(t *testing.T) {
	result := validResult()
	result.TurnCount = 4
	err := ValidateResult(result, ValidateOptions{Budget: Budget{MaxTurns: 3}})
	if err == nil || !strings.Contains(err.Error(), "turn_count exceeds") {
		t.Fatalf("expected budget error, got %v", err)
	}
}

func TestBudgetAllowsAndRemaining(t *testing.T) {
	budget := Budget{MaxTurns: 3, UsedTurns: 1}
	if !budget.Allows(2) {
		t.Fatal("expected two turns to be allowed")
	}
	if budget.Allows(3) {
		t.Fatal("expected three additional turns to exceed budget")
	}
	if got := budget.Remaining(); got != 2 {
		t.Fatalf("remaining mismatch: %d", got)
	}
}

func validResult() Result {
	return Result{
		SchemaVersion: ResultSchemaVersion,
		Kind:          "HostAgentRunnerResult",
		JobID:         "job_runner_001",
		RunnerID:      "codex-app-server",
		Host:          "codex",
		TurnCount:     1,
		Status:        "completed",
		Outcome:       "pass",
		Summary:       "fixture result",
		ArtifactRefs: []ArtifactRef{{
			ID:        "artifact_runner_log",
			Kind:      "runner_log",
			URI:       "runner.log",
			MediaType: "text/plain",
			Privacy:   "project",
		}},
	}
}
