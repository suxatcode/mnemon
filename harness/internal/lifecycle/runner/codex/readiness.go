package codex

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/eventlog"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/layout"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
)

const RunnerID = "codex-app-server"

type Status string

const (
	StatusReady    Status = "ready"
	StatusDegraded Status = "degraded"
	StatusBlocked  Status = "blocked"
)

type FailureClass string

const (
	FailureNone                 FailureClass = ""
	FailureCommandMissing       FailureClass = "command_missing"
	FailureProtocolUnavailable  FailureClass = "protocol_unavailable"
	FailureAuthQuotaUnavailable FailureClass = "auth_quota_unavailable"
)

type CheckOptions struct {
	Command          string
	Args             []string
	Env              []string
	Timeout          time.Duration
	Now              time.Time
	IsolateCodexHome bool
	RunID            string
	ClientName       string
	ClientVersion    string
}

type CheckResult struct {
	Status       Status
	FailureClass FailureClass
	Message      string
	ReportPath   string
	StatusPath   string
	RunDir       string
	Workspace    string
}

type Report struct {
	SchemaVersion int            `json:"schema_version"`
	Kind          string         `json:"kind"`
	RunID         string         `json:"run_id"`
	RunnerID      string         `json:"runner_id"`
	Status        Status         `json:"status"`
	FailureClass  FailureClass   `json:"failure_class,omitempty"`
	Message       string         `json:"message"`
	Command       []string       `json:"command"`
	Workspace     string         `json:"workspace"`
	RunDir        string         `json:"run_dir"`
	StartedAt     string         `json:"started_at"`
	FinishedAt    string         `json:"finished_at"`
	Initialize    map[string]any `json:"initialize,omitempty"`
	SkillsListOK  bool           `json:"skills_list_ok"`
	ModelListOK   bool           `json:"model_list_ok"`
	ArtifactRefs  []ArtifactRef  `json:"artifact_refs"`
	Conditions    []Condition    `json:"conditions,omitempty"`
}

type ArtifactRef struct {
	ID                 string `json:"id,omitempty"`
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
	Message string `json:"message"`
}

type rpcMessage struct {
	ID     *int           `json:"id,omitempty"`
	Method string         `json:"method,omitempty"`
	Params map[string]any `json:"params,omitempty"`
	Result map[string]any `json:"result,omitempty"`
	Error  map[string]any `json:"error,omitempty"`
}

type client struct {
	cmd           *exec.Cmd
	stdin         io.WriteCloser
	lines         chan []byte
	stderr        *os.File
	transcript    *os.File
	nextID        int
	mu            sync.Mutex
	notifications []rpcMessage
	done          chan struct{}
	readErr       error
}

