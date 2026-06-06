package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/eventlog"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/reactor"
	runnercodex "github.com/mnemon-dev/mnemon/harness/internal/lifecycle/runner/codex"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
)

func TestTickRefreshesStatusAndWritesDaemonCheckpoint(t *testing.T) {
	root := t.TempDir()
	store, err := eventlog.New(root)
	if err != nil {
		t.Fatalf("eventlog.New returned error: %v", err)
	}
	event := fixtureEvent("evt_daemon_001", "memory.hot_write_observed")
	if err := store.Append(event); err != nil {
		t.Fatalf("append event: %v", err)
	}

	d, err := New(root, Options{OwnerID: "test-daemon"})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	now := time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC)
	result, err := d.Tick(context.Background(), now)
	if err != nil {
		t.Fatalf("Tick returned error: %v", err)
	}
	if result.LastProcessedEventID != event.ID {
		t.Fatalf("last processed mismatch: %#v", result)
	}

	assertFileExists(t, filepath.Join(root, ".mnemon", "harness", "daemon", "checkpoint.json"))
	assertFileExists(t, filepath.Join(root, ".mnemon", "harness", "daemon", "tick-log.jsonl"))
	assertFileExists(t, filepath.Join(root, ".mnemon", "harness", "status", "daemon.json"))
	assertFileExists(t, filepath.Join(root, ".mnemon", "harness", "status", "loops", "memory.json"))

	events, err := store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	if len(events) != 2 || events[1].Type != "daemon.phase_changed" {
		t.Fatalf("expected one daemon phase event, got %#v", events)
	}
	if _, err := d.Tick(context.Background(), now.Add(time.Minute)); err != nil {
		t.Fatalf("second Tick returned error: %v", err)
	}
	events, err = store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("ready phase should not append duplicate daemon event, got %d events", len(events))
	}
	if got := countLines(t, filepath.Join(root, ".mnemon", "harness", "daemon", "tick-log.jsonl")); got != 4 {
		t.Fatalf("expected started/completed tick records for two ticks, got %d", got)
	}
}

func TestProjectLockWritesPIDAndRemovesOwnedLock(t *testing.T) {
	root := t.TempDir()
	d, err := New(root, Options{OwnerID: "owner-a"})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	now := time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC)
	lockPath := filepath.Join(root, ".mnemon", "harness", "daemon", "daemon.lock")

	if err := withProjectLock(d.paths, "owner-a", now, func() error {
		info, err := readProjectLock(lockPath)
		if err != nil {
			t.Fatalf("readProjectLock returned error: %v", err)
		}
		if info.OwnerID != "owner-a" || info.PID != os.Getpid() || info.Token == "" {
			t.Fatalf("unexpected lock info: %#v", info)
		}
		return nil
	}); err != nil {
		t.Fatalf("withProjectLock returned error: %v", err)
	}
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("expected owned lock to be removed, stat err=%v", err)
	}
}

func TestProjectLockRecoversStaleDeadPIDLock(t *testing.T) {
	root := t.TempDir()
	d, err := New(root, Options{OwnerID: "owner-new"})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	lockPath := filepath.Join(root, ".mnemon", "harness", "daemon", "daemon.lock")
	writeProjectLockFixture(t, lockPath, projectLockInfo{
		SchemaVersion: 1,
		OwnerID:       "owner-old",
		PID:           unusedPID(t),
		AcquiredAt:    time.Date(2026, 5, 24, 8, 0, 0, 0, time.UTC).Format(time.RFC3339),
		Token:         "owner-old-token",
	})

	var ran bool
	now := time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC)
	if err := withProjectLock(d.paths, "owner-new", now, func() error {
		ran = true
		info, err := readProjectLock(lockPath)
		if err != nil {
			t.Fatalf("readProjectLock returned error: %v", err)
		}
		if info.OwnerID != "owner-new" || info.PID != os.Getpid() {
			t.Fatalf("expected recovered lock owner, got %#v", info)
		}
		return nil
	}); err != nil {
		t.Fatalf("withProjectLock should recover stale lock: %v", err)
	}
	if !ran {
		t.Fatalf("expected lock callback to run")
	}
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("expected recovered lock to be removed, stat err=%v", err)
	}
}

