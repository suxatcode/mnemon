package cmd

import (
	"strings"

	"github.com/mnemon-dev/mnemon/internal/memorysvc"
	"github.com/mnemon-dev/mnemon/internal/remoteapi"
	"github.com/spf13/cobra"
)

var (
	recCategory string
	recLimit    int
	recSource   string
	recBasic    bool
	recSmart    bool //nolint:unused // deprecated: smart is now the default; kept for backward compat
	recIntent   string
	recVerbose  bool
)

var recallCmd = &cobra.Command{
	Use:   "recall [keyword]",
	Short: "Retrieve insights by keyword",
	Long: `Search for insights using intent-aware graph-enhanced retrieval. Use --basic for simple SQL LIKE matching.

On a team remote, recall is fully shared: you will see other principals' personal notes plus org layer. Attribute hits by owner_principal and layer. Use --local to search only this machine.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		keyword := strings.Join(args, " ")
		if err := requirePositiveLimit("--limit", recLimit); err != nil {
			return err
		}
		input := memorysvc.RecallInput{
			Query: keyword, Category: recCategory, Limit: recLimit, Source: recSource,
			Basic: recBasic, Intent: recIntent, Verbose: recVerbose,
		}
		if client, ok, err := defaultRemoteClient(); err != nil {
			return err
		} else if ok {
			defer client.Close()
			resp, err := client.Recall(remoteapi.RecallRequest{
				Query: input.Query, Category: input.Category, Limit: input.Limit, Source: input.Source,
				Basic: input.Basic, Intent: input.Intent, Verbose: input.Verbose,
			})
			if err != nil {
				return err
			}
			return printRemoteResponse(resp)
		}
		return withLocalService(func(svc *memorysvc.Service, actor memorysvc.Actor) error {
			return writeResult(svc.Recall(actor, input))
		})
	},
}

func init() {
	recallCmd.Flags().StringVar(&recCategory, "cat", "", "filter by category")
	recallCmd.Flags().IntVar(&recLimit, "limit", 10, "max results")
	recallCmd.Flags().StringVar(&recSource, "source", "", "filter by source")
	recallCmd.Flags().BoolVar(&recBasic, "basic", false, "use simple SQL LIKE matching instead of smart recall")
	recallCmd.Flags().BoolVar(&recSmart, "smart", false, "deprecated: smart is now the default")
	_ = recallCmd.Flags().MarkHidden("smart")
	recallCmd.Flags().StringVar(&recIntent, "intent", "", "override intent (WHY|WHEN|ENTITY|GENERAL)")
	recallCmd.Flags().BoolVar(&recVerbose, "verbose", false, "output full recall response (signals, meta, timestamps)")
	rootCmd.AddCommand(recallCmd)
}
