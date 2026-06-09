package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/internal/capability"
	"github.com/mnemon-dev/mnemon/harness/internal/channel"
	"github.com/mnemon-dev/mnemon/harness/internal/contract"
)

func TestRemoteSyncPushIsIdempotentAndAuthenticated(t *testing.T) {
	ref := contract.ResourceRef{Kind: "memory", ID: "project"}
	host := channel.HostAgentBinding("codex@project", "http://localhost:8787", []contract.ResourceRef{ref})
	replica := channel.ReplicaAgentBinding("replica@project", "http://localhost:8787", []contract.ResourceRef{ref})
	rt, err := OpenRuntime(filepath.Join(t.TempDir(), "remote.db"), RuntimeConfig{
		Bindings: []channel.ChannelBinding{host, replica},
		Subs:     channel.SubsFromBindings([]channel.ChannelBinding{host, replica}),
	})
	if err != nil {
		t.Fatalf("open remote runtime: %v", err)
	}
	defer rt.Close()
	srv := newTokenRuntimeServer(t, rt, map[string]contract.ActorID{
		"host-token":    "codex@project",
		"replica-token": "replica@project",
	})
	defer srv.Close()

	commit := syncAPITestCommit("local-a", "dec-1", ref, map[string]any{"content": "remote accepted memory"})
	replicaClient := channel.NewClientWithToken(srv.URL, "replica-token")
	first, err := replicaClient.SyncPush(contract.SyncPushRequest{
		ReplicaID: "local-a",
		BatchID:   "batch-1",
		Commits:   []contract.LocalCommit{commit},
	})
	if err != nil {
		t.Fatalf("first sync push: %v", err)
	}
	if len(first.Accepted) != 1 || first.Accepted[0].Status != "accepted" {
		t.Fatalf("first push must accept the commit, got %+v", first)
	}

	duplicate, err := replicaClient.SyncPush(contract.SyncPushRequest{
		ReplicaID: "local-a",
		BatchID:   "batch-1",
		Commits:   []contract.LocalCommit{commit},
	})
	if err != nil {
		t.Fatalf("duplicate sync push: %v", err)
	}
	if !reflect.DeepEqual(first.Accepted, duplicate.Accepted) || len(duplicate.Conflicts) != 0 || len(duplicate.Rejected) != 0 {
		t.Fatalf("duplicate push must return the same ack without conflicts: first=%+v duplicate=%+v", first, duplicate)
	}

	mutated := syncAPITestCommit("local-a", "dec-1", ref, map[string]any{"content": "same idempotency key, different body"})
	conflicted, err := replicaClient.SyncPush(contract.SyncPushRequest{
		ReplicaID: "local-a",
		BatchID:   "batch-2",
		Commits:   []contract.LocalCommit{mutated},
	})
	if err != nil {
		t.Fatalf("conflicting duplicate sync push: %v", err)
	}
	if len(conflicted.Conflicts) != 1 || !strings.Contains(conflicted.Conflicts[0].Diagnostic, "idempotency key") {
		t.Fatalf("changed duplicate must be a protocol conflict, got %+v", conflicted)
	}

	if _, err := replicaClient.SyncPush(contract.SyncPushRequest{
		ReplicaID: "forged-local-id",
		BatchID:   "batch-forged",
		Commits:   []contract.LocalCommit{commit},
	}); err == nil {
		t.Fatalf("forged request replica_id must be rejected instead of trusted")
	}

	hostClient := channel.NewClientWithToken(srv.URL, "host-token")
	if _, err := hostClient.SyncPush(contract.SyncPushRequest{
		ReplicaID: "local-a",
		BatchID:   "host-batch",
		Commits:   []contract.LocalCommit{commit},
	}); err == nil {
		t.Fatalf("host-agent credential must not call sync endpoints")
	}
	if _, _, err := replicaClient.Ingest("replica@project", contract.ObservationEnvelope{
		ExternalID: "replica-observe",
		Event: contract.Event{
			Type: capability.MemoryWriteCandidateObserved,
			Payload: map[string]any{
				"content":    "replica should not be able to submit host observations",
				"source":     "test",
				"confidence": "high",
			},
		},
	}); err == nil {
		t.Fatalf("replica-agent credential must not call Agent Integration observe endpoints")
	}
}

