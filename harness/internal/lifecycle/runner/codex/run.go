package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/auditstore"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/eventlog"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/layout"
	lifecyclerunner "github.com/mnemon-dev/mnemon/harness/internal/lifecycle/runner"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
	"github.com/mnemon-dev/mnemon/harness/internal/projection"
)

const defaultMaxTurns = 3

type RunOptions struct {
	CheckOptions
	JobID                string
	JobSpec              string
	Loop                 string
	Prompt               string
	Prompts              []string
	ProjectRoot          string
	TurnTimeout          time.Duration
	MaxTurns             int
	AllowRealTurn        bool
	AcknowledgeModelCost bool
	DeclarationRoot      string
	ProjectLoops         []string
	ProjectHostArgs      []string
	WorkspaceEnv         func(WorkspaceContext) []string
	SetupWorkspace       func(context.Context, WorkspaceContext) error
}

type RunResult struct {
	RunID        string
	Status       Status
	FailureClass FailureClass
	Message      string
	TurnCount    int
	ThreadID     string
	LastEventID  string
	ReportPath   string
	StatusPath   string
	RunDir       string
	Workspace    string
}

type WorkspaceContext struct {
	Workspace string
	MnemonDir string
}

type SemanticReport struct {
	SchemaVersion int                    `json:"schema_version"`
	Kind          string                 `json:"kind"`
	RunID         string                 `json:"run_id"`
	RunnerID      string                 `json:"runner_id"`
	JobID         string                 `json:"job_id"`
	JobSpec       string                 `json:"job_spec"`
	Loop          string                 `json:"loop"`
	Status        Status                 `json:"status"`
	FailureClass  FailureClass           `json:"failure_class,omitempty"`
	Message       string                 `json:"message"`
	Command       []string               `json:"command"`
	Workspace     string                 `json:"workspace"`
	RunDir        string                 `json:"run_dir"`
	StartedAt     string                 `json:"started_at"`
	FinishedAt    string                 `json:"finished_at"`
	ThreadID      string                 `json:"thread_id,omitempty"`
	Turns         []TurnRecord           `json:"turns,omitempty"`
	Budget        lifecyclerunner.Budget `json:"budget"`
	RunnerResult  lifecyclerunner.Result `json:"runner_result,omitempty"`
	ArtifactRefs  []ArtifactRef          `json:"artifact_refs"`
	EventRefs     []string               `json:"event_refs,omitempty"`
	AuditRef      map[string]any         `json:"audit_ref,omitempty"`
	Scope         map[string]any         `json:"scope,omitempty"`
	Conditions    []Condition            `json:"conditions,omitempty"`
}

type TurnRecord struct {
	Index             int            `json:"index"`
	PromptArtifactURI string         `json:"prompt_artifact_uri"`
	Notification      map[string]any `json:"notification,omitempty"`
}

