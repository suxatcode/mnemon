package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDaemonTriggerDryRunAndForce(t *testing.T) {
	root := t.TempDir()
	restoreDaemonFlags(t)
	daemonRoot = root
	writeCommandDaemonJob(t, root, "_example", "daemon.example_requested", "echo hi")

	daemonTriggerDryRun = true
	dryRunCmd, dryRunOutput := testCommand()
	if err := runDaemonTrigger(dryRunCmd, []string{"_example"}); err != nil {
		t.Fatalf("runDaemonTrigger dry-run returned error: %v", err)
	}
	if !strings.Contains(dryRunOutput.String(), "would trigger") {
		t.Fatalf("unexpected dry-run output: %s", dryRunOutput.String())
	}

	daemonTriggerDryRun = false
	daemonTriggerForce = true
	forceCmd, forceOutput := testCommand()
	if err := runDaemonTrigger(forceCmd, []string{"_example"}); err != nil {
		t.Fatalf("runDaemonTrigger force returned error: %v", err)
	}
	if !strings.Contains(forceOutput.String(), "triggered") {
		t.Fatalf("unexpected force output: %s", forceOutput.String())
	}
	if matches, _ := filepath.Glob(filepath.Join(root, ".mnemon", "harness", "jobs", "queued", "job_example_*.json")); len(matches) != 1 {
		t.Fatalf("expected one queued forced job, got %v", matches)
	}
}

func TestDaemonRunDryRunListsLoadedJobs(t *testing.T) {
	root := t.TempDir()
	restoreDaemonFlags(t)
	daemonRoot = root
	daemonRunOnce = true
	daemonRunDryRun = true
	writeCommandDaemonJob(t, root, "_example", "daemon.example_requested", "echo hi")

	cmd, output := testCommand()
	if err := runDaemonRun(cmd, nil); err != nil {
		t.Fatalf("runDaemonRun returned error: %v", err)
	}
	if !strings.Contains(output.String(), "loaded 1 daemon jobs") {
		t.Fatalf("unexpected dry-run output: %s", output.String())
	}
}

func TestDaemonPauseStatusResumeAndTrigger(t *testing.T) {
	root := t.TempDir()
	restoreDaemonFlags(t)
	daemonRoot = root
	writeCommandDaemonJob(t, root, "_example", "daemon.example_requested", "echo hi")

	daemonPauseReason = "operator test"
	pauseCmd, pauseOutput := testCommand()
	if err := runDaemonPause(pauseCmd, nil); err != nil {
		t.Fatalf("runDaemonPause returned error: %v", err)
	}
	if !strings.Contains(pauseOutput.String(), "operator test") {
		t.Fatalf("unexpected pause output: %s", pauseOutput.String())
	}

	daemonTriggerDryRun = true
	dryRunCmd, dryRunOutput := testCommand()
	if err := runDaemonTrigger(dryRunCmd, []string{"_example"}); err != nil {
		t.Fatalf("runDaemonTrigger dry-run returned error: %v", err)
	}
	if !strings.Contains(dryRunOutput.String(), "would trigger") || !strings.Contains(dryRunOutput.String(), "but paused") {
		t.Fatalf("unexpected paused dry-run output: %s", dryRunOutput.String())
	}

	daemonTriggerDryRun = false
	daemonTriggerForce = true
	forceCmd, _ := testCommand()
	if err := runDaemonTrigger(forceCmd, []string{"_example"}); err == nil || !strings.Contains(err.Error(), "daemon paused") {
		t.Fatalf("expected paused force error, got %v", err)
	}

	daemonStatusJSON = false
	statusCmd, statusOutput := testCommand()
	if err := runDaemonStatus(statusCmd, nil); err != nil {
		t.Fatalf("runDaemonStatus returned error: %v", err)
	}
	for _, want := range []string{"daemon status: paused", "queue:", "budget:", "enabled jobs:"} {
		if !strings.Contains(statusOutput.String(), want) {
			t.Fatalf("expected %q in status output:\n%s", want, statusOutput.String())
		}
	}

	daemonStatusJSON = true
	jsonCmd, jsonOutput := testCommand()
	if err := runDaemonStatus(jsonCmd, nil); err != nil {
		t.Fatalf("runDaemonStatus json returned error: %v", err)
	}
	if !strings.Contains(jsonOutput.String(), `"enabled_jobs"`) || !strings.Contains(jsonOutput.String(), `"paused": true`) {
		t.Fatalf("unexpected status json: %s", jsonOutput.String())
	}

	resumeCmd, resumeOutput := testCommand()
	if err := runDaemonResume(resumeCmd, nil); err != nil {
		t.Fatalf("runDaemonResume returned error: %v", err)
	}
	if !strings.Contains(resumeOutput.String(), "daemon resumed") {
		t.Fatalf("unexpected resume output: %s", resumeOutput.String())
	}
}

