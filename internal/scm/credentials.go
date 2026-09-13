package scm

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/sso"
)

// refreshLockKey identifies a credential principal for the purpose of
// serializing concurrent OAuth refresh attempts against it.
//
// Gitea (and Forgejo) rotate refresh tokens on every refresh — the old
// refresh token is invalidated as soon as a new one is issued. Two
// goroutines that both decide to refresh the same expiring token will
// both submit the same old refresh token; the second submission lands on
// an already-rotated value and Gitea returns invalid_grant. Without
// serialization, the second request would mark the credentials dead even
// though the first refresh succeeded. We hold a per-principal mutex
// across the expiry-check and the refresh API call so the second
// goroutine sees the freshly-stored expiry and skips refreshing.
type refreshLockKey struct {
	authSource string // "user" or "workspace"
	id1        int    // userID for "user", connectionID for "workspace"
	id2        int    // providerID for "user", 0 for "workspace"
}

// refreshLocks is a process-global registry of mutexes keyed by
// refreshLockKey. The map grows with the number of unique credentials
// ever refreshed; entries are cheap (one *sync.Mutex each) and never
// cleaned up because the cost is negligible compared to the simplicity.
var refreshLocks sync.Map

func acquireRefreshLock(key refreshLockKey) func() {
	v, _ := refreshLocks.LoadOrStore(key, &sync.Mutex{})
	mu, ok := v.(*sync.Mutex)
	if !ok {
		// Programmer error — refreshLocks is populated only by this
		// function and only ever stores *sync.Mutex.
		panic(fmt.Sprintf("refreshLocks: unexpected value type %T", v))
	}
	mu.Lock()
	return mu.Unlock
}

// CredentialResolver centralizes credential loading for SCM providers.
//
// Per-auth-method hierarchy:
//   - OAuth: user-level (per-user, in user_scm_oauth_tokens) or workspace-level
//     (per workspace_scm_connections). No provider-level fallback — that path
//     was a footgun where the last user to connect overwrote the global
//     provider row.
//   - PAT: workspace-level if set, otherwise the provider's shared PAT.
//   - GitHub App: provider-level keys plus a workspace-level installation ID.
type CredentialResolver struct {
	db         database.Database
	encryption *sso.SecretEncryption
}

// ProviderCredentials contains all credentials needed to create a provider instance
type ProviderCredentials struct {
	ProviderType models.SCMProviderType
	AuthMethod   models.SCMAuthMethod
	BaseURL      string
	AuthSource   string // "user", "workspace", or "provider"
	UserID       int    // The user ID if AuthSource is "user"

	// OAuth credentials (decrypted)
	OAuthAccessToken  string
	OAuthRefreshToken string
	OAuthExpiresAt    *time.Time

	// Personal Access Token (decrypted)
	PersonalAccessToken string

	// GitHub App credentials (decrypted)
	GitHubAppID             string
	GitHubAppPrivateKey     string
	GitHubAppInstallationID string

	// OAuth App config for token refresh
	OAuthClientID     string
	OAuthClientSecret string
}

// NewCredentialResolver creates a new credential resolver
func NewCredentialResolver(db database.Database, encryption *sso.SecretEncryption) *CredentialResolver {
	return &CredentialResolver{
		db:         db,
		encryption: encryption,
	}
}

// applyGitHubAppCredentials fills in the GitHub App fields on creds from the
// provider-level columns, decrypting the private key if present.
func (r *CredentialResolver) applyGitHubAppCredentials(
	creds *ProviderCredentials,
	ghAppID, ghAppInstallationID, ghAppPrivateKeyEnc sql.NullString,
) error {
	creds.AuthSource = "provider"
	creds.GitHubAppID = ghAppID.String
	creds.GitHubAppInstallationID = ghAppInstallationID.String

	if ghAppPrivateKeyEnc.Valid && ghAppPrivateKeyEnc.String != "" {
		key, err := r.encryption.Decrypt(ghAppPrivateKeyEnc.String)
		if err != nil {
			return fmt.Errorf("failed to decrypt GitHub App private key: %w", err)
		}
		creds.GitHubAppPrivateKey = key
	}
	return nil
}

