// Package sso provides Single Sign-On authentication functionality.
package sso

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"windshift/internal/database"
)

const (
	ProviderTypeOIDC = "oidc"
	ProviderTypeSAML = "saml"
)

var (
	ErrProviderNotFound  = errors.New("SSO provider not found")
	ErrProviderDisabled  = errors.New("SSO provider is disabled")
	ErrProviderExists    = errors.New("SSO provider with this slug already exists")
	ErrNoDefaultProvider = errors.New("no default SSO provider configured")
)

// SSOProvider represents an SSO identity provider configuration
type SSOProvider struct {
	ID                    int       `json:"id"`
	Slug                  string    `json:"slug"`
	Name                  string    `json:"name"`
	ProviderType          string    `json:"provider_type"`
	Enabled               bool      `json:"enabled"`
	IsDefault             bool      `json:"is_default"`
	IssuerURL             string    `json:"issuer_url,omitempty"`
	ClientID              string    `json:"client_id,omitempty"`
	ClientSecretEncrypted string    `json:"-"`                       // Never send to client
	ClientSecret          string    `json:"client_secret,omitempty"` // Only used for input, never stored
	Scopes                string    `json:"scopes"`
	AutoProvisionUsers    bool      `json:"auto_provision_users"`
	AllowPasswordLogin    bool      `json:"allow_password_login"`
	RequireVerifiedEmail  bool      `json:"require_verified_email"` // Require email_verified=true from IdP (default: true)
	AttributeMapping      string    `json:"attribute_mapping"`
	// SAML-specific fields
	SAMLIdPMetadataURL string `json:"saml_idp_metadata_url,omitempty"` // IdP metadata URL for auto-configuration
	SAMLIdPSSOURL      string `json:"saml_idp_sso_url,omitempty"`      // IdP Single Sign-On URL
	SAMLIdPCertificate string `json:"saml_idp_certificate,omitempty"`  // IdP X.509 certificate (PEM)
	SAMLSPEntityID     string `json:"saml_sp_entity_id,omitempty"`     // SP Entity ID (defaults to base URL)
	SAMLSignRequests   bool   `json:"saml_sign_requests"`              // Whether to sign AuthnRequests
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// providerColumns is the standard SELECT column list for providers (with secret)
const providerColumnsWithSecret = `id, slug, name, provider_type, enabled, is_default,
	issuer_url, client_id, client_secret_encrypted, scopes,
	auto_provision_users, allow_password_login, require_verified_email,
	attribute_mapping,
	saml_idp_metadata_url, saml_idp_sso_url, saml_idp_certificate, saml_sp_entity_id, saml_sign_requests,
	created_at, updated_at`

// providerColumnsWithoutSecret is the SELECT column list for listing (no secret)
const providerColumnsWithoutSecret = `id, slug, name, provider_type, enabled, is_default,
	issuer_url, client_id, scopes,
	auto_provision_users, allow_password_login, require_verified_email,
	attribute_mapping,
	saml_idp_metadata_url, saml_idp_sso_url, saml_idp_certificate, saml_sp_entity_id, saml_sign_requests,
	created_at, updated_at`

// scanProvider scans a row into an SSOProvider (with secret)
func scanProvider(row interface{ Scan(dest ...interface{}) error }) (*SSOProvider, error) {
	var provider SSOProvider
	var issuerURL, clientID, clientSecretEncrypted, scopes, attributeMapping sql.NullString
	var samlIdPMetadataURL, samlIdPSSOURL, samlIdPCertificate, samlSPEntityID sql.NullString

	err := row.Scan(
		&provider.ID, &provider.Slug, &provider.Name, &provider.ProviderType,
		&provider.Enabled, &provider.IsDefault,
		&issuerURL, &clientID, &clientSecretEncrypted, &scopes,
		&provider.AutoProvisionUsers, &provider.AllowPasswordLogin, &provider.RequireVerifiedEmail,
		&attributeMapping,
		&samlIdPMetadataURL, &samlIdPSSOURL, &samlIdPCertificate, &samlSPEntityID, &provider.SAMLSignRequests,
		&provider.CreatedAt, &provider.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	provider.IssuerURL = issuerURL.String
	provider.ClientID = clientID.String
	provider.ClientSecretEncrypted = clientSecretEncrypted.String
	provider.Scopes = scopes.String
	provider.AttributeMapping = attributeMapping.String
	provider.SAMLIdPMetadataURL = samlIdPMetadataURL.String
	provider.SAMLIdPSSOURL = samlIdPSSOURL.String
	provider.SAMLIdPCertificate = samlIdPCertificate.String
	provider.SAMLSPEntityID = samlSPEntityID.String

	return &provider, nil
}

// scanProviderNoSecret scans a row into an SSOProvider (without secret column)
func scanProviderNoSecret(row interface{ Scan(dest ...interface{}) error }) (*SSOProvider, error) {
	var provider SSOProvider
	var issuerURL, clientID, scopes, attributeMapping sql.NullString
	var samlIdPMetadataURL, samlIdPSSOURL, samlIdPCertificate, samlSPEntityID sql.NullString

	err := row.Scan(
		&provider.ID, &provider.Slug, &provider.Name, &provider.ProviderType,
		&provider.Enabled, &provider.IsDefault,
		&issuerURL, &clientID, &scopes,
		&provider.AutoProvisionUsers, &provider.AllowPasswordLogin, &provider.RequireVerifiedEmail,
		&attributeMapping,
		&samlIdPMetadataURL, &samlIdPSSOURL, &samlIdPCertificate, &samlSPEntityID, &provider.SAMLSignRequests,
		&provider.CreatedAt, &provider.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	provider.IssuerURL = issuerURL.String
	provider.ClientID = clientID.String
	provider.Scopes = scopes.String
	provider.AttributeMapping = attributeMapping.String
	provider.SAMLIdPMetadataURL = samlIdPMetadataURL.String
	provider.SAMLIdPSSOURL = samlIdPSSOURL.String
	provider.SAMLIdPCertificate = samlIdPCertificate.String
	provider.SAMLSPEntityID = samlSPEntityID.String

	return &provider, nil
}

// AttributeMap represents the claim/attribute mapping configuration
type AttributeMap struct {
	Email      string `json:"email"`
	Name       string `json:"name"`
	GivenName  string `json:"given_name"`
	FamilyName string `json:"family_name"`
	Username   string `json:"username"`
}

// GetAttributeMap parses the attribute mapping JSON
func (p *SSOProvider) GetAttributeMap() (*AttributeMap, error) {
	if p.AttributeMapping == "" {
		return &AttributeMap{
			Email:      "email",
			Name:       "name",
			GivenName:  "given_name",
			FamilyName: "family_name",
			Username:   "preferred_username",
		}, nil
	}

	var mapping AttributeMap
	if err := json.Unmarshal([]byte(p.AttributeMapping), &mapping); err != nil {
		return nil, err
	}
	return &mapping, nil
}

// ProviderStore handles database operations for SSO providers
type ProviderStore struct {
	db database.Database
}

// NewProviderStore creates a new provider store
func NewProviderStore(db database.Database) *ProviderStore {
	return &ProviderStore{db: db}
}

// GetByID retrieves a provider by ID
func (s *ProviderStore) GetByID(id int) (*SSOProvider, error) {
	query := `SELECT ` + providerColumnsWithSecret + ` FROM sso_providers WHERE id = ?`
	provider, err := scanProvider(s.db.QueryRow(query, id))
	if err == sql.ErrNoRows {
		return nil, ErrProviderNotFound
	}
	return provider, err
}

// GetBySlug retrieves a provider by slug
func (s *ProviderStore) GetBySlug(slug string) (*SSOProvider, error) {
	query := `SELECT ` + providerColumnsWithSecret + ` FROM sso_providers WHERE slug = ?`
	provider, err := scanProvider(s.db.QueryRow(query, slug))
	if err == sql.ErrNoRows {
		return nil, ErrProviderNotFound
	}
	return provider, err
}

// GetDefault retrieves the default enabled provider
func (s *ProviderStore) GetDefault() (*SSOProvider, error) {
	query := `SELECT ` + providerColumnsWithSecret + ` FROM sso_providers WHERE enabled = true AND is_default = true LIMIT 1`
	provider, err := scanProvider(s.db.QueryRow(query))
	if err == sql.ErrNoRows {
		return nil, ErrNoDefaultProvider
	}
	return provider, err
}

// List retrieves all providers
func (s *ProviderStore) List() ([]*SSOProvider, error) {
	query := `SELECT ` + providerColumnsWithoutSecret + ` FROM sso_providers ORDER BY created_at ASC`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var providers []*SSOProvider
	for rows.Next() {
		provider, err := scanProviderNoSecret(rows)
		if err != nil {
			return nil, err
		}
		providers = append(providers, provider)
	}

	return providers, nil
}

// Create creates a new provider
func (s *ProviderStore) Create(provider *SSOProvider) error {
	// Check if slug already exists
	existing, err := s.GetBySlug(provider.Slug)
	if err == nil && existing != nil {
		return ErrProviderExists
	}

	// If this is the first provider or marked as default, ensure it's the only default
	if provider.IsDefault {
		_, err = s.db.Exec("UPDATE sso_providers SET is_default = false WHERE is_default = true")
		if err != nil {
			return err
		}
	}

	query := `
		INSERT INTO sso_providers (
			slug, name, provider_type, enabled, is_default,
			issuer_url, client_id, client_secret_encrypted, scopes,
			auto_provision_users, allow_password_login, require_verified_email,
			attribute_mapping,
			saml_idp_metadata_url, saml_idp_sso_url, saml_idp_certificate, saml_sp_entity_id, saml_sign_requests,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		RETURNING id
	`

	var id int64
	err = s.db.QueryRow(query,
		provider.Slug,
		provider.Name,
		provider.ProviderType,
		provider.Enabled,
		provider.IsDefault,
		nullString(provider.IssuerURL),
		nullString(provider.ClientID),
		nullString(provider.ClientSecretEncrypted),
		nullString(provider.Scopes),
		provider.AutoProvisionUsers,
		provider.AllowPasswordLogin,
		provider.RequireVerifiedEmail,
		nullString(provider.AttributeMapping),
		nullString(provider.SAMLIdPMetadataURL),
		nullString(provider.SAMLIdPSSOURL),
		nullString(provider.SAMLIdPCertificate),
		nullString(provider.SAMLSPEntityID),
		provider.SAMLSignRequests,
	).Scan(&id)
	if err != nil {
		return err
	}
	provider.ID = int(id)

	return nil
}

// Update updates an existing provider
func (s *ProviderStore) Update(provider *SSOProvider) error {
	// If setting as default, clear other defaults
	if provider.IsDefault {
		_, err := s.db.Exec("UPDATE sso_providers SET is_default = false WHERE is_default = true AND id != ?", provider.ID)
		if err != nil {
			return err
		}
	}

	query := `
		UPDATE sso_providers SET
			slug = ?, name = ?, provider_type = ?, enabled = ?, is_default = ?,
			issuer_url = ?, client_id = ?, scopes = ?,
			auto_provision_users = ?, allow_password_login = ?, require_verified_email = ?,
			attribute_mapping = ?,
			saml_idp_metadata_url = ?, saml_idp_sso_url = ?, saml_idp_certificate = ?,
			saml_sp_entity_id = ?, saml_sign_requests = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`

	_, err := s.db.Exec(query,
		provider.Slug,
		provider.Name,
		provider.ProviderType,
		provider.Enabled,
		provider.IsDefault,
		nullString(provider.IssuerURL),
		nullString(provider.ClientID),
		nullString(provider.Scopes),
		provider.AutoProvisionUsers,
		provider.AllowPasswordLogin,
		provider.RequireVerifiedEmail,
		nullString(provider.AttributeMapping),
		nullString(provider.SAMLIdPMetadataURL),
		nullString(provider.SAMLIdPSSOURL),
		nullString(provider.SAMLIdPCertificate),
		nullString(provider.SAMLSPEntityID),
		provider.SAMLSignRequests,
		provider.ID,
	)
	return err
}

// UpdateSecret updates only the client secret
func (s *ProviderStore) UpdateSecret(id int, encryptedSecret string) error {
	query := `UPDATE sso_providers SET client_secret_encrypted = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	_, err := s.db.Exec(query, encryptedSecret, id)
	return err
}

// Delete deletes a provider by ID
func (s *ProviderStore) Delete(id int) error {
	query := `DELETE FROM sso_providers WHERE id = ?`
	result, err := s.db.Exec(query, id)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrProviderNotFound
	}

	return nil
}

// Count returns the number of providers
func (s *ProviderStore) Count() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM sso_providers").Scan(&count)
	return count, err
}

// nullString helper to convert empty string to sql.NullString
func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
