package cmd

import (
	"fmt"
	"os"

	"github.com/mnemon-dev/mnemon/internal/memorysvc"
	"github.com/mnemon-dev/mnemon/internal/remoteapi"
	"github.com/spf13/cobra"
)

var vizFormat string
var vizOutput string

var vizCmd = &cobra.Command{
	Use:   "viz",
	Short: "Export knowledge graph for visualization",
	Long:  "Export the knowledge graph as DOT (Graphviz) or HTML (vis.js interactive) format.",
	RunE: func(cmd *cobra.Command, args []string) error {
		write := func(text string) error {
			if vizOutput == "" || vizOutput == "-" {
				fmt.Print(text)
				return nil
			}
			if err := os.WriteFile(vizOutput, []byte(text), 0644); err != nil {
				return fmt.Errorf("write file: %w", err)
			}
			fmt.Fprintf(os.Stderr, "written to %s\n", vizOutput)
			return nil
		}
		if client, ok, err := defaultRemoteClient(); err != nil {
			return err
		} else if ok {
			defer client.Close()
			resp, err := client.Viz(remoteapi.VizRequest{Format: vizFormat})
			if err != nil {
				return err
			}
			for _, w := range resp.Warnings {
				fmt.Fprintf(os.Stderr, "warning: %s\n", w)
			}
			return write(resp.Text)
		}
		return withLocalService(func(svc *memorysvc.Service, actor memorysvc.Actor) error {
			res, err := svc.Viz(actor, memorysvc.VizInput{Format: vizFormat})
			if err != nil {
				return err
			}
			for _, w := range res.Warnings {
				fmt.Fprintf(os.Stderr, "warning: %s\n", w)
			}
			return write(res.Text)
		})
	},
}

func init() {
	vizCmd.Flags().StringVar(&vizFormat, "format", "dot", "output format: dot or html")
	vizCmd.Flags().StringVarP(&vizOutput, "output", "o", "-", "output file (- for stdout)")
	rootCmd.AddCommand(vizCmd)
}
