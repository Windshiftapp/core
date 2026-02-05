package scm

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/sso"
)

// CredentialResolver centralizes credential loading for SCM providers.
// It handles the credential hierarchy: user-level > workspace-level > provider-level
// and supports all auth methods (OAuth, PAT, GitHub App).
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

// GetCredentials resolves credentials for a workspace connection.
// It follows this hierarchy:
// 1. For GitHub App: always use provider-level credentials
// 2. For OAuth: use workspace-level token if present
// 3. For PAT: prefer workspace-level, fall back to provider-level
func (r *CredentialResolver) GetCredentials(ctx context.Context, providerID, workspaceID int) (*ProviderCredentials, error) {
	// First, get the workspace_scm_connection
	var connectionID int
	err := r.db.QueryRow(`
		SELECT id FROM workspace_scm_connections
		WHERE workspace_id = ? AND scm_provider_id = ?
	`, workspaceID, providerID).Scan(&connectionID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("workspace connection not found")
		}
		return nil, fmt.Errorf("failed to find workspace connection: %w", err)
	}

	return r.GetCredentialsByConnectionID(ctx, connectionID)
}

// GetCredentialsByConnectionID resolves credentials using a connection ID
func (r *CredentialResolver) GetCredentialsByConnectionID(ctx context.Context, connectionID int) (*ProviderCredentials, error) {
	// Get provider and connection details
	var creds ProviderCredentials
	var providerID int
	var baseURL sql.NullString
	var providerPATEnc, providerOAuthClientSecretEnc sql.NullString
	var providerOAuthTokenEnc, providerOAuthRefreshTokenEnc sql.NullString
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
			sp.oauth_access_token_encrypted, sp.oauth_refresh_token_encrypted,
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
		&providerOAuthTokenEnc, &providerOAuthRefreshTokenEnc,
		&ghAppID, &ghAppPrivateKeyEnc, &ghAppInstallationID,
		&wsOAuthTokenEnc, &wsOAuthRefreshTokenEnc,
		&wsOAuthExpiresAt, &wsPATEnc,
	)
	if err != nil {
		if err == sql.ErrNoRows {
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
		// GitHub App credentials are always at provider level
		creds.AuthSource = "provider"
		creds.GitHubAppID = ghAppID.String
		creds.GitHubAppInstallationID = ghAppInstallationID.String

		if ghAppPrivateKeyEnc.Valid && ghAppPrivateKeyEnc.String != "" {
			key, err := r.encryption.Decrypt(ghAppPrivateKeyEnc.String)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt GitHub App private key: %w", err)
			}
			creds.GitHubAppPrivateKey = key
		}

	case models.SCMAuthMethodOAuth:
		// Prefer workspace-level OAuth token, fall back to provider-level
		switch {
		case wsOAuthTokenEnc.Valid && wsOAuthTokenEnc.String != "":
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
		case providerOAuthTokenEnc.Valid && providerOAuthTokenEnc.String != "":
			// Fall back to provider-level OAuth token
			creds.AuthSource = "provider"
			token, decryptErr := r.encryption.Decrypt(providerOAuthTokenEnc.String)
			if decryptErr != nil {
				return nil, fmt.Errorf("failed to decrypt provider OAuth token: %w", decryptErr)
			}
			creds.OAuthAccessToken = token

			if providerOAuthRefreshTokenEnc.Valid && providerOAuthRefreshTokenEnc.String != "" {
				refresh, refreshErr := r.encryption.Decrypt(providerOAuthRefreshTokenEnc.String)
				if refreshErr != nil {
					creds.OAuthRefreshToken = ""
				} else {
					creds.OAuthRefreshToken = refresh
				}
			}
		default:
			// No OAuth token at either level
			return nil, fmt.Errorf("OAuth token not configured - please connect via OAuth")
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
	var providerPATEnc sql.NullString

	err := r.db.QueryRow(`
		SELECT
			wsc.scm_provider_id,
			sp.provider_type, sp.auth_method, sp.base_url,
			sp.oauth_client_id, sp.oauth_client_secret_encrypted,
			sp.github_app_id, sp.github_app_private_key_encrypted, sp.github_app_installation_id,
			sp.personal_access_token_encrypted
		FROM workspace_scm_connections wsc
		JOIN scm_providers sp ON sp.id = wsc.scm_provider_id
		WHERE wsc.id = ?
	`, connectionID).Scan(
		&providerID,
		&providerType, &authMethod, &baseURL,
		&oauthClientID, &oauthClientSecretEnc,
		&ghAppID, &ghAppPrivateKeyEnc, &ghAppInstallationID,
		&providerPATEnc,
	)
	if err != nil {
		if err == sql.ErrNoRows {
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
		// GitHub App credentials are always at provider level
		creds.AuthSource = "provider"
		creds.GitHubAppID = ghAppID.String
		creds.GitHubAppInstallationID = ghAppInstallationID.String

		if ghAppPrivateKeyEnc.Valid && ghAppPrivateKeyEnc.String != "" {
			key, err := r.encryption.Decrypt(ghAppPrivateKeyEnc.String)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt GitHub App private key: %w", err)
			}
			creds.GitHubAppPrivateKey = key
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

		if err == sql.ErrNoRows {
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
			_, _ = r.db.Exec(`
				UPDATE user_scm_oauth_tokens SET last_used_at = CURRENT_TIMESTAMP
				WHERE user_id = ? AND scm_provider_id = ?
			`, userID, providerID)
		}()

	case models.SCMAuthMethodPAT:
		// For PAT, use provider-level token (not user-specific)
		if providerPATEnc.Valid && providerPATEnc.String != "" {
			creds.AuthSource = "provider"
			token, err := r.encryption.Decrypt(providerPATEnc.String)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt provider PAT: %w", err)
			}
			creds.PersonalAccessToken = token
		} else {
			return nil, fmt.Errorf("PAT not configured for this provider")
		}
	}

	return creds, nil
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

// GetProviderForWorkspace is a convenience method that resolves credentials
// by workspace and provider ID, and creates a provider
func (r *CredentialResolver) GetProviderForWorkspace(ctx context.Context, providerID, workspaceID int) (Provider, error) {
	creds, err := r.GetCredentials(ctx, providerID, workspaceID)
	if err != nil {
		return nil, err
	}

	return r.CreateProvider(creds)
}

// RefreshOAuthTokenIfNeeded checks if the OAuth token is expired or expiring soon,
// and refreshes it if possible. Returns the (possibly new) access token.
func (r *CredentialResolver) RefreshOAuthTokenIfNeeded(ctx context.Context, connectionID int, creds *ProviderCredentials) (string, error) {
	// If no expiration is set, treat token as non-expiring (e.g., GitHub classic OAuth tokens)
	if creds.OAuthExpiresAt == nil {
		return creds.OAuthAccessToken, nil
	}

	// Check if token needs refresh (expiring within 5 minutes)
	if time.Until(*creds.OAuthExpiresAt) > 5*time.Minute {
		return creds.OAuthAccessToken, nil
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
	case models.SCMProviderTypeGitea:
		var giteaProvider *GiteaProvider
		giteaProvider, err = NewGiteaProvider(cfg)
		if err != nil {
			return "", fmt.Errorf("failed to create provider for refresh: %w", err)
		}
		newTokens, err = giteaProvider.RefreshToken(ctx, creds.OAuthRefreshToken)
		if err != nil {
			return "", fmt.Errorf("failed to refresh token: %w", err)
		}
	case models.SCMProviderTypeGitHub:
		var ghProvider *GitHubProvider
		ghProvider, err = NewGitHubProvider(cfg)
		if err != nil {
			return "", fmt.Errorf("failed to create provider for refresh: %w", err)
		}
		newTokens, err = ghProvider.RefreshToken(ctx, creds.OAuthRefreshToken)
		if err != nil {
			return "", fmt.Errorf("failed to refresh token: %w", err)
		}
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
		_, err = r.db.Exec(`
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
		_, err = r.db.Exec(`
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

// nullString returns nil if the string is empty, otherwise returns the string
func nullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
