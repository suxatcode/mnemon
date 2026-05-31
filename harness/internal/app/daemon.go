package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/daemon"
	daemonjob "github.com/mnemon-dev/mnemon/harness/internal/lifecycle/daemon/job"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/daemon/loader"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/daemon/metric"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/daemon/trigger"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/eventlog"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
)

// DaemonOptions carries the Codex/runner configuration for daemon dispatch,
// mirroring daemon.Options so the surface need not import the daemon package.
type DaemonOptions struct {
	EnableCodexSemanticRun bool
	AcknowledgeModelCost   bool
	CodexCommand           string
	CodexMaxTurns          int
	CodexTimeout           time.Duration
	CodexTurnTimeout       time.Duration
	CodexIsolatedHome      bool
}

// DaemonRun runs declarative daemon jobs once or in a background loop, streaming
// per-tick output to out and loader warnings to errw. It owns the tick loop,
// dry-run preview, and run-mode validation that previously lived in the surface.
func (h *Harness) DaemonRun(ctx context.Context, out, errw io.Writer, once, background, dryRun bool, interval time.Duration, opts DaemonOptions) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if once && background {
		return fmt.Errorf("--once and --background are mutually exclusive")
	}
	if !once && !background {
		once = true
	}
	if dryRun {
		return h.previewDaemonRun(ctx, out, errw, opts)
	}
	if catalog, cerr := loader.Load(h.root, loader.Options{AcknowledgeModelCost: opts.AcknowledgeModelCost}); cerr == nil {
		printDaemonWarnings(errw, catalog.Warnings)
	}
	if once {
		runner, err := h.newDaemon(opts)
		if err != nil {
			return err
		}
		result, err := runner.Tick(ctx, time.Now().UTC())
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "daemon tick processed %d events, %d jobs, blocked %d jobs\n", result.EventCount, result.JobsProcessed, result.JobsBlocked)
		return nil
	}
	if interval <= 0 {
		return fmt.Errorf("--interval must be positive")
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		runner, err := h.newDaemon(opts)
		if err != nil {
			return err
		}
		result, err := runner.Tick(ctx, time.Now().UTC())
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "daemon tick processed %d events, %d jobs, blocked %d jobs\n", result.EventCount, result.JobsProcessed, result.JobsBlocked)
		select {
		case <-ctx.Done():
			fmt.Fprintln(out, "daemon background stopped")
			return nil
		case <-ticker.C:
		}
	}
}

// DaemonTrigger evaluates or force-enqueues one declarative daemon job.
func (h *Harness) DaemonTrigger(out io.Writer, jobID string, force, dryRun bool, opts DaemonOptions) error {
	if !dryRun && !force {
		return fmt.Errorf("daemon trigger requires --dry-run or --force")
	}
	pause, err := daemon.IsPaused(h.root)
	if err != nil {
		return err
	}
	def, err := h.findDaemonDefinition(jobID, opts)
	if err != nil {
		return err
	}
	decision := trigger.Decision{Matched: true, Reason: "manual"}
	runtimes, err := daemonjob.Materialize(def, decision, time.Now().UTC())
	if err != nil {
		return err
	}
	if dryRun {
		for _, runtime := range runtimes {
			if pause.Paused {
				fmt.Fprintf(out, "would trigger %s type=%s action=%s but paused: %s\n", runtime.ID, runtime.Type, actionSummary(def), pause.Reason)
				continue
			}
			fmt.Fprintf(out, "would trigger %s type=%s action=%s\n", runtime.ID, runtime.Type, actionSummary(def))
		}
		return nil
	}
	if pause.Paused {
		return fmt.Errorf("daemon paused: %s", pause.Reason)
	}
	runner, err := h.newDaemon(opts)
	if err != nil {
		return err
	}
	for _, runtime := range runtimes {
		if err := runner.Enqueue(runtimeToDaemonJob(runtime)); err != nil {
			return err
		}
		fmt.Fprintf(out, "triggered %s\n", runtime.ID)
	}
	return nil
}

// DaemonStatus writes the daemon queue/tick/budget snapshot to out.
func (h *Harness) DaemonStatus(out io.Writer, limit int, asJSON bool) error {
	snapshot, err := daemon.Inspect(h.root, limit)
	if err != nil {
		return err
	}
	return writeDaemonStatusSnapshot(out, snapshot, asJSON)
}

// DaemonPause pauses daemon enqueueing without stopping existing jobs.
func (h *Harness) DaemonPause(out io.Writer, reason string) error {
	state, err := daemon.Pause(h.root, reason, time.Now().UTC())
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "daemon paused: %s\n", state.Reason)
	return nil
}

// DaemonResume resumes daemon enqueueing.
func (h *Harness) DaemonResume(out io.Writer) error {
	if _, err := daemon.Resume(h.root, time.Now().UTC()); err != nil {
		return err
	}
	fmt.Fprintln(out, "daemon resumed")
	return nil
}

func (h *Harness) previewDaemonRun(ctx context.Context, out, errw io.Writer, opts DaemonOptions) error {
	catalog, err := loader.Load(h.root, loader.Options{AcknowledgeModelCost: opts.AcknowledgeModelCost})
	if err != nil {
		return err
	}
	events, err := h.readDaemonEvents()
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "loaded %d daemon jobs\n", len(catalog.Jobs))
	printDaemonWarnings(errw, catalog.Warnings)
	for _, def := range catalog.Jobs {
		if !def.IsEnabled() {
			fmt.Fprintf(out, "disabled %s\n", def.ID)
			continue
		}
		decision, err := trigger.Evaluate(ctx, def.When, trigger.Input{
			Events: events,
			MetricContext: metric.Context{
				Root: h.root,
				Now:  time.Now().UTC(),
			},
		})
		if err != nil {
			return err
		}
		if decision.Matched {
			fmt.Fprintf(out, "would trigger %s reason=%s action=%s\n", def.ID, decision.Reason, actionSummary(def))
		}
	}
	return nil
}

