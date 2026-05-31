package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	harnesseval "github.com/mnemon-dev/mnemon/harness/internal/eval"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/proposal"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/proposalstore"
	runnercodex "github.com/mnemon-dev/mnemon/harness/internal/lifecycle/runner/codex"
)

// EvalRunInput carries the eval run parameters from the surface flags.
type EvalRunInput struct {
	Suite                string
	Scenario             string
	Host                 string
	Command              string
	Timeout              time.Duration
	TurnTimeout          time.Duration
	MaxTurns             int
	IsolatedHome         bool
	AgentTurn            bool
	AcknowledgeModelCost bool
}

// EvalABInput carries the A/B test parameters from the surface flags.
type EvalABInput struct {
	Suite                string
	Scenarios            []string
	TrialsPerArm         int
	Command              string
	Timeout              time.Duration
	TurnTimeout          time.Duration
	MaxTurns             int
	IsolatedHome         bool
	AgentTurn            bool
	AcknowledgeModelCost bool
	ControlSetupJSON     string
	TreatmentSetupJSON   string
}

// EvalPromoteInput carries the asset promotion parameters from the surface flags.
type EvalPromoteInput struct {
	Scenario      string
	Suite         string
	Rubric        string
	Target        string
	From          string
	ProposalRef   string
	AuditRef      string
	EventID       string
	CorrelationID string
	CausedBy      string
}

func (h *Harness) EvalPlan(out io.Writer, suite, format string) error {
	loaded, err := harnesseval.LoadSuite(h.root, suite)
	if err != nil {
		return err
	}
	switch format {
	case "text", "":
		return writeEvalPlanText(out, loaded)
	case "json":
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(loaded)
	default:
		return fmt.Errorf("unsupported --format %q", format)
	}
}

func (h *Harness) EvalRun(ctx context.Context, out io.Writer, in EvalRunInput) error {
	plan, err := harnesseval.BuildRunPlan(h.root, in.Suite, in.Scenario)
	if err != nil {
		return err
	}
	host := in.Host
	if host == "" {
		host = plan.Suite.Host
	}
	if host == "" {
		host = "codex"
	}
	if host != "codex" {
		return fmt.Errorf("eval run currently supports host %q only; got %q", "codex", host)
	}
	runner := plan.Suite.Runner
	if runner == "" {
		runner = runnercodex.RunnerID
	}
	if runner != runnercodex.RunnerID {
		return fmt.Errorf("eval run currently supports runner %q only; suite %q declares %q", runnercodex.RunnerID, plan.Suite.Name, runner)
	}

	if ctx == nil {
		ctx = context.Background()
	}
	result, err := runnercodex.Run(ctx, h.root, runnercodex.RunOptions{
		CheckOptions: runnercodex.CheckOptions{
			Command:          in.Command,
			Timeout:          in.Timeout,
			IsolateCodexHome: in.IsolatedHome,
		},
		JobID:                evalRunJobID(plan.Suite.Name, plan.ScenarioID),
		JobSpec:              "eval." + plan.ScenarioID,
		Loop:                 "eval",
		Prompt:               plan.Prompt,
		Prompts:              plan.Prompts,
		TurnTimeout:          in.TurnTimeout,
		MaxTurns:             in.MaxTurns,
		AllowRealTurn:        in.AgentTurn,
		AcknowledgeModelCost: in.AcknowledgeModelCost,
		DeclarationRoot:      h.root,
		ProjectLoops:         plan.ProjectLoops,
		WorkspaceEnv: func(workspace runnercodex.WorkspaceContext) []string {
			return harnesseval.SetupEnvPairs(harnesseval.SetupEnv(workspace.MnemonDir, plan.ProjectLoops))
		},
		SetupWorkspace: func(ctx context.Context, workspace runnercodex.WorkspaceContext) error {
			handler := ""
			if plan.Scenario != nil {
				handler = plan.Scenario.SetupHandler
			}
			env := harnesseval.SetupEnv(workspace.MnemonDir, plan.ProjectLoops)
			return harnesseval.SetupRuntime{}.Run(ctx, harnesseval.SetupOptions{
				Handler:      handler,
				WorkspaceDir: workspace.Workspace,
				MnemonDir:    workspace.MnemonDir,
				Loops:        plan.ProjectLoops,
				Env:          env,
			})
		},
	})
	if err != nil {
		return err
	}
	post, err := FinalizeEvalRun(ctx, h.root, plan, result)
	if err != nil {
		return err
	}
	if result.FailureClass != "" {
		fmt.Fprintf(out, "eval run: %s (%s): %s\n", result.Status, result.FailureClass, result.Message)
	} else {
		fmt.Fprintf(out, "eval run: %s: %s\n", result.Status, result.Message)
	}
	fmt.Fprintf(out, "suite: %s\n", plan.Suite.Name)
	fmt.Fprintf(out, "scenario: %s\n", plan.ScenarioID)
	fmt.Fprintf(out, "host: %s\n", host)
	fmt.Fprintf(out, "runner: %s\n", runner)
	fmt.Fprintf(out, "projected loops: %s\n", strings.Join(plan.ProjectLoops, ", "))
	fmt.Fprintf(out, "run-id: %s\n", result.RunID)
	fmt.Fprintf(out, "turns: %d\n", result.TurnCount)
	fmt.Fprintf(out, "report: %s\n", result.ReportPath)
	if post.Outcome != "" {
		fmt.Fprintf(out, "outcome: %s\n", post.Outcome)
		fmt.Fprintf(out, "assertions: %d\n", len(post.Assertions))
	}
	for _, item := range post.Proposals {
		fmt.Fprintf(out, "proposal: %s route=%s status=%s\n", item.ID, item.Route, item.Status)
	}
	return nil
}

