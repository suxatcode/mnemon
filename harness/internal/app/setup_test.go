package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeMemoryFixture(t *testing.T, root string) {
	t.Helper()
	loopDir := filepath.Join(root, "harness", "loops", "memory")
	hostDir := filepath.Join(root, "harness", "hosts", "codex")
	bindingDir := filepath.Join(root, "harness", "bindings")
	for _, dir := range []string{
		filepath.Join(loopDir, "hook-prompts"),
		filepath.Join(loopDir, "skills", "memory-get"),
		filepath.Join(hostDir, "memory", "hooks"),
		bindingDir,
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(p, c string) {
		if err := os.WriteFile(p, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []string{
		filepath.Join(loopDir, "GUIDE.md"), filepath.Join(loopDir, "env.sh"), filepath.Join(loopDir, "MEMORY.md"),
		filepath.Join(loopDir, "hook-prompts", "prime.md"), filepath.Join(loopDir, "hook-prompts", "remind.md"),
		filepath.Join(loopDir, "hook-prompts", "nudge.md"), filepath.Join(loopDir, "hook-prompts", "compact.md"),
		filepath.Join(loopDir, "skills", "memory-get", "SKILL.md"),
	} {
		write(p, "fixture\n")
	}
	for _, name := range []string{"prime.sh", "remind.sh", "nudge.sh", "compact.sh"} {
		write(filepath.Join(hostDir, "memory", "hooks", name), "#!/usr/bin/env bash\necho fixture\n")
	}
	write(filepath.Join(loopDir, "loop.json"), `{
  "schema_version": 2, "name": "memory",
  "control_model": {"state": [], "intent": "fixture", "reality": [], "reconcile": []},
  "entity_profiles": {}, "surfaces": {"projection": [], "observation": []},
  "assets": {"guide": "GUIDE.md", "env": "env.sh", "runtime_files": ["MEMORY.md"],
    "hook_prompts": {"prime": "hook-prompts/prime.md", "remind": "hook-prompts/remind.md", "nudge": "hook-prompts/nudge.md", "compact": "hook-prompts/compact.md"},
    "skills": ["skills/memory-get/SKILL.md"], "subagents": []},
  "host_adapters": {"codex": "../../hosts/codex"}}`)
	write(filepath.Join(hostDir, "host.json"), `{
  "schema_version": 2, "name": "codex",
  "surfaces": {"projection": [".codex/skills", ".codex/hooks", ".codex/hooks.json", ".codex/mnemon-memory"], "observation": []},
  "lifecycle_mapping": {}, "supports": {"skills": true, "hooks": true}}`)
	write(filepath.Join(bindingDir, "codex.memory.json"), `{
  "schema_version": 1, "name": "codex.memory", "host": "codex", "loop": "memory",
  "projection_path": ".codex", "runtime_surface": ".codex/mnemon-memory",
  "lifecycle_mapping": {"prime": "SessionStart", "remind": "UserPromptSubmit", "nudge": "Stop", "compact": "PreCompact"},
  "reconcile": ["read", "write", "no-op"]}`)
}

// TestSetupProjectsLoopAndWiresChannel is the P4.3 integration test: `setup` wraps the loop install
// (projector writes hooks.json + SKILL.md) AND wires the channel (binding entry + token + env). It
// also checks reinstall idempotency, status, and that uninstall removes the managed binding while
// preserving a user-added one.
func TestSetupProjectsLoopAndWiresChannel(t *testing.T) {
	root := t.TempDir()
	writeMemoryFixture(t, root)
	h := New(root)
	var out, errw bytes.Buffer
	opts := SetupOptions{
		Host: "codex", Loops: []string{"memory"}, ControlURL: "http://127.0.0.1:8787",
		Principal: "codex@project", UseToken: true,
	}
	if _, err := h.Setup(context.Background(), &out, &errw, opts); err != nil {
		t.Fatalf("setup: %v\nstderr=%s", err, errw.String())
	}

	// projector ran: managed hooks + skill projected.
	hooksJSON := filepath.Join(root, ".codex", "hooks.json")
	if b, err := os.ReadFile(hooksJSON); err != nil || !strings.Contains(string(b), "mnemon") {
		t.Fatalf(".codex/hooks.json must contain managed hooks; err=%v content=%q", err, string(b))
	}
	if _, err := os.Stat(filepath.Join(root, ".codex", "skills", "memory-get", "SKILL.md")); err != nil {
		t.Fatalf("projected SKILL.md missing: %v", err)
	}

	// channel artifacts: binding entry, token file, runtime env.
	bindingFile := filepath.Join(root, ".mnemon", "harness", "channel", "bindings.json")
	loaded, err := New(root).SetupStatus("", "codex@project") // exercises LoadBindingFile path
	if err != nil {
		t.Fatalf("setup status: %v", err)
	}
	_ = loaded
	bf, err := os.ReadFile(bindingFile)
	if err != nil || !strings.Contains(string(bf), "codex@project") || !strings.Contains(string(bf), "127.0.0.1:8787") {
		t.Fatalf("bindings.json must record the principal + endpoint; err=%v content=%s", err, string(bf))
	}
	tokenFile := filepath.Join(root, ".mnemon", "harness", "channel", "tokens", "codex-project.token")
	if fi, err := os.Stat(tokenFile); err != nil || fi.Size() == 0 {
		t.Fatalf("token file must exist + be non-empty: %v", err)
	}
	envSh := filepath.Join(root, ".mnemon", "harness", "channel", "env.sh")
	env, err := os.ReadFile(envSh)
	if err != nil {
		t.Fatalf("read channel env: %v", err)
	}
	for _, want := range []string{"MNEMON_HARNESS_BIN", "MNEMON_CONTROL_ADDR", "MNEMON_CONTROL_PRINCIPAL", "MNEMON_CONTROL_TOKEN_FILE", "MNEMON_MEMORY_LOOP_DIR"} {
		if !strings.Contains(string(env), want) {
			t.Fatalf("channel env must export %s; got:\n%s", want, string(env))
		}
	}

	// reinstall is idempotent: still exactly one codex binding entry.
	if _, err := h.Setup(context.Background(), &out, &errw, opts); err != nil {
		t.Fatalf("reinstall: %v", err)
	}
	if n := strings.Count(string(mustRead(t, bindingFile)), `"codex@project"`); n != 1 {
		t.Fatalf("reinstall must not duplicate the binding; got %d codex entries", n)
	}

	// a user-added sibling binding must survive uninstall.
	userOpts := SetupOptions{Host: "codex", Loops: []string{"memory"}, ControlURL: "http://127.0.0.1:8787", Principal: "human@project"}
	if _, err := h.Setup(context.Background(), &out, &errw, userOpts); err != nil {
		t.Fatalf("user setup: %v", err)
	}
	if err := h.SetupUninstall(context.Background(), &out, &errw, opts); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	after := string(mustRead(t, bindingFile))
	if strings.Contains(after, "codex@project") {
		t.Fatalf("uninstall must remove the managed binding; still present:\n%s", after)
	}
	if !strings.Contains(after, "human@project") {
		t.Fatalf("uninstall must preserve the user-added binding; gone:\n%s", after)
	}
	if _, err := os.Stat(tokenFile); !os.IsNotExist(err) {
		t.Fatalf("uninstall must remove the managed token file; err=%v", err)
	}
}

// TestSetupDryRunWritesNothing is the P4 gate dry-run check: --dry-run prints changes without
// writing channel artifacts.
func TestSetupDryRunWritesNothing(t *testing.T) {
	root := t.TempDir()
	writeMemoryFixture(t, root)
	var out, errw bytes.Buffer
	_, err := New(root).Setup(context.Background(), &out, &errw, SetupOptions{
		Host: "codex", Loops: []string{"memory"}, ControlURL: "http://127.0.0.1:8787",
		Principal: "codex@project", UseToken: true, DryRun: true,
	})
	if err != nil {
		t.Fatalf("dry-run setup: %v\nstderr=%s", err, errw.String())
	}
	if !strings.Contains(out.String(), "dry-run") {
		t.Fatalf("dry-run must announce changes; got:\n%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(root, ".mnemon", "harness", "channel", "bindings.json")); !os.IsNotExist(err) {
		t.Fatalf("dry-run must not write the binding file; err=%v", err)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}