func Run(ctx context.Context, root string, opts RunOptions) (RunResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	normalizeRunOptions(&opts)
	paths, err := layout.EnsureProject(root)
	if err != nil {
		return RunResult{}, err
	}
	runID := opts.RunID
	runDir := filepath.Join(paths.HarnessDir, "runs", "codex-app-server", runID)
	workspace, managedWorkspace, err := runWorkspace(paths.Root, runDir, opts.ProjectRoot)
	if err != nil {
		return RunResult{}, err
	}
	logsDir := filepath.Join(runDir, "logs")
	reportsDir := filepath.Join(runDir, "reports")
	artifactsDir := filepath.Join(runDir, "artifacts")
	for _, dir := range []string{workspace, filepath.Join(workspace, ".mnemon"), filepath.Join(workspace, ".codex"), logsDir, reportsDir, artifactsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return RunResult{}, fmt.Errorf("create runner dir: %w", err)
		}
	}
	if managedWorkspace {
		if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("# Mnemon Codex App-Server Semantic Run\n"), 0o644); err != nil {
			return RunResult{}, fmt.Errorf("write workspace readme: %w", err)
		}
	}
	if !managedWorkspace {
		if err := os.MkdirAll(filepath.Join(workspace, ".mnemon", "harness"), 0o755); err != nil {
			return RunResult{}, fmt.Errorf("create project harness dir: %w", err)
		}
	}
	if len(opts.ProjectLoops) > 0 {
		declarationRoot := opts.DeclarationRoot
		if declarationRoot == "" {
			declarationRoot = root
		}
		if err := projection.RunCodexProjector(ctx, "install", projection.CodexOptions{
			DeclarationRoot: declarationRoot,
			ProjectRoot:     workspace,
			Loops:           opts.ProjectLoops,
			HostArgs:        opts.ProjectHostArgs,
			Stdout:          io.Discard,
			Stderr:          io.Discard,
		}); err != nil {
			return RunResult{}, fmt.Errorf("project Codex loop assets into runner workspace: %w", err)
		}
	}
	workspaceContext := WorkspaceContext{
		Workspace: workspace,
		MnemonDir: filepath.Join(workspace, ".mnemon"),
	}
	runCheckOptions := opts.CheckOptions
	if opts.WorkspaceEnv != nil {
		runCheckOptions.Env = append(append([]string(nil), runCheckOptions.Env...), opts.WorkspaceEnv(workspaceContext)...)
	}
	if opts.SetupWorkspace != nil {
		if err := opts.SetupWorkspace(ctx, workspaceContext); err != nil {
			return RunResult{}, fmt.Errorf("setup runner workspace: %w", err)
		}
	}
	store, err := eventlog.New(paths.Root)
	if err != nil {
		return RunResult{}, err
	}

	prompts := runPrompts(opts)
	budget := lifecyclerunner.Budget{MaxTurns: opts.MaxTurns}
	startedAt := opts.Now.UTC().Format(time.RFC3339)
	if !opts.AllowRealTurn || !opts.AcknowledgeModelCost {
		report := blockedSemanticReport(runID, runDir, workspace, opts, budget, startedAt, "RealTurnGateMissing", "real Codex turn requires --agent-turn and --i-understand-model-cost")
		return writeBlockedSemanticOutcome(paths, store, report, opts)
	}
	if len(prompts) == 0 {
		report := blockedSemanticReport(runID, runDir, workspace, opts, budget, startedAt, "PromptMissing", "semantic dispatch requires at least one prompt")
		return writeBlockedSemanticOutcome(paths, store, report, opts)
	}
	if !budget.Allows(len(prompts)) {
		report := blockedSemanticReport(runID, runDir, workspace, opts, budget, startedAt, "TurnBudgetExceeded", "requested turns exceed max real-turn budget")
		return writeBlockedSemanticOutcome(paths, store, report, opts)
	}
	if opts.IsolateCodexHome && !hasExplicitCodexAuthEnv(opts.CheckOptions) {
		message := "isolated CODEX_HOME cannot start a real Codex turn without explicit auth context; set OPENAI_API_KEY or CODEX_API_KEY, or run without --isolated-codex-home"
		report := blockedSemanticReport(runID, runDir, workspace, opts, budget, startedAt, "IsolatedCodexHomeAuthMissing", message)
		report.FailureClass = FailureAuthQuotaUnavailable
		return writeBlockedSemanticOutcome(paths, store, report, opts)
	}

	commandPath, err := exec.LookPath(opts.Command)
	if err != nil {
		report := blockedSemanticReport(runID, runDir, workspace, opts, budget, startedAt, "CommandMissing", fmt.Sprintf("codex command %q not found", opts.Command))
		report.FailureClass = FailureCommandMissing
		return writeBlockedSemanticOutcome(paths, store, report, opts)
	}
	startEventID := eventID(runID, "job_started")
	if err := store.Append(jobEvent(startEventID, "job.started", opts, nil, nil)); err != nil {
		return RunResult{}, err
	}
	runnerStartEventID := eventID(runID, "runner_semantic_started")
	if err := store.Append(runnerSemanticEvent(runnerStartEventID, "runner.semantic_run_started", opts, "running", "SemanticRunStarted", "Codex app-server semantic dispatch started.", startEventID, nil)); err != nil {
		return RunResult{}, err
	}

	runCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	stderrPath := filepath.Join(logsDir, "codex-app-server.stderr.log")
	transcriptPath := filepath.Join(artifactsDir, "jsonrpc-transcript.jsonl")
	rpc, err := startClient(runCtx, commandPath, runCheckOptions, workspace, stderrPath, transcriptPath)
	if err != nil {
		return failSemanticRun(paths, store, runID, runDir, workspace, opts, budget, startedAt, FailureProtocolUnavailable, "ProtocolUnavailable", fmt.Sprintf("start app-server: %v", err), startEventID, runnerStartEventID, nil)
	}
	defer rpc.close()

	initResult, err := rpc.request(runCtx, "initialize", map[string]any{
		"clientInfo": map[string]any{
			"name":    opts.ClientName,
			"title":   "Mnemon Lifecycle",
			"version": opts.ClientVersion,
		},
	})
	if err != nil {
		return failSemanticRun(paths, store, runID, runDir, workspace, opts, budget, startedAt, FailureProtocolUnavailable, "ProtocolUnavailable", fmt.Sprintf("initialize failed: %v", err), startEventID, runnerStartEventID, nil)
	}
	_ = initResult
	_ = rpc.notify("initialized", map[string]any{})
	if _, err := rpc.request(runCtx, "skills/list", map[string]any{"cwds": []string{workspace}, "forceReload": true}); err != nil {
		return failSemanticRun(paths, store, runID, runDir, workspace, opts, budget, startedAt, FailureProtocolUnavailable, "ProtocolUnavailable", fmt.Sprintf("skills/list failed: %v", err), startEventID, runnerStartEventID, nil)
	}
	if _, err := rpc.request(runCtx, "model/list", map[string]any{"includeHidden": false}); err != nil {
		class := FailureProtocolUnavailable
		reason := "ProtocolUnavailable"
		if looksLikeAuthQuota(err.Error()) {
			class = FailureAuthQuotaUnavailable
			reason = "AuthQuotaUnavailable"
		}
		return failSemanticRun(paths, store, runID, runDir, workspace, opts, budget, startedAt, class, reason, fmt.Sprintf("model/list failed: %v", err), startEventID, runnerStartEventID, nil)
	}

	thread, err := rpc.request(runCtx, "thread/start", map[string]any{
		"cwd":                   workspace,
		"approvalPolicy":        "never",
		"sandbox":               "danger-full-access",
		"ephemeral":             true,
		"developerInstructions": semanticDeveloperInstructions(opts, paths.MnemonDir),
	})
	if err != nil {
		return failSemanticRun(paths, store, runID, runDir, workspace, opts, budget, startedAt, FailureProtocolUnavailable, "ProtocolUnavailable", fmt.Sprintf("thread/start failed: %v", err), startEventID, runnerStartEventID, nil)
	}
	threadID := nestedString(thread, "thread", "id")
	if threadID == "" {
		return failSemanticRun(paths, store, runID, runDir, workspace, opts, budget, startedAt, FailureProtocolUnavailable, "ProtocolUnavailable", "thread/start did not return thread id", startEventID, runnerStartEventID, nil)
	}

	var turns []TurnRecord
	for index, prompt := range prompts {
		promptPath := filepath.Join(artifactsDir, fmt.Sprintf("prompt-%02d.txt", index+1))
		if err := os.WriteFile(promptPath, []byte(prompt), 0o644); err != nil {
			return RunResult{}, fmt.Errorf("write prompt artifact: %w", err)
		}
		before := rpc.notificationCount()
		turnCtx, cancelTurn := context.WithTimeout(ctx, opts.TurnTimeout)
		_, err := rpc.request(turnCtx, "turn/start", map[string]any{
			"threadId":       threadID,
			"input":          []map[string]any{{"type": "text", "text": prompt}},
			"cwd":            workspace,
			"approvalPolicy": "never",
			"sandboxPolicy":  map[string]any{"type": "dangerFullAccess"},
		})
		if err != nil {
			cancelTurn()
			return failSemanticRun(paths, store, runID, runDir, workspace, opts, budget, startedAt, FailureProtocolUnavailable, "ProtocolUnavailable", fmt.Sprintf("turn/start failed: %v", err), startEventID, runnerStartEventID, turns)
		}
		completed, err := rpc.waitNotification(turnCtx, "turn/completed", before)
		cancelTurn()
		if err != nil {
			return failSemanticRun(paths, store, runID, runDir, workspace, opts, budget, startedAt, FailureProtocolUnavailable, "ProtocolUnavailable", fmt.Sprintf("turn/completed failed: %v", err), startEventID, runnerStartEventID, turns)
		}
		turns = append(turns, TurnRecord{
			Index:             index + 1,
			PromptArtifactURI: relativeTo(paths.Root, promptPath),
			Notification:      rpcMessageMap(completed),
		})
		budget.UsedTurns++
		if failed, reason, message, class := turnCompletionFailure(completed); failed {
			return failSemanticRun(paths, store, runID, runDir, workspace, opts, budget, startedAt, class, reason, message, startEventID, runnerStartEventID, turns)
		}
	}

	refs := semanticArtifactRefs(paths.Root, workspace, stderrPath, transcriptPath, artifactsDir)
	runnerResult := lifecyclerunner.Result{
		SchemaVersion: lifecyclerunner.ResultSchemaVersion,
		Kind:          "HostAgentRunnerResult",
		JobID:         opts.JobID,
		RunnerID:      RunnerID,
		Host:          "codex",
		ThreadID:      threadID,
		TurnCount:     len(turns),
		Status:        "completed",
		Outcome:       "inconclusive",
		Summary:       "Codex app-server semantic dispatch completed; outputs are retained as evidence pending validation/governance.",
		ArtifactRefs:  toRunnerArtifactRefs(refs),
	}
	if err := lifecyclerunner.ValidateResult(runnerResult, lifecyclerunner.ValidateOptions{
		Budget:               lifecyclerunner.Budget{MaxTurns: opts.MaxTurns},
		ArtifactRoot:         paths.Root,
		RequireArtifactFiles: true,
	}); err != nil {
		return failSemanticRun(paths, store, runID, runDir, workspace, opts, budget, startedAt, FailureProtocolUnavailable, "InvalidStructuredResult", fmt.Sprintf("runner result validation failed: %v", err), startEventID, runnerStartEventID, turns)
	}

	resultPath := filepath.Join(artifactsDir, "runner-result.json")
	if err := writeJSONAtomic(resultPath, runnerResult); err != nil {
		return RunResult{}, err
	}
	refs = append(refs, artifactRefFor(paths.Root, "artifact:runner-result", "runner_result", resultPath, "application/json"))

	audits, err := auditstore.New(paths.Root)
	if err != nil {
		return RunResult{}, err
	}
	auditWrite, err := audits.Write(auditstore.WriteOptions{
		ID:   runID + "-codex-app-server",
		Spec: auditSpec(opts, refs, startEventID),
	})
	if err != nil {
		return RunResult{}, err
	}
	auditRef := auditWrite.Ref
	completedEventID := eventID(runID, "job_completed")
	if err := store.Append(jobEvent(completedEventID, "job.completed", opts, refs, auditRef)); err != nil {
		return RunResult{}, err
	}
	runnerCompletedEventID := eventID(runID, "runner_semantic_completed")
	if err := store.Append(runnerSemanticEvent(runnerCompletedEventID, "runner.semantic_run_completed", opts, string(StatusReady), "SemanticRunCompleted", "Codex app-server semantic dispatch completed.", completedEventID, refs)); err != nil {
		return RunResult{}, err
	}
	auditEventID := eventID(runID, "audit_recorded")
	auditEvent, err := audits.AppendRecordedEvent(auditstore.RecordedEventOptions{
		ID:            auditEventID,
		Now:           opts.Now,
		Loop:          opts.Loop,
		Host:          "codex",
		Actor:         "mnemon-manual",
		Source:        "codex.app-server",
		CorrelationID: opts.RunID,
		CausedBy:      completedEventID,
		Payload: map[string]any{
			"job_id":    opts.JobID,
			"runner_id": RunnerID,
			"reason":    "Recorded real Codex app-server dispatch evidence.",
		},
		AuditRef: auditRef,
		Scope:    runScope(paths.Root, opts).Map(),
	})
	if err != nil {
		return RunResult{}, err
	}

	report := SemanticReport{
		SchemaVersion: 1,
		Kind:          "CodexAppServerSemanticRunReport",
		RunID:         runID,
		RunnerID:      RunnerID,
		JobID:         opts.JobID,
		JobSpec:       opts.JobSpec,
		Loop:          opts.Loop,
		Status:        StatusReady,
		Message:       "codex app-server semantic dispatch completed",
		Command:       commandLine(opts.CheckOptions),
		Workspace:     workspace,
		RunDir:        runDir,
		StartedAt:     startedAt,
		FinishedAt:    time.Now().UTC().Format(time.RFC3339),
		ThreadID:      threadID,
		Turns:         turns,
		Budget:        budget,
		RunnerResult:  runnerResult,
		ArtifactRefs:  refs,
		EventRefs:     []string{startEventID, runnerStartEventID, completedEventID, runnerCompletedEventID, auditEvent.ID},
		AuditRef:      auditRef,
		Scope:         runScope(paths.Root, opts).Map(),
		Conditions: []Condition{{
			Type:    "Ready",
			Reason:  "SemanticDispatchCompleted",
			Message: "Real Codex turn artifacts are evidence only until Mnemon validation/governance applies or proposes changes.",
		}},
	}
	return writeSemanticOutcome(paths, report, completedEventID)
}

