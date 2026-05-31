package proposal

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

const SchemaVersion = "mnemon.proposal.v1"

type Status string

const (
	StatusDraft          Status = "draft"
	StatusOpen           Status = "open"
	StatusInReview       Status = "in_review"
	StatusApproved       Status = "approved"
	StatusRejected       Status = "rejected"
	StatusRequestChanges Status = "request_changes"
	StatusBlocked        Status = "blocked"
	StatusApplied        Status = "applied"
	StatusSuperseded     Status = "superseded"
	StatusWithdrawn      Status = "withdrawn"
	StatusExpired        Status = "expired"
)

type Route string

const (
	RouteMemory       Route = "memory"
	RouteSkill        Route = "skill"
	RouteEval         Route = "eval"
	RouteCoordination Route = "coordination"
	RouteProjection   Route = "projection"
	RouteHostAdapter  Route = "host_adapter"
	RouteDocs         Route = "docs"
	RoutePolicy       Route = "policy"
	RouteRuntime      Route = "runtime"
)

type Risk string

const (
	RiskLow      Risk = "low"
	RiskMedium   Risk = "medium"
	RiskHigh     Risk = "high"
	RiskCritical Risk = "critical"
)

type Proposal struct {
	SchemaVersion  string         `json:"schema_version"`
	Kind           string         `json:"kind"`
	ID             string         `json:"id"`
	Route          Route          `json:"route"`
	Status         Status         `json:"status"`
	Risk           Risk           `json:"risk"`
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

type ChangeRequest struct {
	Summary    string      `json:"summary"`
	Targets    []TargetRef `json:"targets"`
	Operations []Operation `json:"operations,omitempty"`
}

type TargetRef struct {
	Type string `json:"type"`
	URI  string `json:"uri"`
}

type Operation struct {
	Type    string         `json:"type"`
	Target  string         `json:"target"`
	Summary string         `json:"summary"`
	Payload map[string]any `json:"payload,omitempty"`
}

type EvidenceRef struct {
	Type    string `json:"type"`
	Ref     string `json:"ref"`
	Summary string `json:"summary,omitempty"`
}

type ValidationPlan struct {
	Summary          string   `json:"summary"`
	Commands         []string `json:"commands,omitempty"`
	Checks           []string `json:"checks,omitempty"`
	RequiredEvidence []string `json:"required_evidence,omitempty"`
}

type ReviewPolicy struct {
	Required        bool     `json:"required"`
	RequiredScope   string   `json:"required_scope,omitempty"`
	RequiredReviews int      `json:"required_reviews,omitempty"`
	Reviewers       []string `json:"reviewers,omitempty"`
	Notes           string   `json:"notes,omitempty"`
}

func New(id string, route Route, risk Risk, title, summary string, now time.Time) Proposal {
	ts := now.UTC().Truncate(time.Second).Format(time.RFC3339)
	return Proposal{
		SchemaVersion: SchemaVersion,
		Kind:          "Proposal",
		ID:            id,
		Route:         route,
		Status:        StatusDraft,
		Risk:          risk,
		Title:         title,
		Summary:       summary,
		CreatedAt:     ts,
		UpdatedAt:     ts,
		Review: ReviewPolicy{
			Required:        risk != RiskLow,
			RequiredScope:   "exact",
			RequiredReviews: 1,
		},
	}
}

func Validate(item Proposal) error {
	var errs []error
	if item.SchemaVersion != SchemaVersion {
		errs = append(errs, fmt.Errorf("schema_version must be %s", SchemaVersion))
	}
	if item.Kind != "Proposal" {
		errs = append(errs, errors.New("kind must be Proposal"))
	}
	if strings.TrimSpace(item.ID) == "" {
		errs = append(errs, errors.New("id is required"))
	}
	if err := ValidateRoute(item.Route); err != nil {
		errs = append(errs, err)
	}
	if err := ValidateStatus(item.Status); err != nil {
		errs = append(errs, err)
	}
	if err := ValidateRisk(item.Risk); err != nil {
		errs = append(errs, err)
	}
	if strings.TrimSpace(item.Title) == "" {
		errs = append(errs, errors.New("title is required"))
	}
	if strings.TrimSpace(item.Summary) == "" {
		errs = append(errs, errors.New("summary is required"))
	}
	if err := validateChange(item.Change); err != nil {
		errs = append(errs, fmt.Errorf("change: %w", err))
	}
	if err := validateValidationPlan(item.ValidationPlan); err != nil {
		errs = append(errs, fmt.Errorf("validation_plan: %w", err))
	}
	if err := validateReview(item.Risk, item.Review); err != nil {
		errs = append(errs, fmt.Errorf("review: %w", err))
	}
	if err := validateRFC3339("created_at", item.CreatedAt); err != nil {
		errs = append(errs, err)
	}
	if err := validateRFC3339("updated_at", item.UpdatedAt); err != nil {
		errs = append(errs, err)
	}
	if item.ClosedAt != "" {
		if err := validateRFC3339("closed_at", item.ClosedAt); err != nil {
			errs = append(errs, err)
		}
	}
	if IsTerminal(item.Status) && item.ClosedAt == "" {
		errs = append(errs, errors.New("closed_at is required for terminal status"))
	}
	if item.Status == StatusSuperseded && strings.TrimSpace(item.SupersededBy) == "" {
		errs = append(errs, errors.New("superseded_by is required when status is superseded"))
	}
	for i, ref := range item.Evidence {
		if strings.TrimSpace(ref.Type) == "" || strings.TrimSpace(ref.Ref) == "" {
			errs = append(errs, fmt.Errorf("evidence[%d] type and ref are required", i))
		}
	}
	return errors.Join(errs...)
}

func ValidateStatus(status Status) error {
	if !slices.Contains(allStatuses, status) {
		return fmt.Errorf("status %q is not allowed", status)
	}
	return nil
}

func ValidateRoute(route Route) error {
	if !slices.Contains(allRoutes, route) {
		return fmt.Errorf("route %q is not allowed", route)
	}
	return nil
}

func ValidateRisk(risk Risk) error {
	if !slices.Contains(allRisks, risk) {
		return fmt.Errorf("risk %q is not allowed", risk)
	}
	return nil
}

func Statuses() []Status {
	return append([]Status(nil), allStatuses...)
}

func CanTransition(from, to Status) bool {
	allowed, ok := transitions[from]
	return ok && slices.Contains(allowed, to)
}

func ValidateTransition(from, to Status) error {
	if err := ValidateStatus(from); err != nil {
		return err
	}
	if err := ValidateStatus(to); err != nil {
		return err
	}
	if !CanTransition(from, to) {
		return fmt.Errorf("proposal status transition %s -> %s is not allowed", from, to)
	}
	return nil
}

func Transition(item Proposal, next Status, now time.Time) (Proposal, error) {
	if err := ValidateTransition(item.Status, next); err != nil {
		return Proposal{}, err
	}
	item.Status = next
	ts := now.UTC().Truncate(time.Second).Format(time.RFC3339)
	item.UpdatedAt = ts
	if IsTerminal(next) {
		item.ClosedAt = ts
	}
	return item, nil
}

func IsTerminal(status Status) bool {
	return status == StatusApplied ||
		status == StatusRejected ||
		status == StatusSuperseded ||
		status == StatusWithdrawn ||
		status == StatusExpired
}

func validateChange(change ChangeRequest) error {
	var errs []error
	if strings.TrimSpace(change.Summary) == "" {
		errs = append(errs, errors.New("summary is required"))
	}
	if len(change.Targets) == 0 {
		errs = append(errs, errors.New("at least one target is required"))
	}
	for i, target := range change.Targets {
		if strings.TrimSpace(target.Type) == "" || strings.TrimSpace(target.URI) == "" {
			errs = append(errs, fmt.Errorf("targets[%d] type and uri are required", i))
		}
	}
	for i, operation := range change.Operations {
		if strings.TrimSpace(operation.Type) == "" || strings.TrimSpace(operation.Target) == "" {
			errs = append(errs, fmt.Errorf("operations[%d] type and target are required", i))
		}
	}
	return errors.Join(errs...)
}

func validateValidationPlan(plan ValidationPlan) error {
	if strings.TrimSpace(plan.Summary) == "" && len(plan.Commands) == 0 && len(plan.Checks) == 0 {
		return errors.New("summary, commands, or checks are required")
	}
	for i, command := range plan.Commands {
		if strings.TrimSpace(command) == "" {
			return fmt.Errorf("commands[%d] is empty", i)
		}
	}
	for i, check := range plan.Checks {
		if strings.TrimSpace(check) == "" {
			return fmt.Errorf("checks[%d] is empty", i)
		}
	}
	return nil
}

func validateReview(risk Risk, review ReviewPolicy) error {
	if risk == RiskLow && !review.Required {
		return nil
	}
	var errs []error
	if !review.Required {
		errs = append(errs, errors.New("review is required for medium, high, and critical risk"))
	}
	if strings.TrimSpace(review.RequiredScope) == "" {
		errs = append(errs, errors.New("required_scope is required"))
	}
	if review.RequiredReviews <= 0 {
		errs = append(errs, errors.New("required_reviews must be positive"))
	}
	return errors.Join(errs...)
}

func validateRFC3339(field, value string) error {
	if _, err := time.Parse(time.RFC3339, value); err != nil {
		return fmt.Errorf("%s must be RFC3339: %w", field, err)
	}
	return nil
}

var allStatuses = []Status{
	StatusDraft,
	StatusOpen,
	StatusInReview,
	StatusApproved,
	StatusRejected,
	StatusRequestChanges,
	StatusBlocked,
	StatusApplied,
	StatusSuperseded,
	StatusWithdrawn,
	StatusExpired,
}

var allRoutes = []Route{
	RouteMemory,
	RouteSkill,
	RouteEval,
	RouteCoordination,
	RouteProjection,
	RouteHostAdapter,
	RouteDocs,
	RoutePolicy,
	RouteRuntime,
}

var allRisks = []Risk{
	RiskLow,
	RiskMedium,
	RiskHigh,
	RiskCritical,
}

var transitions = map[Status][]Status{
	StatusDraft:          {StatusOpen, StatusWithdrawn, StatusExpired},
	StatusOpen:           {StatusInReview, StatusRequestChanges, StatusBlocked, StatusWithdrawn, StatusSuperseded, StatusExpired},
	StatusInReview:       {StatusApproved, StatusRejected, StatusRequestChanges, StatusBlocked, StatusWithdrawn, StatusSuperseded, StatusExpired},
	StatusRequestChanges: {StatusDraft, StatusOpen, StatusWithdrawn, StatusSuperseded, StatusExpired},
	StatusBlocked:        {StatusOpen, StatusInReview, StatusRejected, StatusWithdrawn, StatusSuperseded, StatusExpired},
	StatusApproved:       {StatusApplied, StatusSuperseded, StatusExpired},
	StatusRejected:       {},
	StatusApplied:        {},
	StatusSuperseded:     {},
	StatusWithdrawn:      {},
	StatusExpired:        {},
}
