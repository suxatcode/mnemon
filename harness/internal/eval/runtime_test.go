package eval

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestAssertionRuntimeRunsGoBackend(t *testing.T) {
	runtime := AssertionRuntime{
		GoHandlers: map[string]AssertionHandler{
			"assert_custom": AssertionFunc(func(ctx context.Context, input AssertionContext) ([]AssertionResult, error) {
				if input.Report["command_text"] != "mnemon recall" {
					t.Fatalf("unexpected report: %#v", input.Report)
				}
				return []AssertionResult{
					{Name: "go assertion passed", Passed: true, Expected: "mnemon recall"},
				}, nil
			}),
		},
	}

	results, err := runtime.Run(context.Background(), AssertionRunOptions{
		Backend:      AssertionBackendGo,
		Handler:      "assert_custom",
		Report:       map[string]any{"command_text": "mnemon recall"},
		WorkspaceDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(results) != 1 || !results[0].Passed || results[0].Name != "go assertion passed" {
		t.Fatalf("unexpected results: %#v", results)
	}
}

func TestAssertionRuntimeRunsPythonBackendWithoutCodexTurn(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := findRepoRoot(t)
	workspace := t.TempDir()
	mnemonDir := filepath.Join(workspace, ".mnemon")
	if err := os.MkdirAll(mnemonDir, 0o755); err != nil {
		t.Fatalf("mkdir mnemon dir: %v", err)
	}
	runtime := AssertionRuntime{Root: root}

	results, err := runtime.Run(context.Background(), AssertionRunOptions{
		Backend:    AssertionBackendPython,
		ScenarioID: "memory-focused-recall",
		Report: map[string]any{
			"command_text":      "mnemon recall app-server decision",
			"final_answer_text": "Use the Codex app-server decision.",
		},
		WorkspaceDir: workspace,
		MnemonDir:    mnemonDir,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(results) != 2 || len(FailedAssertions(results)) != 0 {
		t.Fatalf("unexpected python assertion results: %#v", results)
	}
}

func TestAssertionRuntimeReturnsFailedPythonAssertions(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	root := findRepoRoot(t)
	workspace := t.TempDir()
	mnemonDir := filepath.Join(workspace, ".mnemon")
	if err := os.MkdirAll(mnemonDir, 0o755); err != nil {
		t.Fatalf("mkdir mnemon dir: %v", err)
	}
	runtime := AssertionRuntime{Root: root}

	results, err := runtime.Run(context.Background(), AssertionRunOptions{
		Backend:    AssertionBackendPython,
		ScenarioID: "memory-focused-recall",
		Report: map[string]any{
			"command_text":      "mnemon recall",
			"final_answer_text": "",
		},
		WorkspaceDir: workspace,
		MnemonDir:    mnemonDir,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	failed := FailedAssertions(results)
	if len(failed) != 1 || failed[0].Name != "agent used recalled Codex app-server decision" {
		t.Fatalf("unexpected failed assertions: %#v", failed)
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "scripts", "codex_app_server_eval.py")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find repo root from %s", dir)
		}
		dir = parent
	}
}
