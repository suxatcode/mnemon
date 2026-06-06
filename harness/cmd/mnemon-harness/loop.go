package main

import (
	"errors"
	"fmt"

	"github.com/mnemon-dev/mnemon/harness/internal/app"
	"github.com/spf13/cobra"
)

var (
	loopRoot            string
	loopProjectRoot     string
	loopPlanHost        string
	loopPlanLoops       []string
	loopPlanFormat      string
	loopPlanProjectRoot string
)

var loopCmd = &cobra.Command{
	Use:   "loop",
	Short: "Manage declaration-driven harness loops",
}

var loopValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate harness loop, host, and binding declarations",
	RunE:  runLoopValidate,
}

var loopPlanCmd = &cobra.Command{
	Use:   "plan --host HOST [--loop LOOP ...]",
	Short: "Print a declaration-driven loop projection plan",
	RunE:  runLoopPlan,
}

var loopInstallCmd = &cobra.Command{
	Use:                "install --host HOST --loop LOOP [--loop LOOP ...] [host options]",
	Short:              "Install loop projections into a host runtime",
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLoopProjector(cmd, "install", args)
	},
}

var loopDiffCmd = &cobra.Command{
	Use:                "diff --host HOST [--loop LOOP ...] [host options]",
	Short:              "Compare declared loop projections with a host runtime",
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLoopProjector(cmd, "diff", args)
	},
}

var loopReconcileCmd = &cobra.Command{
	Use:                "reconcile --host HOST [--loop LOOP ...] [host options]",
	Short:              "Repair managed loop projection drift",
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLoopProjector(cmd, "reconcile", args)
	},
}

var loopStatusCmd = &cobra.Command{
	Use:                "status --host HOST [--loop LOOP ...] [host options]",
	Short:              "Show loop projection status for a host runtime",
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLoopProjector(cmd, "status", args)
	},
}

var loopUninstallCmd = &cobra.Command{
	Use:                "uninstall --host HOST --loop LOOP [--loop LOOP ...] [host options]",
	Short:              "Uninstall loop projections from a host runtime",
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLoopProjector(cmd, "uninstall", args)
	},
}

func init() {
	loopCmd.PersistentFlags().StringVar(&loopRoot, "root", ".", "repository root containing harness declarations")
	loopPlanCmd.Flags().StringVar(&loopPlanHost, "host", "", "host runtime id")
	loopPlanCmd.Flags().StringArrayVar(&loopPlanLoops, "loop", nil, "loop id; may be repeated")
	loopPlanCmd.Flags().StringVar(&loopPlanProjectRoot, "project-root", "", "project root used as the host projection working directory")
	loopPlanCmd.Flags().StringVar(&loopPlanFormat, "format", "text", "output format: text or json")
	addLoopProjectionHelpFlags(loopInstallCmd)
	addLoopProjectionHelpFlags(loopDiffCmd)
	addLoopProjectionHelpFlags(loopReconcileCmd)
	addLoopProjectionHelpFlags(loopStatusCmd)
	addLoopProjectionHelpFlags(loopUninstallCmd)
	loopCmd.AddCommand(loopValidateCmd, loopPlanCmd, loopInstallCmd, loopDiffCmd, loopReconcileCmd, loopStatusCmd, loopUninstallCmd)
	loopCmd.GroupID = groupSpine
	rootCmd.AddCommand(loopCmd)
}

func addLoopProjectionHelpFlags(command *cobra.Command) {
	command.Flags().String("project-root", "", "project root used as the host projection working directory")
	command.Flags().String("host", "", "host runtime id")
	command.Flags().StringArray("loop", nil, "loop id; may be repeated")
}

func runLoopValidate(cmd *cobra.Command, args []string) error {
	lines, err := app.New(loopRoot).LoopValidate()
	if err != nil {
		return err
	}
	for _, line := range lines {
		fmt.Fprintln(cmd.OutOrStdout(), line)
	}
	return nil
}

func runLoopPlan(cmd *cobra.Command, args []string) error {
	return app.New(loopRoot).LoopPlan(cmd.OutOrStdout(), loopPlanProjectRoot, loopPlanHost, loopPlanLoops, loopPlanFormat)
}

func runLoopProjector(cmd *cobra.Command, action string, args []string) error {
	opts, err := parseLoopProjectorArgs(args)
	if err != nil {
		if errors.Is(err, errLoopHelp) {
			return cmd.Help()
		}
		return err
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = rootCmd.Context()
	}
	return app.New(opts.root).LoopProject(ctx, cmd.OutOrStdout(), cmd.ErrOrStderr(), action, opts.projectRoot, opts.host, opts.loops, opts.hostArgs)
}

type loopProjectorArgs struct {
	root        string
	projectRoot string
	host        string
	loops       []string
	hostArgs    []string
}

var errLoopHelp = errors.New("loop help requested")

func parseLoopProjectorArgs(args []string) (loopProjectorArgs, error) {
	parsed := loopProjectorArgs{
		root:        loopRoot,
		projectRoot: loopProjectRoot,
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-h", "--help":
			return parsed, errLoopHelp
		case "--":
			parsed.hostArgs = append(parsed.hostArgs, args[i+1:]...)
			return parsed, nil
		case "--root":
			if i+1 >= len(args) {
				return parsed, errors.New("missing value for --root")
			}
			parsed.root = args[i+1]
			i++
		case "--project-root":
			if i+1 >= len(args) {
				return parsed, errors.New("missing value for --project-root")
			}
			parsed.projectRoot = args[i+1]
			i++
		case "--host":
			if i+1 >= len(args) {
				return parsed, errors.New("missing value for --host")
			}
			parsed.host = args[i+1]
			i++
		case "--loop":
			if i+1 >= len(args) {
				return parsed, errors.New("missing value for --loop")
			}
			parsed.loops = append(parsed.loops, args[i+1])
			i++
		default:
			parsed.hostArgs = append(parsed.hostArgs, arg)
		}
	}
	return parsed, nil
}
