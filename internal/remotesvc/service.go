package remotesvc

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mnemon-dev/mnemon/internal/embed"
	"github.com/mnemon-dev/mnemon/internal/graph"
	"github.com/mnemon-dev/mnemon/internal/importdraft"
	"github.com/mnemon-dev/mnemon/internal/model"
	"github.com/mnemon-dev/mnemon/internal/remoteapi"
	"github.com/mnemon-dev/mnemon/internal/search"
	"github.com/mnemon-dev/mnemon/internal/store"
)

type Service struct {
	DataDir    string
	StoreName  string
	EmbedModel string
}

func (s Service) openDB() (*store.DB, error) {
	name := s.StoreName
	if name == "" {
		name = store.DefaultStoreName
	}
	if !store.ValidStoreName(name) {
		return nil, fmt.Errorf("invalid store name %q", name)
	}
	if err := store.MigrateIfNeeded(s.DataDir); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return store.Open(store.StoreDir(s.DataDir, name))
}

func encode(v any) ([]byte, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func (s Service) Status() ([]byte, error) {
	db, err := s.openDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	stats, err := db.GetStats()
	if err != nil {
		return nil, err
	}
	var fileSize int64
	if fi, err := os.Stat(db.Path()); err == nil {
		fileSize = fi.Size()
	}
	return encode(map[string]any{
		"total_insights":   stats.Total,
		"deleted_insights": stats.DeletedCount,
		"by_category":      stats.ByCategory,
		"edge_count":       stats.EdgeCount,
		"top_entities":     stats.TopEntities,
		"oplog_count":      stats.OplogCount,
		"db_path":          db.Path(),
		"db_size_bytes":    fileSize,
		"remote":           true,
	})
}

type compactResult struct {
	ID         string  `json:"id"`
	Content    string  `json:"content"`
	Category   string  `json:"category,omitempty"`
	Importance int     `json:"importance,omitempty"`
	Intent     string  `json:"intent"`
	MatchedVia string  `json:"matched_via,omitempty"`
	Confidence string  `json:"confidence"`
	Score      float64 `json:"score"`
}

type compactResponse struct {
	Results []compactResult `json:"results"`
	Hint    string          `json:"hint,omitempty"`
}

func confidenceLabel(score float64) string {
	switch {
	case score < 0.25:
		return "low"
	case score < 0.6:
		return "medium"
	default:
		return "high"
	}
}

func roundScore(s float64) float64 {
	return math.Round(s*1000) / 1000
}

func toCompact(resp search.RecallResponse) compactResponse {
	results := make([]compactResult, 0, len(resp.Results))
	for _, r := range resp.Results {
		rounded := roundScore(r.Score)
		results = append(results, compactResult{
			ID:         r.Insight.ID,
			Content:    r.Insight.Content,
			Category:   string(r.Insight.Category),
			Importance: r.Insight.Importance,
			Intent:     string(r.Intent),
			MatchedVia: r.Via,
			Confidence: confidenceLabel(rounded),
			Score:      rounded,
		})
	}
	return compactResponse{Results: results, Hint: resp.Meta.Hint}
}

func (s Service) Recall(req remoteapi.RecallRequest) ([]byte, error) {
	if req.Limit <= 0 {
		req.Limit = 10
	}
	db, err := s.openDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	if req.Basic {
		results, err := db.QueryInsights(store.QueryFilter{
			Keyword:  req.Query,
			Category: req.Category,
			Source:   req.Source,
			Limit:    req.Limit,
		})
		if err != nil {
			return nil, err
		}
		for _, r := range results {
			_ = db.IncrementAccessCount(r.ID)
		}
		db.LogOp("recall:basic", "", fmt.Sprintf("principal=%s q=%s hits=%d", req.Auth.Principal, req.Query, len(results)))
		return encode(results)
	}

	var intentOverride *search.Intent
	if req.Intent != "" {
		parsed, err := search.IntentFromString(req.Intent)
		if err != nil {
			return nil, err
		}
		intentOverride = &parsed
	}

	var queryVec []float64
	ec := embed.NewClientWithModel(s.EmbedModel)
	if ec.Available() {
		queryVec, _ = ec.Embed(req.Query)
	}
	knownEntities, _ := db.LoadKnownEntities()
	queryEntities := graph.ExtractEntitiesIndexed(req.Query, knownEntities)

	resp, err := search.IntentAwareRecall(db, req.Query, queryVec, queryEntities, req.Limit, intentOverride)
	if err != nil {
		return nil, err
	}
	for _, r := range resp.Results {
		_ = db.IncrementAccessCount(r.Insight.ID)
	}
	db.LogOp("recall", "", fmt.Sprintf("principal=%s q=%s hits=%d", req.Auth.Principal, req.Query, len(resp.Results)))
	if req.Verbose {
		return encode(resp)
	}
	return encode(toCompact(resp))
}

func (s Service) Search(req remoteapi.SearchRequest) ([]byte, error) {
	if req.Limit <= 0 {
		req.Limit = 10
	}
	db, err := s.openDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	all, err := db.GetAllActiveInsights()
	if err != nil {
		return nil, err
	}
	results := search.KeywordSearch(all, req.Query, req.Limit)
	for _, r := range results {
		_ = db.IncrementAccessCount(r.Insight.ID)
	}
	db.LogOp("search", "", fmt.Sprintf("principal=%s q=%s hits=%d", req.Auth.Principal, req.Query, len(results)))

	type outputItem struct {
		ID         string   `json:"id"`
		Content    string   `json:"content"`
		Category   string   `json:"category"`
		Importance int      `json:"importance"`
		Tags       []string `json:"tags"`
		Score      float64  `json:"score"`
	}
	output := make([]outputItem, 0, len(results))
	for _, r := range results {
		output = append(output, outputItem{
			ID:         r.Insight.ID,
			Content:    r.Insight.Content,
			Category:   string(r.Insight.Category),
			Importance: r.Insight.Importance,
			Tags:       r.Insight.Tags,
			Score:      r.Score,
		})
	}
	return encode(output)
}

func parseCSV(raw string, maxItems, maxLen int, label string) ([]string, error) {
	var out []string
	if raw != "" {
		for _, item := range strings.Split(raw, ",") {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			if len(item) > maxLen {
				return nil, fmt.Errorf("%s too long (%d chars, max %d)", label, len(item), maxLen)
			}
			out = append(out, item)
		}
		if len(out) > maxItems {
			return nil, fmt.Errorf("too many %ss (%d, max %d)", label, len(out), maxItems)
		}
	}
	if out == nil {
		out = []string{}
	}
	return out, nil
}

func addProvenanceTags(tags []string, principal, agent string) []string {
	seen := make(map[string]bool, len(tags)+2)
	out := make([]string, 0, len(tags)+2)
	for _, tag := range tags {
		if !seen[tag] {
			seen[tag] = true
			out = append(out, tag)
		}
	}
	for _, tag := range []string{"principal:" + principal, "agent:" + agent} {
		if strings.HasSuffix(tag, ":") {
			continue
		}
		if !seen[tag] {
			out = append(out, tag)
		}
	}
	return out
}

func (s Service) Remember(req remoteapi.RememberRequest) ([]byte, error) {
	content := req.Content
	if len(content) > 8000 {
		return nil, fmt.Errorf("content too long (%d chars, max 8000)", len(content))
	}
	cat := model.Category(req.Category)
	if cat == "" {
		cat = model.CategoryGeneral
	}
	if !model.ValidCategories[cat] {
		return nil, fmt.Errorf("invalid category %q", req.Category)
	}
	if req.Importance == 0 {
		req.Importance = 3
	}
	if req.Importance < 1 || req.Importance > 5 {
		return nil, fmt.Errorf("importance must be 1-5, got %d", req.Importance)
	}
	entityMode := graph.EntityMode(req.EntityMode)
	if entityMode == "" {
		entityMode = graph.EntityModeMerge
	}
	if !graph.ValidEntityMode(entityMode) {
		return nil, fmt.Errorf("invalid entity mode %q", req.EntityMode)
	}
	tags, err := parseCSV(req.Tags, 20, 100, "tag")
	if err != nil {
		return nil, err
	}
	tags = addProvenanceTags(tags, req.Auth.Principal, req.Agent)
	entities, err := parseCSV(req.Entities, 50, 200, "entity")
	if err != nil {
		return nil, err
	}
	source := req.Source
	if source == "" {
		source = "agent"
	}

	now := time.Now().UTC()
	insight := &model.Insight{
		ID:         uuid.New().String(),
		Content:    content,
		Category:   cat,
		Importance: req.Importance,
		Tags:       tags,
		Entities:   entities,
		Source:     source,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	db, err := s.openDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	var embeddingBlob []byte
	var embeddingVec []float64
	ec := embed.NewClientWithModel(s.EmbedModel)
	if ec.Available() {
		if vec, err := ec.Embed(content); err == nil {
			embeddingVec = vec
			embeddingBlob = embed.SerializeVector(vec)
		}
	}

	var diffAction string
	var replacedID string
	var diffSuggestion search.DiffSuggestion
	var embedCache graph.EmbedCache
	if ec.Available() {
		dbEmbeds, err := db.GetAllEmbeddings()
		if err == nil {
			embedCache = make(graph.EmbedCache, len(dbEmbeds))
			for _, e := range dbEmbeds {
				if v := embed.DeserializeVector(e.Embedding); v != nil {
					embedCache[e.ID] = v
				}
			}
		}
	}

	if req.NoDiff {
		diffAction = "added"
		diffSuggestion = search.DiffAdd
	} else {
		allInsights, err := db.GetAllActiveInsights()
		if err != nil {
			return nil, err
		}
		opts := search.DiffOptions{Limit: 5, NewEmbedding: embeddingVec}
		if embedCache != nil {
			opts.ExistingEmbed = make([]search.EmbeddedItem, 0, len(embedCache))
			for id, v := range embedCache {
				opts.ExistingEmbed = append(opts.ExistingEmbed, search.EmbeddedItem{ID: id, Embedding: v})
			}
		}
		result := search.Diff(allInsights, content, opts)
		diffSuggestion = result.Suggestion
		switch result.Suggestion {
		case search.DiffDuplicate:
			diffAction = "skipped"
			if len(result.Matches) > 0 {
				replacedID = result.Matches[0].ID
			}
		case search.DiffConflict, search.DiffUpdate:
			diffAction = "updated"
			if len(result.Matches) > 0 {
				replacedID = result.Matches[0].ID
			}
		default:
			diffAction = "added"
		}
	}

	if diffAction == "skipped" {
		db.LogOp("diff-skip", insight.ID, fmt.Sprintf("principal=%s duplicate of %s", req.Auth.Principal, replacedID))
		return encode(map[string]any{
			"id":              insight.ID,
			"content":         content,
			"action":          "skipped",
			"diff_suggestion": string(diffSuggestion),
			"replaced_id":     replacedID,
		})
	}

	var edgeStats graph.EdgeStats
	var ei float64
	var pruned int
	var embedded bool
	err = db.InTransaction(func() error {
		if diffAction == "updated" && replacedID != "" {
			if err := db.SoftDeleteInsight(replacedID); err != nil {
				fmt.Fprintf(os.Stderr, "warning: soft-delete %s: %v\n", replacedID, err)
			} else {
				db.LogOp("diff-replace", replacedID, fmt.Sprintf("principal=%s replaced by %s", req.Auth.Principal, insight.ID))
				delete(embedCache, replacedID)
			}
		}
		if err := db.InsertInsight(insight); err != nil {
			return err
		}
		if embeddingBlob != nil {
			if err := db.UpdateEmbedding(insight.ID, embeddingBlob); err != nil {
				return err
			}
			embedded = true
			if embedCache != nil {
				embedCache[insight.ID] = embeddingVec
			}
		}
		engine := graph.NewEngineWithEntityMode(db, embedCache, entityMode)
		edgeStats = engine.OnInsightCreated(insight)
		if len(insight.Entities) > 0 {
			_ = db.UpdateEntities(insight.ID, insight.Entities)
		}
		ei, _ = db.RefreshEffectiveImportance(insight.ID)
		pruned, _ = db.AutoPrune(store.MaxInsights, []string{insight.ID})
		db.LogOp("remember", insight.ID, fmt.Sprintf("principal=%s %s", req.Auth.Principal, insight.Content))
		return nil
	})
	if err != nil {
		return nil, err
	}

	semanticCandidates := graph.FindSemanticCandidates(db, insight, embedCache)
	if semanticCandidates == nil {
		semanticCandidates = []graph.SemanticCandidate{}
	}
	causalCandidates := graph.FindCausalCandidates(db, insight)
	if causalCandidates == nil {
		causalCandidates = []graph.CausalCandidate{}
	}
	output := map[string]any{
		"id":                   insight.ID,
		"content":              insight.Content,
		"category":             insight.Category,
		"importance":           insight.Importance,
		"tags":                 insight.Tags,
		"entities":             insight.Entities,
		"action":               diffAction,
		"diff_suggestion":      string(diffSuggestion),
		"created_at":           insight.CreatedAt.Format(time.RFC3339),
		"edges_created":        edgeStats,
		"semantic_candidates":  semanticCandidates,
		"causal_candidates":    causalCandidates,
		"embedded":             embedded,
		"effective_importance": ei,
		"auto_pruned":          pruned,
		"principal":            req.Auth.Principal,
	}
	if replacedID != "" {
		output["replaced_id"] = replacedID
	}
	return encode(output)
}

func (s Service) Link(req remoteapi.LinkRequest) ([]byte, error) {
	edgeType := model.EdgeType(req.Type)
	if edgeType == "" {
		edgeType = model.EdgeSemantic
	}
	if !model.ValidEdgeTypes[edgeType] {
		return nil, fmt.Errorf("invalid edge type %q", req.Type)
	}
	if req.Weight < 0 || req.Weight > 1 {
		return nil, fmt.Errorf("weight must be between 0.0 and 1.0, got %.2f", req.Weight)
	}
	db, err := s.openDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if src, err := db.GetInsightByID(req.SourceID); err != nil || src == nil {
		return nil, fmt.Errorf("source insight %s not found", req.SourceID)
	}
	if tgt, err := db.GetInsightByID(req.TargetID); err != nil || tgt == nil {
		return nil, fmt.Errorf("target insight %s not found", req.TargetID)
	}
	metadata := map[string]string{"created_by": req.Auth.Principal}
	if req.MetaJSON != "" {
		if err := json.Unmarshal([]byte(req.MetaJSON), &metadata); err != nil {
			return nil, fmt.Errorf("invalid metadata JSON: %w", err)
		}
		metadata["created_by"] = req.Auth.Principal
	}
	now := time.Now().UTC()
	for _, edge := range []*model.Edge{
		{SourceID: req.SourceID, TargetID: req.TargetID, EdgeType: edgeType, Weight: req.Weight, Metadata: metadata, CreatedAt: now},
		{SourceID: req.TargetID, TargetID: req.SourceID, EdgeType: edgeType, Weight: req.Weight, Metadata: metadata, CreatedAt: now},
	} {
		if err := db.InsertEdge(edge); err != nil {
			return nil, err
		}
	}
	db.LogOp("link", req.SourceID, fmt.Sprintf("principal=%s %s→%s type=%s weight=%.2f", req.Auth.Principal, truncID(req.SourceID), truncID(req.TargetID), edgeType, req.Weight))
	return encode(map[string]any{
		"status":    "linked",
		"source_id": req.SourceID,
		"target_id": req.TargetID,
		"edge_type": edgeType,
		"weight":    req.Weight,
		"metadata":  metadata,
	})
}

func (s Service) Forget(req remoteapi.ForgetRequest) ([]byte, error) {
	db, err := s.openDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := db.SoftDeleteInsight(req.ID); err != nil {
		return nil, err
	}
	db.LogOp("forget", req.ID, fmt.Sprintf("principal=%s", req.Auth.Principal))
	return encode(map[string]any{
		"id":      req.ID,
		"status":  "deleted",
		"message": "Insight soft-deleted successfully",
	})
}

func (s Service) Log(req remoteapi.LogRequest) ([]byte, error) {
	if req.Limit <= 0 {
		req.Limit = 20
	}
	db, err := s.openDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	entries, err := db.GetOplog(req.Limit)
	if err != nil {
		return nil, err
	}
	return encode(entries)
}

type relatedResult struct {
	ID         string `json:"id"`
	Content    string `json:"content"`
	Category   string `json:"category"`
	Importance int    `json:"importance"`
	Depth      int    `json:"depth"`
	EdgeType   string `json:"via_edge_type,omitempty"`
}

func (s Service) Related(req remoteapi.RelatedRequest) ([]byte, error) {
	if req.Depth <= 0 {
		req.Depth = 2
	}
	db, err := s.openDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	start, err := db.GetInsightByID(req.ID)
	if err != nil {
		return nil, fmt.Errorf("insight not found: %w", err)
	}
	var edgeFilter model.EdgeType
	if req.EdgeType != "" {
		et := model.EdgeType(req.EdgeType)
		if !model.ValidEdgeTypes[et] {
			return nil, fmt.Errorf("invalid edge type %q", req.EdgeType)
		}
		edgeFilter = et
	}
	nodes := graph.BFS(db, start.ID, graph.BFSOptions{MaxDepth: req.Depth, EdgeFilter: edgeFilter})
	results := make([]relatedResult, 0, len(nodes))
	for _, n := range nodes {
		results = append(results, relatedResult{
			ID:         n.Insight.ID,
			Content:    n.Insight.Content,
			Category:   string(n.Insight.Category),
			Importance: n.Insight.Importance,
			Depth:      n.Hop,
			EdgeType:   string(n.ViaEdge.EdgeType),
		})
	}
	db.LogOp("related", req.ID, fmt.Sprintf("principal=%s depth=%d hits=%d", req.Auth.Principal, req.Depth, len(results)))
	return encode(results)
}

func (s Service) GC(req remoteapi.GCRequest) ([]byte, error) {
	if req.Limit <= 0 {
		req.Limit = 20
	}
	db, err := s.openDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if req.KeepID != "" {
		ins, err := db.GetInsightByID(req.KeepID)
		if err != nil || ins == nil {
			return nil, fmt.Errorf("insight %s not found", req.KeepID)
		}
		if err := db.BoostRetention(req.KeepID); err != nil {
			return nil, err
		}
		ei, _ := db.RefreshEffectiveImportance(req.KeepID)
		db.LogOp("gc_keep", req.KeepID, fmt.Sprintf("principal=%s %s", req.Auth.Principal, ins.Content))
		return encode(map[string]any{
			"status":               "retained",
			"id":                   req.KeepID,
			"content":              ins.Content,
			"new_access":           ins.AccessCount + 3,
			"effective_importance": ei,
			"immune":               store.IsImmune(ins.Importance, ins.AccessCount+3),
		})
	}
	candidates, total, err := db.GetRetentionCandidates(req.Threshold, req.Limit)
	if err != nil {
		return nil, err
	}
	db.LogOp("gc", "", fmt.Sprintf("principal=%s threshold=%.2f found=%d total=%d", req.Auth.Principal, req.Threshold, len(candidates), total))
	return encode(map[string]any{
		"total_insights":   total,
		"threshold":        req.Threshold,
		"candidates_found": len(candidates),
		"candidates":       candidates,
		"max_insights":     store.MaxInsights,
		"actions": map[string]string{
			"purge": "mnemon forget <id>",
			"keep":  "mnemon gc --keep <id>",
		},
	})
}

type receiptDocument struct {
	Schema      string           `json:"schema"`
	GeneratedAt string           `json:"generated_at"`
	Store       string           `json:"store"`
	Limit       int              `json:"limit"`
	Count       int              `json:"count"`
	Privacy     map[string]any   `json:"privacy"`
	Events      []map[string]any `json:"events"`
}

func hashIfPresent(value string) string {
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (s Service) Receipt(req remoteapi.ReceiptRequest) ([]byte, error) {
	if req.Limit <= 0 {
		req.Limit = 20
	}
	db, err := s.openDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	entries, err := db.GetOplog(req.Limit)
	if err != nil {
		return nil, err
	}
	events := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		events = append(events, map[string]any{
			"event_name":      "mnemon.memory.operation.observed",
			"operation":       entry.Operation,
			"created_at":      entry.CreatedAt,
			"insight_id_hash": hashIfPresent(entry.InsightID),
			"detail_hash":     hashIfPresent(entry.Detail),
			"detail_present":  entry.Detail != "",
		})
	}
	storeName := s.StoreName
	if storeName == "" {
		storeName = store.DefaultStoreName
	}
	return encode(receiptDocument{
		Schema:      "mnemon.memory.receipt.v1",
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Store:       storeName,
		Limit:       req.Limit,
		Count:       len(events),
		Privacy: map[string]any{
			"raw_detail_included": false,
			"hash_algorithm":      "sha256",
			"note":                "Raw memory contents, recall queries, paths, and operation details are omitted; only hashes and operation metadata are emitted.",
		},
		Events: events,
	})
}

func (s Service) Embed(req remoteapi.EmbedRequest) ([]byte, error) {
	db, err := s.openDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	ec := embed.NewClientWithModel(s.EmbedModel)
	if req.Status {
		total, embedded, err := db.EmbeddingStats()
		if err != nil {
			return nil, err
		}
		return encode(map[string]any{
			"total_insights":   total,
			"embedded":         embedded,
			"coverage":         fmt.Sprintf("%.0f%%", float64(embedded)/float64(max(total, 1))*100),
			"ollama_available": ec.Available(),
			"model":            ec.Model(),
		})
	}
	if !ec.Available() {
		return nil, fmt.Errorf("Ollama not available at %s", ec.Endpoint())
	}
	if req.ID != "" {
		ins, err := db.GetInsightByID(req.ID)
		if err != nil || ins == nil {
			return nil, fmt.Errorf("insight %s not found", req.ID)
		}
		vec, err := ec.Embed(ins.Content)
		if err != nil {
			return nil, err
		}
		if err := db.UpdateEmbedding(req.ID, embed.SerializeVector(vec)); err != nil {
			return nil, err
		}
		db.LogOp("embed", req.ID, fmt.Sprintf("principal=%s dim=%d model=%s", req.Auth.Principal, len(vec), ec.Model()))
		return encode(map[string]any{"status": "embedded", "id": req.ID, "dimension": len(vec), "model": ec.Model()})
	}
	if !req.All {
		return nil, fmt.Errorf("specify --all to backfill, --status to check coverage, or provide an insight ID")
	}
	missing, err := db.GetInsightsWithoutEmbedding(0)
	if err != nil {
		return nil, err
	}
	if len(missing) == 0 {
		return encode(map[string]any{"status": "complete", "message": "all insights already have embeddings"})
	}
	succeeded, failed := 0, 0
	for _, ins := range missing {
		vec, err := ec.Embed(ins.Content)
		if err != nil {
			failed++
			continue
		}
		if err := db.UpdateEmbedding(ins.ID, embed.SerializeVector(vec)); err != nil {
			failed++
			continue
		}
		succeeded++
	}
	db.LogOp("embed:backfill", "", fmt.Sprintf("principal=%s succeeded=%d failed=%d model=%s", req.Auth.Principal, succeeded, failed, ec.Model()))
	return encode(map[string]any{"status": "backfill_complete", "succeeded": succeeded, "failed": failed, "model": ec.Model()})
}

type importResult struct {
	Index   int    `json:"index"`
	ID      string `json:"id"`
	Content string `json:"content"`
	Action  string `json:"action"`
	Error   string `json:"error,omitempty"`
}

func (s Service) Import(req remoteapi.ImportRequest) ([]byte, error) {
	var draft importdraft.MemoryDraft
	if err := json.Unmarshal(req.Draft, &draft); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}
	if err := draft.Validate(); err != nil {
		return nil, fmt.Errorf("invalid draft: %w", err)
	}
	if req.DryRun {
		return encode(map[string]any{
			"status":   "dry_run_ok",
			"insights": len(draft.Insights),
			"edges":    len(draft.Edges),
		})
	}
	results := make([]importResult, 0, len(draft.Insights))
	imported := make(map[int]string, len(draft.Insights))
	for idx, di := range draft.Insights {
		tags := strings.Join(di.Tags, ",")
		entities := strings.Join(di.Entities, ",")
		source := draft.ResolvedSource(idx)
		out, err := s.Remember(remoteapi.RememberRequest{
			Auth:       req.Auth,
			Content:    di.Content,
			Category:   di.Category,
			Importance: di.Importance,
			Tags:       tags,
			Source:     source,
			Entities:   entities,
			EntityMode: string(graph.EntityModeMerge),
			NoDiff:     req.NoDiff,
			Agent:      req.Agent,
		})
		if err != nil {
			results = append(results, importResult{Index: idx, Content: di.Content, Error: err.Error()})
			continue
		}
		var payload struct {
			ID      string `json:"id"`
			Content string `json:"content"`
			Action  string `json:"action"`
		}
		if err := json.Unmarshal(out, &payload); err != nil {
			results = append(results, importResult{Index: idx, Content: di.Content, Error: err.Error()})
			continue
		}
		imported[idx] = payload.ID
		results = append(results, importResult{Index: idx, ID: payload.ID, Content: payload.Content, Action: payload.Action})
	}

	edgesInserted := 0
	db, err := s.openDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err := db.InTransaction(func() error {
		for _, de := range draft.Edges {
			srcID, srcOK := imported[de.SourceIndex]
			tgtID, tgtOK := imported[de.TargetIndex]
			if !srcOK || !tgtOK {
				continue
			}
			w := de.Weight
			if w == 0 {
				w = 0.5
			}
			meta := map[string]string{"created_by": req.Auth.Principal}
			if de.Reason != "" {
				meta["reason"] = de.Reason
			}
			if err := db.InsertEdge(&model.Edge{
				SourceID:  srcID,
				TargetID:  tgtID,
				EdgeType:  model.EdgeType(de.EdgeType),
				Weight:    w,
				Metadata:  meta,
				CreatedAt: time.Now().UTC(),
			}); err != nil {
				return err
			}
			edgesInserted++
		}
		return nil
	}); err != nil {
		return nil, err
	}

	countAction := func(action string) int {
		n := 0
		for _, r := range results {
			if r.Action == action {
				n++
			}
		}
		return n
	}
	countErrors := func() int {
		n := 0
		for _, r := range results {
			if r.Error != "" {
				n++
			}
		}
		return n
	}
	db.LogOp("import", "", fmt.Sprintf("principal=%s insights=%d edges=%d", req.Auth.Principal, len(draft.Insights), edgesInserted))
	return encode(map[string]any{
		"imported":       countAction("added"),
		"updated":        countAction("updated"),
		"skipped":        countAction("skipped"),
		"errors":         countErrors(),
		"edges_inserted": edgesInserted,
		"auto_pruned":    0,
		"results":        results,
	})
}

func (s Service) Viz(req remoteapi.VizRequest) (string, error) {
	db, err := s.openDB()
	if err != nil {
		return "", err
	}
	defer db.Close()
	insights, err := db.GetAllActiveInsights()
	if err != nil {
		return "", err
	}
	edges, err := db.GetAllEdges()
	if err != nil {
		return "", err
	}
	switch req.Format {
	case "", "dot":
		return renderDOT(insights, edges), nil
	case "html":
		return renderHTML(insights, edges), nil
	default:
		return "", fmt.Errorf("unsupported format: %s (use dot or html)", req.Format)
	}
}

func nodeLabel(i *model.Insight) string {
	content := i.Content
	if len(content) > 60 {
		content = content[:60] + "..."
	}
	return fmt.Sprintf("[%s] %s", i.Category, content)
}

func categoryColor(c model.Category) string {
	switch c {
	case model.CategoryDecision:
		return "#e74c3c"
	case model.CategoryFact:
		return "#3498db"
	case model.CategoryInsight:
		return "#9b59b6"
	case model.CategoryPreference:
		return "#2ecc71"
	case model.CategoryContext:
		return "#f39c12"
	default:
		return "#95a5a6"
	}
}

func edgeColor(t model.EdgeType) string {
	switch t {
	case model.EdgeTemporal:
		return "#aaaaaa"
	case model.EdgeSemantic:
		return "#3498db"
	case model.EdgeCausal:
		return "#e74c3c"
	case model.EdgeEntity:
		return "#2ecc71"
	default:
		return "#cccccc"
	}
}

func renderDOT(insights []*model.Insight, edges []*model.Edge) string {
	var b strings.Builder
	b.WriteString("digraph mnemon {\n")
	b.WriteString("  rankdir=LR;\n")
	b.WriteString("  node [shape=box, style=\"filled,rounded\", fontsize=10, fontname=\"Helvetica\"];\n")
	b.WriteString("  edge [fontsize=8, fontname=\"Helvetica\"];\n\n")
	active := make(map[string]bool, len(insights))
	for _, i := range insights {
		active[i.ID] = true
	}
	for _, i := range insights {
		label := strings.ReplaceAll(nodeLabel(i), `"`, `\"`)
		label = strings.ReplaceAll(label, "\n", " ")
		shortID := i.ID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}
		b.WriteString(fmt.Sprintf("  %q [label=%q, fillcolor=%q, fontcolor=\"white\"];\n", i.ID, shortID+": "+label, categoryColor(i.Category)))
	}
	b.WriteString("\n")
	for _, e := range edges {
		if !active[e.SourceID] || !active[e.TargetID] {
			continue
		}
		color := edgeColor(e.EdgeType)
		edgeLabel := string(e.EdgeType)
		if subType := e.Metadata["sub_type"]; subType != "" {
			edgeLabel = subType
		}
		b.WriteString(fmt.Sprintf("  %q -> %q [label=%q, color=%q, fontcolor=%q];\n", e.SourceID, e.TargetID, edgeLabel, color, color))
	}
	b.WriteString("}\n")
	return b.String()
}

func jsStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func renderHTML(insights []*model.Insight, edges []*model.Edge) string {
	active := make(map[string]bool, len(insights))
	for _, i := range insights {
		active[i.ID] = true
	}
	var nodes strings.Builder
	for idx, i := range insights {
		if idx > 0 {
			nodes.WriteString(",\n")
		}
		shortID := i.ID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}
		label := strings.ReplaceAll(nodeLabel(i), "\n", " ")
		nodes.WriteString(fmt.Sprintf(`{id:%s,label:%s,title:%s,color:%s,font:{color:"white"}}`,
			jsStr(i.ID), jsStr(shortID+": "+label), jsStr(strings.ReplaceAll(i.Content, "\n", "\\n")), jsStr(categoryColor(i.Category))))
	}
	var edgesJS strings.Builder
	first := true
	for _, e := range edges {
		if !active[e.SourceID] || !active[e.TargetID] {
			continue
		}
		if !first {
			edgesJS.WriteString(",\n")
		}
		first = false
		color := edgeColor(e.EdgeType)
		edgeLabel := string(e.EdgeType)
		if subType := e.Metadata["sub_type"]; subType != "" {
			edgeLabel = subType
		}
		edgesJS.WriteString(fmt.Sprintf(`{from:%s,to:%s,label:%s,color:{color:%s},arrows:"to",font:{color:%s,size:10}}`,
			jsStr(e.SourceID), jsStr(e.TargetID), jsStr(edgeLabel), jsStr(color), jsStr(color)))
	}
	return fmt.Sprintf(htmlTemplate, nodes.String(), edgesJS.String())
}

const htmlTemplate = `<!DOCTYPE html>
<html><head><meta charset="utf-8"><title>Mnemon Knowledge Graph</title>
<script src="https://unpkg.com/vis-network/standalone/umd/vis-network.min.js"></script>
<style>body{margin:0;padding:0;background:#1a1a2e;font-family:sans-serif}#graph{width:100vw;height:100vh}</style>
</head><body><div id="graph"></div><script>
var nodes = new vis.DataSet([%s]);
var edges = new vis.DataSet([%s]);
new vis.Network(document.getElementById("graph"), {nodes:nodes, edges:edges}, {
  physics:{solver:"forceAtlas2Based", forceAtlas2Based:{gravitationalConstant:-30}},
  interaction:{hover:true, tooltipDelay:100},
  nodes:{shape:"box", margin:8, borderWidth:0, font:{size:11}},
  edges:{smooth:{type:"continuous"}, font:{size:9}}
});
</script></body></html>`

func truncID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
