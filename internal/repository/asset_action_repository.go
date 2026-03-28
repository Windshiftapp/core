package repository

import (
	"database/sql"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// AssetActionRepository provides data access for asset actions
type AssetActionRepository struct {
	db database.Database
}

// NewAssetActionRepository creates a new asset action repository
func NewAssetActionRepository(db database.Database) *AssetActionRepository {
	return &AssetActionRepository{db: db}
}

// applyAssetActionNulls sets nullable fields on an AssetAction from scanned sql.Null values.
func applyAssetActionNulls(a *models.AssetAction, description, triggerConfig sql.NullString, createdBy sql.NullInt64) {
	if description.Valid {
		a.Description = description.String
	}
	if triggerConfig.Valid {
		a.TriggerConfig = triggerConfig.String
	}
	if createdBy.Valid {
		val := int(createdBy.Int64)
		a.CreatedBy = &val
	}
}

// GetByID retrieves an asset action by ID with its nodes and edges
func (r *AssetActionRepository) GetByID(id int) (*models.AssetAction, error) {
	var action models.AssetAction
	var description, triggerConfig sql.NullString
	var createdBy sql.NullInt64

	err := r.db.QueryRow(`
		SELECT a.id, a.set_id, a.name, a.description, a.is_enabled,
		       a.trigger_type, a.trigger_config, a.created_by, a.created_at, a.updated_at,
		       COALESCE(u.first_name || ' ' || u.last_name, '')
		FROM asset_actions a
		LEFT JOIN users u ON a.created_by = u.id
		WHERE a.id = ?
	`, id).Scan(
		&action.ID, &action.SetID, &action.Name, &description, &action.IsEnabled,
		&action.TriggerType, &triggerConfig, &createdBy, &action.CreatedAt, &action.UpdatedAt,
		&action.CreatorName,
	)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find asset action: %w", err)
	}

	applyAssetActionNulls(&action, description, triggerConfig, createdBy)

	nodes, err := r.GetNodesByActionID(id)
	if err != nil {
		return nil, fmt.Errorf("failed to get asset action nodes: %w", err)
	}
	action.Nodes = nodes

	edges, err := r.GetEdgesByActionID(id)
	if err != nil {
		return nil, fmt.Errorf("failed to get asset action edges: %w", err)
	}
	action.Edges = edges

	return &action, nil
}

