package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Grivn/mnemon/internal/graph"
	"github.com/spf13/cobra"
)

var (
	enrichEntities    string
	enrichRebuildEdges bool
)

var enrichCmd = &cobra.Command{
	Use:   "enrich <insight_id>",
	Short: "Supplement entities for an insight (Claude NER enrichment)",
	Long:  "Add entities to an existing insight, optionally rebuilding entity co-occurrence edges.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		insightID := args[0]

		if enrichEntities == "" {
			return fmt.Errorf("--entities is required (comma-separated entity names)")
		}

		var newEntities []string
		for _, e := range strings.Split(enrichEntities, ",") {
			e = strings.TrimSpace(e)
			if e != "" {
				newEntities = append(newEntities, e)
			}
		}
		if len(newEntities) == 0 {
			return fmt.Errorf("no valid entities provided")
		}

		db, err := openDB()
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer db.Close()

		// Verify insight exists
		insight, err := db.GetInsightByID(insightID)
		if err != nil || insight == nil {
			return fmt.Errorf("insight %s not found", insightID)
		}

		// Merge new entities into existing ones
		merged, err := db.MergeEntities(insightID, newEntities)
		if err != nil {
			return fmt.Errorf("merge entities: %w", err)
		}

		// Rebuild entity edges for new entities if requested
		edgesCreated := 0
		if enrichRebuildEdges {
			// Refresh insight with merged entities for edge creation
			insight.Entities = merged
			edgesCreated = graph.CreateEntityEdgesForNewEntities(db, insight, newEntities)
		}

		db.LogOp("enrich", insightID, fmt.Sprintf("added entities: %s", strings.Join(newEntities, ", ")))

		output := map[string]interface{}{
			"status":        "enriched",
			"id":            insightID,
			"added":         newEntities,
			"all_entities":  merged,
			"edges_created": edgesCreated,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(output)
	},
}

func init() {
	enrichCmd.Flags().StringVar(&enrichEntities, "entities", "", `comma-separated entity names (e.g. "React,TypeScript,Vercel")`)
	enrichCmd.Flags().BoolVar(&enrichRebuildEdges, "rebuild-edges", false, "rebuild entity co-occurrence edges for new entities")
	rootCmd.AddCommand(enrichCmd)
}
