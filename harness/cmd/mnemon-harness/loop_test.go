package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoopValidateCommand(t *testing.T) {
	root := t.TempDir()
	writeLoopValidateFixture(t, root)
	restoreLoopFlags(t)
	loopRoot = root

	cmd, output := testCommand()
	if err := runLoopValidate(cmd, nil); err != nil {
		t.Fatalf("runLoopValidate returned error: %v", err)
	}
	for _, want := range []string{"ok memory", "ok host codex", "ok binding codex.memory"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("expected %q in output:\n%s", want, output.String())
		}
	}
}

func restoreLoopFlags(t *testing.T) {
	t.Helper()
	oldRoot := loopRoot
	t.Cleanup(func() {
		loopRoot = oldRoot
	})
	loopRoot = "."
}

func writeLoopValidateFixture(t *testing.T, root string) {
	t.Helper()
	loopDir := filepath.Join(root, "harness", "loops", "memory")
	hostDir := filepath.Join(root, "harness", "hosts", "codex")
	bindingsDir := filepath.Join(root, "harness", "bindings")
	for _, dir := range []string{
		filepath.Join(loopDir, "skills", "memory-get"),
		hostDir,
		bindingsDir,
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	for _, path := range []string{
		filepath.Join(loopDir, "GUIDE.md"),
		filepath.Join(loopDir, "env.sh"),
		filepath.Join(loopDir, "MEMORY.md"),
		filepath.Join(loopDir, "skills", "memory-get", "SKILL.md"),
	} {
		writeLoopValidateFile(t, path, "fixture\n")
	}

	writeLoopValidateFile(t, filepath.Join(loopDir, "loop.json"), `{
  "schema_version": 2,
  "name": "memory",
  "surfaces": {
    "projection": [],
    "observation": []
  },
  "assets": {
    "guide": "GUIDE.md",
    "env": "env.sh",
    "runtime_files": ["MEMORY.md"],
    "skills": ["skills/memory-get/SKILL.md"],
    "subagents": []
  },
  "host_adapters": {
    "codex": "../../hosts/codex"
  }
}`)

	writeLoopValidateFile(t, filepath.Join(hostDir, "host.json"), `{
  "schema_version": 2,
  "name": "codex",
  "surfaces": {
    "projection": [],
    "observation": []
  },
  "lifecycle_mapping": {}
}`)

	writeLoopValidateFile(t, filepath.Join(bindingsDir, "codex.memory.json"), `{
  "schema_version": 1,
  "name": "codex.memory",
  "host": "codex",
  "loop": "memory",
  "projection_path": ".codex",
  "runtime_surface": ".codex/mnemon-memory",
  "lifecycle_mapping": {},
  "reconcile": []
}`)
}

func writeLoopValidateFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
