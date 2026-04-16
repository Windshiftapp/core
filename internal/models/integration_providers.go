package models

import "time"

// IntegrationProviderType represents the type of integration provider
type IntegrationProviderType string

const (
	IntegrationProviderNotion IntegrationProviderType = "notion"
	// Future: IntegrationProviderConfluence, IntegrationProviderGoogleDocs
)

// IntegrationProvider represents a configured integration provider (Notion, Confluence, etc.)
type IntegrationProvider struct {
	ID                         string                  `json:"id" db:"id"`
	Slug                       string                  `json:"slug" db:"slug"`
	Name                       string                  `json:"name" db:"name"`
	ProviderType               IntegrationProviderType `json:"provider_type" db:"provider_type"`
	Enabled                    bool                    `json:"enabled" db:"enabled"`
	OAuthClientID              string                  `json:"oauth_client_id" db:"oauth_client_id"`
	OAuthClientSecretEncrypted string                  `json:"-" db:"oauth_client_secret_encrypted"`
	ProviderConfig             string                  `json:"provider_config,omitempty" db:"provider_config"`
	CreatedAt                  time.Time               `json:"created_at" db:"created_at"`
	UpdatedAt                  time.Time               `json:"updated_at" db:"updated_at"`
}

// IntegrationProviderRequest is used for creating/updating integration providers
type IntegrationProviderRequest struct {
	Slug              string `json:"slug"`
	Name              string `json:"name"`
	ProviderType      string `json:"provider_type"`
	Enabled           *bool  `json:"enabled"`
	OAuthClientID     string `json:"oauth_client_id"`
	OAuthClientSecret string `json:"oauth_client_secret"`
	ProviderConfig    string `json:"provider_config,omitempty"`
}

// IntegrationOAuthState holds temporary OAuth state for the authorization flow
type IntegrationOAuthState struct {
	ID         string    `db:"id"`
	ProviderID string    `db:"provider_id"`
	State      string    `db:"state"`
	UserID     string    `db:"user_id"`
	ExpiresAt  time.Time `db:"expires_at"`
	CreatedAt  time.Time `db:"created_at"`
}

// UserIntegrationToken holds a user's OAuth token for an integration provider
type UserIntegrationToken struct {
	ID                        string    `json:"id" db:"id"`
	UserID                    string    `json:"user_id" db:"user_id"`
	IntegrationProviderID     string    `json:"integration_provider_id" db:"integration_provider_id"`
	OAuthAccessTokenEncrypted string    `json:"-" db:"oauth_access_token_encrypted"`
	ProviderMetadata          string    `json:"provider_metadata,omitempty" db:"provider_metadata"`
	ConnectedAt               time.Time `json:"connected_at" db:"connected_at"`
	CreatedAt                 time.Time `json:"created_at" db:"created_at"`
	UpdatedAt                 time.Time `json:"updated_at" db:"updated_at"`
	// Joined fields for API responses
	ProviderName string `json:"provider_name,omitempty" db:"provider_name"`
	ProviderType string `json:"provider_type,omitempty" db:"provider_type"`
	ProviderSlug string `json:"provider_slug,omitempty" db:"provider_slug"`
}

// ItemIntegrationLink represents a link between a work item and an external page/document
type ItemIntegrationLink struct {
	ID                    string    `json:"id" db:"id"`
	ItemID                string    `json:"item_id" db:"item_id"`
	IntegrationProviderID string    `json:"integration_provider_id" db:"integration_provider_id"`
	ExternalID            string    `json:"external_id" db:"external_id"`
	ExternalURL           string    `json:"external_url" db:"external_url"`
	Title                 string    `json:"title" db:"title"`
	Icon                  string    `json:"icon,omitempty" db:"icon"`
	LinkType              string    `json:"link_type" db:"link_type"`
	LinkMetadata          string    `json:"link_metadata,omitempty" db:"link_metadata"`
	LinkedBy              string    `json:"linked_by" db:"linked_by"`
	CreatedAt             time.Time `json:"created_at" db:"created_at"`
	UpdatedAt             time.Time `json:"updated_at" db:"updated_at"`
	// Joined fields
	ProviderName string `json:"provider_name,omitempty" db:"provider_name"`
	ProviderType string `json:"provider_type,omitempty" db:"provider_type"`
}
