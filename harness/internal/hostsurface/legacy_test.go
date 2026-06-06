package hostsurface

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunLegacyProjectorInvokesProjectorInProjectRoot(t *testing.T) {
	root := t.TempDir()
	projectRoot := t.TempDir()
	logPath := filepath.Join(root, "projector.log")
	writeLegacyProjectorFixture(t, root, logPath, `{
  "schema_version": 1,
  "name": "codex.memory",
  "host": "codex",
  "loop": "memory",
  "projection_path": ".codex",
  "runtime_surface": ".codex/mnemon-memory",
  "lifecycle_mapping": {},
  "reconcile": []
}`)

	err := RunLegacyProjector(context.Background(), "install", LegacyOptions{
		DeclarationRoot: root,
		ProjectRoot:     projectRoot,
		Host:            "codex",
		Loops:           []string{"memory"},
		HostArgs:        []string{"--config-dir", ".codex-test"},
	})
	if err != nil {
		t.Fatalf("RunLegacyProjector returned error: %v", err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, projectRoot+"|install --loop memory --config-dir .codex-test") {
		t.Fatalf("unexpected projector log: %s", got)
	}
}

func TestRunLegacyProjectorStatusDefaultsToBoundLoops(t *testing.T) {
	root := t.TempDir()
	projectRoot := t.TempDir()
	logPath := filepath.Join(root, "projector.log")
	writeLegacyProjectorFixture(t, root, logPath, `{
  "schema_version": 1,
  "name": "codex.memory",
  "host": "codex",
  "loop": "memory",
  "projection_path": ".codex",
  "runtime_surface": ".codex/mnemon-memory",
  "lifecycle_mapping": {},
  "reconcile": []
}`)
	writeFile(t, filepath.Join(root, "harness", "bindings", "codex.goal.json"), `{
  "schema_version": 1,
  "name": "codex.goal",
  "host": "codex",
  "loop": "goal",
  "projection_path": ".codex",
  "runtime_surface": ".codex/mnemon-goal",
  "lifecycle_mapping": {},
  "reconcile": []
}`)

	err := RunLegacyProjector(context.Background(), "status", LegacyOptions{
		DeclarationRoot: root,
		ProjectRoot:     projectRoot,
		Host:            "codex",
	})
	if err != nil {
		t.Fatalf("RunLegacyProjector returned error: %v", err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "status --loop goal") || !strings.Contains(got, "status --loop memory") {
		t.Fatalf("expected status calls for bound loops, got: %s", got)
	}
}

func writeLegacyProjectorFixture(t *testing.T, root, logPath, binding string) {
	t.Helper()
	projectorDir := filepath.Join(root, "harness", "hosts", "codex")
	bindingsDir := filepath.Join(root, "harness", "bindings")
	for _, dir := range []string{projectorDir, bindingsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	script := "#!/usr/bin/env bash\nprintf '%s|%s\\n' \"$PWD\" \"$*\" >> " + shellQuote(logPath) + "\n"
	projector := filepath.Join(projectorDir, "projector.sh")
	if err := os.WriteFile(projector, []byte(script), 0o755); err != nil {
		t.Fatalf("write projector: %v", err)
	}
	writeFile(t, filepath.Join(bindingsDir, "codex.memory.json"), binding)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
