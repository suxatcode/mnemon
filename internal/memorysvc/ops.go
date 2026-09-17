package memorysvc

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/mnemon-dev/mnemon/internal/embed"
	"github.com/mnemon-dev/mnemon/internal/model"
	"github.com/mnemon-dev/mnemon/internal/store"
)

func (s *Service) Status(actor Actor) (Result, error) {
	stats, err := s.db.GetStats()
	if err != nil {
		return Result{}, err
	}
	var fileSize int64
	if fi, err := os.Stat(s.db.Path()); err == nil {
		fileSize = fi.Size()
	}
	return s.encode(map[string]any{
		"total_insights":   stats.Total,
		"deleted_insights": stats.DeletedCount,
		"by_category":      stats.ByCategory,
		"edge_count":       stats.EdgeCount,
		"top_entities":     stats.TopEntities,
		"oplog_count":      stats.OplogCount,
		"db_path":          s.db.Path(),
		"db_size_bytes":    fileSize,
		"remote":           s.enforceACL,
		"dialect":          s.db.Dialect().String(),
		"max_insights":     s.maxInsights,
		"principal":        actor.Principal,
	}, nil)
}

type LinkInput struct {
	SourceID string
	TargetID string
	Type     string
	Weight   float64
	MetaJSON string
}

func (s *Service) Link(actor Actor, req LinkInput) (Result, error) {
	edgeType := model.EdgeType(req.Type)
	if edgeType == "" {
		edgeType = model.EdgeSemantic
	}
	if !model.ValidEdgeTypes[edgeType] {
		return Result{}, fmt.Errorf("invalid edge type %q", req.Type)
	}
	if req.Weight < 0 || req.Weight > 1 {
		return Result{}, fmt.Errorf("weight must be between 0.0 and 1.0, got %.2f", req.Weight)
	}
	if src, err := s.db.GetInsightByID(req.SourceID); err != nil || src == nil {
		return Result{}, fmt.Errorf("source insight %s not found", req.SourceID)
	}
	if tgt, err := s.db.GetInsightByID(req.TargetID); err != nil || tgt == nil {
		return Result{}, fmt.Errorf("target insight %s not found", req.TargetID)
	}
	createdBy := actor.Principal
	if createdBy == "" {
		createdBy = "local"
	}
	metadata := map[string]string{"created_by": createdBy}
	if req.MetaJSON != "" {
		if err := json.Unmarshal([]byte(req.MetaJSON), &metadata); err != nil {
			return Result{}, fmt.Errorf("invalid metadata JSON: %w", err)
		}
		metadata["created_by"] = createdBy
	}
	now := time.Now().UTC()
	for _, edge := range []*model.Edge{
		{SourceID: req.SourceID, TargetID: req.TargetID, EdgeType: edgeType, Weight: req.Weight, Metadata: metadata, CreatedAt: now},
		{SourceID: req.TargetID, TargetID: req.SourceID, EdgeType: edgeType, Weight: req.Weight, Metadata: metadata, CreatedAt: now},
	} {
		if err := s.db.InsertEdge(edge); err != nil {
			return Result{}, err
		}
	}
	s.db.LogOp("link", req.SourceID, fmt.Sprintf("principal=%s %s→%s type=%s weight=%.2f", actor.Principal, truncID(req.SourceID), truncID(req.TargetID), edgeType, req.Weight))
	return s.encode(map[string]any{
		"status":    "linked",
		"source_id": req.SourceID,
		"target_id": req.TargetID,
		"edge_type": edgeType,
		"weight":    req.Weight,
		"metadata":  metadata,
	}, nil)
}

type ForgetInput struct {
	ID string
}

func (s *Service) Forget(actor Actor, req ForgetInput) (Result, error) {
	ins, err := s.db.GetInsightByID(req.ID)
	if err != nil || ins == nil {
		return Result{}, fmt.Errorf("insight %s not found or already deleted", req.ID)
	}
	if err := s.canMutate(actor, ins); err != nil {
		return Result{}, err
	}
	if err := s.db.SoftDeleteInsight(req.ID); err != nil {
		return Result{}, err
	}
	s.db.LogOp("forget", req.ID, fmt.Sprintf("principal=%s", actor.Principal))
	return s.encode(map[string]any{
		"id":      req.ID,
		"status":  "deleted",
		"message": "Insight soft-deleted successfully",
	}, nil)
}

type LogInput struct {
	Limit int
}

func (s *Service) Log(actor Actor, req LogInput) (Result, error) {
	if req.Limit <= 0 {
		req.Limit = 20
	}
	entries, err := s.db.GetOplog(req.Limit)
	if err != nil {
		return Result{}, err
	}
	return s.encode(entries, nil)
}

type GCInput struct {
	Threshold float64
	Limit     int
	KeepID    string
}

