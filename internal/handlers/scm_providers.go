package handlers

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/middleware"
	"windshift/internal/models"
	"windshift/internal/scm"
	"windshift/internal/sso"
)

// SCMProviderHandler handles SCM provider management endpoints
type SCMProviderHandler struct {
	db         database.Database
	encryption *sso.SecretEncryption
	baseURL    string
}

// SCMProviderResponse represents a provider for API responses (without secrets)
type SCMProviderResponse struct {
	ID                       int                    `json:"id"`
	Slug                     string                 `json:"slug"`
	Name                     string                 `json:"name"`
	ProviderType             models.SCMProviderType `json:"provider_type"`
	AuthMethod               models.SCMAuthMethod   `json:"auth_method"`
	Enabled                  bool                   `json:"enabled"`
	IsDefault                bool                   `json:"is_default"`
	BaseURL                  string                 `json:"base_url,omitempty"`
	OAuthClientID            string                 `json:"oauth_client_id,omitempty"`
	HasOAuthClientSecret     bool                   `json:"has_oauth_client_secret"`
	HasPAT                   bool                   `json:"has_pat"`
	GitHubAppID              string                 `json:"github_app_id,omitempty"`
	HasGitHubAppPrivateKey   bool                   `json:"has_github_app_private_key"`
	GitHubAppInstallationID  string                 `json:"github_app_installation_id,omitempty"`
	GitHubOrgID              *int64                 `json:"github_org_id,omitempty"`
	HasOAuthToken            bool                   `json:"has_oauth_token"`
	OAuthTokenExpiresAt      *time.Time             `json:"oauth_token_expires_at,omitempty"`
	Scopes                   string                 `json:"scopes"`
	WorkspaceRestrictionMode string                 `json:"workspace_restriction_mode"` // 'unrestricted' or 'restricted'
	CreatedAt                time.Time              `json:"created_at"`
	UpdatedAt                time.Time              `json:"updated_at"`
}

// NewSCMProviderHandler creates a new SCM provider handler
func NewSCMProviderHandler(db database.Database) *SCMProviderHandler {
	// Get server secret for encryption (reuse SSO secret)
	serverSecret := os.Getenv("SSO_SECRET")
	if serverSecret == "" {
		serverSecret = os.Getenv("SESSION_SECRET")
	}
	if serverSecret == "" {
		slog.Error("SSO_SECRET or SESSION_SECRET environment variable must be set for SCM credential encryption", slog.String("component", "scm"))
		os.Exit(1)
	}

	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = os.Getenv("PUBLIC_URL")
	}

	return &SCMProviderHandler{
		db:         db,
		encryption: sso.NewSecretEncryption(serverSecret),
		baseURL:    baseURL,
	}
}

// GetEncryption returns the encryption service for use by other handlers
func (h *SCMProviderHandler) GetEncryption() *sso.SecretEncryption {
	return h.encryption
}

