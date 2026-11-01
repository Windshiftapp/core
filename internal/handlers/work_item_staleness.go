package handlers

import (
	"errors"
	"net/http"

	"windshift/internal/logger"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// WorkItemStalenessHandler manages the shared work item staleness threshold.
type WorkItemStalenessHandler struct {
	service *services.WorkItemStalenessService
	auditor *logger.Auditor
}

// NewWorkItemStalenessHandler creates a work item staleness settings handler.
func NewWorkItemStalenessHandler(service *services.WorkItemStalenessService, auditor *logger.Auditor) *WorkItemStalenessHandler {
	return &WorkItemStalenessHandler{service: service, auditor: auditor}
}

// Settings returns the shared threshold for composed API responses.
func (h *WorkItemStalenessHandler) Settings() (services.WorkItemStalenessSettings, error) {
	return h.service.Get()
}

// Get returns the current work item staleness settings.
func (h *WorkItemStalenessHandler) Get(w http.ResponseWriter, r *http.Request) {
	settings, err := h.Settings()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, settings)
}

// Update validates and persists the work item staleness settings.
func (h *WorkItemStalenessHandler) Update(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeJSON[services.WorkItemStalenessSettings](w, r)
	if !ok {
		return
	}

	before, err := h.Settings()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	settings, err := h.service.Update(request.StaleAfterDays)
	if errors.Is(err, services.ErrInvalidWorkItemStalenessThreshold) {
		respondValidationError(w, r, err.Error())
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if currentUser := utils.GetCurrentUser(r); currentUser != nil {
		h.auditor.LogWithDetails(
			r,
			currentUser,
			logger.ActionWorkItemStalenessUpdate,
			logger.ResourceWorkItemStaleness,
			nil,
			"work_item_staleness",
			map[string]any{
				"stale_after_days": map[string]any{"old": before.StaleAfterDays, "new": settings.StaleAfterDays},
			},
		)
	}
	respondJSONOK(w, settings)
}