// GetCredentialsByConnectionID resolves credentials using a connection ID.
//
// OAuth credentials are resolved from the workspace connection only; there is
// no provider-level OAuth fallback. Provider-level OAuth tokens used to be
// written by the user-OAuth callback, which conflated whoever-connected-last
// with the workspace's identity — now the column is treated as legacy storage
// only. For per-user OAuth resolution use GetCredentialsForUser.
func (r *CredentialResolver) GetCredentialsByConnectionID(ctx context.Context, connectionID int) (*ProviderCredentials, error) {
	// Get provider and connection details
	var creds ProviderCredentials
	var providerID int
	var baseURL sql.NullString
	var providerPATEnc, providerOAuthClientSecretEnc sql.NullString
	var ghAppID, ghAppPrivateKeyEnc, ghAppInstallationID sql.NullString
	var wsOAuthTokenEnc, wsOAuthRefreshTokenEnc, wsPATEnc sql.NullString
	var wsOAuthExpiresAt sql.NullTime
	var oauthClientID sql.NullString

	err := r.db.QueryRow(`
		SELECT
			wsc.scm_provider_id,
			sp.provider_type, sp.auth_method, sp.base_url,
			sp.personal_access_token_encrypted,
			sp.oauth_client_id, sp.oauth_client_secret_encrypted,
			sp.github_app_id, sp.github_app_private_key_encrypted, sp.github_app_installation_id,
			wsc.oauth_access_token_encrypted, wsc.oauth_refresh_token_encrypted,
			wsc.oauth_token_expires_at, wsc.personal_access_token_encrypted
		FROM workspace_scm_connections wsc
		JOIN scm_providers sp ON sp.id = wsc.scm_provider_id
		WHERE wsc.id = ?
	`, connectionID).Scan(
		&providerID,
		&creds.ProviderType, &creds.AuthMethod, &baseURL,
		&providerPATEnc,
		&oauthClientID, &providerOAuthClientSecretEnc,
		&ghAppID, &ghAppPrivateKeyEnc, &ghAppInstallationID,
		&wsOAuthTokenEnc, &wsOAuthRefreshTokenEnc,
		&wsOAuthExpiresAt, &wsPATEnc,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("connection not found")
		}
		return nil, fmt.Errorf("failed to get connection: %w", err)
	}

	creds.BaseURL = baseURL.String
	creds.OAuthClientID = oauthClientID.String
	if wsOAuthExpiresAt.Valid {
		creds.OAuthExpiresAt = &wsOAuthExpiresAt.Time
	}

	// Decrypt OAuth client secret if present
	if providerOAuthClientSecretEnc.Valid && providerOAuthClientSecretEnc.String != "" {
		secret, err := r.encryption.Decrypt(providerOAuthClientSecretEnc.String)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt OAuth client secret: %w", err)
		}
		creds.OAuthClientSecret = secret
	}

	// Resolve credentials based on auth method
	switch creds.AuthMethod {
	case models.SCMAuthMethodGitHubApp:
		if err := r.applyGitHubAppCredentials(&creds, ghAppID, ghAppInstallationID, ghAppPrivateKeyEnc); err != nil {
			return nil, err
		}

	case models.SCMAuthMethodOAuth:
		if !wsOAuthTokenEnc.Valid || wsOAuthTokenEnc.String == "" {
			return nil, fmt.Errorf("OAuth token not configured for this workspace connection - please connect via OAuth")
		}
		creds.AuthSource = "workspace"
		token, decryptErr := r.encryption.Decrypt(wsOAuthTokenEnc.String)
		if decryptErr != nil {
			return nil, fmt.Errorf("failed to decrypt workspace OAuth token: %w", decryptErr)
		}
		creds.OAuthAccessToken = token

		if wsOAuthRefreshTokenEnc.Valid && wsOAuthRefreshTokenEnc.String != "" {
			refresh, refreshErr := r.encryption.Decrypt(wsOAuthRefreshTokenEnc.String)
			if refreshErr != nil {
				// Log but continue - refresh token is optional
				creds.OAuthRefreshToken = ""
			} else {
				creds.OAuthRefreshToken = refresh
			}
		}

	case models.SCMAuthMethodPAT:
		// Prefer workspace-level PAT, fall back to provider-level
		switch {
		case wsPATEnc.Valid && wsPATEnc.String != "":
			creds.AuthSource = "workspace"
			token, decryptErr := r.encryption.Decrypt(wsPATEnc.String)
			if decryptErr != nil {
				return nil, fmt.Errorf("failed to decrypt workspace PAT: %w", decryptErr)
			}
			creds.PersonalAccessToken = token
		case providerPATEnc.Valid && providerPATEnc.String != "":
			creds.AuthSource = "provider"
			token, decryptErr := r.encryption.Decrypt(providerPATEnc.String)
			if decryptErr != nil {
				return nil, fmt.Errorf("failed to decrypt provider PAT: %w", decryptErr)
			}
			creds.PersonalAccessToken = token
		default:
			return nil, fmt.Errorf("PAT not configured for this connection")
		}
	}

	if err := r.ensureFreshOAuthToken(ctx, connectionID, &creds); err != nil {
		return nil, err
	}

	return &creds, nil
}

