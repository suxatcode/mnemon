package memorysvc

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/mnemon-dev/mnemon/internal/embed"
	"github.com/mnemon-dev/mnemon/internal/graph"
	"github.com/mnemon-dev/mnemon/internal/model"
	"github.com/mnemon-dev/mnemon/internal/search"
	"github.com/mnemon-dev/mnemon/internal/store"
)

type RememberInput struct {
	Content    string
	Category   string
	Importance int
	Tags       string
	Source     string
	Entities   string
	EntityMode string
	NoDiff     bool
}

func (s *Service) Remember(actor Actor, req RememberInput) (Result, error) {
	if err := s.assertWritable(); err != nil {
		return Result{}, err
	}
	var warnings []string
	content := req.Content
	if len(content) > 8000 {
		return Result{}, fmt.Errorf("content too long (%d chars, max 8000)", len(content))
	}
	cat := model.Category(req.Category)
	if cat == "" {
		cat = model.CategoryGeneral
	}
	if !model.ValidCategories[cat] {
		return Result{}, fmt.Errorf("invalid category %q", req.Category)
	}
	if req.Importance == 0 {
		req.Importance = 3
	}
	if req.Importance < 1 || req.Importance > 5 {
		return Result{}, fmt.Errorf("importance must be 1-5, got %d", req.Importance)
	}
	if s.enforceACL && actor.Role != RoleOrg && actor.Role != RoleUser {
		return Result{}, fmt.Errorf("invalid actor role")
	}
	entityMode := graph.EntityMode(req.EntityMode)
	if entityMode == "" {
		entityMode = graph.EntityModeMerge
	}
	if !graph.ValidEntityMode(entityMode) {
		return Result{}, fmt.Errorf("invalid entity mode %q", req.EntityMode)
	}
	tags, err := parseCSV(req.Tags, 20, 100, "tag")
	if err != nil {
		return Result{}, err
	}
	agent := actor.Agent
	if agent == "" {
		agent = "mnemon-cli"
	}
	tags = addProvenanceTags(tags, actor.Principal, agent)
	entities, err := parseCSV(req.Entities, 50, 200, "entity")
	if err != nil {
		return Result{}, err
	}
	source := req.Source
	if source == "" {
		source = "agent"
	}
	if actor.Principal == "" {
		actor.Principal = model.LocalOwner
	}

	now := time.Now().UTC()
	insight := &model.Insight{
		ID:             uuid.New().String(),
		Content:        content,
		Category:       cat,
		Importance:     req.Importance,
		Tags:           tags,
		Entities:       entities,
		Source:         source,
		CreatedAt:      now,
		UpdatedAt:      now,
		OwnerPrincipal: actor.Principal,
		Layer:          actor.layer(),
	}

	db := s.db
	var embeddingBlob []byte
	var embeddingVec []float64
	ec := embed.NewClientWithModel(s.embedModel)
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

	owner, layer := s.diffOwner(actor)
	if req.NoDiff {
		diffAction = "added"
		diffSuggestion = search.DiffAdd
	} else {
		allInsights, err := db.GetActiveInsightsForDiff(owner, layer)
		if err != nil {
			return Result{}, err
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
		db.LogOp("diff-skip", insight.ID, fmt.Sprintf("principal=%s duplicate of %s", actor.Principal, replacedID))
		return s.encode(map[string]any{
			"id":              insight.ID,
			"content":         content,
			"action":          "skipped",
			"diff_suggestion": string(diffSuggestion),
			"replaced_id":     replacedID,
			"owner_principal": actor.Principal,
			"layer":           insight.Layer,
		}, warnings)
	}

	if replacedID != "" {
		existing, err := db.GetInsightByID(replacedID)
		if err == nil {
			if err := s.canMutate(actor, existing); err != nil {
				diffAction = "added"
				replacedID = ""
			}
		}
	}

	var edgeStats graph.EdgeStats
	var ei float64
	var pruned int
	var embedded bool
	err = db.InTransaction(func(tx *store.DB) error {
		if diffAction == "updated" && replacedID != "" {
			if err := tx.SoftDeleteInsight(replacedID); err != nil {
				return fmt.Errorf("soft-delete %s: %w", replacedID, err)
			}
			tx.LogOp("diff-replace", replacedID, fmt.Sprintf("principal=%s replaced by %s", actor.Principal, insight.ID))
			delete(embedCache, replacedID)
		}
		if err := tx.InsertInsight(insight); err != nil {
			return err
		}
		if embeddingBlob != nil {
			if err := tx.UpdateEmbedding(insight.ID, embeddingBlob); err != nil {
				return err
			}
			embedded = true
			if embedCache != nil {
				embedCache[insight.ID] = embeddingVec
			}
		}
		engine := graph.NewEngineWithEntityMode(tx, embedCache, entityMode)
		var statsErr error
		edgeStats, statsErr = engine.OnInsightCreated(insight)
		if statsErr != nil {
			return statsErr
		}
		if len(insight.Entities) > 0 {
			if err := tx.UpdateEntities(insight.ID, insight.Entities); err != nil {
				return fmt.Errorf("update entities: %w", err)
			}
		}
		var eiErr error
		ei, eiErr = tx.RefreshEffectiveImportance(insight.ID)
		if eiErr != nil {
			return fmt.Errorf("refresh EI: %w", eiErr)
		}
		if actor.Role != RoleOrg {
			var pruneErr error
			pruned, pruneErr = tx.AutoPruneOwned(s.pruneOwner(actor), s.maxInsights, []string{insight.ID})
			if pruneErr != nil {
				return fmt.Errorf("auto-prune: %w", pruneErr)
			}
		}
		tx.LogOp("remember", insight.ID, fmt.Sprintf("principal=%s %s", actor.Principal, insight.Content))
		return nil
	})
	if err != nil {
		return Result{}, err
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
		"owner_principal":      insight.OwnerPrincipal,
		"layer":                insight.Layer,
		"principal":            actor.Principal,
	}
	if replacedID != "" {
		output["replaced_id"] = replacedID
	}
	return s.encode(output, warnings)
}
