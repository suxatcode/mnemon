package goal

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	SchemaVersion         = "mnemon.goal.v1"
	PlanSchemaVersion     = "mnemon.goal_plan.v1"
	EvidenceSchemaVersion = "mnemon.goal_evidence.v1"
	ReportSchemaVersion   = "mnemon.goal_report.v1"
	HostLinkSchemaVersion = "mnemon.host_goal_link.v1"
)

type Status string
type GoalStatus = Status

const (
	StatusDraft     Status = "draft"
	StatusPlanned   Status = "planned"
	StatusActive    Status = "active"
	StatusVerifying Status = "verifying"
	StatusComplete  Status = "complete"
	StatusBlocked   Status = "blocked"
	StatusPaused    Status = "paused"
)

type Goal struct {
	SchemaVersion string         `json:"schema_version"`
	Kind          string         `json:"kind"`
	ID            string         `json:"id"`
	Objective     string         `json:"objective"`
	Status        Status         `json:"status"`
	CreatedAt     string         `json:"created_at"`
	UpdatedAt     string         `json:"updated_at"`
	CompletedAt   string         `json:"completed_at,omitempty"`
	BlockedAt     string         `json:"blocked_at,omitempty"`
	PausedAt      string         `json:"paused_at,omitempty"`
	Plan          *GoalPlan      `json:"plan,omitempty"`
	Report        *GoalReport    `json:"report,omitempty"`
	HostLinks     []HostGoalLink `json:"host_links,omitempty"`
	EvidenceCount int            `json:"evidence_count"`
	LatestEventID string         `json:"latest_event_id,omitempty"`
}

type GoalPlan struct {
	SchemaVersion        string   `json:"schema_version"`
	Kind                 string   `json:"kind"`
	GoalID               string   `json:"goal_id"`
	Summary              string   `json:"summary"`
	Steps                []string `json:"steps"`
	MemoryRefs           []string `json:"memory_refs,omitempty"`
	MemoryRecallRequests []string `json:"memory_recall_requests,omitempty"`
	SkillWorkflowRefs    []string `json:"skill_workflow_refs,omitempty"`
	EvalRefs             []string `json:"eval_refs,omitempty"`
	CreatedAt            string   `json:"created_at"`
	UpdatedAt            string   `json:"updated_at"`
}

type GoalEvidence struct {
	SchemaVersion string       `json:"schema_version"`
	Kind          string       `json:"kind"`
	ID            string       `json:"id"`
	GoalID        string       `json:"goal_id"`
	Type          string       `json:"type"`
	Status        string       `json:"status"`
	Summary       string       `json:"summary"`
	RecordedAt    string       `json:"recorded_at"`
	Refs          EvidenceRefs `json:"refs,omitempty"`
}

type EvidenceRefs struct {
	MemoryRefs       []string `json:"memory_refs,omitempty"`
	MemoryRequests   []string `json:"memory_requests,omitempty"`
	SkillSignals     []string `json:"skill_signals,omitempty"`
	EvalReportRefs   []string `json:"eval_report_refs,omitempty"`
	ArtifactRefs     []string `json:"artifact_refs,omitempty"`
	AuditRefs        []string `json:"audit_refs,omitempty"`
	ProposalRefs     []string `json:"proposal_refs,omitempty"`
	HostEvidenceRefs []string `json:"host_evidence_refs,omitempty"`
}

type GoalReport struct {
	SchemaVersion    string           `json:"schema_version"`
	Kind             string           `json:"kind"`
	ID               string           `json:"id"`
	GoalID           string           `json:"goal_id"`
	Status           string           `json:"status"`
	Summary          string           `json:"summary"`
	GeneratedAt      string           `json:"generated_at"`
	VerificationGate VerificationGate `json:"verification_gate"`
	EvidenceRefs     []string         `json:"evidence_refs,omitempty"`
	EvalReportRefs   []string         `json:"eval_report_refs,omitempty"`
	ArtifactRefs     []string         `json:"artifact_refs,omitempty"`
	AuditRefs        []string         `json:"audit_refs,omitempty"`
	ProposalRefs     []string         `json:"proposal_refs,omitempty"`
	NoopReportRefs   []string         `json:"noop_report_refs,omitempty"`
}

type VerificationGate struct {
	Name      string `json:"name"`
	Passed    bool   `json:"passed"`
	CheckedAt string `json:"checked_at"`
	Message   string `json:"message,omitempty"`
}

type HostGoalLink struct {
	SchemaVersion string   `json:"schema_version"`
	Kind          string   `json:"kind"`
	ID            string   `json:"id"`
	GoalID        string   `json:"goal_id"`
	Host          string   `json:"host"`
	ThreadID      string   `json:"thread_id,omitempty"`
	HostGoalID    string   `json:"host_goal_id,omitempty"`
	Objective     string   `json:"objective"`
	Evidence      []string `json:"evidence,omitempty"`
	LinkedAt      string   `json:"linked_at"`
}

