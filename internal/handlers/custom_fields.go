package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/utils"
)

type CustomFieldHandler struct {
	db database.Database
}

type assetTypeUsage struct {
	AssetTypeName string `json:"asset_type_name"`
	SetName       string `json:"set_name"`
}

type customFieldWithUsage struct {
	models.CustomFieldDefinition
	AssetTypeUsages []assetTypeUsage        `json:"asset_type_usages"`
	Indexed         *models.CustomFieldIndexInfo `json:"indexed,omitempty"`
}

type indexCountInfo struct {
	Current int `json:"current"`
	Max     int `json:"max"`
}

type customFieldsResponse struct {
	Data        []customFieldWithUsage    `json:"data"`
	IndexCounts map[string]indexCountInfo  `json:"index_counts"`
}

// indexable field types that benefit from B-tree indexes
var indexableFieldTypes = map[string]bool{
	"number": true,
	"date":   true,
	"text":   true,
}

// allowed target tables for indexing
var indexableTargetTables = map[string]bool{
	"items":  true,
	"assets": true,
}

// logAndRespondDatabaseError logs database errors and responds with a generic message
func (h *CustomFieldHandler) logAndRespondDatabaseError(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("database error in custom field handler", slog.String("component", "custom_fields"), slog.Any("error", err))
	respondInternalError(w, r, err)
}

func NewCustomFieldHandler(db database.Database) *CustomFieldHandler {
	return &CustomFieldHandler{db: db}
}

