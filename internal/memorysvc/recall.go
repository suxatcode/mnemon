package memorysvc

import (
	"fmt"
	"math"

	"github.com/mnemon-dev/mnemon/internal/embed"
	"github.com/mnemon-dev/mnemon/internal/graph"
	"github.com/mnemon-dev/mnemon/internal/model"
	"github.com/mnemon-dev/mnemon/internal/search"
	"github.com/mnemon-dev/mnemon/internal/store"
)

type compactResult struct {
	ID             string  `json:"id"`
	Content        string  `json:"content"`
	Category       string  `json:"category,omitempty"`
	Importance     int     `json:"importance,omitempty"`
	Intent         string  `json:"intent"`
	MatchedVia     string  `json:"matched_via,omitempty"`
	Confidence     string  `json:"confidence"`
	Score          float64 `json:"score"`
	OwnerPrincipal string  `json:"owner_principal,omitempty"`
	Layer          string  `json:"layer,omitempty"`
}

type CompactResponse struct {
	Results []compactResult `json:"results"`
	Hint    string          `json:"hint,omitempty"`
}

const (
	confidenceLowMax    = 0.25
	confidenceMediumMax = 0.6
)

func confidenceLabel(score float64) string {
	switch {
	case score < confidenceLowMax:
		return "low"
	case score < confidenceMediumMax:
		return "medium"
	default:
		return "high"
	}
}

func roundScore(s float64) float64 {
	return math.Round(s*1000) / 1000
}

// ToCompact projects a full RecallResponse into the compact LLM-friendly shape.
func ToCompact(resp search.RecallResponse) CompactResponse {
	results := make([]compactResult, 0, len(resp.Results))
	for _, r := range resp.Results {
		rounded := roundScore(r.Score)
		owner, layer := "", ""
		if r.Insight != nil {
			owner = r.Insight.OwnerPrincipal
			layer = r.Insight.Layer
		}
		results = append(results, compactResult{
			ID:             r.Insight.ID,
			Content:        r.Insight.Content,
			Category:       string(r.Insight.Category),
			Importance:     r.Insight.Importance,
			Intent:         string(r.Intent),
			MatchedVia:     r.Via,
			Confidence:     confidenceLabel(rounded),
			Score:          rounded,
			OwnerPrincipal: owner,
			Layer:          layer,
		})
	}
	return CompactResponse{Results: results, Hint: resp.Meta.Hint}
}

type RecallInput struct {
	Query    string
	Category string
	Limit    int
	Source   string
	Basic    bool
	Intent   string
	Verbose  bool
}

func (s *Service) Recall(actor Actor, req RecallInput) (Result, error) {
	if req.Limit <= 0 {
		req.Limit = 10
	}
	db := s.db
	if req.Basic {
		results, err := db.QueryInsights(store.QueryFilter{
			Keyword:  req.Query,
			Category: req.Category,
			Source:   req.Source,
			Limit:    req.Limit,
		})
		if err != nil {
			return Result{}, err
		}
		for _, r := range results {
			_ = db.IncrementAccessCount(r.ID)
		}
		db.LogOp("recall:basic", "", fmt.Sprintf("principal=%s q=%s hits=%d", actor.Principal, req.Query, len(results)))
		return s.encode(results, nil)
	}

	var intentOverride *search.Intent
	if req.Intent != "" {
		parsed, err := search.IntentFromString(req.Intent)
		if err != nil {
			return Result{}, err
		}
		intentOverride = &parsed
	}

	var queryVec []float64
	ec := embed.NewClientWithModel(s.embedModel)
	if ec.Available() {
		queryVec, _ = ec.Embed(req.Query)
	}
	knownEntities, _ := db.LoadKnownEntities()
	queryEntities := graph.ExtractEntitiesIndexed(req.Query, knownEntities)

	resp, err := search.IntentAwareRecall(db, req.Query, queryVec, queryEntities, req.Limit, intentOverride)
	if err != nil {
		return Result{}, err
	}
	for _, r := range resp.Results {
		_ = db.IncrementAccessCount(r.Insight.ID)
	}
	db.LogOp("recall", "", fmt.Sprintf("principal=%s q=%s hits=%d", actor.Principal, req.Query, len(resp.Results)))
	if req.Verbose {
		return s.encode(resp, nil)
	}
	return s.encode(ToCompact(resp), nil)
}