// refreshOAuthTokenIfNeeded checks if an OAuth token is expired or expiring soon,
// and refreshes it if a refresh token is available. Returns the (possibly new) access token.
func (h *SCMProviderHandler) refreshOAuthTokenIfNeeded(
	ctx context.Context,
	providerID int,
	providerType models.SCMProviderType,
	baseURL string,
	accessToken string,
	refreshTokenEnc string,
	expiresAt *time.Time,
	clientID string,
	clientSecretEnc string,
) (string, error) {
	// If no expiration is set, treat token as non-expiring (e.g., GitHub classic OAuth tokens)
	if expiresAt == nil {
		return accessToken, nil
	}

	// Check if token needs refresh (expiring within 5 minutes)
	if time.Until(*expiresAt) > 5*time.Minute {
		// Token is still valid, no refresh needed
		return accessToken, nil
	}

	// Token is expired or expiring soon - try to refresh
	if refreshTokenEnc == "" {
		return "", fmt.Errorf("token expired and no refresh token available")
	}

	// Decrypt refresh token and client secret
	refreshToken, err := h.encryption.Decrypt(refreshTokenEnc)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt refresh token: %w", err)
	}

	clientSecret := ""
	if clientSecretEnc != "" {
		clientSecret, err = h.encryption.Decrypt(clientSecretEnc)
		if err != nil {
			return "", fmt.Errorf("failed to decrypt client secret: %w", err)
		}
	}

	// Create provider config for token refresh
	cfg := scm.ProviderConfig{
		ProviderType:      providerType,
		AuthMethod:        models.SCMAuthMethodOAuth,
		BaseURL:           baseURL,
		OAuthClientID:     clientID,
		OAuthClientSecret: clientSecret,
	}

	// Refresh token based on provider type
	var newTokens *scm.OAuthTokens
	switch providerType {
	case models.SCMProviderTypeGitea:
		var provider *scm.GiteaProvider
		provider, err = scm.NewGiteaProvider(cfg)
		if err != nil {
			return "", fmt.Errorf("failed to create provider for refresh: %w", err)
		}
		newTokens, err = provider.RefreshToken(ctx, refreshToken)
		if err != nil {
			return "", fmt.Errorf("failed to refresh token: %w", err)
		}
	case models.SCMProviderTypeGitHub:
		var provider *scm.GitHubProvider
		provider, err = scm.NewGitHubProvider(cfg)
		if err != nil {
			return "", fmt.Errorf("failed to create provider for refresh: %w", err)
		}
		newTokens, err = provider.RefreshToken(ctx, refreshToken)
		if err != nil {
			return "", fmt.Errorf("failed to refresh token: %w", err)
		}
	default:
		return "", fmt.Errorf("token refresh not supported for provider type: %s", providerType)
	}

	// Encrypt and store new tokens
	newAccessTokenEnc, err := h.encryption.Encrypt(newTokens.AccessToken)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt new access token: %w", err)
	}

	var newRefreshTokenEnc string
	if newTokens.RefreshToken != "" {
		newRefreshTokenEnc, err = h.encryption.Encrypt(newTokens.RefreshToken)
		if err != nil {
			slog.Warn("failed to encrypt new refresh token", slog.String("component", "scm"), slog.Any("error", err))
			// Continue anyway - we have the access token
		}
	}

	// Update database with new tokens
	_, err = h.db.Exec(`
		UPDATE scm_providers SET
			oauth_access_token_encrypted = ?,
			oauth_refresh_token_encrypted = ?,
			oauth_token_expires_at = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, newAccessTokenEnc, nullString(newRefreshTokenEnc), newTokens.ExpiresAt, providerID)
	if err != nil {
		slog.Warn("failed to store refreshed tokens", slog.String("component", "scm"), slog.Any("error", err))
		// Continue anyway - we can use the new token for this request
	}

	slog.Info("successfully refreshed OAuth token", slog.String("component", "scm"), slog.Int("provider_id", providerID))
	return newTokens.AccessToken, nil
}

// GetProviders returns all SCM providers
func (h *SCMProviderHandler) GetProviders(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(providerListQuery + " ORDER BY name")
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	providers := []SCMProviderResponse{}
	for rows.Next() {
		var row providerRowScanResult
		if err := rows.Scan(row.scanDestinations()...); err != nil {
			slog.Error("failed to scan provider", slog.String("component", "scm"), slog.Any("error", err))
			continue
		}
		providers = append(providers, row.toResponse())
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(providers)
}

// GetProvider returns a single SCM provider
func (h *SCMProviderHandler) GetProvider(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "providerId")
		return
	}

	provider, err := h.getProviderByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "scm_provider")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(provider)
}

// CreateProvider creates a new SCM provider
func (h *SCMProviderHandler) CreateProvider(w http.ResponseWriter, r *http.Request) {
	var req models.SCMProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Validate required fields
	if req.Slug == "" || req.Name == "" || req.ProviderType == "" || req.AuthMethod == "" {
		respondValidationError(w, r, "Missing required fields: slug, name, provider_type, auth_method")
		return
	}

	// Validate provider type (only GitHub and Gitea supported)
	validTypes := map[models.SCMProviderType]bool{
		models.SCMProviderTypeGitHub: true,
		models.SCMProviderTypeGitea:  true,
	}
	if !validTypes[req.ProviderType] {
		respondBadRequest(w, r, "Invalid provider type. Supported: github, gitea")
		return
	}

	// Validate auth method
	validMethods := map[models.SCMAuthMethod]bool{
		models.SCMAuthMethodOAuth:     true,
		models.SCMAuthMethodPAT:       true,
		models.SCMAuthMethodGitHubApp: true,
	}
	if !validMethods[req.AuthMethod] {
		respondBadRequest(w, r, "Invalid auth method")
		return
	}

	// GitHub App auth method is only valid for GitHub providers
	if req.AuthMethod == models.SCMAuthMethodGitHubApp && req.ProviderType != models.SCMProviderTypeGitHub {
		respondBadRequest(w, r, "GitHub App auth method is only valid for GitHub providers")
		return
	}

	// Encrypt secrets
	var oauthSecretEnc, patEnc, ghAppKeyEnc string
	var err error

	if req.OAuthClientSecret != "" {
		oauthSecretEnc, err = h.encryption.Encrypt(req.OAuthClientSecret)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	if req.PersonalAccessToken != "" {
		patEnc, err = h.encryption.Encrypt(req.PersonalAccessToken)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	if req.GitHubAppPrivateKey != "" {
		ghAppKeyEnc, err = h.encryption.Encrypt(req.GitHubAppPrivateKey)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	// Default workspace restriction mode
	workspaceRestrictionMode := req.WorkspaceRestrictionMode
	if workspaceRestrictionMode == "" {
		workspaceRestrictionMode = "unrestricted"
	}

	// If this is set as default, unset other defaults
	if req.IsDefault {
		_, err = h.db.Exec("UPDATE scm_providers SET is_default = false WHERE is_default = true")
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	// Insert the provider
	var id int64
	err = h.db.QueryRow(`
		INSERT INTO scm_providers (
			slug, name, provider_type, auth_method, enabled, is_default,
			base_url, oauth_client_id, oauth_client_secret_encrypted,
			personal_access_token_encrypted, github_app_id,
			github_app_private_key_encrypted, github_app_installation_id, github_org_id,
			scopes, workspace_restriction_mode
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, req.Slug, req.Name, req.ProviderType, req.AuthMethod, req.Enabled, req.IsDefault,
		nullString(req.BaseURL), nullString(req.OAuthClientID), nullString(oauthSecretEnc),
		nullString(patEnc), nullString(req.GitHubAppID),
		nullString(ghAppKeyEnc), nullString(req.GitHubAppInstallationID), nullInt64(req.GitHubOrgID),
		req.Scopes, workspaceRestrictionMode).Scan(&id)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") || strings.Contains(err.Error(), "duplicate key") {
			respondConflict(w, r, "Provider with this slug already exists")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	provider, err := h.getProviderByID(int(id))
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(provider)
}

