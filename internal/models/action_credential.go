package models

import "time"

// ActionCredentialEncryptionInfo is the HKDF info label used to derive the
// action-credentials encryption key from SSO_SECRET. Domain-separated from
// the SSO secret-encryption label so a ciphertext from one realm cannot be
// decrypted with the other realm's key.
const ActionCredentialEncryptionInfo = "windshift-action-credentials-encryption-v1" //nolint:gosec // G101: HKDF domain-separation label, not a credential

// ActionCredentialType enumerates the supported credential shapes.
type ActionCredentialType string

const (
	// CredentialBearerToken stores the raw token; injected as
	// `Authorization: Bearer <token>` (or the scheme configured on the auth ref).
	CredentialBearerToken ActionCredentialType = "bearer_token"
	// CredentialAPIKey stores an opaque API key; injected as the literal
	// header value configured on the auth/secret ref.
	CredentialAPIKey ActionCredentialType = "api_key"
	// CredentialBasicAuth stores "user:password"; injected as
	// `Authorization: Basic <base64(user:password)>`.
	CredentialBasicAuth ActionCredentialType = "basic_auth"
	// CredentialCustomHeader stores an arbitrary header value with no
	// scheme/encoding transform.
	CredentialCustomHeader ActionCredentialType = "custom_header"
)

// ActionCredential represents a stored secret used by HTTP action capabilities.
// EncryptedSecret is intentionally hidden from JSON; the API returns the
// sanitized form instead.
type ActionCredential struct {
	ID              int                  `json:"id"`
	Name            string               `json:"name"`
	CredentialType  ActionCredentialType `json:"credential_type"`
	WorkspaceID     *int                 `json:"workspace_id,omitempty"`
	CreatedBy       *int                 `json:"created_by,omitempty"`
	EncryptedSecret string               `json:"-"` // never returned
	SecretPrefix    string               `json:"secret_prefix,omitempty"`
	SecretMetadata  string               `json:"secret_metadata,omitempty"` // JSON; must not contain plaintext secrets
	IsEnabled       bool                 `json:"is_enabled"`
	CreatedAt       time.Time            `json:"created_at"`
	UpdatedAt       time.Time            `json:"updated_at"`
}

// Sanitize returns a redacted view safe for any client response.
func (c *ActionCredential) Sanitize() ActionCredentialSanitized {
	return ActionCredentialSanitized{
		ID:             c.ID,
		Name:           c.Name,
		CredentialType: c.CredentialType,
		WorkspaceID:    c.WorkspaceID,
		HasSecret:      c.EncryptedSecret != "",
		SecretPrefix:   c.SecretPrefix,
		SecretMetadata: c.SecretMetadata,
		IsEnabled:      c.IsEnabled,
		CreatedAt:      c.CreatedAt,
		UpdatedAt:      c.UpdatedAt,
	}
}

// ActionCredentialSanitized is the only shape sent to clients. has_secret +
// secret_prefix let the UI render a masked indicator without ever seeing the
// ciphertext or plaintext.
type ActionCredentialSanitized struct {
	ID             int                  `json:"id"`
	Name           string               `json:"name"`
	CredentialType ActionCredentialType `json:"credential_type"`
	WorkspaceID    *int                 `json:"workspace_id,omitempty"`
	HasSecret      bool                 `json:"has_secret"`
	SecretPrefix   string               `json:"secret_prefix,omitempty"`
	SecretMetadata string               `json:"secret_metadata,omitempty"`
	IsEnabled      bool                 `json:"is_enabled"`
	CreatedAt      time.Time            `json:"created_at"`
	UpdatedAt      time.Time            `json:"updated_at"`
}

// CreateActionCredentialRequest is the body for credential creation. The
// plaintext Secret travels only on this request and is discarded after
// encryption — it must not be persisted anywhere else.
type CreateActionCredentialRequest struct {
	Name           string               `json:"name"`
	CredentialType ActionCredentialType `json:"credential_type"`
	Secret         string               `json:"secret"`
	WorkspaceID    *int                 `json:"workspace_id,omitempty"`
	SecretMetadata string               `json:"secret_metadata,omitempty"`
	IsEnabled      *bool                `json:"is_enabled,omitempty"`
}

// UpdateActionCredentialRequest patches credential metadata. The plaintext
// secret cannot be set through this endpoint — use the rotate endpoint.
type UpdateActionCredentialRequest struct {
	Name           *string `json:"name,omitempty"`
	SecretMetadata *string `json:"secret_metadata,omitempty"`
	IsEnabled      *bool   `json:"is_enabled,omitempty"`
}

// RotateActionCredentialRequest carries only the new secret value.
type RotateActionCredentialRequest struct {
	Secret string `json:"secret"`
}

// SecretPrefixFor returns a non-sensitive fingerprint suitable for display.
// Short secrets are masked entirely so we don't accidentally leak material.
func SecretPrefixFor(plaintext string) string {
	const prefixLen = 4
	if len(plaintext) <= prefixLen*2 {
		return ""
	}
	return plaintext[:prefixLen] + "…"
}
