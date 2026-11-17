package handlers

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"windshift/internal/models"
	"windshift/internal/scm"
	"windshift/internal/utils"
)

// StartOAuth initiates the OAuth flow for an SCM provider.
//
// Lookup is restricted to providers that are enabled AND use the OAuth auth
// method. A disabled, PAT-only, or GitHub-App-only provider must be
// indistinguishable from a missing one to the caller — otherwise admins lose
// the ability to take a provider out of service without leaking its slug.
//
// When the provider's workspace_restriction_mode is 'restricted', the user
// must additionally belong to at least one workspace on the provider's
// allowlist. Users with no allowlisted workspace get the same 404, matching
// the codebase's "no existence leakage" policy (see base.go:CheckItemPermission).
func (h *SCMProviderHandler) StartOAuth(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	// Get provider by slug — but only if it's enabled and uses OAuth.
	var providerID int
	var providerType models.SCMProviderType
	var clientID sql.NullString
	var baseURL sql.NullString
	var oauthScopes sql.NullString
	var restrictionMode string

	err := h.db.QueryRow(`
		SELECT id, provider_type, oauth_client_id, base_url, scopes,
		       COALESCE(workspace_restriction_mode, 'unrestricted')
		FROM scm_providers
		WHERE slug = ? AND enabled = true AND auth_method = 'oauth'
	`, slug).Scan(&providerID, &providerType, &clientID, &baseURL, &oauthScopes, &restrictionMode)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondNotFound(w, r, "scm_provider")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	if !clientID.Valid || clientID.String == "" {
		respondBadRequest(w, r, "OAuth not configured for this provider")
		return
	}

	// Get user from context (requires authentication)
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	userID := user.ID

	// For restricted providers, the user must belong to at least one workspace
	// on the provider's allowlist. Without this check, any authenticated user
	// can land a personal token for a provider that admins have scoped to a
	// subset of workspaces they don't belong to.
	if restrictionMode == "restricted" {
		var allowed bool
		err = h.db.QueryRow(`
			SELECT EXISTS(
				SELECT 1
				FROM scm_provider_workspace_allowlist al
				JOIN user_workspace_roles uwr
				  ON uwr.workspace_id = al.workspace_id
				 AND uwr.user_id = ?
				WHERE al.provider_id = ?
			)
		`, userID, providerID).Scan(&allowed)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !allowed {
			respondNotFound(w, r, "scm_provider")
			return
		}
	}

	// Generate state
	stateBytes := make([]byte, 32)
	_, _ = rand.Read(stateBytes)
	state := base64.URLEncoding.EncodeToString(stateBytes)

	// Determine redirect URI
	redirectURI, err := h.getOAuthRedirectURI(slug)
	if err != nil {
		slog.Error("scm OAuth start: redirect URI unavailable", slog.String("component", "scm"), slog.String("slug", slug), slog.Any("error", err))
		respondServiceUnavailable(w, r, err.Error())
		return
	}
	slog.Debug("initiating OAuth", slog.String("component", "scm"), slog.String("slug", slug), slog.String("redirect_uri", redirectURI))

	// Store state token
	expiresAt := time.Now().Add(5 * time.Minute)
	_, err = h.db.ExecWrite(`
		INSERT INTO scm_oauth_state (provider_id, state, redirect_uri, user_id, expires_at)
		VALUES (?, ?, ?, ?, ?)
	`, providerID, state, redirectURI, userID, expiresAt)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Generate OAuth URL based on provider type
	var authURL string
	switch providerType {
	case models.SCMProviderTypeGitHub:
		scopes := oauthScopes.String
		authURL = fmt.Sprintf(
			"https://github.com/login/oauth/authorize?client_id=%s&redirect_uri=%s&scope=%s&state=%s",
			clientID.String,
			url.QueryEscape(redirectURI),
			url.QueryEscape(scopes),
			state,
		)
	case models.SCMProviderTypeGitea:
		// Gitea/Forgejo OAuth - requires base_url since it's self-hosted
		if !baseURL.Valid || baseURL.String == "" {
			respondBadRequest(w, r, "Base URL not configured for this provider")
			return
		}
		scopes := oauthScopes.String
		// Gitea OAuth URL format: {base_url}/login/oauth/authorize
		authURL = fmt.Sprintf(
			"%s/login/oauth/authorize?client_id=%s&redirect_uri=%s&response_type=code&scope=%s&state=%s",
			strings.TrimSuffix(baseURL.String, "/"),
			clientID.String,
			url.QueryEscape(redirectURI),
			url.QueryEscape(scopes),
			state,
		)
	case models.SCMProviderTypeGitLab:
		gitlabBaseURL := "https://gitlab.com"
		if baseURL.Valid && strings.TrimSpace(baseURL.String) != "" {
			gitlabBaseURL = strings.TrimSuffix(strings.TrimSuffix(baseURL.String, "/"), "/api/v4")
		}
		scopes := oauthScopes.String
		if scopes == "" {
			scopes = "api"
		}
		authURL = fmt.Sprintf(
			"%s/oauth/authorize?client_id=%s&redirect_uri=%s&response_type=code&scope=%s&state=%s",
			gitlabBaseURL,
			clientID.String,
			url.QueryEscape(redirectURI),
			url.QueryEscape(scopes),
			state,
		)
	default:
		respondBadRequest(w, r, "OAuth not supported for this provider type")
		return
	}

	// Return the auth URL for the frontend to redirect to
	respondJSONOK(w, map[string]string{
		"auth_url": authURL,
	})
}

