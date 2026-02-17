package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Grivn/mnemon/internal/graph"
	"github.com/Grivn/mnemon/internal/store"
	"github.com/spf13/cobra"
)

var (
	consolWindow     string
	consolMinCluster int
	consolCreate     bool
	consolTitle      string
	consolMembers    string
)

var consolidateCmd = &cobra.Command{
	Use:   "consolidate",
	Short: "Find or create narrative clusters (MAGMA §4.1)",
	Long:  "Group temporally close, entity-overlapping insights into narrative nodes with PART_OF edges.",
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := openDB()
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer db.Close()

		if consolCreate {
			return runCreateNarrative(db)
		}
		return runSuggestNarratives(db)
	},
}

func runSuggestNarratives(db *store.DB) error {
	window, err := time.ParseDuration(consolWindow)
	if err != nil {
		return fmt.Errorf("invalid window %q: %w", consolWindow, err)
	}

	clusters, err := graph.FindNarrativeClusters(db, window, consolMinCluster)
	if err != nil {
		return fmt.Errorf("find clusters: %w", err)
	}
	if clusters == nil {
		clusters = []graph.NarrativeCluster{}
	}

	type clusterOutput struct {
		ClusterID      int      `json:"cluster_id"`
		TimeRange      graph.TimeRange `json:"time_range"`
		Insights       []map[string]interface{} `json:"insights"`
		SuggestedTitle string   `json:"suggested_title"`
		SharedEntities []string `json:"shared_entities"`
	}

	var clusterOutputs []clusterOutput
	for i, c := range clusters {
		var insOuts []map[string]interface{}
		var ids []string
		for _, ins := range c.Insights {
			insOuts = append(insOuts, map[string]interface{}{
				"id":       ins.ID,
				"content":  ins.Content,
				"category": ins.Category,
			})
			ids = append(ids, ins.ID)
		}
		clusterOutputs = append(clusterOutputs, clusterOutput{
			ClusterID:      i + 1,
			TimeRange:      c.TimeRange,
			Insights:       insOuts,
			SuggestedTitle: c.SuggestedTitle,
			SharedEntities: c.SharedEntities,
		})
		_ = ids
	}

	// Build action hints
	actions := map[string]string{}
	if len(clusters) > 0 {
		actions["create"] = `mnemon consolidate --create --title "<title>" --members "<id1>,<id2>,<id3>"`
	}

	db.LogOp("consolidate", "", fmt.Sprintf("found %d clusters", len(clusters)))

	output := map[string]interface{}{
		"clusters": clusterOutputs,
		"actions":  actions,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

func runCreateNarrative(db *store.DB) error {
	if consolTitle == "" {
		return fmt.Errorf("--title is required when using --create")
	}
	if consolMembers == "" {
		return fmt.Errorf("--members is required when using --create")
	}

	var memberIDs []string
	for _, id := range strings.Split(consolMembers, ",") {
		id = strings.TrimSpace(id)
		if id != "" {
			memberIDs = append(memberIDs, id)
		}
	}
	if len(memberIDs) < 2 {
		return fmt.Errorf("at least 2 member IDs are required")
	}

	// Validate all members exist
	for _, id := range memberIDs {
		ins, err := db.GetInsightByID(id)
		if err != nil || ins == nil {
			return fmt.Errorf("member insight %s not found", id)
		}
	}

	narrativeInsight, edgeCount, err := graph.CreateNarrativeNode(db, consolTitle, memberIDs)
	if err != nil {
		return fmt.Errorf("create narrative: %w", err)
	}

	db.LogOp("consolidate", narrativeInsight.ID, fmt.Sprintf("created narrative: %s (%d members)", consolTitle, len(memberIDs)))

	output := map[string]interface{}{
		"status":        "created",
		"narrative_id":  narrativeInsight.ID,
		"title":         consolTitle,
		"members":       memberIDs,
		"edges_created": edgeCount,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

func init() {
	consolidateCmd.Flags().StringVar(&consolWindow, "window", "72h", "time window for clustering")
	consolidateCmd.Flags().IntVar(&consolMinCluster, "min-cluster", 3, "minimum insights per cluster")
	consolidateCmd.Flags().BoolVar(&consolCreate, "create", false, "create a narrative node (requires --title and --members)")
	consolidateCmd.Flags().StringVar(&consolTitle, "title", "", "title for the narrative node")
	consolidateCmd.Flags().StringVar(&consolMembers, "members", "", "comma-separated insight IDs to include")
	rootCmd.AddCommand(consolidateCmd)
}
