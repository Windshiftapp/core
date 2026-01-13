package handlers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"windshift/internal/auth"
	"windshift/internal/database"
	"windshift/internal/services"
	"windshift/internal/sso"
	"windshift/internal/utils"

	"github.com/zitadel/oidc/v3/pkg/client/rp"
	"github.com/zitadel/oidc/v3/pkg/oidc"
)

// oidcErrorMessages maps OIDC error codes to safe user-facing messages
// This prevents reflected error injection from IdP error_description
var oidcErrorMessages = map[string]string{
	"access_denied":             "Access was denied by the identity provider",
	"invalid_request":           "Invalid authentication request",
	"unauthorized_client":       "Client not authorized for this operation",
	"unsupported_response_type": "Unsupported response type",
	"invalid_scope":             "Invalid scope requested",
	"server_error":              "Identity provider encountered an error",
	"temporarily_unavailable":   "Identity provider is temporarily unavailable",
	"interaction_required":      "User interaction required",
	"login_required":            "Login required",
	"consent_required":          "Consent required",
	"account_selection_required": "Account selection required",
	"invalid_grant":             "Invalid or expired authorization code",
}

// SSOHandler handles SSO authentication endpoints
type SSOHandler struct {
	db                       database.Database
	sessionManager           *auth.SessionManager
	permissionService        *services.PermissionService
	emailVerificationService *services.EmailVerificationService
	providerStore            *sso.ProviderStore
	userStore                *sso.UserStore
	oidcService              *sso.OIDCService
	encryption               *sso.SecretEncryption
	baseURL                  string             // Base URL of the application (e.g., https://app.example.com)
	allowedHosts             []string           // Allowed hosts for redirect URI validation (from --allowed-hosts)
	devMode                  bool               // Development mode (from --no-csrf flag)
	ipExtractor              *utils.IPExtractor // IP extractor with proxy validation
}

// SSOStatusResponse represents the public SSO status
type SSOStatusResponse struct {
	Enabled            bool   `json:"enabled"`
	ProviderName       string `json:"provider_name,omitempty"`
	ProviderSlug       string `json:"provider_slug,omitempty"`
	AllowPasswordLogin bool   `json:"allow_password_login"`
}

