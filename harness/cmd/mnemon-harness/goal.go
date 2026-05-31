package main

import (
	"fmt"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/app"
	"github.com/spf13/cobra"
)

var (
	goalRoot                   string
	goalID                     string
	goalObjective              string
	goalPlanSummary            string
	goalPlanSteps              []string
	goalMemoryRefs             []string
	goalMemoryRecallRequests   []string
	goalSkillWorkflowRefs      []string
	goalEvalRefs               []string
	goalEvidenceID             string
	goalEvidenceType           string
	goalEvidenceStatus         string
	goalEvidenceSummary        string
	goalEvidenceMemoryRefs     []string
	goalEvidenceMemoryReqs     []string
	goalEvidenceSkillSignals   []string
	goalEvidenceEvalReports    []string
	goalEvidenceArtifactRefs   []string
	goalEvidenceAuditRefs      []string
	goalEvidenceProposalRefs   []string
	goalEvidenceHostRefs       []string
	goalVerifyGate             string
	goalVerifySummary          string
	goalBlockReason            string
	goalPauseReason            string
	goalResumeReason           string
	goalCompleteBlockOnFailure bool
	goalNudgeAllIdle           bool
	goalNudgeIdleAfter         time.Duration
	goalNudgeSummary           string
	goalLinkHost               string
	goalLinkThreadID           string
	goalLinkHostGoalID         string
	goalLinkObjective          string
	goalLinkEvidence           []string
)

var goalCmd = &cobra.Command{
	Use:   "goal",
	Short: "Manage project-scoped Mnemon lifecycle goals",
	Long:  "Manage project-scoped Mnemon goal state under .mnemon/harness/goals.",
}

var goalInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a Mnemon project goal",
	RunE:  runGoalInit,
}

var goalPlanCmd = &cobra.Command{
	Use:   "plan",
	Short: "Record or update a Mnemon goal plan",
	RunE:  runGoalPlan,
}

var goalStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show Mnemon goal status",
	RunE:  runGoalStatus,
}

var goalEvidenceCmd = &cobra.Command{
	Use:   "evidence",
	Short: "Manage Mnemon goal evidence",
}

var goalEvidenceAppendCmd = &cobra.Command{
	Use:   "append",
	Short: "Append one Mnemon goal evidence record",
	RunE:  runGoalEvidenceAppend,
}

var goalVerifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Verify a Mnemon goal against recorded evidence",
	RunE:  runGoalVerify,
}

var goalCompleteCmd = &cobra.Command{
	Use:   "complete",
	Short: "Complete a verified Mnemon goal",
	RunE:  runGoalComplete,
}

var goalBlockCmd = &cobra.Command{
	Use:   "block",
	Short: "Mark a Mnemon goal blocked",
	RunE:  runGoalBlock,
}

var goalPauseCmd = &cobra.Command{
	Use:   "pause",
	Short: "Pause a Mnemon goal",
	RunE:  runGoalPause,
}

var goalResumeCmd = &cobra.Command{
	Use:   "resume",
	Short: "Resume a Mnemon goal",
	RunE:  runGoalResume,
}

var goalNudgeCmd = &cobra.Command{
	Use:   "nudge",
	Short: "Record nudges for idle Mnemon goals",
	RunE:  runGoalNudge,
}

var goalLinkCmd = &cobra.Command{
	Use:   "link",
	Short: "Link a Mnemon goal to public host goal/thread state",
	RunE:  runGoalLink,
}

var goalCodexCmd = &cobra.Command{
	Use:   "codex",
	Short: "Generate Codex goal integration prompts",
}

var goalCodexPromptCmd = &cobra.Command{
	Use:   "prompt",
	Short: "Print a concise Codex /goal objective and Mnemon prompt snippet",
	RunE:  runGoalCodexPrompt,
}

