package eval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	runnercodex "github.com/mnemon-dev/mnemon/harness/internal/lifecycle/runner/codex"
)

const (
	ABTestResultKind          = "ABTestResult"
	ABTestVerdictKind         = "ABTestVerdict"
	ABMetricDeterministicPass = "deterministic_pass_rate"
)

type ABArm string

const (
	ABArmControl   ABArm = "control"
	ABArmTreatment ABArm = "treatment"
)

type ABTestRequest struct {
	SchemaVersion  int            `json:"schema_version"`
	ID             string         `json:"id"`
	Suite          string         `json:"suite"`
	ScenarioIDs    []string       `json:"scenario_ids"`
	TrialsPerArm   int            `json:"trials_per_arm"`
	Metric         string         `json:"metric"`
	ControlSetup   map[string]any `json:"control_setup,omitempty"`
	TreatmentSetup map[string]any `json:"treatment_setup,omitempty"`
}

type ABTrialSpec struct {
	RequestID  string         `json:"request_id"`
	Suite      string         `json:"suite"`
	ScenarioID string         `json:"scenario_id"`
	Arm        ABArm          `json:"arm"`
	TrialIndex int            `json:"trial_index"`
	Metric     string         `json:"metric"`
	Setup      map[string]any `json:"setup,omitempty"`
}

type ABTrialResult struct {
	Arm          ABArm            `json:"arm"`
	ScenarioID   string           `json:"scenario_id"`
	TrialIndex   int              `json:"trial_index"`
	RunID        string           `json:"run_id,omitempty"`
	Status       string           `json:"status"`
	Outcome      Outcome          `json:"outcome"`
	ReportRef    string           `json:"report_ref,omitempty"`
	ArtifactRefs []ReportArtifact `json:"artifact_refs,omitempty"`
	Error        string           `json:"error,omitempty"`
}

type ABArmSummary struct {
	Trials   int             `json:"trials"`
	Passes   int             `json:"passes"`
	PassRate float64         `json:"pass_rate"`
	Outcomes map[Outcome]int `json:"outcomes"`
}

type ABTestResult struct {
	SchemaVersion    int             `json:"schema_version"`
	Kind             string          `json:"kind"`
	Request          ABTestRequest   `json:"request"`
	StartedAt        string          `json:"started_at"`
	FinishedAt       string          `json:"finished_at"`
	Control          ABArmSummary    `json:"control"`
	Treatment        ABArmSummary    `json:"treatment"`
	MeanDiff         float64         `json:"mean_diff"`
	Trials           []ABTrialResult `json:"trials"`
	TranscriptRefs   []string        `json:"transcript_refs,omitempty"`
	ArtifactRefs     []string        `json:"artifact_refs,omitempty"`
	ReportPath       string          `json:"report_path,omitempty"`
	SignificanceNote string          `json:"significance_note"`
}

type ABRecommendation string

const (
	ABRecommendationApprove      ABRecommendation = "approve"
	ABRecommendationReject       ABRecommendation = "reject"
	ABRecommendationMoreData     ABRecommendation = "more_data"
	ABRecommendationInconclusive ABRecommendation = "inconclusive"
)

type ABSignificance string

const (
	ABSignificanceStrong ABSignificance = "strong"
	ABSignificanceWeak   ABSignificance = "weak"
	ABSignificanceNone   ABSignificance = "none"
)

type ABTestVerdict struct {
	SchemaVersion          int              `json:"schema_version"`
	Kind                   string           `json:"kind"`
	ABTestID               string           `json:"ab_test_id"`
	ResultRef              string           `json:"result_ref,omitempty"`
	Significance           ABSignificance   `json:"significance"`
	Recommendation         ABRecommendation `json:"recommendation"`
	Summary                string           `json:"summary"`
	Narrative              string           `json:"narrative"`
	RequiredAdditionalRuns int              `json:"required_additional_runs,omitempty"`
	Evidence               []EvidenceRef    `json:"evidence,omitempty"`
}

type ABTrialRunner interface {
	RunABTrial(context.Context, ABTrialSpec) (ABTrialResult, error)
}

type ABTestRunner struct {
	TrialRunner ABTrialRunner
	Now         func() time.Time
}