type EvalRunPostProcess struct {
	Outcome    harnesseval.Outcome
	Assertions []harnesseval.AssertionResult
	Proposals  []proposal.Proposal
}

func FinalizeEvalRun(ctx context.Context, root string, plan harnesseval.RunPlan, result runnercodex.RunResult) (EvalRunPostProcess, error) {
	if result.Status != runnercodex.StatusReady || plan.Scenario == nil {
		return EvalRunPostProcess{}, nil
	}
	report, err := harnesseval.LoadRunReport(root, result.RunID)
	if err != nil {
		return EvalRunPostProcess{}, err
	}
	transcript, err := harnesseval.LoadRunTranscriptReport(root, result.RunID)
	if err != nil {
		return EvalRunPostProcess{}, err
	}
	mnemonDir := result.Workspace
	if strings.TrimSpace(mnemonDir) != "" {
		mnemonDir = filepath.Join(mnemonDir, ".mnemon")
	}
	env := harnesseval.SetupEnv(mnemonDir, plan.ProjectLoops)
	assertions, assertErr := harnesseval.AssertionRuntime{Root: root}.Run(ctx, harnesseval.AssertionRunOptions{
		Backend:      harnesseval.AssertionBackend(plan.Scenario.AssertionBackend),
		ScenarioID:   plan.ScenarioID,
		Handler:      plan.Scenario.AssertionHandler,
		Report:       transcript.ReportMap(),
		WorkspaceDir: result.Workspace,
		MnemonDir:    mnemonDir,
		Env:          env,
	})
	outcome := harnesseval.DeriveOutcome(harnesseval.OutcomeInput{Assertions: assertions, AssertionErr: assertErr})
	if assertErr != nil {
		return EvalRunPostProcess{Outcome: outcome, Assertions: assertions}, fmt.Errorf("eval assertion failed: %w", assertErr)
	}
	candidates := harnesseval.RouteEvalReport(report, *plan.Scenario, outcome, assertions)
	proposals, err := createEvalProposalDrafts(root, plan.Suite.Name, candidates)
	if err != nil {
		return EvalRunPostProcess{}, err
	}
	return EvalRunPostProcess{
		Outcome:    outcome,
		Assertions: assertions,
		Proposals:  proposals,
	}, nil
}

