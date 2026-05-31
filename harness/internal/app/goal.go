package app

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/eventlog"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/goal"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/goalstore"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
)

// Facade-local types for the goal domain. Surfaces consume these instead of the
// goal/goalstore packages.

type GoalRef struct {
	ID   string
	Path string
}

type GoalState struct {
	ID     string
	Status string
}

type GoalStatusView struct {
	ID            string
	Status        string
	ReportStatus  string
	EvidenceCount int
	Ready         bool
	Path          string
}

type GoalVerifyResult struct {
	GoalID     string
	Status     string
	GateName   string
	GatePassed bool
}

type GoalNudgeResult struct {
	GoalID  string
	Reason  string
	Path    string
	Skipped bool
}

type GoalLink struct {
	GoalID     string
	Host       string
	ThreadID   string
	HostGoalID string
}

// EvidenceRefs is the facade-side mirror of the goal evidence reference bundle.
type EvidenceRefs struct {
	MemoryRefs       []string
	MemoryRequests   []string
	SkillSignals     []string
	EvalReportRefs   []string
	ArtifactRefs     []string
	AuditRefs        []string
	ProposalRefs     []string
	HostEvidenceRefs []string
}

func (h *Harness) GoalInit(id, objective string) (GoalRef, error) {
	store, err := goalstore.New(h.root)
	if err != nil {
		return GoalRef{}, err
	}
	item, err := store.Create(goalstore.CreateOptions{ID: id, Objective: objective})
	if err != nil {
		return GoalRef{}, err
	}
	return GoalRef{ID: item.ID, Path: store.GoalPath(item.ID)}, nil
}

func (h *Harness) GoalPlan(id, summary string, steps, memoryRefs, memoryRecall, skillRefs, evalRefs []string) (GoalState, error) {
	store, err := goalstore.New(h.root)
	if err != nil {
		return GoalState{}, err
	}
	item, err := store.Plan(goalstore.PlanOptions{
		GoalID:               id,
		Summary:              summary,
		Steps:                steps,
		MemoryRefs:           memoryRefs,
		MemoryRecallRequests: memoryRecall,
		SkillWorkflowRefs:    skillRefs,
		EvalRefs:             evalRefs,
	})
	if err != nil {
		return GoalState{}, err
	}
	return GoalState{ID: item.ID, Status: string(item.Status)}, nil
}

func (h *Harness) GoalStatus(id string) (GoalStatusView, error) {
	store, err := goalstore.New(h.root)
	if err != nil {
		return GoalStatusView{}, err
	}
	view, err := store.Status(id)
	if err != nil {
		return GoalStatusView{}, err
	}
	reportStatus := "missing"
	if view.Goal.Report != nil {
		reportStatus = view.Goal.Report.Status
	}
	return GoalStatusView{
		ID:            view.Goal.ID,
		Status:        string(view.Goal.Status),
		ReportStatus:  reportStatus,
		EvidenceCount: len(view.Evidence),
		Ready:         view.Ready,
		Path:          view.Path,
	}, nil
}

func (h *Harness) GoalEvidenceAppend(id, evidenceID, etype, status, summary string, refs EvidenceRefs) (string, error) {
	store, err := goalstore.New(h.root)
	if err != nil {
		return "", err
	}
	evidence, err := store.AppendEvidence(goalstore.EvidenceOptions{
		GoalID:  id,
		ID:      evidenceID,
		Type:    etype,
		Status:  status,
		Summary: summary,
		Refs: goal.EvidenceRefs{
			MemoryRefs:       refs.MemoryRefs,
			MemoryRequests:   refs.MemoryRequests,
			SkillSignals:     refs.SkillSignals,
			EvalReportRefs:   refs.EvalReportRefs,
			ArtifactRefs:     refs.ArtifactRefs,
			AuditRefs:        refs.AuditRefs,
			ProposalRefs:     refs.ProposalRefs,
			HostEvidenceRefs: refs.HostEvidenceRefs,
		},
	})
	if err != nil {
		return "", err
	}
	return evidence.ID, nil
}

func (h *Harness) GoalVerify(id, gate, summary string) (GoalVerifyResult, error) {
	store, err := goalstore.New(h.root)
	if err != nil {
		return GoalVerifyResult{}, err
	}
	report, err := store.Verify(goalstore.VerifyOptions{GoalID: id, GateName: gate, Summary: summary})
	if err != nil {
		return GoalVerifyResult{}, err
	}
	return GoalVerifyResult{
		GoalID:     report.GoalID,
		Status:     string(report.Status),
		GateName:   report.VerificationGate.Name,
		GatePassed: report.VerificationGate.Passed,
	}, nil
}

