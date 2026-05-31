package eval

import "fmt"

type EvidenceItem struct {
	ID         string
	Source     string
	Area       string
	Outcome    Outcome
	Risk       string
	Summary    string
	Refs       []EvidenceRef
	Assertions []AssertionResult
	Metadata   map[string]any
}

func RouteEvidence(items []EvidenceItem) []ProposalCandidate {
	var candidates []ProposalCandidate
	for _, item := range items {
		if !OutcomeNeedsProposal(item.Outcome) {
			continue
		}
		area := normalizeArea(item.Area)
		if area == "" {
			area = "eval"
		}
		route := ProposalRouteForArea(area)
		risk := item.Risk
		if risk == "" {
			risk = riskForOutcome(item.Outcome)
		}
		assertions := FailedAssertions(item.Assertions)
		if len(assertions) == 0 {
			assertions = append([]AssertionResult(nil), item.Assertions...)
		}
		summary := item.Summary
		if summary == "" {
			summary = fmt.Sprintf("%s evidence %s produced outcome %s and needs %s lifecycle review.", item.Source, item.ID, item.Outcome, route)
		}
		candidate := ProposalCandidate{
			Kind:       "ProposalCandidate",
			Route:      route,
			Risk:       risk,
			Title:      proposalCandidateTitle(route, item),
			Summary:    summary,
			ScenarioID: scenarioIDForEvidence(item),
			Source:     item.Source,
			EvidenceID: item.ID,
			Area:       area,
			Outcome:    item.Outcome,
			Assertions: assertions,
			Evidence:   append([]EvidenceRef(nil), item.Refs...),
			Metadata:   item.Metadata,
		}
		candidates = append(candidates, candidate)
	}
	return candidates
}

func RouteEvalReport(report RunReport, scenario Scenario, outcome Outcome, assertions []AssertionResult) []ProposalCandidate {
	reportRef := report.Source
	if reportRef == "" && report.RunID != "" {
		reportRef = RunReportPath("", report.RunID)
	}
	return RouteEvidence([]EvidenceItem{{
		ID:         scenario.ID,
		Source:     "eval",
		Area:       ScenarioArea(scenario),
		Outcome:    outcome,
		Summary:    fmt.Sprintf("Eval scenario %s produced outcome %s and needs %s lifecycle review.", scenario.ID, outcome, ProposalRouteForArea(ScenarioArea(scenario))),
		Refs:       proposalEvidence(RoutingOptions{RunID: report.RunID, ReportRef: reportRef}),
		Assertions: assertions,
		Metadata: map[string]any{
			"run_id":     report.RunID,
			"job_id":     report.JobID,
			"job_spec":   report.JobSpec,
			"runner_id":  report.RunnerID,
			"report_ref": reportRef,
		},
	}})
}

func proposalCandidateTitle(route string, item EvidenceItem) string {
	if item.Source == "eval" && item.ID != "" {
		return fmt.Sprintf("Review %s eval outcome for %s", route, item.ID)
	}
	if item.Source != "" && item.ID != "" {
		return fmt.Sprintf("Review %s evidence from %s:%s", route, item.Source, item.ID)
	}
	return fmt.Sprintf("Review %s evidence", route)
}

func scenarioIDForEvidence(item EvidenceItem) string {
	if item.Source == "eval" {
		return item.ID
	}
	return ""
}
