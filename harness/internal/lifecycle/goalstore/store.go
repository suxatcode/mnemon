package goalstore

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/auditstore"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/eventlog"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/goal"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/layout"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
)

var (
	ErrCompletionNotVerified = errors.New("goal completion requires accepted evidence and a passing verification report")
	ErrGoalNotFound          = errors.New("goal not found")
)

type Store struct {
	paths layout.Paths
}

type CreateOptions struct {
	ID        string
	Objective string
	Now       time.Time
}

type PlanOptions struct {
	GoalID               string
	Summary              string
	Steps                []string
	MemoryRefs           []string
	MemoryRecallRequests []string
	SkillWorkflowRefs    []string
	EvalRefs             []string
	Now                  time.Time
}

type EvidenceOptions struct {
	GoalID  string
	ID      string
	Type    string
	Status  string
	Summary string
	Refs    goal.EvidenceRefs
	Now     time.Time
}

type VerifyOptions struct {
	GoalID   string
	GateName string
	Summary  string
	Now      time.Time
}

type CompleteOptions struct {
	GoalID         string
	Now            time.Time
	BlockOnFailure bool
}

type BlockOptions struct {
	GoalID string
	Reason string
	Now    time.Time
}

type PauseOptions struct {
	GoalID string
	Reason string
	Now    time.Time
}

type ResumeOptions struct {
	GoalID string
	Reason string
	Now    time.Time
}

type LinkOptions struct {
	GoalID     string
	Host       string
	ThreadID   string
	HostGoalID string
	Objective  string
	Evidence   []string
	Now        time.Time
}

type NudgeOptions struct {
	GoalID    string
	AllIdle   bool
	IdleAfter time.Duration
	Summary   string
	Now       time.Time
}

type NudgeResult struct {
	GoalID  string
	NudgeID string
	Path    string
	Skipped bool
	Reason  string
}

type StatusView struct {
	Goal     goal.Goal
	Path     string
	Evidence []goal.GoalEvidence
	Ready    bool
}

func New(root string) (*Store, error) {
	paths, err := layout.Resolve(root)
	if err != nil {
		return nil, err
	}
	return &Store{paths: paths}, nil
}

func (s *Store) Create(opts CreateOptions) (goal.Goal, error) {
	paths, err := layout.EnsureProject(s.paths.Root)
	if err != nil {
		return goal.Goal{}, err
	}
	s.paths = paths
	opts.Now = layout.NormalizeNow(opts.Now)
	id := cleanID(opts.ID)
	if id == "" {
		id = generatedGoalID(opts.Objective, opts.Now)
	}
	if strings.TrimSpace(opts.Objective) == "" {
		return goal.Goal{}, errors.New("objective is required")
	}
	dir := s.goalDir(id)
	if _, err := os.Stat(filepath.Join(dir, "goal.json")); err == nil {
		return goal.Goal{}, fmt.Errorf("goal %q already exists", id)
	} else if !os.IsNotExist(err) {
		return goal.Goal{}, fmt.Errorf("stat goal: %w", err)
	}
	item := goal.Goal{
		SchemaVersion: goal.SchemaVersion,
		Kind:          "Goal",
		ID:            id,
		Objective:     strings.TrimSpace(opts.Objective),
		Status:        goal.StatusDraft,
		CreatedAt:     opts.Now.UTC().Format(time.RFC3339),
		UpdatedAt:     opts.Now.UTC().Format(time.RFC3339),
	}
	if err := goal.ValidateGoal(item); err != nil {
		return goal.Goal{}, err
	}
	event := s.event(opts.Now, id, "goal.created", nil, map[string]any{
		"goal_id":   id,
		"status":    string(item.Status),
		"objective": item.Objective,
	})
	if err := s.writeGoalState(item, nil); err != nil {
		return goal.Goal{}, err
	}
	if err := s.appendEvent(event); err != nil {
		return goal.Goal{}, err
	}
	item.LatestEventID = event.ID
	if err := s.writeGoalState(item, nil); err != nil {
		return goal.Goal{}, err
	}
	return item, nil
}

