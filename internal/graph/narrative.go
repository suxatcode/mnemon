package graph

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Grivn/mnemon/internal/model"
	"github.com/Grivn/mnemon/internal/search"
	"github.com/Grivn/mnemon/internal/store"
	"github.com/google/uuid"
)

// TimeRange represents the time span of a narrative cluster.
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// NarrativeCluster represents a group of temporally close, entity-overlapping insights.
type NarrativeCluster struct {
	Insights       []*model.Insight `json:"insights"`
	TimeRange      TimeRange        `json:"time_range"`
	SharedEntities []string         `json:"shared_entities"`
	SuggestedTitle string           `json:"suggested_title"`
}

// minTokenSimilarityForNarrative is the minimum token similarity to group
// insights without shared entities into a narrative cluster.
const minTokenSimilarityForNarrative = 0.15

// FindNarrativeClusters groups insights by time window and entity overlap.
// Returns clusters with >= minCluster members.
func FindNarrativeClusters(db *store.DB, window time.Duration, minCluster int) ([]NarrativeCluster, error) {
	all, err := db.GetAllActiveInsights()
	if err != nil {
		return nil, err
	}

	// Sort by created_at ascending
	sort.Slice(all, func(i, j int) bool {
		return all[i].CreatedAt.Before(all[j].CreatedAt)
	})

	// Filter out insights that are already part of a narrative
	var filtered []*model.Insight
	for _, ins := range all {
		if ins.Category == model.CategoryNarrative {
			continue
		}
		edges, err := db.GetEdgesByNodeAndType(ins.ID, model.EdgeNarrative)
		if err == nil && len(edges) > 0 {
			continue
		}
		filtered = append(filtered, ins)
	}

	if len(filtered) < minCluster {
		return nil, nil
	}

	// Sliding window grouping: group insights where consecutive gaps < window
	var groups [][]*model.Insight
	current := []*model.Insight{filtered[0]}

	for i := 1; i < len(filtered); i++ {
		gap := filtered[i].CreatedAt.Sub(filtered[i-1].CreatedAt)
		if gap < 0 {
			gap = -gap
		}
		if gap <= window {
			current = append(current, filtered[i])
		} else {
			if len(current) >= minCluster {
				groups = append(groups, current)
			}
			current = []*model.Insight{filtered[i]}
		}
	}
	if len(current) >= minCluster {
		groups = append(groups, current)
	}

	// For each group, further filter by entity overlap or token similarity
	var clusters []NarrativeCluster
	for _, group := range groups {
		cluster := buildCluster(group)
		if len(cluster.Insights) >= minCluster {
			clusters = append(clusters, cluster)
		}
	}

	return clusters, nil
}

// buildCluster filters a temporal group by entity overlap or content similarity
// and builds a NarrativeCluster.
func buildCluster(group []*model.Insight) NarrativeCluster {
	// Collect all entities across the group
	entityCount := make(map[string]int)
	for _, ins := range group {
		for _, e := range ins.Entities {
			entityCount[e]++
		}
	}

	// Find shared entities (appear in >= 2 insights)
	var shared []string
	for e, count := range entityCount {
		if count >= 2 {
			shared = append(shared, e)
		}
	}
	sort.Strings(shared)

	// Filter: keep insights that share entities or have token similarity
	var members []*model.Insight
	if len(shared) > 0 {
		// Keep insights that have at least one shared entity
		sharedSet := make(map[string]bool)
		for _, e := range shared {
			sharedSet[e] = true
		}
		for _, ins := range group {
			for _, e := range ins.Entities {
				if sharedSet[e] {
					members = append(members, ins)
					break
				}
			}
		}
		// Also include insights with high token similarity to members
		for _, ins := range group {
			found := false
			for _, m := range members {
				if m.ID == ins.ID {
					found = true
					break
				}
			}
			if found {
				continue
			}
			for _, m := range members {
				sim := search.ContentSimilarity(ins.Content, m.Content)
				if sim >= minTokenSimilarityForNarrative {
					members = append(members, ins)
					break
				}
			}
		}
	} else {
		// No shared entities — use pairwise token similarity
		// Keep insights that are similar to at least one other in the group
		for i, ins := range group {
			for j := i + 1; j < len(group); j++ {
				sim := search.ContentSimilarity(ins.Content, group[j].Content)
				if sim >= minTokenSimilarityForNarrative {
					members = append(members, ins)
					break
				}
			}
		}
	}

	if len(members) == 0 {
		members = group // fallback: keep the whole group
	}

	// Compute time range
	start := members[0].CreatedAt
	end := members[0].CreatedAt
	for _, m := range members[1:] {
		if m.CreatedAt.Before(start) {
			start = m.CreatedAt
		}
		if m.CreatedAt.After(end) {
			end = m.CreatedAt
		}
	}

	// Generate suggested title
	title := generateTitle(members, shared)

	return NarrativeCluster{
		Insights:       members,
		TimeRange:      TimeRange{Start: start, End: end},
		SharedEntities: shared,
		SuggestedTitle: title,
	}
}

// generateTitle creates a suggested title from shared entities and categories.
func generateTitle(members []*model.Insight, sharedEntities []string) string {
	// Collect categories
	catCount := make(map[string]int)
	for _, m := range members {
		catCount[string(m.Category)]++
	}
	var primaryCat string
	maxCount := 0
	for cat, count := range catCount {
		if count > maxCount {
			maxCount = count
			primaryCat = cat
		}
	}

	parts := []string{}
	if len(sharedEntities) > 0 {
		limit := 3
		if len(sharedEntities) < limit {
			limit = len(sharedEntities)
		}
		parts = append(parts, strings.Join(sharedEntities[:limit], ", "))
	}
	if primaryCat != "" {
		parts = append(parts, primaryCat)
	}
	parts = append(parts, fmt.Sprintf("(%d insights)", len(members)))

	return strings.Join(parts, " — ")
}

// CreateNarrativeNode creates a narrative insight and PART_OF edges for its members.
func CreateNarrativeNode(db *store.DB, title string, memberIDs []string) (*model.Insight, int, error) {
	now := time.Now().UTC()

	narrativeInsight := &model.Insight{
		ID:        generateID(),
		Content:   title,
		Category:  model.CategoryNarrative,
		Importance: 3,
		Tags:      []string{},
		Entities:  []string{},
		Source:    "system",
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := db.InsertInsight(narrativeInsight); err != nil {
		return nil, 0, fmt.Errorf("create narrative insight: %w", err)
	}

	edgeCount := 0
	for _, memberID := range memberIDs {
		// member → narrative (PART_OF)
		err := db.InsertEdge(&model.Edge{
			SourceID:  memberID,
			TargetID:  narrativeInsight.ID,
			EdgeType:  model.EdgeNarrative,
			Weight:    1.0,
			Metadata:  map[string]string{"sub_type": "part_of"},
			CreatedAt: now,
		})
		if err == nil {
			edgeCount++
		}

		// narrative → member (reverse for traversal)
		err = db.InsertEdge(&model.Edge{
			SourceID:  narrativeInsight.ID,
			TargetID:  memberID,
			EdgeType:  model.EdgeNarrative,
			Weight:    1.0,
			Metadata:  map[string]string{"sub_type": "contains"},
			CreatedAt: now,
		})
		if err == nil {
			edgeCount++
		}
	}

	return narrativeInsight, edgeCount, nil
}

func generateID() string {
	return uuid.New().String()
}
