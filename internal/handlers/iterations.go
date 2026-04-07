package handlers

import (
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
		iterations = append(iterations, iterationResultToModel(&r))
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
		// All authenticated users can view global iterations
	} else if result.WorkspaceID != nil {
		if !RequireWorkspacePermission(w, r, user.ID, *result.WorkspaceID, models.PermissionItemView, h.permissionService) {
			return
		}
	}

	respondJSONOK(w, iterationResultToModel(result))
}

func (h *IterationHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	iteration, ok := decodeJSON[models.Iteration](w, r)
	if !ok {
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
	if !isValidIterationStatus(iteration.Status) {
		iteration.Status = "planned" // Default status
	}

	// Validate global vs workspace constraints
	if !validateIterationConstraints(w, r, iteration.IsGlobal, iteration.WorkspaceID) {
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

	// Validate type_id and workspace_id references
	if !h.validateIterationReferences(w, r, iteration.TypeID, iteration.WorkspaceID) {
		return
	}

	iteration.Name = utils.SanitizeTitle(iteration.Name)
	iteration.Description = utils.SanitizeCommentContent(iteration.Description)

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
	createdIteration := iterationResultToModel(result)

	logAudit(h.db, r, user, logger.ActionIterationCreate, logger.ResourceIteration, &createdIteration.ID, createdIteration.Name)
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

	iteration, ok := decodeJSON[models.Iteration](w, r)
	if !ok {
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
	if !isValidIterationStatus(iteration.Status) {
		respondValidationError(w, r, "Invalid status")
		return
	}

	// Validate global vs workspace constraints
	if !validateIterationConstraints(w, r, iteration.IsGlobal, iteration.WorkspaceID) {
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

	// Validate type_id and workspace_id references
	if !h.validateIterationReferences(w, r, iteration.TypeID, iteration.WorkspaceID) {
		return
	}

	iteration.Name = utils.SanitizeTitle(iteration.Name)
	iteration.Description = utils.SanitizeCommentContent(iteration.Description)

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
	updatedIteration := iterationResultToModel(result)

	logAudit(h.db, r, user, logger.ActionIterationUpdate, logger.ResourceIteration, &updatedIteration.ID, updatedIteration.Name)
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

	logAudit(h.db, r, user, logger.ActionIterationDelete, logger.ResourceIteration, &id, "")
	w.WriteHeader(http.StatusNoContent)
}

// requireIterationAccess authenticates the user, parses the iteration ID,
// and checks global or workspace-scoped permission. Returns false if any check fails.
func (h *IterationHandler) requireIterationAccess(w http.ResponseWriter, r *http.Request) (iterationID int, ok bool) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return 0, false
	}

	iterationID, ok = requireIDParam(w, r, "id")
	if !ok {
		return 0, false
	}

	isGlobal, wsID, err := h.planningService.IsIterationGlobal(iterationID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			respondNotFound(w, r, "iteration")
			return 0, false
		}
		respondInternalError(w, r, err)
		return 0, false
	}

	if isGlobal {
		// All authenticated users can view global iteration progress/burndown
		return iterationID, true
	} else if wsID != nil {
		if !RequireWorkspacePermission(w, r, user.ID, *wsID, models.PermissionItemView, h.permissionService) {
			return 0, false
		}
	}

	return iterationID, true
}

// GetProgress handles GET /api/iterations/{id}/progress - returns iteration progress report
func (h *IterationHandler) GetProgress(w http.ResponseWriter, r *http.Request) {
	iterationID, ok := h.requireIterationAccess(w, r)
	if !ok {
		return
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
	iterationID, ok := h.requireIterationAccess(w, r)
	if !ok {
		return
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

func isValidIterationStatus(status string) bool {
	//nolint:misspell // British spelling used in database
	for _, s := range []string{"planned", "active", "completed", "cancelled"} {
		if status == s {
			return true
		}
	}
	return false
}

func iterationResultToModel(r *services.IterationResult) models.Iteration {
	return models.Iteration{
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
}

func validateIterationConstraints(w http.ResponseWriter, r *http.Request, isGlobal bool, workspaceID *int) bool {
	if isGlobal && workspaceID != nil {
		respondValidationError(w, r, "Global iterations cannot have a workspace_id")
		return false
	}
	if !isGlobal && workspaceID == nil {
		respondValidationError(w, r, "Local iterations must have a workspace_id")
		return false
	}
	return true
}

func (h *IterationHandler) validateIterationReferences(w http.ResponseWriter, r *http.Request, typeID, workspaceID *int) bool {
	if typeID != nil {
		exists, err := h.planningService.IterationTypeExists(*typeID)
		if err != nil {
			respondInternalError(w, r, err)
			return false
		}
		if !exists {
			respondValidationError(w, r, "Invalid iteration type ID")
			return false
		}
	}
	if workspaceID != nil {
		exists, err := h.planningService.WorkspaceExists(*workspaceID)
		if err != nil {
			respondInternalError(w, r, err)
			return false
		}
		if !exists {
			respondValidationError(w, r, "Invalid workspace ID")
			return false
		}
	}
	return true
}
