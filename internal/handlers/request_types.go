package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/utils"
)

// serializeIntArray converts a slice of ints to a JSON string pointer
// Returns nil if the slice is empty or nil
func serializeIntArray(ids []int) *string {
	if len(ids) == 0 {
		return nil
	}
	data, err := json.Marshal(ids)
	if err != nil {
		return nil
	}
	s := string(data)
	return &s
}

// deserializeIntArray converts a JSON string pointer to a slice of ints
// Returns nil if the string is nil or empty
func deserializeIntArray(s *string) []int {
	if s == nil || *s == "" {
		return nil
	}
	var ids []int
	if err := json.Unmarshal([]byte(*s), &ids); err != nil {
		return nil
	}
	return ids
}

type RequestTypeHandler struct {
	*BaseHandler
}

func NewRequestTypeHandler(db database.Database) *RequestTypeHandler {
	return &RequestTypeHandler{BaseHandler: NewBaseHandler(db)}
}

// requestTypeSelectColumns is the shared SELECT column list for request type queries.
const requestTypeSelectColumns = `
	rt.id, rt.channel_id, rt.name, rt.description, rt.item_type_id,
	rt.icon, rt.color, rt.display_order, rt.is_active,
	rt.visibility_group_ids, rt.visibility_org_ids, rt.workspace_id,
	rt.created_at, rt.updated_at,
	c.name as channel_name, it.name as item_type_name`

// requestTypeFromJoins is the shared FROM + JOIN clause for request type queries.
const requestTypeFromJoins = `
	FROM request_types rt
	LEFT JOIN channels c ON rt.channel_id = c.id
	LEFT JOIN item_types it ON rt.item_type_id = it.id`

// scanRequestType scans a single row into a RequestType, handling the visibility JSON columns.
func scanRequestType(scanner interface {
	Scan(dest ...interface{}) error
}) (models.RequestType, error) {
	var rt models.RequestType
	var visibilityGroupIDs, visibilityOrgIDs *string
	err := scanner.Scan(&rt.ID, &rt.ChannelID, &rt.Name, &rt.Description, &rt.ItemTypeID,
		&rt.Icon, &rt.Color, &rt.DisplayOrder, &rt.IsActive,
		&visibilityGroupIDs, &visibilityOrgIDs, &rt.WorkspaceID,
		&rt.CreatedAt, &rt.UpdatedAt,
		&rt.ChannelName, &rt.ItemTypeName)
	if err != nil {
		return rt, err
	}
	rt.VisibilityGroupIDs = deserializeIntArray(visibilityGroupIDs)
	rt.VisibilityOrgIDs = deserializeIntArray(visibilityOrgIDs)
	return rt, nil
}

// fetchRequestType loads a single RequestType by ID, including joined channel and item type names.
func (h *RequestTypeHandler) fetchRequestType(id int) (*models.RequestType, error) {
	row := h.db.QueryRow(`SELECT`+requestTypeSelectColumns+requestTypeFromJoins+`
		WHERE rt.id = ?`, id)
	rt, err := scanRequestType(row)
	if err != nil {
		return nil, err
	}
	return &rt, nil
}

// GetAllForChannel returns all request types for a specific channel
func (h *RequestTypeHandler) GetAllForChannel(w http.ResponseWriter, r *http.Request) {
	channelID, ok := requireIDParam(w, r, "channel_id")
	if !ok {
		return
	}

	db, ok := h.requireReadDB(w, r)
	if !ok {
		return
	}

	query := `SELECT` + requestTypeSelectColumns + requestTypeFromJoins + `
		WHERE rt.channel_id = ?
		ORDER BY rt.display_order, rt.name`

	rows, err := db.Query(query, channelID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var requestTypes []models.RequestType
	for rows.Next() {
		rt, scanErr := scanRequestType(rows)
		if scanErr != nil {
			respondInternalError(w, r, scanErr)
			return
		}
		requestTypes = append(requestTypes, rt)
	}

	if requestTypes == nil {
		requestTypes = []models.RequestType{}
	}

	respondJSONOK(w, requestTypes)
}

// Get returns a specific request type by ID
func (h *RequestTypeHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	rt, err := h.fetchRequestType(id)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "request_type")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, rt)
}

