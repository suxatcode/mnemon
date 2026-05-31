package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/eventlog"
)

func TestRunBlocksWithoutExplicitRealTurnGate(t *testing.T) {
	root := t.TempDir()
	result, err := Run(context.Background(), root, RunOptions{
		CheckOptions: CheckOptions{
			Command: "definitely-not-a-codex-command",
			Now:     fixtureNow(),
			RunID:   "gate-blocked",
		},
		Prompt: "Summarize lifecycle state.",
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Status != StatusBlocked || result.TurnCount != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
	data, err := os.ReadFile(result.ReportPath)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	var report SemanticReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if len(report.Conditions) != 1 || report.Conditions[0].Reason != "RealTurnGateMissing" {
		t.Fatalf("report did not block on the real-turn gate: %#v", report)
	}
	assertFileExists(t, result.ReportPath)
	assertFileExists(t, result.StatusPath)
}

func TestRunBlocksBeforeBudgetExceeded(t *testing.T) {
	root := t.TempDir()
	result, err := Run(context.Background(), root, RunOptions{
		CheckOptions: CheckOptions{
			Command: os.Args[0],
			Args:    []string{"-test.run=TestFakeCodexAppServer", "--"},
			Env:     []string{"MNEMON_FAKE_CODEX_APPSERVER=ready"},
			Now:     fixtureNow(),
			RunID:   "budget-blocked",
		},
		Prompts:              []string{"one", "two"},
		MaxTurns:             1,
		AllowRealTurn:        true,
		AcknowledgeModelCost: true,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Status != StatusBlocked || result.TurnCount != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestRunBlocksIsolatedHomeWithoutExplicitAuthBeforeStartingClient(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("CODEX_API_KEY", "")
	root := t.TempDir()
	result, err := Run(context.Background(), root, RunOptions{
		CheckOptions: CheckOptions{
			Command:          "definitely-not-a-codex-command",
			IsolateCodexHome: true,
			Now:              fixtureNow(),
			RunID:            "isolated-auth-preflight",
		},
		Prompt:               "Attempt one isolated Codex turn.",
		MaxTurns:             1,
		AllowRealTurn:        true,
		AcknowledgeModelCost: true,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Status != StatusBlocked || result.FailureClass != FailureAuthQuotaUnavailable || result.TurnCount != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if !strings.Contains(result.Message, "isolated CODEX_HOME") {
		t.Fatalf("message did not explain isolated auth: %q", result.Message)
	}
	data, err := os.ReadFile(result.ReportPath)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	var report SemanticReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if report.Budget.UsedTurns != 0 || len(report.Conditions) != 1 || report.Conditions[0].Reason != "IsolatedCodexHomeAuthMissing" {
		t.Fatalf("report did not block before turn start: %#v", report)
	}
}

func TestRunProjectsLoopsIntoWorkspaceBeforeGate(t *testing.T) {
	root := t.TempDir()
	writeRunnerProjectionFixture(t, root)

	result, err := Run(context.Background(), root, RunOptions{
		CheckOptions: CheckOptions{
			Command: "definitely-not-a-codex-command",
			Now:     fixtureNow(),
			RunID:   "projected-blocked",
		},
		DeclarationRoot: root,
		ProjectLoops:    []string{"memory"},
		Prompt:          "Use projected memory loop.",
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Status != StatusBlocked {
		t.Fatalf("unexpected result: %#v", result)
	}
	assertFileExists(t, filepath.Join(result.Workspace, ".codex", "skills", "memory-get", "SKILL.md"))
	assertFileExists(t, filepath.Join(result.Workspace, ".mnemon", "harness", "memory", "status.json"))
}

func TestRunFakeSemanticDispatchWritesLineage(t *testing.T) {
	root := t.TempDir()
	result, err := Run(context.Background(), root, RunOptions{
		CheckOptions: CheckOptions{
			Command: os.Args[0],
			Args:    []string{"-test.run=TestFakeCodexAppServer", "--"},
			Env:     []string{"MNEMON_FAKE_CODEX_APPSERVER=ready"},
			Now:     fixtureNow(),
			RunID:   "semantic-ready",
		},
		JobID:                "job_semantic_ready",
		JobSpec:              "memory.dreaming",
		Loop:                 "memory",
		Prompt:               "Return a concise structured lifecycle summary.",
		TurnTimeout:          time.Second,
		MaxTurns:             3,
		AllowRealTurn:        true,
		AcknowledgeModelCost: true,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Status != StatusReady || result.TurnCount != 1 || result.ThreadID == "" {
		t.Fatalf("unexpected result: %#v", result)
	}
	assertFileExists(t, result.ReportPath)
	assertFileExists(t, filepath.Join(result.RunDir, "artifacts", "jsonrpc-transcript.jsonl"))
	assertFileExists(t, filepath.Join(result.RunDir, "artifacts", "runner-result.json"))
	assertFileExists(t, filepath.Join(root, ".mnemon", "harness", "audit", "records", "semantic-ready-codex-app-server.json"))
	assertFileExists(t, filepath.Join(root, ".mnemon", "harness", "status", "jobs", "job_semantic_ready.json"))

	store, err := eventlog.New(root)
	if err != nil {
		t.Fatalf("eventlog.New returned error: %v", err)
	}
	events, err := store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	if len(events) != 5 {
		t.Fatalf("expected job, runner, and audit events; got %d", len(events))
	}

	data, err := os.ReadFile(result.ReportPath)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	var report SemanticReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if report.RunnerResult.TurnCount != 1 || len(report.ArtifactRefs) == 0 || len(report.EventRefs) != 5 {
		t.Fatalf("report missing runner evidence: %#v", report)
	}
	if report.Scope["host"] != "codex" || report.Scope["loop"] != "memory" || report.Scope["binding_scope"] != "project" {
		t.Fatalf("report missing run scope: %#v", report.Scope)
	}
	for _, event := range events {
		if event.Scope["host"] != "codex" || event.Scope["loop"] != "memory" {
			t.Fatalf("event %s missing run scope: %#v", event.Type, event.Scope)
		}
	}
}

func TestRunCanReuseExplicitProjectRootAcrossSeparateSessions(t *testing.T) {
	root := t.TempDir()
	projectRoot := filepath.Join(root, "project")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatalf("mkdir project root: %v", err)
	}
	readmePath := filepath.Join(projectRoot, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Shared S2-2 Workspace\n"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}

	seenRunDirs := map[string]bool{}
	for session := 1; session <= 3; session++ {
		if session > 1 {
			assertFileExists(t, filepath.Join(projectRoot, fmt.Sprintf("session-%02d.marker", session-1)))
		}
		runID := fmt.Sprintf("s2-2-session-%02d", session)
		result, err := Run(context.Background(), root, RunOptions{
			CheckOptions: CheckOptions{
				Command: os.Args[0],
				Args:    []string{"-test.run=TestFakeCodexAppServer", "--"},
				Env:     []string{"MNEMON_FAKE_CODEX_APPSERVER=ready"},
				Now:     fixtureNow().Add(time.Duration(session) * time.Second),
				RunID:   runID,
			},
			JobID:                fmt.Sprintf("job_s2_2_session_%02d", session),
			JobSpec:              "goal.long_task_resume",
			Loop:                 "goal",
			Prompt:               fmt.Sprintf("Continue S2-2 session %d against the shared goal workspace.", session),
			ProjectRoot:          "project",
			TurnTimeout:          time.Second,
			MaxTurns:             1,
			AllowRealTurn:        true,
			AcknowledgeModelCost: true,
		})
		if err != nil {
			t.Fatalf("Run session %d returned error: %v", session, err)
		}
		if result.Status != StatusReady || result.TurnCount != 1 {
			t.Fatalf("unexpected session %d result: %#v", session, result)
		}
		if result.Workspace != projectRoot {
			t.Fatalf("session %d used workspace %q, want %q", session, result.Workspace, projectRoot)
		}
		if seenRunDirs[result.RunDir] {
			t.Fatalf("session %d reused run dir %q", session, result.RunDir)
		}
		seenRunDirs[result.RunDir] = true

		data, err := os.ReadFile(result.ReportPath)
		if err != nil {
			t.Fatalf("read session %d report: %v", session, err)
		}
		var report SemanticReport
		if err := json.Unmarshal(data, &report); err != nil {
			t.Fatalf("decode session %d report: %v", session, err)
		}
		if report.Workspace != projectRoot || report.RunDir != result.RunDir {
			t.Fatalf("session %d report lost workspace/run dir identity: %#v", session, report)
		}
		if err := os.WriteFile(filepath.Join(projectRoot, fmt.Sprintf("session-%02d.marker", session)), []byte(runID+"\n"), 0o644); err != nil {
			t.Fatalf("write session marker: %v", err)
		}
	}
	if len(seenRunDirs) != 3 {
		t.Fatalf("expected three separate runner artifact dirs, got %d", len(seenRunDirs))
	}
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read readme: %v", err)
	}
	if string(data) != "# Shared S2-2 Workspace\n" {
		t.Fatalf("explicit project root README was overwritten: %q", data)
	}
}

func TestRunFailsWhenTurnCompletionStatusFailed(t *testing.T) {
	root := t.TempDir()
	result, err := Run(context.Background(), root, RunOptions{
		CheckOptions: CheckOptions{
			Command: os.Args[0],
			Args:    []string{"-test.run=TestFakeCodexAppServer", "--"},
			Env:     []string{"MNEMON_FAKE_CODEX_APPSERVER=turn-failed"},
			Now:     fixtureNow(),
			RunID:   "semantic-turn-failed",
		},
		JobID:                "job_semantic_turn_failed",
		JobSpec:              "memory.write",
		Loop:                 "memory",
		Prompt:               "Attempt one Codex turn.",
		TurnTimeout:          time.Second,
		MaxTurns:             1,
		AllowRealTurn:        true,
		AcknowledgeModelCost: true,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Status != StatusBlocked || result.FailureClass != FailureAuthQuotaUnavailable || result.TurnCount != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}

	data, err := os.ReadFile(result.ReportPath)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	var report SemanticReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if report.Status != StatusBlocked || report.FailureClass != FailureAuthQuotaUnavailable || report.Budget.UsedTurns != 1 {
		t.Fatalf("report did not fail closed: %#v", report)
	}
	if len(report.Conditions) != 1 || report.Conditions[0].Reason != "AuthQuotaUnavailable" {
		t.Fatalf("unexpected conditions: %#v", report.Conditions)
	}
}

func TestRunProtocolSpamDoesNotDeadlockOnClose(t *testing.T) {
	root := t.TempDir()
	result, err := Run(context.Background(), root, RunOptions{
		CheckOptions: CheckOptions{
			Command: os.Args[0],
			Args:    []string{"-test.run=TestFakeCodexAppServer", "--"},
			Env:     []string{"MNEMON_FAKE_CODEX_APPSERVER=protocol-spam"},
			Now:     fixtureNow(),
			RunID:   "semantic-protocol-spam",
		},
		JobID:                "job_semantic_protocol_spam",
		JobSpec:              "memory.injection",
		Loop:                 "memory",
		Prompt:               "Attempt one Codex turn.",
		TurnTimeout:          time.Second,
		MaxTurns:             1,
		AllowRealTurn:        true,
		AcknowledgeModelCost: true,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Status != StatusDegraded || result.FailureClass != FailureProtocolUnavailable || result.TurnCount != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
	assertFileExists(t, result.ReportPath)
}

func writeRunnerProjectionFixture(t *testing.T, root string) {
	t.Helper()
	loopDir := filepath.Join(root, "harness", "loops", "memory")
	hostDir := filepath.Join(root, "harness", "hosts", "codex")
	bindingDir := filepath.Join(root, "harness", "bindings")
	for _, dir := range []string{
		filepath.Join(loopDir, "hook-prompts"),
		filepath.Join(loopDir, "skills", "memory-get"),
		hostDir,
		bindingDir,
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	for _, path := range []string{
		filepath.Join(loopDir, "GUIDE.md"),
		filepath.Join(loopDir, "env.sh"),
		filepath.Join(loopDir, "MEMORY.md"),
		filepath.Join(loopDir, "hook-prompts", "prime.md"),
		filepath.Join(loopDir, "hook-prompts", "remind.md"),
		filepath.Join(loopDir, "hook-prompts", "nudge.md"),
		filepath.Join(loopDir, "hook-prompts", "compact.md"),
		filepath.Join(loopDir, "skills", "memory-get", "SKILL.md"),
	} {
		if err := os.WriteFile(path, []byte("fixture\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	if err := os.WriteFile(filepath.Join(loopDir, "loop.json"), []byte(`{
  "schema_version": 2,
  "name": "memory",
  "control_model": {
    "state": [],
    "intent": "fixture",
    "reality": [],
    "reconcile": []
  },
  "entity_profiles": {},
  "surfaces": {
    "projection": [],
    "observation": []
  },
  "assets": {
    "guide": "GUIDE.md",
    "env": "env.sh",
    "runtime_files": ["MEMORY.md"],
    "hook_prompts": {
      "prime": "hook-prompts/prime.md",
      "remind": "hook-prompts/remind.md",
      "nudge": "hook-prompts/nudge.md",
      "compact": "hook-prompts/compact.md"
    },
    "skills": ["skills/memory-get/SKILL.md"],
    "subagents": []
  },
  "host_adapters": {
    "codex": "../../hosts/codex"
  }
}`), 0o644); err != nil {
		t.Fatalf("write loop manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(hostDir, "host.json"), []byte(`{
  "schema_version": 2,
  "name": "codex",
  "surfaces": {
    "projection": [".codex/skills", ".codex/mnemon-memory"],
    "observation": []
  },
  "lifecycle_mapping": {}
}`), 0o644); err != nil {
		t.Fatalf("write host manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bindingDir, "codex.memory.json"), []byte(`{
  "schema_version": 1,
  "name": "codex.memory",
  "host": "codex",
  "loop": "memory",
  "projection_path": ".codex",
  "runtime_surface": ".codex/mnemon-memory",
  "lifecycle_mapping": {},
  "reconcile": []
}`), 0o644); err != nil {
		t.Fatalf("write binding manifest: %v", err)
	}
}
