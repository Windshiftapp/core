package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/services"
)

type IterationHandler struct {
	db                database.Database
	permissionService *services.PermissionService
	planningService   *services.PlanningService
}

func NewIterationHandler(db database.Database, permissionService *services.PermissionService) *IterationHandler {
	return &IterationHandler{
		db:                db,
		permissionService: permissionService,
		planningService:   services.NewPlanningService(db),
	}
}

func (h *IterationHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Parse query parameters
	workspaceIDStr := r.URL.Query().Get("workspace_id")
	typeIDStr := r.URL.Query().Get("type_id")
	status := r.URL.Query().Get("status")
	includeGlobal := r.URL.Query().Get("include_global") != "false" // Default to true

	// Check workspace permission if workspace_id is specified
	if workspaceIDStr != "" {
		if wsID, err := strconv.Atoi(workspaceIDStr); err == nil {
			if !RequireWorkspacePermission(w, user.ID, wsID, models.PermissionItemView, h.permissionService) {
				return
			}
		}
	} else {
		// For global-only iterations, check global iteration permission
		hasGlobalPerm, err := h.permissionService.HasGlobalPermission(user.ID, models.PermissionIterationManage)
		if err != nil || !hasGlobalPerm {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
	}

	// Build service params
	params := services.IterationListParams{
		Limit:         1000, // Large limit for backwards compatibility
		Offset:        0,
		IncludeGlobal: includeGlobal,
		Status:        status,
	}

	// Parse workspace ID
	if workspaceIDStr != "" {
		if wsID, err := strconv.Atoi(workspaceIDStr); err == nil {
			params.WorkspaceID = &wsID
		}
	}

	// Parse type ID
	if typeIDStr != "" {
		if typeIDStr == "null" || typeIDStr == "0" {
			zero := 0
			params.TypeID = &zero
		} else if typeID, err := strconv.Atoi(typeIDStr); err == nil {
			params.TypeID = &typeID
		}
	}

	// Use service to list iterations
	results, _, err := h.planningService.ListIterations(params)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Convert service results to models for response
	iterations := make([]models.Iteration, 0, len(results))
	for _, r := range results {
		iteration := models.Iteration{
			ID:            r.ID,
			Name:          r.Name,
			Description:   r.Description,
			StartDate:     r.StartDate,
			EndDate:       r.EndDate,
			Status:        r.Status,
			TypeID:        r.TypeID,
			TypeName:      r.TypeName,
			TypeColor:     r.TypeColor,
			IsGlobal:      r.IsGlobal,
			WorkspaceID:   r.WorkspaceID,
			WorkspaceName: r.WorkspaceName,
			CreatedAt:     r.CreatedAt,
			UpdatedAt:     r.UpdatedAt,
		}
		iterations = append(iterations, iteration)
	}

	respondJSONOK(w, iterations)
}

func (h *IterationHandler) Get(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Use service to get iteration
	result, err := h.planningService.GetIteration(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "Iteration not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check permission based on whether iteration is global or workspace-scoped
	if result.IsGlobal {
		hasGlobalPerm, err := h.permissionService.HasGlobalPermission(user.ID, models.PermissionIterationManage)
		if err != nil || !hasGlobalPerm {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
	} else if result.WorkspaceID != nil {
		if !RequireWorkspacePermission(w, user.ID, *result.WorkspaceID, models.PermissionItemView, h.permissionService) {
			return
		}
	}

	// Convert service result to model for response
	iteration := models.Iteration{
		ID:            result.ID,
		Name:          result.Name,
		Description:   result.Description,
		StartDate:     result.StartDate,
		EndDate:       result.EndDate,
		Status:        result.Status,
		TypeID:        result.TypeID,
		TypeName:      result.TypeName,
		TypeColor:     result.TypeColor,
		IsGlobal:      result.IsGlobal,
		WorkspaceID:   result.WorkspaceID,
		WorkspaceName: result.WorkspaceName,
		CreatedAt:     result.CreatedAt,
		UpdatedAt:     result.UpdatedAt,
	}

	respondJSONOK(w, iteration)
}

func (h *IterationHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	var iteration models.Iteration
	if err := json.NewDecoder(r.Body).Decode(&iteration); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if strings.TrimSpace(iteration.Name) == "" {
		http.Error(w, "Iteration name is required", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(iteration.StartDate) == "" {
		http.Error(w, "Start date is required", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(iteration.EndDate) == "" {
		http.Error(w, "End date is required", http.StatusBadRequest)
		return
	}

	// Validate status
	validStatuses := []string{"planned", "active", "completed", "cancelled"}
	statusValid := false
	for _, validStatus := range validStatuses {
		if iteration.Status == validStatus {
			statusValid = true
			break
		}
	}
	if !statusValid {
		iteration.Status = "planned" // Default status
	}

	// Validate global vs workspace constraints
	if iteration.IsGlobal && iteration.WorkspaceID != nil {
		http.Error(w, "Global iterations cannot have a workspace_id", http.StatusBadRequest)
		return
	}
	if !iteration.IsGlobal && iteration.WorkspaceID == nil {
		http.Error(w, "Local iterations must have a workspace_id", http.StatusBadRequest)
		return
	}

	// Check permission based on whether iteration is global or workspace-scoped
	if iteration.IsGlobal {
		hasGlobalPerm, err := h.permissionService.HasGlobalPermission(user.ID, models.PermissionIterationManage)
		if err != nil || !hasGlobalPerm {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
	} else {
		if !RequireWorkspacePermission(w, user.ID, *iteration.WorkspaceID, models.PermissionItemEdit, h.permissionService) {
			return
		}
	}

	// Validate type_id if provided (using service)
	if iteration.TypeID != nil {
		exists, err := h.planningService.IterationTypeExists(*iteration.TypeID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !exists {
			http.Error(w, "Invalid iteration type ID", http.StatusBadRequest)
			return
		}
	}

	// Validate workspace_id if provided (using service)
	if iteration.WorkspaceID != nil {
		exists, err := h.planningService.WorkspaceExists(*iteration.WorkspaceID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !exists {
			http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
			return
		}
	}

	// Use service to create iteration
	result, err := h.planningService.CreateIteration(services.CreateIterationParams{
		Name:        iteration.Name,
		Description: iteration.Description,
		StartDate:   iteration.StartDate,
		EndDate:     iteration.EndDate,
		Status:      iteration.Status,
		TypeID:      iteration.TypeID,
		IsGlobal:    iteration.IsGlobal,
		WorkspaceID: iteration.WorkspaceID,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Convert service result to model for response
	createdIteration := models.Iteration{
		ID:            result.ID,
		Name:          result.Name,
		Description:   result.Description,
		StartDate:     result.StartDate,
		EndDate:       result.EndDate,
		Status:        result.Status,
		TypeID:        result.TypeID,
		TypeName:      result.TypeName,
		TypeColor:     result.TypeColor,
		IsGlobal:      result.IsGlobal,
		WorkspaceID:   result.WorkspaceID,
		WorkspaceName: result.WorkspaceName,
		CreatedAt:     result.CreatedAt,
		UpdatedAt:     result.UpdatedAt,
	}

	respondJSONCreated(w, createdIteration)
}

func (h *IterationHandler) Update(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var iteration models.Iteration
	if err := json.NewDecoder(r.Body).Decode(&iteration); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if strings.TrimSpace(iteration.Name) == "" {
		http.Error(w, "Iteration name is required", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(iteration.StartDate) == "" {
		http.Error(w, "Start date is required", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(iteration.EndDate) == "" {
		http.Error(w, "End date is required", http.StatusBadRequest)
		return
	}

	// Validate status
	validStatuses := []string{"planned", "active", "completed", "cancelled"}
	statusValid := false
	for _, validStatus := range validStatuses {
		if iteration.Status == validStatus {
			statusValid = true
			break
		}
	}
	if !statusValid {
		http.Error(w, "Invalid status", http.StatusBadRequest)
		return
	}

	// Validate global vs workspace constraints
	if iteration.IsGlobal && iteration.WorkspaceID != nil {
		http.Error(w, "Global iterations cannot have a workspace_id", http.StatusBadRequest)
		return
	}
	if !iteration.IsGlobal && iteration.WorkspaceID == nil {
		http.Error(w, "Local iterations must have a workspace_id", http.StatusBadRequest)
		return
	}

	// Check permission based on whether iteration is global or workspace-scoped
	if iteration.IsGlobal {
		hasGlobalPerm, err := h.permissionService.HasGlobalPermission(user.ID, models.PermissionIterationManage)
		if err != nil || !hasGlobalPerm {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
	} else {
		if !RequireWorkspacePermission(w, user.ID, *iteration.WorkspaceID, models.PermissionItemEdit, h.permissionService) {
			return
		}
	}

	// Validate type_id if provided (using service)
	if iteration.TypeID != nil {
		exists, err := h.planningService.IterationTypeExists(*iteration.TypeID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !exists {
			http.Error(w, "Invalid iteration type ID", http.StatusBadRequest)
			return
		}
	}

	// Validate workspace_id if provided (using service)
	if iteration.WorkspaceID != nil {
		exists, err := h.planningService.WorkspaceExists(*iteration.WorkspaceID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !exists {
			http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
			return
		}
	}

	// Use service to update iteration
	result, err := h.planningService.UpdateIteration(services.UpdateIterationParams{
		ID:          id,
		Name:        iteration.Name,
		Description: iteration.Description,
		StartDate:   iteration.StartDate,
		EndDate:     iteration.EndDate,
		Status:      iteration.Status,
		TypeID:      iteration.TypeID,
		IsGlobal:    iteration.IsGlobal,
		WorkspaceID: iteration.WorkspaceID,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Convert service result to model for response
	updatedIteration := models.Iteration{
		ID:            result.ID,
		Name:          result.Name,
		Description:   result.Description,
		StartDate:     result.StartDate,
		EndDate:       result.EndDate,
		Status:        result.Status,
		TypeID:        result.TypeID,
		TypeName:      result.TypeName,
		TypeColor:     result.TypeColor,
		IsGlobal:      result.IsGlobal,
		WorkspaceID:   result.WorkspaceID,
		WorkspaceName: result.WorkspaceName,
		CreatedAt:     result.CreatedAt,
		UpdatedAt:     result.UpdatedAt,
	}

	respondJSONOK(w, updatedIteration)
}

func (h *IterationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// First, fetch the iteration to check its properties for permission validation (using service)
	isGlobal, wsID, err := h.planningService.IsIterationGlobal(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "Iteration not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check permission based on whether iteration is global or workspace-scoped
	if isGlobal {
		hasGlobalPerm, err := h.permissionService.HasGlobalPermission(user.ID, models.PermissionIterationManage)
		if err != nil || !hasGlobalPerm {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
	} else if wsID != nil {
		if !RequireWorkspacePermission(w, user.ID, *wsID, models.PermissionItemEdit, h.permissionService) {
			return
		}
	}

	// Use service to delete iteration
	if err := h.planningService.DeleteIteration(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetProgress handles GET /api/iterations/{id}/progress - returns iteration progress report
func (h *IterationHandler) GetProgress(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	iterationID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// First check permission for this iteration (using service)
	isGlobal, wsID, err := h.planningService.IsIterationGlobal(iterationID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "Iteration not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check permission based on whether iteration is global or workspace-scoped
	if isGlobal {
		hasGlobalPerm, err := h.permissionService.HasGlobalPermission(user.ID, models.PermissionIterationManage)
		if err != nil || !hasGlobalPerm {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
	} else if wsID != nil {
		if !RequireWorkspacePermission(w, user.ID, *wsID, models.PermissionItemView, h.permissionService) {
			return
		}
	}

	// Use service to get progress report
	report, err := h.planningService.GetIterationProgress(iterationID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "Iteration not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	respondJSONOK(w, report)
}