// GetCredentialsForUser resolves credentials with user-level token priority.
// For OAuth auth method, it requires the user to have connected their own account.
// For PAT and GitHub App, it falls back to workspace/provider level.
func (r *CredentialResolver) GetCredentialsForUser(ctx context.Context, connectionID, userID int) (*ProviderCredentials, error) {
	// First get base connection info
	var providerID int
	var providerType models.SCMProviderType
	var authMethod models.SCMAuthMethod
	var baseURL sql.NullString
	var oauthClientID, oauthClientSecretEnc sql.NullString
	var ghAppID, ghAppPrivateKeyEnc, ghAppInstallationID sql.NullString
	var providerPATEnc, wsPATEnc sql.NullString

	err := r.db.QueryRow(`
		SELECT
			wsc.scm_provider_id,
			sp.provider_type, sp.auth_method, sp.base_url,
			sp.oauth_client_id, sp.oauth_client_secret_encrypted,
			sp.github_app_id, sp.github_app_private_key_encrypted, sp.github_app_installation_id,
			sp.personal_access_token_encrypted,
			wsc.personal_access_token_encrypted
		FROM workspace_scm_connections wsc
		JOIN scm_providers sp ON sp.id = wsc.scm_provider_id
		WHERE wsc.id = ?
	`, connectionID).Scan(
		&providerID,
		&providerType, &authMethod, &baseURL,
		&oauthClientID, &oauthClientSecretEnc,
		&ghAppID, &ghAppPrivateKeyEnc, &ghAppInstallationID,
		&providerPATEnc,
		&wsPATEnc,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("connection not found")
		}
		return nil, fmt.Errorf("failed to get connection: %w", err)
	}

	creds := &ProviderCredentials{
		ProviderType:  providerType,
		AuthMethod:    authMethod,
		BaseURL:       baseURL.String,
		OAuthClientID: oauthClientID.String,
	}

	// Decrypt OAuth client secret if present
	if oauthClientSecretEnc.Valid && oauthClientSecretEnc.String != "" {
		secret, err := r.encryption.Decrypt(oauthClientSecretEnc.String)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt OAuth client secret: %w", err)
		}
		creds.OAuthClientSecret = secret
	}

	// Resolve credentials based on auth method
	switch authMethod {
	case models.SCMAuthMethodGitHubApp:
		if err := r.applyGitHubAppCredentials(creds, ghAppID, ghAppInstallationID, ghAppPrivateKeyEnc); err != nil {
			return nil, err
		}

	case models.SCMAuthMethodOAuth:
		// For OAuth, require user-level token
		var userTokenEnc, userRefreshTokenEnc sql.NullString
		var userTokenExpiresAt sql.NullTime

		err := r.db.QueryRow(`
			SELECT oauth_access_token_encrypted, oauth_refresh_token_encrypted, oauth_token_expires_at
			FROM user_scm_oauth_tokens
			WHERE user_id = ? AND scm_provider_id = ?
		`, userID, providerID).Scan(&userTokenEnc, &userRefreshTokenEnc, &userTokenExpiresAt)

		if errors.Is(err, sql.ErrNoRows) {
			// User has not connected their account
			return nil, ErrUserSCMNotConnected
		}
		if err != nil {
			return nil, fmt.Errorf("failed to get user SCM token: %w", err)
		}

		if !userTokenEnc.Valid || userTokenEnc.String == "" {
			return nil, ErrUserSCMNotConnected
		}

		creds.AuthSource = "user"
		creds.UserID = userID
		token, err := r.encryption.Decrypt(userTokenEnc.String)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt user OAuth token: %w", err)
		}
		creds.OAuthAccessToken = token

		if userRefreshTokenEnc.Valid && userRefreshTokenEnc.String != "" {
			refresh, err := r.encryption.Decrypt(userRefreshTokenEnc.String)
			if err == nil {
				creds.OAuthRefreshToken = refresh
			}
		}

		if userTokenExpiresAt.Valid {
			creds.OAuthExpiresAt = &userTokenExpiresAt.Time
		}

		// Update last_used_at for the user token
		go func() {
			_, _ = r.db.ExecWrite(`
				UPDATE user_scm_oauth_tokens SET last_used_at = CURRENT_TIMESTAMP
				WHERE user_id = ? AND scm_provider_id = ?
			`, userID, providerID)
		}()

	case models.SCMAuthMethodPAT:
		// PATs are not user-specific: prefer the workspace-level PAT, fall
		// back to provider-level — same hierarchy as
		// GetCredentialsByConnectionID, so user-aware and connection-level
		// resolution behave identically on PAT connections.
		switch {
		case wsPATEnc.Valid && wsPATEnc.String != "":
			creds.AuthSource = "workspace"
			token, err := r.encryption.Decrypt(wsPATEnc.String)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt workspace PAT: %w", err)
			}
			creds.PersonalAccessToken = token
		case providerPATEnc.Valid && providerPATEnc.String != "":
			creds.AuthSource = "provider"
			token, err := r.encryption.Decrypt(providerPATEnc.String)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt provider PAT: %w", err)
			}
			creds.PersonalAccessToken = token
		default:
			return nil, fmt.Errorf("PAT not configured for this connection")
		}
	}

	if err := r.ensureFreshOAuthToken(ctx, connectionID, creds); err != nil {
		return nil, err
	}

	return creds, nil
}

