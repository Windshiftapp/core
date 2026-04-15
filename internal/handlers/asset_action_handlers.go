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
)

// AssetActionHandler handles asset action automation API endpoints
type AssetActionHandler struct {
	db            database.Database
	repo          *repository.AssetActionRepository
	assetHandler  *AssetHandler
	actionService *services.AssetActionService
}

// NewAssetActionHandler creates a new asset action handler
// requireAssetAction fetches an asset action by ID and verifies set ownership.
func (h *AssetActionHandler) requireAssetAction(w http.ResponseWriter, r *http.Request, actionID, setID int) (*models.AssetAction, bool) {
	action, err := h.repo.GetByID(actionID)
	if err == repository.ErrNotFound || (err == nil && action.SetID != setID) {
		respondNotFound(w, r, "asset action")
		return nil, false
	}
	if err != nil {
		respondInternalError(w, r, err)
		return nil, false
	}
	return action, true
}

func NewAssetActionHandler(db database.Database, assetHandler *AssetHandler, actionService *services.AssetActionService) *AssetActionHandler {
	return &AssetActionHandler{
		db:            db,
		repo:          repository.NewAssetActionRepository(db),
		assetHandler:  assetHandler,
		actionService: actionService,
	}
}

// requireSetAdminAccess parses setID from the "setId" path param and checks admin permission.
func (h *AssetActionHandler) requireSetAdminAccess(w http.ResponseWriter, r *http.Request) (*models.User, int, bool) {
	return h.assetHandler.requireSetAdminAccess(w, r)
}

