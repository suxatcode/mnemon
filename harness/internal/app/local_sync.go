package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/capability"
	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/remotesync"
	"github.com/mnemon-dev/mnemon/harness/internal/runtime"
)

// ImportLocalSyncPull re-enters pulled remote commits through Event Intake (the import runtime), then
// advances the durable pull cursor. It drives Ingest/Tick, so it stays on the app side of the boundary
// (above remotesync's pure store helpers) — never bypassing the kernel. It is the OFFLINE path: it
// boots its own import runtime by path, so it must never run inside a serving process (the in-process
// worker drives importPulledCommits over the LIVE runtime instead — flock, v1.1 #2).
func ImportLocalSyncPull(storePath, remoteID, nextCursor string, commits []contract.LocalCommit) error {
	if len(commits) > 0 {
		refs := refsFromCommits(commits)
		rt, err := OpenSyncImportRuntime(storePath, refs)
		if err != nil {
			return fmt.Errorf("open Local Mnemon import runtime: %w", err)
		}
		if err := importPulledCommits(rt, remoteID, commits); err != nil {
			_ = rt.Close()
			return err
		}
		if err := rt.Close(); err != nil {
			return err
		}
	}
	return remotesync.SetSyncPullCursor(storePath, remoteID, nextCursor)
}

// importPulledCommits is the ONE pull-import loop both paths share (offline ImportLocalSyncPull and
// the in-process worker): each commit re-enters Event Intake under contract.SyncImportActor with the
// six-part pull ExternalID (exactly-once), and a NEW observation is applied by one Tick. A commit
// whose kind has no import mapping is no longer silently dropped (v1.1 #4): it ingests
// sync.import_skipped.observed (ExternalID = six-part key + ":skipped") carrying the attribution
// payload, and the sync-import deny rule turns it into a durable sync.diagnostic. The pull cursor
// still advances either way — a skip is visible, never wedging.
func importPulledCommits(rt *runtime.Runtime, remoteID string, commits []contract.LocalCommit) error {
	pulledAt := time.Now().UTC().Format(time.RFC3339)
	for _, commit := range commits {
		var env contract.ObservationEnvelope
		if eventType, ok := remoteImportEventType(commit.ResourceRef.Kind); ok {
			env = contract.ObservationEnvelope{
				ExternalID: syncPullExternalID(remoteID, commit),
				Event: contract.Event{
					Type: eventType,
					Payload: map[string]any{
						"commit":    commit,
						"remote_id": remoteID,
						"pulled_at": pulledAt,
					},
				},
			}
		} else {
			env = contract.ObservationEnvelope{
				ExternalID: syncPullExternalID(remoteID, commit) + ":skipped",
				Event: contract.Event{
					Type: capability.SyncImportSkippedObserved,
					Payload: map[string]any{
						"kind":              string(commit.ResourceRef.Kind),
						"origin_replica_id": commit.OriginReplicaID,
						"local_decision_id": commit.LocalDecisionID,
						"remote_id":         remoteID,
					},
				},
			}
		}
		_, dup, err := rt.IngestTrusted(contract.SyncImportActor, env)
		if err != nil {
			return fmt.Errorf("ingest remote commit: %w", err)
		}
		if !dup {
			if _, err := rt.Tick(); err != nil {
				return fmt.Errorf("apply remote commit: %w", err)
			}
		}
	}
	return nil
}

// remoteImportEventType maps a synced commit's resource kind to its import observation. Remote
// import is memory/skill-only by design (see SyncImportRuntimeConfig); an unsupported kind returns
// false and the caller ingests the skipped-kind observation instead (durable diagnostic, never a
// silent drop) while the pull cursor still advances.
func remoteImportEventType(kind contract.ResourceKind) (string, bool) {
	switch kind {
	case "memory":
		return capability.RemoteMemoryCommitObserved, true
	case "skill":
		return capability.RemoteSkillCommitObserved, true
	default:
		return "", false
	}
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
