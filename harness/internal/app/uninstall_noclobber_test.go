package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Uninstall must not delete a projected skill the user has hand-edited: only skills still ours (hash
// matches what we recorded) are removed; a user-modified one is preserved.
func TestUninstallPreservesUserEditedSkill(t *testing.T) {
	root := t.TempDir()
	h := New(root)
	var out bytes.Buffer
	if _, err := h.Setup(context.Background(), &out, &out, SetupOptions{
		Host: "codex", Loops: []string{"memory"}, Principal: "codex@project", ProjectRoot: root,
	}); err != nil {
		t.Fatalf("setup: %v", err)
	}

	skill := filepath.Join(root, ".codex", "skills", "memory-get", "SKILL.md")
	orig, err := os.ReadFile(skill)
	if err != nil {
		t.Fatalf("projected skill missing: %v", err)
	}
	if err := os.WriteFile(skill, append([]byte("# USER EDIT — keep me\n\n"), orig...), 0o644); err != nil {
		t.Fatalf("edit skill: %v", err)
	}

	if err := h.SetupUninstall(context.Background(), &out, &out, SetupOptions{
		Host: "codex", Loops: []string{"memory"}, Principal: "codex@project", ProjectRoot: root,
	}); err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	after, err := os.ReadFile(skill)
	if err != nil {
		t.Fatalf("uninstall removed a user-edited skill: %v", err)
	}
	if !bytes.Contains(after, []byte("USER EDIT")) {
		t.Fatal("uninstall clobbered the user's skill edit")
	}
}
