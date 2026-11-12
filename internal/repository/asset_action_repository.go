package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository/actionutil"
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
	ApplyActionNullFieldsToPtr(&a.Description, &a.TriggerConfig, &a.CreatedBy, description, triggerConfig, createdBy)
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

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find asset action: %w", err)
	}

	applyAssetActionNulls(&action, description, triggerConfig, createdBy)

	if err := r.hydrateNodesAndEdges(&action); err != nil {
		return nil, err
	}

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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate asset actions: %w", err)
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
		actions = append(actions, action)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("failed to iterate enabled asset actions: %w", err)
	}
	// Release the cursor before loading nodes/edges: holding it while the
	// nested queries run deadlocks a one-connection read pool.
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := r.populateNodesAndEdges(actions); err != nil {
		return nil, err
	}

	return actions, nil
}

// populateNodesAndEdges attaches nodes and edges to every asset action using
// one query per relation.
func (r *AssetActionRepository) populateNodesAndEdges(actions []*models.AssetAction) error {
	if len(actions) == 0 {
		return nil
	}
	ids := make([]int, len(actions))
	byID := make(map[int]*models.AssetAction, len(actions))
	for i, a := range actions {
		ids[i] = a.ID
		byID[a.ID] = a
	}
	nodeRows, err := r.db.Query(`
		SELECT id, action_id, node_type, node_config, position_x, position_y, created_at, updated_at
		FROM asset_action_nodes
		WHERE action_id IN (`+placeholders(len(ids))+`)
		ORDER BY action_id, id
	`, intArgs(ids)...)
	if err != nil {
		return fmt.Errorf("failed to query asset action nodes: %w", err)
	}
	for nodeRows.Next() {
		var n models.AssetActionNode
		if err := nodeRows.Scan(&n.ID, &n.ActionID, &n.NodeType, &n.NodeConfig, &n.PositionX, &n.PositionY, &n.CreatedAt, &n.UpdatedAt); err != nil {
			_ = nodeRows.Close()
			return fmt.Errorf("failed to scan asset action node: %w", err)
		}
		if a, ok := byID[n.ActionID]; ok {
			a.Nodes = append(a.Nodes, n)
		}
	}
	if err := nodeRows.Err(); err != nil {
		_ = nodeRows.Close()
		return fmt.Errorf("failed to iterate asset action nodes: %w", err)
	}
	if err := nodeRows.Close(); err != nil {
		return fmt.Errorf("failed to close asset action nodes: %w", err)
	}

	edgeRows, err := r.db.Query(`
		SELECT id, action_id, source_node_id, target_node_id, edge_type, source_handle, target_handle, created_at
		FROM asset_action_edges
		WHERE action_id IN (`+placeholders(len(ids))+`)
		ORDER BY action_id, id
	`, intArgs(ids)...)
	if err != nil {
		return fmt.Errorf("failed to query asset action edges: %w", err)
	}
	defer func() { _ = edgeRows.Close() }()
	for edgeRows.Next() {
		var e models.AssetActionEdge
		var sourceHandle, targetHandle sql.NullString
		if err := edgeRows.Scan(&e.ID, &e.ActionID, &e.SourceNodeID, &e.TargetNodeID, &e.EdgeType, &sourceHandle, &targetHandle, &e.CreatedAt); err != nil {
			return fmt.Errorf("failed to scan asset action edge: %w", err)
		}
		if sourceHandle.Valid {
			e.SourceHandle = sourceHandle.String
		}
		if targetHandle.Valid {
			e.TargetHandle = targetHandle.String
		}
		if a, ok := byID[e.ActionID]; ok {
			a.Edges = append(a.Edges, e)
		}
	}
	if err := edgeRows.Err(); err != nil {
		return fmt.Errorf("failed to iterate asset action edges: %w", err)
	}
	return nil
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
	_, err := r.db.ExecWrite(`
		UPDATE asset_actions SET
			name = ?, description = ?, is_enabled = ?, trigger_type = ?, trigger_config = ?, updated_at = ?
		WHERE id = ?
	`,
		action.Name, action.Description, action.IsEnabled, action.TriggerType, action.TriggerConfig,
		time.Now(), action.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update asset action: %w", err)
	}
	return nil
}

