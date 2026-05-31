package projection

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildPlanExplainsCodexMemoryProjection(t *testing.T) {
	root := t.TempDir()
	writePlanFixture(t, root)

	plan, err := BuildPlan(PlanOptions{
		DeclarationRoot: root,
		ProjectRoot:     filepath.Join(root, "work"),
		Host:            "codex",
		Loops:           []string{"memory"},
	})
	if err != nil {
		t.Fatalf("BuildPlan returned error: %v", err)
	}
	if plan.Backend != "go-projector" {
		t.Fatalf("unexpected backend: %s", plan.Backend)
	}
	if len(plan.Loops) != 1 || plan.Loops[0].Binding != "codex.memory" {
		t.Fatalf("unexpected loops: %#v", plan.Loops)
	}
	var output bytes.Buffer
	if err := WritePlanText(&output, plan); err != nil {
		t.Fatalf("WritePlanText returned error: %v", err)
	}
	text := output.String()
	for _, want := range []string{
		"Projection plan for host codex",
		"codex.memory:",
		"project_skill: harness/loops/memory/skills/memory-get/SKILL.md -> .codex/skills/memory-get/SKILL.md",
		"project_native_hook: harness/hosts/codex/memory/hooks/prime.sh -> .codex/hooks/mnemon-memory/prime.sh (SessionStart)",
		"patch_host_hooks: .codex/hooks.json",
		"go_apply_backend (declaration-driven Codex projection engine)",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in plan:\n%s", want, text)
		}
	}
}

func writePlanFixture(t *testing.T, root string) {
	t.Helper()
	loopDir := filepath.Join(root, "harness", "loops", "memory")
	hostDir := filepath.Join(root, "harness", "hosts", "codex")
	bindingDir := filepath.Join(root, "harness", "bindings")
	for _, dir := range []string{
		filepath.Join(loopDir, "hook-prompts"),
		filepath.Join(loopDir, "skills", "memory-get"),
		filepath.Join(hostDir, "memory", "hooks"),
		hostDir,
		bindingDir,
	} {
		mkdir(t, dir)
	}
	for _, path := range []string{
		filepath.Join(loopDir, "GUIDE.md"),
		filepath.Join(loopDir, "env.sh"),
		filepath.Join(loopDir, "MEMORY.md"),
		filepath.Join(loopDir, "hook-prompts", "prime.md"),
		filepath.Join(loopDir, "hook-prompts", "remind.md"),
		filepath.Join(loopDir, "hook-prompts", "nudge.md"),
		filepath.Join(loopDir, "hook-prompts", "compact.md"),
		filepath.Join(loopDir, "skills", "memory-get", "SKILL.md"),
	} {
		writeFile(t, path, "fixture\n")
	}
	for _, name := range []string{"prime.sh", "remind.sh", "nudge.sh", "compact.sh"} {
		writeFile(t, filepath.Join(hostDir, "memory", "hooks", name), "#!/usr/bin/env bash\necho fixture\n")
	}
	writeFile(t, filepath.Join(loopDir, "loop.json"), `{
  "schema_version": 2,
  "name": "memory",
  "control_model": {
    "state": [],
    "intent": "fixture",
    "reality": [],
    "reconcile": []
  },
  "entity_profiles": {},
  "surfaces": {
    "projection": [],
    "observation": []
  },
  "assets": {
    "guide": "GUIDE.md",
    "env": "env.sh",
    "runtime_files": ["MEMORY.md"],
    "hook_prompts": {
      "prime": "hook-prompts/prime.md",
      "remind": "hook-prompts/remind.md",
      "nudge": "hook-prompts/nudge.md",
      "compact": "hook-prompts/compact.md"
    },
    "skills": ["skills/memory-get/SKILL.md"],
    "subagents": []
  },
  "host_adapters": {
    "codex": "../../hosts/codex"
  }
}`)
	writeFile(t, filepath.Join(hostDir, "host.json"), `{
  "schema_version": 2,
  "name": "codex",
  "surfaces": {
    "projection": [".codex/skills", ".codex/hooks", ".codex/hooks.json", ".codex/mnemon-memory"],
    "observation": []
  },
  "lifecycle_mapping": {},
  "supports": {
    "skills": true,
    "hooks": true
  }
}`)
	writeFile(t, filepath.Join(bindingDir, "codex.memory.json"), `{
  "schema_version": 1,
  "name": "codex.memory",
  "host": "codex",
  "loop": "memory",
  "projection_path": ".codex",
  "runtime_surface": ".codex/mnemon-memory",
  "lifecycle_mapping": {
    "prime": "SessionStart",
    "remind": "UserPromptSubmit",
    "nudge": "Stop",
    "compact": "PreCompact"
  },
  "reconcile": ["read", "write", "no-op"]
}`)
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}
