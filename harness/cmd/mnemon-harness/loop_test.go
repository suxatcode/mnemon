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

func TestLoopPlanCommand(t *testing.T) {
	root := t.TempDir()
	writeLoopValidateFixture(t, root)
	restoreLoopFlags(t)
	loopRoot = root
	loopPlanHost = "codex"
	loopPlanLoops = []string{"memory"}
	loopPlanProjectRoot = root
	loopPlanFormat = "text"

	cmd, output := testCommand()
	if err := runLoopPlan(cmd, nil); err != nil {
		t.Fatalf("runLoopPlan returned error: %v", err)
	}
	if !strings.Contains(output.String(), "Projection plan for host codex") {
		t.Fatalf("unexpected plan output: %s", output.String())
	}
	if !strings.Contains(output.String(), "codex.memory") {
		t.Fatalf("plan output did not include binding: %s", output.String())
	}
}

func TestLoopDiffCommand(t *testing.T) {
	root := t.TempDir()
	projectRoot := t.TempDir()
	writeLoopValidateFixture(t, root)
	restoreLoopFlags(t)
	loopRoot = root

	cmd, output := testCommand()
	err := runLoopProjector(cmd, "diff", []string{
		"--host", "codex",
		"--loop", "memory",
		"--project-root", projectRoot,
	})
	if err != nil {
		t.Fatalf("runLoopProjector diff returned error: %v", err)
	}
	if !strings.Contains(output.String(), "Codex memory diff:") {
		t.Fatalf("unexpected diff output: %s", output.String())
	}
	if !strings.Contains(output.String(), "create .codex/skills/memory-get/SKILL.md") {
		t.Fatalf("diff output did not include projected skill: %s", output.String())
	}
}

func TestLoopReconcileCommandRepairsCodexDrift(t *testing.T) {
	root := t.TempDir()
	projectRoot := t.TempDir()
	writeLoopValidateFixture(t, root)
	restoreLoopFlags(t)
	loopRoot = root

	installCmd, _ := testCommand()
	if err := runLoopProjector(installCmd, "install", []string{
		"--host", "codex",
		"--loop", "memory",
		"--project-root", projectRoot,
	}); err != nil {
		t.Fatalf("install returned error: %v", err)
	}
	skillPath := filepath.Join(projectRoot, ".codex", "skills", "memory-get", "SKILL.md")
	if err := os.WriteFile(skillPath, []byte("local edit\n"), 0o644); err != nil {
		t.Fatalf("edit projected skill: %v", err)
	}

	reconcileCmd, output := testCommand()
	if err := runLoopProjector(reconcileCmd, "reconcile", []string{
		"--host", "codex",
		"--loop", "memory",
		"--project-root", projectRoot,
	}); err != nil {
		t.Fatalf("reconcile returned error: %v", err)
	}
	if !strings.Contains(output.String(), "Codex reconcile: repaired 1 drift item") ||
		!strings.Contains(output.String(), "repaired update .codex/skills/memory-get/SKILL.md") {
		t.Fatalf("unexpected reconcile output:\n%s", output.String())
	}
	repaired, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("read repaired skill: %v", err)
	}
	if string(repaired) == "local edit\n" {
		t.Fatalf("expected reconcile to restore projected skill")
	}
	events, err := os.ReadFile(filepath.Join(projectRoot, ".mnemon", "events.jsonl"))
	if err != nil {
		t.Fatalf("read event log: %v", err)
	}
	if !strings.Contains(string(events), `"type":"projection.repaired"`) {
		t.Fatalf("expected projection.repaired event:\n%s", events)
	}
}

func TestParseLoopProjectorArgsKeepsHostOptions(t *testing.T) {
	restoreLoopFlags(t)
	args, err := parseLoopProjectorArgs([]string{
		"--root", "/repo",
		"--project-root", "/work",
		"--host", "codex",
		"--loop", "memory",
		"--loop", "skill",
		"--config-dir", ".codex-test",
		"--global",
	})
	if err != nil {
		t.Fatalf("parseLoopProjectorArgs returned error: %v", err)
	}
	if args.root != "/repo" || args.projectRoot != "/work" || args.host != "codex" {
		t.Fatalf("unexpected parsed roots/host: %#v", args)
	}
	if strings.Join(args.loops, ",") != "memory,skill" {
		t.Fatalf("unexpected loops: %#v", args.loops)
	}
	if strings.Join(args.hostArgs, " ") != "--config-dir .codex-test --global" {
		t.Fatalf("unexpected host args: %#v", args.hostArgs)
	}
}

func restoreLoopFlags(t *testing.T) {
	t.Helper()
	oldRoot := loopRoot
	oldProjectRoot := loopProjectRoot
	oldPlanHost := loopPlanHost
	oldPlanLoops := loopPlanLoops
	oldPlanFormat := loopPlanFormat
	oldPlanProjectRoot := loopPlanProjectRoot
	t.Cleanup(func() {
		loopRoot = oldRoot
		loopProjectRoot = oldProjectRoot
		loopPlanHost = oldPlanHost
		loopPlanLoops = oldPlanLoops
		loopPlanFormat = oldPlanFormat
		loopPlanProjectRoot = oldPlanProjectRoot
	})
	loopRoot = "."
	loopProjectRoot = ""
	loopPlanHost = ""
	loopPlanLoops = nil
	loopPlanFormat = "text"
	loopPlanProjectRoot = ""
}

func writeLoopValidateFixture(t *testing.T, root string) {
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
		writeLoopValidateFile(t, path, "fixture\n")
	}

	writeLoopValidateFile(t, filepath.Join(loopDir, "loop.json"), `{
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
