package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestLifecycleInitAppendAndStatusRefresh(t *testing.T) {
	root := t.TempDir()
	restoreLifecycleFlags(t)
	lifecycleRoot = root

	initCmd, _ := testCommand()
	if err := runLifecycleInit(initCmd, nil); err != nil {
		t.Fatalf("runLifecycleInit returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".mnemon", "events.jsonl")); err != nil {
		t.Fatalf("expected events.jsonl: %v", err)
	}

	lifecycleEventJSON = `{
		"schema_version": 1,
		"id": "evt_cli_memory_001",
		"ts": "2026-05-24T08:30:00Z",
		"type": "memory.hot_write_observed",
		"loop": "memory",
		"host": "codex",
		"actor": "host-agent",
		"source": "fixture",
		"correlation_id": "corr_cli",
		"caused_by": null,
		"payload": {"reason": "fixture"}
	}`
	appendCmd, appendOutput := testCommand()
	if err := runLifecycleEventAppend(appendCmd, nil); err != nil {
		t.Fatalf("runLifecycleEventAppend returned error: %v", err)
	}
	if !strings.Contains(appendOutput.String(), "evt_cli_memory_001") {
		t.Fatalf("append output did not mention event id: %s", appendOutput.String())
	}

	statusCmd, _ := testCommand()
	if err := runLifecycleStatusRefresh(statusCmd, nil); err != nil {
		t.Fatalf("runLifecycleStatusRefresh returned error: %v", err)
	}
	statusPath := filepath.Join(root, ".mnemon", "harness", "status", "loops", "memory.json")
	data, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("read status: %v", err)
	}
	var status struct {
		Status struct {
			LastIncludedEventID string `json:"last_included_event_id"`
		} `json:"status"`
	}
	if err := json.Unmarshal(data, &status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if status.Status.LastIncludedEventID != "evt_cli_memory_001" {
		t.Fatalf("status did not reference event id: %#v", status)
	}

	daemonCmd, daemonOutput := testCommand()
	if err := runLifecycleDaemonTick(daemonCmd, nil); err != nil {
		t.Fatalf("runLifecycleDaemonTick returned error: %v", err)
	}
	if !strings.Contains(daemonOutput.String(), "daemon tick processed") {
		t.Fatalf("daemon tick output mismatch: %s", daemonOutput.String())
	}
	if _, err := os.Stat(filepath.Join(root, ".mnemon", "harness", "status", "daemon.json")); err != nil {
		t.Fatalf("expected daemon status: %v", err)
	}
}

func TestLifecycleEventInputRejectsAmbiguousSource(t *testing.T) {
	restoreLifecycleFlags(t)
	lifecycleEventJSON = `{}`
	lifecycleEventFile = "event.json"
	cmd, _ := testCommand()
	_, err := lifecycleEventInput(cmd)
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected mutually exclusive error, got %v", err)
	}
}

func TestLifecycleRunnerCodexCheckCommandMissing(t *testing.T) {
	root := t.TempDir()
	restoreLifecycleFlags(t)
	lifecycleRoot = root
	lifecycleCodexCommand = "definitely-not-a-codex-command"
	lifecycleRunnerTimeout = time.Second

	cmd, output := testCommand()
	if err := runLifecycleRunnerCodexCheck(cmd, nil); err != nil {
		t.Fatalf("runLifecycleRunnerCodexCheck returned error: %v", err)
	}
	if !strings.Contains(output.String(), "command_missing") {
		t.Fatalf("expected command_missing output, got %s", output.String())
	}
	if _, err := os.Stat(filepath.Join(root, ".mnemon", "harness", "status", "runners", "codex-app-server.json")); err != nil {
		t.Fatalf("expected runner status: %v", err)
	}
}

func TestLifecycleRunnerCodexRunBlocksWithoutGate(t *testing.T) {
	root := t.TempDir()
	restoreLifecycleFlags(t)
	lifecycleRoot = root
	lifecycleCodexPrompt = "Summarize lifecycle state."
	lifecycleCodexCommand = "definitely-not-a-codex-command"

	cmd, output := testCommand()
	if err := runLifecycleRunnerCodexRun(cmd, nil); err != nil {
		t.Fatalf("runLifecycleRunnerCodexRun returned error: %v", err)
	}
	if !strings.Contains(output.String(), "RealTurnGateMissing") && !strings.Contains(output.String(), "blocked") {
		t.Fatalf("expected blocked output, got %s", output.String())
	}
	if _, err := os.Stat(filepath.Join(root, ".mnemon", "harness", "status", "runners", "codex-app-server.json")); err != nil {
		t.Fatalf("expected runner status: %v", err)
	}
}

