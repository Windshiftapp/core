package handlers

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/services"
)

type ProjectHandler struct {
	db                database.Database
	permissionService *services.PermissionService
}

func NewProjectHandler(db database.Database, permissionService *services.PermissionService) *ProjectHandler {
	return &ProjectHandler{
		db:                db,
		permissionService: permissionService,
	}
}

// Helper function to load milestone categories for a project
func (h *ProjectHandler) loadMilestoneCategories(projectID int) ([]int, error) {
	var categories []int
	rows, err := h.db.Query(`
		SELECT category_id FROM project_milestone_categories WHERE project_id = ?
	`, projectID)
	if err != nil {
		return categories, err
	}
	defer rows.Close()
	
	for rows.Next() {
		var categoryID int
		if err := rows.Scan(&categoryID); err != nil {
			return categories, err
		}
		categories = append(categories, categoryID)
	}
	return categories, nil
}

// Helper function to save milestone categories for a project
func (h *ProjectHandler) saveMilestoneCategories(projectID int, categories []int) error {
	// Start transaction
	tx, err := h.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	
	// Delete existing associations
	_, err = tx.Exec("DELETE FROM project_milestone_categories WHERE project_id = ?", projectID)
	if err != nil {
		return err
	}
	
	// Insert new associations
	for _, categoryID := range categories {
		_, err = tx.Exec(`
			INSERT INTO project_milestone_categories (project_id, category_id) VALUES (?, ?)
		`, projectID, categoryID)
		if err != nil {
			return err
		}
	}
	
	return tx.Commit()
}

