package graph

import (
	"fmt"
	"time"

	"github.com/Grivn/mnemon/internal/model"
	"github.com/Grivn/mnemon/internal/store"
)

// contextNeighborK is the ±k positions for context neighbor edges (MAGMA §3.1 default).
const contextNeighborK = 3

// CreateContextNeighborEdges creates bidirectional temporal edges to insights
// within ±k sequence positions (MAGMA §3.1: context neighbor edges).
// Weight: 1.0 / (1.0 + distance * 0.5)
func CreateContextNeighborEdges(db *store.DB, insight *model.Insight, seqIdx int) int {
	neighbors, err := db.GetInsightsBySequenceRange(seqIdx, contextNeighborK, insight.ID)
	if err != nil || len(neighbors) == 0 {
		return 0
	}

	now := time.Now().UTC()
	count := 0

	for _, neighbor := range neighbors {
		neighborIdx, err := db.GetSequenceIndex(neighbor.ID)
		if err != nil || neighborIdx < 0 {
			continue
		}

		distance := seqIdx - neighborIdx
		if distance < 0 {
			distance = -distance
		}
		if distance == 0 {
			continue
		}

		weight := 1.0 / (1.0 + float64(distance)*0.5)
		meta := map[string]string{
			"sub_type":     "context_neighbor",
			"seq_distance": fmt.Sprintf("%d", distance),
		}

		// Bidirectional context neighbor edges (stored as temporal type)
		err = db.InsertEdge(&model.Edge{
			SourceID:  insight.ID,
			TargetID:  neighbor.ID,
			EdgeType:  model.EdgeTemporal,
			Weight:    weight,
			Metadata:  meta,
			CreatedAt: now,
		})
		if err == nil {
			count++
		}

		err = db.InsertEdge(&model.Edge{
			SourceID:  neighbor.ID,
			TargetID:  insight.ID,
			EdgeType:  model.EdgeTemporal,
			Weight:    weight,
			Metadata:  meta,
			CreatedAt: now,
		})
		if err == nil {
			count++
		}
	}

	return count
}