func (s *Store) Plan(opts PlanOptions) (goal.Goal, error) {
	item, evidence, err := s.load(opts.GoalID)
	if err != nil {
		return goal.Goal{}, err
	}
	opts.Now = layout.NormalizeNow(opts.Now)
	plan := goal.GoalPlan{
		SchemaVersion:        goal.PlanSchemaVersion,
		Kind:                 "GoalPlan",
		GoalID:               item.ID,
		Summary:              strings.TrimSpace(opts.Summary),
		Steps:                trimList(opts.Steps),
		MemoryRefs:           trimList(opts.MemoryRefs),
		MemoryRecallRequests: trimList(opts.MemoryRecallRequests),
		SkillWorkflowRefs:    trimList(opts.SkillWorkflowRefs),
		EvalRefs:             trimList(opts.EvalRefs),
		CreatedAt:            opts.Now.UTC().Format(time.RFC3339),
		UpdatedAt:            opts.Now.UTC().Format(time.RFC3339),
	}
	if existing := item.Plan; existing != nil && existing.CreatedAt != "" {
		plan.CreatedAt = existing.CreatedAt
	}
	if err := goal.ValidatePlan(plan); err != nil {
		return goal.Goal{}, err
	}
	if goal.Terminal(item.Status) {
		return goal.Goal{}, goal.TransitionError{From: item.Status, To: goal.StatusPlanned}
	}
	item.Plan = &plan
	if item.Status == goal.StatusDraft {
		if err := goal.ValidateTransition(item.Status, goal.StatusPlanned); err != nil {
			return goal.Goal{}, err
		}
		item.Status = goal.StatusPlanned
	}
	item.UpdatedAt = opts.Now.UTC().Format(time.RFC3339)
	event := s.event(opts.Now, item.ID, "goal.planned", nil, map[string]any{
		"goal_id": item.ID,
		"status":  string(item.Status),
		"summary": plan.Summary,
		"steps":   plan.Steps,
	})
	if err := s.appendEvent(event); err != nil {
		return goal.Goal{}, err
	}
	item.LatestEventID = event.ID
	if err := s.writeGoalState(item, evidence); err != nil {
		return goal.Goal{}, err
	}
	return item, nil
}

func (s *Store) Activate(goalID string, now time.Time) (goal.Goal, error) {
	return s.transition(goalID, goal.StatusActive, "goal.activated", "activated", now, goal.StatusDraft, goal.StatusPlanned, goal.StatusPaused)
}

func (s *Store) AppendEvidence(opts EvidenceOptions) (goal.GoalEvidence, error) {
	item, evidence, err := s.load(opts.GoalID)
	if err != nil {
		return goal.GoalEvidence{}, err
	}
	opts.Now = layout.NormalizeNow(opts.Now)
	if opts.Type == "" {
		opts.Type = "manual"
	}
	if opts.Status == "" {
		opts.Status = "accepted"
	}
	id := cleanID(opts.ID)
	if id == "" {
		id = "evidence-" + cleanID(item.ID) + "-" + layout.TimestampID(opts.Now)
	}
	record := goal.GoalEvidence{
		SchemaVersion: goal.EvidenceSchemaVersion,
		Kind:          "GoalEvidence",
		ID:            id,
		GoalID:        item.ID,
		Type:          opts.Type,
		Status:        opts.Status,
		Summary:       strings.TrimSpace(opts.Summary),
		RecordedAt:    opts.Now.UTC().Format(time.RFC3339),
		Refs:          opts.Refs,
	}
	if err := goal.ValidateEvidence(record); err != nil {
		return goal.GoalEvidence{}, err
	}
	for _, existing := range evidence {
		if existing.ID == record.ID {
			return goal.GoalEvidence{}, fmt.Errorf("evidence id %q already exists", record.ID)
		}
	}
	event := s.event(opts.Now, item.ID, "goal.evidence_recorded", nil, map[string]any{
		"goal_id":     item.ID,
		"evidence_id": record.ID,
		"type":        record.Type,
		"status":      record.Status,
		"summary":     record.Summary,
		"refs":        record.Refs,
	})
	event.ID = eventID(item.ID, "goal.evidence_recorded."+record.ID, opts.Now)
	if err := s.appendEvidence(record); err != nil {
		return goal.GoalEvidence{}, err
	}
	evidence = append(evidence, record)
	item.EvidenceCount = len(evidence)
	item.UpdatedAt = opts.Now.UTC().Format(time.RFC3339)
	if err := s.appendEvent(event); err != nil {
		return goal.GoalEvidence{}, err
	}
	item.LatestEventID = event.ID
	if err := s.writeGoalState(item, evidence); err != nil {
		return goal.GoalEvidence{}, err
	}
	return record, nil
}

