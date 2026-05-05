package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"windshift/internal/logger"
	"windshift/internal/repository"
	"windshift/internal/utils"
)

// SecuritySettingsHandler handles admin security settings
type SecuritySettingsHandler struct {
	settings        *repository.SystemSettingRepository
	auditor         *logger.Auditor
	pluginsDisabled bool
}

// NewSecuritySettingsHandler creates a new security settings handler
func NewSecuritySettingsHandler(settings *repository.SystemSettingRepository, auditor *logger.Auditor, pluginsDisabled bool) *SecuritySettingsHandler {
	return &SecuritySettingsHandler{
		settings:        settings,
		auditor:         auditor,
		pluginsDisabled: pluginsDisabled,
	}
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

	if v, ok, _ := h.settings.GetValue("calendar_feed_enabled"); ok {
		settings.CalendarFeedEnabled = strings.EqualFold(v, "true")
	}
	if v, ok, _ := h.settings.GetValue("plugin_cli_exec_enabled"); ok {
		settings.PluginCLIExecEnabled = strings.EqualFold(v, "true")
	}
	if v, ok, _ := h.settings.GetValue("api_key_creation_policy"); ok {
		settings.APIKeyCreationPolicy = v
	}
	if v, ok, _ := h.settings.GetValue("api_key_allowed_group_ids"); ok {
		var groupIDs []int
		if json.Unmarshal([]byte(v), &groupIDs) == nil {
			settings.APIKeyAllowedGroupIDs = groupIDs
		}
	}
	if v, ok, _ := h.settings.GetValue("allow_user_managed_agents"); ok {
		settings.AllowUserManagedAgents = strings.EqualFold(v, "true")
	}
	if v, ok, _ := h.settings.GetValue("max_agents_per_user"); ok {
		if n, parseErr := strconv.Atoi(v); parseErr == nil && n >= 0 {
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

	if err := h.settings.Upsert(
		"calendar_feed_enabled", boolToString(settings.CalendarFeedEnabled),
		"boolean", "Allow public calendar feed URLs", "security",
	); err != nil {
		respondInternalError(w, r, err)
		return
	}

	if err := h.settings.Upsert(
		"plugin_cli_exec_enabled", boolToString(settings.PluginCLIExecEnabled),
		"boolean", "Allow plugins to execute CLI commands", "security",
	); err != nil {
		respondInternalError(w, r, err)
		return
	}

	if settings.APIKeyCreationPolicy == "" {
		settings.APIKeyCreationPolicy = "all_users"
	}
	if err := h.settings.Upsert(
		"api_key_creation_policy", settings.APIKeyCreationPolicy,
		"string", "API key creation policy", "security",
	); err != nil {
		respondInternalError(w, r, err)
		return
	}

	groupIDsJSON, _ := json.Marshal(settings.APIKeyAllowedGroupIDs)
	if err := h.settings.Upsert(
		"api_key_allowed_group_ids", string(groupIDsJSON),
		"json", "Allowed group IDs for API key creation", "security",
	); err != nil {
		respondInternalError(w, r, err)
		return
	}

	if err := h.settings.Upsert(
		"allow_user_managed_agents", boolToString(settings.AllowUserManagedAgents),
		"boolean", "Allow non-admin users to create and manage their own agent users from their profile", "security",
	); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Clamp max_agents_per_user to a sane range so an admin can't accidentally
	// unbound it or set a negative cap.
	capVal := settings.MaxAgentsPerUser
	if capVal < 0 {
		capVal = 0
	}
	if capVal > 1000 {
		capVal = 1000
	}
	settings.MaxAgentsPerUser = capVal
	if err := h.settings.Upsert(
		"max_agents_per_user", strconv.Itoa(capVal),
		"integer", "Maximum number of owned agents a single non-admin user may create", "security",
	); err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		h.auditor.LogWithDetails(r, currentUser,
			logger.ActionSecuritySettingsUpdate,
			logger.ResourceSecuritySettings,
			nil, "security_settings",
			map[string]interface{}{
				"calendar_feed_enabled":     settings.CalendarFeedEnabled,
				"plugin_cli_exec_enabled":   settings.PluginCLIExecEnabled,
				"api_key_creation_policy":   settings.APIKeyCreationPolicy,
				"api_key_allowed_group_ids": settings.APIKeyAllowedGroupIDs,
				"allow_user_managed_agents": settings.AllowUserManagedAgents,
				"max_agents_per_user":       settings.MaxAgentsPerUser,
			},
		)
	}

	respondJSONOK(w, settings)
}

func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
