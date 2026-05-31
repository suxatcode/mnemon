package goalstore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/eventlog"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/goal"
)

func TestStoreGoalLifecycle(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	now := time.Date(2026, 5, 24, 10, 0, 0, 0, time.UTC)
	item, err := store.Create(CreateOptions{
		ID:        "goal-mvp",
		Objective: "Implement the goal loop MVP.",
		Now:       now,
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	assertGoalFile(t, root, item.ID, "goal.json")
	assertGoalFile(t, root, item.ID, "GOAL.md")
	assertGoalFile(t, root, item.ID, "PLAN.md")
	assertGoalFile(t, root, item.ID, "EVIDENCE.jsonl")
	assertGoalFile(t, root, item.ID, "REPORT.md")

	item, err = store.Plan(PlanOptions{
		GoalID:               item.ID,
		Summary:              "Build the state model and CLI.",
		Steps:                []string{"model", "store", "cli"},
		MemoryRefs:           []string{"memory:goal-loop"},
		MemoryRecallRequests: []string{"recall prior goal state"},
		SkillWorkflowRefs:    []string{"skill:goal-verify"},
		EvalRefs:             []string{"eval:goal-smoke"},
		Now:                  now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}
	if item.Status != goal.StatusPlanned {
		t.Fatalf("expected planned status, got %s", item.Status)
	}

	evidence, err := store.AppendEvidence(EvidenceOptions{
		GoalID:  item.ID,
		ID:      "evidence-cli-smoke",
		Type:    "eval",
		Summary: "CLI smoke passed.",
		Refs: goal.EvidenceRefs{
			EvalReportRefs: []string{"eval-report:goal-smoke"},
			ArtifactRefs:   []string{".mnemon/harness/reports/goal-smoke.json"},
			AuditRefs:      []string{"audit:goal-smoke"},
			ProposalRefs:   []string{"proposal:noop"},
			SkillSignals:   []string{"skill:goal-verify"},
			MemoryRefs:     []string{"memory:goal-loop"},
		},
		Now: now.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("AppendEvidence returned error: %v", err)
	}
	if evidence.Status != "accepted" {
		t.Fatalf("expected accepted evidence, got %s", evidence.Status)
	}

	report, err := store.Verify(VerifyOptions{
		GoalID: item.ID,
		Now:    now.Add(3 * time.Minute),
	})
	if err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}
	if report.Status != "pass" {
		t.Fatalf("expected passing report, got %s", report.Status)
	}

	item, err = store.Complete(CompleteOptions{
		GoalID: item.ID,
		Now:    now.Add(4 * time.Minute),
	})
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	if item.Status != goal.StatusComplete {
		t.Fatalf("expected complete status, got %s", item.Status)
	}

	view, err := store.Status(item.ID)
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if !view.Ready {
		t.Fatal("expected status view to be completion-ready")
	}
	if _, err := os.Stat(filepath.Join(root, ".mnemon", "harness", "status", "goals", item.ID+".json")); err != nil {
		t.Fatalf("expected goal status file: %v", err)
	}
	auditRecords, err := os.ReadDir(filepath.Join(root, ".mnemon", "harness", "audit", "records"))
	if err != nil {
		t.Fatalf("expected audit records: %v", err)
	}
	if len(auditRecords) != 1 {
		t.Fatalf("expected 1 completion audit record, got %d", len(auditRecords))
	}

	events := readEvents(t, root)
	wantTypes := []string{
		"goal.created",
		"goal.planned",
		"goal.evidence_recorded",
		"goal.verified",
		"goal.completed",
		"audit.recorded",
	}
	if len(events) != len(wantTypes) {
		t.Fatalf("expected %d events, got %d", len(wantTypes), len(events))
	}
	for i, want := range wantTypes {
		if events[i].Type != want {
			t.Fatalf("event %d: want %s, got %s", i, want, events[i].Type)
		}
	}
}

func TestVerifyEvalPassedGateRequiresReadyEvalReport(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	now := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	readyRef := ".mnemon/harness/reports/runner/ready.json"
	blockedRef := ".mnemon/harness/reports/runner/blocked.json"
	writeEvalReport(t, root, readyRef, "ready", 1)
	writeEvalReport(t, root, blockedRef, "blocked", 0)

	readyGoal, err := store.Create(CreateOptions{
		ID:        "goal-eval-ready",
		Objective: "Verify with ready eval report.",
		Now:       now,
	})
	if err != nil {
		t.Fatalf("Create ready goal returned error: %v", err)
	}
	if _, err := store.Plan(PlanOptions{
		GoalID:  readyGoal.ID,
		Summary: "Ready eval gate.",
		Steps:   []string{"attach ready eval report"},
		Now:     now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("Plan ready goal returned error: %v", err)
	}
	if _, err := store.AppendEvidence(EvidenceOptions{
		GoalID:  readyGoal.ID,
		Type:    "eval",
		Status:  "accepted",
		Summary: "Ready eval report.",
		Refs: goal.EvidenceRefs{
			EvalReportRefs: []string{readyRef},
		},
		Now: now.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("AppendEvidence ready returned error: %v", err)
	}
	readyReport, err := store.Verify(VerifyOptions{
		GoalID:   readyGoal.ID,
		GateName: "eval-passed",
		Now:      now.Add(3 * time.Minute),
	})
	if err != nil {
		t.Fatalf("Verify ready returned error: %v", err)
	}
	if readyReport.Status != "pass" || !readyReport.VerificationGate.Passed {
		t.Fatalf("expected ready eval report to pass, got %#v", readyReport)
	}

	blockedGoal, err := store.Create(CreateOptions{
		ID:        "goal-eval-blocked",
		Objective: "Verify with blocked eval report.",
		Now:       now.Add(4 * time.Minute),
	})
	if err != nil {
		t.Fatalf("Create blocked goal returned error: %v", err)
	}
	if _, err := store.Plan(PlanOptions{
		GoalID:  blockedGoal.ID,
		Summary: "Blocked eval gate.",
		Steps:   []string{"attach blocked eval report"},
		Now:     now.Add(5 * time.Minute),
	}); err != nil {
		t.Fatalf("Plan blocked goal returned error: %v", err)
	}
	if _, err := store.AppendEvidence(EvidenceOptions{
		GoalID:  blockedGoal.ID,
		Type:    "eval",
		Status:  "accepted",
		Summary: "Blocked eval report.",
		Refs: goal.EvidenceRefs{
			EvalReportRefs: []string{blockedRef},
		},
		Now: now.Add(6 * time.Minute),
	}); err != nil {
		t.Fatalf("AppendEvidence blocked returned error: %v", err)
	}
	blockedReport, err := store.Verify(VerifyOptions{
		GoalID:   blockedGoal.ID,
		GateName: "eval-passed",
		Summary:  "This should be replaced by the gate failure.",
		Now:      now.Add(7 * time.Minute),
	})
	if err != nil {
		t.Fatalf("Verify blocked returned error: %v", err)
	}
	if blockedReport.Status != "blocked" || blockedReport.VerificationGate.Passed {
		t.Fatalf("expected blocked eval report to block, got %#v", blockedReport)
	}
	if !strings.Contains(blockedReport.Summary, `status "blocked"`) {
		t.Fatalf("blocked summary did not explain eval status: %s", blockedReport.Summary)
	}
}

func TestCompleteWithoutEvidenceFails(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	item, err := store.Create(CreateOptions{
		ID:        "goal-no-evidence",
		Objective: "Prove completion gating.",
		Now:       time.Date(2026, 5, 24, 11, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if _, err := store.Complete(CompleteOptions{GoalID: item.ID}); !errors.Is(err, ErrCompletionNotVerified) {
		t.Fatalf("expected ErrCompletionNotVerified, got %v", err)
	}
	view, err := store.Status(item.ID)
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if view.Goal.Status == goal.StatusComplete {
		t.Fatal("goal completed without evidence")
	}
}

func TestAppendEvidenceAllowsSameTimestampWithDifferentEvidenceIDs(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	now := time.Date(2026, 5, 27, 15, 48, 0, 0, time.UTC)
	item, err := store.Create(CreateOptions{
		ID:        "dogfood-s1-2",
		Objective: "Phase 1 dogfood goal cycle smoke",
		Now:       now,
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if _, err := store.Plan(PlanOptions{
		GoalID:  item.ID,
		Summary: "Smoke plan",
		Steps:   []string{"audit", "implement", "verify"},
		Now:     now,
	}); err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}
	for _, evidenceID := range []string{"s1-2-ev-1", "s1-2-ev-2"} {
		if _, err := store.AppendEvidence(EvidenceOptions{
			GoalID:  item.ID,
			ID:      evidenceID,
			Type:    "manual",
			Status:  "accepted",
			Summary: "Smoke evidence " + evidenceID,
			Now:     now,
		}); err != nil {
			t.Fatalf("AppendEvidence(%s) returned error: %v", evidenceID, err)
		}
	}

	events := readEvents(t, root)
	var evidenceEventIDs []string
	for _, event := range events {
		if event.Type == "goal.evidence_recorded" {
			evidenceEventIDs = append(evidenceEventIDs, event.ID)
		}
	}
	if len(evidenceEventIDs) != 2 {
		t.Fatalf("expected 2 evidence events, got %d: %#v", len(evidenceEventIDs), evidenceEventIDs)
	}
	if evidenceEventIDs[0] == evidenceEventIDs[1] {
		t.Fatalf("evidence event ids collided: %#v", evidenceEventIDs)
	}
}

func TestSourceStateGuards(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	now := time.Date(2026, 5, 24, 11, 30, 0, 0, time.UTC)
	item, err := store.Create(CreateOptions{
		ID:        "goal-guards",
		Objective: "Enforce goal source states.",
		Now:       now,
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if _, err := store.Verify(VerifyOptions{GoalID: item.ID, Now: now.Add(time.Minute)}); !isTransitionError(err) {
		t.Fatalf("expected draft verify transition error, got %v", err)
	}
	if _, err := store.Resume(ResumeOptions{GoalID: item.ID, Now: now.Add(2 * time.Minute)}); !isTransitionError(err) {
		t.Fatalf("expected draft resume transition error, got %v", err)
	}
	item, err = store.Plan(PlanOptions{
		GoalID:  item.ID,
		Summary: "Plan the guarded flow.",
		Now:     now.Add(3 * time.Minute),
	})
	if err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}
	if item.Status != goal.StatusPlanned {
		t.Fatalf("expected planned status, got %s", item.Status)
	}
	if _, err := store.AppendEvidence(EvidenceOptions{
		GoalID:  item.ID,
		ID:      "evidence-guard",
		Type:    "manual",
		Summary: "Guarded flow evidence.",
		Now:     now.Add(4 * time.Minute),
	}); err != nil {
		t.Fatalf("AppendEvidence returned error: %v", err)
	}
	if _, err := store.Verify(VerifyOptions{GoalID: item.ID, Now: now.Add(5 * time.Minute)}); err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}
	if _, err := store.Pause(PauseOptions{GoalID: item.ID, Now: now.Add(6 * time.Minute)}); err != nil {
		t.Fatalf("Pause returned error: %v", err)
	}
	if _, err := store.Complete(CompleteOptions{GoalID: item.ID, Now: now.Add(7 * time.Minute)}); !isTransitionError(err) {
		t.Fatalf("expected paused complete transition error, got %v", err)
	}
	if _, err := store.Resume(ResumeOptions{GoalID: item.ID, Now: now.Add(8 * time.Minute)}); err != nil {
		t.Fatalf("Resume returned error: %v", err)
	}
	if _, err := store.Complete(CompleteOptions{GoalID: item.ID, Now: now.Add(9 * time.Minute)}); !isTransitionError(err) {
		t.Fatalf("expected active complete transition error, got %v", err)
	}
	if _, err := store.Verify(VerifyOptions{GoalID: item.ID, Now: now.Add(10 * time.Minute)}); err != nil {
		t.Fatalf("Verify after resume returned error: %v", err)
	}
	item, err = store.Complete(CompleteOptions{GoalID: item.ID, Now: now.Add(11 * time.Minute)})
	if err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}
	if item.Status != goal.StatusComplete {
		t.Fatalf("expected complete status, got %s", item.Status)
	}
	if _, err := store.Block(BlockOptions{GoalID: item.ID, Now: now.Add(12 * time.Minute)}); !isTransitionError(err) {
		t.Fatalf("expected complete block transition error, got %v", err)
	}
	if _, err := store.Plan(PlanOptions{GoalID: item.ID, Summary: "too late", Now: now.Add(13 * time.Minute)}); !isTransitionError(err) {
		t.Fatalf("expected complete plan transition error, got %v", err)
	}
}

func TestLinkPauseResumeAndBlockEvents(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	item, err := store.Create(CreateOptions{
		ID:        "goal-links",
		Objective: "Link host goal state.",
		Now:       now,
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if item, err = store.Activate(item.ID, now.Add(30*time.Second)); err != nil {
		t.Fatalf("Activate returned error: %v", err)
	}
	if item.Status != goal.StatusActive {
		t.Fatalf("expected active status, got %s", item.Status)
	}
	link, err := store.Link(LinkOptions{
		GoalID:   item.ID,
		Host:     "codex",
		ThreadID: "thr_123",
		Now:      now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("Link returned error: %v", err)
	}
	if link.Objective != CodexObjective(item.ID) {
		t.Fatalf("unexpected objective: %q", link.Objective)
	}
	if _, err := store.Pause(PauseOptions{GoalID: item.ID, Reason: "waiting", Now: now.Add(2 * time.Minute)}); err != nil {
		t.Fatalf("Pause returned error: %v", err)
	}
	if item, err = store.Resume(ResumeOptions{GoalID: item.ID, Reason: "continue", Now: now.Add(3 * time.Minute)}); err != nil {
		t.Fatalf("Resume returned error: %v", err)
	}
	if item.Status != goal.StatusActive {
		t.Fatalf("expected active after resume, got %s", item.Status)
	}
	if item, err = store.Block(BlockOptions{GoalID: item.ID, Reason: "blocked", Now: now.Add(4 * time.Minute)}); err != nil {
		t.Fatalf("Block returned error: %v", err)
	}
	if item.Status != goal.StatusBlocked {
		t.Fatalf("expected blocked status, got %s", item.Status)
	}
	events := readEvents(t, root)
	want := map[string]bool{
		"goal.created":     false,
		"goal.activated":   false,
		"goal.host_linked": false,
		"goal.paused":      false,
		"goal.resumed":     false,
		"goal.blocked":     false,
	}
	for _, event := range events {
		if _, ok := want[event.Type]; ok {
			want[event.Type] = true
		}
	}
	for typ, seen := range want {
		if !seen {
			t.Fatalf("missing event type %s in %#v", typ, events)
		}
	}
}

func TestNudgeWritesIdleGoalNudge(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	item, err := store.Create(CreateOptions{
		ID:        "goal-idle",
		Objective: "Keep idle goal visible.",
		Now:       now,
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if _, err := store.Plan(PlanOptions{
		GoalID:  item.ID,
		Summary: "Wait for daemon nudge.",
		Now:     now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("Plan returned error: %v", err)
	}

	results, err := store.Nudge(NudgeOptions{
		AllIdle:   true,
		IdleAfter: 6 * time.Hour,
		Summary:   "Review idle goal.",
		Now:       now.Add(7 * time.Hour),
	})
	if err != nil {
		t.Fatalf("Nudge returned error: %v", err)
	}
	if len(results) != 1 || results[0].Skipped || results[0].NudgeID == "" {
		t.Fatalf("unexpected nudge result: %#v", results)
	}
	data, err := os.ReadFile(filepath.Join(root, ".mnemon", "harness", "goals", item.ID, "nudges.md"))
	if err != nil {
		t.Fatalf("read nudges.md: %v", err)
	}
	if !strings.Contains(string(data), "Review idle goal.") {
		t.Fatalf("unexpected nudge log: %s", string(data))
	}
	events := readEvents(t, root)
	if events[len(events)-1].Type != "goal.nudged" {
		t.Fatalf("expected goal.nudged event, got %#v", events)
	}
}

func assertGoalFile(t *testing.T, root, goalID, name string) {
	t.Helper()
	path := filepath.Join(root, ".mnemon", "harness", "goals", goalID, name)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s: %v", path, err)
	}
}

func writeEvalReport(t *testing.T, root, ref, status string, usedTurns int) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(ref))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir eval report dir: %v", err)
	}
	content := fmt.Sprintf(`{"status":%q,"budget":{"used_turns":%d}}`+"\n", status, usedTurns)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write eval report %s: %v", ref, err)
	}
}

func readEvents(t *testing.T, root string) []eventType {
	t.Helper()
	store, err := eventlog.New(root)
	if err != nil {
		t.Fatalf("eventlog.New returned error: %v", err)
	}
	events, err := store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	out := make([]eventType, 0, len(events))
	for _, event := range events {
		out = append(out, eventType{
			ID:   event.ID,
			Type: event.Type,
		})
	}
	return out
}

type eventType struct {
	ID   string
	Type string
}

func isTransitionError(err error) bool {
	var transitionErr goal.TransitionError
	return errors.As(err, &transitionErr)
}