// ensureFreshOAuthToken is the single refresh choke point for resolved
// credentials: every Get* resolution passes through here, so consumers
// (provider construction, the run-broker git proxy, PR creation) cannot
// end up holding an expired OAuth access token. No-op for non-OAuth
// credentials. A dead refresh token is terminal — the stored credentials
// have already been invalidated by RefreshOAuthTokenIfNeeded, so the
// resolution fails with ErrRefreshTokenInvalid in the chain rather than
// handing back a guaranteed-401 token. Transient refresh failures keep
// the existing token: it may still be accepted upstream, and failing the
// whole resolution for a network blip would be worse.
func (r *CredentialResolver) ensureFreshOAuthToken(ctx context.Context, connectionID int, creds *ProviderCredentials) error {
	if creds.OAuthAccessToken == "" {
		return nil
	}
	newToken, err := r.RefreshOAuthTokenIfNeeded(ctx, connectionID, creds)
	if err != nil {
		if errors.Is(err, ErrRefreshTokenInvalid) {
			return fmt.Errorf("scm credentials require reconnect: %w", err)
		}
		// RefreshOAuthTokenIfNeeded returns the freshly minted token even
		// when persisting it failed — prefer it over the expiring one.
		if newToken != "" {
			creds.OAuthAccessToken = newToken
		}
		slog.Warn("failed to refresh OAuth token, using best available", slog.String("component", "scm"), slog.Int("connection_id", connectionID), slog.Any("error", err))
		return nil
	}
	creds.OAuthAccessToken = newToken
	return nil
}

