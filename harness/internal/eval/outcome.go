package eval

import (
	"errors"
	"strings"
)

type Outcome string

const (
	OutcomePass         Outcome = "pass"
	OutcomeWeak         Outcome = "weak"
	OutcomeFail         Outcome = "fail"
	OutcomeInvalid      Outcome = "invalid"
	OutcomeInconclusive Outcome = "inconclusive"
	OutcomeNoop         Outcome = "noop"
	OutcomeProposal     Outcome = "proposal"
)

type OutcomeInput struct {
	Assertions       []AssertionResult
	AssertionErr     error
	ProposalRequired bool
}

type RoutingOptions struct {
	RunID     string
	ReportRef string
}

type ProposalCandidate struct {
	Kind       string            `json:"kind"`
	Route      string            `json:"route"`
	Risk       string            `json:"risk"`
	Title      string            `json:"title"`
	Summary    string            `json:"summary"`
	ScenarioID string            `json:"scenario_id"`
	Source     string            `json:"source,omitempty"`
	EvidenceID string            `json:"evidence_id,omitempty"`
	Area       string            `json:"area"`
	Outcome    Outcome           `json:"outcome"`
	Assertions []AssertionResult `json:"assertions,omitempty"`
	Evidence   []EvidenceRef     `json:"evidence,omitempty"`
	Metadata   map[string]any    `json:"metadata,omitempty"`
}

type EvidenceRef struct {
	Type    string `json:"type"`
	Ref     string `json:"ref"`
	Summary string `json:"summary,omitempty"`
}

func DeriveOutcome(input OutcomeInput) Outcome {
	if input.AssertionErr != nil {
		return OutcomeInvalid
	}
	if input.ProposalRequired {
		return OutcomeProposal
	}
	if len(input.Assertions) == 0 {
		return OutcomeNoop
	}
	failed := len(FailedAssertions(input.Assertions))
	switch {
	case failed == 0:
		return OutcomePass
	case failed < len(input.Assertions):
		return OutcomeWeak
	default:
		return OutcomeFail
	}
}

func OutcomeNeedsProposal(outcome Outcome) bool {
	switch outcome {
	case OutcomeWeak, OutcomeFail, OutcomeProposal:
		return true
	default:
		return false
	}
}

func ScenarioArea(scenario Scenario) string {
	if area := normalizeArea(scenario.Area); area != "" {
		return area
	}
	for _, loop := range scenario.Loops {
		area := normalizeArea(loop)
		if area != "" && area != "eval" {
			return area
		}
	}
	for _, prefix := range []string{"memory", "skill", "eval", "docs", "projection", "policy", "runtime"} {
		if strings.HasPrefix(scenario.ID, prefix+"-") || strings.HasPrefix(scenario.ID, prefix+"/") {
			return prefix
		}
	}
	if strings.HasPrefix(scenario.ID, "host-") || strings.HasPrefix(scenario.ID, "ops-") {
		return "projection"
	}
	return "eval"
}

func ProposalRouteForArea(area string) string {
	switch normalizeArea(area) {
	case "memory":
		return "memory"
	case "skill":
		return "skill"
	case "projection":
		return "projection"
	case "host_adapter":
		return "host_adapter"
	case "docs":
		return "docs"
	case "policy":
		return "policy"
	case "runtime":
		return "runtime"
	case "eval":
		return "eval"
	default:
		return "eval"
	}
}

func ValidateOutcome(outcome Outcome) error {
	switch outcome {
	case OutcomePass, OutcomeWeak, OutcomeFail, OutcomeInvalid, OutcomeInconclusive, OutcomeNoop, OutcomeProposal:
		return nil
	default:
		return errors.New("outcome is not allowed")
	}
}

func normalizeArea(area string) string {
	area = strings.TrimSpace(strings.ToLower(area))
	area = strings.ReplaceAll(area, "-", "_")
	switch area {
	case "memory", "skill", "eval", "projection", "docs", "policy", "runtime", "host_adapter":
		return area
	case "host":
		return "host_adapter"
	case "ops":
		return "projection"
	default:
		return ""
	}
}

func riskForOutcome(outcome Outcome) string {
	switch outcome {
	case OutcomeWeak:
		return "low"
	case OutcomeProposal:
		return "medium"
	default:
		return "medium"
	}
}

func proposalEvidence(opts RoutingOptions) []EvidenceRef {
	var evidence []EvidenceRef
	if strings.TrimSpace(opts.ReportRef) != "" {
		evidence = append(evidence, EvidenceRef{
			Type:    "eval_report",
			Ref:     opts.ReportRef,
			Summary: "Eval runner report containing assertion evidence.",
		})
	}
	if strings.TrimSpace(opts.RunID) != "" {
		evidence = append(evidence, EvidenceRef{
			Type:    "eval_run",
			Ref:     opts.RunID,
			Summary: "Eval run identifier.",
		})
	}
	return evidence
}
