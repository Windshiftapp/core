package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"windshift/internal/models"
	"windshift/internal/utils"
)

type UserPreferencesHandler struct {
	DB interface {
		Query(query string, args ...interface{}) (*sql.Rows, error)
		QueryRow(query string, args ...interface{}) *sql.Row
		Exec(query string, args ...interface{}) (sql.Result, error)
	}
}

func NewUserPreferencesHandler(db interface {
	Query(query string, args ...interface{}) (*sql.Rows, error)
	QueryRow(query string, args ...interface{}) *sql.Row
	Exec(query string, args ...interface{}) (sql.Result, error)
}) *UserPreferencesHandler {
	return &UserPreferencesHandler{DB: db}
}

// GetUserPreferences returns the current user's preferences
func (h *UserPreferencesHandler) GetUserPreferences(w http.ResponseWriter, r *http.Request) {
	user := utils.GetCurrentUser(r)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}

	// Get user preferences
	var prefsJSON string
	err := h.DB.QueryRow("SELECT preferences FROM user_preferences WHERE user_id = ?", user.ID).Scan(&prefsJSON)
	if errors.Is(err, sql.ErrNoRows) {
		// Return default preferences
		response := models.UserPreferencesResponse{
			ColorMode: "system",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Parse preferences JSON
	var prefs models.UserPreferencesData
	if err := json.Unmarshal([]byte(prefsJSON), &prefs); err != nil {
		respondInternalError(w, r, err)
		return
	}

	response := models.UserPreferencesResponse{
		ThemeID:   prefs.ThemeID,
		ColorMode: prefs.ColorMode,
	}
	if response.ColorMode == "" {
		response.ColorMode = "system"
	}

	// If theme_id is set, fetch the theme details
	if prefs.ThemeID != nil {
		var theme models.Theme
		err := h.DB.QueryRow(`
			SELECT id, name, description, is_default, is_active,
			       nav_background_color_light, nav_text_color_light,
			       nav_background_color_dark, nav_text_color_dark,
			       created_at, updated_at
			FROM themes WHERE id = ?
		`, *prefs.ThemeID).Scan(
			&theme.ID, &theme.Name, &theme.Description,
			&theme.IsDefault, &theme.IsActive,
			&theme.NavBackgroundColorLight, &theme.NavTextColorLight,
			&theme.NavBackgroundColorDark, &theme.NavTextColorDark,
			&theme.CreatedAt, &theme.UpdatedAt,
		)
		if err == nil {
			response.Theme = &theme
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// UpdateUserPreferences updates the current user's preferences
func (h *UserPreferencesHandler) UpdateUserPreferences(w http.ResponseWriter, r *http.Request) {
	user := utils.GetCurrentUser(r)
	if user == nil {
		respondUnauthorized(w, r)
		return
	}

	var req models.UserPreferencesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid JSON")
		return
	}

	// Validate color_mode
	if req.ColorMode != "" && req.ColorMode != "light" && req.ColorMode != "dark" && req.ColorMode != "system" {
		respondValidationError(w, r, "Invalid color_mode: must be 'light', 'dark', or 'system'")
		return
	}

	// Validate theme_id if provided
	if req.ThemeID != nil {
		var exists bool
		err := h.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM themes WHERE id = ?)", *req.ThemeID).Scan(&exists)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !exists {
			respondNotFound(w, r, "theme")
			return
		}
	}

	// Get existing preferences
	var existingPrefsJSON string
	var rowExists bool
	err := h.DB.QueryRow("SELECT preferences FROM user_preferences WHERE user_id = ?", user.ID).Scan(&existingPrefsJSON)
	if err == nil {
		rowExists = true
	} else if !errors.Is(err, sql.ErrNoRows) {
		respondInternalError(w, r, err)
		return
	}

	var prefs models.UserPreferencesData
	if rowExists {
		_ = json.Unmarshal([]byte(existingPrefsJSON), &prefs)
	}

	// Update with new values
	if req.ThemeID != nil {
		prefs.ThemeID = req.ThemeID
	}
	if req.ColorMode != "" {
		prefs.ColorMode = req.ColorMode
	}

	prefsBytes, _ := json.Marshal(prefs)
	prefsJSON := string(prefsBytes)
	now := time.Now()

	if rowExists {
		_, err = h.DB.Exec(
			"UPDATE user_preferences SET preferences = ?, updated_at = ? WHERE user_id = ?",
			prefsJSON, now, user.ID,
		)
	} else {
		_, err = h.DB.Exec(
			"INSERT INTO user_preferences (user_id, preferences, created_at, updated_at) VALUES (?, ?, ?, ?)",
			user.ID, prefsJSON, now, now,
		)
	}

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Return the updated preferences
	h.GetUserPreferences(w, r)
}
