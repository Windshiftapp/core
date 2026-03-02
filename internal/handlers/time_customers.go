package handlers

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"windshift/internal/database"
	"windshift/internal/middleware"
	"windshift/internal/models"
	"windshift/internal/services"
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

// checkCustomerPermission is a helper that checks if the user has customers.manage or project.manage permission
func (h *TimeCustomerHandler) checkCustomerPermission(w http.ResponseWriter, r *http.Request) (*models.User, bool) { //nolint:unparam // User return kept for future use
	user, ok := r.Context().Value(middleware.ContextKeyUser).(*models.User)
	if !ok || user == nil {
		respondUnauthorized(w, r)
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
	// Check permission
	if _, ok := h.checkCustomerPermission(w, r); !ok {
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
	// Check permission
	if _, ok := h.checkCustomerPermission(w, r); !ok {
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
	if _, ok := h.checkCustomerPermission(w, r); !ok {
		return
	}

	var c models.CustomerOrganisation
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		respondBadRequest(w, r, err.Error())
		return
	}

	// Set default active status if not explicitly provided
	// Note: In JSON, if 'active' field is missing, it will be false
	// Only set to true if it's actually missing from the request
	// For now, we'll trust the frontend to send the correct value

	// Serialize custom field values to JSON
	var customFieldValuesJSON []byte
	if len(c.CustomFieldValues) > 0 {
		var err error
		customFieldValuesJSON, err = json.Marshal(c.CustomFieldValues)
		if err != nil {
			respondValidationError(w, r, "Invalid custom field values")
			return
		}
	}

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

	respondJSONCreated(w, c)
}

func (h *TimeCustomerHandler) Update(w http.ResponseWriter, r *http.Request) {
	// Check permission
	if _, ok := h.checkCustomerPermission(w, r); !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var c models.CustomerOrganisation
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		respondBadRequest(w, r, err.Error())
		return
	}

	slog.Debug("updating customer", slog.Int("customer_id", id), slog.String("name", c.Name))

	// Serialize custom field values to JSON
	var customFieldValuesJSON []byte
	if len(c.CustomFieldValues) > 0 {
		var err error
		customFieldValuesJSON, err = json.Marshal(c.CustomFieldValues)
		if err != nil {
			respondValidationError(w, r, "Invalid custom field values")
			return
		}
	}

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

	respondJSONOK(w, c)
}

func (h *TimeCustomerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	// Check permission
	if _, ok := h.checkCustomerPermission(w, r); !ok {
		return
	}

	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	//nolint:misspell // "organisations" is intentional British spelling used throughout codebase
	_, err := h.db.ExecWrite("DELETE FROM customer_organisations WHERE id = ?", id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
