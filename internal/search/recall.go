package search

import (
	"sort"

	"github.com/Grivn/mnemon/internal/model"
	"github.com/Grivn/mnemon/internal/store"
)

// Maximum traversal depth from each anchor point.
const maxTraversalDepth = 2

// RecallResult represents a recalled insight with its relevance score.
type RecallResult struct {
	Insight *model.Insight `json:"insight"`
	Score   float64        `json:"score"`
	Intent  Intent         `json:"intent"`
	Via     string         `json:"via,omitempty"` // how it was found
}

// IntentAwareRecall performs intent-aware retrieval:
// 1. Detect query intent
// 2. Keyword search to find anchor points
// 3. Multi-level BFS from anchors with intent-weighted score decay
// 4. Merge and rank results
func IntentAwareRecall(db *store.DB, query string, limit int) ([]RecallResult, error) {
	intent := DetectIntent(query)
	weights := GetWeights(intent)

	// Step 1: Get all active insights for keyword search
	all, err := db.GetAllActiveInsights()
	if err != nil {
		return nil, err
	}

	// Step 2: Keyword search for anchors
	anchors := KeywordSearch(all, query, 5)

	// Build score map: id -> best score found so far
	scoreMap := make(map[string]float64)
	viaMap := make(map[string]string)
	insightMap := make(map[string]*model.Insight)

	// Score anchors directly
	for _, a := range anchors {
		scoreMap[a.Insight.ID] = a.Score
		viaMap[a.Insight.ID] = "keyword"
		insightMap[a.Insight.ID] = a.Insight
	}

	// Step 3: Multi-level BFS from each anchor
	type bfsItem struct {
		id    string
		score float64 // accumulated score arriving at this node
		depth int
	}

	for _, a := range anchors {
		// BFS queue seeded with the anchor
		queue := []bfsItem{{id: a.Insight.ID, score: a.Score, depth: 0}}
		visited := map[string]bool{a.Insight.ID: true}

		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]

			if cur.depth >= maxTraversalDepth {
				continue
			}

			edges, err := db.GetEdgesByNode(cur.id)
			if err != nil {
				continue
			}

			for _, e := range edges {
				neighborID := e.TargetID
				if neighborID == cur.id {
					neighborID = e.SourceID
				}

				// Score decays: parent_score * intent_weight * edge_weight
				edgeWeight := weights[e.EdgeType]
				neighborScore := cur.score * edgeWeight * e.Weight

				// Update score map if this path is better
				if existing, ok := scoreMap[neighborID]; !ok || neighborScore > existing {
					scoreMap[neighborID] = neighborScore
					viaMap[neighborID] = string(e.EdgeType)
					// Load insight if not yet seen
					if _, loaded := insightMap[neighborID]; !loaded {
						ins, err := db.GetInsightByID(neighborID)
						if err == nil && ins != nil {
							insightMap[neighborID] = ins
						}
					}
				}

				// Continue BFS if not visited from this anchor
				if !visited[neighborID] {
					visited[neighborID] = true
					queue = append(queue, bfsItem{
						id:    neighborID,
						score: neighborScore,
						depth: cur.depth + 1,
					})
				}
			}
		}
	}

	// Step 4: Collect and sort results
	results := make([]RecallResult, 0, len(scoreMap))
	for id, score := range scoreMap {
		ins, ok := insightMap[id]
		if !ok {
			continue
		}
		results = append(results, RecallResult{
			Insight: ins,
			Score:   score,
			Intent:  intent,
			Via:     viaMap[id],
		})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].Insight.Importance > results[j].Insight.Importance
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}
