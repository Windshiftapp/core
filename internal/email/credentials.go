package email

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// decryptOrLegacy unwraps an encrypted secret, distinguishing three cases:
//   - empty string: returns "" (caller shortcuts).
//   - value looks like a base64-encoded AES-GCM ciphertext (decodable and
//     long enough to contain nonce + tag + body): attempt decrypt and
//     propagate failure. A failure here means the serverSecret rotated or
//     the DB is corrupt — don't silently use the ciphertext as plaintext,
//     which would send garbage to upstream IDPs and produce opaque 401s.
//   - anything else: legacy plaintext from before encryption was introduced;
//     return as-is so existing channels keep working.
//
// last review: ser, 280426
func decryptOrLegacy(enc Encryptor, value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if enc == nil {
		return value, nil
	}
	// AES-GCM ciphertext minimum: 12-byte nonce + 16-byte auth tag = 28 bytes
	// raw, ~40 base64 chars. Short-and-not-base64 inputs are legacy plaintext.
	const minCipherBytes = 28
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(raw) < minCipherBytes {
		return value, nil //nolint:nilerr // legacy-plaintext fallback is intentional; see function comment
	}
	plain, err := enc.Decrypt(value)
	if err != nil {
		return "", fmt.Errorf("decrypt secret: %w", err)
	}
	return plain, nil
}

// Encryptor interface for encrypting/decrypting secrets
type Encryptor interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
}

// CredentialManager handles OAuth token management for email channels
type CredentialManager struct {
	db         database.Database
	encryption Encryptor

	// refreshLocks serializes OAuth refreshes per channel within this process.
	// Two scheduler ticks or a scheduler tick racing with an admin-triggered
	// sync for the same channel would otherwise both hit an expired token,
	// both call the provider, and both write back — and since providers like
	// Microsoft rotate refresh tokens, the losing writer's refresh_token is
	// dead on arrival. A per-channel mutex removes that race single-instance.
	// (Multi-instance deployments still need a DB-level lock; that's a
	// separate change.)
	refreshLocks sync.Map // map[int]*sync.Mutex
}

// NewCredentialManager creates a new credential manager
func NewCredentialManager(db database.Database, encryption Encryptor) *CredentialManager {
	return &CredentialManager{
		db:         db,
		encryption: encryption,
	}
}

// lockForChannel returns a dedicated mutex for a channel, creating it on first use.
func (m *CredentialManager) lockForChannel(channelID int) *sync.Mutex {
	if mu, ok := m.refreshLocks.Load(channelID); ok {
		if asMu, ok := mu.(*sync.Mutex); ok {
			return asMu
		}
	}
	actual, _ := m.refreshLocks.LoadOrStore(channelID, &sync.Mutex{})
	if asMu, ok := actual.(*sync.Mutex); ok {
		return asMu
	}
	// Unreachable: only *sync.Mutex values are ever stored.
	return &sync.Mutex{}
}

