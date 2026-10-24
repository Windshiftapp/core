package email

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

const encryptedSecretPrefix = "enc:v1:"

const (
	credentialLeaseDuration = 3 * time.Minute
	credentialLeasePoll     = 200 * time.Millisecond
)

// EncryptSecret writes an unambiguous envelope around new ciphertext. Older
// channel secrets were stored as bare base64, which is indistinguishable by
// shape from a perfectly valid long base64 password.
func EncryptSecret(enc Encryptor, value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if enc == nil {
		return "", fmt.Errorf("secret encryption is not configured")
	}
	ciphertext, err := enc.Encrypt(value)
	if err != nil {
		return "", err
	}
	return encryptedSecretPrefix + ciphertext, nil
}

// DecryptOrLegacy unwraps an encrypted secret, distinguishing three cases:
//   - empty string: returns "" (caller shortcuts).
//   - enc:v1-prefixed value: decrypt strictly and propagate any corruption or
//     key mismatch.
//   - unprefixed value that looks like legacy ciphertext: try decrypting it;
//     if authentication fails, treat it as legacy plaintext because long
//     base64 passwords are otherwise impossible to represent.
//   - anything else: legacy plaintext from before encryption was introduced;
//     return as-is so existing channels keep working.
//
// last review: ser, 280426
func DecryptOrLegacy(enc Encryptor, value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if enc == nil {
		return value, nil
	}
	if strings.HasPrefix(value, encryptedSecretPrefix) {
		plain, err := enc.Decrypt(strings.TrimPrefix(value, encryptedSecretPrefix))
		if err != nil {
			return "", fmt.Errorf("decrypt secret: %w", err)
		}
		return plain, nil
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
		return value, nil //nolint:nilerr // legacy plaintext may itself be long base64
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
	// dead on arrival. A per-channel mutex avoids needless local lease
	// contention; the database-backed lease below covers other instances.
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

func (m *CredentialManager) acquireCredentialLease(ctx context.Context, channelID int) (string, error) {
	ownerBytes := make([]byte, 16)
	if _, err := rand.Read(ownerBytes); err != nil {
		return "", fmt.Errorf("generate credential lease token: %w", err)
	}
	owner := hex.EncodeToString(ownerBytes)
	for {
		expiresAt := time.Now().Add(credentialLeaseDuration)
		result, err := m.db.ExecWriteContext(ctx, `
			INSERT INTO email_credential_leases(channel_id, owner_token, expires_at)
			VALUES (?, ?, ?)
			ON CONFLICT(channel_id) DO UPDATE SET
				owner_token = excluded.owner_token,
				expires_at = excluded.expires_at
			WHERE email_credential_leases.expires_at <= CURRENT_TIMESTAMP
		`, channelID, owner, expiresAt)
		if err != nil {
			return "", fmt.Errorf("claim email credential lease: %w", err)
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return "", fmt.Errorf("count claimed email credential leases: %w", err)
		}
		if rows > 0 {
			return owner, nil
		}

		timer := time.NewTimer(credentialLeasePoll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", ctx.Err()
		case <-timer.C:
		}
	}
}

func (m *CredentialManager) releaseCredentialLease(ctx context.Context, channelID int, owner string) {
	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if _, err := m.db.ExecWriteContext(releaseCtx,
		"DELETE FROM email_credential_leases WHERE channel_id = ? AND owner_token = ?",
		channelID, owner,
	); err != nil {
		slog.Error("failed to release email credential lease", "channel_id", channelID, "error", err)
	}
}

func (m *CredentialManager) withCredentialLease(ctx context.Context, channelID int, mutate func() error) error {
	mu := m.lockForChannel(channelID)
	mu.Lock()
	defer mu.Unlock()
	owner, err := m.acquireCredentialLease(ctx, channelID)
	if err != nil {
		return err
	}
	defer m.releaseCredentialLease(ctx, channelID, owner)
	return mutate()
}

// GetProviderForChannel creates the appropriate provider for a channel
// last review: ser, 210426
func (m *CredentialManager) GetProviderForChannel(ctx context.Context, channelID int) (Provider, *models.ChannelConfig, error) {
	// Get channel and its config
	var configJSON string

	err := m.db.QueryRow(`
		SELECT COALESCE(config, '{}') FROM channels WHERE id = ? AND type = 'email' AND direction = 'inbound'
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
	if tok, err := DecryptOrLegacy(m.encryption, config.EmailOAuthAccessToken); err != nil {
		return nil, nil, fmt.Errorf("channel %d access token: %w", channelID, err)
	} else {
		config.EmailOAuthAccessToken = tok
	}
	if tok, err := DecryptOrLegacy(m.encryption, config.EmailOAuthRefreshToken); err != nil {
		return nil, nil, fmt.Errorf("channel %d refresh token: %w", channelID, err)
	} else {
		config.EmailOAuthRefreshToken = tok
	}

	// Decrypt IMAP basic-auth password for at-rest encryption. Legacy plaintext
	// rows pass through unchanged via DecryptOrLegacy's base64/length heuristic
	// so a deployment can encrypt rolling without re-issuing every channel.
	if pw, err := DecryptOrLegacy(m.encryption, config.IMAPPassword); err != nil {
		return nil, nil, fmt.Errorf("channel %d IMAP password: %w", channelID, err)
	} else {
		config.IMAPPassword = pw
	}

	// Check for inline OAuth credentials first (per-channel OAuth app)
	if config.EmailOAuthProviderType != "" && config.EmailOAuthClientID != "" {
		clientSecret, err := DecryptOrLegacy(m.encryption, config.EmailOAuthClientSecret)
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
		plain, err := DecryptOrLegacy(m.encryption, *clientSecretEnc)
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

	// Serialize refreshes per channel in-process and across application servers.
	// The caller's config may be stale by the time the lease is acquired, so
	// re-read and validate the OAuth identity before calling the provider.
	mu := m.lockForChannel(channelID)
	mu.Lock()
	defer mu.Unlock()
	owner, err := m.acquireCredentialLease(ctx, channelID)
	if err != nil {
		return "", err
	}
	defer m.releaseCredentialLease(ctx, channelID, owner)

	current, err := m.readOAuthConfig(ctx, channelID)
	if err != nil {
		return "", fmt.Errorf("failed to re-read channel config after acquiring refresh lock: %w", err)
	}
	if current.EmailOAuthExpiresAt != nil && time.Until(*current.EmailOAuthExpiresAt) > 5*time.Minute {
		// Someone else refreshed while we were waiting.
		return current.EmailOAuthAccessToken, nil
	}
	if !sameOAuthIdentity(config, current) {
		return "", fmt.Errorf("email OAuth identity changed while waiting to refresh")
	}

	slog.Info("refreshing email OAuth token", "channel_id", channelID)

	if current.EmailOAuthRefreshToken == "" {
		return "", fmt.Errorf("token expired and no refresh token available")
	}

	newTokens, err := provider.RefreshToken(ctx, current.EmailOAuthRefreshToken)
	if err != nil {
		return "", fmt.Errorf("failed to refresh token: %w", err)
	}
	if newTokens == nil || strings.TrimSpace(newTokens.AccessToken) == "" {
		return "", fmt.Errorf("OAuth provider returned no refreshed access token")
	}

	newAccessTokenEnc, err := EncryptSecret(m.encryption, newTokens.AccessToken)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt new access token: %w", err)
	}

	var newRefreshTokenEnc string
	if newTokens.RefreshToken != "" {
		// A silently-dropped encryption error here used to wipe the stored
		// refresh token (empty ciphertext), leaving the channel unable to
		// refresh and requiring manual re-auth. Surface the error instead.
		newRefreshTokenEnc, err = EncryptSecret(m.encryption, newTokens.RefreshToken)
		if err != nil {
			return "", fmt.Errorf("failed to encrypt new refresh token: %w", err)
		}
	}

	// If the DB write fails we must NOT return success — the provider (esp.
	// Microsoft) may have rotated the refresh token, in which case the stored
	// one is now dead and the next tick would refresh with it and fail hard.
	// Surface the error so the caller records it and retries next tick with
	// the hopefully-still-valid old refresh token.
	if err := m.updateChannelTokens(ctx, channelID, newAccessTokenEnc, newRefreshTokenEnc, newTokens.ExpiresAt, current); err != nil {
		return "", fmt.Errorf("failed to store refreshed tokens: %w", err)
	}

	return newTokens.AccessToken, nil
}

// readOAuthConfig reads and decrypts the current OAuth tokens from the DB.
// Used by RefreshOAuthTokenIfNeeded after acquiring the per-channel lock to
// guard against acting on a stale in-memory config.
func (m *CredentialManager) readOAuthConfig(ctx context.Context, channelID int) (*models.ChannelConfig, error) {
	var configJSON string
	if err := m.db.QueryRowContext(ctx, `SELECT COALESCE(config, '{}') FROM channels WHERE id = ?`, channelID).Scan(&configJSON); err != nil {
		return nil, err
	}
	var cfg models.ChannelConfig
	if configJSON != "" {
		if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
			return nil, err
		}
	}
	access, err := DecryptOrLegacy(m.encryption, cfg.EmailOAuthAccessToken)
	if err != nil {
		return nil, err
	}
	refresh, err := DecryptOrLegacy(m.encryption, cfg.EmailOAuthRefreshToken)
	if err != nil {
		return nil, err
	}
	cfg.EmailOAuthAccessToken = access
	cfg.EmailOAuthRefreshToken = refresh
	return &cfg, nil
}

// updateChannelTokens updates the OAuth tokens in the channel config
func (m *CredentialManager) updateChannelTokens(
	ctx context.Context,
	channelID int,
	accessToken, refreshToken string,
	expiresAt *time.Time,
	expected *models.ChannelConfig,
) error {
	_, err := m.patchChannelConfig(ctx, channelID, func(config map[string]any) error {
		encoded, err := json.Marshal(config)
		if err != nil {
			return err
		}
		var current models.ChannelConfig
		if err := json.Unmarshal(encoded, &current); err != nil {
			return err
		}
		currentAccess, err := DecryptOrLegacy(m.encryption, current.EmailOAuthAccessToken)
		if err != nil {
			return err
		}
		currentRefresh, err := DecryptOrLegacy(m.encryption, current.EmailOAuthRefreshToken)
		if err != nil {
			return err
		}
		if !sameOAuthIdentity(expected, &current) || currentAccess != expected.EmailOAuthAccessToken ||
			currentRefresh != expected.EmailOAuthRefreshToken || !sameOptionalTime(current.EmailOAuthExpiresAt, expected.EmailOAuthExpiresAt) {
			return fmt.Errorf("email OAuth credentials changed during token refresh")
		}
		config["email_oauth_access_token"] = accessToken
		if refreshToken != "" {
			config["email_oauth_refresh_token"] = refreshToken
		}
		setOptionalConfigTime(config, "email_oauth_expires_at", expiresAt)
		return nil
	})
	return err
}

func sameOAuthIdentity(left, right *models.ChannelConfig) bool {
	if left == nil || right == nil {
		return false
	}
	return left.EmailAuthMethod == right.EmailAuthMethod &&
		left.EmailOAuthProviderType == right.EmailOAuthProviderType &&
		left.EmailOAuthClientID == right.EmailOAuthClientID &&
		left.EmailOAuthTenantID == right.EmailOAuthTenantID &&
		left.EmailOAuthMailbox == right.EmailOAuthMailbox &&
		sameOptionalInt(left.EmailProviderID, right.EmailProviderID)
}

func sameOptionalInt(left, right *int) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func sameOptionalTime(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}

// SaveOAuthTokens saves OAuth tokens for a channel after successful OAuth flow
func (m *CredentialManager) SaveOAuthTokens(
	ctx context.Context,
	channelID int,
	tokens *OAuthTokens,
	email string,
	expectedIdentity *models.ChannelConfig,
) (string, error) {
	var savedConfig string
	err := m.withCredentialLease(ctx, channelID, func() error {
		var saveErr error
		savedConfig, saveErr = m.saveOAuthTokens(ctx, channelID, tokens, email, nil, expectedIdentity)
		return saveErr
	})
	return savedConfig, err
}

// SaveOAuthTokensForProvider persists a central-provider OAuth result and its
// provider binding in one config mutation. Keeping these fields together
// avoids the old second blind read/modify/write in the provider callback.
func (m *CredentialManager) SaveOAuthTokensForProvider(
	ctx context.Context,
	channelID int,
	tokens *OAuthTokens,
	mailboxEmail string,
	providerID int,
	expectedIdentity *models.ChannelConfig,
) (string, error) {
	if providerID <= 0 {
		return "", fmt.Errorf("invalid email provider id %d", providerID)
	}
	var savedConfig string
	err := m.withCredentialLease(ctx, channelID, func() error {
		var saveErr error
		savedConfig, saveErr = m.saveOAuthTokens(ctx, channelID, tokens, mailboxEmail, &providerID, expectedIdentity)
		return saveErr
	})
	return savedConfig, err
}

func (m *CredentialManager) saveOAuthTokens(
	ctx context.Context,
	channelID int,
	tokens *OAuthTokens,
	mailboxEmail string,
	providerID *int,
	expectedIdentity *models.ChannelConfig,
) (string, error) {
	if tokens == nil {
		return "", fmt.Errorf("OAuth provider returned no tokens")
	}
	if strings.TrimSpace(tokens.AccessToken) == "" {
		return "", fmt.Errorf("OAuth provider returned an empty access token")
	}
	mailboxEmail = strings.TrimSpace(mailboxEmail)
	if mailboxEmail == "" {
		return "", fmt.Errorf("email channel has no monitored mailbox address configured")
	}
	if m.encryption == nil {
		return "", fmt.Errorf("OAuth token encryption is not configured")
	}

	accessTokenEnc, err := EncryptSecret(m.encryption, tokens.AccessToken)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt access token: %w", err)
	}

	var refreshTokenEnc string
	if tokens.RefreshToken != "" {
		refreshTokenEnc, err = EncryptSecret(m.encryption, tokens.RefreshToken)
		if err != nil {
			return "", fmt.Errorf("failed to encrypt refresh token: %w", err)
		}
	}

	savedConfig, err := m.patchChannelConfig(ctx, channelID, func(config map[string]any) error {
		encoded, err := json.Marshal(config)
		if err != nil {
			return err
		}
		var current models.ChannelConfig
		if err := json.Unmarshal(encoded, &current); err != nil {
			return err
		}
		if expectedIdentity != nil && !sameOAuthIdentity(expectedIdentity, &current) {
			return fmt.Errorf("email OAuth identity changed while authorization was in progress")
		}
		config["email_auth_method"] = "oauth"
		config["email_oauth_access_token"] = accessTokenEnc
		if refreshTokenEnc != "" {
			config["email_oauth_refresh_token"] = refreshTokenEnc
		} else if existing, _ := config["email_oauth_refresh_token"].(string); existing == "" && tokens.ExpiresAt != nil {
			return fmt.Errorf("OAuth provider returned no refresh token")
		}
		setOptionalConfigTime(config, "email_oauth_expires_at", tokens.ExpiresAt)
		// Keep the intent and verified address in lockstep after a successful
		// connection, so an enable check can compare them.
		config["email_oauth_mailbox"] = mailboxEmail
		config["email_oauth_email"] = mailboxEmail
		if providerID != nil {
			config["email_provider_id"] = *providerID
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	slog.Info("saved OAuth tokens for email channel", "channel_id", channelID, "email", mailboxEmail)

	return savedConfig, nil
}

const channelConfigPatchAttempts = 5

// patchChannelConfig performs a forward-compatible JSON-object patch with an
// optimistic compare-and-swap retry. It preserves keys unknown to this binary
// and merges over a concurrent admin edit instead of blindly replacing it.
func (m *CredentialManager) patchChannelConfig(ctx context.Context, channelID int, patch func(map[string]any) error) (string, error) {
	for attempt := 0; attempt < channelConfigPatchAttempts; attempt++ {
		var raw string
		if err := m.db.QueryRowContext(ctx, `
			SELECT COALESCE(config, '{}')
			FROM channels
			WHERE id = ? AND type = 'email' AND direction = 'inbound'
		`, channelID).Scan(&raw); err != nil {
			return "", fmt.Errorf("failed to get channel config: %w", err)
		}

		var config map[string]any
		normalized := strings.TrimSpace(raw)
		if normalized == "" || normalized == "null" {
			return "", fmt.Errorf("channel config must be a JSON object")
		}
		if err := json.Unmarshal([]byte(raw), &config); err != nil {
			return "", fmt.Errorf("failed to parse channel config: %w", err)
		}
		if config == nil {
			return "", fmt.Errorf("channel config must be a JSON object")
		}
		if err := patch(config); err != nil {
			return "", err
		}
		updated, err := json.Marshal(config)
		if err != nil {
			return "", fmt.Errorf("failed to marshal updated config: %w", err)
		}

		result, err := m.db.ExecWriteContext(ctx, `
			UPDATE channels
			SET config = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = ? AND type = 'email' AND direction = 'inbound'
			  AND COALESCE(config, '{}') = ?
		`, string(updated), channelID, raw)
		if err != nil {
			return "", fmt.Errorf("failed to update channel config: %w", err)
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return "", fmt.Errorf("failed to count updated channel configs: %w", err)
		}
		if rows > 0 {
			return string(updated), nil
		}
	}
	return "", fmt.Errorf("channel config changed repeatedly while saving OAuth credentials")
}

func setOptionalConfigTime(config map[string]any, key string, value *time.Time) {
	if value == nil {
		delete(config, key)
		return
	}
	config[key] = value
}

// splitScopes splits a space-separated scope string into a slice
func splitScopes(scopes string) []string {
	return strings.Fields(scopes)
}
