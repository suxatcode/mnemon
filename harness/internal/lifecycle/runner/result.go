package runner

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
)

const ResultSchemaVersion = "mnemon.runner_result.v1"

type Result struct {
	SchemaVersion      string         `json:"schema_version"`
	Kind               string         `json:"kind"`
	JobID              string         `json:"job_id"`
	RunnerID           string         `json:"runner_id"`
	Host               string         `json:"host"`
	ThreadID           string         `json:"thread_id,omitempty"`
	TurnCount          int            `json:"turn_count"`
	Status             string         `json:"status"`
	Outcome            string         `json:"outcome"`
	Summary            string         `json:"summary"`
	ArtifactRefs       []ArtifactRef  `json:"artifact_refs"`
	RecommendedEvents  []schema.Event `json:"recommended_events,omitempty"`
	ProposalCandidates []RawObject    `json:"proposal_candidates,omitempty"`
	AuditCandidates    []RawObject    `json:"audit_candidates,omitempty"`
	Conditions         []Condition    `json:"conditions,omitempty"`
}

type ArtifactRef struct {
	ID                 string `json:"id"`
	Kind               string `json:"kind"`
	URI                string `json:"uri"`
	MediaType          string `json:"media_type"`
	SHA256             string `json:"sha256,omitempty"`
	PreRedactionSHA256 string `json:"pre_redaction_sha256,omitempty"`
	Privacy            string `json:"privacy"`
}

type Condition struct {
	Type    string `json:"type"`
	Reason  string `json:"reason"`
	Message string `json:"message,omitempty"`
}

type RawObject map[string]any

type Budget struct {
	MaxTurns  int `json:"max_turns"`
	UsedTurns int `json:"used_turns"`
}

type ValidateOptions struct {
	Budget               Budget
	ArtifactRoot         string
	RequireArtifactFiles bool
}

func ValidateResult(result Result, opts ValidateOptions) error {
	var errs []error
	if result.SchemaVersion != ResultSchemaVersion {
		errs = append(errs, fmt.Errorf("schema_version must be %s", ResultSchemaVersion))
	}
	if result.Kind != "HostAgentRunnerResult" {
		errs = append(errs, errors.New("kind must be HostAgentRunnerResult"))
	}
	if strings.TrimSpace(result.JobID) == "" {
		errs = append(errs, errors.New("job_id is required"))
	}
	if strings.TrimSpace(result.RunnerID) == "" {
		errs = append(errs, errors.New("runner_id is required"))
	}
	if strings.TrimSpace(result.Host) == "" {
		errs = append(errs, errors.New("host is required"))
	}
	if result.TurnCount < 0 {
		errs = append(errs, errors.New("turn_count must be non-negative"))
	}
	if opts.Budget.MaxTurns > 0 && result.TurnCount > opts.Budget.MaxTurns {
		errs = append(errs, fmt.Errorf("turn_count exceeds max turns budget %d", opts.Budget.MaxTurns))
	}
	if !oneOf(result.Status, "completed", "failed", "blocked", "timeout", "interrupted", "invalid") {
		errs = append(errs, fmt.Errorf("status %q is not allowed", result.Status))
	}
	if !oneOf(result.Outcome, "pass", "weak", "fail", "invalid", "inconclusive", "noop", "proposal") {
		errs = append(errs, fmt.Errorf("outcome %q is not allowed", result.Outcome))
	}
	if strings.TrimSpace(result.Summary) == "" {
		errs = append(errs, errors.New("summary is required"))
	}
	if len(result.ArtifactRefs) == 0 {
		errs = append(errs, errors.New("artifact_refs is required"))
	}
	for index, ref := range result.ArtifactRefs {
		if err := validateArtifactRef(ref, opts); err != nil {
			errs = append(errs, fmt.Errorf("artifact_refs[%d]: %w", index, err))
		}
	}
	for index, event := range result.RecommendedEvents {
		if err := schema.ValidateEvent(event); err != nil {
			errs = append(errs, fmt.Errorf("recommended_events[%d]: %w", index, err))
		}
	}
	return errors.Join(errs...)
}

func (budget Budget) Remaining() int {
	if budget.MaxTurns <= 0 {
		return 0
	}
	remaining := budget.MaxTurns - budget.UsedTurns
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (budget Budget) Allows(turns int) bool {
	if turns < 0 {
		return false
	}
	if budget.MaxTurns <= 0 {
		return true
	}
	return budget.UsedTurns+turns <= budget.MaxTurns
}

func validateArtifactRef(ref ArtifactRef, opts ValidateOptions) error {
	var errs []error
	if strings.TrimSpace(ref.ID) == "" {
		errs = append(errs, errors.New("id is required"))
	}
	if strings.TrimSpace(ref.Kind) == "" {
		errs = append(errs, errors.New("kind is required"))
	}
	if strings.TrimSpace(ref.URI) == "" {
		errs = append(errs, errors.New("uri is required"))
	}
	if strings.TrimSpace(ref.MediaType) == "" {
		errs = append(errs, errors.New("media_type is required"))
	}
	if strings.TrimSpace(ref.Privacy) == "" {
		errs = append(errs, errors.New("privacy is required"))
	}
	if opts.RequireArtifactFiles {
		path := ref.URI
		if opts.ArtifactRoot != "" && !filepath.IsAbs(path) {
			path = filepath.Join(opts.ArtifactRoot, path)
		}
		if _, err := os.Stat(path); err != nil {
			errs = append(errs, fmt.Errorf("artifact file missing: %w", err))
		}
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
