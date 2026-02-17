package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Grivn/mnemon/internal/search"
	"github.com/Grivn/mnemon/internal/store"
	"github.com/spf13/cobra"
)

var (
	recCategory string
	recLimit    int
	recSource   string
	recSmart    bool
)

var recallCmd = &cobra.Command{
	Use:   "recall [keyword]",
	Short: "Retrieve insights by keyword",
	Long:  "Search for insights matching a keyword. Use --smart for intent-aware graph-enhanced retrieval.",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		keyword := strings.Join(args, " ")

		db, err := openDB()
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer db.Close()

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")

		if recSmart {
			// Intent-aware recall with graph traversal
			results, err := search.IntentAwareRecall(db, keyword, recLimit)
			if err != nil {
				return fmt.Errorf("smart recall: %w", err)
			}
			for _, r := range results {
				_ = db.IncrementAccessCount(r.Insight.ID)
			}
			db.LogOp("recall:smart", "", fmt.Sprintf("q=%s hits=%d", keyword, len(results)))
			return enc.Encode(results)
		}

		// Basic SQL LIKE recall
		results, err := db.QueryInsights(store.QueryFilter{
			Keyword:  keyword,
			Category: recCategory,
			Source:   recSource,
			Limit:    recLimit,
		})
		if err != nil {
			return fmt.Errorf("query insights: %w", err)
		}

		for _, r := range results {
			_ = db.IncrementAccessCount(r.ID)
		}
		db.LogOp("recall", "", fmt.Sprintf("q=%s hits=%d", keyword, len(results)))
		return enc.Encode(results)
	},
}

func init() {
	recallCmd.Flags().StringVar(&recCategory, "cat", "", "filter by category")
	recallCmd.Flags().IntVar(&recLimit, "limit", 10, "max results")
	recallCmd.Flags().StringVar(&recSource, "source", "", "filter by source")
	recallCmd.Flags().BoolVar(&recSmart, "smart", false, "use intent-aware graph-enhanced recall")
	rootCmd.AddCommand(recallCmd)
}