// GetProviderForChannel creates the appropriate provider for a channel
// last review: ser, 210426
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
	if tok, err := decryptOrLegacy(m.encryption, config.EmailOAuthAccessToken); err != nil {
		return nil, nil, fmt.Errorf("channel %d access token: %w", channelID, err)
	} else {
		config.EmailOAuthAccessToken = tok
	}
	if tok, err := decryptOrLegacy(m.encryption, config.EmailOAuthRefreshToken); err != nil {
		return nil, nil, fmt.Errorf("channel %d refresh token: %w", channelID, err)
	} else {
		config.EmailOAuthRefreshToken = tok
	}

	// Decrypt IMAP basic-auth password for at-rest encryption. Legacy plaintext
	// rows pass through unchanged via decryptOrLegacy's base64/length heuristic
	// so a deployment can encrypt rolling without re-issuing every channel.
	if pw, err := decryptOrLegacy(m.encryption, config.IMAPPassword); err != nil {
		return nil, nil, fmt.Errorf("channel %d IMAP password: %w", channelID, err)
	} else {
		config.IMAPPassword = pw
	}

	// Check for inline OAuth credentials first (per-channel OAuth app)
	if config.EmailOAuthProviderType != "" && config.EmailOAuthClientID != "" {
		clientSecret, err := decryptOrLegacy(m.encryption, config.EmailOAuthClientSecret)
		if err != nil {
			return nil, nil, fmt.Errorf("channel %d client secret: %w", channelID, err)
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
	// last review: ser, 210426, OPTIMIZE: not needed really
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
// last review: ser, 210426, OPTIMIZE: Not needed, marked at callsite
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
	if clientSecretEnc != nil {
		plain, err := decryptOrLegacy(m.encryption, *clientSecretEnc)
		if err != nil {
			return nil, fmt.Errorf("provider %d client secret: %w", providerID, err)
		}
		clientSecret = plain
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

// RefreshOAuthTokenIfNeeded checks if the OAuth token needs refresh and refreshes it.
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

	// Fast path: token has plenty of life left, no lock needed.
	if time.Until(*config.EmailOAuthExpiresAt) > 5*time.Minute {
		return config.EmailOAuthAccessToken, nil
	}

	// Serialize refreshes per channel. The caller's `config` may be stale by the
	// time we get the lock if another goroutine already refreshed — re-read the
	// current state from DB and bail if a fresh token is now present.
	mu := m.lockForChannel(channelID)
	mu.Lock()
	defer mu.Unlock()

	currentAccess, currentRefresh, currentExpiresAt, err := m.readOAuthTokens(ctx, channelID)
	if err != nil {
		return "", fmt.Errorf("failed to re-read channel config after acquiring refresh lock: %w", err)
	}
	if currentExpiresAt != nil && time.Until(*currentExpiresAt) > 5*time.Minute {
		// Someone else refreshed while we were waiting.
		return currentAccess, nil
	}

	slog.Info("refreshing email OAuth token", "channel_id", channelID)

	if currentRefresh == "" {
		return "", fmt.Errorf("token expired and no refresh token available")
	}

	newTokens, err := provider.RefreshToken(ctx, currentRefresh)
	if err != nil {
		return "", fmt.Errorf("failed to refresh token: %w", err)
	}

	newAccessTokenEnc, err := m.encryption.Encrypt(newTokens.AccessToken)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt new access token: %w", err)
	}

	var newRefreshTokenEnc string
	if newTokens.RefreshToken != "" {
		// A silently-dropped encryption error here used to wipe the stored
		// refresh token (empty ciphertext), leaving the channel unable to
		// refresh and requiring manual re-auth. Surface the error instead.
		newRefreshTokenEnc, err = m.encryption.Encrypt(newTokens.RefreshToken)
		if err != nil {
			return "", fmt.Errorf("failed to encrypt new refresh token: %w", err)
		}
	}

	// If the DB write fails we must NOT return success — the provider (esp.
	// Microsoft) may have rotated the refresh token, in which case the stored
	// one is now dead and the next tick would refresh with it and fail hard.
	// Surface the error so the caller records it and retries next tick with
	// the hopefully-still-valid old refresh token.
	if err := m.updateChannelTokens(ctx, channelID, newAccessTokenEnc, newRefreshTokenEnc, newTokens.ExpiresAt); err != nil {
		return "", fmt.Errorf("failed to store refreshed tokens: %w", err)
	}

	return newTokens.AccessToken, nil
}

// readOAuthTokens reads and decrypts the current OAuth tokens from the DB.
// Used by RefreshOAuthTokenIfNeeded after acquiring the per-channel lock to
// guard against acting on a stale in-memory config.
func (m *CredentialManager) readOAuthTokens(ctx context.Context, channelID int) (access, refresh string, expiresAt *time.Time, err error) {
	var configJSON string
	if err := m.db.QueryRowContext(ctx, `SELECT config FROM channels WHERE id = ?`, channelID).Scan(&configJSON); err != nil {
		return "", "", nil, err
	}
	var cfg models.ChannelConfig
	if configJSON != "" {
		if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
			return "", "", nil, err
		}
	}
	access, err = decryptOrLegacy(m.encryption, cfg.EmailOAuthAccessToken)
	if err != nil {
		return "", "", nil, err
	}
	refresh, err = decryptOrLegacy(m.encryption, cfg.EmailOAuthRefreshToken)
	if err != nil {
		return "", "", nil, err
	}
	return access, refresh, cfg.EmailOAuthExpiresAt, nil
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
	err := m.db.QueryRowContext(ctx, `SELECT config FROM channels WHERE id = ?`, channelID).Scan(&configJSON)
	if err != nil {
		return fmt.Errorf("failed to get channel config: %w", err)
	}

	var config models.ChannelConfig
	if configJSON != "" {
		if err = json.Unmarshal([]byte(configJSON), &config); err != nil {
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

	_, err = m.db.ExecContext(ctx, `
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
		if err = json.Unmarshal([]byte(configJSON), &config); err != nil {
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
		refreshTokenEnc, err = m.encryption.Encrypt(tokens.RefreshToken)
		if err != nil {
			return fmt.Errorf("failed to encrypt refresh token: %w", err)
		}
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