func runWorkspace(root, runDir, projectRoot string) (string, bool, error) {
	if strings.TrimSpace(projectRoot) == "" {
		return filepath.Join(runDir, "workspace"), true, nil
	}
	workspace := strings.TrimSpace(projectRoot)
	if !filepath.IsAbs(workspace) {
		workspace = filepath.Join(root, workspace)
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return "", false, fmt.Errorf("resolve project root workspace: %w", err)
	}
	return filepath.Clean(abs), false, nil
}

func normalizeRunOptions(opts *RunOptions) {
	if opts.Timeout <= 0 {
		opts.Timeout = 5 * time.Minute
	}
	if opts.TurnTimeout <= 0 {
		opts.TurnTimeout = 3 * time.Minute
	}
	if opts.Now.IsZero() {
		opts.Now = time.Now().UTC()
	}
	if opts.Command == "" {
		opts.Command = "codex"
	}
	if opts.ClientName == "" {
		opts.ClientName = "mnemon-lifecycle"
	}
	if opts.ClientVersion == "" {
		opts.ClientVersion = "dev"
	}
	if opts.RunID == "" {
		opts.RunID = opts.Now.UTC().Format("20060102T150405Z")
	}
	if opts.JobID == "" {
		opts.JobID = "job_" + opts.RunID
	}
	if opts.JobSpec == "" {
		opts.JobSpec = "manual.semantic"
	}
	if opts.Loop == "" {
		opts.Loop = "eval"
	}
	if opts.MaxTurns <= 0 {
		opts.MaxTurns = defaultMaxTurns
	}
}

