package graph

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/suxatcode/mnemon/internal/model"
	"github.com/suxatcode/mnemon/internal/store"
)

// Temporal proximity window in hours (MAGMA: 24h).
const temporalWindowHours = 24.0

// Maximum number of proximity edges to create.
const maxProximityEdges = 10

// CreateTemporalEdge creates a backbone temporal edge between the new insight
// and the most recent insight from the same source (MAGMA backbone chain),
// plus proximity edges to recent insights within a 24h window.
func CreateTemporalEdge(db *store.DB, insight *model.Insight) (int, error) {
	now := time.Now().UTC()
	count := 0

	// 1. Backbone chain: link to most recent from same source
	prev, err := db.GetLatestInsightBySource(insight.Source, insight.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}

	if prev != nil {
		if err := db.InsertEdge(&model.Edge{
			SourceID:  prev.ID,
			TargetID:  insight.ID,
			EdgeType:  model.EdgeTemporal,
			Weight:    1.0,
			Metadata:  map[string]string{"sub_type": "backbone", "direction": "precedes"},
			CreatedAt: now,
		}); err != nil {
			return count, err
		}
		count++

		if err := db.InsertEdge(&model.Edge{
			SourceID:  insight.ID,
			TargetID:  prev.ID,
			EdgeType:  model.EdgeTemporal,
			Weight:    1.0,
			Metadata:  map[string]string{"sub_type": "backbone", "direction": "succeeds"},
			CreatedAt: now,
		}); err != nil {
			return count, err
		}
		count++
	}

	// 2. Temporal proximity: link to recent insights within 24h window
	// Weight decays with time distance: w = 1/(1 + hours_diff) (MAGMA formula)
	recent, err := db.GetRecentInsightsInWindow(insight.ID, temporalWindowHours, maxProximityEdges)
	if err != nil {
		return count, err
	}
	if len(recent) == 0 {
		return count, nil
	}

	backboneID := ""
	if prev != nil {
		backboneID = prev.ID
	}

	for _, near := range recent {
		// Skip the backbone neighbor (already linked above)
		if near.ID == backboneID {
			continue
		}

		hoursDiff := insight.CreatedAt.Sub(near.CreatedAt).Hours()
		if hoursDiff < 0 {
			hoursDiff = -hoursDiff
		}
		weight := 1.0 / (1.0 + hoursDiff)

		if err := db.InsertEdge(&model.Edge{
			SourceID:  insight.ID,
			TargetID:  near.ID,
			EdgeType:  model.EdgeTemporal,
			Weight:    weight,
			Metadata:  map[string]string{"sub_type": "proximity", "hours_diff": fmt.Sprintf("%.2f", hoursDiff)},
			CreatedAt: now,
		}); err != nil {
			return count, err
		}
		count++

		if err := db.InsertEdge(&model.Edge{
			SourceID:  near.ID,
			TargetID:  insight.ID,
			EdgeType:  model.EdgeTemporal,
			Weight:    weight,
			Metadata:  map[string]string{"sub_type": "proximity", "hours_diff": fmt.Sprintf("%.2f", hoursDiff)},
			CreatedAt: now,
		}); err != nil {
			return count, err
		}
		count++
	}

	return count, nil
}
