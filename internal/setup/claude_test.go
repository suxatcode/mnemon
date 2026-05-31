package setup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPromptDirHonorsMnemonDataDir(t *testing.T) {
	custom := t.TempDir()
	t.Setenv("MNEMON_DATA_DIR", custom)

	got, err := promptDir()
	if err != nil {
		t.Fatalf("promptDir: %v", err)
	}
	want := filepath.Join(custom, "prompt")
	if got != want {
		t.Fatalf("promptDir = %q, want %q", got, want)
	}
}

func TestPromptDirFallsBackToHomeWhenEnvUnset(t *testing.T) {
	t.Setenv("MNEMON_DATA_DIR", "")
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)

	got, err := promptDir()
	if err != nil {
		t.Fatalf("promptDir: %v", err)
	}
	want := filepath.Join(fakeHome, ".mnemon", "prompt")
	if got != want {
		t.Fatalf("promptDir = %q, want %q", got, want)
	}
}

func TestWritePromptFilesWritesUnderMnemonDataDir(t *testing.T) {
	custom := t.TempDir()
	t.Setenv("MNEMON_DATA_DIR", custom)

	path, err := WritePromptFiles()
	if err != nil {
		t.Fatalf("WritePromptFiles: %v", err)
	}

	wantDir := filepath.Join(custom, "prompt")
	if path != wantDir {
		t.Fatalf("returned path = %q, want %q", path, wantDir)
	}

	for _, name := range []string{"guide.md", "skill.md"} {
		full := filepath.Join(wantDir, name)
		info, err := os.Stat(full)
		if err != nil {
			t.Fatalf("stat %s: %v", full, err)
		}
		if info.Size() == 0 {
			t.Fatalf("%s is empty", full)
		}
	}
}
