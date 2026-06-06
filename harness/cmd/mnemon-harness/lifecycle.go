package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/mnemon-dev/mnemon/harness/internal/app"
	"github.com/spf13/cobra"
)

var (
	lifecycleRoot                 string
	lifecycleEventFile            string
	lifecycleEventJSON            string
	lifecycleDaemonInterval       time.Duration
	lifecycleRunnerTimeout        time.Duration
	lifecycleCodexCommand         string
	lifecycleCodexIsolatedHome    bool
	lifecycleCodexAgentTurn       bool
	lifecycleCodexAcknowledgeCost bool
	lifecycleCodexPrompt          string
	lifecycleCodexProjectRoot     string
	lifecycleCodexJobID           string
	lifecycleCodexJobSpec         string
	lifecycleCodexLoop            string
	lifecycleCodexMaxTurns        int
	lifecycleCodexTurnTimeout     time.Duration
	lifecycleAntipatternFormat    string
)

var lifecycleCmd = &cobra.Command{
	Use:   "lifecycle",
	Short: "Experimental ai-native lifecycle runtime",
	Long:  "Experimental ai-native lifecycle runtime for project-local .mnemon state.",
}

var lifecycleInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize experimental project lifecycle layout",
	RunE:  runLifecycleInit,
}

var lifecycleEventCmd = &cobra.Command{
	Use:   "event",
	Short: "Manage lifecycle events",
}

var lifecycleEventAppendCmd = &cobra.Command{
	Use:   "append",
	Short: "Validate and append one lifecycle event JSON object",
	RunE:  runLifecycleEventAppend,
}

var lifecycleStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Materialize lifecycle status",
}

var lifecycleAntipatternCmd = &cobra.Command{
	Use:   "antipattern",
	Short: "Run lifecycle anti-pattern checks",
}

var lifecycleAntipatternScanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Write a deterministic anti-pattern scan report",
	RunE:  runLifecycleAntipatternScan,
}

var lifecycleStatusRefreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Refresh lifecycle status from events",
	RunE:  runLifecycleStatusRefresh,
}

var lifecycleDaemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run the experimental lifecycle daemon",
}

var lifecycleDaemonTickCmd = &cobra.Command{
	Use:   "tick",
	Short: "Run one lifecycle daemon tick",
	RunE:  runLifecycleDaemonTick,
}

var lifecycleDaemonForegroundCmd = &cobra.Command{
	Use:   "foreground",
	Short: "Run the lifecycle daemon in the foreground until interrupted",
	RunE:  runLifecycleDaemonForeground,
}

var lifecycleDaemonStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show daemon queue, tick, budget, and job status",
	RunE:  runLifecycleDaemonStatus,
}

var lifecycleDaemonPauseCmd = &cobra.Command{
	Use:   "pause",
	Short: "Pause daemon enqueueing without stopping existing jobs",
	RunE:  runLifecycleDaemonPause,
}

var lifecycleDaemonResumeCmd = &cobra.Command{
	Use:   "resume",
	Short: "Resume daemon enqueueing",
	RunE:  runLifecycleDaemonResume,
}

var lifecycleRunnerCmd = &cobra.Command{
	Use:   "runner",
	Short: "Manage experimental lifecycle HostAgent runners",
}

var lifecycleRunnerCodexCmd = &cobra.Command{
	Use:   "codex",
	Short: "Manage the experimental Codex app-server runner",
}

var lifecycleRunnerCodexCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Check Codex app-server readiness without starting a real turn",
	RunE:  runLifecycleRunnerCodexCheck,
}

var lifecycleRunnerCodexRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run a gated real Codex app-server semantic lifecycle task",
	RunE:  runLifecycleRunnerCodexRun,
}

