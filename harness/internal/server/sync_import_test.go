package server

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/capability"
	"github.com/mnemon-dev/mnemon/harness/internal/contract"
)

func TestRemoteMemoryImportConflictDiagnosesWithoutOverwrite(t *testing.T) {
	ref := contract.ResourceRef{Kind: "memory", ID: "project"}
	rt, err := OpenSyncImportRuntime(filepath.Join(t.TempDir(), "local.db"), []contract.ResourceRef{ref})
	if err != nil {
		t.Fatalf("open sync import runtime: %v", err)
	}
	defer rt.Close()

	if err := ingestRemoteMemoryForTest(rt, "first", remoteMemoryCommitForTest(ref, "shared-entry", "remote content v1")); err != nil {
		t.Fatalf("first import: %v", err)
	}
	if _, err := rt.Tick(); err != nil {
		t.Fatalf("first tick: %v", err)
	}
	_, fields, err := rt.Resource(ref)
	if err != nil {
		t.Fatalf("read memory: %v", err)
	}
	if content, _ := fields["content"].(string); !strings.Contains(content, "remote content v1") {
		t.Fatalf("first import did not write memory: %+v", fields)
	}

	if err := ingestRemoteMemoryForTest(rt, "conflict", remoteMemoryCommitForTest(ref, "shared-entry", "remote content v2")); err != nil {
		t.Fatalf("conflict import: %v", err)
	}
	if _, err := rt.Tick(); err != nil {
		t.Fatalf("conflict tick: %v", err)
	}
	_, fields, err = rt.Resource(ref)
	if err != nil {
		t.Fatalf("read memory after conflict: %v", err)
	}
	content, _ := fields["content"].(string)
	if strings.Contains(content, "remote content v2") || !strings.Contains(content, "remote content v1") {
		t.Fatalf("conflict import overwrote local memory: %s", content)
	}
	events, err := rt.PendingEvents(0)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	var diagnosed bool
	for _, ev := range events {
		if ev.Type == "remote.diagnostic" || ev.Type == "memory.diagnostic" {
			if reason, _ := ev.Payload["reason"].(string); strings.Contains(reason, "remote memory conflict") {
				diagnosed = true
			}
		}
	}
	if !diagnosed {
		t.Fatalf("conflict import must emit a durable diagnostic, events=%+v", events)
	}
}

func TestRemoteSkillImportAppendsDeclarationsThroughLocalMnemon(t *testing.T) {
	ref := contract.ResourceRef{Kind: "skill", ID: "project"}
	rt, err := OpenSyncImportRuntime(filepath.Join(t.TempDir(), "local.db"), []contract.ResourceRef{ref})
	if err != nil {
		t.Fatalf("open sync import runtime: %v", err)
	}
	defer rt.Close()

	if err := ingestRemoteSkillForTest(rt, "remote-skill", remoteSkillCommitForTest(ref, "release-checklist", "active")); err != nil {
		t.Fatalf("remote skill import: %v", err)
	}
	if _, err := rt.Tick(); err != nil {
		t.Fatalf("tick remote skill import: %v", err)
	}
	_, fields, err := rt.Resource(ref)
	if err != nil {
		t.Fatalf("read skill: %v", err)
	}
	decls, ok := fields["declarations"].([]any)
	if !ok || len(decls) != 1 {
		t.Fatalf("remote skill import must write one declaration, got %+v", fields)
	}
	decl, ok := decls[0].(map[string]any)
	if !ok || decl["skill_id"] != "release-checklist" || decl["status"] != "active" {
		t.Fatalf("unexpected remote skill declaration: %+v", decls[0])
	}
}

func ingestRemoteMemoryForTest(rt *Runtime, externalID string, commit contract.LocalCommit) error {
	_, _, err := rt.API().Ingest(SyncImportActor, contract.ObservationEnvelope{
		ExternalID: externalID,
		Event: contract.Event{
			Type: capability.RemoteMemoryCommitObserved,
			Payload: map[string]any{
				"commit": commit,
			},
		},
	})
	return err
}

func ingestRemoteSkillForTest(rt *Runtime, externalID string, commit contract.LocalCommit) error {
	_, _, err := rt.API().Ingest(SyncImportActor, contract.ObservationEnvelope{
		ExternalID: externalID,
		Event: contract.Event{
			Type: capability.RemoteSkillCommitObserved,
			Payload: map[string]any{
				"commit": commit,
			},
		},
	})
	return err
}

func remoteMemoryCommitForTest(ref contract.ResourceRef, entryID, content string) contract.LocalCommit {
	return contract.LocalCommit{
		OriginReplicaID: "remote-replica",
		LocalDecisionID: "dec-" + entryID + "-" + strings.ReplaceAll(content, " ", "-"),
		LocalIngestSeq:  11,
		Actor:           "codex@remote",
		ResourceRef:     ref,
		ResourceVersion: 1,
		Fields: map[string]any{
			"content": "# Local Memory\n- " + content,
			"entries": []any{map[string]any{
				"id":         entryID,
				"content":    content,
				"source":     "remote",
				"confidence": "high",
				"actor":      "codex@remote",
				"ingest_seq": float64(11),
			}},
		},
		DecidedAt: "2026-06-06T00:00:00Z",
	}
}

func remoteSkillCommitForTest(ref contract.ResourceRef, skillID, status string) contract.LocalCommit {
	return contract.LocalCommit{
		OriginReplicaID: "remote-replica",
		LocalDecisionID: "dec-" + skillID + "-" + status,
		LocalIngestSeq:  21,
		Actor:           "codex@remote",
		ResourceRef:     ref,
		ResourceVersion: 1,
		Fields: map[string]any{
			"name": "project",
			"declarations": []any{map[string]any{
				"id":         "remote/" + skillID + "/" + status,
				"skill_id":   skillID,
				"name":       skillID,
				"status":     status,
				"content":    "Remote declaration for " + skillID,
				"source":     "remote",
				"confidence": "high",
				"actor":      "codex@remote",
				"ingest_seq": float64(21),
			}},
			"updated_by": "codex@remote",
		},
		DecidedAt: "2026-06-06T00:00:00Z",
	}
}
