package repository

import (
	"database/sql"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository/actionutil"
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
	ApplyActionNullFieldsToPtr(&a.Description, &a.TriggerConfig, &a.CreatedBy, description, triggerConfig, createdBy)
}

// scanLogbookActions scans rows of logbook actions (without node/edge hydration).
func scanLogbookActions(rows *sql.Rows) ([]*models.LogbookAction, error) {
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

	if err := r.hydrateNodesAndEdges(&action); err != nil {
		return nil, err
	}

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

	actions, err := scanLogbookActions(rows)
	if err != nil {
		return nil, err
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

	actions, err := scanLogbookActions(rows)
	if err != nil {
		return nil, err
	}

	for _, action := range actions {
		if err := r.hydrateNodesAndEdges(action); err != nil {
			return nil, err
		}
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

// hydrateNodesAndEdges fetches and assigns nodes and edges to a LogbookAction
// using the shared actionutil.HydrateNodesAndEdges helper.
func (r *LogbookActionRepository) hydrateNodesAndEdges(action *models.LogbookAction) error {
	genericNodes, genericEdges, err := actionutil.HydrateNodesAndEdges(r.db,
		`SELECT id, action_id, node_type, node_config, position_x, position_y, created_at, updated_at
		 FROM logbook_action_nodes WHERE action_id = $1 ORDER BY id`,
		`SELECT id, action_id, source_node_id, target_node_id, edge_type, source_handle, target_handle, created_at
		 FROM logbook_action_edges WHERE action_id = $1 ORDER BY id`,
		action.ID,
	)
	if err != nil {
		return err
	}
	nodes := make([]models.LogbookActionNode, len(genericNodes))
	for i, n := range genericNodes {
		nodes[i] = models.LogbookActionNode{
			ID: n.ID, ActionID: n.ActionID, NodeType: models.LogbookActionNodeType(n.NodeType),
			NodeConfig: n.NodeConfig, PositionX: n.PositionX, PositionY: n.PositionY,
			CreatedAt: n.CreatedAt, UpdatedAt: n.UpdatedAt,
		}
	}
	action.Nodes = nodes

	edges := make([]models.LogbookActionEdge, len(genericEdges))
	for i, e := range genericEdges {
		edges[i] = models.LogbookActionEdge{
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

// GetNodesByActionID retrieves all nodes for a logbook action
func (r *LogbookActionRepository) GetNodesByActionID(actionID int) ([]models.LogbookActionNode, error) {
	generic, err := actionutil.ScanNodes(r.db, `
		SELECT id, action_id, node_type, node_config, position_x, position_y, created_at, updated_at
		FROM logbook_action_nodes
		WHERE action_id = $1
		ORDER BY id
	`, actionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query logbook action nodes: %w", err)
	}
	nodes := make([]models.LogbookActionNode, len(generic))
	for i, n := range generic {
		nodes[i] = models.LogbookActionNode{
			ID: n.ID, ActionID: n.ActionID, NodeType: models.LogbookActionNodeType(n.NodeType),
			NodeConfig: n.NodeConfig, PositionX: n.PositionX, PositionY: n.PositionY,
			CreatedAt: n.CreatedAt, UpdatedAt: n.UpdatedAt,
		}
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
	generic, err := actionutil.ScanEdges(r.db, `
		SELECT id, action_id, source_node_id, target_node_id, edge_type, source_handle, target_handle, created_at
		FROM logbook_action_edges
		WHERE action_id = $1
		ORDER BY id
	`, actionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query logbook action edges: %w", err)
	}
	edges := make([]models.LogbookActionEdge, len(generic))
	for i, e := range generic {
		edges[i] = models.LogbookActionEdge{
			ID: e.ID, ActionID: e.ActionID, SourceNodeID: e.SourceNodeID,
			TargetNodeID: e.TargetNodeID, EdgeType: e.EdgeType,
			SourceHandle: e.SourceHandle, TargetHandle: e.TargetHandle,
			CreatedAt: e.CreatedAt,
		}
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
	flowNodes := make([]actionutil.FlowNode, len(nodes))
	for i, n := range nodes {
		flowNodes[i] = actionutil.FlowNode{
			ID: n.ID, ActionID: n.ActionID, NodeType: string(n.NodeType),
			NodeConfig: n.NodeConfig, PositionX: n.PositionX, PositionY: n.PositionY,
		}
	}
	flowEdges := make([]actionutil.FlowEdge, len(edges))
	for i, e := range edges {
		flowEdges[i] = actionutil.FlowEdge{
			ID: e.ID, SourceNodeID: e.SourceNodeID, TargetNodeID: e.TargetNodeID,
			EdgeType: e.EdgeType, SourceHandle: e.SourceHandle, TargetHandle: e.TargetHandle,
		}
	}

	const updateSQL = `
		UPDATE logbook_actions SET
			name = $1, description = $2, is_enabled = $3, trigger_type = $4,
			trigger_config = $5, updated_at = $6
		WHERE id = $7
	`
	args := []interface{}{
		action.Name, action.Description, action.IsEnabled, action.TriggerType,
		action.TriggerConfig, time.Now(), action.ID,
	}

	return actionutil.UpdateActionGraph(
		r.db, updateSQL, args, action.ID,
		flowNodes, flowEdges,
		actionutil.PostgresStatements("logbook_action_nodes", "logbook_action_edges"),
	)
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
