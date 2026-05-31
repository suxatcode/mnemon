package schema

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const Version = 1

var eventTypePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

var allowedActors = map[string]struct{}{
	"user":          {},
	"host-agent":    {},
	"mnemon-manual": {},
	"mnemon-daemon": {},
	"host-runner":   {},
	"reconciler":    {},
	"projector":     {},
	"validator":     {},
}

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
	ProjectRoot   string         `json:"project_root,omitempty"`
	Store         string         `json:"store,omitempty"`
	Scope         map[string]any `json:"scope,omitempty"`
	Severity      string         `json:"severity,omitempty"`
	Privacy       map[string]any `json:"privacy,omitempty"`
	ArtifactRefs  []RawObject    `json:"artifact_refs,omitempty"`
	StatusRef     map[string]any `json:"status_ref,omitempty"`
	ProposalRef   map[string]any `json:"proposal_ref,omitempty"`
	AuditRef      map[string]any `json:"audit_ref,omitempty"`
	Hashes        map[string]any `json:"hashes,omitempty"`
}

type ScopeRef struct {
	ID           string `json:"id,omitempty"`
	Type         string `json:"type,omitempty"`
	ProjectRoot  string `json:"project_root,omitempty"`
	Store        string `json:"store,omitempty"`
	Host         string `json:"host,omitempty"`
	Loop         string `json:"loop,omitempty"`
	ProfileRef   string `json:"profile_ref,omitempty"`
	BindingScope string `json:"binding_scope,omitempty"`
}

type ScopeOptions struct {
	ID           string
	Type         string
	ProjectRoot  string
	Store        string
	Host         string
	Loop         string
	ProfileRef   string
	BindingScope string
}

// ProjectScopeWithProfile is the single project-scope constructor. Callers that
// have no profile pass "" for profileRef; the field is omitted from the scope map.
func ProjectScopeWithProfile(projectRoot, store, host, loop, profileRef string) ScopeRef {
	return CurrentScope(ScopeOptions{
		ProjectRoot:  projectRoot,
		Store:        store,
		Host:         host,
		Loop:         loop,
		ProfileRef:   profileRef,
		BindingScope: "project",
	})
}

func CurrentScope(opts ScopeOptions) ScopeRef {
	scopeType := strings.TrimSpace(opts.Type)
	if scopeType == "" {
		scopeType = "project"
	}
	bindingScope := strings.TrimSpace(opts.BindingScope)
	if bindingScope == "" && scopeType == "project" {
		bindingScope = "project"
	}
	id := strings.TrimSpace(opts.ID)
	if id == "" && scopeType == "project" {
		id = "project"
	}
	return ScopeRef{
		ID:           id,
		Type:         scopeType,
		ProjectRoot:  strings.TrimSpace(opts.ProjectRoot),
		Store:        strings.TrimSpace(opts.Store),
		Host:         strings.TrimSpace(opts.Host),
		Loop:         strings.TrimSpace(opts.Loop),
		ProfileRef:   strings.TrimSpace(opts.ProfileRef),
		BindingScope: bindingScope,
	}
}

