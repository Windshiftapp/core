package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"windshift/internal/constants"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/utils"
)

type StatusHandler struct {
	repo     *repository.StatusRepository
	itemRepo *repository.ItemRepository
	auditor  *logger.Auditor
}

func NewStatusHandler(
	repo *repository.StatusRepository,
	itemRepo *repository.ItemRepository,
	auditor *logger.Auditor,
) *StatusHandler {
	return &StatusHandler{
		repo:     repo,
		itemRepo: itemRepo,
		auditor:  auditor,
	}
}

// validateStatusFields checks required fields and verifies the category exists.
// It writes an HTTP error response and returns false on failure.
func (h *StatusHandler) validateStatusFields(w http.ResponseWriter, r *http.Request, status models.Status) bool {
	if strings.TrimSpace(status.Name) == "" {
		respondValidationError(w, r, "Name is required")
		return false
	}
	if status.CategoryID <= 0 {
		respondValidationError(w, r, "Category ID is required")
		return false
	}
	categoryExists, err := h.repo.CategoryExists(status.CategoryID)
	if err != nil {
		respondInternalError(w, r, err)
		return false
	}
	if !categoryExists {
		respondValidationError(w, r, "Status category not found")
		return false
	}
	return true
}

// GetAll returns all statuses.
//
// deadcode-keep: legacy CRUD endpoint exercised only by
// core-tests/internal/handlers/workflow_components_test.go. The production
// server routes the v1 status handler instead.
func (h *StatusHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	statuses, err := h.repo.List()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, statuses)
}

// Get returns a single status by ID.
//
// deadcode-keep: see StatusHandler.GetAll comment.
func (h *StatusHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	status, err := h.repo.GetByID(id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "status")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, status)
}

// Create creates a new status.
//
// deadcode-keep: see StatusHandler.GetAll comment.
func (h *StatusHandler) Create(w http.ResponseWriter, r *http.Request) {
	status, ok := decodeJSON[models.Status](w, r)
	if !ok {
		return
	}

	if !h.validateStatusFields(w, r, status) {
		return
	}

	exists, err := h.repo.NameExists(status.Name, 0)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if exists {
		respondConflict(w, r, "Status with this name already exists")
		return
	}

	status.Name = utils.SanitizeTitle(status.Name)
	status.Description = utils.SanitizeCommentContent(status.Description)

	id, _, err := h.repo.Create(&status)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	createdStatus, err := h.repo.GetByID(int(id))
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		intID := int(id)
		h.auditor.Log(r, currentUser, logger.ActionStatusCreate, logger.ResourceStatus, &intID, status.Name)
	}

	respondJSONCreated(w, createdStatus)
}

// Update updates an existing status.
//
// deadcode-keep: see StatusHandler.GetAll comment.
func (h *StatusHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	status, ok := decodeJSON[models.Status](w, r)
	if !ok {
		return
	}

	if !h.validateStatusFields(w, r, status) {
		return
	}

	exists, err := h.repo.NameExists(status.Name, id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if exists {
		respondConflict(w, r, "Status with this name already exists")
		return
	}

	status.Name = utils.SanitizeTitle(status.Name)
	status.Description = utils.SanitizeCommentContent(status.Description)

	if err := h.repo.Update(id, &status); err != nil {
		respondInternalError(w, r, err)
		return
	}

	updatedStatus, err := h.repo.GetByID(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		h.auditor.Log(r, currentUser, logger.ActionStatusUpdate, logger.ResourceStatus, &id, updatedStatus.Name)
	}

	respondJSONOK(w, updatedStatus)
}

// Delete deletes a status by ID.
//
// deadcode-keep: see StatusHandler.GetAll comment.
func (h *StatusHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Protect system-critical statuses from deletion
	if id == constants.StatusIDOpen || id == constants.StatusIDDone {
		respondForbidden(w, r)
		return
	}

	transitionCount, err := h.repo.CountTransitionsUsing(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if transitionCount > 0 {
		respondConflict(w, r, "Cannot delete status that is in use by workflow transitions")
		return
	}

	itemCount, err := h.itemRepo.CountByField("status_id", id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if itemCount > 0 {
		respondConflict(w, r, "Cannot delete status that is in use by "+strconv.Itoa(itemCount)+" work item(s)")
		return
	}

	if err := h.repo.Delete(id); err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		h.auditor.Log(r, currentUser, logger.ActionStatusDelete, logger.ResourceStatus, &id, "")
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetNonDoneStatusIDs returns the IDs of statuses that are not in "Done" category
func (h *StatusHandler) GetNonDoneStatusIDs(w http.ResponseWriter, r *http.Request) {
	ids, err := h.repo.ListNonDoneIDs()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, ids)
}