func (runner ABTestRunner) Run(ctx context.Context, request ABTestRequest) (ABTestResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	request = normalizeABTestRequest(request, runner.now())
	if err := ValidateABTestRequest(request); err != nil {
		return ABTestResult{}, err
	}
	if runner.TrialRunner == nil {
		return ABTestResult{}, fmt.Errorf("ab trial runner is required")
	}

	started := runner.now().UTC()
	var trials []ABTrialResult
	for _, arm := range []ABArm{ABArmControl, ABArmTreatment} {
		for _, scenarioID := range request.ScenarioIDs {
			for trial := 1; trial <= request.TrialsPerArm; trial++ {
				spec := ABTrialSpec{
					RequestID:  request.ID,
					Suite:      request.Suite,
					ScenarioID: scenarioID,
					Arm:        arm,
					TrialIndex: trial,
					Metric:     request.Metric,
					Setup:      setupForArm(request, arm),
				}
				result, err := runner.TrialRunner.RunABTrial(ctx, spec)
				if err != nil {
					result = ABTrialResult{
						Status:  "invalid",
						Outcome: OutcomeInvalid,
						Error:   err.Error(),
					}
				}
				trials = append(trials, normalizeABTrialResult(spec, result))
			}
		}
	}

	control := summarizeABArm(trials, ABArmControl)
	treatment := summarizeABArm(trials, ABArmTreatment)
	result := ABTestResult{
		SchemaVersion:    1,
		Kind:             ABTestResultKind,
		Request:          request,
		StartedAt:        started.Format(time.RFC3339),
		FinishedAt:       runner.now().UTC().Format(time.RFC3339),
		Control:          control,
		Treatment:        treatment,
		MeanDiff:         treatment.PassRate - control.PassRate,
		Trials:           trials,
		TranscriptRefs:   collectABTranscriptRefs(trials),
		ArtifactRefs:     collectABArtifactRefs(trials),
		SignificanceNote: "T41 records deterministic pass-rate deltas only; statistical significance and L4 ab-judge verdict are T43/T42 responsibilities.",
	}
	return result, nil
}

func ValidateABTestRequest(request ABTestRequest) error {
	var errs []error
	if strings.TrimSpace(request.ID) == "" {
		errs = append(errs, fmt.Errorf("id is required"))
	}
	if strings.TrimSpace(request.Suite) == "" {
		errs = append(errs, fmt.Errorf("suite is required"))
	}
	if len(request.ScenarioIDs) == 0 {
		errs = append(errs, fmt.Errorf("scenario_ids is required"))
	}
	for index, scenarioID := range request.ScenarioIDs {
		if strings.TrimSpace(scenarioID) == "" {
			errs = append(errs, fmt.Errorf("scenario_ids[%d] is required", index))
		}
	}
	if request.TrialsPerArm <= 0 {
		errs = append(errs, fmt.Errorf("trials_per_arm must be positive"))
	}
	if request.Metric != ABMetricDeterministicPass {
		errs = append(errs, fmt.Errorf("metric %q is not supported", request.Metric))
	}
	return joinErrors(errs)
}

func ValidateABTestResult(result ABTestResult) error {
	var errs []error
	if result.SchemaVersion != 1 {
		errs = append(errs, fmt.Errorf("schema_version must be 1"))
	}
	if result.Kind != ABTestResultKind {
		errs = append(errs, fmt.Errorf("kind must be %s", ABTestResultKind))
	}
	if err := ValidateABTestRequest(result.Request); err != nil {
		errs = append(errs, err)
	}
	if _, err := time.Parse(time.RFC3339, result.StartedAt); err != nil {
		errs = append(errs, fmt.Errorf("started_at must be RFC3339"))
	}
	if _, err := time.Parse(time.RFC3339, result.FinishedAt); err != nil {
		errs = append(errs, fmt.Errorf("finished_at must be RFC3339"))
	}
	if len(result.Trials) == 0 {
		errs = append(errs, fmt.Errorf("trials is required"))
	}
	for index, trial := range result.Trials {
		if err := validateABTrialResult(trial); err != nil {
			errs = append(errs, fmt.Errorf("trials[%d]: %w", index, err))
		}
	}
	expectedControl := summarizeABArm(result.Trials, ABArmControl)
	expectedTreatment := summarizeABArm(result.Trials, ABArmTreatment)
	if result.Control.Trials != expectedControl.Trials || result.Control.Passes != expectedControl.Passes {
		errs = append(errs, fmt.Errorf("control summary does not match trials"))
	}
	if result.Treatment.Trials != expectedTreatment.Trials || result.Treatment.Passes != expectedTreatment.Passes {
		errs = append(errs, fmt.Errorf("treatment summary does not match trials"))
	}
	if strings.TrimSpace(result.SignificanceNote) == "" {
		errs = append(errs, fmt.Errorf("significance_note is required"))
	}
	return joinErrors(errs)
}

