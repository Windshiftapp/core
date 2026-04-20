package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/utils"
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
	CalendarFeedEnabled    bool   `json:"calendar_feed_enabled"`
	PluginCLIExecEnabled   bool   `json:"plugin_cli_exec_enabled"`
	PluginsDisabled        bool   `json:"plugins_disabled"`
	APIKeyCreationPolicy   string `json:"api_key_creation_policy"`   // "all_users", "groups_only", or "disabled"
	APIKeyAllowedGroupIDs  []int  `json:"api_key_allowed_group_ids"` // Group IDs when policy = "groups_only"
	AllowUserManagedAgents bool   `json:"allow_user_managed_agents"` // When true, non-admin users may create and administer their own agent users from profile
	MaxAgentsPerUser       int    `json:"max_agents_per_user"`       // Cap on owned agents per non-admin user (service users are not counted)
}

// GetSecuritySettings returns current security settings
func (h *SecuritySettingsHandler) GetSecuritySettings(w http.ResponseWriter, r *http.Request) {
	settings := SecuritySettings{
		CalendarFeedEnabled:    true,              // Default enabled
		PluginCLIExecEnabled:   false,             // Default disabled for security
		PluginsDisabled:        h.pluginsDisabled, // Read-only, set by startup flag
		APIKeyCreationPolicy:   "all_users",       // Default: everyone can create
		APIKeyAllowedGroupIDs:  []int{},
		AllowUserManagedAgents: false, // Default: locked down
		MaxAgentsPerUser:       5,
	}

	// Get calendar_feed_enabled setting
	var value string
	err := h.db.QueryRow("SELECT value FROM system_settings WHERE key = 'calendar_feed_enabled'").Scan(&value)
	if err == nil {
		settings.CalendarFeedEnabled = strings.EqualFold(value, "true")
	}

	// Get plugin_cli_exec_enabled setting
	err = h.db.QueryRow("SELECT value FROM system_settings WHERE key = 'plugin_cli_exec_enabled'").Scan(&value)
	if err == nil {
		settings.PluginCLIExecEnabled = strings.EqualFold(value, "true")
	}

	// Get api_key_creation_policy setting
	err = h.db.QueryRow("SELECT value FROM system_settings WHERE key = 'api_key_creation_policy'").Scan(&value)
	if err == nil {
		settings.APIKeyCreationPolicy = value
	}

	// Get api_key_allowed_group_ids setting
	err = h.db.QueryRow("SELECT value FROM system_settings WHERE key = 'api_key_allowed_group_ids'").Scan(&value)
	if err == nil {
		var groupIDs []int
		if json.Unmarshal([]byte(value), &groupIDs) == nil {
			settings.APIKeyAllowedGroupIDs = groupIDs
		}
	}

	// Get allow_user_managed_agents setting
	err = h.db.QueryRow("SELECT value FROM system_settings WHERE key = 'allow_user_managed_agents'").Scan(&value)
	if err == nil {
		settings.AllowUserManagedAgents = strings.EqualFold(value, "true")
	}

	// Get max_agents_per_user setting
	err = h.db.QueryRow("SELECT value FROM system_settings WHERE key = 'max_agents_per_user'").Scan(&value)
	if err == nil {
		if n, parseErr := strconv.Atoi(value); parseErr == nil && n >= 0 {
			settings.MaxAgentsPerUser = n
		}
	}

	respondJSONOK(w, settings)
}

// UpdateSecuritySettings updates security settings
func (h *SecuritySettingsHandler) UpdateSecuritySettings(w http.ResponseWriter, r *http.Request) {
	settings, ok := decodeJSON[SecuritySettings](w, r)
	if !ok {
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
		respondInternalError(w, r, err)
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
		respondInternalError(w, r, err)
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
			respondInternalError(w, r, err)
			return
		}
	}

	// Update api_key_creation_policy
	if settings.APIKeyCreationPolicy == "" {
		settings.APIKeyCreationPolicy = "all_users"
	}
	h.upsertSetting("api_key_creation_policy", settings.APIKeyCreationPolicy, "string", "API key creation policy")

	// Update api_key_allowed_group_ids
	groupIDsJSON, _ := json.Marshal(settings.APIKeyAllowedGroupIDs)
	h.upsertSetting("api_key_allowed_group_ids", string(groupIDsJSON), "json", "Allowed group IDs for API key creation")

	// Update allow_user_managed_agents
	umaValue := "false"
	if settings.AllowUserManagedAgents {
		umaValue = "true"
	}
	h.upsertSetting("allow_user_managed_agents", umaValue, "boolean", "Allow non-admin users to create and manage their own agent users from their profile")

	// Update max_agents_per_user (clamp to a sane range so an admin can't
	// accidentally unbound it or set a negative cap)
	capVal := settings.MaxAgentsPerUser
	if capVal < 0 {
		capVal = 0
	}
	if capVal > 1000 {
		capVal = 1000
	}
	h.upsertSetting("max_agents_per_user", strconv.Itoa(capVal), "integer", "Maximum number of owned agents a single non-admin user may create")

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionSecuritySettingsUpdate,
			ResourceType: logger.ResourceSecuritySettings,
			ResourceID:   nil,
			ResourceName: "security_settings",
			Details: map[string]interface{}{
				"calendar_feed_enabled":     settings.CalendarFeedEnabled,
				"plugin_cli_exec_enabled":   settings.PluginCLIExecEnabled,
				"api_key_creation_policy":   settings.APIKeyCreationPolicy,
				"api_key_allowed_group_ids": settings.APIKeyAllowedGroupIDs,
				"allow_user_managed_agents": settings.AllowUserManagedAgents,
				"max_agents_per_user":       settings.MaxAgentsPerUser,
			},
			Success: true,
		})
	}

	// Return updated settings
	respondJSONOK(w, settings)
}

// upsertSetting updates or inserts a system setting in the "security" category.
func (h *SecuritySettingsHandler) upsertSetting(key, value, valueType, description string) {
	result, err := h.db.Exec(`
		UPDATE system_settings SET value = ?, updated_at = CURRENT_TIMESTAMP
		WHERE key = ?
	`, value, key)
	if err != nil {
		return
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		_, _ = h.db.Exec(`
			INSERT INTO system_settings (key, value, value_type, description, category, created_at, updated_at)
			VALUES (?, ?, ?, ?, 'security', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, key, value, valueType, description)
	}
}
