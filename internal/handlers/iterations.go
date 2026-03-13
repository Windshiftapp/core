package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"
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
	// Check workspace permission if workspace_id is specified.
	// No workspace_id: allow any authenticated user to list global iterations.
	// Write operations (create/update/delete) still require PermissionIterationManage.
	if workspaceIDStr != "" {
		if wsID, err := strconv.Atoi(workspaceIDStr); err == nil {
			if !RequireWorkspacePermission(w, r, user.ID, wsID, models.PermissionItemView, h.permissionService) {
				return
			}
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
		respondInternalError(w, r, err)
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
			respondNotFound(w, r, "iteration")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Check permission based on whether iteration is global or workspace-scoped
	if result.IsGlobal {
		hasGlobalPerm, err := h.permissionService.HasGlobalPermission(user.ID, models.PermissionIterationManage)
		if err != nil || !hasGlobalPerm {
			respondForbidden(w, r)
			return
		}
	} else if result.WorkspaceID != nil {
		if !RequireWorkspacePermission(w, r, user.ID, *result.WorkspaceID, models.PermissionItemView, h.permissionService) {
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
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Validate required fields
	if strings.TrimSpace(iteration.Name) == "" {
		respondValidationError(w, r, "Iteration name is required")
		return
	}

	if strings.TrimSpace(iteration.StartDate) == "" {
		respondValidationError(w, r, "Start date is required")
		return
	}

	if strings.TrimSpace(iteration.EndDate) == "" {
		respondValidationError(w, r, "End date is required")
		return
	}

	// Validate status
	validStatuses := []string{"planned", "active", "completed", "cancelled"} //nolint:misspell // British spelling used in database
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
		respondValidationError(w, r, "Global iterations cannot have a workspace_id")
		return
	}
	if !iteration.IsGlobal && iteration.WorkspaceID == nil {
		respondValidationError(w, r, "Local iterations must have a workspace_id")
		return
	}

	// Check permission based on whether iteration is global or workspace-scoped
	if iteration.IsGlobal {
		hasGlobalPerm, err := h.permissionService.HasGlobalPermission(user.ID, models.PermissionIterationManage)
		if err != nil || !hasGlobalPerm {
			respondForbidden(w, r)
			return
		}
	} else if !RequireWorkspacePermission(w, r, user.ID, *iteration.WorkspaceID, models.PermissionItemEdit, h.permissionService) {
		return
	}

	// Validate type_id if provided (using service)
	if iteration.TypeID != nil {
		exists, err := h.planningService.IterationTypeExists(*iteration.TypeID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !exists {
			respondValidationError(w, r, "Invalid iteration type ID")
			return
		}
	}

	// Validate workspace_id if provided (using service)
	if iteration.WorkspaceID != nil {
		exists, err := h.planningService.WorkspaceExists(*iteration.WorkspaceID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !exists {
			respondValidationError(w, r, "Invalid workspace ID")
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
		respondInternalError(w, r, err)
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

	_ = logger.LogAudit(h.db, logger.AuditEvent{
		UserID:       user.ID,
		Username:     user.Username,
		IPAddress:    utils.GetClientIP(r),
		UserAgent:    r.UserAgent(),
		ActionType:   logger.ActionIterationCreate,
		ResourceType: logger.ResourceIteration,
		ResourceID:   &createdIteration.ID,
		ResourceName: createdIteration.Name,
		Success:      true,
	})
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
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Fetch existing iteration and merge to support partial updates
	existing, err := h.planningService.GetIteration(id)
	if err != nil {
		respondNotFound(w, r, "iteration")
		return
	}
	if iteration.Name == "" {
		iteration.Name = existing.Name
	}
	if iteration.StartDate == "" {
		iteration.StartDate = existing.StartDate
	}
	if iteration.EndDate == "" {
		iteration.EndDate = existing.EndDate
	}
	if iteration.Status == "" {
		iteration.Status = existing.Status
	}
	if iteration.WorkspaceID == nil {
		iteration.WorkspaceID = existing.WorkspaceID
	}
	if iteration.TypeID == nil {
		iteration.TypeID = existing.TypeID
	}
	if !iteration.IsGlobal && iteration.WorkspaceID == nil {
		iteration.IsGlobal = existing.IsGlobal
	}
	if iteration.Description == "" {
		iteration.Description = existing.Description
	}

	// Validate required fields
	if strings.TrimSpace(iteration.Name) == "" {
		respondValidationError(w, r, "Iteration name is required")
		return
	}

	if strings.TrimSpace(iteration.StartDate) == "" {
		respondValidationError(w, r, "Start date is required")
		return
	}

	if strings.TrimSpace(iteration.EndDate) == "" {
		respondValidationError(w, r, "End date is required")
		return
	}

	// Validate status
	validStatuses := []string{"planned", "active", "completed", "cancelled"} //nolint:misspell // British spelling used in database
	statusValid := false
	for _, validStatus := range validStatuses {
		if iteration.Status == validStatus {
			statusValid = true
			break
		}
	}
	if !statusValid {
		respondValidationError(w, r, "Invalid status")
		return
	}

	// Validate global vs workspace constraints
	if iteration.IsGlobal && iteration.WorkspaceID != nil {
		respondValidationError(w, r, "Global iterations cannot have a workspace_id")
		return
	}
	if !iteration.IsGlobal && iteration.WorkspaceID == nil {
		respondValidationError(w, r, "Local iterations must have a workspace_id")
		return
	}

	// Check permission based on whether iteration is global or workspace-scoped
	if iteration.IsGlobal {
		hasGlobalPerm, err := h.permissionService.HasGlobalPermission(user.ID, models.PermissionIterationManage)
		if err != nil || !hasGlobalPerm {
			respondForbidden(w, r)
			return
		}
	} else if !RequireWorkspacePermission(w, r, user.ID, *iteration.WorkspaceID, models.PermissionItemEdit, h.permissionService) {
		return
	}

	// Validate type_id if provided (using service)
	if iteration.TypeID != nil {
		exists, err := h.planningService.IterationTypeExists(*iteration.TypeID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !exists {
			respondValidationError(w, r, "Invalid iteration type ID")
			return
		}
	}

	// Validate workspace_id if provided (using service)
	if iteration.WorkspaceID != nil {
		exists, err := h.planningService.WorkspaceExists(*iteration.WorkspaceID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !exists {
			respondValidationError(w, r, "Invalid workspace ID")
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
		respondInternalError(w, r, err)
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

	_ = logger.LogAudit(h.db, logger.AuditEvent{
		UserID:       user.ID,
		Username:     user.Username,
		IPAddress:    utils.GetClientIP(r),
		UserAgent:    r.UserAgent(),
		ActionType:   logger.ActionIterationUpdate,
		ResourceType: logger.ResourceIteration,
		ResourceID:   &updatedIteration.ID,
		ResourceName: updatedIteration.Name,
		Success:      true,
	})
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
			respondNotFound(w, r, "iteration")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Check permission based on whether iteration is global or workspace-scoped
	if isGlobal {
		hasGlobalPerm, err := h.permissionService.HasGlobalPermission(user.ID, models.PermissionIterationManage)
		if err != nil || !hasGlobalPerm {
			respondForbidden(w, r)
			return
		}
	} else if wsID != nil {
		if !RequireWorkspacePermission(w, r, user.ID, *wsID, models.PermissionItemEdit, h.permissionService) {
			return
		}
	}

	// Use service to delete iteration
	if err := h.planningService.DeleteIteration(id); err != nil {
		respondInternalError(w, r, err)
		return
	}

	_ = logger.LogAudit(h.db, logger.AuditEvent{
		UserID:       user.ID,
		Username:     user.Username,
		IPAddress:    utils.GetClientIP(r),
		UserAgent:    r.UserAgent(),
		ActionType:   logger.ActionIterationDelete,
		ResourceType: logger.ResourceIteration,
		ResourceID:   &id,
		Success:      true,
	})
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
			respondNotFound(w, r, "iteration")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Check permission based on whether iteration is global or workspace-scoped
	if isGlobal {
		var hasGlobalPerm bool
		hasGlobalPerm, err = h.permissionService.HasGlobalPermission(user.ID, models.PermissionIterationManage)
		if err != nil || !hasGlobalPerm {
			respondForbidden(w, r)
			return
		}
	} else if wsID != nil {
		if !RequireWorkspacePermission(w, r, user.ID, *wsID, models.PermissionItemView, h.permissionService) {
			return
		}
	}

	// Use service to get progress report
	report, err := h.planningService.GetIterationProgress(iterationID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			respondNotFound(w, r, "iteration")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, report)
}

// GetBurndown handles GET /api/iterations/{id}/burndown - returns iteration burndown chart data
func (h *IterationHandler) GetBurndown(w http.ResponseWriter, r *http.Request) {
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
			respondNotFound(w, r, "iteration")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Check permission based on whether iteration is global or workspace-scoped
	if isGlobal {
		var hasGlobalPerm bool
		hasGlobalPerm, err = h.permissionService.HasGlobalPermission(user.ID, models.PermissionIterationManage)
		if err != nil || !hasGlobalPerm {
			respondForbidden(w, r)
			return
		}
	} else if wsID != nil {
		if !RequireWorkspacePermission(w, r, user.ID, *wsID, models.PermissionItemView, h.permissionService) {
			return
		}
	}

	// Use service to get burndown data
	burndown, err := h.planningService.GetIterationBurndown(iterationID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			respondNotFound(w, r, "iteration")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, burndown)
}
