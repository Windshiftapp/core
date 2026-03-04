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

type ScreenHandler struct {
	db database.Database
}

func NewScreenHandler(db database.Database) *ScreenHandler {
	return &ScreenHandler{db: db}
}

func (h *ScreenHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	query := `SELECT id, name, description, created_at, updated_at FROM screens ORDER BY name`

	rows, err := h.db.Query(query)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var screens []models.Screen
	for rows.Next() {
		var screen models.Screen
		err := rows.Scan(&screen.ID, &screen.Name, &screen.Description, &screen.CreatedAt, &screen.UpdatedAt)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		screens = append(screens, screen)
	}

	if screens == nil {
		screens = []models.Screen{}
	}

	respondJSONOK(w, screens)
}

func (h *ScreenHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var screen models.Screen
	err := h.db.QueryRow(`
		SELECT id, name, description, created_at, updated_at
		FROM screens WHERE id = ?
	`, id).Scan(&screen.ID, &screen.Name, &screen.Description, &screen.CreatedAt, &screen.UpdatedAt)

	if err == sql.ErrNoRows {
		respondNotFound(w, r, "screen")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Load screen fields
	fields, err := h.getScreenFields(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	screen.Fields = fields

	// Load system fields
	systemFields, err := h.getSystemFields(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	screen.SystemFields = systemFields

	respondJSONOK(w, screen)
}

func (h *ScreenHandler) Create(w http.ResponseWriter, r *http.Request) {
	var screen models.Screen
	if err := json.NewDecoder(r.Body).Decode(&screen); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Validate required fields
	if strings.TrimSpace(screen.Name) == "" {
		respondValidationError(w, r, "Screen name is required")
		return
	}

	now := time.Now()
	var id int64
	err := h.db.QueryRow(`
		INSERT INTO screens (name, description, created_at, updated_at)
		VALUES (?, ?, ?, ?) RETURNING id
	`, screen.Name, screen.Description, now, now).Scan(&id)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Add default title and status fields
	_, err = h.db.ExecWrite(`
		INSERT INTO screen_fields (screen_id, field_type, field_identifier, display_order, is_required, field_width)
		VALUES (?, 'system', 'title', 0, true, 'full')
	`, id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	_, err = h.db.ExecWrite(`
		INSERT INTO screen_fields (screen_id, field_type, field_identifier, display_order, is_required, field_width)
		VALUES (?, 'system', 'status', 1, false, 'half')
	`, id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Return the created screen
	err = h.db.QueryRow(`
		SELECT id, name, description, created_at, updated_at
		FROM screens WHERE id = ?
	`, id).Scan(&screen.ID, &screen.Name, &screen.Description, &screen.CreatedAt, &screen.UpdatedAt)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		intID := int(id)
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionScreenCreate,
			ResourceType: logger.ResourceScreen,
			ResourceID:   &intID,
			ResourceName: screen.Name,
			Success:      true,
		})
	}

	respondJSONCreated(w, screen)
}

func (h *ScreenHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var screen models.Screen
	if err := json.NewDecoder(r.Body).Decode(&screen); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	now := time.Now()
	_, err := h.db.ExecWrite(`
		UPDATE screens
		SET name = ?, description = ?, updated_at = ?
		WHERE id = ?
	`, screen.Name, screen.Description, now, id)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Return the updated screen
	err = h.db.QueryRow(`
		SELECT id, name, description, created_at, updated_at
		FROM screens WHERE id = ?
	`, id).Scan(&screen.ID, &screen.Name, &screen.Description, &screen.CreatedAt, &screen.UpdatedAt)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionScreenUpdate,
			ResourceType: logger.ResourceScreen,
			ResourceID:   &id,
			ResourceName: screen.Name,
			Success:      true,
		})
	}

	respondJSONOK(w, screen)
}

