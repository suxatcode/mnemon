package graph

import (
	"fmt"
	"regexp"
	"time"

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

// CreateCausalEdges creates causal edges if the new insight has causal signals
// and shares sufficient token overlap with recent insights.
func CreateCausalEdges(db *store.DB, insight *model.Insight) int {
	if !HasCausalSignal(insight.Content) {
		return 0
	}

	recent, err := db.GetRecentInsightsBySource(insight.Source, insight.ID, causalLookback)
	if err != nil || len(recent) == 0 {
		return 0
	}

	newTokens := search.Tokenize(insight.Content)
	if len(newTokens) == 0 {
		return 0
	}

	now := time.Now().UTC()
	count := 0

	for _, prev := range recent {
		prevTokens := search.Tokenize(prev.Content)
		overlap := tokenOverlap(newTokens, prevTokens)
		if overlap >= minCausalOverlap {
			err = db.InsertEdge(&model.Edge{
				SourceID:  insight.ID,
				TargetID:  prev.ID,
				EdgeType:  model.EdgeCausal,
				Weight:    overlap,
				Metadata:  map[string]string{"overlap": formatFloat(overlap)},
				CreatedAt: now,
			})
			if err == nil {
				count++
			}
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

// FindCausalCandidates returns insights that may have causal relationships
// with the given insight. Claude evaluates direction, sub_type, and weight.
func FindCausalCandidates(db *store.DB, insight *model.Insight) []CausalCandidate {
	newTokens := search.Tokenize(insight.Content)
	if len(newTokens) == 0 {
		return nil
	}

	// Check recent insights (broader than auto-causal: include those without causal signal too)
	recent, err := db.GetRecentInsightsBySource(insight.Source, insight.ID, causalLookback)
	if err != nil || len(recent) == 0 {
		return nil
	}

	var candidates []CausalCandidate

	for _, prev := range recent {
		// At least one of the pair should have a causal signal
		newSignal := findCausalSignal(insight.Content)
		prevSignal := findCausalSignal(prev.Content)
		if newSignal == "" && prevSignal == "" {
			continue
		}

		prevTokens := search.Tokenize(prev.Content)
		overlap := tokenOverlap(newTokens, prevTokens)
		if overlap < minCausalCandidateOverlap {
			continue
		}

		// Use the signal from whichever has it
		signal := newSignal
		if signal == "" {
			signal = prevSignal
		}

		// Determine sub_type from combined text
		subType := suggestSubType(insight.Content + " " + prev.Content)

		candidates = append(candidates, CausalCandidate{
			ID:               prev.ID,
			Content:          prev.Content,
			Category:         string(prev.Category),
			CausalSignal:     signal,
			TokenOverlap:     overlap,
			SuggestedSubType: subType,
		})
	}

	if len(candidates) > maxCausalCandidates {
		candidates = candidates[:maxCausalCandidates]
	}

	return candidates
}
