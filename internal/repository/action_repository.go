// Package repository provides data access layer implementations.
package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository/actionutil"
)

// ActionRepository provides data access methods for actions and related entities
type ActionRepository struct {
	db database.Database
}

// NewActionRepository creates a new action repository
func NewActionRepository(db database.Database) *ActionRepository {
	return &ActionRepository{db: db}
}

// IsEnabledLLMConnection reports whether an LLM connection exists and is enabled.
func (r *ActionRepository) IsEnabledLLMConnection(connectionID int) bool {
	var exists int
	return r.db.QueryRow(`SELECT 1 FROM llm_connections WHERE id = ? AND is_enabled = true`, connectionID).Scan(&exists) == nil
}

// applyActionNulls sets nullable fields on an Action from scanned sql.Null values.
func applyActionNulls(a *models.Action, description, triggerConfig sql.NullString, createdBy, actorUserID sql.NullInt64) {
	ApplyActionNullFieldsToPtr(&a.Description, &a.TriggerConfig, &a.CreatedBy, description, triggerConfig, createdBy)
	if actorUserID.Valid {
		v := int(actorUserID.Int64)
		a.ActorUserID = &v
	}
}

// GetByID retrieves an action by ID with its nodes and edges
func (r *ActionRepository) GetByID(id int) (*models.Action, error) {
	var action models.Action
	var description, triggerConfig sql.NullString
	var createdBy, actorUserID sql.NullInt64
	var creatorName, actorName sql.NullString

	err := r.db.QueryRow(`
		SELECT a.id, a.workspace_id, a.name, a.description, a.is_enabled,
		       a.trigger_type, a.trigger_config, a.created_by, a.actor_user_id,
		       a.created_at, a.updated_at,
		       u.first_name || ' ' || u.last_name,
		       actor.first_name || ' ' || actor.last_name
		FROM actions a
		LEFT JOIN users u ON a.created_by = u.id
		LEFT JOIN users actor ON a.actor_user_id = actor.id
		WHERE a.id = ?
	`, id).Scan(
		&action.ID, &action.WorkspaceID, &action.Name, &description, &action.IsEnabled,
		&action.TriggerType, &triggerConfig, &createdBy, &actorUserID,
		&action.CreatedAt, &action.UpdatedAt,
		&creatorName, &actorName,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find action: %w", err)
	}

	applyActionNulls(&action, description, triggerConfig, createdBy, actorUserID)
	if creatorName.Valid {
		action.CreatorName = creatorName.String
	}
	if actorName.Valid {
		action.ActorName = actorName.String
	}

	// Load nodes
	nodes, err := r.GetNodesByActionID(id)
	if err != nil {
		return nil, fmt.Errorf("failed to get action nodes: %w", err)
	}
	action.Nodes = nodes

	// Load edges
	edges, err := r.GetEdgesByActionID(id)
	if err != nil {
		return nil, fmt.Errorf("failed to get action edges: %w", err)
	}
	action.Edges = edges

	return &action, nil
}

