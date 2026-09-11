package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/integrations"
	"windshift/internal/integrations/notion"
	"windshift/internal/integrations/todoist"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sso"

	"uuid"
)

// IntegrationOAuthHandler handles OAuth flows and user connection management
type IntegrationOAuthHandler struct {
	db          database.Database
	encryption  *sso.SecretEncryption
	baseURL     string
	systemFlows map[models.IntegrationProviderType]systemOAuthRegistration
}

type systemOAuthRegistration struct {
	flow    integrations.SystemOAuthFlow
	auditor *logger.Auditor
}

// NewIntegrationOAuthHandler creates a new integration OAuth handler.
// baseURL: public URL of the application (from config.Load).
func NewIntegrationOAuthHandler(db database.Database, encryption *sso.SecretEncryption, baseURL string) *IntegrationOAuthHandler {
	return &IntegrationOAuthHandler{
		db:          db,
		encryption:  encryption,
		baseURL:     baseURL,
		systemFlows: make(map[models.IntegrationProviderType]systemOAuthRegistration),
	}
}

// RegisterSystemOAuthFlow adds a system-owned provider extension to the same
// OAuth handler used by user-owned integrations.
func (h *IntegrationOAuthHandler) RegisterSystemOAuthFlow(providerType models.IntegrationProviderType, flow integrations.SystemOAuthFlow, auditor *logger.Auditor) {
	capabilities, ok := integrations.Capabilities(providerType)
	if !ok || !capabilities.OAuth || capabilities.CredentialOwner != integrations.CredentialOwnerSystem || flow == nil {
		return
	}
	h.systemFlows[providerType] = systemOAuthRegistration{flow: flow, auditor: auditor}
}

// StartSystemOAuth initiates OAuth for an administrator-managed provider row.
func (h *IntegrationOAuthHandler) StartSystemOAuth(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	providerID := r.PathValue("id")
	var providerType models.IntegrationProviderType
	if err := h.db.QueryRow("SELECT provider_type FROM integration_providers WHERE id = ? AND enabled = true", providerID).Scan(&providerType); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondNotFound(w, r, "integration_provider")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}
	registration, registered := h.systemFlows[providerType]
	if !registered || registration.flow == nil {
		respondBadRequest(w, r, "OAuth is not supported for this provider")
		return
	}
	authURL, err := registration.flow.StartOAuth(r.Context(), providerID, user.ID, h.baseURL)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "integration_provider")
		} else {
			respondBadRequest(w, r, "OAuth is not configured for this provider")
		}
		return
	}
	if registration.auditor != nil {
		registration.auditor.LogWithDetails(r, user, logger.ActionIntegrationProviderUpdate, logger.ResourceIntegrationProvider, nil, providerID, map[string]any{
			"provider_id": providerID, "provider_type": providerType, "oauth_started": true,
		})
	}
	respondJSONOK(w, map[string]string{"auth_url": authURL})
}

// StartOAuth initiates the OAuth flow for an integration provider
func (h *IntegrationOAuthHandler) StartOAuth(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Get provider
	var providerID string
	var providerType models.IntegrationProviderType
	var clientID sql.NullString

	err := h.db.QueryRow(`
		SELECT id, provider_type, oauth_client_id
		FROM integration_providers WHERE slug = ? AND enabled = true
	`, slug).Scan(&providerID, &providerType, &clientID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondNotFound(w, r, "integration_provider")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}
	if !integrations.SupportsUserOAuth(providerType) {
		respondNotFound(w, r, "integration_provider")
		return
	}

	if !clientID.Valid || clientID.String == "" {
		respondBadRequest(w, r, "OAuth not configured for this provider")
		return
	}

	// Generate state
	stateBytes := make([]byte, 32)
	_, _ = rand.Read(stateBytes)
	state := base64.URLEncoding.EncodeToString(stateBytes)

	redirectURI, err := h.getRedirectURI(slug)
	if err != nil {
		slog.Error("integration OAuth misconfigured", slog.String("component", "integrations"), slog.Any("error", err))
		respondServiceUnavailable(w, r, "OAuth is not configured")
		return
	}

	// Store state
	expiresAt := time.Now().Add(5 * time.Minute)
	_, err = h.db.ExecWrite(`
		INSERT INTO integration_oauth_state (id, provider_id, state, user_id, expires_at)
		VALUES (?, ?, ?, ?, ?)
	`, uuid.New().String(), providerID, state, fmt.Sprintf("%d", user.ID), expiresAt)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Build auth URL based on provider type
	var authURL string
	switch providerType {
	case models.IntegrationProviderNotion:
		authURL = fmt.Sprintf(
			"https://api.notion.com/v1/oauth/authorize?client_id=%s&response_type=code&state=%s&redirect_uri=%s&owner=user",
			clientID.String,
			state,
			url.QueryEscape(redirectURI),
		)
	case models.IntegrationProviderTodoist:
		authURL = todoist.AuthorizeURL(clientID.String, "data:read_write", state)
	default:
		respondBadRequest(w, r, "OAuth not supported for this provider type")
		return
	}

	respondJSONOK(w, map[string]string{
		"auth_url": authURL,
	})
}