func TestRemoteSyncPushRejectsBadCommitsWithDiagnostics(t *testing.T) {
	ref := contract.ResourceRef{Kind: "memory", ID: "project"}
	replica := channel.ReplicaAgentBinding("replica@project", "http://localhost:8787", []contract.ResourceRef{ref})
	rt, err := OpenRuntime(filepath.Join(t.TempDir(), "remote.db"), RuntimeConfig{
		Bindings: []channel.ChannelBinding{replica},
		Subs:     channel.SubsFromBindings([]channel.ChannelBinding{replica}),
	})
	if err != nil {
		t.Fatalf("open remote runtime: %v", err)
	}
	defer rt.Close()
	srv := newTokenRuntimeServer(t, rt, map[string]contract.ActorID{"replica-token": "replica@project"})
	defer srv.Close()

	bad := syncAPITestCommit("local-a", "dec-bad", ref, map[string]any{"content": "bad digest"})
	bad.FieldsDigest = "wrong"
	resp, err := channel.NewClientWithToken(srv.URL, "replica-token").SyncPush(contract.SyncPushRequest{
		ReplicaID: "local-a",
		BatchID:   "batch-bad",
		Commits:   []contract.LocalCommit{bad},
	})
	if err != nil {
		t.Fatalf("bad commit should return diagnostics, not transport failure: %v", err)
	}
	if len(resp.Rejected) != 1 || !strings.Contains(resp.Rejected[0].Diagnostic, "fields_digest") {
		t.Fatalf("bad commit must be rejected with a diagnostic, got %+v", resp)
	}
}

func newTokenRuntimeServer(t *testing.T, rt *Runtime, tokens map[string]contract.ActorID) *httptest.Server {
	t.Helper()
	return httptest.NewServer(NewRuntimeHandler(rt, channel.TokenAuthenticator{Tokens: tokens}))
}

func syncAPITestCommit(replicaID, decisionID string, ref contract.ResourceRef, fields map[string]any) contract.LocalCommit {
	return contract.LocalCommit{
		OriginReplicaID: replicaID,
		LocalDecisionID: decisionID,
		LocalIngestSeq:  1,
		Actor:           "codex@project",
		ResourceRef:     ref,
		ResourceVersion: 1,
		FieldsDigest:    syncAPITestDigest(fields),
		Fields:          fields,
		DecidedAt:       "2026-06-06T00:00:00Z",
		Status:          "pending",
	}
}

func syncAPITestDigest(fields map[string]any) string {
	b, _ := json.Marshal(fields)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// The sync path's scope clamp had ZERO test coverage while being the only pre-SQL enforcement on
// that path. Pin the ClampRefs semantics here: in-scope narrowing passes, out-of-scope is denied,
// empty-requested defaults to the binding scope, and — the deliberate tightening — an empty-scope
// replica binding now denies explicit refs instead of passing them through.
func TestClampSyncScopesEnforcesBindingScope(t *testing.T) {
	mem := contract.ResourceRef{Kind: "memory", ID: "project"}
	skill := contract.ResourceRef{Kind: "skill", ID: "project"}
	b := channel.ReplicaAgentBinding("replica@peer", "http://x", []contract.ResourceRef{mem, skill})

	if got, err := clampSyncScopes(b, []contract.ResourceRef{mem}); err != nil || len(got) != 1 || got[0] != mem {
		t.Fatalf("in-scope narrowing must pass: %v err=%v", got, err)
	}
	if _, err := clampSyncScopes(b, []contract.ResourceRef{{Kind: "note", ID: "project"}}); err == nil {
		t.Fatal("an out-of-scope sync ref must be denied")
	}
	if got, err := clampSyncScopes(b, nil); err != nil || len(got) != 2 {
		t.Fatalf("empty requested must default to the binding scope: %v err=%v", got, err)
	}
	unscoped := channel.ReplicaAgentBinding("replica@peer", "http://x", nil)
	if _, err := clampSyncScopes(unscoped, []contract.ResourceRef{mem}); err == nil {
		t.Fatal("an empty-scope replica binding must deny explicit refs (fail closed)")
	}
}
