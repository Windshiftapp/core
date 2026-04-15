package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/repository/actionutil"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// ActionsHandler handles action automation API endpoints
type ActionsHandler struct {
	db                database.Database
	repo              *repository.ActionRepository
	actionService     *services.ActionService
	permissionService *services.PermissionService
	keyCache          *WorkspaceKeyCache
}

// NewActionsHandler creates a new actions handler
func NewActionsHandler(db database.Database, actionService *services.ActionService, permissionService *services.PermissionService, keyCache *WorkspaceKeyCache) *ActionsHandler {
	return &ActionsHandler{
		db:                db,
		repo:              repository.NewActionRepository(db),
		actionService:     actionService,
		permissionService: permissionService,
		keyCache:          keyCache,
	}
}

// requireWorkspaceAction parses workspace+action IDs and verifies ownership.
func (h *ActionsHandler) requireWorkspaceAction(w http.ResponseWriter, r *http.Request) (workspaceID int, action *models.Action, ok bool) {
	workspaceID, ok = requireWorkspaceIDParam(w, r, h.keyCache, "workspaceId")
	if !ok {
		return 0, nil, false
	}
	actionID, ok := requireIDParam(w, r, "id")
	if !ok {
		return 0, nil, false
	}
	action, ok = h.requireAction(w, r, actionID, workspaceID)
	return workspaceID, action, ok
}

// requireCapability parses capability ID and fetches it.
func (h *ActionsHandler) requireCapability(w http.ResponseWriter, r *http.Request) (*models.ActionCapability, bool) {
	capID, ok := requireIDParam(w, r, "capabilityId")
	if !ok {
		return nil, false
	}
	capability, err := h.repo.GetCapabilityByID(capID)
	if err == repository.ErrNotFound {
		respondNotFound(w, r, "capability")
		return nil, false
	}
	if err != nil {
		respondInternalError(w, r, err)
		return nil, false
	}
	return capability, true
}

// requireAction fetches an action by ID and verifies workspace ownership.
// Returns nil, false if not found or mismatched (error already written).
func (h *ActionsHandler) requireAction(w http.ResponseWriter, r *http.Request, actionID, workspaceID int) (*models.Action, bool) {
	action, err := h.repo.GetByID(actionID)
	if err == repository.ErrNotFound || (err == nil && action.WorkspaceID != workspaceID) {
		respondNotFound(w, r, "action")
		return nil, false
	}
	if err != nil {
		respondInternalError(w, r, err)
		return nil, false
	}
	return action, true
}

