package store

import "time"

// LogOp records an operation to the oplog.
func (db *DB) LogOp(operation, insightID, detail string) {
	db.conn.Exec(
		`INSERT INTO oplog (operation, insight_id, detail, created_at) VALUES (?, ?, ?, ?)`,
		operation, insightID, detail, time.Now().UTC().Format(time.RFC3339))
}

// OplogEntry represents a single operation log entry.
type OplogEntry struct {
	ID        int    `json:"id"`
	Operation string `json:"operation"`
	InsightID string `json:"insight_id,omitempty"`
	Detail    string `json:"detail,omitempty"`
	CreatedAt string `json:"created_at"`
}

// GetOplog returns the most recent N oplog entries.
func (db *DB) GetOplog(limit int) ([]OplogEntry, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := db.conn.Query(
		`SELECT id, operation, insight_id, detail, created_at FROM oplog ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]OplogEntry, 0)
	for rows.Next() {
		var e OplogEntry
		if err := rows.Scan(&e.ID, &e.Operation, &e.InsightID, &e.Detail, &e.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, nil
}
