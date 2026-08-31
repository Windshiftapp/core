package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/objecttranslation"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
	"windshift/internal/utils"
)

type ScreenHandler struct {
	db           database.Database
	translations *objecttranslation.Service
}

func (h *ScreenHandler) WithObjectTranslations(service *objecttranslation.Service) *ScreenHandler {
	h.translations = service
	return h
}

var alwaysVisibleScreenFields = []struct {
	identifier string
	required   bool
	width      string
}{
	{identifier: "title", required: true, width: "full"},
	{identifier: "description", required: false, width: "full"},
	{identifier: "status", required: false, width: "half"},
}

func NewScreenHandler(db database.Database) *ScreenHandler {
	return &ScreenHandler{db: db}
}

func (h *ScreenHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	query := `SELECT id, COALESCE(builtin_key, ''), name, description, created_at, updated_at FROM screens ORDER BY name`

	rows, err := h.db.Query(query)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	var screens []models.Screen
	for rows.Next() {
		var screen models.Screen
		err := rows.Scan(&screen.ID, &screen.BuiltinKey, &screen.Name, &screen.Description, &screen.CreatedAt, &screen.UpdatedAt)
		if err != nil {
			_ = rows.Close()
			respondInternalError(w, r, err)
			return
		}
		screens = append(screens, screen)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		respondInternalError(w, r, err)
		return
	}
	if err := rows.Close(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	if screens == nil {
		screens = []models.Screen{}
	}
	if r.URL.Query().Get("include_fields") == "true" {
		fieldsByScreen, err := h.getAllScreenFields()
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		for i := range screens {
			screens[i].Fields = ensureAlwaysVisibleScreenFields(screens[i].ID, fieldsByScreen[screens[i].ID])
		}
	}
	if !localizeObjectResponse(w, r, h.translations, "screen", screens) {
		return
	}

	respondJSONOK(w, screens)
}

func (h *ScreenHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	screen, err := h.loadScreen(id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "screen")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !localizeObjectResponse(w, r, h.translations, "screen", screen) {
		return
	}
	respondJSONOK(w, screen)
}

func (h *ScreenHandler) loadScreen(id int) (*models.Screen, error) {
	var screen models.Screen
	err := h.db.QueryRow(`
		SELECT id, COALESCE(builtin_key, ''), name, description, created_at, updated_at
		FROM screens WHERE id = ?
	`, id).Scan(&screen.ID, &screen.BuiltinKey, &screen.Name, &screen.Description, &screen.CreatedAt, &screen.UpdatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	// Load screen fields
	fields, err := h.getScreenFields(id)
	if err != nil {
		return nil, err
	}
	screen.Fields = ensureAlwaysVisibleScreenFields(id, fields)

	// Load system fields
	systemFields, err := h.getSystemFields(id)
	if err != nil {
		return nil, err
	}
	screen.SystemFields = systemFields

	return &screen, nil
}

func (h *ScreenHandler) Create(w http.ResponseWriter, r *http.Request) {
	screen, ok := decodeJSON[models.Screen](w, r)
	if !ok {
		return
	}
	// Screen Name labels the create/edit form picker; Description shows
	// in the screen directory.
	warnings := sanitize.ApplyAllWithWarnings(
		sanitize.Pair{Target: &screen.Name, Policy: sanitize.PlainTextField, Label: "Name"},
		sanitize.Pair{Target: &screen.Description, Policy: sanitize.RichText, Label: "Description"},
	)

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

	// Add fields that are always visible on item screens.
	for displayOrder, field := range alwaysVisibleScreenFields {
		_, err = h.db.ExecWrite(`
			INSERT INTO screen_fields (screen_id, field_type, field_identifier, display_order, is_required, field_width)
			VALUES (?, 'system', ?, ?, ?, ?)
		`, id, field.identifier, displayOrder, field.required, field.width)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	// Return the created screen
	err = h.db.QueryRow(`
		SELECT id, COALESCE(builtin_key, ''), name, description, created_at, updated_at
		FROM screens WHERE id = ?
	`, id).Scan(&screen.ID, &screen.BuiltinKey, &screen.Name, &screen.Description, &screen.CreatedAt, &screen.UpdatedAt)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		intID := int(id)
		logAudit(h.db, r, currentUser, logger.ActionScreenCreate, logger.ResourceScreen, &intID, screen.Name)
	}
	if !localizeObjectResponse(w, r, h.translations, "screen", &screen) {
		return
	}

	respondJSONCreated(w, struct {
		models.Screen
		Warnings []string `json:"warnings,omitempty"`
	}{screen, warnings})
}

func (h *ScreenHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	screen, ok := decodeJSON[models.Screen](w, r)
	if !ok {
		return
	}
	warnings := sanitize.ApplyAllWithWarnings(
		sanitize.Pair{Target: &screen.Name, Policy: sanitize.PlainTextField, Label: "Name"},
		sanitize.Pair{Target: &screen.Description, Policy: sanitize.RichText, Label: "Description"},
	)

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
		SELECT id, COALESCE(builtin_key, ''), name, description, created_at, updated_at
		FROM screens WHERE id = ?
	`, id).Scan(&screen.ID, &screen.BuiltinKey, &screen.Name, &screen.Description, &screen.CreatedAt, &screen.UpdatedAt)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logAudit(h.db, r, currentUser, logger.ActionScreenUpdate, logger.ResourceScreen, &id, screen.Name)
	}

	if !localizeObjectResponse(w, r, h.translations, "screen", &screen) {
		return
	}
	respondJSONOK(w, struct {
		models.Screen
		Warnings []string `json:"warnings,omitempty"`
	}{screen, warnings})
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
		logAudit(h.db, r, currentUser, logger.ActionScreenDelete, logger.ResourceScreen, &id, "")
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

	respondJSONOK(w, ensureAlwaysVisibleScreenFields(screenID, fields))
}

func (h *ScreenHandler) UpdateFields(w http.ResponseWriter, r *http.Request) {
	screenID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	fields, ok := decodeJSON[[]models.ScreenField](w, r)
	if !ok {
		return
	}
	fields = ensureAlwaysVisibleScreenFields(screenID, fields)

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

	h.logScreenUpdate(r, screenID, "fields")

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

	systemFields, ok := decodeJSON[[]string](w, r)
	if !ok {
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

	h.logScreenUpdate(r, screenID, "system_fields")

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
		  AND (sf.field_type != 'custom' OR cfd.id IS NOT NULL)
		ORDER BY sf.display_order, sf.id
	`, screenID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanScreenFields(rows)
}

// getAllScreenFields returns all screen assignments in one query for enriched
// list responses, grouped by screen ID in memory.
func (h *ScreenHandler) getAllScreenFields() (map[int][]models.ScreenField, error) {
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
		WHERE sf.field_type != 'custom' OR cfd.id IS NOT NULL
		ORDER BY sf.screen_id, sf.display_order, sf.id
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	fields, err := scanScreenFields(rows)
	if err != nil {
		return nil, err
	}
	fieldsByScreen := make(map[int][]models.ScreenField)
	for _, field := range fields {
		fieldsByScreen[field.ScreenID] = append(fieldsByScreen[field.ScreenID], field)
	}
	return fieldsByScreen, nil
}

func scanScreenFields(rows *sql.Rows) ([]models.ScreenField, error) {

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
			var config map[string]any
			if err := json.Unmarshal([]byte(configStr.String), &config); err == nil {
				field.FieldConfig = config
			}
		}

		fields = append(fields, field)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return fields, nil
}

func ensureAlwaysVisibleScreenFields(screenID int, fields []models.ScreenField) []models.ScreenField {
	out := append([]models.ScreenField(nil), fields...)
	for _, requiredField := range alwaysVisibleScreenFields {
		if index := indexOfScreenSystemField(out, requiredField.identifier); index >= 0 {
			out[index].IsRequired = requiredField.required
			out[index].FieldWidth = requiredField.width
			continue
		}
		insertIndex := alwaysVisibleScreenFieldInsertIndex(out, requiredField.identifier)
		out = append(out, models.ScreenField{})
		copy(out[insertIndex+1:], out[insertIndex:])
		out[insertIndex] = models.ScreenField{
			ScreenID:        screenID,
			FieldType:       "system",
			FieldIdentifier: requiredField.identifier,
			IsRequired:      requiredField.required,
			FieldWidth:      requiredField.width,
		}
	}
	for i := range out {
		out[i].ScreenID = screenID
		out[i].DisplayOrder = i
	}
	return out
}

func alwaysVisibleScreenFieldInsertIndex(fields []models.ScreenField, identifier string) int {
	orderIndex := -1
	for i, field := range alwaysVisibleScreenFields {
		if field.identifier == identifier {
			orderIndex = i
			break
		}
	}
	if orderIndex < 0 {
		return len(fields)
	}

	for i := orderIndex + 1; i < len(alwaysVisibleScreenFields); i++ {
		if index := indexOfScreenSystemField(fields, alwaysVisibleScreenFields[i].identifier); index >= 0 {
			return index
		}
	}
	for i := orderIndex - 1; i >= 0; i-- {
		if index := indexOfScreenSystemField(fields, alwaysVisibleScreenFields[i].identifier); index >= 0 {
			return index + 1
		}
	}
	if orderIndex < len(fields) {
		return orderIndex
	}
	return len(fields)
}

func indexOfScreenSystemField(fields []models.ScreenField, identifier string) int {
	for i, field := range fields {
		if field.FieldType == "system" && field.FieldIdentifier == identifier {
			return i
		}
	}
	return -1
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
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return systemFields, nil
}

// logScreenUpdate logs an audit event for a screen field update.
func (h *ScreenHandler) logScreenUpdate(r *http.Request, screenID int, updateType string) {
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
			Details:      map[string]any{"update_type": updateType},
			Success:      true,
		})
	}
}