// UpdateProvider updates an existing SCM provider
func (h *SCMProviderHandler) UpdateProvider(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "providerId")
		return
	}

	var req models.SCMProviderRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Check if provider exists
	_, err = h.getProviderByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "scm_provider")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	// Validate provider type (only GitHub and Gitea supported)
	validTypes := map[models.SCMProviderType]bool{
		models.SCMProviderTypeGitHub: true,
		models.SCMProviderTypeGitea:  true,
	}
	if req.ProviderType != "" && !validTypes[req.ProviderType] {
		respondBadRequest(w, r, "Invalid provider type. Supported: github, gitea")
		return
	}

	// Validate auth method
	validMethods := map[models.SCMAuthMethod]bool{
		models.SCMAuthMethodOAuth:     true,
		models.SCMAuthMethodPAT:       true,
		models.SCMAuthMethodGitHubApp: true,
	}
	if req.AuthMethod != "" && !validMethods[req.AuthMethod] {
		respondBadRequest(w, r, "Invalid auth method")
		return
	}

	// GitHub App auth method is only valid for GitHub providers
	if req.AuthMethod == models.SCMAuthMethodGitHubApp && req.ProviderType != models.SCMProviderTypeGitHub {
		respondBadRequest(w, r, "GitHub App auth method is only valid for GitHub providers")
		return
	}

	// Encrypt secrets if provided
	var oauthSecretEnc, patEnc, ghAppKeyEnc *string

	if req.OAuthClientSecret != "" {
		var enc string
		enc, err = h.encryption.Encrypt(req.OAuthClientSecret)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		oauthSecretEnc = &enc
	}

	if req.PersonalAccessToken != "" {
		var enc string
		enc, err = h.encryption.Encrypt(req.PersonalAccessToken)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		patEnc = &enc
	}

	if req.GitHubAppPrivateKey != "" {
		var enc string
		enc, err = h.encryption.Encrypt(req.GitHubAppPrivateKey)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		ghAppKeyEnc = &enc
	}

	// Default workspace restriction mode if not provided
	workspaceRestrictionMode := req.WorkspaceRestrictionMode
	if workspaceRestrictionMode == "" {
		workspaceRestrictionMode = "unrestricted"
	}

	// If this is set as default, unset other defaults
	if req.IsDefault {
		_, err = h.db.Exec("UPDATE scm_providers SET is_default = false WHERE is_default = true AND id != ?", id)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	// Build update query dynamically
	query := `UPDATE scm_providers SET
		slug = ?, name = ?, provider_type = ?, auth_method = ?,
		enabled = ?, is_default = ?, base_url = ?, oauth_client_id = ?,
		github_app_id = ?, github_app_installation_id = ?, github_org_id = ?,
		scopes = ?, workspace_restriction_mode = ?, updated_at = CURRENT_TIMESTAMP`
	args := []interface{}{
		req.Slug, req.Name, req.ProviderType, req.AuthMethod,
		req.Enabled, req.IsDefault, nullString(req.BaseURL), nullString(req.OAuthClientID),
		nullString(req.GitHubAppID), nullString(req.GitHubAppInstallationID), nullInt64(req.GitHubOrgID),
		req.Scopes, workspaceRestrictionMode,
	}

	// Only update secrets if provided
	if oauthSecretEnc != nil {
		query += ", oauth_client_secret_encrypted = ?"
		args = append(args, *oauthSecretEnc)
	}
	if patEnc != nil {
		query += ", personal_access_token_encrypted = ?"
		args = append(args, *patEnc)
	}
	if ghAppKeyEnc != nil {
		query += ", github_app_private_key_encrypted = ?"
		args = append(args, *ghAppKeyEnc)
	}

	query += " WHERE id = ?"
	args = append(args, id)

	_, err = h.db.Exec(query, args...)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") || strings.Contains(err.Error(), "duplicate key") {
			respondConflict(w, r, "Provider with this slug already exists")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	provider, err := h.getProviderByID(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(provider)
}

// DeleteProvider deletes an SCM provider
func (h *SCMProviderHandler) DeleteProvider(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "providerId")
		return
	}

	result, err := h.db.Exec("DELETE FROM scm_providers WHERE id = ?", id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		slog.Warn("failed to get rows affected", slog.String("component", "scm"), slog.Int("provider_id", id), slog.Any("error", err))
		// Continue - the delete likely succeeded
	} else if rowsAffected == 0 {
		respondNotFound(w, r, "scm_provider")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// TestProvider tests the connection to an SCM provider
func (h *SCMProviderHandler) TestProvider(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "providerId")
		return
	}

	// Get the provider with encrypted credentials
	var p models.SCMProvider
	var baseURL, oauthClientID, oauthClientSecretEnc sql.NullString
	var patEnc, ghAppID, ghAppKeyEnc, ghAppInstallID sql.NullString
	var oauthAccessTokenEnc, oauthRefreshTokenEnc sql.NullString
	var oauthTokenExpiresAt sql.NullTime

	err = h.db.QueryRow(`
		SELECT id, slug, name, provider_type, auth_method, enabled, base_url,
			   oauth_client_id, oauth_client_secret_encrypted,
			   personal_access_token_encrypted, github_app_id,
			   github_app_private_key_encrypted, github_app_installation_id,
			   oauth_access_token_encrypted, oauth_refresh_token_encrypted,
			   oauth_token_expires_at
		FROM scm_providers WHERE id = ?
	`, id).Scan(
		&p.ID, &p.Slug, &p.Name, &p.ProviderType, &p.AuthMethod, &p.Enabled,
		&baseURL, &oauthClientID, &oauthClientSecretEnc,
		&patEnc, &ghAppID, &ghAppKeyEnc, &ghAppInstallID,
		&oauthAccessTokenEnc, &oauthRefreshTokenEnc, &oauthTokenExpiresAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "scm_provider")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	// Build provider config
	cfg := scm.ProviderConfig{
		ProviderType: p.ProviderType,
		AuthMethod:   p.AuthMethod,
		BaseURL:      baseURL.String,
	}

	// Decrypt and set credentials based on auth method
	switch p.AuthMethod {
	case models.SCMAuthMethodOAuth:
		if oauthAccessTokenEnc.Valid && oauthAccessTokenEnc.String != "" {
			var token string
			token, err = h.encryption.Decrypt(oauthAccessTokenEnc.String)
			if err != nil {
				respondInternalError(w, r, err)
				return
			}

			// Check if token needs refresh
			var expiresAt *time.Time
			if oauthTokenExpiresAt.Valid {
				expiresAt = &oauthTokenExpiresAt.Time
			}

			// Try to refresh if expired or expiring soon
			var refreshedToken string
			refreshedToken, err = h.refreshOAuthTokenIfNeeded(
				r.Context(),
				p.ID,
				p.ProviderType,
				baseURL.String,
				token,
				oauthRefreshTokenEnc.String,
				expiresAt,
				oauthClientID.String,
				oauthClientSecretEnc.String,
			)
			if err != nil {
				// Log the error but try with existing token anyway
				slog.Warn("token refresh failed, trying with existing token", slog.String("component", "scm"), slog.Int("provider_id", id), slog.Any("error", err))
				cfg.OAuthAccessToken = token
			} else {
				cfg.OAuthAccessToken = refreshedToken
			}
		} else {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "OAuth not connected. Please complete the OAuth flow first.",
			})
			return
		}
	case models.SCMAuthMethodPAT:
		if patEnc.Valid && patEnc.String != "" {
			var token string
			token, err = h.encryption.Decrypt(patEnc.String)
			if err != nil {
				respondInternalError(w, r, err)
				return
			}
			cfg.PersonalAccessToken = token
		} else {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "Personal Access Token not configured",
			})
			return
		}
	case models.SCMAuthMethodGitHubApp:
		// Check required fields
		if !ghAppID.Valid || ghAppID.String == "" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "GitHub App ID not configured",
			})
			return
		}
		if !ghAppKeyEnc.Valid || ghAppKeyEnc.String == "" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "GitHub App private key not configured",
			})
			return
		}
		if !ghAppInstallID.Valid || ghAppInstallID.String == "" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "GitHub App installation ID not configured. Use 'Discover Installations' to select an organization.",
			})
			return
		}

		// Decrypt the private key
		var privateKey string
		privateKey, err = h.encryption.Decrypt(ghAppKeyEnc.String)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		cfg.GitHubAppID = ghAppID.String
		cfg.GitHubAppPrivateKey = privateKey
		cfg.GitHubAppInstallationID = ghAppInstallID.String
	}

	// Create provider instance and test connection
	provider, err := scm.NewProvider(cfg)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	err = provider.TestConnection(ctx)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Connection successful",
	})
}

