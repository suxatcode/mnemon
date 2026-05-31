package status

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/coordination"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/eventlog"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/layout"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
)

func TestRefreshWritesStatusesReferencingEventIDs(t *testing.T) {
	root := t.TempDir()
	store, err := eventlog.New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	for _, event := range []schema.Event{
		fixtureEvent("evt_memory", "memory.hot_write_observed", "memory", "codex", map[string]any{"reason": "fixture"}),
		fixtureEvent("evt_skill", "skill.usage_observed", "skill", "codex", map[string]any{"reason": "fixture"}),
		fixtureEvent("evt_eval", "eval.run_observed", "eval", "codex", map[string]any{"reason": "fixture"}),
		fixtureEvent("evt_projection", "projection.drift_observed", "memory", "codex", map[string]any{"binding": "codex.memory"}),
		fixtureEvent("evt_proposal", "proposal.created", "memory", "codex", map[string]any{"proposal_id": "prop_memory"}),
		fixtureEvent("evt_audit", "audit.recorded", "memory", "codex", map[string]any{"audit_id": "audit_memory"}),
		fixtureEvent("evt_noop", "reconcile.noop", "memory", "codex", map[string]any{"reason": "current"}),
		fixtureEvent("evt_failed", "job.failed", "eval", "codex", map[string]any{"job_id": "job_eval", "reason": "fixture failure"}),
	} {
		if err := store.Append(event); err != nil {
			t.Fatalf("append %s: %v", event.ID, err)
		}
	}

	now := time.Date(2026, 5, 24, 8, 40, 0, 0, time.UTC)
	result, err := Refresh(root, now)
	if err != nil {
		t.Fatalf("Refresh returned error: %v", err)
	}
	if result.EventCount != 8 {
		t.Fatalf("event count mismatch: %d", result.EventCount)
	}
	if result.LastIncludedEventID != "evt_failed" {
		t.Fatalf("last event id mismatch: %q", result.LastIncludedEventID)
	}
	for _, rel := range []string{
		"project.json",
		filepath.Join("loops", "memory.json"),
		filepath.Join("loops", "skill.json"),
		filepath.Join("loops", "eval.json"),
		filepath.Join("hosts", "codex.json"),
		filepath.Join("jobs", "job_eval.json"),
		filepath.Join("projections", "codex.memory.json"),
	} {
		assertStatusEventRef(t, filepath.Join(root, ".mnemon", "harness", "status", rel))
	}
}

func TestRefreshWithNoEventsIsNoop(t *testing.T) {
	result, err := Refresh(t.TempDir(), time.Now().UTC())
	if err != nil {
		t.Fatalf("Refresh returned error: %v", err)
	}
	if result.EventCount != 0 || len(result.Written) != 0 {
		t.Fatalf("expected no-op refresh, got %#v", result)
	}
}

