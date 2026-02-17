package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Grivn/mnemon/internal/embed"
	"github.com/Grivn/mnemon/internal/graph"
	"github.com/Grivn/mnemon/internal/search"
	"github.com/Grivn/mnemon/internal/store"
	"github.com/spf13/cobra"
)

var (
	recCategory string
	recLimit    int
	recSource   string
	recSmart    bool
	recIntent   string
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
			// Parse intent override
			var intentOverride *search.Intent
			if recIntent != "" {
				parsed, err := search.IntentFromString(recIntent)
				if err != nil {
					return err
				}
				intentOverride = &parsed
			}

			// Try to get query embedding for hybrid search
			var queryVec []float64
			ec := embed.NewClient()
			if ec.Available() {
				queryVec, _ = ec.Embed(keyword)
			}

			// Extract query entities at cmd layer (avoid graph→search circular dep)
			queryEntities := graph.ExtractEntities(keyword)

			// Intent-aware recall with graph traversal (+ optional vector search)
			resp, err := search.IntentAwareRecall(db, keyword, queryVec, queryEntities, recLimit, intentOverride)
			if err != nil {
				return fmt.Errorf("smart recall: %w", err)
			}
			for _, r := range resp.Results {
				_ = db.IncrementAccessCount(r.Insight.ID)
			}
			db.LogOp("recall:smart", "", fmt.Sprintf("q=%s hits=%d", keyword, len(resp.Results)))
			return enc.Encode(resp)
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
	recallCmd.Flags().StringVar(&recIntent, "intent", "", "override intent (WHY|WHEN|ENTITY|GENERAL)")
	rootCmd.AddCommand(recallCmd)
}