func (s ScopeRef) Map() map[string]any {
	out := map[string]any{}
	if strings.TrimSpace(s.ID) != "" {
		out["id"] = strings.TrimSpace(s.ID)
	}
	if strings.TrimSpace(s.Type) != "" {
		out["type"] = strings.TrimSpace(s.Type)
	}
	if strings.TrimSpace(s.ProjectRoot) != "" {
		out["project_root"] = strings.TrimSpace(s.ProjectRoot)
	}
	if strings.TrimSpace(s.Store) != "" {
		out["store"] = strings.TrimSpace(s.Store)
	}
	if strings.TrimSpace(s.Host) != "" {
		out["host"] = strings.TrimSpace(s.Host)
	}
	if strings.TrimSpace(s.Loop) != "" {
		out["loop"] = strings.TrimSpace(s.Loop)
	}
	if strings.TrimSpace(s.ProfileRef) != "" {
		out["profile_ref"] = strings.TrimSpace(s.ProfileRef)
	}
	if strings.TrimSpace(s.BindingScope) != "" {
		out["binding_scope"] = strings.TrimSpace(s.BindingScope)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

type RawObject map[string]any

type Metadata struct {
	Name        string            `json:"name"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type Condition struct {
	Type             string `json:"type"`
	Status           string `json:"status"`
	Reason           string `json:"reason"`
	Message          string `json:"message,omitempty"`
	LastTransitionTS string `json:"last_transition_ts"`
	LastEventID      string `json:"last_event_id,omitempty"`
}

type Audit struct {
	SchemaVersion int            `json:"schema_version"`
	Kind          string         `json:"kind"`
	Metadata      Metadata       `json:"metadata"`
	Spec          map[string]any `json:"spec"`
}

func DecodeEvent(data []byte) (Event, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	var raw map[string]json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return Event{}, fmt.Errorf("decode event: %w", err)
	}
	required := []string{
		"schema_version", "id", "ts", "type", "loop", "host", "actor",
		"source", "correlation_id", "caused_by", "payload",
	}
	for _, field := range required {
		if _, ok := raw[field]; !ok {
			return Event{}, fmt.Errorf("event missing required field %q", field)
		}
	}

	var event Event
	if err := json.Unmarshal(data, &event); err != nil {
		return Event{}, fmt.Errorf("decode event: %w", err)
	}
	if err := ValidateEvent(event); err != nil {
		return Event{}, err
	}
	return event, nil
}

func ValidateEvent(event Event) error {
	var errs []error
	if event.SchemaVersion != Version {
		errs = append(errs, fmt.Errorf("schema_version must be %d", Version))
	}
	if strings.TrimSpace(event.ID) == "" {
		errs = append(errs, errors.New("id is required"))
	}
	if _, err := time.Parse(time.RFC3339, event.TS); err != nil {
		errs = append(errs, fmt.Errorf("ts must be RFC3339: %w", err))
	}
	if !eventTypePattern.MatchString(event.Type) {
		errs = append(errs, errors.New("type must be lower-case dot-separated"))
	}
	if event.Loop != nil && strings.TrimSpace(*event.Loop) == "" {
		errs = append(errs, errors.New("loop must be null or non-empty"))
	}
	if event.Host != nil && strings.TrimSpace(*event.Host) == "" {
		errs = append(errs, errors.New("host must be null or non-empty"))
	}
	if _, ok := allowedActors[event.Actor]; !ok {
		errs = append(errs, fmt.Errorf("actor %q is not allowed", event.Actor))
	}
	if strings.TrimSpace(event.Source) == "" {
		errs = append(errs, errors.New("source is required"))
	}
	if strings.TrimSpace(event.CorrelationID) == "" {
		errs = append(errs, errors.New("correlation_id is required"))
	}
	if event.CausedBy != nil && strings.TrimSpace(*event.CausedBy) == "" {
		errs = append(errs, errors.New("caused_by must be null or non-empty"))
	}
	if event.Payload == nil {
		errs = append(errs, errors.New("payload must be an object"))
	}
	if event.Severity != "" && !oneOf(event.Severity, "debug", "info", "warning", "error", "critical") {
		errs = append(errs, fmt.Errorf("severity %q is not allowed", event.Severity))
	}
	return errors.Join(errs...)
}

func ValidateAudit(audit Audit) error {
	return validateControlledObject(audit.SchemaVersion, audit.Kind, "Audit", audit.Metadata, audit.Spec, map[string]any{})
}

func validateControlledObject(version int, kind, wantKind string, metadata Metadata, spec, status map[string]any) error {
	var errs []error
	if version != Version {
		errs = append(errs, fmt.Errorf("schema_version must be %d", Version))
	}
	if kind != wantKind {
		errs = append(errs, fmt.Errorf("kind must be %s", wantKind))
	}
	if strings.TrimSpace(metadata.Name) == "" {
		errs = append(errs, errors.New("metadata.name is required"))
	}
	if spec == nil {
		errs = append(errs, errors.New("spec is required"))
	}
	if status == nil {
		errs = append(errs, errors.New("status is required"))
	}
	return errors.Join(errs...)
}

func oneOf(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}
