package read

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// moduleRoot resolves the repository root from this test file's location so the
// "real data" tests run against the project's own .mnemon (dogfood), regardless
// of the working directory.
func moduleRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve caller path")
	}
	// .../harness/internal/ui/read/snapshot_test.go -> up 5 dirs to module root.
	dir := filepath.Dir(thisFile)
	for i := 0; i < 4; i++ {
		dir = filepath.Dir(dir)
	}
	return dir
}

func TestLoadRealProjectRendersData(t *testing.T) {
	root := moduleRoot(t)
	if _, err := os.Stat(EventLogPath(mustAbs(t, root))); err != nil {
		t.Skipf("no project event log to read: %v", err)
	}
	snap := Load(root)

	if snap.Scope.ProjectRoot == "" {
		t.Error("scope project root should be set")
	}
	if snap.Scope.EventLogPath == "" {
		t.Error("scope event log path should be set")
	}
	if snap.Err.Events != nil {
		t.Errorf("events should load from the real project: %v", snap.Err.Events)
	}
	if len(snap.Events) == 0 {
		t.Error("expected real events in the project log")
	}
	// The project carries draft proposals; proposals must load without error.
	if snap.Err.Proposals != nil {
		t.Errorf("proposals should load: %v", snap.Err.Proposals)
	}
	if len(snap.Proposals) == 0 {
		t.Error("expected real proposals in the project")
	}
	// Goals dir exists (we dogfood this goal), so goals must load.
	if snap.Err.Goals != nil {
		t.Errorf("goals should load: %v", snap.Err.Goals)
	}
	// Events are newest-first.
	for i := 1; i < len(snap.Events); i++ {
		if snap.Events[i-1].TS < snap.Events[i].TS {
			t.Errorf("events not newest-first at %d: %q then %q", i, snap.Events[i-1].TS, snap.Events[i].TS)
			break
		}
	}
}

// TestMissingEventLogDegradesOnlyEvents proves a missing store file degrades only
// its own section: with no .mnemon at all, Load still returns a usable snapshot
// and only the affected sections carry errors.
func TestMissingEventLogDegradesOnlyEvents(t *testing.T) {
	tmp := t.TempDir()
	snap := Load(tmp)

	if snap.Err.Events == nil {
		t.Error("missing events.jsonl should set the Events error")
	}
	if snap.Scope.ProjectRoot == "" {
		t.Error("scope should still be derived (project root) despite missing stores")
	}
	// Proposals over a fresh root return an empty (not errored) list — the section
	// degrades gracefully to empty, not to a crash.
	if len(snap.Proposals) != 0 {
		t.Errorf("expected no proposals in a fresh root, got %d", len(snap.Proposals))
	}
}

// TestEventParseIsolation proves a single malformed JSONL line is skipped rather
// than failing the whole stream.
func TestEventParseIsolation(t *testing.T) {
	tmp := t.TempDir()
	mnemon := filepath.Join(tmp, ".mnemon")
	if err := os.MkdirAll(mnemon, 0o755); err != nil {
		t.Fatal(err)
	}
	good := `{"schema_version":1,"id":"evt_a","ts":"2026-05-30T00:00:00Z","type":"session.started","loop":null,"host":null,"actor":"user","source":"test","correlation_id":"c","caused_by":null,"payload":{}}`
	content := good + "\n{ this is not json }\n\n"
	if err := os.WriteFile(filepath.Join(mnemon, "events.jsonl"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	snap := Load(tmp)
	if snap.Err.Events != nil {
		t.Fatalf("events should load: %v", snap.Err.Events)
	}
	if len(snap.Events) != 1 {
		t.Fatalf("expected 1 parsed event (garbage line skipped), got %d", len(snap.Events))
	}
	if snap.Events[0].ID != "evt_a" {
		t.Errorf("unexpected event id %q", snap.Events[0].ID)
	}
	if snap.Scope.LastWriteback != "2026-05-30T00:00:00Z" {
		t.Errorf("last writeback should reflect newest event ts, got %q", snap.Scope.LastWriteback)
	}
}

// TestPassiveLoadWritesNoReport proves the read-only health wiring (audit
// integrity + anti-pattern status) never writes to the project: a passive refresh
// must not emit the anti-pattern report file that the explicit scan produces.
func TestPassiveLoadWritesNoReport(t *testing.T) {
	tmp := t.TempDir()
	_ = Load(tmp)
	_ = Load(tmp) // a second refresh must also stay read-only
	reportDir := filepath.Join(tmp, ".mnemon", "harness", "reports", "antipattern")
	if entries, err := os.ReadDir(reportDir); err == nil && len(entries) > 0 {
		t.Fatalf("passive Load wrote %d anti-pattern report file(s); refresh must be read-only", len(entries))
	}
}

func mustAbs(t *testing.T, p string) string {
	t.Helper()
	a, err := filepath.Abs(p)
	if err != nil {
		t.Fatal(err)
	}
	return a
}
