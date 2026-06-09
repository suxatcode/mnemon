package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

// A projected subagent in the SHARED .claude/agents dir is a managed file too: uninstall must not
// delete one the user has hand-edited, and install must not clobber a pre-existing one. (Also the only
// coverage of claude-code skill install/uninstall.)
func TestClaudeUninstallPreservesUserEditedSubagent(t *testing.T) {
	root := t.TempDir()
	h := New(root)
	var out bytes.Buffer
	if _, err := h.Setup(context.Background(), &out, &out, SetupOptions{
		Host: "claude-code", Loops: []string{"skill"}, Principal: "claude@project", ProjectRoot: root,
	}); err != nil {
		t.Fatalf("setup claude skill: %v", err)
	}

	agent := filepath.Join(root, ".claude", "agents", "mnemon-skill-curator.md")
	orig, err := os.ReadFile(agent)
	if err != nil {
		t.Fatalf("subagent not projected: %v", err)
	}
	if err := os.WriteFile(agent, append([]byte("# USER EDIT — keep me\n"), orig...), 0o644); err != nil {
		t.Fatalf("edit subagent: %v", err)
	}

	if err := h.SetupUninstall(context.Background(), &out, &out, SetupOptions{
		Host: "claude-code", Loops: []string{"skill"}, Principal: "claude@project", ProjectRoot: root,
	}); err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	after, err := os.ReadFile(agent)
	if err != nil {
		t.Fatalf("uninstall removed a user-edited subagent: %v", err)
	}
	if !bytes.Contains(after, []byte("USER EDIT")) {
		t.Fatal("uninstall clobbered the user's subagent edit")
	}
}