type SearchInput struct {
	Query string
	Limit int
}

func (s *Service) Search(actor Actor, req SearchInput) (Result, error) {
	if req.Limit <= 0 {
		req.Limit = 10
	}
	all, err := s.db.GetAllActiveInsights()
	if err != nil {
		return Result{}, err
	}
	results := search.KeywordSearch(all, req.Query, req.Limit)
	for _, r := range results {
		_ = s.db.IncrementAccessCount(r.Insight.ID)
	}
	s.db.LogOp("search", "", fmt.Sprintf("principal=%s q=%s hits=%d", actor.Principal, req.Query, len(results)))
	type outputItem struct {
		ID             string   `json:"id"`
		Content        string   `json:"content"`
		Category       string   `json:"category"`
		Importance     int      `json:"importance"`
		Tags           []string `json:"tags"`
		Score          float64  `json:"score"`
		OwnerPrincipal string   `json:"owner_principal,omitempty"`
		Layer          string   `json:"layer,omitempty"`
	}
	output := make([]outputItem, 0, len(results))
	for _, r := range results {
		output = append(output, outputItem{
			ID:             r.Insight.ID,
			Content:        r.Insight.Content,
			Category:       string(r.Insight.Category),
			Importance:     r.Insight.Importance,
			Tags:           r.Insight.Tags,
			Score:          r.Score,
			OwnerPrincipal: r.Insight.OwnerPrincipal,
			Layer:          r.Insight.Layer,
		})
	}
	return s.encode(output, nil)
}

type RelatedInput struct {
	ID       string
	EdgeType string
	Depth    int
}

type RelatedResult struct {
	ID             string `json:"id"`
	Content        string `json:"content"`
	Category       string `json:"category"`
	Importance     int    `json:"importance"`
	Depth          int    `json:"depth"`
	EdgeType       string `json:"via_edge_type,omitempty"`
	OwnerPrincipal string `json:"owner_principal,omitempty"`
	Layer          string `json:"layer,omitempty"`
}

func (s *Service) Related(actor Actor, req RelatedInput) (Result, error) {
	if req.Depth <= 0 {
		req.Depth = 2
	}
	start, err := s.db.GetInsightByID(req.ID)
	if err != nil {
		return Result{}, fmt.Errorf("insight not found: %w", err)
	}
	var edgeFilter model.EdgeType
	if req.EdgeType != "" {
		et := model.EdgeType(req.EdgeType)
		if !model.ValidEdgeTypes[et] {
			return Result{}, fmt.Errorf("invalid edge type %q", req.EdgeType)
		}
		edgeFilter = et
	}
	nodes := graph.BFS(s.db, start.ID, graph.BFSOptions{MaxDepth: req.Depth, EdgeFilter: edgeFilter})
	results := make([]RelatedResult, 0, len(nodes))
	for _, n := range nodes {
		results = append(results, RelatedResult{
			ID:             n.Insight.ID,
			Content:        n.Insight.Content,
			Category:       string(n.Insight.Category),
			Importance:     n.Insight.Importance,
			Depth:          n.Hop,
			EdgeType:       string(n.ViaEdge.EdgeType),
			OwnerPrincipal: n.Insight.OwnerPrincipal,
			Layer:          n.Insight.Layer,
		})
	}
	s.db.LogOp("related", req.ID, fmt.Sprintf("principal=%s depth=%d hits=%d", actor.Principal, req.Depth, len(results)))
	return s.encode(results, nil)
}
