package handlers

import (
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"

	"windshift/internal/logger"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
	"windshift/internal/utils"
)

// SecuritySettingsHandler handles admin security settings
type SecuritySettingsHandler struct {
	settings              *repository.SystemSettingRepository
	auditor               *logger.Auditor
	pluginsDisabled       bool
	externalImagesAllowed atomic.Bool
}

// NewSecuritySettingsHandler creates a new security settings handler
func NewSecuritySettingsHandler(settings *repository.SystemSettingRepository, auditor *logger.Auditor, pluginsDisabled bool) *SecuritySettingsHandler {
	h := &SecuritySettingsHandler{
		settings:        settings,
		auditor:         auditor,
		pluginsDisabled: pluginsDisabled,
	}
	if value, ok, err := settings.GetValue("allow_external_images"); err == nil && ok {
		h.externalImagesAllowed.Store(strings.EqualFold(value, "true"))
	}
	return h
}

// ExternalImagesAllowed reports whether CSP may load images from arbitrary
// HTTP and HTTPS origins.
func (h *SecuritySettingsHandler) ExternalImagesAllowed() bool {
	return h.externalImagesAllowed.Load()
}

// SecuritySettings represents the security configuration
type SecuritySettings struct {
	CalendarFeedEnabled    bool   `json:"calendar_feed_enabled"`
	PluginCLIExecEnabled   bool   `json:"plugin_cli_exec_enabled"`
	AllowExternalImages    bool   `json:"allow_external_images"`
	PluginsDisabled        bool   `json:"plugins_disabled"`
	APIKeyCreationPolicy   string `json:"api_key_creation_policy"`   // "all_users", "groups_only", or "disabled"
	APIKeyAllowedGroupIDs  []int  `json:"api_key_allowed_group_ids"` // Group IDs when policy = "groups_only"
	AllowUserManagedAgents bool   `json:"allow_user_managed_agents"` // When true, non-admin users may create and administer their own agent users from profile
	MaxAgentsPerUser       int    `json:"max_agents_per_user"`       // Cap on owned agents per non-admin user (service users are not counted)
	WorkspaceManagedAgents bool   `json:"workspace_managed_agents"`  // When true, workspace admins may create workspace-owned agent identities
}

// GetSecuritySettings returns current security settings
func (h *SecuritySettingsHandler) GetSecuritySettings(w http.ResponseWriter, r *http.Request) {
	respondJSONOK(w, h.loadSecuritySettings())
}

// loadSecuritySettings reads the persisted security settings with the same
// defaults the API returns. UpdateSecuritySettings uses it to capture the
// previous values for the audit trail.
func (h *SecuritySettingsHandler) loadSecuritySettings() SecuritySettings {
	settings := SecuritySettings{
		CalendarFeedEnabled:    true,              // Default enabled
		PluginCLIExecEnabled:   false,             // Default disabled for security
		AllowExternalImages:    false,             // Default restricted to approved image origins
		PluginsDisabled:        h.pluginsDisabled, // Read-only, set by startup flag
		APIKeyCreationPolicy:   "all_users",       // Default: everyone can create
		APIKeyAllowedGroupIDs:  []int{},
		AllowUserManagedAgents: false, // Default: locked down
		MaxAgentsPerUser:       5,
		WorkspaceManagedAgents: true,
	}

	if v, ok, _ := h.settings.GetValue("calendar_feed_enabled"); ok {
		settings.CalendarFeedEnabled = strings.EqualFold(v, "true")
	}
	if v, ok, _ := h.settings.GetValue("plugin_cli_exec_enabled"); ok {
		settings.PluginCLIExecEnabled = strings.EqualFold(v, "true")
	}
	if v, ok, _ := h.settings.GetValue("allow_external_images"); ok {
		settings.AllowExternalImages = strings.EqualFold(v, "true")
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
	if v, ok, _ := h.settings.GetValue("workspace_managed_agents"); ok {
		settings.WorkspaceManagedAgents = strings.EqualFold(v, "true")
	}

	return settings
}

// UpdateSecuritySettings updates security settings
func (h *SecuritySettingsHandler) UpdateSecuritySettings(w http.ResponseWriter, r *http.Request) {
	settings, ok := decodeJSON[SecuritySettings](w, r)
	if !ok {
		return
	}
	// Policy is identifier-shaped ("all_users"/"groups_only"/"disabled")
	// and persisted + echoed verbatim in the settings UI and audit log.
	sanitize.Apply(&settings.APIKeyCreationPolicy, sanitize.ShortIdentifier)

	before := h.loadSecuritySettings()

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

	if err := h.settings.Upsert(
		"workspace_managed_agents", boolToString(settings.WorkspaceManagedAgents),
		"boolean", "Allow workspace admins to create agent identities owned by their workspace", "security",
	); err != nil {
		respondInternalError(w, r, err)
		return
	}

	if err := h.settings.Upsert(
		"allow_external_images", boolToString(settings.AllowExternalImages),
		"boolean", "Allow Markdown images from arbitrary HTTP and HTTPS origins", "security",
	); err != nil {
		respondInternalError(w, r, err)
		return
	}
	h.externalImagesAllowed.Store(settings.AllowExternalImages)

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		h.auditor.LogWithDetails(r, currentUser,
			logger.ActionSecuritySettingsUpdate,
			logger.ResourceSecuritySettings,
			nil, "security_settings",
			securitySettingsChanges(before, settings),
		)
	}

	respondJSONOK(w, settings)
}

// securitySettingsChanges reports each changed security setting with its old
// and new value, so the audit row shows what changed instead of only the final
// state.
func securitySettingsChanges(before, after SecuritySettings) map[string]any {
	changes := map[string]any{}
	add := func(field string, old, new any) {
		changes[field] = map[string]any{"old": old, "new": new}
	}
	if before.CalendarFeedEnabled != after.CalendarFeedEnabled {
		add("calendar_feed_enabled", before.CalendarFeedEnabled, after.CalendarFeedEnabled)
	}
	if before.PluginCLIExecEnabled != after.PluginCLIExecEnabled {
		add("plugin_cli_exec_enabled", before.PluginCLIExecEnabled, after.PluginCLIExecEnabled)
	}
	if before.AllowExternalImages != after.AllowExternalImages {
		add("allow_external_images", before.AllowExternalImages, after.AllowExternalImages)
	}
	if before.APIKeyCreationPolicy != after.APIKeyCreationPolicy {
		add("api_key_creation_policy", before.APIKeyCreationPolicy, after.APIKeyCreationPolicy)
	}
	if !slices.Equal(before.APIKeyAllowedGroupIDs, after.APIKeyAllowedGroupIDs) {
		add("api_key_allowed_group_ids", before.APIKeyAllowedGroupIDs, after.APIKeyAllowedGroupIDs)
	}
	if before.AllowUserManagedAgents != after.AllowUserManagedAgents {
		add("allow_user_managed_agents", before.AllowUserManagedAgents, after.AllowUserManagedAgents)
	}
	if before.MaxAgentsPerUser != after.MaxAgentsPerUser {
		add("max_agents_per_user", before.MaxAgentsPerUser, after.MaxAgentsPerUser)
	}
	if before.WorkspaceManagedAgents != after.WorkspaceManagedAgents {
		add("workspace_managed_agents", before.WorkspaceManagedAgents, after.WorkspaceManagedAgents)
	}
	return changes
}

func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