func runPrompts(opts RunOptions) []string {
	if len(opts.Prompts) > 0 {
		return opts.Prompts
	}
	if strings.TrimSpace(opts.Prompt) == "" {
		return nil
	}
	return []string{opts.Prompt}
}

func hasExplicitCodexAuthEnv(opts CheckOptions) bool {
	env := map[string]string{}
	for _, pair := range os.Environ() {
		if key, value, ok := strings.Cut(pair, "="); ok {
			env[key] = value
		}
	}
	for _, pair := range opts.Env {
		if key, value, ok := strings.Cut(pair, "="); ok {
			env[key] = value
		}
	}
	for _, key := range []string{"OPENAI_API_KEY", "CODEX_API_KEY"} {
		if strings.TrimSpace(env[key]) != "" {
			return true
		}
	}
	return false
}

func blockedSemanticReport(runID, runDir, workspace string, opts RunOptions, budget lifecyclerunner.Budget, startedAt, reason, message string) SemanticReport {
	return SemanticReport{
		SchemaVersion: 1,
		Kind:          "CodexAppServerSemanticRunReport",
		RunID:         runID,
		RunnerID:      RunnerID,
		JobID:         opts.JobID,
		JobSpec:       opts.JobSpec,
		Loop:          opts.Loop,
		Status:        StatusBlocked,
		Message:       message,
		Command:       commandLine(opts.CheckOptions),
		Workspace:     workspace,
		RunDir:        runDir,
		StartedAt:     startedAt,
		FinishedAt:    time.Now().UTC().Format(time.RFC3339),
		Budget:        budget,
		Scope:         runScope("", opts).Map(),
		Conditions: []Condition{{
			Type:    "Blocked",
			Reason:  reason,
			Message: message,
		}},
	}
}

