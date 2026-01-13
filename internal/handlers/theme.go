package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"windshift/internal/models"
)

type ThemeHandler struct {
	DB interface {
		Query(query string, args ...interface{}) (*sql.Rows, error)
		QueryRow(query string, args ...interface{}) *sql.Row
		Exec(query string, args ...interface{}) (sql.Result, error)
	}
}

func NewThemeHandler(db interface {
	Query(query string, args ...interface{}) (*sql.Rows, error)
	QueryRow(query string, args ...interface{}) *sql.Row
	Exec(query string, args ...interface{}) (sql.Result, error)
}) *ThemeHandler {
	return &ThemeHandler{DB: db}
}

// GetThemes returns all themes
func (h *ThemeHandler) GetThemes(w http.ResponseWriter, r *http.Request) {
	query := `
		SELECT id, name, description, is_default, is_active,
		       nav_background_color_light, nav_text_color_light,
		       nav_background_color_dark, nav_text_color_dark,
		       created_at, updated_at
		FROM themes
		ORDER BY is_default DESC, name ASC
	`

	rows, err := h.DB.Query(query)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to query themes: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var themes []models.Theme
	for rows.Next() {
		var theme models.Theme
		err := rows.Scan(
			&theme.ID, &theme.Name, &theme.Description,
			&theme.IsDefault, &theme.IsActive,
			&theme.NavBackgroundColorLight, &theme.NavTextColorLight,
			&theme.NavBackgroundColorDark, &theme.NavTextColorDark,
			&theme.CreatedAt, &theme.UpdatedAt,
		)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to scan theme: %v", err), http.StatusInternalServerError)
			return
		}
		themes = append(themes, theme)
	}

	if err = rows.Err(); err != nil {
		http.Error(w, fmt.Sprintf("Error iterating themes: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(themes)
}

// GetActiveTheme returns the currently active theme
func (h *ThemeHandler) GetActiveTheme(w http.ResponseWriter, r *http.Request) {
	query := `
		SELECT id, name, description, is_default, is_active,
		       nav_background_color_light, nav_text_color_light,
		       nav_background_color_dark, nav_text_color_dark,
		       created_at, updated_at
		FROM themes
		WHERE is_active = true
		ORDER BY is_default DESC
		LIMIT 1
	`

	var theme models.Theme
	err := h.DB.QueryRow(query).Scan(
		&theme.ID, &theme.Name, &theme.Description,
		&theme.IsDefault, &theme.IsActive,
		&theme.NavBackgroundColorLight, &theme.NavTextColorLight,
		&theme.NavBackgroundColorDark, &theme.NavTextColorDark,
		&theme.CreatedAt, &theme.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		// No active theme found - return null
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("null"))
		return
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get active theme: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(theme)
}

// CreateTheme creates a new theme
func (h *ThemeHandler) CreateTheme(w http.ResponseWriter, r *http.Request) {
	var req models.ThemeCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}
	if req.NavBackgroundColorLight == "" {
		http.Error(w, "Navigation background color (light) is required", http.StatusBadRequest)
		return
	}
	if req.NavTextColorLight == "" {
		http.Error(w, "Navigation text color (light) is required", http.StatusBadRequest)
		return
	}
	if req.NavBackgroundColorDark == "" {
		http.Error(w, "Navigation background color (dark) is required", http.StatusBadRequest)
		return
	}
	if req.NavTextColorDark == "" {
		http.Error(w, "Navigation text color (dark) is required", http.StatusBadRequest)
		return
	}

	query := `
		INSERT INTO themes (name, description, nav_background_color_light, nav_text_color_light, nav_background_color_dark, nav_text_color_dark, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`

	now := time.Now()
	var themeID int64
	err := h.DB.QueryRow(query, req.Name, req.Description, req.NavBackgroundColorLight, req.NavTextColorLight, req.NavBackgroundColorDark, req.NavTextColorDark, now, now).Scan(&themeID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create theme: %v", err), http.StatusInternalServerError)
		return
	}

	// Return the created theme
	getQuery := `
		SELECT id, name, description, is_default, is_active,
		       nav_background_color_light, nav_text_color_light,
		       nav_background_color_dark, nav_text_color_dark,
		       created_at, updated_at
		FROM themes
		WHERE id = ?
	`

	var theme models.Theme
	err = h.DB.QueryRow(getQuery, themeID).Scan(
		&theme.ID, &theme.Name, &theme.Description,
		&theme.IsDefault, &theme.IsActive,
		&theme.NavBackgroundColorLight, &theme.NavTextColorLight,
		&theme.NavBackgroundColorDark, &theme.NavTextColorDark,
		&theme.CreatedAt, &theme.UpdatedAt,
	)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get created theme: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(theme)
}

// UpdateTheme updates an existing theme
func (h *ThemeHandler) UpdateTheme(w http.ResponseWriter, r *http.Request) {
	themeID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid theme ID", http.StatusBadRequest)
		return
	}

	var req models.ThemeUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}
	if req.NavBackgroundColorLight == "" {
		http.Error(w, "Navigation background color (light) is required", http.StatusBadRequest)
		return
	}
	if req.NavTextColorLight == "" {
		http.Error(w, "Navigation text color (light) is required", http.StatusBadRequest)
		return
	}
	if req.NavBackgroundColorDark == "" {
		http.Error(w, "Navigation background color (dark) is required", http.StatusBadRequest)
		return
	}
	if req.NavTextColorDark == "" {
		http.Error(w, "Navigation text color (dark) is required", http.StatusBadRequest)
		return
	}

	// If activating this theme, deactivate all others
	if req.IsActive {
		_, err = h.DB.Exec("UPDATE themes SET is_active = false WHERE id != ?", themeID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to deactivate other themes: %v", err), http.StatusInternalServerError)
			return
		}
	}

	query := `
		UPDATE themes
		SET name = ?, description = ?, nav_background_color_light = ?, nav_text_color_light = ?,
		    nav_background_color_dark = ?, nav_text_color_dark = ?, is_active = ?, updated_at = ?
		WHERE id = ?
	`

	now := time.Now()
	_, err = h.DB.Exec(query, req.Name, req.Description, req.NavBackgroundColorLight, req.NavTextColorLight, req.NavBackgroundColorDark, req.NavTextColorDark, req.IsActive, now, themeID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to update theme: %v", err), http.StatusInternalServerError)
		return
	}

	// Return the updated theme
	getQuery := `
		SELECT id, name, description, is_default, is_active,
		       nav_background_color_light, nav_text_color_light,
		       nav_background_color_dark, nav_text_color_dark,
		       created_at, updated_at
		FROM themes
		WHERE id = ?
	`

	var theme models.Theme
	err = h.DB.QueryRow(getQuery, themeID).Scan(
		&theme.ID, &theme.Name, &theme.Description,
		&theme.IsDefault, &theme.IsActive,
		&theme.NavBackgroundColorLight, &theme.NavTextColorLight,
		&theme.NavBackgroundColorDark, &theme.NavTextColorDark,
		&theme.CreatedAt, &theme.UpdatedAt,
	)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get updated theme: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(theme)
}

