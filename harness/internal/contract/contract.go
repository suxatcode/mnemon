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
type ResourceSnapshot struct {
	Ref     ResourceRef
	Version Version
	Fields  map[string]any
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
	OpID          string
	Actor         ActorID
	Writes        []ResourceWrite
	ReadSet       []ResourceVersion
	IngestSeq     int64
	CorrelationID string // trusted envelope field; the durable key for liveness escalation (Invariant #10)
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
	DecisionID    string
	OpID          string
	IngestSeq     int64
	Actor         ActorID
	CorrelationID string // carries the triggering event's correlation; the durable escalation key (Invariant #10)
	Status        DecisionStatus
	Reason        string
	Conflicts     []Conflict
	NextAction    string // "" (terminal) | "rebase" | "human_review"
	AppliedAt     string // RFC3339; set iff Accepted
	NewVersions   []ResourceVersion
	NewResources  []ResourceSnapshot
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

// ObservationEnvelope is what an edge submits to the control server. Source is the AUTHENTICATED principal
// (the server overwrites Event.Actor from it — never the client payload, D7/S9); ExternalID is the edge's
// idempotency key for exactly-once ingest (S1: a retried (Source,ExternalID) returns the original seq).
type ObservationEnvelope struct {
	Source     ActorID
	ExternalID string
	Event      Event
}

// ---- rule pre-gate (D4) ----
// A rule is an ADMISSION CONTROLLER: it PROPOSES or ENQUEUES; it can NEVER write (S12). The kernel stays the
// only writer. The rich semantics live in this server-side pre-gate, not in the minimal kernel.
type RuleVerdict string

const (
	VerdictAllow           RuleVerdict = "allow"
	VerdictDeny            RuleVerdict = "deny"
	VerdictWarn            RuleVerdict = "warn"
	VerdictRequestEvidence RuleVerdict = "request_evidence"
	VerdictPropose         RuleVerdict = "propose"
	VerdictEnqueueJob      RuleVerdict = "enqueue_job"
)

// RuleDecision is a rule's output: a verdict plus (for propose) a Proposal or (for enqueue_job) a Job. It is
// return-only — a rule never holds a Store/Kernel, so it can describe an effect but never perform one (S12).
type RuleDecision struct {
	Verdict  RuleVerdict
	Reasons  []string
	Proposal *ProposedEvent
	Job      *JobSpec
	// ProposalActor is the TRUSTED origin actor of the carried Proposal — stamped by the RuleSet reducer from
	// the producing rule's Actor(), never by a rule's own output (json:"-" so an untrusted wasm rule cannot
	// forge it: the field is dropped on decode and re-set from the trusted Rule.Actor()). The server stamps the
	// bridge write identity from this instead of guessing the producer by scanning Handles/Emits.
	ProposalActor ActorID `json:"-"`
}

// JobSpec describes an effectful job for the at-least-once job lane. IdempotencyKey backs provider idempotency
// (S4); EstCostUSD feeds the budget reserve (S6).
type JobSpec struct {
	Kind           string
	IdempotencyKey string
	EstCostUSD     float64
	Args           map[string]any
}

// Diagnostic is the durable "why" of a reject/defer (S7: no silent drop). It is emitted as a "*.diagnostic"
// event so every rejection class leaves an auditable trail.
type Diagnostic struct {
	Stage  string
	Reason string
	Ref    string
}

// Subscription is a scope descriptor: which refs an actor may see, at what privacy tier. It lives in contract
// (not server) to avoid a projection<->server cycle (D11/blocker #3). The server builds an actor's scoped
// projection from its Subscription, and the projection identity (forActor) is the authenticated principal — a
// client never names its own scope on the wire (S9).
type Subscription struct {
	Actor       ActorID
	Refs        []ResourceRef
	PrivacyTier string
}

// LocalCommit is the append-only local sync unit materialized from an accepted local decision.
// It is durable local state; push/pull transports may serialize it, but Agent Integration never
// handles it directly.
type LocalCommit struct {
	OriginReplicaID string
	LocalDecisionID string
	LocalIngestSeq  int64
	Actor           ActorID
	CorrelationID   string
	ResourceRef     ResourceRef
	ResourceVersion Version
	FieldsDigest    string
	Fields          map[string]any
	DecidedAt       string
	Status          string
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

// KindCatalog — the third define≠select guard (alongside the mode catalogs). The resolver checks
// actor permissions and projection scopes against this; an actor may NOT be authorized to write, nor a
// scope reference, a kind the schema guard does not know (else config could DEFINE a phantom kind that
// the kernel silently accepts — an unknown kind has no required fields, so SchemaGuard.Validate passes).
// Invariant: keys(kernel.DefaultSchemaGuard().Required) == KindCatalog (enforced by a kernel test).
// lease/budget are first-class versioned resources (D3): their per-resource Version is the fence / CAS counter.
// receipt is the durable record of an external effect (S4: the job lane writes a receipt resource via CAS).
// coordination is the host-lifecycle teamwork-topology kind (P2.2 route 3/3): an approved
// coordination op is recorded as a governed coordination resource so the mutation flows
// through the kernel single-writer before the host emits its mirror topology events.
var KindCatalog = map[ResourceKind]bool{"memory": true, "goal": true, "skill": true, "lease": true, "budget": true, "receipt": true, "coordination": true, "note": true}
