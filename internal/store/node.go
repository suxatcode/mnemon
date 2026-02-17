package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Grivn/mnemon/internal/model"
)

// InsertInsight inserts a new insight into the database.
func (db *DB) InsertInsight(i *model.Insight) error {
	_, err := db.conn.Exec(
		`INSERT INTO insights (id, content, category, importance, tags, entities, source, access_count, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		i.ID, i.Content, string(i.Category), i.Importance,
		i.TagsJSON(), i.EntitiesJSON(), i.Source, i.AccessCount,
		i.CreatedAt.Format(time.RFC3339), i.UpdatedAt.Format(time.RFC3339),
	)
	return err
}

// GetInsightByID returns a single insight by ID (excludes soft-deleted).
func (db *DB) GetInsightByID(id string) (*model.Insight, error) {
	row := db.conn.QueryRow(
		`SELECT id, content, category, importance, tags, entities, source, access_count, created_at, updated_at, deleted_at
		 FROM insights WHERE id = ? AND deleted_at IS NULL`, id)
	return scanInsight(row)
}

// GetInsightByIDIncludeDeleted returns a single insight by ID, including soft-deleted.
func (db *DB) GetInsightByIDIncludeDeleted(id string) (*model.Insight, error) {
	row := db.conn.QueryRow(
		`SELECT id, content, category, importance, tags, entities, source, access_count, created_at, updated_at, deleted_at
		 FROM insights WHERE id = ?`, id)
	return scanInsight(row)
}

// QueryFilter holds optional filters for querying insights.
type QueryFilter struct {
	Keyword    string
	Category   string
	MinImportance int
	Source     string
	Limit      int
}

// QueryInsights returns insights matching the filter, ordered by importance DESC, created_at DESC.
func (db *DB) QueryInsights(f QueryFilter) ([]*model.Insight, error) {
	var conditions []string
	var args []interface{}

	conditions = append(conditions, "deleted_at IS NULL")

	if f.Keyword != "" {
		conditions = append(conditions, "content LIKE ?")
		args = append(args, "%"+f.Keyword+"%")
	}
	if f.Category != "" {
		conditions = append(conditions, "category = ?")
		args = append(args, f.Category)
	}
	if f.MinImportance > 0 {
		conditions = append(conditions, "importance >= ?")
		args = append(args, f.MinImportance)
	}
	if f.Source != "" {
		conditions = append(conditions, "source = ?")
		args = append(args, f.Source)
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 20
	}

	query := fmt.Sprintf(
		`SELECT id, content, category, importance, tags, entities, source, access_count, created_at, updated_at, deleted_at
		 FROM insights WHERE %s ORDER BY importance DESC, created_at DESC LIMIT ?`,
		strings.Join(conditions, " AND "))
	args = append(args, limit)

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanInsights(rows)
}

// SoftDeleteInsight sets deleted_at on an insight.
func (db *DB) SoftDeleteInsight(id string) error {
	res, err := db.conn.Exec(
		`UPDATE insights SET deleted_at = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL`,
		time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("insight %s not found or already deleted", id)
	}
	return nil
}

// UpdateEntities updates the entities field for an insight.
func (db *DB) UpdateEntities(id string, entities []string) error {
	i := &model.Insight{Entities: entities}
	_, err := db.conn.Exec(
		`UPDATE insights SET entities = ?, updated_at = ? WHERE id = ?`,
		i.EntitiesJSON(), time.Now().UTC().Format(time.RFC3339), id)
	return err
}

// IncrementAccessCount bumps the access count and refreshes last_accessed_at.
func (db *DB) IncrementAccessCount(id string) error {
	_, err := db.conn.Exec(
		`UPDATE insights SET access_count = access_count + 1, last_accessed_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339), id)
	return err
}

// RetentionCandidate holds an insight and its retention score breakdown.
type RetentionCandidate struct {
	Insight         *model.Insight `json:"insight"`
	RetentionScore  float64        `json:"retention_score"`
	DaysSinceAccess float64        `json:"days_since_access"`
	EdgeCount       int            `json:"edge_count"`
	Components      struct {
		Importance float64 `json:"importance"`
		Access     float64 `json:"access"`
		Recency    float64 `json:"recency"`
		Edge       float64 `json:"edge"`
	} `json:"components"`
}

