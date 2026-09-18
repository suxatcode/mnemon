package memorysvc

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mnemon-dev/mnemon/internal/embed"
	"github.com/mnemon-dev/mnemon/internal/graph"
	"github.com/mnemon-dev/mnemon/internal/importdraft"
	"github.com/mnemon-dev/mnemon/internal/model"
	"github.com/mnemon-dev/mnemon/internal/search"
	"github.com/mnemon-dev/mnemon/internal/store"
)

type ImportInput struct {
	Draft  []byte
	NoDiff bool
	DryRun bool
}

type ImportResult struct {
	Index   int    `json:"index"`
	ID      string `json:"id"`
	Content string `json:"content"`
	Action  string `json:"action"`
	Error   string `json:"error,omitempty"`
}

func (s *Service) Import(actor Actor, req ImportInput) (Result, error) {
	if !req.DryRun {
		if err := s.assertWritable(); err != nil {
			return Result{}, err
		}
	}
	var warnings []string
	var draft importdraft.MemoryDraft
	if err := json.Unmarshal(req.Draft, &draft); err != nil {
		return Result{}, fmt.Errorf("parse JSON: %w", err)
	}
	if err := draft.Validate(); err != nil {
		return Result{}, fmt.Errorf("invalid draft: %w", err)
	}
	if req.DryRun {
		return s.encode(map[string]any{
			"status":   "dry_run_ok",
			"insights": len(draft.Insights),
			"edges":    len(draft.Edges),
		}, warnings)
	}

	db := s.db
	ec := embed.NewClientWithModel(s.embedModel)
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

	imported := make(map[int]string, len(draft.Insights))
	importedIDs := make(map[string]bool, len(draft.Insights))
	importedSources := make(map[string]bool)
	refreshIDs := make(map[string]bool)
	results := make([]ImportResult, 0, len(draft.Insights))
	owner, layer := s.diffOwner(actor)
	if actor.Principal == "" {
		actor.Principal = model.LocalOwner
	}

	for idx, di := range draft.Insights {
		cat := model.Category(di.Category)
		if cat == "" {
			cat = model.CategoryGeneral
		}
		imp := di.Importance
		if imp == 0 {
			imp = 3
		}
		tags := di.Tags
		if tags == nil {
			tags = []string{}
		}
		tags = addProvenanceTags(tags, actor.Principal, actor.Agent)
		entities := di.Entities
		if entities == nil {
			entities = []string{}
		}
		var createdAt time.Time
		if di.CreatedAt != "" {
			if t, err := time.Parse(time.RFC3339, di.CreatedAt); err == nil {
				createdAt = t.UTC()
			}
		}
		if createdAt.IsZero() {
			createdAt = time.Now().UTC()
		}
		insight := &model.Insight{
			ID:             uuid.New().String(),
			Content:        di.Content,
			Category:       cat,
			Importance:     imp,
			Tags:           tags,
			Entities:       entities,
			Source:         draft.ResolvedSource(idx),
			CreatedAt:      createdAt,
			UpdatedAt:      createdAt,
			OwnerPrincipal: actor.Principal,
			Layer:          actor.layer(),
		}

		var embeddingBlob []byte
		var embeddingVec []float64
		if ec.Available() {
			if vec, err := ec.Embed(insight.Content); err == nil {
				embeddingVec = vec
				embeddingBlob = embed.SerializeVector(vec)
			}
		}

		action := "added"
		replacedID := ""
		if !req.NoDiff {
			allInsights, err := db.GetActiveInsightsForDiff(owner, layer)
			if err != nil {
				results = append(results, ImportResult{Index: idx, ID: insight.ID, Content: insight.Content, Error: err.Error()})
				continue
			}
			opts := search.DiffOptions{Limit: 5, NewEmbedding: embeddingVec}
			if embedCache != nil {
				opts.ExistingEmbed = make([]search.EmbeddedItem, 0, len(embedCache))
				for id, v := range embedCache {
					opts.ExistingEmbed = append(opts.ExistingEmbed, search.EmbeddedItem{ID: id, Embedding: v})
				}
			}
			result := search.Diff(allInsights, insight.Content, opts)
			switch result.Suggestion {
			case search.DiffDuplicate:
				action = "skipped"
				if len(result.Matches) > 0 {
					replacedID = result.Matches[0].ID
				}
			case search.DiffConflict, search.DiffUpdate:
				action = "updated"
				if len(result.Matches) > 0 {
					replacedID = result.Matches[0].ID
				}
			}
		}

		if action == "skipped" {
			db.LogOp("import-skip", insight.ID, fmt.Sprintf("duplicate of %s", replacedID))
			if replacedID != "" {
				imported[idx] = replacedID
			} else {
				imported[idx] = insight.ID
			}
			results = append(results, ImportResult{Index: idx, ID: imported[idx], Content: insight.Content, Action: action})
			continue
		}
		if replacedID != "" {
			existing, err := db.GetInsightByID(replacedID)
			if err == nil && s.canMutate(actor, existing) != nil {
				action = "added"
				replacedID = ""
			}
		}

		err := db.InTransaction(func(tx *store.DB) error {
			if action == "updated" && replacedID != "" {
				if err := tx.SoftDeleteInsight(replacedID); err != nil {
					return fmt.Errorf("soft-delete %s: %w", replacedID, err)
				}
				tx.LogOp("import-replace", replacedID, fmt.Sprintf("replaced by %s", insight.ID))
				delete(embedCache, replacedID)
			}
			if err := tx.InsertInsight(insight); err != nil {
				return fmt.Errorf("insert insight: %w", err)
			}
			if embeddingBlob != nil {
				if err := tx.UpdateEmbedding(insight.ID, embeddingBlob); err != nil {
					return fmt.Errorf("update embedding: %w", err)
				}
				if embedCache != nil {
					embedCache[insight.ID] = embeddingVec
				}
			}
			engine := graph.NewEngineWithOptions(tx, embedCache, graph.EngineOptions{
				EntityMode:   graph.EntityModeMerge,
				TemporalMode: graph.TemporalDisabled,
			})
			if _, err := engine.OnInsightCreated(insight); err != nil {
				return err
			}
			if len(insight.Entities) > 0 {
				if err := tx.UpdateEntities(insight.ID, insight.Entities); err != nil {
					return fmt.Errorf("update entities: %w", err)
				}
			}
			if _, err := tx.RefreshEffectiveImportance(insight.ID); err != nil {
				return fmt.Errorf("refresh EI for %s: %w", insight.ID, err)
			}
			tx.LogOp("import", insight.ID, insight.Content)
			return nil
		})
		if err != nil {
			embedCache = nil
			results = append(results, ImportResult{Index: idx, ID: insight.ID, Content: insight.Content, Error: err.Error()})
			continue
		}
		imported[idx] = insight.ID
		importedIDs[insight.ID] = true
		importedSources[insight.Source] = true
		refreshIDs[insight.ID] = true
		results = append(results, ImportResult{Index: idx, ID: insight.ID, Content: insight.Content, Action: action})
	}

	edgesInserted := 0
	pruned := 0
	if err := db.InTransaction(func(tx *store.DB) error {
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
			meta := map[string]string{"created_by": actor.Principal}
			if de.Reason != "" {
				meta["reason"] = de.Reason
			}
			if err := tx.InsertEdge(&model.Edge{
				SourceID:  srcID,
				TargetID:  tgtID,
				EdgeType:  model.EdgeType(de.EdgeType),
				Weight:    w,
				Metadata:  meta,
				CreatedAt: time.Now().UTC(),
			}); err != nil {
				return fmt.Errorf("insert explicit edge %d→%d: %w", de.SourceIndex, de.TargetIndex, err)
			}
			edgesInserted++
			refreshIDs[srcID] = true
			refreshIDs[tgtID] = true
		}
		repaired, touched, err := repairImportedTemporalEdges(tx, importedSources, importedIDs)
		if err != nil {
			return err
		}
		_ = repaired
		for id := range touched {
			refreshIDs[id] = true
		}
		for id := range refreshIDs {
			if _, err := tx.RefreshEffectiveImportance(id); err != nil {
				return fmt.Errorf("refresh EI for %s: %w", id, err)
			}
		}
		if actor.Role != RoleOrg {
			var pruneErr error
			pruned, pruneErr = tx.AutoPruneOwned(s.pruneOwner(actor), s.maxInsights, nil)
			return pruneErr
		}
		return nil
	}); err != nil {
		return Result{}, fmt.Errorf("finalize import graph: %w", err)
	}

	db.LogOp("import", "", fmt.Sprintf("principal=%s insights=%d edges=%d", actor.Principal, len(draft.Insights), edgesInserted))
	return s.encode(map[string]any{
		"imported":       countAction(results, "added"),
		"updated":        countAction(results, "updated"),
		"skipped":        countAction(results, "skipped"),
		"errors":         countErrors(results),
		"edges_inserted": edgesInserted,
		"auto_pruned":    pruned,
		"results":        results,
	}, warnings)
}

