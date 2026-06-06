package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	daemonjob "github.com/mnemon-dev/mnemon/harness/internal/lifecycle/daemon/job"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/daemon/loader"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/daemon/metric"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/daemon/trigger"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/eventlog"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/layout"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/reactor"
	runnercodex "github.com/mnemon-dev/mnemon/harness/internal/lifecycle/runner/codex"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
)

const JobSchemaVersion = daemonjob.SchemaVersion

var ErrLeaseHeld = errors.New("job lease is held")

type Options struct {
	OwnerID                string
	LeaseTTL               time.Duration
	EnableCodexSemanticRun bool
	AcknowledgeModelCost   bool
	CodexCommand           string
	CodexArgs              []string
	CodexEnv               []string
	CodexMaxTurns          int
	CodexTimeout           time.Duration
	CodexTurnTimeout       time.Duration
	CodexIsolatedHome      bool
}

type Daemon struct {
	paths layout.Paths
	opts  Options
}

type Checkpoint struct {
	SchemaVersion        int    `json:"schema_version"`
	LastProcessedEventID string `json:"last_processed_event_id,omitempty"`
	UpdatedAt            string `json:"updated_at"`
}

type TickResult struct {
	LastProcessedEventID string
	EventCount           int
	StatusFilesWritten   int
	JobsProcessed        int
	JobsFailed           int
	JobsBlocked          int
	RealTurnsUsed        int
	Paused               bool
	PauseReason          string
	CostGateBlocked      bool
}

type TickLogRecord struct {
	SchemaVersion        int    `json:"schema_version"`
	TickID               string `json:"tick_id"`
	Status               string `json:"status"`
	TS                   string `json:"ts"`
	OwnerID              string `json:"owner_id"`
	LastProcessedEventID string `json:"last_processed_event_id,omitempty"`
	EventCount           int    `json:"event_count"`
	StatusFilesWritten   int    `json:"status_files_written"`
	JobsProcessed        int    `json:"jobs_processed"`
	JobsFailed           int    `json:"jobs_failed"`
	JobsBlocked          int    `json:"jobs_blocked"`
	RealTurnsUsed        int    `json:"real_turns_used"`
	Reason               string `json:"reason,omitempty"`
	Message              string `json:"message,omitempty"`
}

// Job is the canonical daemon job, defined once in the daemon/job leaf package and
// aliased here so the queue's persistence/lease logic and the materializer share ONE
// struct (no Runtime/Job/jobFromRuntime triple). Lease is likewise the job lease.
type Job = daemonjob.Job

type Lease = daemonjob.Lease