// Create creates a new request type
func (h *RequestTypeHandler) Create(w http.ResponseWriter, r *http.Request) {
	channelID, ok := requireIDParam(w, r, "channel_id")
	if !ok {
		return
	}

	var err error

	rt, ok := decodeJSON[models.RequestType](w, r)
	if !ok {
		return
	}

	// Set channel ID from URL
	rt.ChannelID = channelID

	// Validate required fields
	if strings.TrimSpace(rt.Name) == "" {
		respondValidationError(w, r, "Request type name is required")
		return
	}
	if rt.ItemTypeID == 0 {
		respondValidationError(w, r, "Item type ID is required")
		return
	}

	// Verify channel exists
	var channelExists bool
	err = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM channels WHERE id = ?)", rt.ChannelID).Scan(&channelExists)
	if err != nil || !channelExists {
		respondValidationError(w, r, "Channel not found")
		return
	}

	// Verify item type exists
	var itemTypeExists bool
	err = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM item_types WHERE id = ?)", rt.ItemTypeID).Scan(&itemTypeExists)
	if err != nil || !itemTypeExists {
		respondValidationError(w, r, "Item type not found")
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
		_ = h.db.QueryRow("SELECT COALESCE(MAX(display_order), 0) FROM request_types WHERE channel_id = ?", rt.ChannelID).Scan(&maxOrder)
		rt.DisplayOrder = maxOrder + 1
	}

	// Check uniqueness before insert
	var nameExists bool
	_ = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM request_types WHERE name = ? AND channel_id = ?)", rt.Name, rt.ChannelID).Scan(&nameExists)
	if nameExists {
		respondConflict(w, r, "Request type with this name already exists for this channel")
		return
	}

	now := time.Now()
	var id int64
	err = h.db.QueryRow(`
		INSERT INTO request_types (channel_id, name, description, item_type_id, icon, color, display_order, is_active, visibility_group_ids, visibility_org_ids, workspace_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, rt.ChannelID, rt.Name, rt.Description, rt.ItemTypeID, rt.Icon, rt.Color, rt.DisplayOrder, rt.IsActive,
		serializeIntArray(rt.VisibilityGroupIDs), serializeIntArray(rt.VisibilityOrgIDs), rt.WorkspaceID, now, now).Scan(&id)

	if err != nil {
		if database.IsUniqueConstraintError(err) {
			respondConflict(w, r, "Request type with this name already exists for this channel")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	// Return the created request type
	created, err := h.fetchRequestType(int(id))
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	rt = *created

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
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

	respondJSONCreated(w, rt)
}

// Update updates an existing request type
func (h *RequestTypeHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var err error

	// Get the old request type for audit logging
	var oldName, oldIcon, oldColor string
	var oldItemTypeID int
	err = h.db.QueryRow(`SELECT name, item_type_id, icon, color FROM request_types WHERE id = ?`, id).
		Scan(&oldName, &oldItemTypeID, &oldIcon, &oldColor)

	if err == sql.ErrNoRows {
		respondNotFound(w, r, "request_type")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	rt, ok := decodeJSON[models.RequestType](w, r)
	if !ok {
		return
	}

	// Validate required fields
	if strings.TrimSpace(rt.Name) == "" {
		respondValidationError(w, r, "Request type name is required")
		return
	}
	if rt.ItemTypeID == 0 {
		respondValidationError(w, r, "Item type ID is required")
		return
	}

	// Verify item type exists
	var itemTypeExists bool
	err = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM item_types WHERE id = ?)", rt.ItemTypeID).Scan(&itemTypeExists)
	if err != nil || !itemTypeExists {
		respondValidationError(w, r, "Item type not found")
		return
	}

	// Check uniqueness before update
	var nameExists bool
	_ = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM request_types WHERE name = ? AND channel_id = (SELECT channel_id FROM request_types WHERE id = ?) AND id != ?)", rt.Name, id, id).Scan(&nameExists)
	if nameExists {
		respondConflict(w, r, "Request type with this name already exists for this channel")
		return
	}

	now := time.Now()
	_, err = h.db.ExecWrite(`
		UPDATE request_types
		SET name = ?, description = ?, item_type_id = ?, icon = ?, color = ?, display_order = ?, is_active = ?,
		    visibility_group_ids = ?, visibility_org_ids = ?, workspace_id = ?, updated_at = ?
		WHERE id = ?
	`, rt.Name, rt.Description, rt.ItemTypeID, rt.Icon, rt.Color, rt.DisplayOrder, rt.IsActive,
		serializeIntArray(rt.VisibilityGroupIDs), serializeIntArray(rt.VisibilityOrgIDs), rt.WorkspaceID, now, id)

	if err != nil {
		if database.IsUniqueConstraintError(err) {
			respondConflict(w, r, "Request type with this name already exists for this channel")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	// Return the updated request type
	updated, err := h.fetchRequestType(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	rt = *updated

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		details := make(map[string]interface{})

		// Track what changed
		if oldName != rt.Name {
			details["name_changed"] = map[string]interface{}{
				"old": oldName,
				"new": rt.Name,
			}
		}
		if oldItemTypeID != rt.ItemTypeID {
			details["item_type_changed"] = map[string]interface{}{
				"old": oldItemTypeID,
				"new": rt.ItemTypeID,
			}
		}
		if oldIcon != rt.Icon {
			details["icon_changed"] = map[string]interface{}{
				"old": oldIcon,
				"new": rt.Icon,
			}
		}
		if oldColor != rt.Color {
			details["color_changed"] = map[string]interface{}{
				"old": oldColor,
				"new": rt.Color,
			}
		}

		_ = logger.LogAudit(h.db, logger.AuditEvent{
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

	respondJSONOK(w, rt)
}

// Delete deletes a request type
func (h *RequestTypeHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var err error

	// Get the request type details for audit logging
	var requestTypeName string
	var channelID int
	err = h.db.QueryRow(`
		SELECT name, channel_id
		FROM request_types
		WHERE id = ?
	`, id).Scan(&requestTypeName, &channelID)

	if err == sql.ErrNoRows {
		respondNotFound(w, r, "request_type")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Clean up portal sections: remove this request type ID from all sections
	removeIDFromPortalSections(h.db, channelID, id,
		func(s *models.PortalSection) []int { return s.RequestTypeIDs },
		func(s *models.PortalSection, ids []int) { s.RequestTypeIDs = ids },
	)

	// Delete related fields first (cascade)
	_, err = h.db.ExecWrite("DELETE FROM request_type_fields WHERE request_type_id = ?", id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Delete the request type
	_, err = h.db.ExecWrite("DELETE FROM request_types WHERE id = ?", id)
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
	requestTypeID, ok := requireIDParam(w, r, "id")
	if !ok {
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
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var fields []models.RequestTypeField
	for rows.Next() {
		var field models.RequestTypeField
		err := rows.Scan(&field.ID, &field.RequestTypeID, &field.FieldIdentifier, &field.FieldType,
			&field.DisplayOrder, &field.IsRequired, &field.DisplayName, &field.Description,
			&field.StepNumber, &field.VirtualFieldType, &field.VirtualFieldOptions,
			&field.CreatedAt, &field.UpdatedAt,
			&field.FieldName, &field.FieldLabel)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		fields = append(fields, field)
	}

	if fields == nil {
		fields = []models.RequestTypeField{}
	}

	respondJSONOK(w, fields)
}

// UpdateFields updates the fields for a request type
func (h *RequestTypeHandler) UpdateFields(w http.ResponseWriter, r *http.Request) {
	requestTypeID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var err error

	// Verify request type exists
	var requestTypeExists bool
	err = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM request_types WHERE id = ?)", requestTypeID).Scan(&requestTypeExists)
	if err != nil || !requestTypeExists {
		respondNotFound(w, r, "request_type")
		return
	}

	fields, ok := decodeJSON[[]models.RequestTypeField](w, r)
	if !ok {
		return
	}

	// Delete existing fields
	_, err = h.db.ExecWrite("DELETE FROM request_type_fields WHERE request_type_id = ?", requestTypeID)
	if err != nil {
		respondInternalError(w, r, err)
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
			respondInternalError(w, r, err)
			return
		}
	}

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
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

// GetAvailableFields returns all fields available for a request type based on its item type and workspace.
// Resolves fields via: workspace → workspace_configuration_sets → configuration_set_item_types → create_screen → screen_fields.
// Falls back to default fields (title, description) when workspace_id is not set or no screen is found.
func (h *RequestTypeHandler) GetAvailableFields(w http.ResponseWriter, r *http.Request) {
	requestTypeID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get the request type to find its item_type_id and workspace_id
	var itemTypeID int
	var workspaceID *int
	err := h.db.QueryRow("SELECT item_type_id, workspace_id FROM request_types WHERE id = ?", requestTypeID).Scan(&itemTypeID, &workspaceID)
	if err == sql.ErrNoRows {
		respondNotFound(w, r, "request_type")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	type AvailableField struct {
		Identifier string `json:"identifier"`
		Name       string `json:"name"`
		Type       string `json:"type"` // "default" or "custom"
		FieldType  string `json:"field_type,omitempty"`
	}

	// Always include default fields
	fields := []AvailableField{
		{Identifier: "title", Name: "Title", Type: "default"},
		{Identifier: "description", Name: "Description", Type: "default"},
	}

	// If no workspace_id, return only defaults
	if workspaceID == nil {
		respondJSONOK(w, fields)
		return
	}

	// Resolve create_screen_id via workspace config sets → item type mapping
	var createScreenID *int
	err = h.db.QueryRow(`
		SELECT csit.create_screen_id
		FROM workspace_configuration_sets wcs
		JOIN configuration_set_item_types csit
		  ON csit.configuration_set_id = wcs.configuration_set_id
		WHERE wcs.workspace_id = ? AND csit.item_type_id = ?
		LIMIT 1
	`, *workspaceID, itemTypeID).Scan(&createScreenID)
	if err == sql.ErrNoRows || createScreenID == nil {
		// No screen mapping found, return only defaults
		respondJSONOK(w, fields)
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Query screen_fields joined with custom_field_definitions (same pattern as screens.go getScreenFields)
	rows, err := h.db.Query(`
		SELECT sf.field_type, sf.field_identifier,
		       CASE
		           WHEN sf.field_type = 'custom' THEN cfd.name
		           ELSE ''
		       END as field_name,
		       CASE
		           WHEN sf.field_type = 'custom' THEN cfd.field_type
		           ELSE ''
		       END as custom_field_type
		FROM screen_fields sf
		LEFT JOIN custom_field_definitions cfd ON sf.field_type = 'custom' AND (CASE WHEN sf.field_type = 'custom' THEN CAST(sf.field_identifier AS INTEGER) END) = cfd.id
		WHERE sf.screen_id = ?
		ORDER BY sf.display_order, sf.id
	`, *createScreenID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var sfType, sfIdentifier, fieldName, customFieldType string
		if err := rows.Scan(&sfType, &sfIdentifier, &fieldName, &customFieldType); err != nil {
			respondInternalError(w, r, err)
			return
		}

		if sfType == "custom" {
			fields = append(fields, AvailableField{
				Identifier: sfIdentifier,
				Name:       fieldName,
				Type:       "custom",
				FieldType:  customFieldType,
			})
		}
		// System fields from the screen are already covered by the defaults above
	}

	respondJSONOK(w, fields)
}

// UpdateVisibility updates only the visibility settings for a request type
func (h *RequestTypeHandler) UpdateVisibility(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	if !decodeAndUpdateVisibility(w, r, h.db, "request_types", "request_type", id) {
		return
	}

	// Return the updated request type
	rt, err := h.fetchRequestType(id)
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
			ActionType:   "request_type_visibility_update",
			ResourceType: "request_type",
			ResourceID:   &rt.ID,
			ResourceName: rt.Name,
			Details: map[string]interface{}{
				"visibility_group_ids": rt.VisibilityGroupIDs,
				"visibility_org_ids":   rt.VisibilityOrgIDs,
			},
			Success: true,
		})
	}

	respondJSONOK(w, *rt)
}
