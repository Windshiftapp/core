package handlers

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/email"
	"windshift/internal/models"
)

// EmailProviderHandler handles email provider API endpoints
type EmailProviderHandler struct {
	db         database.Database
	encryption email.Encryptor
	baseURL    string // Base URL for OAuth callbacks
}

// NewEmailProviderHandler creates a new email provider handler
func NewEmailProviderHandler(db database.Database, encryption email.Encryptor, baseURL string) *EmailProviderHandler {
	return &EmailProviderHandler{
		db:         db,
		encryption: encryption,
		baseURL:    baseURL,
	}
}

// GetEmailProviders returns all email providers
func (h *EmailProviderHandler) GetEmailProviders(w http.ResponseWriter, r *http.Request) {
	// Check admin permission
	userID := r.Context().Value("user_id")
	if userID == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	rows, err := h.db.Query(`
		SELECT id, name, slug, type, is_enabled,
		       oauth_client_id, oauth_scopes, oauth_tenant_id,
		       imap_host, imap_port, imap_encryption,
		       created_at, updated_at
		FROM email_providers
		ORDER BY name ASC
	`)
	if err != nil {
		http.Error(w, "Failed to query providers", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var providers []models.EmailProvider
	for rows.Next() {
		var p models.EmailProvider
		err := rows.Scan(
			&p.ID, &p.Name, &p.Slug, &p.Type, &p.IsEnabled,
			&p.OAuthClientID, &p.OAuthScopes, &p.OAuthTenantID,
			&p.IMAPHost, &p.IMAPPort, &p.IMAPEncryption,
			&p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil {
			continue
		}
		providers = append(providers, p)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(providers)
}

// GetEmailProvider returns a single email provider
func (h *EmailProviderHandler) GetEmailProvider(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid provider ID", http.StatusBadRequest)
		return
	}

	var p models.EmailProvider
	err = h.db.QueryRow(`
		SELECT id, name, slug, type, is_enabled,
		       oauth_client_id, oauth_scopes, oauth_tenant_id,
		       imap_host, imap_port, imap_encryption,
		       created_at, updated_at
		FROM email_providers WHERE id = ?
	`, id).Scan(
		&p.ID, &p.Name, &p.Slug, &p.Type, &p.IsEnabled,
		&p.OAuthClientID, &p.OAuthScopes, &p.OAuthTenantID,
		&p.IMAPHost, &p.IMAPPort, &p.IMAPEncryption,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		http.Error(w, "Provider not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Failed to get provider", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(p)
}

// CreateEmailProviderRequest represents the request body for creating a provider
type CreateEmailProviderRequest struct {
	Name       string `json:"name"`
	Slug       string `json:"slug"`
	Type       string `json:"type"` // microsoft, google, generic
	IsEnabled  bool   `json:"is_enabled"`

	// OAuth fields
	OAuthClientID     string `json:"oauth_client_id,omitempty"`
	OAuthClientSecret string `json:"oauth_client_secret,omitempty"`
	OAuthScopes       string `json:"oauth_scopes,omitempty"`
	OAuthTenantID     string `json:"oauth_tenant_id,omitempty"`

	// Generic IMAP fields
	IMAPHost       string `json:"imap_host,omitempty"`
	IMAPPort       int    `json:"imap_port,omitempty"`
	IMAPEncryption string `json:"imap_encryption,omitempty"`
}

// CreateEmailProvider creates a new email provider
func (h *EmailProviderHandler) CreateEmailProvider(w http.ResponseWriter, r *http.Request) {
	var req CreateEmailProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Name == "" || req.Slug == "" || req.Type == "" {
		http.Error(w, "Name, slug, and type are required", http.StatusBadRequest)
		return
	}

	// Validate type
	if req.Type != models.EmailProviderTypeMicrosoft &&
		req.Type != models.EmailProviderTypeGoogle &&
		req.Type != models.EmailProviderTypeGeneric {
		http.Error(w, "Invalid provider type", http.StatusBadRequest)
		return
	}

	// Encrypt client secret if provided
	var clientSecretEnc *string
	if req.OAuthClientSecret != "" && h.encryption != nil {
		encrypted, err := h.encryption.Encrypt(req.OAuthClientSecret)
		if err != nil {
			http.Error(w, "Failed to encrypt client secret", http.StatusInternalServerError)
			return
		}
		clientSecretEnc = &encrypted
	}

	// Default port for generic
	if req.Type == models.EmailProviderTypeGeneric && req.IMAPPort == 0 {
		req.IMAPPort = 993
	}

	// Insert provider
	result, err := h.db.Exec(`
		INSERT INTO email_providers (
			name, slug, type, is_enabled,
			oauth_client_id, oauth_client_secret_encrypted, oauth_scopes, oauth_tenant_id,
			imap_host, imap_port, imap_encryption,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`,
		req.Name, req.Slug, req.Type, req.IsEnabled,
		nullString(req.OAuthClientID), clientSecretEnc, nullString(req.OAuthScopes), nullString(req.OAuthTenantID),
		nullString(req.IMAPHost), req.IMAPPort, nullString(req.IMAPEncryption),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			http.Error(w, "A provider with this slug already exists", http.StatusConflict)
			return
		}
		http.Error(w, "Failed to create provider", http.StatusInternalServerError)
		return
	}

	id, _ := result.LastInsertId()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":   id,
		"slug": req.Slug,
	})
}

// UpdateEmailProvider updates an email provider
func (h *EmailProviderHandler) UpdateEmailProvider(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid provider ID", http.StatusBadRequest)
		return
	}

	var req CreateEmailProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Encrypt client secret if provided
	var clientSecretEnc *string
	if req.OAuthClientSecret != "" && h.encryption != nil {
		encrypted, err := h.encryption.Encrypt(req.OAuthClientSecret)
		if err != nil {
			http.Error(w, "Failed to encrypt client secret", http.StatusInternalServerError)
			return
		}
		clientSecretEnc = &encrypted
	}

	// Build update query dynamically
	query := `UPDATE email_providers SET name = ?, slug = ?, type = ?, is_enabled = ?,
	          oauth_client_id = ?, oauth_scopes = ?, oauth_tenant_id = ?,
	          imap_host = ?, imap_port = ?, imap_encryption = ?,
	          updated_at = CURRENT_TIMESTAMP`
	args := []interface{}{
		req.Name, req.Slug, req.Type, req.IsEnabled,
		nullString(req.OAuthClientID), nullString(req.OAuthScopes), nullString(req.OAuthTenantID),
		nullString(req.IMAPHost), req.IMAPPort, nullString(req.IMAPEncryption),
	}

	// Only update client secret if provided
	if clientSecretEnc != nil {
		query += `, oauth_client_secret_encrypted = ?`
		args = append(args, clientSecretEnc)
	}

	query += ` WHERE id = ?`
	args = append(args, id)

	_, err = h.db.Exec(query, args...)
	if err != nil {
		http.Error(w, "Failed to update provider", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "updated"})
}

// DeleteEmailProvider deletes an email provider
func (h *EmailProviderHandler) DeleteEmailProvider(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid provider ID", http.StatusBadRequest)
		return
	}

	_, err = h.db.Exec(`DELETE FROM email_providers WHERE id = ?`, id)
	if err != nil {
		http.Error(w, "Failed to delete provider", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// StartEmailOAuth initiates the OAuth flow for an email channel
func (h *EmailProviderHandler) StartEmailOAuth(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if slug == "" {
		http.Error(w, "Provider slug required", http.StatusBadRequest)
		return
	}

	// Get channel ID from query params
	channelIDStr := r.URL.Query().Get("channel_id")
	channelID, err := strconv.Atoi(channelIDStr)
	if err != nil {
		http.Error(w, "channel_id query parameter required", http.StatusBadRequest)
		return
	}

	// Get user ID
	userID := r.Context().Value("user_id")
	if userID == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get provider
	var provider models.EmailProvider
	var clientSecretEnc *string
	err = h.db.QueryRow(`
		SELECT id, name, slug, type, oauth_client_id, oauth_client_secret_encrypted, oauth_scopes, oauth_tenant_id
		FROM email_providers WHERE slug = ? AND is_enabled = 1
	`, slug).Scan(
		&provider.ID, &provider.Name, &provider.Slug, &provider.Type,
		&provider.OAuthClientID, &clientSecretEnc, &provider.OAuthScopes, &provider.OAuthTenantID,
	)
	if err == sql.ErrNoRows {
		http.Error(w, "Provider not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Failed to get provider", http.StatusInternalServerError)
		return
	}

	if provider.Type == models.EmailProviderTypeGeneric {
		http.Error(w, "OAuth not supported for generic IMAP provider", http.StatusBadRequest)
		return
	}

	// Decrypt client secret
	var clientSecret string
	if clientSecretEnc != nil && *clientSecretEnc != "" && h.encryption != nil {
		clientSecret, _ = h.encryption.Decrypt(*clientSecretEnc)
	}

	// Generate state token
	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		http.Error(w, "Failed to generate state", http.StatusInternalServerError)
		return
	}
	state := hex.EncodeToString(stateBytes)

	// Store state in database (expires in 5 minutes)
	expiresAt := time.Now().Add(5 * time.Minute)
	_, err = h.db.Exec(`
		INSERT INTO email_oauth_state (provider_id, channel_id, state, user_id, expires_at)
		VALUES (?, ?, ?, ?, ?)
	`, provider.ID, channelID, state, userID, expiresAt)
	if err != nil {
		http.Error(w, "Failed to store OAuth state", http.StatusInternalServerError)
		return
	}

	// Build redirect URI
	redirectURI := fmt.Sprintf("%s/api/email-providers/%s/oauth/callback", h.baseURL, slug)

	// Create provider and get OAuth URL
	var authURL string
	scopes := strings.Fields(provider.OAuthScopes)

	switch provider.Type {
	case models.EmailProviderTypeMicrosoft:
		p := email.NewMicrosoftProvider(provider.OAuthClientID, clientSecret, provider.OAuthTenantID, scopes)
		authURL = p.GetOAuthURL(state, redirectURI)
	case models.EmailProviderTypeGoogle:
		p := email.NewGoogleProvider(provider.OAuthClientID, clientSecret, scopes)
		authURL = p.GetOAuthURL(state, redirectURI)
	default:
		http.Error(w, "OAuth not supported for this provider type", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"auth_url": authURL,
	})
}

// EmailOAuthCallback handles the OAuth callback from the provider
func (h *EmailProviderHandler) EmailOAuthCallback(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	errorParam := r.URL.Query().Get("error")

	if errorParam != "" {
		errorDesc := r.URL.Query().Get("error_description")
		slog.Error("OAuth error", "error", errorParam, "description", errorDesc)
		// URL-encode the error parameter to prevent open redirect attacks
		http.Redirect(w, r, "/admin/channels?oauth_error="+url.QueryEscape(errorParam), http.StatusFound)
		return
	}

	if code == "" || state == "" {
		http.Error(w, "Missing code or state parameter", http.StatusBadRequest)
		return
	}

	// Validate state and get associated data
	var providerID, channelID, userID int
	err := h.db.QueryRow(`
		SELECT provider_id, channel_id, user_id
		FROM email_oauth_state
		WHERE state = ? AND expires_at > CURRENT_TIMESTAMP
	`, state).Scan(&providerID, &channelID, &userID)
	if err == sql.ErrNoRows {
		http.Error(w, "Invalid or expired state", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "Failed to validate state", http.StatusInternalServerError)
		return
	}

	// Delete used state
	h.db.Exec(`DELETE FROM email_oauth_state WHERE state = ?`, state)

	// Get provider
	var provider models.EmailProvider
	var clientSecretEnc *string
	err = h.db.QueryRow(`
		SELECT id, type, oauth_client_id, oauth_client_secret_encrypted, oauth_scopes, oauth_tenant_id
		FROM email_providers WHERE id = ?
	`, providerID).Scan(
		&provider.ID, &provider.Type,
		&provider.OAuthClientID, &clientSecretEnc, &provider.OAuthScopes, &provider.OAuthTenantID,
	)
	if err != nil {
		http.Error(w, "Provider not found", http.StatusInternalServerError)
		return
	}

	// Decrypt client secret
	var clientSecret string
	if clientSecretEnc != nil && *clientSecretEnc != "" && h.encryption != nil {
		clientSecret, _ = h.encryption.Decrypt(*clientSecretEnc)
	}

	// Build redirect URI (must match the one used in StartOAuth)
	redirectURI := fmt.Sprintf("%s/api/email-providers/%s/oauth/callback", h.baseURL, slug)

	// Exchange code for tokens
	ctx := context.Background()
	scopes := strings.Fields(provider.OAuthScopes)

	var tokens *email.OAuthTokens
	var userEmail string

	switch provider.Type {
	case models.EmailProviderTypeMicrosoft:
		p := email.NewMicrosoftProvider(provider.OAuthClientID, clientSecret, provider.OAuthTenantID, scopes)
		tokens, err = p.ExchangeCode(ctx, code, redirectURI)
		if err != nil {
			slog.Error("failed to exchange code", "error", err)
			http.Redirect(w, r, "/admin/channels?oauth_error=exchange_failed", http.StatusFound)
			return
		}
		userEmail, _ = p.GetUserEmail(ctx, tokens.AccessToken)

	case models.EmailProviderTypeGoogle:
		p := email.NewGoogleProvider(provider.OAuthClientID, clientSecret, scopes)
		tokens, err = p.ExchangeCode(ctx, code, redirectURI)
		if err != nil {
			slog.Error("failed to exchange code", "error", err)
			http.Redirect(w, r, "/admin/channels?oauth_error=exchange_failed", http.StatusFound)
			return
		}
		userEmail, _ = p.GetUserEmail(ctx, tokens.AccessToken)

	default:
		http.Redirect(w, r, "/admin/channels?oauth_error=unsupported_provider", http.StatusFound)
		return
	}

	// Save tokens to channel
	credManager := email.NewCredentialManager(h.db, h.encryption)
	err = credManager.SaveOAuthTokens(ctx, channelID, tokens, userEmail)
	if err != nil {
		slog.Error("failed to save tokens", "error", err)
		http.Redirect(w, r, "/admin/channels?oauth_error=save_failed", http.StatusFound)
		return
	}

	// Also update provider ID in channel config
	h.updateChannelProviderID(channelID, providerID)

	slog.Info("OAuth completed for email channel",
		"channel_id", channelID,
		"provider_id", providerID,
		"email", userEmail,
	)

	// Redirect back to admin
	http.Redirect(w, r, fmt.Sprintf("/admin/channels/%d?oauth_success=true", channelID), http.StatusFound)
}

// updateChannelProviderID updates the email_provider_id in channel config
func (h *EmailProviderHandler) updateChannelProviderID(channelID, providerID int) {
	var configJSON string
	h.db.QueryRow(`SELECT config FROM channels WHERE id = ?`, channelID).Scan(&configJSON)

	var config models.ChannelConfig
	if configJSON != "" {
		json.Unmarshal([]byte(configJSON), &config)
	}

	config.EmailProviderID = &providerID

	updatedJSON, _ := json.Marshal(config)
	h.db.Exec(`UPDATE channels SET config = ? WHERE id = ?`, string(updatedJSON), channelID)
}

// TestEmailChannel tests an email channel connection
func (h *EmailProviderHandler) TestEmailChannel(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	channelID, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid channel ID", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	credManager := email.NewCredentialManager(h.db, h.encryption)

	provider, config, err := credManager.GetProviderForChannel(ctx, channelID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"success": "false",
			"error":   err.Error(),
		})
		return
	}

	// Test connection
	err = provider.TestConnection(ctx, config)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"email":   config.EmailOAuthEmail,
	})
}