// ListActions lists all actions for an asset set
func (h *AssetActionHandler) ListActions(w http.ResponseWriter, r *http.Request) {
	_, setID, ok := h.requireSetAdminAccess(w, r)
	if !ok {
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

	respondJSONOK(w, actions)
}

// GetAction gets a single asset action by ID
func (h *AssetActionHandler) GetAction(w http.ResponseWriter, r *http.Request) {
	_, setID, ok := h.requireSetAdminAccess(w, r)
	if !ok {
		return
	}

	actionID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	action, ok := h.requireAssetAction(w, r, actionID, setID)
	if !ok {
		return
	}

	respondJSONOK(w, action)
}

// CreateAction creates a new asset action
func (h *AssetActionHandler) CreateAction(w http.ResponseWriter, r *http.Request) {
	currentUser, setID, ok := h.requireSetAdminAccess(w, r)
	if !ok {
		return
	}

	req, ok := decodeJSON[models.CreateAssetActionRequest](w, r)
	if !ok {
		return
	}

	if msg := actionutil.ValidateActionFields(req.Name, string(req.TriggerType)); msg != "" {
		respondValidationError(w, r, msg)
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

	// Create nodes and edges if provided
	if flowErr := actionutil.CreateFlowNodesAndEdges[
		models.AssetActionNode, *models.AssetActionNode,
		models.AssetActionEdge, *models.AssetActionEdge,
	](
		actionID, req.Nodes, req.Edges,
		func(n *models.AssetActionNode) (int, error) { return h.createNode(*n) },
		func(e *models.AssetActionEdge) (int, error) { return h.createEdge(*e) },
		func() { _ = h.repo.Delete(actionID) },
	); flowErr != nil {
		respondInternalError(w, r, flowErr)
		return
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

	logAudit(h.db, r, currentUser, logger.ActionAutomationCreate, logger.ResourceAutomation, &createdAction.ID, createdAction.Name)

	respondJSONCreated(w, createdAction)
}

// applyAssetActionUpdateFields applies non-nil fields from the update request to the asset action.
func applyAssetActionUpdateFields(action *models.AssetAction, req *models.UpdateAssetActionRequest) {
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

// UpdateAction updates an existing asset action
func (h *AssetActionHandler) UpdateAction(w http.ResponseWriter, r *http.Request) {
	currentUser, setID, ok := h.requireSetAdminAccess(w, r)
	if !ok {
		return
	}

	actionID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	action, ok := h.requireAssetAction(w, r, actionID, setID)
	if !ok {
		return
	}

	req, ok := decodeJSON[models.UpdateAssetActionRequest](w, r)
	if !ok {
		return
	}

	var err error

	applyAssetActionUpdateFields(action, &req)

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

	if currentUser != nil {
		logAudit(h.db, r, currentUser, logger.ActionAutomationUpdate, logger.ResourceAutomation, &actionID, updatedAction.Name)
	}

	respondJSONOK(w, updatedAction)
}

// DeleteAction deletes an asset action
func (h *AssetActionHandler) DeleteAction(w http.ResponseWriter, r *http.Request) {
	currentUser, setID, ok := h.requireSetAdminAccess(w, r)
	if !ok {
		return
	}

	actionID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	if _, ok := h.requireAssetAction(w, r, actionID, setID); !ok {
		return
	}

	if err := h.repo.Delete(actionID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	if h.actionService != nil {
		h.actionService.InvalidateSetCache(setID)
	}

	if currentUser != nil {
		logAudit(h.db, r, currentUser, logger.ActionAutomationDelete, logger.ResourceAutomation, &actionID, "")
	}

	w.WriteHeader(http.StatusNoContent)
}

// ToggleAction enables or disables an asset action
func (h *AssetActionHandler) ToggleAction(w http.ResponseWriter, r *http.Request) {
	_, setID, ok := h.requireSetAdminAccess(w, r)
	if !ok {
		return
	}

	actionID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	action, ok := h.requireAssetAction(w, r, actionID, setID)
	if !ok {
		return
	}

	var req struct {
		IsEnabled bool `json:"is_enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.IsEnabled = !action.IsEnabled
	}

	if err := h.repo.SetEnabled(actionID, req.IsEnabled); err != nil {
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

	respondJSONOK(w, updatedAction)
}

// ExecuteAssetActionRequest represents the request body for manual asset action execution
type ExecuteAssetActionRequest struct {
	AssetID int `json:"asset_id"`
}

// ExecuteAction manually executes an asset action
func (h *AssetActionHandler) ExecuteAction(w http.ResponseWriter, r *http.Request) {
	currentUser, setID, ok := h.requireSetAdminAccess(w, r)
	if !ok {
		return
	}

	actionID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var err error

	req, ok := decodeJSON[ExecuteAssetActionRequest](w, r)
	if !ok {
		return
	}

	if req.AssetID == 0 {
		respondValidationError(w, r, "asset_id is required")
		return
	}

	action, ok := h.requireAssetAction(w, r, actionID, setID)
	if !ok {
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

	respondJSONOK(w, map[string]string{"status": "completed"})
}

// GetActionLogs gets execution logs for an asset action
func (h *AssetActionHandler) GetActionLogs(w http.ResponseWriter, r *http.Request) {
	_, setID, ok := h.requireSetAdminAccess(w, r)
	if !ok {
		return
	}

	actionID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	if _, ok := h.requireAssetAction(w, r, actionID, setID); !ok {
		return
	}

	limit, offset := parseOffsetPagination(r, 50, 100)

	logs, err := h.repo.GetExecutionLogs(actionID, limit, offset)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if logs == nil {
		logs = []*models.AssetActionExecutionLog{}
	}

	respondJSONOK(w, logs)
}

// GetSetLogs gets all execution logs for an asset set
func (h *AssetActionHandler) GetSetLogs(w http.ResponseWriter, r *http.Request) {
	_, setID, ok := h.requireSetAdminAccess(w, r)
	if !ok {
		return
	}

	limit, offset := parseOffsetPagination(r, 50, 100)

	logs, err := h.repo.GetSetExecutionLogs(setID, limit, offset)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if logs == nil {
		logs = []*models.AssetActionExecutionLog{}
	}

	respondJSONOK(w, logs)
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