// GetProviderForUser is a convenience method that resolves user-specific credentials
// and creates a provider in one call
func (r *CredentialResolver) GetProviderForUser(ctx context.Context, connectionID, userID int) (Provider, error) {
	creds, err := r.GetCredentialsForUser(ctx, connectionID, userID)
	if err != nil {
		return nil, err
	}

	return r.CreateProvider(creds)
}

// CreateProvider creates a Provider instance from resolved credentials
func (r *CredentialResolver) CreateProvider(creds *ProviderCredentials) (Provider, error) {
	cfg := ProviderConfig{
		ProviderType:            creds.ProviderType,
		AuthMethod:              creds.AuthMethod,
		BaseURL:                 creds.BaseURL,
		OAuthClientID:           creds.OAuthClientID,
		OAuthClientSecret:       creds.OAuthClientSecret,
		OAuthAccessToken:        creds.OAuthAccessToken,
		OAuthRefreshToken:       creds.OAuthRefreshToken,
		PersonalAccessToken:     creds.PersonalAccessToken,
		GitHubAppID:             creds.GitHubAppID,
		GitHubAppPrivateKey:     creds.GitHubAppPrivateKey,
		GitHubAppInstallationID: creds.GitHubAppInstallationID,
	}

	return NewProvider(cfg)
}

// GetProviderForConnection is a convenience method that resolves credentials
// and creates a provider in one call
func (r *CredentialResolver) GetProviderForConnection(ctx context.Context, connectionID int) (Provider, error) {
	creds, err := r.GetCredentialsByConnectionID(ctx, connectionID)
	if err != nil {
		return nil, err
	}

	return r.CreateProvider(creds)
}

