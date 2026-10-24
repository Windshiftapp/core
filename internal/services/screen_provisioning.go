package services

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/sanitize"
)

// ScreenProvisioningService owns the screen configuration flows shared by the
// admin browser surface and public API v2 (WI-1306): validation,
// sanitization, always-visible system fields, and audit.
type ScreenProvisioningService struct {
	db database.Database
}

func NewScreenProvisioningService(db database.Database) *ScreenProvisioningService {
	return &ScreenProvisioningService{db: db}
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

// ListScreens returns every screen, optionally enriched with field
// configuration.
func (s *ScreenProvisioningService) ListScreens(includeFields bool) ([]models.Screen, error) {
	query := `SELECT id, COALESCE(builtin_key, ''), name, description, created_at, updated_at FROM screens ORDER BY name`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}

	var screens []models.Screen
	var scanErr error
	func() {
		defer rows.Close()
		for rows.Next() {
			var screen models.Screen
			if err := rows.Scan(&screen.ID, &screen.BuiltinKey, &screen.Name, &screen.Description, &screen.CreatedAt, &screen.UpdatedAt); err != nil {
				scanErr = err
				return
			}
			screens = append(screens, screen)
		}
		if err := rows.Err(); err != nil {
			scanErr = err
			return
		}
		if err := rows.Close(); err != nil {
			scanErr = err
		}
	}()
	if scanErr != nil {
		return nil, scanErr
	}

	if screens == nil {
		screens = []models.Screen{}
	}
	if includeFields {
		fieldsByScreen, err := s.getAllScreenFields()
		if err != nil {
			return nil, err
		}
		for i := range screens {
			screens[i].Fields = ensureAlwaysVisibleScreenFields(screens[i].ID, fieldsByScreen[screens[i].ID])
		}
	}
	return screens, nil
}

// GetScreen loads one screen with its field and system-field configuration.
func (s *ScreenProvisioningService) GetScreen(id int) (*models.Screen, error) {
	var screen models.Screen
	err := s.db.QueryRow(`
		SELECT id, COALESCE(builtin_key, ''), name, description, created_at, updated_at
		FROM screens WHERE id = ?
	`, id).Scan(&screen.ID, &screen.BuiltinKey, &screen.Name, &screen.Description, &screen.CreatedAt, &screen.UpdatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, NewServiceError(404, "Screen not found")
	}
	if err != nil {
		return nil, err
	}

	fields, err := s.getScreenFields(id)
	if err != nil {
		return nil, err
	}
	screen.Fields = ensureAlwaysVisibleScreenFields(id, fields)

	systemFields, err := s.getSystemFields(id)
	if err != nil {
		return nil, err
	}
	screen.SystemFields = systemFields

	return &screen, nil
}

// Create validates and persists a new screen with the always-visible system
// fields. Returns the stored screen and sanitize warnings.
func (s *ScreenProvisioningService) Create(actor AuditActor, screen *models.Screen) (*models.Screen, []string, error) {
	// Screen Name labels the create/edit form picker; Description shows
	// in the screen directory.
	warnings := sanitize.ApplyAllWithWarnings(
		sanitize.Pair{Target: &screen.Name, Policy: sanitize.PlainTextField, Label: "Name"},
		sanitize.Pair{Target: &screen.Description, Policy: sanitize.RichText, Label: "Description"},
	)

	if strings.TrimSpace(screen.Name) == "" {
		return nil, nil, NewServiceError(400, "Screen name is required")
	}

	now := time.Now()
	var id int64
	err := s.db.QueryRow(`
		INSERT INTO screens (name, description, created_at, updated_at)
		VALUES (?, ?, ?, ?) RETURNING id
	`, screen.Name, screen.Description, now, now).Scan(&id)
	if err != nil {
		return nil, nil, fmt.Errorf("create screen: %w", err)
	}

	// Add fields that are always visible on item screens.
	for displayOrder, field := range alwaysVisibleScreenFields {
		_, err = s.db.ExecWrite(`
			INSERT INTO screen_fields (screen_id, field_type, field_identifier, display_order, is_required, field_width)
			VALUES (?, 'system', ?, ?, ?, ?)
		`, id, field.identifier, displayOrder, field.required, field.width)
		if err != nil {
			return nil, nil, fmt.Errorf("create screen: %w", err)
		}
	}

	err = s.db.QueryRow(`
		SELECT id, COALESCE(builtin_key, ''), name, description, created_at, updated_at
		FROM screens WHERE id = ?
	`, id).Scan(&screen.ID, &screen.BuiltinKey, &screen.Name, &screen.Description, &screen.CreatedAt, &screen.UpdatedAt)
	if err != nil {
		return nil, nil, fmt.Errorf("create screen: %w", err)
	}

	intID := int(id)
	emitServiceAudit(s.db, actor, logger.ActionScreenCreate, logger.ResourceScreen, &intID, screen.Name, nil)
	return screen, warnings, nil
}

