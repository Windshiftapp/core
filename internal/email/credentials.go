package email

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// Encryptor interface for encrypting/decrypting secrets
type Encryptor interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
}

// CredentialManager handles OAuth token management for email channels
type CredentialManager struct {
	db         database.Database
	encryption Encryptor
}

// NewCredentialManager creates a new credential manager
func NewCredentialManager(db database.Database, encryption Encryptor) *CredentialManager {
	return &CredentialManager{
		db:         db,
		encryption: encryption,
	}
}

// GetProviderForChannel creates the appropriate provider for a channel
func (m *CredentialManager) GetProviderForChannel(ctx context.Context, channelID int) (Provider, *models.ChannelConfig, error) {
	// Get channel and its config
	var configJSON string

	err := m.db.QueryRow(`
		SELECT config FROM channels WHERE id = ? AND type = 'email' AND direction = 'inbound'
	`, channelID).Scan(&configJSON)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get channel: %w", err)
	}

	var config models.ChannelConfig
	if configJSON != "" {
		if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
			return nil, nil, fmt.Errorf("failed to parse channel config: %w", err)
		}
	}

	// Decrypt OAuth tokens if present
	if config.EmailOAuthAccessToken != "" && m.encryption != nil {
		decrypted, err := m.encryption.Decrypt(config.EmailOAuthAccessToken)
		if err == nil {
			config.EmailOAuthAccessToken = decrypted
		}
	}
	if config.EmailOAuthRefreshToken != "" && m.encryption != nil {
		decrypted, err := m.encryption.Decrypt(config.EmailOAuthRefreshToken)
		if err == nil {
			config.EmailOAuthRefreshToken = decrypted
		}
	}

	// Check for inline OAuth credentials first (per-channel OAuth app)
	if config.EmailOAuthProviderType != "" && config.EmailOAuthClientID != "" {
		// Decrypt client secret
		var clientSecret string
		if config.EmailOAuthClientSecret != "" && m.encryption != nil {
			decrypted, err := m.encryption.Decrypt(config.EmailOAuthClientSecret)
			if err == nil {
				clientSecret = decrypted
			}
		}

		switch config.EmailOAuthProviderType {
		case models.EmailProviderTypeMicrosoft:
			tenant := config.EmailOAuthTenantID
			if tenant == "" {
				tenant = "common"
			}
			provider := NewMicrosoftProvider(config.EmailOAuthClientID, clientSecret, tenant, nil)
			return provider, &config, nil

		case models.EmailProviderTypeGoogle:
			provider := NewGoogleProvider(config.EmailOAuthClientID, clientSecret, nil)
			return provider, &config, nil
		}
	}

	// Fall back to email_provider_id if set (legacy/central provider management)
	if config.EmailProviderID != nil {
		provider, err := m.GetProvider(ctx, *config.EmailProviderID)
		if err != nil {
			return nil, nil, err
		}
		return provider, &config, nil
	}

	// Fall back to basic IMAP (generic provider with channel's IMAP credentials)
	if config.IMAPHost != "" {
		provider := NewGenericProvider(config.IMAPHost, config.IMAPPort, config.IMAPEncryption)
		return provider, &config, nil
	}

	return nil, nil, fmt.Errorf("no email provider configured for channel")
}

// GetProvider retrieves and constructs a provider by ID
func (m *CredentialManager) GetProvider(ctx context.Context, providerID int) (Provider, error) {
	var ep models.EmailProvider
	var clientSecretEnc *string

	// Use sql.NullString for nullable columns
	var oauthClientID, oauthScopes, oauthTenantID, imapHost, imapEncryption sql.NullString
	var imapPort sql.NullInt64

	err := m.db.QueryRow(`
		SELECT id, name, slug, type, is_enabled,
		       oauth_client_id, oauth_client_secret_encrypted, oauth_scopes, oauth_tenant_id,
		       imap_host, imap_port, imap_encryption
		FROM email_providers WHERE id = ?
	`, providerID).Scan(
		&ep.ID, &ep.Name, &ep.Slug, &ep.Type, &ep.IsEnabled,
		&oauthClientID, &clientSecretEnc, &oauthScopes, &oauthTenantID,
		&imapHost, &imapPort, &imapEncryption,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get email provider: %w", err)
	}

	// Copy nullable values to struct
	ep.OAuthClientID = oauthClientID.String
	ep.OAuthScopes = oauthScopes.String
	ep.OAuthTenantID = oauthTenantID.String
	ep.IMAPHost = imapHost.String
	ep.IMAPPort = int(imapPort.Int64)
	ep.IMAPEncryption = imapEncryption.String

	if !ep.IsEnabled {
		return nil, fmt.Errorf("email provider is disabled")
	}

	// Decrypt client secret
	var clientSecret string
	if clientSecretEnc != nil && *clientSecretEnc != "" && m.encryption != nil {
		decrypted, err := m.encryption.Decrypt(*clientSecretEnc)
		if err == nil {
			clientSecret = decrypted
		}
	}

	// Create appropriate provider
	switch ep.Type {
	case models.EmailProviderTypeMicrosoft:
		var scopes []string
		if ep.OAuthScopes != "" {
			scopes = splitScopes(ep.OAuthScopes)
		}
		return NewMicrosoftProvider(ep.OAuthClientID, clientSecret, ep.OAuthTenantID, scopes), nil

	case models.EmailProviderTypeGoogle:
		var scopes []string
		if ep.OAuthScopes != "" {
			scopes = splitScopes(ep.OAuthScopes)
		}
		return NewGoogleProvider(ep.OAuthClientID, clientSecret, scopes), nil

	case models.EmailProviderTypeGeneric:
		return NewGenericProvider(ep.IMAPHost, ep.IMAPPort, ep.IMAPEncryption), nil

	default:
		return nil, fmt.Errorf("unknown provider type: %s", ep.Type)
	}
}