type CodexABTrialRunner struct {
	Root                 string
	Command              string
	Timeout              time.Duration
	TurnTimeout          time.Duration
	MaxTurns             int
	IsolatedHome         bool
	AllowRealTurn        bool
	AcknowledgeModelCost bool
	Now                  time.Time
	AssertionRuntime     AssertionRuntime
	SkipAssertionRuntime bool
}

func (runner CodexABTrialRunner) RunABTrial(ctx context.Context, spec ABTrialSpec) (ABTrialResult, error) {
	root := cleanRoot(runner.Root)
	plan, err := BuildRunPlan(root, spec.Suite, spec.ScenarioID)
	if err != nil {
		return ABTrialResult{}, err
	}
	now := runner.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	runID := abTrialRunID(spec)
	result, err := runnercodex.Run(ctx, root, runnercodex.RunOptions{
		CheckOptions: runnercodex.CheckOptions{
			Command:          runner.Command,
			Timeout:          runner.Timeout,
			IsolateCodexHome: runner.IsolatedHome,
			Now:              now,
			RunID:            runID,
		},
		JobID:                abTrialJobID(spec),
		JobSpec:              "abtest." + sanitizeABID(spec.Suite) + "." + sanitizeABID(spec.ScenarioID) + "." + string(spec.Arm),
		Loop:                 "eval",
		Prompt:               annotateABPrompt(plan.Prompt, spec),
		Prompts:              annotateABPrompts(plan.Prompts, spec),
		TurnTimeout:          runner.TurnTimeout,
		MaxTurns:             runner.MaxTurns,
		AllowRealTurn:        runner.AllowRealTurn,
		AcknowledgeModelCost: runner.AcknowledgeModelCost,
		DeclarationRoot:      root,
		ProjectLoops:         plan.ProjectLoops,
		WorkspaceEnv: func(workspace runnercodex.WorkspaceContext) []string {
			env := SetupEnv(workspace.MnemonDir, plan.ProjectLoops)
			addABSetupEnv(env, spec)
			return SetupEnvPairs(env)
		},
		SetupWorkspace: func(ctx context.Context, workspace runnercodex.WorkspaceContext) error {
			handler := ""
			if plan.Scenario != nil {
				handler = plan.Scenario.SetupHandler
			}
			env := SetupEnv(workspace.MnemonDir, plan.ProjectLoops)
			addABSetupEnv(env, spec)
			if err := (SetupRuntime{}).Run(ctx, SetupOptions{
				Handler:      handler,
				WorkspaceDir: workspace.Workspace,
				MnemonDir:    workspace.MnemonDir,
				Loops:        plan.ProjectLoops,
				Env:          env,
			}); err != nil {
				return err
			}
			return writeABSetupEvidence(workspace.MnemonDir, spec)
		},
	})
	if err != nil {
		return ABTrialResult{}, err
	}

	trial := ABTrialResult{
		Arm:        spec.Arm,
		ScenarioID: spec.ScenarioID,
		TrialIndex: spec.TrialIndex,
		RunID:      result.RunID,
		Status:     string(result.Status),
		Outcome:    OutcomeInvalid,
		ReportRef:  relativeReportRef(root, result.ReportPath),
	}
	report, reportErr := LoadRunReport(root, result.RunID)
	if reportErr == nil {
		trial.ArtifactRefs = report.ArtifactRefs
	}
	if string(result.Status) != "ready" {
		return trial, nil
	}
	if runner.SkipAssertionRuntime || plan.Scenario == nil {
		trial.Outcome = OutcomeInconclusive
		return trial, nil
	}
	outcome, err := runner.assertOutcome(ctx, root, plan, result)
	if err != nil {
		trial.Outcome = OutcomeInvalid
		trial.Error = err.Error()
		return trial, nil
	}
	trial.Outcome = outcome
	return trial, nil
}

func annotateABPrompt(prompt string, spec ABTrialSpec) string {
	if strings.TrimSpace(prompt) == "" || len(spec.Setup) == 0 {
		return prompt
	}
	return abSetupPrefix(spec) + "\n\nScenario prompt:\n" + prompt
}