func createEvalProposalDrafts(root, suite string, candidates []harnesseval.ProposalCandidate) ([]proposal.Proposal, error) {
	if len(candidates) == 0 {
		return nil, nil
	}
	store, err := proposalstore.New(root)
	if err != nil {
		return nil, err
	}
	var proposals []proposal.Proposal
	for _, candidate := range candidates {
		item, err := store.Create(proposalstore.CreateOptions{
			ID:      evalProposalID(candidate),
			Route:   proposal.Route(candidate.Route),
			Risk:    proposal.Risk(candidate.Risk),
			Title:   candidate.Title,
			Summary: candidate.Summary,
			Change: proposal.ChangeRequest{
				Summary: candidate.Summary,
				Targets: []proposal.TargetRef{{
					Type: "route",
					URI:  candidate.Route,
				}},
				Operations: []proposal.Operation{{
					Type:    "review",
					Target:  candidate.Route,
					Summary: "Review routed eval evidence and decide the owning loop response.",
				}},
			},
			Evidence:       evalCandidateEvidence(candidate.Evidence),
			ValidationPlan: evalCandidateValidation(suite, candidate),
			Now:            time.Now().UTC(),
		})
		if err != nil {
			return nil, err
		}
		proposals = append(proposals, item)
	}
	return proposals, nil
}

func (h *Harness) EvalAssert(ctx context.Context, out io.Writer, suite, scenario, runIDFlag string) error {
	plan, err := harnesseval.BuildRunPlan(h.root, suite, scenario)
	if err != nil {
		return err
	}
	if plan.Scenario == nil {
		return fmt.Errorf("scenario metadata is required for assertion-only eval: %s", plan.ScenarioID)
	}
	runID := strings.TrimSpace(runIDFlag)
	if runID == "" {
		runID = evalAssertRunIDFor(plan.Suite.Name, plan.ScenarioID)
	}
	root := filepath.Clean(h.root)
	workspace := filepath.Join(root, ".mnemon", "harness", "runs", "assertion-only", runID, "workspace")
	mnemonDir := filepath.Join(workspace, ".mnemon")
	env := harnesseval.SetupEnv(mnemonDir, plan.ProjectLoops)
	if ctx == nil {
		ctx = context.Background()
	}
	if err := (harnesseval.SetupRuntime{}).Run(ctx, harnesseval.SetupOptions{
		Handler:      plan.Scenario.SetupHandler,
		WorkspaceDir: workspace,
		MnemonDir:    mnemonDir,
		Loops:        plan.ProjectLoops,
		Env:          env,
	}); err != nil {
		return err
	}
	assertions, assertErr := (harnesseval.AssertionRuntime{Root: h.root}).Run(ctx, harnesseval.AssertionRunOptions{
		Backend:      harnesseval.AssertionBackend(plan.Scenario.AssertionBackend),
		ScenarioID:   plan.ScenarioID,
		Handler:      plan.Scenario.AssertionHandler,
		Report:       map[string]any{},
		WorkspaceDir: workspace,
		MnemonDir:    mnemonDir,
		Env:          env,
	})
	outcome := harnesseval.DeriveOutcome(harnesseval.OutcomeInput{Assertions: assertions, AssertionErr: assertErr})
	report := harnesseval.RunReport{
		SchemaVersion: 1,
		Kind:          "EvalAssertionOnlyRunReport",
		RunID:         runID,
		RunnerID:      "assertion-only",
		JobID:         evalRunJobID(plan.Suite.Name, plan.ScenarioID),
		JobSpec:       "eval." + plan.ScenarioID,
		Loop:          "eval",
		Status:        "ready",
		Message:       "assertion-only eval fixture completed without starting Codex",
	}
	if assertErr != nil {
		report.Status = "degraded"
		report.FailureClass = "assertion_runtime_failed"
		report.Message = assertErr.Error()
	}
	report, err = writeEvalAssertionRunReport(h.root, report)
	if err != nil {
		return err
	}
	proposals, err := createEvalProposalDrafts(h.root, plan.Suite.Name, harnesseval.RouteEvalReport(report, *plan.Scenario, outcome, assertions))
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "eval assert: %s\n", outcome)
	fmt.Fprintf(out, "suite: %s\n", plan.Suite.Name)
	fmt.Fprintf(out, "scenario: %s\n", plan.ScenarioID)
	fmt.Fprintf(out, "run-id: %s\n", runID)
	fmt.Fprintf(out, "assertions: %d\n", len(assertions))
	fmt.Fprintf(out, "report: %s\n", report.Source)
	for _, item := range proposals {
		fmt.Fprintf(out, "proposal: %s route=%s status=%s\n", item.ID, item.Route, item.Status)
	}
	if assertErr != nil {
		return fmt.Errorf("eval assertion failed: %w", assertErr)
	}
	return nil
}

