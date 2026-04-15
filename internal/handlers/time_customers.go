package handlers

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"
)

type TimeCustomerHandler struct {
	db                    database.Database
	timePermissionService *services.TimePermissionService
}

func NewTimeCustomerHandler(db database.Database, timePermissionService *services.TimePermissionService) *TimeCustomerHandler {
	return &TimeCustomerHandler{
		db:                    db,
		timePermissionService: timePermissionService,
	}
}

// marshalCustomFieldValues serializes custom field values to a JSON *string for JSONB columns.
// Returns nil when there are no custom field values (which becomes SQL NULL).
// On marshal failure it writes a 400 response and returns false.
func marshalCustomFieldValues(w http.ResponseWriter, r *http.Request, values map[string]interface{}) (*string, bool) {
	if len(values) == 0 {
		return nil, true
	}
	b, err := json.Marshal(values)
	if err != nil {
		respondValidationError(w, r, "Invalid custom field values")
		return nil, false
	}
	s := string(b)
	return &s, true
}

// checkCustomerPermission is a helper that checks if the user has customers.manage or project.manage permission
func (h *TimeCustomerHandler) checkCustomerPermission(w http.ResponseWriter, r *http.Request) (*models.User, bool) { //nolint:unparam // User return kept for future use
	user, ok := RequireAuth(w, r)
	if !ok {
		return nil, false
	}

	if h.timePermissionService != nil {
		hasPermission, err := h.timePermissionService.HasCustomersManagePermission(user.ID)
		if err != nil {
			respondInternalError(w, r, err)
			return nil, false
		}
		if !hasPermission {
			respondForbidden(w, r)
			return nil, false
		}
	}

	return user, true
}

func (h *TimeCustomerHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	if _, ok := RequireAuth(w, r); !ok {
		return
	}

	//nolint:misspell // "organisation" is intentional British spelling used throughout codebase
	rows, err := h.db.Query(`
		SELECT id, name, email, description, active, avatar_url, custom_field_values, created_at, updated_at
		FROM customer_organisations
		ORDER BY name ASC
	`)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var customers []models.CustomerOrganisation
	for rows.Next() {
		var c models.CustomerOrganisation
		var customFieldValuesStr sql.NullString
		var avatarURL sql.NullString
		err := rows.Scan(&c.ID, &c.Name, &c.Email, &c.Description, &c.Active, &avatarURL, &customFieldValuesStr, &c.CreatedAt, &c.UpdatedAt)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		// Set avatar URL
		if avatarURL.Valid {
			c.AvatarURL = avatarURL.String
		}

		// Parse custom field values
		if customFieldValuesStr.Valid && customFieldValuesStr.String != "" {
			if err := json.Unmarshal([]byte(customFieldValuesStr.String), &c.CustomFieldValues); err != nil {
				// Log error but continue with other customers
				continue
			}
		}

		customers = append(customers, c)
	}

	respondJSONOK(w, customers)
}

