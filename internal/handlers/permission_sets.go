package handlers

import (
	"errors"
	"fmt"
	"net/http"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/utils"
)

type PermissionSetHandler struct {
	repo              *repository.PermissionSetRepository
	permissionService *services.PermissionService
	auditor           *logger.Auditor
}

func NewPermissionSetHandlerWithPool(
	repo *repository.PermissionSetRepository,
	permissionService *services.PermissionService,
	auditor *logger.Auditor,
) *PermissionSetHandler {
	return &PermissionSetHandler{
		repo:              repo,
		permissionService: permissionService,
		auditor:           auditor,
	}
}

// GetAll returns all permission sets
func (h *PermissionSetHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	permissionSets, err := h.repo.List()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, permissionSets)
}

// Get returns a single permission set with its permissions
func (h *PermissionSetHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	ps, err := h.repo.GetByIDWithPermissions(id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "permission_set")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, ps)
}

// Create creates a new permission set
func (h *PermissionSetHandler) Create(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[models.PermissionSetCreateRequest](w, r)
	if !ok {
		return
	}

	if req.Name == "" {
		respondValidationError(w, r, "Name is required")
		return
	}

	userID := h.getSessionUserID(r)
	if userID == 0 {
		respondUnauthorized(w, r)
		return
	}

	permSetID, err := h.repo.Create(req.Name, req.Description, userID, req.PermissionIDs)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	ps, err := h.repo.GetByID(int(permSetID))
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		psID := int(permSetID)
		h.auditor.LogWithDetails(r, currentUser,
			logger.ActionPermissionSetCreate, logger.ResourcePermissionSet,
			&psID, req.Name,
			map[string]interface{}{
				"description":      req.Description,
				"permission_count": len(req.PermissionIDs),
			},
		)
	}

	respondJSONCreated(w, ps)
}

