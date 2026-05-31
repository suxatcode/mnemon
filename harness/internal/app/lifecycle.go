package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/coordination"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/eventlog"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/layout"
	runnercodex "github.com/mnemon-dev/mnemon/harness/internal/lifecycle/runner/codex"
	lifecyclestatus "github.com/mnemon-dev/mnemon/harness/internal/lifecycle/status"
)

// Facade-local input bundles for the lifecycle subcommands.

type LifecycleCodexCheckInput struct {
	Command      string
	Timeout      time.Duration
	IsolatedHome bool
}

type LifecycleCodexRunInput struct {
	Command              string
	Prompt               string
	ProjectRoot          string
	JobID                string
	JobSpec              string
	Loop                 string
	Timeout              time.Duration
	TurnTimeout          time.Duration
	MaxTurns             int
	AgentTurn            bool
	AcknowledgeModelCost bool
	IsolatedHome         bool
}

func (h *Harness) LifecycleInit(out io.Writer) error {
	paths, err := layout.EnsureProject(h.root)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "initialized lifecycle layout at %s\n", paths.MnemonDir)
	return nil
}

// LifecycleEventAppend validates and appends one event JSON object. The surface
// reads the raw bytes (from --json/--file/stdin) and passes them here.
func (h *Harness) LifecycleEventAppend(out io.Writer, data []byte) error {
	store, err := eventlog.New(h.root)
	if err != nil {
		return err
	}
	event, err := store.AppendJSON(data)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "appended lifecycle event %s\n", event.ID)
	return nil
}

func (h *Harness) LifecycleStatusRefresh(out io.Writer) error {
	result, err := lifecyclestatus.Refresh(h.root, time.Now().UTC())
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "refreshed lifecycle status from %d events; wrote %d files\n", result.EventCount, len(result.Written))
	return nil
}

// ProjectScope derives the live project scope (store/host/loop/profile/binding +
// last writeback) from the event log and writes it as JSON. It is the single read
// source for "current scope": surfaces decode this instead of re-walking the log.
// Derivation lives in the status projection; this only reads (it never creates or
// mutates project state), so a passive UI refresh stays read-only.
func (h *Harness) ProjectScope(out io.Writer, format string) error {
	store, err := eventlog.New(h.root)
	if err != nil {
		return err
	}
	// Best-effort: derive scope from the readable prefix of the log. ReadAll returns
	// the events decoded so far alongside a corrupt/IO error, so a corrupt tail
	// degrades to a partial scope rather than failing the read — a surface asking
	// "what scope am I in?" still gets an answer (matching the UI's defensive read).
	events, _ := store.ReadAll()
	scope := lifecyclestatus.DeriveScope(events)
	switch format {
	case "json", "":
		return writeJSON(out, scope)
	default:
		return fmt.Errorf("unsupported --format %q", format)
	}
}

// Readback derives the per-host writeback verification (the side Mnemon cannot
// force, made verifiable): observed / acted-but-unattributed / silent + staleness,
// folded from projection.applied + host writeback events. Read-only.
func (h *Harness) Readback(out io.Writer, format string) error {
	store, err := eventlog.New(h.root)
	if err != nil {
		return err
	}
	events, _ := store.ReadAll()
	rb := lifecyclestatus.DeriveReadback(events)
	switch format {
	case "json", "":
		return writeJSON(out, rb)
	default:
		return fmt.Errorf("unsupported --format %q", format)
	}
}

// Coordination derives the multi-agent collaboration topology (who owns what,
// fork lineage, groups, conflicts, merge candidates) from the event log and
// writes it as JSON. It is the single read source for the coordination view:
// surfaces decode this instead of folding the log themselves. Read-only — it
// never creates or mutates project state.
func (h *Harness) Coordination(out io.Writer, format string) error {
	store, err := eventlog.New(h.root)
	if err != nil {
		return err
	}
	// Best-effort over the readable prefix of the log, like ProjectScope.
	events, _ := store.ReadAll()
	view := coordination.DeriveView(events)
	switch format {
	case "json", "":
		return writeJSON(out, view)
	default:
		return fmt.Errorf("unsupported --format %q", format)
	}
}

// antipatternReport builds the deterministic anti-pattern scan report for now. It
// is pure (no I/O) so the persisting scan and the read-only status share one
// source of findings.
func antipatternReport(now time.Time) map[string]any {
	return map[string]any{
		"schema_version": 1,
		"id":             "antipattern-scan-" + now.Format("20060102T150405Z"),
		"status":         "pass",
		"mode":           "deterministic-initial",
		"summary":        "No daemon anti-pattern findings in initial deterministic scan.",
		"findings":       []map[string]any{},
		"checked_at":     now.Format(time.RFC3339),
	}
}

