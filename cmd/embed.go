package cmd

import (
	"github.com/mnemon-dev/mnemon/internal/memorysvc"
	"github.com/mnemon-dev/mnemon/internal/remoteapi"
	"github.com/spf13/cobra"
)

var (
	embedAll    bool
	embedStatus bool
)

var embedCmd = &cobra.Command{
	Use:   "embed [id]",
	Short: "Generate embeddings for insights via Ollama",
	Long: `Generate embedding vectors for insights using a local Ollama model.

Modes:
  mnemon embed --status            Show embedding coverage statistics
  mnemon embed --all               Backfill embeddings for all un-embedded insights
  mnemon embed <id>                Generate embedding for a specific insight`,
	RunE: func(cmd *cobra.Command, args []string) error {
		id := ""
		if len(args) > 0 {
			id = args[0]
		}
		if client, ok, err := defaultRemoteClient(); err != nil {
			return err
		} else if ok {
			defer client.Close()
			resp, err := client.Embed(remoteapi.EmbedRequest{ID: id, All: embedAll, Status: embedStatus})
			if err != nil {
				return err
			}
			return printRemoteResponse(resp)
		}
		return withLocalService(func(svc *memorysvc.Service, actor memorysvc.Actor) error {
			return writeResult(svc.Embed(actor, memorysvc.EmbedInput{ID: id, All: embedAll, Status: embedStatus}))
		})
	},
}

func init() {
	embedCmd.Flags().BoolVar(&embedAll, "all", false, "backfill embeddings for all un-embedded insights")
	embedCmd.Flags().BoolVar(&embedStatus, "status", false, "show embedding coverage statistics")
	rootCmd.AddCommand(embedCmd)
}
