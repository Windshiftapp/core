// Package actionutil provides shared helpers for scanning and saving
// action flow nodes and edges. These helpers deduplicate the identical
// transaction logic that exists across action, asset-action, and
// logbook-action repositories.
package actionutil

import (
	"database/sql"
	"fmt"
	"time"

	"windshift/internal/database"
)

// FlowNode is a database-level representation of an action flow node.
// Each domain (action, asset-action, logbook-action) has its own Go type,
// but the database columns are identical, so we scan/insert through this
// common struct and let callers convert to their domain type.
type FlowNode struct {
	ID         int
	ActionID   int
	NodeType   string
	NodeConfig string
	PositionX  float64
	PositionY  float64
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// FlowEdge is a database-level representation of an action flow edge.
type FlowEdge struct {
	ID           int
	ActionID     int
	SourceNodeID int
	TargetNodeID int
	EdgeType     string
	SourceHandle string
	TargetHandle string
	CreatedAt    time.Time
}

// Querier is the common subset of database.Database and database.Tx needed
// for read-only helpers.
type Querier interface {
	Query(query string, args ...interface{}) (*sql.Rows, error)
}

// HydrateNodesAndEdges fetches both nodes and edges for a given action ID using
// the provided SQL queries. This eliminates the duplicated fetch-nodes, fetch-edges,
// check-error pattern that appears in every action repository's GetByID and
// ListEnabled methods.
//
// nodeQuery and edgeQuery must be full SELECT statements with a single placeholder
// for the actionID (? for SQLite, $1 for PostgreSQL).
func HydrateNodesAndEdges(q Querier, nodeQuery, edgeQuery string, actionID int) ([]FlowNode, []FlowEdge, error) {
	nodes, err := ScanNodes(q, nodeQuery, actionID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get action nodes: %w", err)
	}
	edges, err := ScanEdges(q, edgeQuery, actionID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get action edges: %w", err)
	}
	return nodes, edges, nil
}

// ScanNodes queries and scans action-flow nodes from any table that follows
// the standard column layout (id, action_id, node_type, node_config,
// position_x, position_y, created_at, updated_at).
//
// query must be a full SELECT statement with a single placeholder for actionID.
func ScanNodes(q Querier, query string, actionID int) ([]FlowNode, error) {
	rows, err := q.Query(query, actionID)
	if err != nil {
		return nil, fmt.Errorf("query nodes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var nodes []FlowNode
	for rows.Next() {
		var n FlowNode
		if err := rows.Scan(
			&n.ID, &n.ActionID, &n.NodeType, &n.NodeConfig,
			&n.PositionX, &n.PositionY, &n.CreatedAt, &n.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan node: %w", err)
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

// ScanEdges queries and scans action-flow edges from any table that follows
// the standard column layout (id, action_id, source_node_id, target_node_id,
// edge_type, source_handle, target_handle, created_at).
//
// query must be a full SELECT statement with a single placeholder for actionID.
func ScanEdges(q Querier, query string, actionID int) ([]FlowEdge, error) {
	rows, err := q.Query(query, actionID)
	if err != nil {
		return nil, fmt.Errorf("query edges: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var edges []FlowEdge
	for rows.Next() {
		var e FlowEdge
		var sourceHandle, targetHandle sql.NullString
		if err := rows.Scan(
			&e.ID, &e.ActionID, &e.SourceNodeID, &e.TargetNodeID,
			&e.EdgeType, &sourceHandle, &targetHandle, &e.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan edge: %w", err)
		}
		if sourceHandle.Valid {
			e.SourceHandle = sourceHandle.String
		}
		if targetHandle.Valid {
			e.TargetHandle = targetHandle.String
		}
		edges = append(edges, e)
	}
	return edges, nil
}

// SaveNodesAndEdges deletes existing nodes/edges for the given actionID,
// then inserts the provided nodes and edges inside a transaction. It handles
// the ID-remapping required when edges reference old (client-side) node IDs.
//
// The caller is responsible for beginning the transaction and committing it
// afterward; this function only performs the delete+insert steps.
//
// Placeholder style: the caller passes the SQL statements with the correct
// placeholders for the target database (? for SQLite, $N for PostgreSQL).
// The statements struct bundles them together.
func SaveNodesAndEdges(tx database.Tx, actionID int, nodes []FlowNode, edges []FlowEdge, stmts Statements) error {
	// Delete existing edges first (they reference nodes via FK).
	_, err := tx.Exec(stmts.DeleteEdges, actionID)
	if err != nil {
		return fmt.Errorf("delete existing edges: %w", err)
	}

	// Delete existing nodes.
	_, err = tx.Exec(stmts.DeleteNodes, actionID)
	if err != nil {
		return fmt.Errorf("delete existing nodes: %w", err)
	}

	// Insert nodes and build ID mapping (old ID -> new ID).
	now := time.Now()
	nodeIDMap := make(map[int]int)
	for _, node := range nodes {
		var newID int
		err = tx.QueryRow(stmts.InsertNode,
			actionID, node.NodeType, node.NodeConfig, node.PositionX, node.PositionY,
			now, now,
		).Scan(&newID)
		if err != nil {
			return fmt.Errorf("insert node: %w", err)
		}
		nodeIDMap[node.ID] = newID
	}

	// Insert edges using remapped node IDs.
	for _, edge := range edges {
		sourceID, ok := nodeIDMap[edge.SourceNodeID]
		if !ok {
			return fmt.Errorf("source node ID %d not found in node map", edge.SourceNodeID)
		}
		targetID, ok := nodeIDMap[edge.TargetNodeID]
		if !ok {
			return fmt.Errorf("target node ID %d not found in node map", edge.TargetNodeID)
		}

		_, err := tx.Exec(stmts.InsertEdge,
			actionID, sourceID, targetID, edge.EdgeType,
			edge.SourceHandle, edge.TargetHandle, now,
		)
		if err != nil {
			return fmt.Errorf("insert edge: %w", err)
		}
	}

	return nil
}

// Statements holds the parameterised SQL strings needed by SaveNodesAndEdges.
// Each repository provides its own instance with the correct table names and
// placeholder style.
type Statements struct {
	DeleteEdges string // e.g. "DELETE FROM action_edges WHERE action_id = ?"
	DeleteNodes string // e.g. "DELETE FROM action_nodes WHERE action_id = ?"
	InsertNode  string // INSERT ... (action_id, node_type, ...) VALUES (?, ...) RETURNING id
	InsertEdge  string // INSERT ... (action_id, source_node_id, ...) VALUES (?, ...)
}

// SQLiteStatements returns Statements for a SQLite-backed table pair.
// nodeTable / edgeTable are the bare table names (e.g. "action_nodes", "asset_action_edges").
func SQLiteStatements(nodeTable, edgeTable string) Statements {
	return Statements{
		DeleteEdges: `DELETE FROM ` + edgeTable + ` WHERE action_id = ?`,
		DeleteNodes: `DELETE FROM ` + nodeTable + ` WHERE action_id = ?`,
		InsertNode: `INSERT INTO ` + nodeTable + ` (action_id, node_type, node_config, position_x, position_y, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		InsertEdge: `INSERT INTO ` + edgeTable + ` (action_id, source_node_id, target_node_id, edge_type, source_handle, target_handle, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
	}
}

// PostgresStatements returns Statements for a PostgreSQL-backed table pair.
func PostgresStatements(nodeTable, edgeTable string) Statements {
	return Statements{
		DeleteEdges: `DELETE FROM ` + edgeTable + ` WHERE action_id = $1`,
		DeleteNodes: `DELETE FROM ` + nodeTable + ` WHERE action_id = $1`,
		InsertNode: `INSERT INTO ` + nodeTable + ` (action_id, node_type, node_config, position_x, position_y, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		InsertEdge: `INSERT INTO ` + edgeTable + ` (action_id, source_node_id, target_node_id, edge_type, source_handle, target_handle, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
	}
}
