package hostsurface

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --dry-run must be accepted by the claude-code projector and write nothing (the codex projector has
// supported it since the diff engine landed; claude previously hard-failed on the option).
func TestClaudeProjectorDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	err := RunClaudeProjector(context.Background(), "install", ClaudeOptions{
		ProjectRoot: dir,
		Loops:       []string{"memory"},
		HostArgs:    []string{"--dry-run"},
		Stdout:      &out,
	})
	if err != nil {
		t.Fatalf("dry-run must be accepted: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, ".claude")); !os.IsNotExist(statErr) {
		t.Fatal("dry-run must not create the projection surface")
	}
	if _, statErr := os.Stat(filepath.Join(dir, ".mnemon")); !os.IsNotExist(statErr) {
		t.Fatal("dry-run must not create harness state")
	}
	if !strings.Contains(out.String(), "dry-run") {
		t.Fatalf("dry-run must report itself, got: %q", out.String())
	}
}

func TestClaudeProjectorReportDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	rep, err := RunClaudeProjectorReport(context.Background(), ClaudeOptions{
		ProjectRoot: dir,
		Loops:       []string{"memory"},
		HostArgs:    []string{"--dry-run"},
	})
	if err != nil {
		t.Fatalf("dry-run must be accepted: %v", err)
	}
	if len(rep.Conflicts) != 0 {
		t.Fatalf("dry-run report must be empty, got %v", rep.Conflicts)
	}
	if _, statErr := os.Stat(filepath.Join(dir, ".claude")); !os.IsNotExist(statErr) {
		t.Fatal("dry-run must not create the projection surface")
	}
}