func TestRefreshMaterializesDaemonAndRunnerStatus(t *testing.T) {
	root := t.TempDir()
	paths, err := layout.EnsureProject(root)
	if err != nil {
		t.Fatalf("EnsureProject returned error: %v", err)
	}
	store, err := eventlog.New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	for _, event := range []schema.Event{
		fixtureSystemEvent("evt_daemon_ready", "daemon.phase_changed", nil, map[string]any{
			"from_phase":              "",
			"to_phase":                "ready",
			"reason":                  "TickCompleted",
			"last_processed_event_id": "evt_runner_ready",
		}),
		fixtureSystemEvent("evt_runner_ready", "runner.readiness_passed", ptr("codex"), map[string]any{
			"runner_id":  "codex-app-server",
			"run_id":     "ready",
			"from_phase": "",
			"to_phase":   "ready",
			"report_ref": map[string]any{"uri": ".mnemon/harness/reports/runner/ready.json"},
		}),
	} {
		if err := store.Append(event); err != nil {
			t.Fatalf("append %s: %v", event.ID, err)
		}
	}
	if err := os.WriteFile(filepath.Join(paths.HarnessDir, "daemon", "tick-log.jsonl"), []byte(`{"schema_version":1,"tick_id":"tick-ready","status":"completed","jobs_processed":2}`+"\n"), 0o644); err != nil {
		t.Fatalf("write tick log: %v", err)
	}
	if err := os.RemoveAll(paths.StatusDir); err != nil {
		t.Fatalf("remove status dir: %v", err)
	}

	result, err := Refresh(root, time.Date(2026, 5, 24, 8, 40, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Refresh returned error: %v", err)
	}
	if result.EventCount != 2 {
		t.Fatalf("event count mismatch: %d", result.EventCount)
	}
	assertStatusPhase(t, filepath.Join(root, ".mnemon", "harness", "status", "daemon.json"), "DaemonStatus", "ready")
	assertStatusPhase(t, filepath.Join(root, ".mnemon", "harness", "status", "runners", "codex-app-server.json"), "RunnerStatus", "ready")
}

func TestRefreshFullLifecycleFixture(t *testing.T) {
	root := t.TempDir()
	paths, err := layout.EnsureProject(root)
	if err != nil {
		t.Fatalf("EnsureProject returned error: %v", err)
	}
	fixture, err := os.ReadFile(filepath.Join("..", "testdata", "full_lifecycle_events.jsonl"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := os.WriteFile(paths.EventLog, fixture, 0o644); err != nil {
		t.Fatalf("write fixture event log: %v", err)
	}

	result, err := Refresh(root, time.Date(2026, 5, 24, 8, 40, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Refresh returned error: %v", err)
	}
	if result.EventCount != 8 {
		t.Fatalf("event count mismatch: %d", result.EventCount)
	}
	for _, rel := range []string{
		filepath.Join("loops", "memory.json"),
		filepath.Join("loops", "skill.json"),
		filepath.Join("loops", "eval.json"),
		filepath.Join("projections", "codex.memory.json"),
		filepath.Join("jobs", "job_fixture_failure.json"),
	} {
		assertStatusEventRef(t, filepath.Join(root, ".mnemon", "harness", "status", rel))
	}
}

func TestDeriveScope(t *testing.T) {
	// Events arrive oldest-first as the log returns them. The older event carries a
	// full scope map; the newer one carries none, so its host/loop must fall back to
	// the event's own fields and take precedence (newest-first walk).
	older := fixtureEvent("evt_old", "memory.hot_write_observed", "memory", "codex", map[string]any{})
	older.TS = "2026-05-24T08:00:00Z"
	older.Scope = schema.ProjectScopeWithProfile("/repo", "default", "codex", "memory", "personal-default").Map()

	newer := fixtureEvent("evt_new", "session.started", "skill", "claude-code", map[string]any{})
	newer.TS = "2026-05-24T09:00:00Z"

	scope := DeriveScope([]schema.Event{older, newer})

	if scope.LastWriteback != "2026-05-24T09:00:00Z" {
		t.Errorf("last_writeback = %q, want newest event ts", scope.LastWriteback)
	}
	// Newest event wins host/loop (from its own fields, lacking a scope map).
	if scope.Host != "claude-code" {
		t.Errorf("host = %q, want claude-code (newest event)", scope.Host)
	}
	if scope.Loop != "skill" {
		t.Errorf("loop = %q, want skill (newest event)", scope.Loop)
	}
	// store/profile/binding only exist on the older event; the walk fills them down.
	if scope.Store != "default" {
		t.Errorf("store = %q, want default (older event scope)", scope.Store)
	}
	if scope.ProfileRef != "personal-default" {
		t.Errorf("profile_ref = %q, want personal-default", scope.ProfileRef)
	}
	if scope.BindingScope != "project" {
		t.Errorf("binding_scope = %q, want project", scope.BindingScope)
	}
}

func TestDeriveScopeEmpty(t *testing.T) {
	if got := DeriveScope(nil); got != (Scope{}) {
		t.Errorf("empty events should derive empty scope, got %#v", got)
	}
}

// TestRefreshMaterializesBothHosts proves the "both pull projection" half of the
// Band 1 substrate: events from two host identities on one ledger each
// materialize their own host status document referencing their own events.
func TestRefreshMaterializesBothHosts(t *testing.T) {
	root := t.TempDir()
	store, err := eventlog.New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	for _, event := range []schema.Event{
		fixtureEvent("evt_codex", "memory.hot_write_observed", "memory", "codex", map[string]any{"reason": "fixture"}),
		fixtureEvent("evt_claude", "memory.hot_write_observed", "memory", "claude-code", map[string]any{"reason": "fixture"}),
	} {
		if err := store.Append(event); err != nil {
			t.Fatalf("append %s: %v", event.ID, err)
		}
	}
	if _, err := Refresh(root, time.Date(2026, 5, 30, 8, 40, 0, 0, time.UTC)); err != nil {
		t.Fatalf("Refresh returned error: %v", err)
	}
	for _, host := range []string{"codex", "claude-code"} {
		assertStatusEventRef(t, filepath.Join(root, ".mnemon", "harness", "status", "hosts", host+".json"))
	}
}

// TestHostScopeCarriesEndToEnd proves per-host identity flows append → log →
// ReadAll → derived scope: the newest writer's host/loop is the live scope.
func TestHostScopeCarriesEndToEnd(t *testing.T) {
	root := t.TempDir()
	store, err := eventlog.New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	for _, event := range []schema.Event{
		fixtureEvent("evt_codex", "memory.hot_write_observed", "memory", "codex", map[string]any{"reason": "x"}),
		fixtureEvent("evt_claude", "skill.usage_observed", "skill", "claude-code", map[string]any{"reason": "y"}),
	} {
		if err := store.Append(event); err != nil {
			t.Fatalf("append %s: %v", event.ID, err)
		}
	}
	events, err := store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	scope := DeriveScope(events)
	if scope.Host != "claude-code" {
		t.Errorf("scope host = %q, want claude-code (newest writer)", scope.Host)
	}
	if scope.Loop != "skill" {
		t.Errorf("scope loop = %q, want skill (newest writer)", scope.Loop)
	}
}

// TestRefreshMaterializesCoordination proves the coordination topology is
// materialized in the status projection when collaboration events exist.
func TestRefreshMaterializesCoordination(t *testing.T) {
	root := t.TempDir()
	store, err := eventlog.New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	for _, event := range []schema.Event{
		fixtureEvent("evt_claim", coordination.EventTaskClaimed, "coordination", "codex", map[string]any{coordination.FieldTaskID: "T1"}),
		fixtureEvent("evt_fork", coordination.EventTaskForked, "coordination", "claude-code", map[string]any{coordination.FieldTaskID: "T2", coordination.FieldForkedFrom: "T1"}),
	} {
		if err := store.Append(event); err != nil {
			t.Fatalf("append %s: %v", event.ID, err)
		}
	}
	if _, err := Refresh(root, time.Date(2026, 5, 30, 8, 40, 0, 0, time.UTC)); err != nil {
		t.Fatalf("Refresh returned error: %v", err)
	}
	path := filepath.Join(root, ".mnemon", "harness", "status", "coordination.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("coordination topology not materialized: %v", err)
	}
	for _, want := range []string{"CoordinationStatus", "T1", "T2", "forked_from"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("coordination doc missing %q:\n%s", want, data)
		}
	}
}

func assertStatusEventRef(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read status %s: %v", path, err)
	}
	var doc struct {
		Status struct {
			LastIncludedEventID string `json:"last_included_event_id"`
			Conditions          []struct {
				LastEventID string `json:"last_event_id"`
			} `json:"conditions"`
		} `json:"status"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("decode status %s: %v", path, err)
	}
	if doc.Status.LastIncludedEventID == "" {
		t.Fatalf("status %s missing last_included_event_id", path)
	}
	if len(doc.Status.Conditions) == 0 || doc.Status.Conditions[0].LastEventID == "" {
		t.Fatalf("status %s missing condition last_event_id", path)
	}
}

func fixtureEvent(id, typ, loop, host string, payload map[string]any) schema.Event {
	return schema.Event{
		SchemaVersion: 1,
		ID:            id,
		TS:            "2026-05-24T08:30:00Z",
		Type:          typ,
		Loop:          &loop,
		Host:          &host,
		Actor:         "host-agent",
		Source:        "fixture",
		CorrelationID: "corr_fixture",
		CausedBy:      nil,
		Payload:       payload,
	}
}

func fixtureSystemEvent(id, typ string, host *string, payload map[string]any) schema.Event {
	var actor string
	var source string
	switch {
	case strings.HasPrefix(typ, "daemon."):
		actor = "mnemon-daemon"
		source = "daemon"
	default:
		actor = "host-runner"
		source = "codex.app-server"
	}
	return schema.Event{
		SchemaVersion: 1,
		ID:            id,
		TS:            "2026-05-24T08:30:00Z",
		Type:          typ,
		Loop:          nil,
		Host:          host,
		Actor:         actor,
		Source:        source,
		CorrelationID: "corr_fixture",
		CausedBy:      nil,
		Payload:       payload,
	}
}

func assertStatusPhase(t *testing.T, path, kind, phase string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read status %s: %v", path, err)
	}
	var doc struct {
		Kind   string `json:"kind"`
		Status struct {
			Phase               string         `json:"phase"`
			LastIncludedEventID string         `json:"last_included_event_id"`
			LastTick            map[string]any `json:"last_tick,omitempty"`
		} `json:"status"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("decode status %s: %v", path, err)
	}
	if doc.Kind != kind || doc.Status.Phase != phase || doc.Status.LastIncludedEventID == "" {
		t.Fatalf("unexpected status %s: %#v", path, doc)
	}
}

func ptr(value string) *string {
	return &value
}