// StartOAuth initiates the OAuth flow for an SCM provider
func (h *SCMProviderHandler) StartOAuth(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	// Get provider by slug
	var providerID int
	var providerType models.SCMProviderType
	var clientID sql.NullString
	var baseURL sql.NullString
	var oauthScopes sql.NullString

	err := h.db.QueryRow(`
		SELECT id, provider_type, oauth_client_id, base_url, scopes
		FROM scm_providers WHERE slug = ?
	`, slug).Scan(&providerID, &providerType, &clientID, &baseURL, &oauthScopes)
	if err != nil {
		if err == sql.ErrNoRows {
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
	user, ok := r.Context().Value(middleware.ContextKeyUser).(*models.User)
	if !ok {
		respondUnauthorized(w, r)
		return
	}
	userID := user.ID

	// Generate state
	stateBytes := make([]byte, 32)
	_, _ = rand.Read(stateBytes)
	state := base64.URLEncoding.EncodeToString(stateBytes)

	// Determine redirect URI
	redirectURI := h.getOAuthRedirectURI(r, slug)
	slog.Debug("initiating OAuth", slog.String("component", "scm"), slog.String("slug", slug), slog.String("redirect_uri", redirectURI))

	// Store state token
	expiresAt := time.Now().Add(5 * time.Minute)
	_, err = h.db.Exec(`
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
	default:
		respondBadRequest(w, r, "OAuth not supported for this provider type")
		return
	}

	// Return the auth URL for the frontend to redirect to
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"auth_url": authURL,
	})
}

// OAuthCallback handles the OAuth callback
// Routes tokens to workspace-level storage when workspace_id is present in state
func (h *SCMProviderHandler) OAuthCallback(w http.ResponseWriter, r *http.Request) {
	slog.Debug("OAuth callback received", slog.String("component", "scm"), slog.String("remote_addr", r.RemoteAddr))
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		errorMsg := r.URL.Query().Get("error")
		if errorMsg != "" {
			h.redirectWithOAuthError(w, r, errorMsg)
		} else {
			h.redirectWithOAuthError(w, r, "Missing code or state parameter")
		}
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
		h.redirectWithOAuthError(w, r, "Invalid or expired state")
		return
	}

	// Delete used state (check error)
	if _, err = h.db.Exec("DELETE FROM scm_oauth_state WHERE state = ?", state); err != nil {
		slog.Warn("failed to delete OAuth state", slog.String("component", "scm"), slog.Any("error", err))
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
		h.redirectWithOAuthError(w, r, "Provider not found")
		return
	}

	// Decrypt client secret
	clientSecret, err := h.encryption.Decrypt(clientSecretEnc.String)
	if err != nil {
		slog.Error("failed to decrypt client secret", slog.String("component", "scm"), slog.Int("provider_id", providerID), slog.Any("error", err))
		h.redirectWithOAuthError(w, r, "Configuration error")
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
		h.redirectWithOAuthError(w, r, "Failed to exchange token")
		return
	}

	// Encrypt tokens
	encTokens, err := h.encryptOAuthTokens(tokenResult.accessToken, tokenResult.refreshToken)
	if err != nil {
		slog.Error("failed to encrypt tokens", slog.String("component", "scm"), slog.Int("provider_id", providerID), slog.Any("error", err))
		h.redirectWithOAuthError(w, r, "Failed to store token")
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
		h.redirectWithOAuthError(w, r, "Failed to store token")
		return
	}

	slog.Info("OAuth token stored successfully at user level", slog.String("component", "scm"), slog.Int("user_id", userID), slog.String("slug", providerSlug), slog.String("scm_username", userInfo.username))

	// Also store at provider level (for admin status display and TestProvider)
	if _, err := h.db.Exec(`
		UPDATE scm_providers SET
			oauth_access_token_encrypted = ?,
			oauth_refresh_token_encrypted = ?,
			oauth_token_expires_at = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, encTokens.accessToken, nullString(encTokens.refreshToken), tokenResult.expiresAt, providerID); err != nil {
		slog.Warn("failed to store provider-level OAuth token", slog.String("component", "scm"), slog.Int("provider_id", providerID), slog.Any("error", err))
		// Non-fatal: user-level token was already stored successfully
	}

	// Store at workspace connection level when initiated from workspace settings
	if workspaceID.Valid {
		if _, err := h.db.Exec(`
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

	// Redirect based on context
	if workspaceID.Valid {
		// Came from workspace settings - redirect back there
		var workspaceKey string
		if err := h.db.QueryRow("SELECT key FROM workspaces WHERE id = ?", workspaceID.Int64).Scan(&workspaceKey); err != nil {
			slog.Warn("failed to get workspace key for redirect", slog.String("component", "scm"), slog.Any("error", err))
		}
		http.Redirect(w, r, fmt.Sprintf("/workspaces/%s/settings/source-control?oauth=success&provider=%s",
			url.QueryEscape(workspaceKey), url.QueryEscape(providerSlug)), http.StatusFound)
		return
	}

	// Default: redirect to user profile connected accounts
	http.Redirect(w, r, "/profile?tab=connected-accounts&oauth=success&provider="+url.QueryEscape(providerSlug), http.StatusFound)
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

// providerRowScanResult holds scanned values from a provider database row
type providerRowScanResult struct {
	Provider                 models.SCMProvider
	BaseURL                  sql.NullString
	OAuthClientID            sql.NullString
	OAuthClientSecretEnc     sql.NullString
	PATEnc                   sql.NullString
	GHAppID                  sql.NullString
	GHAppKeyEnc              sql.NullString
	GHAppInstallID           sql.NullString
	GHOrgID                  sql.NullInt64
	OAuthAccessTokenEnc      sql.NullString
	OAuthTokenExpiresAt      sql.NullTime
	WorkspaceRestrictionMode sql.NullString
}

// scanDestinations returns the scan destinations for a provider query row
func (r *providerRowScanResult) scanDestinations() []interface{} {
	return []interface{}{
		&r.Provider.ID, &r.Provider.Slug, &r.Provider.Name, &r.Provider.ProviderType, &r.Provider.AuthMethod,
		&r.Provider.Enabled, &r.Provider.IsDefault, &r.BaseURL, &r.OAuthClientID, &r.OAuthClientSecretEnc,
		&r.PATEnc, &r.GHAppID, &r.GHAppKeyEnc, &r.GHAppInstallID, &r.GHOrgID,
		&r.OAuthAccessTokenEnc, &r.OAuthTokenExpiresAt,
		&r.Provider.Scopes, &r.WorkspaceRestrictionMode, &r.Provider.CreatedAt, &r.Provider.UpdatedAt,
	}
}

// toResponse converts scanned row data to an SCMProviderResponse
func (r *providerRowScanResult) toResponse() SCMProviderResponse {
	// Default to unrestricted if not set
	restrictionMode := "unrestricted"
	if r.WorkspaceRestrictionMode.Valid && r.WorkspaceRestrictionMode.String != "" {
		restrictionMode = r.WorkspaceRestrictionMode.String
	}

	resp := SCMProviderResponse{
		ID:                       r.Provider.ID,
		Slug:                     r.Provider.Slug,
		Name:                     r.Provider.Name,
		ProviderType:             r.Provider.ProviderType,
		AuthMethod:               r.Provider.AuthMethod,
		Enabled:                  r.Provider.Enabled,
		IsDefault:                r.Provider.IsDefault,
		BaseURL:                  r.BaseURL.String,
		OAuthClientID:            r.OAuthClientID.String,
		HasOAuthClientSecret:     r.OAuthClientSecretEnc.Valid && r.OAuthClientSecretEnc.String != "",
		HasPAT:                   r.PATEnc.Valid && r.PATEnc.String != "",
		GitHubAppID:              r.GHAppID.String,
		HasGitHubAppPrivateKey:   r.GHAppKeyEnc.Valid && r.GHAppKeyEnc.String != "",
		GitHubAppInstallationID:  r.GHAppInstallID.String,
		HasOAuthToken:            r.OAuthAccessTokenEnc.Valid && r.OAuthAccessTokenEnc.String != "",
		Scopes:                   r.Provider.Scopes,
		WorkspaceRestrictionMode: restrictionMode,
		CreatedAt:                r.Provider.CreatedAt,
		UpdatedAt:                r.Provider.UpdatedAt,
	}

	if r.OAuthTokenExpiresAt.Valid {
		resp.OAuthTokenExpiresAt = &r.OAuthTokenExpiresAt.Time
	}
	if r.GHOrgID.Valid {
		resp.GitHubOrgID = &r.GHOrgID.Int64
	}

	return resp
}

// providerListQuery is the SQL query for fetching provider list data
const providerListQuery = `
	SELECT id, slug, name, provider_type, auth_method, enabled, is_default,
		   base_url, oauth_client_id, oauth_client_secret_encrypted,
		   personal_access_token_encrypted, github_app_id,
		   github_app_private_key_encrypted, github_app_installation_id, github_org_id,
		   oauth_access_token_encrypted, oauth_token_expires_at,
		   scopes, workspace_restriction_mode, created_at, updated_at
	FROM scm_providers`

func (h *SCMProviderHandler) getProviderByID(id int) (*SCMProviderResponse, error) {
	var row providerRowScanResult
	err := h.db.QueryRow(providerListQuery+" WHERE id = ?", id).Scan(row.scanDestinations()...)
	if err != nil {
		return nil, err
	}

	resp := row.toResponse()
	return &resp, nil
}

func (h *SCMProviderHandler) getOAuthRedirectURI(r *http.Request, slug string) string {
	if h.baseURL != "" {
		return h.baseURL + "/api/scm/oauth/" + slug + "/callback"
	}

	scheme := "https"
	if r.TLS == nil {
		if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
			scheme = proto
		} else {
			scheme = "http"
		}
	}

	host := r.Host
	if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
		host = fwdHost
	}

	return fmt.Sprintf("%s://%s/api/scm/oauth/%s/callback", scheme, host, slug)
}

func (h *SCMProviderHandler) redirectWithOAuthError(w http.ResponseWriter, r *http.Request, message string) {
	http.Redirect(w, r, "/admin?tab=scm-providers&oauth=error&message="+url.QueryEscape(message), http.StatusFound)
}

func nullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func nullInt64(i *int64) interface{} {
	if i == nil {
		return nil
	}
	return *i
}

// GetProviderAllowedWorkspaces lists all workspaces allowed to use an SCM provider
func (h *SCMProviderHandler) GetProviderAllowedWorkspaces(w http.ResponseWriter, r *http.Request) {
	providerID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "providerId")
		return
	}

	// Check if provider exists
	_, err = h.getProviderByID(providerID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "scm_provider")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	rows, err := h.db.Query(`
		SELECT a.id, a.provider_id, a.workspace_id, a.created_at, a.created_by,
			   w.name as workspace_name, w.key as workspace_key
		FROM scm_provider_workspace_allowlist a
		JOIN workspaces w ON a.workspace_id = w.id
		WHERE a.provider_id = ?
		ORDER BY w.name
	`, providerID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var allowlist []models.SCMProviderWorkspaceAllowlist
	for rows.Next() {
		var entry models.SCMProviderWorkspaceAllowlist
		var createdBy sql.NullInt64
		if err := rows.Scan(&entry.ID, &entry.ProviderID, &entry.WorkspaceID,
			&entry.CreatedAt, &createdBy, &entry.WorkspaceName, &entry.WorkspaceKey); err != nil {
			slog.Error("failed to scan allowlist entry", slog.String("component", "scm"), slog.Any("error", err))
			continue
		}
		if createdBy.Valid {
			createdByInt := int(createdBy.Int64)
			entry.CreatedBy = &createdByInt
		}
		allowlist = append(allowlist, entry)
	}

	if allowlist == nil {
		allowlist = []models.SCMProviderWorkspaceAllowlist{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(allowlist)
}

// AddWorkspaceToProviderAllowlist adds a workspace to the provider's allowlist
func (h *SCMProviderHandler) AddWorkspaceToProviderAllowlist(w http.ResponseWriter, r *http.Request) {
	providerID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "providerId")
		return
	}

	var req struct {
		WorkspaceID int `json:"workspace_id"`
	}
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	if req.WorkspaceID == 0 {
		respondValidationError(w, r, "workspace_id is required")
		return
	}

	// Check if provider exists
	_, err = h.getProviderByID(providerID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "scm_provider")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	// Check if workspace exists
	var workspaceExists bool
	err = h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM workspaces WHERE id = ?)", req.WorkspaceID).Scan(&workspaceExists)
	if err != nil || !workspaceExists {
		respondNotFound(w, r, "workspace")
		return
	}

	// Get user ID from context if available
	var createdBy interface{}
	if user, ok := r.Context().Value(middleware.ContextKeyUser).(*models.User); ok && user != nil {
		createdBy = user.ID
	}

	// Insert the allowlist entry
	_, err = h.db.Exec(`
		INSERT INTO scm_provider_workspace_allowlist (provider_id, workspace_id, created_by)
		VALUES (?, ?, ?)
	`, providerID, req.WorkspaceID, createdBy)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") || strings.Contains(err.Error(), "duplicate key") {
			respondConflict(w, r, "Workspace is already in the allowlist")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// RemoveWorkspaceFromProviderAllowlist removes a workspace from the provider's allowlist
func (h *SCMProviderHandler) RemoveWorkspaceFromProviderAllowlist(w http.ResponseWriter, r *http.Request) {
	providerID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "providerId")
		return
	}

	workspaceID, err := strconv.Atoi(r.PathValue("workspace_id"))
	if err != nil {
		respondInvalidID(w, r, "workspaceId")
		return
	}

	// Check if provider exists
	_, err = h.getProviderByID(providerID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "scm_provider")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	result, err := h.db.Exec(`
		DELETE FROM scm_provider_workspace_allowlist
		WHERE provider_id = ? AND workspace_id = ?
	`, providerID, workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondNotFound(w, r, "allowlist_entry")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// UpdateProviderAllowedWorkspaces replaces the entire allowlist for a provider
func (h *SCMProviderHandler) UpdateProviderAllowedWorkspaces(w http.ResponseWriter, r *http.Request) {
	providerID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "providerId")
		return
	}

	var req struct {
		WorkspaceIDs []int `json:"workspace_ids"`
	}
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	// Check if provider exists
	_, err = h.getProviderByID(providerID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "scm_provider")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	// Get user ID from context if available
	var createdBy interface{}
	if user, ok := r.Context().Value(middleware.ContextKeyUser).(*models.User); ok && user != nil {
		createdBy = user.ID
	}

	// Start a transaction to replace the entire allowlist
	tx, err := h.db.Begin()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Delete all existing entries for this provider
	_, err = tx.Exec("DELETE FROM scm_provider_workspace_allowlist WHERE provider_id = ?", providerID)
	if err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			slog.Error("failed to rollback transaction", slog.String("component", "scm"), slog.Any("error", rbErr))
		}
		respondInternalError(w, r, err)
		return
	}

	// Insert new entries
	for _, workspaceID := range req.WorkspaceIDs {
		_, err = tx.Exec(`
			INSERT INTO scm_provider_workspace_allowlist (provider_id, workspace_id, created_by)
			VALUES (?, ?, ?)
		`, providerID, workspaceID, createdBy)
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				slog.Error("failed to rollback transaction", slog.String("component", "scm"), slog.Any("error", rbErr))
			}
			respondInternalError(w, r, err)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Return the updated allowlist
	h.GetProviderAllowedWorkspaces(w, r)
}

// IsWorkspaceAllowedForProvider checks if a workspace is allowed to use an SCM provider
// This is a helper method used by other handlers for enforcement
func (h *SCMProviderHandler) IsWorkspaceAllowedForProvider(providerID, workspaceID int) (bool, error) {
	provider, err := h.getProviderByID(providerID)
	if err != nil {
		return false, err
	}

	// If unrestricted, all workspaces are allowed
	if provider.WorkspaceRestrictionMode == "unrestricted" {
		return true, nil
	}

	// Check if workspace is in the allowlist
	var exists bool
	err = h.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM scm_provider_workspace_allowlist
			WHERE provider_id = ? AND workspace_id = ?
		)
	`, providerID, workspaceID).Scan(&exists)
	if err != nil {
		return false, err
	}

	return exists, nil
}

// GitHubAppInstallation represents a GitHub App installation for discovery
type GitHubAppInstallation struct {
	ID               int64  `json:"id"`
	AccountLogin     string `json:"account_login"`
	AccountType      string `json:"account_type"`
	AccountID        int64  `json:"account_id"`
	AccountAvatarURL string `json:"account_avatar_url,omitempty"`
}

// DiscoverGitHubAppInstallationsRequest represents request for discovering installations
type DiscoverGitHubAppInstallationsRequest struct {
	AppID      string `json:"app_id"`
	PrivateKey string `json:"private_key"`
}

// DiscoverGitHubAppInstallations discovers GitHub App installations for configuration
// POST /api/scm-providers/github-app/discover-installations
func (h *SCMProviderHandler) DiscoverGitHubAppInstallations(w http.ResponseWriter, r *http.Request) {
	var req DiscoverGitHubAppInstallationsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	if req.AppID == "" || req.PrivateKey == "" {
		respondValidationError(w, r, "app_id and private_key are required")
		return
	}

	// Create GitHub provider with App credentials for discovery
	cfg := scm.ProviderConfig{
		ProviderType:        models.SCMProviderTypeGitHub,
		AuthMethod:          models.SCMAuthMethodGitHubApp,
		GitHubAppID:         req.AppID,
		GitHubAppPrivateKey: req.PrivateKey,
	}

	provider, err := scm.NewGitHubProvider(cfg)
	if err != nil {
		slog.Error("failed to create GitHub provider for discovery", slog.String("component", "scm"), slog.Any("error", err))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success":       false,
			"error":         "Failed to initialize GitHub App: " + err.Error(),
			"installations": []interface{}{},
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	installations, err := provider.ListAppInstallations(ctx)
	if err != nil {
		slog.Error("failed to discover GitHub App installations", slog.String("component", "scm"), slog.Any("error", err))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success":       false,
			"error":         "Failed to list installations: " + err.Error(),
			"installations": []interface{}{},
		})
		return
	}

	// Convert to response format
	result := make([]GitHubAppInstallation, 0, len(installations))
	for _, inst := range installations {
		result = append(result, GitHubAppInstallation{
			ID:               inst.ID,
			AccountLogin:     inst.AccountLogin,
			AccountType:      inst.AccountType,
			AccountID:        inst.AccountID,
			AccountAvatarURL: inst.AccountAvatarURL,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":       true,
		"installations": result,
	})
}

// RefreshGitHubAppInstallation refreshes the installation_id for a provider using org_id
// POST /api/scm-providers/{id}/github-app/refresh-installation
func (h *SCMProviderHandler) RefreshGitHubAppInstallation(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		respondInvalidID(w, r, "providerId")
		return
	}

	// Get provider details
	var authMethod models.SCMAuthMethod
	var ghAppID, ghAppKeyEnc sql.NullString
	var ghOrgID sql.NullInt64

	err = h.db.QueryRow(`
		SELECT auth_method, github_app_id, github_app_private_key_encrypted, github_org_id
		FROM scm_providers WHERE id = ?
	`, id).Scan(&authMethod, &ghAppID, &ghAppKeyEnc, &ghOrgID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "scm_provider")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	if authMethod != models.SCMAuthMethodGitHubApp {
		respondBadRequest(w, r, "Provider does not use GitHub App authentication")
		return
	}

	if !ghAppID.Valid || !ghAppKeyEnc.Valid || !ghOrgID.Valid {
		respondBadRequest(w, r, "GitHub App not fully configured (missing app_id, private_key, or org_id)")
		return
	}

	// Decrypt private key
	privateKey, err := h.encryption.Decrypt(ghAppKeyEnc.String)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Create provider and find installation for org
	cfg := scm.ProviderConfig{
		ProviderType:        models.SCMProviderTypeGitHub,
		AuthMethod:          models.SCMAuthMethodGitHubApp,
		GitHubAppID:         ghAppID.String,
		GitHubAppPrivateKey: privateKey,
	}

	provider, err := scm.NewGitHubProvider(cfg)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to initialize GitHub App: %w", err))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	installations, err := provider.ListAppInstallations(ctx)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("failed to list installations: %w", err))
		return
	}

	// Find installation matching our org_id
	var foundInstallation *scm.GitHubAppInstallation
	for i := range installations {
		if installations[i].AccountID == ghOrgID.Int64 {
			foundInstallation = &installations[i]
			break
		}
	}

	if foundInstallation == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "App is no longer installed for this organization",
		})
		return
	}

	// Update installation_id
	_, err = h.db.Exec(`
		UPDATE scm_providers SET
			github_app_installation_id = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, fmt.Sprintf("%d", foundInstallation.ID), id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":         true,
		"installation_id": foundInstallation.ID,
		"account_login":   foundInstallation.AccountLogin,
	})
}
