package graph

import (
	"regexp"
	"time"

	"github.com/Grivn/mnemon/internal/model"
	"github.com/Grivn/mnemon/internal/store"
)

// Maximum number of existing nodes to link per entity (avoid hot-entity explosion).
const maxEntityLinks = 5

var entityPatterns = []*regexp.Regexp{
	// CamelCase identifiers (e.g., MyClass, HttpServer)
	regexp.MustCompile(`\b([A-Z][a-z]+(?:[A-Z][a-z]+)+)\b`),
	// File paths (e.g., ./cmd/root.go, /etc/config.yml)
	regexp.MustCompile(`(?:^|[\s"'(])([.\w/-]+\.\w{1,10})(?:[\s"'),.]|$)`),
	// URLs
	regexp.MustCompile(`https?://[^\s"'<>)]+`),
	// @mentions
	regexp.MustCompile(`@([a-zA-Z_]\w+)`),
	// Chinese book title marks / quotes
	regexp.MustCompile(`[《「]([^》」]+)[》」]`),
}

// ExtractEntities extracts named entities from text using regex patterns.
func ExtractEntities(text string) []string {
	seen := make(map[string]bool)
	var entities []string

	for _, pat := range entityPatterns {
		matches := pat.FindAllStringSubmatch(text, -1)
		for _, m := range matches {
			// Use the last capturing group, or the full match if no groups
			entity := m[len(m)-1]
			if entity != "" && !seen[entity] {
				seen[entity] = true
				entities = append(entities, entity)
			}
		}
	}
	return entities
}

// CreateEntityEdgesForNewEntities creates entity edges only for newly added entities
// (used by the enrich command when --rebuild-edges is specified).
func CreateEntityEdgesForNewEntities(db *store.DB, insight *model.Insight, newEntities []string) int {
	if len(newEntities) == 0 {
		return 0
	}

	now := time.Now().UTC()
	count := 0

	for _, entity := range newEntities {
		ids, err := db.FindInsightsWithEntity(entity, insight.ID, maxEntityLinks)
		if err != nil || len(ids) == 0 {
			continue
		}

		for _, targetID := range ids {
			err = db.InsertEdge(&model.Edge{
				SourceID:  insight.ID,
				TargetID:  targetID,
				EdgeType:  model.EdgeEntity,
				Weight:    1.0,
				Metadata:  map[string]string{"entity": entity},
				CreatedAt: now,
			})
			if err == nil {
				count++
			}
			err = db.InsertEdge(&model.Edge{
				SourceID:  targetID,
				TargetID:  insight.ID,
				EdgeType:  model.EdgeEntity,
				Weight:    1.0,
				Metadata:  map[string]string{"entity": entity},
				CreatedAt: now,
			})
			if err == nil {
				count++
			}
		}
	}
	return count
}

// CreateEntityEdges creates entity co-occurrence edges between the new insight
// and existing insights that share the same entities.
func CreateEntityEdges(db *store.DB, insight *model.Insight) int {
	if len(insight.Entities) == 0 {
		return 0
	}

	now := time.Now().UTC()
	count := 0

	for _, entity := range insight.Entities {
		ids, err := db.FindInsightsWithEntity(entity, insight.ID, maxEntityLinks)
		if err != nil || len(ids) == 0 {
			continue
		}

		for _, targetID := range ids {
			// new → old
			err = db.InsertEdge(&model.Edge{
				SourceID:  insight.ID,
				TargetID:  targetID,
				EdgeType:  model.EdgeEntity,
				Weight:    1.0,
				Metadata:  map[string]string{"entity": entity},
				CreatedAt: now,
			})
			if err == nil {
				count++
			}
			// old → new (reverse)
			err = db.InsertEdge(&model.Edge{
				SourceID:  targetID,
				TargetID:  insight.ID,
				EdgeType:  model.EdgeEntity,
				Weight:    1.0,
				Metadata:  map[string]string{"entity": entity},
				CreatedAt: now,
			})
			if err == nil {
				count++
			}
		}
	}
	return count
}