func ValidateGoal(item Goal) error {
	var errs []error
	if item.SchemaVersion != SchemaVersion {
		errs = append(errs, fmt.Errorf("schema_version must be %s", SchemaVersion))
	}
	if item.Kind != "Goal" {
		errs = append(errs, errors.New("kind must be Goal"))
	}
	if strings.TrimSpace(item.ID) == "" {
		errs = append(errs, errors.New("id is required"))
	}
	if strings.TrimSpace(item.Objective) == "" {
		errs = append(errs, errors.New("objective is required"))
	}
	if err := ValidateStatus(item.Status); err != nil {
		errs = append(errs, err)
	}
	if err := validateRFC3339("created_at", item.CreatedAt); err != nil {
		errs = append(errs, err)
	}
	if err := validateRFC3339("updated_at", item.UpdatedAt); err != nil {
		errs = append(errs, err)
	}
	if item.CompletedAt != "" {
		if err := validateRFC3339("completed_at", item.CompletedAt); err != nil {
			errs = append(errs, err)
		}
	}
	if item.BlockedAt != "" {
		if err := validateRFC3339("blocked_at", item.BlockedAt); err != nil {
			errs = append(errs, err)
		}
	}
	if item.PausedAt != "" {
		if err := validateRFC3339("paused_at", item.PausedAt); err != nil {
			errs = append(errs, err)
		}
	}
	if item.Plan != nil {
		if err := ValidatePlan(*item.Plan); err != nil {
			errs = append(errs, fmt.Errorf("plan: %w", err))
		}
	}
	if item.Report != nil {
		if err := ValidateReport(*item.Report); err != nil {
			errs = append(errs, fmt.Errorf("report: %w", err))
		}
	}
	for i, link := range item.HostLinks {
		if err := ValidateHostGoalLink(link); err != nil {
			errs = append(errs, fmt.Errorf("host_links[%d]: %w", i, err))
		}
	}
	if item.EvidenceCount < 0 {
		errs = append(errs, errors.New("evidence_count must be non-negative"))
	}
	return errors.Join(errs...)
}

