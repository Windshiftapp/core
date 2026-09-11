package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/restapi"
	"windshift/internal/services"
)

// AgentRunHandler exposes read-only, item-scoped coding-agent run history on
// the bearer surface — the data behind the item detail "Agent log" panel.
// Mutations (rerun/cancel) stay on the session surface.
type AgentRunHandler struct {
	BaseHandler
	reads *services.AgentRunReadService
}

// NewAgentRunHandler creates a new agent-run handler.
func NewAgentRunHandler(db database.Database, permissionService *services.PermissionService) *AgentRunHandler {
	return &AgentRunHandler{
		BaseHandler: NewBaseHandler(db, permissionService),
		reads: services.NewAgentRunReadService(
			repository.NewItemRepository(db),
			repository.NewAgentRunRepository(db),
			permissionService,
		),
	}
}

// AgentRunResponse is the public API representation of a coding-agent run.
type AgentRunResponse struct {
	ID          int        `json:"id"`
	WorkspaceID int        `json:"workspace_id"`
	ItemID      *int       `json:"item_id,omitempty"`
	Status      string     `json:"status"`
	JobKind     string     `json:"job_kind,omitempty"`
	QueuedAt    time.Time  `json:"queued_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	EndedAt     *time.Time `json:"ended_at,omitempty"`
	Error       string     `json:"error,omitempty"`
}

// ListForItem handles GET /rest/api/v1/items/{id}/agent-runs
//
// @Summary      List coding-agent runs for an item
// @Description  Returns the coding-agent runs triggered against one work item, newest first. Mirrors the session surface's item agent-run list. Gated on view access to the item's workspace; both a missing item and a missing permission return 404 so item existence never leaks.
// @Tags         items
// @Produce      json
// @Security     BearerAuth
// @Param        id         path      int  true   "Item ID"
// @Param        limit      query     int  false  "Max runs to return (default 50)"
// @Param        before_id  query     int  false  "Cursor: only runs with id < before_id"
// @Success      200  {array}   handlers.AgentRunResponse
// @Failure      401  {object}  handlers.ErrorResponse
// @Failure      403  {object}  handlers.ErrorResponse  "Token lacks the items:read scope"
// @Failure      404  {object}  handlers.ErrorResponse  "Item not found or not accessible"
// @Failure      500  {object}  handlers.ErrorResponse
// @Router       /items/{id}/agent-runs [get]
func (h *AgentRunHandler) ListForItem(w http.ResponseWriter, r *http.Request) {
	user, ok := h.RequireAuth(w, r)
	if !ok {
		return
	}
	itemID, ok := h.ParsePathID(w, r, "id", "item ID")
	if !ok {
		return
	}

	limit := 50
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 {
		limit = n
	}
	beforeID := 0
	if n, err := strconv.Atoi(r.URL.Query().Get("before_id")); err == nil && n > 0 {
		beforeID = n
	}

	runs, err := h.reads.ListForItem(r.Context(), user.ID, itemID, limit, beforeID)
	if errors.Is(err, services.ErrAgentRunItemNotVisible) {
		h.RespondError(w, r, restapi.ErrItemNotFound)
		return
	}
	if err != nil {
		h.RespondInternalError(w, r)
		return
	}

	out := make([]AgentRunResponse, 0, len(runs))
	for _, run := range runs {
		out = append(out, mapAgentRunToResponse(run))
	}
	h.RespondOK(w, out)
}

func mapAgentRunToResponse(r *models.AgentRun) AgentRunResponse {
	return AgentRunResponse{
		ID:          r.ID,
		WorkspaceID: r.WorkspaceID,
		ItemID:      r.ItemID,
		Status:      r.Status,
		JobKind:     r.JobKind,
		QueuedAt:    r.QueuedAt,
		StartedAt:   r.StartedAt,
		EndedAt:     r.EndedAt,
		Error:       r.Error,
	}
}