func annotateABPrompts(prompts []string, spec ABTrialSpec) []string {
	if len(prompts) == 0 || len(spec.Setup) == 0 {
		return prompts
	}
	out := make([]string, 0, len(prompts))
	for _, prompt := range prompts {
		out = append(out, annotateABPrompt(prompt, spec))
	}
	return out
}

func abSetupPrefix(spec ABTrialSpec) string {
	return fmt.Sprintf("AB test arm context:\n- arm: %s\n- setup_json: %s\nUse this setup as the experimental condition for this arm and preserve candidate-specific evidence when relevant.", spec.Arm, mustABSetupJSON(spec.Setup))
}

func addABSetupEnv(env map[string]string, spec ABTrialSpec) {
	if len(spec.Setup) == 0 {
		return
	}
	env["MNEMON_AB_ARM"] = string(spec.Arm)
	env["MNEMON_AB_SETUP_JSON"] = mustABSetupJSON(spec.Setup)
}

func writeABSetupEvidence(mnemonDir string, spec ABTrialSpec) error {
	if len(spec.Setup) == 0 {
		return nil
	}
	dir := filepath.Join(mnemonDir, "harness")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create ab setup evidence dir: %w", err)
	}
	path := filepath.Join(dir, "abtest-arm-setup.json")
	data, err := json.MarshalIndent(map[string]any{
		"request_id": spec.RequestID,
		"suite":      spec.Suite,
		"scenario":   spec.ScenarioID,
		"arm":        spec.Arm,
		"setup":      spec.Setup,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal ab setup evidence: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write ab setup evidence: %w", err)
	}
	return nil
}