// ListByWorkspace lists all actions for a workspace
func (r *ActionRepository) ListByWorkspace(workspaceID int) ([]*models.Action, error) {
	rows, err := r.db.Query(`
		SELECT a.id, a.workspace_id, a.name, a.description, a.is_enabled,
		       a.trigger_type, a.trigger_config, a.created_by, a.actor_user_id,
		       a.created_at, a.updated_at,
		       u.first_name || ' ' || u.last_name,
		       actor.first_name || ' ' || actor.last_name
		FROM actions a
		LEFT JOIN users u ON a.created_by = u.id
		LEFT JOIN users actor ON a.actor_user_id = actor.id
		WHERE a.workspace_id = ?
		ORDER BY a.created_at DESC
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to query actions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var actions []*models.Action
	for rows.Next() {
		action := &models.Action{}
		var description, triggerConfig sql.NullString
		var createdBy, actorUserID sql.NullInt64
		var creatorName, actorName sql.NullString

		err := rows.Scan(
			&action.ID, &action.WorkspaceID, &action.Name, &description, &action.IsEnabled,
			&action.TriggerType, &triggerConfig, &createdBy, &actorUserID,
			&action.CreatedAt, &action.UpdatedAt,
			&creatorName, &actorName,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan action: %w", err)
		}

		applyActionNulls(action, description, triggerConfig, createdBy, actorUserID)
		if creatorName.Valid {
			action.CreatorName = creatorName.String
		}
		if actorName.Valid {
			action.ActorName = actorName.String
		}

		actions = append(actions, action)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate actions: %w", err)
	}

	return actions, nil
}

// ListEnabledByWorkspace lists all enabled actions for a workspace
func (r *ActionRepository) ListEnabledByWorkspace(workspaceID int) ([]*models.Action, error) {
	rows, err := r.db.Query(`
		SELECT a.id, a.workspace_id, a.name, a.description, a.is_enabled,
		       a.trigger_type, a.trigger_config, a.created_by, a.actor_user_id,
		       a.created_at, a.updated_at
		FROM actions a
		WHERE a.workspace_id = ? AND a.is_enabled = true
		ORDER BY a.created_at DESC
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to query enabled actions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var actions []*models.Action
	for rows.Next() {
		action := &models.Action{}
		var description, triggerConfig sql.NullString
		var createdBy, actorUserID sql.NullInt64

		err := rows.Scan(
			&action.ID, &action.WorkspaceID, &action.Name, &description, &action.IsEnabled,
			&action.TriggerType, &triggerConfig, &createdBy, &actorUserID,
			&action.CreatedAt, &action.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan action: %w", err)
		}

		applyActionNulls(action, description, triggerConfig, createdBy, actorUserID)

		// Load nodes for execution
		nodes, err := r.GetNodesByActionID(action.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get action nodes: %w", err)
		}
		action.Nodes = nodes

		// Load edges for execution
		edges, err := r.GetEdgesByActionID(action.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get action edges: %w", err)
		}
		action.Edges = edges

		actions = append(actions, action)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate enabled actions: %w", err)
	}

	return actions, nil
}

// Create creates a new action
func (r *ActionRepository) Create(action *models.Action) (int, error) {
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO actions (
			workspace_id, name, description, is_enabled, trigger_type, trigger_config,
			created_by, actor_user_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`,
		action.WorkspaceID, action.Name, action.Description, action.IsEnabled,
		action.TriggerType, action.TriggerConfig, action.CreatedBy, action.ActorUserID,
		time.Now(), time.Now(),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create action: %w", err)
	}

	return int(id), nil
}

// Update updates an action (actor_user_id is patched separately via SetActor).
func (r *ActionRepository) Update(action *models.Action) error {
	_, err := r.db.Exec(`
		UPDATE actions SET
			name = ?, description = ?, is_enabled = ?, trigger_type = ?,
			trigger_config = ?, updated_at = ?
		WHERE id = ?
	`,
		action.Name, action.Description, action.IsEnabled, action.TriggerType,
		action.TriggerConfig, time.Now(), action.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update action: %w", err)
	}
	return nil
}

// SetActor updates only the actor_user_id of an action. The handler is responsible
// for verifying the caller has the global action.set_actor permission before
// calling this. Passing nil clears the override (action will run as triggering user).
func (r *ActionRepository) SetActor(actionID int, actorUserID *int) error {
	_, err := r.db.Exec(
		`UPDATE actions SET actor_user_id = ?, updated_at = ? WHERE id = ?`,
		actorUserID, time.Now(), actionID,
	)
	if err != nil {
		return fmt.Errorf("failed to set action actor: %w", err)
	}
	return nil
}

// Delete deletes an action and its associated nodes and edges
func (r *ActionRepository) Delete(id int) error {
	result, err := r.db.Exec(`DELETE FROM actions WHERE id = ?`, id)
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

// SetEnabled enables or disables an action
func (r *ActionRepository) SetEnabled(id int, enabled bool) error {
	_, err := r.db.Exec(`UPDATE actions SET is_enabled = ?, updated_at = ? WHERE id = ?`,
		enabled, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to set action enabled status: %w", err)
	}
	return nil
}

// --------- Node Operations ---------

// GetNodesByActionID retrieves all nodes for an action
func (r *ActionRepository) GetNodesByActionID(actionID int) ([]models.ActionNode, error) {
	generic, err := actionutil.ScanNodes(r.db, `
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

// CreateNode creates a new action node
func (r *ActionRepository) CreateNode(node *models.ActionNode) (int, error) {
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO action_nodes (action_id, node_type, node_config, position_x, position_y, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id
	`,
		node.ActionID, node.NodeType, node.NodeConfig, node.PositionX, node.PositionY,
		time.Now(), time.Now(),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create action node: %w", err)
	}

	return int(id), nil
}

// UpdateNode updates an action node
func (r *ActionRepository) UpdateNode(node *models.ActionNode) error {
	_, err := r.db.Exec(`
		UPDATE action_nodes SET
			node_type = ?, node_config = ?, position_x = ?, position_y = ?, updated_at = ?
		WHERE id = ?
	`,
		node.NodeType, node.NodeConfig, node.PositionX, node.PositionY, time.Now(), node.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update action node: %w", err)
	}
	return nil
}

// DeleteNode deletes an action node
func (r *ActionRepository) DeleteNode(id int) error {
	_, err := r.db.Exec(`DELETE FROM action_nodes WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete action node: %w", err)
	}
	return nil
}

// DeleteNodesByActionID deletes all nodes for an action
func (r *ActionRepository) DeleteNodesByActionID(actionID int) error {
	_, err := r.db.Exec(`DELETE FROM action_nodes WHERE action_id = ?`, actionID)
	if err != nil {
		return fmt.Errorf("failed to delete action nodes: %w", err)
	}
	return nil
}

// --------- Edge Operations ---------

// GetEdgesByActionID retrieves all edges for an action
func (r *ActionRepository) GetEdgesByActionID(actionID int) ([]models.ActionEdge, error) {
	generic, err := actionutil.ScanEdges(r.db, `
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

// CreateEdge creates a new action edge
func (r *ActionRepository) CreateEdge(edge *models.ActionEdge) (int, error) {
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO action_edges (action_id, source_node_id, target_node_id, edge_type, source_handle, target_handle, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id
	`,
		edge.ActionID, edge.SourceNodeID, edge.TargetNodeID, edge.EdgeType,
		edge.SourceHandle, edge.TargetHandle, time.Now(),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create action edge: %w", err)
	}

	return int(id), nil
}

// DeleteEdge deletes an action edge
func (r *ActionRepository) DeleteEdge(id int) error {
	_, err := r.db.Exec(`DELETE FROM action_edges WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete action edge: %w", err)
	}
	return nil
}

// DeleteEdgesByActionID deletes all edges for an action
func (r *ActionRepository) DeleteEdgesByActionID(actionID int) error {
	_, err := r.db.Exec(`DELETE FROM action_edges WHERE action_id = ?`, actionID)
	if err != nil {
		return fmt.Errorf("failed to delete action edges: %w", err)
	}
	return nil
}

// --------- Execution Log Operations ---------

// CreateExecutionLog creates a new execution log entry
func (r *ActionRepository) CreateExecutionLog(log *models.ActionExecutionLog) (int, error) {
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO action_execution_logs (
			action_id, item_id, trigger_event, status,
			trigger_user_id, effective_actor_user_id, started_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id
	`,
		log.ActionID, log.ItemID, log.TriggerEvent, log.Status,
		log.TriggerUserID, log.EffectiveActorUserID, time.Now(),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create execution log: %w", err)
	}

	return int(id), nil
}

// UpdateExecutionLog updates an execution log entry
func (r *ActionRepository) UpdateExecutionLog(log *models.ActionExecutionLog) error {
	_, err := r.db.Exec(`
		UPDATE action_execution_logs SET
			status = ?, completed_at = ?, error_message = ?, execution_trace = ?
		WHERE id = ?
	`,
		log.Status, log.CompletedAt, log.ErrorMessage, log.ExecutionTrace, log.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update execution log: %w", err)
	}
	return nil
}

// GetExecutionLogsByActionID retrieves execution logs for an action
func (r *ActionRepository) GetExecutionLogsByActionID(actionID, limit, offset int) ([]*models.ActionExecutionLog, error) {
	rows, err := r.db.Query(`
		SELECT l.id, l.action_id, l.item_id, l.trigger_event, l.status,
		       l.trigger_user_id, l.effective_actor_user_id,
		       l.started_at, l.completed_at, l.error_message, l.execution_trace,
		       a.name, i.title
		FROM action_execution_logs l
		LEFT JOIN actions a ON l.action_id = a.id
		LEFT JOIN items i ON l.item_id = i.id
		WHERE l.action_id = ?
		ORDER BY l.started_at DESC
		LIMIT ? OFFSET ?
	`, actionID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query execution logs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return r.scanExecutionLogs(rows)
}

// GetExecutionLogsByWorkspaceID retrieves execution logs for a workspace
func (r *ActionRepository) GetExecutionLogsByWorkspaceID(workspaceID, limit, offset int) ([]*models.ActionExecutionLog, error) {
	rows, err := r.db.Query(`
		SELECT l.id, l.action_id, l.item_id, l.trigger_event, l.status,
		       l.trigger_user_id, l.effective_actor_user_id,
		       l.started_at, l.completed_at, l.error_message, l.execution_trace,
		       a.name, i.title
		FROM action_execution_logs l
		LEFT JOIN actions a ON l.action_id = a.id
		LEFT JOIN items i ON l.item_id = i.id
		WHERE a.workspace_id = ?
		ORDER BY l.started_at DESC
		LIMIT ? OFFSET ?
	`, workspaceID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query execution logs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return r.scanExecutionLogs(rows)
}

// RecentExecutionLogsOpts controls cross-workspace queries against the
// action_execution_logs table for admin diagnostics.
type RecentExecutionLogsOpts struct {
	Status string    // "" for any; e.g. "failed", "completed", "running"
	Since  time.Time // include rows with started_at >= Since (zero = no lower bound)
	Limit  int       // hard-capped to 200 by the caller
	// SortBy controls ordering:
	//   "started_at" (default) — newest first
	//   "duration"             — longest-running completed runs first
	SortBy string
}

// GetRecentExecutionLogs returns logs across all workspaces, joined with action,
// item, and workspace metadata. Intended for the admin diagnostics page.
//
// When SortBy == "duration", only rows with completed_at IS NOT NULL are returned,
// ordered by run length descending — useful for surfacing the slowest recent runs.
func (r *ActionRepository) GetRecentExecutionLogs(opts RecentExecutionLogsOpts) ([]*models.ActionExecutionLog, error) {
	conds := []string{"1=1"}
	args := []any{}
	if opts.Status != "" {
		conds = append(conds, "l.status = ?")
		args = append(args, opts.Status)
	}
	if !opts.Since.IsZero() {
		conds = append(conds, "l.started_at >= ?")
		args = append(args, opts.Since)
	}

	orderBy := "l.started_at DESC"
	if opts.SortBy == "duration" {
		conds = append(conds, "l.completed_at IS NOT NULL")
		// Postgres timestamp subtraction yields an interval (orderable);
		// SQLite has no interval type, so use the JulianDay diff there.
		if r.db.GetDriverName() == "postgres" {
			orderBy = "(l.completed_at - l.started_at) DESC"
		} else {
			orderBy = "(julianday(l.completed_at) - julianday(l.started_at)) DESC"
		}
	}

	limit := opts.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	query := fmt.Sprintf(`
		SELECT l.id, l.action_id, l.item_id, l.trigger_event, l.status,
		       l.trigger_user_id, l.effective_actor_user_id,
		       l.started_at, l.completed_at, l.error_message, l.execution_trace,
		       a.name, i.title, w.id, w.name
		FROM action_execution_logs l
		LEFT JOIN actions a ON l.action_id = a.id
		LEFT JOIN items i ON l.item_id = i.id
		LEFT JOIN workspaces w ON a.workspace_id = w.id
		WHERE %s
		ORDER BY %s
		LIMIT ?
	`, strings.Join(conds, " AND "), orderBy)
	args = append(args, limit)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query recent execution logs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var logs []*models.ActionExecutionLog
	for rows.Next() {
		log := &models.ActionExecutionLog{}
		var itemID, triggerUserID, effectiveActorUserID, workspaceID sql.NullInt64
		var completedAt sql.NullTime
		var errorMessage, executionTrace, actionName, itemTitle, workspaceName sql.NullString

		if err := rows.Scan(
			&log.ID, &log.ActionID, &itemID, &log.TriggerEvent, &log.Status,
			&triggerUserID, &effectiveActorUserID,
			&log.StartedAt, &completedAt, &errorMessage, &executionTrace,
			&actionName, &itemTitle, &workspaceID, &workspaceName,
		); err != nil {
			return nil, fmt.Errorf("failed to scan recent execution log: %w", err)
		}

		if itemID.Valid {
			v := int(itemID.Int64)
			log.ItemID = &v
		}
		if triggerUserID.Valid {
			v := int(triggerUserID.Int64)
			log.TriggerUserID = &v
		}
		if effectiveActorUserID.Valid {
			v := int(effectiveActorUserID.Int64)
			log.EffectiveActorUserID = &v
		}
		if completedAt.Valid {
			log.CompletedAt = &completedAt.Time
			ms := completedAt.Time.Sub(log.StartedAt).Milliseconds()
			log.DurationMs = &ms
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
		if itemTitle.Valid {
			log.ItemTitle = itemTitle.String
		}
		if workspaceID.Valid {
			v := int(workspaceID.Int64)
			log.WorkspaceID = &v
		}
		if workspaceName.Valid {
			log.WorkspaceName = workspaceName.String
		}

		logs = append(logs, log)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate recent execution logs: %w", err)
	}

	return logs, nil
}

func (r *ActionRepository) scanExecutionLogs(rows *sql.Rows) ([]*models.ActionExecutionLog, error) {
	var logs []*models.ActionExecutionLog
	for rows.Next() {
		log := &models.ActionExecutionLog{}
		var itemID, triggerUserID, effectiveActorUserID sql.NullInt64
		var completedAt sql.NullTime
		var errorMessage, executionTrace, actionName, itemTitle sql.NullString

		err := rows.Scan(
			&log.ID, &log.ActionID, &itemID, &log.TriggerEvent, &log.Status,
			&triggerUserID, &effectiveActorUserID,
			&log.StartedAt, &completedAt, &errorMessage, &executionTrace,
			&actionName, &itemTitle,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan execution log: %w", err)
		}

		if itemID.Valid {
			val := int(itemID.Int64)
			log.ItemID = &val
		}
		if triggerUserID.Valid {
			v := int(triggerUserID.Int64)
			log.TriggerUserID = &v
		}
		if effectiveActorUserID.Valid {
			v := int(effectiveActorUserID.Int64)
			log.EffectiveActorUserID = &v
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
		if itemTitle.Valid {
			log.ItemTitle = itemTitle.String
		}

		logs = append(logs, log)
	}

	return logs, nil
}

// BatchInsertExecutionLogs inserts multiple execution logs in a single transaction
func (r *ActionRepository) BatchInsertExecutionLogs(logs []models.ActionExecutionLog) error {
	if len(logs) == 0 {
		return nil
	}

	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, log := range logs {
		_, err := tx.Exec(`
			INSERT INTO action_execution_logs (
				action_id, item_id, trigger_event, status,
				trigger_user_id, effective_actor_user_id,
				started_at, completed_at, error_message, execution_trace
			)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
			log.ActionID, log.ItemID, log.TriggerEvent, log.Status,
			log.TriggerUserID, log.EffectiveActorUserID,
			log.StartedAt, log.CompletedAt, log.ErrorMessage, log.ExecutionTrace,
		)
		if err != nil {
			return fmt.Errorf("failed to insert execution log: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// --- Capability CRUD ---

// scanCapability scans a single capability row from the standard column set
// (id, name, capability_type, config, is_enabled, applies_to_all_workspaces,
// created_by, created_at, updated_at). Does NOT populate WorkspaceIDs — that's
// a separate query against action_capability_workspaces; callers do it on
// demand to avoid an N+1 on the list path.
func scanCapability(scanner interface{ Scan(dest ...any) error }) (*models.ActionCapability, error) {
	var capability models.ActionCapability
	var createdBy sql.NullInt64
	if err := scanner.Scan(
		&capability.ID, &capability.Name, &capability.CapabilityType, &capability.Config,
		&capability.IsEnabled, &capability.AppliesToAllWorkspaces,
		&createdBy, &capability.CreatedAt, &capability.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if createdBy.Valid {
		val := int(createdBy.Int64)
		capability.CreatedBy = &val
	}
	return &capability, nil
}

// GetCapabilityByID retrieves a capability by ID. WorkspaceIDs is populated
// from the join table when AppliesToAllWorkspaces is false.
func (r *ActionRepository) GetCapabilityByID(id int) (*models.ActionCapability, error) {
	capability, err := scanCapability(r.db.QueryRow(`
		SELECT id, name, capability_type, config, is_enabled, applies_to_all_workspaces,
		       created_by, created_at, updated_at
		FROM action_capabilities WHERE id = ?
	`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get capability: %w", err)
	}
	if !capability.AppliesToAllWorkspaces {
		ws, err := r.GetCapabilityWorkspaceIDs(capability.ID)
		if err != nil {
			return nil, err
		}
		capability.WorkspaceIDs = ws
	}
	return capability, nil
}

// queryCapabilities runs a SELECT that returns capability rows and scans them
// into a slice via scanCapability. Populates WorkspaceIDs in a single follow-up
// query that joins all scoped capabilities at once.
func (r *ActionRepository) queryCapabilities(errLabel, query string, args ...interface{}) ([]*models.ActionCapability, error) {
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", errLabel, err)
	}
	defer func() { _ = rows.Close() }()

	var caps []*models.ActionCapability
	for rows.Next() {
		c, err := scanCapability(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan capability: %w", err)
		}
		caps = append(caps, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate capabilities: %w", err)
	}

	if err := r.populateWorkspaceIDs(caps); err != nil {
		return nil, err
	}
	return caps, nil
}

// populateWorkspaceIDs fills the WorkspaceIDs slice on each capability whose
// AppliesToAllWorkspaces is false. Uses a single IN-query rather than per-row
// lookups to keep the list path O(1) DB calls.
func (r *ActionRepository) populateWorkspaceIDs(caps []*models.ActionCapability) error {
	scopedByID := map[int]*models.ActionCapability{}
	ids := []interface{}{}
	for _, c := range caps {
		if !c.AppliesToAllWorkspaces {
			scopedByID[c.ID] = c
			ids = append(ids, c.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}

	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	query := "SELECT capability_id, workspace_id FROM action_capability_workspaces WHERE capability_id IN (" + placeholders + ") ORDER BY workspace_id"
	rows, err := r.db.Query(query, ids...)
	if err != nil {
		return fmt.Errorf("failed to load capability workspace scope: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var capID, wsID int
		if err := rows.Scan(&capID, &wsID); err != nil {
			return fmt.Errorf("failed to scan capability workspace scope row: %w", err)
		}
		if c, ok := scopedByID[capID]; ok {
			c.WorkspaceIDs = append(c.WorkspaceIDs, wsID)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to iterate capability workspace scope rows: %w", err)
	}
	return nil
}

// ListCapabilities retrieves all capabilities.
func (r *ActionRepository) ListCapabilities() ([]*models.ActionCapability, error) {
	return r.queryCapabilities("failed to list capabilities", `
		SELECT id, name, capability_type, config, is_enabled, applies_to_all_workspaces,
		       created_by, created_at, updated_at
		FROM action_capabilities ORDER BY id
	`)
}

// ListEnabledCapabilities retrieves all enabled capabilities.
func (r *ActionRepository) ListEnabledCapabilities() ([]*models.ActionCapability, error) {
	return r.queryCapabilities("failed to list enabled capabilities", `
		SELECT id, name, capability_type, config, is_enabled, applies_to_all_workspaces,
		       created_by, created_at, updated_at
		FROM action_capabilities WHERE is_enabled = true ORDER BY id
	`)
}

// ListCapabilitiesForWorkspace returns capabilities a given workspace's actions
// may reference: every enabled capability whose AppliesToAllWorkspaces is true,
// PLUS every enabled capability explicitly scoped to this workspace via the
// join table. Optional capType filter narrows by capability_type.
func (r *ActionRepository) ListCapabilitiesForWorkspace(workspaceID int, capType string) ([]*models.ActionCapability, error) {
	args := []interface{}{}
	typeFilter := ""
	if capType != "" {
		typeFilter = " AND capability_type = ?"
		args = append(args, capType)
	}
	query := `
		SELECT id, name, capability_type, config, is_enabled, applies_to_all_workspaces,
		       created_by, created_at, updated_at
		FROM action_capabilities
		WHERE is_enabled = true` + typeFilter + `
		  AND (
		    applies_to_all_workspaces = true
		    OR id IN (SELECT capability_id FROM action_capability_workspaces WHERE workspace_id = ?)
		  )
		ORDER BY name`
	args = append(args, workspaceID)
	return r.queryCapabilities("failed to list capabilities for workspace", query, args...)
}

// IsCapabilityScopedToWorkspace returns true if the capability either applies
// to all workspaces or is explicitly scoped to the given workspace. Used by
// resolveCapability to gate execution.
func (r *ActionRepository) IsCapabilityScopedToWorkspace(capabilityID, workspaceID int) (bool, error) {
	var appliesAll bool
	err := r.db.QueryRow(`SELECT applies_to_all_workspaces FROM action_capabilities WHERE id = ?`, capabilityID).Scan(&appliesAll)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("failed to load capability scope: %w", err)
	}
	if appliesAll {
		return true, nil
	}
	var n int
	err = r.db.QueryRow(`SELECT COUNT(*) FROM action_capability_workspaces WHERE capability_id = ? AND workspace_id = ?`, capabilityID, workspaceID).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("failed to check capability workspace scope: %w", err)
	}
	return n > 0, nil
}

// GetCapabilityWorkspaceIDs returns the workspace IDs scoped to a capability.
// Empty when the capability applies to all workspaces.
func (r *ActionRepository) GetCapabilityWorkspaceIDs(capabilityID int) ([]int, error) {
	rows, err := r.db.Query(`SELECT workspace_id FROM action_capability_workspaces WHERE capability_id = ? ORDER BY workspace_id`, capabilityID)
	if err != nil {
		return nil, fmt.Errorf("failed to load capability workspace ids: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan capability workspace id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate capability workspace ids: %w", err)
	}
	return ids, nil
}

// SetCapabilityWorkspaces replaces the workspace allowlist for a capability.
// Pass an empty slice to clear (only meaningful when AppliesToAllWorkspaces is
// false; the caller is responsible for that invariant).
func (r *ActionRepository) SetCapabilityWorkspaces(capabilityID int, workspaceIDs []int) error {
	if _, err := r.db.Exec(`DELETE FROM action_capability_workspaces WHERE capability_id = ?`, capabilityID); err != nil {
		return fmt.Errorf("failed to clear capability workspace scope: %w", err)
	}
	for _, wsID := range workspaceIDs {
		if _, err := r.db.Exec(`INSERT INTO action_capability_workspaces (capability_id, workspace_id) VALUES (?, ?)`, capabilityID, wsID); err != nil {
			return fmt.Errorf("failed to add capability workspace scope (ws %d): %w", wsID, err)
		}
	}
	return nil
}

// CreateCapability creates a new capability.
func (r *ActionRepository) CreateCapability(c *models.ActionCapability) (int, error) {
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO action_capabilities (name, capability_type, config, is_enabled, applies_to_all_workspaces, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`,
		c.Name, c.CapabilityType, c.Config, c.IsEnabled, c.AppliesToAllWorkspaces,
		c.CreatedBy, time.Now(), time.Now(),
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create capability: %w", err)
	}
	return int(id), nil
}

// UpdateCapability updates a capability.
func (r *ActionRepository) UpdateCapability(c *models.ActionCapability) error {
	_, err := r.db.Exec(`
		UPDATE action_capabilities SET name = ?, config = ?, is_enabled = ?, applies_to_all_workspaces = ?, updated_at = ?
		WHERE id = ?
	`,
		c.Name, c.Config, c.IsEnabled, c.AppliesToAllWorkspaces, time.Now(), c.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update capability: %w", err)
	}
	return nil
}

// DeleteCapability deletes a capability by ID.
func (r *ActionRepository) DeleteCapability(id int) error {
	_, err := r.db.Exec(`DELETE FROM action_capabilities WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete capability: %w", err)
	}
	return nil
}

// SaveActionWithNodesAndEdges saves an action along with its nodes and edges in a transaction
func (r *ActionRepository) SaveActionWithNodesAndEdges(action *models.Action, nodes []models.ActionNode, edges []models.ActionEdge) error {
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
		UPDATE actions SET
			name = ?, description = ?, is_enabled = ?, trigger_type = ?,
			trigger_config = ?, updated_at = ?
		WHERE id = ?
	`
	args := []interface{}{
		action.Name, action.Description, action.IsEnabled, action.TriggerType,
		action.TriggerConfig, time.Now(), action.ID,
	}

	return actionutil.UpdateActionGraph(
		r.db, updateSQL, args, action.ID,
		flowNodes, flowEdges,
		actionutil.SQLiteStatements("action_nodes", "action_edges"),
	)
}
