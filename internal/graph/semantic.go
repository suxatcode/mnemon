package graph

import (
	"sort"

	"github.com/Grivn/mnemon/internal/embed"
	"github.com/Grivn/mnemon/internal/model"
	"github.com/Grivn/mnemon/internal/search"
	"github.com/Grivn/mnemon/internal/store"
)

// Minimum similarity to be considered a semantic candidate.
const minSemanticSimilarity = 0.10

// Minimum cosine similarity when using embedding-based candidates.
const minEmbeddingCosine = 0.30

// Maximum number of semantic candidates to return.
const maxSemanticCandidates = 5

// SemanticCandidate represents a potential semantic link for Claude to evaluate.
type SemanticCandidate struct {
	ID              string  `json:"id"`
	Content         string  `json:"content"`
	Category        string  `json:"category"`
	TokenSimilarity float64 `json:"token_similarity"`
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
		if cosSim >= minEmbeddingCosine {
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
			TokenSimilarity: c.similarity, // actually cosine, but same JSON field
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
			TokenSimilarity: c.similarity,
		}
	}
	return result
}
