package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Grivn/mnemon/internal/embed"
	"github.com/Grivn/mnemon/internal/graph"
	"github.com/Grivn/mnemon/internal/model"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

var (
	remCategory   string
	remImportance int
	remTags       string
	remSource     string
)

var rememberCmd = &cobra.Command{
	Use:   "remember [content]",
	Short: "Store a new insight",
	Long:  "Store a new insight into the memory graph with optional category, importance, and tags.",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		content := strings.Join(args, " ")

		cat := model.Category(remCategory)
		if !model.ValidCategories[cat] {
			return fmt.Errorf("invalid category %q; valid: preference, decision, fact, insight, context, general, narrative", remCategory)
		}
		if remImportance < 1 || remImportance > 5 {
			return fmt.Errorf("importance must be 1-5, got %d", remImportance)
		}

		var tags []string
		if remTags != "" {
			for _, t := range strings.Split(remTags, ",") {
				t = strings.TrimSpace(t)
				if t != "" {
					tags = append(tags, t)
				}
			}
		}
		if tags == nil {
			tags = []string{}
		}

		now := time.Now().UTC()
		insight := &model.Insight{
			ID:         uuid.New().String(),
			Content:    content,
			Category:   cat,
			Importance: remImportance,
			Tags:       tags,
			Entities:   []string{},
			Source:     remSource,
			CreatedAt:  now,
			UpdatedAt:  now,
		}

		db, err := openDB()
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer db.Close()

		if err := db.InsertInsight(insight); err != nil {
			return fmt.Errorf("insert insight: %w", err)
		}

		// Generate embedding BEFORE graph engine so semantic edges can use it
		embedded := false
		ec := embed.NewClient()
		if ec.Available() {
			if vec, err := ec.Embed(content); err == nil {
				blob := embed.SerializeVector(vec)
				if err := db.UpdateEmbedding(insight.ID, blob); err == nil {
					embedded = true
				}
			}
		}

		// Run graph edge engine (includes auto semantic edges when embedded)
		engine := graph.NewEngine(db)
		edgeStats := engine.OnInsightCreated(insight)

		// Update entities extracted by the engine
		if len(insight.Entities) > 0 {
			_ = db.UpdateEntities(insight.ID, insight.Entities)
		}

		// Find semantic candidates for Claude to evaluate (additional manual linking)
		semanticCandidates := graph.FindSemanticCandidates(db, insight)
		if semanticCandidates == nil {
			semanticCandidates = []graph.SemanticCandidate{}
		}

		// Find causal candidates for Claude to evaluate
		causalCandidates := graph.FindCausalCandidates(db, insight)
		if causalCandidates == nil {
			causalCandidates = []graph.CausalCandidate{}
		}

		db.LogOp("remember", insight.ID, insight.Content)

		output := map[string]interface{}{
			"id":                  insight.ID,
			"content":             insight.Content,
			"category":            insight.Category,
			"importance":          insight.Importance,
			"tags":                insight.Tags,
			"entities":            insight.Entities,
			"entity_hints":        "Auto-extracted by regex. Consider running `mnemon enrich " + insight.ID + " --entities \"X,Y\" --rebuild-edges` if important entities were missed.",
			"created_at":          insight.CreatedAt.Format(time.RFC3339),
			"edges_created":       edgeStats,
			"semantic_candidates": semanticCandidates,
			"causal_candidates":   causalCandidates,
			"embedded":            embedded,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(output)
	},
}

func init() {
	rememberCmd.Flags().StringVar(&remCategory, "cat", "general", "category (preference|decision|fact|insight|context|general)")
	rememberCmd.Flags().IntVar(&remImportance, "imp", 3, "importance (1-5)")
	rememberCmd.Flags().StringVar(&remTags, "tags", "", "comma-separated tags")
	rememberCmd.Flags().StringVar(&remSource, "source", "user", "source (user|agent|external)")
	rootCmd.AddCommand(rememberCmd)
}
