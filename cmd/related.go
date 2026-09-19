package cmd

import (
	"fmt"

	"github.com/suxatcode/mnemon/internal/memorysvc"
	"github.com/suxatcode/mnemon/internal/model"
	"github.com/suxatcode/mnemon/internal/remoteapi"
	"github.com/spf13/cobra"
)

var (
	relEdgeType string
	relDepth    int
)

var relatedCmd = &cobra.Command{
	Use:   "related [id]",
	Short: "Find related insights via graph traversal",
	Long:  "BFS traversal from a given insight, optionally filtered by edge type.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		startID := args[0]
		if relEdgeType != "" {
			et := model.EdgeType(relEdgeType)
			if !model.ValidEdgeTypes[et] {
				return fmt.Errorf("invalid edge type %q; valid: temporal, semantic, causal, entity", relEdgeType)
			}
		}
		if client, ok, err := defaultRemoteClient(); err != nil {
			return err
		} else if ok {
			defer client.Close()
			resp, err := client.Related(remoteapi.RelatedRequest{ID: startID, EdgeType: relEdgeType, Depth: relDepth})
			if err != nil {
				return err
			}
			return printRemoteResponse(resp)
		}
		return withLocalService(func(svc *memorysvc.Service, actor memorysvc.Actor) error {
			return writeResult(svc.Related(actor, memorysvc.RelatedInput{ID: startID, EdgeType: relEdgeType, Depth: relDepth}))
		})
	},
}

func init() {
	relatedCmd.Flags().StringVar(&relEdgeType, "edge", "", "filter by edge type (temporal|semantic|causal|entity)")
	relatedCmd.Flags().IntVar(&relDepth, "depth", 2, "max traversal depth")
	rootCmd.AddCommand(relatedCmd)
}
