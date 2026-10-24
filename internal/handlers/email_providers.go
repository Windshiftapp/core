package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/email"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// EmailProviderHandler handles email provider API endpoints
type EmailProviderHandler struct {
	db          database.Database
	encryption  email.Encryptor
	baseURL     string // Base URL for OAuth callbacks
	channels    *services.ChannelService
	credentials *email.CredentialManager
}

// NewEmailProviderHandler creates a new email provider handler
func NewEmailProviderHandler(db database.Database, encryption email.Encryptor, baseURL string, channels *services.ChannelService) *EmailProviderHandler {
	return &EmailProviderHandler{
		db:          db,
		encryption:  encryption,
		baseURL:     baseURL,
		channels:    channels,
		credentials: email.NewCredentialManager(db, encryption),
	}
}

// SetCredentialManager lets the server share one per-channel refresh lock
// across the scheduler, inline callback, provider callback, and manual test.
func (h *EmailProviderHandler) SetCredentialManager(credentials *email.CredentialManager) {
	if credentials != nil {
		h.credentials = credentials
	}
}

func scanEmailProvider(row rowScanner) (models.EmailProvider, error) {
	var provider models.EmailProvider
	var oauthClientID, oauthScopes, oauthTenantID sql.NullString
	var imapHost, imapEncryption sql.NullString
	var imapPort sql.NullInt64
	err := row.Scan(
		&provider.ID, &provider.Name, &provider.Slug, &provider.Type, &provider.IsEnabled,
		&oauthClientID, &oauthScopes, &oauthTenantID,
		&imapHost, &imapPort, &imapEncryption,
		&provider.CreatedAt, &provider.UpdatedAt,
	)
	if err != nil {
		return models.EmailProvider{}, err
	}
	provider.OAuthClientID = oauthClientID.String
	provider.OAuthScopes = oauthScopes.String
	provider.OAuthTenantID = oauthTenantID.String
	provider.IMAPHost = imapHost.String
	provider.IMAPPort = int(imapPort.Int64)
	provider.IMAPEncryption = imapEncryption.String
	return provider, nil
}

// requireManagedInboundEmailChannel is defense-in-depth for the legacy email
// endpoints. Route middleware performs the same management check, but OAuth
// callbacks are intentionally unauthenticated and must re-authorize the user
// captured in the state row before changing channel credentials.
func (h *EmailProviderHandler) requireManagedInboundEmailChannel(ctx context.Context, userID, channelID int) error {
	if h.channels == nil {
		return fmt.Errorf("channel service is not configured")
	}
	canManage, err := h.channels.UserCanManage(ctx, userID, channelID)
	if err != nil {
		return err
	}
	if !canManage {
		return repository.ErrNotFound
	}
	channel, err := h.channels.GetByID(ctx, channelID)
	if err != nil {
		return err
	}
	if channel == nil || channel.Type != "email" || channel.Direction != "inbound" {
		return repository.ErrNotFound
	}
	if channel.IsDefault {
		isAdmin, adminErr := h.channels.UserIsSystemAdmin(userID)
		if adminErr != nil {
			return adminErr
		}
		if !isAdmin {
			return repository.ErrNotFound
		}
	}
	return nil
}

