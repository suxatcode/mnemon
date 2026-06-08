package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateHarnessAcceptsFixtureDeclarations(t *testing.T) {
	root := t.TempDir()
	writeFixtureHarness(t, root, "skills/memory-get/SKILL.md")

	result, err := ValidateHarness(root)
	if err != nil {
		t.Fatalf("ValidateHarness returned error: %v", err)
	}
	got := strings.Join(result.Lines, "\n")
	for _, want := range []string{
		"ok memory",
		"ok host codex",
		"ok binding codex.memory",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in output:\n%s", want, got)
		}
	}
}

func TestValidateHarnessRejectsMissingDeclaredAsset(t *testing.T) {
	root := t.TempDir()
	writeFixtureHarness(t, root, "skills/missing/SKILL.md")

	_, err := ValidateHarness(root)
	if err == nil || !strings.Contains(err.Error(), "missing memory asset: skills/missing/SKILL.md") {
		t.Fatalf("expected missing asset error, got %v", err)
	}
}

func TestValidateHarnessRejectsDuplicateBindingName(t *testing.T) {
	root := t.TempDir()
	writeFixtureHarness(t, root, "skills/memory-get/SKILL.md")
	writeFile(t, filepath.Join(root, "harness", "bindings", "codex.memory.duplicate.json"), `{
  "schema_version": 1,
  "name": "codex.memory",
  "host": "codex",
  "loop": "memory",
  "projection_path": ".codex",
  "runtime_surface": ".codex/mnemon-memory",
  "lifecycle_mapping": {},
  "reconcile": []
}`)

	_, err := ValidateHarness(root)
	if err == nil || !strings.Contains(err.Error(), `duplicate binding name "codex.memory"`) {
		t.Fatalf("expected duplicate binding name error, got %v", err)
	}
}

func TestValidateHarnessAcceptsBindingSchemaV2(t *testing.T) {
	root := t.TempDir()
	writeFixtureHarness(t, root, "skills/memory-get/SKILL.md")
	writeFile(t, filepath.Join(root, "harness", "bindings", "codex.memory.json"), `{
  "schema_version": 2,
  "name": "codex.memory",
  "host": "codex",
  "loop": "memory",
  "spec": {
    "scope": "project",
    "enabled": true,
    "hook_mode": "native",
    "projection": {
      "path": ".codex",
      "runtime_surface": ".codex/mnemon-memory"
    },
    "lifecycle_mapping": {
      "prime": "SessionStart",
      "remind": "UserPromptSubmit"
    },
    "reconcile": ["read", "write", "no-op"]
  }
}`)

	result, err := ValidateHarness(root)
	if err != nil {
		t.Fatalf("ValidateHarness returned error: %v", err)
	}
	if got := strings.Join(result.Lines, "\n"); !strings.Contains(got, "ok binding codex.memory") {
		t.Fatalf("expected v2 binding in output:\n%s", got)
	}
}

func TestValidateHarnessRejectsBindingSchemaV2MissingHookMode(t *testing.T) {
	root := t.TempDir()
	writeFixtureHarness(t, root, "skills/memory-get/SKILL.md")
	writeFile(t, filepath.Join(root, "harness", "bindings", "codex.memory.json"), `{
  "schema_version": 2,
  "name": "codex.memory",
  "host": "codex",
  "loop": "memory",
  "spec": {
    "scope": "project",
    "enabled": true,
    "projection": {
      "path": ".codex",
      "runtime_surface": ".codex/mnemon-memory"
    },
    "lifecycle_mapping": {},
    "reconcile": []
  }
}`)

	_, err := ValidateHarness(root)
	if err == nil || !strings.Contains(err.Error(), "missing hook_mode") {
		t.Fatalf("expected missing hook_mode error, got %v", err)
	}
}

func writeFixtureHarness(t *testing.T, root, skillPath string) {
	t.Helper()
	loopDir := filepath.Join(root, "harness", "loops", "memory")
	hostDir := filepath.Join(root, "harness", "hosts", "codex")
	bindingsDir := filepath.Join(root, "harness", "bindings")
	for _, dir := range []string{
		filepath.Join(loopDir, "hook-prompts"),
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
		filepath.Join(loopDir, "hook-prompts", "prime.md"),
		filepath.Join(loopDir, "hook-prompts", "remind.md"),
		filepath.Join(loopDir, "hook-prompts", "nudge.md"),
		filepath.Join(loopDir, "hook-prompts", "compact.md"),
		filepath.Join(loopDir, "skills", "memory-get", "SKILL.md"),
	} {
		if err := os.WriteFile(path, []byte("fixture\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
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
    "skills": [`+quote(skillPath)+`],
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
    "projection": [],
    "observation": []
  },
  "lifecycle_mapping": {}
}`)

	writeFile(t, filepath.Join(bindingsDir, "codex.memory.json"), `{
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

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func quote(value string) string {
	return `"` + value + `"`
}