func evalAssertRunIDFor(suite, scenario string) string {
	return "assert_" + sanitizeEvalID(suite) + "_" + sanitizeEvalID(scenario) + "_" + time.Now().UTC().Format("20060102T150405Z")
}

func writeEvalAssertionRunReport(root string, report harnesseval.RunReport) (harnesseval.RunReport, error) {
	path := harnesseval.RunReportPath(root, report.RunID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return harnesseval.RunReport{}, err
	}
	rel, err := filepath.Rel(filepath.Clean(root), path)
	if err != nil {
		rel = path
	}
	report.Source = filepath.ToSlash(rel)
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return harnesseval.RunReport{}, err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return harnesseval.RunReport{}, err
	}
	return report, nil
}

func evalProposalID(candidate harnesseval.ProposalCandidate) string {
	parts := []string{"eval", candidate.Route, candidate.ScenarioID}
	if candidate.Metadata != nil {
		if runID, ok := candidate.Metadata["run_id"].(string); ok {
			parts = append(parts, runID)
		}
	}
	return strings.Join(parts, "-")
}

func evalCandidateEvidence(refs []harnesseval.EvidenceRef) []proposal.EvidenceRef {
	out := make([]proposal.EvidenceRef, 0, len(refs))
	for _, ref := range refs {
		out = append(out, proposal.EvidenceRef{
			Type:    ref.Type,
			Ref:     ref.Ref,
			Summary: ref.Summary,
		})
	}
	return out
}

func evalCandidateValidation(suite string, candidate harnesseval.ProposalCandidate) proposal.ValidationPlan {
	command := "mnemon-harness eval run --suite " + suite + " --scenario " + candidate.ScenarioID + " --agent-turn --i-understand-model-cost"
	return proposal.ValidationPlan{
		Summary: "Rerun the eval scenario and verify the routed finding is resolved or intentionally accepted.",
		Commands: []string{
			command,
		},
		Checks: []string{
			"proposal route matches the owning loop",
			"proposal evidence includes the eval report ref",
		},
		RequiredEvidence: []string{"eval_report"},
	}
}

func (h *Harness) EvalABTest(ctx context.Context, out io.Writer, in EvalABInput) error {
	scenarios := append([]string(nil), in.Scenarios...)
	if len(scenarios) == 0 {
		plan, err := harnesseval.BuildRunPlan(h.root, in.Suite, "")
		if err != nil {
			return err
		}
		scenarios = []string{plan.ScenarioID}
	}
	request := harnesseval.ABTestRequest{
		Suite:        in.Suite,
		ScenarioIDs:  scenarios,
		TrialsPerArm: in.TrialsPerArm,
		Metric:       harnesseval.ABMetricDeterministicPass,
	}
	var err error
	request.ControlSetup, err = parseABSetupJSON("control", in.ControlSetupJSON)
	if err != nil {
		return err
	}
	request.TreatmentSetup, err = parseABSetupJSON("treatment", in.TreatmentSetupJSON)
	if err != nil {
		return err
	}
	runner := harnesseval.ABTestRunner{
		TrialRunner: harnesseval.CodexABTrialRunner{
			Root:                 h.root,
			Command:              in.Command,
			Timeout:              in.Timeout,
			TurnTimeout:          in.TurnTimeout,
			MaxTurns:             in.MaxTurns,
			IsolatedHome:         in.IsolatedHome,
			AllowRealTurn:        in.AgentTurn,
			AcknowledgeModelCost: in.AcknowledgeModelCost,
		},
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := runner.Run(ctx, request)
	if err != nil {
		return err
	}
	reportPath, err := harnesseval.WriteABTestResult(h.root, result)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "abtest: %s\n", result.Request.ID)
	fmt.Fprintf(out, "suite: %s\n", result.Request.Suite)
	fmt.Fprintf(out, "scenarios: %s\n", strings.Join(result.Request.ScenarioIDs, ", "))
	fmt.Fprintf(out, "trials: %d\n", len(result.Trials))
	fmt.Fprintf(out, "control pass rate: %.2f\n", result.Control.PassRate)
	fmt.Fprintf(out, "treatment pass rate: %.2f\n", result.Treatment.PassRate)
	fmt.Fprintf(out, "mean diff: %.2f\n", result.MeanDiff)
	fmt.Fprintf(out, "report: %s\n", reportPath)
	if !in.AgentTurn || !in.AcknowledgeModelCost {
		fmt.Fprintln(out, "real turns: blocked unless --agent-turn and --i-understand-model-cost are both set")
	}
	return nil
}