func init() {
	goalCmd.PersistentFlags().StringVar(&goalRoot, "root", ".", "project root for harness goal state")

	goalInitCmd.Flags().StringVar(&goalID, "goal-id", "", "goal id; generated when unset")
	goalInitCmd.Flags().StringVar(&goalObjective, "objective", "", "goal objective")

	addGoalIDFlag(goalPlanCmd)
	goalPlanCmd.Flags().StringVar(&goalPlanSummary, "summary", "", "plan summary")
	goalPlanCmd.Flags().StringArrayVar(&goalPlanSteps, "step", nil, "plan step; may be repeated")
	goalPlanCmd.Flags().StringArrayVar(&goalMemoryRefs, "memory-ref", nil, "memory ref; may be repeated")
	goalPlanCmd.Flags().StringArrayVar(&goalMemoryRecallRequests, "memory-recall", nil, "memory recall request; may be repeated")
	goalPlanCmd.Flags().StringArrayVar(&goalSkillWorkflowRefs, "skill-ref", nil, "skill workflow ref; may be repeated")
	goalPlanCmd.Flags().StringArrayVar(&goalEvalRefs, "eval-ref", nil, "eval ref; may be repeated")

	addGoalIDFlag(goalStatusCmd)

	addGoalIDFlag(goalEvidenceAppendCmd)
	goalEvidenceAppendCmd.Flags().StringVar(&goalEvidenceID, "evidence-id", "", "evidence id; generated when unset")
	goalEvidenceAppendCmd.Flags().StringVar(&goalEvidenceType, "type", "manual", "evidence type")
	goalEvidenceAppendCmd.Flags().StringVar(&goalEvidenceStatus, "status", "accepted", "evidence status")
	goalEvidenceAppendCmd.Flags().StringVar(&goalEvidenceSummary, "summary", "", "evidence summary")
	goalEvidenceAppendCmd.Flags().StringArrayVar(&goalEvidenceMemoryRefs, "memory-ref", nil, "memory ref; may be repeated")
	goalEvidenceAppendCmd.Flags().StringArrayVar(&goalEvidenceMemoryReqs, "memory-request", nil, "memory request ref; may be repeated")
	goalEvidenceAppendCmd.Flags().StringArrayVar(&goalEvidenceSkillSignals, "skill-signal", nil, "skill signal ref; may be repeated")
	goalEvidenceAppendCmd.Flags().StringArrayVar(&goalEvidenceEvalReports, "eval-report-ref", nil, "eval report ref; may be repeated")
	goalEvidenceAppendCmd.Flags().StringArrayVar(&goalEvidenceArtifactRefs, "artifact-ref", nil, "artifact ref; may be repeated")
	goalEvidenceAppendCmd.Flags().StringArrayVar(&goalEvidenceAuditRefs, "audit-ref", nil, "audit ref; may be repeated")
	goalEvidenceAppendCmd.Flags().StringArrayVar(&goalEvidenceProposalRefs, "proposal-ref", nil, "proposal ref; may be repeated")
	goalEvidenceAppendCmd.Flags().StringArrayVar(&goalEvidenceHostRefs, "host-evidence-ref", nil, "host evidence ref; may be repeated")

	addGoalIDFlag(goalVerifyCmd)
	goalVerifyCmd.Flags().StringVar(&goalVerifyGate, "gate", "", "verification gate name")
	goalVerifyCmd.Flags().StringVar(&goalVerifySummary, "summary", "", "verification summary")

	addGoalIDFlag(goalCompleteCmd)
	goalCompleteCmd.Flags().BoolVar(&goalCompleteBlockOnFailure, "block-on-failure", false, "move the goal to blocked instead of returning an error when completion gates fail")

	addGoalIDFlag(goalBlockCmd)
	goalBlockCmd.Flags().StringVar(&goalBlockReason, "reason", "", "blocked reason")

	addGoalIDFlag(goalPauseCmd)
	goalPauseCmd.Flags().StringVar(&goalPauseReason, "reason", "", "pause reason")

	addGoalIDFlag(goalResumeCmd)
	goalResumeCmd.Flags().StringVar(&goalResumeReason, "reason", "", "resume reason")

	addGoalIDFlag(goalNudgeCmd)
	goalNudgeCmd.Flags().BoolVar(&goalNudgeAllIdle, "all-idle", false, "nudge all non-terminal idle goals")
	goalNudgeCmd.Flags().DurationVar(&goalNudgeIdleAfter, "idle-after", 6*time.Hour, "minimum idle duration before nudging")
	goalNudgeCmd.Flags().StringVar(&goalNudgeSummary, "summary", "", "nudge summary")

	addGoalIDFlag(goalLinkCmd)
	goalLinkCmd.Flags().StringVar(&goalLinkHost, "host", "codex", "host id")
	goalLinkCmd.Flags().StringVar(&goalLinkThreadID, "thread-id", "", "public host thread id")
	goalLinkCmd.Flags().StringVar(&goalLinkHostGoalID, "host-goal-id", "", "public host goal id")
	goalLinkCmd.Flags().StringVar(&goalLinkObjective, "objective", "", "linked host objective; generated when unset")
	goalLinkCmd.Flags().StringArrayVar(&goalLinkEvidence, "evidence", nil, "link evidence ref; may be repeated")

	addGoalIDFlag(goalCodexPromptCmd)

	goalEvidenceCmd.AddCommand(goalEvidenceAppendCmd)
	goalCodexCmd.AddCommand(goalCodexPromptCmd)
	goalCmd.AddCommand(
		goalInitCmd,
		goalPlanCmd,
		goalStatusCmd,
		goalEvidenceCmd,
		goalVerifyCmd,
		goalCompleteCmd,
		goalBlockCmd,
		goalPauseCmd,
		goalResumeCmd,
		goalNudgeCmd,
		goalLinkCmd,
		goalCodexCmd,
	)
	rootCmd.AddCommand(goalCmd)
}

func addGoalIDFlag(command *cobra.Command) {
	command.Flags().StringVar(&goalID, "goal-id", "", "goal id")
}

