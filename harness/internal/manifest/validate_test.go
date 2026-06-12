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

// G6: unknown manifest keys are junk that decoded silently for an entire dev cycle (the P1
// clearcut stripped six of them). Both decode paths must refuse them — the struct path
// (LoadLoop/LoadBinding, install) and the map path (ValidateFS) must agree, fail-closed.
func TestLoadLoopRejectsUnknownKey(t *testing.T) {
	root := t.TempDir()
	writeFixtureHarness(t, root, "skills/memory-get/SKILL.md")
	writeFile(t, filepath.Join(root, "loops", "memory", "loop.json"), `{
  "schema_version": 2,
  "name": "memory",
  "surfaces": { "projection": [], "observation": [] },
  "assets": { "guide": "GUIDE.md", "env": "env.sh", "skills": [], "subagents": [] },
  "lifecycle_events": ["prime"]
}`)

	if _, err := LoadLoop(os.DirFS(root), "memory"); err == nil || !strings.Contains(err.Error(), "lifecycle_events") {
		t.Fatalf("LoadLoop must reject an unknown manifest key; got %v", err)
	}
}

// PD4 store sink: the optional `store` declaration loads, and an unknown key inside it fails closed
// on both decode paths (struct LoadLoop + map ValidateFS), the G6 discipline.
func TestLoopStoreDeclaration(t *testing.T) {
	root := t.TempDir()
	writeFixtureHarness(t, root, "skills/memory-get/SKILL.md")
	writeFile(t, filepath.Join(root, "loops", "memory", "loop.json"), `{
  "schema_version": 2,
  "name": "memory",
  "surfaces": { "projection": [], "observation": [] },
  "assets": { "guide": "GUIDE.md", "env": "env.sh", "runtime_files": ["MEMORY.md"], "skills": ["skills/memory-get/SKILL.md"], "subagents": [] },
  "store": { "native": true }
}`)
	loop, err := LoadLoop(os.DirFS(root), "memory")
	if err != nil {
		t.Fatalf("a loop declaring a native store must load: %v", err)
	}
	if loop.Store == nil || !loop.Store.Native {
		t.Fatalf("store.native must decode true, got %+v", loop.Store)
	}

	writeFile(t, filepath.Join(root, "loops", "memory", "loop.json"), `{
  "schema_version": 2,
  "name": "memory",
  "surfaces": { "projection": [], "observation": [] },
  "assets": { "guide": "GUIDE.md", "env": "env.sh", "runtime_files": ["MEMORY.md"], "skills": ["skills/memory-get/SKILL.md"], "subagents": [] },
  "store": { "bogus": true }
}`)
	if _, err := LoadLoop(os.DirFS(root), "memory"); err == nil || !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("LoadLoop must reject an unknown store key (struct path); got %v", err)
	}
	if _, err := ValidateFS(os.DirFS(root)); err == nil || !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("ValidateFS must reject an unknown store key (map path); got %v", err)
	}
}

// PD4 state_dirs sink: the declaration loads, and a path-traversal entry fails closed on both paths.
func TestLoopStateDirsDeclaration(t *testing.T) {
	root := t.TempDir()
	writeFixtureHarness(t, root, "skills/memory-get/SKILL.md")
	good := `{
  "schema_version": 2, "name": "memory",
  "surfaces": { "projection": [], "observation": [] },
  "assets": { "guide": "GUIDE.md", "env": "env.sh", "runtime_files": ["MEMORY.md"], "skills": ["skills/memory-get/SKILL.md"], "subagents": [] },
  "state_dirs": ["skills/active", "proposals"]
}`
	writeFile(t, filepath.Join(root, "loops", "memory", "loop.json"), good)
	loop, err := LoadLoop(os.DirFS(root), "memory")
	if err != nil {
		t.Fatalf("a loop declaring state_dirs must load: %v", err)
	}
	if len(loop.StateDirs) != 2 || loop.StateDirs[0] != "skills/active" {
		t.Fatalf("state_dirs must decode, got %v", loop.StateDirs)
	}

	bad := `{
  "schema_version": 2, "name": "memory",
  "surfaces": { "projection": [], "observation": [] },
  "assets": { "guide": "GUIDE.md", "env": "env.sh", "runtime_files": ["MEMORY.md"], "skills": ["skills/memory-get/SKILL.md"], "subagents": [] },
  "state_dirs": ["../../etc"]
}`
	writeFile(t, filepath.Join(root, "loops", "memory", "loop.json"), bad)
	if _, err := ValidateFS(os.DirFS(root)); err == nil || !strings.Contains(err.Error(), "unsafe state_dir") {
		t.Fatalf("ValidateFS must reject a path-traversal state_dir; got %v", err)
	}
}