func failSemanticRun(paths layout.Paths, store *eventlog.Store, runID, runDir, workspace string, opts RunOptions, budget lifecyclerunner.Budget, startedAt string, class FailureClass, reason, message, startEventID, runnerStartEventID string, turns []TurnRecord) (RunResult, error) {
	refs := semanticArtifactRefs(paths.Root, workspace, filepath.Join(runDir, "logs", "codex-app-server.stderr.log"), filepath.Join(runDir, "artifacts", "jsonrpc-transcript.jsonl"), filepath.Join(runDir, "artifacts"))
	failedEventID := eventID(runID, "job_failed")
	_ = store.Append(jobEvent(failedEventID, "job.failed", opts, refs, nil))
	runnerFailedEventID := eventID(runID, "runner_semantic_failed")
	runnerStatus := StatusDegraded
	if class == FailureAuthQuotaUnavailable {
		runnerStatus = StatusBlocked
	}
	_ = store.Append(runnerSemanticEvent(runnerFailedEventID, "runner.semantic_run_failed", opts, string(runnerStatus), reason, message, failedEventID, refs))
	report := SemanticReport{
		SchemaVersion: 1,
		Kind:          "CodexAppServerSemanticRunReport",
		RunID:         runID,
		RunnerID:      RunnerID,
		JobID:         opts.JobID,
		JobSpec:       opts.JobSpec,
		Loop:          opts.Loop,
		Status:        StatusDegraded,
		FailureClass:  class,
		Message:       message,
		Command:       commandLine(opts.CheckOptions),
		Workspace:     workspace,
		RunDir:        runDir,
		StartedAt:     startedAt,
		FinishedAt:    time.Now().UTC().Format(time.RFC3339),
		Turns:         turns,
		Budget:        budget,
		ArtifactRefs:  refs,
		EventRefs:     []string{startEventID, runnerStartEventID, failedEventID, runnerFailedEventID},
		Scope:         runScope(paths.Root, opts).Map(),
		Conditions: []Condition{{
			Type:    conditionType(StatusDegraded),
			Reason:  reason,
			Message: message,
		}},
	}
	if class == FailureAuthQuotaUnavailable {
		report.Status = StatusBlocked
		report.Conditions[0].Type = "Blocked"
	}
	return writeSemanticOutcome(paths, report, runnerFailedEventID)
}

