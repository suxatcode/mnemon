package eval

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRunReportReadsMirroredRunnerReport(t *testing.T) {
	root := t.TempDir()
	reportDir := filepath.Join(root, ".mnemon", "harness", "reports", "runner")
	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		t.Fatalf("mkdir report dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(reportDir, "run-001-codex-app-server-semantic-run.json"), []byte(`{
  "schema_version": 1,
  "kind": "CodexAppServerSemanticRunReport",
  "run_id": "run-001",
  "runner_id": "codex-app-server",
  "job_id": "eval_default_eval_smoke",
  "job_spec": "eval.eval-smoke",
  "loop": "eval",
  "status": "blocked",
  "message": "real Codex turn requires explicit gates",
  "turns": [],
  "artifact_refs": [{"kind": "report", "uri": "reports/runner/run-001.json", "media_type": "application/json", "privacy": "local"}],
  "event_refs": ["evt_run_001"]
}`), 0o644); err != nil {
		t.Fatalf("write report: %v", err)
	}

	report, err := LoadRunReport(root, "run-001")
	if err != nil {
		t.Fatalf("LoadRunReport returned error: %v", err)
	}
	if report.RunID != "run-001" || report.Status != "blocked" || report.JobSpec != "eval.eval-smoke" {
		t.Fatalf("unexpected report: %#v", report)
	}
	if report.Source != ".mnemon/harness/reports/runner/run-001-codex-app-server-semantic-run.json" {
		t.Fatalf("unexpected source: %s", report.Source)
	}
	if len(report.ArtifactRefs) != 1 || len(report.EventRefs) != 1 {
		t.Fatalf("expected artifact and event refs: %#v", report)
	}
}