func (h *TimeCustomerHandler) Get(w http.ResponseWriter, r *http.Request) {
	if _, ok := RequireAuth(w, r); !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var c models.CustomerOrganisation
	var customFieldValuesStr sql.NullString
	var avatarURL sql.NullString
	//nolint:misspell // "organisation" is intentional British spelling used throughout codebase
	err := h.db.QueryRow(`
		SELECT id, name, email, description, active, avatar_url, custom_field_values, created_at, updated_at
		FROM customer_organisations
		WHERE id = ?
	`, id).Scan(&c.ID, &c.Name, &c.Email, &c.Description, &c.Active, &avatarURL, &customFieldValuesStr, &c.CreatedAt, &c.UpdatedAt)

	if err == sql.ErrNoRows {
		respondNotFound(w, r, "customer")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Set avatar URL
	if avatarURL.Valid {
		c.AvatarURL = avatarURL.String
	}

	// Parse custom field values
	if customFieldValuesStr.Valid && customFieldValuesStr.String != "" {
		if err := json.Unmarshal([]byte(customFieldValuesStr.String), &c.CustomFieldValues); err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	respondJSONOK(w, c)
}

func (h *TimeCustomerHandler) Create(w http.ResponseWriter, r *http.Request) {
	// Check permission
	user, ok := h.checkCustomerPermission(w, r)
	if !ok {
		return
	}

	c, ok := decodeJSON[models.CustomerOrganisation](w, r)
	if !ok {
		return
	}

	// Set default active status if not explicitly provided
	// Note: In JSON, if 'active' field is missing, it will be false
	// Only set to true if it's actually missing from the request
	// For now, we'll trust the frontend to send the correct value

	customFieldValuesJSON, ok := marshalCustomFieldValues(w, r, c.CustomFieldValues)
	if !ok {
		return
	}

	c.Name = utils.SanitizeTitle(c.Name)
	c.Description = utils.SanitizeCommentContent(c.Description)

	now := time.Now()
	var id int64
	//nolint:misspell // "organisations" is intentional British spelling used throughout codebase
	err := h.db.QueryRow(`
		INSERT INTO customer_organisations (name, email, description, active, avatar_url, custom_field_values, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, c.Name, c.Email, c.Description, c.Active, c.AvatarURL, customFieldValuesJSON, now, now).Scan(&id)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	c.ID = int(id)
	c.CreatedAt = now
	c.UpdatedAt = now

	if user != nil {
		customerID := c.ID
		logAudit(h.db, r, user, logger.ActionTimeCustomerCreate, logger.ResourceTimeCustomer, &customerID, c.Name)
	}

	respondJSONCreated(w, c)
}

func (h *TimeCustomerHandler) Update(w http.ResponseWriter, r *http.Request) {
	// Check permission
	user, ok := h.checkCustomerPermission(w, r)
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	c, ok := decodeJSON[models.CustomerOrganisation](w, r)
	if !ok {
		return
	}

	slog.Debug("updating customer", slog.Int("customer_id", id), slog.String("name", c.Name))

	customFieldValuesJSON, ok := marshalCustomFieldValues(w, r, c.CustomFieldValues)
	if !ok {
		return
	}

	c.Name = utils.SanitizeTitle(c.Name)
	c.Description = utils.SanitizeCommentContent(c.Description)

	//nolint:misspell // "organisations" is intentional British spelling used throughout codebase
	_, err := h.db.ExecWrite(`
		UPDATE customer_organisations
		SET name = ?, email = ?, description = ?, active = ?, avatar_url = ?, custom_field_values = ?, updated_at = ?
		WHERE id = ?
	`, c.Name, c.Email, c.Description, c.Active, c.AvatarURL, customFieldValuesJSON, time.Now(), id)

	if err != nil {
		slog.Error("failed to update customer", slog.Int("customer_id", id), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	slog.Debug("customer updated successfully", slog.Int("customer_id", id))

	c.ID = id
	c.UpdatedAt = time.Now()

	if user != nil {
		logAudit(h.db, r, user, logger.ActionTimeCustomerUpdate, logger.ResourceTimeCustomer, &id, c.Name)
	}

	respondJSONOK(w, c)
}

func (h *TimeCustomerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	// Check permission
	user, ok := h.checkCustomerPermission(w, r)
	if !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Check for associated projects
	var projectCount int
	err := h.db.QueryRow("SELECT COUNT(*) FROM time_projects WHERE customer_id = ?", id).Scan(&projectCount)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if projectCount > 0 {
		respondValidationError(w, r, "Cannot delete customer with associated projects")
		return
	}

	//nolint:misspell // "organisations" is intentional British spelling used throughout codebase
	_, err = h.db.ExecWrite("DELETE FROM customer_organisations WHERE id = ?", id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if user != nil {
		logAudit(h.db, r, user, logger.ActionTimeCustomerDelete, logger.ResourceTimeCustomer, &id, "")
	}

	w.WriteHeader(http.StatusNoContent)
}
