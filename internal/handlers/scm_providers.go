package handlers

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/scm"
	"windshift/internal/sso"
	"windshift/internal/utils"
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

// NewSCMProviderHandler creates a new SCM provider handler.
// sessionSecret: session-signing secret (resolved upstream by config.Load).
// baseURL: public URL of the application.
func NewSCMProviderHandler(db database.Database, sessionSecret, baseURL string) *SCMProviderHandler {
	if sessionSecret == "" {
		slog.Error("NewSCMProviderHandler received empty session secret (config wiring bug)", slog.String("component", "scm"))
		panic("config: empty session secret passed to NewSCMProviderHandler")
	}
	return &SCMProviderHandler{
		db:         db,
		encryption: sso.NewSecretEncryption(sessionSecret),
		baseURL:    baseURL,
	}
}

// GetEncryption returns the encryption service for use by other handlers
func (h *SCMProviderHandler) GetEncryption() *sso.SecretEncryption {
	return h.encryption
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

	respondJSONOK(w, providers)
}

// GetProvider returns a single SCM provider
func (h *SCMProviderHandler) GetProvider(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
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

	respondJSONOK(w, provider)
}

// CreateProvider creates a new SCM provider
func (h *SCMProviderHandler) CreateProvider(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[models.SCMProviderRequest](w, r)
	if !ok {
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
		if database.IsUniqueConstraintError(err) {
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

	if user := utils.GetCurrentUser(r); user != nil {
		providerID := int(id)
		logAudit(h.db, r, user, logger.ActionSCMProviderCreate, logger.ResourceSCMProvider, &providerID, provider.Name)
	}
	respondJSONCreated(w, provider)
}

// UpdateProvider updates an existing SCM provider
func (h *SCMProviderHandler) UpdateProvider(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var err error

	req, ok := decodeJSON[models.SCMProviderRequest](w, r)
	if !ok {
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
		if database.IsUniqueConstraintError(err) {
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

	if user := utils.GetCurrentUser(r); user != nil {
		logAudit(h.db, r, user, logger.ActionSCMProviderUpdate, logger.ResourceSCMProvider, &id, provider.Name)
	}
	respondJSONOK(w, provider)
}

// DeleteProvider deletes an SCM provider
func (h *SCMProviderHandler) DeleteProvider(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
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

	if user := utils.GetCurrentUser(r); user != nil {
		logAudit(h.db, r, user, logger.ActionSCMProviderDelete, logger.ResourceSCMProvider, &id, "")
	}
	w.WriteHeader(http.StatusNoContent)
}

// TestProvider tests the connection to an SCM provider
func (h *SCMProviderHandler) TestProvider(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	var err error

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
			respondJSONOK(w, map[string]interface{}{
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
			respondJSONOK(w, map[string]interface{}{
				"success": false,
				"error":   "Personal Access Token not configured",
			})
			return
		}
	case models.SCMAuthMethodGitHubApp:
		// Check required fields
		if !ghAppID.Valid || ghAppID.String == "" {
			respondJSONOK(w, map[string]interface{}{
				"success": false,
				"error":   "GitHub App ID not configured",
			})
			return
		}
		if !ghAppKeyEnc.Valid || ghAppKeyEnc.String == "" {
			respondJSONOK(w, map[string]interface{}{
				"success": false,
				"error":   "GitHub App private key not configured",
			})
			return
		}
		if !ghAppInstallID.Valid || ghAppInstallID.String == "" {
			respondJSONOK(w, map[string]interface{}{
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
		respondJSONOK(w, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	err = provider.TestConnection(ctx)
	if err != nil {
		respondJSONOK(w, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	respondJSONOK(w, map[string]interface{}{
		"success": true,
		"message": "Connection successful",
	})
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
