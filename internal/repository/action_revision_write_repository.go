package repository

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository/actionutil"
)

// This file holds the transaction-scoped write helpers used by the action
// definition service. They let the service persist the action row, its graph,
// its revision, and its audit events inside one transaction.

// CreateTx inserts a new action row and its allowed roles inside tx.
func (r *ActionRepository) CreateTx(tx database.Tx, action *models.Action) (int, error) {
	var id int
	now := time.Now()
	if err := tx.QueryRow(`
		INSERT INTO actions (
			workspace_id, name, description, is_enabled, trigger_type, trigger_config,
			created_by, actor_user_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`,
		action.WorkspaceID, action.Name, action.Description, action.IsEnabled,
		action.TriggerType, action.TriggerConfig, action.CreatedBy, action.ActorUserID,
		now, now,
	).Scan(&id); err != nil {
		return 0, fmt.Errorf("failed to create action: %w", err)
	}
	if err := replaceAllowedRoleIDs(tx, id, action.AllowedRoleIDs); err != nil {
		return 0, err
	}
	return id, nil
}

// UpdateTx updates an action's metadata and allowed roles inside tx. It also
// locks the row on PostgreSQL so concurrent edits serialize before the next
// revision number is computed.
func (r *ActionRepository) UpdateTx(tx database.Tx, action *models.Action) error {
	if _, err := tx.Exec(`
		UPDATE actions SET
			name = ?, description = ?, is_enabled = ?, trigger_type = ?,
			trigger_config = ?, updated_at = ?
		WHERE id = ?
	`,
		action.Name, action.Description, action.IsEnabled, action.TriggerType,
		action.TriggerConfig, time.Now(), action.ID,
	); err != nil {
		return fmt.Errorf("failed to update action: %w", err)
	}
	if err := replaceAllowedRoleIDs(tx, action.ID, action.AllowedRoleIDs); err != nil {
		return err
	}
	return nil
}

// SaveGraphTx replaces an action's nodes and edges inside tx.
func (r *ActionRepository) SaveGraphTx(tx database.Tx, actionID int, nodes []models.ActionNode, edges []models.ActionEdge) error {
	flowNodes := actionutil.ToFlowNodes(nodes)
	flowEdges := actionutil.ToFlowEdges(edges)
	if err := actionutil.SaveNodesAndEdges(
		tx, actionID, flowNodes, flowEdges,
		actionutil.SQLiteStatements("action_nodes", "action_edges"),
	); err != nil {
		return fmt.Errorf("failed to save nodes and edges: %w", err)
	}
	return nil
}

// SetActorTx updates only the actor override inside tx.
func (r *ActionRepository) SetActorTx(tx database.Tx, actionID int, actorUserID *int) error {
	if _, err := tx.Exec(
		`UPDATE actions SET actor_user_id = ?, updated_at = ? WHERE id = ?`,
		actorUserID, time.Now(), actionID,
	); err != nil {
		return fmt.Errorf("failed to set action actor: %w", err)
	}
	return nil
}

// SetCurrentRevisionTx points the action at the revision that is now live.
func (r *ActionRepository) SetCurrentRevisionTx(tx database.Tx, actionID, revisionID int) error {
	if _, err := tx.Exec(`UPDATE actions SET current_revision_id = ? WHERE id = ?`, revisionID, actionID); err != nil {
		return fmt.Errorf("failed to bind current action revision: %w", err)
	}
	return nil
}

// DeleteTx deletes an action and its graph inside tx. Revisions are not
// deleted: they are the durable record of what ran and outlive the action.
func (r *ActionRepository) DeleteTx(tx database.Tx, actionID int) error {
	result, err := tx.Exec(`DELETE FROM actions WHERE id = ?`, actionID)
	if err != nil {
		return fmt.Errorf("failed to delete action: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// GetNodesByActionIDTx loads an action's nodes inside tx.
func (r *ActionRepository) GetNodesByActionIDTx(tx database.Tx, actionID int) ([]models.ActionNode, error) {
	generic, err := actionutil.ScanNodes(tx, `
		SELECT id, action_id, node_type, node_config, position_x, position_y, created_at, updated_at
		FROM action_nodes
		WHERE action_id = ?
		ORDER BY id
	`, actionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query action nodes: %w", err)
	}
	nodes := make([]models.ActionNode, len(generic))
	for i, n := range generic {
		nodes[i] = models.ActionNode{
			ID: n.ID, ActionID: n.ActionID, NodeType: models.ActionNodeType(n.NodeType),
			NodeConfig: n.NodeConfig, PositionX: n.PositionX, PositionY: n.PositionY,
			CreatedAt: n.CreatedAt, UpdatedAt: n.UpdatedAt,
		}
	}
	return nodes, nil
}

// GetEdgesByActionIDTx loads an action's edges inside tx.
func (r *ActionRepository) GetEdgesByActionIDTx(tx database.Tx, actionID int) ([]models.ActionEdge, error) {
	generic, err := actionutil.ScanEdges(tx, `
		SELECT id, action_id, source_node_id, target_node_id, edge_type, source_handle, target_handle, created_at
		FROM action_edges
		WHERE action_id = ?
		ORDER BY id
	`, actionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query action edges: %w", err)
	}
	edges := make([]models.ActionEdge, len(generic))
	for i, e := range generic {
		edges[i] = models.ActionEdge{
			ID: e.ID, ActionID: e.ActionID, SourceNodeID: e.SourceNodeID,
			TargetNodeID: e.TargetNodeID, EdgeType: e.EdgeType,
			SourceHandle: e.SourceHandle, TargetHandle: e.TargetHandle,
			CreatedAt: e.CreatedAt,
		}
	}
	return edges, nil
}

// GetCurrentRevisionID returns the action's live revision pointer, or nil when
// the action predates revisions (or has none).
func (r *ActionRepository) GetCurrentRevisionID(actionID int) (*int, error) {
	var revisionID sql.NullInt64
	err := r.db.QueryRow(`SELECT current_revision_id FROM actions WHERE id = ?`, actionID).Scan(&revisionID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load current action revision: %w", err)
	}
	if !revisionID.Valid {
		return nil, nil
	}
	value := int(revisionID.Int64)
	return &value, nil
}

// LoadDefinitionSnapshot returns the executable snapshot for a revision. It is
// used by the runtime to execute the immutable definition rather than the
// mutable action row.
func (r *ActionRevisionRepository) LoadDefinitionSnapshot(revisionID int) (*models.ActionDefinitionSnapshot, error) {
	var raw string
	err := r.db.QueryRow(`SELECT snapshot FROM action_definition_revisions WHERE id = ?`, revisionID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load action definition snapshot: %w", err)
	}
	var snapshot models.ActionDefinitionSnapshot
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		return nil, fmt.Errorf("decode action definition snapshot %d: %w", revisionID, err)
	}
	return &snapshot, nil
}