// Delete deletes an asset action and its associated nodes and edges (cascade)
func (r *AssetActionRepository) Delete(id int) error {
	result, err := r.db.ExecWrite(`DELETE FROM asset_actions WHERE id = ?`, id)
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
	_, err := r.db.ExecWrite(`UPDATE asset_actions SET is_enabled = ?, updated_at = ? WHERE id = ?`,
		enabled, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to set asset action enabled status: %w", err)
	}
	return nil
}

// hydrateNodesAndEdges fetches and assigns nodes and edges to an AssetAction
// using the shared actionutil.HydrateNodesAndEdges helper.
func (r *AssetActionRepository) hydrateNodesAndEdges(action *models.AssetAction) error {
	genericNodes, genericEdges, err := actionutil.HydrateNodesAndEdges(r.db,
		`SELECT id, action_id, node_type, node_config, position_x, position_y, created_at, updated_at
		 FROM asset_action_nodes WHERE action_id = ? ORDER BY id`,
		`SELECT id, action_id, source_node_id, target_node_id, edge_type, source_handle, target_handle, created_at
		 FROM asset_action_edges WHERE action_id = ? ORDER BY id`,
		action.ID,
	)
	if err != nil {
		return err
	}
	nodes := make([]models.AssetActionNode, len(genericNodes))
	for i, n := range genericNodes {
		nodes[i] = models.AssetActionNode{
			ID: n.ID, ActionID: n.ActionID, NodeType: models.AssetActionNodeType(n.NodeType),
			NodeConfig: n.NodeConfig, PositionX: n.PositionX, PositionY: n.PositionY,
			CreatedAt: n.CreatedAt, UpdatedAt: n.UpdatedAt,
		}
	}
	action.Nodes = nodes

	edges := make([]models.AssetActionEdge, len(genericEdges))
	for i, e := range genericEdges {
		edges[i] = models.AssetActionEdge{
			ID: e.ID, ActionID: e.ActionID, SourceNodeID: e.SourceNodeID,
			TargetNodeID: e.TargetNodeID, EdgeType: e.EdgeType,
			SourceHandle: e.SourceHandle, TargetHandle: e.TargetHandle,
			CreatedAt: e.CreatedAt,
		}
	}
	action.Edges = edges

	return nil
}

// --------- Node Operations ---------

// GetNodesByActionID retrieves all nodes for an asset action
func (r *AssetActionRepository) GetNodesByActionID(actionID int) ([]models.AssetActionNode, error) {
	generic, err := actionutil.ScanNodes(r.db, `
		SELECT id, action_id, node_type, node_config, position_x, position_y, created_at, updated_at
		FROM asset_action_nodes
		WHERE action_id = ?
		ORDER BY id
	`, actionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query asset action nodes: %w", err)
	}
	nodes := make([]models.AssetActionNode, len(generic))
	for i, n := range generic {
		nodes[i] = models.AssetActionNode{
			ID: n.ID, ActionID: n.ActionID, NodeType: models.AssetActionNodeType(n.NodeType),
			NodeConfig: n.NodeConfig, PositionX: n.PositionX, PositionY: n.PositionY,
			CreatedAt: n.CreatedAt, UpdatedAt: n.UpdatedAt,
		}
	}
	return nodes, nil
}

// GetEdgesByActionID retrieves all edges for an asset action
func (r *AssetActionRepository) GetEdgesByActionID(actionID int) ([]models.AssetActionEdge, error) {
	generic, err := actionutil.ScanEdges(r.db, `
		SELECT id, action_id, source_node_id, target_node_id, edge_type, source_handle, target_handle, created_at
		FROM asset_action_edges
		WHERE action_id = ?
		ORDER BY id
	`, actionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query asset action edges: %w", err)
	}
	edges := make([]models.AssetActionEdge, len(generic))
	for i, e := range generic {
		edges[i] = models.AssetActionEdge{
			ID: e.ID, ActionID: e.ActionID, SourceNodeID: e.SourceNodeID,
			TargetNodeID: e.TargetNodeID, EdgeType: e.EdgeType,
			SourceHandle: e.SourceHandle, TargetHandle: e.TargetHandle,
			CreatedAt: e.CreatedAt,
		}
	}
	return edges, nil
}