// PD4 env sink (injection lock): a loop.json env value carrying shell metacharacters fails closed
// on both decode paths (struct LoadLoop + map ValidateFS).
func TestLoopEnvInjectionRejected(t *testing.T) {
	root := t.TempDir()
	writeFixtureHarness(t, root, "skills/memory-get/SKILL.md")
	writeFile(t, filepath.Join(root, "loops", "memory", "loop.json"), `{
  "schema_version": 2, "name": "memory",
  "surfaces": { "projection": [], "observation": [] },
  "assets": { "guide": "GUIDE.md", "env": "env.sh", "runtime_files": ["MEMORY.md"], "skills": ["skills/memory-get/SKILL.md"], "subagents": [] },
  "env": [ { "name": "MNEMON_X", "value": "$(rm -rf /)" } ]
}`)
	if _, err := LoadLoop(os.DirFS(root), "memory"); err == nil || !strings.Contains(err.Error(), "env value") {
		t.Fatalf("LoadLoop must reject a shell-injection env value (struct path); got %v", err)
	}
	if _, err := ValidateFS(os.DirFS(root)); err == nil || !strings.Contains(err.Error(), "env value") {
		t.Fatalf("ValidateFS must reject a shell-injection env value (map path); got %v", err)
	}
}

func TestValidateHarnessRejectsUnknownLoopKey(t *testing.T) {
	root := t.TempDir()
	writeFixtureHarness(t, root, "skills/memory-get/SKILL.md")
	writeFile(t, filepath.Join(root, "loops", "memory", "loop.json"), `{
  "schema_version": 2,
  "name": "memory",
  "surfaces": { "projection": [], "observation": [] },
  "assets": { "guide": "GUIDE.md", "env": "env.sh", "skills": [], "subagents": [] },
  "controllers": []
}`)

	_, err := ValidateFS(os.DirFS(root))
	if err == nil || !strings.Contains(err.Error(), "unknown key") || !strings.Contains(err.Error(), "controllers") {
		t.Fatalf("validate must reject an unknown loop manifest key; got %v", err)
	}
}

func TestValidateHarnessRejectsUnknownAssetsKey(t *testing.T) {
	root := t.TempDir()
	writeFixtureHarness(t, root, "skills/memory-get/SKILL.md")
	writeFile(t, filepath.Join(root, "loops", "memory", "loop.json"), `{
  "schema_version": 2,
  "name": "memory",
  "surfaces": { "projection": [], "observation": [] },
  "assets": { "guide": "GUIDE.md", "env": "env.sh", "skills": [], "subagents": [], "hook_prompts": {} }
}`)

	_, err := ValidateFS(os.DirFS(root))
	if err == nil || !strings.Contains(err.Error(), "unknown key") || !strings.Contains(err.Error(), "hook_prompts") {
		t.Fatalf("validate must reject an unknown assets key; got %v", err)
	}
}

func TestValidateHarnessRejectsUnknownBindingKey(t *testing.T) {
	root := t.TempDir()
	writeFixtureHarness(t, root, "skills/memory-get/SKILL.md")
	writeFile(t, filepath.Join(root, "bindings", "codex.memory.json"), `{
  "schema_version": 1,
  "name": "codex.memory",
  "host": "codex",
  "loop": "memory",
  "projection_path": ".codex",
  "runtime_surface": ".codex/mnemon-memory",
  "lifecycle_mapping": {},
  "reconcile": [],
  "runner_bindings": {}
}`)

	_, err := ValidateFS(os.DirFS(root))
	if err == nil || !strings.Contains(err.Error(), "unknown key") || !strings.Contains(err.Error(), "runner_bindings") {
		t.Fatalf("validate must reject an unknown binding manifest key; got %v", err)
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
  "projection_path": ".codex",
  "runtime_surface": ".codex/mnemon-memory",
  "lifecycle_mapping": {},
  "reconcile": []
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
