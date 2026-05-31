package layout

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureProjectCreatesMinimumLayout(t *testing.T) {
	root := t.TempDir()
	paths, err := EnsureProject(root)
	if err != nil {
		t.Fatalf("EnsureProject returned error: %v", err)
	}

	for _, path := range []string{
		paths.EventLog,
		filepath.Join(paths.HarnessDir, "README.md"),
		filepath.Join(paths.HarnessDir, "bindings"),
		filepath.Join(paths.HarnessDir, "loops", "memory", "state"),
		filepath.Join(paths.HarnessDir, "loops", "skill", "proposals"),
		filepath.Join(paths.HarnessDir, "loops", "eval", "artifacts"),
		filepath.Join(paths.StatusDir, "loops"),
		filepath.Join(paths.StatusDir, "hosts"),
		filepath.Join(paths.StatusDir, "jobs"),
		filepath.Join(paths.HarnessDir, "proposals", "draft"),
		filepath.Join(paths.HarnessDir, "audit", "records"),
		filepath.Join(paths.JobsDir, "requested"),
		filepath.Join(paths.ArtifactsDir, "projection"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s to exist: %v", path, err)
		}
	}
}

func TestEnsureProjectIsIdempotent(t *testing.T) {
	root := t.TempDir()
	paths, err := EnsureProject(root)
	if err != nil {
		t.Fatalf("EnsureProject returned error: %v", err)
	}
	if err := os.WriteFile(paths.EventLog, []byte(""), 0o644); err != nil {
		t.Fatalf("write event log: %v", err)
	}
	if _, err := EnsureProject(root); err != nil {
		t.Fatalf("EnsureProject second run returned error: %v", err)
	}
}

func TestWriteJSONAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "out.json")

	if err := WriteJSONAtomic(path, map[string]any{"k": "v"}, 0o600); err != nil {
		t.Fatalf("WriteJSONAtomic returned error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	const want = "{\n  \"k\": \"v\"\n}\n"
	if string(data) != want {
		t.Fatalf("content mismatch: want %q got %q", want, string(data))
	}
	if info, err := os.Stat(path); err != nil {
		t.Fatalf("stat: %v", err)
	} else if info.Mode().Perm() != 0o600 {
		t.Errorf("perm: want 0600 got %o", info.Mode().Perm())
	}

	// Overwrite atomically with a different perm; the temp file must not linger.
	if err := WriteJSONAtomic(path, map[string]any{"k": "v2"}, 0o644); err != nil {
		t.Fatalf("second WriteJSONAtomic returned error: %v", err)
	}
	if data, _ := os.ReadFile(path); string(data) != "{\n  \"k\": \"v2\"\n}\n" {
		t.Fatalf("overwrite content mismatch: got %q", string(data))
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o644 {
		t.Errorf("overwrite perm: want 0644 got %o", info.Mode().Perm())
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected only the final file, got %d entries (temp leftover?)", len(entries))
	}
}