func parseABSetupJSON(arm, raw string) (map[string]any, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var setup map[string]any
	if err := json.Unmarshal([]byte(raw), &setup); err != nil {
		return nil, fmt.Errorf("parse %s setup json: %w", arm, err)
	}
	if len(setup) == 0 {
		return nil, nil
	}
	return setup, nil
}

func (h *Harness) EvalPromote(out io.Writer, in EvalPromoteInput) error {
	kind, id, err := selectedEvalPromotionAsset(in)
	if err != nil {
		return err
	}
	result, err := harnesseval.PromoteAsset(h.root, harnesseval.PromotionOptions{
		Kind:          kind,
		ID:            id,
		Target:        harnesseval.EvalAssetState(in.Target),
		From:          harnesseval.EvalAssetState(in.From),
		ProposalRef:   in.ProposalRef,
		AuditRef:      in.AuditRef,
		EventID:       in.EventID,
		CorrelationID: in.CorrelationID,
		CausedBy:      in.CausedBy,
		Now:           time.Now().UTC(),
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "eval asset promoted: %s %s\n", result.Asset.Kind, result.Asset.ID)
	fmt.Fprintf(out, "from: %s\n", result.FromState)
	fmt.Fprintf(out, "to: %s\n", result.ToState)
	fmt.Fprintf(out, "proposal: %s\n", result.ProposalID)
	fmt.Fprintf(out, "event: %s\n", result.Event.ID)
	return nil
}

func selectedEvalPromotionAsset(in EvalPromoteInput) (harnesseval.EvalAssetKind, string, error) {
	type selection struct {
		kind harnesseval.EvalAssetKind
		id   string
	}
	var selected []selection
	if strings.TrimSpace(in.Scenario) != "" {
		selected = append(selected, selection{kind: harnesseval.EvalAssetScenario, id: in.Scenario})
	}
	if strings.TrimSpace(in.Suite) != "" {
		selected = append(selected, selection{kind: harnesseval.EvalAssetSuite, id: in.Suite})
	}
	if strings.TrimSpace(in.Rubric) != "" {
		selected = append(selected, selection{kind: harnesseval.EvalAssetRubric, id: in.Rubric})
	}
	if len(selected) != 1 {
		return "", "", fmt.Errorf("exactly one of --scenario, --suite, or --rubric is required")
	}
	return selected[0].kind, strings.TrimSpace(selected[0].id), nil
}

func (h *Harness) EvalReport(out io.Writer, runID, format string) error {
	report, err := harnesseval.LoadRunReport(h.root, runID)
	if err != nil {
		return err
	}
	switch format {
	case "text", "":
		return writeEvalReportText(out, report)
	case "json":
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	default:
		return fmt.Errorf("unsupported --format %q", format)
	}
}

func (h *Harness) EvalReplay(out io.Writer, tier, format string) error {
	tiers, err := parseReplayTiers(tier)
	if err != nil {
		return err
	}
	result, err := harnesseval.ReplayRegression(h.root, harnesseval.ReplayOptions{
		Tiers: tiers,
		Now:   time.Now().UTC(),
	})
	if err != nil {
		return err
	}
	switch format {
	case "json":
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	case "text", "":
		fmt.Fprintf(out, "regression replay: %s\n", result.Status)
		fmt.Fprintf(out, "tiers: %s\n", tier)
		fmt.Fprintf(out, "checks: %d\n", len(result.Checks))
		fmt.Fprintf(out, "report: %s\n", result.ReportPath)
		if result.Status != "pass" {
			return fmt.Errorf("regression replay failed")
		}
		return nil
	default:
		return fmt.Errorf("unsupported --format %q", format)
	}
}

func parseReplayTiers(raw string) ([]int, error) {
	if strings.TrimSpace(raw) == "" {
		return []int{1}, nil
	}
	var tiers []int
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		tier, err := strconv.Atoi(part)
		if err != nil || tier <= 0 {
			return nil, fmt.Errorf("invalid replay tier %q", part)
		}
		tiers = append(tiers, tier)
	}
	if len(tiers) == 0 {
		return []int{1}, nil
	}
	return tiers, nil
}

