package main

import (
	"fmt"

	"github.com/mnemon-dev/mnemon/harness/internal/app"
	"github.com/spf13/cobra"
)

var (
	setupRoot        string
	setupProjectRoot string
	setupHost        string
	setupLoops       []string
	setupMemory      bool
	setupSkills      bool
	setupPrincipal   string
	setupControlURL  string
	setupActorKind   string
	setupUseToken    bool
	setupDryRun      bool
)

// setup is the everyday install front door (P4): it wraps the declaration-driven `loop install`
// projector (no second projector) and additionally wires the channel — the binding manifest entry,
// an optional bearer token file, and the runtime env (MNEMON_CONTROL_* / MNEMON_HARNESS_BIN) — so a
// projected host agent reaches the governed control plane through one channel.
var setupCmd = &cobra.Command{
	Use:   "setup --host HOST (--memory | --skills | --loop LOOP) --control-url URL --principal PRINCIPAL",
	Short: "Install Agent Integration for memory and skill",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := app.New(setupRoot).Setup(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), app.SetupOptions{
			Host:        setupHost,
			Loops:       selectedSetupLoops(),
			ControlURL:  setupControlURL,
			Principal:   setupPrincipal,
			ActorKind:   setupActorKind,
			UseToken:    setupUseToken,
			ProjectRoot: setupProjectRoot,
			DryRun:      setupDryRun,
		})
		return err
	},
}

var setupStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Report Agent Integration setup health",
	RunE: func(cmd *cobra.Command, args []string) error {
		lines, err := app.New(setupRoot).SetupStatus(setupProjectRoot, setupPrincipal)
		if err != nil {
			return err
		}
		for _, l := range lines {
			fmt.Fprintln(cmd.OutOrStdout(), l)
		}
		return nil
	},
}

var setupUninstallCmd = &cobra.Command{
	Use:   "uninstall --host HOST (--memory | --skills | --loop LOOP) --principal PRINCIPAL",
	Short: "Uninstall Agent Integration assets for a principal",
	RunE: func(cmd *cobra.Command, args []string) error {
		return app.New(setupRoot).SetupUninstall(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), app.SetupOptions{
			Host:        setupHost,
			Loops:       selectedSetupLoops(),
			Principal:   setupPrincipal,
			ProjectRoot: setupProjectRoot,
		})
	},
}

func init() {
	setupCmd.PersistentFlags().StringVar(&setupRoot, "root", ".", "repository root containing harness declarations")
	setupCmd.PersistentFlags().StringVar(&setupProjectRoot, "project-root", "", "project root for Agent Integration artifacts (defaults to root)")
	setupCmd.PersistentFlags().StringVar(&setupHost, "host", "", "Agent Integration host id")
	setupCmd.PersistentFlags().StringArrayVar(&setupLoops, "loop", nil, "integration id; may be repeated")
	setupCmd.PersistentFlags().BoolVar(&setupMemory, "memory", false, "install memory Agent Integration")
	setupCmd.PersistentFlags().BoolVar(&setupSkills, "skills", false, "install skill Agent Integration")
	setupCmd.PersistentFlags().StringVar(&setupPrincipal, "principal", "", "Agent Integration principal")

	setupCmd.Flags().StringVar(&setupControlURL, "control-url", "", "Local Mnemon endpoint URL")
	setupCmd.Flags().StringVar(&setupActorKind, "actor-kind", "host-agent", "agent kind: host-agent or control-agent")
	setupCmd.Flags().BoolVar(&setupUseToken, "token", false, "generate a local access token")
	setupCmd.Flags().BoolVar(&setupDryRun, "dry-run", false, "print changes without writing")

	setupCmd.AddCommand(setupStatusCmd, setupUninstallCmd)
	setupCmd.GroupID = groupSpine
	rootCmd.AddCommand(setupCmd)
}

func selectedSetupLoops() []string {
	seen := map[string]bool{}
	var loops []string
	add := func(loop string) {
		if loop == "" || seen[loop] {
			return
		}
		seen[loop] = true
		loops = append(loops, loop)
	}
	for _, loop := range setupLoops {
		add(loop)
	}
	if setupMemory {
		add("memory")
	}
	if setupSkills {
		add("skill")
	}
	return loops
}
