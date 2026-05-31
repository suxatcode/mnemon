package job

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/daemon/loader"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/daemon/trigger"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
)

func TestMaterializeCLIJobFromEvent(t *testing.T) {
	cost := 0.0
	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	jobs, err := Materialize(loader.Definition{
		ID:     "goal.idle_nudge",
		Do:     loader.Action{CLI: "echo nudge"},
		Budget: loader.Budget{CostUSD: &cost, MaxSec: 5},
	}, trigger.Decision{Events: []schema.Event{{ID: "evt_1", Type: "goal.completed", CorrelationID: "goal:1"}}}, now)
	if err != nil {
		t.Fatalf("Materialize returned error: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Type != "cli" || jobs[0].Target["cli"] != "echo nudge" || jobs[0].CorrelationID != "goal:1" {
		t.Fatalf("unexpected runtime job: %#v", jobs)
	}
	if jobs[0].Budget["max_sec"] != 5 || jobs[0].Budget["max_turns"] != 3 {
		t.Fatalf("budget fallback mismatch: %#v", jobs[0].Budget)
	}
}

// Regression for the background re-enqueue flood: an event-less (cron/interval/
// threshold) job must produce a dedup-stable id within a minute so a persistently
// matching trigger does not enqueue once per distinct-second tick.
func TestMaterializeEventlessIDStableWithinMinute(t *testing.T) {
	def := loader.Definition{ID: "pool.budget.enforce", Do: loader.Action{CLI: "echo over-budget"}}
	within := time.Date(2026, 5, 29, 3, 0, 10, 0, time.UTC)
	sameMinute := time.Date(2026, 5, 29, 3, 0, 55, 0, time.UTC)
	nextMinute := time.Date(2026, 5, 29, 3, 1, 5, 0, time.UTC)

	first, err := Materialize(def, trigger.Decision{Matched: true}, within)
	if err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	again, err := Materialize(def, trigger.Decision{Matched: true}, sameMinute)
	if err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	later, err := Materialize(def, trigger.Decision{Matched: true}, nextMinute)
	if err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	if first[0].ID != again[0].ID {
		t.Fatalf("event-less job id must be stable within a minute: %q vs %q", first[0].ID, again[0].ID)
	}
	if first[0].ID == later[0].ID {
		t.Fatalf("event-less job id must differ across minutes, both %q", first[0].ID)
	}
}

func TestMaterializeSemanticAndSpawnRunnerJobs(t *testing.T) {
	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	semantic, err := Materialize(loader.Definition{
		ID:       "daemon.memory_dream",
		Do:       loader.Action{Subagent: "memory.dreaming", PromptOverride: "summarize"},
		Metadata: map[string]any{"loop": "memory"},
	}, trigger.Decision{Matched: true}, now)
	if err != nil {
		t.Fatalf("Materialize semantic returned error: %v", err)
	}
	if semantic[0].Type != "semantic" || semantic[0].ReactorID != "memory.dreaming" || semantic[0].Target["prompt"] != "summarize" || semantic[0].Target["loop"] != "memory" {
		t.Fatalf("unexpected semantic job: %#v", semantic[0])
	}
	inferred, err := Materialize(loader.Definition{
		ID: "eval.semantic_check",
		Do: loader.Action{Subagent: "eval.evaluator"},
	}, trigger.Decision{Matched: true}, now)
	if err != nil {
		t.Fatalf("Materialize inferred semantic returned error: %v", err)
	}
	if inferred[0].Target["loop"] != "eval" {
		t.Fatalf("expected semantic loop inferred from id, got %#v", inferred[0])
	}
	spawn, err := Materialize(loader.Definition{
		ID: "autoregress.signal",
		Do: loader.Action{SpawnRunner: "codex", Prompt: "materialize", MaxTurns: 2},
	}, trigger.Decision{Matched: true}, now)
	if err != nil {
		t.Fatalf("Materialize spawn returned error: %v", err)
	}
	if spawn[0].Type != "spawn_runner" || spawn[0].Target["runner_id"] != "codex" || spawn[0].Target["max_turns"] != 2 {
		t.Fatalf("unexpected spawn runner job: %#v", spawn[0])
	}
}

func TestExecuteCLI(t *testing.T) {
	result, err := ExecuteCLI(context.Background(), t.TempDir(), loader.Action{CLI: "printf hello"}, 5)
	if err != nil {
		t.Fatalf("ExecuteCLI returned error: %v", err)
	}
	if result.ExitCode != 0 || strings.TrimSpace(result.Stdout) != "hello" || result.Stderr != "" {
		t.Fatalf("unexpected CLI result: %#v", result)
	}
}
