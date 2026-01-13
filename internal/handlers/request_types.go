package handlers

import (
	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/utils"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

)

type RequestTypeHandler struct {
	db database.Database
}

func NewRequestTypeHandler(db database.Database) *RequestTypeHandler {
	return &RequestTypeHandler{db: db}
}

// GetAllForChannel returns all request types for a specific channel
func (h *RequestTypeHandler) GetAllForChannel(w http.ResponseWriter, r *http.Request) {
	channelID, err := strconv.Atoi(r.PathValue("channel_id"))
	if err != nil {
		http.Error(w, "Invalid channel ID", http.StatusBadRequest)
		return
	}

	query := `
		SELECT rt.id, rt.channel_id, rt.name, rt.description, rt.item_type_id,
		       rt.icon, rt.color, rt.display_order, rt.is_active, rt.created_at, rt.updated_at,
		       c.name as channel_name, it.name as item_type_name
		FROM request_types rt
		LEFT JOIN channels c ON rt.channel_id = c.id
		LEFT JOIN item_types it ON rt.item_type_id = it.id
		WHERE rt.channel_id = ?
		ORDER BY rt.display_order, rt.name`

	rows, err := h.db.Query(query, channelID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var requestTypes []models.RequestType
	for rows.Next() {
		var rt models.RequestType
		err := rows.Scan(&rt.ID, &rt.ChannelID, &rt.Name, &rt.Description, &rt.ItemTypeID,
			&rt.Icon, &rt.Color, &rt.DisplayOrder, &rt.IsActive, &rt.CreatedAt, &rt.UpdatedAt,
			&rt.ChannelName, &rt.ItemTypeName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		requestTypes = append(requestTypes, rt)
	}

	if requestTypes == nil {
		requestTypes = []models.RequestType{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(requestTypes)
}

// Get returns a specific request type by ID
func (h *RequestTypeHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	var rt models.RequestType
	err = h.db.QueryRow(`
		SELECT rt.id, rt.channel_id, rt.name, rt.description, rt.item_type_id,
		       rt.icon, rt.color, rt.display_order, rt.is_active, rt.created_at, rt.updated_at,
		       c.name as channel_name, it.name as item_type_name
		FROM request_types rt
		LEFT JOIN channels c ON rt.channel_id = c.id
		LEFT JOIN item_types it ON rt.item_type_id = it.id
		WHERE rt.id = ?
	`, id).Scan(&rt.ID, &rt.ChannelID, &rt.Name, &rt.Description, &rt.ItemTypeID,
		&rt.Icon, &rt.Color, &rt.DisplayOrder, &rt.IsActive, &rt.CreatedAt, &rt.UpdatedAt,
		&rt.ChannelName, &rt.ItemTypeName)

	if err == sql.ErrNoRows {
		http.Error(w, "Request type not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rt)
}

// Create creates a new request type
func (h *RequestTypeHandler) Create(w http.ResponseWriter, r *http.Request) {
	channelID, err := strconv.Atoi(r.PathValue("channel_id"))
	if err != nil {
		http.Error(w, "Invalid channel ID", http.StatusBadRequest)
		return
	}

	var rt models.RequestType
	if err := json.NewDecoder(r.Body).Decode(&rt); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Set channel ID from URL
	rt.ChannelID = channelID

	// Validate required fields
	if strings.TrimSpace(rt.Name) == "" {
		http.Error(w, "Request type name is required", http.StatusBadRequest)
		return
	}
	if rt.ItemTypeID == 0 {
		http.Error(w, "Item type ID is required", http.StatusBadRequest)
		return
	}

	// Verify channel exists
	var channelExists bool
	err = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM channels WHERE id = ?)", rt.ChannelID).Scan(&channelExists)
	if err != nil || !channelExists {
		http.Error(w, "Channel not found", http.StatusBadRequest)
		return
	}

	// Verify item type exists
	var itemTypeExists bool
	err = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM item_types WHERE id = ?)", rt.ItemTypeID).Scan(&itemTypeExists)
	if err != nil || !itemTypeExists {
		http.Error(w, "Item type not found", http.StatusBadRequest)
		return
	}

	// Set default values if not provided
	if rt.Icon == "" {
		rt.Icon = "FileText"
	}
	if rt.Color == "" {
		rt.Color = "#3b82f6"
	}
	if rt.DisplayOrder == 0 {
		// Get next display order
		var maxOrder int
		h.db.QueryRow("SELECT COALESCE(MAX(display_order), 0) FROM request_types WHERE channel_id = ?", rt.ChannelID).Scan(&maxOrder)
		rt.DisplayOrder = maxOrder + 1
	}

	now := time.Now()
	var id int64
	err = h.db.QueryRow(`
		INSERT INTO request_types (channel_id, name, description, item_type_id, icon, color, display_order, is_active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, rt.ChannelID, rt.Name, rt.Description, rt.ItemTypeID, rt.Icon, rt.Color, rt.DisplayOrder, rt.IsActive, now, now).Scan(&id)

	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			http.Error(w, "Request type with this name already exists for this channel", http.StatusConflict)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// Return the created request type
	err = h.db.QueryRow(`
		SELECT rt.id, rt.channel_id, rt.name, rt.description, rt.item_type_id,
		       rt.icon, rt.color, rt.display_order, rt.is_active, rt.created_at, rt.updated_at,
		       c.name as channel_name, it.name as item_type_name
		FROM request_types rt
		LEFT JOIN channels c ON rt.channel_id = c.id
		LEFT JOIN item_types it ON rt.item_type_id = it.id
		WHERE rt.id = ?
	`, id).Scan(&rt.ID, &rt.ChannelID, &rt.Name, &rt.Description, &rt.ItemTypeID,
		&rt.Icon, &rt.Color, &rt.DisplayOrder, &rt.IsActive, &rt.CreatedAt, &rt.UpdatedAt,
		&rt.ChannelName, &rt.ItemTypeName)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   "request_type_create",
			ResourceType: "request_type",
			ResourceID:   &rt.ID,
			ResourceName: rt.Name,
			Details: map[string]interface{}{
				"channel_id":   rt.ChannelID,
				"item_type_id": rt.ItemTypeID,
				"icon":         rt.Icon,
				"color":        rt.Color,
			},
			Success: true,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(rt)
}

// Update updates an existing request type
func (h *RequestTypeHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	// Get the old request type for audit logging
	var oldRT models.RequestType
	err = h.db.QueryRow(`
		SELECT rt.id, rt.channel_id, rt.name, rt.description, rt.item_type_id,
		       rt.icon, rt.color, rt.display_order, rt.is_active, rt.created_at, rt.updated_at,
		       c.name as channel_name, it.name as item_type_name
		FROM request_types rt
		LEFT JOIN channels c ON rt.channel_id = c.id
		LEFT JOIN item_types it ON rt.item_type_id = it.id
		WHERE rt.id = ?
	`, id).Scan(&oldRT.ID, &oldRT.ChannelID, &oldRT.Name, &oldRT.Description, &oldRT.ItemTypeID,
		&oldRT.Icon, &oldRT.Color, &oldRT.DisplayOrder, &oldRT.IsActive, &oldRT.CreatedAt, &oldRT.UpdatedAt,
		&oldRT.ChannelName, &oldRT.ItemTypeName)

	if err == sql.ErrNoRows {
		http.Error(w, "Request type not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var rt models.RequestType
	if err := json.NewDecoder(r.Body).Decode(&rt); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if strings.TrimSpace(rt.Name) == "" {
		http.Error(w, "Request type name is required", http.StatusBadRequest)
		return
	}
	if rt.ItemTypeID == 0 {
		http.Error(w, "Item type ID is required", http.StatusBadRequest)
		return
	}

	// Verify item type exists
	var itemTypeExists bool
	err = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM item_types WHERE id = ?)", rt.ItemTypeID).Scan(&itemTypeExists)
	if err != nil || !itemTypeExists {
		http.Error(w, "Item type not found", http.StatusBadRequest)
		return
	}

	now := time.Now()
	_, err = h.db.ExecWrite(`
		UPDATE request_types
		SET name = ?, description = ?, item_type_id = ?, icon = ?, color = ?, display_order = ?, is_active = ?, updated_at = ?
		WHERE id = ?
	`, rt.Name, rt.Description, rt.ItemTypeID, rt.Icon, rt.Color, rt.DisplayOrder, rt.IsActive, now, id)

	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			http.Error(w, "Request type with this name already exists for this channel", http.StatusConflict)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// Return the updated request type
	err = h.db.QueryRow(`
		SELECT rt.id, rt.channel_id, rt.name, rt.description, rt.item_type_id,
		       rt.icon, rt.color, rt.display_order, rt.is_active, rt.created_at, rt.updated_at,
		       c.name as channel_name, it.name as item_type_name
		FROM request_types rt
		LEFT JOIN channels c ON rt.channel_id = c.id
		LEFT JOIN item_types it ON rt.item_type_id = it.id
		WHERE rt.id = ?
	`, id).Scan(&rt.ID, &rt.ChannelID, &rt.Name, &rt.Description, &rt.ItemTypeID,
		&rt.Icon, &rt.Color, &rt.DisplayOrder, &rt.IsActive, &rt.CreatedAt, &rt.UpdatedAt,
		&rt.ChannelName, &rt.ItemTypeName)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		details := make(map[string]interface{})

		// Track what changed
		if oldRT.Name != rt.Name {
			details["name_changed"] = map[string]interface{}{
				"old": oldRT.Name,
				"new": rt.Name,
			}
		}
		if oldRT.ItemTypeID != rt.ItemTypeID {
			details["item_type_changed"] = map[string]interface{}{
				"old": oldRT.ItemTypeID,
				"new": rt.ItemTypeID,
			}
		}
		if oldRT.Icon != rt.Icon {
			details["icon_changed"] = map[string]interface{}{
				"old": oldRT.Icon,
				"new": rt.Icon,
			}
		}
		if oldRT.Color != rt.Color {
			details["color_changed"] = map[string]interface{}{
				"old": oldRT.Color,
				"new": rt.Color,
			}
		}

		logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   "request_type_update",
			ResourceType: "request_type",
			ResourceID:   &rt.ID,
			ResourceName: rt.Name,
			Details:      details,
			Success:      true,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rt)
}

// Delete deletes a request type
func (h *RequestTypeHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	// Get the request type details for audit logging
	var requestTypeName string
	var channelID int
	err = h.db.QueryRow(`
		SELECT name, channel_id
		FROM request_types
		WHERE id = ?
	`, id).Scan(&requestTypeName, &channelID)

	if err == sql.ErrNoRows {
		http.Error(w, "Request type not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Clean up portal sections: remove this request type ID from all sections
	var configStr string
	err = h.db.QueryRow("SELECT config FROM channels WHERE id = ?", channelID).Scan(&configStr)
	if err == nil && configStr != "" {
		var config models.ChannelConfig
		if err := json.Unmarshal([]byte(configStr), &config); err == nil {
			// Remove the request type ID from all portal sections
			modified := false
			for i := range config.PortalSections {
				newIDs := []int{}
				for _, rtID := range config.PortalSections[i].RequestTypeIDs {
					if rtID != id {
						newIDs = append(newIDs, rtID)
					} else {
						modified = true
					}
				}
				config.PortalSections[i].RequestTypeIDs = newIDs
			}

			// Update the config if we made changes
			if modified {
				updatedConfigJSON, err := json.Marshal(config)
				if err == nil {
					_, _ = h.db.ExecWrite("UPDATE channels SET config = ?, updated_at = ? WHERE id = ?",
						string(updatedConfigJSON), time.Now(), channelID)
				}
			}
		}
	}

	// Delete related fields first (cascade)
	_, err = h.db.ExecWrite("DELETE FROM request_type_fields WHERE request_type_id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Delete the request type
	_, err = h.db.ExecWrite("DELETE FROM request_types WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   "request_type_delete",
			ResourceType: "request_type",
			ResourceID:   &id,
			ResourceName: requestTypeName,
			Details: map[string]interface{}{
				"channel_id": channelID,
			},
			Success: true,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetFields returns all fields for a request type
func (h *RequestTypeHandler) GetFields(w http.ResponseWriter, r *http.Request) {
	requestTypeID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid request type ID", http.StatusBadRequest)
		return
	}

	query := `
		SELECT rtf.id, rtf.request_type_id, rtf.field_identifier, rtf.field_type,
		       rtf.display_order, rtf.is_required, rtf.display_name, rtf.description,
		       COALESCE(rtf.step_number, 1) as step_number,
		       rtf.virtual_field_type, rtf.virtual_field_options,
		       rtf.created_at, rtf.updated_at,
		       CASE
		           WHEN rtf.field_type = 'virtual' THEN rtf.field_identifier
		           ELSE COALESCE(cfd.name, rtf.field_identifier)
		       END as field_name,
		       CASE
		           WHEN rtf.display_name IS NOT NULL AND rtf.display_name != '' THEN rtf.display_name
		           WHEN rtf.field_type = 'virtual' THEN rtf.field_identifier
		           ELSE COALESCE(cfd.name, rtf.field_identifier)
		       END as field_label
		FROM request_type_fields rtf
		LEFT JOIN custom_field_definitions cfd ON rtf.field_type = 'custom' AND rtf.field_identifier = CAST(cfd.id AS TEXT)
		WHERE rtf.request_type_id = ?
		ORDER BY rtf.step_number, rtf.display_order, rtf.id`

	rows, err := h.db.Query(query, requestTypeID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var fields []models.RequestTypeField
	for rows.Next() {
		var field models.RequestTypeField
		err := rows.Scan(&field.ID, &field.RequestTypeID, &field.FieldIdentifier, &field.FieldType,
			&field.DisplayOrder, &field.IsRequired, &field.DisplayName, &field.Description,
			&field.StepNumber, &field.VirtualFieldType, &field.VirtualFieldOptions,
			&field.CreatedAt, &field.UpdatedAt,
			&field.FieldName, &field.FieldLabel)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fields = append(fields, field)
	}

	if fields == nil {
		fields = []models.RequestTypeField{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(fields)
}

// UpdateFields updates the fields for a request type
func (h *RequestTypeHandler) UpdateFields(w http.ResponseWriter, r *http.Request) {
	requestTypeID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid request type ID", http.StatusBadRequest)
		return
	}

	// Verify request type exists
	var requestTypeExists bool
	err = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM request_types WHERE id = ?)", requestTypeID).Scan(&requestTypeExists)
	if err != nil || !requestTypeExists {
		http.Error(w, "Request type not found", http.StatusNotFound)
		return
	}

	var fields []models.RequestTypeField
	if err := json.NewDecoder(r.Body).Decode(&fields); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Delete existing fields
	_, err = h.db.ExecWrite("DELETE FROM request_type_fields WHERE request_type_id = ?", requestTypeID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Insert new fields
	now := time.Now()
	for _, field := range fields {
		// Default step_number to 1 if not set
		stepNumber := field.StepNumber
		if stepNumber == 0 {
			stepNumber = 1
		}

		_, err = h.db.ExecWrite(`
			INSERT INTO request_type_fields (request_type_id, field_identifier, field_type, display_order, is_required,
			                                  display_name, description, step_number, virtual_field_type, virtual_field_options,
			                                  created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, requestTypeID, field.FieldIdentifier, field.FieldType, field.DisplayOrder, field.IsRequired,
			field.DisplayName, field.Description, stepNumber, field.VirtualFieldType, field.VirtualFieldOptions,
			now, now)

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   "request_type_fields_update",
			ResourceType: "request_type",
			ResourceID:   &requestTypeID,
			Details: map[string]interface{}{
				"field_count": len(fields),
			},
			Success: true,
		})
	}

	// Return the updated fields
	h.GetFields(w, r)
}
