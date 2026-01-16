package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"windshift/internal/database"
)

// SecuritySettingsHandler handles admin security settings
type SecuritySettingsHandler struct {
	db              database.Database
	pluginsDisabled bool
}

// NewSecuritySettingsHandler creates a new security settings handler
func NewSecuritySettingsHandler(db database.Database, pluginsDisabled bool) *SecuritySettingsHandler {
	return &SecuritySettingsHandler{db: db, pluginsDisabled: pluginsDisabled}
}

// SecuritySettings represents the security configuration
type SecuritySettings struct {
	CalendarFeedEnabled  bool `json:"calendar_feed_enabled"`
	PluginCLIExecEnabled bool `json:"plugin_cli_exec_enabled"`
	PluginsDisabled      bool `json:"plugins_disabled"`
}

// GetSecuritySettings returns current security settings
func (h *SecuritySettingsHandler) GetSecuritySettings(w http.ResponseWriter, r *http.Request) {
	settings := SecuritySettings{
		CalendarFeedEnabled:  true,                // Default enabled
		PluginCLIExecEnabled: false,               // Default disabled for security
		PluginsDisabled:      h.pluginsDisabled,   // Read-only, set by startup flag
	}

	// Get calendar_feed_enabled setting
	var value string
	err := h.db.QueryRow("SELECT value FROM system_settings WHERE key = 'calendar_feed_enabled'").Scan(&value)
	if err == nil {
		settings.CalendarFeedEnabled = strings.ToLower(value) == "true"
	}

	// Get plugin_cli_exec_enabled setting
	err = h.db.QueryRow("SELECT value FROM system_settings WHERE key = 'plugin_cli_exec_enabled'").Scan(&value)
	if err == nil {
		settings.PluginCLIExecEnabled = strings.ToLower(value) == "true"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(settings)
}

// UpdateSecuritySettings updates security settings
func (h *SecuritySettingsHandler) UpdateSecuritySettings(w http.ResponseWriter, r *http.Request) {
	var settings SecuritySettings
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Update calendar_feed_enabled
	value := "false"
	if settings.CalendarFeedEnabled {
		value = "true"
	}

	// Update or insert the setting
	_, err := h.db.Exec(`
		UPDATE system_settings SET value = ?, updated_at = CURRENT_TIMESTAMP
		WHERE key = 'calendar_feed_enabled'
	`, value)
	if err != nil {
		http.Error(w, "Failed to update settings", http.StatusInternalServerError)
		return
	}

	// Update plugin_cli_exec_enabled
	value = "false"
	if settings.PluginCLIExecEnabled {
		value = "true"
	}

	// Try UPDATE first, then INSERT if row doesn't exist
	result, err := h.db.Exec(`
		UPDATE system_settings SET value = ?, updated_at = CURRENT_TIMESTAMP
		WHERE key = 'plugin_cli_exec_enabled'
	`, value)
	if err != nil {
		http.Error(w, "Failed to update settings", http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		// Row doesn't exist, insert it
		_, err = h.db.Exec(`
			INSERT INTO system_settings (key, value, value_type, description, category, created_at, updated_at)
			VALUES ('plugin_cli_exec_enabled', ?, 'boolean', 'Allow plugins to execute CLI commands', 'security', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, value)
		if err != nil {
			http.Error(w, "Failed to create settings", http.StatusInternalServerError)
			return
		}
	}

	// Return updated settings
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(settings)
}