// oauthLanding is the page an OAuth flow returns to, derived from the state
// row once it has been validated. The admin provider list is the fallback for
// failures that carry no usable state (missing or expired state), since the
// origin page cannot be recovered at that point.
type oauthLanding string

const (
	oauthLandingProfile oauthLanding = "/profile?tab=connected-accounts"
	oauthLandingAdmin   oauthLanding = "/admin/scm-providers"
)

// oauthLandingFromState resolves the landing for a validated state row.
// Workspace flows return to the workspace source-control settings with the
// numeric workspace ID: the SPA shell and workspace settings resolve only
// numeric IDs, not keys (the v2 API rejects non-numeric workspace IDs).
func oauthLandingFromState(workspaceID sql.NullInt64) oauthLanding {
	if workspaceID.Valid {
		return oauthLanding(fmt.Sprintf("/workspaces/%d/settings/source-control", workspaceID.Int64))
	}
	return oauthLandingProfile
}

// redirectOAuthOutcome sends the browser back to the flow's landing page with
// the outcome in the query string. Success and error share this builder so
// every landing page can acknowledge both outcomes the same way.
func redirectOAuthOutcome(w http.ResponseWriter, r *http.Request, landing oauthLanding, outcome, providerSlug, message string) {
	params := url.Values{}
	params.Set("oauth", outcome)
	if providerSlug != "" {
		params.Set("provider", providerSlug)
	}
	if message != "" {
		params.Set("message", message)
	}
	separator := "?"
	if strings.Contains(string(landing), "?") {
		separator = "&"
	}
	http.Redirect(w, r, string(landing)+separator+params.Encode(), http.StatusFound)
}