func writeSemanticOutcome(paths layout.Paths, report SemanticReport, lastEventID string) (RunResult, error) {
	reportPath := filepath.Join(report.RunDir, "reports", "semantic-run.json")
	if err := writeJSONAtomic(reportPath, report); err != nil {
		return RunResult{}, err
	}
	mirrorReportPath := filepath.Join(paths.ReportsDir, "runner", report.RunID+"-codex-app-server-semantic-run.json")
	if err := writeJSONAtomic(mirrorReportPath, report); err != nil {
		return RunResult{}, err
	}
	statusPath := filepath.Join(paths.StatusDir, "runners", RunnerID+".json")
	if err := writeJSONAtomic(statusPath, semanticRunnerStatus(report, mirrorReportPath, lastEventID)); err != nil {
		return RunResult{}, err
	}
	jobStatusPath := filepath.Join(paths.StatusDir, "jobs", report.JobID+".json")
	if err := writeJSONAtomic(jobStatusPath, semanticJobStatus(report, mirrorReportPath, lastEventID)); err != nil {
		return RunResult{}, err
	}
	return RunResult{
		RunID:        report.RunID,
		Status:       report.Status,
		FailureClass: report.FailureClass,
		Message:      report.Message,
		TurnCount:    report.Budget.UsedTurns,
		ThreadID:     report.ThreadID,
		LastEventID:  lastEventID,
		ReportPath:   mirrorReportPath,
		StatusPath:   statusPath,
		RunDir:       report.RunDir,
		Workspace:    report.Workspace,
	}, nil
}

func writeBlockedSemanticOutcome(paths layout.Paths, store *eventlog.Store, report SemanticReport, opts RunOptions) (RunResult, error) {
	blockedEventID := eventID(report.RunID, "job_blocked")
	if err := store.Append(jobEvent(blockedEventID, "job.blocked", opts, nil, nil)); err != nil {
		return RunResult{}, err
	}
	runnerEventType := "runner.semantic_run_failed"
	runnerEventSuffix := "runner_semantic_failed"
	if len(report.Conditions) > 0 && report.Conditions[0].Reason == "TurnBudgetExceeded" {
		runnerEventType = "runner.budget_exhausted"
		runnerEventSuffix = "runner_budget_exhausted"
	}
	reason := "SemanticRunBlocked"
	message := report.Message
	if len(report.Conditions) > 0 {
		reason = report.Conditions[0].Reason
	}
	runnerBlockedEventID := eventID(report.RunID, runnerEventSuffix)
	if err := store.Append(runnerSemanticEvent(runnerBlockedEventID, runnerEventType, opts, string(StatusBlocked), reason, message, blockedEventID, nil)); err != nil {
		return RunResult{}, err
	}
	report.EventRefs = []string{blockedEventID, runnerBlockedEventID}
	return writeSemanticOutcome(paths, report, runnerBlockedEventID)
}

func semanticRunnerStatus(report SemanticReport, reportPath, lastEventID string) map[string]any {
	return map[string]any{
		"schema_version": 1,
		"kind":           "RunnerStatus",
		"metadata": map[string]any{
			"name":      RunnerID,
			"runner_id": RunnerID,
		},
		"status": map[string]any{
			"phase":                  string(report.Status),
			"last_refreshed_at":      report.FinishedAt,
			"last_included_event_id": lastEventID,
			"turn_budget":            report.Budget,
			"last_report_ref":        map[string]any{"uri": relativeOrAbsolute(reportPath)},
			"conditions": []schema.Condition{{
				Type:             conditionType(report.Status),
				Status:           "true",
				Reason:           semanticReason(report),
				Message:          report.Message,
				LastTransitionTS: report.FinishedAt,
				LastEventID:      lastEventID,
			}},
		},
	}
}

