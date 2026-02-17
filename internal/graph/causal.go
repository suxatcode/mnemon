package graph

import (
	"fmt"
	"regexp"
	"sort"
	"time"

	"github.com/Grivn/mnemon/internal/embed"
	"github.com/Grivn/mnemon/internal/model"
	"github.com/Grivn/mnemon/internal/search"
	"github.com/Grivn/mnemon/internal/store"
)

// Minimum token overlap ratio to create a causal edge.
const minCausalOverlap = 0.15

// Number of recent insights to check for causal links.
const causalLookback = 10

var causalPattern = regexp.MustCompile(
	`(?i)\b(because|therefore|due to|caused by|as a result|decided to|` +
		`chosen because|so that|in order to|leads to|results in)\b|` +
		`(因为|所以|由于|导致|因此|决定|为了|以便)`)

// HasCausalSignal returns true if the text contains causal keywords.
func HasCausalSignal(text string) bool {
	return causalPattern.MatchString(text)
}

// CreateCausalEdges creates causal edges when either the new insight or a recent
// insight has causal signals and they share sufficient token overlap.
// Direction is inferred from which side has the causal keyword (MAGMA §3.3).
func CreateCausalEdges(db *store.DB, insight *model.Insight) int {
	recent, err := db.GetRecentInsightsBySource(insight.Source, insight.ID, causalLookback)
	if err != nil || len(recent) == 0 {
		return 0
	}

	newTokens := search.Tokenize(insight.Content)
	if len(newTokens) == 0 {
		return 0
	}

	newHasSignal := HasCausalSignal(insight.Content)

	now := time.Now().UTC()
	count := 0

	for _, prev := range recent {
		prevHasSignal := HasCausalSignal(prev.Content)
		// At least one side must have a causal signal
		if !newHasSignal && !prevHasSignal {
			continue
		}

		prevTokens := search.Tokenize(prev.Content)
		overlap := tokenOverlap(newTokens, prevTokens)
		if overlap < minCausalOverlap {
			continue
		}

		// Infer direction: the side with the causal keyword is the "effect" side
		// e.g. "chose X because of Y" → this insight explains WHY, so new→prev
		// e.g. prev="chose X", new="latency improved" + no keyword → prev caused new, so prev→new
		sourceID := insight.ID
		targetID := prev.ID
		if !newHasSignal && prevHasSignal {
			// Only prev has signal — prev is the "effect" explaining the cause
			// Direction: prev→new (prev caused/explains new)
			sourceID = prev.ID
			targetID = insight.ID
		}

		subType := suggestSubType(insight.Content + " " + prev.Content)

		err = db.InsertEdge(&model.Edge{
			SourceID:  sourceID,
			TargetID:  targetID,
			EdgeType:  model.EdgeCausal,
			Weight:    overlap,
			Metadata: map[string]string{
				"overlap":  formatFloat(overlap),
				"sub_type": subType,
			},
			CreatedAt: now,
		})
		if err == nil {
			count++
		}
	}
	return count
}

// tokenOverlap computes |intersection| / max(|a|, |b|).
func tokenOverlap(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	intersection := 0
	for k := range a {
		if b[k] {
			intersection++
		}
	}
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	return float64(intersection) / float64(maxLen)
}

func formatFloat(f float64) string {
	return fmt.Sprintf("%.4f", f)
}

// Minimum overlap for causal candidate consideration (low for recall, Claude decides).
const minCausalCandidateOverlap = 0.10

// Maximum number of causal candidates to return.
const maxCausalCandidates = 5

// CausalCandidate represents a potential causal link for Claude to evaluate.
type CausalCandidate struct {
	ID               string  `json:"id"`
	Content          string  `json:"content"`
	Category         string  `json:"category"`
	CausalSignal     string  `json:"causal_signal"`
	TokenOverlap     float64 `json:"token_overlap"`
	SuggestedSubType string  `json:"suggested_sub_type"`
}

// Patterns for suggesting causal sub_type.
var (
	causesPattern  = regexp.MustCompile(`(?i)\b(because|caused by|due to)\b|(因为|由于)`)
	enablesPattern = regexp.MustCompile(`(?i)\b(so that|in order to|enables|leads to)\b|(为了|以便)`)
	preventsPattern = regexp.MustCompile(`(?i)\b(despite|prevented|prevents|blocked)\b|(阻止|防止)`)
)

// suggestSubType guesses a causal sub_type from the content text.
func suggestSubType(text string) string {
	if preventsPattern.MatchString(text) {
		return "prevents"
	}
	if enablesPattern.MatchString(text) {
		return "enables"
	}
	if causesPattern.MatchString(text) {
		return "causes"
	}
	return "causes"
}