func TestLifecycleRunnerCodexRunUsesExplicitProjectRootBeforeGate(t *testing.T) {
	root := t.TempDir()
	projectRoot := filepath.Join(root, "project")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatalf("mkdir project root: %v", err)
	}
	readmePath := filepath.Join(projectRoot, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Existing Project\n"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	restoreLifecycleFlags(t)
	lifecycleRoot = root
	lifecycleCodexPrompt = "Continue the existing goal workspace."
	lifecycleCodexCommand = "definitely-not-a-codex-command"
	lifecycleCodexProjectRoot = "project"

	cmd, _ := testCommand()
	if err := runLifecycleRunnerCodexRun(cmd, nil); err != nil {
		t.Fatalf("runLifecycleRunnerCodexRun returned error: %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(root, ".mnemon", "harness", "reports", "runner", "*-codex-app-server-semantic-run.json"))
	if err != nil {
		t.Fatalf("glob runner reports: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one runner report, got %v", matches)
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read runner report: %v", err)
	}
	var report struct {
		Workspace string `json:"workspace"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("decode runner report: %v", err)
	}
	if report.Workspace != projectRoot {
		t.Fatalf("report workspace = %q, want %q", report.Workspace, projectRoot)
	}
	readme, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read readme: %v", err)
	}
	if string(readme) != "# Existing Project\n" {
		t.Fatalf("explicit project README was overwritten: %q", readme)
	}
}

func TestLifecycleAntipatternScanWritesReport(t *testing.T) {
	root := t.TempDir()
	restoreLifecycleFlags(t)
	lifecycleRoot = root

	cmd, output := testCommand()
	if err := runLifecycleAntipatternScan(cmd, nil); err != nil {
		t.Fatalf("runLifecycleAntipatternScan returned error: %v", err)
	}
	if !strings.Contains(output.String(), "antipattern scan: pass") || !strings.Contains(output.String(), "report:") {
		t.Fatalf("unexpected antipattern output: %s", output.String())
	}
	matches, err := filepath.Glob(filepath.Join(root, ".mnemon", "harness", "reports", "antipattern", "antipattern-scan-*.json"))
	if err != nil {
		t.Fatalf("glob antipattern reports: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one antipattern report, got %v", matches)
	}
}

func TestLifecycleDaemonControlCommands(t *testing.T) {
	root := t.TempDir()
	restoreLifecycleFlags(t)
	lifecycleRoot = root
	daemonPauseReason = "lifecycle test"

	pauseCmd, pauseOutput := testCommand()
	if err := runLifecycleDaemonPause(pauseCmd, nil); err != nil {
		t.Fatalf("runLifecycleDaemonPause returned error: %v", err)
	}
	if !strings.Contains(pauseOutput.String(), "lifecycle test") {
		t.Fatalf("unexpected pause output: %s", pauseOutput.String())
	}

	statusCmd, statusOutput := testCommand()
	if err := runLifecycleDaemonStatus(statusCmd, nil); err != nil {
		t.Fatalf("runLifecycleDaemonStatus returned error: %v", err)
	}
	if !strings.Contains(statusOutput.String(), "daemon status: paused") {
		t.Fatalf("unexpected status output: %s", statusOutput.String())
	}

	resumeCmd, resumeOutput := testCommand()
	if err := runLifecycleDaemonResume(resumeCmd, nil); err != nil {
		t.Fatalf("runLifecycleDaemonResume returned error: %v", err)
	}
	if !strings.Contains(resumeOutput.String(), "daemon resumed") {
		t.Fatalf("unexpected resume output: %s", resumeOutput.String())
	}
}

func TestLifecycleDaemonForegroundStopsOnContextCancel(t *testing.T) {
	root := t.TempDir()
	restoreLifecycleFlags(t)
	lifecycleRoot = root
	lifecycleDaemonInterval = time.Hour

	cmd, output := testCommand()
	ctx, cancel := context.WithCancel(context.Background())
	cmd.SetContext(ctx)
	done := make(chan error, 1)
	go func() {
		done <- runLifecycleDaemonForeground(cmd, nil)
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runLifecycleDaemonForeground returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("foreground daemon did not stop after context cancellation")
	}
	if !strings.Contains(output.String(), "daemon foreground stopped") {
		t.Fatalf("expected stopped output, got %s", output.String())
	}
	if _, err := os.Stat(filepath.Join(root, ".mnemon", "harness", "status", "daemon.json")); err != nil {
		t.Fatalf("expected daemon status: %v", err)
	}
}

func testCommand() (*cobra.Command, *bytes.Buffer) {
	output := &bytes.Buffer{}
	cmd := &cobra.Command{}
	cmd.SetOut(output)
	cmd.SetErr(output)
	cmd.SetIn(bytes.NewReader(nil))
	return cmd, output
}

func restoreLifecycleFlags(t *testing.T) {
	t.Helper()
	oldRoot := lifecycleRoot
	oldFile := lifecycleEventFile
	oldJSON := lifecycleEventJSON
	oldInterval := lifecycleDaemonInterval
	oldRunnerTimeout := lifecycleRunnerTimeout
	oldCodexCommand := lifecycleCodexCommand
	oldCodexIsolatedHome := lifecycleCodexIsolatedHome
	oldCodexAgentTurn := lifecycleCodexAgentTurn
	oldCodexAcknowledgeCost := lifecycleCodexAcknowledgeCost
	oldCodexPrompt := lifecycleCodexPrompt
	oldCodexProjectRoot := lifecycleCodexProjectRoot
	oldCodexJobID := lifecycleCodexJobID
	oldCodexJobSpec := lifecycleCodexJobSpec
	oldCodexLoop := lifecycleCodexLoop
	oldCodexMaxTurns := lifecycleCodexMaxTurns
	oldCodexTurnTimeout := lifecycleCodexTurnTimeout
	oldAntipatternFormat := lifecycleAntipatternFormat
	oldDaemonStatusJSON := daemonStatusJSON
	oldDaemonStatusLimit := daemonStatusLimit
	oldDaemonPauseReason := daemonPauseReason
	t.Cleanup(func() {
		lifecycleRoot = oldRoot
		lifecycleEventFile = oldFile
		lifecycleEventJSON = oldJSON
		lifecycleDaemonInterval = oldInterval
		lifecycleRunnerTimeout = oldRunnerTimeout
		lifecycleCodexCommand = oldCodexCommand
		lifecycleCodexIsolatedHome = oldCodexIsolatedHome
		lifecycleCodexAgentTurn = oldCodexAgentTurn
		lifecycleCodexAcknowledgeCost = oldCodexAcknowledgeCost
		lifecycleCodexPrompt = oldCodexPrompt
		lifecycleCodexProjectRoot = oldCodexProjectRoot
		lifecycleCodexJobID = oldCodexJobID
		lifecycleCodexJobSpec = oldCodexJobSpec
		lifecycleCodexLoop = oldCodexLoop
		lifecycleCodexMaxTurns = oldCodexMaxTurns
		lifecycleCodexTurnTimeout = oldCodexTurnTimeout
		lifecycleAntipatternFormat = oldAntipatternFormat
		daemonStatusJSON = oldDaemonStatusJSON
		daemonStatusLimit = oldDaemonStatusLimit
		daemonPauseReason = oldDaemonPauseReason
	})
	lifecycleRoot = "."
	lifecycleEventFile = ""
	lifecycleEventJSON = ""
	lifecycleDaemonInterval = 5 * time.Second
	lifecycleRunnerTimeout = 30 * time.Second
	lifecycleCodexCommand = "codex"
	lifecycleCodexIsolatedHome = false
	lifecycleCodexAgentTurn = false
	lifecycleCodexAcknowledgeCost = false
	lifecycleCodexPrompt = ""
	lifecycleCodexProjectRoot = ""
	lifecycleCodexJobID = ""
	lifecycleCodexJobSpec = "manual.semantic"
	lifecycleCodexLoop = "eval"
	lifecycleCodexMaxTurns = 3
	lifecycleCodexTurnTimeout = 3 * time.Minute
	lifecycleAntipatternFormat = "text"
	daemonStatusJSON = false
	daemonStatusLimit = 10
	daemonPauseReason = "manual"
}
