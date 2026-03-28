package repository

import (
	"database/sql"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// LogbookActionRepository provides data access for logbook actions (PostgreSQL)
type LogbookActionRepository struct {
	db database.Database
}

// NewLogbookActionRepository creates a new logbook action repository
func NewLogbookActionRepository(db database.Database) *LogbookActionRepository {
	return &LogbookActionRepository{db: db}
}

// applyLogbookActionNulls sets nullable fields on a LogbookAction from scanned sql.Null values.
func applyLogbookActionNulls(a *models.LogbookAction, description, triggerConfig sql.NullString, createdBy sql.NullInt64) {
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

// GetByID retrieves a logbook action by ID with its nodes and edges
func (r *LogbookActionRepository) GetByID(id int) (*models.LogbookAction, error) {
	var action models.LogbookAction
	var description, triggerConfig sql.NullString
	var createdBy sql.NullInt64

	err := r.db.QueryRow(`
		SELECT id, bucket_id, name, description, is_enabled,
		       trigger_type, trigger_config, created_by, created_at, updated_at
		FROM logbook_actions
		WHERE id = $1
	`, id).Scan(
		&action.ID, &action.BucketID, &action.Name, &description, &action.IsEnabled,
		&action.TriggerType, &triggerConfig, &createdBy, &action.CreatedAt, &action.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find logbook action: %w", err)
	}

	applyLogbookActionNulls(&action, description, triggerConfig, createdBy)

	nodes, err := r.GetNodesByActionID(id)
	if err != nil {
		return nil, fmt.Errorf("failed to get logbook action nodes: %w", err)
	}
	action.Nodes = nodes

	edges, err := r.GetEdgesByActionID(id)
	if err != nil {
		return nil, fmt.Errorf("failed to get logbook action edges: %w", err)
	}
	action.Edges = edges

	return &action, nil
}

// ListByBucket lists all actions for a bucket
func (r *LogbookActionRepository) ListByBucket(bucketID string) ([]*models.LogbookAction, error) {
	rows, err := r.db.Query(`
		SELECT id, bucket_id, name, description, is_enabled,
		       trigger_type, trigger_config, created_by, created_at, updated_at
		FROM logbook_actions
		WHERE bucket_id = $1
		ORDER BY created_at DESC
	`, bucketID)
	if err != nil {
		return nil, fmt.Errorf("failed to query logbook actions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var actions []*models.LogbookAction
	for rows.Next() {
		action := &models.LogbookAction{}
		var description, triggerConfig sql.NullString
		var createdBy sql.NullInt64

		err := rows.Scan(
			&action.ID, &action.BucketID, &action.Name, &description, &action.IsEnabled,
			&action.TriggerType, &triggerConfig, &createdBy, &action.CreatedAt, &action.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan logbook action: %w", err)
		}

		applyLogbookActionNulls(action, description, triggerConfig, createdBy)

		actions = append(actions, action)
	}

	return actions, nil
}

// ListEnabledByBucket lists all enabled actions for a bucket with nodes and edges
func (r *LogbookActionRepository) ListEnabledByBucket(bucketID string) ([]*models.LogbookAction, error) {
	rows, err := r.db.Query(`
		SELECT id, bucket_id, name, description, is_enabled,
		       trigger_type, trigger_config, created_by, created_at, updated_at
		FROM logbook_actions
		WHERE bucket_id = $1 AND is_enabled = true
		ORDER BY created_at DESC
	`, bucketID)
	if err != nil {
		return nil, fmt.Errorf("failed to query enabled logbook actions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var actions []*models.LogbookAction
	for rows.Next() {
		action := &models.LogbookAction{}
		var description, triggerConfig sql.NullString
		var createdBy sql.NullInt64

		err := rows.Scan(
			&action.ID, &action.BucketID, &action.Name, &description, &action.IsEnabled,
			&action.TriggerType, &triggerConfig, &createdBy, &action.CreatedAt, &action.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan logbook action: %w", err)
		}

		applyLogbookActionNulls(action, description, triggerConfig, createdBy)

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

// Create creates a new logbook action
func (r *LogbookActionRepository) Create(action *models.LogbookAction) (int, error) {
	var id int
	err := r.db.QueryRow(`
		INSERT INTO logbook_actions (
			bucket_id, name, description, is_enabled, trigger_type, trigger_config,
			created_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id
	`,
		action.BucketID, action.Name, action.Description, action.IsEnabled,
		action.TriggerType, action.TriggerConfig, action.CreatedBy,
		time.Now(), time.Now(),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create logbook action: %w", err)
	}

	return id, nil
}

// Update updates a logbook action
func (r *LogbookActionRepository) Update(action *models.LogbookAction) error {
	_, err := r.db.Exec(`
		UPDATE logbook_actions SET
			name = $1, description = $2, is_enabled = $3, trigger_type = $4,
			trigger_config = $5, updated_at = $6
		WHERE id = $7
	`,
		action.Name, action.Description, action.IsEnabled, action.TriggerType,
		action.TriggerConfig, time.Now(), action.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update logbook action: %w", err)
	}
	return nil
}

// Delete deletes a logbook action and its associated nodes and edges (cascade)
func (r *LogbookActionRepository) Delete(id int) error {
	result, err := r.db.Exec(`DELETE FROM logbook_actions WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete logbook action: %w", err)
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

// SetEnabled enables or disables a logbook action
func (r *LogbookActionRepository) SetEnabled(id int, enabled bool) error {
	_, err := r.db.Exec(`UPDATE logbook_actions SET is_enabled = $1, updated_at = $2 WHERE id = $3`,
		enabled, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to set logbook action enabled status: %w", err)
	}
	return nil
}

// --------- Node Operations ---------

// GetNodesByActionID retrieves all nodes for a logbook action
func (r *LogbookActionRepository) GetNodesByActionID(actionID int) ([]models.LogbookActionNode, error) {
	rows, err := r.db.Query(`
		SELECT id, action_id, node_type, node_config, position_x, position_y, created_at, updated_at
		FROM logbook_action_nodes
		WHERE action_id = $1
		ORDER BY id
	`, actionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query logbook action nodes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var nodes []models.LogbookActionNode
	for rows.Next() {
		var node models.LogbookActionNode
		err := rows.Scan(
			&node.ID, &node.ActionID, &node.NodeType, &node.NodeConfig,
			&node.PositionX, &node.PositionY, &node.CreatedAt, &node.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan logbook action node: %w", err)
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

// CreateNode creates a new logbook action node
func (r *LogbookActionRepository) CreateNode(node *models.LogbookActionNode) (int, error) {
	var id int
	err := r.db.QueryRow(`
		INSERT INTO logbook_action_nodes (action_id, node_type, node_config, position_x, position_y, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id
	`,
		node.ActionID, node.NodeType, node.NodeConfig, node.PositionX, node.PositionY,
		time.Now(), time.Now(),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create logbook action node: %w", err)
	}

	return id, nil
}

// --------- Edge Operations ---------

// GetEdgesByActionID retrieves all edges for a logbook action
func (r *LogbookActionRepository) GetEdgesByActionID(actionID int) ([]models.LogbookActionEdge, error) {
	rows, err := r.db.Query(`
		SELECT id, action_id, source_node_id, target_node_id, edge_type, source_handle, target_handle, created_at
		FROM logbook_action_edges
		WHERE action_id = $1
		ORDER BY id
	`, actionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query logbook action edges: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var edges []models.LogbookActionEdge
	for rows.Next() {
		var edge models.LogbookActionEdge
		var sourceHandle, targetHandle sql.NullString
		err := rows.Scan(
			&edge.ID, &edge.ActionID, &edge.SourceNodeID, &edge.TargetNodeID,
			&edge.EdgeType, &sourceHandle, &targetHandle, &edge.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan logbook action edge: %w", err)
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

// CreateEdge creates a new logbook action edge
func (r *LogbookActionRepository) CreateEdge(edge *models.LogbookActionEdge) (int, error) {
	var id int
	err := r.db.QueryRow(`
		INSERT INTO logbook_action_edges (action_id, source_node_id, target_node_id, edge_type, source_handle, target_handle, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id
	`,
		edge.ActionID, edge.SourceNodeID, edge.TargetNodeID, edge.EdgeType,
		edge.SourceHandle, edge.TargetHandle, time.Now(),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create logbook action edge: %w", err)
	}

	return id, nil
}

// SaveActionWithNodesAndEdges saves a logbook action with its nodes and edges in a transaction
func (r *LogbookActionRepository) SaveActionWithNodesAndEdges(action *models.LogbookAction, nodes []models.LogbookActionNode, edges []models.LogbookActionEdge) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Delete existing edges then nodes
	_, err = tx.Exec(`DELETE FROM logbook_action_edges WHERE action_id = $1`, action.ID)
	if err != nil {
		return fmt.Errorf("failed to delete existing edges: %w", err)
	}
	_, err = tx.Exec(`DELETE FROM logbook_action_nodes WHERE action_id = $1`, action.ID)
	if err != nil {
		return fmt.Errorf("failed to delete existing nodes: %w", err)
	}

	// Update action
	_, err = tx.Exec(`
		UPDATE logbook_actions SET
			name = $1, description = $2, is_enabled = $3, trigger_type = $4,
			trigger_config = $5, updated_at = $6
		WHERE id = $7
	`,
		action.Name, action.Description, action.IsEnabled, action.TriggerType,
		action.TriggerConfig, time.Now(), action.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update logbook action: %w", err)
	}

	// Insert nodes and build ID mapping
	nodeIDMap := make(map[int]int)
	for _, node := range nodes {
		var newID int
		err = tx.QueryRow(`
			INSERT INTO logbook_action_nodes (action_id, node_type, node_config, position_x, position_y, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id
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
			INSERT INTO logbook_action_edges (action_id, source_node_id, target_node_id, edge_type, source_handle, target_handle, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
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

// CreateExecutionLog creates a new logbook action execution log entry
func (r *LogbookActionRepository) CreateExecutionLog(log *models.LogbookActionExecutionLog) (int, error) {
	var id int
	err := r.db.QueryRow(`
		INSERT INTO logbook_action_execution_logs (action_id, document_id, trigger_event, status, started_at)
		VALUES ($1, $2, $3, $4, $5) RETURNING id
	`,
		log.ActionID, log.DocumentID, log.TriggerEvent, log.Status, time.Now(),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create logbook execution log: %w", err)
	}

	return id, nil
}

// UpdateExecutionLog updates a logbook action execution log entry
func (r *LogbookActionRepository) UpdateExecutionLog(log *models.LogbookActionExecutionLog) error {
	_, err := r.db.Exec(`
		UPDATE logbook_action_execution_logs SET
			status = $1, completed_at = $2, error_message = $3, execution_trace = $4
		WHERE id = $5
	`,
		log.Status, log.CompletedAt, log.ErrorMessage, log.ExecutionTrace, log.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update logbook execution log: %w", err)
	}
	return nil
}

// GetExecutionLogs retrieves execution logs for a specific logbook action
func (r *LogbookActionRepository) GetExecutionLogs(actionID, limit, offset int) ([]*models.LogbookActionExecutionLog, error) {
	rows, err := r.db.Query(`
		SELECT l.id, l.action_id, l.document_id, l.trigger_event, l.status,
		       l.started_at, l.completed_at, l.error_message, l.execution_trace,
		       a.name
		FROM logbook_action_execution_logs l
		LEFT JOIN logbook_actions a ON l.action_id = a.id
		WHERE l.action_id = $1
		ORDER BY l.started_at DESC
		LIMIT $2 OFFSET $3
	`, actionID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query logbook execution logs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return r.scanExecutionLogs(rows)
}

// GetBucketExecutionLogs retrieves execution logs for all actions in a bucket
func (r *LogbookActionRepository) GetBucketExecutionLogs(bucketID string, limit, offset int) ([]*models.LogbookActionExecutionLog, error) {
	rows, err := r.db.Query(`
		SELECT l.id, l.action_id, l.document_id, l.trigger_event, l.status,
		       l.started_at, l.completed_at, l.error_message, l.execution_trace,
		       a.name
		FROM logbook_action_execution_logs l
		LEFT JOIN logbook_actions a ON l.action_id = a.id
		WHERE a.bucket_id = $1
		ORDER BY l.started_at DESC
		LIMIT $2 OFFSET $3
	`, bucketID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query bucket execution logs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return r.scanExecutionLogs(rows)
}

func (r *LogbookActionRepository) scanExecutionLogs(rows *sql.Rows) ([]*models.LogbookActionExecutionLog, error) {
	var logs []*models.LogbookActionExecutionLog
	for rows.Next() {
		log := &models.LogbookActionExecutionLog{}
		var documentID sql.NullString
		var completedAt sql.NullTime
		var errorMessage, executionTrace, actionName sql.NullString

		err := rows.Scan(
			&log.ID, &log.ActionID, &documentID, &log.TriggerEvent, &log.Status,
			&log.StartedAt, &completedAt, &errorMessage, &executionTrace,
			&actionName,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan logbook execution log: %w", err)
		}

		if documentID.Valid {
			log.DocumentID = &documentID.String
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
		if actionName.Valid {
			log.ActionName = actionName.String
		}

		logs = append(logs, log)
	}

	return logs, nil
}

// UpdateDocumentCustomerAssociation updates customer fields on a logbook document
func (r *LogbookActionRepository) UpdateDocumentCustomerAssociation(documentID string, customerOrgID, portalCustomerID *int) error {
	_, err := r.db.Exec(`
		UPDATE logbook_documents SET
			customer_organisation_id = $1, portal_customer_id = $2, updated_at = $3
		WHERE id = $4
	`,
		customerOrgID, portalCustomerID, time.Now(), documentID,
	)
	if err != nil {
		return fmt.Errorf("failed to update document customer association: %w", err)
	}
	return nil
}

// HasBucketPermission checks if a user has a specific permission on a bucket
func (r *LogbookActionRepository) HasBucketPermission(userID int, bucketID, permission string) (bool, error) {
	var count int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM logbook_bucket_permissions
		WHERE bucket_id = $1 AND principal_type = 'user' AND principal_id = $2 AND permission = $3
	`, bucketID, userID, permission).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check bucket permission: %w", err)
	}
	return count > 0, nil
}