func ValidatePlan(item GoalPlan) error {
	var errs []error
	if item.SchemaVersion != PlanSchemaVersion {
		errs = append(errs, fmt.Errorf("schema_version must be %s", PlanSchemaVersion))
	}
	if item.Kind != "GoalPlan" {
		errs = append(errs, errors.New("kind must be GoalPlan"))
	}
	if strings.TrimSpace(item.GoalID) == "" {
		errs = append(errs, errors.New("goal_id is required"))
	}
	if strings.TrimSpace(item.Summary) == "" && len(item.Steps) == 0 {
		errs = append(errs, errors.New("summary or steps are required"))
	}
	for i, step := range item.Steps {
		if strings.TrimSpace(step) == "" {
			errs = append(errs, fmt.Errorf("steps[%d] is empty", i))
		}
	}
	if err := validateRFC3339("created_at", item.CreatedAt); err != nil {
		errs = append(errs, err)
	}
	if err := validateRFC3339("updated_at", item.UpdatedAt); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func ValidateEvidence(item GoalEvidence) error {
	var errs []error
	if item.SchemaVersion != EvidenceSchemaVersion {
		errs = append(errs, fmt.Errorf("schema_version must be %s", EvidenceSchemaVersion))
	}
	if item.Kind != "GoalEvidence" {
		errs = append(errs, errors.New("kind must be GoalEvidence"))
	}
	if strings.TrimSpace(item.ID) == "" {
		errs = append(errs, errors.New("id is required"))
	}
	if strings.TrimSpace(item.GoalID) == "" {
		errs = append(errs, errors.New("goal_id is required"))
	}
	if !oneOf(item.Type, "manual", "memory", "skill", "eval", "artifact", "audit", "proposal", "host", "app-server", "verification", "blocker") {
		errs = append(errs, fmt.Errorf("type %q is not allowed", item.Type))
	}
	if !oneOf(item.Status, "accepted", "rejected", "degraded", "blocked") {
		errs = append(errs, fmt.Errorf("status %q is not allowed", item.Status))
	}
	if strings.TrimSpace(item.Summary) == "" {
		errs = append(errs, errors.New("summary is required"))
	}
	if err := validateRFC3339("recorded_at", item.RecordedAt); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func ValidateReport(item GoalReport) error {
	var errs []error
	if item.SchemaVersion != ReportSchemaVersion {
		errs = append(errs, fmt.Errorf("schema_version must be %s", ReportSchemaVersion))
	}
	if item.Kind != "GoalReport" {
		errs = append(errs, errors.New("kind must be GoalReport"))
	}
	if strings.TrimSpace(item.ID) == "" {
		errs = append(errs, errors.New("id is required"))
	}
	if strings.TrimSpace(item.GoalID) == "" {
		errs = append(errs, errors.New("goal_id is required"))
	}
	if !oneOf(item.Status, "pass", "fail", "blocked") {
		errs = append(errs, fmt.Errorf("status %q is not allowed", item.Status))
	}
	if strings.TrimSpace(item.Summary) == "" {
		errs = append(errs, errors.New("summary is required"))
	}
	if err := validateRFC3339("generated_at", item.GeneratedAt); err != nil {
		errs = append(errs, err)
	}
	if strings.TrimSpace(item.VerificationGate.Name) == "" {
		errs = append(errs, errors.New("verification_gate.name is required"))
	}
	if err := validateRFC3339("verification_gate.checked_at", item.VerificationGate.CheckedAt); err != nil {
		errs = append(errs, err)
	}
	if item.Status == "pass" && !item.VerificationGate.Passed {
		errs = append(errs, errors.New("passing report requires verification_gate.passed"))
	}
	if item.Status == "pass" && len(item.EvidenceRefs) == 0 {
		errs = append(errs, errors.New("passing report requires evidence_refs"))
	}
	return errors.Join(errs...)
}

func ValidateHostGoalLink(item HostGoalLink) error {
	var errs []error
	if item.SchemaVersion != HostLinkSchemaVersion {
		errs = append(errs, fmt.Errorf("schema_version must be %s", HostLinkSchemaVersion))
	}
	if item.Kind != "HostGoalLink" {
		errs = append(errs, errors.New("kind must be HostGoalLink"))
	}
	if strings.TrimSpace(item.ID) == "" {
		errs = append(errs, errors.New("id is required"))
	}
	if strings.TrimSpace(item.GoalID) == "" {
		errs = append(errs, errors.New("goal_id is required"))
	}
	if strings.TrimSpace(item.Host) == "" {
		errs = append(errs, errors.New("host is required"))
	}
	if strings.TrimSpace(item.ThreadID) == "" && strings.TrimSpace(item.HostGoalID) == "" {
		errs = append(errs, errors.New("thread_id or host_goal_id is required"))
	}
	if strings.TrimSpace(item.Objective) == "" {
		errs = append(errs, errors.New("objective is required"))
	}
	if err := validateRFC3339("linked_at", item.LinkedAt); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func ValidateStatus(status Status) error {
	if oneOf(string(status),
		string(StatusDraft),
		string(StatusPlanned),
		string(StatusActive),
		string(StatusVerifying),
		string(StatusComplete),
		string(StatusBlocked),
		string(StatusPaused),
	) {
		return nil
	}
	return fmt.Errorf("status %q is not allowed", status)
}

func CompletionReady(report *GoalReport, evidence []GoalEvidence) bool {
	if report == nil || report.Status != "pass" || !report.VerificationGate.Passed {
		return false
	}
	accepted := map[string]struct{}{}
	for _, item := range evidence {
		if item.Status == "accepted" {
			accepted[item.ID] = struct{}{}
		}
	}
	if len(accepted) == 0 {
		return false
	}
	for _, ref := range report.EvidenceRefs {
		if _, ok := accepted[ref]; ok {
			return true
		}
	}
	return false
}

func Terminal(status Status) bool {
	return status == StatusComplete || status == StatusBlocked
}

type TransitionError struct {
	From Status
	To   Status
}

func (e TransitionError) Error() string {
	return fmt.Sprintf("invalid goal status transition %s -> %s", e.From, e.To)
}

func ValidateTransition(from, to Status) error {
	if err := ValidateStatus(from); err != nil {
		return err
	}
	if err := ValidateStatus(to); err != nil {
		return err
	}
	if CanTransition(from, to) {
		return nil
	}
	return TransitionError{From: from, To: to}
}

func CanTransition(from, to Status) bool {
	switch from {
	case StatusDraft:
		return oneOf(string(to), string(StatusPlanned), string(StatusActive), string(StatusPaused), string(StatusBlocked))
	case StatusPlanned:
		return oneOf(string(to), string(StatusActive), string(StatusVerifying), string(StatusPaused), string(StatusBlocked))
	case StatusActive:
		return oneOf(string(to), string(StatusVerifying), string(StatusPaused), string(StatusBlocked))
	case StatusVerifying:
		return oneOf(string(to), string(StatusVerifying), string(StatusComplete), string(StatusPaused), string(StatusBlocked))
	case StatusPaused:
		return oneOf(string(to), string(StatusActive), string(StatusBlocked))
	case StatusComplete, StatusBlocked:
		return false
	default:
		return false
	}
}

func validateRFC3339(field string, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", field)
	}
	if _, err := time.Parse(time.RFC3339, value); err != nil {
		return fmt.Errorf("%s must be RFC3339: %w", field, err)
	}
	return nil
}

func oneOf(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}