func (s *Store) Verify(opts VerifyOptions) (goal.GoalReport, error) {
	item, evidence, err := s.load(opts.GoalID)
	if err != nil {
		return goal.GoalReport{}, err
	}
	opts.Now = layout.NormalizeNow(opts.Now)
	if opts.GateName == "" {
		opts.GateName = "mnemon-goal-evidence-present"
	}
	if err := goal.ValidateTransition(item.Status, goal.StatusVerifying); err != nil {
		return goal.GoalReport{}, err
	}
	accepted := acceptedEvidenceIDs(evidence)
	status := "pass"
	passed := true
	summary := strings.TrimSpace(opts.Summary)
	if summary == "" {
		summary = "Goal verification passed with accepted evidence."
	}
	if len(accepted) == 0 {
		status = "blocked"
		passed = false
		summary = "Goal verification blocked: no accepted evidence has been recorded."
	} else if isEvalPassedGate(opts.GateName) {
		gatePassed, gateSummary := s.verifyEvalPassedGate(evidence)
		if !gatePassed {
			status = "blocked"
			passed = false
			summary = gateSummary
		} else if strings.TrimSpace(opts.Summary) == "" {
			summary = gateSummary
		}
	}
	report := goal.GoalReport{
		SchemaVersion: goal.ReportSchemaVersion,
		Kind:          "GoalReport",
		ID:            "report-" + cleanID(item.ID) + "-" + layout.TimestampID(opts.Now),
		GoalID:        item.ID,
		Status:        status,
		Summary:       summary,
		GeneratedAt:   opts.Now.UTC().Format(time.RFC3339),
		VerificationGate: goal.VerificationGate{
			Name:      opts.GateName,
			Passed:    passed,
			CheckedAt: opts.Now.UTC().Format(time.RFC3339),
			Message:   summary,
		},
		EvidenceRefs: accepted,
	}
	mergeEvidenceRefs(&report, evidence)
	if err := goal.ValidateReport(report); err != nil {
		return goal.GoalReport{}, err
	}
	item.Report = &report
	item.Status = goal.StatusVerifying
	item.UpdatedAt = opts.Now.UTC().Format(time.RFC3339)
	event := s.event(opts.Now, item.ID, "goal.verified", nil, map[string]any{
		"goal_id": item.ID,
		"status":  report.Status,
		"passed":  report.VerificationGate.Passed,
		"report":  report.ID,
	})
	if err := s.appendEvent(event); err != nil {
		return goal.GoalReport{}, err
	}
	item.LatestEventID = event.ID
	if err := s.writeGoalState(item, evidence); err != nil {
		return goal.GoalReport{}, err
	}
	return report, nil
}

func isEvalPassedGate(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), "eval-passed")
}

func (s *Store) verifyEvalPassedGate(records []goal.GoalEvidence) (bool, string) {
	refs := acceptedEvalReportRefs(records)
	if len(refs) == 0 {
		return false, "Goal verification blocked: eval-passed gate requires accepted eval report evidence."
	}
	for _, ref := range refs {
		status, usedTurns, err := s.readEvalReportGateFields(ref)
		if err != nil {
			return false, fmt.Sprintf("Goal verification blocked: eval-passed report %s is not readable: %v", ref, err)
		}
		if status != "ready" {
			return false, fmt.Sprintf("Goal verification blocked: eval-passed report %s has status %q.", ref, status)
		}
		if usedTurns <= 0 {
			return false, fmt.Sprintf("Goal verification blocked: eval-passed report %s used no model turns.", ref)
		}
	}
	return true, fmt.Sprintf("Goal verification passed with %d ready eval report(s).", len(refs))
}

func acceptedEvalReportRefs(records []goal.GoalEvidence) []string {
	var refs []string
	seen := map[string]bool{}
	for _, record := range records {
		if record.Status != "accepted" {
			continue
		}
		for _, ref := range record.Refs.EvalReportRefs {
			ref = strings.TrimSpace(ref)
			if ref == "" || seen[ref] {
				continue
			}
			seen[ref] = true
			refs = append(refs, ref)
		}
	}
	sort.Strings(refs)
	return refs
}

