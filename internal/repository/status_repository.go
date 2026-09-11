package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"windshift/internal/database"
	"windshift/internal/models"
)

// StatusRepository provides status data used by workspace and workflow services.
type StatusRepository struct {
	db database.Database
}

// ListForWorkspaces returns the union of statuses available through each
// workspace's effective workflows. An empty list returns the global catalog.
func (r *StatusRepository) ListForWorkspaces(workspaceIDs []int) ([]models.Status, error) {
	if len(workspaceIDs) == 0 {
		return r.List()
	}
	placeholders := make([]string, len(workspaceIDs))
	args := make([]any, len(workspaceIDs))
	for i, workspaceID := range workspaceIDs {
		placeholders[i] = "?"
		args[i] = workspaceID
	}
	rows, err := r.db.Query(fmt.Sprintf(`
		WITH target_workspaces AS (
			SELECT id, is_personal FROM workspaces WHERE id IN (%s)
		), effective_workflows AS (
			SELECT target.id AS workspace_id, target.is_personal,
			       COALESCE(csit.workflow_id, cs.workflow_id,
			         (SELECT id FROM workflows WHERE is_default = true ORDER BY id LIMIT 1)) AS workflow_id
			FROM target_workspaces target
			LEFT JOIN workspace_configuration_sets wcs ON wcs.workspace_id = target.id
			LEFT JOIN configuration_sets cs ON cs.id = wcs.configuration_set_id
			LEFT JOIN configuration_set_item_types csit ON csit.configuration_set_id = cs.id
		), available_statuses AS (
			SELECT wt.from_status_id AS status_id FROM effective_workflows ew
			JOIN workflow_transitions wt ON wt.workflow_id = ew.workflow_id WHERE wt.from_status_id IS NOT NULL
			UNION
			SELECT wt.to_status_id AS status_id FROM effective_workflows ew
			JOIN workflow_transitions wt ON wt.workflow_id = ew.workflow_id
		)
		SELECT DISTINCT s.id, COALESCE(s.builtin_key, ''), s.name, s.description, s.category_id, s.is_default,
		       sc.name, COALESCE(sc.builtin_key, ''), sc.color, sc.is_completed
		FROM statuses s
		JOIN status_categories sc ON s.category_id = sc.id
		WHERE s.id IN (SELECT status_id FROM available_statuses)
		   OR EXISTS (SELECT 1 FROM target_workspaces WHERE is_personal = true)
		ORDER BY s.category_id, s.name
	`, strings.Join(placeholders, ",")), args...)
	if err != nil {
		return nil, fmt.Errorf("list statuses for workspaces: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]models.Status, 0)
	for rows.Next() {
		var status models.Status
		var description sql.NullString
		if err := rows.Scan(&status.ID, &status.BuiltinKey, &status.Name, &description, &status.CategoryID,
			&status.IsDefault, &status.CategoryName, &status.CategoryBuiltinKey, &status.CategoryColor, &status.IsCompleted); err != nil {
			return nil, fmt.Errorf("scan workspace status: %w", err)
		}
		status.Description = description.String
		out = append(out, status)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspace statuses: %w", err)
	}
	return out, nil
}

// StatusTransitionOption is a status option exposed by a workflow transition.
type StatusTransitionOption struct {
	TransitionID  int
	StatusID      int
	BuiltinKey    string
	StatusName    string
	CategoryColor *string
}

// NewStatusRepository creates a StatusRepository.
func NewStatusRepository(db database.Database) *StatusRepository {
	return &StatusRepository{db: db}
}

const statusJoinedSelect = `
	SELECT s.id, COALESCE(s.builtin_key, ''), s.name, s.description, s.category_id, s.is_default, s.created_at, s.updated_at,
	       sc.name as category_name, COALESCE(sc.builtin_key, ''), sc.color as category_color, sc.is_completed
	FROM statuses s
	JOIN status_categories sc ON s.category_id = sc.id`

// List returns all statuses with their joined category data, ordered the
// same way the handler did (defaults first, then by category, then by name).
func (r *StatusRepository) List() ([]models.Status, error) {
	rows, err := r.db.Query(statusJoinedSelect + ` ORDER BY s.is_default DESC, sc.name ASC, s.name ASC`)
	if err != nil {
		return nil, fmt.Errorf("list statuses: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var statuses []models.Status
	for rows.Next() {
		s, scanErr := scanStatusJoined(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan status: %w", scanErr)
		}
		statuses = append(statuses, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate statuses: %w", err)
	}
	if statuses == nil {
		statuses = []models.Status{}
	}
	return statuses, nil
}

// ListForWorkflow returns statuses participating in a workflow.
func (r *StatusRepository) ListForWorkflow(workflowID int) ([]models.Status, error) {
	rows, err := r.db.Query(statusJoinedSelect+`
		WHERE s.id IN (
			SELECT to_status_id FROM workflow_transitions WHERE workflow_id = ?
			UNION
			SELECT from_status_id FROM workflow_transitions WHERE workflow_id = ? AND from_status_id IS NOT NULL
		)
		ORDER BY s.is_default DESC, s.name
	`, workflowID, workflowID)
	if err != nil {
		return nil, fmt.Errorf("list statuses for workflow %d: %w", workflowID, err)
	}
	defer func() { _ = rows.Close() }()
	statuses := []models.Status{}
	for rows.Next() {
		status, err := scanStatusJoined(rows)
		if err != nil {
			return nil, fmt.Errorf("scan workflow status: %w", err)
		}
		statuses = append(statuses, status)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workflow statuses: %w", err)
	}
	return statuses, nil
}

// GetByID returns a single status with category fields. ErrNotFound when missing.
func (r *StatusRepository) GetByID(id int) (*models.Status, error) {
	row := r.db.QueryRow(statusJoinedSelect+" WHERE s.id = ?", id)
	s, err := scanStatusJoined(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get status %d: %w", id, err)
	}
	return &s, nil
}

// FindByName returns the status with the exact name, or ErrNotFound.
func (r *StatusRepository) FindByName(name string) (*models.Status, error) {
	row := r.db.QueryRow(statusJoinedSelect+" WHERE s.name = ?", name)
	status, err := scanStatusJoined(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find status %q: %w", name, err)
	}
	return &status, nil
}

// CategoryIDs returns status category IDs keyed by status ID.
func (r *StatusRepository) CategoryIDs(statusIDs []int) (map[int]int, error) {
	result := make(map[int]int, len(statusIDs))
	for _, statusID := range statusIDs {
		var categoryID int
		if err := r.db.QueryRow(`SELECT category_id FROM statuses WHERE id = ?`, statusID).Scan(&categoryID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return nil, fmt.Errorf("get status %d category: %w", statusID, err)
		}
		result[statusID] = categoryID
	}
	return result, nil
}

// GetName returns a status name. Missing statuses return an empty name for
// compatibility with the status-transition API.
func (r *StatusRepository) GetName(statusID int64) (string, error) {
	var name string
	err := r.db.QueryRow(`SELECT name FROM statuses WHERE id = ?`, statusID).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return name, err
}

// GetTransitionOption returns display metadata for a status.
func (r *StatusRepository) GetTransitionOption(statusID int64) (*StatusTransitionOption, error) {
	var option StatusTransitionOption
	var categoryColor sql.NullString
	err := r.db.QueryRow(`
		SELECT s.id, COALESCE(s.builtin_key, ''), s.name, sc.color
		FROM statuses s
		LEFT JOIN status_categories sc ON s.category_id = sc.id
		WHERE s.id = ?
	`, statusID).Scan(&option.StatusID, &option.BuiltinKey, &option.StatusName, &categoryColor)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if categoryColor.Valid {
		option.CategoryColor = &categoryColor.String
	}
	return &option, nil
}

// ListAvailableTransitionOptions returns the workflow transitions usable from
// a status: direct edges plus from-all rows targeting statuses without a
// direct edge from this status.
func (r *StatusRepository) ListAvailableTransitionOptions(workflowID int, fromStatusID int64) ([]StatusTransitionOption, error) {
	rows, err := r.db.Query(`
		SELECT wt.id, s.id, COALESCE(s.builtin_key, ''), s.name, sc.color
		FROM workflow_transitions wt
		JOIN statuses s ON wt.to_status_id = s.id
		LEFT JOIN status_categories sc ON s.category_id = sc.id
		WHERE wt.workflow_id = ?
		  AND (
			wt.from_status_id = ?
			OR (
				wt.from_all_statuses = TRUE
				AND wt.to_status_id NOT IN (
					SELECT to_status_id FROM workflow_transitions WHERE workflow_id = ? AND from_status_id = ?
				)
			)
		  )
		ORDER BY CASE WHEN wt.from_all_statuses THEN 1 ELSE 0 END, wt.display_order
	`, workflowID, fromStatusID, workflowID, fromStatusID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var options []StatusTransitionOption
	for rows.Next() {
		var option StatusTransitionOption
		var categoryColor sql.NullString
		if err := rows.Scan(&option.TransitionID, &option.StatusID, &option.BuiltinKey, &option.StatusName, &categoryColor); err != nil {
			continue
		}
		if categoryColor.Valid {
			option.CategoryColor = &categoryColor.String
		}
		options = append(options, option)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return options, nil
}

// ListNonDoneIDs returns the IDs of statuses whose category is not completed.
func (r *StatusRepository) ListNonDoneIDs() ([]int, error) {
	rows, err := r.db.Query(`
		SELECT s.id
		FROM statuses s
		JOIN status_categories sc ON s.category_id = sc.id
		WHERE COALESCE(sc.is_completed, FALSE) = FALSE
		ORDER BY s.id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list non-done status ids: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan status id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate status ids: %w", err)
	}
	if ids == nil {
		ids = []int{}
	}
	return ids, nil
}

func scanStatusJoined(scanner interface {
	Scan(dest ...any) error
}) (models.Status, error) {
	var s models.Status
	if err := scanner.Scan(
		&s.ID, &s.BuiltinKey, &s.Name, &s.Description, &s.CategoryID, &s.IsDefault,
		&s.CreatedAt, &s.UpdatedAt,
		&s.CategoryName, &s.CategoryBuiltinKey, &s.CategoryColor, &s.IsCompleted,
	); err != nil {
		return s, err
	}
	return s, nil
}
