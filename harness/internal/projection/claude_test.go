package projection

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRunClaudeProjectorPullsScopedProfileFragment mirrors the Codex pull proof
// for Claude Code: a profile entry targeted at claude-code/memory is projected to
// the Claude runtime surface, scoped (a codex-targeted entry is excluded).
func TestRunClaudeProjectorPullsScopedProfileFragment(t *testing.T) {
	root := t.TempDir()
	projectRoot := t.TempDir()
	writeClaudeFixture(t, root)

	seedProfileEntry(t, projectRoot, "claude-pref", time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC), "claude-code", "memory")
	seedProfileEntry(t, projectRoot, "codex-pref", time.Date(2026, 5, 30, 0, 0, 1, 0, time.UTC), "codex", "memory")

	if err := RunClaudeProjector(context.Background(), "install", ClaudeOptions{
		DeclarationRoot: root,
		ProjectRoot:     projectRoot,
		Loops:           []string{"memory"},
		Stdout:          &bytes.Buffer{},
	}); err != nil {
		t.Fatalf("RunClaudeProjector install returned error: %v", err)
	}

	frag := readProfileFragment(t, filepath.Join(projectRoot, ".claude", "mnemon-memory", "PROFILE.json"))
	if len(frag.Entries) != 1 {
		t.Fatalf("claude fragment should hold only the claude-code/memory entry, got %d: %#v", len(frag.Entries), frag.Entries)
	}
	if frag.Entries[0].ID != "claude-pref" {
		t.Fatalf("claude fragment entry = %q, want claude-pref", frag.Entries[0].ID)
	}
}

// TestRunClaudeProjectorInheritsMergeDecision proves the Band 4 "next run
// inherits it" gate point: after a merge applied T2 into T1, the host that owned
// T2 pulls a COORDINATION.json showing T2 joined into T1 — the next run inherits
// the merge decision.
func TestRunClaudeProjectorInheritsMergeDecision(t *testing.T) {
	root := t.TempDir()
	projectRoot := t.TempDir()
	writeClaudeFixture(t, root)
	seedCoordinationLedger(t, projectRoot)

	if err := RunClaudeProjector(context.Background(), "install", ClaudeOptions{
		DeclarationRoot: root,
		ProjectRoot:     projectRoot,
		Loops:           []string{"memory"},
		Stdout:          &bytes.Buffer{},
	}); err != nil {
		t.Fatalf("install: %v", err)
	}
	frag := readCoordinationFragment(t, filepath.Join(projectRoot, ".claude", "mnemon-memory", "COORDINATION.json"))
	if len(frag.Tasks) != 1 || frag.Tasks[0].ID != "T2" {
		t.Fatalf("claude fragment should hold its own task T2, got %#v", frag.Tasks)
	}
	if frag.Tasks[0].Status != "joined" || frag.Tasks[0].JoinedInto != "T1" {
		t.Fatalf("next run should inherit the merge: T2 joined into T1, got %#v", frag.Tasks[0])
	}
}