// OAuthCallback handles the OAuth callback from the integration provider
func (h *IntegrationOAuthHandler) OAuthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		errorMsg := r.URL.Query().Get("error")
		if errorMsg == "" {
			errorMsg = "Missing code or state parameter"
		}
		h.redirectWithError(w, r, errorMsg)
		return
	}

	// Validate state
	var providerID, userID string
	err := h.db.QueryRow(`
		SELECT provider_id, user_id FROM integration_oauth_state
		WHERE state = ? AND expires_at > CURRENT_TIMESTAMP
	`, state).Scan(&providerID, &userID)
	if err != nil {
		slog.Warn("invalid integration OAuth state", slog.String("component", "integrations"), slog.Any("error", err))
		h.redirectWithError(w, r, "Invalid or expired state")
		return
	}

	// Delete used state
	_, _ = h.db.ExecWrite("DELETE FROM integration_oauth_state WHERE state = ?", state)

	// Get provider details
	var providerType models.IntegrationProviderType
	var clientIDVal, clientSecretEnc sql.NullString
	var providerSlug string

	err = h.db.QueryRow(`
		SELECT provider_type, oauth_client_id, oauth_client_secret_encrypted, slug
		FROM integration_providers WHERE id = ?
	`, providerID).Scan(&providerType, &clientIDVal, &clientSecretEnc, &providerSlug)
	if err != nil {
		slog.Error("failed to get integration provider", slog.String("component", "integrations"), slog.Any("error", err))
		h.redirectWithError(w, r, "Provider not found")
		return
	}
	if !integrations.SupportsUserOAuth(providerType) {
		h.redirectWithError(w, r, "Unsupported provider type")
		return
	}

	if !clientSecretEnc.Valid || clientSecretEnc.String == "" {
		h.redirectWithError(w, r, "OAuth secret not configured")
		return
	}

	// Decrypt client secret
	clientSecret, err := h.encryption.Decrypt(clientSecretEnc.String)
	if err != nil {
		slog.Error("failed to decrypt integration client secret", slog.String("component", "integrations"), slog.Any("error", err))
		h.redirectWithError(w, r, "Configuration error")
		return
	}

	redirectURI, err := h.getRedirectURI(providerSlug)
	if err != nil {
		slog.Error("integration OAuth misconfigured", slog.String("component", "integrations"), slog.Any("error", err))
		h.redirectWithError(w, r, "OAuth is not configured")
		return
	}

	// Exchange code based on provider type
	switch providerType {
	case models.IntegrationProviderNotion:
		h.handleNotionCallback(w, r, clientIDVal.String, clientSecret, code, redirectURI, providerID, userID, providerSlug)
	case models.IntegrationProviderTodoist:
		h.handleTodoistCallback(w, r, clientIDVal.String, clientSecret, code, providerID, userID, providerSlug)
	default:
		h.redirectWithError(w, r, "Unsupported provider type")
	}
}

// SystemOAuthCallback completes an administrator-managed provider flow on a
// path that cannot collide with user-configurable provider slugs.
func (h *IntegrationOAuthHandler) SystemOAuthCallback(w http.ResponseWriter, r *http.Request) {
	providerType := models.IntegrationProviderType(r.PathValue("providerType"))
	registration, ok := h.systemFlows[providerType]
	if !ok || registration.flow == nil {
		respondNotFound(w, r, "integration_provider")
		return
	}
	h.handleSystemOAuthCallback(w, r, providerType, registration)
}

func (h *IntegrationOAuthHandler) handleSystemOAuthCallback(w http.ResponseWriter, r *http.Request, providerType models.IntegrationProviderType, registration systemOAuthRegistration) {
	state, code := r.URL.Query().Get("state"), r.URL.Query().Get("code")
	var result *integrations.SystemOAuthCallbackResult
	var err error
	if state == "" || code == "" || r.URL.Query().Get("error") != "" {
		if state != "" {
			result, _ = registration.flow.ConsumeFailedOAuthCallback(state)
		}
		err = errors.New("OAuth callback was rejected")
	} else {
		result, err = registration.flow.CompleteOAuth(r.Context(), state, code, h.baseURL)
	}
	h.auditSystemOAuthCallback(r, providerType, registration.auditor, result, err == nil)
	h.redirectSystemOAuthResult(w, r, providerType, err == nil)
}

