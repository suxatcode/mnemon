package graph

import (
	"time"

	"github.com/Grivn/mnemon/internal/model"
	"github.com/Grivn/mnemon/internal/store"
)

// CreateTemporalEdge creates a backbone temporal edge between the new insight
// and the most recent insight from the same source (MAGMA backbone chain).
func CreateTemporalEdge(db *store.DB, insight *model.Insight) int {
	prev, err := db.GetLatestInsightBySource(insight.Source, insight.ID)
	if err != nil || prev == nil {
		return 0
	}

	now := time.Now().UTC()
	count := 0

	// prev → new (PRECEDES)
	err = db.InsertEdge(&model.Edge{
		SourceID:  prev.ID,
		TargetID:  insight.ID,
		EdgeType:  model.EdgeTemporal,
		Weight:    1.0,
		Metadata:  map[string]string{"sub_type": "backbone", "direction": "precedes"},
		CreatedAt: now,
	})
	if err == nil {
		count++
	}

	// new → prev (SUCCEEDS)
	err = db.InsertEdge(&model.Edge{
		SourceID:  insight.ID,
		TargetID:  prev.ID,
		EdgeType:  model.EdgeTemporal,
		Weight:    1.0,
		Metadata:  map[string]string{"sub_type": "backbone", "direction": "succeeds"},
		CreatedAt: now,
	})
	if err == nil {
		count++
	}

	return count
}