func Check(ctx context.Context, root string, opts CheckOptions) (CheckResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 30 * time.Second
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

	paths, err := layout.EnsureProject(root)
	if err != nil {
		return CheckResult{}, err
	}
	runID := opts.RunID
	if runID == "" {
		runID = opts.Now.UTC().Format("20060102T150405Z")
	}
	runDir := filepath.Join(paths.HarnessDir, "runs", "codex-app-server", runID)
	workspace := filepath.Join(runDir, "workspace")
	logsDir := filepath.Join(runDir, "logs")
	reportsDir := filepath.Join(runDir, "reports")
	artifactsDir := filepath.Join(runDir, "artifacts")
	for _, dir := range []string{workspace, filepath.Join(workspace, ".mnemon"), filepath.Join(workspace, ".codex"), logsDir, reportsDir, artifactsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return CheckResult{}, fmt.Errorf("create runner dir: %w", err)
		}
	}
	if err := os.WriteFile(filepath.Join(workspace, "README.md"), []byte("# Mnemon Codex App-Server Readiness\n"), 0o644); err != nil {
		return CheckResult{}, fmt.Errorf("write workspace readme: %w", err)
	}

	commandPath, err := exec.LookPath(opts.Command)
	if err != nil {
		return writeOutcome(paths, runDir, workspace, opts, Report{
			SchemaVersion: 1,
			Kind:          "CodexAppServerReadinessReport",
			RunID:         runID,
			RunnerID:      RunnerID,
			Status:        StatusBlocked,
			FailureClass:  FailureCommandMissing,
			Message:       fmt.Sprintf("codex command %q not found", opts.Command),
			Command:       commandLine(opts),
			Workspace:     workspace,
			RunDir:        runDir,
			StartedAt:     opts.Now.UTC().Format(time.RFC3339),
			FinishedAt:    opts.Now.UTC().Format(time.RFC3339),
			Conditions: []Condition{{
				Type:    "Blocked",
				Reason:  "CommandMissing",
				Message: "Codex CLI command is unavailable.",
			}},
		})
	}

	checkCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	stderrPath := filepath.Join(logsDir, "codex-app-server.stderr.log")
	rpc, err := startClient(checkCtx, commandPath, opts, workspace, stderrPath, "")
	if err != nil {
		report := protocolReport(paths.Root, runID, runDir, workspace, opts, stderrPath, opts.Now, fmt.Sprintf("start app-server: %v", err))
		return writeOutcome(paths, runDir, workspace, opts, report)
	}
	defer rpc.close()

	startedAt := opts.Now.UTC().Format(time.RFC3339)
	initResult, err := rpc.request(checkCtx, "initialize", map[string]any{
		"clientInfo": map[string]any{
			"name":    opts.ClientName,
			"title":   "Mnemon Lifecycle",
			"version": opts.ClientVersion,
		},
	})
	if err != nil {
		report := protocolReport(paths.Root, runID, runDir, workspace, opts, stderrPath, opts.Now, fmt.Sprintf("initialize failed: %v", err))
		return writeOutcome(paths, runDir, workspace, opts, report)
	}
	_ = rpc.notify("initialized", map[string]any{})

	if _, err := rpc.request(checkCtx, "skills/list", map[string]any{"cwds": []string{workspace}, "forceReload": true}); err != nil {
		report := protocolReport(paths.Root, runID, runDir, workspace, opts, stderrPath, opts.Now, fmt.Sprintf("skills/list failed: %v", err))
		return writeOutcome(paths, runDir, workspace, opts, report)
	}

	modelListOK := true
	if _, err := rpc.request(checkCtx, "model/list", map[string]any{"includeHidden": false}); err != nil {
		class := FailureProtocolUnavailable
		status := StatusDegraded
		reason := "ProtocolUnavailable"
		if looksLikeAuthQuota(err.Error()) {
			class = FailureAuthQuotaUnavailable
			status = StatusBlocked
			reason = "AuthQuotaUnavailable"
		}
		report := Report{
			SchemaVersion: 1,
			Kind:          "CodexAppServerReadinessReport",
			RunID:         runID,
			RunnerID:      RunnerID,
			Status:        status,
			FailureClass:  class,
			Message:       fmt.Sprintf("model/list failed: %v", err),
			Command:       commandLine(opts),
			Workspace:     workspace,
			RunDir:        runDir,
			StartedAt:     startedAt,
			FinishedAt:    time.Now().UTC().Format(time.RFC3339),
			Initialize:    initResult,
			SkillsListOK:  true,
			ModelListOK:   false,
			ArtifactRefs:  artifactRefs(paths.Root, stderrPath, workspace),
			Conditions: []Condition{{
				Type:    conditionType(status),
				Reason:  reason,
				Message: "Codex app-server protocol is available but model/provider readiness failed.",
			}},
		}
		return writeOutcome(paths, runDir, workspace, opts, report)
	}

	report := Report{
		SchemaVersion: 1,
		Kind:          "CodexAppServerReadinessReport",
		RunID:         runID,
		RunnerID:      RunnerID,
		Status:        StatusReady,
		Message:       "codex app-server readiness check passed without starting a real turn",
		Command:       commandLine(opts),
		Workspace:     workspace,
		RunDir:        runDir,
		StartedAt:     startedAt,
		FinishedAt:    time.Now().UTC().Format(time.RFC3339),
		Initialize:    initResult,
		SkillsListOK:  true,
		ModelListOK:   modelListOK,
		ArtifactRefs:  artifactRefs(paths.Root, stderrPath, workspace),
		Conditions: []Condition{{
			Type:    "Ready",
			Reason:  "ReadinessPassed",
			Message: "initialize, skills/list, and model/list completed without a real Codex turn.",
		}},
	}
	return writeOutcome(paths, runDir, workspace, opts, report)
}

