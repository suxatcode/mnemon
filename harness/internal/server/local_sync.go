package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/store"
)

type LocalSyncPushBatch struct {
	ReplicaID string
	Commits   []contract.LocalCommit
}

type LocalSyncPullState struct {
	ReplicaID    string
	RemoteCursor string
}

type LocalSyncCounts struct {
	Pending   int
	Synced    int
	Conflicts int
}

func ReadLocalSyncPushBatch(storePath string) (LocalSyncPushBatch, error) {
	store, err := openLocalSyncStore(storePath)
	if err != nil {
		return LocalSyncPushBatch{}, fmt.Errorf("open Local Mnemon store: %w", err)
	}
	defer store.Close()
	pending, err := store.PendingSyncCommits()
	if err != nil {
		return LocalSyncPushBatch{}, fmt.Errorf("read pending sync commits: %w", err)
	}
	if len(pending) == 0 {
		return LocalSyncPushBatch{}, nil
	}
	replicaID, err := store.ReplicaID()
	if err != nil {
		return LocalSyncPushBatch{}, fmt.Errorf("read local replica id: %w", err)
	}
	return LocalSyncPushBatch{ReplicaID: replicaID, Commits: pending}, nil
}

func ApplyLocalSyncPushResponse(storePath, remoteID string, resp contract.SyncPushResponse) error {
	store, err := openLocalSyncStore(storePath)
	if err != nil {
		return fmt.Errorf("open Local Mnemon store for sync ack: %w", err)
	}
	defer store.Close()
	now := time.Now().UTC().Format(time.RFC3339)
	for _, item := range resp.Accepted {
		if err := store.MarkSyncCommitStatus(item.OriginReplicaID, item.LocalDecisionID, item.ResourceRef, "synced", remoteID, now, ""); err != nil {
			return err
		}
	}
	for _, item := range resp.Rejected {
		if err := store.MarkSyncCommitStatus(item.OriginReplicaID, item.LocalDecisionID, item.ResourceRef, "rejected", remoteID, now, item.Diagnostic); err != nil {
			return err
		}
	}
	for _, item := range resp.Conflicts {
		if err := store.MarkSyncCommitStatus(item.OriginReplicaID, item.LocalDecisionID, item.ResourceRef, "conflict", remoteID, now, item.Diagnostic); err != nil {
			return err
		}
	}
	return nil
}

func ReadLocalSyncPullState(storePath, remoteID string) (LocalSyncPullState, error) {
	store, err := openLocalSyncStore(storePath)
	if err != nil {
		return LocalSyncPullState{}, fmt.Errorf("open Local Mnemon store: %w", err)
	}
	defer store.Close()
	replicaID, err := store.ReplicaID()
	if err != nil {
		return LocalSyncPullState{}, fmt.Errorf("read local replica id: %w", err)
	}
	cursor := store.GetCursor(syncPullCursorName(remoteID))
	return LocalSyncPullState{ReplicaID: replicaID, RemoteCursor: strconv.FormatInt(cursor, 10)}, nil
}

func ImportLocalSyncPull(storePath, remoteID, nextCursor string, commits []contract.LocalCommit) error {
	if len(commits) > 0 {
		refs := refsFromCommits(commits)
		rt, err := OpenSyncImportRuntime(storePath, refs)
		if err != nil {
			return fmt.Errorf("open Local Mnemon import runtime: %w", err)
		}
		pulledAt := time.Now().UTC().Format(time.RFC3339)
		for _, commit := range commits {
			eventType, ok := remoteImportEventType(commit.ResourceRef.Kind)
			if !ok {
				continue
			}
			_, dup, err := rt.API().Ingest(SyncImportActor, contract.ObservationEnvelope{
				ExternalID: syncPullExternalID(remoteID, commit),
				Event: contract.Event{
					Type: eventType,
					Payload: map[string]any{
						"commit":    commit,
						"remote_id": remoteID,
						"pulled_at": pulledAt,
					},
				},
			})
			if err != nil {
				_ = rt.Close()
				return fmt.Errorf("ingest remote commit: %w", err)
			}
			if !dup {
				if _, err := rt.Tick(); err != nil {
					_ = rt.Close()
					return fmt.Errorf("apply remote commit: %w", err)
				}
			}
		}
		if err := rt.Close(); err != nil {
			return err
		}
	}
	return setSyncPullCursor(storePath, remoteID, nextCursor)
}

func ReadLocalSyncCounts(storePath string) (LocalSyncCounts, error) {
	store, err := openLocalSyncStore(storePath)
	if err != nil {
		return LocalSyncCounts{}, err
	}
	defer store.Close()
	counts, err := store.SyncCommitCounts()
	if err != nil {
		return LocalSyncCounts{}, err
	}
	return LocalSyncCounts{
		Pending:   counts.Pending,
		Synced:    counts.Synced,
		Conflicts: counts.Conflicts,
	}, nil
}

func remoteImportEventType(kind contract.ResourceKind) (string, bool) {
	switch kind {
	case "memory":
		return RemoteMemoryCommitObserved, true
	case "skill":
		return RemoteSkillCommitObserved, true
	default:
		return "", false
	}
}

func setSyncPullCursor(storePath, remoteID, cursor string) error {
	if strings.TrimSpace(cursor) == "" {
		return nil
	}
	seq, err := strconv.ParseInt(cursor, 10, 64)
	if err != nil {
		return fmt.Errorf("parse sync pull cursor: %w", err)
	}
	store, err := openLocalSyncStore(storePath)
	if err != nil {
		return fmt.Errorf("open Local Mnemon store for sync cursor: %w", err)
	}
	defer store.Close()
	return store.SetCursor(syncPullCursorName(remoteID), seq)
}

func syncPullCursorName(remoteID string) string {
	return "sync_pull:" + remoteID
}

func refsFromCommits(commits []contract.LocalCommit) []contract.ResourceRef {
	seen := map[contract.ResourceRef]bool{}
	var refs []contract.ResourceRef
	for _, commit := range commits {
		if !seen[commit.ResourceRef] {
			seen[commit.ResourceRef] = true
			refs = append(refs, commit.ResourceRef)
		}
	}
	return refs
}

func syncPullExternalID(remoteID string, commit contract.LocalCommit) string {
	return strings.Join([]string{
		"pull",
		remoteID,
		commit.OriginReplicaID,
		commit.LocalDecisionID,
		string(commit.ResourceRef.Kind),
		string(commit.ResourceRef.ID),
	}, ":")
}

func openLocalSyncStore(path string) (*store.Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	return store.OpenStore(path)
}
