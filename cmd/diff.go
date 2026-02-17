package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Grivn/mnemon/internal/search"
	"github.com/spf13/cobra"
)

var diffLimit int

var diffCmd = &cobra.Command{
	Use:   "diff [new content]",
	Short: "Check for duplicates or conflicts",
	Long:  "Compare new content against existing insights to detect duplicates, conflicts, or updates.",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		newFact := strings.Join(args, " ")

		db, err := openDB()
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer db.Close()

		all, err := db.GetAllActiveInsights()
		if err != nil {
			return fmt.Errorf("get insights: %w", err)
		}

		results := search.KeywordSearch(all, newFact, diffLimit)

		type similarItem struct {
			ID         string  `json:"id"`
			Content    string  `json:"content"`
			Similarity float64 `json:"similarity"`
			Suggestion string  `json:"suggestion"`
		}

		similar := make([]similarItem, 0)
		for _, r := range results {
			sim := search.ContentSimilarity(newFact, r.Insight.Content)
			suggestion := classifySuggestion(sim, newFact, r.Insight.Content)
			similar = append(similar, similarItem{
				ID:         r.Insight.ID,
				Content:    r.Insight.Content,
				Similarity: sim,
				Suggestion: suggestion,
			})
		}

		// Overall suggestion
		overallSuggestion := "ADD"
		if len(similar) > 0 {
			overallSuggestion = similar[0].Suggestion
		}

		db.LogOp("diff", "", fmt.Sprintf("suggestion=%s content=%s", overallSuggestion, newFact))

		output := map[string]interface{}{
			"new_fact":   newFact,
			"suggestion": overallSuggestion,
			"similar":    similar,
		}

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(output)
	},
}

// negationPatterns detects potential contradictions.
var negationWords = []string{
	"not", "no longer", "don't", "doesn't", "never", "switched from",
	"instead of", "rather than", "replaced", "deprecated",
	"不", "没有", "不再", "放弃", "替换", "取消",
}

func classifySuggestion(similarity float64, newText, existingText string) string {
	if similarity < 0.5 {
		return "ADD"
	}

	// Check for negation/conflict signals first (even at high similarity)
	newLower := strings.ToLower(newText)
	existLower := strings.ToLower(existingText)
	for _, neg := range negationWords {
		if strings.Contains(newLower, neg) || strings.Contains(existLower, neg) {
			return "CONFLICT"
		}
	}

	if similarity > 0.9 {
		return "DUPLICATE"
	}
	return "UPDATE"
}

func init() {
	diffCmd.Flags().IntVar(&diffLimit, "limit", 5, "max similar results to compare")
	rootCmd.AddCommand(diffCmd)
}
