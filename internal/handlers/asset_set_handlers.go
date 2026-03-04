package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/utils"
)

// GetAssetSets returns all asset sets the user has access to
func (h *AssetHandler) GetAssetSets(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return
	}

	// Check if user is system admin
	isAdmin, _ := h.permissionService.HasGlobalPermission(currentUser.ID, "system.admin")

	query := `
		SELECT ams.id, ams.name, ams.description, ams.is_default,
		       ams.created_by, ams.created_at, ams.updated_at,
		       COALESCE(u.first_name || ' ' || u.last_name, u.username, '') as creator_name,
		       (SELECT COUNT(*) FROM asset_types WHERE set_id = ams.id) as asset_type_count,
		       (SELECT COUNT(*) FROM assets WHERE set_id = ams.id) as asset_count
		FROM asset_management_sets ams
		LEFT JOIN users u ON ams.created_by = u.id
	`

	var args []interface{}

	// System admins see all sets, others see only permitted sets
	if !isAdmin {
		query += ` WHERE (
			EXISTS (SELECT 1 FROM user_asset_set_roles WHERE set_id = ams.id AND user_id = ?)
			OR EXISTS (
				SELECT 1 FROM group_asset_set_roles gasr
				JOIN group_members gm ON gasr.group_id = gm.group_id
				WHERE gasr.set_id = ams.id AND gm.user_id = ?
			)
			OR EXISTS (SELECT 1 FROM asset_set_everyone_roles WHERE set_id = ams.id AND role_id IS NOT NULL)
		)`
		args = append(args, currentUser.ID, currentUser.ID)
	}

	query += ` ORDER BY ams.is_default DESC, ams.name`

	rows, err := h.db.Query(query, args...)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var sets []models.AssetManagementSet
	for rows.Next() {
		var set models.AssetManagementSet
		var creatorName sql.NullString
		var description sql.NullString

		err := rows.Scan(
			&set.ID, &set.Name, &description, &set.IsDefault,
			&set.CreatedBy, &set.CreatedAt, &set.UpdatedAt,
			&creatorName, &set.AssetTypeCount, &set.AssetCount,
		)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		set.CreatorName = creatorName.String
		set.Description = description.String

		// Get user's role for this set (stored as UserPermission for backwards compatibility)
		if isAdmin {
			set.UserPermission = AssetRoleAdministrator
		} else {
			set.UserPermission, _ = h.getUserSetRoleName(currentUser.ID, set.ID)
		}

		sets = append(sets, set)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(sets)
}

// GetAssetSet returns a single asset set
func (h *AssetHandler) GetAssetSet(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return
	}

	setID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	// Check permission
	canView, err := h.canViewSet(currentUser.ID, setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canView {
		respondForbidden(w, r)
		return
	}

	var set models.AssetManagementSet
	var creatorName, description sql.NullString

	err = h.db.QueryRow(`
		SELECT ams.id, ams.name, ams.description, ams.is_default,
		       ams.created_by, ams.created_at, ams.updated_at,
		       COALESCE(u.first_name || ' ' || u.last_name, u.username, '') as creator_name,
		       (SELECT COUNT(*) FROM asset_types WHERE set_id = ams.id) as asset_type_count,
		       (SELECT COUNT(*) FROM assets WHERE set_id = ams.id) as asset_count
		FROM asset_management_sets ams
		LEFT JOIN users u ON ams.created_by = u.id
		WHERE ams.id = ?
	`, setID).Scan(
		&set.ID, &set.Name, &description, &set.IsDefault,
		&set.CreatedBy, &set.CreatedAt, &set.UpdatedAt,
		&creatorName, &set.AssetTypeCount, &set.AssetCount,
	)

	if err == sql.ErrNoRows {
		respondNotFound(w, r, "set")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	set.CreatorName = creatorName.String
	set.Description = description.String

	set.UserPermission, _ = h.getUserSetRoleName(currentUser.ID, setID)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(set)
}

// CreateAssetSetRequest represents the request body for creating an asset set
type CreateAssetSetRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsDefault   bool   `json:"is_default"`
}

