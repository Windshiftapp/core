package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"windshift/internal/database"
	"windshift/internal/middleware"
	"windshift/internal/models"
	"windshift/internal/services"
)

type ProjectHandler struct {
	db                database.Database
	permissionService *services.PermissionService
	planningService   *services.PlanningService
}

func NewProjectHandler(db database.Database, permissionService *services.PermissionService) *ProjectHandler {
	return &ProjectHandler{
		db:                db,
		permissionService: permissionService,
		planningService:   services.NewPlanningService(db),
	}
}

func (h *ProjectHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	// Require authentication
	user := h.getUserFromContext(r)
	if user == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	// Build service params
	params := services.ProjectListParams{
		Limit:  1000, // Large limit for backwards compatibility
		Offset: 0,
	}

	// Parse workspace ID filter
	if workspaceIDStr := r.URL.Query().Get("workspace_id"); workspaceIDStr != "" {
		if wsID, err := strconv.Atoi(workspaceIDStr); err == nil {
			params.WorkspaceID = &wsID
		}
	}

	// Use service to list projects
	results, _, err := h.planningService.ListProjects(params)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Convert service results to models and filter by permission
	var projects []models.Project
	for _, r := range results {
		// Check if user has permission to view projects in this workspace
		if r.WorkspaceID != nil {
			canView, err := h.canViewProject(user.ID, *r.WorkspaceID)
			if err != nil {
				http.Error(w, "Permission check failed: "+err.Error(), http.StatusInternalServerError)
				return
			}
			if !canView {
				continue // Skip projects user doesn't have permission to view
			}
		}

		project := models.Project{
			ID:            r.ID,
			Name:          r.Name,
			Description:   r.Description,
			Active:        r.Active,
			WorkspaceID:   r.WorkspaceID,
			WorkspaceName: r.WorkspaceName,
			CreatedAt:     r.CreatedAt,
			UpdatedAt:     r.UpdatedAt,
		}

		// Load milestone categories
		categories, err := h.planningService.LoadProjectMilestoneCategories(project.ID)
		if err != nil {
			slog.Warn("failed to load milestone categories", slog.String("component", "projects"), slog.Int("project_id", project.ID), slog.Any("error", err))
		}
		project.MilestoneCategories = categories

		projects = append(projects, project)
	}

	if projects == nil {
		projects = []models.Project{}
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

	// Use service to get project
	result, err := h.planningService.GetProject(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "Project not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check if user has permission to view projects in this workspace
	if result.WorkspaceID != nil {
		canView, err := h.canViewProject(user.ID, *result.WorkspaceID)
		if err != nil {
			http.Error(w, "Permission check failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if !canView {
			http.Error(w, "Insufficient permissions to view this project", http.StatusForbidden)
			return
		}
	}

	// Convert service result to model for response
	project := models.Project{
		ID:            result.ID,
		Name:          result.Name,
		Description:   result.Description,
		Active:        result.Active,
		WorkspaceID:   result.WorkspaceID,
		WorkspaceName: result.WorkspaceName,
		CreatedAt:     result.CreatedAt,
		UpdatedAt:     result.UpdatedAt,
	}

	// Load milestone categories
	categories, err := h.planningService.LoadProjectMilestoneCategories(project.ID)
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
		exists, err := h.planningService.WorkspaceExists(*project.WorkspaceID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !exists {
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

	// Use service to create project
	result, err := h.planningService.CreateProject(services.CreateProjectParams{
		Name:        project.Name,
		Description: project.Description,
		WorkspaceID: project.WorkspaceID,
		Active:      project.Active,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Save milestone categories if provided
	if len(project.MilestoneCategories) > 0 {
		if err := h.planningService.SaveProjectMilestoneCategories(result.ID, project.MilestoneCategories); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Convert service result to model for response
	createdProject := models.Project{
		ID:            result.ID,
		Name:          result.Name,
		Description:   result.Description,
		Active:        result.Active,
		WorkspaceID:   result.WorkspaceID,
		WorkspaceName: result.WorkspaceName,
		CreatedAt:     result.CreatedAt,
		UpdatedAt:     result.UpdatedAt,
	}

	// Load the saved categories
	categories, err := h.planningService.LoadProjectMilestoneCategories(result.ID)
	if err != nil {
		slog.Warn("failed to load milestone categories after create", slog.String("component", "projects"), slog.Int("project_id", result.ID), slog.Any("error", err))
	}
	createdProject.MilestoneCategories = categories

	respondJSONCreated(w, createdProject)
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

	// Get existing project's workspace_id for permission check
	existingWorkspaceID, err := h.planningService.GetProjectWorkspaceID(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
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
		exists, err := h.planningService.WorkspaceExists(*project.WorkspaceID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !exists {
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

	// Use service to update project
	result, err := h.planningService.UpdateProject(services.UpdateProjectParams{
		ID:          id,
		Name:        project.Name,
		Description: project.Description,
		WorkspaceID: project.WorkspaceID,
		Active:      project.Active,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Update milestone categories
	if err := h.planningService.SaveProjectMilestoneCategories(id, project.MilestoneCategories); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Convert service result to model for response
	updatedProject := models.Project{
		ID:            result.ID,
		Name:          result.Name,
		Description:   result.Description,
		Active:        result.Active,
		WorkspaceID:   result.WorkspaceID,
		WorkspaceName: result.WorkspaceName,
		CreatedAt:     result.CreatedAt,
		UpdatedAt:     result.UpdatedAt,
	}

	// Load milestone categories
	categories, err := h.planningService.LoadProjectMilestoneCategories(id)
	if err != nil {
		slog.Warn("failed to load milestone categories after update", slog.String("component", "projects"), slog.Int("project_id", id), slog.Any("error", err))
	}
	updatedProject.MilestoneCategories = categories

	respondJSONOK(w, updatedProject)
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
	workspaceID, err := h.planningService.GetProjectWorkspaceID(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
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

	// Use service to delete project
	if err := h.planningService.DeleteProject(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Permission helper methods

// getUserFromContext extracts the user from the request context
func (h *ProjectHandler) getUserFromContext(r *http.Request) *models.User {
	if user := r.Context().Value(middleware.ContextKeyUser); user != nil {
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
