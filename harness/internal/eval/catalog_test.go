package eval

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSuiteReadsScenarioIDs(t *testing.T) {
	root := t.TempDir()
	suiteDir := filepath.Join(root, "harness", "loops", "eval", "suites")
	if err := os.MkdirAll(suiteDir, 0o755); err != nil {
		t.Fatalf("mkdir suite dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(suiteDir, "custom.json"), []byte(`{
  "name": "custom",
  "description": "fixture",
  "host": "codex",
  "runner": "codex-app-server",
  "scenario_ids": ["memory-focused-recall"],
  "rubrics": ["interface-loop-behavior"]
}`), 0o644); err != nil {
		t.Fatalf("write suite: %v", err)
	}

	suite, err := LoadSuite(root, "custom")
	if err != nil {
		t.Fatalf("LoadSuite returned error: %v", err)
	}
	if suite.Source != "harness/loops/eval/suites/custom.json" {
		t.Fatalf("unexpected suite source: %#v", suite)
	}
	if len(suite.ScenarioIDs) != 1 || suite.ScenarioIDs[0] != "memory-focused-recall" {
		t.Fatalf("unexpected scenario ids: %#v", suite)
	}
}

func TestLoadSuiteAcceptsFilenameStemAlias(t *testing.T) {
	root := t.TempDir()
	suiteDir := filepath.Join(root, "harness", "loops", "eval", "suites")
	if err := os.MkdirAll(suiteDir, 0o755); err != nil {
		t.Fatalf("mkdir suite dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(suiteDir, "codex-app-default.json"), []byte(`{
  "name": "default",
  "host": "codex",
  "runner": "codex-app-server",
  "scenario_ids": ["memory-skip-local"]
}`), 0o644); err != nil {
		t.Fatalf("write suite: %v", err)
	}

	suite, err := LoadSuite(root, "codex-app-default")
	if err != nil {
		t.Fatalf("LoadSuite returned error: %v", err)
	}
	if suite.Name != "default" {
		t.Fatalf("expected declared suite name to remain default, got %#v", suite)
	}
	if suite.Source != "harness/loops/eval/suites/codex-app-default.json" {
		t.Fatalf("unexpected suite source: %#v", suite)
	}
}

func TestBuildRunPlanSelectsScenarioAndProjectionLoops(t *testing.T) {
	root := t.TempDir()
	suiteDir := filepath.Join(root, "harness", "loops", "eval", "suites")
	if err := os.MkdirAll(suiteDir, 0o755); err != nil {
		t.Fatalf("mkdir suite dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(suiteDir, "default.json"), []byte(`{
  "name": "default",
  "host": "codex",
  "runner": "codex-app-server",
  "scenario_ids": ["skill-observe-evidence", "memory-focused-recall"]
}`), 0o644); err != nil {
		t.Fatalf("write suite: %v", err)
	}

	plan, err := BuildRunPlan(root, "default", "memory-focused-recall")
	if err != nil {
		t.Fatalf("BuildRunPlan returned error: %v", err)
	}
	if plan.ScenarioID != "memory-focused-recall" {
		t.Fatalf("unexpected scenario: %#v", plan)
	}
	if len(plan.ProjectLoops) != 2 || plan.ProjectLoops[0] != "eval" || plan.ProjectLoops[1] != "memory" {
		t.Fatalf("unexpected projection loops: %#v", plan.ProjectLoops)
	}
	if plan.Prompt == "" {
		t.Fatalf("expected generated prompt")
	}
}

func TestBuildRunPlanUsesScenarioMetadata(t *testing.T) {
	root := t.TempDir()
	suiteDir := filepath.Join(root, "harness", "loops", "eval", "suites")
	scenarioDir := filepath.Join(root, "harness", "loops", "eval", "scenarios")
	for _, dir := range []string{suiteDir, scenarioDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(suiteDir, "custom.json"), []byte(`{
  "name": "custom",
  "host": "codex",
  "runner": "codex-app-server",
  "scenario_ids": ["custom-scenario"]
}`), 0o644); err != nil {
		t.Fatalf("write suite: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scenarioDir, "codex-app.json"), []byte(`{
  "schema_version": 1,
  "name": "codex-app",
  "scenarios": [
    {
      "id": "custom-scenario",
      "area": "skill",
      "loops": ["skill"],
      "expected_skills": ["skill-observe"],
      "setup_handler": "setup_none",
      "assertion_handler": "assert_custom",
      "assertion_backend": "go",
      "prompts": ["Use the declared scenario prompt."]
    }
  ]
}`), 0o644); err != nil {
		t.Fatalf("write scenario catalog: %v", err)
	}

	plan, err := BuildRunPlan(root, "custom", "custom-scenario")
	if err != nil {
		t.Fatalf("BuildRunPlan returned error: %v", err)
	}
	if plan.Prompt != "Use the declared scenario prompt." || len(plan.Prompts) != 1 {
		t.Fatalf("unexpected prompt plan: %#v", plan)
	}
	if len(plan.ProjectLoops) != 2 || plan.ProjectLoops[0] != "eval" || plan.ProjectLoops[1] != "skill" {
		t.Fatalf("unexpected projection loops: %#v", plan.ProjectLoops)
	}
	if plan.Scenario == nil {
		t.Fatalf("expected scenario metadata")
	}
	if plan.Scenario.Area != "skill" || plan.Scenario.SetupHandler != "setup_none" || plan.Scenario.AssertionBackend != "go" || plan.Scenario.Source != "harness/loops/eval/scenarios/codex-app.json" {
		t.Fatalf("unexpected scenario metadata: %#v", plan.Scenario)
	}
}
