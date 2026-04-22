package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/utils"
)

type ThemeHandler struct {
	DB interface {
		Query(query string, args ...interface{}) (*sql.Rows, error)
		QueryRow(query string, args ...interface{}) *sql.Row
		Exec(query string, args ...interface{}) (sql.Result, error)
	}
	auditDB database.Database
}

func NewThemeHandler(db interface {
	Query(query string, args ...interface{}) (*sql.Rows, error)
	QueryRow(query string, args ...interface{}) *sql.Row
	Exec(query string, args ...interface{}) (sql.Result, error)
}, auditDB database.Database) *ThemeHandler {
	return &ThemeHandler{DB: db, auditDB: auditDB}
}

// themeColumns is the shared SELECT column list used by every theme query.
const themeColumns = `id, name, description, is_default, is_active,
	nav_background_color_light, nav_text_color_light,
	nav_background_color_dark, nav_text_color_dark,
	created_at, updated_at`

// rowScanner abstracts sql.Row and sql.Rows for Scan.
type rowScanner interface {
	Scan(dest ...interface{}) error
}

// scanTheme reads a themes row from s into t.
func scanTheme(s rowScanner, t *models.Theme) error {
	return s.Scan(
		&t.ID, &t.Name, &t.Description,
		&t.IsDefault, &t.IsActive,
		&t.NavBackgroundColorLight, &t.NavTextColorLight,
		&t.NavBackgroundColorDark, &t.NavTextColorDark,
		&t.CreatedAt, &t.UpdatedAt,
	)
}

// getThemeByID fetches a single theme by primary key.
func (h *ThemeHandler) getThemeByID(id int64) (models.Theme, error) {
	var theme models.Theme
	err := scanTheme(h.DB.QueryRow(`SELECT `+themeColumns+` FROM themes WHERE id = ?`, id), &theme)
	return theme, err
}

// GetThemes returns all themes
func (h *ThemeHandler) GetThemes(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(`SELECT ` + themeColumns + ` FROM themes ORDER BY is_default DESC, name ASC`)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to query themes: %w", err))
		return
	}
	defer func() { _ = rows.Close() }()

	var themes []models.Theme
	for rows.Next() {
		var theme models.Theme
		if err := scanTheme(rows, &theme); err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to scan theme: %w", err))
			return
		}
		themes = append(themes, theme)
	}

	if err := rows.Err(); err != nil {
		respondInternalError(w, r, fmt.Errorf("error iterating themes: %w", err))
		return
	}

	respondJSONOK(w, themes)
}

// GetActiveTheme returns the currently active theme
func (h *ThemeHandler) GetActiveTheme(w http.ResponseWriter, r *http.Request) {
	var theme models.Theme
	err := scanTheme(
		h.DB.QueryRow(`SELECT `+themeColumns+` FROM themes WHERE is_active = true ORDER BY is_default DESC LIMIT 1`),
		&theme,
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

	respondJSONOK(w, theme)
}

// validateThemeFields checks the required color and name fields shared by create and update requests.
func validateThemeFields(name, navBgLight, navTextLight, navBgDark, navTextDark string) string {
	if name == "" {
		return "Name is required"
	}
	if navBgLight == "" {
		return "Navigation background color (light) is required"
	}
	if navTextLight == "" {
		return "Navigation text color (light) is required"
	}
	if navBgDark == "" {
		return "Navigation background color (dark) is required"
	}
	if navTextDark == "" {
		return "Navigation text color (dark) is required"
	}
	return ""
}

// CreateTheme creates a new theme
func (h *ThemeHandler) CreateTheme(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[models.ThemeCreateRequest](w, r)
	if !ok {
		return
	}

	if msg := validateThemeFields(req.Name, req.NavBackgroundColorLight, req.NavTextColorLight, req.NavBackgroundColorDark, req.NavTextColorDark); msg != "" {
		respondValidationError(w, r, msg)
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

	theme, err := h.getThemeByID(themeID)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to get created theme: %w", err))
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		themeIDInt := int(themeID)
		logAudit(h.auditDB, r, currentUser, logger.ActionThemeCreate, logger.ResourceTheme, &themeIDInt, theme.Name)
	}

	respondJSONCreated(w, theme)
}

// UpdateTheme updates an existing theme
func (h *ThemeHandler) UpdateTheme(w http.ResponseWriter, r *http.Request) {
	themeID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := decodeJSON[models.ThemeUpdateRequest](w, r)
	if !ok {
		return
	}

	if msg := validateThemeFields(req.Name, req.NavBackgroundColorLight, req.NavTextColorLight, req.NavBackgroundColorDark, req.NavTextColorDark); msg != "" {
		respondValidationError(w, r, msg)
		return
	}

	// If activating this theme, deactivate all others
	if req.IsActive {
		if _, err := h.DB.Exec("UPDATE themes SET is_active = false WHERE id != ?", themeID); err != nil {
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
	_, err := h.DB.Exec(query, req.Name, req.Description, req.NavBackgroundColorLight, req.NavTextColorLight, req.NavBackgroundColorDark, req.NavTextColorDark, req.IsActive, now, themeID)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to update theme: %w", err))
		return
	}

	theme, err := h.getThemeByID(int64(themeID))
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to get updated theme: %w", err))
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logAudit(h.auditDB, r, currentUser, logger.ActionThemeUpdate, logger.ResourceTheme, &themeID, theme.Name)
	}

	respondJSONOK(w, theme)
}

// DeleteTheme deletes a theme
func (h *ThemeHandler) DeleteTheme(w http.ResponseWriter, r *http.Request) {
	themeID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var err error

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

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logAudit(h.auditDB, r, currentUser, logger.ActionThemeDelete, logger.ResourceTheme, &themeID, "")
	}

	w.WriteHeader(http.StatusNoContent)
}

// ActivateTheme sets a theme as active
func (h *ThemeHandler) ActivateTheme(w http.ResponseWriter, r *http.Request) {
	themeID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var err error

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

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logAudit(h.auditDB, r, currentUser, logger.ActionThemeActivate, logger.ResourceTheme, &themeID, "")
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"success": true}`))
}
