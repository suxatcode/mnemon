package contract

// Channel-facing DTOs shared across the channel/runtime/app layers. They live in contract (zero
// deps) so the channel port and the runtime that satisfies it can both name them without a back-edge.

// ActorKind classifies a channel principal by role. It is NOT a privilege path: the channel is
// the same for every principal; the role differs by binding, never by a privileged code path
// (D6). HostAgent pushes host observations; ControlAgent is an operator/control client;
// ReplicaAgent is the background Remote Workspace sync actor.
type ActorKind string

const (
	KindHostAgent    ActorKind = "host-agent"
	KindControlAgent ActorKind = "control-agent"
	KindReplicaAgent ActorKind = "replica-agent"
)

// ChannelStatus is the principal's channel status surface (digest + scope counts + sync state).
type ChannelStatus struct {
	Principal     ActorID   `json:"principal"`
	Digest        string    `json:"digest"`
	Resources     int       `json:"resources"`
	ActorKind     ActorKind `json:"actor_kind,omitempty"`
	StoreRef      string    `json:"store_ref"`
	Mode          string    `json:"mode"`
	SyncPending   int       `json:"sync_pending"`
	SyncSynced    int       `json:"sync_synced"`
	SyncConflicts int       `json:"sync_conflicts"`
}

// Sync{Push,Pull,Status} request/response DTOs for the Remote Workspace sync verbs.

type SyncPushRequest struct {
	ReplicaID string        `json:"replica_id"`
	BatchID   string        `json:"batch_id"`
	Commits   []LocalCommit `json:"commits"`
}

type SyncPushResponse struct {
	Accepted   []SyncCommitResult `json:"accepted"`
	Rejected   []SyncCommitResult `json:"rejected"`
	Conflicts  []SyncCommitResult `json:"conflicts"`
	NextCursor string             `json:"next_cursor,omitempty"`
}

type SyncPullRequest struct {
	ReplicaID    string        `json:"replica_id"`
	RemoteCursor string        `json:"remote_cursor"`
	Scopes       []ResourceRef `json:"scopes"`
}

type SyncPullResponse struct {
	Commits     []LocalCommit      `json:"commits"`
	Diagnostics []SyncCommitResult `json:"diagnostics"`
	NextCursor  string             `json:"next_cursor"`
}

type SyncStatusResponse struct {
	Principal       ActorID `json:"principal"`
	RemoteWorkspace string  `json:"remote_workspace"`
}

type SyncCommitResult struct {
	OriginReplicaID string      `json:"origin_replica_id"`
	LocalDecisionID string      `json:"local_decision_id"`
	ResourceRef     ResourceRef `json:"resource_ref"`
	Status          string      `json:"status"`
	Diagnostic      string      `json:"diagnostic,omitempty"`
}