func startClient(ctx context.Context, command string, opts CheckOptions, workspace, stderrPath, transcriptPath string) (*client, error) {
	args := opts.Args
	if args == nil {
		args = []string{"app-server", "--listen", "stdio://"}
	}
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = workspace
	env := append([]string{}, os.Environ()...)
	env = append(env, opts.Env...)
	if opts.IsolateCodexHome {
		codexHome := filepath.Join(filepath.Dir(workspace), "codex-home")
		if err := os.MkdirAll(codexHome, 0o755); err != nil {
			return nil, err
		}
		env = append(env, "CODEX_HOME="+codexHome)
	}
	cmd.Env = env
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := os.Create(stderrPath)
	if err != nil {
		return nil, err
	}
	var transcript *os.File
	if transcriptPath != "" {
		transcript, err = os.Create(transcriptPath)
		if err != nil {
			_ = stderr.Close()
			return nil, err
		}
	}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		_ = stderr.Close()
		if transcript != nil {
			_ = transcript.Close()
		}
		return nil, err
	}
	rpc := &client{
		cmd:        cmd,
		stdin:      stdin,
		lines:      make(chan []byte, 64),
		stderr:     stderr,
		transcript: transcript,
		nextID:     1,
		done:       make(chan struct{}),
	}
	go rpc.read(stdout)
	return rpc, nil
}

func (c *client) read(stdout io.Reader) {
	defer close(c.done)
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		c.writeTranscript("server", line)
		c.lines <- line
	}
	c.readErr = scanner.Err()
	close(c.lines)
}

func (c *client) request(ctx context.Context, method string, params map[string]any) (map[string]any, error) {
	c.mu.Lock()
	id := c.nextID
	c.nextID++
	c.mu.Unlock()
	idCopy := id
	if err := c.write(rpcMessage{ID: &idCopy, Method: method, Params: params}); err != nil {
		return nil, err
	}
	for {
		msg, err := c.nextMessage(ctx)
		if err != nil {
			return nil, err
		}
		if msg.ID == nil {
			c.mu.Lock()
			c.notifications = append(c.notifications, msg)
			c.mu.Unlock()
			continue
		}
		if *msg.ID != id {
			continue
		}
		if msg.Error != nil {
			return nil, fmt.Errorf("json-rpc error: %v", msg.Error)
		}
		return msg.Result, nil
	}
}

func (c *client) notify(method string, params map[string]any) error {
	return c.write(rpcMessage{Method: method, Params: params})
}

func (c *client) notificationCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.notifications)
}

func (c *client) waitNotification(ctx context.Context, method string, startIndex int) (rpcMessage, error) {
	for {
		c.mu.Lock()
		for _, msg := range c.notifications[startIndex:] {
			if msg.Method == method {
				c.mu.Unlock()
				return msg, nil
			}
		}
		startIndex = len(c.notifications)
		c.mu.Unlock()

		msg, err := c.nextMessage(ctx)
		if err != nil {
			return rpcMessage{}, err
		}
		if msg.ID == nil {
			c.mu.Lock()
			c.notifications = append(c.notifications, msg)
			c.mu.Unlock()
			if msg.Method == method {
				return msg, nil
			}
		}
	}
}

func (c *client) nextMessage(ctx context.Context) (rpcMessage, error) {
	select {
	case <-ctx.Done():
		return rpcMessage{}, ctx.Err()
	case line, ok := <-c.lines:
		if !ok {
			if c.readErr != nil {
				return rpcMessage{}, c.readErr
			}
			if c.cmd.ProcessState != nil {
				return rpcMessage{}, fmt.Errorf("app-server exited: %s", c.cmd.ProcessState.String())
			}
			return rpcMessage{}, errors.New("app-server stdout closed")
		}
		var msg rpcMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			return rpcMessage{}, fmt.Errorf("invalid JSON-RPC line %q: %w", string(line), err)
		}
		return msg, nil
	}
}

