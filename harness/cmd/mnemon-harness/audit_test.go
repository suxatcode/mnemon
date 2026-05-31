package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/auditstore"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/eventlog"
)

func TestAuditCommandSmoke(t *testing.T) {
	root := t.TempDir()
	restoreAuditFlags(t)
	auditRoot = root
	auditID = "audit-cli-smoke"
	auditKind = "eval"
	auditDecision = "retain eval run evidence"
	auditReason = "CLI smoke"
	auditProposalRefs = []string{"proposal:eval-smoke"}
	auditEventRefs = []string{"evt_eval_smoke"}
	auditArtifactRefs = []string{".mnemon/harness/reports/eval-smoke.json"}
	auditEventID = "evt_audit_cli_smoke_recorded"
	auditLoop = "eval"
	auditHost = "codex"
	auditCorrelationID = "corr_audit_cli"

	appendCmd, appendOutput := testCommand()
	if err := runAuditAppend(appendCmd, nil); err != nil {
		t.Fatalf("runAuditAppend returned error: %v", err)
	}
	if !strings.Contains(appendOutput.String(), "appended audit audit-cli-smoke") {
		t.Fatalf("unexpected append output: %s", appendOutput.String())
	}
	if _, err := os.Stat(filepath.Join(root, ".mnemon", "harness", "audit", "records", "audit-cli-smoke.json")); err != nil {
		t.Fatalf("expected audit file: %v", err)
	}

	listCmd, listOutput := testCommand()
	clearAuditQueryFlags()
	auditRoot = root
	auditListKind = "eval"
	if err := runAuditList(listCmd, nil); err != nil {
		t.Fatalf("runAuditList returned error: %v", err)
	}
	if !strings.Contains(listOutput.String(), "audit-cli-smoke") || !strings.Contains(listOutput.String(), "retain eval run evidence") {
		t.Fatalf("unexpected list output: %s", listOutput.String())
	}

	showCmd, showOutput := testCommand()
	clearAuditQueryFlags()
	auditRoot = root
	auditID = "audit-cli-smoke"
	if err := runAuditShow(showCmd, nil); err != nil {
		t.Fatalf("runAuditShow returned error: %v", err)
	}
	if !strings.Contains(showOutput.String(), "proposal_refs: 1") {
		t.Fatalf("unexpected show output: %s", showOutput.String())
	}

	store, err := eventlog.New(root)
	if err != nil {
		t.Fatalf("eventlog.New returned error: %v", err)
	}
	events, err := store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	if len(events) != 1 || events[0].Type != "audit.recorded" {
		t.Fatalf("unexpected audit events: %#v", events)
	}

	clearAuditQueryFlags()
	auditRoot = root
	auditID = "audit-cli-smoke"
	auditDecision = "duplicate should fail"
	err = runAuditAppend(mustTestCommand(t), nil)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected duplicate audit error, got %v", err)
	}
}

func TestAuditShowMissing(t *testing.T) {
	root := t.TempDir()
	restoreAuditFlags(t)
	auditRoot = root
	auditID = "missing"
	err := runAuditShow(mustTestCommand(t), nil)
	if !errors.Is(err, auditstore.ErrAuditNotFound) {
		t.Fatalf("expected ErrAuditNotFound, got %v", err)
	}
}

func TestAuditVerifyDetectsMissingRecordedAudit(t *testing.T) {
	root := t.TempDir()
	restoreAuditFlags(t)
	store, err := auditstore.New(root)
	if err != nil {
		t.Fatalf("auditstore.New returned error: %v", err)
	}
	written, err := store.Write(auditstore.WriteOptions{
		ID: "audit-cli-missing",
		Spec: map[string]any{
			"decision": "recorded then deleted",
		},
	})
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if _, err := store.AppendRecordedEvent(auditstore.RecordedEventOptions{
		ID:       "evt_audit_cli_missing_recorded",
		AuditRef: written.Ref,
		Payload:  map[string]any{"audit_id": "audit-cli-missing"},
	}); err != nil {
		t.Fatalf("AppendRecordedEvent returned error: %v", err)
	}
	if err := os.Remove(written.Path); err != nil {
		t.Fatalf("remove audit record: %v", err)
	}

	clearAuditQueryFlags()
	auditRoot = root
	verifyCmd, verifyOutput := testCommand()
	err = runAuditVerify(verifyCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "audit integrity failed: 1 issue(s)") {
		t.Fatalf("expected audit integrity error, got %v", err)
	}
	if !strings.Contains(verifyOutput.String(), "missing_audit_record") ||
		!strings.Contains(verifyOutput.String(), "evt_audit_cli_missing_recorded") {
		t.Fatalf("unexpected verify output: %s", verifyOutput.String())
	}
}

func restoreAuditFlags(t *testing.T) {
	t.Helper()
	oldRoot := auditRoot
	oldID := auditID
	oldKind := auditKind
	oldDecision := auditDecision
	oldReason := auditReason
	oldJobID := auditJobID
	oldRunnerID := auditRunnerID
	oldProposalRefs := auditProposalRefs
	oldEventRefs := auditEventRefs
	oldArtifactRefs := auditArtifactRefs
	oldSpecJSON := auditSpecJSON
	oldEventID := auditEventID
	oldLoop := auditLoop
	oldHost := auditHost
	oldSource := auditSource
	oldCorrelationID := auditCorrelationID
	oldCausedBy := auditCausedBy
	oldListKind := auditListKind
	oldFormat := auditFormat
	t.Cleanup(func() {
		auditRoot = oldRoot
		auditID = oldID
		auditKind = oldKind
		auditDecision = oldDecision
		auditReason = oldReason
		auditJobID = oldJobID
		auditRunnerID = oldRunnerID
		auditProposalRefs = oldProposalRefs
		auditEventRefs = oldEventRefs
		auditArtifactRefs = oldArtifactRefs
		auditSpecJSON = oldSpecJSON
		auditEventID = oldEventID
		auditLoop = oldLoop
		auditHost = oldHost
		auditSource = oldSource
		auditCorrelationID = oldCorrelationID
		auditCausedBy = oldCausedBy
		auditListKind = oldListKind
		auditFormat = oldFormat
	})
	clearAuditQueryFlags()
	auditRoot = "."
}

func clearAuditQueryFlags() {
	auditID = ""
	auditKind = "manual"
	auditDecision = ""
	auditReason = ""
	auditJobID = ""
	auditRunnerID = ""
	auditProposalRefs = nil
	auditEventRefs = nil
	auditArtifactRefs = nil
	auditSpecJSON = ""
	auditEventID = ""
	auditLoop = ""
	auditHost = ""
	auditSource = "mnemon.audit"
	auditCorrelationID = ""
	auditCausedBy = ""
	auditListKind = ""
	auditFormat = "text"
}