// GoalComplete completes a verified goal and, on success, appends the
// goal.completed event (cross-ring composition: store + event log). It wraps the
// not-verified sentinel with the original CLI guidance so the surface stays thin.
func (h *Harness) GoalComplete(id string, blockOnFailure bool) (string, error) {
	store, err := goalstore.New(h.root)
	if err != nil {
		return "", err
	}
	item, err := store.Complete(goalstore.CompleteOptions{GoalID: id, BlockOnFailure: blockOnFailure})
	if err != nil {
		if errors.Is(err, goalstore.ErrCompletionNotVerified) {
			return "", fmt.Errorf("%w; run mnemon-harness goal evidence append and mnemon-harness goal verify first", err)
		}
		return "", err
	}
	_ = h.appendGoalCompletedEvent(item.ID)
	return item.ID, nil
}

func (h *Harness) appendGoalCompletedEvent(goalID string) error {
	store, err := eventlog.New(h.root)
	if err != nil {
		return err
	}
	loop := "goal"
	now := time.Now().UTC()
	return store.Append(schema.Event{
		SchemaVersion: schema.Version,
		ID:            "evt_goal_completed_" + strings.ReplaceAll(goalID, "-", "_") + "_" + now.Format("20060102T150405.000000000"),
		TS:            now.Format(time.RFC3339),
		Type:          "goal.completed",
		Loop:          &loop,
		Actor:         "mnemon-manual",
		Source:        "mnemon.goal",
		CorrelationID: goalID,
		CausedBy:      nil,
		Payload: map[string]any{
			"goal_id": goalID,
		},
	})
}

// GoalTransition applies a block/pause/resume lifecycle action and returns the
// goal id. The surface supplies the past-tense verb for output.
func (h *Harness) GoalTransition(action, id, reason string) (string, error) {
	store, err := goalstore.New(h.root)
	if err != nil {
		return "", err
	}
	switch action {
	case "block":
		item, err := store.Block(goalstore.BlockOptions{GoalID: id, Reason: reason})
		if err != nil {
			return "", err
		}
		return item.ID, nil
	case "pause":
		item, err := store.Pause(goalstore.PauseOptions{GoalID: id, Reason: reason})
		if err != nil {
			return "", err
		}
		return item.ID, nil
	case "resume":
		item, err := store.Resume(goalstore.ResumeOptions{GoalID: id, Reason: reason})
		if err != nil {
			return "", err
		}
		return item.ID, nil
	default:
		return "", fmt.Errorf("unknown goal transition %q", action)
	}
}

func (h *Harness) GoalNudge(id string, allIdle bool, idleAfter time.Duration, summary string) ([]GoalNudgeResult, error) {
	store, err := goalstore.New(h.root)
	if err != nil {
		return nil, err
	}
	results, err := store.Nudge(goalstore.NudgeOptions{
		GoalID:    id,
		AllIdle:   allIdle,
		IdleAfter: idleAfter,
		Summary:   summary,
		Now:       time.Now().UTC(),
	})
	if err != nil {
		return nil, err
	}
	out := make([]GoalNudgeResult, 0, len(results))
	for _, r := range results {
		out = append(out, GoalNudgeResult{GoalID: r.GoalID, Reason: r.Reason, Path: r.Path, Skipped: r.Skipped})
	}
	return out, nil
}

func (h *Harness) GoalLink(id, host, threadID, hostGoalID, objective string, evidence []string) (GoalLink, error) {
	store, err := goalstore.New(h.root)
	if err != nil {
		return GoalLink{}, err
	}
	link, err := store.Link(goalstore.LinkOptions{
		GoalID:     id,
		Host:       host,
		ThreadID:   threadID,
		HostGoalID: hostGoalID,
		Objective:  objective,
		Evidence:   evidence,
	})
	if err != nil {
		return GoalLink{}, err
	}
	return GoalLink{GoalID: link.GoalID, Host: link.Host, ThreadID: link.ThreadID, HostGoalID: link.HostGoalID}, nil
}

func (h *Harness) GoalCodexPrompt(id string) (string, error) {
	store, err := goalstore.New(h.root)
	if err != nil {
		return "", err
	}
	view, err := store.Status(id)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(goalstore.CodexPrompt(view.Goal), "\n"), nil
}