func TestRunClaudeProjectorInstallsSettingsAndUninstallsMemory(t *testing.T) {
	root := t.TempDir()
	projectRoot := t.TempDir()
	writeClaudeFixture(t, root)
	settingsPath := filepath.Join(projectRoot, ".claude", "settings.json")
	mkdir(t, filepath.Dir(settingsPath))
	writeFile(t, settingsPath, `{
  // keep unrelated hooks and tolerate trailing commas
  "hooks": {
    "SessionStart": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "custom.sh"
          }
        ]
      },
    ],
  },
}`)

	var installOut bytes.Buffer
	err := RunClaudeProjector(context.Background(), "install", ClaudeOptions{
		DeclarationRoot: root,
		ProjectRoot:     projectRoot,
		Loops:           []string{"memory"},
		Stdout:          &installOut,
	})
	if err != nil {
		t.Fatalf("RunClaudeProjector install returned error: %v", err)
	}
	for _, rel := range []string{
		".mnemon/harness/memory/GUIDE.md",
		".mnemon/harness/memory/MEMORY.md",
		".mnemon/harness/memory/status.json",
		".claude/mnemon-memory/env.sh",
		".claude/mnemon-memory/GUIDE.md",
		".claude/skills/memory-get/SKILL.md",
		".claude/agents/mnemon-dreaming.md",
		".claude/hooks/mnemon-memory/prime.sh",
		".mnemon/hosts/claude-code/manifest.json",
	} {
		if _, err := os.Stat(filepath.Join(projectRoot, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("expected projected file %s: %v", rel, err)
		}
	}
	settings := readSettings(t, settingsPath)
	if !settingsContains(settings, "custom.sh") {
		t.Fatalf("settings lost unrelated hook: %#v", settings)
	}
	if !settingsContains(settings, ".claude/hooks/mnemon-memory/prime.sh") {
		t.Fatalf("settings missing mnemon memory hook: %#v", settings)
	}

	var statusOut bytes.Buffer
	err = RunClaudeProjector(context.Background(), "status", ClaudeOptions{
		DeclarationRoot: root,
		ProjectRoot:     projectRoot,
		Stdout:          &statusOut,
	})
	if err != nil {
		t.Fatalf("RunClaudeProjector status returned error: %v", err)
	}
	if !strings.Contains(statusOut.String(), "Claude Code memory:") {
		t.Fatalf("unexpected status:\n%s", statusOut.String())
	}

	err = RunClaudeProjector(context.Background(), "uninstall", ClaudeOptions{
		DeclarationRoot: root,
		ProjectRoot:     projectRoot,
		Loops:           []string{"memory"},
	})
	if err != nil {
		t.Fatalf("RunClaudeProjector uninstall returned error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectRoot, ".claude", "skills", "memory-get")); !os.IsNotExist(err) {
		t.Fatalf("expected projected memory skill to be removed, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectRoot, ".mnemon", "harness", "memory", "MEMORY.md")); err != nil {
		t.Fatalf("expected MEMORY.md to be preserved, got %v", err)
	}
	settings = readSettings(t, settingsPath)
	if !settingsContains(settings, "custom.sh") {
		t.Fatalf("settings lost unrelated hook after uninstall: %#v", settings)
	}
	if settingsContains(settings, "mnemon-memory") {
		t.Fatalf("settings retained mnemon memory hook after uninstall: %#v", settings)
	}
}

func writeClaudeFixture(t *testing.T, root string) {
	t.Helper()
	loopDir := filepath.Join(root, "harness", "loops", "memory")
	hostDir := filepath.Join(root, "harness", "hosts", "claude-code")
	bindingDir := filepath.Join(root, "harness", "bindings")
	for _, dir := range []string{
		filepath.Join(loopDir, "hook-prompts"),
		filepath.Join(loopDir, "skills", "memory-get"),
		filepath.Join(loopDir, "subagents"),
		filepath.Join(hostDir, "memory", "hooks"),
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
		filepath.Join(loopDir, "subagents", "dreaming.md"),
		filepath.Join(hostDir, "memory", "hooks", "prime.sh"),
		filepath.Join(hostDir, "memory", "hooks", "remind.sh"),
		filepath.Join(hostDir, "memory", "hooks", "nudge.sh"),
		filepath.Join(hostDir, "memory", "hooks", "compact.sh"),
	} {
		writeFile(t, path, "fixture\n")
	}
	writeFile(t, filepath.Join(loopDir, "loop.json"), `{
  "schema_version": 2,
  "name": "memory",
  "version": "0.1.0",
  "control_model": {
    "state": [],
    "intent": "fixture",
    "reality": [],
    "reconcile": ["read"]
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
    "subagents": ["subagents/dreaming.md"]
  },
  "host_adapters": {
    "claude-code": "../../hosts/claude-code"
  }
}`)
	writeFile(t, filepath.Join(hostDir, "host.json"), `{
  "schema_version": 2,
  "name": "claude-code",
  "surfaces": {
    "projection": [".claude/skills", ".claude/agents", ".claude/hooks", ".claude/settings.json"],
    "observation": []
  },
  "lifecycle_mapping": {},
  "supports": {
    "skills": true,
    "hooks": true,
    "subagents": true
  }
}`)
	writeFile(t, filepath.Join(bindingDir, "claude-code.memory.json"), `{
  "schema_version": 1,
  "name": "claude-code.memory",
  "host": "claude-code",
  "loop": "memory",
  "projection_path": ".claude",
  "runtime_surface": ".claude/mnemon-memory",
  "lifecycle_mapping": {
    "prime": "SessionStart",
    "remind": "UserPromptSubmit",
    "nudge": "Stop",
    "compact": "PreCompact"
  },
  "reconcile": ["read"]
}`)
}

func readSettings(t *testing.T, settingsPath string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parse settings: %v", err)
	}
	return settings
}

func settingsContains(value any, needle string) bool {
	switch typed := value.(type) {
	case string:
		return strings.Contains(typed, needle)
	case []any:
		for _, item := range typed {
			if settingsContains(item, needle) {
				return true
			}
		}
	case map[string]any:
		for _, item := range typed {
			if settingsContains(item, needle) {
				return true
			}
		}
	}
	return false
}
