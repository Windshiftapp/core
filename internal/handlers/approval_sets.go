package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

// ApprovalSetHandler handles CRUD for approval sets — the asynchronous sibling
// of ConditionSetHandler. Approval sets are templates: a set is owned by a
// workflow and contains approval_set_statuses (one per status that fires an
// approval) and approval_steps (the sequential or parallel approver steps).
//
// Data access flows through service; the auditor handles audit logging.
type ApprovalSetHandler struct {
	service *services.ApprovalSetService
	auditor *logger.Auditor
}

// NewApprovalSetHandler constructs an ApprovalSetHandler bound to the given
// service.
func NewApprovalSetHandler(service *services.ApprovalSetService, auditor *logger.Auditor) *ApprovalSetHandler {
	return &ApprovalSetHandler{service: service, auditor: auditor}
}

// GetAll returns all approval sets, optionally filtered by workflow_id.
func (h *ApprovalSetHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	if _, ok := RequireAuth(w, r); !ok {
		return
	}

	var workflowID *int
	if workflowIDStr := r.URL.Query().Get("workflow_id"); workflowIDStr != "" {
		id, err := strconv.Atoi(workflowIDStr)
		if err != nil {
			respondValidationError(w, r, "Invalid workflow_id")
			return
		}
		workflowID = &id
	}

	out, err := h.service.List(r.Context(), workflowID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, out)
}

// Get returns a single approval set with its approval_set_statuses and steps.
func (h *ApprovalSetHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	if _, ok := RequireAuth(w, r); !ok {
		return
	}

	set, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "Approval set")
			return
		}
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, set)
}

// GetByWorkflow returns approval sets for a specific workflow.
func (h *ApprovalSetHandler) GetByWorkflow(w http.ResponseWriter, r *http.Request) {
	workflowID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	if _, ok := RequireAuth(w, r); !ok {
		return
	}

	out, err := h.service.List(r.Context(), &workflowID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, out)
}

// Create creates a new approval set with nested set-statuses and steps.
func (h *ApprovalSetHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, input, ok := h.decodeForEdit(w, r)
	if !ok {
		return
	}

	out, err := h.service.Create(r.Context(), input)
	if err != nil {
		h.respondServiceError(w, r, err)
		return
	}

	h.auditor.Log(r, user, logger.ActionApprovalSetCreate, logger.ResourceApprovalSet, &out.ID, input.Name)
	respondJSONCreated(w, out)
}

// Update replaces an approval set's name/description and its nested
// set-statuses + steps. workflow_id is immutable.
func (h *ApprovalSetHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	user, input, ok := h.decodeForEdit(w, r)
	if !ok {
		return
	}

	out, err := h.service.Update(r.Context(), id, input)
	if err != nil {
		h.respondServiceError(w, r, err)
		return
	}

	h.auditor.Log(r, user, logger.ActionApprovalSetUpdate, logger.ResourceApprovalSet, &id, input.Name)
	respondJSONOK(w, out)
}

// Delete deletes an approval set. Refuses if it's referenced by any
// configuration_set or has any non-canceled requests.
func (h *ApprovalSetHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	name, err := h.service.Delete(r.Context(), id)
	if err != nil {
		h.respondServiceError(w, r, err)
		return
	}

	h.auditor.Log(r, user, logger.ActionApprovalSetDelete, logger.ResourceApprovalSet, &id, name)
	w.WriteHeader(http.StatusNoContent)
}

// --- internals ---

func (h *ApprovalSetHandler) decodeForEdit(w http.ResponseWriter, r *http.Request) (*models.User, models.ApprovalSet, bool) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return nil, models.ApprovalSet{}, false
	}
	var input models.ApprovalSet
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondValidationError(w, r, "Invalid request body")
		return nil, models.ApprovalSet{}, false
	}
	if input.Name == "" {
		respondValidationError(w, r, "Name is required")
		return nil, models.ApprovalSet{}, false
	}
	return user, input, true
}

// respondServiceError maps the typed errors that ApprovalSetService returns
// into the right HTTP response.
func (h *ApprovalSetHandler) respondServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		respondNotFound(w, r, "Approval set")
	case errors.Is(err, services.ErrApprovalSetInUseByConfigSet):
		respondValidationError(w, r, "Cannot delete approval set: it is in use by one or more configuration sets")
	case errors.Is(err, services.ErrApprovalSetHasPendingRequests):
		respondConflict(w, r, fmt.Sprintf("Cannot delete approval set: %s", err.Error()))
	case errors.Is(err, services.ErrApprovalSetValidation):
		respondValidationError(w, r, err.Error())
	default:
		respondInternalError(w, r, err)
	}
}
