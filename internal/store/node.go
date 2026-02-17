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

// IncrementAccessCount bumps the access count for an insight.
func (db *DB) IncrementAccessCount(id string) error {
	_, err := db.conn.Exec(
		`UPDATE insights SET access_count = access_count + 1 WHERE id = ?`, id)
	return err
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
