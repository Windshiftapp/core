package services

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// ProjectResult represents a project with workspace details.
type ProjectResult struct {
	ID            int
	Name          string
	Description   string
	Active        bool
	WorkspaceID   *int
	WorkspaceName string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// ProjectListParams contains parameters for listing projects.
type ProjectListParams struct {
	Limit       int
	Offset      int
	WorkspaceID *int // Filter by workspace
}

// ListProjects retrieves projects with pagination and filtering.
func (s *PlanningService) ListProjects(params ProjectListParams) ([]ProjectResult, int, error) {
	query := `
		SELECT p.id, p.name, p.description, p.active, p.workspace_id,
		       w.name as workspace_name, p.created_at, p.updated_at
		FROM projects p
		LEFT JOIN workspaces w ON p.workspace_id = w.id
		WHERE 1=1`
	countQuery := "SELECT COUNT(*) FROM projects p WHERE 1=1"

	var args []interface{}
	var countArgs []interface{}

	// Filter by workspace
	if params.WorkspaceID != nil {
		query += " AND p.workspace_id = ?"
		countQuery += " AND p.workspace_id = ?"
		args = append(args, *params.WorkspaceID)
		countArgs = append(countArgs, *params.WorkspaceID)
	}

	query += " ORDER BY p.name"
	query += " LIMIT ? OFFSET ?"
	args = append(args, params.Limit, params.Offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list projects: %w", err)
	}
	defer rows.Close()

	var projects []ProjectResult
	for rows.Next() {
		var p ProjectResult
		var description, workspaceName sql.NullString
		var workspaceID sql.NullInt64
		err := rows.Scan(&p.ID, &p.Name, &description, &p.Active, &workspaceID, &workspaceName, &p.CreatedAt, &p.UpdatedAt)
		if err != nil {
			continue
		}
		p.Description = description.String
		p.WorkspaceName = workspaceName.String
		if workspaceID.Valid {
			id := int(workspaceID.Int64)
			p.WorkspaceID = &id
		}
		projects = append(projects, p)
	}

	if projects == nil {
		projects = []ProjectResult{}
	}

	var total int
	if err := s.db.QueryRow(countQuery, countArgs...).Scan(&total); err != nil {
		slog.Warn("failed to get project pagination count", slog.Any("error", err))
	}

	return projects, total, nil
}

// GetProject retrieves a project by ID.
func (s *PlanningService) GetProject(id int) (*ProjectResult, error) {
	var p ProjectResult
	var description, workspaceName sql.NullString
	var workspaceID sql.NullInt64
	err := s.db.QueryRow(`
		SELECT p.id, p.name, p.description, p.active, p.workspace_id,
		       w.name as workspace_name, p.created_at, p.updated_at
		FROM projects p
		LEFT JOIN workspaces w ON p.workspace_id = w.id
		WHERE p.id = ?
	`, id).Scan(&p.ID, &p.Name, &description, &p.Active, &workspaceID, &workspaceName, &p.CreatedAt, &p.UpdatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("project not found: %d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}

	p.Description = description.String
	p.WorkspaceName = workspaceName.String
	if workspaceID.Valid {
		wid := int(workspaceID.Int64)
		p.WorkspaceID = &wid
	}

	return &p, nil
}

// CreateProjectParams contains parameters for creating a project.
type CreateProjectParams struct {
	Name        string
	Description string
	WorkspaceID *int
	Active      bool
}

// CreateProject creates a new project.
func (s *PlanningService) CreateProject(params CreateProjectParams) (*ProjectResult, error) {
	var id int64
	err := s.db.QueryRow(`
		INSERT INTO projects (name, description, workspace_id, active)
		VALUES (?, ?, ?, ?) RETURNING id
	`, params.Name, params.Description, params.WorkspaceID, params.Active).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("failed to create project: %w", err)
	}

	return s.GetProject(int(id))
}

// UpdateProjectParams contains parameters for updating a project.
type UpdateProjectParams struct {
	ID          int
	Name        string
	Description string
	WorkspaceID *int
	Active      bool
}

// UpdateProject updates an existing project.
func (s *PlanningService) UpdateProject(params UpdateProjectParams) (*ProjectResult, error) {
	_, err := s.db.ExecWrite(`
		UPDATE projects SET name = ?, description = ?, workspace_id = ?, active = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, params.Name, params.Description, params.WorkspaceID, params.Active, params.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to update project: %w", err)
	}

	return s.GetProject(params.ID)
}

// DeleteProject deletes a project.
func (s *PlanningService) DeleteProject(id int) error {
	_, err := s.db.ExecWrite("DELETE FROM projects WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}
	return nil
}

// GetProjectWorkspaceID returns the workspace_id for a project (for permission checks).
//
// deadcode-keep: only reached from the legacy projects handler (see
// internal/handlers/projects.go), which is itself test-only-reachable.
func (s *PlanningService) GetProjectWorkspaceID(id int) (*int, error) {
	var workspaceID sql.NullInt64
	err := s.db.QueryRow("SELECT workspace_id FROM projects WHERE id = ?", id).Scan(&workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("project not found: %d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get project workspace: %w", err)
	}
	if workspaceID.Valid {
		wid := int(workspaceID.Int64)
		return &wid, nil
	}
	return nil, nil
}

// CategoryExists checks if a milestone category exists.
func (s *PlanningService) CategoryExists(categoryID int) (bool, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM milestone_categories WHERE id = ?", categoryID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check category: %w", err)
	}
	return count > 0, nil
}

// WorkspaceExists checks if a workspace exists.
func (s *PlanningService) WorkspaceExists(workspaceID int) (bool, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM workspaces WHERE id = ?", workspaceID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check workspace: %w", err)
	}
	return count > 0, nil
}

// IterationTypeExists checks if an iteration type exists.
func (s *PlanningService) IterationTypeExists(typeID int) (bool, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM iteration_types WHERE id = ?", typeID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check iteration type: %w", err)
	}
	return count > 0, nil
}

// LoadProjectMilestoneCategories loads milestone categories for a project.
//
// deadcode-keep: only reached from the legacy projects handler (see
// internal/handlers/projects.go), which is itself test-only-reachable.
func (s *PlanningService) LoadProjectMilestoneCategories(projectID int) ([]int, error) {
	var categories []int
	rows, err := s.db.Query(`
		SELECT category_id FROM project_milestone_categories WHERE project_id = ?
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to load milestone categories: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var categoryID int
		if err := rows.Scan(&categoryID); err != nil {
			return nil, fmt.Errorf("failed to scan category ID: %w", err)
		}
		categories = append(categories, categoryID)
	}
	return categories, nil
}

// SaveProjectMilestoneCategories saves milestone categories for a project.
//
// deadcode-keep: only reached from the legacy projects handler (see
// internal/handlers/projects.go), which is itself test-only-reachable.
func (s *PlanningService) SaveProjectMilestoneCategories(projectID int, categories []int) error {
	// Start transaction
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Delete existing associations
	_, err = tx.Exec("DELETE FROM project_milestone_categories WHERE project_id = ?", projectID)
	if err != nil {
		return fmt.Errorf("failed to delete existing categories: %w", err)
	}

	// Insert new associations
	for _, categoryID := range categories {
		_, err = tx.Exec(`
			INSERT INTO project_milestone_categories (project_id, category_id) VALUES (?, ?)
		`, projectID, categoryID)
		if err != nil {
			return fmt.Errorf("failed to insert category: %w", err)
		}
	}

	return tx.Commit()
}

// parseDate tries date-only format first, then falls back to RFC3339.
func parseDate(s string) (time.Time, error) {
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, s)
}
