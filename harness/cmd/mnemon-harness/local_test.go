package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/capability"
	"github.com/mnemon-dev/mnemon/harness/internal/server"
)

func TestLocalStatusReportsProductBoundary(t *testing.T) {
	root := t.TempDir()
	restoreLocalFlags(t)
	localRoot = root

	cmd, output := testCommand()
	if err := runLocalStatus(cmd, nil); err != nil {
		t.Fatalf("runLocalStatus returned error: %v", err)
	}
	got := output.String()
	for _, want := range []string{
		"Local Mnemon: ready",
		"Remote Workspace: disconnected",
		"Mode: local",
		filepath.Join(root, server.DefaultStorePath),
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("local status missing %q:\n%s", want, got)
		}
	}
	for _, blocked := range []string{"channel", "runtime", "kernel", "outbox", "cursor"} {
		if strings.Contains(strings.ToLower(got), blocked) {
			t.Fatalf("local status leaked %q:\n%s", blocked, got)
		}
	}
}

func TestLocalBootAutoDiscoversSetupConfig(t *testing.T) {
	projectRoot := t.TempDir()
	setupProductIntegration(t, projectRoot)
	restoreLocalFlags(t)
	localRoot = projectRoot

	boot, err := resolveLocalBoot()
	if err != nil {
		t.Fatalf("resolve local boot from setup config: %v", err)
	}
	if !boot.Configured {
		t.Fatal("local boot must use setup config when --bindings is omitted")
	}
	if boot.StorePath != filepath.Join(projectRoot, server.DefaultStorePath) {
		t.Fatalf("store path = %q, want project default", boot.StorePath)
	}
	if len(boot.Loaded.Tokens) == 0 {
		t.Fatal("local boot must load setup token credentials")
	}
	cfg := server.LocalRuntimeConfigFromBindings(boot.Loaded.Bindings)
	var handlesMemory, handlesSkill bool
	for _, r := range cfg.Rules.Rules() {
		handlesMemory = handlesMemory || r.Handles(capability.MemoryWriteCandidateObserved)
		handlesSkill = handlesSkill || r.Handles(capability.SkillWriteCandidateObserved)
	}
	if !handlesMemory || !handlesSkill {
		t.Fatalf("local boot must enable memory and skill rules; memory=%v skill=%v", handlesMemory, handlesSkill)
	}
}

func TestLocalBootMissingSetupShowsProductRemediation(t *testing.T) {
	restoreLocalFlags(t)
	localRoot = t.TempDir()
	_, err := resolveLocalBoot()
	if err == nil {
		t.Fatal("local boot without setup must fail")
	}
	for _, want := range []string{
		"Local Mnemon is not set up.",
		"mnemon-harness setup --host codex --memory --skills",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("missing remediation %q in error:\n%v", want, err)
		}
	}
	for _, blocked := range []string{"binding", "channel", "runtime", "kernel", "token file"} {
		if strings.Contains(strings.ToLower(err.Error()), blocked) {
			t.Fatalf("local boot remediation leaked %q:\n%v", blocked, err)
		}
	}
}

func restoreLocalFlags(t *testing.T) {
	t.Helper()
	oldRoot := localRoot
	oldAddr := localAddr
	oldStore := localStorePath
	oldBindings := localBindingsPath
	t.Cleanup(func() {
		localRoot = oldRoot
		localAddr = oldAddr
		localStorePath = oldStore
		localBindingsPath = oldBindings
	})
	localRoot = "."
	localAddr = "127.0.0.1:8787"
	localStorePath = ""
	localBindingsPath = ""
}
