package store

import (
	"time"

	"github.com/Grivn/mnemon/internal/model"
)

// InsertEdge inserts or replaces an edge.
func (db *DB) InsertEdge(e *model.Edge) error {
	_, err := db.execer().Exec(
		`INSERT OR REPLACE INTO edges (source_id, target_id, edge_type, weight, metadata, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		e.SourceID, e.TargetID, string(e.EdgeType), e.Weight,
		e.MetadataJSON(), e.CreatedAt.Format(time.RFC3339),
	)
	return err
}

// GetEdgesByNode returns all edges where the given node is source or target.
func (db *DB) GetEdgesByNode(nodeID string) ([]*model.Edge, error) {
	rows, err := db.execer().Query(
		`SELECT source_id, target_id, edge_type, weight, metadata, created_at
		 FROM edges WHERE source_id = ? OR target_id = ?`, nodeID, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEdges(rows)
}

// GetEdgesByNodeAndType returns edges for a node filtered by edge type.
func (db *DB) GetEdgesByNodeAndType(nodeID string, edgeType model.EdgeType) ([]*model.Edge, error) {
	rows, err := db.execer().Query(
		`SELECT source_id, target_id, edge_type, weight, metadata, created_at
		 FROM edges WHERE (source_id = ? OR target_id = ?) AND edge_type = ?`,
		nodeID, nodeID, string(edgeType))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEdges(rows)
}

// GetEdgesBySourceAndType returns edges where the given node is source, filtered by type.
func (db *DB) GetEdgesBySourceAndType(sourceID string, edgeType model.EdgeType) ([]*model.Edge, error) {
	rows, err := db.execer().Query(
		`SELECT source_id, target_id, edge_type, weight, metadata, created_at
		 FROM edges WHERE source_id = ? AND edge_type = ?`, sourceID, string(edgeType))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEdges(rows)
}

// FindInsightsWithEntity returns insight IDs that have the given entity in their entities JSON array.
func (db *DB) FindInsightsWithEntity(entity string, excludeID string, limit int) ([]string, error) {
	rows, err := db.execer().Query(
		`SELECT id FROM insights
		 WHERE deleted_at IS NULL AND id != ? AND entities LIKE ?
		 ORDER BY created_at DESC LIMIT ?`,
		excludeID, `%"`+entity+`"%`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// GetAllEdges returns all edges in the graph.
func (db *DB) GetAllEdges() ([]*model.Edge, error) {
	rows, err := db.execer().Query(
		`SELECT source_id, target_id, edge_type, weight, metadata, created_at FROM edges`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEdges(rows)
}

// DeleteEdgesByNode removes all edges referencing a node.
func (db *DB) DeleteEdgesByNode(nodeID string) error {
	_, err := db.execer().Exec(
		`DELETE FROM edges WHERE source_id = ? OR target_id = ?`, nodeID, nodeID)
	return err
}

func scanEdges(rows interface{ Next() bool; Scan(...interface{}) error }) ([]*model.Edge, error) {
	var results []*model.Edge
	for rows.Next() {
		var e model.Edge
		var edgeType, metadata, createdAt string
		err := rows.Scan(&e.SourceID, &e.TargetID, &edgeType, &e.Weight, &metadata, &createdAt)
		if err != nil {
			return nil, err
		}
		e.EdgeType = model.EdgeType(edgeType)
		e.ParseMetadata(metadata)
		e.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		results = append(results, &e)
	}
	return results, nil
}
