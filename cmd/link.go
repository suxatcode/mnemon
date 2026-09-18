package cmd

import (
	"fmt"

	"github.com/mnemon-dev/mnemon/internal/memorysvc"
	"github.com/mnemon-dev/mnemon/internal/model"
	"github.com/mnemon-dev/mnemon/internal/remoteapi"
	"github.com/spf13/cobra"
)

var (
	linkType   string
	linkWeight float64
	linkMeta   string
)

var linkCmd = &cobra.Command{
	Use:   "link <source_id> <target_id>",
	Short: "Create or update an edge between two insights",
	Long: `Create or update a typed edge between two insights. Used by Claude to create semantic edges after evaluating candidates.

Link is not owner-scoped: any two insights the caller can recall may be linked, including another person's. Forget, GC, and --keep remain owner-scoped.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		sourceID := args[0]
		targetID := args[1]
		edgeType := model.EdgeType(linkType)
		if !model.ValidEdgeTypes[edgeType] {
			return fmt.Errorf("invalid edge type %q; valid: temporal, semantic, causal, entity", linkType)
		}
		if linkWeight < 0.0 || linkWeight > 1.0 {
			return fmt.Errorf("weight must be between 0.0 and 1.0, got %.2f", linkWeight)
		}
		if client, ok, err := defaultRemoteClient(); err != nil {
			return err
		} else if ok {
			defer client.Close()
			resp, err := client.Link(remoteapi.LinkRequest{
				SourceID: sourceID, TargetID: targetID, Type: linkType, Weight: linkWeight, MetaJSON: linkMeta,
			})
			if err != nil {
				return err
			}
			return printRemoteResponse(resp)
		}
		return withLocalService(func(svc *memorysvc.Service, actor memorysvc.Actor) error {
			return writeResult(svc.Link(actor, memorysvc.LinkInput{
				SourceID: sourceID, TargetID: targetID, Type: linkType, Weight: linkWeight, MetaJSON: linkMeta,
			}))
		})
	},
}

func init() {
	linkCmd.Flags().StringVar(&linkType, "type", "semantic", "edge type (temporal|semantic|causal|entity)")
	linkCmd.Flags().Float64Var(&linkWeight, "weight", 0.5, "edge weight (0.0-1.0)")
	linkCmd.Flags().StringVar(&linkMeta, "meta", "", `optional metadata JSON (e.g. '{"reason":"similar topic"}')`)
	rootCmd.AddCommand(linkCmd)
}
