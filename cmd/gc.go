package cmd

import (
	"github.com/suxatcode/mnemon/internal/memorysvc"
	"github.com/suxatcode/mnemon/internal/remoteapi"
	"github.com/spf13/cobra"
)

var (
	gcThreshold float64
	gcLimit     int
	gcKeepID    string
)

var gcCmd = &cobra.Command{
	Use:   "gc",
	Short: "Review memory retention and suggest cleanup",
	Long: `Garbage collection for memory insights. Two modes:

Suggest mode (default):
  mnemon gc [--threshold 0.5] [--limit 20]
  Lists non-immune insights with effective_importance below threshold.
  Immune insights (importance >= 4 or access_count >= 3) are never listed.

Keep mode:
	mnemon gc --keep <id>
  Boosts an insight's retention (access_count +3, refreshes timestamp).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requirePositiveLimit("--limit", gcLimit); err != nil {
			return err
		}
		if err := requireNonNegativeFloat("--threshold", gcThreshold); err != nil {
			return err
		}
		if client, ok, err := defaultRemoteClient(); err != nil {
			return err
		} else if ok {
			defer client.Close()
			resp, err := client.GC(remoteapi.GCRequest{Threshold: gcThreshold, Limit: gcLimit, KeepID: gcKeepID})
			if err != nil {
				return err
			}
			return printRemoteResponse(resp)
		}
		return withLocalService(func(svc *memorysvc.Service, actor memorysvc.Actor) error {
			return writeResult(svc.GC(actor, memorysvc.GCInput{Threshold: gcThreshold, Limit: gcLimit, KeepID: gcKeepID}))
		})
	},
}

func init() {
	gcCmd.Flags().Float64Var(&gcThreshold, "threshold", 0.5, "effective_importance threshold (insights below this are candidates)")
	gcCmd.Flags().IntVar(&gcLimit, "limit", 20, "max candidates to return")
	gcCmd.Flags().StringVar(&gcKeepID, "keep", "", "boost retention for this insight ID")
	rootCmd.AddCommand(gcCmd)
}
