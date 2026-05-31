package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/eventlog"
	"github.com/mnemon-dev/mnemon/harness/internal/lifecycle/schema"
	"github.com/spf13/cobra"
)

func TestGoalCommandSmoke(t *testing.T) {
	root := t.TempDir()
	restoreGoalFlags(t)
	goalRoot = root
	goalID = "goal-cli-smoke"
	goalObjective = "Implement a CLI smoke for Mnemon Goal Loop."

	initCmd, initOutput := testCommand()
	if err := runGoalInit(initCmd, nil); err != nil {
		t.Fatalf("runGoalInit returned error: %v", err)
	}
	if !strings.Contains(initOutput.String(), "goal-cli-smoke") {
		t.Fatalf("init output did not mention goal id: %s", initOutput.String())
	}
	for _, name := range []string{"goal.json", "GOAL.md", "PLAN.md", "EVIDENCE.jsonl", "REPORT.md"} {
		if _, err := os.Stat(filepath.Join(root, ".mnemon", "harness", "goals", "goal-cli-smoke", name)); err != nil {
			t.Fatalf("expected %s: %v", name, err)
		}
	}

	goalPlanSummary = "Exercise goal commands."
	goalPlanSteps = []string{"init", "plan", "evidence", "verify", "complete"}
	goalMemoryRefs = []string{"memory:cli-smoke"}
	goalMemoryRecallRequests = []string{"recall lifecycle goal docs"}
	goalSkillWorkflowRefs = []string{"skill:goal-cli"}
	goalEvalRefs = []string{"eval:goal-cli-smoke"}
	planCmd, _ := testCommand()
	if err := runGoalPlan(planCmd, nil); err != nil {
		t.Fatalf("runGoalPlan returned error: %v", err)
	}

	statusCmd, statusOutput := testCommand()
	if err := runGoalStatus(statusCmd, nil); err != nil {
		t.Fatalf("runGoalStatus returned error: %v", err)
	}
	if !strings.Contains(statusOutput.String(), "goal goal-cli-smoke: planned") {
		t.Fatalf("unexpected status output: %s", statusOutput.String())
	}

	goalEvidenceID = "evidence-cli"
	goalEvidenceType = "eval"
	goalEvidenceStatus = "accepted"
	goalEvidenceSummary = "Goal CLI smoke evidence."
	goalEvidenceEvalReports = []string{"eval-report:goal-cli"}
	goalEvidenceArtifactRefs = []string{".mnemon/harness/reports/goal-cli.json"}
	goalEvidenceAuditRefs = []string{"audit:goal-cli"}
	goalEvidenceProposalRefs = []string{"proposal:goal-cli-noop"}
	goalEvidenceSkillSignals = []string{"skill:goal-cli"}
	goalEvidenceMemoryRefs = []string{"memory:cli-smoke"}
	evidenceCmd, evidenceOutput := testCommand()
	if err := runGoalEvidenceAppend(evidenceCmd, nil); err != nil {
		t.Fatalf("runGoalEvidenceAppend returned error: %v", err)
	}
	if !strings.Contains(evidenceOutput.String(), "evidence-cli") {
		t.Fatalf("unexpected evidence output: %s", evidenceOutput.String())
	}

	verifyCmd, verifyOutput := testCommand()
	if err := runGoalVerify(verifyCmd, nil); err != nil {
		t.Fatalf("runGoalVerify returned error: %v", err)
	}
	if !strings.Contains(verifyOutput.String(), "pass") {
		t.Fatalf("unexpected verify output: %s", verifyOutput.String())
	}

	completeCmd, completeOutput := testCommand()
	if err := runGoalComplete(completeCmd, nil); err != nil {
		t.Fatalf("runGoalComplete returned error: %v", err)
	}
	if !strings.Contains(completeOutput.String(), "completed goal goal-cli-smoke") {
		t.Fatalf("unexpected complete output: %s", completeOutput.String())
	}

	codexCmd, codexOutput := testCommand()
	if err := runGoalCodexPrompt(codexCmd, nil); err != nil {
		t.Fatalf("runGoalCodexPrompt returned error: %v", err)
	}
	if !strings.Contains(codexOutput.String(), "/goal Follow .mnemon/harness/goals/goal-cli-smoke/GOAL.md") {
		t.Fatalf("codex prompt did not include concise objective: %s", codexOutput.String())
	}
	if strings.Contains(codexOutput.String(), "goals_1.sqlite") {
		t.Fatalf("codex prompt referenced internal sqlite: %s", codexOutput.String())
	}

	types := eventTypes(t, root)
	for _, want := range []string{"goal.created", "goal.planned", "goal.evidence_recorded", "goal.verified", "goal.completed"} {
		if !types[want] {
			t.Fatalf("missing event type %s", want)
		}
	}
	if count := eventTypeCount(t, root, "goal.completed"); count < 2 {
		t.Fatalf("expected canonical completion plus daemon signal, got %d goal.completed events", count)
	}
}

