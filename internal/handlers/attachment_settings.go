package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"windshift/internal/models"
	"windshift/internal/database"

)

type AttachmentSettingsHandler struct {
	*BaseHandler
}

func NewAttachmentSettingsHandler(db database.Database) *AttachmentSettingsHandler {
	// Legacy constructor for backward compatibility
	panic("Use NewAttachmentSettingsHandlerWithPool instead")
}

func NewAttachmentSettingsHandlerWithPool(db database.Database) *AttachmentSettingsHandler {
	return &AttachmentSettingsHandler{
		BaseHandler: NewBaseHandler(db),
	}
}

// Get retrieves current attachment settings
func (h *AttachmentSettingsHandler) Get(w http.ResponseWriter, r *http.Request) {
	settings := &models.AttachmentSettings{
		ID:               1,
		MaxFileSize:      52428800, // 50MB default
		AllowedMimeTypes: "[]",     // Empty array by default (all types allowed)
		AttachmentPath:   "",
		Enabled:          false, // Disabled by default if no path is set
	}

	// Try to get settings from database
	err := h.getReadDB().QueryRow(`
		SELECT id, max_file_size, allowed_mime_types, attachment_path, enabled, created_at, updated_at
		FROM attachment_settings ORDER BY id DESC LIMIT 1
	`).Scan(
		&settings.ID, &settings.MaxFileSize, &settings.AllowedMimeTypes,
		&settings.AttachmentPath, &settings.Enabled, &settings.CreatedAt, &settings.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		// No settings in database, return defaults
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(settings)
		return
	}

	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(settings)
}

// Update modifies attachment settings
func (h *AttachmentSettingsHandler) Update(w http.ResponseWriter, r *http.Request) {
	settingsID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid settings ID", http.StatusBadRequest)
		return
	}

	var req models.AttachmentSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate max file size
	if req.MaxFileSize <= 0 {
		http.Error(w, "Max file size must be greater than 0", http.StatusBadRequest)
		return
	}

	// Convert allowed MIME types to JSON string
	allowedMimeTypesJSON, err := json.Marshal(req.AllowedMimeTypes)
	if err != nil {
		http.Error(w, "Invalid allowed MIME types", http.StatusBadRequest)
		return
	}

	// Check if settings record exists
	var exists bool
	err = h.getReadDB().QueryRow("SELECT EXISTS(SELECT 1 FROM attachment_settings WHERE id = ?)", settingsID).Scan(&exists)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	if exists {
		// Update existing settings
		_, err = h.getWriteDB().Exec(`
			UPDATE attachment_settings 
			SET max_file_size = ?, allowed_mime_types = ?, enabled = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, req.MaxFileSize, string(allowedMimeTypesJSON), req.Enabled, settingsID)
	} else {
		// Create new settings record
		_, err = h.getWriteDB().Exec(`
			INSERT INTO attachment_settings (id, max_file_size, allowed_mime_types, attachment_path, enabled)
			VALUES (?, ?, ?, ?, ?)
		`, settingsID, req.MaxFileSize, string(allowedMimeTypesJSON), "", req.Enabled)
	}

	if err != nil {
		http.Error(w, "Failed to save settings", http.StatusInternalServerError)
		return
	}

	// Return updated settings
	h.Get(w, r)
}

// GetStatus returns attachment system status (enabled/disabled, path info)
func (h *AttachmentSettingsHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	// This would be populated from the main application's attachment path setting
	status := map[string]interface{}{
		"enabled":         false,
		"attachment_path": "",
		"writable":        false,
	}

	// Try to get current settings
	var enabled bool
	var attachmentPath string
	err := h.getReadDB().QueryRow(`
		SELECT enabled, attachment_path FROM attachment_settings ORDER BY id DESC LIMIT 1
	`).Scan(&enabled, &attachmentPath)

	if err == sql.ErrNoRows {
		// No settings in database
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
		return
	}

	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	status["enabled"] = enabled
	status["attachment_path"] = attachmentPath

	// Check if path is writable (if it exists)
	if attachmentPath != "" {
		writable := false

		// Check if path exists and is a directory
		info, err := os.Stat(attachmentPath)
		if err == nil && info.IsDir() {
			// Try to create a test file to verify write permissions
			testFile := filepath.Join(attachmentPath, ".write-test")
			if f, err := os.Create(testFile); err == nil {
				f.Close()
				os.Remove(testFile)
				writable = true
			}
		}

		status["writable"] = writable
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}
