package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"windshift/internal/database"
)

// AuthPolicy represents the authentication policy configuration
type AuthPolicy string

const (
	// AuthPolicyPassword is password-only authentication (default)
	AuthPolicyPassword AuthPolicy = "password"
	// AuthPolicyPasswordPasskey2FA requires password + passkey 2FA (when no SSO configured)
	AuthPolicyPasswordPasskey2FA AuthPolicy = "password_passkey_2fa"
	// AuthPolicyPasskeyOnly is passkey-only, password becomes enrollment-only credential
	AuthPolicyPasskeyOnly AuthPolicy = "passkey_only"
	// AuthPolicySSOPrimary requires SSO, password disabled (requires SSO configured)
	AuthPolicySSOPrimary AuthPolicy = "sso_primary"
)

// AuthPolicyConfig represents the full authentication policy configuration
type AuthPolicyConfig struct {
	Policy           AuthPolicy `json:"policy"`
	PreviewMode      bool       `json:"preview_mode"`
	EnabledAt        *time.Time `json:"enabled_at,omitempty"`
	SSOConfigured    bool       `json:"sso_configured"`
	PasskeyRequired  bool       `json:"passkey_required"`   // Derived from policy
	FallbackEnabled  bool       `json:"fallback_enabled"`   // Whether admin fallback is enabled (via --enable-fallback)
	HidePasswordForm bool       `json:"hide_password_form"` // True if fallback disabled + restrictive policy
}

// AuthPolicyStats contains statistics for auth policy planning
type AuthPolicyStats struct {
	TotalUsers         int `json:"total_users"`
	UsersWithPasskey   int `json:"users_with_passkey"`
	UsersWithoutPasskey int `json:"users_without_passkey"`
	SSOUsers           int `json:"sso_users"`
	SystemAdmins       int `json:"system_admins"`
	AdminsWithPasskey  int `json:"admins_with_passkey"`
}

// AffectedUser represents a user who would be affected by the policy change
type AffectedUser struct {
	ID         int    `json:"id"`
	Email      string `json:"email"`
	Username   string `json:"username"`
	FullName   string `json:"full_name"`
	HasPasskey bool   `json:"has_passkey"`
	HasSSO     bool   `json:"has_sso"`
	IsAdmin    bool   `json:"is_admin"`
}

// AuthPolicyHandler handles authentication policy endpoints
type AuthPolicyHandler struct {
	db              database.Database
	fallbackEnabled bool // Whether admin fallback is enabled via --enable-fallback flag
}

// NewAuthPolicyHandler creates a new authentication policy handler (fallback disabled by default)
func NewAuthPolicyHandler(db database.Database) *AuthPolicyHandler {
	return &AuthPolicyHandler{db: db, fallbackEnabled: false}
}

// NewAuthPolicyHandlerWithFallback creates a new authentication policy handler with explicit fallback setting
func NewAuthPolicyHandlerWithFallback(db database.Database, fallbackEnabled bool) *AuthPolicyHandler {
	return &AuthPolicyHandler{db: db, fallbackEnabled: fallbackEnabled}
}

