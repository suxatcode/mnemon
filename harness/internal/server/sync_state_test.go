package server

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/capability"
	"github.com/mnemon-dev/mnemon/harness/internal/channel"
	"github.com/mnemon-dev/mnemon/harness/internal/contract"
)

func TestAcceptedLocalMemoryCreatesPendingSyncCommit(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "governed.db")
	ref := contract.ResourceRef{Kind: "memory", ID: "project"}
	binding := channel.HostAgentBinding("codex@project", "http://127.0.0.1:8787", []contract.ResourceRef{ref})
	binding.AllowedObservedTypes = []string{capability.MemoryWriteCandidateObserved}
	rt, err := OpenLocalRuntime(storePath, channel.LoadedBindings{Bindings: []channel.ChannelBinding{binding}})
	if err != nil {
		t.Fatalf("open local runtime: %v", err)
	}
	srv := httptest.NewServer(NewRuntimeHandler(rt, channel.HeaderAuthenticator{}))
	client := channel.NewClient(srv.URL, "codex@project")
	if rec, err := client.IngestObserve("codex@project", contract.ObservationEnvelope{
		ExternalID: "sync-memory-1",
		Event: contract.Event{Type: capability.MemoryWriteCandidateObserved, Payload: map[string]any{
			"content": "Sync should queue this local memory entry.",
			"source":  "user", "confidence": "high",
		}},
	}); err != nil || !rec.Ticked {
		t.Fatalf("observe memory candidate: rec=%+v err=%v", rec, err)
	}
	if _, err := rt.Tick(); err != nil {
		t.Fatalf("extra tick: %v", err)
	}

	pending, err := rt.store.PendingSyncCommits()
	if err != nil {
		t.Fatalf("pending sync commits: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("want one pending sync commit, got %+v", pending)
	}
	commit := pending[0]
	if commit.OriginReplicaID == "" || commit.LocalDecisionID == "" || commit.Status != "pending" {
		t.Fatalf("pending commit missing identity/status: %+v", commit)
	}
	if commit.ResourceRef != ref || commit.ResourceVersion != 1 {
		t.Fatalf("pending commit has wrong resource: %+v", commit)
	}
	if commit.FieldsDigest == "" {
		t.Fatalf("pending commit must include fields digest: %+v", commit)
	}
	if content, _ := commit.Fields["content"].(string); !strings.Contains(content, "Sync should queue") {
		t.Fatalf("pending commit fields missing memory content: %+v", commit.Fields)
	}
	st, err := rt.Status("codex@project")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if st.SyncPending != 1 {
		t.Fatalf("status must report one pending sync commit, got %+v", st)
	}

	replicaID := commit.OriginReplicaID
	srv.Close()
	if err := rt.Close(); err != nil {
		t.Fatalf("close runtime: %v", err)
	}
	rt2, err := OpenLocalRuntime(storePath, channel.LoadedBindings{Bindings: []channel.ChannelBinding{binding}})
	if err != nil {
		t.Fatalf("reopen local runtime: %v", err)
	}
	defer rt2.Close()
	pending2, err := rt2.store.PendingSyncCommits()
	if err != nil {
		t.Fatalf("pending sync commits after reopen: %v", err)
	}
	if len(pending2) != 1 || pending2[0].OriginReplicaID != replicaID || pending2[0].LocalDecisionID != commit.LocalDecisionID {
		t.Fatalf("pending commit must survive restart without duplication:\n before=%+v\n after=%+v", pending, pending2)
	}
}
