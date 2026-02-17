package graph

import (
	"github.com/Grivn/mnemon/internal/model"
	"github.com/Grivn/mnemon/internal/store"
)

// EdgeStats tracks how many edges of each type were created.
type EdgeStats struct {
	Temporal        int `json:"temporal"`
	ContextNeighbor int `json:"context_neighbor"`
	Entity          int `json:"entity"`
	Causal          int `json:"causal"`
	Semantic        int `json:"semantic"`
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

	// 2.5. Context neighbor edges (MAGMA §3.1)
	seqIdx, _ := e.db.GetNextSequenceIndex()
	_ = e.db.UpdateSequenceIndex(insight.ID, seqIdx)
	stats.ContextNeighbor = CreateContextNeighborEdges(e.db, insight, seqIdx)

	// 3. Entity co-occurrence edges
	stats.Entity = CreateEntityEdges(e.db, insight)

	// 4. Causal keyword edges
	stats.Causal = CreateCausalEdges(e.db, insight)

	// 5. Auto semantic edges (when embeddings available)
	stats.Semantic = CreateSemanticEdges(e.db, insight)

	return stats
}