// CreateNode inserts a single asset action node and returns its id.
func (r *AssetActionRepository) CreateNode(node models.AssetActionNode) (int, error) {
	var id int
	err := r.db.QueryRow(`
		INSERT INTO asset_action_nodes (action_id, node_type, node_config, position_x, position_y, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP) RETURNING id
	`, node.ActionID, node.NodeType, node.NodeConfig, node.PositionX, node.PositionY).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create asset action node: %w", err)
	}
	return id, nil
}

// CreateEdge inserts a single asset action edge and returns its id.
func (r *AssetActionRepository) CreateEdge(edge models.AssetActionEdge) (int, error) {
	var id int
	err := r.db.QueryRow(`
		INSERT INTO asset_action_edges (action_id, source_node_id, target_node_id, edge_type, source_handle, target_handle, created_at)
		VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP) RETURNING id
	`, edge.ActionID, edge.SourceNodeID, edge.TargetNodeID, edge.EdgeType, edge.SourceHandle, edge.TargetHandle).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create asset action edge: %w", err)
	}
	return id, nil
}

// SaveActionWithNodesAndEdges saves an asset action with its nodes and edges in a transaction
func (r *AssetActionRepository) SaveActionWithNodesAndEdges(action *models.AssetAction, nodes []models.AssetActionNode, edges []models.AssetActionEdge) error {
	flowNodes := actionutil.ToFlowNodes(nodes)
	flowEdges := actionutil.ToFlowEdges(edges)

	const updateSQL = `
		UPDATE asset_actions SET
			name = ?, description = ?, is_enabled = ?, trigger_type = ?, trigger_config = ?, updated_at = ?
		WHERE id = ?
	`
	args := []any{
		action.Name, action.Description, action.IsEnabled, action.TriggerType, action.TriggerConfig,
		time.Now(), action.ID,
	}

	return actionutil.UpdateActionGraph(
		r.db, updateSQL, args, action.ID,
		flowNodes, flowEdges,
		actionutil.SQLiteStatements("asset_action_nodes", "asset_action_edges"),
	)
}

// --------- Execution Log Operations ---------

// CreateExecutionLog creates a new asset action execution log entry
func (r *AssetActionRepository) CreateExecutionLog(log *models.AssetActionExecutionLog) (int, error) {
	var id int
	err := r.db.QueryRow(`
		INSERT INTO asset_action_execution_logs (action_id, asset_id, trigger_event, status, started_at, durable_event_key)
		VALUES (?, ?, ?, ?, ?, ?) RETURNING id
	`,
		log.ActionID, log.AssetID, log.TriggerEvent, log.Status, time.Now(), durableEventKeyValue(log.DurableEventKey),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create asset action execution log: %w", err)
	}

	return id, nil
}

// GetExecutionLogByDurableTarget returns the stable log for one durable event
// and frozen asset-action target.
func (r *AssetActionRepository) GetExecutionLogByDurableTarget(eventKey string, actionID int) (*models.AssetActionExecutionLog, error) {
	log := &models.AssetActionExecutionLog{}
	var assetID sql.NullInt64
	var completedAt sql.NullTime
	var errorMessage, executionTrace sql.NullString
	err := r.db.QueryRow(`
		SELECT id, action_id, asset_id, trigger_event, status, started_at,
		       completed_at, error_message, execution_trace, durable_event_key
		FROM asset_action_execution_logs
		WHERE durable_event_key = ? AND action_id = ?
	`, eventKey, actionID).Scan(
		&log.ID, &log.ActionID, &assetID, &log.TriggerEvent, &log.Status, &log.StartedAt,
		&completedAt, &errorMessage, &executionTrace, &log.DurableEventKey,
	)
	if err != nil {
		return nil, err
	}
	if assetID.Valid {
		value := int(assetID.Int64)
		log.AssetID = &value
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
	return log, nil
}

// UpdateExecutionLog updates an asset action execution log entry
func (r *AssetActionRepository) UpdateExecutionLog(log *models.AssetActionExecutionLog) error {
	_, err := r.db.ExecWrite(`
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