func mustABSetupJSON(setup map[string]any) string {
	data, err := json.Marshal(setup)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func (runner CodexABTrialRunner) assertOutcome(ctx context.Context, root string, plan RunPlan, result runnercodex.RunResult) (Outcome, error) {
	transcript, err := LoadRunTranscriptReport(root, result.RunID)
	if err != nil {
		return OutcomeInvalid, err
	}
	runtime := runner.AssertionRuntime
	if runtime.Root == "" {
		runtime.Root = root
	}
	backend := AssertionBackend("")
	handler := ""
	if plan.Scenario != nil {
		backend = AssertionBackend(plan.Scenario.AssertionBackend)
		handler = plan.Scenario.AssertionHandler
	}
	mnemonDir := filepath.Join(result.Workspace, ".mnemon")
	env := SetupEnv(mnemonDir, plan.ProjectLoops)
	assertions, assertErr := runtime.Run(ctx, AssertionRunOptions{
		Backend:      backend,
		ScenarioID:   plan.ScenarioID,
		Handler:      handler,
		Report:       transcript.ReportMap(),
		WorkspaceDir: result.Workspace,
		MnemonDir:    mnemonDir,
		Env:          env,
	})
	if assertErr != nil {
		return OutcomeInvalid, assertErr
	}
	return DeriveOutcome(OutcomeInput{Assertions: assertions}), nil
}

func WriteABTestResult(root string, result ABTestResult) (string, error) {
	root = cleanRoot(root)
	if strings.TrimSpace(result.Request.ID) == "" {
		return "", fmt.Errorf("ab test result request id is required")
	}
	dir := filepath.Join(root, ".mnemon", "harness", "reports", "abtest")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create abtest report dir: %w", err)
	}
	path := filepath.Join(dir, result.Request.ID+".json")
	result.ReportPath = filepath.ToSlash(filepath.Join(".mnemon", "harness", "reports", "abtest", result.Request.ID+".json"))
	if err := ValidateABTestResult(result); err != nil {
		return "", fmt.Errorf("validate abtest result: %w", err)
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal abtest result: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "."+result.Request.ID+"-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create abtest report temp file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return "", fmt.Errorf("write abtest report: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return "", fmt.Errorf("close abtest report: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return "", fmt.Errorf("rename abtest report: %w", err)
	}
	return path, nil
}

func normalizeABTestRequest(request ABTestRequest, now time.Time) ABTestRequest {
	if request.SchemaVersion == 0 {
		request.SchemaVersion = 1
	}
	if request.ID == "" {
		request.ID = "abtest-" + now.UTC().Format("20060102T150405Z")
	}
	if request.TrialsPerArm == 0 {
		request.TrialsPerArm = 1
	}
	if request.Metric == "" {
		request.Metric = ABMetricDeterministicPass
	}
	for index, scenarioID := range request.ScenarioIDs {
		request.ScenarioIDs[index] = strings.TrimSpace(scenarioID)
	}
	return request
}

func normalizeABTrialResult(spec ABTrialSpec, result ABTrialResult) ABTrialResult {
	if result.Arm == "" {
		result.Arm = spec.Arm
	}
	if result.ScenarioID == "" {
		result.ScenarioID = spec.ScenarioID
	}
	if result.TrialIndex == 0 {
		result.TrialIndex = spec.TrialIndex
	}
	if result.Status == "" {
		result.Status = "completed"
	}
	if result.Outcome == "" {
		result.Outcome = OutcomeInconclusive
	}
	return result
}

func validateABTrialResult(trial ABTrialResult) error {
	var errs []error
	if trial.Arm != ABArmControl && trial.Arm != ABArmTreatment {
		errs = append(errs, fmt.Errorf("arm %q is not allowed", trial.Arm))
	}
	if strings.TrimSpace(trial.ScenarioID) == "" {
		errs = append(errs, fmt.Errorf("scenario_id is required"))
	}
	if trial.TrialIndex <= 0 {
		errs = append(errs, fmt.Errorf("trial_index must be positive"))
	}
	if strings.TrimSpace(trial.Status) == "" {
		errs = append(errs, fmt.Errorf("status is required"))
	}
	if err := ValidateOutcome(trial.Outcome); err != nil {
		errs = append(errs, err)
	}
	return joinErrors(errs)
}

func summarizeABArm(trials []ABTrialResult, arm ABArm) ABArmSummary {
	summary := ABArmSummary{Outcomes: map[Outcome]int{}}
	for _, trial := range trials {
		if trial.Arm != arm {
			continue
		}
		summary.Trials++
		summary.Outcomes[trial.Outcome]++
		if trial.Outcome == OutcomePass {
			summary.Passes++
		}
	}
	if summary.Trials > 0 {
		summary.PassRate = float64(summary.Passes) / float64(summary.Trials)
	}
	return summary
}

func setupForArm(request ABTestRequest, arm ABArm) map[string]any {
	switch arm {
	case ABArmTreatment:
		return request.TreatmentSetup
	default:
		return request.ControlSetup
	}
}

func collectABTranscriptRefs(trials []ABTrialResult) []string {
	seen := map[string]bool{}
	var refs []string
	for _, trial := range trials {
		for _, ref := range trial.ArtifactRefs {
			if ref.Kind != "transcript" && !strings.Contains(ref.URI, "jsonrpc-transcript") {
				continue
			}
			if !seen[ref.URI] {
				seen[ref.URI] = true
				refs = append(refs, ref.URI)
			}
		}
	}
	return refs
}

func collectABArtifactRefs(trials []ABTrialResult) []string {
	seen := map[string]bool{}
	var refs []string
	for _, trial := range trials {
		if trial.ReportRef != "" && !seen[trial.ReportRef] {
			seen[trial.ReportRef] = true
			refs = append(refs, trial.ReportRef)
		}
		for _, ref := range trial.ArtifactRefs {
			if ref.URI == "" || seen[ref.URI] {
				continue
			}
			seen[ref.URI] = true
			refs = append(refs, ref.URI)
		}
	}
	return refs
}

func abTrialRunID(spec ABTrialSpec) string {
	return sanitizeABID(spec.RequestID) + "_" + sanitizeABID(spec.ScenarioID) + "_" + string(spec.Arm) + fmt.Sprintf("_%02d", spec.TrialIndex)
}

func abTrialJobID(spec ABTrialSpec) string {
	return "abtest_" + sanitizeABID(spec.Suite) + "_" + sanitizeABID(spec.ScenarioID) + "_" + string(spec.Arm) + fmt.Sprintf("_%02d", spec.TrialIndex)
}

func relativeReportRef(root, path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func (runner ABTestRunner) now() time.Time {
	if runner.Now != nil {
		return runner.Now()
	}
	return time.Now().UTC()
}

func joinErrors(errs []error) error {
	var messages []string
	for _, err := range errs {
		if err != nil {
			messages = append(messages, err.Error())
		}
	}
	if len(messages) == 0 {
		return nil
	}
	return errors.New(strings.Join(messages, "; "))
}

func sanitizeABID(value string) string {
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
		return "item"
	}
	return strings.ToLower(trimmed)
}