func (c *client) write(msg rpcMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	c.writeTranscript("client", data)
	if _, err := c.stdin.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

func (c *client) writeTranscript(direction string, payload []byte) {
	if c.transcript == nil {
		return
	}
	record := map[string]any{
		"direction": direction,
		"payload":   json.RawMessage(payload),
	}
	data, err := json.Marshal(record)
	if err != nil {
		return
	}
	_, _ = c.transcript.Write(append(data, '\n'))
}

func (c *client) close() {
	_ = c.stdin.Close()
	if c.cmd.Process != nil && c.cmd.ProcessState == nil {
		_ = c.cmd.Process.Signal(os.Interrupt)
		done := make(chan struct{})
		go func() {
			_ = c.cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			_ = c.cmd.Process.Kill()
			<-done
		}
	}
	c.waitReaderDone()
	_ = c.stderr.Close()
	if c.transcript != nil {
		_ = c.transcript.Close()
	}
}

func (c *client) waitReaderDone() {
	timeout := time.After(3 * time.Second)
	for {
		select {
		case <-c.done:
			return
		case _, ok := <-c.lines:
			if !ok {
				<-c.done
				return
			}
		case <-timeout:
			return
		}
	}
}

func protocolReport(root, runID, runDir, workspace string, opts CheckOptions, stderrPath string, now time.Time, message string) Report {
	return Report{
		SchemaVersion: 1,
		Kind:          "CodexAppServerReadinessReport",
		RunID:         runID,
		RunnerID:      RunnerID,
		Status:        StatusDegraded,
		FailureClass:  FailureProtocolUnavailable,
		Message:       message,
		Command:       commandLine(opts),
		Workspace:     workspace,
		RunDir:        runDir,
		StartedAt:     now.UTC().Format(time.RFC3339),
		FinishedAt:    time.Now().UTC().Format(time.RFC3339),
		ArtifactRefs:  artifactRefs(root, stderrPath, workspace),
		Conditions: []Condition{{
			Type:    "Degraded",
			Reason:  "ProtocolUnavailable",
			Message: "Codex app-server did not complete the readiness protocol.",
		}},
	}
}

func writeOutcome(paths layout.Paths, runDir, workspace string, opts CheckOptions, report Report) (CheckResult, error) {
	if report.ArtifactRefs == nil {
		report.ArtifactRefs = artifactRefs(paths.Root, filepath.Join(runDir, "logs", "codex-app-server.stderr.log"), workspace)
	}
	reportPath := filepath.Join(runDir, "reports", "readiness.json")
	if err := writeJSONAtomic(reportPath, report); err != nil {
		return CheckResult{}, err
	}
	mirrorReportPath := filepath.Join(paths.ReportsDir, "runner", report.RunID+"-codex-app-server-readiness.json")
	if err := writeJSONAtomic(mirrorReportPath, report); err != nil {
		return CheckResult{}, err
	}
	readinessEventID, err := appendReadinessEvent(paths, report, mirrorReportPath)
	if err != nil {
		return CheckResult{}, err
	}
	statusPath := filepath.Join(paths.StatusDir, "runners", RunnerID+".json")
	if err := writeJSONAtomic(statusPath, runnerStatus(report, mirrorReportPath, readinessEventID)); err != nil {
		return CheckResult{}, err
	}
	return CheckResult{
		Status:       report.Status,
		FailureClass: report.FailureClass,
		Message:      report.Message,
		ReportPath:   mirrorReportPath,
		StatusPath:   statusPath,
		RunDir:       runDir,
		Workspace:    workspace,
	}, nil
}

func appendReadinessEvent(paths layout.Paths, report Report, reportPath string) (string, error) {
	previousPhase, previousEventID, err := lastRunnerPhase(paths.Root)
	if err != nil {
		return "", err
	}
	if previousPhase == string(report.Status) {
		return previousEventID, nil
	}
	store, err := eventlog.New(paths.Root)
	if err != nil {
		return "", err
	}
	host := "codex"
	event := schema.Event{
		SchemaVersion: schema.Version,
		ID:            eventID(report.RunID, readinessEventSuffix(report.Status)),
		TS:            report.FinishedAt,
		Type:          readinessEventType(report.Status),
		Host:          &host,
		Actor:         "host-runner",
		Source:        "codex.app-server",
		CorrelationID: report.RunID,
		Payload: map[string]any{
			"runner_id":     RunnerID,
			"run_id":        report.RunID,
			"from_phase":    previousPhase,
			"to_phase":      string(report.Status),
			"failure_class": string(report.FailureClass),
			"message":       report.Message,
			"report_ref":    map[string]any{"uri": relativeOrAbsolute(reportPath)},
		},
	}
	if err := store.Append(event); err != nil {
		return "", err
	}
	return event.ID, nil
}

func runnerStatus(report Report, reportPath, lastEventID string) map[string]any {
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
			"last_report_ref": map[string]any{
				"uri": relativeOrAbsolute(reportPath),
			},
			"failure_class": report.FailureClass,
			"conditions": []schema.Condition{{
				Type:             conditionType(report.Status),
				Status:           "true",
				Reason:           statusReason(report),
				Message:          report.Message,
				LastTransitionTS: report.FinishedAt,
			}},
		},
	}
}

