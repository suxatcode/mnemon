package eval

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupRuntimeRunsFileHandlers(t *testing.T) {
	workspace := t.TempDir()
	mnemonDir := filepath.Join(workspace, ".mnemon")
	runtime := SetupRuntime{}

	if err := runtime.Run(context.Background(), SetupOptions{
		Handler:      "setup_local_fact",
		WorkspaceDir: workspace,
		MnemonDir:    mnemonDir,
		Loops:        []string{"memory"},
	}); err != nil {
		t.Fatalf("setup_local_fact returned error: %v", err)
	}
	facts, err := os.ReadFile(filepath.Join(workspace, "FACTS.md"))
	if err != nil {
		t.Fatalf("read facts: %v", err)
	}
	if !strings.Contains(string(facts), "cerulean") {
		t.Fatalf("unexpected facts file: %s", facts)
	}

	if err := runtime.Run(context.Background(), SetupOptions{
		Handler:      "setup_memory_merge",
		WorkspaceDir: workspace,
		MnemonDir:    mnemonDir,
		Loops:        []string{"memory"},
	}); err != nil {
		t.Fatalf("setup_memory_merge returned error: %v", err)
	}
	memory, err := os.ReadFile(filepath.Join(mnemonDir, "harness", "memory", "MEMORY.md"))
	if err != nil {
		t.Fatalf("read memory: %v", err)
	}
	if !strings.Contains(string(memory), "broad host expansion") {
		t.Fatalf("unexpected memory file: %s", memory)
	}
}

func TestSetupRuntimeRunsSkillHandlers(t *testing.T) {
	workspace := t.TempDir()
	mnemonDir := filepath.Join(workspace, ".mnemon")
	runtime := SetupRuntime{}

	if err := runtime.Run(context.Background(), SetupOptions{
		Handler:      "setup_skill_curate_evidence",
		WorkspaceDir: workspace,
		MnemonDir:    mnemonDir,
		Loops:        []string{"skill"},
	}); err != nil {
		t.Fatalf("setup_skill_curate_evidence returned error: %v", err)
	}
	usage, err := os.ReadFile(filepath.Join(mnemonDir, "harness", "skill", "skills", ".usage.jsonl"))
	if err != nil {
		t.Fatalf("read skill usage: %v", err)
	}
	if count := strings.Count(strings.ToLower(string(usage)), "release handoff checklist"); count != 3 {
		t.Fatalf("expected three usage entries, got %d:\n%s", count, usage)
	}

	if err := runtime.Run(context.Background(), SetupOptions{
		Handler:      "setup_skill_active_release",
		WorkspaceDir: workspace,
		MnemonDir:    mnemonDir,
		Loops:        []string{"skill"},
	}); err != nil {
		t.Fatalf("setup_skill_active_release returned error: %v", err)
	}
	skill, err := os.ReadFile(filepath.Join(mnemonDir, "harness", "skill", "skills", "active", "release-checklist", "SKILL.md"))
	if err != nil {
		t.Fatalf("read skill: %v", err)
	}
	if !strings.Contains(string(skill), "name: release-checklist") {
		t.Fatalf("unexpected skill file: %s", skill)
	}
}

func TestSetupRuntimeRunsMnemonHandlersWithConfiguredCommand(t *testing.T) {
	workspace := t.TempDir()
	mnemonDir := filepath.Join(workspace, ".mnemon")
	logPath := filepath.Join(workspace, "mnemon.log")
	fakeMnemon := filepath.Join(workspace, "fake-mnemon.sh")
	if err := os.WriteFile(fakeMnemon, []byte("#!/usr/bin/env bash\nprintf '%s\\n' \"$*\" >> \"$MNEMON_FAKE_LOG\"\n"), 0o755); err != nil {
		t.Fatalf("write fake mnemon: %v", err)
	}
	env := SetupEnv(mnemonDir, []string{"memory"})
	env["MNEMON_FAKE_LOG"] = logPath

	runtime := SetupRuntime{MnemonCommand: fakeMnemon}
	if err := runtime.Run(context.Background(), SetupOptions{
		Handler:      "setup_memory_noise",
		WorkspaceDir: workspace,
		MnemonDir:    mnemonDir,
		Loops:        []string{"memory"},
		Env:          env,
	}); err != nil {
		t.Fatalf("setup_memory_noise returned error: %v", err)
	}
	logData, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake mnemon log: %v", err)
	}
	log := string(logData)
	if strings.Count(log, "remember") != 3 || !strings.Contains(log, "real Codex app-server evals") || !strings.Contains(log, "magenta") {
		t.Fatalf("unexpected fake mnemon log:\n%s", log)
	}
}

func TestSetupEnvPairs(t *testing.T) {
	env := SetupEnv("/tmp/mnemon", []string{"skill", "memory"})
	pairs := SetupEnvPairs(env)
	joined := strings.Join(pairs, "\n")
	for _, want := range []string{
		"MNEMON_DATA_DIR=/tmp/mnemon/data",
		"MNEMON_MEMORY_LOOP_DIR=/tmp/mnemon/harness/memory",
		"MNEMON_SKILL_LOOP_USAGE_FILE=/tmp/mnemon/harness/skill/skills/.usage.jsonl",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected %q in env pairs:\n%s", want, joined)
		}
	}
}
