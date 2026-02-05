package models

import "time"

// User represents a system user
type User struct {
	ID                    int    `json:"id"`
	Email                 string `json:"email"`
	Username              string `json:"username"`
	FirstName             string `json:"first_name"`
	LastName              string `json:"last_name"`
	IsActive              bool   `json:"is_active"`
	AvatarURL             string `json:"avatar_url,omitempty"`
	PasswordHash          string `json:"-"` // Never send password hash to client
	RequiresPasswordReset bool   `json:"requires_password_reset"`
	Timezone              string `json:"timezone"` // User's timezone (IANA timezone, e.g., "America/New_York")
	Language              string `json:"language"` // User's preferred language (ISO 639-1 code, e.g., "en", "de")
	// Email verification fields (for SSO users when IdP doesn't provide email_verified)
	EmailVerified            bool       `json:"email_verified"`
	EmailVerificationToken   string     `json:"-"` // Never send token to client
	EmailVerificationExpires *time.Time `json:"-"` // Never send expiry to client
	CreatedAt                time.Time  `json:"created_at"`
	UpdatedAt                time.Time  `json:"updated_at"`
	// Virtual fields for display
	FullName      string `json:"full_name,omitempty"`
	IsSystemAdmin bool   `json:"is_system_admin"` // Populated from permissions, cached at login
	// SCIM fields
	SCIMExternalID string `json:"scim_external_id,omitempty"` // External ID from identity provider
	SCIMManaged    bool   `json:"scim_managed"`               // If true, user is managed via SCIM and cannot be edited locally
}

