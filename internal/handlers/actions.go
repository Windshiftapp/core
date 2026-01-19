package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// ActionsHandler handles action automation API endpoints
type ActionsHandler struct {
	db            database.Database
	repo          *repository.ActionRepository
	actionService *services.ActionService
}

// NewActionsHandler creates a new actions handler
func NewActionsHandler(db database.Database, actionService *services.ActionService) *ActionsHandler {
	return &ActionsHandler{
		db:            db,
		repo:          repository.NewActionRepository(db),
		actionService: actionService,
	}
}

// ListActions lists all actions for a workspace
func (h *ActionsHandler) ListActions(w http.ResponseWriter, r *http.Request) {
	workspaceIDStr := r.PathValue("workspaceId")
	workspaceID, err := strconv.Atoi(workspaceIDStr)
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	actions, err := h.repo.ListByWorkspace(workspaceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if actions == nil {
		actions = []*models.Action{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(actions)
}

// GetAction gets a single action by ID
func (h *ActionsHandler) GetAction(w http.ResponseWriter, r *http.Request) {
	actionIDStr := r.PathValue("id")
	actionID, err := strconv.Atoi(actionIDStr)
	if err != nil {
		http.Error(w, "Invalid action ID", http.StatusBadRequest)
		return
	}

	action, err := h.repo.GetByID(actionID)
	if err == repository.ErrNotFound {
		http.Error(w, "Action not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(action)
}

// CreateAction creates a new action
func (h *ActionsHandler) CreateAction(w http.ResponseWriter, r *http.Request) {
	workspaceIDStr := r.PathValue("workspaceId")
	workspaceID, err := strconv.Atoi(workspaceIDStr)
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	// Parse request body
	var req models.CreateActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}
	if req.TriggerType == "" {
		http.Error(w, "Trigger type is required", http.StatusBadRequest)
		return
	}

	// Get current user
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	action.ID = actionID

	// Create nodes if provided
	if len(req.Nodes) > 0 {
		nodeIDMap := make(map[int]int) // old ID -> new ID
		for _, node := range req.Nodes {
			node.ActionID = actionID
			newID, err := h.repo.CreateNode(&node)
			if err != nil {
				// Rollback by deleting the action
				h.repo.Delete(actionID)
				http.Error(w, "Failed to create nodes: "+err.Error(), http.StatusInternalServerError)
				return
			}
			nodeIDMap[node.ID] = newID
		}

		// Create edges with mapped node IDs
		for _, edge := range req.Edges {
			edge.ActionID = actionID
			if newSourceID, ok := nodeIDMap[edge.SourceNodeID]; ok {
				edge.SourceNodeID = newSourceID
			}
			if newTargetID, ok := nodeIDMap[edge.TargetNodeID]; ok {
				edge.TargetNodeID = newTargetID
			}
			_, err := h.repo.CreateEdge(&edge)
			if err != nil {
				// Rollback by deleting the action
				h.repo.Delete(actionID)
				http.Error(w, "Failed to create edges: "+err.Error(), http.StatusInternalServerError)
				return
			}
		}
	}

	// Invalidate cache
	if h.actionService != nil {
		h.actionService.InvalidateWorkspaceCache(workspaceID)
	}

	// Fetch the created action with nodes and edges
	createdAction, err := h.repo.GetByID(actionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(createdAction)
}

// UpdateAction updates an existing action
func (h *ActionsHandler) UpdateAction(w http.ResponseWriter, r *http.Request) {
	actionIDStr := r.PathValue("id")
	actionID, err := strconv.Atoi(actionIDStr)
	if err != nil {
		http.Error(w, "Invalid action ID", http.StatusBadRequest)
		return
	}

	// Get existing action
	action, err := h.repo.GetByID(actionID)
	if err == repository.ErrNotFound {
		http.Error(w, "Action not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Parse request body
	var req models.UpdateActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Update fields if provided
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

	// If nodes and edges are provided, update them atomically
	if req.Nodes != nil {
		err = h.repo.SaveActionWithNodesAndEdges(action, req.Nodes, req.Edges)
		if err != nil {
			http.Error(w, "Failed to save action: "+err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		// Just update the action metadata
		err = h.repo.Update(action)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Invalidate cache
	if h.actionService != nil {
		h.actionService.InvalidateWorkspaceCache(action.WorkspaceID)
	}

	// Fetch updated action
	updatedAction, err := h.repo.GetByID(actionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updatedAction)
}

// DeleteAction deletes an action
func (h *ActionsHandler) DeleteAction(w http.ResponseWriter, r *http.Request) {
	actionIDStr := r.PathValue("id")
	actionID, err := strconv.Atoi(actionIDStr)
	if err != nil {
		http.Error(w, "Invalid action ID", http.StatusBadRequest)
		return
	}

	// Get the action to get workspace ID for cache invalidation
	action, err := h.repo.GetByID(actionID)
	if err == repository.ErrNotFound {
		http.Error(w, "Action not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	workspaceID := action.WorkspaceID

	err = h.repo.Delete(actionID)
	if err == repository.ErrNotFound {
		http.Error(w, "Action not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Invalidate cache
	if h.actionService != nil {
		h.actionService.InvalidateWorkspaceCache(workspaceID)
	}

	w.WriteHeader(http.StatusNoContent)
}

// ToggleAction enables or disables an action
func (h *ActionsHandler) ToggleAction(w http.ResponseWriter, r *http.Request) {
	actionIDStr := r.PathValue("id")
	actionID, err := strconv.Atoi(actionIDStr)
	if err != nil {
		http.Error(w, "Invalid action ID", http.StatusBadRequest)
		return
	}

	// Get existing action
	action, err := h.repo.GetByID(actionID)
	if err == repository.ErrNotFound {
		http.Error(w, "Action not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Parse request body to get new state
	var req struct {
		IsEnabled bool `json:"is_enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// If no body, toggle the current state
		req.IsEnabled = !action.IsEnabled
	}

	err = h.repo.SetEnabled(actionID, req.IsEnabled)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Invalidate cache
	if h.actionService != nil {
		h.actionService.InvalidateWorkspaceCache(action.WorkspaceID)
	}

	// Return updated action
	updatedAction, err := h.repo.GetByID(actionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updatedAction)
}

// GetActionLogs gets execution logs for an action
func (h *ActionsHandler) GetActionLogs(w http.ResponseWriter, r *http.Request) {
	actionIDStr := r.PathValue("id")
	actionID, err := strconv.Atoi(actionIDStr)
	if err != nil {
		http.Error(w, "Invalid action ID", http.StatusBadRequest)
		return
	}

	// Parse pagination params
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

	logs, err := h.repo.GetExecutionLogsByActionID(actionID, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if logs == nil {
		logs = []*models.ActionExecutionLog{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}

// GetWorkspaceLogs gets all execution logs for a workspace
func (h *ActionsHandler) GetWorkspaceLogs(w http.ResponseWriter, r *http.Request) {
	workspaceIDStr := r.PathValue("workspaceId")
	workspaceID, err := strconv.Atoi(workspaceIDStr)
	if err != nil {
		http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
		return
	}

	// Parse pagination params
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

	logs, err := h.repo.GetExecutionLogsByWorkspaceID(workspaceID, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if logs == nil {
		logs = []*models.ActionExecutionLog{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}
