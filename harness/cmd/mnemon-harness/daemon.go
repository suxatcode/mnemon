package main

import (
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/app"
	"github.com/spf13/cobra"
)

var (
	daemonRoot              string
	daemonRunOnce           bool
	daemonRunBackground     bool
	daemonRunDryRun         bool
	daemonInterval          time.Duration
	daemonCodexSemanticRun  bool
	daemonAcknowledgeCost   bool
	daemonCodexCommand      string
	daemonCodexMaxTurns     int
	daemonCodexTimeout      time.Duration
	daemonCodexTurnTimeout  time.Duration
	daemonCodexIsolatedHome bool
	daemonTriggerForce      bool
	daemonTriggerDryRun     bool
	daemonStatusJSON        bool
	daemonStatusLimit       int
	daemonPauseReason       string
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run or trigger declarative daemon jobs",
}

var daemonRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run declarative daemon jobs once or in the background",
	RunE:  runDaemonRun,
}

var daemonTriggerCmd = &cobra.Command{
	Use:   "trigger <job-id>",
	Short: "Evaluate or force one declarative daemon job",
	Args:  cobra.ExactArgs(1),
	RunE:  runDaemonTrigger,
}

var daemonStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show daemon queue, tick, budget, and job status",
	RunE:  runDaemonStatus,
}

var daemonPauseCmd = &cobra.Command{
	Use:   "pause",
	Short: "Pause daemon enqueueing without stopping existing jobs",
	RunE:  runDaemonPause,
}

var daemonResumeCmd = &cobra.Command{
	Use:   "resume",
	Short: "Resume daemon enqueueing",
	RunE:  runDaemonResume,
}

func init() {
	daemonCmd.PersistentFlags().StringVar(&daemonRoot, "root", ".", "project root for harness daemon state")
	daemonRunCmd.Flags().BoolVar(&daemonRunOnce, "once", false, "run one daemon tick")
	daemonRunCmd.Flags().BoolVar(&daemonRunBackground, "background", false, "run daemon ticks until interrupted")
	daemonRunCmd.Flags().BoolVar(&daemonRunDryRun, "dry-run", false, "evaluate daemon jobs without enqueueing or executing")
	daemonRunCmd.Flags().DurationVar(&daemonInterval, "interval", 5*time.Second, "daemon background poll interval")
	addDaemonRunnerFlags(daemonRunCmd)
	daemonTriggerCmd.Flags().BoolVar(&daemonTriggerForce, "force", false, "enqueue the job even when its trigger does not currently match")
	daemonTriggerCmd.Flags().BoolVar(&daemonTriggerDryRun, "dry-run", false, "print what would be triggered without enqueueing")
	addDaemonRunnerFlags(daemonTriggerCmd)
	daemonStatusCmd.Flags().BoolVar(&daemonStatusJSON, "json", false, "print daemon status as JSON")
	daemonStatusCmd.Flags().IntVar(&daemonStatusLimit, "limit", 10, "number of recent ticks to show")
	daemonPauseCmd.Flags().StringVar(&daemonPauseReason, "reason", "manual", "pause reason")
	daemonCmd.AddCommand(daemonRunCmd, daemonTriggerCmd, daemonStatusCmd, daemonPauseCmd, daemonResumeCmd)
	daemonCmd.GroupID = groupAdvanced
	rootCmd.AddCommand(daemonCmd)
}

func addDaemonRunnerFlags(command *cobra.Command) {
	command.Flags().BoolVar(&daemonCodexSemanticRun, "agent-turn", false, "allow daemon semantic jobs to start real Codex turns")
	command.Flags().BoolVar(&daemonAcknowledgeCost, "i-understand-model-cost", false, "acknowledge daemon semantic dispatch may consume model quota")
	command.Flags().StringVar(&daemonCodexCommand, "codex-command", "codex", "Codex CLI command for daemon semantic dispatch")
	command.Flags().IntVar(&daemonCodexMaxTurns, "max-real-turns", 3, "maximum real Codex turns for one daemon tick")
	command.Flags().DurationVar(&daemonCodexTimeout, "codex-timeout", 5*time.Minute, "overall Codex app-server timeout")
	command.Flags().DurationVar(&daemonCodexTurnTimeout, "codex-turn-timeout", 3*time.Minute, "per-turn Codex timeout")
	command.Flags().BoolVar(&daemonCodexIsolatedHome, "isolated-codex-home", false, "use isolated CODEX_HOME for daemon semantic dispatch")
}

func daemonOptions() app.DaemonOptions {
	return app.DaemonOptions{
		EnableCodexSemanticRun: daemonCodexSemanticRun,
		AcknowledgeModelCost:   daemonAcknowledgeCost,
		CodexCommand:           daemonCodexCommand,
		CodexMaxTurns:          daemonCodexMaxTurns,
		CodexTimeout:           daemonCodexTimeout,
		CodexTurnTimeout:       daemonCodexTurnTimeout,
		CodexIsolatedHome:      daemonCodexIsolatedHome,
	}
}

func runDaemonRun(cmd *cobra.Command, args []string) error {
	return app.New(daemonRoot).DaemonRun(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), daemonRunOnce, daemonRunBackground, daemonRunDryRun, daemonInterval, daemonOptions())
}

func runDaemonTrigger(cmd *cobra.Command, args []string) error {
	return app.New(daemonRoot).DaemonTrigger(cmd.OutOrStdout(), args[0], daemonTriggerForce, daemonTriggerDryRun, daemonOptions())
}

func runDaemonStatus(cmd *cobra.Command, args []string) error {
	return app.New(daemonRoot).DaemonStatus(cmd.OutOrStdout(), daemonStatusLimit, daemonStatusJSON)
}

func runDaemonPause(cmd *cobra.Command, args []string) error {
	return app.New(daemonRoot).DaemonPause(cmd.OutOrStdout(), daemonPauseReason)
}

func runDaemonResume(cmd *cobra.Command, args []string) error {
	return app.New(daemonRoot).DaemonResume(cmd.OutOrStdout())
}