// Update updates a permission set
func (h *PermissionSetHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	oldPS, err := h.repo.GetByID(id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "permission_set")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	req, ok := decodeJSON[models.PermissionSetUpdateRequest](w, r)
	if !ok {
		return
	}

	userID := h.getSessionUserID(r)

	if err := h.repo.UpdateMetadata(id, req.Name, req.Description); err != nil {
		respondInternalError(w, r, err)
		return
	}

	if err := h.repo.ReplacePermissions(id, req.PermissionIDs, userID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Invalidate cache for all configuration sets using this permission set
	var warnings []models.APIWarning
	if h.permissionService != nil {
		if err := h.permissionService.OnPermissionSetChanged(id); err != nil {
			warnings = append(warnings, createCacheWarning("permission_set", err, fmt.Sprintf("permission_set_id:%d", id)))
		}
	}

	ps, err := h.repo.GetByID(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		details := make(map[string]interface{})

		if oldPS.Name != ps.Name {
			details["name_changed"] = map[string]interface{}{
				"old": oldPS.Name,
				"new": ps.Name,
			}
		}
		if oldPS.Description != ps.Description {
			details["description_changed"] = map[string]interface{}{
				"old": oldPS.Description,
				"new": ps.Description,
			}
		}
		details["permission_count"] = len(req.PermissionIDs)

		h.auditor.LogWithDetails(r, currentUser,
			logger.ActionPermissionSetUpdate, logger.ResourcePermissionSet,
			&id, ps.Name, details,
		)
	}

	respondJSONOKWithWarnings(w, ps, warnings)
}

// Delete deletes a permission set
func (h *PermissionSetHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	oldPS, err := h.repo.GetByID(id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "permission_set")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	usageCount, err := h.repo.CountConfigSetUsage(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if usageCount > 0 {
		respondConflict(w, r, "Cannot delete permission set that is in use by configuration sets")
		return
	}

	if err := h.repo.Delete(id); err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		h.auditor.LogWithDetails(r, currentUser,
			logger.ActionPermissionSetDelete, logger.ResourcePermissionSet,
			&id, oldPS.Name,
			map[string]interface{}{
				"description": oldPS.Description,
				"is_system":   oldPS.IsSystem,
			},
		)
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetAssignments returns all assignments (roles/groups/users) for a permission set
func (h *PermissionSetHandler) GetAssignments(w http.ResponseWriter, r *http.Request) {
	setID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	exists, err := h.repo.Exists(setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !exists {
		respondNotFound(w, r, "permission_set")
		return
	}

	assignments, err := h.repo.ListAssignments(setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, struct {
		RoleAssignments  []models.PermissionSetRoleAssignment  `json:"role_assignments"`
		GroupAssignments []models.PermissionSetGroupAssignment `json:"group_assignments"`
		UserAssignments  []models.PermissionSetUserAssignment  `json:"user_assignments"`
	}{
		RoleAssignments:  assignments.RoleAssignments,
		GroupAssignments: assignments.GroupAssignments,
		UserAssignments:  assignments.UserAssignments,
	})
}

// CreateAssignment adds a role/group/user assignment to a permission in the set
func (h *PermissionSetHandler) CreateAssignment(w http.ResponseWriter, r *http.Request) {
	setID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := decodeJSON[models.PermissionSetAssignmentRequest](w, r)
	if !ok {
		return
	}

	// Validate that exactly one of role/group/user is specified
	count := 0
	var kind repository.AssignmentKind
	var targetID int
	if req.RoleID != nil {
		count++
		kind = repository.AssignmentKindRole
		targetID = *req.RoleID
	}
	if req.GroupID != nil {
		count++
		kind = repository.AssignmentKindGroup
		targetID = *req.GroupID
	}
	if req.UserID != nil {
		count++
		kind = repository.AssignmentKindUser
		targetID = *req.UserID
	}
	if count != 1 {
		respondValidationError(w, r, "Must specify exactly one of role_id, group_id, or user_id")
		return
	}

	createdBy := h.getSessionUserID(r)
	if createdBy == 0 {
		respondUnauthorized(w, r)
		return
	}

	if err := h.repo.CreateAssignment(setID, req.PermissionID, targetID, createdBy, kind); err != nil {
		if errors.Is(err, repository.ErrDuplicateEntry) {
			respondConflict(w, r, "This assignment already exists")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		h.auditor.LogWithDetails(r, currentUser, logger.ActionPermissionSetAssignmentCreate, logger.ResourcePermissionSet, &setID, "", map[string]interface{}{
			"permission_id":   req.PermissionID,
			"assignment_kind": string(kind),
			"target_id":       targetID,
		})
	}

	// Invalidate cache
	var warnings []models.APIWarning
	if h.permissionService != nil {
		if err := h.permissionService.OnPermissionSetChanged(setID); err != nil {
			warnings = append(warnings, createCacheWarning("permission_set", err, fmt.Sprintf("permission_set_id:%d", setID)))
		}
	}

	respondJSONCreatedWithWarnings(w, map[string]bool{"success": true}, warnings)
}

// DeleteAssignment removes an assignment
func (h *PermissionSetHandler) DeleteAssignment(w http.ResponseWriter, r *http.Request) {
	setID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	assignmentID, ok := requireIDParam(w, r, "assignmentId")
	if !ok {
		return
	}

	assignmentType := r.URL.Query().Get("type") // "role", "group", or "user"
	if assignmentType == "" {
		respondValidationError(w, r, "Assignment type parameter required")
		return
	}

	kind := repository.AssignmentKind(assignmentType)
	switch kind {
	case repository.AssignmentKindRole, repository.AssignmentKindGroup, repository.AssignmentKindUser:
		// ok
	default:
		respondValidationError(w, r, "Invalid assignment type")
		return
	}

	if err := h.repo.DeleteAssignment(setID, assignmentID, kind); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "assignment")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		h.auditor.LogWithDetails(r, currentUser, logger.ActionPermissionSetAssignmentDelete, logger.ResourcePermissionSet, &setID, "", map[string]interface{}{
			"assignment_id":   assignmentID,
			"assignment_kind": string(kind),
		})
	}

	// Invalidate cache
	var warnings []models.APIWarning
	if h.permissionService != nil {
		if err := h.permissionService.OnPermissionSetChanged(setID); err != nil {
			warnings = append(warnings, createCacheWarning("permission_set", err, fmt.Sprintf("permission_set_id:%d", setID)))
		}
	}

	if len(warnings) > 0 {
		respondJSONOKWithWarnings(w, map[string]string{"message": "Assignment deleted successfully"}, warnings)
	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

// getSessionUserID extracts user ID from session context
func (h *PermissionSetHandler) getSessionUserID(r *http.Request) int {
	if user := utils.GetCurrentUser(r); user != nil {
		return user.ID
	}
	return 0
}
