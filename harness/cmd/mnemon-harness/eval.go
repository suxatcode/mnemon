package main

import (
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/app"
	"github.com/spf13/cobra"
)

var (
	evalRoot                    string
	evalPlanSuite               string
	evalPlanFormat              string
	evalRunSuite                string
	evalRunScenario             string
	evalRunHost                 string
	evalRunCommand              string
	evalRunTimeout              time.Duration
	evalRunTurnTimeout          time.Duration
	evalRunMaxTurns             int
	evalRunIsolatedHome         bool
	evalRunAgentTurn            bool
	evalRunAcknowledgeModelCost bool
	evalAssertSuite             string
	evalAssertScenario          string
	evalAssertRunID             string
	evalABSuite                 string
	evalABScenarios             []string
	evalABTrialsPerArm          int
	evalABCommand               string
	evalABTimeout               time.Duration
	evalABTurnTimeout           time.Duration
	evalABMaxTurns              int
	evalABIsolatedHome          bool
	evalABAgentTurn             bool
	evalABAcknowledgeModelCost  bool
	evalABControlSetupJSON      string
	evalABTreatmentSetupJSON    string
	evalPromoteScenario         string
	evalPromoteSuite            string
	evalPromoteRubric           string
	evalPromoteTarget           string
	evalPromoteFrom             string
	evalPromoteProposalRef      string
	evalPromoteAuditRef         string
	evalPromoteEventID          string
	evalPromoteCorrelationID    string
	evalPromoteCausedBy         string
	evalReportRunID             string
	evalReportFormat            string
	evalReplayTier              string
	evalReplayFormat            string
)

var evalCmd = &cobra.Command{
	Use:   "eval",
	Short: "Manage declaration-driven harness evals",
}

var evalPlanCmd = &cobra.Command{
	Use:   "plan --suite SUITE",
	Short: "Print a declaration-driven eval suite plan",
	RunE:  runEvalPlan,
}

var evalRunCmd = &cobra.Command{
	Use:   "run --suite SUITE [--scenario SCENARIO]",
	Short: "Run an eval scenario through the Codex app-server runner",
	RunE:  runEvalRun,
}

var evalAssertCmd = &cobra.Command{
	Use:   "assert --suite SUITE --scenario SCENARIO",
	Short: "Run eval scenario setup and assertions without starting Codex",
	RunE:  runEvalAssert,
}

var evalABTestCmd = &cobra.Command{
	Use:   "abtest --suite SUITE [--scenario SCENARIO]",
	Short: "Run paired control/treatment eval trials and compare deterministic pass rate",
	RunE:  runEvalABTest,
}

var evalPromoteCmd = &cobra.Command{
	Use:   "promote (--scenario ID | --suite NAME | --rubric ID) --proposal-ref PROPOSAL",
	Short: "Record a governed eval asset promotion event",
	RunE:  runEvalPromote,
}

var evalReportCmd = &cobra.Command{
	Use:   "report --run-id RUN_ID",
	Short: "Print an eval runner report",
	RunE:  runEvalReport,
}

var evalReplayCmd = &cobra.Command{
	Use:   "replay",
	Short: "Run deterministic regression replay checks",
	RunE:  runEvalReplay,
}

