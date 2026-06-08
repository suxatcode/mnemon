package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/channel"
)

func TestSetupProductFlagsSelectLoops(t *testing.T) {
	oldLoops := setupLoops
	oldMemory := setupMemory
	oldSkills := setupSkills
	t.Cleanup(func() {
		setupLoops = oldLoops
		setupMemory = oldMemory
		setupSkills = oldSkills
	})

	setupLoops = []string{"memory"}
	setupMemory = true
	setupSkills = true

	got := selectedSetupLoops()
	want := []string{"memory", "skill"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("selectedSetupLoops() = %#v, want %#v", got, want)
	}
}

func TestSetupCommandUsesProductDefaults(t *testing.T) {
	restoreSetupFlags(t)
	projectRoot := t.TempDir()
	setupRoot = cmdRepoRoot(t)
	setupProjectRoot = projectRoot
	setupHost = "codex"
	setupMemory = true
	setupSkills = true
	setupPrincipal = ""
	setupControlURL = ""
	setupUseToken = false

	var out, errw bytes.Buffer
	setupCmd.SetOut(&out)
	setupCmd.SetErr(&errw)
	t.Cleanup(func() {
		setupCmd.SetOut(os.Stdout)
		setupCmd.SetErr(os.Stderr)
	})
	if err := setupCmd.RunE(setupCmd, nil); err != nil {
		t.Fatalf("setup command with product defaults: %v\nstderr=%s", err, errw.String())
	}
	got := out.String()
	for _, want := range []string{"Agent Integration:", "Local Mnemon:", "Remote Workspace:"} {
		if !strings.Contains(got, want) {
			t.Fatalf("setup output missing %q:\n%s", want, got)
		}
	}

	bindingJSON := string(mustReadCmd(t, filepath.Join(projectRoot, channel.DefaultBindingFile)))
	for _, want := range []string{
		`"principal": "codex@project"`,
		`"endpoint": "http://127.0.0.1:8787"`,
		`"memory.write_candidate_observed"`,
		`"skill.write_candidate_observed"`,
		`.mnemon/harness/channel/credentials/codex-project.token`,
	} {
		if !strings.Contains(bindingJSON, want) {
			t.Fatalf("setup defaults missing %q from bindings:\n%s", want, bindingJSON)
		}
	}
	if _, err := os.Stat(filepath.Join(projectRoot, ".mnemon", "harness", "channel", "credentials", "codex-project.token")); err != nil {
		t.Fatalf("setup must generate the default local token: %v", err)
	}
	configJSON := string(mustReadCmd(t, filepath.Join(projectRoot, ".mnemon", "harness", "local", "config.json")))
	for _, want := range []string{`"endpoint": "http://127.0.0.1:8787"`, `"principal": "codex@project"`, "bindings.json", "governed.db"} {
		if !strings.Contains(configJSON, want) {
			t.Fatalf("Local Mnemon config missing %q:\n%s", want, configJSON)
		}
	}
}

func restoreSetupFlags(t *testing.T) {
	t.Helper()
	oldRoot := setupRoot
	oldProjectRoot := setupProjectRoot
	oldHost := setupHost
	oldLoops := setupLoops
	oldMemory := setupMemory
	oldSkills := setupSkills
	oldPrincipal := setupPrincipal
	oldControlURL := setupControlURL
	oldActorKind := setupActorKind
	oldUseToken := setupUseToken
	oldDryRun := setupDryRun
	t.Cleanup(func() {
		setupRoot = oldRoot
		setupProjectRoot = oldProjectRoot
		setupHost = oldHost
		setupLoops = oldLoops
		setupMemory = oldMemory
		setupSkills = oldSkills
		setupPrincipal = oldPrincipal
		setupControlURL = oldControlURL
		setupActorKind = oldActorKind
		setupUseToken = oldUseToken
		setupDryRun = oldDryRun
	})
	setupRoot = "."
	setupProjectRoot = ""
	setupHost = ""
	setupLoops = nil
	setupMemory = false
	setupSkills = false
	setupPrincipal = ""
	setupControlURL = ""
	setupActorKind = "host-agent"
	setupUseToken = false
	setupDryRun = false
}

func cmdRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve command test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func setupProductIntegration(t *testing.T, projectRoot string) {
	t.Helper()
	restoreSetupFlags(t)
	setupRoot = cmdRepoRoot(t)
	setupProjectRoot = projectRoot
	setupHost = "codex"
	setupMemory = true
	setupSkills = true
	setupPrincipal = ""
	setupControlURL = ""
	setupUseToken = false
	var out, errw bytes.Buffer
	setupCmd.SetOut(&out)
	setupCmd.SetErr(&errw)
	t.Cleanup(func() {
		setupCmd.SetOut(os.Stdout)
		setupCmd.SetErr(os.Stderr)
	})
	ctx := context.Background()
	setupCmd.SetContext(ctx)
	if err := setupCmd.RunE(setupCmd, nil); err != nil {
		t.Fatalf("setup product integration: %v\nstdout=%s\nstderr=%s", err, out.String(), errw.String())
	}
}
