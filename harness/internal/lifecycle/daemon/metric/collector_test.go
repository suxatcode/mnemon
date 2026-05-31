package metric

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultRegistryCollectsFileMetrics(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "harness", "loops", "memory", "MEMORY.md"), "one\ntwo\n")
	writeFile(t, filepath.Join(root, ".mnemon", "events.jsonl"), "{}\n")
	writeFile(t, filepath.Join(root, ".mnemon", "harness", "jobs", "queued", "job.json"), "{}")

	registry := DefaultRegistry()
	input := Context{Root: root, Now: time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC), BudgetUsedUSDToday: 0.75}
	assertMetric(t, registry, "memory.lines", input, 2)
	assertMetric(t, registry, "memory.entries", input, 2)
	assertMetric(t, registry, "daemon.queue.depth", input, 1)
	assertMetric(t, registry, "daemon.budget.used_usd_today", input, 0.75)
}

func assertMetric(t *testing.T, registry Registry, name string, input Context, want float64) {
	t.Helper()
	got, err := registry[name].Collect(context.Background(), input)
	if err != nil {
		t.Fatalf("Collect(%s) returned error: %v", name, err)
	}
	if got != want {
		t.Fatalf("Collect(%s)=%v, want %v", name, got, want)
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
