package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

// The runtime-surface env.sh is a managed file too: install must not clobber a pre-existing one, and
// uninstall must not delete a user-edited one. (It was written with a raw writeFile — no recorded hash
// — so removeManagedTree deleted it unconditionally and install overwrote it.)
func TestRuntimeSurfaceEnvNoClobber(t *testing.T) {
	root := t.TempDir()
	h := New(root)
	var out bytes.Buffer

	// A pre-existing env.sh at the runtime surface must survive the first install.
	surf := filepath.Join(root, ".codex", "mnemon-memory")
	if err := os.MkdirAll(surf, 0o755); err != nil {
		t.Fatal(err)
	}
	env := filepath.Join(surf, "env.sh")
	if err := os.WriteFile(env, []byte("# PRE-EXISTING USER ENV\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Setup(context.Background(), &out, &out, SetupOptions{
		Host: "codex", Loops: []string{"memory"}, Principal: "codex@project", ProjectRoot: root,
	}); err != nil {
		t.Fatalf("setup: %v", err)
	}
	data, err := os.ReadFile(env)
	if err != nil || !bytes.Contains(data, []byte("PRE-EXISTING USER ENV")) {
		t.Fatalf("install clobbered a pre-existing runtime env.sh (data=%q err=%v)", data, err)
	}

	// In a clean project, an edited (Mnemon-written, then hand-edited) env.sh must survive uninstall.
	root2 := t.TempDir()
	h2 := New(root2)
	if _, err := h2.Setup(context.Background(), &out, &out, SetupOptions{
		Host: "codex", Loops: []string{"memory"}, Principal: "codex@project", ProjectRoot: root2,
	}); err != nil {
		t.Fatalf("setup2: %v", err)
	}
	env2 := filepath.Join(root2, ".codex", "mnemon-memory", "env.sh")
	orig, err := os.ReadFile(env2)
	if err != nil {
		t.Fatalf("runtime env not projected: %v", err)
	}
	if err := os.WriteFile(env2, append([]byte("# USER EDIT — keep me\n"), orig...), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := h2.SetupUninstall(context.Background(), &out, &out, SetupOptions{
		Host: "codex", Loops: []string{"memory"}, Principal: "codex@project", ProjectRoot: root2,
	}); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	after, err := os.ReadFile(env2)
	if err != nil || !bytes.Contains(after, []byte("USER EDIT")) {
		t.Fatalf("uninstall removed/clobbered a user-edited runtime env.sh (data=%q err=%v)", after, err)
	}
}