func semanticJobStatus(report SemanticReport, reportPath, lastEventID string) map[string]any {
	return map[string]any{
		"schema_version": 1,
		"kind":           "JobStatus",
		"metadata": map[string]any{
			"name": report.JobID,
			"job":  report.JobID,
		},
		"status": map[string]any{
			"phase":                  string(report.Status),
			"last_refreshed_at":      report.FinishedAt,
			"last_included_event_id": lastEventID,
			"runner_id":              RunnerID,
			"turn_count":             report.Budget.UsedTurns,
			"report_ref":             map[string]any{"uri": relativeOrAbsolute(reportPath)},
			"conditions": []schema.Condition{{
				Type:             conditionType(report.Status),
				Status:           "true",
				Reason:           semanticReason(report),
				Message:          report.Message,
				LastTransitionTS: report.FinishedAt,
				LastEventID:      lastEventID,
			}},
		},
	}
}

func semanticReason(report SemanticReport) string {
	if len(report.Conditions) > 0 && report.Conditions[0].Reason != "" {
		return report.Conditions[0].Reason
	}
	if report.Status == StatusReady {
		return "SemanticDispatchCompleted"
	}
	return "SemanticDispatchBlocked"
}

func jobEvent(id, typ string, opts RunOptions, refs []ArtifactRef, auditRef map[string]any) schema.Event {
	host := "codex"
	loop := opts.Loop
	scope := runScope("", opts).Map()
	payload := map[string]any{
		"job_id":        opts.JobID,
		"job_spec":      opts.JobSpec,
		"runner_id":     RunnerID,
		"real_turn":     true,
		"max_turns":     opts.MaxTurns,
		"target":        map[string]any{"loop": opts.Loop, "job_id": opts.JobID},
		"artifact_refs": artifactRawObjects(refs),
	}
	event := schema.Event{
		SchemaVersion: 1,
		ID:            id,
		TS:            opts.Now.UTC().Format(time.RFC3339),
		Type:          typ,
		Loop:          &loop,
		Host:          &host,
		Actor:         "host-runner",
		Source:        "codex.app-server",
		CorrelationID: opts.RunID,
		CausedBy:      nil,
		Payload:       payload,
		Scope:         scope,
		ArtifactRefs:  artifactRawObjects(refs),
	}
	if auditRef != nil {
		event.AuditRef = auditRef
	}
	return event
}

func runnerSemanticEvent(id, typ string, opts RunOptions, toPhase, reason, message, causedBy string, refs []ArtifactRef) schema.Event {
	host := "codex"
	loop := opts.Loop
	scope := runScope("", opts).Map()
	payload := map[string]any{
		"runner_id": RunnerID,
		"run_id":    opts.RunID,
		"job_id":    opts.JobID,
		"job_spec":  opts.JobSpec,
		"from_phase": func() string {
			if typ == "runner.semantic_run_started" {
				return ""
			}
			return "running"
		}(),
		"to_phase": toPhase,
		"reason":   reason,
		"message":  message,
	}
	event := schema.Event{
		SchemaVersion: schema.Version,
		ID:            id,
		TS:            opts.Now.UTC().Format(time.RFC3339),
		Type:          typ,
		Loop:          &loop,
		Host:          &host,
		Actor:         "host-runner",
		Source:        "codex.app-server",
		CorrelationID: opts.RunID,
		Payload:       payload,
		Scope:         scope,
		ArtifactRefs:  artifactRawObjects(refs),
	}
	if strings.TrimSpace(causedBy) != "" {
		event.CausedBy = &causedBy
	}
	return event
}

func auditSpec(opts RunOptions, refs []ArtifactRef, eventID string) map[string]any {
	return map[string]any{
		"job_id":        opts.JobID,
		"job_spec":      opts.JobSpec,
		"runner_id":     RunnerID,
		"scope":         runScope("", opts).Map(),
		"event_refs":    []string{eventID},
		"artifact_refs": artifactRawObjects(refs),
		"decision":      "retain real app-server run evidence only; no canonical lifecycle mutation applied",
	}
}

func runScope(root string, opts RunOptions) schema.ScopeRef {
	projectRoot := root
	if strings.TrimSpace(projectRoot) == "" {
		projectRoot = opts.ProjectRoot
	}
	return schema.ProjectScopeWithProfile(projectRoot, "", "codex", opts.Loop, "")
}