func (s *Service) GC(actor Actor, req GCInput) (Result, error) {
	if req.Limit <= 0 {
		req.Limit = 20
	}
	if req.KeepID != "" {
		ins, err := s.db.GetInsightByID(req.KeepID)
		if err != nil || ins == nil {
			return Result{}, fmt.Errorf("insight %s not found", req.KeepID)
		}
		if err := s.canMutate(actor, ins); err != nil {
			return Result{}, err
		}
		if err := s.db.BoostRetention(req.KeepID); err != nil {
			return Result{}, err
		}
		ei, _ := s.db.RefreshEffectiveImportance(req.KeepID)
		s.db.LogOp("gc_keep", req.KeepID, fmt.Sprintf("principal=%s %s", actor.Principal, ins.Content))
		return s.encode(map[string]any{
			"status":               "retained",
			"id":                   req.KeepID,
			"content":              ins.Content,
			"new_access":           ins.AccessCount + 3,
			"effective_importance": ei,
			"immune":               store.IsImmune(ins.Importance, ins.AccessCount+3),
		}, nil)
	}
	owner := ""
	if s.enforceACL {
		owner = actor.Principal
	}
	candidates, total, err := s.db.GetRetentionCandidatesOwned(req.Threshold, req.Limit, owner)
	if err != nil {
		return Result{}, err
	}
	s.db.LogOp("gc", "", fmt.Sprintf("principal=%s threshold=%.2f found=%d total=%d", actor.Principal, req.Threshold, len(candidates), total))
	return s.encode(map[string]any{
		"total_insights":   total,
		"threshold":        req.Threshold,
		"candidates_found": len(candidates),
		"candidates":       candidates,
		"max_insights":     s.maxInsights,
		"actions": map[string]string{
			"purge": "mnemon forget <id>",
			"keep":  "mnemon gc --keep <id>",
		},
	}, nil)
}

type ReceiptInput struct {
	Limit int
}

type ReceiptDocument struct {
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

func BuildReceipt(storeName string, limit int, entries []store.OplogEntry, generatedAt time.Time) ReceiptDocument {
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
	return ReceiptDocument{
		Schema:      "mnemon.memory.receipt.v1",
		GeneratedAt: generatedAt.Format(time.RFC3339),
		Store:       storeName,
		Limit:       limit,
		Count:       len(events),
		Privacy: map[string]any{
			"raw_detail_included": false,
			"hash_algorithm":      "sha256",
			"note":                "Raw memory contents, recall queries, paths, and operation details are omitted; only hashes and operation metadata are emitted.",
		},
		Events: events,
	}
}

func (s *Service) Receipt(actor Actor, req ReceiptInput) (Result, error) {
	if req.Limit <= 0 {
		req.Limit = 20
	}
	entries, err := s.db.GetOplog(req.Limit)
	if err != nil {
		return Result{}, err
	}
	return s.encode(BuildReceipt(s.storeName, req.Limit, entries, time.Now().UTC()), nil)
}

type EmbedInput struct {
	ID     string
	All    bool
	Status bool
}

func (s *Service) Embed(actor Actor, req EmbedInput) (Result, error) {
	ec := embed.NewClientWithModel(s.embedModel)
	if req.Status {
		total, embedded, err := s.db.EmbeddingStats()
		if err != nil {
			return Result{}, err
		}
		return s.encode(map[string]any{
			"total_insights":   total,
			"embedded":         embedded,
			"coverage":         fmt.Sprintf("%.0f%%", float64(embedded)/float64(max(total, 1))*100),
			"ollama_available": ec.Available(),
			"model":            ec.Model(),
		}, nil)
	}
	if !ec.Available() {
		return Result{}, fmt.Errorf("Ollama not available at %s", ec.Endpoint())
	}
	if req.ID != "" {
		ins, err := s.db.GetInsightByID(req.ID)
		if err != nil || ins == nil {
			return Result{}, fmt.Errorf("insight %s not found", req.ID)
		}
		if err := s.canMutate(actor, ins); err != nil {
			return Result{}, err
		}
		vec, err := ec.Embed(ins.Content)
		if err != nil {
			return Result{}, err
		}
		if err := s.db.UpdateEmbedding(req.ID, embed.SerializeVector(vec)); err != nil {
			return Result{}, err
		}
		s.db.LogOp("embed", req.ID, fmt.Sprintf("principal=%s dim=%d model=%s", actor.Principal, len(vec), ec.Model()))
		return s.encode(map[string]any{"status": "embedded", "id": req.ID, "dimension": len(vec), "model": ec.Model()}, nil)
	}
	if !req.All {
		return Result{}, fmt.Errorf("specify --all to backfill, --status to check coverage, or provide an insight ID")
	}
	missing, err := s.db.GetInsightsWithoutEmbedding(0)
	if err != nil {
		return Result{}, err
	}
	if len(missing) == 0 {
		return s.encode(map[string]any{"status": "complete", "message": "all insights already have embeddings"}, nil)
	}
	succeeded, failed := 0, 0
	for _, ins := range missing {
		if s.canMutate(actor, ins) != nil {
			continue
		}
		vec, err := ec.Embed(ins.Content)
		if err != nil {
			failed++
			continue
		}
		if err := s.db.UpdateEmbedding(ins.ID, embed.SerializeVector(vec)); err != nil {
			failed++
			continue
		}
		succeeded++
	}
	s.db.LogOp("embed:backfill", "", fmt.Sprintf("principal=%s succeeded=%d failed=%d model=%s", actor.Principal, succeeded, failed, ec.Model()))
	return s.encode(map[string]any{"status": "backfill_complete", "succeeded": succeeded, "failed": failed, "model": ec.Model()}, nil)
}