// ListBySet lists all actions for an asset set
func (r *AssetActionRepository) ListBySet(setID int) ([]*models.AssetAction, error) {
	rows, err := r.db.Query(`
		SELECT a.id, a.set_id, a.name, a.description, a.is_enabled,
		       a.trigger_type, a.trigger_config, a.created_by, a.created_at, a.updated_at,
		       COALESCE(u.first_name || ' ' || u.last_name, '')
		FROM asset_actions a
		LEFT JOIN users u ON a.created_by = u.id
		WHERE a.set_id = ?
		ORDER BY a.created_at DESC
	`, setID)
	if err != nil {
		return nil, fmt.Errorf("failed to query asset actions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var actions []*models.AssetAction
	for rows.Next() {
		action := &models.AssetAction{}
		var description, triggerConfig sql.NullString
		var createdBy sql.NullInt64

		err := rows.Scan(
			&action.ID, &action.SetID, &action.Name, &description, &action.IsEnabled,
			&action.TriggerType, &triggerConfig, &createdBy, &action.CreatedAt, &action.UpdatedAt,
			&action.CreatorName,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan asset action: %w", err)
		}

		applyAssetActionNulls(action, description, triggerConfig, createdBy)

		actions = append(actions, action)
	}

	return actions, nil
}

// ListEnabledBySet lists all enabled actions for a set with nodes and edges
func (r *AssetActionRepository) ListEnabledBySet(setID int) ([]*models.AssetAction, error) {
	rows, err := r.db.Query(`
		SELECT id, set_id, name, description, is_enabled,
		       trigger_type, trigger_config, created_by, created_at, updated_at
		FROM asset_actions
		WHERE set_id = ? AND is_enabled = true
		ORDER BY created_at DESC
	`, setID)
	if err != nil {
		return nil, fmt.Errorf("failed to query enabled asset actions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var actions []*models.AssetAction
	for rows.Next() {
		action := &models.AssetAction{}
		var description, triggerConfig sql.NullString
		var createdBy sql.NullInt64

		err := rows.Scan(
			&action.ID, &action.SetID, &action.Name, &description, &action.IsEnabled,
			&action.TriggerType, &triggerConfig, &createdBy, &action.CreatedAt, &action.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan asset action: %w", err)
		}

		applyAssetActionNulls(action, description, triggerConfig, createdBy)

		nodes, err := r.GetNodesByActionID(action.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get action nodes: %w", err)
		}
		action.Nodes = nodes

		edges, err := r.GetEdgesByActionID(action.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get action edges: %w", err)
		}
		action.Edges = edges

		actions = append(actions, action)
	}

	return actions, nil
}

// Create creates a new asset action
func (r *AssetActionRepository) Create(action *models.AssetAction) (int, error) {
	var id int
	err := r.db.QueryRow(`
		INSERT INTO asset_actions (set_id, name, description, is_enabled, trigger_type, trigger_config, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`,
		action.SetID, action.Name, action.Description, action.IsEnabled,
		action.TriggerType, action.TriggerConfig, action.CreatedBy,
		time.Now(), time.Now(),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create asset action: %w", err)
	}

	return id, nil
}

// Update updates an asset action
func (r *AssetActionRepository) Update(action *models.AssetAction) error {
	_, err := r.db.Exec(`
		UPDATE asset_actions SET
			name = ?, description = ?, trigger_type = ?, trigger_config = ?, updated_at = ?
		WHERE id = ?
	`,
		action.Name, action.Description, action.TriggerType, action.TriggerConfig,
		time.Now(), action.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update asset action: %w", err)
	}
	return nil
}

// Delete deletes an asset action and its associated nodes and edges (cascade)
func (r *AssetActionRepository) Delete(id int) error {
	result, err := r.db.Exec(`DELETE FROM asset_actions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete asset action: %w", err)
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

// SetEnabled enables or disables an asset action
func (r *AssetActionRepository) SetEnabled(id int, enabled bool) error {
	_, err := r.db.Exec(`UPDATE asset_actions SET is_enabled = ?, updated_at = ? WHERE id = ?`,
		enabled, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to set asset action enabled status: %w", err)
	}
	return nil
}

// --------- Node Operations ---------

// GetNodesByActionID retrieves all nodes for an asset action
func (r *AssetActionRepository) GetNodesByActionID(actionID int) ([]models.AssetActionNode, error) {
	rows, err := r.db.Query(`
		SELECT id, action_id, node_type, node_config, position_x, position_y, created_at, updated_at
		FROM asset_action_nodes
		WHERE action_id = ?
		ORDER BY id
	`, actionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query asset action nodes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var nodes []models.AssetActionNode
	for rows.Next() {
		var node models.AssetActionNode
		err := rows.Scan(
			&node.ID, &node.ActionID, &node.NodeType, &node.NodeConfig,
			&node.PositionX, &node.PositionY, &node.CreatedAt, &node.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan asset action node: %w", err)
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

// GetEdgesByActionID retrieves all edges for an asset action
func (r *AssetActionRepository) GetEdgesByActionID(actionID int) ([]models.AssetActionEdge, error) {
	rows, err := r.db.Query(`
		SELECT id, action_id, source_node_id, target_node_id, edge_type, source_handle, target_handle, created_at
		FROM asset_action_edges
		WHERE action_id = ?
		ORDER BY id
	`, actionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query asset action edges: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var edges []models.AssetActionEdge
	for rows.Next() {
		var edge models.AssetActionEdge
		var sourceHandle, targetHandle sql.NullString
		err := rows.Scan(
			&edge.ID, &edge.ActionID, &edge.SourceNodeID, &edge.TargetNodeID,
			&edge.EdgeType, &sourceHandle, &targetHandle, &edge.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan asset action edge: %w", err)
		}
		if sourceHandle.Valid {
			edge.SourceHandle = sourceHandle.String
		}
		if targetHandle.Valid {
			edge.TargetHandle = targetHandle.String
		}
		edges = append(edges, edge)
	}

	return edges, nil
}

// SaveActionWithNodesAndEdges saves an asset action with its nodes and edges in a transaction
func (r *AssetActionRepository) SaveActionWithNodesAndEdges(action *models.AssetAction, nodes []models.AssetActionNode, edges []models.AssetActionEdge) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Delete existing edges then nodes
	_, err = tx.Exec(`DELETE FROM asset_action_edges WHERE action_id = ?`, action.ID)
	if err != nil {
		return fmt.Errorf("failed to delete existing edges: %w", err)
	}
	_, err = tx.Exec(`DELETE FROM asset_action_nodes WHERE action_id = ?`, action.ID)
	if err != nil {
		return fmt.Errorf("failed to delete existing nodes: %w", err)
	}

	// Update action
	_, err = tx.Exec(`
		UPDATE asset_actions SET
			name = ?, description = ?, trigger_type = ?, trigger_config = ?, updated_at = ?
		WHERE id = ?
	`,
		action.Name, action.Description, action.TriggerType, action.TriggerConfig,
		time.Now(), action.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update asset action: %w", err)
	}

	// Insert nodes and build ID mapping
	nodeIDMap := make(map[int]int)
	for _, node := range nodes {
		var newID int
		err = tx.QueryRow(`
			INSERT INTO asset_action_nodes (action_id, node_type, node_config, position_x, position_y, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id
		`,
			action.ID, node.NodeType, node.NodeConfig, node.PositionX, node.PositionY,
			time.Now(), time.Now(),
		).Scan(&newID)
		if err != nil {
			return fmt.Errorf("failed to insert node: %w", err)
		}
		nodeIDMap[node.ID] = newID
	}

	// Insert edges using mapped node IDs
	for _, edge := range edges {
		sourceID, ok := nodeIDMap[edge.SourceNodeID]
		if !ok {
			return fmt.Errorf("source node ID %d not found in node map", edge.SourceNodeID)
		}
		targetID, ok := nodeIDMap[edge.TargetNodeID]
		if !ok {
			return fmt.Errorf("target node ID %d not found in node map", edge.TargetNodeID)
		}

		_, err := tx.Exec(`
			INSERT INTO asset_action_edges (action_id, source_node_id, target_node_id, edge_type, source_handle, target_handle, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`,
			action.ID, sourceID, targetID, edge.EdgeType,
			edge.SourceHandle, edge.TargetHandle, time.Now(),
		)
		if err != nil {
			return fmt.Errorf("failed to insert edge: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// --------- Execution Log Operations ---------

// CreateExecutionLog creates a new asset action execution log entry
func (r *AssetActionRepository) CreateExecutionLog(log *models.AssetActionExecutionLog) (int, error) {
	var id int
	err := r.db.QueryRow(`
		INSERT INTO asset_action_execution_logs (action_id, asset_id, trigger_event, status, started_at)
		VALUES (?, ?, ?, ?, ?) RETURNING id
	`,
		log.ActionID, log.AssetID, log.TriggerEvent, log.Status, time.Now(),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create asset action execution log: %w", err)
	}

	return id, nil
}

// UpdateExecutionLog updates an asset action execution log entry
func (r *AssetActionRepository) UpdateExecutionLog(log *models.AssetActionExecutionLog) error {
	_, err := r.db.Exec(`
		UPDATE asset_action_execution_logs SET
			status = ?, completed_at = ?, error_message = ?, execution_trace = ?
		WHERE id = ?
	`,
		log.Status, log.CompletedAt, log.ErrorMessage, log.ExecutionTrace, log.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update asset action execution log: %w", err)
	}
	return nil
}

// GetExecutionLogs retrieves execution logs for a specific asset action
func (r *AssetActionRepository) GetExecutionLogs(actionID, limit, offset int) ([]*models.AssetActionExecutionLog, error) {
	rows, err := r.db.Query(`
		SELECT l.id, l.action_id, l.asset_id, l.trigger_event, l.status,
		       l.started_at, l.completed_at, l.error_message, l.execution_trace,
		       COALESCE(a.name, ''), COALESCE(ast.title, '')
		FROM asset_action_execution_logs l
		LEFT JOIN asset_actions a ON l.action_id = a.id
		LEFT JOIN assets ast ON l.asset_id = ast.id
		WHERE l.action_id = ?
		ORDER BY l.started_at DESC
		LIMIT ? OFFSET ?
	`, actionID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query asset action execution logs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return r.scanExecutionLogs(rows)
}

// GetSetExecutionLogs retrieves execution logs for all actions in an asset set
func (r *AssetActionRepository) GetSetExecutionLogs(setID, limit, offset int) ([]*models.AssetActionExecutionLog, error) {
	rows, err := r.db.Query(`
		SELECT l.id, l.action_id, l.asset_id, l.trigger_event, l.status,
		       l.started_at, l.completed_at, l.error_message, l.execution_trace,
		       COALESCE(a.name, ''), COALESCE(ast.title, '')
		FROM asset_action_execution_logs l
		LEFT JOIN asset_actions a ON l.action_id = a.id
		LEFT JOIN assets ast ON l.asset_id = ast.id
		WHERE a.set_id = ?
		ORDER BY l.started_at DESC
		LIMIT ? OFFSET ?
	`, setID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query set execution logs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return r.scanExecutionLogs(rows)
}

func (r *AssetActionRepository) scanExecutionLogs(rows *sql.Rows) ([]*models.AssetActionExecutionLog, error) {
	var logs []*models.AssetActionExecutionLog
	for rows.Next() {
		log := &models.AssetActionExecutionLog{}
		var assetID sql.NullInt64
		var completedAt sql.NullTime
		var errorMessage, executionTrace sql.NullString

		err := rows.Scan(
			&log.ID, &log.ActionID, &assetID, &log.TriggerEvent, &log.Status,
			&log.StartedAt, &completedAt, &errorMessage, &executionTrace,
			&log.ActionName, &log.AssetTitle,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan asset action execution log: %w", err)
		}

		if assetID.Valid {
			val := int(assetID.Int64)
			log.AssetID = &val
		}
		if completedAt.Valid {
			log.CompletedAt = &completedAt.Time
		}
		if errorMessage.Valid {
			log.ErrorMessage = errorMessage.String
		}
		if executionTrace.Valid {
			log.ExecutionTrace = executionTrace.String
		}

		logs = append(logs, log)
	}

	return logs, nil
}