func runGoalInit(cmd *cobra.Command, args []string) error {
	ref, err := app.New(goalRoot).GoalInit(goalID, goalObjective)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "created goal %s\n", ref.ID)
	fmt.Fprintf(cmd.OutOrStdout(), "path: %s\n", ref.Path)
	return nil
}

func runGoalPlan(cmd *cobra.Command, args []string) error {
	state, err := app.New(goalRoot).GoalPlan(goalID, goalPlanSummary, goalPlanSteps, goalMemoryRefs, goalMemoryRecallRequests, goalSkillWorkflowRefs, goalEvalRefs)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "planned goal %s (%s)\n", state.ID, state.Status)
	return nil
}

func runGoalStatus(cmd *cobra.Command, args []string) error {
	view, err := app.New(goalRoot).GoalStatus(goalID)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "goal %s: %s\n", view.ID, view.Status)
	fmt.Fprintf(cmd.OutOrStdout(), "evidence: %d\n", view.EvidenceCount)
	fmt.Fprintf(cmd.OutOrStdout(), "report: %s\n", view.ReportStatus)
	fmt.Fprintf(cmd.OutOrStdout(), "completion_ready: %t\n", view.Ready)
	fmt.Fprintf(cmd.OutOrStdout(), "path: %s\n", view.Path)
	return nil
}

func runGoalEvidenceAppend(cmd *cobra.Command, args []string) error {
	id, err := app.New(goalRoot).GoalEvidenceAppend(goalID, goalEvidenceID, goalEvidenceType, goalEvidenceStatus, goalEvidenceSummary, app.EvidenceRefs{
		MemoryRefs:       goalEvidenceMemoryRefs,
		MemoryRequests:   goalEvidenceMemoryReqs,
		SkillSignals:     goalEvidenceSkillSignals,
		EvalReportRefs:   goalEvidenceEvalReports,
		ArtifactRefs:     goalEvidenceArtifactRefs,
		AuditRefs:        goalEvidenceAuditRefs,
		ProposalRefs:     goalEvidenceProposalRefs,
		HostEvidenceRefs: goalEvidenceHostRefs,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "appended goal evidence %s\n", id)
	return nil
}

func runGoalVerify(cmd *cobra.Command, args []string) error {
	result, err := app.New(goalRoot).GoalVerify(goalID, goalVerifyGate, goalVerifySummary)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "verified goal %s: %s\n", result.GoalID, result.Status)
	fmt.Fprintf(cmd.OutOrStdout(), "gate: %s passed=%t\n", result.GateName, result.GatePassed)
	return nil
}

func runGoalComplete(cmd *cobra.Command, args []string) error {
	id, err := app.New(goalRoot).GoalComplete(goalID, goalCompleteBlockOnFailure)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "completed goal %s\n", id)
	return nil
}

func runGoalBlock(cmd *cobra.Command, args []string) error {
	id, err := app.New(goalRoot).GoalTransition("block", goalID, goalBlockReason)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "blocked goal %s\n", id)
	return nil
}

func runGoalPause(cmd *cobra.Command, args []string) error {
	id, err := app.New(goalRoot).GoalTransition("pause", goalID, goalPauseReason)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "paused goal %s\n", id)
	return nil
}

func runGoalResume(cmd *cobra.Command, args []string) error {
	id, err := app.New(goalRoot).GoalTransition("resume", goalID, goalResumeReason)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "resumed goal %s\n", id)
	return nil
}

func runGoalNudge(cmd *cobra.Command, args []string) error {
	results, err := app.New(goalRoot).GoalNudge(goalID, goalNudgeAllIdle, goalNudgeIdleAfter, goalNudgeSummary)
	if err != nil {
		return err
	}
	nudged := 0
	for _, result := range results {
		if result.Skipped {
			fmt.Fprintf(cmd.OutOrStdout(), "skipped goal %s: %s\n", result.GoalID, result.Reason)
			continue
		}
		nudged++
		fmt.Fprintf(cmd.OutOrStdout(), "nudged goal %s: %s\n", result.GoalID, result.Path)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "nudged %d goals\n", nudged)
	return nil
}

func runGoalLink(cmd *cobra.Command, args []string) error {
	link, err := app.New(goalRoot).GoalLink(goalID, goalLinkHost, goalLinkThreadID, goalLinkHostGoalID, goalLinkObjective, goalLinkEvidence)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "linked goal %s to %s\n", link.GoalID, link.Host)
	if link.ThreadID != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "thread_id: %s\n", link.ThreadID)
	}
	if link.HostGoalID != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "host_goal_id: %s\n", link.HostGoalID)
	}
	return nil
}

func runGoalCodexPrompt(cmd *cobra.Command, args []string) error {
	prompt, err := app.New(goalRoot).GoalCodexPrompt(goalID)
	if err != nil {
		return err
	}
	fmt.Fprint(cmd.OutOrStdout(), prompt)
	fmt.Fprintln(cmd.OutOrStdout())
	return nil
}
