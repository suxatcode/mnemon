package layout

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Paths struct {
	Root         string
	MnemonDir    string
	EventLog     string
	HarnessDir   string
	StatusDir    string
	ReportsDir   string
	ArtifactsDir string
	JobsDir      string
	TmpDir       string
}

func Resolve(root string) (Paths, error) {
	if root == "" {
		root = "."
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return Paths{}, fmt.Errorf("resolve project root: %w", err)
	}
	abs = filepath.Clean(abs)
	mnemon := filepath.Join(abs, ".mnemon")
	harness := filepath.Join(mnemon, "harness")
	return Paths{
		Root:         abs,
		MnemonDir:    mnemon,
		EventLog:     filepath.Join(mnemon, "events.jsonl"),
		HarnessDir:   harness,
		StatusDir:    filepath.Join(harness, "status"),
		ReportsDir:   filepath.Join(harness, "reports"),
		ArtifactsDir: filepath.Join(harness, "artifacts"),
		JobsDir:      filepath.Join(harness, "jobs"),
		TmpDir:       filepath.Join(harness, "tmp"),
	}, nil
}

func EnsureProject(root string) (Paths, error) {
	paths, err := Resolve(root)
	if err != nil {
		return Paths{}, err
	}
	if err := os.MkdirAll(paths.Root, 0o755); err != nil {
		return Paths{}, fmt.Errorf("create project root: %w", err)
	}
	for _, dir := range requiredDirs(paths) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return Paths{}, fmt.Errorf("create %s: %w", dir, err)
		}
	}
	if err := ensureFile(paths.EventLog, nil, 0o644); err != nil {
		return Paths{}, err
	}
	readme := filepath.Join(paths.HarnessDir, "README.md")
	if err := ensureFile(readme, []byte("# Mnemon Lifecycle Harness\n\nExperimental project-local lifecycle state.\n"), 0o644); err != nil {
		return Paths{}, err
	}
	return paths, nil
}

func requiredDirs(paths Paths) []string {
	return []string{
		paths.MnemonDir,
		paths.HarnessDir,
		filepath.Join(paths.HarnessDir, "bindings"),
		filepath.Join(paths.HarnessDir, "loops", "memory", "state"),
		filepath.Join(paths.HarnessDir, "loops", "memory", "reports"),
		filepath.Join(paths.HarnessDir, "loops", "skill", "state"),
		filepath.Join(paths.HarnessDir, "loops", "skill", "reports"),
		filepath.Join(paths.HarnessDir, "loops", "skill", "proposals"),
		filepath.Join(paths.HarnessDir, "loops", "eval", "state"),
		filepath.Join(paths.HarnessDir, "loops", "eval", "reports"),
		filepath.Join(paths.HarnessDir, "loops", "eval", "artifacts"),
		filepath.Join(paths.HarnessDir, "hosts"),
		filepath.Join(paths.StatusDir, "loops"),
		filepath.Join(paths.StatusDir, "hosts"),
		filepath.Join(paths.StatusDir, "projections"),
		filepath.Join(paths.StatusDir, "jobs"),
		filepath.Join(paths.StatusDir, "goals"),
		filepath.Join(paths.StatusDir, "runners"),
		filepath.Join(paths.ReportsDir, "validation"),
		filepath.Join(paths.ReportsDir, "projection"),
		filepath.Join(paths.ReportsDir, "eval"),
		filepath.Join(paths.ReportsDir, "reconcile"),
		filepath.Join(paths.ReportsDir, "runner"),
		filepath.Join(paths.HarnessDir, "proposals", "draft"),
		filepath.Join(paths.HarnessDir, "proposals", "open"),
		filepath.Join(paths.HarnessDir, "proposals", "in_review"),
		filepath.Join(paths.HarnessDir, "proposals", "approved"),
		filepath.Join(paths.HarnessDir, "proposals", "rejected"),
		filepath.Join(paths.HarnessDir, "proposals", "request_changes"),
		filepath.Join(paths.HarnessDir, "proposals", "blocked"),
		filepath.Join(paths.HarnessDir, "proposals", "applied"),
		filepath.Join(paths.HarnessDir, "proposals", "superseded"),
		filepath.Join(paths.HarnessDir, "proposals", "withdrawn"),
		filepath.Join(paths.HarnessDir, "proposals", "expired"),
		filepath.Join(paths.HarnessDir, "profiles"),
		filepath.Join(paths.HarnessDir, "audit", "records"),
		filepath.Join(paths.HarnessDir, "goals"),
		filepath.Join(paths.HarnessDir, "daemon"),
		filepath.Join(paths.JobsDir, "queued"),
		filepath.Join(paths.JobsDir, "requested"),
		filepath.Join(paths.JobsDir, "running"),
		filepath.Join(paths.JobsDir, "completed"),
		filepath.Join(paths.JobsDir, "failed"),
		filepath.Join(paths.JobsDir, "blocked"),
		filepath.Join(paths.JobsDir, "skipped"),
		filepath.Join(paths.ArtifactsDir, "memory"),
		filepath.Join(paths.ArtifactsDir, "skill"),
		filepath.Join(paths.ArtifactsDir, "eval"),
		filepath.Join(paths.ArtifactsDir, "projection"),
		filepath.Join(paths.ArtifactsDir, "runner"),
		filepath.Join(paths.HarnessDir, "runs", "codex-app-server"),
		paths.TmpDir,
	}
}

func ensureFile(path string, contents []byte, mode os.FileMode) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create parent for %s: %w", path, err)
	}
	if err := os.WriteFile(path, contents, mode); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// WriteJSONAtomic marshals value as indented JSON with a trailing newline and
// writes it to path atomically (temp file + rename), creating parent dirs. The
// final file is set to perm. This is the shared implementation for the lifecycle
// stores' per-file JSON persistence.
func WriteJSONAtomic(path string, value any, perm os.FileMode) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create parent for %s: %w", path, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp for %s: %w", path, err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write temp %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close temp %s: %w", tmpPath, err)
	}
	if err := os.Chmod(tmpPath, perm); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("chmod temp %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}

// NormalizeNow returns now in UTC, substituting the current time when now is the
// zero value. This is the shared timestamp primitive for lifecycle stores that
// stamp records at write time. Stores needing a different rounding (e.g.
// proposalstore truncates to whole seconds for deterministic event IDs) keep
// their own local variant rather than reusing this one.
func NormalizeNow(now time.Time) time.Time {
	if now.IsZero() {
		return time.Now().UTC()
	}
	return now.UTC()
}

// TimestampID renders now as a sortable, UTC, nanosecond-precision timestamp
// suitable for composing deterministic record and event IDs.
func TimestampID(now time.Time) string {
	return now.UTC().Format("20060102T150405000000000")
}