func TestGoalBlockPauseResumeAndLinkCommands(t *testing.T) {
	root := t.TempDir()
	restoreGoalFlags(t)
	goalRoot = root
	goalID = "goal-host-link"
	goalObjective = "Link and block a host goal."
	if err := runGoalInit(mustTestCommand(t), nil); err != nil {
		t.Fatalf("runGoalInit returned error: %v", err)
	}

	goalLinkHost = "codex"
	goalLinkThreadID = "thr_goal_cli"
	goalLinkEvidence = []string{"event:thread-goal-updated"}
	linkCmd, linkOutput := testCommand()
	if err := runGoalLink(linkCmd, nil); err != nil {
		t.Fatalf("runGoalLink returned error: %v", err)
	}
	if !strings.Contains(linkOutput.String(), "thread_id: thr_goal_cli") {
		t.Fatalf("unexpected link output: %s", linkOutput.String())
	}

	goalPauseReason = "waiting for external dependency"
	if err := runGoalPause(mustTestCommand(t), nil); err != nil {
		t.Fatalf("runGoalPause returned error: %v", err)
	}
	goalResumeReason = "dependency ready"
	if err := runGoalResume(mustTestCommand(t), nil); err != nil {
		t.Fatalf("runGoalResume returned error: %v", err)
	}
	goalBlockReason = "blocked by acceptance evidence"
	blockCmd, blockOutput := testCommand()
	if err := runGoalBlock(blockCmd, nil); err != nil {
		t.Fatalf("runGoalBlock returned error: %v", err)
	}
	if !strings.Contains(blockOutput.String(), "blocked goal goal-host-link") {
		t.Fatalf("unexpected block output: %s", blockOutput.String())
	}

	types := eventTypes(t, root)
	for _, want := range []string{"goal.host_linked", "goal.paused", "goal.resumed", "goal.blocked"} {
		if !types[want] {
			t.Fatalf("missing event type %s", want)
		}
	}
}

func TestGoalNudgeCommand(t *testing.T) {
	root := t.TempDir()
	restoreGoalFlags(t)
	goalRoot = root
	goalID = "goal-nudge-cli"
	goalObjective = "Exercise goal nudge command."
	if err := runGoalInit(mustTestCommand(t), nil); err != nil {
		t.Fatalf("runGoalInit returned error: %v", err)
	}
	goalPlanSummary = "Create an idle planned goal."
	if err := runGoalPlan(mustTestCommand(t), nil); err != nil {
		t.Fatalf("runGoalPlan returned error: %v", err)
	}

	goalID = ""
	goalNudgeAllIdle = true
	goalNudgeIdleAfter = 0
	goalNudgeSummary = "CLI nudge smoke."
	nudgeCmd, nudgeOutput := testCommand()
	if err := runGoalNudge(nudgeCmd, nil); err != nil {
		t.Fatalf("runGoalNudge returned error: %v", err)
	}
	if !strings.Contains(nudgeOutput.String(), "nudged 1 goals") {
		t.Fatalf("unexpected nudge output: %s", nudgeOutput.String())
	}
	if _, err := os.Stat(filepath.Join(root, ".mnemon", "harness", "goals", "goal-nudge-cli", "nudges.md")); err != nil {
		t.Fatalf("expected nudges.md: %v", err)
	}
}

func TestGoalCompleteWithoutEvidenceFails(t *testing.T) {
	root := t.TempDir()
	restoreGoalFlags(t)
	goalRoot = root
	goalID = "goal-no-evidence"
	goalObjective = "Completion should require evidence."
	if err := runGoalInit(mustTestCommand(t), nil); err != nil {
		t.Fatalf("runGoalInit returned error: %v", err)
	}
	err := runGoalComplete(mustTestCommand(t), nil)
	if err == nil || !strings.Contains(err.Error(), "completion requires accepted evidence") {
		t.Fatalf("expected completion gate error, got %v", err)
	}
}

func mustTestCommand(t *testing.T) *cobra.Command {
	t.Helper()
	cmd, _ := testCommand()
	return cmd
}

func eventTypes(t *testing.T, root string) map[string]bool {
	t.Helper()
	events := readGoalEvents(t, root)
	types := map[string]bool{}
	for _, event := range events {
		types[event.Type] = true
	}
	return types
}

func eventTypeCount(t *testing.T, root, eventType string) int {
	t.Helper()
	events := readGoalEvents(t, root)
	count := 0
	for _, event := range events {
		if event.Type == eventType {
			count++
		}
	}
	return count
}

func readGoalEvents(t *testing.T, root string) []schema.Event {
	t.Helper()
	store, err := eventlog.New(root)
	if err != nil {
		t.Fatalf("eventlog.New returned error: %v", err)
	}
	events, err := store.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	return events
}

