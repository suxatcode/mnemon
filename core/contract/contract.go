package contract

// ---- resources ----
type ResourceKind string // "memory", "goal", "skill"
type ResourceID string
type Version int64 // per-resource; 1 on create; +1 each accepted write. NEVER global.
type ResourceRef struct {
	Kind ResourceKind
	ID   ResourceID
}
type ResourceVersion struct {
	Ref     ResourceRef
	Version Version
}

// ActorID is an IDENTITY, not a role enum (Invariant #11/#15).
type ActorID string

// ---- ops ----
type OpKind string

const (
	OpCreate OpKind = "create"
	OpUpdate OpKind = "update"
) // OpDelete is out of P0 scope.

type ResourceWrite struct {
	Ref     ResourceRef
	Kind    OpKind
	BasedOn Version // expected current version; 0 for create
	Fields  map[string]any
}

// KernelOp is ALL-OR-NOTHING (Invariant #5). ReadSet = versions the proposer READ (Invariant #6).
// IngestSeq is the triggering event's durable seq (events.rowid), stamped by the reconciler from a
// TRUSTED source; 0 for a direct (non-event) Apply. It is the event<->decision audit link and the basis
// for the reconciler's durable cursor.
type KernelOp struct {
	OpID      string
	Actor     ActorID
	Writes    []ResourceWrite
	ReadSet   []ResourceVersion
	IngestSeq int64
}

// ---- decisions ----
type DecisionStatus string

const (
	Accepted DecisionStatus = "accepted"
	Rejected DecisionStatus = "rejected"
	Deferred DecisionStatus = "deferred"
)

type ConflictKind string

const (
	WriteWrite ConflictKind = "write_write"
	ReadStale  ConflictKind = "read_stale"
)

type Conflict struct {
	Ref             ResourceRef
	ExpectedVersion Version
	ActualVersion   Version
	Kind            ConflictKind
}
type Decision struct {
	DecisionID  string
	OpID        string
	IngestSeq   int64
	Actor       ActorID
	Status      DecisionStatus
	Reason      string
	Conflicts   []Conflict
	NextAction  string // "" (terminal) | "rebase" | "human_review"
	AppliedAt   string // RFC3339; set iff Accepted
	NewVersions []ResourceVersion
}

// ---- events ----
type Event struct {
	SchemaVersion int               `json:"schema_version"`
	ID            string            `json:"id"`
	IngestSeq     int64             `json:"ingest_seq"` // = events.rowid; the ONLY ordering key (Invariant #9)
	TS            string            `json:"ts"`         // provenance only; NEVER orders
	Type          string            `json:"type"`
	Actor         ActorID           `json:"actor"`
	ResourceRefs  []ResourceRef     `json:"resource_refs"`
	BasedOn       []ResourceVersion `json:"based_on"`       // read-set (Invariant #6)
	ProjectionRef string            `json:"projection_ref"` // provenance of the projection acted on
	ContextDigest string            `json:"context_digest"` // provenance; P1 may promote to a validation anchor
	CorrelationID string            `json:"correlation_id"`
	CausedBy      string            `json:"caused_by,omitempty"`
	Payload       map[string]any    `json:"payload"`
}

// ---- callback intent ----
type ProposedEvent struct {
	Type    string
	Payload map[string]any
}

// ---- modes (the catalog NAMES live here — the standard advertises them) ----
type Modes struct{ Conflict, Isolation, Authz string }

const (
	ConflictReject            = "reject"
	ConflictRebase            = "rebase"
	ConflictAutoMergeDisjoint = "auto_merge_disjoint"
	ConflictDeferToHuman      = "defer_to_human"

	IsolationWriteCAS          = "write_cas"
	IsolationProjectionReadSet = "projection_read_set"
	// "serializable" intentionally ABSENT until P1 evidence shows it differs from projection_read_set (§10).

	AuthzStrict = "strict" // enforce rules; violation -> Rejected. The only IMPLEMENTED authz mode.
	// Reserved — NOT in AuthzCatalog until implemented with real, distinct teeth (mirrors `serializable`).
	// The kernel currently enforces rules UNCONDITIONALLY (= strict, fail-closed), so advertising these as
	// selectable would promise behavior it cannot deliver — and selecting dry_run would still commit.
	// Deferred semantics if/when built: permissive & audit_only would both be "allow-despite-violation"
	// (byte-identical — the anti-pattern that dropped `serializable`); dry_run = validate-but-never-commit.
	AuthzPermissive = "permissive"
	AuthzAuditOnly  = "audit_only"
	AuthzDryRun     = "dry_run"
)

// Catalog membership — the define≠select guard (Invariant #12) checks against these.
var (
	ConflictCatalog  = map[string]bool{ConflictReject: true, ConflictRebase: true, ConflictAutoMergeDisjoint: true, ConflictDeferToHuman: true}
	IsolationCatalog = map[string]bool{IsolationWriteCAS: true, IsolationProjectionReadSet: true}
	AuthzCatalog     = map[string]bool{AuthzStrict: true} // only strict is implemented; the rest are reserved (see consts above)
)