func init() {
	evalCmd.PersistentFlags().StringVar(&evalRoot, "root", ".", "repository root containing eval declarations")
	evalPlanCmd.Flags().StringVar(&evalPlanSuite, "suite", "default", "eval suite name")
	evalPlanCmd.Flags().StringVar(&evalPlanFormat, "format", "text", "output format: text or json")
	evalRunCmd.Flags().StringVar(&evalRunSuite, "suite", "default", "eval suite name")
	evalRunCmd.Flags().StringVar(&evalRunScenario, "scenario", "", "eval scenario id; defaults to the suite's first scenario")
	evalRunCmd.Flags().StringVar(&evalRunHost, "host", "", "host adapter; defaults to the suite host")
	evalRunCmd.Flags().StringVar(&evalRunCommand, "command", "codex", "Codex CLI command")
	evalRunCmd.Flags().DurationVar(&evalRunTimeout, "timeout", 5*time.Minute, "overall Codex app-server eval run timeout")
	evalRunCmd.Flags().DurationVar(&evalRunTurnTimeout, "turn-timeout", 3*time.Minute, "per-turn timeout")
	evalRunCmd.Flags().IntVar(&evalRunMaxTurns, "max-turns", 0, "maximum real Codex turns; defaults to the runner limit")
	evalRunCmd.Flags().BoolVar(&evalRunIsolatedHome, "isolated-codex-home", false, "use an isolated CODEX_HOME for the run")
	evalRunCmd.Flags().BoolVar(&evalRunAgentTurn, "agent-turn", false, "allow starting a real Codex turn")
	evalRunCmd.Flags().BoolVar(&evalRunAcknowledgeModelCost, "i-understand-model-cost", false, "acknowledge that a real Codex turn may consume model quota")
	evalAssertCmd.Flags().StringVar(&evalAssertSuite, "suite", "default", "eval suite name")
	evalAssertCmd.Flags().StringVar(&evalAssertScenario, "scenario", "", "eval scenario id")
	evalAssertCmd.Flags().StringVar(&evalAssertRunID, "run-id", "", "assertion fixture run id; generated when unset")
	evalABTestCmd.Flags().StringVar(&evalABSuite, "suite", "default", "eval suite name")
	evalABTestCmd.Flags().StringSliceVar(&evalABScenarios, "scenario", nil, "eval scenario id; may be repeated; defaults to the suite's first scenario")
	evalABTestCmd.Flags().IntVar(&evalABTrialsPerArm, "trials-per-arm", 1, "number of repeated runs per arm")
	evalABTestCmd.Flags().StringVar(&evalABCommand, "command", "codex", "Codex CLI command")
	evalABTestCmd.Flags().DurationVar(&evalABTimeout, "timeout", 5*time.Minute, "overall Codex app-server eval run timeout per trial")
	evalABTestCmd.Flags().DurationVar(&evalABTurnTimeout, "turn-timeout", 3*time.Minute, "per-turn timeout")
	evalABTestCmd.Flags().IntVar(&evalABMaxTurns, "max-turns", 0, "maximum real Codex turns per trial; defaults to the runner limit")
	evalABTestCmd.Flags().BoolVar(&evalABIsolatedHome, "isolated-codex-home", false, "use an isolated CODEX_HOME for each trial")
	evalABTestCmd.Flags().BoolVar(&evalABAgentTurn, "agent-turn", false, "allow starting real Codex turns for A/B trials")
	evalABTestCmd.Flags().BoolVar(&evalABAcknowledgeModelCost, "i-understand-model-cost", false, "acknowledge that A/B trials may consume model quota")
	evalABTestCmd.Flags().StringVar(&evalABControlSetupJSON, "control-setup-json", "", "JSON object describing control arm setup metadata")
	evalABTestCmd.Flags().StringVar(&evalABTreatmentSetupJSON, "treatment-setup-json", "", "JSON object describing treatment arm setup metadata")
	evalPromoteCmd.Flags().StringVar(&evalPromoteScenario, "scenario", "", "eval scenario id or scenario file path under harness/loops/eval/scenarios")
	evalPromoteCmd.Flags().StringVar(&evalPromoteSuite, "suite", "", "eval suite name")
	evalPromoteCmd.Flags().StringVar(&evalPromoteRubric, "rubric", "", "eval rubric id or rubric filename")
	evalPromoteCmd.Flags().StringVar(&evalPromoteTarget, "target", "promoted", "promotion target: candidate, promoted, or canonical")
	evalPromoteCmd.Flags().StringVar(&evalPromoteFrom, "from", "", "optional source state: ephemeral, candidate, promoted, or canonical")
	evalPromoteCmd.Flags().StringVar(&evalPromoteProposalRef, "proposal-ref", "", "approved eval proposal id authorizing the promotion")
	evalPromoteCmd.Flags().StringVar(&evalPromoteAuditRef, "audit-ref", "", "optional audit ref to include on the promotion event")
	evalPromoteCmd.Flags().StringVar(&evalPromoteEventID, "event-id", "", "event id; generated when unset")
	evalPromoteCmd.Flags().StringVar(&evalPromoteCorrelationID, "correlation-id", "", "correlation id; generated from proposal when unset")
	evalPromoteCmd.Flags().StringVar(&evalPromoteCausedBy, "caused-by", "", "causal event id")
	evalReportCmd.Flags().StringVar(&evalReportRunID, "run-id", "", "eval run id")
	evalReportCmd.Flags().StringVar(&evalReportFormat, "format", "text", "output format: text or json")
	evalReplayCmd.Flags().StringVar(&evalReplayTier, "tier", "1", "comma-separated regression tiers to replay, such as 1 or 1,2")
	evalReplayCmd.Flags().StringVar(&evalReplayFormat, "format", "text", "output format: text or json")
	evalCmd.AddCommand(evalPlanCmd, evalRunCmd, evalAssertCmd, evalABTestCmd, evalPromoteCmd, evalReportCmd, evalReplayCmd)
	evalCmd.GroupID = groupAdvanced
	rootCmd.AddCommand(evalCmd)
}

