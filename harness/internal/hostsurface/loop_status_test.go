package hostsurface

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeLoopStatus is hoisted onto projectorCore: both hosts record their own host id and the ACTUAL
// projection target (paths.configDir — the codex shape; binding.ProjectionPath is the declared
// default and goes stale under a custom --config-dir). Pin the claude shape post-hoist.
func TestClaudeLoopStatusRecordsHostAndActualProjectionPath(t *testing.T) {
	dir := t.TempDir()
	if err := RunClaudeProjector(context.Background(), "install", ClaudeOptions{
		ProjectRoot: dir,
		Loops:       []string{"memory"},
	}); err != nil {
		t.Fatalf("install: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".mnemon", "harness", "memory", "status.json"))
	if err != nil {
		t.Fatal(err)
	}
	var status struct {
		Host           string `json:"host"`
		ProjectionPath string `json:"projection_path"`
	}
	if err := json.Unmarshal(raw, &status); err != nil {
		t.Fatal(err)
	}
	if status.Host != "claude-code" {
		t.Fatalf("host = %q, want claude-code", status.Host)
	}
	if status.ProjectionPath != ".claude" {
		t.Fatalf("projection_path = %q, want .claude (the actual projection target)", status.ProjectionPath)
	}
}