func writeEvalPlanText(out io.Writer, suite harnesseval.Suite) error {
	if _, err := fmt.Fprintf(out, "Eval suite %s\n", suite.Name); err != nil {
		return err
	}
	if suite.Description != "" {
		if _, err := fmt.Fprintf(out, "Description: %s\n", suite.Description); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(out, "Source: %s\n", suite.Source); err != nil {
		return err
	}
	if suite.Host != "" {
		if _, err := fmt.Fprintf(out, "Host: %s\n", suite.Host); err != nil {
			return err
		}
	}
	if suite.Runner != "" {
		if _, err := fmt.Fprintf(out, "Runner: %s\n", suite.Runner); err != nil {
			return err
		}
	}
	scenarios := suite.ScenarioIDs
	if len(scenarios) == 0 {
		scenarios = suite.Scenarios
	}
	if _, err := fmt.Fprintln(out, "Scenarios:"); err != nil {
		return err
	}
	for _, scenario := range scenarios {
		if _, err := fmt.Fprintf(out, "- %s\n", scenario); err != nil {
			return err
		}
	}
	return nil
}

func writeEvalReportText(out io.Writer, report harnesseval.RunReport) error {
	if _, err := fmt.Fprintf(out, "Eval report %s\n", report.RunID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Status: %s\n", report.Status); err != nil {
		return err
	}
	if report.FailureClass != "" {
		if _, err := fmt.Fprintf(out, "Failure class: %s\n", report.FailureClass); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(out, "Message: %s\n", report.Message); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Runner: %s\n", report.RunnerID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Job: %s (%s)\n", report.JobID, report.JobSpec); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Loop: %s\n", report.Loop); err != nil {
		return err
	}
	if report.ThreadID != "" {
		if _, err := fmt.Fprintf(out, "Thread: %s\n", report.ThreadID); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(out, "Turns: %d\n", len(report.Turns)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Artifacts: %d\n", len(report.ArtifactRefs)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Events: %d\n", len(report.EventRefs)); err != nil {
		return err
	}
	if report.Source != "" {
		if _, err := fmt.Fprintf(out, "Source: %s\n", report.Source); err != nil {
			return err
		}
	}
	return nil
}

func evalRunJobID(suiteName, scenarioID string) string {
	return "eval_" + sanitizeEvalID(suiteName) + "_" + sanitizeEvalID(scenarioID)
}

func sanitizeEvalID(value string) string {
	value = strings.TrimSpace(value)
	var builder strings.Builder
	lastUnderscore := false
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			builder.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			builder.WriteByte('_')
			lastUnderscore = true
		}
	}
	trimmed := strings.Trim(builder.String(), "_")
	if trimmed == "" {
		return "scenario"
	}
	return strings.ToLower(trimmed)
}