// UserCredential represents a user's authentication credential (FIDO key, TOTP secret, etc.)
type UserCredential struct {
	ID             string     `json:"id"` // Changed to string to support both int (legacy) and string (WebAuthn) IDs
	UserID         int        `json:"user_id"`
	CredentialType string     `json:"credential_type"` // 'fido', 'totp'
	CredentialName string     `json:"credential_name"` // User-friendly name
	CredentialData string     `json:"-"`               // JSON data, never send to client
	IsActive       bool       `json:"is_active"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	LastUsedAt     *time.Time `json:"last_used_at,omitempty"`
}

// UserSession represents an active user session
type UserSession struct {
	ID           int       `json:"id"`
	UserID       int       `json:"user_id"`
	SessionToken string    `json:"session_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	IPAddress    string    `json:"ip_address,omitempty"`
	UserAgent    string    `json:"user_agent,omitempty"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
}

// UserAppToken represents a user's application token
type UserAppToken struct {
	ID          int        `json:"id"`
	UserID      int        `json:"user_id"`
	TokenName   string     `json:"token_name"`
	TokenHash   string     `json:"-"` // Never send hash to client
	TokenPrefix string     `json:"token_prefix"`
	Scopes      string     `json:"scopes"`
	ExpiresAt   *time.Time `json:"expires_at"`
	IsActive    bool       `json:"is_active"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// UserPermissionCache represents a user's complete cached permission set
type UserPermissionCache struct {
	UserID               int                       `json:"user_id"`
	IsSystemAdmin        bool                      `json:"is_system_admin"`
	GlobalPermissions    map[string]bool           `json:"global_permissions"`    // permission_key -> has_permission
	WorkspacePermissions map[int]map[string]bool   `json:"workspace_permissions"` // workspace_id -> permission_key -> has_permission
	WorkspaceEveryone    map[int]map[string]bool   `json:"workspace_everyone"`    // workspace_id -> permission_key -> has_permission (applies to all users)
	GroupMemberships     []int                     `json:"group_memberships"`     // group_ids
	RoleAssignments      map[int][]int             `json:"role_assignments"`      // workspace_id -> role_ids
	DirectPermissions    map[int][]string          `json:"direct_permissions"`    // workspace_id -> permission_keys (direct assignments)
	PermissionSources    map[int]map[string]string `json:"permission_sources"`    // workspace_id -> permission_key -> source (role/direct/group)
	ItemWorkspaceMap     map[int]int               `json:"item_workspace_map"`    // item_id -> workspace_id (lazy-loaded on demand)
	CachedAt             time.Time                 `json:"cached_at"`
	ExpiresAt            time.Time                 `json:"expires_at"`
}

// UserPreferences represents user-specific preferences stored as JSON
type UserPreferences struct {
	ID          int       `json:"id"`
	UserID      int       `json:"user_id"`
	Preferences string    `json:"preferences"` // JSON string for database storage
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// UserPreferencesData represents the parsed preferences JSON structure
type UserPreferencesData struct {
	ColorMode string `json:"color_mode,omitempty"` // "light", "dark", or "system"
	ThemeID   *int   `json:"theme_id,omitempty"`
}

// UserPreferencesRequest represents the API request for updating preferences
type UserPreferencesRequest struct {
	ColorMode string `json:"color_mode,omitempty"`
	ThemeID   *int   `json:"theme_id,omitempty"`
}

// UserPreferencesResponse represents the API response with resolved data
type UserPreferencesResponse struct {
	ColorMode string `json:"color_mode"`
	ThemeID   *int   `json:"theme_id,omitempty"`
	Theme     *Theme `json:"theme,omitempty"` // Resolved theme if theme_id is set
}

// TeamGroup represents a user group for access control and organization
type TeamGroup struct {
	ID          int    `json:"id"`
	Name        string `json:"name"` // Group name (e.g., "Developers", "Managers")
	Description string `json:"description"`
	// LDAP sync fields
	LDAPDistinguishedName string     `json:"ldap_distinguished_name,omitempty"` // Full LDAP DN for sync
	LDAPCommonName        string     `json:"ldap_common_name,omitempty"`        // CN from LDAP
	LDAPSyncEnabled       bool       `json:"ldap_sync_enabled"`                 // Whether this group syncs from LDAP
	LDAPLastSyncAt        *time.Time `json:"ldap_last_sync_at,omitempty"`       // Last successful LDAP sync
	// Group metadata
	IsSystemGroup bool      `json:"is_system_group"`      // Whether this is a system-defined group
	IsActive      bool      `json:"is_active"`            // Whether the group is active
	CreatedBy     *int      `json:"created_by,omitempty"` // User who created the group
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	// Joined fields for API responses
	CreatedByName string            `json:"created_by_name,omitempty"`
	MemberCount   int               `json:"member_count,omitempty"` // Number of members in this group
	Members       []TeamGroupMember `json:"members,omitempty"`      // Group members (for detailed views)
	// SCIM fields
	SCIMExternalID string `json:"scim_external_id,omitempty"` // External ID from identity provider
	SCIMManaged    bool   `json:"scim_managed"`               // If true, group is managed via SCIM and cannot be edited locally
}

// TeamGroupMember represents a user's membership in a group
type TeamGroupMember struct {
	ID      int `json:"id"`
	GroupID int `json:"group_id"`
	UserID  int `json:"user_id"`
	// LDAP sync fields
	LDAPSyncEnabled bool       `json:"ldap_sync_enabled"`           // Whether this membership is managed by LDAP
	LDAPLastSyncAt  *time.Time `json:"ldap_last_sync_at,omitempty"` // Last LDAP sync for this membership
	// Membership metadata
	AddedBy     *int      `json:"added_by,omitempty"` // User who added this member (NULL for LDAP)
	AddedAt     time.Time `json:"added_at"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	SCIMManaged bool      `json:"scim_managed"` // Whether this membership is managed via SCIM
	// Joined fields for API responses
	UserEmail    string `json:"user_email,omitempty"`
	UserName     string `json:"user_name,omitempty"` // Full name (first + last)
	UserUsername string `json:"user_username,omitempty"`
	GroupName    string `json:"group_name,omitempty"`
	AddedByName  string `json:"added_by_name,omitempty"`
}

// TeamGroupCreateRequest represents the payload for creating a new group
type TeamGroupCreateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// TeamGroupUpdateRequest represents the payload for updating a group
type TeamGroupUpdateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsActive    bool   `json:"is_active"`
}

// TeamGroupMemberRequest represents the payload for adding/removing group members
type TeamGroupMemberRequest struct {
	UserIDs []int `json:"user_ids"`
}

// TeamGroupMembershipResponse represents a user's group memberships
type TeamGroupMembershipResponse struct {
	UserID int         `json:"user_id"`
	Groups []TeamGroup `json:"groups"`
}

// Theme represents the application's visual theme settings
type Theme struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsDefault   bool   `json:"is_default"`
	IsActive    bool   `json:"is_active"`
	// Navigation bar theme properties for light mode
	NavBackgroundColorLight string `json:"nav_background_color_light"` // CSS color value (hex, rgb, etc.)
	NavTextColorLight       string `json:"nav_text_color_light"`       // CSS color value (hex, rgb, etc.)
	// Navigation bar theme properties for dark mode
	NavBackgroundColorDark string    `json:"nav_background_color_dark"` // CSS color value (hex, rgb, etc.)
	NavTextColorDark       string    `json:"nav_text_color_dark"`       // CSS color value (hex, rgb, etc.)
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

// ThemeCreateRequest represents the payload for creating a new theme
type ThemeCreateRequest struct {
	Name                    string `json:"name"`
	Description             string `json:"description"`
	NavBackgroundColorLight string `json:"nav_background_color_light"`
	NavTextColorLight       string `json:"nav_text_color_light"`
	NavBackgroundColorDark  string `json:"nav_background_color_dark"`
	NavTextColorDark        string `json:"nav_text_color_dark"`
}

// ThemeUpdateRequest represents the payload for updating a theme
type ThemeUpdateRequest struct {
	Name                    string `json:"name"`
	Description             string `json:"description"`
	NavBackgroundColorLight string `json:"nav_background_color_light"`
	NavTextColorLight       string `json:"nav_text_color_light"`
	NavBackgroundColorDark  string `json:"nav_background_color_dark"`
	NavTextColorDark        string `json:"nav_text_color_dark"`
	IsActive                bool   `json:"is_active"`
}