func repairImportedTemporalEdges(db *store.DB, sources map[string]bool, importedIDs map[string]bool) (int, map[string]bool, error) {
	touched := make(map[string]bool)
	if len(importedIDs) == 0 {
		return 0, touched, nil
	}
	inserted := 0
	for source := range sources {
		timeline, err := db.GetActiveInsightsBySourceOrdered(source)
		if err != nil {
			return inserted, touched, fmt.Errorf("load temporal timeline for source %q: %w", source, err)
		}
		if len(timeline) == 0 {
			continue
		}
		for idx, insight := range timeline {
			if !importedIDs[insight.ID] {
				continue
			}
			touched[insight.ID] = true
			prevExisting := nearestNonImportedBefore(timeline, importedIDs, idx)
			nextExisting := nearestNonImportedAfter(timeline, importedIDs, idx)
			if prevExisting != nil && nextExisting != nil {
				if err := db.DeleteEdge(prevExisting.ID, nextExisting.ID, model.EdgeTemporal); err != nil {
					return inserted, touched, fmt.Errorf("delete temporal edge %s→%s: %w", prevExisting.ID, nextExisting.ID, err)
				}
				if err := db.DeleteEdge(nextExisting.ID, prevExisting.ID, model.EdgeTemporal); err != nil {
					return inserted, touched, fmt.Errorf("delete temporal edge %s→%s: %w", nextExisting.ID, prevExisting.ID, err)
				}
				touched[prevExisting.ID] = true
				touched[nextExisting.ID] = true
			}
		}
		now := time.Now().UTC()
		for idx := 0; idx < len(timeline)-1; idx++ {
			prev := timeline[idx]
			next := timeline[idx+1]
			if !importedIDs[prev.ID] && !importedIDs[next.ID] {
				continue
			}
			if err := db.InsertEdge(&model.Edge{
				SourceID:  prev.ID,
				TargetID:  next.ID,
				EdgeType:  model.EdgeTemporal,
				Weight:    1.0,
				Metadata:  map[string]string{"sub_type": "backbone", "direction": "precedes"},
				CreatedAt: now,
			}); err != nil {
				return inserted, touched, fmt.Errorf("insert temporal edge %s→%s: %w", prev.ID, next.ID, err)
			}
			inserted++
			if err := db.InsertEdge(&model.Edge{
				SourceID:  next.ID,
				TargetID:  prev.ID,
				EdgeType:  model.EdgeTemporal,
				Weight:    1.0,
				Metadata:  map[string]string{"sub_type": "backbone", "direction": "succeeds"},
				CreatedAt: now,
			}); err != nil {
				return inserted, touched, fmt.Errorf("insert temporal edge %s→%s: %w", next.ID, prev.ID, err)
			}
			inserted++
			touched[prev.ID] = true
			touched[next.ID] = true
		}
	}
	return inserted, touched, nil
}

