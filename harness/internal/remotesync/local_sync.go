// Package remotesync holds the pure store-side helpers for Remote Workspace sync: reading the pending
// push batch, applying a push response's per-commit status, reading pull state/counts, and advancing
// the pull cursor. It imports store + contract only. The ingest-driving pull import (which re-enters
// Event Intake via a runtime) lives in app, not here, so remotesync never depends upward.
package remotesync

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
	s, err := openLocalSyncStore(storePath)
	if err != nil {
		return LocalSyncPushBatch{}, fmt.Errorf("open Local Mnemon store: %w", err)
	}
	defer s.Close()
	pending, err := s.PendingSyncCommits()
	if err != nil {
		return LocalSyncPushBatch{}, fmt.Errorf("read pending sync commits: %w", err)
	}
	if len(pending) == 0 {
		return LocalSyncPushBatch{}, nil
	}
	replicaID, err := s.ReplicaID()
	if err != nil {
		return LocalSyncPushBatch{}, fmt.Errorf("read local replica id: %w", err)
	}
	return LocalSyncPushBatch{ReplicaID: replicaID, Commits: pending}, nil
}

func ApplyLocalSyncPushResponse(storePath, remoteID string, resp contract.SyncPushResponse) error {
	s, err := openLocalSyncStore(storePath)
	if err != nil {
		return fmt.Errorf("open Local Mnemon store for sync ack: %w", err)
	}
	defer s.Close()
	now := time.Now().UTC().Format(time.RFC3339)
	for _, item := range resp.Accepted {
		if err := s.MarkSyncCommitStatus(item.OriginReplicaID, item.LocalDecisionID, item.ResourceRef, "synced", remoteID, now, ""); err != nil {
			return err
		}
	}
	for _, item := range resp.Rejected {
		if err := s.MarkSyncCommitStatus(item.OriginReplicaID, item.LocalDecisionID, item.ResourceRef, "rejected", remoteID, now, item.Diagnostic); err != nil {
			return err
		}
	}
	for _, item := range resp.Conflicts {
		if err := s.MarkSyncCommitStatus(item.OriginReplicaID, item.LocalDecisionID, item.ResourceRef, "conflict", remoteID, now, item.Diagnostic); err != nil {
			return err
		}
	}
	return nil
}

func ReadLocalSyncPullState(storePath, remoteID string) (LocalSyncPullState, error) {
	s, err := openLocalSyncStore(storePath)
	if err != nil {
		return LocalSyncPullState{}, fmt.Errorf("open Local Mnemon store: %w", err)
	}
	defer s.Close()
	replicaID, err := s.ReplicaID()
	if err != nil {
		return LocalSyncPullState{}, fmt.Errorf("read local replica id: %w", err)
	}
	cursor := s.GetCursor(syncPullCursorName(remoteID))
	return LocalSyncPullState{ReplicaID: replicaID, RemoteCursor: strconv.FormatInt(cursor, 10)}, nil
}

func ReadLocalSyncCounts(storePath string) (LocalSyncCounts, error) {
	s, err := openLocalSyncStore(storePath)
	if err != nil {
		return LocalSyncCounts{}, err
	}
	defer s.Close()
	counts, err := s.SyncCommitCounts()
	if err != nil {
		return LocalSyncCounts{}, err
	}
	return LocalSyncCounts{
		Pending:   counts.Pending,
		Synced:    counts.Synced,
		Conflicts: counts.Conflicts,
	}, nil
}

// SetSyncPullCursor advances the durable pull cursor for remoteID. It is the store-side tail of the
// pull import; the import itself (re-entering Event Intake) lives in app.
func SetSyncPullCursor(storePath, remoteID, cursor string) error {
	if strings.TrimSpace(cursor) == "" {
		return nil
	}
	seq, err := strconv.ParseInt(cursor, 10, 64)
	if err != nil {
		return fmt.Errorf("parse sync pull cursor: %w", err)
	}
	s, err := openLocalSyncStore(storePath)
	if err != nil {
		return fmt.Errorf("open Local Mnemon store for sync cursor: %w", err)
	}
	defer s.Close()
	return s.SetCursor(syncPullCursorName(remoteID), seq)
}

func syncPullCursorName(remoteID string) string {
	return "sync_pull:" + remoteID
}

func openLocalSyncStore(path string) (*store.Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	return store.OpenStore(path)
}

// ProbeAvailable reports whether the local store can be opened for an offline sync pass. It returns an
// error when a co-hosted Local Mnemon (`local run`) already holds the single-writer lock — so the
// standalone sync can refuse cleanly up front instead of failing every pass. It is side-effect free:
// a not-yet-created store is "available" (nothing holds it) without materializing the db or its dirs;
// an existing free store is opened and immediately released.
func ProbeAvailable(storePath string) error {
	if _, err := os.Stat(storePath); os.IsNotExist(err) {
		return nil
	}
	s, err := store.OpenStore(storePath)
	if err != nil {
		return err
	}
	return s.Close()
}