// GetRetentionCandidates returns insights with retention scores below the threshold.
// retention_score = 0.35*(importance/5) + 0.20*min(access_count/10,1) + 0.30*max(0,1-days/90) + 0.15*min(edges/5,1)
func (db *DB) GetRetentionCandidates(threshold float64, limit int) ([]RetentionCandidate, int, error) {
	all, err := db.GetAllActiveInsights()
	if err != nil {
		return nil, 0, err
	}

	now := time.Now().UTC()
	var candidates []RetentionCandidate

	for _, ins := range all {
		// Compute days since last access (fallback to created_at)
		lastAccess := ins.CreatedAt
		var lastAccessedAt sql.NullString
		db.conn.QueryRow(`SELECT last_accessed_at FROM insights WHERE id = ?`, ins.ID).Scan(&lastAccessedAt)
		if lastAccessedAt.Valid && lastAccessedAt.String != "" {
			if t, err := time.Parse(time.RFC3339, lastAccessedAt.String); err == nil {
				lastAccess = t
			}
		}
		daysSince := now.Sub(lastAccess).Hours() / 24.0

		// Count edges
		var edgeCount int
		db.conn.QueryRow(`SELECT COUNT(*) FROM edges WHERE source_id = ? OR target_id = ?`, ins.ID, ins.ID).Scan(&edgeCount)

		// Compute components
		impComp := float64(ins.Importance) / 5.0
		accessComp := float64(ins.AccessCount) / 10.0
		if accessComp > 1.0 {
			accessComp = 1.0
		}
		recencyComp := 1.0 - daysSince/90.0
		if recencyComp < 0 {
			recencyComp = 0
		}
		edgeComp := float64(edgeCount) / 5.0
		if edgeComp > 1.0 {
			edgeComp = 1.0
		}

		score := 0.35*impComp + 0.20*accessComp + 0.30*recencyComp + 0.15*edgeComp

		if score < threshold {
			c := RetentionCandidate{
				Insight:         ins,
				RetentionScore:  score,
				DaysSinceAccess: daysSince,
				EdgeCount:       edgeCount,
			}
			c.Components.Importance = impComp
			c.Components.Access = accessComp
			c.Components.Recency = recencyComp
			c.Components.Edge = edgeComp
			candidates = append(candidates, c)
		}
	}

	// Sort by retention score ascending (weakest first)
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].RetentionScore < candidates[i].RetentionScore {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	total := len(all)
	if limit > 0 && len(candidates) > limit {
		candidates = candidates[:limit]
	}
	return candidates, total, nil
}

// BoostRetention boosts an insight's retention: access_count +3, refreshes last_accessed_at.
func (db *DB) BoostRetention(id string) error {
	res, err := db.conn.Exec(
		`UPDATE insights SET access_count = access_count + 3, last_accessed_at = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL`,
		time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("insight %s not found or already deleted", id)
	}
	return nil
}