// OAuthCallback handles the OAuth callback
// Routes tokens to workspace-level storage when workspace_id is present in state
func (h *SCMProviderHandler) OAuthCallback(w http.ResponseWriter, r *http.Request) {
	slog.Debug("OAuth callback received", slog.String("component", "scm"), slog.String("remote_addr", r.RemoteAddr))
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		errorMsg := r.URL.Query().Get("error")
		if errorMsg == "" {
			errorMsg = "Missing code or state parameter"
		}
		redirectOAuthOutcome(w, r, oauthLandingAdmin, "error", "", errorMsg)
		return
	}

	// Validate state and get provider info
	var providerID, userID int
	var redirectURI string
	var workspaceID sql.NullInt64
	err := h.db.QueryRow(`
		SELECT provider_id, user_id, redirect_uri, workspace_id FROM scm_oauth_state
		WHERE state = ? AND expires_at > CURRENT_TIMESTAMP
	`, state).Scan(&providerID, &userID, &redirectURI, &workspaceID)
	if err != nil {
		slog.Warn("invalid OAuth state", slog.String("component", "scm"), slog.Any("error", err))
		redirectOAuthOutcome(w, r, oauthLandingAdmin, "error", "", "Invalid or expired state")
		return
	}
	// The state row is the only record of where the flow started, so every
	// outcome from here on returns to that landing page.
	landing := oauthLandingFromState(workspaceID)

	// Delete used state (check error)
	if _, err = h.db.ExecWrite("DELETE FROM scm_oauth_state WHERE state = ?", state); err != nil {
		slog.Warn("failed to delete OAuth state", slog.String("component", "scm"), slog.Any("error", err))
	}

	// Bind the flow to the initiating account so a forwarded authorization URL
	// cannot attach the victim's provider credentials to another identity.
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil || currentUser.ID != userID {
		slog.Warn("SCM OAuth state/account mismatch", slog.String("component", "scm"))
		redirectOAuthOutcome(w, r, landing, "error", "", "Sign in to the account that started this connection and try again")
		return
	}

	slog.Debug("OAuth state validated", slog.String("component", "scm"), slog.Int("provider_id", providerID), slog.String("redirect_uri", redirectURI), slog.Any("workspace_id", workspaceID))

	// Get provider details
	var providerType models.SCMProviderType
	var clientID, clientSecretEnc, providerBaseURL sql.NullString
	var providerSlug string

	err = h.db.QueryRow(`
		SELECT provider_type, oauth_client_id, oauth_client_secret_encrypted, base_url, slug
		FROM scm_providers WHERE id = ?
	`, providerID).Scan(&providerType, &clientID, &clientSecretEnc, &providerBaseURL, &providerSlug)
	if err != nil {
		slog.Error("failed to get provider", slog.String("component", "scm"), slog.Int("provider_id", providerID), slog.Any("error", err))
		redirectOAuthOutcome(w, r, landing, "error", "", "Provider not found")
		return
	}

	// Decrypt client secret
	clientSecret, err := h.encryption.Decrypt(clientSecretEnc.String)
	if err != nil {
		slog.Error("failed to decrypt client secret", slog.String("component", "scm"), slog.Int("provider_id", providerID), slog.Any("error", err))
		redirectOAuthOutcome(w, r, landing, "error", "", "Configuration error")
		return
	}

	// Exchange code for tokens
	tokenResult, err := h.exchangeOAuthCode(r.Context(), oauthTokenExchangeParams{
		providerType: providerType,
		baseURL:      providerBaseURL.String,
		clientID:     clientID.String,
		clientSecret: clientSecret,
		code:         code,
		redirectURI:  redirectURI,
	})
	if err != nil {
		slog.Error("failed to exchange OAuth code", slog.String("component", "scm"), slog.Int("provider_id", providerID), slog.Any("error", err))
		redirectOAuthOutcome(w, r, landing, "error", "", "Failed to exchange token")
		return
	}

	// Encrypt tokens
	encTokens, err := h.encryptOAuthTokens(tokenResult.accessToken, tokenResult.refreshToken)
	if err != nil {
		slog.Error("failed to encrypt tokens", slog.String("component", "scm"), slog.Int("provider_id", providerID), slog.Any("error", err))
		redirectOAuthOutcome(w, r, landing, "error", "", "Failed to store token")
		return
	}

	slog.Debug("OAuth token exchange successful", slog.String("component", "scm"), slog.String("slug", providerSlug), slog.Bool("has_refresh", tokenResult.refreshToken != ""))

	// Fetch SCM user info
	userInfo := h.fetchSCMUserInfo(r.Context(), providerType, providerBaseURL.String, tokenResult.accessToken)

	// Store token at user level
	slog.Debug("storing token at user level", slog.String("component", "scm"), slog.Int("user_id", userID), slog.Int("provider_id", providerID), slog.String("slug", providerSlug))
	err = h.storeUserOAuthToken(r.Context(), userID, providerID, encTokens, tokenResult.expiresAt, userInfo)
	if err != nil {
		slog.Error("failed to store user OAuth token", slog.String("component", "scm"), slog.Int("user_id", userID), slog.Int("provider_id", providerID), slog.Any("error", err))
		redirectOAuthOutcome(w, r, landing, "error", "", "Failed to store token")
		return
	}

	slog.Info("OAuth token stored successfully at user level", slog.String("component", "scm"), slog.Int("user_id", userID), slog.String("slug", providerSlug), slog.String("scm_username", userInfo.username))

	// Store at workspace connection level when initiated from workspace settings
	if workspaceID.Valid {
		if _, err := h.db.ExecWrite(`
			UPDATE workspace_scm_connections SET
				oauth_access_token_encrypted = ?,
				oauth_refresh_token_encrypted = ?,
				oauth_token_expires_at = ?,
				updated_at = CURRENT_TIMESTAMP
			WHERE workspace_id = ? AND scm_provider_id = ?
		`, encTokens.accessToken, nullString(encTokens.refreshToken), tokenResult.expiresAt,
			workspaceID.Int64, providerID); err != nil {
			slog.Warn("failed to store workspace-level OAuth token",
				slog.String("component", "scm"),
				slog.Int64("workspace_id", workspaceID.Int64),
				slog.Int("provider_id", providerID),
				slog.Any("error", err))
		}
	}

	// Both outcomes return to the same landing page so the origin UI can
	// acknowledge success and failure alike.
	redirectOAuthOutcome(w, r, landing, "success", providerSlug, "")
}

