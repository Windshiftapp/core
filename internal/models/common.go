package models

import "time"

// APIWarning represents a non-fatal warning in an API response
type APIWarning struct {
	Code    string `json:"code"`              // Machine-readable code, e.g., "cache_invalidation_failed"
	Message string `json:"message"`           // Human-readable message
	Context string `json:"context,omitempty"` // Additional context, e.g., "user_id:123"
}

// PaginationMeta represents pagination metadata
type PaginationMeta struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

// SystemSetting represents a system configuration setting
type SystemSetting struct {
	ID          int       `json:"id"`
	Key         string    `json:"key"`
	Value       string    `json:"value"`
	ValueType   string    `json:"value_type"` // string, boolean, integer, json
	Description string    `json:"description"`
	Category    string    `json:"category"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// SetupStatus represents the system setup status
type SetupStatus struct {
	SetupCompleted        bool `json:"setup_completed"`
	AdminUserCreated      bool `json:"admin_user_created"`
	TimeTrackingEnabled   bool `json:"time_tracking_enabled"`
	TestManagementEnabled bool `json:"test_management_enabled"`
}

// SetupRequest represents the initial setup configuration
type SetupRequest struct {
	AdminUser      SetupUser      `json:"admin_user"`
	ModuleSettings ModuleSettings `json:"module_settings"`
}

// SetupUser represents a user for initial setup (includes password)
type SetupUser struct {
	Email        string `json:"email"`
	Username     string `json:"username"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	PasswordHash string `json:"password_hash"` // For setup only, will be hashed server-side
}

// ModuleSettings represents module visibility settings
type ModuleSettings struct {
	TimeTrackingEnabled   bool `json:"time_tracking_enabled"`
	TestManagementEnabled bool `json:"test_management_enabled"`
}

// APIToken represents a bearer token for API authentication
type APIToken struct {
	ID          int        `json:"id"`
	UserID      int        `json:"user_id"`
	Name        string     `json:"name"`         // Human-readable name for the token
	Token       string     `json:"-"`            // The actual token (hashed in DB, not sent to client)
	TokenPrefix string     `json:"token_prefix"` // First few characters for identification
	Permissions string     `json:"permissions"`  // JSON array of permissions
	IsTemporary bool       `json:"is_temporary"` // True for SSH session tokens
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	// Joined fields
	UserEmail string `json:"user_email,omitempty"`
	UserName  string `json:"user_name,omitempty"`
}

// APITokenCreate represents the request for creating an API token
type APITokenCreate struct {
	Name        string     `json:"name"`
	UserID      *int       `json:"user_id,omitempty"` // Optional: admins can create tokens for other users
	Permissions []string   `json:"permissions"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
}

// APITokenResponse represents the response when creating an API token
type APITokenResponse struct {
	Token    string   `json:"token"`     // Only returned on creation
	APIToken APIToken `json:"api_token"` // Token metadata
}

// CacheStats represents permission cache performance metrics
type CacheStats struct {
	Hits        int64   `json:"hits"`
	Misses      int64   `json:"misses"`
	Errors      int64   `json:"errors"`
	HitRatio    float64 `json:"hit_ratio"`
	AvgLoadTime int64   `json:"avg_load_time_ms"`
	TotalUsers  int64   `json:"total_cached_users"`
}
