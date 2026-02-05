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
		respondInternalError(w, r, fmt.Errorf("failed to query themes: %w", err))
		return
	}
	defer func() { _ = rows.Close() }()

	var themes []models.Theme
	for rows.Next() {
		var theme models.Theme
		err = rows.Scan(
			&theme.ID, &theme.Name, &theme.Description,
			&theme.IsDefault, &theme.IsActive,
			&theme.NavBackgroundColorLight, &theme.NavTextColorLight,
			&theme.NavBackgroundColorDark, &theme.NavTextColorDark,
			&theme.CreatedAt, &theme.UpdatedAt,
		)
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to scan theme: %w", err))
			return
		}
		themes = append(themes, theme)
	}

	if err = rows.Err(); err != nil {
		respondInternalError(w, r, fmt.Errorf("error iterating themes: %w", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(themes)
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
		_, _ = w.Write([]byte("null"))
		return
	}
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to get active theme: %w", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(theme)
}

// CreateTheme creates a new theme
func (h *ThemeHandler) CreateTheme(w http.ResponseWriter, r *http.Request) {
	var req models.ThemeCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Validate required fields
	if req.Name == "" {
		respondValidationError(w, r, "Name is required")
		return
	}
	if req.NavBackgroundColorLight == "" {
		respondValidationError(w, r, "Navigation background color (light) is required")
		return
	}
	if req.NavTextColorLight == "" {
		respondValidationError(w, r, "Navigation text color (light) is required")
		return
	}
	if req.NavBackgroundColorDark == "" {
		respondValidationError(w, r, "Navigation background color (dark) is required")
		return
	}
	if req.NavTextColorDark == "" {
		respondValidationError(w, r, "Navigation text color (dark) is required")
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
		respondInternalError(w, r, fmt.Errorf("failed to create theme: %w", err))
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
		respondInternalError(w, r, fmt.Errorf("failed to get created theme: %w", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(theme)
}

// UpdateTheme updates an existing theme
func (h *ThemeHandler) UpdateTheme(w http.ResponseWriter, r *http.Request) {
	themeID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	var req models.ThemeUpdateRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Validate required fields
	if req.Name == "" {
		respondValidationError(w, r, "Name is required")
		return
	}
	if req.NavBackgroundColorLight == "" {
		respondValidationError(w, r, "Navigation background color (light) is required")
		return
	}
	if req.NavTextColorLight == "" {
		respondValidationError(w, r, "Navigation text color (light) is required")
		return
	}
	if req.NavBackgroundColorDark == "" {
		respondValidationError(w, r, "Navigation background color (dark) is required")
		return
	}
	if req.NavTextColorDark == "" {
		respondValidationError(w, r, "Navigation text color (dark) is required")
		return
	}

	// If activating this theme, deactivate all others
	if req.IsActive {
		_, err = h.DB.Exec("UPDATE themes SET is_active = false WHERE id != ?", themeID)
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to deactivate other themes: %w", err))
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
		respondInternalError(w, r, fmt.Errorf("failed to update theme: %w", err))
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
		respondInternalError(w, r, fmt.Errorf("failed to get updated theme: %w", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(theme)
}

// DeleteTheme deletes a theme
func (h *ThemeHandler) DeleteTheme(w http.ResponseWriter, r *http.Request) {
	themeID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	// Check if theme exists and is not default
	var isDefault bool
	err = h.DB.QueryRow("SELECT is_default FROM themes WHERE id = ?", themeID).Scan(&isDefault)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "theme")
			return
		}
		respondInternalError(w, r, fmt.Errorf("failed to check theme: %w", err))
		return
	}

	if isDefault {
		respondValidationError(w, r, "Cannot delete default theme")
		return
	}

	_, err = h.DB.Exec("DELETE FROM themes WHERE id = ?", themeID)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to delete theme: %w", err))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ActivateTheme sets a theme as active
func (h *ThemeHandler) ActivateTheme(w http.ResponseWriter, r *http.Request) {
	themeID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	// Check if theme exists
	var exists bool
	err = h.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM themes WHERE id = ?)", themeID).Scan(&exists)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to check theme: %w", err))
		return
	}

	if !exists {
		respondNotFound(w, r, "theme")
		return
	}

	// Deactivate all themes and activate the selected one
	_, err = h.DB.Exec("UPDATE themes SET is_active = false")
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to deactivate themes: %w", err))
		return
	}

	_, err = h.DB.Exec("UPDATE themes SET is_active = true, updated_at = ? WHERE id = ?", time.Now(), themeID)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to activate theme: %w", err))
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"success": true}`))
}
