package search

import (
	"container/heap"
	"sort"

	"github.com/Grivn/mnemon/internal/embed"
	"github.com/Grivn/mnemon/internal/model"
	"github.com/Grivn/mnemon/internal/store"
)

// Beam search parameters (MAGMA-aligned).
const (
	beamWidth      = 10  // candidates retained per level
	maxDepth       = 5   // max traversal hops
	maxVisited     = 50  // total node budget
	anchorTopK     = 20  // per-signal anchor limit (MAGMA: 15-30)
	lambda1        = 0.6 // structural weight (MAGMA default)
	lambda2        = 0.4 // semantic weight (MAGMA default)
)

// RRF constant (standard value from Cormack et al. 2009).
const rrfK = 60

// RecallResult represents a recalled insight with its relevance score.
type RecallResult struct {
	Insight *model.Insight `json:"insight"`
	Score   float64        `json:"score"`
	Intent  Intent         `json:"intent"`
	Via     string         `json:"via,omitempty"`
}

// IntentAwareRecall performs MAGMA-aligned intent-aware retrieval:
// 1. Detect query intent
// 2. Multi-signal anchor selection via RRF (keyword + vector + time)
// 3. Beam search from anchors with additive transition scoring
// 4. Merge and rank results
func IntentAwareRecall(db *store.DB, query string, queryVec []float64, limit int) ([]RecallResult, error) {
	intent := DetectIntent(query)
	weights := GetWeights(intent)

	// Step 1: Get all active insights
	all, err := db.GetAllActiveInsights()
	if err != nil {
		return nil, err
	}

	// Step 2: Multi-signal anchor selection via RRF
	type anchor struct {
		insight *model.Insight
		score   float64
		via     string
	}
	anchorMap := make(map[string]*anchor)

	// Signal 1: Keyword search
	keywordAnchors := KeywordSearch(all, query, anchorTopK)
	for rank, a := range keywordAnchors {
		anchorMap[a.Insight.ID] = &anchor{
			insight: a.Insight,
			score:   1.0 / float64(rrfK+rank+1),
			via:     "keyword",
		}
	}

	// Signal 2: Vector search (when available)
	if queryVec != nil {
		vectorHits := vectorSearch(db, queryVec, anchorTopK)
		for rank, vh := range vectorHits {
			rrfScore := 1.0 / float64(rrfK+rank+1)
			if existing, ok := anchorMap[vh.id]; ok {
				existing.score += rrfScore
				existing.via = "hybrid"
			} else {
				ins, err := db.GetInsightByID(vh.id)
				if err != nil || ins == nil {
					continue
				}
				anchorMap[vh.id] = &anchor{
					insight: ins,
					score:   rrfScore,
					via:     "vector",
				}
			}
		}
	}

	// Signal 3: Time-based ranking (P5: MAGMA third RRF signal)
	// Sort all insights by recency, fuse via RRF
	timeSorted := make([]*model.Insight, len(all))
	copy(timeSorted, all)
	sort.Slice(timeSorted, func(i, j int) bool {
		return timeSorted[i].CreatedAt.After(timeSorted[j].CreatedAt)
	})
	timeLimit := anchorTopK
	if timeLimit > len(timeSorted) {
		timeLimit = len(timeSorted)
	}
	for rank := 0; rank < timeLimit; rank++ {
		ins := timeSorted[rank]
		rrfScore := 1.0 / float64(rrfK+rank+1)
		if existing, ok := anchorMap[ins.ID]; ok {
			existing.score += rrfScore
			if existing.via == "keyword" || existing.via == "vector" {
				existing.via = "hybrid"
			}
		} else {
			anchorMap[ins.ID] = &anchor{
				insight: ins,
				score:   rrfScore,
				via:     "time",
			}
		}
	}

	// Normalize anchor scores to [0, 1]
	var maxAnchorScore float64
	for _, a := range anchorMap {
		if a.score > maxAnchorScore {
			maxAnchorScore = a.score
		}
	}
	if maxAnchorScore > 0 {
		for _, a := range anchorMap {
			a.score /= maxAnchorScore
		}
	}

	// Initialize score map with anchors
	scoreMap := make(map[string]float64)
	viaMap := make(map[string]string)
	insightMap := make(map[string]*model.Insight)

	for id, a := range anchorMap {
		scoreMap[id] = a.score
		viaMap[id] = a.via
		insightMap[id] = a.insight
	}

	// Step 3: Beam search from each anchor (P0: replaces BFS)
	// MAGMA transition score: score_v = score_u + λ₁·φ(edgeType, intent) + λ₂·sim(v_neighbor, v_query)
	for id, a := range anchorMap {
		beamSearchFromAnchor(db, id, a.score, queryVec, weights, scoreMap, viaMap, insightMap)
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

// beamSearchFromAnchor performs beam search starting from a single anchor node.
// It uses a priority queue to keep the top beamWidth candidates at each depth level.
func beamSearchFromAnchor(
	db *store.DB,
	startID string,
	startScore float64,
	queryVec []float64,
	weights IntentWeights,
	scoreMap map[string]float64,
	viaMap map[string]string,
	insightMap map[string]*model.Insight,
) {
	visited := map[string]bool{startID: true}
	totalVisited := 1

	// Seed the beam with the anchor
	current := &beamHeap{{id: startID, score: startScore, depth: 0}}
	heap.Init(current)

	for depth := 0; depth < maxDepth; depth++ {
		if current.Len() == 0 || totalVisited >= maxVisited {
			break
		}

		// Collect all candidates for the next level
		next := &beamHeap{}
		heap.Init(next)

		// Process all nodes at the current level
		for current.Len() > 0 {
			cur := heap.Pop(current).(beamItem)
			if cur.depth != depth {
				// Put it back — it's for a future level
				heap.Push(current, cur)
				break
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

				// MAGMA transition score (P6): additive accumulation
				// score_v = score_u + λ₁·φ(edgeType, intent) + λ₂·sim(v_neighbor, v_query)
				structural := weights[e.EdgeType] * e.Weight // φ(edgeType, intent) * edge_weight
				semantic := 0.0
				if queryVec != nil {
					if blob, err := db.GetEmbedding(neighborID); err == nil && len(blob) > 0 {
						nVec := embed.DeserializeVector(blob)
						cosSim := embed.CosineSimilarity(queryVec, nVec)
						if cosSim > 0 {
							semantic = cosSim
						}
					}
				}
				neighborScore := cur.score + lambda1*structural + lambda2*semantic

				// Update global score map if this path is better
				if existing, ok := scoreMap[neighborID]; !ok || neighborScore > existing {
					scoreMap[neighborID] = neighborScore
					viaMap[neighborID] = string(e.EdgeType)
					if _, loaded := insightMap[neighborID]; !loaded {
						ins, err := db.GetInsightByID(neighborID)
						if err == nil && ins != nil {
							insightMap[neighborID] = ins
						}
					}
				}

				if !visited[neighborID] {
					visited[neighborID] = true
					totalVisited++
					heap.Push(next, beamItem{
						id:    neighborID,
						score: neighborScore,
						depth: depth + 1,
					})
				}
			}
		}

		// Prune beam: keep only top beamWidth candidates for next level
		pruned := &beamHeap{}
		heap.Init(pruned)
		for next.Len() > 0 && pruned.Len() < beamWidth {
			heap.Push(pruned, heap.Pop(next).(beamItem))
		}
		current = pruned
	}
}

// beamItem is a node in the beam search priority queue.
type beamItem struct {
	id    string
	score float64
	depth int
}

// beamHeap implements a max-heap for beam search (highest score first).
type beamHeap []beamItem

func (h beamHeap) Len() int            { return len(h) }
func (h beamHeap) Less(i, j int) bool  { return h[i].score > h[j].score } // max-heap
func (h beamHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *beamHeap) Push(x interface{}) { *h = append(*h, x.(beamItem)) }
func (h *beamHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}

// vectorHit is a vector search result.
type vectorHit struct {
	id         string
	similarity float64
}

// vectorSearch performs brute-force cosine similarity search over all embedded insights.
func vectorSearch(db *store.DB, queryVec []float64, limit int) []vectorHit {
	embedded, err := db.GetAllEmbeddings()
	if err != nil || len(embedded) == 0 {
		return nil
	}

	var hits []vectorHit
	for _, e := range embedded {
		vec := embed.DeserializeVector(e.Embedding)
		if vec == nil {
			continue
		}
		sim := embed.CosineSimilarity(queryVec, vec)
		if sim > 0.1 {
			hits = append(hits, vectorHit{id: e.ID, similarity: sim})
		}
	}

	sort.Slice(hits, func(i, j int) bool {
		return hits[i].similarity > hits[j].similarity
	})

	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}