func semanticArtifactRefs(root, workspace, stderrPath, transcriptPath, artifactsDir string) []ArtifactRef {
	refs := artifactRefs(root, stderrPath, workspace)
	if stat, err := os.Stat(transcriptPath); err == nil && !stat.IsDir() {
		refs = append(refs, artifactRefFor(root, "artifact:jsonrpc-transcript", "transcript", transcriptPath, "application/jsonl"))
	}
	entries, _ := os.ReadDir(artifactsDir)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "prompt-") {
			continue
		}
		path := filepath.Join(artifactsDir, entry.Name())
		refs = append(refs, artifactRefFor(root, "artifact:"+strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())), "command", path, "text/plain"))
	}
	return refs
}

func artifactRefFor(root, id, kind, path, mediaType string) ArtifactRef {
	ref := ArtifactRef{
		ID:        id,
		Kind:      kind,
		URI:       relativeTo(root, path),
		MediaType: mediaType,
		Privacy:   "project",
	}
	if preHash, err := redactArtifactFile(path, DefaultArtifactRedactor()); err == nil {
		ref.PreRedactionSHA256 = preHash
	}
	if hash, err := fileSHA256(path); err == nil {
		ref.SHA256 = "sha256:" + hash
	}
	return ref
}

func toRunnerArtifactRefs(refs []ArtifactRef) []lifecyclerunner.ArtifactRef {
	result := make([]lifecyclerunner.ArtifactRef, 0, len(refs))
	for _, ref := range refs {
		result = append(result, lifecyclerunner.ArtifactRef{
			ID:                 ref.ID,
			Kind:               ref.Kind,
			URI:                ref.URI,
			MediaType:          ref.MediaType,
			SHA256:             ref.SHA256,
			PreRedactionSHA256: ref.PreRedactionSHA256,
			Privacy:            ref.Privacy,
		})
	}
	return result
}

func artifactRawObjects(refs []ArtifactRef) []schema.RawObject {
	result := make([]schema.RawObject, 0, len(refs))
	for _, ref := range refs {
		object := schema.RawObject{
			"id":         ref.ID,
			"kind":       ref.Kind,
			"uri":        ref.URI,
			"media_type": ref.MediaType,
			"sha256":     ref.SHA256,
			"privacy":    ref.Privacy,
		}
		if ref.PreRedactionSHA256 != "" {
			object["pre_redaction_sha256"] = ref.PreRedactionSHA256
		}
		result = append(result, object)
	}
	return result
}

func semanticDeveloperInstructions(opts RunOptions, mnemonDir string) string {
	return "You are running a Mnemon lifecycle semantic job in an isolated workspace. " +
		"Return concise structured evidence. Do not modify canonical memory, skill, projection, docs, or policy state. " +
		"Any semantic change must be described as a proposal candidate. " +
		fmt.Sprintf("Job spec: %s. Mnemon state source: %s.", opts.JobSpec, mnemonDir)
}

func turnCompletionFailure(completed rpcMessage) (bool, string, string, FailureClass) {
	status := strings.TrimSpace(nestedString(completed.Params, "turn", "status"))
	errorMessage := nestedErrorMessage(completed.Params["turn"])
	if status == "" {
		status = strings.TrimSpace(stringValue(completed.Params["status"]))
	}
	if errorMessage == "" {
		errorMessage = nestedErrorMessage(completed.Params["error"])
	}
	if status == "" {
		return true, "TurnCompletionStatusMissing", "turn/completed did not include a terminal turn status", FailureProtocolUnavailable
	}
	if status == "completed" || status == "succeeded" {
		return false, "", "", FailureNone
	}
	if errorMessage == "" {
		errorMessage = "turn/completed returned status " + status
	}
	class := FailureProtocolUnavailable
	reason := "TurnFailed"
	if looksLikeAuthQuota(errorMessage) {
		class = FailureAuthQuotaUnavailable
		reason = "AuthQuotaUnavailable"
	}
	return true, reason, "turn/completed failed: " + errorMessage, class
}

func nestedErrorMessage(value any) string {
	object, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	if msg := stringValue(object["message"]); msg != "" {
		return msg
	}
	if errorValue, ok := object["error"]; ok {
		return nestedErrorMessage(errorValue)
	}
	return ""
}

func stringValue(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func nestedString(value map[string]any, parent, key string) string {
	parentValue, ok := value[parent].(map[string]any)
	if !ok {
		return ""
	}
	text, _ := parentValue[key].(string)
	return text
}

func rpcMessageMap(msg rpcMessage) map[string]any {
	data, err := json.Marshal(msg)
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return map[string]any{}
	}
	return out
}

func eventID(runID, suffix string) string {
	clean := strings.NewReplacer(":", "_", "-", "_", ".", "_", "/", "_").Replace(runID)
	return "evt_" + clean + "_" + suffix
}
