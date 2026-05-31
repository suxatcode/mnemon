package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReplayRegressionWritesReport(t *testing.T) {
	root := t.TempDir()
	writeReplayFixture(t, root)
	result, err := ReplayRegression(root, ReplayOptions{
		Tiers: []int{2, 1},
		Now:   time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("ReplayRegression returned error: %v", err)
	}
	if result.Status != "pass" || len(result.Checks) != 4 {
		t.Fatalf("unexpected replay result: %#v", result)
	}
	if result.ReportPath == "" {
		t.Fatalf("expected report path")
	}
	reportPath := filepath.Join(root, ".mnemon", "harness", "reports", "regression", "replay-20260528T120000Z.json")
	if _, err := os.Stat(reportPath); err != nil {
		t.Fatalf("expected replay report: %v", err)
	}
	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read replay report: %v", err)
	}
	var persisted ReplayResult
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("decode replay report: %v", err)
	}
	if persisted.ReportPath == "" || persisted.ReportPath != result.ReportPath {
		t.Fatalf("persisted report path mismatch: persisted=%q result=%q", persisted.ReportPath, result.ReportPath)
	}
}

func TestReplayRegressionFailsUnsupportedTier(t *testing.T) {
	root := t.TempDir()
	writeReplayFixture(t, root)
	result, err := ReplayRegression(root, ReplayOptions{Tiers: []int{9}})
	if err != nil {
		t.Fatalf("ReplayRegression returned error: %v", err)
	}
	if result.Status != "fail" {
		t.Fatalf("expected fail result for unsupported tier: %#v", result)
	}
}

func writeReplayFixture(t *testing.T, root string) {
	t.Helper()
	suiteDir := filepath.Join(root, "harness", "loops", "eval", "suites")
	scenarioDir := filepath.Join(root, "harness", "loops", "eval", "scenarios")
	for _, dir := range []string{suiteDir, scenarioDir, filepath.Join(scenarioDir, "ops")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(suiteDir, "smoke.json"), []byte(`{
  "name": "smoke",
  "scenarios": ["ops/host-projection-smoke"]
}`), 0o644); err != nil {
		t.Fatalf("write smoke suite: %v", err)
	}
	if err := os.WriteFile(filepath.Join(suiteDir, "regression.json"), []byte(`{
  "name": "regression",
  "scenario_ids": ["memory-focused-recall"]
}`), 0o644); err != nil {
		t.Fatalf("write regression suite: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scenarioDir, "ops", "host-projection-smoke.md"), []byte("# Host Projection Smoke\n"), 0o644); err != nil {
		t.Fatalf("write markdown scenario: %v", err)
	}
	if err := os.WriteFile(filepath.Join(scenarioDir, "codex-app.json"), []byte(`{
  "scenarios": [
    {
      "id": "memory-focused-recall",
      "loops": ["memory"],
      "prompts": ["Recall the seeded project preference."]
    }
  ]
}`), 0o644); err != nil {
		t.Fatalf("write scenario catalog: %v", err)
	}
}