func runEvalPlan(cmd *cobra.Command, args []string) error {
	return app.New(evalRoot).EvalPlan(cmd.OutOrStdout(), evalPlanSuite, evalPlanFormat)
}

func runEvalRun(cmd *cobra.Command, args []string) error {
	return app.New(evalRoot).EvalRun(cmd.Context(), cmd.OutOrStdout(), app.EvalRunInput{
		Suite:                evalRunSuite,
		Scenario:             evalRunScenario,
		Host:                 evalRunHost,
		Command:              evalRunCommand,
		Timeout:              evalRunTimeout,
		TurnTimeout:          evalRunTurnTimeout,
		MaxTurns:             evalRunMaxTurns,
		IsolatedHome:         evalRunIsolatedHome,
		AgentTurn:            evalRunAgentTurn,
		AcknowledgeModelCost: evalRunAcknowledgeModelCost,
	})
}

func runEvalAssert(cmd *cobra.Command, args []string) error {
	return app.New(evalRoot).EvalAssert(cmd.Context(), cmd.OutOrStdout(), evalAssertSuite, evalAssertScenario, evalAssertRunID)
}

func runEvalABTest(cmd *cobra.Command, args []string) error {
	return app.New(evalRoot).EvalABTest(cmd.Context(), cmd.OutOrStdout(), app.EvalABInput{
		Suite:                evalABSuite,
		Scenarios:            evalABScenarios,
		TrialsPerArm:         evalABTrialsPerArm,
		Command:              evalABCommand,
		Timeout:              evalABTimeout,
		TurnTimeout:          evalABTurnTimeout,
		MaxTurns:             evalABMaxTurns,
		IsolatedHome:         evalABIsolatedHome,
		AgentTurn:            evalABAgentTurn,
		AcknowledgeModelCost: evalABAcknowledgeModelCost,
		ControlSetupJSON:     evalABControlSetupJSON,
		TreatmentSetupJSON:   evalABTreatmentSetupJSON,
	})
}

func runEvalPromote(cmd *cobra.Command, args []string) error {
	return app.New(evalRoot).EvalPromote(cmd.OutOrStdout(), app.EvalPromoteInput{
		Scenario:      evalPromoteScenario,
		Suite:         evalPromoteSuite,
		Rubric:        evalPromoteRubric,
		Target:        evalPromoteTarget,
		From:          evalPromoteFrom,
		ProposalRef:   evalPromoteProposalRef,
		AuditRef:      evalPromoteAuditRef,
		EventID:       evalPromoteEventID,
		CorrelationID: evalPromoteCorrelationID,
		CausedBy:      evalPromoteCausedBy,
	})
}

func runEvalReport(cmd *cobra.Command, args []string) error {
	return app.New(evalRoot).EvalReport(cmd.OutOrStdout(), evalReportRunID, evalReportFormat)
}

func runEvalReplay(cmd *cobra.Command, args []string) error {
	return app.New(evalRoot).EvalReplay(cmd.OutOrStdout(), evalReplayTier, evalReplayFormat)
}
