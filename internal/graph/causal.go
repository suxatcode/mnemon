package graph

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/Grivn/mnemon/internal/model"
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

	newTokens := tokenize(insight.Content)
	if len(newTokens) == 0 {
		return 0
	}

	now := time.Now().UTC()
	count := 0

	for _, prev := range recent {
		prevTokens := tokenize(prev.Content)
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

// tokenize splits text into lowercase tokens. For English words it splits on
// whitespace/punctuation; for CJK characters it generates character bigrams.
func tokenize(text string) map[string]bool {
	tokens := make(map[string]bool)
	text = strings.ToLower(text)

	// Extract English words
	var word strings.Builder
	runes := []rune(text)
	var cjkBuf []rune

	for _, r := range runes {
		if unicode.Is(unicode.Han, r) {
			// Flush any English word
			if word.Len() > 0 {
				tokens[word.String()] = true
				word.Reset()
			}
			cjkBuf = append(cjkBuf, r)
		} else {
			// Flush CJK bigrams
			if len(cjkBuf) > 0 {
				for j := 0; j < len(cjkBuf)-1; j++ {
					tokens[string(cjkBuf[j:j+2])] = true
				}
				if len(cjkBuf) == 1 {
					tokens[string(cjkBuf)] = true
				}
				cjkBuf = cjkBuf[:0]
			}

			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				word.WriteRune(r)
			} else {
				if word.Len() > 0 {
					tokens[word.String()] = true
					word.Reset()
				}
			}
		}
	}
	// Flush remaining
	if word.Len() > 0 {
		tokens[word.String()] = true
	}
	if len(cjkBuf) > 0 {
		for j := 0; j < len(cjkBuf)-1; j++ {
			tokens[string(cjkBuf[j:j+2])] = true
		}
		if len(cjkBuf) == 1 {
			tokens[string(cjkBuf)] = true
		}
	}
	return tokens
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