func (h *CustomFieldHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	//nolint:misspell // database uses British spelling
	query := `
		SELECT id, name, field_type, description, required, options, display_order, system_default,
		       applies_to_portal_customers, applies_to_customer_organisations, created_at, updated_at
		FROM custom_field_definitions
		ORDER BY display_order, name`

	rows, err := h.db.Query(query)
	if err != nil {
		h.logAndRespondDatabaseError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var customFields []models.CustomFieldDefinition
	for rows.Next() {
		var cf models.CustomFieldDefinition
		var optionsJSON sql.NullString
		var description sql.NullString

		err := rows.Scan(&cf.ID, &cf.Name, &cf.FieldType, &description,
			&cf.Required, &optionsJSON, &cf.DisplayOrder, &cf.SystemDefault,
			&cf.AppliesToPortalCustomers, &cf.AppliesToCustomerOrganisations,
			&cf.CreatedAt, &cf.UpdatedAt)
		if err != nil {
			h.logAndRespondDatabaseError(w, r, err)
			return
		}

		cf.Description = description.String
		// Set options string
		if optionsJSON.Valid {
			cf.Options = optionsJSON.String
		}

		customFields = append(customFields, cf)
	}

	// Always return an array, even if empty
	if customFields == nil {
		customFields = []models.CustomFieldDefinition{}
	}

	// Load asset type usages for all custom fields
	assetTypeUsages := make(map[int][]assetTypeUsage)
	usageRows, err := h.db.Query(`
		SELECT atf.custom_field_id, at.name, s.name
		FROM asset_type_fields atf
		JOIN asset_types at ON atf.asset_type_id = at.id
		JOIN asset_management_sets s ON at.set_id = s.id
		ORDER BY atf.custom_field_id, s.name, at.name`)
	if err != nil {
		h.logAndRespondDatabaseError(w, r, err)
		return
	}
	defer func() { _ = usageRows.Close() }()

	for usageRows.Next() {
		var fieldID int
		var typeName, setName string
		if err := usageRows.Scan(&fieldID, &typeName, &setName); err != nil {
			h.logAndRespondDatabaseError(w, r, err)
			return
		}
		assetTypeUsages[fieldID] = append(assetTypeUsages[fieldID], assetTypeUsage{
			AssetTypeName: typeName,
			SetName:       setName,
		})
	}

	// Load index info for all custom fields
	fieldIndexes := make(map[int]*models.CustomFieldIndexInfo)
	indexRows, err := h.db.Query(`SELECT custom_field_id, target_table FROM custom_field_indexes`)
	if err != nil {
		h.logAndRespondDatabaseError(w, r, err)
		return
	}
	defer func() { _ = indexRows.Close() }()

	indexCounts := map[string]int{"items": 0, "assets": 0}
	for indexRows.Next() {
		var fieldID int
		var targetTable string
		if err := indexRows.Scan(&fieldID, &targetTable); err != nil {
			h.logAndRespondDatabaseError(w, r, err)
			return
		}
		if fieldIndexes[fieldID] == nil {
			fieldIndexes[fieldID] = &models.CustomFieldIndexInfo{}
		}
		switch targetTable {
		case "items":
			fieldIndexes[fieldID].Items = true
			indexCounts["items"]++
		case "assets":
			fieldIndexes[fieldID].Assets = true
			indexCounts["assets"]++
		}
	}

	// Get max index limit
	maxIndexes := 20
	var maxStr sql.NullString
	if err := h.db.QueryRow(`SELECT value FROM system_settings WHERE key = 'max_custom_field_indexes_per_table'`).Scan(&maxStr); err == nil && maxStr.Valid {
		if v, err := strconv.Atoi(maxStr.String); err == nil {
			maxIndexes = v
		}
	}

	// Wrap each field with its asset type usages and index info
	result := make([]customFieldWithUsage, len(customFields))
	for i, cf := range customFields {
		usages := assetTypeUsages[cf.ID]
		if usages == nil {
			usages = []assetTypeUsage{}
		}
		entry := customFieldWithUsage{
			CustomFieldDefinition: cf,
			AssetTypeUsages:       usages,
		}
		if idx, ok := fieldIndexes[cf.ID]; ok {
			entry.Indexed = idx
		} else if indexableFieldTypes[cf.FieldType] {
			entry.Indexed = &models.CustomFieldIndexInfo{}
		}
		result[i] = entry
	}

	respondJSONOK(w, customFieldsResponse{
		Data: result,
		IndexCounts: map[string]indexCountInfo{
			"items":  {Current: indexCounts["items"], Max: maxIndexes},
			"assets": {Current: indexCounts["assets"], Max: maxIndexes},
		},
	})
}

func (h *CustomFieldHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var cf models.CustomFieldDefinition
	var optionsJSON sql.NullString
	var description sql.NullString

	//nolint:misspell // database uses British spelling
	err := h.db.QueryRow(`
		SELECT id, name, field_type, description, required, options, display_order, system_default,
		       applies_to_portal_customers, applies_to_customer_organisations, created_at, updated_at
		FROM custom_field_definitions
		WHERE id = ?
	`, id).Scan(&cf.ID, &cf.Name, &cf.FieldType, &description,
		&cf.Required, &optionsJSON, &cf.DisplayOrder, &cf.SystemDefault,
		&cf.AppliesToPortalCustomers, &cf.AppliesToCustomerOrganisations,
		&cf.CreatedAt, &cf.UpdatedAt)

	if err == sql.ErrNoRows {
		respondNotFound(w, r, "custom_field")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	cf.Description = description.String
	// Set options string
	if optionsJSON.Valid {
		cf.Options = optionsJSON.String
	}

	respondJSONOK(w, cf)
}

func (h *CustomFieldHandler) Create(w http.ResponseWriter, r *http.Request) {
	var cf models.CustomFieldDefinition
	if err := json.NewDecoder(r.Body).Decode(&cf); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Validate required fields
	if strings.TrimSpace(cf.Name) == "" {
		respondValidationError(w, r, "Field name is required")
		return
	}

	// Validate field type
	if cf.FieldType != "text" && cf.FieldType != "textarea" && cf.FieldType != "select" && cf.FieldType != "multiselect" && cf.FieldType != "number" && cf.FieldType != "milestone" && cf.FieldType != "date" && cf.FieldType != "user" && cf.FieldType != "iteration" && cf.FieldType != "asset" && cf.FieldType != "portalcustomer" && cf.FieldType != "customerorganisation" {
		respondValidationError(w, r, "Invalid field type")
		return
	}

	// Validate options for asset fields
	if cf.FieldType == "asset" {
		var config struct {
			AssetSetID int    `json:"asset_set_id"`
			QLQuery    string `json:"ql_query"`
		}
		if cf.Options == "" {
			respondValidationError(w, r, "Asset fields require asset_set_id in options")
			return
		}
		if err := json.Unmarshal([]byte(cf.Options), &config); err != nil || config.AssetSetID == 0 {
			respondValidationError(w, r, "Asset fields require asset_set_id in options")
			return
		}
	}

	// Validate options for select fields
	if (cf.FieldType == "select" || cf.FieldType == "multiselect") && cf.Options != "" {
		var options []string
		if err := json.Unmarshal([]byte(cf.Options), &options); err != nil {
			respondValidationError(w, r, "Invalid options format")
			return
		}
		if len(options) == 0 {
			respondValidationError(w, r, "Select fields must have at least one option")
			return
		}
	}

	// Validate options JSON if provided (for select/multiselect fields only)
	if cf.Options != "" && cf.FieldType != "asset" && cf.FieldType != "portalcustomer" && cf.FieldType != "customerorganisation" {
		var testOptions []string
		if err := json.Unmarshal([]byte(cf.Options), &testOptions); err != nil {
			respondValidationError(w, r, "Invalid options JSON format")
			return
		}
	}

	// Sanitize user input to prevent XSS
	cf.Name = utils.SanitizeName(cf.Name)
	cf.Description = utils.SanitizeCommentContent(cf.Description)

	now := time.Now()
	var id int64
	//nolint:misspell // database uses British spelling (applies_to_customer_organisations)
	err := h.db.QueryRow(`
		INSERT INTO custom_field_definitions (name, field_type, description, required, options, display_order,
		                                       applies_to_portal_customers, applies_to_customer_organisations,
		                                       created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, cf.Name, cf.FieldType, cf.Description, cf.Required, cf.Options, cf.DisplayOrder,
		cf.AppliesToPortalCustomers, cf.AppliesToCustomerOrganisations, now, now).Scan(&id)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Return the created custom field
	var createdCF models.CustomFieldDefinition
	var returnOptionsJSON sql.NullString
	var returnDescription sql.NullString

	//nolint:misspell // database uses British spelling (applies_to_customer_organisations)
	err = h.db.QueryRow(`
		SELECT id, name, field_type, description, required, options, display_order, system_default,
		       applies_to_portal_customers, applies_to_customer_organisations, created_at, updated_at
		FROM custom_field_definitions
		WHERE id = ?
	`, id).Scan(&createdCF.ID, &createdCF.Name, &createdCF.FieldType, &returnDescription,
		&createdCF.Required, &returnOptionsJSON, &createdCF.DisplayOrder, &createdCF.SystemDefault,
		&createdCF.AppliesToPortalCustomers, &createdCF.AppliesToCustomerOrganisations,
		&createdCF.CreatedAt, &createdCF.UpdatedAt)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	createdCF.Description = returnDescription.String
	// Set options string
	if returnOptionsJSON.Valid {
		createdCF.Options = returnOptionsJSON.String
	}

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionCustomFieldCreate,
			ResourceType: logger.ResourceCustomField,
			ResourceID:   &createdCF.ID,
			ResourceName: createdCF.Name,
			Details: map[string]interface{}{
				"field_type":    createdCF.FieldType,
				"required":      createdCF.Required,
				"display_order": createdCF.DisplayOrder,
			},
			Success: true,
		})
	}

	respondJSONCreated(w, createdCF)
}

// updateRequest extends the custom field definition with optional indexing control
type updateRequest struct {
	models.CustomFieldDefinition
	Indexed *models.CustomFieldIndexInfo `json:"indexed,omitempty"`
}

func (h *CustomFieldHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get the old custom field for audit logging
	var oldCF models.CustomFieldDefinition
	var oldOptionsJSON sql.NullString
	var oldDescription sql.NullString
	//nolint:misspell // database uses British spelling (applies_to_customer_organisations)
	err := h.db.QueryRow(`
		SELECT id, name, field_type, description, required, options, display_order, system_default,
		       applies_to_portal_customers, applies_to_customer_organisations, created_at, updated_at
		FROM custom_field_definitions
		WHERE id = ?
	`, id).Scan(&oldCF.ID, &oldCF.Name, &oldCF.FieldType, &oldDescription,
		&oldCF.Required, &oldOptionsJSON, &oldCF.DisplayOrder, &oldCF.SystemDefault,
		&oldCF.AppliesToPortalCustomers, &oldCF.AppliesToCustomerOrganisations,
		&oldCF.CreatedAt, &oldCF.UpdatedAt)

	if err == sql.ErrNoRows {
		respondNotFound(w, r, "custom_field")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	oldCF.Description = oldDescription.String
	if oldOptionsJSON.Valid {
		oldCF.Options = oldOptionsJSON.String
	}

	var req updateRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	cf := req.CustomFieldDefinition

	// Validate required fields
	if strings.TrimSpace(cf.Name) == "" {
		respondValidationError(w, r, "Field name is required")
		return
	}

	// Validate field type
	if cf.FieldType != "text" && cf.FieldType != "textarea" && cf.FieldType != "select" && cf.FieldType != "multiselect" && cf.FieldType != "number" && cf.FieldType != "milestone" && cf.FieldType != "date" && cf.FieldType != "user" && cf.FieldType != "iteration" && cf.FieldType != "asset" && cf.FieldType != "portalcustomer" && cf.FieldType != "customerorganisation" {
		respondValidationError(w, r, "Invalid field type")
		return
	}

	// Validate options for asset fields
	if cf.FieldType == "asset" {
		var config struct {
			AssetSetID int    `json:"asset_set_id"`
			QLQuery    string `json:"ql_query"`
		}
		if cf.Options == "" {
			respondValidationError(w, r, "Asset fields require asset_set_id in options")
			return
		}
		if err = json.Unmarshal([]byte(cf.Options), &config); err != nil || config.AssetSetID == 0 {
			respondValidationError(w, r, "Asset fields require asset_set_id in options")
			return
		}
	}

	// Validate options JSON if provided (for select/multiselect fields)
	if cf.Options != "" && cf.FieldType != "asset" && cf.FieldType != "portalcustomer" && cf.FieldType != "customerorganisation" {
		var testOptions []string
		if err = json.Unmarshal([]byte(cf.Options), &testOptions); err != nil {
			respondValidationError(w, r, "Invalid options JSON format")
			return
		}
	}

	// Sanitize user input to prevent XSS
	cf.Name = utils.SanitizeName(cf.Name)
	cf.Description = utils.SanitizeCommentContent(cf.Description)

	now := time.Now()
	//nolint:misspell // customer_organisations is a database table name
	_, err = h.db.ExecWrite(`
		UPDATE custom_field_definitions
		SET name = ?, field_type = ?, description = ?, required = ?, options = ?, display_order = ?,
		    applies_to_portal_customers = ?, applies_to_customer_organisations = ?, updated_at = ?
		WHERE id = ?
	`, cf.Name, cf.FieldType, cf.Description, cf.Required, cf.Options, cf.DisplayOrder,
		cf.AppliesToPortalCustomers, cf.AppliesToCustomerOrganisations, now, id)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Handle indexing changes if provided
	if req.Indexed != nil {
		// Validate that field type is indexable
		if !indexableFieldTypes[oldCF.FieldType] {
			respondValidationError(w, r, fmt.Sprintf("Field type '%s' cannot be indexed. Only number, date, and text fields support indexing.", oldCF.FieldType))
			return
		}

		// Process each target table
		for _, table := range []struct {
			name   string
			wanted bool
		}{
			{"items", req.Indexed.Items},
			{"assets", req.Indexed.Assets},
		} {
			if err := h.manageFieldIndex(id, oldCF.FieldType, table.name, table.wanted); err != nil {
				if strings.Contains(err.Error(), "index limit") {
					respondBadRequest(w, r, err.Error())
					return
				}
				respondInternalError(w, r, err)
				return
			}
		}
	}

	// Return the updated custom field
	var updatedCF models.CustomFieldDefinition
	var returnOptionsJSON sql.NullString
	var updatedDescription sql.NullString

	//nolint:misspell // customer_organisations is a database table name
	err = h.db.QueryRow(`
		SELECT id, name, field_type, description, required, options, display_order, system_default,
		       applies_to_portal_customers, applies_to_customer_organisations, created_at, updated_at
		FROM custom_field_definitions
		WHERE id = ?
	`, id).Scan(&updatedCF.ID, &updatedCF.Name, &updatedCF.FieldType, &updatedDescription,
		&updatedCF.Required, &returnOptionsJSON, &updatedCF.DisplayOrder, &updatedCF.SystemDefault,
		&updatedCF.AppliesToPortalCustomers, &updatedCF.AppliesToCustomerOrganisations,
		&updatedCF.CreatedAt, &updatedCF.UpdatedAt)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	updatedCF.Description = updatedDescription.String
	// Set options string
	if returnOptionsJSON.Valid {
		updatedCF.Options = returnOptionsJSON.String
	}

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		details := make(map[string]interface{})

		// Track what changed
		if oldCF.Name != updatedCF.Name {
			details["name_changed"] = map[string]interface{}{
				"old": oldCF.Name,
				"new": updatedCF.Name,
			}
		}
		if oldCF.FieldType != updatedCF.FieldType {
			details["field_type_changed"] = map[string]interface{}{
				"old": oldCF.FieldType,
				"new": updatedCF.FieldType,
			}
		}
		if oldCF.Required != updatedCF.Required {
			details["required_changed"] = map[string]interface{}{
				"old": oldCF.Required,
				"new": updatedCF.Required,
			}
		}
		if oldCF.DisplayOrder != updatedCF.DisplayOrder {
			details["display_order_changed"] = map[string]interface{}{
				"old": oldCF.DisplayOrder,
				"new": updatedCF.DisplayOrder,
			}
		}
		if oldCF.Options != updatedCF.Options {
			details["options_changed"] = map[string]interface{}{
				"old": oldCF.Options,
				"new": updatedCF.Options,
			}
		}
		if req.Indexed != nil {
			details["indexed"] = req.Indexed
		}

		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionCustomFieldUpdate,
			ResourceType: logger.ResourceCustomField,
			ResourceID:   &updatedCF.ID,
			ResourceName: updatedCF.Name,
			Details:      details,
			Success:      true,
		})
	}

	respondJSONOK(w, updatedCF)
}

func (h *CustomFieldHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get the custom field details for audit logging
	var fieldName string
	var fieldType string
	var systemDefault bool
	err := h.db.QueryRow(`
		SELECT name, field_type, system_default
		FROM custom_field_definitions
		WHERE id = ?
	`, id).Scan(&fieldName, &fieldType, &systemDefault)

	if err == sql.ErrNoRows {
		respondNotFound(w, r, "custom_field")
		return
	}
	if err != nil {
		h.logAndRespondDatabaseError(w, r, err)
		return
	}

	if systemDefault {
		respondForbidden(w, r)
		return
	}

	// Drop any database indexes before deleting the field
	indexRows, err := h.db.Query(`SELECT index_name FROM custom_field_indexes WHERE custom_field_id = ?`, id)
	if err != nil {
		h.logAndRespondDatabaseError(w, r, err)
		return
	}
	defer func() { _ = indexRows.Close() }()

	var indexNames []string
	for indexRows.Next() {
		var indexName string
		if err := indexRows.Scan(&indexName); err != nil {
			h.logAndRespondDatabaseError(w, r, err)
			return
		}
		indexNames = append(indexNames, indexName)
	}

	for _, indexName := range indexNames {
		dropSQL := fmt.Sprintf("DROP INDEX IF EXISTS %s", indexName)
		if _, err := h.db.ExecWrite(dropSQL); err != nil {
			slog.Warn("failed to drop index during field deletion", slog.String("component", "custom_fields"), slog.String("index", indexName), slog.Any("error", err))
		}
	}

	_, err = h.db.ExecWrite("DELETE FROM custom_field_definitions WHERE id = ?", id)
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
			ActionType:   logger.ActionCustomFieldDelete,
			ResourceType: logger.ResourceCustomField,
			ResourceID:   &id,
			ResourceName: fieldName,
			Details: map[string]interface{}{
				"field_type": fieldType,
			},
			Success: true,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

// manageFieldIndex creates or drops a database index for a custom field on a target table.
func (h *CustomFieldHandler) manageFieldIndex(fieldID int, fieldType, targetTable string, enable bool) error {
	if !indexableTargetTables[targetTable] {
		return fmt.Errorf("invalid target table: %s", targetTable)
	}

	indexName := fmt.Sprintf("idx_cf_%s_%d", targetTable, fieldID)

	// Check current state
	var exists int
	err := h.db.QueryRow(`SELECT COUNT(*) FROM custom_field_indexes WHERE custom_field_id = ? AND target_table = ?`, fieldID, targetTable).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check index state: %w", err)
	}

	currentlyEnabled := exists > 0

	if enable == currentlyEnabled {
		return nil // no change needed
	}

	if enable {
		// Check limit
		var currentCount int
		err := h.db.QueryRow(`SELECT COUNT(*) FROM custom_field_indexes WHERE target_table = ?`, targetTable).Scan(&currentCount)
		if err != nil {
			return fmt.Errorf("failed to count indexes: %w", err)
		}

		maxIndexes := 20
		var maxStr sql.NullString
		if err := h.db.QueryRow(`SELECT value FROM system_settings WHERE key = 'max_custom_field_indexes_per_table'`).Scan(&maxStr); err == nil && maxStr.Valid {
			if v, err := strconv.Atoi(maxStr.String); err == nil {
				maxIndexes = v
			}
		}

		if currentCount >= maxIndexes {
			return fmt.Errorf("index limit reached: %d of %d indexes used on %s", currentCount, maxIndexes, targetTable)
		}

		// Create the database index
		createSQL := h.buildCreateIndexSQL(fieldID, fieldType, targetTable, indexName)
		if _, err := h.db.ExecWrite(createSQL); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}

		// Record in junction table
		if _, err := h.db.ExecWrite(`INSERT INTO custom_field_indexes (custom_field_id, target_table, index_name) VALUES (?, ?, ?)`,
			fieldID, targetTable, indexName); err != nil {
			// Attempt to drop the index we just created
			_, _ = h.db.ExecWrite(fmt.Sprintf("DROP INDEX IF EXISTS %s", indexName))
			return fmt.Errorf("failed to record index: %w", err)
		}
	} else {
		// Drop the database index
		dropSQL := fmt.Sprintf("DROP INDEX IF EXISTS %s", indexName)
		if _, err := h.db.ExecWrite(dropSQL); err != nil {
			return fmt.Errorf("failed to drop index: %w", err)
		}

		// Remove from junction table
		if _, err := h.db.ExecWrite(`DELETE FROM custom_field_indexes WHERE custom_field_id = ? AND target_table = ?`,
			fieldID, targetTable); err != nil {
			return fmt.Errorf("failed to remove index record: %w", err)
		}
	}

	return nil
}

type customFieldSettings struct {
	MaxIndexesPerTable int `json:"max_indexes_per_table"`
}

func (h *CustomFieldHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var settings customFieldSettings
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	if settings.MaxIndexesPerTable < 1 || settings.MaxIndexesPerTable > 100 {
		respondValidationError(w, r, "Maximum indexes per table must be between 1 and 100")
		return
	}

	// Check that new limit is not below current usage for any table
	rows, err := h.db.Query(`SELECT target_table, COUNT(*) FROM custom_field_indexes GROUP BY target_table`)
	if err != nil {
		h.logAndRespondDatabaseError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var table string
		var count int
		if err := rows.Scan(&table, &count); err != nil {
			h.logAndRespondDatabaseError(w, r, err)
			return
		}
		if count > settings.MaxIndexesPerTable {
			respondBadRequest(w, r, fmt.Sprintf("Cannot set limit to %d: %s table already has %d indexes", settings.MaxIndexesPerTable, table, count))
			return
		}
	}

	// Upsert system_settings (UPDATE then INSERT)
	value := strconv.Itoa(settings.MaxIndexesPerTable)
	result, err := h.db.ExecWrite(`
		UPDATE system_settings SET value = ?, updated_at = CURRENT_TIMESTAMP
		WHERE key = 'max_custom_field_indexes_per_table'
	`, value)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		_, err = h.db.ExecWrite(`
			INSERT INTO system_settings (key, value, value_type, description, category, created_at, updated_at)
			VALUES ('max_custom_field_indexes_per_table', ?, 'integer', 'Maximum number of custom field indexes per table', 'performance', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, value)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	// Audit log
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionCustomFieldUpdate,
			ResourceType: logger.ResourceCustomField,
			ResourceName: "custom_field_settings",
			Details: map[string]interface{}{
				"max_indexes_per_table": settings.MaxIndexesPerTable,
			},
			Success: true,
		})
	}

	respondJSONOK(w, settings)
}

