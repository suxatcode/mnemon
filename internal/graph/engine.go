package graph

import (
	"github.com/Grivn/mnemon/internal/model"
	"github.com/Grivn/mnemon/internal/store"
)

// EdgeStats tracks how many edges of each type were created.
type EdgeStats struct {
	Temporal int `json:"temporal"`
	Entity   int `json:"entity"`
	Causal   int `json:"causal"`
}

// Engine orchestrates automatic edge creation when insights are stored.
type Engine struct {
	db *store.DB
}

// NewEngine creates a new graph edge engine.
func NewEngine(db *store.DB) *Engine {
	return &Engine{db: db}
}

// OnInsightCreated runs all edge generators for a newly created insight.
// It also populates the insight's Entities field as a side effect.
func (e *Engine) OnInsightCreated(insight *model.Insight) EdgeStats {
	var stats EdgeStats

	// 1. Extract entities and update insight
	entities := ExtractEntities(insight.Content)
	if entities == nil {
		entities = []string{}
	}
	insight.Entities = entities

	// 2. Temporal backbone edge
	stats.Temporal = CreateTemporalEdge(e.db, insight)

	// 3. Entity co-occurrence edges
	stats.Entity = CreateEntityEdges(e.db, insight)

	// 4. Causal keyword edges
	stats.Causal = CreateCausalEdges(e.db, insight)

	return stats
}