// GetRecentInsightsInWindow returns non-deleted insights created within the given
// time window (hours), excluding the given ID. Ordered by created_at DESC.
func (db *DB) GetRecentInsightsInWindow(excludeID string, windowHours float64, limit int) ([]*model.Insight, error) {
	cutoff := time.Now().UTC().Add(-time.Duration(windowHours * float64(time.Hour)))
	rows, err := db.conn.Query(
		`SELECT id, content, category, importance, tags, entities, source, access_count, created_at, updated_at, deleted_at
		 FROM insights WHERE id != ? AND deleted_at IS NULL AND created_at >= ?
		 ORDER BY created_at DESC LIMIT ?`,
		excludeID, cutoff.Format(time.RFC3339), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanInsights(rows)
}

// GetLatestInsightBySource returns the most recent non-deleted insight for a given source, excluding the given ID.
func (db *DB) GetLatestInsightBySource(source string, excludeID string) (*model.Insight, error) {
	row := db.conn.QueryRow(
		`SELECT id, content, category, importance, tags, entities, source, access_count, created_at, updated_at, deleted_at
		 FROM insights WHERE source = ? AND id != ? AND deleted_at IS NULL
		 ORDER BY created_at DESC, rowid DESC LIMIT 1`, source, excludeID)
	return scanInsight(row)
}

// GetRecentInsightsBySource returns the N most recent non-deleted insights for a source, excluding the given ID.
func (db *DB) GetRecentInsightsBySource(source string, excludeID string, limit int) ([]*model.Insight, error) {
	rows, err := db.conn.Query(
		`SELECT id, content, category, importance, tags, entities, source, access_count, created_at, updated_at, deleted_at
		 FROM insights WHERE source = ? AND id != ? AND deleted_at IS NULL
		 ORDER BY created_at DESC LIMIT ?`, source, excludeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanInsights(rows)
}

// GetAllActiveInsights returns all non-deleted insights.
func (db *DB) GetAllActiveInsights() ([]*model.Insight, error) {
	rows, err := db.conn.Query(
		`SELECT id, content, category, importance, tags, entities, source, access_count, created_at, updated_at, deleted_at
		 FROM insights WHERE deleted_at IS NULL ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanInsights(rows)
}

// InsightStats holds aggregate statistics.
type InsightStats struct {
	Total         int            `json:"total"`
	ByCategory    map[string]int `json:"by_category"`
	EdgeCount     int            `json:"edge_count"`
	DeletedCount  int            `json:"deleted_count"`
}

// GetStats returns aggregate statistics.
func (db *DB) GetStats() (*InsightStats, error) {
	stats := &InsightStats{ByCategory: make(map[string]int)}

	// Total active
	db.conn.QueryRow(`SELECT COUNT(*) FROM insights WHERE deleted_at IS NULL`).Scan(&stats.Total)

	// Deleted
	db.conn.QueryRow(`SELECT COUNT(*) FROM insights WHERE deleted_at IS NOT NULL`).Scan(&stats.DeletedCount)

	// By category
	rows, err := db.conn.Query(`SELECT category, COUNT(*) FROM insights WHERE deleted_at IS NULL GROUP BY category`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var cat string
		var count int
		if err := rows.Scan(&cat, &count); err != nil {
			return nil, err
		}
		stats.ByCategory[cat] = count
	}

	// Edge count
	db.conn.QueryRow(`SELECT COUNT(*) FROM edges`).Scan(&stats.EdgeCount)

	return stats, nil
}

// UpdateEmbedding stores an embedding vector for an insight.
func (db *DB) UpdateEmbedding(id string, blob []byte) error {
	_, err := db.conn.Exec(
		`UPDATE insights SET embedding = ?, updated_at = ? WHERE id = ?`,
		blob, time.Now().UTC().Format(time.RFC3339), id)
	return err
}

// GetEmbedding returns the raw embedding blob for an insight.
func (db *DB) GetEmbedding(id string) ([]byte, error) {
	var blob []byte
	err := db.conn.QueryRow(`SELECT embedding FROM insights WHERE id = ? AND deleted_at IS NULL`, id).Scan(&blob)
	if err != nil {
		return nil, err
	}
	return blob, nil
}

// EmbeddedInsight pairs an insight ID, content, and its embedding blob.
type EmbeddedInsight struct {
	ID        string
	Content   string
	Embedding []byte
}

// GetAllEmbeddings returns all active insights that have embeddings.
func (db *DB) GetAllEmbeddings() ([]EmbeddedInsight, error) {
	rows, err := db.conn.Query(
		`SELECT id, content, embedding FROM insights WHERE deleted_at IS NULL AND embedding IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []EmbeddedInsight
	for rows.Next() {
		var e EmbeddedInsight
		if err := rows.Scan(&e.ID, &e.Content, &e.Embedding); err != nil {
			return nil, err
		}
		if len(e.Embedding) > 0 {
			results = append(results, e)
		}
	}
	return results, nil
}

// EmbeddingStats returns total insights and how many have embeddings.
func (db *DB) EmbeddingStats() (total int, embedded int, err error) {
	db.conn.QueryRow(`SELECT COUNT(*) FROM insights WHERE deleted_at IS NULL`).Scan(&total)
	db.conn.QueryRow(`SELECT COUNT(*) FROM insights WHERE deleted_at IS NULL AND embedding IS NOT NULL`).Scan(&embedded)
	return total, embedded, nil
}

// GetInsightsWithoutEmbedding returns active insights that lack embeddings.
func (db *DB) GetInsightsWithoutEmbedding(limit int) ([]*model.Insight, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := db.conn.Query(
		`SELECT id, content, category, importance, tags, entities, source, access_count, created_at, updated_at, deleted_at
		 FROM insights WHERE deleted_at IS NULL AND embedding IS NULL
		 ORDER BY importance DESC, created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanInsights(rows)
}

// GetNextSequenceIndex returns the next sequence index for a new insight.
func (db *DB) GetNextSequenceIndex() (int, error) {
	var maxIdx sql.NullInt64
	err := db.conn.QueryRow(`SELECT MAX(sequence_index) FROM insights WHERE deleted_at IS NULL`).Scan(&maxIdx)
	if err != nil {
		return 0, err
	}
	if !maxIdx.Valid {
		return 0, nil
	}
	return int(maxIdx.Int64) + 1, nil
}

// GetInsightsBySequenceRange returns active insights within [seqIdx-k, seqIdx+k], excluding the given ID.
func (db *DB) GetInsightsBySequenceRange(seqIdx, k int, excludeID string) ([]*model.Insight, error) {
	lo := seqIdx - k
	hi := seqIdx + k
	rows, err := db.conn.Query(
		`SELECT id, content, category, importance, tags, entities, source, access_count, created_at, updated_at, deleted_at
		 FROM insights WHERE deleted_at IS NULL AND id != ? AND sequence_index IS NOT NULL
		 AND sequence_index >= ? AND sequence_index <= ?
		 ORDER BY sequence_index ASC`, excludeID, lo, hi)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanInsights(rows)
}

// UpdateSequenceIndex sets the sequence_index for an insight.
func (db *DB) UpdateSequenceIndex(id string, idx int) error {
	_, err := db.conn.Exec(`UPDATE insights SET sequence_index = ? WHERE id = ?`, idx, id)
	return err
}

// GetSequenceIndex returns the sequence_index for an insight.
func (db *DB) GetSequenceIndex(id string) (int, error) {
	var idx sql.NullInt64
	err := db.conn.QueryRow(`SELECT sequence_index FROM insights WHERE id = ?`, id).Scan(&idx)
	if err != nil {
		return 0, err
	}
	if !idx.Valid {
		return -1, nil
	}
	return int(idx.Int64), nil
}

// MergeEntities merges new entities into existing ones (deduplicates).
func (db *DB) MergeEntities(id string, newEntities []string) ([]string, error) {
	insight, err := db.GetInsightByID(id)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var merged []string
	for _, e := range insight.Entities {
		if !seen[e] {
			seen[e] = true
			merged = append(merged, e)
		}
	}
	for _, e := range newEntities {
		if !seen[e] {
			seen[e] = true
			merged = append(merged, e)
		}
	}

	if err := db.UpdateEntities(id, merged); err != nil {
		return nil, err
	}
	return merged, nil
}

func scanInsight(row *sql.Row) (*model.Insight, error) {
	var i model.Insight
	var cat, tags, entities, source, createdAt, updatedAt string
	var deletedAt sql.NullString

	err := row.Scan(&i.ID, &i.Content, &cat, &i.Importance, &tags, &entities,
		&source, &i.AccessCount, &createdAt, &updatedAt, &deletedAt)
	if err != nil {
		return nil, err
	}

	i.Category = model.Category(cat)
	i.Source = source
	i.ParseTags(tags)
	i.ParseEntities(entities)
	i.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	i.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	if deletedAt.Valid {
		t, _ := time.Parse(time.RFC3339, deletedAt.String)
		i.DeletedAt = &t
	}
	return &i, nil
}

func scanInsights(rows *sql.Rows) ([]*model.Insight, error) {
	results := make([]*model.Insight, 0)
	for rows.Next() {
		var i model.Insight
		var cat, tags, entities, source, createdAt, updatedAt string
		var deletedAt sql.NullString

		err := rows.Scan(&i.ID, &i.Content, &cat, &i.Importance, &tags, &entities,
			&source, &i.AccessCount, &createdAt, &updatedAt, &deletedAt)
		if err != nil {
			return nil, err
		}

		i.Category = model.Category(cat)
		i.Source = source
		i.ParseTags(tags)
		i.ParseEntities(entities)
		i.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		i.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		if deletedAt.Valid {
			t, _ := time.Parse(time.RFC3339, deletedAt.String)
			i.DeletedAt = &t
		}
		results = append(results, &i)
	}
	return results, nil
}