func init() {
	lifecycleCmd.PersistentFlags().StringVar(&lifecycleRoot, "root", ".", "project root for harness lifecycle state")
	lifecycleEventAppendCmd.Flags().StringVar(&lifecycleEventFile, "file", "", "path to event JSON object; reads stdin when unset")
	lifecycleEventAppendCmd.Flags().StringVar(&lifecycleEventJSON, "json", "", "event JSON object literal")
	lifecycleAntipatternScanCmd.Flags().StringVar(&lifecycleAntipatternFormat, "format", "text", "output format: text or json")
	lifecycleDaemonForegroundCmd.Flags().DurationVar(&lifecycleDaemonInterval, "interval", 5*time.Second, "daemon poll interval")
	lifecycleDaemonStatusCmd.Flags().BoolVar(&daemonStatusJSON, "json", false, "print daemon status as JSON")
	lifecycleDaemonStatusCmd.Flags().IntVar(&daemonStatusLimit, "limit", 10, "number of recent ticks to show")
	lifecycleDaemonPauseCmd.Flags().StringVar(&daemonPauseReason, "reason", "manual", "pause reason")
	addDaemonCodexFlags(lifecycleDaemonTickCmd)
	addDaemonCodexFlags(lifecycleDaemonForegroundCmd)
	lifecycleRunnerCodexCheckCmd.Flags().DurationVar(&lifecycleRunnerTimeout, "timeout", 30*time.Second, "Codex app-server readiness timeout")
	lifecycleRunnerCodexCheckCmd.Flags().StringVar(&lifecycleCodexCommand, "command", "codex", "Codex CLI command")
	lifecycleRunnerCodexCheckCmd.Flags().BoolVar(&lifecycleCodexIsolatedHome, "isolated-codex-home", false, "use an isolated CODEX_HOME for readiness")
	lifecycleRunnerCodexRunCmd.Flags().DurationVar(&lifecycleRunnerTimeout, "timeout", 5*time.Minute, "overall Codex app-server semantic run timeout")
	lifecycleRunnerCodexRunCmd.Flags().DurationVar(&lifecycleCodexTurnTimeout, "turn-timeout", 3*time.Minute, "per-turn timeout")
	lifecycleRunnerCodexRunCmd.Flags().StringVar(&lifecycleCodexCommand, "command", "codex", "Codex CLI command")
	lifecycleRunnerCodexRunCmd.Flags().StringVar(&lifecycleCodexPrompt, "prompt", "", "semantic lifecycle task prompt")
	lifecycleRunnerCodexRunCmd.Flags().StringVar(&lifecycleCodexProjectRoot, "project-root", "", "existing project root to use as the Codex cwd; relative paths resolve under --root")
	lifecycleRunnerCodexRunCmd.Flags().StringVar(&lifecycleCodexJobID, "job-id", "", "semantic lifecycle job id")
	lifecycleRunnerCodexRunCmd.Flags().StringVar(&lifecycleCodexJobSpec, "job-spec", "manual.semantic", "semantic lifecycle job spec")
	lifecycleRunnerCodexRunCmd.Flags().StringVar(&lifecycleCodexLoop, "loop", "eval", "lifecycle loop id")
	lifecycleRunnerCodexRunCmd.Flags().IntVar(&lifecycleCodexMaxTurns, "max-turns", 3, "maximum real Codex turns")
	lifecycleRunnerCodexRunCmd.Flags().BoolVar(&lifecycleCodexAgentTurn, "agent-turn", false, "allow starting a real Codex turn")
	lifecycleRunnerCodexRunCmd.Flags().BoolVar(&lifecycleCodexAcknowledgeCost, "i-understand-model-cost", false, "acknowledge that a real Codex turn may consume model quota")
	lifecycleRunnerCodexRunCmd.Flags().BoolVar(&lifecycleCodexIsolatedHome, "isolated-codex-home", false, "use an isolated CODEX_HOME for the run")

	lifecycleEventCmd.AddCommand(lifecycleEventAppendCmd)
	lifecycleStatusCmd.AddCommand(lifecycleStatusRefreshCmd)
	lifecycleAntipatternCmd.AddCommand(lifecycleAntipatternScanCmd)
	lifecycleDaemonCmd.AddCommand(lifecycleDaemonTickCmd, lifecycleDaemonForegroundCmd, lifecycleDaemonStatusCmd, lifecycleDaemonPauseCmd, lifecycleDaemonResumeCmd)
	lifecycleRunnerCodexCmd.AddCommand(lifecycleRunnerCodexCheckCmd, lifecycleRunnerCodexRunCmd)
	lifecycleRunnerCmd.AddCommand(lifecycleRunnerCodexCmd)
	lifecycleCmd.AddCommand(lifecycleInitCmd, lifecycleEventCmd, lifecycleStatusCmd, lifecycleAntipatternCmd, lifecycleDaemonCmd, lifecycleRunnerCmd)
	lifecycleCmd.GroupID = groupAdvanced
	rootCmd.AddCommand(lifecycleCmd)
}

func addDaemonCodexFlags(command *cobra.Command) {
	command.Flags().BoolVar(&lifecycleCodexAgentTurn, "codex-semantic-run", false, "allow daemon to dispatch semantic jobs to real Codex app-server")
	command.Flags().BoolVar(&lifecycleCodexAcknowledgeCost, "i-understand-model-cost", false, "acknowledge daemon semantic dispatch may consume model quota")
	command.Flags().StringVar(&lifecycleCodexCommand, "codex-command", "codex", "Codex CLI command for daemon semantic dispatch")
	command.Flags().DurationVar(&lifecycleRunnerTimeout, "codex-timeout", 5*time.Minute, "overall Codex app-server semantic run timeout")
	command.Flags().DurationVar(&lifecycleCodexTurnTimeout, "codex-turn-timeout", 3*time.Minute, "per-turn timeout for daemon semantic dispatch")
	command.Flags().IntVar(&lifecycleCodexMaxTurns, "max-real-turns", 3, "maximum real Codex turns for one daemon tick")
	command.Flags().BoolVar(&lifecycleCodexIsolatedHome, "isolated-codex-home", false, "use an isolated CODEX_HOME for daemon semantic dispatch")
}