func restoreGoalFlags(t *testing.T) {
	t.Helper()
	oldRoot := goalRoot
	oldID := goalID
	oldObjective := goalObjective
	oldPlanSummary := goalPlanSummary
	oldPlanSteps := goalPlanSteps
	oldMemoryRefs := goalMemoryRefs
	oldMemoryRecallRequests := goalMemoryRecallRequests
	oldSkillWorkflowRefs := goalSkillWorkflowRefs
	oldEvalRefs := goalEvalRefs
	oldEvidenceID := goalEvidenceID
	oldEvidenceType := goalEvidenceType
	oldEvidenceStatus := goalEvidenceStatus
	oldEvidenceSummary := goalEvidenceSummary
	oldEvidenceMemoryRefs := goalEvidenceMemoryRefs
	oldEvidenceMemoryReqs := goalEvidenceMemoryReqs
	oldEvidenceSkillSignals := goalEvidenceSkillSignals
	oldEvidenceEvalReports := goalEvidenceEvalReports
	oldEvidenceArtifactRefs := goalEvidenceArtifactRefs
	oldEvidenceAuditRefs := goalEvidenceAuditRefs
	oldEvidenceProposalRefs := goalEvidenceProposalRefs
	oldEvidenceHostRefs := goalEvidenceHostRefs
	oldVerifyGate := goalVerifyGate
	oldVerifySummary := goalVerifySummary
	oldBlockReason := goalBlockReason
	oldPauseReason := goalPauseReason
	oldResumeReason := goalResumeReason
	oldCompleteBlockOnFailure := goalCompleteBlockOnFailure
	oldNudgeAllIdle := goalNudgeAllIdle
	oldNudgeIdleAfter := goalNudgeIdleAfter
	oldNudgeSummary := goalNudgeSummary
	oldLinkHost := goalLinkHost
	oldLinkThreadID := goalLinkThreadID
	oldLinkHostGoalID := goalLinkHostGoalID
	oldLinkObjective := goalLinkObjective
	oldLinkEvidence := goalLinkEvidence
	t.Cleanup(func() {
		goalRoot = oldRoot
		goalID = oldID
		goalObjective = oldObjective
		goalPlanSummary = oldPlanSummary
		goalPlanSteps = oldPlanSteps
		goalMemoryRefs = oldMemoryRefs
		goalMemoryRecallRequests = oldMemoryRecallRequests
		goalSkillWorkflowRefs = oldSkillWorkflowRefs
		goalEvalRefs = oldEvalRefs
		goalEvidenceID = oldEvidenceID
		goalEvidenceType = oldEvidenceType
		goalEvidenceStatus = oldEvidenceStatus
		goalEvidenceSummary = oldEvidenceSummary
		goalEvidenceMemoryRefs = oldEvidenceMemoryRefs
		goalEvidenceMemoryReqs = oldEvidenceMemoryReqs
		goalEvidenceSkillSignals = oldEvidenceSkillSignals
		goalEvidenceEvalReports = oldEvidenceEvalReports
		goalEvidenceArtifactRefs = oldEvidenceArtifactRefs
		goalEvidenceAuditRefs = oldEvidenceAuditRefs
		goalEvidenceProposalRefs = oldEvidenceProposalRefs
		goalEvidenceHostRefs = oldEvidenceHostRefs
		goalVerifyGate = oldVerifyGate
		goalVerifySummary = oldVerifySummary
		goalBlockReason = oldBlockReason
		goalPauseReason = oldPauseReason
		goalResumeReason = oldResumeReason
		goalCompleteBlockOnFailure = oldCompleteBlockOnFailure
		goalNudgeAllIdle = oldNudgeAllIdle
		goalNudgeIdleAfter = oldNudgeIdleAfter
		goalNudgeSummary = oldNudgeSummary
		goalLinkHost = oldLinkHost
		goalLinkThreadID = oldLinkThreadID
		goalLinkHostGoalID = oldLinkHostGoalID
		goalLinkObjective = oldLinkObjective
		goalLinkEvidence = oldLinkEvidence
	})
	goalRoot = "."
	goalID = ""
	goalObjective = ""
	goalPlanSummary = ""
	goalPlanSteps = nil
	goalMemoryRefs = nil
	goalMemoryRecallRequests = nil
	goalSkillWorkflowRefs = nil
	goalEvalRefs = nil
	goalEvidenceID = ""
	goalEvidenceType = "manual"
	goalEvidenceStatus = "accepted"
	goalEvidenceSummary = ""
	goalEvidenceMemoryRefs = nil
	goalEvidenceMemoryReqs = nil
	goalEvidenceSkillSignals = nil
	goalEvidenceEvalReports = nil
	goalEvidenceArtifactRefs = nil
	goalEvidenceAuditRefs = nil
	goalEvidenceProposalRefs = nil
	goalEvidenceHostRefs = nil
	goalVerifyGate = ""
	goalVerifySummary = ""
	goalBlockReason = ""
	goalPauseReason = ""
	goalResumeReason = ""
	goalCompleteBlockOnFailure = false
	goalNudgeAllIdle = false
	goalNudgeIdleAfter = 6 * time.Hour
	goalNudgeSummary = ""
	goalLinkHost = "codex"
	goalLinkThreadID = ""
	goalLinkHostGoalID = ""
	goalLinkObjective = ""
	goalLinkEvidence = nil
}
