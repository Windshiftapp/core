package logbook

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/restapi"
)

// ActionHandlers holds HTTP handlers for logbook action automation.
type ActionHandlers struct {
	repo          *repository.LogbookActionRepository
	permService   *PermissionService
	actionService *LogbookActionService
	logbookRepo   *Repository
}

// requireActionID parses the actionID path parameter and returns it, or responds with an error.
func (h *ActionHandlers) requireActionID(w http.ResponseWriter, r *http.Request) (int, bool) {
	actionID, err := strconv.Atoi(r.PathValue("actionID"))
	if err != nil {
		restapi.RespondErrorWithMessage(w, r, http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid action ID")
		return 0, false
	}
	return actionID, true
}

// requireAction fetches a logbook action by ID and verifies bucket ownership.
func (h *ActionHandlers) requireAction(w http.ResponseWriter, r *http.Request, actionID int, bucketID string) (*models.LogbookAction, bool) {
	action, err := h.repo.GetByID(actionID)
	if err == repository.ErrNotFound || (err == nil && action.BucketID != bucketID) {
		respondNotFound(w, r)
		return nil, false
	}
	if err != nil {
		respondInternalError(w, r, err)
		return nil, false
	}
	return action, true
}

// NewActionHandlers creates a new set of action handlers for the sidecar.
func NewActionHandlers(repo *repository.LogbookActionRepository, permService *PermissionService, actionService *LogbookActionService, logbookRepo *Repository) *ActionHandlers {
	return &ActionHandlers{
		repo:          repo,
		permService:   permService,
		actionService: actionService,
		logbookRepo:   logbookRepo,
	}
}

// requireBucketAdmin checks bucket.admin permission and returns bucketID + LogbookUser if authorized.
func (h *ActionHandlers) requireBucketAdmin(w http.ResponseWriter, r *http.Request) (string, *LogbookUser, bool) {
	lbUser, ok := requireLogbookAuth(w, r)
	if !ok {
		return "", nil, false
	}

	bucketID := r.PathValue("bucketID")
	if bucketID == "" {
		restapi.RespondErrorWithMessage(w, r, http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Missing bucket ID")
		return "", nil, false
	}

	has, err := h.permService.HasBucketPermission(lbUser.ID, lbUser.IsAdmin, lbUser.GroupIDs, bucketID, models.LogbookPermissionBucketAdmin)
	if err != nil {
		respondInternalError(w, r, err)
		return "", nil, false
	}
	if !has {
		respondNotFound(w, r)
		return "", nil, false
	}

	return bucketID, lbUser, true
}

// ListActions lists all actions for a bucket.
func (h *ActionHandlers) ListActions(w http.ResponseWriter, r *http.Request) {
	bucketID, _, ok := h.requireBucketAdmin(w, r)
	if !ok {
		return
	}

	actions, err := h.repo.ListByBucket(bucketID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if actions == nil {
		actions = []*models.LogbookAction{}
	}

	restapi.RespondOK(w, actions)
}

// GetAction gets a single logbook action by ID.
func (h *ActionHandlers) GetAction(w http.ResponseWriter, r *http.Request) {
	bucketID, _, ok := h.requireBucketAdmin(w, r)
	if !ok {
		return
	}

	actionID, ok := h.requireActionID(w, r)
	if !ok {
		return
	}

	action, ok := h.requireAction(w, r, actionID, bucketID)
	if !ok {
		return
	}

	restapi.RespondOK(w, action)
}

// CreateAction creates a new logbook action.
func (h *ActionHandlers) CreateAction(w http.ResponseWriter, r *http.Request) {
	bucketID, lbUser, ok := h.requireBucketAdmin(w, r)
	if !ok {
		return
	}

	var req models.CreateLogbookActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		restapi.RespondErrorWithMessage(w, r, http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid request body")
		return
	}

	if req.Name == "" {
		restapi.RespondErrorWithMessage(w, r, http.StatusBadRequest, restapi.ErrCodeValidationFailed, "Name is required")
		return
	}
	if req.TriggerType == "" {
		restapi.RespondErrorWithMessage(w, r, http.StatusBadRequest, restapi.ErrCodeValidationFailed, "Trigger type is required")
		return
	}

	userID := lbUser.ID
	action := &models.LogbookAction{
		BucketID:      bucketID,
		Name:          req.Name,
		Description:   req.Description,
		IsEnabled:     true,
		TriggerType:   req.TriggerType,
		TriggerConfig: req.TriggerConfig,
		CreatedBy:     &userID,
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
			newID, err := h.repo.CreateNode(&node)
			if err != nil {
				_ = h.repo.Delete(actionID)
				respondInternalError(w, r, fmt.Errorf("failed to create nodes: %w", err))
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
			_, err := h.repo.CreateEdge(&edge)
			if err != nil {
				_ = h.repo.Delete(actionID)
				respondInternalError(w, r, fmt.Errorf("failed to create edges: %w", err))
				return
			}
		}
	}

	if h.actionService != nil {
		h.actionService.InvalidateBucketCache(bucketID)
	}

	createdAction, err := h.repo.GetByID(actionID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	restapi.RespondCreated(w, createdAction)
}

// applyLogbookActionUpdateFields applies non-nil fields from the update request to the logbook action.
func applyLogbookActionUpdateFields(action *models.LogbookAction, req *models.UpdateLogbookActionRequest) {
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

// UpdateAction updates an existing logbook action.
func (h *ActionHandlers) UpdateAction(w http.ResponseWriter, r *http.Request) {
	bucketID, _, ok := h.requireBucketAdmin(w, r)
	if !ok {
		return
	}

	actionID, ok := h.requireActionID(w, r)
	if !ok {
		return
	}

	action, ok := h.requireAction(w, r, actionID, bucketID)
	if !ok {
		return
	}

	var req models.UpdateLogbookActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		restapi.RespondErrorWithMessage(w, r, http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid request body")
		return
	}

	applyLogbookActionUpdateFields(action, &req)

	if req.Nodes != nil {
		if err := h.repo.SaveActionWithNodesAndEdges(action, req.Nodes, req.Edges); err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to save logbook action: %w", err))
			return
		}
	} else {
		if err := h.repo.Update(action); err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	if h.actionService != nil {
		h.actionService.InvalidateBucketCache(bucketID)
	}

	updatedAction, err := h.repo.GetByID(actionID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	restapi.RespondOK(w, updatedAction)
}

// DeleteAction deletes a logbook action.
func (h *ActionHandlers) DeleteAction(w http.ResponseWriter, r *http.Request) {
	bucketID, _, ok := h.requireBucketAdmin(w, r)
	if !ok {
		return
	}

	actionID, ok := h.requireActionID(w, r)
	if !ok {
		return
	}

	if _, ok := h.requireAction(w, r, actionID, bucketID); !ok {
		return
	}

	if err := h.repo.Delete(actionID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	if h.actionService != nil {
		h.actionService.InvalidateBucketCache(bucketID)
	}

	restapi.RespondNoContent(w)
}

// ToggleAction enables or disables a logbook action.
func (h *ActionHandlers) ToggleAction(w http.ResponseWriter, r *http.Request) {
	bucketID, _, ok := h.requireBucketAdmin(w, r)
	if !ok {
		return
	}

	actionID, ok := h.requireActionID(w, r)
	if !ok {
		return
	}

	action, ok := h.requireAction(w, r, actionID, bucketID)
	if !ok {
		return
	}

	newEnabled := !action.IsEnabled
	if err := h.repo.SetEnabled(actionID, newEnabled); err != nil {
		respondInternalError(w, r, err)
		return
	}

	if h.actionService != nil {
		h.actionService.InvalidateBucketCache(bucketID)
	}

	action.IsEnabled = newEnabled
	restapi.RespondOK(w, action)
}

// ExecuteAction manually triggers a logbook action for a specific document.
func (h *ActionHandlers) ExecuteAction(w http.ResponseWriter, r *http.Request) {
	bucketID, lbUser, ok := h.requireBucketAdmin(w, r)
	if !ok {
		return
	}

	actionID, ok := h.requireActionID(w, r)
	if !ok {
		return
	}

	if _, ok := h.requireAction(w, r, actionID, bucketID); !ok {
		return
	}

	var req struct {
		DocumentID string `json:"document_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		restapi.RespondErrorWithMessage(w, r, http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid request body")
		return
	}
	if req.DocumentID == "" {
		restapi.RespondErrorWithMessage(w, r, http.StatusBadRequest, restapi.ErrCodeValidationFailed, "document_id is required")
		return
	}

	if h.actionService != nil {
		// Build event with document metadata from DB
		event := &models.LogbookActionEvent{
			EventType:   models.LogbookTriggerManual,
			BucketID:    bucketID,
			DocumentID:  req.DocumentID,
			ActorUserID: lbUser.ID,
		}

		// Enrich event with document metadata
		doc, err := h.logbookRepo.GetDocument(req.DocumentID)
		if err == nil && doc != nil {
			event.Title = doc.Title
			event.ContentType = doc.ContentType
			event.MimeType = doc.MimeType
			event.SourceType = doc.SourceType
			event.Author = doc.Author
		}

		slog.Info("starting manual action execution",
			slog.String("component", "logbook-actions"),
			slog.Int("action_id", actionID),
			slog.String("document_id", req.DocumentID),
			slog.String("bucket_id", bucketID),
		)

		// Execute directly (synchronous for manual triggers) so logs are immediately available
		go func() {
			if execErr := h.actionService.ExecuteActionDirectly(actionID, event); execErr != nil {
				slog.Error("manual action execution failed",
					slog.String("component", "logbook-actions"),
					slog.Int("action_id", actionID),
					slog.String("document_id", req.DocumentID),
					slog.Any("error", execErr),
				)
			}
		}()
	}

	restapi.RespondOK(w, map[string]string{"status": "queued"})
}

// GetActionLogs gets execution logs for a specific action.
func (h *ActionHandlers) GetActionLogs(w http.ResponseWriter, r *http.Request) {
	bucketID, _, ok := h.requireBucketAdmin(w, r)
	if !ok {
		return
	}

	actionID, ok := h.requireActionID(w, r)
	if !ok {
		return
	}

	if _, ok := h.requireAction(w, r, actionID, bucketID); !ok {
		return
	}

	limit := 50
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 200 {
			limit = parsed
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	logs, err := h.repo.GetExecutionLogs(actionID, limit, offset)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if logs == nil {
		logs = []*models.LogbookActionExecutionLog{}
	}

	restapi.RespondOK(w, logs)
}

// GetBucketLogs gets execution logs for all actions in a bucket.
func (h *ActionHandlers) GetBucketLogs(w http.ResponseWriter, r *http.Request) {
	bucketID, _, ok := h.requireBucketAdmin(w, r)
	if !ok {
		return
	}

	limit := 50
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 200 {
			limit = parsed
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	logs, err := h.repo.GetBucketExecutionLogs(bucketID, limit, offset)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if logs == nil {
		logs = []*models.LogbookActionExecutionLog{}
	}

	restapi.RespondOK(w, logs)
}