func nearestNonImportedBefore(timeline []*model.Insight, importedIDs map[string]bool, idx int) *model.Insight {
	for i := idx - 1; i >= 0; i-- {
		if !importedIDs[timeline[i].ID] {
			return timeline[i]
		}
	}
	return nil
}

func nearestNonImportedAfter(timeline []*model.Insight, importedIDs map[string]bool, idx int) *model.Insight {
	for i := idx + 1; i < len(timeline); i++ {
		if !importedIDs[timeline[i].ID] {
			return timeline[i]
		}
	}
	return nil
}

func countAction(results []ImportResult, action string) int {
	n := 0
	for _, r := range results {
		if r.Action == action {
			n++
		}
	}
	return n
}

func countErrors(results []ImportResult) int {
	n := 0
	for _, r := range results {
		if r.Error != "" {
			n++
		}
	}
	return n
}

type VizInput struct {
	Format string
}

func (s *Service) Viz(actor Actor, req VizInput) (Result, error) {
	insights, err := s.db.GetAllActiveInsights()
	if err != nil {
		return Result{}, err
	}
	edges, err := s.db.GetAllEdges()
	if err != nil {
		return Result{}, err
	}
	switch req.Format {
	case "", "dot":
		return s.textResult(renderDOT(insights, edges), nil), nil
	case "html":
		return s.textResult(renderHTML(insights, edges), nil), nil
	default:
		return Result{}, fmt.Errorf("unsupported format: %s (use dot or html)", req.Format)
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