// GetAuthPolicy returns the current authentication policy configuration
func (h *AuthPolicyHandler) GetAuthPolicy(w http.ResponseWriter, r *http.Request) {
	config := AuthPolicyConfig{
		Policy:      AuthPolicyPassword, // Default
		PreviewMode: false,
	}

	// Get auth_policy setting
	var value string
	err := h.db.QueryRow("SELECT value FROM system_settings WHERE key = 'auth_policy'").Scan(&value)
	if err == nil && value != "" {
		config.Policy = AuthPolicy(value)
	}

	// Get preview mode setting
	err = h.db.QueryRow("SELECT value FROM system_settings WHERE key = 'auth_policy_preview'").Scan(&value)
	if err == nil {
		config.PreviewMode = strings.ToLower(value) == "true"
	}

	// Get enabled_at timestamp
	err = h.db.QueryRow("SELECT value FROM system_settings WHERE key = 'auth_policy_enabled_at'").Scan(&value)
	if err == nil && value != "" {
		t, parseErr := time.Parse(time.RFC3339, value)
		if parseErr == nil {
			config.EnabledAt = &t
		}
	}

	// Check if SSO is configured
	config.SSOConfigured = h.isSSOConfigured()

	// Derive passkey requirement
	config.PasskeyRequired = config.Policy == AuthPolicyPasskeyOnly || config.Policy == AuthPolicyPasswordPasskey2FA

	// Set fallback status
	config.FallbackEnabled = h.fallbackEnabled

	// Hide password form if:
	// 1. Fallback is disabled (default)
	// 2. AND policy is restrictive (passkey_only or sso_primary)
	// 3. AND not in preview mode
	isRestrictivePolicy := config.Policy == AuthPolicyPasskeyOnly || config.Policy == AuthPolicySSOPrimary
	config.HidePasswordForm = !h.fallbackEnabled && isRestrictivePolicy && !config.PreviewMode

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

// UpdateAuthPolicy updates the authentication policy
func (h *AuthPolicyHandler) UpdateAuthPolicy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Policy      AuthPolicy `json:"policy"`
		PreviewMode bool       `json:"preview_mode"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid JSON")
		return
	}

	// Validate policy value
	validPolicies := map[AuthPolicy]bool{
		AuthPolicyPassword:           true,
		AuthPolicyPasswordPasskey2FA: true,
		AuthPolicyPasskeyOnly:        true,
		AuthPolicySSOPrimary:         true,
	}
	if !validPolicies[req.Policy] {
		respondBadRequest(w, r, "Invalid policy value")
		return
	}

	// Validate policy requirements
	ssoConfigured := h.isSSOConfigured()

	// password_passkey_2fa should not be used when SSO is configured (SSO provides 2FA)
	if req.Policy == AuthPolicyPasswordPasskey2FA && ssoConfigured {
		respondBadRequest(w, r, "Password+Passkey 2FA policy is not recommended when SSO is configured")
		return
	}

	// sso_primary requires SSO to be configured
	if req.Policy == AuthPolicySSOPrimary && !ssoConfigured {
		respondBadRequest(w, r, "SSO Primary policy requires SSO to be configured")
		return
	}

	// When enabling passkey_only or sso_primary with fallback disabled,
	// verify that ALL admin users have a passkey enrolled to prevent lockout
	if !h.fallbackEnabled && !req.PreviewMode && (req.Policy == AuthPolicyPasskeyOnly || req.Policy == AuthPolicySSOPrimary) {
		var adminsWithoutPasskey int
		// Check both direct user permissions AND group-based permissions
		err := h.db.QueryRow(`
			SELECT COUNT(DISTINCT admin_user_id) FROM (
				-- Direct user permissions
				SELECT ugp.user_id as admin_user_id
				FROM user_global_permissions ugp
				JOIN permissions gp ON ugp.permission_id = gp.id
				JOIN users u ON ugp.user_id = u.id
				WHERE gp.permission_key = 'system.admin'
				AND u.is_active = 1
				AND NOT EXISTS(
					SELECT 1 FROM user_credentials uc
					WHERE uc.user_id = ugp.user_id
					AND uc.credential_type = 'fido'
					AND uc.is_active = 1
				)
				UNION
				-- Group-based permissions
				SELECT gm.user_id as admin_user_id
				FROM group_members gm
				JOIN groups g ON gm.group_id = g.id
				JOIN group_global_permissions ggp ON gm.group_id = ggp.group_id
				JOIN permissions gp ON ggp.permission_id = gp.id
				JOIN users u ON gm.user_id = u.id
				WHERE gp.permission_key = 'system.admin'
				AND g.is_active = 1
				AND u.is_active = 1
				AND NOT EXISTS(
					SELECT 1 FROM user_credentials uc
					WHERE uc.user_id = gm.user_id
					AND uc.credential_type = 'fido'
					AND uc.is_active = 1
				)
			)
		`).Scan(&adminsWithoutPasskey)

		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		if adminsWithoutPasskey > 0 {
			respondBadRequest(w, r, "Cannot enable this policy: some administrators do not have passkeys enrolled. Enable --enable-fallback flag or ensure all admins have passkeys.")
			return
		}
	}

	// Update or insert auth_policy
	h.upsertSetting("auth_policy", string(req.Policy), "string", "Authentication policy", "auth")

	// Update preview mode
	previewValue := "false"
	if req.PreviewMode {
		previewValue = "true"
	}
	h.upsertSetting("auth_policy_preview", previewValue, "boolean", "Preview mode for auth policy", "auth")

	// Set enabled_at if transitioning from preview to active or changing policy
	if !req.PreviewMode && req.Policy != AuthPolicyPassword {
		// Check if there's already an enabled_at time
		var existingEnabled string
		err := h.db.QueryRow("SELECT value FROM system_settings WHERE key = 'auth_policy_enabled_at'").Scan(&existingEnabled)
		if err == sql.ErrNoRows || existingEnabled == "" {
			h.upsertSetting("auth_policy_enabled_at", time.Now().UTC().Format(time.RFC3339), "string", "When policy was activated", "auth")
		}
	}

	// Return updated config
	h.GetAuthPolicy(w, r)
}

// GetAuthPolicyStats returns statistics about users for policy planning
func (h *AuthPolicyHandler) GetAuthPolicyStats(w http.ResponseWriter, r *http.Request) {
	stats := AuthPolicyStats{}

	// Total active users
	h.db.QueryRow("SELECT COUNT(*) FROM users WHERE is_active = 1").Scan(&stats.TotalUsers)

	// Users with at least one active passkey (FIDO credential)
	h.db.QueryRow(`
		SELECT COUNT(DISTINCT user_id) FROM user_credentials
		WHERE credential_type = 'fido' AND is_active = 1
	`).Scan(&stats.UsersWithPasskey)

	stats.UsersWithoutPasskey = stats.TotalUsers - stats.UsersWithPasskey

	// Users with SSO external account linked
	h.db.QueryRow(`
		SELECT COUNT(DISTINCT user_id) FROM user_external_accounts
		WHERE user_id IS NOT NULL
	`).Scan(&stats.SSOUsers)

	// System admins (users with system.admin global permission - direct OR via group)
	h.db.QueryRow(`
		SELECT COUNT(DISTINCT admin_user_id) FROM (
			SELECT ugp.user_id as admin_user_id
			FROM user_global_permissions ugp
			JOIN permissions gp ON ugp.permission_id = gp.id
			WHERE gp.permission_key = 'system.admin'
			UNION
			SELECT gm.user_id as admin_user_id
			FROM group_members gm
			JOIN groups g ON gm.group_id = g.id
			JOIN group_global_permissions ggp ON gm.group_id = ggp.group_id
			JOIN permissions gp ON ggp.permission_id = gp.id
			WHERE gp.permission_key = 'system.admin'
			AND g.is_active = 1
		)
	`).Scan(&stats.SystemAdmins)

	// Admins with passkeys
	h.db.QueryRow(`
		SELECT COUNT(DISTINCT admin_user_id) FROM (
			SELECT ugp.user_id as admin_user_id
			FROM user_global_permissions ugp
			JOIN permissions gp ON ugp.permission_id = gp.id
			JOIN user_credentials uc ON ugp.user_id = uc.user_id
			WHERE gp.permission_key = 'system.admin'
			AND uc.credential_type = 'fido' AND uc.is_active = 1
			UNION
			SELECT gm.user_id as admin_user_id
			FROM group_members gm
			JOIN groups g ON gm.group_id = g.id
			JOIN group_global_permissions ggp ON gm.group_id = ggp.group_id
			JOIN permissions gp ON ggp.permission_id = gp.id
			JOIN user_credentials uc ON gm.user_id = uc.user_id
			WHERE gp.permission_key = 'system.admin'
			AND g.is_active = 1
			AND uc.credential_type = 'fido' AND uc.is_active = 1
		)
	`).Scan(&stats.AdminsWithPasskey)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// GetAffectedUsers returns users who would be affected by the current policy
func (h *AuthPolicyHandler) GetAffectedUsers(w http.ResponseWriter, r *http.Request) {
	// Get current policy
	var policy AuthPolicy = AuthPolicyPassword
	var policyStr string
	err := h.db.QueryRow("SELECT value FROM system_settings WHERE key = 'auth_policy'").Scan(&policyStr)
	if err == nil && policyStr != "" {
		policy = AuthPolicy(policyStr)
	}

	// If policy is just password, no users are affected
	if policy == AuthPolicyPassword {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]AffectedUser{})
		return
	}

	// Build query based on policy
	var query string
	switch policy {
	case AuthPolicyPasskeyOnly, AuthPolicyPasswordPasskey2FA:
		// Users without passkeys (excluding system admins who have fallback)
		query = `
			SELECT u.id, u.email, u.username, u.first_name, u.last_name,
				EXISTS(SELECT 1 FROM user_credentials uc WHERE uc.user_id = u.id AND uc.credential_type = 'fido' AND uc.is_active = 1) as has_passkey,
				EXISTS(SELECT 1 FROM user_external_accounts sea WHERE sea.user_id = u.id) as has_sso,
				(
					EXISTS(SELECT 1 FROM user_global_permissions ugp JOIN permissions gp ON ugp.permission_id = gp.id WHERE ugp.user_id = u.id AND gp.permission_key = 'system.admin')
					OR EXISTS(SELECT 1 FROM group_members gm JOIN groups g ON gm.group_id = g.id JOIN group_global_permissions ggp ON gm.group_id = ggp.group_id JOIN permissions gp ON ggp.permission_id = gp.id WHERE gm.user_id = u.id AND gp.permission_key = 'system.admin' AND g.is_active = 1)
				) as is_admin
			FROM users u
			WHERE u.is_active = 1
			AND NOT EXISTS(SELECT 1 FROM user_credentials uc WHERE uc.user_id = u.id AND uc.credential_type = 'fido' AND uc.is_active = 1)
			ORDER BY u.email
		`
	case AuthPolicySSOPrimary:
		// Users without SSO linked (excluding system admins who have fallback)
		query = `
			SELECT u.id, u.email, u.username, u.first_name, u.last_name,
				EXISTS(SELECT 1 FROM user_credentials uc WHERE uc.user_id = u.id AND uc.credential_type = 'fido' AND uc.is_active = 1) as has_passkey,
				EXISTS(SELECT 1 FROM user_external_accounts sea WHERE sea.user_id = u.id) as has_sso,
				(
					EXISTS(SELECT 1 FROM user_global_permissions ugp JOIN permissions gp ON ugp.permission_id = gp.id WHERE ugp.user_id = u.id AND gp.permission_key = 'system.admin')
					OR EXISTS(SELECT 1 FROM group_members gm JOIN groups g ON gm.group_id = g.id JOIN group_global_permissions ggp ON gm.group_id = ggp.group_id JOIN permissions gp ON ggp.permission_id = gp.id WHERE gm.user_id = u.id AND gp.permission_key = 'system.admin' AND g.is_active = 1)
				) as is_admin
			FROM users u
			WHERE u.is_active = 1
			AND NOT EXISTS(SELECT 1 FROM user_external_accounts sea WHERE sea.user_id = u.id)
			ORDER BY u.email
		`
	default:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]AffectedUser{})
		return
	}

	rows, err := h.db.Query(query)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer rows.Close()

	users := []AffectedUser{}
	for rows.Next() {
		var u AffectedUser
		var firstName, lastName string
		if err := rows.Scan(&u.ID, &u.Email, &u.Username, &firstName, &lastName, &u.HasPasskey, &u.HasSSO, &u.IsAdmin); err != nil {
			continue
		}
		u.FullName = strings.TrimSpace(firstName + " " + lastName)
		users = append(users, u)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}

// isSSOConfigured checks if any SSO provider is configured and enabled
func (h *AuthPolicyHandler) isSSOConfigured() bool {
	var count int
	err := h.db.QueryRow("SELECT COUNT(*) FROM sso_providers WHERE enabled = 1").Scan(&count)
	return err == nil && count > 0
}

// upsertSetting updates or inserts a system setting
func (h *AuthPolicyHandler) upsertSetting(key, value, valueType, description, category string) error {
	// Try UPDATE first
	result, err := h.db.Exec(`
		UPDATE system_settings SET value = ?, updated_at = CURRENT_TIMESTAMP
		WHERE key = ?
	`, value, key)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		// INSERT if row doesn't exist
		_, err = h.db.Exec(`
			INSERT INTO system_settings (key, value, value_type, description, category, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, key, value, valueType, description, category)
		return err
	}
	return nil
}

