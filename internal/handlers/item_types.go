package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/utils"
)

type ItemTypeHandler struct {
	db database.Database
}

func NewItemTypeHandler(db database.Database) *ItemTypeHandler {
	return &ItemTypeHandler{db: db}
}

func (h *ItemTypeHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	// Base query for item types
	query := `
		SELECT it.id, it.name, it.description, it.is_default,
		       it.icon, it.color, it.hierarchy_level, it.sort_order, it.created_at, it.updated_at
		FROM item_types it`

	args := []interface{}{}
	whereClause := ""

	// Filter by configuration set if specified (via junction table)
	if configSetID := r.URL.Query().Get("configuration_set_id"); configSetID != "" {
		query += `
		INNER JOIN configuration_set_item_types csit ON it.id = csit.item_type_id`
		whereClause = " WHERE csit.configuration_set_id = ?"
		args = append(args, configSetID)
	}

	query += whereClause + " ORDER BY it.hierarchy_level, it.sort_order, it.name"

	rows, err := h.db.Query(query, args...)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var itemTypes []models.ItemType
	for rows.Next() {
		var it models.ItemType
		err := rows.Scan(&it.ID, &it.Name, &it.Description, &it.IsDefault,
			&it.Icon, &it.Color, &it.HierarchyLevel, &it.SortOrder, &it.CreatedAt, &it.UpdatedAt)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		// Load configuration set associations from junction table
		configSetQuery := `
			SELECT cs.id, cs.name
			FROM configuration_set_item_types csit
			JOIN configuration_sets cs ON csit.configuration_set_id = cs.id
			WHERE csit.item_type_id = ?
			ORDER BY cs.name`

		configSetRows, err := h.db.Query(configSetQuery, it.ID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		var configSetIDs []int
		var configSetNames []string
		for configSetRows.Next() {
			var configSetID int
			var configSetName string
			if err := configSetRows.Scan(&configSetID, &configSetName); err != nil {
				_ = configSetRows.Close()
				respondInternalError(w, r, err)
				return
			}
			configSetIDs = append(configSetIDs, configSetID)
			configSetNames = append(configSetNames, configSetName)
		}
		_ = configSetRows.Close()

		it.ConfigurationSetIDs = configSetIDs
		it.ConfigurationSetNames = configSetNames

		// For backward compatibility, populate deprecated fields with first config set
		if len(configSetIDs) > 0 {
			it.ConfigurationSetID = configSetIDs[0]
			it.ConfigurationSetName = configSetNames[0]
		}

		itemTypes = append(itemTypes, it)
	}

	if itemTypes == nil {
		itemTypes = []models.ItemType{}
	}

	respondJSONOK(w, itemTypes)
}

func (h *ItemTypeHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var it models.ItemType
	err := h.db.QueryRow(`
		SELECT id, name, description, is_default,
		       icon, color, hierarchy_level, sort_order, created_at, updated_at
		FROM item_types
		WHERE id = ?
	`, id).Scan(&it.ID, &it.Name, &it.Description, &it.IsDefault,
		&it.Icon, &it.Color, &it.HierarchyLevel, &it.SortOrder, &it.CreatedAt, &it.UpdatedAt)

	if err == sql.ErrNoRows {
		respondNotFound(w, r, "item_type")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Load configuration set associations from junction table
	configSetQuery := `
		SELECT cs.id, cs.name
		FROM configuration_set_item_types csit
		JOIN configuration_sets cs ON csit.configuration_set_id = cs.id
		WHERE csit.item_type_id = ?
		ORDER BY cs.name`

	configSetRows, err := h.db.Query(configSetQuery, it.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = configSetRows.Close() }()

	var configSetIDs []int
	var configSetNames []string
	for configSetRows.Next() {
		var configSetID int
		var configSetName string
		if err := configSetRows.Scan(&configSetID, &configSetName); err != nil {
			respondInternalError(w, r, err)
			return
		}
		configSetIDs = append(configSetIDs, configSetID)
		configSetNames = append(configSetNames, configSetName)
	}

	it.ConfigurationSetIDs = configSetIDs
	it.ConfigurationSetNames = configSetNames

	// For backward compatibility, populate deprecated fields with first config set
	if len(configSetIDs) > 0 {
		it.ConfigurationSetID = configSetIDs[0]
		it.ConfigurationSetName = configSetNames[0]
	}

	respondJSONOK(w, it)
}

func (h *ItemTypeHandler) Create(w http.ResponseWriter, r *http.Request) {
	var it models.ItemType
	if err := json.NewDecoder(r.Body).Decode(&it); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Validate required fields
	if strings.TrimSpace(it.Name) == "" {
		respondValidationError(w, r, "Item type name is required")
		return
	}

	// Support both old (single) and new (multiple) configuration set IDs
	configSetIDs := it.ConfigurationSetIDs
	if len(configSetIDs) == 0 && it.ConfigurationSetID != 0 {
		// Backward compatibility: convert single ID to array
		configSetIDs = []int{it.ConfigurationSetID}
	}

	// Verify all configuration sets exist (if any are provided)
	if len(configSetIDs) > 0 {
		for _, csID := range configSetIDs {
			var configSetExists bool
			err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM configuration_sets WHERE id = ?)", csID).Scan(&configSetExists)
			if err != nil || !configSetExists {
				respondValidationError(w, r, fmt.Sprintf("Configuration set %d not found", csID))
				return
			}
		}
	}

	// Check uniqueness before insert
	var nameExists bool
	_ = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM item_types WHERE name = ?)", it.Name).Scan(&nameExists)
	if nameExists {
		respondConflict(w, r, "Item type with this name already exists")
		return
	}

	now := time.Now()
	var id int64
	err := h.db.QueryRow(`
		INSERT INTO item_types (name, description, is_default, icon, color, hierarchy_level, sort_order, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, it.Name, it.Description, it.IsDefault, it.Icon, it.Color, it.HierarchyLevel, it.SortOrder, now, now).Scan(&id)

	if err != nil {
		if database.IsUniqueConstraintError(err) {
			respondConflict(w, r, "Item type with this name already exists")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	// Insert configuration set associations into junction table (if any are provided)
	if len(configSetIDs) > 0 {
		for _, csID := range configSetIDs {
			_, err = h.db.Exec(`
				INSERT INTO configuration_set_item_types (configuration_set_id, item_type_id, created_at)
				VALUES (?, ?, ?)
			`, csID, id, now)
			if err != nil {
				respondInternalError(w, r, fmt.Errorf("failed to associate with configuration set %d: %w", csID, err))
				return
			}
		}
	}

	// Create default screens for the new item type
	h.createDefaultScreens(int(id))

	// Load and return the created item type with configuration sets
	err = h.db.QueryRow(`
		SELECT id, name, description, is_default,
		       icon, color, hierarchy_level, sort_order, created_at, updated_at
		FROM item_types
		WHERE id = ?
	`, id).Scan(&it.ID, &it.Name, &it.Description, &it.IsDefault,
		&it.Icon, &it.Color, &it.HierarchyLevel, &it.SortOrder, &it.CreatedAt, &it.UpdatedAt)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Load configuration set associations
	configSetQuery := `
		SELECT cs.id, cs.name
		FROM configuration_set_item_types csit
		JOIN configuration_sets cs ON csit.configuration_set_id = cs.id
		WHERE csit.item_type_id = ?
		ORDER BY cs.name`

	configSetRows, err := h.db.Query(configSetQuery, it.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = configSetRows.Close() }()

	var configSetIDsResult []int
	var configSetNames []string
	for configSetRows.Next() {
		var configSetID int
		var configSetName string
		if err := configSetRows.Scan(&configSetID, &configSetName); err != nil {
			respondInternalError(w, r, err)
			return
		}
		configSetIDsResult = append(configSetIDsResult, configSetID)
		configSetNames = append(configSetNames, configSetName)
	}

	it.ConfigurationSetIDs = configSetIDsResult
	it.ConfigurationSetNames = configSetNames

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionItemTypeCreate,
			ResourceType: logger.ResourceItemType,
			ResourceID:   &it.ID,
			ResourceName: it.Name,
			Details: map[string]interface{}{
				"icon":                    it.Icon,
				"color":                   it.Color,
				"hierarchy_level":         it.HierarchyLevel,
				"configuration_set_ids":   configSetIDsResult,
				"configuration_set_names": configSetNames,
			},
			Success: true,
		})
	}

	respondJSONCreated(w, it)
}

func (h *ItemTypeHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get the old item type for audit logging
	var oldIT models.ItemType
	err := h.db.QueryRow(`
		SELECT id, name, description, is_default, icon, color, hierarchy_level, sort_order, created_at, updated_at
		FROM item_types
		WHERE id = ?
	`, id).Scan(&oldIT.ID, &oldIT.Name, &oldIT.Description, &oldIT.IsDefault,
		&oldIT.Icon, &oldIT.Color, &oldIT.HierarchyLevel, &oldIT.SortOrder, &oldIT.CreatedAt, &oldIT.UpdatedAt)

	if err == sql.ErrNoRows {
		respondNotFound(w, r, "item_type")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	var it models.ItemType
	if err = json.NewDecoder(r.Body).Decode(&it); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Validate required fields
	if strings.TrimSpace(it.Name) == "" {
		respondValidationError(w, r, "Item type name is required")
		return
	}

	// Check uniqueness before update
	var nameExists bool
	_ = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM item_types WHERE name = ? AND id != ?)", it.Name, id).Scan(&nameExists)
	if nameExists {
		respondConflict(w, r, "Item type with this name already exists")
		return
	}

	now := time.Now()
	_, err = h.db.ExecWrite(`
		UPDATE item_types
		SET name = ?, description = ?, is_default = ?, icon = ?, color = ?, hierarchy_level = ?, sort_order = ?, updated_at = ?
		WHERE id = ?
	`, it.Name, it.Description, it.IsDefault, it.Icon, it.Color, it.HierarchyLevel, it.SortOrder, now, id)

	if err != nil {
		if database.IsUniqueConstraintError(err) {
			respondConflict(w, r, "Item type with this name already exists")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	// Update configuration set associations if provided
	if len(it.ConfigurationSetIDs) > 0 {
		// Verify all configuration sets exist
		for _, csID := range it.ConfigurationSetIDs {
			var configSetExists bool
			err = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM configuration_sets WHERE id = ?)", csID).Scan(&configSetExists)
			if err != nil || !configSetExists {
				respondValidationError(w, r, fmt.Sprintf("Configuration set %d not found", csID))
				return
			}
		}

		// Delete existing associations
		_, err = h.db.ExecWrite("DELETE FROM configuration_set_item_types WHERE item_type_id = ?", id)
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to update configuration set associations: %w", err))
			return
		}

		// Insert new associations
		for _, csID := range it.ConfigurationSetIDs {
			_, err = h.db.ExecWrite(`
				INSERT INTO configuration_set_item_types (configuration_set_id, item_type_id, created_at)
				VALUES (?, ?, ?)
			`, csID, id, now)
			if err != nil {
				respondInternalError(w, r, fmt.Errorf("failed to associate with configuration set %d: %w", csID, err))
				return
			}
		}
	}

	// Load and return the updated item type with configuration sets
	err = h.db.QueryRow(`
		SELECT id, name, description, is_default, icon, color, hierarchy_level, sort_order, created_at, updated_at
		FROM item_types
		WHERE id = ?
	`, id).Scan(&it.ID, &it.Name, &it.Description, &it.IsDefault,
		&it.Icon, &it.Color, &it.HierarchyLevel, &it.SortOrder, &it.CreatedAt, &it.UpdatedAt)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Load configuration set associations
	configSetQuery := `
		SELECT cs.id, cs.name
		FROM configuration_set_item_types csit
		JOIN configuration_sets cs ON csit.configuration_set_id = cs.id
		WHERE csit.item_type_id = ?
		ORDER BY cs.name`

	configSetRows, err := h.db.Query(configSetQuery, it.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = configSetRows.Close() }()

	var configSetIDsResult []int
	var configSetNames []string
	for configSetRows.Next() {
		var configSetID int
		var configSetName string
		if err := configSetRows.Scan(&configSetID, &configSetName); err != nil {
			respondInternalError(w, r, err)
			return
		}
		configSetIDsResult = append(configSetIDsResult, configSetID)
		configSetNames = append(configSetNames, configSetName)
	}

	it.ConfigurationSetIDs = configSetIDsResult
	it.ConfigurationSetNames = configSetNames

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		details := make(map[string]interface{})

		// Track what changed
		if oldIT.Name != it.Name {
			details["name_changed"] = map[string]interface{}{
				"old": oldIT.Name,
				"new": it.Name,
			}
		}
		if oldIT.Icon != it.Icon {
			details["icon_changed"] = map[string]interface{}{
				"old": oldIT.Icon,
				"new": it.Icon,
			}
		}
		if oldIT.Color != it.Color {
			details["color_changed"] = map[string]interface{}{
				"old": oldIT.Color,
				"new": it.Color,
			}
		}
		if oldIT.HierarchyLevel != it.HierarchyLevel {
			details["hierarchy_level_changed"] = map[string]interface{}{
				"old": oldIT.HierarchyLevel,
				"new": it.HierarchyLevel,
			}
		}
		if oldIT.SortOrder != it.SortOrder {
			details["sort_order_changed"] = map[string]interface{}{
				"old": oldIT.SortOrder,
				"new": it.SortOrder,
			}
		}
		if len(it.ConfigurationSetIDs) > 0 {
			details["configuration_sets"] = configSetNames
		}

		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionItemTypeUpdate,
			ResourceType: logger.ResourceItemType,
			ResourceID:   &it.ID,
			ResourceName: it.Name,
			Details:      details,
			Success:      true,
		})
	}

	respondJSONOK(w, it)
}

func (h *ItemTypeHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get the item type details for audit logging
	var itemTypeName string
	var icon string
	var color string
	err := h.db.QueryRow(`
		SELECT name, icon, color
		FROM item_types
		WHERE id = ?
	`, id).Scan(&itemTypeName, &icon, &color)

	if err == sql.ErrNoRows {
		respondNotFound(w, r, "item_type")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	_, err = h.db.ExecWrite("DELETE FROM item_types WHERE id = ?", id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionItemTypeDelete,
			ResourceType: logger.ResourceItemType,
			ResourceID:   &id,
			ResourceName: itemTypeName,
			Details: map[string]interface{}{
				"icon":  icon,
				"color": color,
			},
			Success: true,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *ItemTypeHandler) createDefaultScreens(itemTypeID int) {
	contexts := []string{"create", "edit", "view"}
	for _, context := range contexts {
		_, _ = h.db.ExecWrite(`
			INSERT INTO screens (item_type_id, name, description, screen_type, context, created_at, updated_at)
			VALUES (?, ?, ?, 'issue', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, itemTypeID, context+" Screen", "Default "+context+" screen", context)
	}
}