// RefreshOAuthTokenIfNeeded checks if the OAuth token is expired or
// expiring soon, and refreshes it if possible. Returns the (possibly
// new) access token.
//
// On ErrRefreshTokenInvalid the stored credentials are invalidated
// (user-level row deleted; workspace tokens nulled) so the caller's
// next read reports the credential as disconnected and the user is
// prompted to reconnect rather than wedged on a dead token.
func (r *CredentialResolver) RefreshOAuthTokenIfNeeded(ctx context.Context, connectionID int, creds *ProviderCredentials) (string, error) {
	// If no expiration is set, treat token as non-expiring (e.g., GitHub classic OAuth tokens)
	if creds.OAuthExpiresAt == nil {
		return creds.OAuthAccessToken, nil
	}

	// Check if token needs refresh (expiring within 5 minutes)
	if time.Until(*creds.OAuthExpiresAt) > 5*time.Minute {
		return creds.OAuthAccessToken, nil
	}

	// Serialize concurrent refresh attempts on the same principal so a
	// race doesn't burn the rotated refresh token on the loser. After
	// acquiring the lock we re-read the persisted expiry: if the winner
	// already refreshed, the new expiry will be in the future and we can
	// return without making a second refresh call.
	lockKey := refreshLockKeyFor(creds, connectionID)
	unlock := acquireRefreshLock(lockKey)
	defer unlock()

	if storedAccessToken, storedExpiresAt, ok := r.readStoredAccessToken(ctx, creds, connectionID); ok {
		if storedExpiresAt != nil && time.Until(*storedExpiresAt) > 5*time.Minute {
			// Another goroutine already refreshed.
			return storedAccessToken, nil
		}
	}

	// Token is expired or expiring soon - try to refresh
	if creds.OAuthRefreshToken == "" {
		return "", fmt.Errorf("token expired and no refresh token available")
	}

	// Create provider config for token refresh
	cfg := ProviderConfig{
		ProviderType:      creds.ProviderType,
		AuthMethod:        models.SCMAuthMethodOAuth,
		BaseURL:           creds.BaseURL,
		OAuthClientID:     creds.OAuthClientID,
		OAuthClientSecret: creds.OAuthClientSecret,
	}

	// Refresh token based on provider type
	var newTokens *OAuthTokens
	var err error

	switch creds.ProviderType {
	case models.SCMProviderTypeGitLab:
		var gitlabProvider *GitLabProvider
		gitlabProvider, err = NewGitLabProvider(cfg)
		if err != nil {
			return "", fmt.Errorf("failed to create provider for refresh: %w", err)
		}
		newTokens, err = gitlabProvider.RefreshToken(ctx, creds.OAuthRefreshToken)
		if err != nil {
			if errors.Is(err, ErrRefreshTokenInvalid) {
				r.invalidateStoredCredentials(ctx, creds, connectionID)
			}
			return "", fmt.Errorf("failed to refresh token: %w", err)
		}
	case models.SCMProviderTypeGitea:
		var giteaProvider *GiteaProvider
		giteaProvider, err = NewGiteaProvider(cfg)
		if err != nil {
			return "", fmt.Errorf("failed to create provider for refresh: %w", err)
		}
		newTokens, err = giteaProvider.RefreshToken(ctx, creds.OAuthRefreshToken)
		if err != nil {
			if errors.Is(err, ErrRefreshTokenInvalid) {
				r.invalidateStoredCredentials(ctx, creds, connectionID)
			}
			return "", fmt.Errorf("failed to refresh token: %w", err)
		}
	case models.SCMProviderTypeGitHub:
		return "", fmt.Errorf("token refresh not supported for GitHub OAuth")
	default:
		return "", fmt.Errorf("token refresh not supported for provider type: %s", creds.ProviderType)
	}

	// Encrypt and store new tokens
	newAccessTokenEnc, err := r.encryption.Encrypt(newTokens.AccessToken)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt new access token: %w", err)
	}

	var newRefreshTokenEnc string
	if newTokens.RefreshToken != "" {
		newRefreshTokenEnc, err = r.encryption.Encrypt(newTokens.RefreshToken)
		if err != nil {
			// Continue anyway - we have the access token
			newRefreshTokenEnc = ""
		}
	}

	// Update token storage based on auth source
	if creds.AuthSource == "user" && creds.UserID > 0 {
		// Update user-level token
		_, err = r.db.ExecWrite(`
			UPDATE user_scm_oauth_tokens SET
				oauth_access_token_encrypted = ?,
				oauth_refresh_token_encrypted = ?,
				oauth_token_expires_at = ?,
				updated_at = CURRENT_TIMESTAMP
			WHERE user_id = ? AND scm_provider_id = (
				SELECT scm_provider_id FROM workspace_scm_connections WHERE id = ?
			)
		`, newAccessTokenEnc, nullString(newRefreshTokenEnc), newTokens.ExpiresAt, creds.UserID, connectionID)
	} else {
		// Update workspace connection with new tokens
		_, err = r.db.ExecWrite(`
			UPDATE workspace_scm_connections SET
				oauth_access_token_encrypted = ?,
				oauth_refresh_token_encrypted = ?,
				oauth_token_expires_at = ?,
				updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, newAccessTokenEnc, nullString(newRefreshTokenEnc), newTokens.ExpiresAt, connectionID)
	}
	if err != nil {
		// Log but continue - we can use the new token for this request
		return newTokens.AccessToken, err
	}

	return newTokens.AccessToken, nil
}

// refreshLockKeyFor builds the lock key for a credential principal.
func refreshLockKeyFor(creds *ProviderCredentials, connectionID int) refreshLockKey {
	if creds.AuthSource == "user" && creds.UserID > 0 {
		// providerID is not directly on creds; resolve via connection in
		// readStoredAccessToken instead. Using connectionID + userID is
		// sufficient as a uniqueness key here because (user, connection)
		// → (user, provider) is a stable mapping in our schema.
		return refreshLockKey{authSource: "user", id1: creds.UserID, id2: connectionID}
	}
	return refreshLockKey{authSource: "workspace", id1: connectionID}
}

// readStoredAccessToken re-reads the currently stored access token and
// expiry for a credential principal. Used after acquiring the refresh
// lock to detect that another goroutine already performed the refresh.
// Returns ok=false if the credential row has been wiped (e.g. by a
// concurrent invalidate), in which case the caller should error out.
func (r *CredentialResolver) readStoredAccessToken(ctx context.Context, creds *ProviderCredentials, connectionID int) (string, *time.Time, bool) {
	var tokenEnc sql.NullString
	var expiresAt sql.NullTime
	var err error

	if creds.AuthSource == "user" && creds.UserID > 0 {
		err = r.db.QueryRowContext(ctx, `
			SELECT oauth_access_token_encrypted, oauth_token_expires_at
			FROM user_scm_oauth_tokens
			WHERE user_id = ? AND scm_provider_id = (
				SELECT scm_provider_id FROM workspace_scm_connections WHERE id = ?
			)
		`, creds.UserID, connectionID).Scan(&tokenEnc, &expiresAt)
	} else {
		err = r.db.QueryRowContext(ctx, `
			SELECT oauth_access_token_encrypted, oauth_token_expires_at
			FROM workspace_scm_connections
			WHERE id = ?
		`, connectionID).Scan(&tokenEnc, &expiresAt)
	}
	if err != nil || !tokenEnc.Valid || tokenEnc.String == "" {
		return "", nil, false
	}

	token, err := r.encryption.Decrypt(tokenEnc.String)
	if err != nil {
		return "", nil, false
	}
	var exp *time.Time
	if expiresAt.Valid {
		t := expiresAt.Time
		exp = &t
	}
	return token, exp, true
}

// invalidateStoredCredentials wipes the access/refresh tokens for a
// credential principal so subsequent reads report it as disconnected.
// Called when the provider has signaled that the refresh token is no
// longer redeemable; reconnecting via the OAuth flow is the only fix.
func (r *CredentialResolver) invalidateStoredCredentials(ctx context.Context, creds *ProviderCredentials, connectionID int) {
	var err error
	if creds.AuthSource == "user" && creds.UserID > 0 {
		_, err = r.db.ExecWriteContext(ctx, `
			DELETE FROM user_scm_oauth_tokens
			WHERE user_id = ? AND scm_provider_id = (
				SELECT scm_provider_id FROM workspace_scm_connections WHERE id = ?
			)
		`, creds.UserID, connectionID)
		if err != nil {
			slog.Error("failed to invalidate dead user OAuth credentials", slog.String("component", "scm"), slog.Int("user_id", creds.UserID), slog.Int("connection_id", connectionID), slog.Any("error", err))
			return
		}
		slog.Warn("invalidated user OAuth credentials after refresh failure; user must reconnect", slog.String("component", "scm"), slog.Int("user_id", creds.UserID), slog.Int("connection_id", connectionID))
		return
	}

	_, err = r.db.ExecWriteContext(ctx, `
		UPDATE workspace_scm_connections SET
			oauth_access_token_encrypted = NULL,
			oauth_refresh_token_encrypted = NULL,
			oauth_token_expires_at = NULL,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, connectionID)
	if err != nil {
		slog.Error("failed to invalidate dead workspace OAuth credentials", slog.String("component", "scm"), slog.Int("connection_id", connectionID), slog.Any("error", err))
		return
	}
	slog.Warn("invalidated workspace OAuth credentials after refresh failure; workspace admin must reconnect", slog.String("component", "scm"), slog.Int("connection_id", connectionID))
}

// nullString returns nil if the string is empty, otherwise returns the string
func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
