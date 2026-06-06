package main

import (
	"bytes"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mnemon-dev/mnemon/harness/core/contract"
	"github.com/mnemon-dev/mnemon/harness/core/server"
)

func TestSyncPushOnceAcksPendingLocalCommits(t *testing.T) {
	restoreSyncFlags(t)
	root := t.TempDir()
	storePath := filepath.Join(root, server.DefaultStorePath)
	ref := contract.ResourceRef{Kind: "memory", ID: "project"}

	localBinding := server.ChannelBinding{
		Principal:            "codex@project",
		ActorKind:            server.KindHostAgent,
		Transport:            server.TransportHTTP,
		Endpoint:             "http://127.0.0.1:8787",
		AllowedVerbs:         []server.Verb{server.VerbObserve, server.VerbPull, server.VerbStatus},
		AllowedObservedTypes: []string{server.MemoryWriteCandidateObserved},
		SubscriptionScope:    []contract.ResourceRef{ref},
		IdempotencyNamespace: "host:codex@project",
	}
	local, err := server.OpenLocalRuntime(storePath, server.LoadedBindings{Bindings: []server.ChannelBinding{localBinding}})
	if err != nil {
		t.Fatalf("open local runtime: %v", err)
	}
	localSrv := httptest.NewServer(server.NewRuntimeHandler(local, server.HeaderAuthenticator{}))
	client := server.NewClient(localSrv.URL, "codex@project")
	if _, err := client.IngestObserve("codex@project", contract.ObservationEnvelope{
		ExternalID: "sync-push-memory",
		Event: contract.Event{Type: server.MemoryWriteCandidateObserved, Payload: map[string]any{
			"content":    "sync push should ack this local memory",
			"source":     "test",
			"confidence": "high",
		}},
	}); err != nil {
		t.Fatalf("local observe: %v", err)
	}
	localSrv.Close()
	if err := local.Close(); err != nil {
		t.Fatalf("close local runtime: %v", err)
	}

	syncRoot = root
	syncStorePath = storePath
	syncRemoteID = "workspace"
	syncRemoteURL = "http://127.0.0.1:1"
	syncRemoteToken = "remote-token"
	var down bytes.Buffer
	cmd := mustTestCommand(t)
	cmd.SetOut(&down)
	if err := runSyncPush(cmd, nil); err == nil || !strings.Contains(err.Error(), "sync push failed") {
		t.Fatalf("remote-down push must report transport failure, got %v", err)
	}
	st, err := syncStatusForTest(storePath)
	if err != nil {
		t.Fatalf("status after remote down: %v", err)
	}
	if st.SyncPending != 1 || st.SyncSynced != 0 {
		t.Fatalf("remote-down push must leave local commit pending, got %+v", st)
	}

	remoteBinding := server.ReplicaAgentBinding("replica@project", "http://127.0.0.1:8787", []contract.ResourceRef{ref})
	remote, err := server.OpenRuntime(filepath.Join(t.TempDir(), "remote.db"), server.RuntimeConfig{
		Bindings: []server.ChannelBinding{remoteBinding},
		Subs:     server.SubsFromBindings([]server.ChannelBinding{remoteBinding}),
	})
	if err != nil {
		t.Fatalf("open remote runtime: %v", err)
	}
	defer remote.Close()
	remoteSrv := httptest.NewServer(server.NewRuntimeHandler(remote, server.TokenAuthenticator{Tokens: map[string]contract.ActorID{"remote-token": "replica@project"}}))
	defer remoteSrv.Close()

	syncRemoteURL = remoteSrv.URL
	var out bytes.Buffer
	cmd = mustTestCommand(t)
	cmd.SetOut(&out)
	if err := runSyncPush(cmd, nil); err != nil {
		t.Fatalf("sync push once: %v", err)
	}
	if !strings.Contains(out.String(), "Sync push: 1 accepted, 0 rejected, 0 conflicts") {
		t.Fatalf("unexpected sync output: %s", out.String())
	}
	st, err = syncStatusForTest(storePath)
	if err != nil {
		t.Fatalf("status after push: %v", err)
	}
	if st.SyncPending != 0 || st.SyncSynced != 1 || st.SyncConflicts != 0 {
		t.Fatalf("successful push must mark the local commit synced, got %+v", st)
	}
}

func TestSyncRemoteConfigLoadsCredentialRef(t *testing.T) {
	restoreSyncFlags(t)
	root := t.TempDir()
	credRel := filepath.Join(".mnemon", "harness", "sync", "credentials", "workspace.token")
	credPath := filepath.Join(root, credRel)
	if err := os.MkdirAll(filepath.Dir(credPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(credPath, []byte("tok-workspace\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	remotesPath := filepath.Join(root, ".mnemon", "harness", "sync", "remotes.json")
	if err := os.MkdirAll(filepath.Dir(remotesPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(remotesPath, []byte(`{
	  "schema_version": 1,
	  "remotes": [{
	    "id": "workspace",
	    "endpoint": "http://127.0.0.1:8787",
	    "credential_ref": ".mnemon/harness/sync/credentials/workspace.token"
	  }]
	}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	syncRoot = root
	syncRemoteID = "workspace"
	remote, err := resolveSyncRemote()
	if err != nil {
		t.Fatalf("resolve remote config: %v", err)
	}
	if remote.ID != "workspace" || remote.Endpoint != "http://127.0.0.1:8787" || remote.Token != "tok-workspace" {
		t.Fatalf("remote config not loaded: %+v", remote)
	}
}

func restoreSyncFlags(t *testing.T) {
	t.Helper()
	oldRoot := syncRoot
	oldStorePath := syncStorePath
	oldRemotesPath := syncRemotesPath
	oldRemoteID := syncRemoteID
	oldRemoteURL := syncRemoteURL
	oldRemoteToken := syncRemoteToken
	oldRemoteTokenFile := syncRemoteTokenFile
	t.Cleanup(func() {
		syncRoot = oldRoot
		syncStorePath = oldStorePath
		syncRemotesPath = oldRemotesPath
		syncRemoteID = oldRemoteID
		syncRemoteURL = oldRemoteURL
		syncRemoteToken = oldRemoteToken
		syncRemoteTokenFile = oldRemoteTokenFile
	})
	syncRoot = "."
	syncStorePath = ""
	syncRemotesPath = ""
	syncRemoteID = "default"
	syncRemoteURL = ""
	syncRemoteToken = ""
	syncRemoteTokenFile = ""
}

func syncStatusForTest(storePath string) (server.ChannelStatus, error) {
	rt, err := server.OpenRuntime(storePath, server.RuntimeConfig{})
	if err != nil {
		return server.ChannelStatus{}, err
	}
	defer rt.Close()
	return rt.Status("status@test")
}
