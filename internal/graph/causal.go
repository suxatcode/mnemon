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

// Maximum number of causal candidates to return.
const maxCausalCandidates = 10

// CausalCandidate represents a potential causal link for Claude to evaluate.
type CausalCandidate struct {
	ID               string `json:"id"`
	Content          string `json:"content"`
	Category         string `json:"category"`
	Hop              int    `json:"hop"`               // graph distance (1 or 2)
	ViaEdge          string `json:"via_edge"`           // edge type that connected
	CausalSignal     string `json:"causal_signal"`      // keyword if found, else ""
	SuggestedSubType string `json:"suggested_sub_type"` // heuristic suggestion
}

// Patterns for suggesting causal sub_type.
var (
	causesPattern   = regexp.MustCompile(`(?i)\b(because|caused by|due to)\b|(因为|由于)`)
	enablesPattern  = regexp.MustCompile(`(?i)\b(so that|in order to|enables|leads to)\b|(为了|以便)`)
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
	return causalPattern.FindString(text)
}

// NeighborNode represents a node discovered by BFS neighborhood traversal.
type NeighborNode struct {
	ID       string `json:"id"`
	Content  string `json:"content"`
	Category string `json:"category"`
	Hop      int    `json:"hop"`      // 1 or 2
	ViaEdge  string `json:"via_edge"` // edge type that led here
}

// GetNeighborhood performs a BFS from nodeID up to maxHops, following all edge
// types (temporal, semantic, causal, entity). Returns up to maxNodes neighbor
// nodes, excluding the start node and soft-deleted nodes.
func GetNeighborhood(db *store.DB, nodeID string, maxHops int, maxNodes int) []NeighborNode {
	type bfsEntry struct {
		id      string
		hop     int
		viaEdge string
	}

	visited := map[string]bool{nodeID: true}
	queue := []bfsEntry{{id: nodeID, hop: 0, viaEdge: ""}}
	var result []NeighborNode

	for len(queue) > 0 && len(result) < maxNodes {
		current := queue[0]
		queue = queue[1:]

		if current.hop >= maxHops {
			continue
		}

		edges, err := db.GetEdgesByNode(current.id)
		if err != nil {
			continue
		}

		for _, edge := range edges {
			// Determine the neighbor ID (the other end of the edge)
			neighborID := edge.TargetID
			if neighborID == current.id {
				neighborID = edge.SourceID
			}

			if visited[neighborID] {
				continue
			}
			visited[neighborID] = true

			// Skip soft-deleted nodes
			insight, err := db.GetInsightByID(neighborID)
			if err != nil || insight == nil {
				continue
			}

			nextHop := current.hop + 1
			result = append(result, NeighborNode{
				ID:       insight.ID,
				Content:  insight.Content,
				Category: string(insight.Category),
				Hop:      nextHop,
				ViaEdge:  string(edge.EdgeType),
			})

			if len(result) >= maxNodes {
				break
			}

			// Enqueue for further traversal
			queue = append(queue, bfsEntry{
				id:      neighborID,
				hop:     nextHop,
				viaEdge: string(edge.EdgeType),
			})
		}
	}

	return result
}

// FindCausalCandidates returns insights that may have causal relationships
// with the given insight. Uses 2-hop BFS neighborhood traversal (MAGMA §3.3)
// to discover candidates through the existing graph structure, then annotates
// each with causal signal keywords as auxiliary labels for Claude to evaluate.
func FindCausalCandidates(db *store.DB, insight *model.Insight) []CausalCandidate {
	neighbors := GetNeighborhood(db, insight.ID, 2, maxCausalCandidates)
	if len(neighbors) == 0 {
		return nil
	}

	var candidates []CausalCandidate
	for _, n := range neighbors {
		// Check for causal keywords in either the new insight or the neighbor
		combinedText := insight.Content + " " + n.Content
		signal := findCausalSignal(n.Content)
		if signal == "" {
			signal = findCausalSignal(insight.Content)
		}

		subType := suggestSubType(combinedText)

		candidates = append(candidates, CausalCandidate{
			ID:               n.ID,
			Content:          n.Content,
			Category:         n.Category,
			Hop:              n.Hop,
			ViaEdge:          n.ViaEdge,
			CausalSignal:     signal,
			SuggestedSubType: subType,
		})
	}

	return candidates
}