func TestProjectLockKeepsLivePIDLock(t *testing.T) {
	root := t.TempDir()
	d, err := New(root, Options{OwnerID: "owner-new"})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	lockPath := filepath.Join(root, ".mnemon", "harness", "daemon", "daemon.lock")
	writeProjectLockFixture(t, lockPath, projectLockInfo{
		SchemaVersion: 1,
		OwnerID:       "owner-live",
		PID:           os.Getpid(),
		AcquiredAt:    time.Date(2026, 5, 24, 8, 0, 0, 0, time.UTC).Format(time.RFC3339),
		Token:         "owner-live-token",
	})

	err = withProjectLock(d.paths, "owner-new", time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC), func() error {
		t.Fatalf("callback should not run for live lock")
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "daemon lock already held") {
		t.Fatalf("expected live lock error, got %v", err)
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("expected live lock to remain: %v", err)
	}
}

func TestLeaseJobPreventsDuplicateExecutionBeforeExpiry(t *testing.T) {
	root := t.TempDir()
	d, err := New(root, Options{OwnerID: "owner-a", LeaseTTL: time.Minute})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	job := fixtureJob("job_once", "deterministic", reactor.StatusRefreshID)
	if err := d.Enqueue(job); err != nil {
		t.Fatalf("Enqueue returned error: %v", err)
	}

	now := time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC)
	if _, err := d.LeaseJob(job.ID, now); err != nil {
		t.Fatalf("first LeaseJob returned error: %v", err)
	}
	if _, err := d.LeaseJob(job.ID, now.Add(10*time.Second)); !errors.Is(err, ErrLeaseHeld) {
		t.Fatalf("expected ErrLeaseHeld, got %v", err)
	}
}

func TestExpiredLeaseCanBeRecovered(t *testing.T) {
	root := t.TempDir()
	d, err := New(root, Options{OwnerID: "owner-a", LeaseTTL: time.Minute})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	job := fixtureJob("job_recover", "deterministic", reactor.StatusRefreshID)
	if err := d.Enqueue(job); err != nil {
		t.Fatalf("Enqueue returned error: %v", err)
	}

	start := time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC)
	if _, err := d.LeaseJob(job.ID, start); err != nil {
		t.Fatalf("first LeaseJob returned error: %v", err)
	}
	recovered, err := d.LeaseJob(job.ID, start.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("expired lease should recover: %v", err)
	}
	if recovered.Attempts != 2 {
		t.Fatalf("expected attempts to increment, got %d", recovered.Attempts)
	}
}

func TestTickProcessesDeterministicAndBlocksSemanticJob(t *testing.T) {
	root := t.TempDir()
	store, err := eventlog.New(root)
	if err != nil {
		t.Fatalf("eventlog.New returned error: %v", err)
	}
	if err := store.Append(fixtureEvent("evt_daemon_002", "skill.usage_observed")); err != nil {
		t.Fatalf("append event: %v", err)
	}

	d, err := New(root, Options{OwnerID: "test-daemon"})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if err := d.Enqueue(fixtureJob("job_status", "deterministic", reactor.StatusRefreshID)); err != nil {
		t.Fatalf("enqueue deterministic job: %v", err)
	}
	if err := d.Enqueue(fixtureJob("job_semantic", "semantic", "skill.curator")); err != nil {
		t.Fatalf("enqueue semantic job: %v", err)
	}

	result, err := d.Tick(context.Background(), time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Tick returned error: %v", err)
	}
	if result.JobsProcessed != 2 || result.JobsBlocked != 1 || !result.CostGateBlocked {
		t.Fatalf("unexpected job result: %#v", result)
	}
	assertFileExists(t, filepath.Join(root, ".mnemon", "harness", "jobs", "completed", "job_status.json"))
	assertFileExists(t, filepath.Join(root, ".mnemon", "harness", "jobs", "blocked", "job_semantic.json"))

	data, err := os.ReadFile(filepath.Join(root, ".mnemon", "harness", "status", "daemon.json"))
	if err != nil {
		t.Fatalf("read daemon status: %v", err)
	}
	var daemonStatus struct {
		Status struct {
			JobsBlocked int `json:"jobs_blocked"`
			QueueDepth  struct {
				Blocked int `json:"blocked"`
			} `json:"queue_depth"`
		} `json:"status"`
	}
	if err := json.Unmarshal(data, &daemonStatus); err != nil {
		t.Fatalf("decode daemon status: %v", err)
	}
	if daemonStatus.Status.JobsBlocked != 1 || daemonStatus.Status.QueueDepth.Blocked != 1 {
		t.Fatalf("daemon status missing blocked job: %#v", daemonStatus)
	}
	if !tickLogContainsReason(t, root, "cost_gate_off") {
		t.Fatalf("tick log did not record cost_gate_off")
	}
}

