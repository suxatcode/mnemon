package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const codexSemanticReportSuffix = "-codex-app-server-semantic-run.json"

type RunReport struct {
	SchemaVersion int               `json:"schema_version"`
	Kind          string            `json:"kind"`
	RunID         string            `json:"run_id"`
	RunnerID      string            `json:"runner_id"`
	JobID         string            `json:"job_id"`
	JobSpec       string            `json:"job_spec"`
	Loop          string            `json:"loop"`
	Status        string            `json:"status"`
	FailureClass  string            `json:"failure_class,omitempty"`
	Message       string            `json:"message"`
	ThreadID      string            `json:"thread_id,omitempty"`
	Turns         []RunReportTurn   `json:"turns,omitempty"`
	ArtifactRefs  []ReportArtifact  `json:"artifact_refs,omitempty"`
	EventRefs     []string          `json:"event_refs,omitempty"`
	Scope         map[string]any    `json:"scope,omitempty"`
	Conditions    []ReportCondition `json:"conditions,omitempty"`
	Source        string            `json:"source,omitempty"`
}

type RunReportTurn struct {
	Index             int            `json:"index"`
	PromptArtifactURI string         `json:"prompt_artifact_uri"`
	Notification      map[string]any `json:"notification,omitempty"`
}

type ReportArtifact struct {
	ID        string `json:"id,omitempty"`
	Kind      string `json:"kind"`
	URI       string `json:"uri"`
	MediaType string `json:"media_type"`
	SHA256    string `json:"sha256,omitempty"`
	Privacy   string `json:"privacy"`
}

type ReportCondition struct {
	Type    string `json:"type"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

func LoadRunReport(root, runID string) (RunReport, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return RunReport{}, fmt.Errorf("run id is required")
	}
	path := RunReportPath(root, runID)
	data, err := os.ReadFile(path)
	if err != nil {
		return RunReport{}, fmt.Errorf("read eval report %s: %w", path, err)
	}
	var report RunReport
	if err := json.Unmarshal(data, &report); err != nil {
		return RunReport{}, fmt.Errorf("parse eval report %s: %w", path, err)
	}
	if report.RunID == "" {
		report.RunID = runID
	}
	rel, err := filepath.Rel(cleanRoot(root), path)
	if err != nil {
		rel = path
	}
	report.Source = filepath.ToSlash(rel)
	return report, nil
}

func RunReportPath(root, runID string) string {
	return filepath.Join(cleanRoot(root), ".mnemon", "harness", "reports", "runner", runID+codexSemanticReportSuffix)
}

func cleanRoot(root string) string {
	if root == "" {
		root = "."
	}
	return filepath.Clean(root)
}
