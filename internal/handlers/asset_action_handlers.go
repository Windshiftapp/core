package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// AssetActionHandler handles asset action automation API endpoints
type AssetActionHandler struct {
	db            database.Database
	repo          *repository.AssetActionRepository
	assetHandler  *AssetHandler
	actionService *services.AssetActionService
}

// NewAssetActionHandler creates a new asset action handler
func NewAssetActionHandler(db database.Database, assetHandler *AssetHandler, actionService *services.AssetActionService) *AssetActionHandler {
	return &AssetActionHandler{
		db:            db,
		repo:          repository.NewAssetActionRepository(db),
		assetHandler:  assetHandler,
		actionService: actionService,
	}
}

// ListActions lists all actions for an asset set
func (h *AssetActionHandler) ListActions(w http.ResponseWriter, r *http.Request) {
	setID, err := strconv.Atoi(r.PathValue("setId"))
	if err != nil {
		respondInvalidID(w, r, "setId")
		return
	}

	// Check admin permission
	if !h.checkAdminPermission(w, r, setID) {
		return
	}

	actions, err := h.repo.ListBySet(setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if actions == nil {
		actions = []*models.AssetAction{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(actions)
}

// GetAction gets a single asset action by ID
func (h *AssetActionHandler) GetAction(w http.ResponseWriter, r *http.Request) {
	setID, err := strconv.Atoi(r.PathValue("setId"))
	if err != nil {
		respondInvalidID(w, r, "setId")
		return
	}

	actionID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	if !h.checkAdminPermission(w, r, setID) {
		return
	}

	action, err := h.repo.GetByID(actionID)
	if err == repository.ErrNotFound || (err == nil && action.SetID != setID) {
		respondNotFound(w, r, "asset action")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(action)
}

// CreateAction creates a new asset action
func (h *AssetActionHandler) CreateAction(w http.ResponseWriter, r *http.Request) {
	setID, err := strconv.Atoi(r.PathValue("setId"))
	if err != nil {
		respondInvalidID(w, r, "setId")
		return
	}

	if !h.checkAdminPermission(w, r, setID) {
		return
	}

	var req models.CreateAssetActionRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	if req.Name == "" {
		respondValidationError(w, r, "Name is required")
		return
	}
	if req.TriggerType == "" {
		respondValidationError(w, r, "Trigger type is required")
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return
	}

	action := &models.AssetAction{
		SetID:         setID,
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

	// Create nodes if provided
	if len(req.Nodes) > 0 {
		nodeIDMap := make(map[int]int)
		for _, node := range req.Nodes {
			node.ActionID = actionID
			newID, nodeErr := h.createNode(node)
			if nodeErr != nil {
				_ = h.repo.Delete(actionID)
				respondInternalError(w, r, fmt.Errorf("failed to create nodes: %w", nodeErr))
				return
			}
			nodeIDMap[node.ID] = newID
		}

		for _, edge := range req.Edges {
			edge.ActionID = actionID
			if newSourceID, ok := nodeIDMap[edge.SourceNodeID]; ok {
				edge.SourceNodeID = newSourceID
			}
			if newTargetID, ok := nodeIDMap[edge.TargetNodeID]; ok {
				edge.TargetNodeID = newTargetID
			}
			if _, edgeErr := h.createEdge(edge); edgeErr != nil {
				_ = h.repo.Delete(actionID)
				respondInternalError(w, r, fmt.Errorf("failed to create edges: %w", edgeErr))
				return
			}
		}
	}

	// Invalidate cache
	if h.actionService != nil {
		h.actionService.InvalidateSetCache(setID)
	}

	createdAction, err := h.repo.GetByID(actionID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	_ = logger.LogAudit(h.db, logger.AuditEvent{
		UserID:       currentUser.ID,
		Username:     currentUser.Username,
		IPAddress:    utils.GetClientIP(r),
		UserAgent:    r.UserAgent(),
		ActionType:   logger.ActionAutomationCreate,
		ResourceType: logger.ResourceAutomation,
		ResourceID:   &createdAction.ID,
		ResourceName: createdAction.Name,
		Success:      true,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(createdAction)
}

// UpdateAction updates an existing asset action
func (h *AssetActionHandler) UpdateAction(w http.ResponseWriter, r *http.Request) {
	setID, err := strconv.Atoi(r.PathValue("setId"))
	if err != nil {
		respondInvalidID(w, r, "setId")
		return
	}

	actionID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	if !h.checkAdminPermission(w, r, setID) {
		return
	}

	action, err := h.repo.GetByID(actionID)
	if err == repository.ErrNotFound || (err == nil && action.SetID != setID) {
		respondNotFound(w, r, "asset action")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	var req models.UpdateAssetActionRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

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

	if req.Nodes != nil {
		err = h.repo.SaveActionWithNodesAndEdges(action, req.Nodes, req.Edges)
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to save action: %w", err))
			return
		}
	} else {
		err = h.repo.Update(action)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	if h.actionService != nil {
		h.actionService.InvalidateSetCache(setID)
	}

	updatedAction, err := h.repo.GetByID(actionID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionAutomationUpdate,
			ResourceType: logger.ResourceAutomation,
			ResourceID:   &actionID,
			ResourceName: updatedAction.Name,
			Success:      true,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(updatedAction)
}

// DeleteAction deletes an asset action
func (h *AssetActionHandler) DeleteAction(w http.ResponseWriter, r *http.Request) {
	setID, err := strconv.Atoi(r.PathValue("setId"))
	if err != nil {
		respondInvalidID(w, r, "setId")
		return
	}

	actionID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	if !h.checkAdminPermission(w, r, setID) {
		return
	}

	action, err := h.repo.GetByID(actionID)
	if err == repository.ErrNotFound || (err == nil && action.SetID != setID) {
		respondNotFound(w, r, "asset action")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	err = h.repo.Delete(actionID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if h.actionService != nil {
		h.actionService.InvalidateSetCache(setID)
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionAutomationDelete,
			ResourceType: logger.ResourceAutomation,
			ResourceID:   &actionID,
			Success:      true,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

// ToggleAction enables or disables an asset action
func (h *AssetActionHandler) ToggleAction(w http.ResponseWriter, r *http.Request) {
	setID, err := strconv.Atoi(r.PathValue("setId"))
	if err != nil {
		respondInvalidID(w, r, "setId")
		return
	}

	actionID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	if !h.checkAdminPermission(w, r, setID) {
		return
	}

	action, err := h.repo.GetByID(actionID)
	if err == repository.ErrNotFound || (err == nil && action.SetID != setID) {
		respondNotFound(w, r, "asset action")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	var req struct {
		IsEnabled bool `json:"is_enabled"`
	}
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.IsEnabled = !action.IsEnabled
	}

	err = h.repo.SetEnabled(actionID, req.IsEnabled)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if h.actionService != nil {
		h.actionService.InvalidateSetCache(setID)
	}

	updatedAction, err := h.repo.GetByID(actionID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(updatedAction)
}

// ExecuteAssetActionRequest represents the request body for manual asset action execution
type ExecuteAssetActionRequest struct {
	AssetID int `json:"asset_id"`
}

// ExecuteAction manually executes an asset action
func (h *AssetActionHandler) ExecuteAction(w http.ResponseWriter, r *http.Request) {
	setID, err := strconv.Atoi(r.PathValue("setId"))
	if err != nil {
		respondInvalidID(w, r, "setId")
		return
	}

	actionID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	if !h.checkAdminPermission(w, r, setID) {
		return
	}

	var req ExecuteAssetActionRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	if req.AssetID == 0 {
		respondValidationError(w, r, "asset_id is required")
		return
	}

	action, err := h.repo.GetByID(actionID)
	if err == repository.ErrNotFound || (err == nil && action.SetID != setID) {
		respondNotFound(w, r, "asset action")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return
	}

	if h.actionService == nil {
		respondInternalError(w, r, fmt.Errorf("asset action service not available"))
		return
	}

	err = h.actionService.ExecuteActionManually(action, req.AssetID, currentUser.ID)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to execute action: %w", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "completed"})
}

// GetActionLogs gets execution logs for an asset action
func (h *AssetActionHandler) GetActionLogs(w http.ResponseWriter, r *http.Request) {
	setID, err := strconv.Atoi(r.PathValue("setId"))
	if err != nil {
		respondInvalidID(w, r, "setId")
		return
	}

	actionID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	if !h.checkAdminPermission(w, r, setID) {
		return
	}

	action, err := h.repo.GetByID(actionID)
	if err == repository.ErrNotFound || (err == nil && action.SetID != setID) {
		respondNotFound(w, r, "asset action")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	limit, offset := parsePagination(r)

	logs, err := h.repo.GetExecutionLogs(actionID, limit, offset)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if logs == nil {
		logs = []*models.AssetActionExecutionLog{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(logs)
}

// GetSetLogs gets all execution logs for an asset set
func (h *AssetActionHandler) GetSetLogs(w http.ResponseWriter, r *http.Request) {
	setID, err := strconv.Atoi(r.PathValue("setId"))
	if err != nil {
		respondInvalidID(w, r, "setId")
		return
	}

	if !h.checkAdminPermission(w, r, setID) {
		return
	}

	limit, offset := parsePagination(r)

	logs, err := h.repo.GetSetExecutionLogs(setID, limit, offset)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if logs == nil {
		logs = []*models.AssetActionExecutionLog{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(logs)
}

// checkAdminPermission verifies the user has admin permission on the asset set
func (h *AssetActionHandler) checkAdminPermission(w http.ResponseWriter, r *http.Request, setID int) bool {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return false
	}

	hasAdmin, err := h.assetHandler.hasAssetPermission(currentUser.ID, setID, AssetPermissionKeyAdmin)
	if err != nil {
		respondInternalError(w, r, err)
		return false
	}
	if !hasAdmin {
		respondNotFound(w, r, "asset set")
		return false
	}
	return true
}

// createNode creates a single asset action node
func (h *AssetActionHandler) createNode(node models.AssetActionNode) (int, error) {
	var id int
	err := h.db.QueryRow(`
		INSERT INTO asset_action_nodes (action_id, node_type, node_config, position_x, position_y, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP) RETURNING id
	`, node.ActionID, node.NodeType, node.NodeConfig, node.PositionX, node.PositionY).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create node: %w", err)
	}
	return id, nil
}

// createEdge creates a single asset action edge
func (h *AssetActionHandler) createEdge(edge models.AssetActionEdge) (int, error) {
	var id int
	err := h.db.QueryRow(`
		INSERT INTO asset_action_edges (action_id, source_node_id, target_node_id, edge_type, source_handle, target_handle, created_at)
		VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP) RETURNING id
	`, edge.ActionID, edge.SourceNodeID, edge.TargetNodeID, edge.EdgeType, edge.SourceHandle, edge.TargetHandle).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create edge: %w", err)
	}
	return id, nil
}

// parsePagination extracts limit/offset from query params
func parsePagination(r *http.Request) (int, int) {
	limit := 50
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}
	return limit, offset
}
