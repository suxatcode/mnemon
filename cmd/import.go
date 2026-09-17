package cmd

import (
	"os"

	"github.com/mnemon-dev/mnemon/internal/memorysvc"
	"github.com/mnemon-dev/mnemon/internal/remoteapi"
	"github.com/spf13/cobra"
)

var (
	importNoDiff bool
	importDryRun bool
)

var importCmd = &cobra.Command{
	Use:   "import <file>",
	Short: "Import a memory draft file",
	Long: `Import insights from a memory draft JSON file (schema_version: "1").

Each insight passes through Mnemon's normal write path: deduplication,
graph edge construction, embeddings, and lifecycle scoring are all applied
automatically.

The draft format and a reference LLM prompt for generating it from chat
exports are documented in docs/IMPORT.md.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := os.ReadFile(args[0])
		if err != nil {
			return err
		}
		if client, ok, err := defaultRemoteClient(); err != nil {
			return err
		} else if ok {
			defer client.Close()
			resp, err := client.Import(remoteapi.ImportRequest{Draft: data, NoDiff: importNoDiff, DryRun: importDryRun})
			if err != nil {
				return err
			}
			return printRemoteResponse(resp)
		}
		return withLocalService(func(svc *memorysvc.Service, actor memorysvc.Actor) error {
			return writeResult(svc.Import(actor, memorysvc.ImportInput{Draft: data, NoDiff: importNoDiff, DryRun: importDryRun}))
		})
	},
}

func init() {
	importCmd.Flags().BoolVar(&importNoDiff, "no-diff", false, "skip deduplication; insert all insights as new")
	importCmd.Flags().BoolVar(&importDryRun, "dry-run", false, "validate the draft file without writing to the database")
	rootCmd.AddCommand(importCmd)
}