func lastRunnerPhase(root string) (string, string, error) {
	store, err := eventlog.New(root)
	if err != nil {
		return "", "", err
	}
	events, err := store.ReadAll()
	if err != nil {
		return "", "", err
	}
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		if !strings.HasPrefix(event.Type, "runner.") {
			continue
		}
		runnerID, _ := event.Payload["runner_id"].(string)
		if runnerID != RunnerID {
			continue
		}
		phase, _ := event.Payload["to_phase"].(string)
		if phase != "" {
			return phase, event.ID, nil
		}
	}
	return "", "", nil
}

func readinessEventType(status Status) string {
	switch status {
	case StatusReady:
		return "runner.readiness_passed"
	case StatusBlocked:
		return "runner.readiness_blocked"
	default:
		return "runner.readiness_degraded"
	}
}

func readinessEventSuffix(status Status) string {
	switch status {
	case StatusReady:
		return "readiness_passed"
	case StatusBlocked:
		return "readiness_blocked"
	default:
		return "readiness_degraded"
	}
}

func artifactRefs(root, stderrPath, workspace string) []ArtifactRef {
	refs := []ArtifactRef{{
		ID:        "artifact:workspace",
		Kind:      "workspace_snapshot",
		URI:       relativeTo(root, workspace),
		MediaType: "inode/directory",
		Privacy:   "project",
	}}
	if stat, err := os.Stat(stderrPath); err == nil && !stat.IsDir() {
		refs = append(refs, artifactRefFor(root, "artifact:runner-log", "runner_log", stderrPath, "text/plain"))
	}
	return refs
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func looksLikeAuthQuota(message string) bool {
	lower := strings.ToLower(message)
	for _, needle := range []string{"auth", "login", "quota", "rate limit", "rate-limit", "model"} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

func commandLine(opts CheckOptions) []string {
	command := opts.Command
	if command == "" {
		command = "codex"
	}
	args := opts.Args
	if args == nil {
		args = []string{"app-server", "--listen", "stdio://"}
	}
	return append([]string{command}, args...)
}

func conditionType(status Status) string {
	switch status {
	case StatusBlocked:
		return "Blocked"
	case StatusDegraded:
		return "Degraded"
	default:
		return "Ready"
	}
}

func statusReason(report Report) string {
	switch report.FailureClass {
	case FailureCommandMissing:
		return "CommandMissing"
	case FailureProtocolUnavailable:
		return "ProtocolUnavailable"
	case FailureAuthQuotaUnavailable:
		return "AuthQuotaUnavailable"
	default:
		return "ReadinessPassed"
	}
}

func relativeTo(root, path string) string {
	if rel, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return path
}

func relativeOrAbsolute(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Clean(path)
}

func writeJSONAtomic(path string, value any) error {
	return layout.WriteJSONAtomic(path, value, 0o600)
}
