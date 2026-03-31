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

	if req.StartDate == "" {
		respondValidationError(w, r, "start_date is required")
		return
	}
	if req.EndDate == "" {
		respondValidationError(w, r, "end_date is required")
		return
	}
	if req.EndDate < req.StartDate {
		respondValidationError(w, r, "end_date must be greater than or equal to start_date")
		return
	}

	if req.SubstituteUserID != nil {
		if *req.SubstituteUserID == userID {
			respondValidationError(w, r, "substitute_user_id cannot be the same as the user")
			return
		}

		var exists bool
		err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE id = ?)", *req.SubstituteUserID).Scan(&exists)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !exists {
			respondValidationError(w, r, "substitute user does not exist")
			return
		}
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
	userID, ok := requireIDParam(w, r, "userId")
	if !ok {
		return
	}

	leaveID, ok := requireIDParam(w, r, "leaveId")
	if !ok {
		return
	}

	if AuthorizeUserRequest(w, r, userID, h.permissionService) == nil {
		return
	}

	leave, err := h.leaveRepo.GetByID(leaveID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "leave period")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	if leave.UserID != userID {
		respondNotFound(w, r, "leave period")
		return
	}

	req, ok := decodeJSON[models.UserLeavePeriodRequest](w, r)
	if !ok {
		return
	}

	if req.StartDate == "" {
		respondValidationError(w, r, "start_date is required")
		return
	}
	if req.EndDate == "" {
		respondValidationError(w, r, "end_date is required")
		return
	}
	if req.EndDate < req.StartDate {
		respondValidationError(w, r, "end_date must be greater than or equal to start_date")
		return
	}

	if req.SubstituteUserID != nil {
		if *req.SubstituteUserID == userID {
			respondValidationError(w, r, "substitute_user_id cannot be the same as the user")
			return
		}

		var exists bool
		err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE id = ?)", *req.SubstituteUserID).Scan(&exists)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !exists {
			respondValidationError(w, r, "substitute user does not exist")
			return
		}
	}

	err = h.leaveRepo.Update(leaveID, req.SubstituteUserID, req.StartDate, req.EndDate, req.Reason)
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
	userID, ok := requireIDParam(w, r, "userId")
	if !ok {
		return
	}

	leaveID, ok := requireIDParam(w, r, "leaveId")
	if !ok {
		return
	}

	if AuthorizeUserRequest(w, r, userID, h.permissionService) == nil {
		return
	}

	leave, err := h.leaveRepo.GetByID(leaveID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "leave period")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	if leave.UserID != userID {
		respondNotFound(w, r, "leave period")
		return
	}

	err = h.leaveRepo.Delete(leaveID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