// buildCreateIndexSQL generates the CREATE INDEX SQL based on driver and field type.
func (h *CustomFieldHandler) buildCreateIndexSQL(fieldID int, fieldType, targetTable, indexName string) string {
	fieldIDStr := strconv.Itoa(fieldID)
	driver := h.db.GetDriverName()

	if driver == "postgres" {
		switch fieldType {
		case "number":
			return fmt.Sprintf(`CREATE INDEX %s ON %s(CAST(%s->>'%s' AS NUMERIC))`,
				indexName, targetTable, "custom_field_values", fieldIDStr)
		case "text":
			return fmt.Sprintf(`CREATE INDEX %s ON %s((%s->>'%s'))`,
				indexName, targetTable, "custom_field_values", fieldIDStr)
		case "date":
			return fmt.Sprintf(`CREATE INDEX %s ON %s(CAST(%s->>'%s' AS TEXT))`,
				indexName, targetTable, "custom_field_values", fieldIDStr)
		}
	}

	// SQLite
	castType := "TEXT"
	if fieldType == "number" {
		castType = "NUMERIC"
	}
	return fmt.Sprintf(`CREATE INDEX %s ON %s(CAST(NULLIF(custom_field_values,'') ->> '$.\"%s\"' AS %s))`,
		indexName, targetTable, fieldIDStr, castType)
}
