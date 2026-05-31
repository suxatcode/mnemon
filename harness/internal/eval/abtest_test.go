package eval

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	runnercodex "github.com/mnemon-dev/mnemon/harness/internal/lifecycle/runner/codex"
)

// ABTrialRunnerFunc adapts a plain function to the ABTrialRunner interface for tests.
type ABTrialRunnerFunc func(context.Context, ABTrialSpec) (ABTrialResult, error)

func (fn ABTrialRunnerFunc) RunABTrial(ctx context.Context, spec ABTrialSpec) (ABTrialResult, error) {
	if fn == nil {
		return ABTrialResult{}, fmt.Errorf("ab trial runner is nil")
	}
	return fn(ctx, spec)
}

func TestABTestRunnerAggregatesPassRates(t *testing.T) {
	outcomes := map[string]Outcome{
		"control-1":   OutcomePass,
		"control-2":   OutcomeFail,
		"treatment-1": OutcomePass,
		"treatment-2": OutcomePass,
	}
	runner := ABTestRunner{
		Now: func() time.Time { return time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC) },
		TrialRunner: ABTrialRunnerFunc(func(ctx context.Context, spec ABTrialSpec) (ABTrialResult, error) {
			key := string(spec.Arm) + "-" + string(rune('0'+spec.TrialIndex))
			return ABTrialResult{
				RunID:     "run-" + key,
				Status:    "completed",
				Outcome:   outcomes[key],
				ReportRef: filepath.ToSlash(filepath.Join(".mnemon", "harness", "reports", "runner", key+".json")),
			}, nil
		}),
	}

	result, err := runner.Run(context.Background(), ABTestRequest{
		ID:           "guide-rule-ab",
		Suite:        "default",
		ScenarioIDs:  []string{"memory-no-pollution"},
		TrialsPerArm: 2,
		Metric:       ABMetricDeterministicPass,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Control.Trials != 2 || result.Control.Passes != 1 || result.Control.PassRate != 0.5 {
		t.Fatalf("unexpected control summary: %#v", result.Control)
	}
	if result.Treatment.Trials != 2 || result.Treatment.Passes != 2 || result.Treatment.PassRate != 1 {
		t.Fatalf("unexpected treatment summary: %#v", result.Treatment)
	}
	if result.MeanDiff != 0.5 {
		t.Fatalf("mean diff mismatch: %v", result.MeanDiff)
	}
	if len(result.Trials) != 4 || len(result.ArtifactRefs) != 4 {
		t.Fatalf("expected four trial records and report refs, got trials=%d refs=%d", len(result.Trials), len(result.ArtifactRefs))
	}
	if result.SignificanceNote == "" {
		t.Fatalf("expected significance boundary note")
	}
}

func TestABTestRunnerCapturesTrialErrorsAsInvalid(t *testing.T) {
	runner := ABTestRunner{
		Now: func() time.Time { return time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC) },
		TrialRunner: ABTrialRunnerFunc(func(ctx context.Context, spec ABTrialSpec) (ABTrialResult, error) {
			return ABTrialResult{}, os.ErrNotExist
		}),
	}

	result, err := runner.Run(context.Background(), ABTestRequest{
		ID:           "error-ab",
		Suite:        "default",
		ScenarioIDs:  []string{"memory-no-pollution"},
		TrialsPerArm: 1,
		Metric:       ABMetricDeterministicPass,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Control.Outcomes[OutcomeInvalid] != 1 || result.Treatment.Outcomes[OutcomeInvalid] != 1 {
		t.Fatalf("expected invalid outcomes for both arms: control=%#v treatment=%#v", result.Control, result.Treatment)
	}
	if result.Trials[0].Error == "" {
		t.Fatalf("expected captured trial error")
	}
}

func TestABTestRunnerPassesArmSetup(t *testing.T) {
	seen := map[ABArm]map[string]any{}
	runner := ABTestRunner{
		Now: func() time.Time { return time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC) },
		TrialRunner: ABTrialRunnerFunc(func(ctx context.Context, spec ABTrialSpec) (ABTrialResult, error) {
			seen[spec.Arm] = spec.Setup
			return ABTrialResult{Status: "completed", Outcome: OutcomePass}, nil
		}),
	}

	result, err := runner.Run(context.Background(), ABTestRequest{
		ID:             "guide-setup-ab",
		Suite:          "default",
		ScenarioIDs:    []string{"memory-focused-recall"},
		TrialsPerArm:   1,
		Metric:         ABMetricDeterministicPass,
		ControlSetup:   map[string]any{"baseline": "current-guide"},
		TreatmentSetup: map[string]any{"candidate_id": "dogfood-s3-4-no-console-log-guide"},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if seen[ABArmControl]["baseline"] != "current-guide" {
		t.Fatalf("control setup was not passed to trial runner: %#v", seen[ABArmControl])
	}
	if seen[ABArmTreatment]["candidate_id"] != "dogfood-s3-4-no-console-log-guide" {
		t.Fatalf("treatment setup was not passed to trial runner: %#v", seen[ABArmTreatment])
	}
	if result.Request.TreatmentSetup["candidate_id"] != "dogfood-s3-4-no-console-log-guide" {
		t.Fatalf("treatment setup was not persisted in request: %#v", result.Request.TreatmentSetup)
	}
}

func TestAnnotateABPromptAddsArmSetupWithoutExtraTurn(t *testing.T) {
	prompts := []string{"Answer the eval question."}
	got := annotateABPrompts(prompts, ABTrialSpec{
		RequestID:  "guide-setup-ab",
		Suite:      "memory-deep",
		ScenarioID: "memory-focused-recall",
		Arm:        ABArmTreatment,
		Setup: map[string]any{
			"candidate_id": "dogfood-s3-4-no-console-log-guide",
			"summary":      "guide candidate under test",
		},
	})
	if len(got) != 1 {
		t.Fatalf("setup annotation must not add turns: %#v", got)
	}
	for _, want := range []string{"AB test arm context", "arm: treatment", "candidate_id", "dogfood-s3-4-no-console-log-guide", "Scenario prompt:"} {
		if !strings.Contains(got[0], want) {
			t.Fatalf("expected %q in annotated prompt:\n%s", want, got[0])
		}
	}
}

func TestCodexABTrialRunnerCapturesAssertionBackendError(t *testing.T) {
	root := t.TempDir()
	runID := "run-assertion-error"
	writeFile(t, root, ".mnemon/harness/reports/runner/"+runID+"-codex-app-server-semantic-run.json", `{
  "schema_version": 1,
  "kind": "CodexAppServerSemanticRunReport",
  "run_id": "run-assertion-error",
  "runner_id": "codex-app-server",
  "job_id": "eval_default_memory",
  "job_spec": "eval.memory",
  "loop": "eval",
  "status": "ready",
  "message": "ok",
  "artifact_refs": [
    {"id": "artifact:jsonrpc-transcript", "kind": "transcript", "uri": ".mnemon/harness/runs/codex-app-server/run-assertion-error/artifacts/jsonrpc-transcript.jsonl", "media_type": "application/jsonl", "privacy": "project"}
  ]
}`)
	writeFile(t, root, ".mnemon/harness/runs/codex-app-server/"+runID+"/artifacts/jsonrpc-transcript.jsonl", `{"direction":"client","payload":{"id":1,"method":"thread/start","params":{}}}
{"direction":"server","payload":{"id":1,"result":{"thread":{"id":"thread-from-artifact"}}}}
`)

	runner := CodexABTrialRunner{
		Root: root,
		AssertionRuntime: AssertionRuntime{
			Root:         root,
			PythonScript: filepath.Join(root, "missing-assertion-backend.py"),
		},
	}
	outcome, err := runner.assertOutcome(context.Background(), root, RunPlan{
		ScenarioID: "memory-focused-recall",
		Scenario: &Scenario{
			ID:               "memory-focused-recall",
			AssertionHandler: "assert_memory_recall",
		},
	}, runnercodex.RunResult{
		RunID:     runID,
		Workspace: filepath.Join(root, "workspace"),
	})
	if outcome != OutcomeInvalid {
		t.Fatalf("expected invalid outcome, got %s", outcome)
	}
	if err == nil || !strings.Contains(err.Error(), "python assertion backend failed") {
		t.Fatalf("expected assertion backend diagnostic, got %v", err)
	}
}

func TestWriteABTestResult(t *testing.T) {
	root := t.TempDir()
	result, err := ABTestRunner{
		Now: func() time.Time { return time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC) },
		TrialRunner: ABTrialRunnerFunc(func(ctx context.Context, spec ABTrialSpec) (ABTrialResult, error) {
			return ABTrialResult{
				Status:  "completed",
				Outcome: OutcomePass,
			}, nil
		}),
	}.Run(context.Background(), ABTestRequest{
		ID:           "write-ab",
		Suite:        "default",
		ScenarioIDs:  []string{"memory-no-pollution"},
		TrialsPerArm: 1,
		Metric:       ABMetricDeterministicPass,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	path, err := WriteABTestResult(root, result)
	if err != nil {
		t.Fatalf("WriteABTestResult returned error: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected report file: %v", err)
	}
	if filepath.Base(path) != "write-ab.json" {
		t.Fatalf("unexpected report path: %s", path)
	}
}
