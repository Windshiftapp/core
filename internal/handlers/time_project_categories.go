package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"
	"windshift/internal/database"
	"windshift/internal/models"
)

type TimeProjectCategoryHandler struct {
	db database.Database
}

func NewTimeProjectCategoryHandler(db database.Database) *TimeProjectCategoryHandler {
	return &TimeProjectCategoryHandler{db: db}
}

// GetCategories retrieves all time project categories
func (h *TimeProjectCategoryHandler) GetCategories(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(`
		SELECT id, name, description, color, display_order, created_at, updated_at
		FROM time_project_categories
		ORDER BY display_order ASC, name ASC
	`)
	if err != nil {
		http.Error(w, "Failed to fetch categories: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	categories := []models.TimeProjectCategory{}
	for rows.Next() {
		var c models.TimeProjectCategory
		var description, color sql.NullString

		err := rows.Scan(
			&c.ID,
			&c.Name,
			&description,
			&color,
			&c.DisplayOrder,
			&c.CreatedAt,
			&c.UpdatedAt,
		)
		if err != nil {
			http.Error(w, "Failed to scan category: "+err.Error(), http.StatusInternalServerError)
			return
		}

		if description.Valid {
			c.Description = description.String
		}
		if color.Valid {
			c.Color = color.String
		}

		categories = append(categories, c)
	}

	respondJSONOK(w, categories)
}

// GetCategory retrieves a single time project category by ID
func (h *TimeProjectCategoryHandler) GetCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var c models.TimeProjectCategory
	var description, color sql.NullString

	err := h.db.QueryRow(`
		SELECT id, name, description, color, display_order, created_at, updated_at
		FROM time_project_categories
		WHERE id = ?
	`, id).Scan(
		&c.ID,
		&c.Name,
		&description,
		&color,
		&c.DisplayOrder,
		&c.CreatedAt,
		&c.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		http.Error(w, "Category not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Failed to fetch category: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if description.Valid {
		c.Description = description.String
	}
	if color.Valid {
		c.Color = color.String
	}

	respondJSONOK(w, c)
}

// CreateCategory creates a new time project category
func (h *TimeProjectCategoryHandler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	var c models.TimeProjectCategory
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if c.Name == "" {
		http.Error(w, "Category name is required", http.StatusBadRequest)
		return
	}

	// Get max display_order to position new category at the end
	var maxOrder sql.NullInt64
	err := h.db.QueryRow("SELECT MAX(display_order) FROM time_project_categories").Scan(&maxOrder)
	if err != nil && err != sql.ErrNoRows {
		http.Error(w, "Failed to determine display order: "+err.Error(), http.StatusInternalServerError)
		return
	}

	displayOrder := 0
	if maxOrder.Valid {
		displayOrder = int(maxOrder.Int64) + 1
	}

	now := time.Now()
	result, err := h.db.Exec(`
		INSERT INTO time_project_categories (name, description, color, display_order, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, c.Name, c.Description, c.Color, displayOrder, now, now)

	if err != nil {
		http.Error(w, "Failed to create category: "+err.Error(), http.StatusInternalServerError)
		return
	}

	id, err := result.LastInsertId()
	if err != nil {
		http.Error(w, "Failed to get new category ID: "+err.Error(), http.StatusInternalServerError)
		return
	}

	c.ID = int(id)
	c.DisplayOrder = displayOrder
	c.CreatedAt = now
	c.UpdatedAt = now

	respondJSONCreated(w, c)
}

// UpdateCategory updates an existing time project category
func (h *TimeProjectCategoryHandler) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var c models.TimeProjectCategory
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if c.Name == "" {
		http.Error(w, "Category name is required", http.StatusBadRequest)
		return
	}

	// Check if category exists
	var exists bool
	err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM time_project_categories WHERE id = ?)", id).Scan(&exists)
	if err != nil {
		http.Error(w, "Failed to check category existence: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, "Category not found", http.StatusNotFound)
		return
	}

	now := time.Now()
	_, err = h.db.Exec(`
		UPDATE time_project_categories
		SET name = ?, description = ?, color = ?, display_order = ?, updated_at = ?
		WHERE id = ?
	`, c.Name, c.Description, c.Color, c.DisplayOrder, now, id)

	if err != nil {
		http.Error(w, "Failed to update category: "+err.Error(), http.StatusInternalServerError)
		return
	}

	c.ID = id
	c.UpdatedAt = now

	respondJSONOK(w, c)
}

// DeleteCategory deletes a time project category
func (h *TimeProjectCategoryHandler) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Check if any projects use this category
	var projectCount int
	err := h.db.QueryRow("SELECT COUNT(*) FROM time_projects WHERE category_id = ?", id).Scan(&projectCount)
	if err != nil {
		http.Error(w, "Failed to check category usage: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if projectCount > 0 {
		http.Error(w, "Cannot delete category: it is used by "+strconv.Itoa(projectCount)+" project(s)", http.StatusConflict)
		return
	}

	result, err := h.db.Exec("DELETE FROM time_project_categories WHERE id = ?", id)
	if err != nil {
		http.Error(w, "Failed to delete category: "+err.Error(), http.StatusInternalServerError)
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		http.Error(w, "Failed to check deletion result: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if rowsAffected == 0 {
		http.Error(w, "Category not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ReorderCategories updates the display order of multiple categories
func (h *TimeProjectCategoryHandler) ReorderCategories(w http.ResponseWriter, r *http.Request) {
	var orderUpdates []struct {
		ID           int `json:"id"`
		DisplayOrder int `json:"display_order"`
	}

	if err := json.NewDecoder(r.Body).Decode(&orderUpdates); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Update display order for each category
	now := time.Now()
	for _, update := range orderUpdates {
		_, err := h.db.Exec(`
			UPDATE time_project_categories
			SET display_order = ?, updated_at = ?
			WHERE id = ?
		`, update.DisplayOrder, now, update.ID)

		if err != nil {
			http.Error(w, "Failed to update category order: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	respondJSONOK(w, map[string]string{"message": "Category order updated successfully"})
}