// CreateAssetSet creates a new asset management set
func (h *AssetHandler) CreateAssetSet(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return
	}

	// Check if user has asset.manage permission or is system admin
	hasPermission, err := h.permissionService.HasGlobalPermission(currentUser.ID, "system.admin")
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !hasPermission {
		hasPermission, err = h.permissionService.HasGlobalPermission(currentUser.ID, "asset.manage")
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
	}
	if !hasPermission {
		respondForbidden(w, r)
		return
	}

	var req CreateAssetSetRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	if req.Name == "" {
		respondValidationError(w, r, "Name is required")
		return
	}

	now := time.Now()

	// If this set is marked as default, unset any existing default
	if req.IsDefault {
		_, err = h.db.ExecWrite("UPDATE asset_management_sets SET is_default = false WHERE is_default = true")
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	var setID int64
	err = h.db.QueryRow(`
		INSERT INTO asset_management_sets (name, description, is_default, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?) RETURNING id
	`, req.Name, req.Description, req.IsDefault, currentUser.ID, now, now).Scan(&setID)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Grant Administrator role to creator
	var adminRoleID int
	err = h.db.QueryRow(`SELECT id FROM asset_roles WHERE name = 'Administrator'`).Scan(&adminRoleID)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to find Administrator role: %w", err))
		return
	}
	_, err = h.db.ExecWrite(`
		INSERT INTO user_asset_set_roles (set_id, user_id, role_id, granted_by, granted_at)
		VALUES (?, ?, ?, ?, ?)
	`, setID, currentUser.ID, adminRoleID, currentUser.ID, now)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Create default statuses for the new set
	if err := h.createDefaultStatuses(int(setID)); err != nil {
		respondInternalError(w, r, err)
		return
	}

	id := int(setID)
	_ = logger.LogAudit(h.db, logger.AuditEvent{
		UserID:       currentUser.ID,
		Username:     currentUser.Username,
		IPAddress:    utils.GetClientIP(r),
		UserAgent:    r.UserAgent(),
		ActionType:   logger.ActionAssetSetCreate,
		ResourceType: logger.ResourceAssetSet,
		ResourceID:   &id,
		ResourceName: req.Name,
		Success:      true,
	})

	// Return the created set
	set := models.AssetManagementSet{
		ID:             int(setID),
		Name:           req.Name,
		Description:    req.Description,
		IsDefault:      req.IsDefault,
		CreatedBy:      &currentUser.ID,
		CreatedAt:      now,
		UpdatedAt:      now,
		UserPermission: "Administrator",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(set)
}

// UpdateAssetSetRequest represents the request body for updating an asset set
type UpdateAssetSetRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsDefault   bool   `json:"is_default"`
}

// UpdateAssetSet updates an asset management set
func (h *AssetHandler) UpdateAssetSet(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return
	}

	setID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	// Check admin permission
	canAdmin, err := h.canAdminSet(currentUser.ID, setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !canAdmin {
		respondForbidden(w, r)
		return
	}

	var req UpdateAssetSetRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	if req.Name == "" {
		respondValidationError(w, r, "Name is required")
		return
	}

	now := time.Now()

	// If this set is marked as default, unset any existing default
	if req.IsDefault {
		_, err = h.db.ExecWrite("UPDATE asset_management_sets SET is_default = false WHERE is_default = true AND id != ?", setID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	result, err := h.db.ExecWrite(`
		UPDATE asset_management_sets
		SET name = ?, description = ?, is_default = ?, updated_at = ?
		WHERE id = ?
	`, req.Name, req.Description, req.IsDefault, now, setID)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondNotFound(w, r, "set")
		return
	}

	_ = logger.LogAudit(h.db, logger.AuditEvent{
		UserID:       currentUser.ID,
		Username:     currentUser.Username,
		IPAddress:    utils.GetClientIP(r),
		UserAgent:    r.UserAgent(),
		ActionType:   logger.ActionAssetSetUpdate,
		ResourceType: logger.ResourceAssetSet,
		ResourceID:   &setID,
		ResourceName: req.Name,
		Success:      true,
	})

	// Return updated set
	var set models.AssetManagementSet
	_ = h.db.QueryRow(`
		SELECT id, name, description, is_default, created_by, created_at, updated_at
		FROM asset_management_sets WHERE id = ?
	`, setID).Scan(&set.ID, &set.Name, &set.Description, &set.IsDefault, &set.CreatedBy, &set.CreatedAt, &set.UpdatedAt)

	set.UserPermission = "Administrator"

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(set)
}

// DeleteAssetSet deletes an asset management set
func (h *AssetHandler) DeleteAssetSet(w http.ResponseWriter, r *http.Request) {
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil {
		respondUnauthorized(w, r)
		return
	}

	setID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	// Only system admins can delete sets
	isAdmin, err := h.permissionService.HasGlobalPermission(currentUser.ID, "system.admin")
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !isAdmin {
		respondAdminRequired(w, r)
		return
	}

	result, err := h.db.ExecWrite("DELETE FROM asset_management_sets WHERE id = ?", setID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondNotFound(w, r, "set")
		return
	}

	_ = logger.LogAudit(h.db, logger.AuditEvent{
		UserID:       currentUser.ID,
		Username:     currentUser.Username,
		IPAddress:    utils.GetClientIP(r),
		UserAgent:    r.UserAgent(),
		ActionType:   logger.ActionAssetSetDelete,
		ResourceType: logger.ResourceAssetSet,
		ResourceID:   &setID,
		Success:      true,
	})

	w.WriteHeader(http.StatusNoContent)
}