func (h *ProjectHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	// Require authentication
	user := h.getUserFromContext(r)
	if user == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	query := `
		SELECT p.id, p.workspace_id, p.name, p.description, p.active, p.created_at, p.updated_at,
		       w.name as workspace_name
		FROM projects p
		LEFT JOIN workspaces w ON p.workspace_id = w.id
		WHERE 1=1`

	args := []interface{}{}

	// Filter by workspace if specified
	if workspaceID := r.URL.Query().Get("workspace_id"); workspaceID != "" {
		query += " AND p.workspace_id = ?"
		args = append(args, workspaceID)
	}

	query += " ORDER BY p.name"

	rows, err := h.db.Query(query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var projects []models.Project
	for rows.Next() {
		var project models.Project
		var workspaceName sql.NullString

		err := rows.Scan(&project.ID, &project.WorkspaceID, &project.Name, &project.Description,
			&project.Active, &project.CreatedAt, &project.UpdatedAt, &workspaceName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Check if user has permission to view projects in this workspace
		if project.WorkspaceID != nil {
			canView, err := h.canViewProject(user.ID, *project.WorkspaceID)
			if err != nil {
				http.Error(w, "Permission check failed: "+err.Error(), http.StatusInternalServerError)
				return
			}
			if !canView {
				continue // Skip projects user doesn't have permission to view
			}
		}

		if workspaceName.Valid {
			project.WorkspaceName = workspaceName.String
		}

		// Load milestone categories
		categories, err := h.loadMilestoneCategories(project.ID)
		if err != nil {
			slog.Warn("failed to load milestone categories", slog.String("component", "projects"), slog.Int("project_id", project.ID), slog.Any("error", err))
		}
		project.MilestoneCategories = categories

		projects = append(projects, project)
	}

	respondJSONOK(w, projects)
}

func (h *ProjectHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Require authentication
	user := h.getUserFromContext(r)
	if user == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	var project models.Project
	var workspaceName sql.NullString

	err := h.db.QueryRow(`
		SELECT p.id, p.workspace_id, p.name, p.description, p.active, p.created_at, p.updated_at,
		       w.name as workspace_name
		FROM projects p
		LEFT JOIN workspaces w ON p.workspace_id = w.id
		WHERE p.id = ?
	`, id).Scan(&project.ID, &project.WorkspaceID, &project.Name, &project.Description,
		&project.Active, &project.CreatedAt, &project.UpdatedAt, &workspaceName)

	if err == sql.ErrNoRows {
		http.Error(w, "Project not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check if user has permission to view projects in this workspace
	if project.WorkspaceID != nil {
		canView, err := h.canViewProject(user.ID, *project.WorkspaceID)
		if err != nil {
			http.Error(w, "Permission check failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if !canView {
			http.Error(w, "Insufficient permissions to view this project", http.StatusForbidden)
			return
		}
	}

	if workspaceName.Valid {
		project.WorkspaceName = workspaceName.String
	}

	// Load milestone categories
	categories, err := h.loadMilestoneCategories(project.ID)
	if err != nil {
		slog.Warn("failed to load milestone categories", slog.String("component", "projects"), slog.Int("project_id", project.ID), slog.Any("error", err))
	}
	project.MilestoneCategories = categories

	respondJSONOK(w, project)
}

func (h *ProjectHandler) Create(w http.ResponseWriter, r *http.Request) {
	// Require authentication
	user := h.getUserFromContext(r)
	if user == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	var project models.Project
	if err := json.NewDecoder(r.Body).Decode(&project); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate workspace exists if specified
	if project.WorkspaceID != nil && *project.WorkspaceID != 0 {
		var workspaceExists bool
		err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM workspaces WHERE id = ?)", *project.WorkspaceID).Scan(&workspaceExists)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !workspaceExists {
			http.Error(w, "Workspace not found", http.StatusBadRequest)
			return
		}

		// Check if user has permission to create projects in this workspace
		canCreate, err := h.canCreateProject(user.ID, *project.WorkspaceID)
		if err != nil {
			http.Error(w, "Permission check failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if !canCreate {
			http.Error(w, "Insufficient permissions to create projects in this workspace", http.StatusForbidden)
			return
		}
	}

	now := time.Now()
	var id int64
	err := h.db.QueryRow(`
		INSERT INTO projects (workspace_id, name, description, active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?) RETURNING id
	`, project.WorkspaceID, project.Name, project.Description, project.Active, now, now).Scan(&id)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	// Save milestone categories if provided
	if len(project.MilestoneCategories) > 0 {
		if err := h.saveMilestoneCategories(int(id), project.MilestoneCategories); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	
	// Return the created project with joined data
	var workspaceName sql.NullString
	err = h.db.QueryRow(`
		SELECT p.id, p.workspace_id, p.name, p.description, p.active, p.created_at, p.updated_at,
		       w.name as workspace_name
		FROM projects p
		LEFT JOIN workspaces w ON p.workspace_id = w.id
		WHERE p.id = ?
	`, id).Scan(&project.ID, &project.WorkspaceID, &project.Name, &project.Description, 
		&project.Active, &project.CreatedAt, &project.UpdatedAt, &workspaceName)
	
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	if workspaceName.Valid {
		project.WorkspaceName = workspaceName.String
	}

	// Load the saved categories
	categories, err := h.loadMilestoneCategories(int(id))
	if err != nil {
		slog.Warn("failed to load milestone categories after create", slog.String("component", "projects"), slog.Int64("project_id", id), slog.Any("error", err))
	}
	project.MilestoneCategories = categories

	respondJSONCreated(w, project)
}

func (h *ProjectHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Require authentication
	user := h.getUserFromContext(r)
	if user == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	// Get existing project to check workspace
	var existingWorkspaceID *int
	err := h.db.QueryRow("SELECT workspace_id FROM projects WHERE id = ?", id).Scan(&existingWorkspaceID)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Project not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to fetch project: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Check if user has permission to edit projects in the existing workspace
	if existingWorkspaceID != nil {
		canEdit, err := h.canEditProject(user.ID, *existingWorkspaceID)
		if err != nil {
			http.Error(w, "Permission check failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if !canEdit {
			http.Error(w, "Insufficient permissions to edit this project", http.StatusForbidden)
			return
		}
	}

	var project models.Project
	if err := json.NewDecoder(r.Body).Decode(&project); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate workspace exists if specified and check permission for new workspace if changing
	if project.WorkspaceID != nil && *project.WorkspaceID != 0 {
		var workspaceExists bool
		err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM workspaces WHERE id = ?)", *project.WorkspaceID).Scan(&workspaceExists)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !workspaceExists {
			http.Error(w, "Workspace not found", http.StatusBadRequest)
			return
		}

		// If moving to a different workspace, check permission for new workspace
		if existingWorkspaceID == nil || *project.WorkspaceID != *existingWorkspaceID {
			canEdit, err := h.canEditProject(user.ID, *project.WorkspaceID)
			if err != nil {
				http.Error(w, "Permission check failed: "+err.Error(), http.StatusInternalServerError)
				return
			}
			if !canEdit {
				http.Error(w, "Insufficient permissions to move project to this workspace", http.StatusForbidden)
				return
			}
		}
	}

	now := time.Now()
	_, err = h.db.ExecWrite(`
		UPDATE projects 
		SET workspace_id = ?, name = ?, description = ?, active = ?, updated_at = ?
		WHERE id = ?
	`, project.WorkspaceID, project.Name, project.Description, project.Active, now, id)
	
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	// Update milestone categories
	if err := h.saveMilestoneCategories(id, project.MilestoneCategories); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the updated project with joined data
	var workspaceName sql.NullString
	err = h.db.QueryRow(`
		SELECT p.id, p.workspace_id, p.name, p.description, p.active, p.created_at, p.updated_at,
		       w.name as workspace_name
		FROM projects p
		LEFT JOIN workspaces w ON p.workspace_id = w.id
		WHERE p.id = ?
	`, id).Scan(&project.ID, &project.WorkspaceID, &project.Name, &project.Description, 
		&project.Active, &project.CreatedAt, &project.UpdatedAt, &workspaceName)
	
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	if workspaceName.Valid {
		project.WorkspaceName = workspaceName.String
	}

	// Load milestone categories
	categories, err := h.loadMilestoneCategories(id)
	if err != nil {
		slog.Warn("failed to load milestone categories after update", slog.String("component", "projects"), slog.Int("project_id", id), slog.Any("error", err))
	}
	project.MilestoneCategories = categories

	respondJSONOK(w, project)
}

func (h *ProjectHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Require authentication
	user := h.getUserFromContext(r)
	if user == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	// Get project's workspace_id for permission check
	var workspaceID *int
	err := h.db.QueryRow("SELECT workspace_id FROM projects WHERE id = ?", id).Scan(&workspaceID)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Project not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to fetch project: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Check if user has permission to delete projects in this workspace
	if workspaceID != nil {
		canDelete, permErr := h.canDeleteProject(user.ID, *workspaceID)
		if permErr != nil {
			http.Error(w, "Permission check failed: "+permErr.Error(), http.StatusInternalServerError)
			return
		}
		if !canDelete {
			http.Error(w, "Insufficient permissions to delete this project", http.StatusForbidden)
			return
		}
	}

	_, err = h.db.ExecWrite("DELETE FROM projects WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Permission helper methods

// getUserFromContext extracts the user from the request context
func (h *ProjectHandler) getUserFromContext(r *http.Request) *models.User {
	if user := r.Context().Value("user"); user != nil {
		if u, ok := user.(*models.User); ok {
			return u
		}
	}
	return nil
}

// canViewProject checks if a user can view projects in a specific workspace
func (h *ProjectHandler) canViewProject(userID, workspaceID int) (bool, error) {
	if h.permissionService == nil {
		return true, nil
	}
	return h.permissionService.HasWorkspacePermission(userID, workspaceID, models.PermissionProjectView)
}

// canCreateProject checks if a user can create projects in a specific workspace
func (h *ProjectHandler) canCreateProject(userID, workspaceID int) (bool, error) {
	if h.permissionService == nil {
		return true, nil
	}
	return h.permissionService.HasWorkspacePermission(userID, workspaceID, models.PermissionProjectCreate)
}

// canEditProject checks if a user can edit projects in a specific workspace
func (h *ProjectHandler) canEditProject(userID, workspaceID int) (bool, error) {
	if h.permissionService == nil {
		return true, nil
	}
	return h.permissionService.HasWorkspacePermission(userID, workspaceID, models.PermissionProjectEdit)
}

// canDeleteProject checks if a user can delete projects in a specific workspace
func (h *ProjectHandler) canDeleteProject(userID, workspaceID int) (bool, error) {
	if h.permissionService == nil {
		return true, nil
	}
	return h.permissionService.HasWorkspacePermission(userID, workspaceID, models.PermissionProjectDelete)
}