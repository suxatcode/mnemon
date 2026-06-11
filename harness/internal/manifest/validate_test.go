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

	result, err := ValidateFS(os.DirFS(root))
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

	_, err := ValidateFS(os.DirFS(root))
	if err == nil || !strings.Contains(err.Error(), "missing memory asset: skills/missing/SKILL.md") {
		t.Fatalf("expected missing asset error, got %v", err)
	}
}

func TestValidateHarnessRejectsDuplicateBindingName(t *testing.T) {
	root := t.TempDir()
	writeFixtureHarness(t, root, "skills/memory-get/SKILL.md")
	writeFile(t, filepath.Join(root, "bindings", "codex.memory.duplicate.json"), `{
  "schema_version": 1,
  "name": "codex.memory",
  "host": "codex",
  "loop": "memory",
  "projection_path": ".codex",
  "runtime_surface": ".codex/mnemon-memory",
  "lifecycle_mapping": {},
  "reconcile": []
}`)

	_, err := ValidateFS(os.DirFS(root))
	if err == nil || !strings.Contains(err.Error(), `duplicate binding name "codex.memory"`) {
		t.Fatalf("expected duplicate binding name error, got %v", err)
	}
}

// The speculative v2 binding format never shipped an instance; P1 clearcut removed it. Any
// non-v1 schema_version must be rejected so validate and LoadBinding agree (fail-closed).
func TestValidateHarnessRejectsNonV1BindingSchema(t *testing.T) {
	root := t.TempDir()
	writeFixtureHarness(t, root, "skills/memory-get/SKILL.md")
	writeFile(t, filepath.Join(root, "bindings", "codex.memory.json"), `{
  "schema_version": 2,
  "name": "codex.memory",
  "host": "codex",
  "loop": "memory",
  "spec": {}
}`)

	_, err := ValidateFS(os.DirFS(root))
	if err == nil || !strings.Contains(err.Error(), "schema_version must be 1") {
		t.Fatalf("expected schema_version rejection, got %v", err)
	}
}

func writeFixtureHarness(t *testing.T, root, skillPath string) {
	t.Helper()
	loopDir := filepath.Join(root, "loops", "memory")
	hostDir := filepath.Join(root, "hosts", "codex")
	bindingsDir := filepath.Join(root, "bindings")
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
		if err := os.WriteFile(path, []byte("fixture\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	writeFile(t, filepath.Join(loopDir, "loop.json"), `{
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