func (h *IntegrationOAuthHandler) auditSystemOAuthCallback(r *http.Request, providerType models.IntegrationProviderType, auditor *logger.Auditor, result *integrations.SystemOAuthCallbackResult, success bool) {
	if auditor == nil || result == nil || result.Initiator == nil {
		return
	}
	details := map[string]any{
		"provider_id": result.ProviderID, "provider_type": providerType,
		"oauth_callback": true, "credential_mutation": true, "generation": result.Generation,
	}
	if success {
		auditor.LogWithDetails(r, result.Initiator, logger.ActionIntegrationProviderOAuthCredentialSet, logger.ResourceIntegrationProvider, nil, result.ProviderName, details)
		return
	}
	auditor.LogFailure(r, result.Initiator, logger.ActionIntegrationProviderOAuthCredentialSet, logger.ResourceIntegrationProvider, nil, result.ProviderName, "oauth_callback_failed", details)
}

func (h *IntegrationOAuthHandler) redirectSystemOAuthResult(w http.ResponseWriter, r *http.Request, providerType models.IntegrationProviderType, success bool) {
	result := "error"
	if success {
		result = "success"
	}
	contextPath := ""
	if publicURL, err := url.Parse(h.baseURL); err == nil {
		contextPath = strings.TrimRight(publicURL.EscapedPath(), "/")
	}
	http.Redirect(w, r, contextPath+"/admin/integration-providers?tab="+url.QueryEscape(string(providerType))+"&oauth="+url.QueryEscape(result), http.StatusFound)
}

func (h *IntegrationOAuthHandler) handleTodoistCallback(w http.ResponseWriter, r *http.Request, clientID, clientSecret, code, providerID, userID, providerSlug string) {
	result, err := todoist.ExchangeOAuthCode(clientID, clientSecret, code)
	if err != nil {
		slog.Error("failed to exchange Todoist OAuth code", slog.String("component", "integrations"), slog.Any("error", err))
		h.redirectWithError(w, r, "Failed to exchange token")
		return
	}

	encToken, err := h.encryption.Encrypt(result.AccessToken)
	if err != nil {
		slog.Error("failed to encrypt integration token", slog.String("component", "integrations"), slog.Any("error", err))
		h.redirectWithError(w, r, "Failed to store token")
		return
	}

	_, err = h.db.ExecWrite(`
		INSERT INTO user_integration_tokens (id, user_id, integration_provider_id, oauth_access_token_encrypted, provider_metadata, connected_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT (user_id, integration_provider_id) DO UPDATE SET
			oauth_access_token_encrypted = excluded.oauth_access_token_encrypted,
			provider_metadata = excluded.provider_metadata,
			connected_at = CURRENT_TIMESTAMP,
			updated_at = CURRENT_TIMESTAMP
	`, uuid.New().String(), userID, providerID, encToken, "{}")
	if err != nil {
		slog.Error("failed to store integration token", slog.String("component", "integrations"), slog.Any("error", err))
		h.redirectWithError(w, r, "Failed to store token")
		return
	}

	slog.Info("integration OAuth completed", slog.String("component", "integrations"), slog.String("provider", providerSlug), slog.String("user_id", userID))

	http.Redirect(w, r, "/profile?tab=connected-accounts&oauth=success&provider="+url.QueryEscape(providerSlug), http.StatusFound)
}

func (h *IntegrationOAuthHandler) handleNotionCallback(w http.ResponseWriter, r *http.Request, clientID, clientSecret, code, redirectURI, providerID, userID, providerSlug string) {
	result, err := notion.ExchangeOAuthCode(clientID, clientSecret, code, redirectURI)
	if err != nil {
		slog.Error("failed to exchange Notion OAuth code", slog.String("component", "integrations"), slog.Any("error", err))
		h.redirectWithError(w, r, "Failed to exchange token")
		return
	}

	// Encrypt access token
	encToken, err := h.encryption.Encrypt(result.AccessToken)
	if err != nil {
		slog.Error("failed to encrypt integration token", slog.String("component", "integrations"), slog.Any("error", err))
		h.redirectWithError(w, r, "Failed to store token")
		return
	}

	// Build provider metadata
	metadata := map[string]string{
		"workspace_id":   result.WorkspaceID,
		"workspace_name": result.WorkspaceName,
		"workspace_icon": result.WorkspaceIcon,
		"bot_id":         result.BotID,
	}
	metadataJSON, _ := json.Marshal(metadata)

	// Upsert user token
	_, err = h.db.ExecWrite(`
		INSERT INTO user_integration_tokens (id, user_id, integration_provider_id, oauth_access_token_encrypted, provider_metadata, connected_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT (user_id, integration_provider_id) DO UPDATE SET
			oauth_access_token_encrypted = excluded.oauth_access_token_encrypted,
			provider_metadata = excluded.provider_metadata,
			connected_at = CURRENT_TIMESTAMP,
			updated_at = CURRENT_TIMESTAMP
	`, uuid.New().String(), userID, providerID, encToken, string(metadataJSON))
	if err != nil {
		slog.Error("failed to store integration token", slog.String("component", "integrations"), slog.Any("error", err))
		h.redirectWithError(w, r, "Failed to store token")
		return
	}

	slog.Info("integration OAuth completed", slog.String("component", "integrations"), slog.String("provider", providerSlug), slog.String("user_id", userID))

	// Redirect to connected accounts
	http.Redirect(w, r, "/profile?tab=connected-accounts&oauth=success&provider="+url.QueryEscape(providerSlug), http.StatusFound)
}

