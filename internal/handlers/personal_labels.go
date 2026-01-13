package handlers

import (
	"windshift/internal/database"
	"database/sql"
	"encoding/json"
	"windshift/internal/models"
	"net/http"
	"strconv"
	"strings"
	"time"

)

type PersonalLabelHandler struct {
	db database.Database
}

func NewPersonalLabelHandler(db database.Database) *PersonalLabelHandler {
	return &PersonalLabelHandler{db: db}
}

func (h *PersonalLabelHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	userID := r.URL.Query().Get("user_id")
	
	query := `
		SELECT id, name, color, user_id, created_at, updated_at
		FROM personal_labels
		WHERE 1=1`
	
	var args []interface{}
	
	// Filter by user_id if provided, otherwise show global labels (user_id IS NULL)
	if userID != "" {
		if userID == "null" || userID == "0" {
			query += " AND user_id IS NULL"
		} else {
			query += " AND (user_id = ? OR user_id IS NULL)"
			if id, err := strconv.Atoi(userID); err == nil {
				args = append(args, id)
			}
		}
	} else {
		// Default: show only global labels
		query += " AND user_id IS NULL"
	}
	
	query += " ORDER BY name"
	
	rows, err := h.db.Query(query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var labels []models.PersonalLabel
	for rows.Next() {
		var label models.PersonalLabel
		var userID sql.NullInt64
		
		err := rows.Scan(&label.ID, &label.Name, &label.Color, &userID, 
			&label.CreatedAt, &label.UpdatedAt)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		
		if userID.Valid {
			id := int(userID.Int64)
			label.UserID = &id
		}
		
		labels = append(labels, label)
	}

	// Always return an array, even if empty
	if labels == nil {
		labels = []models.PersonalLabel{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(labels)
}

func (h *PersonalLabelHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	var label models.PersonalLabel
	var userID sql.NullInt64
	
	err = h.db.QueryRow(`
		SELECT id, name, color, user_id, created_at, updated_at
		FROM personal_labels
		WHERE id = ?
	`, id).Scan(&label.ID, &label.Name, &label.Color, &userID, 
		&label.CreatedAt, &label.UpdatedAt)
	
	if err == sql.ErrNoRows {
		http.Error(w, "Personal label not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if userID.Valid {
		id := int(userID.Int64)
		label.UserID = &id
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(label)
}

func (h *PersonalLabelHandler) Create(w http.ResponseWriter, r *http.Request) {
	var label models.PersonalLabel
	if err := json.NewDecoder(r.Body).Decode(&label); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if strings.TrimSpace(label.Name) == "" {
		http.Error(w, "Label name is required", http.StatusBadRequest)
		return
	}

	// Set default color
	label.Color = "#3B82F6" // Default blue

	// Check for duplicate name within the same scope (global or user-specific)
	var existingCount int
	if label.UserID != nil {
		err := h.db.QueryRow("SELECT COUNT(*) FROM personal_labels WHERE name = ? AND user_id = ?", 
			label.Name, *label.UserID).Scan(&existingCount)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		err := h.db.QueryRow("SELECT COUNT(*) FROM personal_labels WHERE name = ? AND user_id IS NULL", 
			label.Name).Scan(&existingCount)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	
	if existingCount > 0 {
		http.Error(w, "A label with this name already exists", http.StatusBadRequest)
		return
	}

	now := time.Now()
	var id int64
	err := h.db.QueryRow(`
		INSERT INTO personal_labels (name, color, user_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?) RETURNING id
	`, label.Name, label.Color, label.UserID, now, now).Scan(&id)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	// Return the created label
	var createdLabel models.PersonalLabel
	var userID sql.NullInt64
	
	err = h.db.QueryRow(`
		SELECT id, name, color, user_id, created_at, updated_at
		FROM personal_labels
		WHERE id = ?
	`, id).Scan(&createdLabel.ID, &createdLabel.Name, &createdLabel.Color, &userID, 
		&createdLabel.CreatedAt, &createdLabel.UpdatedAt)
	
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if userID.Valid {
		id := int(userID.Int64)
		createdLabel.UserID = &id
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(createdLabel)
}

func (h *PersonalLabelHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	var label models.PersonalLabel
	if err := json.NewDecoder(r.Body).Decode(&label); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if strings.TrimSpace(label.Name) == "" {
		http.Error(w, "Label name is required", http.StatusBadRequest)
		return
	}

	// Set default color
	label.Color = "#3B82F6" // Default blue

	// Check for duplicate name within the same scope (excluding current record)
	var existingCount int
	if label.UserID != nil {
		err := h.db.QueryRow("SELECT COUNT(*) FROM personal_labels WHERE name = ? AND user_id = ? AND id != ?", 
			label.Name, *label.UserID, id).Scan(&existingCount)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		err := h.db.QueryRow("SELECT COUNT(*) FROM personal_labels WHERE name = ? AND user_id IS NULL AND id != ?", 
			label.Name, id).Scan(&existingCount)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	
	if existingCount > 0 {
		http.Error(w, "A label with this name already exists", http.StatusBadRequest)
		return
	}

	now := time.Now()
	_, err = h.db.ExecWrite(`
		UPDATE personal_labels 
		SET name = ?, color = ?, user_id = ?, updated_at = ?
		WHERE id = ?
	`, label.Name, label.Color, label.UserID, now, id)
	
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the updated label
	var updatedLabel models.PersonalLabel
	var userID sql.NullInt64
	
	err = h.db.QueryRow(`
		SELECT id, name, color, user_id, created_at, updated_at
		FROM personal_labels
		WHERE id = ?
	`, id).Scan(&updatedLabel.ID, &updatedLabel.Name, &updatedLabel.Color, &userID, 
		&updatedLabel.CreatedAt, &updatedLabel.UpdatedAt)
	
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if userID.Valid {
		id := int(userID.Int64)
		updatedLabel.UserID = &id
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updatedLabel)
}

func (h *PersonalLabelHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	_, err = h.db.ExecWrite("DELETE FROM personal_labels WHERE id = ?", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}