// Helper methods

// oauthTokenExchangeParams contains parameters for OAuth token exchange
type oauthTokenExchangeParams struct {
	providerType models.SCMProviderType
	baseURL      string
	clientID     string
	clientSecret string
	code         string
	redirectURI  string
}

// oauthTokenResult contains the result of OAuth token exchange
type oauthTokenResult struct {
	accessToken  string
	refreshToken string
	expiresAt    *time.Time
}

// exchangeOAuthCode exchanges an OAuth code for tokens based on provider type
func (h *SCMProviderHandler) exchangeOAuthCode(ctx context.Context, params oauthTokenExchangeParams) (*oauthTokenResult, error) {
	switch params.providerType {
	case models.SCMProviderTypeGitHub:
		cfg := scm.ProviderConfig{
			HTTPClientPool:    h.httpClients,
			ProviderType:      params.providerType,
			AuthMethod:        models.SCMAuthMethodOAuth,
			OAuthClientID:     params.clientID,
			OAuthClientSecret: params.clientSecret,
		}
		ghProvider, err := scm.NewGitHubProvider(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create GitHub provider: %w", err)
		}

		tokens, err := ghProvider.ExchangeCode(ctx, params.code, params.redirectURI)
		if err != nil {
			return nil, fmt.Errorf("failed to exchange code: %w", err)
		}

		return &oauthTokenResult{
			accessToken:  tokens.AccessToken,
			refreshToken: tokens.RefreshToken,
			expiresAt:    tokens.ExpiresAt,
		}, nil

	case models.SCMProviderTypeGitea:
		cfg := scm.ProviderConfig{
			HTTPClientPool:    h.httpClients,
			ProviderType:      params.providerType,
			AuthMethod:        models.SCMAuthMethodOAuth,
			BaseURL:           params.baseURL,
			OAuthClientID:     params.clientID,
			OAuthClientSecret: params.clientSecret,
		}
		giteaProvider, err := scm.NewGiteaProvider(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create Gitea provider: %w", err)
		}

		tokens, err := giteaProvider.ExchangeCode(ctx, params.code, params.redirectURI)
		if err != nil {
			return nil, fmt.Errorf("failed to exchange code: %w", err)
		}

		return &oauthTokenResult{
			accessToken:  tokens.AccessToken,
			refreshToken: tokens.RefreshToken,
			expiresAt:    tokens.ExpiresAt,
		}, nil

	case models.SCMProviderTypeGitLab:
		cfg := scm.ProviderConfig{
			HTTPClientPool:    h.httpClients,
			ProviderType:      params.providerType,
			AuthMethod:        models.SCMAuthMethodOAuth,
			BaseURL:           params.baseURL,
			OAuthClientID:     params.clientID,
			OAuthClientSecret: params.clientSecret,
		}
		gitlabProvider, err := scm.NewGitLabProvider(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create GitLab provider: %w", err)
		}
		tokens, err := gitlabProvider.ExchangeCode(ctx, params.code, params.redirectURI)
		if err != nil {
			return nil, fmt.Errorf("failed to exchange code: %w", err)
		}
		return &oauthTokenResult{accessToken: tokens.AccessToken, refreshToken: tokens.RefreshToken, expiresAt: tokens.ExpiresAt}, nil

	default:
		return nil, fmt.Errorf("OAuth not supported for provider type: %s", params.providerType)
	}
}

// encryptedTokens contains encrypted OAuth tokens
type encryptedTokens struct {
	accessToken  string
	refreshToken string
}

// encryptOAuthTokens encrypts OAuth access and refresh tokens
func (h *SCMProviderHandler) encryptOAuthTokens(accessToken, refreshToken string) (*encryptedTokens, error) {
	accessTokenEnc, err := h.encryption.Encrypt(accessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt access token: %w", err)
	}

	var refreshTokenEnc string
	if refreshToken != "" {
		refreshTokenEnc, err = h.encryption.Encrypt(refreshToken)
		if err != nil {
			slog.Warn("failed to encrypt refresh token", slog.String("component", "scm"), slog.Any("error", err))
			// Continue anyway - access token is the important one
		}
	}

	return &encryptedTokens{
		accessToken:  accessTokenEnc,
		refreshToken: refreshTokenEnc,
	}, nil
}

// scmUserInfo contains user information from an SCM provider
type scmUserInfo struct {
	username  string
	userID    string
	avatarURL string
}