func TestTickSkipsUnknownDeterministicReactor(t *testing.T) {
	root := t.TempDir()
	d, err := New(root, Options{OwnerID: "test-daemon"})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if err := d.Enqueue(fixtureJob("job_unknown_reactor", "deterministic", "unknown.reactor")); err != nil {
		t.Fatalf("enqueue deterministic job: %v", err)
	}

	result, err := d.Tick(context.Background(), time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Tick returned error: %v", err)
	}
	if result.JobsProcessed != 1 || result.JobsBlocked != 0 {
		t.Fatalf("unexpected job result: %#v", result)
	}
	assertFileExists(t, filepath.Join(root, ".mnemon", "harness", "jobs", "skipped", "job_unknown_reactor.json"))
}

func TestTickDispatchesSemanticJobToCodexRunner(t *testing.T) {
	root := t.TempDir()
	writeDaemonCodexProjectionFixture(t, root)
	store, err := eventlog.New(root)
	if err != nil {
		t.Fatalf("eventlog.New returned error: %v", err)
	}
	if err := store.Append(fixtureEvent("evt_daemon_003", "memory.nightly_dream_requested")); err != nil {
		t.Fatalf("append event: %v", err)
	}

	d, err := New(root, Options{
		OwnerID:                "test-daemon",
		EnableCodexSemanticRun: true,
		AcknowledgeModelCost:   true,
		CodexCommand:           os.Args[0],
		CodexArgs:              []string{"-test.run=TestFakeDaemonCodexAppServer", "--"},
		CodexEnv:               []string{"MNEMON_FAKE_DAEMON_CODEX=ready"},
		CodexMaxTurns:          1,
		CodexTurnTimeout:       time.Second,
		CodexTimeout:           5 * time.Second,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	job := fixtureJob("job_semantic_codex", "semantic", "memory.dreaming")
	job.JobSpecRef = "memory.dreaming"
	job.Target = map[string]any{
		"loop":   "memory",
		"prompt": "Return a lifecycle memory summary.",
	}
	job.Budget = map[string]any{"max_turns": 1}
	if err := d.Enqueue(job); err != nil {
		t.Fatalf("enqueue semantic job: %v", err)
	}

	result, err := d.Tick(context.Background(), time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Tick returned error: %v", err)
	}
	if result.JobsProcessed != 1 || result.JobsBlocked != 0 || result.RealTurnsUsed != 1 {
		t.Fatalf("unexpected tick result: %#v", result)
	}
	assertFileExists(t, filepath.Join(root, ".mnemon", "harness", "jobs", "completed", "job_semantic_codex.json"))
	assertFileExists(t, filepath.Join(root, ".mnemon", "harness", "status", "runners", "codex-app-server.json"))
	assertFileExists(t, filepath.Join(root, ".mnemon", "harness", "status", "jobs", "job_semantic_codex.json"))
	reports, err := filepath.Glob(filepath.Join(root, ".mnemon", "harness", "reports", "runner", "*.json"))
	if err != nil || len(reports) != 1 {
		t.Fatalf("expected one runner report, got %v err=%v", reports, err)
	}
	var report runnercodex.SemanticReport
	readJSONFile(t, reports[0], &report)
	if report.Loop != "memory" {
		t.Fatalf("expected memory loop report, got %#v", report)
	}
	assertFileExists(t, filepath.Join(report.Workspace, ".mnemon", "harness", "memory", "MEMORY.md"))
	assertFileExists(t, filepath.Join(report.Workspace, ".codex", "mnemon-memory", "env.sh"))

	events, err := store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	if len(events) != 7 {
		t.Fatalf("expected request plus runner, audit, and daemon phase events, got %d", len(events))
	}
}

func TestTickEnqueuesDeclaredControllerJobWithRunnerBinding(t *testing.T) {
	root := t.TempDir()
	writeDaemonControllerFixture(t, root)
	store, err := eventlog.New(root)
	if err != nil {
		t.Fatalf("eventlog.New returned error: %v", err)
	}
	event := fixtureEvent("evt_controller_001", "memory.hot_write_observed")
	host := "claude-code"
	event.Host = &host
	if err := store.Append(event); err != nil {
		t.Fatalf("append event: %v", err)
	}

	d, err := New(root, Options{OwnerID: "test-daemon"})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	result, err := d.Tick(context.Background(), time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Tick returned error: %v", err)
	}
	if result.JobsProcessed != 1 || result.JobsBlocked != 1 {
		t.Fatalf("unexpected tick result: %#v", result)
	}

	jobPath := filepath.Join(root, ".mnemon", "harness", "jobs", "blocked", "job_memory_dreaming_on_hot_write_evt_controller_001.json")
	var job Job
	readJSONFile(t, jobPath, &job)
	if job.JobSpecRef != "memory.dreaming" {
		t.Fatalf("unexpected job spec ref: %#v", job)
	}
	if got := targetString(job.Target, "runner_mode"); got != "native_subagent" {
		t.Fatalf("expected native subagent runner binding, got %q", got)
	}
	if got := targetString(job.Target, "agent"); got != "mnemon-dreaming" {
		t.Fatalf("expected mnemon-dreaming agent, got %q", got)
	}
	if !strings.Contains(targetString(job.Target, "prompt"), "dreaming fixture") {
		t.Fatalf("job prompt did not include declared prompt asset: %s", targetString(job.Target, "prompt"))
	}
	selection, _ := job.Result["runner_selection"].(map[string]any)
	if selection["selected_runner"] != "codex-app-server" || selection["degraded"] != true {
		t.Fatalf("unexpected runner selection: %#v", selection)
	}
}

func TestTickProcessesDeclarativeCLIJob(t *testing.T) {
	root := t.TempDir()
	writeDaemonJobFixture(t, root, "test.echo", "daemon.example_requested", "printf declarative")
	store, err := eventlog.New(root)
	if err != nil {
		t.Fatalf("eventlog.New returned error: %v", err)
	}
	if err := store.Append(fixtureEvent("evt_declarative_001", "daemon.example_requested")); err != nil {
		t.Fatalf("append event: %v", err)
	}

	d, err := New(root, Options{OwnerID: "test-daemon"})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	result, err := d.Tick(context.Background(), time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Tick returned error: %v", err)
	}
	if result.JobsProcessed != 1 || result.JobsBlocked != 0 {
		t.Fatalf("unexpected tick result: %#v", result)
	}
	var job Job
	readJSONFile(t, filepath.Join(root, ".mnemon", "harness", "jobs", "completed", "job_test.echo_evt_declarative_001.json"), &job)
	if job.Type != "cli" || job.Result["stdout"] != "declarative" {
		t.Fatalf("unexpected cli job: %#v", job)
	}
}

func TestTickPausedBlocksNewEnqueueButProcessesQueuedJobs(t *testing.T) {
	root := t.TempDir()
	writeDaemonJobFixture(t, root, "test.echo", "daemon.example_requested", "printf declarative")
	store, err := eventlog.New(root)
	if err != nil {
		t.Fatalf("eventlog.New returned error: %v", err)
	}
	if err := store.Append(fixtureEvent("evt_paused_001", "daemon.example_requested")); err != nil {
		t.Fatalf("append event: %v", err)
	}
	d, err := New(root, Options{OwnerID: "test-daemon"})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if err := d.Enqueue(fixtureJob("job_existing_cli", "cli", "test.echo")); err != nil {
		t.Fatalf("enqueue existing job: %v", err)
	}
	existingPath := filepath.Join(root, ".mnemon", "harness", "jobs", "queued", "job_existing_cli.json")
	var existing Job
	readJSONFile(t, existingPath, &existing)
	existing.Target = map[string]any{"cli": "printf existing"}
	if err := writeJSONAtomic(existingPath, existing); err != nil {
		t.Fatalf("rewrite existing job: %v", err)
	}
	if _, err := Pause(root, "test pause", time.Date(2026, 5, 24, 8, 59, 0, 0, time.UTC)); err != nil {
		t.Fatalf("Pause returned error: %v", err)
	}

	result, err := d.Tick(context.Background(), time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("paused Tick returned error: %v", err)
	}
	if !result.Paused || result.JobsProcessed != 1 {
		t.Fatalf("expected paused tick to process existing job only: %#v", result)
	}
	assertFileExists(t, filepath.Join(root, ".mnemon", "harness", "jobs", "completed", "job_existing_cli.json"))
	if matches, _ := filepath.Glob(filepath.Join(root, ".mnemon", "harness", "jobs", "queued", "job_test.echo_*.json")); len(matches) != 0 {
		t.Fatalf("paused tick enqueued new declarative jobs: %v", matches)
	}

	if _, err := Resume(root, time.Date(2026, 5, 24, 9, 1, 0, 0, time.UTC)); err != nil {
		t.Fatalf("Resume returned error: %v", err)
	}
	result, err = d.Tick(context.Background(), time.Date(2026, 5, 24, 9, 1, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("resumed Tick returned error: %v", err)
	}
	if result.Paused || result.JobsProcessed != 1 {
		t.Fatalf("expected resumed tick to process declarative job: %#v", result)
	}
	if matches, _ := filepath.Glob(filepath.Join(root, ".mnemon", "harness", "jobs", "completed", "job_test.echo_*.json")); len(matches) != 1 {
		t.Fatalf("expected resumed declarative job completion, got %v", matches)
	}
}

func TestTickAutoPausesWhenGlobalBudgetExhausted(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "harness", "control", "jobs"), 0o755); err != nil {
		t.Fatalf("mkdir control jobs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "harness", "control", "daemon.yaml"), []byte("global_budget:\n  daily_cost_usd: 0.01\n  daily_real_turns: 20\n  enabled: true\n"), 0o644); err != nil {
		t.Fatalf("write global budget: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "harness", "control", "jobs", "runaway.yaml"), []byte("id: runaway.echo\nwhen:\n  event: runaway.tick\ndo:\n  cli: \"printf runaway\"\nbudget:\n  cost_usd: 0.01\n  max_sec: 5\n"), 0o644); err != nil {
		t.Fatalf("write runaway job: %v", err)
	}
	store, err := eventlog.New(root)
	if err != nil {
		t.Fatalf("eventlog.New returned error: %v", err)
	}
	if err := store.Append(fixtureEvent("evt_runaway_001", "runaway.tick")); err != nil {
		t.Fatalf("append event: %v", err)
	}
	d, err := New(root, Options{OwnerID: "test-daemon"})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	first, err := d.Tick(context.Background(), time.Date(2026, 5, 24, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("first Tick returned error: %v", err)
	}
	if first.JobsProcessed != 1 || first.Paused {
		t.Fatalf("unexpected first tick: %#v", first)
	}
	second, err := d.Tick(context.Background(), time.Date(2026, 5, 24, 9, 1, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("second Tick returned error: %v", err)
	}
	if !second.Paused || second.PauseReason == "" || second.JobsProcessed != 0 {
		t.Fatalf("expected auto-paused budget tick: %#v", second)
	}
	pause, err := IsPaused(root)
	if err != nil {
		t.Fatalf("IsPaused returned error: %v", err)
	}
	if !pause.Paused || !strings.Contains(pause.Reason, "budget_exhausted") {
		t.Fatalf("unexpected pause state: %#v", pause)
	}
}

func TestTickRecordsFailedCLIJobAndContinues(t *testing.T) {
	root := t.TempDir()
	d, err := New(root, Options{OwnerID: "test-daemon"})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	failing := fixtureJob("job_cli_fail", "cli", "test.fail")
	failing.Target = map[string]any{"cli": "printf fail >&2; exit 1"}
	succeeding := fixtureJob("job_cli_ok", "cli", "test.ok")
	succeeding.Target = map[string]any{"cli": "printf ok"}
	if err := d.Enqueue(failing); err != nil {
		t.Fatalf("enqueue failing CLI job: %v", err)
	}
	if err := d.Enqueue(succeeding); err != nil {
		t.Fatalf("enqueue succeeding CLI job: %v", err)
	}

	result, err := d.Tick(context.Background(), time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Tick returned error: %v", err)
	}
	if result.JobsProcessed != 2 || result.JobsFailed != 1 || result.JobsBlocked != 0 {
		t.Fatalf("unexpected tick result: %#v", result)
	}
	var failed Job
	readJSONFile(t, filepath.Join(root, ".mnemon", "harness", "jobs", "failed", "job_cli_fail.json"), &failed)
	if failed.Result["reason"] != "CLIJobFailed" || failed.Result["stderr"] != "fail" {
		t.Fatalf("unexpected failed job result: %#v", failed.Result)
	}
	var completed Job
	readJSONFile(t, filepath.Join(root, ".mnemon", "harness", "jobs", "completed", "job_cli_ok.json"), &completed)
	if completed.Result["stdout"] != "ok" {
		t.Fatalf("unexpected completed job result: %#v", completed.Result)
	}
	tickLog, err := os.ReadFile(filepath.Join(root, ".mnemon", "harness", "daemon", "tick-log.jsonl"))
	if err != nil {
		t.Fatalf("read tick log: %v", err)
	}
	if !strings.Contains(string(tickLog), `"jobs_failed":1`) {
		t.Fatalf("tick log did not record failed job: %s", string(tickLog))
	}
	if _, err := d.Tick(context.Background(), time.Date(2026, 5, 28, 12, 1, 0, 0, time.UTC)); err != nil {
		t.Fatalf("next Tick returned error: %v", err)
	}
}

func TestTickReloadsDeclarativeJobOnNextTick(t *testing.T) {
	root := t.TempDir()
	writeDaemonJobFixture(t, root, "test.reload", "daemon.first", "printf first")
	store, err := eventlog.New(root)
	if err != nil {
		t.Fatalf("eventlog.New returned error: %v", err)
	}
	if err := store.Append(fixtureEvent("evt_reload_001", "daemon.first")); err != nil {
		t.Fatalf("append first event: %v", err)
	}
	d, err := New(root, Options{OwnerID: "test-daemon"})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if _, err := d.Tick(context.Background(), time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("first Tick returned error: %v", err)
	}

	writeDaemonJobFixture(t, root, "test.reload", "daemon.second", "printf second")
	if err := store.Append(fixtureEvent("evt_reload_002", "daemon.second")); err != nil {
		t.Fatalf("append second event: %v", err)
	}
	if _, err := d.Tick(context.Background(), time.Date(2026, 5, 28, 12, 1, 0, 0, time.UTC)); err != nil {
		t.Fatalf("second Tick returned error: %v", err)
	}
	var job Job
	readJSONFile(t, filepath.Join(root, ".mnemon", "harness", "jobs", "completed", "job_test.reload_evt_reload_002.json"), &job)
	if job.Result["stdout"] != "second" {
		t.Fatalf("expected hot reloaded CLI output, got %#v", job.Result)
	}
}

func TestFakeDaemonCodexAppServer(t *testing.T) {
	if os.Getenv("MNEMON_FAKE_DAEMON_CODEX") == "" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var msg map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			fmt.Fprintln(os.Stdout, `{"id":1,"error":{"message":"bad request"}}`)
			continue
		}
		id, _ := msg["id"].(float64)
		method, _ := msg["method"].(string)
		if id == 0 {
			continue
		}
		switch method {
		case "initialize":
			fmt.Fprintf(os.Stdout, `{"id":%d,"result":{"userAgent":"fake-codex","codexHome":"/tmp/fake"}}`+"\n", int(id))
		case "skills/list":
			fmt.Fprintf(os.Stdout, `{"id":%d,"result":{"skills":[]}}`+"\n", int(id))
		case "model/list":
			fmt.Fprintf(os.Stdout, `{"id":%d,"result":{"models":[]}}`+"\n", int(id))
		case "thread/start":
			fmt.Fprintf(os.Stdout, `{"id":%d,"result":{"thread":{"id":"thread_fake"}}}`+"\n", int(id))
		case "turn/start":
			fmt.Fprintf(os.Stdout, `{"id":%d,"result":{"turn":{"id":"turn_fake"}}}`+"\n", int(id))
			fmt.Fprintln(os.Stdout, `{"method":"turn/completed","params":{"threadId":"thread_fake","turnId":"turn_fake","status":"completed"}}`)
		default:
			fmt.Fprintf(os.Stdout, `{"id":%d,"result":{}}`+"\n", int(id))
		}
	}
	os.Exit(0)
}

func writeDaemonControllerFixture(t *testing.T, root string) {
	t.Helper()
	loopDir := filepath.Join(root, "harness", "loops", "memory")
	bindingDir := filepath.Join(root, "harness", "bindings")
	for _, dir := range []string{filepath.Join(loopDir, "subagents"), bindingDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(loopDir, "subagents", "dreaming.md"), []byte("dreaming fixture\n"), 0o644); err != nil {
		t.Fatalf("write dreaming fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(loopDir, "loop.json"), []byte(`{
  "schema_version": 2,
  "name": "memory",
  "assets": {
    "guide": "GUIDE.md",
    "env": "env.sh",
    "hook_prompts": {},
    "skills": [],
    "subagents": ["subagents/dreaming.md"]
  },
  "controllers": [
    {
      "name": "memory.dreaming.on_hot_write",
      "watches": ["memory.hot_write_observed"],
      "enqueue": "memory.dreaming",
      "reason": "fixture"
    }
  ],
  "jobs": {
    "memory.dreaming": {
      "type": "semantic",
      "spec": "subagents/dreaming.md",
      "preferred_runner": "host-subagent",
      "fallback_runner": "codex-app-server",
      "prompt": "controller prompt",
      "max_turns": 2
    }
  }
}`), 0o644); err != nil {
		t.Fatalf("write loop manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bindingDir, "claude-code.memory.json"), []byte(`{
  "schema_version": 1,
  "name": "claude-code.memory",
  "host": "claude-code",
  "loop": "memory",
  "projection_path": ".claude",
  "runtime_surface": ".claude/mnemon-memory",
  "lifecycle_mapping": {},
  "runner_bindings": {
    "memory.dreaming": {
      "mode": "native_subagent",
      "agent": "mnemon-dreaming",
      "fallback_runner": "codex-app-server"
    }
  },
  "reconcile": ["read"]
}`), 0o644); err != nil {
		t.Fatalf("write binding manifest: %v", err)
	}
}

func writeDaemonCodexProjectionFixture(t *testing.T, root string) {
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

func writeDaemonJobFixture(t *testing.T, root, id, eventType, command string) {
	t.Helper()
	body := fmt.Sprintf("id: %s\nwhen:\n  event: %s\ndo:\n  cli: %q\nbudget:\n  cost_usd: 0\n  max_sec: 5\n", id, eventType, command)
	path := filepath.Join(root, "harness", "control", "jobs", id+".yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir control jobs: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write daemon job fixture: %v", err)
	}
}

func readJSONFile(t *testing.T, path string, target any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
}

func writeProjectLockFixture(t *testing.T, path string, info projectLockInfo) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create lock parent: %v", err)
	}
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("marshal lock info: %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write lock fixture: %v", err)
	}
}

func unusedPID(t *testing.T) int {
	t.Helper()
	for pid := 999999; pid > 100000; pid-- {
		if !processAlive(pid) {
			return pid
		}
	}
	t.Fatalf("could not find an unused PID")
	return 0
}

func fixtureEvent(id, typ string) schema.Event {
	loop := "memory"
	if len(typ) >= len("skill") && typ[:len("skill")] == "skill" {
		loop = "skill"
	}
	host := "codex"
	return schema.Event{
		SchemaVersion: 1,
		ID:            id,
		TS:            "2026-05-24T08:30:00Z",
		Type:          typ,
		Loop:          &loop,
		Host:          &host,
		Actor:         "host-agent",
		Source:        "fixture",
		CorrelationID: "corr_fixture",
		CausedBy:      nil,
		Payload:       map[string]any{"reason": "fixture"},
	}
}

func fixtureJob(id, jobType, reactorID string) Job {
	return Job{
		SchemaVersion: JobSchemaVersion,
		ID:            id,
		Type:          jobType,
		ReactorID:     reactorID,
		Target:        map[string]any{"loop": "memory"},
		Priority:      "normal",
		Status:        "queued",
		DueAt:         "2026-05-24T08:30:00Z",
		Attempts:      0,
		MaxAttempts:   3,
		EvidenceRefs:  []string{"evt_daemon_002"},
		CorrelationID: "corr_fixture",
	}
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}

func countLines(t *testing.T, path string) int {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	var count int
	for scanner.Scan() {
		count++
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	return count
}

func tickLogContainsReason(t *testing.T, root, reason string) bool {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".mnemon", "harness", "daemon", "tick-log.jsonl"))
	if err != nil {
		t.Fatalf("read tick log: %v", err)
	}
	return strings.Contains(string(data), `"reason":"`+reason+`"`)
}
