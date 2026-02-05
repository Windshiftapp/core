package services

import (
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// WorkspaceService encapsulates workspace business logic used by both HTTP handlers
// and other services.
type WorkspaceService struct {
	db   database.Database
	repo *repository.WorkspaceRepository
}

// NewWorkspaceService creates a new WorkspaceService.
func NewWorkspaceService(db database.Database) *WorkspaceService {
	return &WorkspaceService{
		db:   db,
		repo: repository.NewWorkspaceRepository(db),
	}
}

// WorkspaceListParams contains the parameters for listing workspaces.
type WorkspaceListParams struct {
	UserID int
	Limit  int
	Offset int
}

// WorkspaceListResult contains a workspace with minimal fields for list views.
type WorkspaceListResult struct {
	ID          int
	Name        string
	Key         string
	Description string
	Active      bool
	IsPersonal  bool
	Icon        string
	Color       string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// List retrieves all workspaces accessible to a user with pagination.
// This checks both direct user workspace roles and group workspace roles.
func (s *WorkspaceService) List(params WorkspaceListParams) ([]WorkspaceListResult, int, error) {
	rows, err := s.db.Query(`
		SELECT DISTINCT w.id, w.name, w.key, w.description, w.active, w.is_personal,
		       w.icon, w.color, w.created_at, w.updated_at
		FROM workspaces w
		LEFT JOIN user_workspace_roles uwr ON w.id = uwr.workspace_id AND uwr.user_id = ?
		LEFT JOIN (
			SELECT DISTINCT gwr.workspace_id
			FROM group_workspace_roles gwr
			JOIN group_members gm ON gwr.group_id = gm.group_id
			WHERE gm.user_id = ?
		) grp ON w.id = grp.workspace_id
		WHERE w.active = 1
		   OR (w.active = 0 AND uwr.role_id IS NOT NULL)
		   OR (w.active = 0 AND grp.workspace_id IS NOT NULL)
		   OR (w.is_personal = 1 AND w.owner_id = ?)
		ORDER BY w.name
		LIMIT ? OFFSET ?
	`, params.UserID, params.UserID, params.UserID, params.Limit, params.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list workspaces: %w", err)
	}
	defer rows.Close()

	var workspaces []WorkspaceListResult
	for rows.Next() {
		var ws WorkspaceListResult
		var icon, color sql.NullString
		err = rows.Scan(&ws.ID, &ws.Name, &ws.Key, &ws.Description, &ws.Active, &ws.IsPersonal,
			&icon, &color, &ws.CreatedAt, &ws.UpdatedAt)
		if err != nil {
			continue
		}
		ws.Icon = icon.String
		ws.Color = color.String
		workspaces = append(workspaces, ws)
	}

	if workspaces == nil {
		workspaces = []WorkspaceListResult{}
	}

	// Get total count
	var total int
	err = s.db.QueryRow(`
		SELECT COUNT(DISTINCT w.id)
		FROM workspaces w
		LEFT JOIN user_workspace_roles uwr ON w.id = uwr.workspace_id AND uwr.user_id = ?
		LEFT JOIN (
			SELECT DISTINCT gwr.workspace_id
			FROM group_workspace_roles gwr
			JOIN group_members gm ON gwr.group_id = gm.group_id
			WHERE gm.user_id = ?
		) grp ON w.id = grp.workspace_id
		WHERE w.active = 1
		   OR (w.active = 0 AND uwr.role_id IS NOT NULL)
		   OR (w.active = 0 AND grp.workspace_id IS NOT NULL)
		   OR (w.is_personal = 1 AND w.owner_id = ?)
	`, params.UserID, params.UserID, params.UserID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count workspaces: %w", err)
	}

	return workspaces, total, nil
}

// GetByID retrieves a workspace by ID with minimal fields.
func (s *WorkspaceService) GetByID(id int) (*WorkspaceListResult, error) {
	var ws WorkspaceListResult
	var icon, color sql.NullString
	err := s.db.QueryRow(`
		SELECT id, name, key, description, active, is_personal, icon, color, created_at, updated_at
		FROM workspaces WHERE id = ?
	`, id).Scan(&ws.ID, &ws.Name, &ws.Key, &ws.Description, &ws.Active, &ws.IsPersonal,
		&icon, &color, &ws.CreatedAt, &ws.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("workspace not found: %d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}

	ws.Icon = icon.String
	ws.Color = color.String

	return &ws, nil
}

// CreateWorkspaceParams contains the parameters for creating a workspace.
type CreateWorkspaceParams struct {
	Name        string
	Key         string
	Description string
	Icon        string
	Color       string
	CreatorID   int
}

// CreateWorkspaceResult contains the result of creating a workspace.
type CreateWorkspaceResult struct {
	Workspace *WorkspaceListResult
}

// Create creates a new workspace and grants admin permission to the creator.
func (s *WorkspaceService) Create(params CreateWorkspaceParams) (*CreateWorkspaceResult, error) {
	// Normalize key to uppercase
	key := strings.ToUpper(params.Key)

	// Check for duplicate key
	exists, err := s.repo.KeyExists(key)
	if err != nil {
		return nil, fmt.Errorf("failed to check key existence: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("workspace key already exists: %s", key)
	}

	// Create workspace
	result, err := s.db.ExecWrite(`
		INSERT INTO workspaces (name, key, description, icon, color, active)
		VALUES (?, ?, ?, ?, ?, 1)
	`, params.Name, key, params.Description, params.Icon, params.Color)
	if err != nil {
		return nil, fmt.Errorf("failed to create workspace: %w", err)
	}

	id, _ := result.LastInsertId()

	// Grant admin permission to creator
	_, err = s.db.ExecWrite(`
		INSERT INTO user_workspace_roles (workspace_id, user_id, role_id, granted_by, granted_at)
		SELECT ?, ?, id, ?, CURRENT_TIMESTAMP FROM workspace_roles WHERE name = 'Administrator'
	`, id, params.CreatorID, params.CreatorID)
	if err != nil {
		slog.Warn("failed to grant admin permission to workspace creator", "error", err, "workspace_id", id)
	}

	// Return created workspace
	ws, err := s.GetByID(int(id))
	if err != nil {
		return nil, fmt.Errorf("workspace created but failed to retrieve: %w", err)
	}

	return &CreateWorkspaceResult{Workspace: ws}, nil
}

// UpdateWorkspaceParams contains the parameters for updating a workspace.
type UpdateWorkspaceParams struct {
	ID          int
	Name        *string
	Description *string
	Active      *bool
	Icon        *string
	Color       *string
}

// Update updates an existing workspace.
func (s *WorkspaceService) Update(params UpdateWorkspaceParams) (*WorkspaceListResult, error) {
	// Load existing workspace
	var ws struct {
		ID          int
		Name        string
		Description string
		Active      bool
		Icon        sql.NullString
		Color       sql.NullString
	}
	err := s.db.QueryRow("SELECT id, name, description, active, icon, color FROM workspaces WHERE id = ?", params.ID).
		Scan(&ws.ID, &ws.Name, &ws.Description, &ws.Active, &ws.Icon, &ws.Color)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("workspace not found: %d", params.ID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load workspace: %w", err)
	}

	// Apply updates
	if params.Name != nil {
		ws.Name = *params.Name
	}
	if params.Description != nil {
		ws.Description = *params.Description
	}
	if params.Active != nil {
		ws.Active = *params.Active
	}
	if params.Icon != nil {
		ws.Icon = sql.NullString{String: *params.Icon, Valid: true}
	}
	if params.Color != nil {
		ws.Color = sql.NullString{String: *params.Color, Valid: true}
	}

	_, err = s.db.ExecWrite(`
		UPDATE workspaces SET name = ?, description = ?, active = ?, icon = ?, color = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, ws.Name, ws.Description, ws.Active, ws.Icon.String, ws.Color.String, params.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to update workspace: %w", err)
	}

	// Return updated workspace
	return s.GetByID(params.ID)
}

// Delete removes a workspace by ID.
func (s *WorkspaceService) Delete(id int) error {
	// Check workspace exists
	exists, err := s.repo.Exists(id)
	if err != nil {
		return fmt.Errorf("failed to check workspace existence: %w", err)
	}
	if !exists {
		return fmt.Errorf("workspace not found: %d", id)
	}

	// Delete workspace (cascade will handle related records)
	err = s.repo.Delete(id)
	if err != nil {
		return fmt.Errorf("failed to delete workspace: %w", err)
	}

	return nil
}

// Exists checks if a workspace exists.
func (s *WorkspaceService) Exists(id int) (bool, error) {
	return s.repo.Exists(id)
}

// KeyExists checks if a workspace key exists.
func (s *WorkspaceService) KeyExists(key string) (bool, error) {
	return s.repo.KeyExists(strings.ToUpper(key))
}

// GetStatuses retrieves statuses available for a workspace via its configuration set.
func (s *WorkspaceService) GetStatuses(workspaceID int) ([]models.Status, error) {
	rows, err := s.db.Query(`
		SELECT DISTINCT s.id, s.name, s.description, s.category_id, s.is_default,
		       sc.name as category_name, sc.color as category_color, sc.is_completed
		FROM statuses s
		JOIN status_categories sc ON s.category_id = sc.id
		LEFT JOIN workflow_transitions wt ON s.id = wt.from_status_id OR s.id = wt.to_status_id
		LEFT JOIN workflows wf ON wt.workflow_id = wf.id
		LEFT JOIN configuration_sets cs ON wf.id = cs.workflow_id
		LEFT JOIN workspace_configuration_sets wcs ON cs.id = wcs.configuration_set_id
		WHERE wcs.workspace_id = ? OR wcs.workspace_id IS NULL
		ORDER BY sc.id, s.name
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace statuses: %w", err)
	}
	defer rows.Close()

	var statuses []models.Status
	for rows.Next() {
		var status models.Status
		var description sql.NullString
		err := rows.Scan(
			&status.ID, &status.Name, &description, &status.CategoryID, &status.IsDefault,
			&status.CategoryName, &status.CategoryColor, &status.IsCompleted,
		)
		if err != nil {
			continue
		}
		status.Description = description.String
		statuses = append(statuses, status)
	}

	if statuses == nil {
		statuses = []models.Status{}
	}

	return statuses, nil
}

// GetRepository returns the underlying workspace repository for advanced operations.
func (s *WorkspaceService) GetRepository() *repository.WorkspaceRepository {
	return s.repo
}