// fetchSCMUserInfo fetches user information from the SCM provider
func (h *SCMProviderHandler) fetchSCMUserInfo(ctx context.Context, providerType models.SCMProviderType, baseURL, accessToken string) *scmUserInfo {
	info := &scmUserInfo{}

	switch providerType {
	case models.SCMProviderTypeGitHub:
		cfg := scm.ProviderConfig{
			HTTPClientPool:   h.httpClients,
			ProviderType:     providerType,
			AuthMethod:       models.SCMAuthMethodOAuth,
			OAuthAccessToken: accessToken,
		}
		ghProvider, err := scm.NewGitHubProvider(cfg)
		if err != nil {
			slog.Warn("failed to create GitHub provider for user info", slog.String("component", "scm"), slog.Any("error", err))
			return info
		}
		scmUser, err := ghProvider.GetCurrentUser(ctx)
		if err != nil {
			slog.Warn("failed to get GitHub user info", slog.String("component", "scm"), slog.Any("error", err))
			return info
		}
		info.username = scmUser.Username
		info.userID = scmUser.ID
		info.avatarURL = scmUser.AvatarURL

	case models.SCMProviderTypeGitea:
		cfg := scm.ProviderConfig{
			HTTPClientPool:   h.httpClients,
			ProviderType:     providerType,
			AuthMethod:       models.SCMAuthMethodOAuth,
			BaseURL:          baseURL,
			OAuthAccessToken: accessToken,
		}
		giteaProvider, err := scm.NewGiteaProvider(cfg)
		if err != nil {
			slog.Warn("failed to create Gitea provider for user info", slog.String("component", "scm"), slog.Any("error", err))
			return info
		}
		scmUser, err := giteaProvider.GetCurrentUser(ctx)
		if err != nil {
			slog.Warn("failed to get Gitea user info", slog.String("component", "scm"), slog.Any("error", err))
			return info
		}
		info.username = scmUser.Username
		info.userID = scmUser.ID
		info.avatarURL = scmUser.AvatarURL

	case models.SCMProviderTypeGitLab:
		cfg := scm.ProviderConfig{HTTPClientPool: h.httpClients, ProviderType: providerType, AuthMethod: models.SCMAuthMethodOAuth, BaseURL: baseURL, OAuthAccessToken: accessToken}
		gitlabProvider, err := scm.NewGitLabProvider(cfg)
		if err != nil {
			slog.Warn("failed to create GitLab provider for user info", slog.String("component", "scm"), slog.Any("error", err))
			return info
		}
		scmUser, err := gitlabProvider.GetCurrentUser(ctx)
		if err != nil {
			slog.Warn("failed to get GitLab user info", slog.String("component", "scm"), slog.Any("error", err))
			return info
		}
		info.username = scmUser.Username
		info.userID = scmUser.ID
		info.avatarURL = scmUser.AvatarURL
	}

	if info.username != "" {
		slog.Debug("got user info", slog.String("component", "scm"), slog.String("username", info.username), slog.String("scm_user_id", info.userID))
	}

	return info
}

// storeUserOAuthToken stores OAuth tokens at the user level
func (h *SCMProviderHandler) storeUserOAuthToken(ctx context.Context, userID, providerID int, tokens *encryptedTokens, expiresAt *time.Time, userInfo *scmUserInfo) error {
	_, err := h.db.ExecWriteContext(ctx, `
		INSERT INTO user_scm_oauth_tokens (
			user_id, scm_provider_id, oauth_access_token_encrypted,
			oauth_refresh_token_encrypted, oauth_token_expires_at,
			scm_username, scm_user_id, scm_avatar_url
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, scm_provider_id) DO UPDATE SET
			oauth_access_token_encrypted = excluded.oauth_access_token_encrypted,
			oauth_refresh_token_encrypted = excluded.oauth_refresh_token_encrypted,
			oauth_token_expires_at = excluded.oauth_token_expires_at,
			scm_username = excluded.scm_username,
			scm_user_id = excluded.scm_user_id,
			scm_avatar_url = excluded.scm_avatar_url,
			updated_at = CURRENT_TIMESTAMP
	`, userID, providerID, tokens.accessToken, nullString(tokens.refreshToken), expiresAt,
		nullString(userInfo.username), nullString(userInfo.userID), nullString(userInfo.avatarURL))
	return err
}

// getOAuthRedirectURI uses only the configured baseURL; request hosts are untrusted.
func (h *SCMProviderHandler) getOAuthRedirectURI(slug string) (string, error) {
	if h.baseURL == "" {
		return "", fmt.Errorf("scm OAuth is not configured: server baseURL is unset")
	}
	return h.baseURL + "/api/scm/oauth/" + slug + "/callback", nil
}