type QueueDepth struct {
	Queued    int `json:"queued"`
	Leased    int `json:"leased"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
	Blocked   int `json:"blocked"`
	Skipped   int `json:"skipped"`
}

type projectLockInfo struct {
	SchemaVersion int    `json:"schema_version"`
	OwnerID       string `json:"owner_id"`
	PID           int    `json:"pid"`
	AcquiredAt    string `json:"acquired_at"`
	Token         string `json:"token"`
}

func New(root string, opts Options) (*Daemon, error) {
	paths, err := layout.Resolve(root)
	if err != nil {
		return nil, err
	}
	if opts.OwnerID == "" {
		opts.OwnerID = fmt.Sprintf("mnemon-daemon-%d", os.Getpid())
	}
	if opts.LeaseTTL <= 0 {
		opts.LeaseTTL = 5 * time.Minute
	}
	return &Daemon{paths: paths, opts: opts}, nil
}

func (d *Daemon) Enqueue(job Job) error {
	if _, err := layout.EnsureProject(d.paths.Root); err != nil {
		return err
	}
	if err := validateJob(job); err != nil {
		return err
	}
	path := d.jobPath("queued", job.ID)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("job %q already exists", job.ID)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat job: %w", err)
	}
	return writeJSONAtomic(path, job)
}

func (d *Daemon) LeaseJob(jobID string, now time.Time) (Job, error) {
	if _, err := layout.EnsureProject(d.paths.Root); err != nil {
		return Job{}, err
	}
	path := d.jobPath("queued", jobID)
	var job Job
	if err := readJSON(path, &job); err != nil {
		return Job{}, err
	}
	if err := validateJob(job); err != nil {
		return Job{}, err
	}
	if job.Lease != nil && !leaseExpired(*job.Lease, now) {
		return Job{}, ErrLeaseHeld
	}
	job.Status = "leased"
	job.Attempts++
	job.Lease = &Lease{
		OwnerID:    d.opts.OwnerID,
		AcquiredAt: now.UTC().Format(time.RFC3339),
		ExpiresAt:  now.UTC().Add(d.opts.LeaseTTL).Format(time.RFC3339),
	}
	job.UpdatedAt = now.UTC().Format(time.RFC3339)
	if err := writeJSONAtomic(path, job); err != nil {
		return Job{}, err
	}
	return job, nil
}

func (d *Daemon) Tick(ctx context.Context, now time.Time) (TickResult, error) {
	paths, err := layout.EnsureProject(d.paths.Root)
	if err != nil {
		return TickResult{}, err
	}
	d.paths = paths

	var result TickResult
	finalPhase := "ready"
	finalReason := "TickCompleted"
	finalMessage := "daemon tick completed"
	tickID := daemonTickID(now)
	_ = d.appendTickLog(tickLogRecord(tickID, "started", now, d.opts.OwnerID, result, "TickStarted", "daemon tick started"))
	err = withProjectLock(d.paths, d.opts.OwnerID, now, func() error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		events, err := d.readEvents()
		if err != nil {
			if statusErr := d.writeDaemonStatus(now, result, "degraded", "EventReplayFailed", err.Error()); statusErr != nil {
				return errors.Join(err, statusErr)
			}
			return err
		}
		result.EventCount = len(events)
		if len(events) > 0 {
			result.LastProcessedEventID = events[len(events)-1].ID
		}

		statusResult, err := reactor.RunStatusRefresh(d.paths.Root, now)
		if err != nil {
			if statusErr := d.writeDaemonStatus(now, result, "degraded", "StatusRefreshFailed", err.Error()); statusErr != nil {
				return errors.Join(err, statusErr)
			}
			return err
		}
		result.StatusFilesWritten = len(statusResult.Status.Written)

		if exceeded, reason, err := d.budgetExceeded(now); err != nil {
			if statusErr := d.writeDaemonStatus(now, result, "degraded", "BudgetCheckFailed", err.Error()); statusErr != nil {
				return errors.Join(err, statusErr)
			}
			return err
		} else if exceeded {
			if _, err := Pause(d.paths.Root, "budget_exhausted: "+reason, now); err != nil {
				return err
			}
		}

		pause, err := d.pauseState()
		if err != nil {
			return err
		}
		if pause.Paused {
			result.Paused = true
			result.PauseReason = pause.Reason
			finalPhase = "paused"
			if strings.HasPrefix(pause.Reason, "budget_exhausted") {
				finalReason = "BudgetExhausted"
				finalMessage = pause.Reason
			} else {
				finalReason = "Paused"
				finalMessage = "daemon paused: " + pause.Reason
			}
		} else {
			if _, err := d.enqueueDeclarativeJobs(ctx, events, now); err != nil {
				if statusErr := d.writeDaemonStatus(now, result, "degraded", "DeclarativeEnqueueFailed", err.Error()); statusErr != nil {
					return errors.Join(err, statusErr)
				}
				return err
			}
			if _, err := d.enqueueDeclaredControllerJobs(events, now); err != nil {
				if statusErr := d.writeDaemonStatus(now, result, "degraded", "ControllerEnqueueFailed", err.Error()); statusErr != nil {
					return errors.Join(err, statusErr)
				}
				return err
			}
		}

		processed, failed, blocked, turnsUsed, costGateBlocked, err := d.processDueJobs(ctx, now)
		if err != nil {
			if statusErr := d.writeDaemonStatus(now, result, "degraded", "JobProcessingFailed", err.Error()); statusErr != nil {
				return errors.Join(err, statusErr)
			}
			return err
		}
		result.JobsProcessed = processed
		result.JobsFailed = failed
		result.JobsBlocked = blocked
		result.RealTurnsUsed = turnsUsed
		result.CostGateBlocked = costGateBlocked
		if costGateBlocked && !result.Paused {
			finalReason = "cost_gate_off"
			finalMessage = "semantic jobs blocked because model-cost gate is off"
		}

		if err := d.writeCheckpoint(now, result.LastProcessedEventID); err != nil {
			return err
		}
		return d.writeDaemonStatus(now, result, finalPhase, finalReason, finalMessage)
	})
	if err != nil {
		if strings.Contains(err.Error(), "daemon lock already held") {
			_ = d.appendDaemonPhaseEvent(now, result, "blocked", "LockFailed", err.Error())
		}
		_ = d.appendTickLog(tickLogRecord(tickID, "failed", now, d.opts.OwnerID, result, "TickFailed", err.Error()))
		return TickResult{}, err
	}
	_ = d.appendTickLog(tickLogRecord(tickID, "completed", now, d.opts.OwnerID, result, finalReason, finalMessage))
	return result, nil
}

func (d *Daemon) readEvents() ([]schema.Event, error) {
	store, err := eventlog.New(d.paths.Root)
	if err != nil {
		return nil, err
	}
	return store.ReadAll()
}

func (d *Daemon) LoadCatalog() (loader.Catalog, error) {
	return loader.Load(d.paths.Root, loader.Options{AcknowledgeModelCost: d.opts.AcknowledgeModelCost})
}

func (d *Daemon) enqueueDeclarativeJobs(ctx context.Context, events []schema.Event, now time.Time) (int, error) {
	catalog, err := d.LoadCatalog()
	if err != nil {
		return 0, err
	}
	lastFired, err := d.loadLastFired()
	if err != nil {
		return 0, err
	}
	firedDirty := false
	enqueued := 0
	for _, def := range catalog.Jobs {
		if !def.IsEnabled() || def.Source.Kind == "loop_controller" {
			continue
		}
		var lastAt time.Time
		if ts, ok := lastFired[def.ID]; ok {
			lastAt, _ = time.Parse(time.RFC3339, ts)
		}
		decision, err := trigger.Evaluate(ctx, def.When, trigger.Input{
			Events: events,
			MetricContext: metric.Context{
				Root: d.paths.Root,
				Now:  now,
			},
			LastTriggeredAt: lastAt,
		})
		if err != nil {
			return enqueued, err
		}
		if !decision.Matched {
			continue
		}
		jobs, err := daemonjob.Materialize(def, decision, now)
		if err != nil {
			return enqueued, err
		}
		for _, job := range jobs {
			exists, err := d.jobExistsAnyStatus(job.ID)
			if err != nil {
				return enqueued, err
			}
			if exists {
				continue
			}
			if err := d.Enqueue(job); err != nil {
				return enqueued, err
			}
			enqueued++
			lastFired[def.ID] = now.UTC().Format(time.RFC3339)
			firedDirty = true
		}
	}
	if firedDirty {
		if err := d.writeLastFired(lastFired); err != nil {
			return enqueued, err
		}
	}
	return enqueued, nil
}

// loadLastFired reads the per-job last-fired timestamps used to gate interval
// (and other event-less) triggers. A missing file is treated as empty.
func (d *Daemon) loadLastFired() (map[string]string, error) {
	path := filepath.Join(d.paths.HarnessDir, "daemon", "last-fired.json")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	var m map[string]string
	if err := readJSON(path, &m); err != nil {
		return nil, err
	}
	if m == nil {
		m = map[string]string{}
	}
	return m, nil
}

// writeLastFired persists the per-job last-fired timestamps.
func (d *Daemon) writeLastFired(m map[string]string) error {
	return writeJSONAtomic(filepath.Join(d.paths.HarnessDir, "daemon", "last-fired.json"), m)
}

func (d *Daemon) processDueJobs(ctx context.Context, now time.Time) (int, int, int, int, bool, error) {
	jobs, err := d.dueJobs(now)
	if err != nil {
		return 0, 0, 0, 0, false, err
	}
	var processed int
	var failed int
	var blocked int
	var turnsUsed int
	var costGateBlocked bool
	for _, job := range jobs {
		leased, err := d.LeaseJob(job.ID, now)
		if err != nil {
			if errors.Is(err, ErrLeaseHeld) {
				continue
			}
			return processed, failed, blocked, turnsUsed, costGateBlocked, err
		}
		if leased.Type == "cli" {
			result, err := daemonjob.ExecuteCLI(ctx, d.paths.Root, loader.Action{
				CLI: targetString(leased.Target, "cli"),
				CWD: targetString(leased.Target, "cwd"),
				Env: targetStringMap(leased.Target, "env"),
			}, budgetInt(leased.Budget, "max_sec"))
			if err != nil {
				if failErr := d.finishJob(leased, "failed", now, map[string]any{
					"reason":    "CLIJobFailed",
					"message":   err.Error(),
					"exit_code": result.ExitCode,
					"stdout":    result.Stdout,
					"stderr":    result.Stderr,
				}); failErr != nil {
					return processed, failed, blocked, turnsUsed, costGateBlocked, errors.Join(err, failErr)
				}
				processed++
				failed++
				continue
			}
			if err := d.finishJob(leased, "completed", now, map[string]any{
				"outcome":   "completed",
				"exit_code": result.ExitCode,
				"stdout":    result.Stdout,
				"stderr":    result.Stderr,
			}); err != nil {
				return processed, failed, blocked, turnsUsed, costGateBlocked, err
			}
			processed++
			continue
		}
		if leased.Type == "deterministic" {
			result, err := reactor.DefaultRegistry().Run(ctx, leased.ReactorID, reactor.Context{
				Root: d.paths.Root,
				Now:  now,
			})
			if errors.Is(err, reactor.ErrNotFound) {
				stub := reactor.DispatchStub(leased.Type)
				if err := d.finishJob(leased, "skipped", now, map[string]any{
					"reactor_id": leased.ReactorID,
					"outcome":    stub.Outcome,
					"message":    stub.Message,
				}); err != nil {
					return processed, failed, blocked, turnsUsed, costGateBlocked, err
				}
				processed++
				continue
			}
			if err != nil {
				if failErr := d.finishJob(leased, "failed", now, map[string]any{"reason": "DeterministicReactorFailed", "message": err.Error()}); failErr != nil {
					return processed, failed, blocked, turnsUsed, costGateBlocked, errors.Join(err, failErr)
				}
				return processed, failed, blocked, turnsUsed, costGateBlocked, err
			}
			if err := d.finishJob(leased, "completed", now, map[string]any{
				"reactor_id": result.ReactorID,
				"outcome":    result.Outcome,
				"message":    result.Message,
			}); err != nil {
				return processed, failed, blocked, turnsUsed, costGateBlocked, err
			}
			processed++
			continue
		}
		statusValue, jobResult, jobTurns, err := d.dispatchSemanticJob(ctx, leased, now)
		if err != nil {
			if failErr := d.finishJob(leased, "failed", now, map[string]any{"reason": "SemanticDispatchFailed", "message": err.Error()}); failErr != nil {
				return processed, failed, blocked, turnsUsed, costGateBlocked, errors.Join(err, failErr)
			}
			return processed, failed, blocked, turnsUsed, costGateBlocked, err
		}
		if statusValue == "blocked" {
			blocked++
			if reason, _ := jobResult["reason"].(string); reason == "cost_gate_off" {
				costGateBlocked = true
			}
		} else if statusValue == "failed" {
			failed++
		}
		turnsUsed += jobTurns
		if err := d.finishJob(leased, statusValue, now, jobResult); err != nil {
			return processed, failed, blocked, turnsUsed, costGateBlocked, err
		}
		processed++
	}
	return processed, failed, blocked, turnsUsed, costGateBlocked, nil
}

func (d *Daemon) dispatchSemanticJob(ctx context.Context, job Job, now time.Time) (string, map[string]any, int, error) {
	if (job.Type == "semantic" || job.Type == "spawn_runner") && (!d.opts.EnableCodexSemanticRun || !d.opts.AcknowledgeModelCost) {
		selection := semanticRunnerSelection(job)
		stub := reactor.DispatchStub(job.Type)
		return "blocked", map[string]any{
			"reason":           "cost_gate_off",
			"outcome":          stub.Outcome,
			"message":          "semantic job requires explicit Codex runner and model-cost gate",
			"runner_selection": selection,
		}, 0, nil
	}
	if job.Type != "semantic" {
		stub := reactor.DispatchStub(job.Type)
		return "skipped", map[string]any{"outcome": stub.Outcome, "message": stub.Message}, 0, nil
	}
	selection := semanticRunnerSelection(job)
	if selected, _ := selection["selected_runner"].(string); selected != "" && selected != runnercodex.RunnerID {
		return "blocked", map[string]any{
			"outcome":          "blocked",
			"message":          "host-native semantic runner dispatch is declared but not implemented; no usable Codex fallback was selected",
			"runner_selection": selection,
		}, 0, nil
	}
	loop := targetString(job.Target, "loop")
	if loop == "" {
		loop = "eval"
	}
	jobSpec := job.JobSpecRef
	if jobSpec == "" {
		jobSpec = job.ReactorID
	}
	prompt := targetString(job.Target, "prompt")
	if prompt == "" {
		prompt = fmt.Sprintf("Run Mnemon semantic lifecycle job %s for loop %s. Return structured evidence only; do not modify canonical state.", jobSpec, loop)
	}
	maxTurns := d.codexMaxTurns()
	if jobBudget := budgetInt(job.Budget, "max_turns"); jobBudget > 0 && jobBudget < maxTurns {
		maxTurns = jobBudget
	}
	projectLoops := semanticProjectLoops(d.paths.Root, loop)
	result, err := runnercodex.Run(ctx, d.paths.Root, runnercodex.RunOptions{
		CheckOptions: runnercodex.CheckOptions{
			Command:          d.opts.CodexCommand,
			Args:             d.opts.CodexArgs,
			Env:              d.opts.CodexEnv,
			Timeout:          d.codexTimeout(),
			Now:              now,
			IsolateCodexHome: d.opts.CodexIsolatedHome,
			RunID:            fmt.Sprintf("%s-%s", now.UTC().Format("20060102T150405Z"), job.ID),
		},
		JobID:                job.ID,
		JobSpec:              jobSpec,
		Loop:                 loop,
		Prompt:               prompt,
		TurnTimeout:          d.codexTurnTimeout(),
		MaxTurns:             maxTurns,
		AllowRealTurn:        true,
		AcknowledgeModelCost: true,
		DeclarationRoot:      d.paths.Root,
		ProjectLoops:         projectLoops,
		WorkspaceEnv:         semanticWorkspaceEnv(loop, len(projectLoops) > 0),
	})
	if err != nil {
		return "failed", nil, 0, err
	}
	statusValue := "completed"
	if result.Status == runnercodex.StatusBlocked {
		statusValue = "blocked"
	} else if result.Status == runnercodex.StatusDegraded {
		statusValue = "failed"
	}
	jobResult := map[string]any{
		"outcome":          string(result.Status),
		"message":          result.Message,
		"runner_id":        runnercodex.RunnerID,
		"runner_selection": selection,
		"report_ref":       map[string]any{"uri": result.ReportPath},
		"thread_id":        result.ThreadID,
		"turn_count":       result.TurnCount,
		"last_event_id":    result.LastEventID,
	}
	if result.FailureClass != "" {
		jobResult["failure_class"] = string(result.FailureClass)
	}
	return statusValue, jobResult, result.TurnCount, nil
}

func semanticProjectLoops(root, loop string) []string {
	if loop == "" {
		return nil
	}
	if _, err := os.Stat(filepath.Join(root, "harness", "loops", loop, "loop.json")); err != nil {
		return nil
	}
	if _, err := os.Stat(filepath.Join(root, "harness", "bindings", "codex."+loop+".json")); err != nil {
		return nil
	}
	return []string{loop}
}

func semanticWorkspaceEnv(loop string, projected bool) func(runnercodex.WorkspaceContext) []string {
	if !projected || loop == "" {
		return nil
	}
	keyBase := strings.ToUpper(strings.ReplaceAll(loop, "-", "_"))
	return func(workspace runnercodex.WorkspaceContext) []string {
		loopDir := filepath.Join(workspace.MnemonDir, "harness", loop)
		return []string{
			"MNEMON_" + keyBase + "_LOOP_DIR=" + loopDir,
			"MNEMON_" + keyBase + "_LOOP_ENV=" + filepath.Join(loopDir, "env.sh"),
		}
	}
}

func (d *Daemon) dueJobs(now time.Time) ([]Job, error) {
	dir := filepath.Join(d.paths.JobsDir, "queued")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read queue: %w", err)
	}
	var jobs []Job
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var job Job
		if err := readJSON(filepath.Join(dir, entry.Name()), &job); err != nil {
			return jobs, err
		}
		if err := validateJob(job); err != nil {
			return jobs, err
		}
		dueAt, err := time.Parse(time.RFC3339, job.DueAt)
		if err != nil {
			return jobs, fmt.Errorf("job %s has invalid due_at: %w", job.ID, err)
		}
		if !dueAt.After(now.UTC()) {
			jobs = append(jobs, job)
		}
	}
	sort.Slice(jobs, func(i, j int) bool {
		if jobs[i].Priority == jobs[j].Priority {
			return jobs[i].ID < jobs[j].ID
		}
		return priorityRank(jobs[i].Priority) > priorityRank(jobs[j].Priority)
	})
	return jobs, nil
}

func (d *Daemon) finishJob(job Job, statusValue string, now time.Time, result map[string]any) error {
	job.Status = statusValue
	job.Result = result
	job.Lease = nil
	job.UpdatedAt = now.UTC().Format(time.RFC3339)
	source := d.jobPath("queued", job.ID)
	target := d.jobPath(statusValue, job.ID)
	if err := writeJSONAtomic(target, job); err != nil {
		return err
	}
	if err := os.Remove(source); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove queued job: %w", err)
	}
	return d.writeJobStatus(job, now)
}

func (d *Daemon) writeCheckpoint(now time.Time, lastEventID string) error {
	path := filepath.Join(d.paths.HarnessDir, "daemon", "checkpoint.json")
	return writeJSONAtomic(path, Checkpoint{
		SchemaVersion:        1,
		LastProcessedEventID: lastEventID,
		UpdatedAt:            now.UTC().Format(time.RFC3339),
	})
}

func (d *Daemon) writeDaemonStatus(now time.Time, tick TickResult, phase, reason, message string) error {
	depth, err := d.queueDepth()
	if err != nil {
		return err
	}
	if err := d.appendDaemonPhaseEvent(now, tick, phase, reason, message); err != nil {
		return err
	}
	status := map[string]any{
		"schema_version": 1,
		"kind":           "DaemonStatus",
		"metadata": map[string]any{
			"name":     "project-daemon",
			"owner_id": d.opts.OwnerID,
		},
		"status": map[string]any{
			"phase":                   phase,
			"last_refreshed_at":       now.UTC().Format(time.RFC3339),
			"last_processed_event_id": tick.LastProcessedEventID,
			"last_included_event_id":  tick.LastProcessedEventID,
			"queue_depth":             depth,
			"jobs_processed":          tick.JobsProcessed,
			"jobs_failed":             tick.JobsFailed,
			"jobs_blocked":            tick.JobsBlocked,
			"real_turn_budget": map[string]any{
				"default_max_turns": d.codexMaxTurns(),
				"used":              tick.RealTurnsUsed,
				"remaining":         max(0, d.codexMaxTurns()-tick.RealTurnsUsed),
			},
			"conditions": []schema.Condition{{
				Type:             conditionType(phase),
				Status:           "true",
				Reason:           reason,
				Message:          message,
				LastTransitionTS: now.UTC().Format(time.RFC3339),
				LastEventID:      tick.LastProcessedEventID,
			}},
		},
	}
	return writeJSONAtomic(filepath.Join(d.paths.StatusDir, "daemon.json"), status)
}

func (d *Daemon) appendDaemonPhaseEvent(now time.Time, tick TickResult, phase, reason, message string) error {
	previous, _, err := d.lastDaemonPhase()
	if err != nil {
		return err
	}
	if previous == phase {
		return nil
	}
	store, err := eventlog.New(d.paths.Root)
	if err != nil {
		return err
	}
	event := schema.Event{
		SchemaVersion: schema.Version,
		ID:            fmt.Sprintf("evt_daemon_%s_%d", cleanEventToken(reason), now.UTC().UnixNano()),
		TS:            now.UTC().Format(time.RFC3339),
		Type:          daemonEventType(phase, reason),
		Actor:         "mnemon-daemon",
		Source:        "daemon",
		CorrelationID: "daemon:" + d.opts.OwnerID,
		Payload: map[string]any{
			"from_phase":              previous,
			"to_phase":                phase,
			"reason":                  reason,
			"message":                 message,
			"last_processed_event_id": tick.LastProcessedEventID,
			"event_count":             tick.EventCount,
			"jobs_processed":          tick.JobsProcessed,
			"jobs_failed":             tick.JobsFailed,
			"jobs_blocked":            tick.JobsBlocked,
			"real_turns_used":         tick.RealTurnsUsed,
		},
	}
	return store.Append(event)
}

func (d *Daemon) lastDaemonPhase() (string, string, error) {
	store, err := eventlog.New(d.paths.Root)
	if err != nil {
		return "", "", err
	}
	events, err := store.ReadAll()
	if err != nil {
		return "", "", err
	}
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		if !strings.HasPrefix(event.Type, "daemon.") {
			continue
		}
		phase, _ := event.Payload["to_phase"].(string)
		if phase != "" {
			return phase, event.ID, nil
		}
	}
	var status struct {
		Status struct {
			Phase string `json:"phase"`
		} `json:"status"`
	}
	if err := readJSON(filepath.Join(d.paths.StatusDir, "daemon.json"), &status); err == nil {
		return status.Status.Phase, "", nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", err
	}
	return "", "", nil
}

func (d *Daemon) appendTickLog(record TickLogRecord) error {
	path := filepath.Join(d.paths.HarnessDir, "daemon", "tick-log.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

func (d *Daemon) writeJobStatus(job Job, now time.Time) error {
	phase := job.Status
	if phase == "completed" {
		phase = "ready"
	}
	status := map[string]any{
		"schema_version": 1,
		"kind":           "JobStatus",
		"metadata": map[string]any{
			"name": job.ID,
			"job":  job.ID,
		},
		"status": map[string]any{
			"phase":                  phase,
			"last_refreshed_at":      now.UTC().Format(time.RFC3339),
			"last_included_event_id": lastEvidenceRef(job),
			"attempts":               job.Attempts,
			"conditions": []schema.Condition{{
				Type:             conditionType(phase),
				Status:           "true",
				Reason:           "Job" + titleStatus(job.Status),
				LastTransitionTS: now.UTC().Format(time.RFC3339),
				LastEventID:      lastEvidenceRef(job),
			}},
		},
	}
	return writeJSONAtomic(filepath.Join(d.paths.StatusDir, "jobs", job.ID+".json"), status)
}

func (d *Daemon) queueDepth() (QueueDepth, error) {
	var depth QueueDepth
	statusDirs := map[string]*int{
		"queued":    &depth.Queued,
		"completed": &depth.Completed,
		"failed":    &depth.Failed,
		"blocked":   &depth.Blocked,
		"skipped":   &depth.Skipped,
	}
	for name, target := range statusDirs {
		count, err := countJSONFiles(filepath.Join(d.paths.JobsDir, name))
		if err != nil {
			return depth, err
		}
		*target = count
	}
	queuedJobs, err := d.dueAndFutureQueuedJobs()
	if err != nil {
		return depth, err
	}
	for _, job := range queuedJobs {
		if job.Status == "leased" {
			depth.Leased++
			depth.Queued--
		}
	}
	return depth, nil
}

func (d *Daemon) dueAndFutureQueuedJobs() ([]Job, error) {
	dir := filepath.Join(d.paths.JobsDir, "queued")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var jobs []Job
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var job Job
		if err := readJSON(filepath.Join(dir, entry.Name()), &job); err != nil {
			return jobs, err
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func (d *Daemon) jobPath(statusValue, jobID string) string {
	return filepath.Join(d.paths.JobsDir, statusValue, jobID+".json")
}

func validateJob(job Job) error {
	if job.SchemaVersion != JobSchemaVersion {
		return fmt.Errorf("job schema_version must be %s", JobSchemaVersion)
	}
	if job.ID == "" {
		return errors.New("job id is required")
	}
	if job.Type != "deterministic" && job.Type != "semantic" && job.Type != "cli" && job.Type != "spawn_runner" {
		return errors.New("job type must be deterministic, semantic, cli, or spawn_runner")
	}
	if job.ReactorID == "" {
		return errors.New("job reactor_id is required")
	}
	if job.Target == nil {
		return errors.New("job target is required")
	}
	if job.Priority == "" {
		return errors.New("job priority is required")
	}
	if job.Status == "" {
		return errors.New("job status is required")
	}
	if _, err := time.Parse(time.RFC3339, job.DueAt); err != nil {
		return fmt.Errorf("job due_at must be RFC3339: %w", err)
	}
	if job.MaxAttempts <= 0 {
		return errors.New("job max_attempts must be positive")
	}
	if job.CorrelationID == "" {
		return errors.New("job correlation_id is required")
	}
	return nil
}

func withProjectLock(paths layout.Paths, owner string, now time.Time, fn func() error) error {
	lock := filepath.Join(paths.HarnessDir, "daemon", "daemon.lock")
	if err := os.MkdirAll(filepath.Dir(lock), 0o755); err != nil {
		return err
	}
	info := projectLockInfo{
		SchemaVersion: 1,
		OwnerID:       owner,
		PID:           os.Getpid(),
		AcquiredAt:    now.UTC().Format(time.RFC3339),
		Token:         fmt.Sprintf("%s:%d:%d", owner, os.Getpid(), now.UTC().UnixNano()),
	}
	for attempt := 0; attempt < 2; attempt++ {
		file, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			data, marshalErr := json.Marshal(info)
			if marshalErr != nil {
				_ = file.Close()
				_ = os.Remove(lock)
				return fmt.Errorf("marshal daemon lock: %w", marshalErr)
			}
			if _, err := file.Write(append(data, '\n')); err != nil {
				_ = file.Close()
				_ = os.Remove(lock)
				return fmt.Errorf("write daemon lock: %w", err)
			}
			_ = file.Close()
			defer removeProjectLock(lock, info.Token)
			return fn()
		}
		if !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("create daemon lock: %w", err)
		}
		existing, readErr := readProjectLock(lock)
		if readErr == nil && staleProjectLock(existing) {
			if removeErr := removeProjectLock(lock, existing.Token); removeErr != nil {
				return fmt.Errorf("remove stale daemon lock: %w", removeErr)
			}
			continue
		}
		if readErr != nil {
			return fmt.Errorf("daemon lock already held; read lock: %w", readErr)
		}
		if existing.PID > 0 {
			return fmt.Errorf("daemon lock already held by pid %d owner %s", existing.PID, existing.OwnerID)
		}
		return fmt.Errorf("daemon lock already held")
	}
	return fmt.Errorf("daemon lock already held")
}

func readProjectLock(path string) (projectLockInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return projectLockInfo{}, err
	}
	var info projectLockInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return projectLockInfo{}, err
	}
	return info, nil
}

func staleProjectLock(info projectLockInfo) bool {
	return info.PID > 0 && !processAlive(info.PID)
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}

func removeProjectLock(path, token string) error {
	if token != "" {
		info, err := readProjectLock(path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.Token != token {
			return nil
		}
	}
	return os.Remove(path)
}

func leaseExpired(lease Lease, now time.Time) bool {
	expires, err := time.Parse(time.RFC3339, lease.ExpiresAt)
	if err != nil {
		return true
	}
	return !expires.After(now.UTC())
}

func priorityRank(priority string) int {
	switch priority {
	case "critical":
		return 4
	case "high":
		return 3
	case "normal":
		return 2
	default:
		return 1
	}
}

func conditionType(phase string) string {
	switch phase {
	case "blocked":
		return "Blocked"
	case "failed", "degraded":
		return "Degraded"
	case "paused":
		return "Paused"
	default:
		return "Ready"
	}
}

func daemonEventType(phase, reason string) string {
	switch reason {
	case "EventReplayFailed":
		return "daemon.replay_failed"
	case "LockFailed":
		return "daemon.lock_failed"
	case "BudgetExhausted":
		return "daemon.budget_exhausted"
	}
	if phase == "degraded" {
		return "daemon.degraded"
	}
	return "daemon.phase_changed"
}

func daemonTickID(now time.Time) string {
	return fmt.Sprintf("tick-%s-%d", now.UTC().Format("20060102T150405Z"), now.UTC().UnixNano())
}

func tickLogRecord(tickID, status string, now time.Time, owner string, result TickResult, reason, message string) TickLogRecord {
	return TickLogRecord{
		SchemaVersion:        1,
		TickID:               tickID,
		Status:               status,
		TS:                   now.UTC().Format(time.RFC3339),
		OwnerID:              owner,
		LastProcessedEventID: result.LastProcessedEventID,
		EventCount:           result.EventCount,
		StatusFilesWritten:   result.StatusFilesWritten,
		JobsProcessed:        result.JobsProcessed,
		JobsFailed:           result.JobsFailed,
		JobsBlocked:          result.JobsBlocked,
		RealTurnsUsed:        result.RealTurnsUsed,
		Reason:               reason,
		Message:              message,
	}
}

func cleanEventToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "phase"
	}
	value = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		case r >= '0' && r <= '9':
			return r
		case r == '_' || r == '-' || r == '.':
			return r
		default:
			return '_'
		}
	}, value)
	return strings.Trim(value, "_.-")
}

func titleStatus(statusValue string) string {
	if statusValue == "" {
		return "Unknown"
	}
	return string(statusValue[0]-32) + statusValue[1:]
}

func lastEvidenceRef(job Job) string {
	if job.Result != nil {
		if lastEventID, ok := job.Result["last_event_id"].(string); ok && lastEventID != "" {
			return lastEventID
		}
	}
	if len(job.EvidenceRefs) == 0 {
		return ""
	}
	return job.EvidenceRefs[len(job.EvidenceRefs)-1]
}

func (d *Daemon) codexMaxTurns() int {
	if d.opts.CodexMaxTurns > 0 {
		return d.opts.CodexMaxTurns
	}
	return 3
}

func (d *Daemon) codexTimeout() time.Duration {
	if d.opts.CodexTimeout > 0 {
		return d.opts.CodexTimeout
	}
	return 5 * time.Minute
}

func (d *Daemon) codexTurnTimeout() time.Duration {
	if d.opts.CodexTurnTimeout > 0 {
		return d.opts.CodexTurnTimeout
	}
	return 3 * time.Minute
}

func semanticRunnerSelection(job Job) map[string]any {
	mode := targetString(job.Target, "runner_mode")
	if mode == "" {
		mode = "app_server"
	}
	requestedRunner := targetString(job.Target, "runner_id")
	if requestedRunner == "" && mode == "app_server" {
		requestedRunner = runnercodex.RunnerID
	}
	if requestedRunner == "" && mode == "native_subagent" {
		host := targetString(job.Target, "host")
		agent := targetString(job.Target, "agent")
		if host != "" && agent != "" {
			requestedRunner = host + ":" + agent
		}
	}
	fallbackRunner := targetString(job.Target, "fallback_runner")
	selectedRunner := requestedRunner
	degraded := false
	if mode == "native_subagent" && fallbackRunner == runnercodex.RunnerID {
		selectedRunner = runnercodex.RunnerID
		degraded = true
	}
	if selectedRunner == "" {
		selectedRunner = runnercodex.RunnerID
	}
	return map[string]any{
		"mode":             mode,
		"requested_runner": requestedRunner,
		"selected_runner":  selectedRunner,
		"fallback_runner":  fallbackRunner,
		"degraded":         degraded,
	}
}

func targetString(target map[string]any, key string) string {
	value, ok := target[key]
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return text
}

func targetStringMap(target map[string]any, key string) map[string]string {
	value, ok := target[key]
	if !ok {
		return nil
	}
	typed, ok := value.(map[string]string)
	if ok {
		return typed
	}
	generic, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	result := map[string]string{}
	for key, value := range generic {
		result[key] = fmt.Sprint(value)
	}
	return result
}

func budgetInt(budget map[string]any, key string) int {
	value, ok := budget[key]
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		item, _ := typed.Int64()
		return int(item)
	default:
		return 0
	}
}

func max(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func countJSONFiles(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	var count int
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			count++
		}
	}
	return count, nil
}

func readJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, value); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func writeJSONAtomic(path string, value any) error {
	return layout.WriteJSONAtomic(path, value, 0o600)
}
