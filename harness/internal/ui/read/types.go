// Package read builds an immutable, per-refresh snapshot of the project's
// .mnemon state for the cognition console. It is the read half of the surface:
// it imports only the internal/app facade (ring 6) and the standard library —
// never the inner store/eventlog/audit packages. Facade JSON output is decoded
// into the local read-model DTOs below, which mirror the facade's JSON contract
// field-for-field. The raw event stream (events.jsonl) is the one source with no
// facade reader, so it is read from disk directly via stdlib.
//
// Why local DTOs instead of the inner contract types: the ui surface (ring 7)
// must depend on the facade alone (see docs/harness/16-ring-architecture.md). The
// inner types (proposal.Proposal, schema.Event, profile.Profile, …) live in rings
// 0–2; importing them would puncture the ring boundary and, for profile, would
// pull the store in alongside the type. Mirroring the JSON keeps the contract
// without the coupling.
package read

// Proposal mirrors proposal.Proposal's JSON (proposal list/show, format="json").
type Proposal struct {
	SchemaVersion  string         `json:"schema_version"`
	Kind           string         `json:"kind"`
	ID             string         `json:"id"`
	Route          string         `json:"route"`
	Status         string         `json:"status"`
	Risk           string         `json:"risk"`
	Title          string         `json:"title"`
	Summary        string         `json:"summary"`
	Change         ChangeRequest  `json:"change"`
	Evidence       []EvidenceRef  `json:"evidence,omitempty"`
	ValidationPlan ValidationPlan `json:"validation_plan"`
	Review         ReviewPolicy   `json:"review"`
	Scope          map[string]any `json:"scope,omitempty"`
	DecisionRefs   []string       `json:"decision_refs,omitempty"`
	AuditRefs      []string       `json:"audit_refs,omitempty"`
	CreatedAt      string         `json:"created_at"`
	UpdatedAt      string         `json:"updated_at"`
	ClosedAt       string         `json:"closed_at,omitempty"`
	Supersedes     []string       `json:"supersedes,omitempty"`
	SupersededBy   string         `json:"superseded_by,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

// ChangeRequest mirrors proposal.ChangeRequest.
type ChangeRequest struct {
	Summary    string      `json:"summary"`
	Targets    []TargetRef `json:"targets"`
	Operations []Operation `json:"operations,omitempty"`
}

// TargetRef mirrors proposal.TargetRef.
type TargetRef struct {
	Type string `json:"type"`
	URI  string `json:"uri"`
}

// Operation mirrors proposal.Operation.
type Operation struct {
	Type    string         `json:"type"`
	Target  string         `json:"target"`
	Summary string         `json:"summary"`
	Payload map[string]any `json:"payload,omitempty"`
}

// EvidenceRef mirrors proposal.EvidenceRef / profile.EvidenceRef (same shape).
type EvidenceRef struct {
	Type    string `json:"type"`
	Ref     string `json:"ref"`
	Summary string `json:"summary,omitempty"`
}

// ValidationPlan mirrors proposal.ValidationPlan.
type ValidationPlan struct {
	Summary          string   `json:"summary"`
	Commands         []string `json:"commands,omitempty"`
	Checks           []string `json:"checks,omitempty"`
	RequiredEvidence []string `json:"required_evidence,omitempty"`
}

// ReviewPolicy mirrors proposal.ReviewPolicy.
type ReviewPolicy struct {
	Required        bool     `json:"required"`
	RequiredScope   string   `json:"required_scope,omitempty"`
	RequiredReviews int      `json:"required_reviews,omitempty"`
	Reviewers       []string `json:"reviewers,omitempty"`
	Notes           string   `json:"notes,omitempty"`
}

// Event mirrors schema.Event's wire shape (one JSON object per events.jsonl line).
// The free-form ref maps are kept as-is; Raw carries the verbatim line for the
// detail view.
type Event struct {
	SchemaVersion int            `json:"schema_version"`
	ID            string         `json:"id"`
	TS            string         `json:"ts"`
	Type          string         `json:"type"`
	Loop          *string        `json:"loop"`
	Host          *string        `json:"host"`
	Actor         string         `json:"actor"`
	Source        string         `json:"source"`
	CorrelationID string         `json:"correlation_id"`
	CausedBy      *string        `json:"caused_by"`
	Payload       map[string]any `json:"payload"`
	Scope         map[string]any `json:"scope,omitempty"`
	Severity      string         `json:"severity,omitempty"`
	ProposalRef   map[string]any `json:"proposal_ref,omitempty"`
	AuditRef      map[string]any `json:"audit_ref,omitempty"`
	StatusRef     map[string]any `json:"status_ref,omitempty"`

	// Raw is the verbatim JSONL line, retained for the detail pane. Not decoded.
	Raw string `json:"-"`
}

// LoopName returns the event's loop or "" when unscoped.
func (e Event) LoopName() string {
	if e.Loop == nil {
		return ""
	}
	return *e.Loop
}

// HostName returns the event's host or "" when unscoped.
func (e Event) HostName() string {
	if e.Host == nil {
		return ""
	}
	return *e.Host
}

// Profile mirrors profile.Profile's JSON (profile show, format="json").
type Profile struct {
	SchemaVersion string         `json:"schema_version"`
	Kind          string         `json:"kind"`
	ID            string         `json:"id"`
	ScopeType     string         `json:"scope_type"`
	Summary       string         `json:"summary,omitempty"`
	Entries       []ProfileEntry `json:"entries,omitempty"`
	CreatedAt     string         `json:"created_at"`
	UpdatedAt     string         `json:"updated_at"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

// ProfileEntry mirrors profile.Entry.
type ProfileEntry struct {
	ID                string             `json:"id"`
	Type              string             `json:"type"`
	Summary           string             `json:"summary"`
	Content           string             `json:"content"`
	Evidence          []EvidenceRef      `json:"evidence"`
	ProjectionTargets []ProjectionTarget `json:"projection_targets,omitempty"`
	CreatedAt         string             `json:"created_at"`
	UpdatedAt         string             `json:"updated_at"`
}

// ProjectionTarget mirrors profile.ProjectionTarget.
type ProjectionTarget struct {
	Host string `json:"host"`
	Loop string `json:"loop"`
}

// AuditRecord mirrors auditstore.WriteResult as emitted by AuditList(format=json).
// WriteResult has NO json tags, so the top-level keys are the capitalized Go field
// names (Audit/Path/Ref); the nested Audit object uses lowercase json tags. This
// asymmetry is intentional and load-bearing — do not "fix" the tags.
type AuditRecord struct {
	Audit AuditDoc       `json:"Audit"`
	Path  string         `json:"Path"`
	Ref   map[string]any `json:"Ref"`
}

// AuditDoc mirrors schema.Audit (the object AuditShow emits at top level).
type AuditDoc struct {
	SchemaVersion int            `json:"schema_version"`
	Kind          string         `json:"kind"`
	Metadata      AuditMetadata  `json:"metadata"`
	Spec          map[string]any `json:"spec"`
}

// AuditMetadata mirrors schema.Metadata.
type AuditMetadata struct {
	Name        string            `json:"name"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// URI returns the audit record's stored uri (from the Ref map), or "".
func (a AuditRecord) URI() string {
	if a.Ref == nil {
		return ""
	}
	if u, ok := a.Ref["uri"].(string); ok {
		return u
	}
	return ""
}

// Kind returns the audit_kind from the audit spec, or "".
func (a AuditRecord) Kind() string {
	if a.Audit.Spec == nil {
		return ""
	}
	if k, ok := a.Audit.Spec["audit_kind"].(string); ok {
		return k
	}
	return ""
}

// Goal is a minimal mirror of goal.Goal's JSON, decoded from goal.json on disk to
// recover the objective + plan (which the facade's flat GoalStatusView drops).
type Goal struct {
	ID            string    `json:"id"`
	Objective     string    `json:"objective"`
	Status        string    `json:"status"`
	UpdatedAt     string    `json:"updated_at"`
	EvidenceCount int       `json:"evidence_count"`
	Plan          *GoalPlan `json:"plan,omitempty"`
}

// GoalPlan mirrors the goal plan summary + steps.
type GoalPlan struct {
	Summary string   `json:"summary"`
	Steps   []string `json:"steps,omitempty"`
}

// Coordination mirrors coordination.View (app.Coordination, format="json") — the
// materialized multi-agent collaboration topology.
type Coordination struct {
	Tasks           []CoordTask     `json:"tasks,omitempty"`
	Groups          []CoordGroup    `json:"groups,omitempty"`
	Conflicts       []CoordConflict `json:"conflicts,omitempty"`
	MergeCandidates []CoordMerge    `json:"merge_candidates,omitempty"`
}

// CoordTask mirrors coordination.Task.
type CoordTask struct {
	ID           string   `json:"id"`
	Owner        string   `json:"owner,omitempty"`
	Status       string   `json:"status"`
	ForkedFrom   string   `json:"forked_from,omitempty"`
	JoinedInto   string   `json:"joined_into,omitempty"`
	EvidenceRefs []string `json:"evidence_refs,omitempty"`
	LastEventID  string   `json:"last_event_id,omitempty"`
	LastTS       string   `json:"last_ts,omitempty"`
}

// CoordGroup mirrors coordination.Group.
type CoordGroup struct {
	ID      string   `json:"id"`
	Members []string `json:"members,omitempty"`
	LastTS  string   `json:"last_ts,omitempty"`
}

// CoordConflict mirrors coordination.Conflict.
type CoordConflict struct {
	Between      []string `json:"between"`
	Reason       string   `json:"reason,omitempty"`
	EvidenceRefs []string `json:"evidence_refs,omitempty"`
	LastEventID  string   `json:"last_event_id,omitempty"`
	LastTS       string   `json:"last_ts,omitempty"`
}

// CoordMerge mirrors coordination.MergeCandidate.
type CoordMerge struct {
	EvidenceRef string   `json:"evidence_ref"`
	Tasks       []string `json:"tasks"`
}

// HostReadback mirrors status.HostReadback (app.Readback, format="json") — the
// per-host writeback verification state.
type HostReadback struct {
	Host              string `json:"host"`
	State             string `json:"state"` // observed | acted-but-unattributed | silent
	Stale             bool   `json:"stale,omitempty"`
	LiveProjectionRef string `json:"live_projection_ref,omitempty"`
	LiveDigest        string `json:"live_digest,omitempty"`
	ObservedDigest    string `json:"observed_digest,omitempty"`
	LiveTS            string `json:"live_ts,omitempty"`
	LastWritebackTS   string `json:"last_writeback_ts,omitempty"`
}