func (h *EmailProviderHandler) channelsUsingProvider(ctx context.Context, providerID int) ([]int, error) {
	rows, err := h.db.QueryContext(ctx, `
		SELECT id, COALESCE(config, '{}')
		FROM channels
		WHERE type = 'email' AND direction = 'inbound'
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var channelIDs []int
	for rows.Next() {
		var channelID int
		var raw string
		if err := rows.Scan(&channelID, &raw); err != nil {
			return nil, err
		}
		var config models.ChannelConfig
		if err := json.Unmarshal([]byte(raw), &config); err != nil {
			return nil, fmt.Errorf("parse email channel %d while checking provider references: %w", channelID, err)
		}
		if config.EmailProviderID != nil && *config.EmailProviderID == providerID {
			channelIDs = append(channelIDs, channelID)
		}
	}
	return channelIDs, rows.Err()
}

// GetEmailProviders returns all email providers
func (h *EmailProviderHandler) GetEmailProviders(w http.ResponseWriter, r *http.Request) {
	// Check admin permission
	userID := r.Context().Value("user_id")
	if userID == nil {
		respondUnauthorized(w, r)
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
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	var providers []models.EmailProvider
	for rows.Next() {
		p, err := scanEmailProvider(rows)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		providers = append(providers, p)
	}
	if err := rows.Err(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, providers)
}

// GetEmailProvider returns a single email provider
func (h *EmailProviderHandler) GetEmailProvider(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	p, err := scanEmailProvider(h.db.QueryRow(`
		SELECT id, name, slug, type, is_enabled,
		       oauth_client_id, oauth_scopes, oauth_tenant_id,
		       imap_host, imap_port, imap_encryption,
		       created_at, updated_at
		FROM email_providers WHERE id = ?
	`, id))
	if errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "provider")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, p)
}

// CreateEmailProviderRequest represents the request body for creating a provider
type CreateEmailProviderRequest struct {
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	Type      string `json:"type"` // microsoft, google, generic
	IsEnabled bool   `json:"is_enabled"`

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

func validateEmailProviderRequest(req CreateEmailProviderRequest, requireOAuthSecret bool) error {
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Slug) == "" {
		return fmt.Errorf("name and slug are required")
	}
	switch req.Type {
	case models.EmailProviderTypeMicrosoft, models.EmailProviderTypeGoogle:
		if strings.TrimSpace(req.OAuthClientID) == "" {
			return fmt.Errorf("OAuth client ID is required")
		}
		if requireOAuthSecret && strings.TrimSpace(req.OAuthClientSecret) == "" {
			return fmt.Errorf("OAuth client secret is required")
		}
	case models.EmailProviderTypeGeneric:
		if strings.TrimSpace(req.IMAPHost) == "" {
			return fmt.Errorf("IMAP host is required")
		}
		if req.IMAPPort <= 0 || req.IMAPPort > 65535 {
			return fmt.Errorf("IMAP port must be between 1 and 65535")
		}
		if req.IMAPEncryption != "ssl" && req.IMAPEncryption != "starttls" {
			return fmt.Errorf("IMAP encryption must be ssl or starttls")
		}
	default:
		return fmt.Errorf("invalid provider type")
	}
	return nil
}

func normalizeEmailProviderRequest(req *CreateEmailProviderRequest) {
	if req.Type == models.EmailProviderTypeGeneric {
		req.OAuthClientID = ""
		req.OAuthClientSecret = ""
		req.OAuthScopes = ""
		req.OAuthTenantID = ""
		return
	}
	req.IMAPHost = ""
	req.IMAPPort = 0
	req.IMAPEncryption = ""
}

// CreateEmailProvider creates a new email provider
func (h *EmailProviderHandler) CreateEmailProvider(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeChannelJSON[CreateEmailProviderRequest](w, r)
	if !ok {
		return
	}
	// Name renders in the provider admin list + per-channel picker;
	// Slug is the routing key. Secrets (OAuthClientSecret) stay raw —
	// encrypted further down. Type / IMAPEncryption are enums.
	warnings := sanitize.ApplyAllWithWarnings(
		sanitize.Pair{Target: &req.Name, Policy: sanitize.PlainTextField, Label: "Name"},
		sanitize.Pair{Target: &req.Slug, Policy: sanitize.ShortIdentifier, Label: "Slug"},
	)
	normalizeEmailProviderRequest(&req)
	if req.Type == models.EmailProviderTypeGeneric {
		if req.IMAPPort == 0 {
			req.IMAPPort = 993
		}
		if req.IMAPEncryption == "" {
			req.IMAPEncryption = "ssl"
		}
	}

	if err := validateEmailProviderRequest(req, true); err != nil {
		respondValidationError(w, r, err.Error())
		return
	}

	// Encrypt client secret if provided
	var clientSecretEnc *string
	if req.Type != models.EmailProviderTypeGeneric && req.OAuthClientSecret != "" {
		encrypted, err := email.EncryptSecret(h.encryption, req.OAuthClientSecret)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		clientSecretEnc = &encrypted
	}

	// Check uniqueness before insert
	var slugExists bool
	if err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM email_providers WHERE slug = ?)", req.Slug).Scan(&slugExists); err != nil {
		respondInternalError(w, r, err)
		return
	}
	if slugExists {
		respondConflict(w, r, "A provider with this slug already exists")
		return
	}

	// Insert provider
	var id int64
	err := h.db.QueryRow(`
		INSERT INTO email_providers (
			name, slug, type, is_enabled,
			oauth_client_id, oauth_client_secret_encrypted, oauth_scopes, oauth_tenant_id,
			imap_host, imap_port, imap_encryption,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP) RETURNING id
	`,
		req.Name, req.Slug, req.Type, req.IsEnabled,
		nullString(req.OAuthClientID), clientSecretEnc, nullString(req.OAuthScopes), nullString(req.OAuthTenantID),
		nullString(req.IMAPHost), req.IMAPPort, nullString(req.IMAPEncryption),
	).Scan(&id)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			respondConflict(w, r, "A provider with this slug already exists")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		providerID := int(id)
		logAudit(h.db, r, currentUser, logger.ActionEmailProviderCreate, logger.ResourceEmailProvider, &providerID, req.Name)
	}

	resp := map[string]any{
		"id":   id,
		"slug": req.Slug,
	}
	if len(warnings) > 0 {
		resp["warnings"] = warnings
	}
	respondJSONCreated(w, resp)
}

// UpdateEmailProvider updates an email provider
func (h *EmailProviderHandler) UpdateEmailProvider(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	req, ok := decodeChannelJSON[CreateEmailProviderRequest](w, r)
	if !ok {
		return
	}
	warnings := sanitize.ApplyAllWithWarnings(
		sanitize.Pair{Target: &req.Name, Policy: sanitize.PlainTextField, Label: "Name"},
		sanitize.Pair{Target: &req.Slug, Policy: sanitize.ShortIdentifier, Label: "Slug"},
	)
	normalizeEmailProviderRequest(&req)
	if req.Type == models.EmailProviderTypeGeneric {
		if req.IMAPPort == 0 {
			req.IMAPPort = 993
		}
		if req.IMAPEncryption == "" {
			req.IMAPEncryption = "ssl"
		}
	}
	var existingType string
	var existingEnabled bool
	var existingClientID, existingSecret, existingTenantID sql.NullString
	if err = h.db.QueryRow(`
		SELECT type, is_enabled, oauth_client_id, oauth_client_secret_encrypted, oauth_tenant_id
		FROM email_providers WHERE id = ?
	`, id).Scan(&existingType, &existingEnabled, &existingClientID, &existingSecret, &existingTenantID); errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "provider")
		return
	} else if err != nil {
		respondInternalError(w, r, err)
		return
	}
	requireOAuthSecret := req.Type != models.EmailProviderTypeGeneric &&
		(!existingSecret.Valid || existingSecret.String == "" || existingType != req.Type || existingClientID.String != req.OAuthClientID)
	if err = validateEmailProviderRequest(req, requireOAuthSecret); err != nil {
		respondValidationError(w, r, err.Error())
		return
	}
	identityChanged := existingType != req.Type || existingClientID.String != req.OAuthClientID || existingTenantID.String != req.OAuthTenantID
	if identityChanged || (existingEnabled && !req.IsEnabled) {
		channelIDs, refErr := h.channelsUsingProvider(r.Context(), id)
		if refErr != nil {
			respondInternalError(w, r, refErr)
			return
		}
		if len(channelIDs) > 0 {
			respondConflict(w, r, fmt.Sprintf("Provider is still connected to email channels %v; disconnect it before changing its identity or disabling it", channelIDs))
			return
		}
	}

	// Encrypt client secret if provided
	var clientSecretEnc *string
	if req.Type != models.EmailProviderTypeGeneric && req.OAuthClientSecret != "" {
		var encrypted string
		encrypted, err = email.EncryptSecret(h.encryption, req.OAuthClientSecret)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		clientSecretEnc = &encrypted
	}

	// Build update query dynamically
	query := `UPDATE email_providers SET name = ?, slug = ?, type = ?, is_enabled = ?,
	          oauth_client_id = ?, oauth_scopes = ?, oauth_tenant_id = ?,
	          imap_host = ?, imap_port = ?, imap_encryption = ?,
	          updated_at = CURRENT_TIMESTAMP`
	args := []any{
		req.Name, req.Slug, req.Type, req.IsEnabled,
		nullString(req.OAuthClientID), nullString(req.OAuthScopes), nullString(req.OAuthTenantID),
		nullString(req.IMAPHost), req.IMAPPort, nullString(req.IMAPEncryption),
	}

	// Secrets belong to one OAuth application. Clear them when switching to
	// generic IMAP; changing provider type/client ID requires a replacement.
	if req.Type == models.EmailProviderTypeGeneric {
		query += `, oauth_client_secret_encrypted = NULL`
	} else if clientSecretEnc != nil {
		query += `, oauth_client_secret_encrypted = ?`
		args = append(args, clientSecretEnc)
	}

	query += ` WHERE id = ?`
	args = append(args, id)

	result, err := h.db.ExecWrite(query, args...)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			respondConflict(w, r, "A provider with this slug already exists")
			return
		}
		respondInternalError(w, r, err)
		return
	}
	if rows, rowsErr := result.RowsAffected(); rowsErr != nil {
		respondInternalError(w, r, rowsErr)
		return
	} else if rows == 0 {
		respondNotFound(w, r, "provider")
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logAudit(h.db, r, currentUser, logger.ActionEmailProviderUpdate, logger.ResourceEmailProvider, &id, req.Name)
	}

	resp := map[string]any{"status": "updated"}
	if len(warnings) > 0 {
		resp["warnings"] = warnings
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// DeleteEmailProvider deletes an email provider
func (h *EmailProviderHandler) DeleteEmailProvider(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}
	channelIDs, refErr := h.channelsUsingProvider(r.Context(), id)
	if refErr != nil {
		respondInternalError(w, r, refErr)
		return
	}
	if len(channelIDs) > 0 {
		respondConflict(w, r, fmt.Sprintf("Provider is still connected to email channels %v; disconnect it before deleting it", channelIDs))
		return
	}

	result, err := h.db.ExecWrite(`DELETE FROM email_providers WHERE id = ?`, id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if rows, rowsErr := result.RowsAffected(); rowsErr != nil {
		respondInternalError(w, r, rowsErr)
		return
	} else if rows == 0 {
		respondNotFound(w, r, "provider")
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logAudit(h.db, r, currentUser, logger.ActionEmailProviderDelete, logger.ResourceEmailProvider, &id, "")
	}

	w.WriteHeader(http.StatusNoContent)
}

// StartEmailOAuth initiates the OAuth flow for an email channel.
// Route: POST /api/channels/{channel_id}/email-providers/{slug}/oauth/start
func (h *EmailProviderHandler) StartEmailOAuth(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if slug == "" {
		respondValidationError(w, r, "Provider slug required")
		return
	}

	channelID, err := strconv.Atoi(r.PathValue("channel_id"))
	if err != nil {
		respondInvalidID(w, r, "channel ID")
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	userID := user.ID
	if err = h.requireManagedInboundEmailChannel(r.Context(), userID, channelID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "channel")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Get provider
	var provider models.EmailProvider
	var clientSecretEnc *string
	var oauthClientID, oauthScopes, oauthTenantID sql.NullString
	err = h.db.QueryRow(`
		SELECT id, name, slug, type, oauth_client_id, oauth_client_secret_encrypted, oauth_scopes, oauth_tenant_id
		FROM email_providers WHERE slug = ? AND is_enabled = true
	`, slug).Scan(
		&provider.ID, &provider.Name, &provider.Slug, &provider.Type,
		&oauthClientID, &clientSecretEnc, &oauthScopes, &oauthTenantID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "provider")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	provider.OAuthClientID = oauthClientID.String
	provider.OAuthScopes = oauthScopes.String
	provider.OAuthTenantID = oauthTenantID.String

	if provider.Type == models.EmailProviderTypeGeneric {
		respondBadRequest(w, r, "OAuth not supported for generic IMAP provider")
		return
	}
	if provider.OAuthClientID == "" || clientSecretEnc == nil || *clientSecretEnc == "" {
		respondValidationError(w, r, "OAuth provider credentials are incomplete")
		return
	}

	// Decrypt client secret
	var clientSecret string
	if clientSecretEnc != nil && *clientSecretEnc != "" {
		clientSecret, err = email.DecryptOrLegacy(h.encryption, *clientSecretEnc)
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("decrypt OAuth client secret: %w", err))
			return
		}
	}

	startingConfig, err := h.channels.GetConfig(r.Context(), channelID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	// The monitored mailbox is entered by the admin and is what IMAP XOAUTH2
	// authenticates as; require it before starting so the callback never has to
	// guess an identity from the token.
	var startingChannelConfig models.ChannelConfig
	if startingConfig != "" {
		if err := json.Unmarshal([]byte(startingConfig), &startingChannelConfig); err != nil {
			respondInternalError(w, r, err)
			return
		}
	}
	if _, ok := bareEmailAddress(startingChannelConfig.EmailOAuthMailbox); !ok {
		respondValidationError(w, r, "Enter the mailbox address to connect before starting OAuth")
		return
	}
	// Bind the state to the channel config at flow start. Central providers are
	// independent of the channel config, so without this guard a callback from
	// an old tab could silently switch a channel back from a newer basic/inline
	// configuration.
	state, err := newEmailOAuthState(startingConfig)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Store state in database (expires in 5 minutes)
	expiresAt := time.Now().Add(5 * time.Minute)
	_, err = h.db.ExecWrite(`
		INSERT INTO email_oauth_state (provider_id, channel_id, state, user_id, expires_at)
		VALUES (?, ?, ?, ?, ?)
	`, provider.ID, channelID, state, userID, expiresAt)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Build redirect URI — must match the registered callback route
	// (internal/routes/scm.go: GET /email/oauth/{slug}/callback under /api).
	redirectURI := fmt.Sprintf("%s/api/email/oauth/%s/callback", h.baseURL, slug)

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
		respondBadRequest(w, r, "OAuth not supported for this provider type")
		return
	}

	respondJSONOK(w, map[string]string{
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
		if state != "" {
			// Provider-declined/error callbacks are terminal too. Consume their
			// state so a captured value cannot be replayed until expiry.
			_, _, _, _, _ = repository.NewChannelRepository(h.db).ConsumeOAuthState(r.Context(), state, true)
		}
		// URL-encode the error parameter to prevent open redirect attacks
		http.Redirect(w, r, "/admin/channels?oauth_error="+url.QueryEscape(errorParam), http.StatusFound)
		return
	}

	if code == "" || state == "" {
		respondValidationError(w, r, "Missing code or state parameter")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// Consume state atomically so concurrent callbacks cannot replay it.
	providerID, channelID, userID, _, err := repository.NewChannelRepository(h.db).ConsumeOAuthState(ctx, state, true)
	if errors.Is(err, repository.ErrNotFound) {
		respondBadRequest(w, r, "Invalid or expired state")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	// Bind the callback to the account that started the flow, matching the
	// channel-inline callback so a forwarded authorization URL cannot attach the
	// victim's mailbox credentials to another identity.
	currentUser := utils.GetCurrentUser(r)
	if currentUser == nil || currentUser.ID != userID {
		slog.Warn("email OAuth state/account mismatch", slog.String("component", "email_providers"), slog.Int("channel_id", channelID))
		http.Redirect(w, r, "/admin/channels?oauth_error=authorization_failed", http.StatusFound)
		return
	}
	if err = h.requireManagedInboundEmailChannel(ctx, userID, channelID); err != nil {
		handleEmailOAuthAuthorizationFailure(w, r, err)
		return
	}
	expectedConfigJSON, err := h.channels.GetConfig(ctx, channelID)
	if err != nil {
		http.Redirect(w, r, "/admin/channels?oauth_error=channel_not_found", http.StatusFound)
		return
	}
	if !emailOAuthStateMatchesConfig(state, expectedConfigJSON) {
		http.Redirect(w, r, "/admin/channels?oauth_error=config_changed", http.StatusFound)
		return
	}
	var expectedConfig models.ChannelConfig
	if err := json.Unmarshal([]byte(expectedConfigJSON), &expectedConfig); err != nil {
		http.Redirect(w, r, "/admin/channels?oauth_error=invalid_config", http.StatusFound)
		return
	}

	// Get provider
	var provider models.EmailProvider
	var clientSecretEnc *string
	var oauthClientID, oauthScopes, oauthTenantID sql.NullString
	err = h.db.QueryRow(`
		SELECT id, slug, type, oauth_client_id, oauth_client_secret_encrypted, oauth_scopes, oauth_tenant_id
		FROM email_providers WHERE id = ? AND is_enabled = true
	`, providerID).Scan(
		&provider.ID, &provider.Slug, &provider.Type,
		&oauthClientID, &clientSecretEnc, &oauthScopes, &oauthTenantID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		http.Redirect(w, r, "/admin/channels?oauth_error=provider_unavailable", http.StatusFound)
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	provider.OAuthClientID = oauthClientID.String
	provider.OAuthScopes = oauthScopes.String
	provider.OAuthTenantID = oauthTenantID.String
	if provider.Slug != slug {
		respondBadRequest(w, r, "OAuth callback provider does not match state")
		return
	}
	if provider.OAuthClientID == "" || clientSecretEnc == nil || *clientSecretEnc == "" {
		http.Redirect(w, r, "/admin/channels?oauth_error=provider_unavailable", http.StatusFound)
		return
	}

	// Decrypt client secret
	var clientSecret string
	if clientSecretEnc != nil && *clientSecretEnc != "" {
		clientSecret, err = email.DecryptOrLegacy(h.encryption, *clientSecretEnc)
		if err != nil {
			slog.Error("failed to decrypt OAuth client secret in callback", slog.String("component", "email_providers"), slog.Int("provider_id", provider.ID), slog.Any("error", err))
			http.Redirect(w, r, "/admin/channels?oauth_error=decrypt_failed", http.StatusFound)
			return
		}
	}

	// Build redirect URI (must match the one used in StartEmailOAuth and the
	// registered callback route at /api/email/oauth/{slug}/callback).
	redirectURI := fmt.Sprintf("%s/api/email/oauth/%s/callback", h.baseURL, slug)

	// Exchange code for tokens. The mailbox address is configuration the admin
	// already saved, not a value discovered from the token.
	scopes := strings.Fields(provider.OAuthScopes)

	userEmail, ok := bareEmailAddress(expectedConfig.EmailOAuthMailbox)
	if !ok {
		slog.Error("email OAuth callback has no usable mailbox address", slog.String("component", "email_providers"), slog.Int("channel_id", channelID))
		http.Redirect(w, r, "/admin/channels?oauth_error=identity_failed", http.StatusFound)
		return
	}

	var tokens *email.OAuthTokens

	switch provider.Type {
	case models.EmailProviderTypeMicrosoft:
		p := email.NewMicrosoftProvider(provider.OAuthClientID, clientSecret, provider.OAuthTenantID, scopes)
		tokens, err = p.ExchangeCode(ctx, code, redirectURI)
		if err != nil {
			slog.Error("failed to exchange code", "error", err)
			http.Redirect(w, r, "/admin/channels?oauth_error=exchange_failed", http.StatusFound)
			return
		}

	case models.EmailProviderTypeGoogle:
		p := email.NewGoogleProvider(provider.OAuthClientID, clientSecret, scopes)
		tokens, err = p.ExchangeCode(ctx, code, redirectURI)
		if err != nil {
			slog.Error("failed to exchange code", "error", err)
			http.Redirect(w, r, "/admin/channels?oauth_error=exchange_failed", http.StatusFound)
			return
		}

	default:
		http.Redirect(w, r, "/admin/channels?oauth_error=unsupported_provider", http.StatusFound)
		return
	}

	// Save tokens to channel
	// Permissions may have been revoked while the user was at the provider.
	// Re-check immediately before the credential mutation.
	if err = h.requireManagedInboundEmailChannel(ctx, userID, channelID); err != nil {
		handleEmailOAuthAuthorizationFailure(w, r, err)
		return
	}
	var currentType string
	var currentClientID, currentTenantID sql.NullString
	if err = h.db.QueryRow(`
		SELECT type, oauth_client_id, oauth_tenant_id
		FROM email_providers WHERE id = ? AND is_enabled = true
	`, providerID).Scan(&currentType, &currentClientID, &currentTenantID); err != nil ||
		currentType != provider.Type || currentClientID.String != provider.OAuthClientID || currentTenantID.String != provider.OAuthTenantID {
		http.Redirect(w, r, "/admin/channels?oauth_error=provider_unavailable", http.StatusFound)
		return
	}
	_, err = h.credentials.SaveOAuthTokensForProvider(ctx, channelID, tokens, userEmail, providerID, &expectedConfig)
	if err != nil {
		slog.Error("failed to save tokens", "error", err)
		http.Redirect(w, r, "/admin/channels?oauth_error=save_failed", http.StatusFound)
		return
	}

	slog.Info("OAuth completed for email channel",
		"channel_id", channelID,
		"provider_id", providerID,
		"email", userEmail,
	)

	// Redirect back to admin
	// #nosec G710 -- local relative URL built from a database integer; no caller-controlled text reaches the destination
	http.Redirect(w, r, fmt.Sprintf("/admin/channels/%d?oauth_success=true", channelID), http.StatusFound)
}

// TestEmailChannel tests an email channel connection
func (h *EmailProviderHandler) TestEmailChannel(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	channelID, err := strconv.Atoi(idStr)
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	if err = h.requireManagedInboundEmailChannel(r.Context(), user.ID, channelID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "channel")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	ctx := r.Context()
	provider, config, err := h.credentials.GetProviderForChannel(ctx, channelID)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{
			"success": "false",
			"error":   err.Error(),
		})
		return
	}

	// Test connection
	err = provider.TestConnection(ctx, config)
	if err != nil {
		respondJSONOK(w, map[string]any{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	respondJSONOK(w, map[string]any{
		"success": true,
		"email":   config.EmailOAuthEmail,
	})
}

func handleEmailOAuthAuthorizationFailure(w http.ResponseWriter, r *http.Request, err error) {
	if !errors.Is(err, repository.ErrNotFound) {
		slog.Error("failed to re-authorize email OAuth callback", "error", err)
	}
	http.Redirect(w, r, "/admin/channels?oauth_error=authorization_failed", http.StatusFound)
}
