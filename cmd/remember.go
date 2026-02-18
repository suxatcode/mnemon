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
	"github.com/Grivn/mnemon/internal/store"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

var (
	remCategory   string
	remImportance int
	remTags       string
	remSource     string
	remEntities   string
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
			return fmt.Errorf("invalid category %q; valid: preference, decision, fact, insight, context, general", remCategory)
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

		var entities []string
		if remEntities != "" {
			for _, e := range strings.Split(remEntities, ",") {
				e = strings.TrimSpace(e)
				if e != "" {
					entities = append(entities, e)
				}
			}
		}
		if entities == nil {
			entities = []string{}
		}

		now := time.Now().UTC()
		insight := &model.Insight{
			ID:         uuid.New().String(),
			Content:    content,
			Category:   cat,
			Importance: remImportance,
			Tags:       tags,
			Entities:   entities,
			Source:     remSource,
			CreatedAt:  now,
			UpdatedAt:  now,
		}

		db, err := openDB()
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer db.Close()

		// 1. Compute embedding BEFORE the transaction (HTTP call should not hold a DB lock)
		var embeddingBlob []byte
		ec := embed.NewClient()
		if ec.Available() {
			if vec, err := ec.Embed(content); err == nil {
				embeddingBlob = embed.SerializeVector(vec)
			}
		}

		// 2. All DB writes in a single atomic transaction
		var (
			edgeStats graph.EdgeStats
			ei        float64
			pruned    int
			embedded  bool
		)
		err = db.InTransaction(func() error {
			if err := db.InsertInsight(insight); err != nil {
				return fmt.Errorf("insert insight: %w", err)
			}

			if embeddingBlob != nil {
				if err := db.UpdateEmbedding(insight.ID, embeddingBlob); err != nil {
					return fmt.Errorf("update embedding: %w", err)
				}
				embedded = true
			}

			// Run graph edge engine (includes auto semantic edges when embedded)
			engine := graph.NewEngine(db)
			edgeStats = engine.OnInsightCreated(insight)

			// Update entities extracted by the engine
			if len(insight.Entities) > 0 {
				_ = db.UpdateEntities(insight.ID, insight.Entities)
			}

			// Compute and store effective_importance (after edges are created)
			var eiErr error
			ei, eiErr = db.RefreshEffectiveImportance(insight.ID)
			if eiErr != nil {
				fmt.Fprintf(os.Stderr, "warning: refresh EI: %v\n", eiErr)
			}

			// Auto-prune if over capacity (excludeID protects the just-created insight)
			var pruneErr error
			pruned, pruneErr = db.AutoPrune(store.MaxInsights, insight.ID)
			if pruneErr != nil {
				fmt.Fprintf(os.Stderr, "warning: auto-prune: %v\n", pruneErr)
			}

			db.LogOp("remember", insight.ID, insight.Content)
			return nil
		})
		if err != nil {
			return err
		}

		// 3. Read-only operations outside the transaction (data already committed)
		semanticCandidates := graph.FindSemanticCandidates(db, insight)
		if semanticCandidates == nil {
			semanticCandidates = []graph.SemanticCandidate{}
		}

		causalCandidates := graph.FindCausalCandidates(db, insight)
		if causalCandidates == nil {
			causalCandidates = []graph.CausalCandidate{}
		}

		output := map[string]interface{}{
			"id":                  insight.ID,
			"content":             insight.Content,
			"category":            insight.Category,
			"importance":          insight.Importance,
			"tags":                insight.Tags,
			"entities":            insight.Entities,
			"created_at":          insight.CreatedAt.Format(time.RFC3339),
			"edges_created":       edgeStats,
			"semantic_candidates": semanticCandidates,
			"causal_candidates":   causalCandidates,
			"embedded":              embedded,
			"effective_importance":  ei,
			"auto_pruned":          pruned,
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
	rememberCmd.Flags().StringVar(&remEntities, "entities", "", "comma-separated entities (LLM-extracted, merged with auto-extraction)")
	rootCmd.AddCommand(rememberCmd)
}