func restoreDaemonFlags(t *testing.T) {
	t.Helper()
	oldRoot := daemonRoot
	oldRunOnce := daemonRunOnce
	oldRunBackground := daemonRunBackground
	oldRunDryRun := daemonRunDryRun
	oldInterval := daemonInterval
	oldSemanticRun := daemonCodexSemanticRun
	oldAcknowledgeCost := daemonAcknowledgeCost
	oldCodexCommand := daemonCodexCommand
	oldMaxTurns := daemonCodexMaxTurns
	oldTimeout := daemonCodexTimeout
	oldTurnTimeout := daemonCodexTurnTimeout
	oldIsolatedHome := daemonCodexIsolatedHome
	oldForce := daemonTriggerForce
	oldTriggerDryRun := daemonTriggerDryRun
	oldStatusJSON := daemonStatusJSON
	oldStatusLimit := daemonStatusLimit
	oldPauseReason := daemonPauseReason
	t.Cleanup(func() {
		daemonRoot = oldRoot
		daemonRunOnce = oldRunOnce
		daemonRunBackground = oldRunBackground
		daemonRunDryRun = oldRunDryRun
		daemonInterval = oldInterval
		daemonCodexSemanticRun = oldSemanticRun
		daemonAcknowledgeCost = oldAcknowledgeCost
		daemonCodexCommand = oldCodexCommand
		daemonCodexMaxTurns = oldMaxTurns
		daemonCodexTimeout = oldTimeout
		daemonCodexTurnTimeout = oldTurnTimeout
		daemonCodexIsolatedHome = oldIsolatedHome
		daemonTriggerForce = oldForce
		daemonTriggerDryRun = oldTriggerDryRun
		daemonStatusJSON = oldStatusJSON
		daemonStatusLimit = oldStatusLimit
		daemonPauseReason = oldPauseReason
	})
	daemonRoot = "."
	daemonRunOnce = false
	daemonRunBackground = false
	daemonRunDryRun = false
	daemonInterval = 5 * time.Second
	daemonCodexSemanticRun = false
	daemonAcknowledgeCost = false
	daemonCodexCommand = "codex"
	daemonCodexMaxTurns = 3
	daemonCodexTimeout = 5 * time.Minute
	daemonCodexTurnTimeout = 3 * time.Minute
	daemonCodexIsolatedHome = false
	daemonTriggerForce = false
	daemonTriggerDryRun = false
	daemonStatusJSON = false
	daemonStatusLimit = 10
	daemonPauseReason = "manual"
}

func writeCommandDaemonJob(t *testing.T, root, id, eventType, command string) {
	t.Helper()
	path := filepath.Join(root, "harness", "control", "jobs", id+".yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir control jobs: %v", err)
	}
	body := "id: " + id + "\nwhen:\n  event: " + eventType + "\ndo:\n  cli: " + strconvQuote(command) + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write daemon job: %v", err)
	}
}

func strconvQuote(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}