// findCausalSignal returns the first matching causal keyword in the text.
func findCausalSignal(text string) string {
	match := causalPattern.FindString(text)
	if match != "" {
		return match
	}
	return ""
}

// Minimum cosine similarity for embedding-based causal candidate discovery.
const minCausalCosine = 0.40

// FindCausalCandidates returns insights that may have causal relationships
// with the given insight. Uses two discovery paths:
// 1. Keyword signal + token overlap (explicit causation)
// 2. Embedding cosine similarity (implicit causation, MAGMA §3.3 alignment)
// Claude evaluates direction, sub_type, and weight.
func FindCausalCandidates(db *store.DB, insight *model.Insight) []CausalCandidate {
	newTokens := search.Tokenize(insight.Content)

	// Check recent insights
	recent, err := db.GetRecentInsightsBySource(insight.Source, insight.ID, causalLookback)
	if err != nil || len(recent) == 0 {
		return nil
	}

	// Get embedding for the new insight (for implicit causation discovery)
	var insightVec []float64
	if blob, err := db.GetEmbedding(insight.ID); err == nil && len(blob) > 0 {
		insightVec = embed.DeserializeVector(blob)
	}

	newSignal := findCausalSignal(insight.Content)

	type scoredCandidate struct {
		candidate CausalCandidate
		score     float64 // for sorting: higher is better
	}
	var scored []scoredCandidate

	for _, prev := range recent {
		prevSignal := findCausalSignal(prev.Content)

		// Path 1: keyword signal — at least one side has causal keyword
		if newSignal != "" || prevSignal != "" {
			prevTokens := search.Tokenize(prev.Content)
			overlap := tokenOverlap(newTokens, prevTokens)
			if overlap >= minCausalCandidateOverlap {
				signal := newSignal
				if signal == "" {
					signal = prevSignal
				}
				subType := suggestSubType(insight.Content + " " + prev.Content)
				scored = append(scored, scoredCandidate{
					candidate: CausalCandidate{
						ID:               prev.ID,
						Content:          prev.Content,
						Category:         string(prev.Category),
						CausalSignal:     signal,
						TokenOverlap:     overlap,
						SuggestedSubType: subType,
					},
					score: overlap + 0.5, // boost keyword-matched candidates
				})
				continue
			}
		}

		// Path 2: embedding similarity — implicit causation (no keyword required)
		// Especially useful for decision→outcome pairs without explicit causal words
		if insightVec != nil {
			if blob, err := db.GetEmbedding(prev.ID); err == nil && len(blob) > 0 {
				prevVec := embed.DeserializeVector(blob)
				if prevVec != nil {
					cosSim := embed.CosineSimilarity(insightVec, prevVec)
					if cosSim >= minCausalCosine {
						// Check category pair heuristic: decision+fact or decision+context
						// are more likely causal than fact+fact
						catPair := string(insight.Category) + "+" + string(prev.Category)
						subType := "causes" // default for implicit
						if isCausalCategoryPair(catPair) {
							scored = append(scored, scoredCandidate{
								candidate: CausalCandidate{
									ID:               prev.ID,
									Content:          prev.Content,
									Category:         string(prev.Category),
									CausalSignal:     "(implicit: embedding similarity)",
									TokenOverlap:     cosSim,
									SuggestedSubType: subType,
								},
								score: cosSim,
							})
						}
					}
				}
			}
		}
	}

	if len(scored) == 0 {
		return nil
	}

	// Sort by score descending, deduplicate by ID
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})
	seen := make(map[string]bool)
	var candidates []CausalCandidate
	for _, s := range scored {
		if seen[s.candidate.ID] {
			continue
		}
		seen[s.candidate.ID] = true
		candidates = append(candidates, s.candidate)
		if len(candidates) >= maxCausalCandidates {
			break
		}
	}

	return candidates
}

// isCausalCategoryPair returns true if the category combination suggests
// a likely causal relationship (decision→fact, decision→context, etc.)
func isCausalCategoryPair(pair string) bool {
	causalPairs := map[string]bool{
		"decision+fact":       true,
		"fact+decision":       true,
		"decision+context":    true,
		"context+decision":    true,
		"decision+insight":    true,
		"insight+decision":    true,
		"decision+preference": true,
		"preference+decision": true,
		"fact+context":        true,
		"context+fact":        true,
		"insight+fact":        true,
		"fact+insight":        true,
		"insight+context":     true,
		"context+insight":     true,
	}
	return causalPairs[pair]
}