// DeleteTheme deletes a theme
func (h *ThemeHandler) DeleteTheme(w http.ResponseWriter, r *http.Request) {
	themeID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid theme ID", http.StatusBadRequest)
		return
	}

	// Check if theme exists and is not default
	var isDefault bool
	err = h.DB.QueryRow("SELECT is_default FROM themes WHERE id = ?", themeID).Scan(&isDefault)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			http.Error(w, "Theme not found", http.StatusNotFound)
			return
		}
		http.Error(w, fmt.Sprintf("Failed to check theme: %v", err), http.StatusInternalServerError)
		return
	}

	if isDefault {
		http.Error(w, "Cannot delete default theme", http.StatusBadRequest)
		return
	}

	_, err = h.DB.Exec("DELETE FROM themes WHERE id = ?", themeID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to delete theme: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ActivateTheme sets a theme as active
func (h *ThemeHandler) ActivateTheme(w http.ResponseWriter, r *http.Request) {
	themeID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid theme ID", http.StatusBadRequest)
		return
	}

	// Check if theme exists
	var exists bool
	err = h.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM themes WHERE id = ?)", themeID).Scan(&exists)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to check theme: %v", err), http.StatusInternalServerError)
		return
	}

	if !exists {
		http.Error(w, "Theme not found", http.StatusNotFound)
		return
	}

	// Deactivate all themes and activate the selected one
	_, err = h.DB.Exec("UPDATE themes SET is_active = false")
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to deactivate themes: %v", err), http.StatusInternalServerError)
		return
	}

	_, err = h.DB.Exec("UPDATE themes SET is_active = true, updated_at = ? WHERE id = ?", time.Now(), themeID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to activate theme: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"success": true}`))
}