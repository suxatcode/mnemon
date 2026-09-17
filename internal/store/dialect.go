package store

import (
	"fmt"
	"strconv"
	"strings"
)

// Dialect is the SQL dialect used by a store connection.
type Dialect int

const (
	DialectSQLite Dialect = iota
	DialectPostgres
)

func (d Dialect) String() string {
	if d == DialectPostgres {
		return "postgres"
	}
	return "sqlite"
}

func rebind(d Dialect, query string) string {
	if d != DialectPostgres {
		return query
	}
	var b strings.Builder
	n := 0
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			continue
		}
		b.WriteByte(query[i])
	}
	return b.String()
}

func (d Dialect) likeOp() string {
	if d == DialectPostgres {
		return "ILIKE"
	}
	return "LIKE"
}

func (d Dialect) jsonEach(column, alias string) string {
	if d == DialectPostgres {
		return fmt.Sprintf("LATERAL jsonb_array_elements_text(%s::jsonb) AS %s(value)", column, alias)
	}
	return fmt.Sprintf("json_each(%s) %s", column, alias)
}

func (d Dialect) insertOrderDesc() string {
	if d == DialectPostgres {
		return "id DESC"
	}
	return "rowid DESC"
}

func (d Dialect) insertEdgeSQL() string {
	if d == DialectPostgres {
		return `INSERT INTO edges (source_id, target_id, edge_type, weight, metadata, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT (source_id, target_id, edge_type) DO UPDATE SET
		   weight = EXCLUDED.weight,
		   metadata = EXCLUDED.metadata,
		   created_at = EXCLUDED.created_at`
	}
	return `INSERT OR REPLACE INTO edges (source_id, target_id, edge_type, weight, metadata, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`
}

func (d Dialect) blobType() string {
	if d == DialectPostgres {
		return "BYTEA"
	}
	return "BLOB"
}
