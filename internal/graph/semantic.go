package graph

import (
	"fmt"
	"sort"
	"time"

	"github.com/mnemon-dev/mnemon/internal/embed"
	"github.com/mnemon-dev/mnemon/internal/model"
	"github.com/mnemon-dev/mnemon/internal/search"
	"github.com/mnemon-dev/mnemon/internal/store"
)

// Minimum similarity to be considered a semantic candidate (token overlap fallback).
const minSemanticSimilarity = 0.10

// Minimum cosine similarity to appear as a review candidate (grey zone lower bound).
const reviewSemanticThreshold = 0.40

// Minimum cosine similarity to auto-create a semantic edge (high confidence).
const autoSemanticThreshold = 0.80

// Maximum number of semantic candidates to return.
const maxSemanticCandidates = 5

// Maximum number of auto-created semantic edges per insight.
const maxAutoSemanticEdges = 3

// SemanticCandidate represents a potential semantic link for Claude to evaluate.
// When AutoLinked is true, the edge was already created automatically (high confidence).
// When false, the candidate is in the review zone and needs LLM judgment.
type SemanticCandidate struct {
	ID         string  `json:"id"`
	Content    string  `json:"content"`
	Category   string  `json:"category"`
	Similarity float64 `json:"similarity"`
	AutoLinked bool    `json:"auto_linked"`
}

// CreateSemanticEdges auto-creates semantic edges for insights with high
// embedding cosine similarity (MAGMA §3.2: cos(v_i, v_j) > θ_sim).
// Returns the number of edges created.
func CreateSemanticEdges(db *store.DB, insight *model.Insight) int {
	blob, err := db.GetEmbedding(insight.ID)
	if err != nil || len(blob) == 0 {
		return 0
	}
	insightVec := embed.DeserializeVector(blob)
	if insightVec == nil {
		return 0
	}

	allEmbedded, err := db.GetAllEmbeddings()
	if err != nil || len(allEmbedded) == 0 {
		return 0
	}

	type scored struct {
		id         string
		similarity float64
	}
	var candidates []scored
	for _, other := range allEmbedded {
		if other.ID == insight.ID {
			continue
		}
		otherVec := embed.DeserializeVector(other.Embedding)
		if otherVec == nil {
			continue
		}
		cosSim := embed.CosineSimilarity(insightVec, otherVec)
		if cosSim >= autoSemanticThreshold {
			candidates = append(candidates, scored{id: other.ID, similarity: cosSim})
		}
	}

	if len(candidates) == 0 {
		return 0
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].similarity > candidates[j].similarity
	})
	if len(candidates) > maxAutoSemanticEdges {
		candidates = candidates[:maxAutoSemanticEdges]
	}

	now := time.Now().UTC()
	count := 0
	for _, c := range candidates {
		// Bidirectional semantic edges (MAGMA: undirected)
		meta := map[string]string{
			"created_by": "auto",
			"cosine":     fmt.Sprintf("%.4f", c.similarity),
		}
		err1 := db.InsertEdge(&model.Edge{
			SourceID: insight.ID, TargetID: c.id,
			EdgeType: model.EdgeSemantic, Weight: c.similarity,
			Metadata: meta, CreatedAt: now,
		})
		err2 := db.InsertEdge(&model.Edge{
			SourceID: c.id, TargetID: insight.ID,
			EdgeType: model.EdgeSemantic, Weight: c.similarity,
			Metadata: meta, CreatedAt: now,
		})
		if err1 == nil {
			count++
		}
		if err2 == nil {
			count++
		}
	}
	return count
}

// FindSemanticCandidates returns insights that are potential semantic matches
// for the given insight. When embeddings are available, uses cosine similarity
// (MAGMA-compliant); falls back to token overlap otherwise.
// These are candidates only — Claude evaluates and creates actual semantic
// edges via `mnemon link`.
func FindSemanticCandidates(db *store.DB, insight *model.Insight) []SemanticCandidate {
	// Try embedding-based candidates first (P4: MAGMA compliance)
	if candidates := findCandidatesByEmbedding(db, insight); candidates != nil {
		return candidates
	}
	// Fallback: token overlap
	return findCandidatesByTokenOverlap(db, insight)
}

// findCandidatesByEmbedding uses cosine similarity over stored embeddings.
// Candidates with cosine >= autoSemanticThreshold are marked as auto-linked.
// Candidates in [reviewSemanticThreshold, autoSemanticThreshold) need LLM review.
func findCandidatesByEmbedding(db *store.DB, insight *model.Insight) []SemanticCandidate {
	// Get the new insight's embedding
	blob, err := db.GetEmbedding(insight.ID)
	if err != nil || len(blob) == 0 {
		return nil
	}
	insightVec := embed.DeserializeVector(blob)
	if insightVec == nil {
		return nil
	}

	// Get all embeddings for comparison
	allEmbedded, err := db.GetAllEmbeddings()
	if err != nil || len(allEmbedded) == 0 {
		return nil
	}

	type scored struct {
		id         string
		content    string
		category   string
		similarity float64
	}

	var candidates []scored
	for _, other := range allEmbedded {
		if other.ID == insight.ID {
			continue
		}
		otherVec := embed.DeserializeVector(other.Embedding)
		if otherVec == nil {
			continue
		}
		cosSim := embed.CosineSimilarity(insightVec, otherVec)
		if cosSim >= reviewSemanticThreshold {
			// Look up category
			ins, err := db.GetInsightByID(other.ID)
			cat := ""
			if err == nil && ins != nil {
				cat = string(ins.Category)
			}
			candidates = append(candidates, scored{
				id: other.ID, content: other.Content,
				category: cat, similarity: cosSim,
			})
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].similarity > candidates[j].similarity
	})

	if len(candidates) > maxSemanticCandidates {
		candidates = candidates[:maxSemanticCandidates]
	}

	result := make([]SemanticCandidate, len(candidates))
	for i, c := range candidates {
		result[i] = SemanticCandidate{
			ID:              c.id,
			Content:         c.content,
			Category:        c.category,
			Similarity: c.similarity, // actually cosine, but same JSON field
			AutoLinked:      c.similarity >= autoSemanticThreshold,
		}
	}
	return result
}

// findCandidatesByTokenOverlap is the fallback when embeddings are unavailable.
func findCandidatesByTokenOverlap(db *store.DB, insight *model.Insight) []SemanticCandidate {
	all, err := db.GetAllActiveInsights()
	if err != nil || len(all) == 0 {
		return nil
	}

	type scored struct {
		insight    *model.Insight
		similarity float64
	}

	var candidates []scored
	for _, other := range all {
		if other.ID == insight.ID {
			continue
		}
		sim := search.ContentSimilarity(insight.Content, other.Content)
		if sim >= minSemanticSimilarity {
			candidates = append(candidates, scored{insight: other, similarity: sim})
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].similarity > candidates[j].similarity
	})

	if len(candidates) > maxSemanticCandidates {
		candidates = candidates[:maxSemanticCandidates]
	}

	result := make([]SemanticCandidate, len(candidates))
	for i, c := range candidates {
		result[i] = SemanticCandidate{
			ID:              c.insight.ID,
			Content:         c.insight.Content,
			Category:        string(c.insight.Category),
			Similarity: c.similarity,
		}
	}
	return result
}