// Update renames / re-describes an existing screen.
func (s *ScreenProvisioningService) Update(actor AuditActor, id int, screen *models.Screen) (*models.Screen, []string, error) {
	warnings := sanitize.ApplyAllWithWarnings(
		sanitize.Pair{Target: &screen.Name, Policy: sanitize.PlainTextField, Label: "Name"},
		sanitize.Pair{Target: &screen.Description, Policy: sanitize.RichText, Label: "Description"},
	)

	now := time.Now()
	if _, err := s.db.ExecWrite(`
		UPDATE screens
		SET name = ?, description = ?, updated_at = ?
		WHERE id = ?
	`, screen.Name, screen.Description, now, id); err != nil {
		return nil, nil, fmt.Errorf("update screen: %w", err)
	}

	err := s.db.QueryRow(`
		SELECT id, COALESCE(builtin_key, ''), name, description, created_at, updated_at
		FROM screens WHERE id = ?
	`, id).Scan(&screen.ID, &screen.BuiltinKey, &screen.Name, &screen.Description, &screen.CreatedAt, &screen.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, NewServiceError(404, "Screen not found")
	}
	if err != nil {
		return nil, nil, fmt.Errorf("update screen: %w", err)
	}

	emitServiceAudit(s.db, actor, logger.ActionScreenUpdate, logger.ResourceScreen, &id, screen.Name, nil)
	return screen, warnings, nil
}

// Delete removes a screen. Built-in screens are protected because the default
// configuration set and fallback resolution depend on them.
func (s *ScreenProvisioningService) Delete(actor AuditActor, id int) error {
	var builtinKey sql.NullString
	err := s.db.QueryRow("SELECT builtin_key FROM screens WHERE id = ?", id).Scan(&builtinKey)
	if errors.Is(err, sql.ErrNoRows) {
		return NewServiceError(404, "Screen not found")
	}
	if err != nil {
		return fmt.Errorf("load screen %d: %w", id, err)
	}
	if strings.TrimSpace(builtinKey.String) != "" {
		return NewServiceError(400, "Cannot delete builtin screen")
	}

	if _, err := s.db.ExecWrite("DELETE FROM screens WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete screen: %w", err)
	}

	emitServiceAudit(s.db, actor, logger.ActionScreenDelete, logger.ResourceScreen, &id, "", nil)
	return nil
}

// GetScreenFields returns the fields configured for a screen, normalized so
// the always-visible system fields are present with their fixed settings.
func (s *ScreenProvisioningService) GetScreenFields(screenID int) ([]models.ScreenField, error) {
	fields, err := s.getScreenFields(screenID)
	if err != nil {
		return nil, err
	}
	return ensureAlwaysVisibleScreenFields(screenID, fields), nil
}