func (s *Store) readEvalReportGateFields(ref string) (string, int, error) {
	path := ref
	if !filepath.IsAbs(path) {
		path = filepath.Join(s.paths.Root, filepath.FromSlash(ref))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", 0, err
	}
	var report struct {
		Status string `json:"status"`
		Budget struct {
			UsedTurns int `json:"used_turns"`
		} `json:"budget"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return "", 0, err
	}
	return strings.TrimSpace(report.Status), report.Budget.UsedTurns, nil
}

func (s *Store) Complete(opts CompleteOptions) (goal.Goal, error) {
	item, evidence, err := s.load(opts.GoalID)
	if err != nil {
		return goal.Goal{}, err
	}
	opts.Now = layout.NormalizeNow(opts.Now)
	if !goal.CompletionReady(item.Report, evidence) {
		if opts.BlockOnFailure {
			return s.Block(BlockOptions{
				GoalID: item.ID,
				Reason: ErrCompletionNotVerified.Error(),
				Now:    opts.Now,
			})
		}
		return goal.Goal{}, ErrCompletionNotVerified
	}
	if err := goal.ValidateTransition(item.Status, goal.StatusComplete); err != nil {
		return goal.Goal{}, err
	}
	item.Status = goal.StatusComplete
	item.CompletedAt = opts.Now.UTC().Format(time.RFC3339)
	item.UpdatedAt = item.CompletedAt
	event := s.event(opts.Now, item.ID, "goal.completed", nil, map[string]any{
		"goal_id": item.ID,
		"status":  string(item.Status),
		"report":  item.Report.ID,
	})
	auditRef, err := s.writeCompletionAuditRecord(item, evidence, event, opts.Now)
	if err != nil {
		return goal.Goal{}, err
	}
	event.AuditRef = auditRef
	if err := s.appendEvent(event); err != nil {
		return goal.Goal{}, err
	}
	item.LatestEventID = event.ID
	if err := s.appendCompletionAuditEvent(item, event, auditRef, opts.Now); err != nil {
		return goal.Goal{}, err
	}
	if err := s.writeGoalState(item, evidence); err != nil {
		return goal.Goal{}, err
	}
	return item, nil
}

func (s *Store) Block(opts BlockOptions) (goal.Goal, error) {
	if strings.TrimSpace(opts.Reason) == "" {
		opts.Reason = "Goal blocked."
	}
	return s.transition(opts.GoalID, goal.StatusBlocked, "goal.blocked", opts.Reason, opts.Now, goal.StatusDraft, goal.StatusPlanned, goal.StatusActive, goal.StatusVerifying, goal.StatusPaused)
}

func (s *Store) Pause(opts PauseOptions) (goal.Goal, error) {
	if strings.TrimSpace(opts.Reason) == "" {
		opts.Reason = "Goal paused."
	}
	return s.transition(opts.GoalID, goal.StatusPaused, "goal.paused", opts.Reason, opts.Now, goal.StatusDraft, goal.StatusPlanned, goal.StatusActive, goal.StatusVerifying)
}

func (s *Store) Resume(opts ResumeOptions) (goal.Goal, error) {
	if strings.TrimSpace(opts.Reason) == "" {
		opts.Reason = "Goal resumed."
	}
	return s.transition(opts.GoalID, goal.StatusActive, "goal.resumed", opts.Reason, opts.Now, goal.StatusPaused)
}

func (s *Store) Link(opts LinkOptions) (goal.HostGoalLink, error) {
	item, evidence, err := s.load(opts.GoalID)
	if err != nil {
		return goal.HostGoalLink{}, err
	}
	opts.Now = layout.NormalizeNow(opts.Now)
	if opts.Host == "" {
		opts.Host = "codex"
	}
	if strings.TrimSpace(opts.Objective) == "" {
		opts.Objective = CodexObjective(item.ID)
	}
	link := goal.HostGoalLink{
		SchemaVersion: goal.HostLinkSchemaVersion,
		Kind:          "HostGoalLink",
		ID:            "link-" + cleanID(opts.Host) + "-" + layout.TimestampID(opts.Now),
		GoalID:        item.ID,
		Host:          opts.Host,
		ThreadID:      strings.TrimSpace(opts.ThreadID),
		HostGoalID:    strings.TrimSpace(opts.HostGoalID),
		Objective:     strings.TrimSpace(opts.Objective),
		Evidence:      trimList(opts.Evidence),
		LinkedAt:      opts.Now.UTC().Format(time.RFC3339),
	}
	if err := goal.ValidateHostGoalLink(link); err != nil {
		return goal.HostGoalLink{}, err
	}
	item.HostLinks = append(item.HostLinks, link)
	item.UpdatedAt = opts.Now.UTC().Format(time.RFC3339)
	host := link.Host
	event := s.event(opts.Now, item.ID, "goal.host_linked", &host, map[string]any{
		"goal_id":      item.ID,
		"host":         link.Host,
		"thread_id":    link.ThreadID,
		"host_goal_id": link.HostGoalID,
		"objective":    link.Objective,
		"evidence":     link.Evidence,
	})
	if err := s.appendEvent(event); err != nil {
		return goal.HostGoalLink{}, err
	}
	item.LatestEventID = event.ID
	if err := s.writeGoalState(item, evidence); err != nil {
		return goal.HostGoalLink{}, err
	}
	return link, nil
}

func (s *Store) Nudge(opts NudgeOptions) ([]NudgeResult, error) {
	opts.Now = layout.NormalizeNow(opts.Now)
	if strings.TrimSpace(opts.Summary) == "" {
		opts.Summary = "Daemon idle goal nudge: review whether this goal needs evidence, verification, blocking, or pausing."
	}
	ids, err := s.nudgeGoalIDs(opts)
	if err != nil {
		return nil, err
	}
	var results []NudgeResult
	for _, id := range ids {
		item, evidence, err := s.load(id)
		if err != nil {
			return results, err
		}
		result := NudgeResult{GoalID: item.ID}
		if item.Status == goal.StatusComplete || item.Status == goal.StatusBlocked || item.Status == goal.StatusPaused {
			result.Skipped = true
			result.Reason = "terminal-or-paused"
			results = append(results, result)
			continue
		}
		lastActivity := latestGoalActivity(item, evidence)
		if opts.IdleAfter > 0 && opts.Now.Sub(lastActivity) < opts.IdleAfter {
			result.Skipped = true
			result.Reason = "not-idle"
			results = append(results, result)
			continue
		}
		nudgeID := "nudge-" + cleanID(item.ID) + "-" + layout.TimestampID(opts.Now)
		path := filepath.Join(s.goalDir(item.ID), "nudges.md")
		if err := appendGoalNudge(path, nudgeID, item, lastActivity, opts.Summary, opts.Now); err != nil {
			return results, err
		}
		event := s.event(opts.Now, item.ID, "goal.nudged", nil, map[string]any{
			"goal_id":       item.ID,
			"nudge_id":      nudgeID,
			"summary":       opts.Summary,
			"last_activity": lastActivity.UTC().Format(time.RFC3339),
		})
		if err := s.appendEvent(event); err != nil {
			return results, err
		}
		result.NudgeID = nudgeID
		result.Path = path
		results = append(results, result)
	}
	return results, nil
}

func (s *Store) nudgeGoalIDs(opts NudgeOptions) ([]string, error) {
	if strings.TrimSpace(opts.GoalID) != "" {
		return []string{cleanID(opts.GoalID)}, nil
	}
	if !opts.AllIdle {
		return nil, errors.New("goal id or --all-idle is required")
	}
	dir := filepath.Join(s.paths.HarnessDir, "goals")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read goals dir: %w", err)
	}
	var ids []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, entry.Name(), "goal.json")); err == nil {
			ids = append(ids, entry.Name())
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("stat goal %s: %w", entry.Name(), err)
		}
	}
	sort.Strings(ids)
	return ids, nil
}

func (s *Store) Status(goalID string) (StatusView, error) {
	item, evidence, err := s.load(goalID)
	if err != nil {
		return StatusView{}, err
	}
	return StatusView{
		Goal:     item,
		Path:     filepath.Join(s.goalDir(item.ID), "goal.json"),
		Evidence: evidence,
		Ready:    goal.CompletionReady(item.Report, evidence),
	}, nil
}

func (s *Store) GoalPath(goalID string) string {
	return s.goalDir(goalID)
}

func CodexObjective(goalID string) string {
	return fmt.Sprintf("Follow .mnemon/harness/goals/%s/GOAL.md, keep EVIDENCE.jsonl updated, and do not mark the work complete until mnemon-harness goal verify --goal-id %s passes.", goalID, goalID)
}

func CodexPrompt(item goal.Goal) string {
	objective := CodexObjective(item.ID)
	var out strings.Builder
	fmt.Fprintf(&out, "/goal %s\n\n", objective)
	fmt.Fprintf(&out, "Prompt snippet name: /mnemon-goal\n\n")
	fmt.Fprintf(&out, "Mnemon project goal: %s\n\n", item.Objective)
	fmt.Fprintf(&out, "Use only supported Mnemon and Codex surfaces:\n")
	fmt.Fprintf(&out, "- Read .mnemon/harness/goals/%s/GOAL.md and PLAN.md before acting.\n", item.ID)
	fmt.Fprintf(&out, "- Record evidence with mnemon-harness goal evidence append --goal-id %s --summary <summary>.\n", item.ID)
	fmt.Fprintf(&out, "- Run mnemon-harness goal verify --goal-id %s before considering completion.\n", item.ID)
	fmt.Fprintf(&out, "- Do not write Codex internal sqlite state; link host ids with mnemon-harness goal link when public APIs expose them.\n")
	return out.String()
}

func (s *Store) load(goalID string) (goal.Goal, []goal.GoalEvidence, error) {
	if strings.TrimSpace(goalID) == "" {
		return goal.Goal{}, nil, errors.New("goal_id is required")
	}
	path := filepath.Join(s.goalDir(goalID), "goal.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return goal.Goal{}, nil, ErrGoalNotFound
		}
		return goal.Goal{}, nil, fmt.Errorf("read goal: %w", err)
	}
	var item goal.Goal
	if err := json.Unmarshal(data, &item); err != nil {
		return goal.Goal{}, nil, fmt.Errorf("decode goal: %w", err)
	}
	evidence, err := s.readEvidence(item.ID)
	if err != nil {
		return goal.Goal{}, nil, err
	}
	item.EvidenceCount = len(evidence)
	if err := goal.ValidateGoal(item); err != nil {
		return goal.Goal{}, nil, err
	}
	return item, evidence, nil
}

func (s *Store) transition(goalID string, status goal.Status, eventType, reason string, now time.Time, allowedSources ...goal.Status) (goal.Goal, error) {
	item, evidence, err := s.load(goalID)
	if err != nil {
		return goal.Goal{}, err
	}
	now = layout.NormalizeNow(now)
	if len(allowedSources) > 0 && !statusIn(item.Status, allowedSources) {
		return goal.Goal{}, goal.TransitionError{From: item.Status, To: status}
	}
	if len(allowedSources) == 0 {
		if err := goal.ValidateTransition(item.Status, status); err != nil {
			return goal.Goal{}, err
		}
	}
	if err := goal.ValidateTransition(item.Status, status); err != nil {
		return goal.Goal{}, err
	}
	item.Status = status
	item.UpdatedAt = now.UTC().Format(time.RFC3339)
	switch status {
	case goal.StatusBlocked:
		item.BlockedAt = item.UpdatedAt
	case goal.StatusPaused:
		item.PausedAt = item.UpdatedAt
	case goal.StatusActive:
		item.PausedAt = ""
	}
	event := s.event(now, item.ID, eventType, nil, map[string]any{
		"goal_id": item.ID,
		"status":  string(item.Status),
		"reason":  reason,
	})
	if err := s.appendEvent(event); err != nil {
		return goal.Goal{}, err
	}
	item.LatestEventID = event.ID
	if err := s.writeGoalState(item, evidence); err != nil {
		return goal.Goal{}, err
	}
	return item, nil
}

func statusIn(status goal.Status, allowed []goal.Status) bool {
	for _, item := range allowed {
		if status == item {
			return true
		}
	}
	return false
}

func (s *Store) writeGoalState(item goal.Goal, evidence []goal.GoalEvidence) error {
	item.EvidenceCount = len(evidence)
	if err := goal.ValidateGoal(item); err != nil {
		return err
	}
	dir := s.goalDir(item.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create goal dir: %w", err)
	}
	if err := writeJSONAtomic(filepath.Join(dir, "goal.json"), item); err != nil {
		return err
	}
	if err := writeTextAtomic(filepath.Join(dir, "GOAL.md"), renderGoalMarkdown(item)); err != nil {
		return err
	}
	if err := writeTextAtomic(filepath.Join(dir, "PLAN.md"), renderPlanMarkdown(item)); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dir, "EVIDENCE.jsonl")); os.IsNotExist(err) {
		if err := writeTextAtomic(filepath.Join(dir, "EVIDENCE.jsonl"), ""); err != nil {
			return err
		}
	} else if err != nil {
		return fmt.Errorf("stat evidence: %w", err)
	}
	if err := writeTextAtomic(filepath.Join(dir, "REPORT.md"), renderReportMarkdown(item)); err != nil {
		return err
	}
	if err := writeJSONAtomic(filepath.Join(s.paths.StatusDir, "goals", item.ID+".json"), goalStatusDocument(item)); err != nil {
		return err
	}
	return nil
}

func appendGoalNudge(path, nudgeID string, item goal.Goal, lastActivity time.Time, summary string, now time.Time) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create nudge parent: %w", err)
	}
	var out strings.Builder
	fmt.Fprintf(&out, "## %s\n\n", nudgeID)
	fmt.Fprintf(&out, "- Time: %s\n", now.UTC().Format(time.RFC3339))
	fmt.Fprintf(&out, "- Goal: %s\n", item.ID)
	fmt.Fprintf(&out, "- Status: %s\n", item.Status)
	fmt.Fprintf(&out, "- Last activity: %s\n", lastActivity.UTC().Format(time.RFC3339))
	fmt.Fprintf(&out, "- Summary: %s\n\n", summary)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open nudge log: %w", err)
	}
	defer file.Close()
	if _, err := file.WriteString(out.String()); err != nil {
		return fmt.Errorf("append nudge: %w", err)
	}
	return nil
}

func latestGoalActivity(item goal.Goal, evidence []goal.GoalEvidence) time.Time {
	latest, _ := time.Parse(time.RFC3339, item.UpdatedAt)
	for _, record := range evidence {
		recordedAt, err := time.Parse(time.RFC3339, record.RecordedAt)
		if err == nil && recordedAt.After(latest) {
			latest = recordedAt
		}
	}
	return latest
}

func (s *Store) appendEvidence(item goal.GoalEvidence) error {
	if err := goal.ValidateEvidence(item); err != nil {
		return err
	}
	path := filepath.Join(s.goalDir(item.GoalID), "EVIDENCE.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create evidence parent: %w", err)
	}
	data, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("marshal evidence: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open evidence: %w", err)
	}
	defer file.Close()
	if _, err := file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("append evidence: %w", err)
	}
	return nil
}

func (s *Store) readEvidence(goalID string) ([]goal.GoalEvidence, error) {
	path := filepath.Join(s.goalDir(goalID), "EVIDENCE.jsonl")
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open evidence: %w", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	var records []goal.GoalEvidence
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var record goal.GoalEvidence
		if err := json.Unmarshal(line, &record); err != nil {
			return records, fmt.Errorf("decode evidence %s line %d: %w", path, lineNo, err)
		}
		if err := goal.ValidateEvidence(record); err != nil {
			return records, fmt.Errorf("validate evidence %s line %d: %w", path, lineNo, err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return records, fmt.Errorf("read evidence: %w", err)
	}
	return records, nil
}

func (s *Store) appendEvent(event schema.Event) error {
	store, err := eventlog.New(s.paths.Root)
	if err != nil {
		return err
	}
	return store.Append(event)
}

func (s *Store) writeCompletionAuditRecord(item goal.Goal, evidence []goal.GoalEvidence, event schema.Event, now time.Time) (map[string]any, error) {
	audits, err := auditstore.New(s.paths.Root)
	if err != nil {
		return nil, err
	}
	reportID := ""
	reportStatus := ""
	if item.Report != nil {
		reportID = item.Report.ID
		reportStatus = item.Report.Status
	}
	result, err := audits.Write(auditstore.WriteOptions{
		ID: "goal-" + item.ID + "-completion-" + layout.TimestampID(now),
		Labels: map[string]string{
			"audit_kind": "goal.completion",
			"goal_id":    item.ID,
		},
		Spec: map[string]any{
			"audit_kind":        "goal.completion",
			"goal_id":           item.ID,
			"status":            string(item.Status),
			"report_id":         reportID,
			"report_status":     reportStatus,
			"evidence_count":    len(evidence),
			"accepted_evidence": acceptedEvidenceIDs(evidence),
			"event_id":          event.ID,
		},
	})
	if err != nil {
		return nil, err
	}
	return result.Ref, nil
}

func (s *Store) appendCompletionAuditEvent(item goal.Goal, event schema.Event, auditRef map[string]any, now time.Time) error {
	audits, err := auditstore.New(s.paths.Root)
	if err != nil {
		return err
	}
	_, err = audits.AppendRecordedEvent(auditstore.RecordedEventOptions{
		ID:            eventID(item.ID, "audit.recorded.goal.completed", now),
		Now:           now,
		Loop:          "goal",
		Actor:         "mnemon-manual",
		Source:        "mnemon.goal",
		CorrelationID: item.ID,
		CausedBy:      event.ID,
		Payload: map[string]any{
			"audit_kind": "goal.completion",
			"goal_id":    item.ID,
			"event_id":   event.ID,
		},
		AuditRef: auditRef,
	})
	return err
}

func (s *Store) event(now time.Time, goalID, eventType string, host *string, payload map[string]any) schema.Event {
	loop := "goal"
	return schema.Event{
		SchemaVersion: 1,
		ID:            eventID(goalID, eventType, now),
		TS:            now.UTC().Format(time.RFC3339),
		Type:          eventType,
		Loop:          &loop,
		Host:          host,
		Actor:         "mnemon-manual",
		Source:        "mnemon.goal",
		CorrelationID: goalID,
		CausedBy:      nil,
		Payload:       payload,
	}
}

func (s *Store) goalDir(goalID string) string {
	return filepath.Join(s.paths.HarnessDir, "goals", cleanID(goalID))
}

func renderGoalMarkdown(item goal.Goal) string {
	var out strings.Builder
	fmt.Fprintf(&out, "# Mnemon Goal %s\n\n", item.ID)
	fmt.Fprintf(&out, "Status: `%s`\n\n", item.Status)
	fmt.Fprintf(&out, "Created: %s\n\n", item.CreatedAt)
	fmt.Fprintf(&out, "Updated: %s\n\n", item.UpdatedAt)
	fmt.Fprintf(&out, "## Objective\n\n%s\n", item.Objective)
	return out.String()
}

func renderPlanMarkdown(item goal.Goal) string {
	if item.Plan == nil {
		return "# Goal Plan\n\nNo plan recorded yet.\n"
	}
	plan := item.Plan
	var out strings.Builder
	fmt.Fprintln(&out, "# Goal Plan")
	if plan.Summary != "" {
		fmt.Fprintf(&out, "\n%s\n", plan.Summary)
	}
	if len(plan.Steps) > 0 {
		fmt.Fprintln(&out, "\n## Steps")
		for _, step := range plan.Steps {
			fmt.Fprintf(&out, "- %s\n", step)
		}
	}
	renderRefs := func(title string, refs []string) {
		if len(refs) == 0 {
			return
		}
		fmt.Fprintf(&out, "\n## %s\n", title)
		for _, ref := range refs {
			fmt.Fprintf(&out, "- `%s`\n", ref)
		}
	}
	renderRefs("Memory Refs", plan.MemoryRefs)
	renderRefs("Memory Recall Requests", plan.MemoryRecallRequests)
	renderRefs("Skill Workflow Refs", plan.SkillWorkflowRefs)
	renderRefs("Eval Refs", plan.EvalRefs)
	return out.String()
}

func renderReportMarkdown(item goal.Goal) string {
	if item.Report == nil {
		return "# Goal Report\n\nNo verification report recorded yet.\n"
	}
	report := item.Report
	var out strings.Builder
	fmt.Fprintln(&out, "# Goal Report")
	fmt.Fprintf(&out, "\nStatus: `%s`\n\n", report.Status)
	fmt.Fprintf(&out, "Verification gate: `%s` passed=%t\n\n", report.VerificationGate.Name, report.VerificationGate.Passed)
	fmt.Fprintf(&out, "%s\n", report.Summary)
	if len(report.EvidenceRefs) > 0 {
		fmt.Fprintln(&out, "\n## Evidence")
		for _, ref := range report.EvidenceRefs {
			fmt.Fprintf(&out, "- `%s`\n", ref)
		}
	}
	return out.String()
}

func goalStatusDocument(item goal.Goal) map[string]any {
	return map[string]any{
		"schema_version": 1,
		"kind":           "GoalStatus",
		"metadata": map[string]any{
			"name":    item.ID,
			"goal_id": item.ID,
		},
		"status": map[string]any{
			"phase":                  string(item.Status),
			"last_refreshed_at":      item.UpdatedAt,
			"last_included_event_id": item.LatestEventID,
			"evidence_count":         item.EvidenceCount,
			"report_status":          reportStatus(item.Report),
			"conditions": []schema.Condition{{
				Type:             conditionType(item.Status),
				Status:           "true",
				Reason:           conditionReason(item.Status),
				LastTransitionTS: item.UpdatedAt,
				LastEventID:      item.LatestEventID,
			}},
		},
	}
}

func reportStatus(report *goal.GoalReport) string {
	if report == nil {
		return "missing"
	}
	return report.Status
}

func conditionType(status goal.Status) string {
	switch status {
	case goal.StatusBlocked:
		return "Blocked"
	case goal.StatusPaused:
		return "Paused"
	case goal.StatusComplete:
		return "Complete"
	default:
		return "Ready"
	}
}

func conditionReason(status goal.Status) string {
	switch status {
	case goal.StatusDraft:
		return "GoalCreated"
	case goal.StatusPlanned:
		return "GoalPlanned"
	case goal.StatusActive:
		return "GoalActive"
	case goal.StatusVerifying:
		return "GoalVerified"
	case goal.StatusComplete:
		return "GoalCompleted"
	case goal.StatusBlocked:
		return "GoalBlocked"
	case goal.StatusPaused:
		return "GoalPaused"
	default:
		return "GoalStatus"
	}
}

func mergeEvidenceRefs(report *goal.GoalReport, records []goal.GoalEvidence) {
	add := func(items []string, item string) []string {
		if item == "" {
			return items
		}
		for _, existing := range items {
			if existing == item {
				return items
			}
		}
		return append(items, item)
	}
	for _, record := range records {
		if record.Status != "accepted" {
			continue
		}
		for _, ref := range record.Refs.EvalReportRefs {
			report.EvalReportRefs = add(report.EvalReportRefs, ref)
		}
		for _, ref := range record.Refs.ArtifactRefs {
			report.ArtifactRefs = add(report.ArtifactRefs, ref)
		}
		for _, ref := range record.Refs.AuditRefs {
			report.AuditRefs = add(report.AuditRefs, ref)
		}
		for _, ref := range record.Refs.ProposalRefs {
			report.ProposalRefs = add(report.ProposalRefs, ref)
		}
	}
}

func acceptedEvidenceIDs(records []goal.GoalEvidence) []string {
	var ids []string
	for _, record := range records {
		if record.Status == "accepted" {
			ids = append(ids, record.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

var nonID = regexp.MustCompile(`[^a-z0-9._-]+`)

func cleanID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "-")
	value = nonID.ReplaceAllString(value, "-")
	value = strings.Trim(value, ".-_")
	return value
}

func generatedGoalID(objective string, now time.Time) string {
	words := strings.Fields(strings.ToLower(objective))
	limit := 4
	if len(words) < limit {
		limit = len(words)
	}
	slug := cleanID(strings.Join(words[:limit], "-"))
	if slug == "" {
		slug = "goal"
	}
	return fmt.Sprintf("%s-%s", slug, now.UTC().Format("20060102T150405"))
}

func eventID(goalID, eventType string, now time.Time) string {
	cleanType := strings.ReplaceAll(eventType, ".", "_")
	return fmt.Sprintf("evt_goal_%s_%s_%s", cleanID(goalID), cleanID(cleanType), layout.TimestampID(now))
}

func trimList(values []string) []string {
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func writeJSONAtomic(path string, value any) error {
	return layout.WriteJSONAtomic(path, value, 0o600)
}

func writeTextAtomic(path string, text string) error {
	return writeBytesAtomic(path, []byte(text))
}

func writeBytesAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create parent for %s: %w", path, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp for %s: %w", path, err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write temp %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close temp %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}
