package main

import (
	"github.com/mnemon-dev/mnemon/harness/internal/app"
	"github.com/spf13/cobra"
)

var (
	supervisorRoot   string
	supervisorFormat string
	supervisorKind   string
)

var supervisorCmd = &cobra.Command{
	Use:   "supervisor",
	Short: "Pluggable advisory coordination supervisor (proposes only)",
	Long: "Read the coordination context and propose coordination changes. The\n" +
		"supervisor only PROPOSES: suggestions land as route=coordination proposals\n" +
		"in the review queue and mutate nothing directly. The brain is swappable by\n" +
		"--kind, not code; mutation happens later only via review → apply → audit.",
}

var supervisorContextCmd = &cobra.Command{
	Use:   "context",
	Short: "Show the supervisor read contract (coordination topology + open proposals)",
	RunE:  runSupervisorContext,
}

var supervisorProposeCmd = &cobra.Command{
	Use:   "propose",
	Short: "Run the configured supervisor; land route=coordination proposals for review",
	RunE:  runSupervisorPropose,
}

func init() {
	supervisorCmd.PersistentFlags().StringVar(&supervisorRoot, "root", ".", "project root for harness coordination state")
	supervisorContextCmd.Flags().StringVar(&supervisorFormat, "format", "json", "output format: json")
	supervisorProposeCmd.Flags().StringVar(&supervisorKind, "kind", "rule-standin", "supervisor kind (swappable by config); host-agent kinds run externally via the runner")
	supervisorCmd.AddCommand(supervisorContextCmd)
	supervisorCmd.AddCommand(supervisorProposeCmd)
	rootCmd.AddCommand(supervisorCmd)
}

func runSupervisorContext(cmd *cobra.Command, args []string) error {
	return app.New(supervisorRoot).CoordinationContext(cmd.OutOrStdout(), supervisorFormat)
}

func runSupervisorPropose(cmd *cobra.Command, args []string) error {
	return app.New(supervisorRoot).SupervisorPropose(cmd.OutOrStdout(), supervisorKind)
}
