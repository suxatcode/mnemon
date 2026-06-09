package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/capability"
	"github.com/mnemon-dev/mnemon/harness/internal/contract"
	"github.com/mnemon-dev/mnemon/harness/internal/remotesync"
)

// ImportLocalSyncPull re-enters pulled remote commits through Event Intake (the import runtime), then
// advances the durable pull cursor. It drives Ingest/Tick, so it stays on the app side of the boundary
// (above remotesync's pure store helpers) — never bypassing the kernel.
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
			_, dup, err := rt.API().Ingest(contract.SyncImportActor, contract.ObservationEnvelope{
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
	return remotesync.SetSyncPullCursor(storePath, remoteID, nextCursor)
}

// remoteImportEventType maps a synced commit's resource kind to its import observation. Remote
// import is memory/skill-only by design (see SyncImportRuntimeConfig); an unsupported kind returns
// false and the caller SKIPS the commit while the pull cursor still advances — the commit is
// permanently dropped with no diagnostic. Acceptable while sync is offline/manual; revisit (emit a
// diagnostic or hold the cursor) when a third capability gains a remote producer.
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