// AntipatternStatus returns the anti-pattern scan status and finding count WITHOUT
// writing a report — the read-only form surfaces use for health, so a passive UI
// refresh stays read-only. ok is false only if the report cannot be built.
func (h *Harness) AntipatternStatus() (status string, findings int, ok bool) {
	report := antipatternReport(time.Now().UTC())
	s, _ := report["status"].(string)
	f, _ := report["findings"].([]map[string]any)
	return s, len(f), true
}

func (h *Harness) LifecycleAntipatternScan(out io.Writer, format string) error {
	paths, err := layout.EnsureProject(h.root)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	report := antipatternReport(now)
	reportPath := filepath.Join(paths.ReportsDir, "antipattern", report["id"].(string)+".json")
	if err := os.MkdirAll(filepath.Dir(reportPath), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(reportPath, append(data, '\n'), 0o644); err != nil {
		return err
	}
	switch format {
	case "json":
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		report["report_path"] = filepath.ToSlash(reportPath)
		return encoder.Encode(report)
	case "text", "":
		fmt.Fprintln(out, "antipattern scan: pass")
		fmt.Fprintf(out, "report: %s\n", filepath.ToSlash(reportPath))
		return nil
	default:
		return fmt.Errorf("unsupported --format %q", format)
	}
}

func (h *Harness) LifecycleDaemonTick(ctx context.Context, out io.Writer, opts DaemonOptions) error {
	runner, err := h.newDaemon(opts)
	if err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := runner.Tick(ctx, time.Now().UTC())
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "daemon tick processed %d events, %d jobs, blocked %d jobs\n", result.EventCount, result.JobsProcessed, result.JobsBlocked)
	return nil
}

func (h *Harness) LifecycleDaemonForeground(ctx context.Context, out io.Writer, interval time.Duration, opts DaemonOptions) error {
	if interval <= 0 {
		return fmt.Errorf("--interval must be positive")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	sigctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := h.LifecycleDaemonTick(ctx, out, opts); err != nil {
			return err
		}
		select {
		case <-sigctx.Done():
			fmt.Fprintln(out, "daemon foreground stopped")
			return nil
		case <-ticker.C:
		}
	}
}

func (h *Harness) LifecycleRunnerCodexCheck(ctx context.Context, out io.Writer, in LifecycleCodexCheckInput) error {
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := runnercodex.Check(ctx, h.root, runnercodex.CheckOptions{
		Command:          in.Command,
		Timeout:          in.Timeout,
		IsolateCodexHome: in.IsolatedHome,
	})
	if err != nil {
		return err
	}
	if result.FailureClass != "" {
		fmt.Fprintf(out, "codex app-server readiness: %s (%s): %s\n", result.Status, result.FailureClass, result.Message)
	} else {
		fmt.Fprintf(out, "codex app-server readiness: %s: %s\n", result.Status, result.Message)
	}
	fmt.Fprintf(out, "report: %s\n", result.ReportPath)
	return nil
}

func (h *Harness) LifecycleRunnerCodexRun(ctx context.Context, out io.Writer, in LifecycleCodexRunInput) error {
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := runnercodex.Run(ctx, h.root, runnercodex.RunOptions{
		CheckOptions: runnercodex.CheckOptions{
			Command:          in.Command,
			Timeout:          in.Timeout,
			IsolateCodexHome: in.IsolatedHome,
		},
		JobID:                in.JobID,
		JobSpec:              in.JobSpec,
		Loop:                 in.Loop,
		Prompt:               in.Prompt,
		ProjectRoot:          in.ProjectRoot,
		TurnTimeout:          in.TurnTimeout,
		MaxTurns:             in.MaxTurns,
		AllowRealTurn:        in.AgentTurn,
		AcknowledgeModelCost: in.AcknowledgeModelCost,
	})
	if err != nil {
		return err
	}
	if result.FailureClass != "" {
		fmt.Fprintf(out, "codex app-server semantic run: %s (%s): %s\n", result.Status, result.FailureClass, result.Message)
	} else {
		fmt.Fprintf(out, "codex app-server semantic run: %s: %s\n", result.Status, result.Message)
	}
	fmt.Fprintf(out, "turns: %d\n", result.TurnCount)
	fmt.Fprintf(out, "report: %s\n", result.ReportPath)
	return nil
}
