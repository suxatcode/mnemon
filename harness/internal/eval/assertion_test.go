package eval

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestAssertionResultDecodesPythonShape(t *testing.T) {
	data := []byte(`[
  {"name": "agent ran mnemon recall", "passed": true, "expected": "mnemon recall"},
  {"name": "memory file skipped transient token", "passed": false, "path": "/tmp/MEMORY.md", "rejected": "742913"},
  {"name": "memory has one eval-first entry", "passed": true, "path": "/tmp/MEMORY.md", "observed": "single-entry"}
]`)

	var results []AssertionResult
	if err := json.Unmarshal(data, &results); err != nil {
		t.Fatalf("unmarshal assertion results: %v", err)
	}
	if err := ValidateAssertionResults(results); err != nil {
		t.Fatalf("ValidateAssertionResults returned error: %v", err)
	}
	if !results[0].Passed || results[0].Expected != "mnemon recall" {
		t.Fatalf("unexpected first result: %#v", results[0])
	}
	if results[1].Passed || results[1].Path != "/tmp/MEMORY.md" || results[1].Rejected != "742913" {
		t.Fatalf("unexpected rejected result: %#v", results[1])
	}
	if results[2].Extra["observed"] != "single-entry" {
		t.Fatalf("expected extra evidence to be preserved: %#v", results[2].Extra)
	}
	if len(FailedAssertions(results)) != 1 {
		t.Fatalf("unexpected failed assertion helpers for %#v", results)
	}
}

func TestAssertionResultRejectsInvalidJSONShape(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    string
	}{
		{
			name:    "missing name",
			payload: `{"passed": true}`,
			want:    "name is required",
		},
		{
			name:    "empty name",
			payload: `{"name": " ", "passed": true}`,
			want:    "name is required",
		},
		{
			name:    "missing passed",
			payload: `{"name": "agent ran recall"}`,
			want:    "passed is required",
		},
		{
			name:    "non boolean passed",
			payload: `{"name": "agent ran recall", "passed": "yes"}`,
			want:    "passed must be a boolean",
		},
		{
			name:    "non string path",
			payload: `{"name": "agent ran recall", "passed": true, "path": 7}`,
			want:    "path must be a string",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var result AssertionResult
			err := json.Unmarshal([]byte(tc.payload), &result)
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestAssertionFuncUsesPythonCompatibleContext(t *testing.T) {
	handler := AssertionFunc(func(ctx context.Context, input AssertionContext) ([]AssertionResult, error) {
		if input.WorkspaceDir != "/tmp/workspace" {
			t.Fatalf("unexpected workspace: %s", input.WorkspaceDir)
		}
		if input.Report["command_text"] != "mnemon recall project preference" {
			t.Fatalf("unexpected report: %#v", input.Report)
		}
		return []AssertionResult{
			{Name: "agent ran recall", Passed: true, Expected: "mnemon recall"},
			{Name: "agent used recalled fact", Passed: false, Rejected: "missing final answer"},
		}, nil
	})

	results, err := handler.Assert(context.Background(), AssertionContext{
		Report:       map[string]any{"command_text": "mnemon recall project preference"},
		WorkspaceDir: "/tmp/workspace",
		MnemonDir:    "/tmp/workspace/.mnemon",
		Env:          map[string]string{"MNEMON_ROOT": "/tmp/workspace"},
	})
	if err != nil {
		t.Fatalf("Assert returned error: %v", err)
	}
	if err := ValidateAssertionResults(results); err != nil {
		t.Fatalf("ValidateAssertionResults returned error: %v", err)
	}
	failed := FailedAssertions(results)
	if len(failed) != 1 || failed[0].Name != "agent used recalled fact" {
		t.Fatalf("unexpected failed assertions: %#v", failed)
	}
}

func TestAssertionResultMarshalPreservesTopLevelEvidence(t *testing.T) {
	result := AssertionResult{
		Name:     "agent did not use irrelevant magenta fact",
		Passed:   true,
		Rejected: "magenta",
		Extra:    map[string]any{"observed": "cyan"},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal assertion result: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode marshaled result: %v", err)
	}
	if decoded["name"] != result.Name || decoded["passed"] != true || decoded["rejected"] != "magenta" || decoded["observed"] != "cyan" {
		t.Fatalf("unexpected marshaled data: %#v", decoded)
	}
	if _, ok := decoded["extra"]; ok {
		t.Fatalf("extra should not be nested: %#v", decoded)
	}
}