// ListActions lists all actions for a workspace
func (h *ActionsHandler) ListActions(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireWorkspaceIDParam(w, r, h.keyCache, "workspaceId")
	if !ok {
		return
	}

	actions, err := h.repo.ListByWorkspace(workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if actions == nil {
		actions = []*models.Action{}
	}

	respondJSONOK(w, actions)
}

// GetAction gets a single action by ID
func (h *ActionsHandler) GetAction(w http.ResponseWriter, r *http.Request) {
	_, action, ok := h.requireWorkspaceAction(w, r)
	if !ok {
		return
	}
	respondJSONOK(w, action)
}

// CreateAction creates a new action
func (h *ActionsHandler) CreateAction(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireWorkspaceIDParam(w, r, h.keyCache, "workspaceId")
	if !ok {
		return
	}

	// Parse request body
	req, ok := decodeJSON[models.CreateActionRequest](w, r)
	if !ok {
		return
	}

	// Validate required fields
	if msg := actionutil.ValidateActionFields(req.Name, string(req.TriggerType)); msg != "" {
		respondValidationError(w, r, msg)
		return
	}

	// Get current user
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Create action
	action := &models.Action{
		WorkspaceID:   workspaceID,
		Name:          req.Name,
		Description:   req.Description,
		IsEnabled:     true,
		TriggerType:   req.TriggerType,
		TriggerConfig: req.TriggerConfig,
		CreatedBy:     &currentUser.ID,
	}

	actionID, err := h.repo.Create(action)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	action.ID = actionID

	// Create nodes and edges if provided
	if flowErr := actionutil.CreateFlowNodesAndEdges[
		models.ActionNode, *models.ActionNode,
		models.ActionEdge, *models.ActionEdge,
	](
		actionID, req.Nodes, req.Edges,
		func(n *models.ActionNode) (int, error) { return h.repo.CreateNode(n) },
		func(e *models.ActionEdge) (int, error) { return h.repo.CreateEdge(e) },
		func() { _ = h.repo.Delete(actionID) },
	); flowErr != nil {
		respondInternalError(w, r, flowErr)
		return
	}

	// Invalidate cache
	if h.actionService != nil {
		h.actionService.InvalidateWorkspaceCache(workspaceID)
	}

	// Fetch the created action with nodes and edges
	createdAction, err := h.repo.GetByID(actionID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	logAudit(h.db, r, currentUser, logger.ActionAutomationCreate, logger.ResourceAutomation, &createdAction.ID, createdAction.Name)

	respondJSONCreated(w, createdAction)
}

// applyActionUpdateFields applies non-nil fields from the update request to the action.
func applyActionUpdateFields(action *models.Action, req *models.UpdateActionRequest) {
	if req.Name != nil {
		action.Name = *req.Name
	}
	if req.Description != nil {
		action.Description = *req.Description
	}
	if req.TriggerType != nil {
		action.TriggerType = *req.TriggerType
	}
	if req.TriggerConfig != nil {
		action.TriggerConfig = *req.TriggerConfig
	}
	if req.IsEnabled != nil {
		action.IsEnabled = *req.IsEnabled
	}
}

// UpdateAction updates an existing action
func (h *ActionsHandler) UpdateAction(w http.ResponseWriter, r *http.Request) {
	workspaceID, action, ok := h.requireWorkspaceAction(w, r)
	if !ok {
		return
	}
	actionID := action.ID

	// Parse request body
	req, ok := decodeJSON[models.UpdateActionRequest](w, r)
	if !ok {
		return
	}

	var err error

	applyActionUpdateFields(action, &req)

	// If nodes and edges are provided, update them atomically
	if req.Nodes != nil {
		err = h.repo.SaveActionWithNodesAndEdges(action, req.Nodes, req.Edges)
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to save action: %w", err))
			return
		}
	} else {
		// Just update the action metadata
		err = h.repo.Update(action)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	// Invalidate cache
	if h.actionService != nil {
		h.actionService.InvalidateWorkspaceCache(workspaceID)
	}

	// Fetch updated action
	updatedAction, err := h.repo.GetByID(actionID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logAudit(h.db, r, currentUser, logger.ActionAutomationUpdate, logger.ResourceAutomation, &actionID, updatedAction.Name)
	}

	respondJSONOK(w, updatedAction)
}

// DeleteAction deletes an action
func (h *ActionsHandler) DeleteAction(w http.ResponseWriter, r *http.Request) {
	workspaceID, action, ok := h.requireWorkspaceAction(w, r)
	if !ok {
		return
	}
	actionID := action.ID

	err := h.repo.Delete(actionID)
	if err == repository.ErrNotFound {
		respondNotFound(w, r, "action")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Invalidate cache
	if h.actionService != nil {
		h.actionService.InvalidateWorkspaceCache(workspaceID)
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logAudit(h.db, r, currentUser, logger.ActionAutomationDelete, logger.ResourceAutomation, &actionID, "")
	}

	w.WriteHeader(http.StatusNoContent)
}

// ToggleAction enables or disables an action
func (h *ActionsHandler) ToggleAction(w http.ResponseWriter, r *http.Request) {
	_, action, ok := h.requireWorkspaceAction(w, r)
	if !ok {
		return
	}
	actionID := action.ID

	// Parse request body to get new state
	var req struct {
		IsEnabled bool `json:"is_enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// If no body, toggle the current state
		req.IsEnabled = !action.IsEnabled
	}

	if err := h.repo.SetEnabled(actionID, req.IsEnabled); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Invalidate cache
	if h.actionService != nil {
		h.actionService.InvalidateWorkspaceCache(action.WorkspaceID)
	}

	// Return updated action
	updatedAction, err := h.repo.GetByID(actionID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logAudit(h.db, r, currentUser, logger.ActionAutomationToggle, logger.ResourceAutomation, &actionID, updatedAction.Name)
	}

	respondJSONOK(w, updatedAction)
}

// GetActionLogs gets execution logs for an action
func (h *ActionsHandler) GetActionLogs(w http.ResponseWriter, r *http.Request) {
	_, action, ok := h.requireWorkspaceAction(w, r)
	if !ok {
		return
	}
	actionID := action.ID

	limit, offset := parseOffsetPagination(r, 50, 100)

	logs, err := h.repo.GetExecutionLogsByActionID(actionID, limit, offset)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if logs == nil {
		logs = []*models.ActionExecutionLog{}
	}

	respondJSONOK(w, logs)
}

// GetWorkspaceLogs gets all execution logs for a workspace
func (h *ActionsHandler) GetWorkspaceLogs(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireWorkspaceIDParam(w, r, h.keyCache, "workspaceId")
	if !ok {
		return
	}

	// Parse pagination params
	limit, offset := parseOffsetPagination(r, 50, 100)

	logs, err := h.repo.GetExecutionLogsByWorkspaceID(workspaceID, limit, offset)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if logs == nil {
		logs = []*models.ActionExecutionLog{}
	}

	respondJSONOK(w, logs)
}

// ExecuteActionRequest represents the request body for manual action execution
type ExecuteActionRequest struct {
	ItemID int `json:"item_id"`
}

// ExecuteAction manually executes an action for a specific item
func (h *ActionsHandler) ExecuteAction(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireWorkspaceIDParam(w, r, h.keyCache, "workspaceId")
	if !ok {
		return
	}

	actionID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var err error

	// Parse request body
	req, ok := decodeJSON[ExecuteActionRequest](w, r)
	if !ok {
		return
	}

	if req.ItemID == 0 {
		respondValidationError(w, r, "item_id is required")
		return
	}

	// Get action and verify workspace ownership
	action, ok := h.requireAction(w, r, actionID, workspaceID)
	if !ok {
		return
	}

	// Verify user has edit permission on the item's workspace
	if !CheckItemPermission(w, r, h.db, h.permissionService, req.ItemID, models.PermissionItemEdit) {
		return
	}

	// Get current user
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Execute action manually
	if h.actionService == nil {
		respondInternalError(w, r, fmt.Errorf("action service not available"))
		return
	}

	// Execute the action (this is synchronous for immediate feedback)
	err = h.actionService.ExecuteActionManually(action, req.ItemID, currentUser.ID)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to execute action: %w", err))
		return
	}

	respondJSONOK(w, map[string]string{"status": "completed"})
}

// --- Capability management endpoints ---

// ListCapabilities lists all action capabilities
func (h *ActionsHandler) ListCapabilities(w http.ResponseWriter, r *http.Request) {
	caps, err := h.repo.ListCapabilities()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if caps == nil {
		caps = []*models.ActionCapability{}
	}

	respondJSONOK(w, caps)
}

// GetCapability gets a single capability by ID
func (h *ActionsHandler) GetCapability(w http.ResponseWriter, r *http.Request) {
	capability, ok := h.requireCapability(w, r)
	if !ok {
		return
	}
	respondJSONOK(w, capability)
}

// CreateCapability creates a new action capability
func (h *ActionsHandler) CreateCapability(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[models.CreateCapabilityRequest](w, r)
	if !ok {
		return
	}

	if req.Name == "" {
		respondValidationError(w, r, "Name is required")
		return
	}
	if req.CapabilityType == "" {
		respondValidationError(w, r, "Capability type is required")
		return
	}
	// Validate capability type
	switch req.CapabilityType {
	case models.CapabilityDockerEnvironment, models.CapabilityHTTPClient, models.CapabilityLLMConnection:
		// valid
	default:
		respondValidationError(w, r, fmt.Sprintf("Invalid capability type: %s", req.CapabilityType))
		return
	}
	if req.Config == "" {
		respondValidationError(w, r, "Config is required")
		return
	}

	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	capability := &models.ActionCapability{
		Name:           req.Name,
		CapabilityType: req.CapabilityType,
		Config:         req.Config,
		IsEnabled:      true,
		CreatedBy:      &currentUser.ID,
	}

	id, err := h.repo.CreateCapability(capability)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	created, err := h.repo.GetCapabilityByID(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONCreated(w, created)
}

// UpdateCapability updates an existing capability
func (h *ActionsHandler) UpdateCapability(w http.ResponseWriter, r *http.Request) {
	capability, ok := h.requireCapability(w, r)
	if !ok {
		return
	}

	req, ok := decodeJSON[models.UpdateCapabilityRequest](w, r)
	if !ok {
		return
	}

	if req.Name != nil {
		capability.Name = *req.Name
	}
	if req.Config != nil {
		capability.Config = *req.Config
	}
	if req.IsEnabled != nil {
		capability.IsEnabled = *req.IsEnabled
	}

	if err := h.repo.UpdateCapability(capability); err != nil {
		respondInternalError(w, r, err)
		return
	}

	updated, err := h.repo.GetCapabilityByID(capability.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, updated)
}

// DeleteCapability deletes a capability
func (h *ActionsHandler) DeleteCapability(w http.ResponseWriter, r *http.Request) {
	capID, ok := requireIDParam(w, r, "capabilityId")
	if !ok {
		return
	}

	if _, err := h.repo.GetCapabilityByID(capID); err == repository.ErrNotFound {
		respondNotFound(w, r, "capability")
		return
	}

	if err := h.repo.DeleteCapability(capID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