func (h *Harness) findDaemonDefinition(id string, opts DaemonOptions) (loader.Definition, error) {
	catalog, err := loader.Load(h.root, loader.Options{AcknowledgeModelCost: opts.AcknowledgeModelCost})
	if err != nil {
		return loader.Definition{}, err
	}
	for _, def := range catalog.Jobs {
		if def.ID == id {
			return def, nil
		}
	}
	return loader.Definition{}, fmt.Errorf("daemon job %q not found", id)
}

func (h *Harness) newDaemon(opts DaemonOptions) (*daemon.Daemon, error) {
	return daemon.New(h.root, daemon.Options{
		EnableCodexSemanticRun: opts.EnableCodexSemanticRun,
		AcknowledgeModelCost:   opts.AcknowledgeModelCost,
		CodexCommand:           opts.CodexCommand,
		CodexMaxTurns:          opts.CodexMaxTurns,
		CodexTimeout:           opts.CodexTimeout,
		CodexTurnTimeout:       opts.CodexTurnTimeout,
		CodexIsolatedHome:      opts.CodexIsolatedHome,
	})
}

func (h *Harness) readDaemonEvents() ([]schema.Event, error) {
	store, err := eventlog.New(h.root)
	if err != nil {
		return nil, err
	}
	return store.ReadAll()
}

func printDaemonWarnings(errw io.Writer, warnings []string) {
	for _, w := range warnings {
		fmt.Fprintf(errw, "warning: %s\n", w)
	}
}

func runtimeToDaemonJob(runtime daemonjob.Runtime) daemon.Job {
	return daemon.Job{
		SchemaVersion: daemon.JobSchemaVersion,
		ID:            runtime.ID,
		Type:          runtime.Type,
		ReactorID:     runtime.ReactorID,
		JobSpecRef:    runtime.JobSpecRef,
		Target:        runtime.Target,
		Priority:      runtime.Priority,
		Status:        runtime.Status,
		DueAt:         runtime.DueAt,
		MaxAttempts:   runtime.MaxAttempts,
		Budget:        runtime.Budget,
		EvidenceRefs:  runtime.EvidenceRefs,
		CorrelationID: runtime.CorrelationID,
		UpdatedAt:     runtime.UpdatedAt,
	}
}

func actionSummary(def loader.Definition) string {
	switch {
	case def.Do.CLI != "":
		return "cli"
	case def.Do.Subagent != "":
		return "subagent:" + def.Do.Subagent
	case def.Do.SpawnRunner != "":
		return "spawn_runner:" + def.Do.SpawnRunner
	default:
		return "unknown"
	}
}

func writeDaemonStatusSnapshot(out io.Writer, snapshot daemon.StatusSnapshot, asJSON bool) error {
	if asJSON {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(snapshot)
	}
	state := "active"
	if snapshot.Paused.Paused {
		state = "paused"
	}
	fmt.Fprintf(out, "daemon status: %s\n", state)
	if snapshot.Paused.Paused {
		fmt.Fprintf(out, "pause reason: %s\n", snapshot.Paused.Reason)
	}
	fmt.Fprintf(out, "queue: queued=%d leased=%d blocked=%d failed=%d completed=%d skipped=%d\n",
		snapshot.QueueDepth.Queued,
		snapshot.QueueDepth.Leased,
		snapshot.QueueDepth.Blocked,
		snapshot.QueueDepth.Failed,
		snapshot.QueueDepth.Completed,
		snapshot.QueueDepth.Skipped,
	)
	costLimit := "unlimited"
	if snapshot.Budget.DailyCostUSD != nil {
		costLimit = fmt.Sprintf("%.4f", *snapshot.Budget.DailyCostUSD)
	}
	turnLimit := "unlimited"
	if snapshot.Budget.DailyRealTurns > 0 {
		turnLimit = fmt.Sprintf("%d", snapshot.Budget.DailyRealTurns)
	}
	fmt.Fprintf(out, "budget: cost=%.4f/%s real_turns=%d/%s\n", snapshot.Budget.UsedUSDToday, costLimit, snapshot.Budget.RealTurnsToday, turnLimit)
	fmt.Fprintf(out, "enabled jobs: %d\n", len(snapshot.EnabledJobs))
	for _, job := range snapshot.EnabledJobs {
		fmt.Fprintf(out, "- %s trigger=%s action=%s\n", job.ID, job.Trigger, job.Action)
	}
	fmt.Fprintf(out, "recent ticks: %d\n", len(snapshot.RecentTicks))
	for _, tick := range snapshot.RecentTicks {
		fmt.Fprintf(out, "- %s status=%s reason=%s events=%d jobs=%d failed=%d blocked=%d turns=%d\n",
			tick.TS,
			tick.Status,
			tick.Reason,
			tick.EventCount,
			tick.JobsProcessed,
			tick.JobsFailed,
			tick.JobsBlocked,
			tick.RealTurnsUsed,
		)
	}
	return nil
}
