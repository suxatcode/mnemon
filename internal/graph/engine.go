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
	Semantic int `json:"semantic"`
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
// It merges any pre-provided entities (e.g. from LLM) with regex-extracted ones.
func (e *Engine) OnInsightCreated(insight *model.Insight) EdgeStats {
	var stats EdgeStats

	// 1. Extract entities via regex+dictionary, merge with pre-provided (LLM-extracted)
	extracted := ExtractEntities(insight.Content)
	insight.Entities = mergeEntities(insight.Entities, extracted)

	// 2. Temporal backbone + proximity edges
	stats.Temporal = CreateTemporalEdge(e.db, insight)

	// 3. Entity co-occurrence edges
	stats.Entity = CreateEntityEdges(e.db, insight)

	// 4. Causal keyword edges
	stats.Causal = CreateCausalEdges(e.db, insight)

	// 5. Auto semantic edges (when embeddings available)
	stats.Semantic = CreateSemanticEdges(e.db, insight)

	return stats
}
