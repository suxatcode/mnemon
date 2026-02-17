package search

import (
	"sort"
	"strings"
	"unicode"

	"github.com/Grivn/mnemon/internal/model"
)

// ScoredInsight pairs an insight with a relevance score.
type ScoredInsight struct {
	Insight *model.Insight `json:"insight"`
	Score   float64        `json:"score"`
}

// KeywordSearch scores insights by token overlap with the query.
// Score = |intersection(query_tokens, content_tokens)| / |query_tokens|
func KeywordSearch(insights []*model.Insight, query string, limit int) []ScoredInsight {
	queryTokens := Tokenize(query)
	if len(queryTokens) == 0 {
		return nil
	}

	var scored []ScoredInsight
	for _, ins := range insights {
		contentTokens := Tokenize(ins.Content)
		// Also include tags and entities as tokens
		for _, tag := range ins.Tags {
			for t := range Tokenize(tag) {
				contentTokens[t] = true
			}
		}
		for _, ent := range ins.Entities {
			for t := range Tokenize(ent) {
				contentTokens[t] = true
			}
		}

		intersection := 0
		for t := range queryTokens {
			if contentTokens[t] {
				intersection++
			}
		}
		if intersection == 0 {
			continue
		}
		score := float64(intersection) / float64(len(queryTokens))
		scored = append(scored, ScoredInsight{Insight: ins, Score: score})
	}

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		return scored[i].Insight.Importance > scored[j].Insight.Importance
	})

	if limit > 0 && len(scored) > limit {
		scored = scored[:limit]
	}
	return scored
}

// Tokenize splits text into lowercase tokens. English words split on
// whitespace/punctuation; CJK characters generate character bigrams.
func Tokenize(text string) map[string]bool {
	tokens := make(map[string]bool)
	text = strings.ToLower(text)

	var word strings.Builder
	runes := []rune(text)
	var cjkBuf []rune

	for _, r := range runes {
		if unicode.Is(unicode.Han, r) {
			if word.Len() > 0 {
				tokens[word.String()] = true
				word.Reset()
			}
			cjkBuf = append(cjkBuf, r)
		} else {
			if len(cjkBuf) > 0 {
				flushCJK(cjkBuf, tokens)
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
	if word.Len() > 0 {
		tokens[word.String()] = true
	}
	if len(cjkBuf) > 0 {
		flushCJK(cjkBuf, tokens)
	}
	return tokens
}

func flushCJK(buf []rune, tokens map[string]bool) {
	for j := 0; j < len(buf)-1; j++ {
		tokens[string(buf[j:j+2])] = true
	}
	if len(buf) == 1 {
		tokens[string(buf)] = true
	}
}

// ContentSimilarity computes bidirectional token overlap between two texts.
// Returns max(overlap_a_to_b, overlap_b_to_a) for a symmetric measure.
func ContentSimilarity(a, b string) float64 {
	tokA := Tokenize(a)
	tokB := Tokenize(b)
	if len(tokA) == 0 || len(tokB) == 0 {
		return 0
	}

	intersection := 0
	for t := range tokA {
		if tokB[t] {
			intersection++
		}
	}

	scoreA := float64(intersection) / float64(len(tokA))
	scoreB := float64(intersection) / float64(len(tokB))
	if scoreA > scoreB {
		return scoreA
	}
	return scoreB
}
