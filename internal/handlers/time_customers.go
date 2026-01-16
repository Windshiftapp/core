package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"time"
	"windshift/internal/database"
	"windshift/internal/models"
)

type TimeCustomerHandler struct {
	db database.Database
}

func NewTimeCustomerHandler(db database.Database) *TimeCustomerHandler {
	return &TimeCustomerHandler{db: db}
}

func (h *TimeCustomerHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(`
		SELECT id, name, email, description, active, avatar_url, custom_field_values, created_at, updated_at
		FROM customer_organisations
		ORDER BY name ASC
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var customers []models.CustomerOrganisation
	for rows.Next() {
		var c models.CustomerOrganisation
		var customFieldValuesStr sql.NullString
		var avatarURL sql.NullString
		err := rows.Scan(&c.ID, &c.Name, &c.Email, &c.Description, &c.Active, &avatarURL, &customFieldValuesStr, &c.CreatedAt, &c.UpdatedAt)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var c models.CustomerOrganisation
	var customFieldValuesStr sql.NullString
	var avatarURL sql.NullString
	err := h.db.QueryRow(`
		SELECT id, name, email, description, active, avatar_url, custom_field_values, created_at, updated_at
		FROM customer_organisations
		WHERE id = ?
	`, id).Scan(&c.ID, &c.Name, &c.Email, &c.Description, &c.Active, &avatarURL, &customFieldValuesStr, &c.CreatedAt, &c.UpdatedAt)

	if err == sql.ErrNoRows {
		http.Error(w, "Customer not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Set avatar URL
	if avatarURL.Valid {
		c.AvatarURL = avatarURL.String
	}

	// Parse custom field values
	if customFieldValuesStr.Valid && customFieldValuesStr.String != "" {
		if err := json.Unmarshal([]byte(customFieldValuesStr.String), &c.CustomFieldValues); err != nil {
			http.Error(w, "Failed to parse custom field values", http.StatusInternalServerError)
			return
		}
	}

	respondJSONOK(w, c)
}

func (h *TimeCustomerHandler) Create(w http.ResponseWriter, r *http.Request) {
	var c models.CustomerOrganisation
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Set default active status if not explicitly provided
	// Note: In JSON, if 'active' field is missing, it will be false
	// Only set to true if it's actually missing from the request
	// For now, we'll trust the frontend to send the correct value

	// Serialize custom field values to JSON
	var customFieldValuesJSON []byte
	if c.CustomFieldValues != nil && len(c.CustomFieldValues) > 0 {
		var err error
		customFieldValuesJSON, err = json.Marshal(c.CustomFieldValues)
		if err != nil {
			http.Error(w, "Invalid custom field values", http.StatusBadRequest)
			return
		}
	}

	now := time.Now()
	var id int64
	err := h.db.QueryRow(`
		INSERT INTO customer_organisations (name, email, description, active, avatar_url, custom_field_values, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, c.Name, c.Email, c.Description, c.Active, c.AvatarURL, customFieldValuesJSON, now, now).Scan(&id)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	c.ID = int(id)
	c.CreatedAt = now
	c.UpdatedAt = now

	respondJSONCreated(w, c)
}

func (h *TimeCustomerHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var c models.CustomerOrganisation
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("[DEBUG] Update customer %d - received: name=%q, avatar_url=%q", id, c.Name, c.AvatarURL)

	// Serialize custom field values to JSON
	var customFieldValuesJSON []byte
	if c.CustomFieldValues != nil && len(c.CustomFieldValues) > 0 {
		var err error
		customFieldValuesJSON, err = json.Marshal(c.CustomFieldValues)
		if err != nil {
			http.Error(w, "Invalid custom field values", http.StatusBadRequest)
			return
		}
	}

	_, err := h.db.ExecWrite(`
		UPDATE customer_organisations
		SET name = ?, email = ?, description = ?, active = ?, avatar_url = ?, custom_field_values = ?, updated_at = ?
		WHERE id = ?
	`, c.Name, c.Email, c.Description, c.Active, c.AvatarURL, customFieldValuesJSON, time.Now(), id)

	if err != nil {
		log.Printf("[DEBUG] Update customer %d - ERROR: %v", id, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("[DEBUG] Update customer %d - SUCCESS, avatar_url saved: %q", id, c.AvatarURL)

	c.ID = id
	c.UpdatedAt = time.Now()

	respondJSONOK(w, c)
}

func (h *TimeCustomerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	_, err := h.db.ExecWrite("DELETE FROM customer_organisations WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