// SSOProviderResponse represents a provider for API responses (without secrets)
type SSOProviderResponse struct {
	ID                   int       `json:"id"`
	Slug                 string    `json:"slug"`
	Name                 string    `json:"name"`
	ProviderType         string    `json:"provider_type"`
	Enabled              bool      `json:"enabled"`
	IsDefault            bool      `json:"is_default"`
	IssuerURL            string    `json:"issuer_url,omitempty"`
	ClientID             string    `json:"client_id,omitempty"`
	HasClientSecret      bool      `json:"has_client_secret"`
	Scopes               string    `json:"scopes"`
	AutoProvisionUsers   bool      `json:"auto_provision_users"`
	AllowPasswordLogin   bool      `json:"allow_password_login"`
	RequireVerifiedEmail bool      `json:"require_verified_email"`
	AttributeMapping     string    `json:"attribute_mapping"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// SSOProviderRequest represents the request body for creating/updating a provider
type SSOProviderRequest struct {
	Slug                 string `json:"slug"`
	Name                 string `json:"name"`
	ProviderType         string `json:"provider_type"`
	Enabled              bool   `json:"enabled"`
	IsDefault            bool   `json:"is_default"`
	IssuerURL            string `json:"issuer_url"`
	ClientID             string `json:"client_id"`
	ClientSecret         string `json:"client_secret,omitempty"`
	Scopes               string `json:"scopes"`
	AutoProvisionUsers   bool   `json:"auto_provision_users"`
	AllowPasswordLogin   bool   `json:"allow_password_login"`
	RequireVerifiedEmail *bool  `json:"require_verified_email"` // Pointer to distinguish between false and not set
	AttributeMapping     string `json:"attribute_mapping"`
}

// NewSSOHandler creates a new SSO handler
// allowedHostsStr: comma-separated list of allowed hosts from --allowed-hosts flag
// devMode: true if --no-csrf flag is set (development mode)
// emailVerificationService: service for handling email verification (can be nil if SMTP not configured)
func NewSSOHandler(db database.Database, sessionManager *auth.SessionManager, permissionService *services.PermissionService, emailVerificationService *services.EmailVerificationService, allowedHostsStr string, devMode bool, ipExtractor *utils.IPExtractor) *SSOHandler {
	// Get server secret for encryption
	serverSecret := os.Getenv("SSO_SECRET")
	if serverSecret == "" {
		serverSecret = os.Getenv("SESSION_SECRET")
	}
	if serverSecret == "" {
		log.Fatal("FATAL: SSO_SECRET or SESSION_SECRET environment variable must be set for SSO credential encryption")
	}

	// Get base URL from environment or construct from request later
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = os.Getenv("PUBLIC_URL")
	}

	// Parse allowed hosts
	var allowedHosts []string
	if allowedHostsStr != "" {
		for _, h := range strings.Split(allowedHostsStr, ",") {
			if trimmed := strings.TrimSpace(h); trimmed != "" {
				allowedHosts = append(allowedHosts, trimmed)
			}
		}
	}

	// Log warning for production without BASE_URL
	if !devMode && baseURL == "" {
		if len(allowedHosts) > 0 {
			slog.Info("SSO running without BASE_URL, redirect URIs will use allowed-hosts and default to HTTPS")
		} else {
			slog.Warn("SSO running without BASE_URL or allowed-hosts, this is insecure in production")
		}
	}

	// Create cookie key from server secret (32 bytes for AES-256)
	cookieKey := make([]byte, 32)
	copy(cookieKey, []byte(serverSecret))

	return &SSOHandler{
		db:                       db,
		sessionManager:           sessionManager,
		permissionService:        permissionService,
		emailVerificationService: emailVerificationService,
		providerStore:            sso.NewProviderStore(db),
		userStore:                sso.NewUserStore(db),
		oidcService:              sso.NewOIDCService(cookieKey),
		encryption:               sso.NewSecretEncryption(serverSecret),
		baseURL:                  baseURL,
		allowedHosts:             allowedHosts,
		devMode:                  devMode,
		ipExtractor:              ipExtractor,
	}
}

// GetStatus returns the public SSO status (no auth required)
func (h *SSOHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	provider, err := h.providerStore.GetDefault()
	if err != nil {
		// No default provider or error - SSO not enabled
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SSOStatusResponse{
			Enabled:            false,
			AllowPasswordLogin: true,
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(SSOStatusResponse{
		Enabled:            provider.Enabled,
		ProviderName:       provider.Name,
		ProviderSlug:       provider.Slug,
		AllowPasswordLogin: provider.AllowPasswordLogin,
	})
}

// StartLogin initiates the SSO login flow
func (h *SSOHandler) StartLogin(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	provider, err := h.providerStore.GetBySlug(slug)
	if err != nil {
		http.Error(w, "SSO provider not found", http.StatusNotFound)
		return
	}

	if !provider.Enabled {
		http.Error(w, "SSO provider is disabled", http.StatusBadRequest)
		return
	}

	if provider.ProviderType != sso.ProviderTypeOIDC {
		http.Error(w, "Provider type not supported", http.StatusBadRequest)
		return
	}

	// Decrypt client secret
	clientSecret, err := h.encryption.Decrypt(provider.ClientSecretEncrypted)
	if err != nil {
		slog.Error("failed to decrypt client secret", slog.String("component", "sso"), slog.Any("error", err))
		http.Error(w, "SSO configuration error", http.StatusInternalServerError)
		return
	}

	// Determine redirect URI
	redirectURI := h.getRedirectURI(r, slug)

	// Create relying party
	ctx := context.Background()
	relyingParty, err := h.oidcService.CreateRelyingParty(ctx, provider, redirectURI, clientSecret)
	if err != nil {
		slog.Error("failed to create relying party", slog.String("component", "sso"), slog.Any("error", err))
		http.Error(w, "Failed to initialize SSO", http.StatusInternalServerError)
		return
	}

	// Generate state with random data
	state := generateRandomState()

	// Get the auth URL handler and redirect
	authHandler := h.oidcService.GetAuthURLHandler(relyingParty, func() string {
		return state
	})
	authHandler(w, r)
}

// Callback handles the OIDC callback
func (h *SSOHandler) Callback(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	provider, err := h.providerStore.GetBySlug(slug)
	if err != nil {
		h.redirectWithError(w, r, "SSO provider not found")
		return
	}

	if !provider.Enabled {
		h.redirectWithError(w, r, "SSO provider is disabled")
		return
	}

	// Check for error from provider
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		errDesc := r.URL.Query().Get("error_description")
		slog.Warn("provider returned error", slog.String("component", "sso"), slog.String("error_code", errParam), slog.String("error_description", errDesc))

		// Map to safe internal message (don't expose raw IdP error_description to prevent XSS)
		safeMessage := oidcErrorMessages[errParam]
		if safeMessage == "" {
			safeMessage = "Authentication failed"
		}

		h.redirectWithError(w, r, safeMessage)
		return
	}

	// Decrypt client secret
	clientSecret, err := h.encryption.Decrypt(provider.ClientSecretEncrypted)
	if err != nil {
		slog.Error("failed to decrypt client secret", slog.String("component", "sso"), slog.Any("error", err))
		h.redirectWithError(w, r, "SSO configuration error")
		return
	}

	// Determine redirect URI (must match the one used in StartLogin)
	redirectURI := h.getRedirectURI(r, slug)

	// Create relying party
	ctx := context.Background()
	relyingParty, err := h.oidcService.CreateRelyingParty(ctx, provider, redirectURI, clientSecret)
	if err != nil {
		slog.Error("failed to create relying party", slog.String("component", "sso"), slog.Any("error", err))
		h.redirectWithError(w, r, "Failed to initialize SSO")
		return
	}

	// Get attribute mapping
	attributeMap, err := provider.GetAttributeMap()
	if err != nil {
		slog.Warn("failed to parse attribute mapping, using defaults", slog.String("component", "sso"), slog.Any("error", err))
		attributeMap = nil // Use defaults
	}

	// Create callback handler
	callbackHandler := h.oidcService.GetCodeExchangeHandler(relyingParty, func(w http.ResponseWriter, r *http.Request, tokens *oidc.Tokens[*oidc.IDTokenClaims], state string, rp rp.RelyingParty) {
		// Extract claims
		claims, err := h.oidcService.ExtractClaims(tokens, attributeMap)
		if err != nil {
			slog.Error("failed to extract claims", slog.String("component", "sso"), slog.Any("error", err))
			h.redirectWithError(w, r, "Failed to process authentication")
			return
		}

		// Find or create user
		result, err := h.userStore.FindOrCreateUser(provider, claims)
		if err != nil {
			slog.Error("failed to find/create user", slog.String("component", "sso"), slog.Any("error", err))
			if err == sso.ErrAutoProvisionDisabled {
				h.redirectWithError(w, r, "User account not found. Contact your administrator.")
			} else if errors.Is(err, sso.ErrEmailNotVerified) {
				h.redirectWithError(w, r, "Your email address has not been verified by the identity provider")
			} else {
				h.redirectWithError(w, r, "Failed to process user account")
			}
			return
		}

		user := result.User

		// Check if user is active
		if !user.IsActive {
			h.redirectWithError(w, r, "Account is disabled")
			return
		}

		// Handle email verification if needed
		if result.NeedsEmailVerification && !user.EmailVerified {
			// User needs email verification - send verification email
			if h.emailVerificationService != nil {
				token, err := h.emailVerificationService.GenerateVerificationToken(user.ID)
				if err != nil {
					slog.Warn("failed to generate verification token", slog.String("component", "sso"), slog.Int("user_id", user.ID), slog.Any("error", err))
					// Continue with login but log the error
				} else {
					if err := h.emailVerificationService.SendVerificationEmail(user, token); err != nil {
						slog.Warn("failed to send verification email", slog.String("component", "sso"), slog.Int("user_id", user.ID), slog.Any("error", err))
						// Continue with login but log the error
					} else {
						slog.Info("sent verification email", slog.String("component", "sso"), slog.Int("user_id", user.ID), slog.String("email", user.Email))
					}
				}
			} else {
				slog.Warn("user needs email verification but SMTP is not configured", slog.String("component", "sso"), slog.Int("user_id", user.ID))
			}
		}

		// Get client IP for session
		ipAddress := h.ipExtractor.GetClientIP(r)

		// Create session
		slog.Debug("creating session", slog.String("component", "sso"), slog.Int("user_id", user.ID), slog.String("ip_address", ipAddress))
		session, err := h.sessionManager.CreateSession(user.ID, ipAddress, r.UserAgent(), false)
		if err != nil {
			slog.Error("failed to create session", slog.String("component", "sso"), slog.Any("error", err))
			h.redirectWithError(w, r, "Failed to create session")
			return
		}
		slog.Debug("session created", slog.String("component", "sso"))

		// Set session cookie
		slog.Debug("setting session cookie", slog.String("component", "sso"))
		if err := h.sessionManager.SetSessionCookie(w, r, session.Token, false); err != nil {
			slog.Error("failed to set session cookie", slog.String("component", "sso"), slog.Any("error", err))
			h.redirectWithError(w, r, "Failed to set session")
			return
		}
		slog.Debug("session cookie set, redirecting", slog.String("component", "sso"))

		// Redirect based on email verification status
		if result.NeedsEmailVerification && !user.EmailVerified {
			// Redirect to verification pending page
			http.Redirect(w, r, "/?verify_email=pending", http.StatusFound)
		} else {
			// Redirect to app
			http.Redirect(w, r, "/", http.StatusFound)
		}
	})

	callbackHandler(w, r)
}

// ListProviders returns all SSO providers (admin only)
func (h *SSOHandler) ListProviders(w http.ResponseWriter, r *http.Request) {
	providers, err := h.providerStore.List()
	if err != nil {
		http.Error(w, "Failed to list providers", http.StatusInternalServerError)
		return
	}

	response := make([]*SSOProviderResponse, len(providers))
	for i, p := range providers {
		response[i] = h.providerToResponse(p)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetProvider returns a specific provider (admin only)
func (h *SSOHandler) GetProvider(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid provider ID", http.StatusBadRequest)
		return
	}

	provider, err := h.providerStore.GetByID(id)
	if err != nil {
		if err == sso.ErrProviderNotFound {
			http.Error(w, "Provider not found", http.StatusNotFound)
		} else {
			http.Error(w, "Failed to get provider", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h.providerToResponse(provider))
}

// CreateProvider creates a new SSO provider (admin only)
func (h *SSOHandler) CreateProvider(w http.ResponseWriter, r *http.Request) {
	var req SSOProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Slug == "" {
		http.Error(w, "Slug is required", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "Name is required", http.StatusBadRequest)
		return
	}
	if req.ProviderType == "" {
		req.ProviderType = sso.ProviderTypeOIDC
	}
	if req.ProviderType != sso.ProviderTypeOIDC {
		http.Error(w, "Only OIDC provider type is currently supported", http.StatusBadRequest)
		return
	}

	// Validate OIDC-specific fields
	if req.IssuerURL == "" {
		http.Error(w, "Issuer URL is required for OIDC providers", http.StatusBadRequest)
		return
	}
	if req.ClientID == "" {
		http.Error(w, "Client ID is required for OIDC providers", http.StatusBadRequest)
		return
	}
	if req.ClientSecret == "" {
		http.Error(w, "Client secret is required for OIDC providers", http.StatusBadRequest)
		return
	}

	// MVP: Only allow one provider
	count, err := h.providerStore.Count()
	if err != nil {
		http.Error(w, "Failed to check provider count", http.StatusInternalServerError)
		return
	}
	if count > 0 {
		http.Error(w, "Only one SSO provider is allowed in this version", http.StatusBadRequest)
		return
	}

	// Encrypt client secret
	encryptedSecret, err := h.encryption.Encrypt(req.ClientSecret)
	if err != nil {
		http.Error(w, "Failed to encrypt client secret", http.StatusInternalServerError)
		return
	}

	// Set default scopes if not provided
	if req.Scopes == "" {
		req.Scopes = "openid email profile"
	}

	// Default RequireVerifiedEmail to true for security
	requireVerifiedEmail := true
	if req.RequireVerifiedEmail != nil {
		requireVerifiedEmail = *req.RequireVerifiedEmail
	}

	provider := &sso.SSOProvider{
		Slug:                  req.Slug,
		Name:                  req.Name,
		ProviderType:          req.ProviderType,
		Enabled:               req.Enabled,
		IsDefault:             true, // First provider is always default
		IssuerURL:             req.IssuerURL,
		ClientID:              req.ClientID,
		ClientSecretEncrypted: encryptedSecret,
		Scopes:                req.Scopes,
		AutoProvisionUsers:    req.AutoProvisionUsers,
		AllowPasswordLogin:    req.AllowPasswordLogin,
		RequireVerifiedEmail:  requireVerifiedEmail,
		AttributeMapping:      req.AttributeMapping,
	}

	if err := h.providerStore.Create(provider); err != nil {
		if err == sso.ErrProviderExists {
			http.Error(w, "A provider with this slug already exists", http.StatusConflict)
		} else {
			http.Error(w, "Failed to create provider", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(h.providerToResponse(provider))
}

// UpdateProvider updates an existing provider (admin only)
func (h *SSOHandler) UpdateProvider(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid provider ID", http.StatusBadRequest)
		return
	}

	// Get existing provider
	existing, err := h.providerStore.GetByID(id)
	if err != nil {
		if err == sso.ErrProviderNotFound {
			http.Error(w, "Provider not found", http.StatusNotFound)
		} else {
			http.Error(w, "Failed to get provider", http.StatusInternalServerError)
		}
		return
	}

	var req SSOProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Update fields
	if req.Slug != "" {
		existing.Slug = req.Slug
	}
	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.IssuerURL != "" {
		existing.IssuerURL = req.IssuerURL
	}
	if req.ClientID != "" {
		existing.ClientID = req.ClientID
	}
	if req.Scopes != "" {
		existing.Scopes = req.Scopes
	}
	existing.Enabled = req.Enabled
	existing.IsDefault = req.IsDefault
	existing.AutoProvisionUsers = req.AutoProvisionUsers
	existing.AllowPasswordLogin = req.AllowPasswordLogin
	if req.RequireVerifiedEmail != nil {
		existing.RequireVerifiedEmail = *req.RequireVerifiedEmail
	}
	if req.AttributeMapping != "" {
		existing.AttributeMapping = req.AttributeMapping
	}

	if err := h.providerStore.Update(existing); err != nil {
		http.Error(w, "Failed to update provider", http.StatusInternalServerError)
		return
	}

	// Update secret if provided
	if req.ClientSecret != "" {
		encryptedSecret, err := h.encryption.Encrypt(req.ClientSecret)
		if err != nil {
			http.Error(w, "Failed to encrypt client secret", http.StatusInternalServerError)
			return
		}
		if err := h.providerStore.UpdateSecret(id, encryptedSecret); err != nil {
			http.Error(w, "Failed to update client secret", http.StatusInternalServerError)
			return
		}
		existing.ClientSecretEncrypted = encryptedSecret
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h.providerToResponse(existing))
}

// DeleteProvider deletes a provider (admin only)
func (h *SSOHandler) DeleteProvider(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid provider ID", http.StatusBadRequest)
		return
	}

	if err := h.providerStore.Delete(id); err != nil {
		if err == sso.ErrProviderNotFound {
			http.Error(w, "Provider not found", http.StatusNotFound)
		} else {
			http.Error(w, "Failed to delete provider", http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// TestProvider tests the connection to a provider (admin only)
func (h *SSOHandler) TestProvider(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid provider ID", http.StatusBadRequest)
		return
	}

	provider, err := h.providerStore.GetByID(id)
	if err != nil {
		if err == sso.ErrProviderNotFound {
			http.Error(w, "Provider not found", http.StatusNotFound)
		} else {
			http.Error(w, "Failed to get provider", http.StatusInternalServerError)
		}
		return
	}

	// Decrypt client secret
	clientSecret, err := h.encryption.Decrypt(provider.ClientSecretEncrypted)
	if err != nil {
		http.Error(w, "Failed to decrypt client secret", http.StatusInternalServerError)
		return
	}

	// Test connection
	ctx := context.Background()
	if err := h.oidcService.TestConnection(ctx, provider, clientSecret); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Successfully connected to OIDC provider",
	})
}

// GetExternalAccounts returns the external accounts linked to the current user
func (h *SSOHandler) GetExternalAccounts(w http.ResponseWriter, r *http.Request) {
	session, ok := r.Context().Value("session").(*auth.Session)
	if !ok || session == nil {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	accounts, err := h.userStore.GetExternalAccountsForUser(session.UserID)
	if err != nil {
		http.Error(w, "Failed to get external accounts", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(accounts)
}

// UnlinkExternalAccount removes a linked external account
func (h *SSOHandler) UnlinkExternalAccount(w http.ResponseWriter, r *http.Request) {
	session, ok := r.Context().Value("session").(*auth.Session)
	if !ok || session == nil {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	accountID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid account ID", http.StatusBadRequest)
		return
	}

	if err := h.userStore.UnlinkExternalAccount(accountID, session.UserID); err != nil {
		if err == sso.ErrExternalAccountNotFound {
			http.Error(w, "External account not found", http.StatusNotFound)
		} else {
			http.Error(w, "Failed to unlink account", http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Helper functions

func (h *SSOHandler) getRedirectURI(r *http.Request, slug string) string {
	// If BASE_URL is set, always use it (trusted source)
	if h.baseURL != "" {
		return strings.TrimSuffix(h.baseURL, "/") + "/api/sso/callback/" + slug
	}

	// Get host from request or forwarded header
	host := r.Host
	if forwardedHost := r.Header.Get("X-Forwarded-Host"); forwardedHost != "" {
		host = forwardedHost
	}

	// Validate host against allowed hosts (unless in dev mode)
	if !h.isAllowedHost(host) {
		slog.Warn("rejected untrusted host header", slog.String("component", "sso"), slog.String("host", host))
		// Fall back to first allowed host if available
		if len(h.allowedHosts) > 0 {
			host = h.allowedHosts[0]
		}
		// If no allowed hosts, continue with the request host but log warning
	}

	// Determine scheme - default to HTTPS for security
	scheme := "https"

	if h.devMode {
		// Dev mode: allow HTTP fallback for local development
		if r.TLS == nil {
			if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
				scheme = proto
			} else {
				scheme = "http"
			}
		}
	} else {
		// Production: only trust X-Forwarded-Proto if it says HTTPS
		// Otherwise, always use HTTPS (never fall back to HTTP)
		if proto := r.Header.Get("X-Forwarded-Proto"); proto == "https" {
			scheme = "https"
		}
		// Default remains HTTPS - never use HTTP in production
	}

	return fmt.Sprintf("%s://%s/api/sso/callback/%s", scheme, host, slug)
}

// isAllowedHost checks if a host is in the allowed hosts list
func (h *SSOHandler) isAllowedHost(host string) bool {
	// In dev mode, allow any host
	if h.devMode {
		return true
	}

	// If no allowed hosts configured, allow any (but we'll have logged a warning on startup)
	if len(h.allowedHosts) == 0 {
		return true
	}

	// Strip port for comparison
	hostOnly := strings.Split(host, ":")[0]
	for _, allowed := range h.allowedHosts {
		if strings.EqualFold(hostOnly, allowed) {
			return true
		}
	}
	return false
}

func (h *SSOHandler) redirectWithError(w http.ResponseWriter, r *http.Request, message string) {
	// Redirect to login page with URL-encoded error message to prevent injection
	encodedMessage := url.QueryEscape(message)
	http.Redirect(w, r, "/?sso_error="+encodedMessage, http.StatusFound)
}

func (h *SSOHandler) providerToResponse(p *sso.SSOProvider) *SSOProviderResponse {
	return &SSOProviderResponse{
		ID:                   p.ID,
		Slug:                 p.Slug,
		Name:                 p.Name,
		ProviderType:         p.ProviderType,
		Enabled:              p.Enabled,
		IsDefault:            p.IsDefault,
		IssuerURL:            p.IssuerURL,
		ClientID:             p.ClientID,
		HasClientSecret:      p.ClientSecretEncrypted != "",
		Scopes:               p.Scopes,
		AutoProvisionUsers:   p.AutoProvisionUsers,
		AllowPasswordLogin:   p.AllowPasswordLogin,
		RequireVerifiedEmail: p.RequireVerifiedEmail,
		AttributeMapping:     p.AttributeMapping,
		CreatedAt:            p.CreatedAt,
		UpdatedAt:            p.UpdatedAt,
	}
}

func generateRandomState() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

