package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/core/server"
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
