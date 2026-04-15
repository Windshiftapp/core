package handlers

import (
	"errors"
	"net/http"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

type LeaveHandler struct {
	db                database.Database
	leaveRepo         *repository.LeaveRepository
	permissionService *services.PermissionService
}

func NewLeaveHandler(db database.Database, leaveRepo *repository.LeaveRepository, permissionService *services.PermissionService) *LeaveHandler {
	return &LeaveHandler{
		db:                db,
		leaveRepo:         leaveRepo,
		permissionService: permissionService,
	}
}

// validateLeaveRequest validates leave period dates and substitute user.
// Returns false and writes an error response on failure.
func (h *LeaveHandler) validateLeaveRequest(w http.ResponseWriter, r *http.Request, req models.UserLeavePeriodRequest, userID int) bool {
	if req.StartDate == "" {
		respondValidationError(w, r, "start_date is required")
		return false
	}
	if req.EndDate == "" {
		respondValidationError(w, r, "end_date is required")
		return false
	}
	if req.EndDate < req.StartDate {
		respondValidationError(w, r, "end_date must be greater than or equal to start_date")
		return false
	}

	if req.SubstituteUserID != nil {
		if *req.SubstituteUserID == userID {
			respondValidationError(w, r, "substitute_user_id cannot be the same as the user")
			return false
		}

		var exists bool
		err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE id = ?)", *req.SubstituteUserID).Scan(&exists)
		if err != nil {
			respondInternalError(w, r, err)
			return false
		}
		if !exists {
			respondValidationError(w, r, "substitute user does not exist")
			return false
		}
	}

	return true
}

// getOwnedLeave fetches a leave period by ID and verifies the user owns it.
// Returns false on failure (response already written).
func (h *LeaveHandler) getOwnedLeave(w http.ResponseWriter, r *http.Request, leaveID, userID int) bool {
	leave, err := h.leaveRepo.GetByID(leaveID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "leave period")
			return false
		}
		respondInternalError(w, r, err)
		return false
	}

	if leave.UserID != userID {
		respondNotFound(w, r, "leave period")
		return false
	}

	return true
}

// requireOwnedLeave parses userID+leaveID, authorizes the request, and verifies ownership.
func (h *LeaveHandler) requireOwnedLeave(w http.ResponseWriter, r *http.Request) (userID, leaveID int, ok bool) {
	userID, ok = requireIDParam(w, r, "userId")
	if !ok {
		return 0, 0, false
	}

	leaveID, ok = requireIDParam(w, r, "leaveId")
	if !ok {
		return 0, 0, false
	}

	if AuthorizeUserRequest(w, r, userID, h.permissionService) == nil {
		return 0, 0, false
	}

	if !h.getOwnedLeave(w, r, leaveID, userID) {
		return 0, 0, false
	}

	return userID, leaveID, true
}

// GetForUser returns all leave periods for a user
func (h *LeaveHandler) GetForUser(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireIDParam(w, r, "userId")
	if !ok {
		return
	}

	if AuthorizeUserRequest(w, r, userID, h.permissionService) == nil {
		return
	}

	periods, err := h.leaveRepo.GetForUser(userID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if periods == nil {
		periods = []models.UserLeavePeriod{}
	}

	respondJSONOK(w, periods)
}

// Create creates a new leave period for a user
func (h *LeaveHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireIDParam(w, r, "userId")
	if !ok {
		return
	}

	if AuthorizeUserRequest(w, r, userID, h.permissionService) == nil {
		return
	}

	req, ok := decodeJSON[models.UserLeavePeriodRequest](w, r)
	if !ok {
		return
	}

	if !h.validateLeaveRequest(w, r, req, userID) {
		return
	}

	id, err := h.leaveRepo.Create(userID, req.SubstituteUserID, req.StartDate, req.EndDate, req.Reason)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	leave, err := h.leaveRepo.GetByID(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONCreated(w, leave)
}

// Update updates an existing leave period for a user
func (h *LeaveHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID, leaveID, ok := h.requireOwnedLeave(w, r)
	if !ok {
		return
	}

	req, ok := decodeJSON[models.UserLeavePeriodRequest](w, r)
	if !ok {
		return
	}

	if !h.validateLeaveRequest(w, r, req, userID) {
		return
	}

	err := h.leaveRepo.Update(leaveID, req.SubstituteUserID, req.StartDate, req.EndDate, req.Reason)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	updated, err := h.leaveRepo.GetByID(leaveID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, updated)
}

// Delete deletes a leave period for a user
func (h *LeaveHandler) Delete(w http.ResponseWriter, r *http.Request) {
	_, leaveID, ok := h.requireOwnedLeave(w, r)
	if !ok {
		return
	}

	err := h.leaveRepo.Delete(leaveID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