// RefreshOAuthTokenIfNeeded checks if the OAuth token needs refresh and refreshes it
func (m *CredentialManager) RefreshOAuthTokenIfNeeded(
	ctx context.Context,
	channelID int,
	config *models.ChannelConfig,
	provider OAuthProvider,
) (string, error) {
	// If no expiration set, token doesn't expire
	if config.EmailOAuthExpiresAt == nil {
		return config.EmailOAuthAccessToken, nil
	}

	// Check if token needs refresh (expiring within 5 minutes)
	if time.Until(*config.EmailOAuthExpiresAt) > 5*time.Minute {
		return config.EmailOAuthAccessToken, nil
	}

	slog.Info("refreshing email OAuth token", "channel_id", channelID)

	// Token is expired or expiring soon - try to refresh
	if config.EmailOAuthRefreshToken == "" {
		return "", fmt.Errorf("token expired and no refresh token available")
	}

	// Refresh the token
	newTokens, err := provider.RefreshToken(ctx, config.EmailOAuthRefreshToken)
	if err != nil {
		return "", fmt.Errorf("failed to refresh token: %w", err)
	}

	// Encrypt new tokens
	newAccessTokenEnc, err := m.encryption.Encrypt(newTokens.AccessToken)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt new access token: %w", err)
	}

	var newRefreshTokenEnc string
	if newTokens.RefreshToken != "" {
		newRefreshTokenEnc, _ = m.encryption.Encrypt(newTokens.RefreshToken)
	}

	// Update channel config with new tokens
	err = m.updateChannelTokens(ctx, channelID, newAccessTokenEnc, newRefreshTokenEnc, newTokens.ExpiresAt)
	if err != nil {
		// Log but continue - we can use the new token for this request
		slog.Error("failed to store refreshed tokens", "error", err)
	}

	return newTokens.AccessToken, nil
}

// updateChannelTokens updates the OAuth tokens in the channel config
func (m *CredentialManager) updateChannelTokens(
	ctx context.Context,
	channelID int,
	accessToken, refreshToken string,
	expiresAt *time.Time,
) error {
	// Get current config
	var configJSON string
	err := m.db.QueryRow(`SELECT config FROM channels WHERE id = ?`, channelID).Scan(&configJSON)
	if err != nil {
		return fmt.Errorf("failed to get channel config: %w", err)
	}

	var config models.ChannelConfig
	if configJSON != "" {
		if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
			return fmt.Errorf("failed to parse channel config: %w", err)
		}
	}

	// Update token fields
	config.EmailOAuthAccessToken = accessToken
	if refreshToken != "" {
		config.EmailOAuthRefreshToken = refreshToken
	}
	config.EmailOAuthExpiresAt = expiresAt

	// Save updated config
	updatedConfigJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal updated config: %w", err)
	}

	_, err = m.db.Exec(`
		UPDATE channels SET config = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?
	`, string(updatedConfigJSON), channelID)
	if err != nil {
		return fmt.Errorf("failed to update channel config: %w", err)
	}

	return nil
}

// SaveOAuthTokens saves OAuth tokens for a channel after successful OAuth flow
func (m *CredentialManager) SaveOAuthTokens(
	ctx context.Context,
	channelID int,
	tokens *OAuthTokens,
	email string,
) error {
	// Get current config
	var configJSON string
	err := m.db.QueryRow(`SELECT config FROM channels WHERE id = ?`, channelID).Scan(&configJSON)
	if err != nil {
		return fmt.Errorf("failed to get channel config: %w", err)
	}

	var config models.ChannelConfig
	if configJSON != "" {
		if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
			return fmt.Errorf("failed to parse channel config: %w", err)
		}
	}

	// Encrypt tokens
	accessTokenEnc, err := m.encryption.Encrypt(tokens.AccessToken)
	if err != nil {
		return fmt.Errorf("failed to encrypt access token: %w", err)
	}

	var refreshTokenEnc string
	if tokens.RefreshToken != "" {
		refreshTokenEnc, _ = m.encryption.Encrypt(tokens.RefreshToken)
	}

	// Update config
	config.EmailAuthMethod = "oauth"
	config.EmailOAuthAccessToken = accessTokenEnc
	config.EmailOAuthRefreshToken = refreshTokenEnc
	config.EmailOAuthExpiresAt = tokens.ExpiresAt
	config.EmailOAuthEmail = email

	// Save updated config
	updatedConfigJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal updated config: %w", err)
	}

	_, err = m.db.Exec(`
		UPDATE channels SET config = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?
	`, string(updatedConfigJSON), channelID)
	if err != nil {
		return fmt.Errorf("failed to update channel config: %w", err)
	}

	slog.Info("saved OAuth tokens for email channel", "channel_id", channelID, "email", email)

	return nil
}

// splitScopes splits a space-separated scope string into a slice
func splitScopes(scopes string) []string {
	if scopes == "" {
		return nil
	}
	var result []string
	for _, s := range []byte(scopes) {
		if s == ' ' {
			continue
		}
	}
	// Simple space split
	var current string
	for _, c := range scopes {
		if c == ' ' {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}
