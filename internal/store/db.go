package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DB wraps the SQLite database connection.
type DB struct {
	conn *sql.DB
	path string
}

// DefaultDataDir returns ~/.mnemon.
func DefaultDataDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".mnemon")
}

// Open opens (or creates) the SQLite database at the given directory.
func Open(dataDir string) (*DB, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	dbPath := filepath.Join(dataDir, "mnemon.db")
	conn, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	conn.SetMaxOpenConns(1) // SQLite single-writer

	db := &DB{conn: conn, path: dbPath}
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.conn.Close()
}

// Path returns the database file path.
func (db *DB) Path() string {
	return db.path
}

// Conn returns the underlying sql.DB for advanced queries.
func (db *DB) Conn() *sql.DB {
	return db.conn
}

func (db *DB) migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS insights (
    id          TEXT PRIMARY KEY,
    content     TEXT NOT NULL,
    category    TEXT DEFAULT 'general',
    importance  INTEGER DEFAULT 3,
    tags        TEXT DEFAULT '[]',
    entities    TEXT DEFAULT '[]',
    source      TEXT DEFAULT 'user',
    access_count INTEGER DEFAULT 0,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    deleted_at  TEXT
);

CREATE TABLE IF NOT EXISTS edges (
    source_id   TEXT NOT NULL,
    target_id   TEXT NOT NULL,
    edge_type   TEXT NOT NULL CHECK(edge_type IN ('temporal','semantic','causal','entity','narrative')),
    weight      REAL DEFAULT 1.0,
    metadata    TEXT DEFAULT '{}',
    created_at  TEXT NOT NULL,
    PRIMARY KEY (source_id, target_id, edge_type),
    FOREIGN KEY (source_id) REFERENCES insights(id) ON DELETE CASCADE,
    FOREIGN KEY (target_id) REFERENCES insights(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_insights_category ON insights(category);
CREATE INDEX IF NOT EXISTS idx_insights_importance ON insights(importance);
CREATE INDEX IF NOT EXISTS idx_insights_created ON insights(created_at);
CREATE INDEX IF NOT EXISTS idx_insights_deleted ON insights(deleted_at);
CREATE INDEX IF NOT EXISTS idx_edges_source ON edges(source_id);
CREATE INDEX IF NOT EXISTS idx_edges_target ON edges(target_id);
CREATE INDEX IF NOT EXISTS idx_edges_type ON edges(edge_type);

CREATE TABLE IF NOT EXISTS oplog (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    operation   TEXT NOT NULL,
    insight_id  TEXT,
    detail      TEXT DEFAULT '',
    created_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_oplog_created ON oplog(created_at);
`
	_, err := db.conn.Exec(schema)
	if err != nil {
		return err
	}

	// Phase 2 migration: add last_accessed_at column (safe for existing DBs)
	db.conn.Exec(`ALTER TABLE insights ADD COLUMN last_accessed_at TEXT`)

	// Phase 3 migration: add embedding column (safe for existing DBs)
	db.conn.Exec(`ALTER TABLE insights ADD COLUMN embedding BLOB`)

	// Phase 3A migration: add sequence_index for context neighbor edges
	db.conn.Exec(`ALTER TABLE insights ADD COLUMN sequence_index INTEGER`)
	// Backfill: assign sequential index by created_at order for existing insights
	db.conn.Exec(`UPDATE insights SET sequence_index = (
		SELECT COUNT(*) FROM insights i2
		WHERE i2.created_at <= insights.created_at AND i2.id != insights.id
	) WHERE sequence_index IS NULL`)

	// Phase 3D migration: add narrative to edge_type CHECK constraint
	// Test if narrative edge type is already allowed
	_, testErr := db.conn.Exec(`INSERT INTO edges VALUES ('__test','__test','narrative',0,'{}',datetime('now'))`)
	if testErr != nil {
		// CHECK constraint doesn't allow 'narrative', recreate table
		db.conn.Exec(`ALTER TABLE edges RENAME TO edges_old`)
		db.conn.Exec(`CREATE TABLE edges (
			source_id   TEXT NOT NULL,
			target_id   TEXT NOT NULL,
			edge_type   TEXT NOT NULL CHECK(edge_type IN ('temporal','semantic','causal','entity','narrative')),
			weight      REAL DEFAULT 1.0,
			metadata    TEXT DEFAULT '{}',
			created_at  TEXT NOT NULL,
			PRIMARY KEY (source_id, target_id, edge_type),
			FOREIGN KEY (source_id) REFERENCES insights(id) ON DELETE CASCADE,
			FOREIGN KEY (target_id) REFERENCES insights(id) ON DELETE CASCADE
		)`)
		db.conn.Exec(`INSERT INTO edges SELECT * FROM edges_old`)
		db.conn.Exec(`DROP TABLE edges_old`)
		// Recreate indexes after table rebuild
		db.conn.Exec(`CREATE INDEX IF NOT EXISTS idx_edges_source ON edges(source_id)`)
		db.conn.Exec(`CREATE INDEX IF NOT EXISTS idx_edges_target ON edges(target_id)`)
		db.conn.Exec(`CREATE INDEX IF NOT EXISTS idx_edges_type ON edges(edge_type)`)
	} else {
		// Clean up test row
		db.conn.Exec(`DELETE FROM edges WHERE source_id = '__test'`)
	}

	return nil
}