// lifecycleEventInput reads the event JSON bytes from --json, --file, or stdin.
// It is pure surface I/O and stays in the cmd layer.
func lifecycleEventInput(cmd *cobra.Command) ([]byte, error) {
	if lifecycleEventJSON != "" && lifecycleEventFile != "" {
		return nil, fmt.Errorf("--json and --file are mutually exclusive")
	}
	if lifecycleEventJSON != "" {
		return []byte(lifecycleEventJSON), nil
	}
	if lifecycleEventFile != "" {
		data, err := os.ReadFile(lifecycleEventFile)
		if err != nil {
			return nil, fmt.Errorf("read event file: %w", err)
		}
		return data, nil
	}
	data, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return nil, fmt.Errorf("read event stdin: %w", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("event JSON is required via --json, --file, or stdin")
	}
	return data, nil
}

func lifecycleDaemonOptions() app.DaemonOptions {
	return app.DaemonOptions{
		EnableCodexSemanticRun: lifecycleCodexAgentTurn,
		AcknowledgeModelCost:   lifecycleCodexAcknowledgeCost,
		CodexCommand:           lifecycleCodexCommand,
		CodexMaxTurns:          lifecycleCodexMaxTurns,
		CodexTimeout:           lifecycleRunnerTimeout,
		CodexTurnTimeout:       lifecycleCodexTurnTimeout,
		CodexIsolatedHome:      lifecycleCodexIsolatedHome,
	}
}

func runLifecycleInit(cmd *cobra.Command, args []string) error {
	return app.New(lifecycleRoot).LifecycleInit(cmd.OutOrStdout())
}

func runLifecycleEventAppend(cmd *cobra.Command, args []string) error {
	data, err := lifecycleEventInput(cmd)
	if err != nil {
		return err
	}
	return app.New(lifecycleRoot).LifecycleEventAppend(cmd.OutOrStdout(), data)
}

func runLifecycleStatusRefresh(cmd *cobra.Command, args []string) error {
	return app.New(lifecycleRoot).LifecycleStatusRefresh(cmd.OutOrStdout())
}

func runLifecycleAntipatternScan(cmd *cobra.Command, args []string) error {
	return app.New(lifecycleRoot).LifecycleAntipatternScan(cmd.OutOrStdout(), lifecycleAntipatternFormat)
}

func runLifecycleDaemonTick(cmd *cobra.Command, args []string) error {
	return app.New(lifecycleRoot).LifecycleDaemonTick(cmd.Context(), cmd.OutOrStdout(), lifecycleDaemonOptions())
}

func runLifecycleDaemonForeground(cmd *cobra.Command, args []string) error {
	return app.New(lifecycleRoot).LifecycleDaemonForeground(cmd.Context(), cmd.OutOrStdout(), lifecycleDaemonInterval, lifecycleDaemonOptions())
}

func runLifecycleDaemonStatus(cmd *cobra.Command, args []string) error {
	return app.New(lifecycleRoot).DaemonStatus(cmd.OutOrStdout(), daemonStatusLimit, daemonStatusJSON)
}

func runLifecycleDaemonPause(cmd *cobra.Command, args []string) error {
	return app.New(lifecycleRoot).DaemonPause(cmd.OutOrStdout(), daemonPauseReason)
}

func runLifecycleDaemonResume(cmd *cobra.Command, args []string) error {
	return app.New(lifecycleRoot).DaemonResume(cmd.OutOrStdout())
}

func runLifecycleRunnerCodexCheck(cmd *cobra.Command, args []string) error {
	return app.New(lifecycleRoot).LifecycleRunnerCodexCheck(cmd.Context(), cmd.OutOrStdout(), app.LifecycleCodexCheckInput{
		Command:      lifecycleCodexCommand,
		Timeout:      lifecycleRunnerTimeout,
		IsolatedHome: lifecycleCodexIsolatedHome,
	})
}

func runLifecycleRunnerCodexRun(cmd *cobra.Command, args []string) error {
	return app.New(lifecycleRoot).LifecycleRunnerCodexRun(cmd.Context(), cmd.OutOrStdout(), app.LifecycleCodexRunInput{
		Command:              lifecycleCodexCommand,
		Prompt:               lifecycleCodexPrompt,
		ProjectRoot:          lifecycleCodexProjectRoot,
		JobID:                lifecycleCodexJobID,
		JobSpec:              lifecycleCodexJobSpec,
		Loop:                 lifecycleCodexLoop,
		Timeout:              lifecycleRunnerTimeout,
		TurnTimeout:          lifecycleCodexTurnTimeout,
		MaxTurns:             lifecycleCodexMaxTurns,
		AgentTurn:            lifecycleCodexAgentTurn,
		AcknowledgeModelCost: lifecycleCodexAcknowledgeCost,
		IsolatedHome:         lifecycleCodexIsolatedHome,
	})
}
