package cmd

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/mnemon-dev/mnemon/internal/memorysvc"
	"github.com/mnemon-dev/mnemon/internal/remoteapi"
	"github.com/spf13/cobra"
)

var searchLimit int

var searchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Search insights with token-based scoring",
	Long:  "Search insights using tokenized keyword matching. Returns results ranked by relevance score. On a team remote this is fully shared, same as recall.",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := strings.Join(args, " ")
		if err := requirePositiveLimit("--limit", searchLimit); err != nil {
			return err
		}
		if client, ok, err := defaultRemoteClient(); err != nil {
			return err
		} else if ok {
			defer client.Close()
			resp, err := client.Search(remoteapi.SearchRequest{Query: query, Limit: searchLimit})
			if err != nil {
				return err
			}
			return printRemoteResponse(resp)
		}
		return withLocalService(func(svc *memorysvc.Service, actor memorysvc.Actor) error {
			return writeResult(svc.Search(actor, memorysvc.SearchInput{Query: query, Limit: searchLimit}))
		})
	},
}

func init() {
	searchCmd.Flags().IntVar(&searchLimit, "limit", 10, "max results")
	rootCmd.AddCommand(searchCmd)
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show memory statistics",
	Long:  "Display aggregate statistics about stored insights and graph edges.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if client, ok, err := defaultRemoteClient(); err != nil {
			return err
		} else if ok {
			defer client.Close()
			resp, err := client.Status()
			if err != nil {
				return err
			}
			return printRemoteResponse(resp)
		}
		return withLocalService(func(svc *memorysvc.Service, actor memorysvc.Actor) error {
			return writeResult(svc.Status(actor))
		})
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

var forgetCmd = &cobra.Command{
	Use:   "forget [id]",
	Short: "Soft-delete an insight",
	Long:  "Mark an insight as deleted (soft delete). The data is preserved but excluded from queries.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		if client, ok, err := defaultRemoteClient(); err != nil {
			return err
		} else if ok {
			defer client.Close()
			resp, err := client.Forget(remoteapi.ForgetRequest{ID: id})
			if err != nil {
				return err
			}
			return printRemoteResponse(resp)
		}
		return withLocalService(func(svc *memorysvc.Service, actor memorysvc.Actor) error {
			return writeResult(svc.Forget(actor, memorysvc.ForgetInput{ID: id}))
		})
	},
}

func init() {
	rootCmd.AddCommand(forgetCmd)
}

var logLimit int

var logCmd = &cobra.Command{
	Use:   "log",
	Short: "Show recent operations",
	Long:  "Display the operation log showing what mnemon has been doing (remember, recall, forget, etc).",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requirePositiveLimit("--limit", logLimit); err != nil {
			return err
		}
		if client, ok, err := defaultRemoteClient(); err != nil {
			return err
		} else if ok {
			defer client.Close()
			resp, err := client.Log(remoteapi.LogRequest{Limit: logLimit})
			if err != nil {
				return err
			}
			return printRemoteResponse(resp)
		}
		return withLocalService(func(svc *memorysvc.Service, actor memorysvc.Actor) error {
			entries, err := svc.DB().GetOplog(logLimit)
			if err != nil {
				return fmt.Errorf("get oplog: %w", err)
			}
			if len(entries) == 0 {
				fmt.Println("No operations recorded yet.")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "TIME\tOP\tINSIGHT\tDETAIL\n")
			fmt.Fprintf(w, "----\t--\t-------\t------\n")
			for _, e := range entries {
				insightID := e.InsightID
				if len(insightID) > 8 {
					insightID = insightID[:8]
				}
				detail := e.Detail
				if len(detail) > 60 {
					detail = detail[:57] + "..."
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.CreatedAt, e.Operation, insightID, detail)
			}
			return w.Flush()
		})
	},
}

func init() {
	logCmd.Flags().IntVar(&logLimit, "limit", 20, "max entries to show")
	rootCmd.AddCommand(logCmd)
}
