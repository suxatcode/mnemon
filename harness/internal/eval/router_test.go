package eval

import "testing"

func TestRouteEvidenceRoutesMultipleAreas(t *testing.T) {
	candidates := RouteEvidence([]EvidenceItem{
		{
			ID:      "memory-no-pollution",
			Source:  "eval",
			Area:    "memory",
			Outcome: OutcomeFail,
			Refs:    []EvidenceRef{{Type: "eval_report", Ref: "reports/memory.json"}},
			Assertions: []AssertionResult{
				{Name: "agent avoided recall", Passed: true},
				{Name: "memory stayed clean", Passed: false},
			},
		},
		{
			ID:      "docs-bilingual-sync",
			Source:  "docs-check",
			Area:    "docs",
			Outcome: OutcomeWeak,
			Refs:    []EvidenceRef{{Type: "command", Ref: "make harness-docs-check"}},
		},
		{
			ID:      "passing-evidence",
			Source:  "eval",
			Area:    "skill",
			Outcome: OutcomePass,
		},
	})

	if len(candidates) != 2 {
		t.Fatalf("expected two candidates, got %#v", candidates)
	}
	if candidates[0].Route != "memory" || candidates[0].ScenarioID != "memory-no-pollution" || candidates[0].EvidenceID != "memory-no-pollution" {
		t.Fatalf("unexpected memory candidate: %#v", candidates[0])
	}
	if len(candidates[0].Assertions) != 1 || candidates[0].Assertions[0].Name != "memory stayed clean" {
		t.Fatalf("expected failed assertion only: %#v", candidates[0].Assertions)
	}
	if candidates[1].Route != "docs" || candidates[1].ScenarioID != "" || candidates[1].Source != "docs-check" {
		t.Fatalf("unexpected docs candidate: %#v", candidates[1])
	}
}

func TestRouteEvalReportBuildsCandidateFromRunReport(t *testing.T) {
	report := RunReport{
		RunID:    "run-001",
		RunnerID: "codex-app-server",
		JobID:    "eval_default_memory",
		JobSpec:  "eval.memory-no-pollution",
		Source:   ".mnemon/harness/reports/runner/run-001.json",
	}
	assertions := []AssertionResult{{Name: "memory stayed clean", Passed: false}}

	candidates := RouteEvalReport(report, Scenario{ID: "memory-no-pollution", Loops: []string{"memory"}}, OutcomeFail, assertions)
	if len(candidates) != 1 {
		t.Fatalf("expected one candidate, got %#v", candidates)
	}
	candidate := candidates[0]
	if candidate.Route != "memory" || candidate.Source != "eval" || candidate.Metadata["run_id"] != "run-001" {
		t.Fatalf("unexpected candidate: %#v", candidate)
	}
	if len(candidate.Evidence) != 2 || candidate.Evidence[0].Ref != report.Source || candidate.Evidence[1].Ref != "run-001" {
		t.Fatalf("unexpected evidence refs: %#v", candidate.Evidence)
	}
}