// GetUserConnections returns the current user's integration connections
func (h *IntegrationOAuthHandler) GetUserConnections(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	rows, err := h.db.Query(`
		SELECT
			uit.id, uit.user_id, uit.integration_provider_id,
			uit.provider_metadata, uit.connected_at,
			uit.created_at, uit.updated_at,
			ip.name AS provider_name, ip.provider_type, ip.slug AS provider_slug
		FROM user_integration_tokens uit
		JOIN integration_providers ip ON ip.id = uit.integration_provider_id
		WHERE uit.user_id = ?
		ORDER BY uit.connected_at DESC
	`, fmt.Sprintf("%d", user.ID))
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	connections := []models.UserIntegrationToken{}
	for rows.Next() {
		var conn models.UserIntegrationToken
		var metadata sql.NullString
		err := rows.Scan(
			&conn.ID, &conn.UserID, &conn.IntegrationProviderID,
			&metadata, &conn.ConnectedAt,
			&conn.CreatedAt, &conn.UpdatedAt,
			&conn.ProviderName, &conn.ProviderType, &conn.ProviderSlug,
		)
		if err != nil {
			slog.Error("failed to scan integration connection", slog.String("component", "integrations"), slog.Any("error", err))
			continue
		}
		if metadata.Valid {
			conn.ProviderMetadata = metadata.String
		}
		connections = append(connections, conn)
	}
	if err := rows.Err(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, connections)
}

// DisconnectProvider removes a user's integration connection
func (h *IntegrationOAuthHandler) DisconnectProvider(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	providerID := r.PathValue("provider_id")
	if providerID == "" {
		respondBadRequest(w, r, "Missing provider ID")
		return
	}

	result, err := h.db.ExecWrite(`
		DELETE FROM user_integration_tokens
		WHERE user_id = ? AND integration_provider_id = ?
	`, fmt.Sprintf("%d", user.ID), providerID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		respondNotFound(w, r, "integration_connection")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GetAvailableProviders returns enabled integration providers for the connect flow
func (h *IntegrationOAuthHandler) GetAvailableProviders(w http.ResponseWriter, r *http.Request) {
	_, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	rows, err := h.db.Query(`
		SELECT id, slug, name, provider_type
		FROM integration_providers
		WHERE enabled = true
		ORDER BY name
	`)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	type AvailableProvider struct {
		ID           string `json:"id"`
		Slug         string `json:"slug"`
		Name         string `json:"name"`
		ProviderType string `json:"provider_type"`
	}

	providers := []AvailableProvider{}
	for rows.Next() {
		var p AvailableProvider
		if err := rows.Scan(&p.ID, &p.Slug, &p.Name, &p.ProviderType); err != nil {
			continue
		}
		if integrations.SupportsUserOAuth(models.IntegrationProviderType(p.ProviderType)) {
			providers = append(providers, p)
		}
	}
	if err := rows.Err(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, providers)
}

// Helper methods

// getRedirectURI uses configured baseURL only, never spoofable request headers.
// Missing baseURL is a 503-worthy OAuth misconfiguration.
func (h *IntegrationOAuthHandler) getRedirectURI(slug string) (string, error) {
	if h.baseURL == "" {
		return "", fmt.Errorf("integration OAuth is not configured: baseURL is unset")
	}
	return h.baseURL + "/api/integrations/oauth/" + slug + "/callback", nil
}

func (h *IntegrationOAuthHandler) redirectWithError(w http.ResponseWriter, r *http.Request, message string) {
	http.Redirect(w, r, "/profile?tab=connected-accounts&oauth=error&message="+url.QueryEscape(message), http.StatusFound)
}