// GetCurrentPolicy returns the current auth policy (for use by other handlers)
func (h *AuthPolicyHandler) GetCurrentPolicy() AuthPolicy {
	var value string
	err := h.db.QueryRow("SELECT value FROM system_settings WHERE key = 'auth_policy'").Scan(&value)
	if err != nil || value == "" {
		return AuthPolicyPassword
	}
	return AuthPolicy(value)
}

// IsPreviewMode returns whether preview mode is enabled
func (h *AuthPolicyHandler) IsPreviewMode() bool {
	var value string
	err := h.db.QueryRow("SELECT value FROM system_settings WHERE key = 'auth_policy_preview'").Scan(&value)
	if err != nil {
		return false
	}
	return strings.ToLower(value) == "true"
}

// PublicPolicyStatus represents the public auth policy status for the login page
type PublicPolicyStatus struct {
	HidePasswordForm bool `json:"hide_password_form"` // True if password form should be hidden
	SSOEnabled       bool `json:"sso_enabled"`        // True if SSO is configured
	PasskeyRequired  bool `json:"passkey_required"`   // True if passkey authentication is required
}

// GetPublicPolicyStatus returns the auth policy status for the login page (no auth required)
// This endpoint only exposes minimal information needed by the login UI
func (h *AuthPolicyHandler) GetPublicPolicyStatus(w http.ResponseWriter, r *http.Request) {
	status := PublicPolicyStatus{}

	// Get current policy
	var policyStr string
	err := h.db.QueryRow("SELECT value FROM system_settings WHERE key = 'auth_policy'").Scan(&policyStr)
	policy := AuthPolicyPassword
	if err == nil && policyStr != "" {
		policy = AuthPolicy(policyStr)
	}

	// Check preview mode
	var previewValue string
	err = h.db.QueryRow("SELECT value FROM system_settings WHERE key = 'auth_policy_preview'").Scan(&previewValue)
	previewMode := err == nil && strings.ToLower(previewValue) == "true"

	// Determine if SSO is enabled
	status.SSOEnabled = h.isSSOConfigured()

	// Derive passkey requirement
	status.PasskeyRequired = policy == AuthPolicyPasskeyOnly || policy == AuthPolicyPasswordPasskey2FA

	// Hide password form if:
	// 1. Fallback is disabled (default)
	// 2. AND policy is restrictive (passkey_only or sso_primary)
	// 3. AND not in preview mode
	isRestrictivePolicy := policy == AuthPolicyPasskeyOnly || policy == AuthPolicySSOPrimary
	status.HidePasswordForm = !h.fallbackEnabled && isRestrictivePolicy && !previewMode

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// IsFallbackEnabled returns whether admin fallback is enabled
func (h *AuthPolicyHandler) IsFallbackEnabled() bool {
	return h.fallbackEnabled
}

// LogAuditEvent logs an auth policy related event
func (h *AuthPolicyHandler) LogAuditEvent(userID int, eventType string, ipAddress, userAgent string, details map[string]interface{}) error {
	policy := h.GetCurrentPolicy()

	var detailsJSON []byte
	if details != nil {
		detailsJSON, _ = json.Marshal(details)
	}

	_, err := h.db.ExecWrite(`
		INSERT INTO auth_policy_audit (user_id, event_type, policy_at_time, ip_address, user_agent, details, created_at)
		VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	`, userID, eventType, string(policy), ipAddress, userAgent, string(detailsJSON))
	return err
}