// ReplaceScreenFields atomically replaces a screen's field configuration.
func (s *ScreenProvisioningService) ReplaceScreenFields(actor AuditActor, screenID int, fields []models.ScreenField) ([]models.ScreenField, error) {
	fields = ensureAlwaysVisibleScreenFields(screenID, fields)
	cleanedFields, err := s.clearLinkingRequiredFlags(fields)
	if err != nil {
		return nil, err
	}
	fields = cleanedFields

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("update screen fields: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err = tx.Exec("DELETE FROM screen_fields WHERE screen_id = ?", screenID); err != nil {
		return nil, fmt.Errorf("update screen fields: %w", err)
	}
	for _, field := range fields {
		if _, err = tx.Exec(`
			INSERT INTO screen_fields (screen_id, field_type, field_identifier, display_order, is_required, field_width)
			VALUES (?, ?, ?, ?, ?, ?)
		`, screenID, field.FieldType, field.FieldIdentifier, field.DisplayOrder, field.IsRequired, field.FieldWidth); err != nil {
			return nil, fmt.Errorf("update screen fields: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("update screen fields: %w", err)
	}

	s.logScreenUpdate(actor, screenID, "fields")

	updatedFields, err := s.getScreenFields(screenID)
	if err != nil {
		return nil, fmt.Errorf("update screen fields: %w", err)
	}
	return updatedFields, nil
}

// clearLinkingRequiredFlags forces is_required off for linking custom fields.
// A linking field stores its value as an item link and needs the owning item
// to exist, so a create form cannot satisfy a required linking field. The
// screen editor disables the toggle; this keeps the API from persisting the
// impossible state too.
func (s *ScreenProvisioningService) clearLinkingRequiredFlags(fields []models.ScreenField) ([]models.ScreenField, error) {
	for i := range fields {
		if fields[i].FieldType != "custom" || !fields[i].IsRequired {
			continue
		}
		fieldID, err := strconv.Atoi(strings.TrimSpace(fields[i].FieldIdentifier))
		if err != nil {
			continue
		}
		var fieldType string
		err = s.db.QueryRow("SELECT field_type FROM custom_field_definitions WHERE id = ?", fieldID).Scan(&fieldType)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("load custom field %d: %w", fieldID, err)
		}
		if fieldType == "linking" {
			fields[i].IsRequired = false
		}
	}
	return fields, nil
}

// ReplaceScreenSystemFields atomically replaces a screen's system-field
// visibility configuration.
func (s *ScreenProvisioningService) ReplaceScreenSystemFields(actor AuditActor, screenID int, systemFields []string) ([]string, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("update screen system fields: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err = tx.Exec("DELETE FROM screen_system_fields WHERE screen_id = ?", screenID); err != nil {
		return nil, fmt.Errorf("update screen system fields: %w", err)
	}
	for _, fieldName := range systemFields {
		if _, err = tx.Exec(`
			INSERT INTO screen_system_fields (screen_id, field_name)
			VALUES (?, ?)
		`, screenID, fieldName); err != nil {
			return nil, fmt.Errorf("update screen system fields: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("update screen system fields: %w", err)
	}

	s.logScreenUpdate(actor, screenID, "system_fields")

	return s.getSystemFields(screenID)
}

func (s *ScreenProvisioningService) logScreenUpdate(actor AuditActor, screenID int, updateType string) {
	emitServiceAudit(s.db, actor, logger.ActionScreenUpdate, logger.ResourceScreen, &screenID, "", map[string]any{"update_type": updateType})
}

// Helper function to get screen fields with joined data
func (s *ScreenProvisioningService) getScreenFields(screenID int) ([]models.ScreenField, error) {
	rows, err := s.db.Query(`
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
func (s *ScreenProvisioningService) getAllScreenFields() (map[int][]models.ScreenField, error) {
	rows, err := s.db.Query(`
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

	byScreen := make(map[int][]models.ScreenField)
	for _, field := range fields {
		byScreen[field.ScreenID] = append(byScreen[field.ScreenID], field)
	}
	return byScreen, nil
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
	return fields, rows.Err()
}

// EnsureAlwaysVisibleScreenFields normalizes a screen's field list so the
// always-visible system fields are present, ordered, and carry their fixed
// required/width settings. Exported because the field-config contract is
// asserted directly by white-box tests.
func EnsureAlwaysVisibleScreenFields(screenID int, fields []models.ScreenField) []models.ScreenField {
	return ensureAlwaysVisibleScreenFields(screenID, fields)
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
func (s *ScreenProvisioningService) getSystemFields(screenID int) ([]string, error) {
	rows, err := s.db.Query(`
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
