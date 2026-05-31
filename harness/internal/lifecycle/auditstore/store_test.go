package auditstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/eventlog"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
)

func TestStoreWritesAuditAndRecordedEvent(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	now := time.Date(2026, 5, 27, 8, 30, 0, 0, time.UTC)
	written, err := store.Write(WriteOptions{
		ID: "audit-run-001",
		Spec: map[string]any{
			"job_id":   "job_memory",
			"decision": "retain evidence",
		},
	})
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if written.Audit.Metadata.Name != "audit-run-001" {
		t.Fatalf("unexpected audit metadata: %#v", written.Audit.Metadata)
	}
	if written.Ref["uri"] != filepath.Join(".mnemon", "harness", "audit", "records", "audit-run-001.json") {
		t.Fatalf("unexpected audit ref: %#v", written.Ref)
	}
	assertExists(t, written.Path)

	var audit schema.Audit
	data, err := os.ReadFile(written.Path)
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if err := json.Unmarshal(data, &audit); err != nil {
		t.Fatalf("decode audit: %v", err)
	}
	if err := schema.ValidateAudit(audit); err != nil {
		t.Fatalf("audit failed validation: %v", err)
	}

	event, err := store.AppendRecordedEvent(RecordedEventOptions{
		ID:            "evt_audit_run_001_recorded",
		Now:           now,
		Loop:          "memory",
		Host:          "codex",
		Source:        "codex.app-server",
		CorrelationID: "run-001",
		CausedBy:      "evt_run_001_completed",
		Payload: map[string]any{
			"job_id": "job_memory",
		},
		AuditRef: written.Ref,
	})
	if err != nil {
		t.Fatalf("AppendRecordedEvent returned error: %v", err)
	}
	if event.Type != "audit.recorded" || event.AuditRef["uri"] != written.Ref["uri"] {
		t.Fatalf("unexpected audit event: %#v", event)
	}
	loaded, err := store.Load("audit-run-001")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if loaded.Audit.Metadata.Name != written.Audit.Metadata.Name {
		t.Fatalf("loaded audit mismatch: %#v", loaded.Audit)
	}
	records, err := store.List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(records) != 1 || records[0].Audit.Metadata.Name != "audit-run-001" {
		t.Fatalf("unexpected audit records: %#v", records)
	}

	events, err := eventlog.New(root)
	if err != nil {
		t.Fatalf("eventlog.New returned error: %v", err)
	}
	allEvents, err := events.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	if len(allEvents) != 1 || allEvents[0].ID != event.ID {
		t.Fatalf("unexpected events: %#v", allEvents)
	}
	issues, err := store.VerifyIntegrity()
	if err != nil {
		t.Fatalf("VerifyIntegrity returned error: %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("expected no integrity issues, got %#v", issues)
	}
}

func TestStoreRejectsInvalidAudit(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if _, err := store.Write(WriteOptions{ID: "invalid"}); err == nil {
		t.Fatal("expected invalid audit error")
	}
}

func TestVerifyIntegrityDetectsMissingAuditRecord(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	written, err := store.Write(WriteOptions{
		ID: "audit-missing",
		Spec: map[string]any{
			"decision": "recorded then deleted",
		},
	})
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if _, err := store.AppendRecordedEvent(RecordedEventOptions{
		ID:       "evt_audit_missing_recorded",
		AuditRef: written.Ref,
		Payload:  map[string]any{"audit_id": "audit-missing"},
	}); err != nil {
		t.Fatalf("AppendRecordedEvent returned error: %v", err)
	}
	if err := os.Remove(written.Path); err != nil {
		t.Fatalf("remove audit record: %v", err)
	}
	issues, err := store.VerifyIntegrity()
	if err != nil {
		t.Fatalf("VerifyIntegrity returned error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 integrity issue, got %#v", issues)
	}
	if issues[0].Kind != "missing_audit_record" || issues[0].EventID != "evt_audit_missing_recorded" {
		t.Fatalf("unexpected integrity issue: %#v", issues[0])
	}
}

func TestVerifyIntegrityDetectsUnrecordedAuditRecord(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if _, err := store.Write(WriteOptions{
		ID: "audit-unrecorded",
		Spec: map[string]any{
			"decision": "record without audit.recorded event",
		},
	}); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	issues, err := store.VerifyIntegrity()
	if err != nil {
		t.Fatalf("VerifyIntegrity returned error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 integrity issue, got %#v", issues)
	}
	if issues[0].Kind != "unrecorded_audit_record" || issues[0].AuditID != "audit-unrecorded" {
		t.Fatalf("unexpected integrity issue: %#v", issues[0])
	}
}

func assertExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}
