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
	Use:   "setup --host HOST --loop LOOP --control-url URL --principal PRINCIPAL",
	Short: "Project a loop into a host runtime and wire the channel (binding + token + env)",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := app.New(setupRoot).Setup(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), app.SetupOptions{
			Host:        setupHost,
			Loops:       setupLoops,
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
	Short: "Report channel binding health for the project",
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
	Use:   "uninstall --host HOST --loop LOOP --principal PRINCIPAL",
	Short: "Uninstall loop projections and remove the principal's channel binding",
	RunE: func(cmd *cobra.Command, args []string) error {
		return app.New(setupRoot).SetupUninstall(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), app.SetupOptions{
			Host:        setupHost,
			Loops:       setupLoops,
			Principal:   setupPrincipal,
			ProjectRoot: setupProjectRoot,
		})
	},
}

func init() {
	setupCmd.PersistentFlags().StringVar(&setupRoot, "root", ".", "repository root containing harness declarations")
	setupCmd.PersistentFlags().StringVar(&setupProjectRoot, "project-root", "", "project root for host projection + channel artifacts (defaults to root)")
	setupCmd.PersistentFlags().StringVar(&setupHost, "host", "", "host runtime id")
	setupCmd.PersistentFlags().StringArrayVar(&setupLoops, "loop", nil, "loop id; may be repeated")
	setupCmd.PersistentFlags().StringVar(&setupPrincipal, "principal", "", "authenticated channel principal")

	setupCmd.Flags().StringVar(&setupControlURL, "control-url", "", "channel endpoint URL")
	setupCmd.Flags().StringVar(&setupActorKind, "actor-kind", "host-agent", "binding actor kind: host-agent or control-agent")
	setupCmd.Flags().BoolVar(&setupUseToken, "token", false, "generate + reference a bearer token file (vs trusted-header auth)")
	setupCmd.Flags().BoolVar(&setupDryRun, "dry-run", false, "print all projection + channel changes without writing")

	setupCmd.AddCommand(setupStatusCmd, setupUninstallCmd)
	setupCmd.GroupID = groupSpine
	rootCmd.AddCommand(setupCmd)
}
