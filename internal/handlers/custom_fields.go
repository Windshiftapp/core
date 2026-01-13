package handlers

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
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

// logAndRespondError logs the actual error and responds with a generic message
func (h *CustomFieldHandler) logAndRespondError(w http.ResponseWriter, err error, message string, statusCode int) {
	slog.Error("custom field handler error", slog.String("component", "custom_fields"), slog.Any("error", err))
	http.Error(w, message, statusCode)
}

// logAndRespondDatabaseError logs database errors and responds with a generic message
func (h *CustomFieldHandler) logAndRespondDatabaseError(w http.ResponseWriter, err error) {
	slog.Error("database error in custom field handler", slog.String("component", "custom_fields"), slog.Any("error", err))
	http.Error(w, "Internal server error", http.StatusInternalServerError)
}

func NewCustomFieldHandler(db database.Database) *CustomFieldHandler {
	return &CustomFieldHandler{db: db}
}

func (h *CustomFieldHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	query := `
		SELECT id, name, field_type, description, required, options, display_order, system_default,
		       applies_to_portal_customers, applies_to_customer_organisations, created_at, updated_at
		FROM custom_field_definitions
		ORDER BY display_order, name`

	rows, err := h.db.Query(query)
	if err != nil {
		h.logAndRespondDatabaseError(w, err)
		return
	}
	defer rows.Close()

	var customFields []models.CustomFieldDefinition
	for rows.Next() {
		var cf models.CustomFieldDefinition
		var optionsJSON sql.NullString

		err := rows.Scan(&cf.ID, &cf.Name, &cf.FieldType, &cf.Description,
			&cf.Required, &optionsJSON, &cf.DisplayOrder, &cf.SystemDefault,
			&cf.AppliesToPortalCustomers, &cf.AppliesToCustomerOrganisations,
			&cf.CreatedAt, &cf.UpdatedAt)
		if err != nil {
			h.logAndRespondDatabaseError(w, err)
			return
		}

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

	respondJSONOK(w, customFields)
}

func (h *CustomFieldHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var cf models.CustomFieldDefinition
	var optionsJSON sql.NullString

	err := h.db.QueryRow(`
		SELECT id, name, field_type, description, required, options, display_order, system_default,
		       applies_to_portal_customers, applies_to_customer_organisations, created_at, updated_at
		FROM custom_field_definitions
		WHERE id = ?
	`, id).Scan(&cf.ID, &cf.Name, &cf.FieldType, &cf.Description,
		&cf.Required, &optionsJSON, &cf.DisplayOrder, &cf.SystemDefault,
		&cf.AppliesToPortalCustomers, &cf.AppliesToCustomerOrganisations,
		&cf.CreatedAt, &cf.UpdatedAt)
	
	if err == sql.ErrNoRows {
		http.Error(w, "Custom field not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Set options string
	if optionsJSON.Valid {
		cf.Options = optionsJSON.String
	}

	respondJSONOK(w, cf)
}

func (h *CustomFieldHandler) Create(w http.ResponseWriter, r *http.Request) {
	var cf models.CustomFieldDefinition
	if err := json.NewDecoder(r.Body).Decode(&cf); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if strings.TrimSpace(cf.Name) == "" {
		http.Error(w, "Field name is required", http.StatusBadRequest)
		return
	}

	// Validate field type
	if cf.FieldType != "text" && cf.FieldType != "textarea" && cf.FieldType != "select" && cf.FieldType != "multiselect" && cf.FieldType != "number" && cf.FieldType != "milestone" && cf.FieldType != "date" && cf.FieldType != "user" && cf.FieldType != "iteration" && cf.FieldType != "asset" {
		http.Error(w, "Invalid field type", http.StatusBadRequest)
		return
	}

	// Validate options for asset fields
	if cf.FieldType == "asset" {
		var config struct {
			AssetSetID int    `json:"asset_set_id"`
			QLQuery   string `json:"ql_query"`
		}
		if cf.Options == "" {
			http.Error(w, "Asset fields require asset_set_id in options", http.StatusBadRequest)
			return
		}
		if err := json.Unmarshal([]byte(cf.Options), &config); err != nil || config.AssetSetID == 0 {
			http.Error(w, "Asset fields require asset_set_id in options", http.StatusBadRequest)
			return
		}
	}

	// Validate options for select fields
	if (cf.FieldType == "select" || cf.FieldType == "multiselect") && cf.Options != "" {
		var options []string
		if err := json.Unmarshal([]byte(cf.Options), &options); err != nil {
			http.Error(w, "Invalid options format", http.StatusBadRequest)
			return
		}
		if len(options) == 0 {
			http.Error(w, "Select fields must have at least one option", http.StatusBadRequest)
			return
		}
	}

	// Validate options JSON if provided (for select/multiselect fields only)
	if cf.Options != "" && cf.FieldType != "asset" {
		var testOptions []string
		if err := json.Unmarshal([]byte(cf.Options), &testOptions); err != nil {
			http.Error(w, "Invalid options JSON format", http.StatusBadRequest)
			return
		}
	}

	// Sanitize user input to prevent XSS
	cf.Name = utils.StripHTMLTags(cf.Name)
	cf.Description = utils.StripHTMLTags(cf.Description)

	now := time.Now()
	var id int64
	err := h.db.QueryRow(`
		INSERT INTO custom_field_definitions (name, field_type, description, required, options, display_order,
		                                       applies_to_portal_customers, applies_to_customer_organisations,
		                                       created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, cf.Name, cf.FieldType, cf.Description, cf.Required, cf.Options, cf.DisplayOrder,
	   cf.AppliesToPortalCustomers, cf.AppliesToCustomerOrganisations, now, now).Scan(&id)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the created custom field
	var createdCF models.CustomFieldDefinition
	var returnOptionsJSON sql.NullString

	err = h.db.QueryRow(`
		SELECT id, name, field_type, description, required, options, display_order, system_default,
		       applies_to_portal_customers, applies_to_customer_organisations, created_at, updated_at
		FROM custom_field_definitions
		WHERE id = ?
	`, id).Scan(&createdCF.ID, &createdCF.Name, &createdCF.FieldType, &createdCF.Description,
		&createdCF.Required, &returnOptionsJSON, &createdCF.DisplayOrder, &createdCF.SystemDefault,
		&createdCF.AppliesToPortalCustomers, &createdCF.AppliesToCustomerOrganisations,
		&createdCF.CreatedAt, &createdCF.UpdatedAt)
	
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Set options string
	if returnOptionsJSON.Valid {
		createdCF.Options = returnOptionsJSON.String
	}

	// Log audit event
	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logger.LogAudit(h.db, logger.AuditEvent{
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

func (h *CustomFieldHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get the old custom field for audit logging
	var oldCF models.CustomFieldDefinition
	var oldOptionsJSON sql.NullString
	err := h.db.QueryRow(`
		SELECT id, name, field_type, description, required, options, display_order, system_default,
		       applies_to_portal_customers, applies_to_customer_organisations, created_at, updated_at
		FROM custom_field_definitions
		WHERE id = ?
	`, id).Scan(&oldCF.ID, &oldCF.Name, &oldCF.FieldType, &oldCF.Description,
		&oldCF.Required, &oldOptionsJSON, &oldCF.DisplayOrder, &oldCF.SystemDefault,
		&oldCF.AppliesToPortalCustomers, &oldCF.AppliesToCustomerOrganisations,
		&oldCF.CreatedAt, &oldCF.UpdatedAt)

	if err == sql.ErrNoRows {
		http.Error(w, "Custom field not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if oldOptionsJSON.Valid {
		oldCF.Options = oldOptionsJSON.String
	}

	var cf models.CustomFieldDefinition
	if err := json.NewDecoder(r.Body).Decode(&cf); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if strings.TrimSpace(cf.Name) == "" {
		http.Error(w, "Field name is required", http.StatusBadRequest)
		return
	}

	// Validate field type
	if cf.FieldType != "text" && cf.FieldType != "textarea" && cf.FieldType != "select" && cf.FieldType != "multiselect" && cf.FieldType != "number" && cf.FieldType != "milestone" && cf.FieldType != "date" && cf.FieldType != "user" && cf.FieldType != "iteration" && cf.FieldType != "asset" {
		http.Error(w, "Invalid field type", http.StatusBadRequest)
		return
	}

	// Validate options for asset fields
	if cf.FieldType == "asset" {
		var config struct {
			AssetSetID int    `json:"asset_set_id"`
			QLQuery   string `json:"ql_query"`
		}
		if cf.Options == "" {
			http.Error(w, "Asset fields require asset_set_id in options", http.StatusBadRequest)
			return
		}
		if err := json.Unmarshal([]byte(cf.Options), &config); err != nil || config.AssetSetID == 0 {
			http.Error(w, "Asset fields require asset_set_id in options", http.StatusBadRequest)
			return
		}
	}

	// Validate options JSON if provided (for select/multiselect fields)
	if cf.Options != "" && cf.FieldType != "asset" {
		var testOptions []string
		if err := json.Unmarshal([]byte(cf.Options), &testOptions); err != nil {
			http.Error(w, "Invalid options JSON format", http.StatusBadRequest)
			return
		}
	}

	// Sanitize user input to prevent XSS
	cf.Name = utils.StripHTMLTags(cf.Name)
	cf.Description = utils.StripHTMLTags(cf.Description)

	now := time.Now()
	_, err = h.db.ExecWrite(`
		UPDATE custom_field_definitions
		SET name = ?, field_type = ?, description = ?, required = ?, options = ?, display_order = ?,
		    applies_to_portal_customers = ?, applies_to_customer_organisations = ?, updated_at = ?
		WHERE id = ?
	`, cf.Name, cf.FieldType, cf.Description, cf.Required, cf.Options, cf.DisplayOrder,
	   cf.AppliesToPortalCustomers, cf.AppliesToCustomerOrganisations, now, id)
	
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the updated custom field
	var updatedCF models.CustomFieldDefinition
	var returnOptionsJSON sql.NullString

	err = h.db.QueryRow(`
		SELECT id, name, field_type, description, required, options, display_order, system_default,
		       applies_to_portal_customers, applies_to_customer_organisations, created_at, updated_at
		FROM custom_field_definitions
		WHERE id = ?
	`, id).Scan(&updatedCF.ID, &updatedCF.Name, &updatedCF.FieldType, &updatedCF.Description,
		&updatedCF.Required, &returnOptionsJSON, &updatedCF.DisplayOrder, &updatedCF.SystemDefault,
		&updatedCF.AppliesToPortalCustomers, &updatedCF.AppliesToCustomerOrganisations,
		&updatedCF.CreatedAt, &updatedCF.UpdatedAt)
	
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

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

		logger.LogAudit(h.db, logger.AuditEvent{
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
		http.Error(w, "Custom field not found", http.StatusNotFound)
		return
	}
	if err != nil {
		h.logAndRespondDatabaseError(w, err)
		return
	}

	if systemDefault {
		http.Error(w, "System default custom fields cannot be deleted", http.StatusForbidden)
		return
	}

	_, err = h.db.ExecWrite("DELETE FROM custom_field_definitions WHERE id = ?", id)
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