func (h *ScreenHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Prevent deletion of default screen (ID 1)
	if id == 1 {
		respondValidationError(w, r, "Cannot delete default screen")
		return
	}

	_, err := h.db.ExecWrite("DELETE FROM screens WHERE id = ?", id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionScreenDelete,
			ResourceType: logger.ResourceScreen,
			ResourceID:   &id,
			Success:      true,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetFields returns the fields configured for a screen.
func (h *ScreenHandler) GetFields(w http.ResponseWriter, r *http.Request) {
	screenID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	fields, err := h.getScreenFields(screenID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, fields)
}

func (h *ScreenHandler) UpdateFields(w http.ResponseWriter, r *http.Request) {
	screenID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var fields []models.ScreenField
	if err := json.NewDecoder(r.Body).Decode(&fields); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Start transaction
	tx, err := h.db.Begin()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	// Delete existing screen fields
	_, err = tx.Exec("DELETE FROM screen_fields WHERE screen_id = ?", screenID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Insert new fields
	for _, field := range fields {
		_, err = tx.Exec(`
			INSERT INTO screen_fields (screen_id, field_type, field_identifier, display_order, is_required, field_width)
			VALUES (?, ?, ?, ?, ?, ?)
		`, screenID, field.FieldType, field.FieldIdentifier, field.DisplayOrder, field.IsRequired, field.FieldWidth)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	if err = tx.Commit(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionScreenUpdate,
			ResourceType: logger.ResourceScreen,
			ResourceID:   &screenID,
			Details:      map[string]interface{}{"update_type": "fields"},
			Success:      true,
		})
	}

	// Return updated fields
	updatedFields, err := h.getScreenFields(screenID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, updatedFields)
}

func (h *ScreenHandler) UpdateSystemFields(w http.ResponseWriter, r *http.Request) {
	screenID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var systemFields []string
	if err := json.NewDecoder(r.Body).Decode(&systemFields); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Start transaction
	tx, err := h.db.Begin()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	// Delete existing system fields
	_, err = tx.Exec("DELETE FROM screen_system_fields WHERE screen_id = ?", screenID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Insert new system fields
	for _, fieldName := range systemFields {
		_, err = tx.Exec(`
			INSERT INTO screen_system_fields (screen_id, field_name)
			VALUES (?, ?)
		`, screenID, fieldName)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	if err = tx.Commit(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionScreenUpdate,
			ResourceType: logger.ResourceScreen,
			ResourceID:   &screenID,
			Details:      map[string]interface{}{"update_type": "system_fields"},
			Success:      true,
		})
	}

	// Return updated system fields
	updatedSystemFields, err := h.getSystemFields(screenID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, updatedSystemFields)
}

// Helper function to get screen fields with joined data
func (h *ScreenHandler) getScreenFields(screenID int) ([]models.ScreenField, error) {
	rows, err := h.db.Query(`
		SELECT sf.id, sf.screen_id, sf.field_type, sf.field_identifier, sf.display_order, sf.is_required, sf.field_width,
		       CASE 
		           WHEN sf.field_type = 'custom' THEN cfd.name
		           ELSE ''
		       END as field_name,
		       CASE 
		           WHEN sf.field_type = 'custom' THEN cfd.name
		           ELSE ''
		       END as field_label,
		       CASE 
		           WHEN sf.field_type = 'custom' THEN cfd.options
		           ELSE NULL
		       END as field_config
		FROM screen_fields sf
		LEFT JOIN custom_field_definitions cfd ON sf.field_type = 'custom' AND (CASE WHEN sf.field_type = 'custom' THEN CAST(sf.field_identifier AS INTEGER) END) = cfd.id
		WHERE sf.screen_id = ?
		ORDER BY sf.display_order, sf.id
	`, screenID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var fields []models.ScreenField
	for rows.Next() {
		var field models.ScreenField
		var configStr sql.NullString

		err := rows.Scan(&field.ID, &field.ScreenID, &field.FieldType, &field.FieldIdentifier,
			&field.DisplayOrder, &field.IsRequired, &field.FieldWidth,
			&field.FieldName, &field.FieldLabel, &configStr)
		if err != nil {
			return nil, err
		}

		// Parse field config if it exists
		if configStr.Valid && configStr.String != "" {
			var config map[string]interface{}
			if err := json.Unmarshal([]byte(configStr.String), &config); err == nil {
				field.FieldConfig = config
			}
		}

		fields = append(fields, field)
	}

	return fields, nil
}

// Helper function to get system fields for a screen
func (h *ScreenHandler) getSystemFields(screenID int) ([]string, error) {
	rows, err := h.db.Query(`
		SELECT field_name
		FROM screen_system_fields
		WHERE screen_id = ?
		ORDER BY field_name
	`, screenID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var systemFields []string
	for rows.Next() {
		var fieldName string
		if err := rows.Scan(&fieldName); err != nil {
			return nil, err
		}
		systemFields = append(systemFields, fieldName)
	}

	return systemFields, nil
}
