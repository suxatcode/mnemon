package loader

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReadsExplicitJobsAndGlobalBudget(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "harness", "daemon-jobs", "_global.yaml"), "global_budget:\n  daily_cost_usd: 1.00\n  daily_real_turns: 10\n  enabled: true\n")
	writeFile(t, filepath.Join(root, "harness", "daemon-jobs", "echo.yaml"), "id: test.echo\nwhen:\n  event: test.observed\ndo:\n  cli: \"echo hello\"\nbudget:\n  cost_usd: 0\n  max_sec: 5\n")

	catalog, err := Load(root, Options{})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(catalog.Jobs) != 1 {
		t.Fatalf("expected one job, got %#v", catalog.Jobs)
	}
	if catalog.Jobs[0].ID != "test.echo" || catalog.Jobs[0].Do.CLI == "" || !catalog.Jobs[0].IsEnabled() {
		t.Fatalf("unexpected job: %#v", catalog.Jobs[0])
	}
	if catalog.GlobalBudget.DailyCostUSD == nil || *catalog.GlobalBudget.DailyCostUSD != 1 {
		t.Fatalf("global budget not loaded: %#v", catalog.GlobalBudget)
	}
}

func TestLoadDisablesSpawnRunnerWithoutCostAcknowledgement(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "harness", "daemon-jobs", "spawn.yaml"), "id: test.spawn\nwhen:\n  event: signal.observed\ndo:\n  spawn_runner: codex\n  prompt: hi\n")

	catalog, err := Load(root, Options{})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(catalog.Jobs) != 1 || catalog.Jobs[0].IsEnabled() {
		t.Fatalf("spawn_runner should be disabled without cost acknowledgement: %#v", catalog.Jobs)
	}
	if len(catalog.Warnings) == 0 {
		t.Fatalf("expected warning for disabled spawn runner")
	}

	acknowledged, err := Load(root, Options{AcknowledgeModelCost: true})
	if err != nil {
		t.Fatalf("Load with acknowledgement returned error: %v", err)
	}
	if !acknowledged.Jobs[0].IsEnabled() {
		t.Fatalf("spawn_runner should stay enabled with cost acknowledgement: %#v", acknowledged.Jobs[0])
	}
}

func TestLoadLiftsLoopControllers(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "harness", "loops", "memory", "loop.json"), `{
  "schema_version": 2,
  "name": "memory",
  "surfaces": {"projection": [], "observation": []},
  "assets": {"guide": "", "env": "", "hook_prompts": {}, "skills": [], "subagents": []},
  "host_adapters": {},
  "controllers": [{"name": "memory.dreaming.on_hot_write", "watches": ["memory.hot_write_observed"], "enqueue": "memory.dreaming", "reason": "hot memory"}],
  "jobs": {"memory.dreaming": {"type": "semantic", "max_turns": 3}}
}`)

	catalog, err := Load(root, Options{})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(catalog.Jobs) != 1 {
		t.Fatalf("expected lifted job, got %#v", catalog.Jobs)
	}
	job := catalog.Jobs[0]
	if job.ID != "memory.dreaming.on_hot_write" || job.When.Event != "memory.hot_write_observed" || job.Do.Subagent != "memory.dreaming" || job.Budget.MaxTurns != 3 {
		t.Fatalf("unexpected lifted job: %#v", job)
	}
}

func TestLoadValidatesTriggerAndActionRules(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "harness", "daemon-jobs", "bad.yaml"), "id: bad job\nwhen:\n  threshold: {metric: missing.metric, op: \">\", value: 1}\ndo:\n  cli: echo\n")

	if _, err := Load(root, Options{}); err == nil {
		t.Fatalf("expected invalid job to fail")
	}
}

func TestLoadValidationCoversSchemaRules(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "missing-trigger",
			body: "id: missing.trigger\nwhen: {}\ndo:\n  cli: echo\n",
		},
		{
			name: "multiple-actions",
			body: "id: multiple.actions\nwhen:\n  event: test\ndo:\n  cli: echo\n  subagent: memory.dreaming\n",
		},
		{
			name: "invalid-cron",
			body: "id: invalid.cron\nwhen:\n  cron: \"0 3 *\"\ndo:\n  cli: echo\n",
		},
		{
			name: "invalid-interval",
			body: "id: invalid.interval\nwhen:\n  interval: nope\ndo:\n  cli: echo\n",
		},
		{
			name: "invalid-threshold-op",
			body: "id: invalid.threshold\nwhen:\n  threshold: {metric: memory.lines, op: contains, value: 1}\ndo:\n  cli: echo\n",
		},
		{
			name: "composite-depth",
			body: "id: invalid.depth\nwhen:\n  any:\n    - any:\n        - any:\n            - any:\n                - event: too.deep\ndo:\n  cli: echo\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, filepath.Join(root, "harness", "daemon-jobs", "bad.yaml"), tt.body)
			if _, err := Load(root, Options{}); err == nil {
				t.Fatalf("expected invalid job to fail")
			}
		})
	}
}

func TestLoadRejectsDuplicateExplicitIDs(t *testing.T) {
	root := t.TempDir()
	body := "id: duplicate.id\nwhen:\n  event: test\ndo:\n  cli: echo\n"
	writeFile(t, filepath.Join(root, "harness", "daemon-jobs", "one.yaml"), body)
	writeFile(t, filepath.Join(root, "harness", "daemon-jobs", "two.yaml"), body)
	if _, err := Load(root, Options{}); err == nil {
		t.Fatalf("expected duplicate id to fail")
	}
}

func TestLoadWarnsWhenJobBudgetExceedsGlobalBudget(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "harness", "daemon-jobs", "_global.yaml"), "global_budget:\n  daily_cost_usd: 0.10\n  enabled: true\n")
	writeFile(t, filepath.Join(root, "harness", "daemon-jobs", "cost.yaml"), "id: cost.warn\nwhen:\n  event: test\ndo:\n  cli: echo\nbudget:\n  cost_usd: 0.25\n")
	catalog, err := Load(root, Options{})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(catalog.Warnings) == 0 {
		t.Fatalf("expected budget warning")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
