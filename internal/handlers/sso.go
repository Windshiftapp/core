package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"windshift/internal/auth"
	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/middleware"
	"windshift/internal/plugins"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
	"windshift/internal/services"
	"windshift/internal/sso"
	"windshift/internal/utils"

	"github.com/zitadel/oidc/v3/pkg/client/rp"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"golang.org/x/crypto/hkdf"
)

// oidcErrorMessages maps OIDC error codes to safe user-facing messages
// This prevents reflected error injection from IdP error_description
var oidcErrorMessages = map[string]string{
	"access_denied":              "Access was denied by the identity provider",
	"invalid_request":            "Invalid authentication request",
	"unauthorized_client":        "Client not authorized for this operation",
	"unsupported_response_type":  "Unsupported response type",
	"invalid_scope":              "Invalid scope requested",
	"server_error":               "Identity provider encountered an error",
	"temporarily_unavailable":    "Identity provider is temporarily unavailable",
	"interaction_required":       "User interaction required",
	"login_required":             "Login required",
	"consent_required":           "Consent required",
	"account_selection_required": "Account selection required",
	"invalid_grant":              "Invalid or expired authorization code",
}

// ssoStateTokenTTL leaves enough time for slower identity-provider flows,
// including MFA and account-selection prompts, while keeping state short-lived.
const ssoStateTokenTTL = 15 * time.Minute

// SSOHandler handles SSO authentication endpoints
type SSOHandler struct {
	db                       database.Database
	sessionManager           *auth.SessionManager
	permissionService        *services.PermissionService
	emailVerificationService *services.EmailVerificationService
	pluginManager            *plugins.Manager
	providerStore            *sso.ProviderStore
	userStore                *sso.UserStore
	oidcService              *sso.OIDCService
	encryption               *sso.SecretEncryption
	baseURL                  string             // Base URL of the application (e.g., https://app.example.com)
	allowedHosts             []string           // Allowed hosts for redirect URI validation (from --allowed-hosts)
	devMode                  bool               // Development mode (from --no-csrf flag)
	ipExtractor              *utils.IPExtractor // IP extractor with proxy validation
	useProxy                 bool               // Whether proxy mode is enabled
	additionalProxies        []net.IP           // Additional trusted proxy IPs beyond private ranges
}

// SSOStatusProviderInfo represents a single provider in the public status response
type SSOStatusProviderInfo struct {
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	ProviderType string `json:"provider_type"`
}

// SSOStatusResponse represents the public SSO status
type SSOStatusResponse struct {
	Enabled      bool                    `json:"enabled"`
	ProviderName string                  `json:"provider_name,omitempty"`
	ProviderSlug string                  `json:"provider_slug,omitempty"`
	Providers    []SSOStatusProviderInfo `json:"providers,omitempty"`
}

// SSOProviderResponse represents a provider for API responses (without secrets)
type SSOProviderResponse struct {
	ID                   int    `json:"id"`
	Slug                 string `json:"slug"`
	Name                 string `json:"name"`
	ProviderType         string `json:"provider_type"`
	Enabled              bool   `json:"enabled"`
	IsDefault            bool   `json:"is_default"`
	IssuerURL            string `json:"issuer_url,omitempty"`
	ClientID             string `json:"client_id,omitempty"`
	HasClientSecret      bool   `json:"has_client_secret"`
	Scopes               string `json:"scopes"`
	AutoProvisionUsers   bool   `json:"auto_provision_users"`
	RequireVerifiedEmail bool   `json:"require_verified_email"` // Trust provider email when verification status is absent.
	AttributeMapping     string `json:"attribute_mapping"`
	// SAML-specific fields
	SAMLIdPMetadataURL string    `json:"saml_idp_metadata_url,omitempty"`
	SAMLIdPSSOURL      string    `json:"saml_idp_sso_url,omitempty"`
	HasSAMLIdPCert     bool      `json:"has_saml_idp_certificate"`
	SAMLSPEntityID     string    `json:"saml_sp_entity_id,omitempty"`
	SAMLSignRequests   bool      `json:"saml_sign_requests"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
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
	RequireVerifiedEmail *bool  `json:"require_verified_email"` // Pointer distinguishes false from an omitted trust setting.
	AttributeMapping     string `json:"attribute_mapping"`
	// SAML-specific fields
	SAMLIdPMetadataURL string `json:"saml_idp_metadata_url,omitempty"`
	SAMLIdPSSOURL      string `json:"saml_idp_sso_url,omitempty"`
	SAMLIdPCertificate string `json:"saml_idp_certificate,omitempty"`
	SAMLSPEntityID     string `json:"saml_sp_entity_id,omitempty"`
	SAMLSignRequests   bool   `json:"saml_sign_requests"`
}

// sanitizeSSOProviderRequest bounds the user-supplied text fields shared by
// CreateProvider and UpdateProvider. ProviderType is a strict enum and
// ClientSecret is an opaque machine token — both stay untouched.
// AttributeMapping is a JSON blob — HTML stripping would corrupt it, so
// it is validated (size + well-formed JSON) and rejected instead of
// scrubbed. Writes a validation error and returns false when it is
// rejected.
func sanitizeSSOProviderRequest(w http.ResponseWriter, r *http.Request, req *SSOProviderRequest) bool {
	sanitize.ApplyAll(
		sanitize.Pair{Target: &req.Slug, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: &req.Name, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &req.IssuerURL, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &req.ClientID, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: &req.Scopes, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &req.SAMLIdPMetadataURL, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &req.SAMLIdPSSOURL, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &req.SAMLIdPCertificate, Policy: sanitize.RichText},
		sanitize.Pair{Target: &req.SAMLSPEntityID, Policy: sanitize.PlainTextField},
	)
	if err := sanitize.ValidateJSONPayload("attribute_mapping", req.AttributeMapping); err != nil {
		respondValidationError(w, r, err.Error())
		return false
	}
	return true
}

// NewSSOHandler creates a new SSO handler.
// sessionSecret: session-signing secret (resolved by config.Load from
//
//	SSO_SECRET with SESSION_SECRET fallback). Must be non-empty.
//
// baseURL: public URL of the application (used for redirect URIs).
// allowedHostsStr: comma-separated list of allowed hosts from --allowed-hosts flag
// devMode: true if --no-csrf flag is set (development mode)
// emailVerificationService: service for handling email verification (can be nil if SMTP not configured)
// pluginManager: plugin manager for capability checks (can be nil)
// useProxy: whether to trust proxy headers from trusted sources
// additionalProxiesStr: comma-separated list of additional trusted proxy IPs
func NewSSOHandler(db database.Database, sessionManager *auth.SessionManager, permissionService *services.PermissionService, emailVerificationService *services.EmailVerificationService, pluginManager *plugins.Manager, sessionSecret, baseURL, allowedHostsStr string, devMode bool, ipExtractor *utils.IPExtractor, useProxy bool, additionalProxiesStr []string) *SSOHandler {
	// Defensive: config.Load guarantees non-empty, but a wiring bug upstream
	// would silently break session encryption — fail fast instead.
	if sessionSecret == "" {
		log.Fatal("FATAL: NewSSOHandler received empty session secret (config wiring bug)")
	}
	serverSecret := sessionSecret

	// Parse allowed hosts
	var allowedHosts []string
	if allowedHostsStr != "" {
		for _, h := range strings.Split(allowedHostsStr, ",") {
			if trimmed := strings.TrimSpace(h); trimmed != "" {
				allowedHosts = append(allowedHosts, trimmed)
			}
		}
	}

	// Parse additional proxy IPs (beyond auto-trusted private ranges)
	var additionalProxies []net.IP
	for _, proxyStr := range additionalProxiesStr {
		if ip := net.ParseIP(strings.TrimSpace(proxyStr)); ip != nil {
			additionalProxies = append(additionalProxies, ip)
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

	// Derive a 32-byte cookie key using HKDF (HMAC-based Key Derivation Function)
	// This ensures proper key derivation even with short secrets, unlike direct byte copy
	hkdfReader := hkdf.New(sha256.New, []byte(serverSecret), nil, []byte("windshift-sso-cookie-key-v1"))
	cookieKey := make([]byte, 32)
	if _, err := io.ReadFull(hkdfReader, cookieKey); err != nil {
		log.Fatal("FATAL: Failed to derive cookie encryption key")
	}

	return &SSOHandler{
		db:                       db,
		sessionManager:           sessionManager,
		permissionService:        permissionService,
		emailVerificationService: emailVerificationService,
		pluginManager:            pluginManager,
		providerStore:            sso.NewProviderStore(db),
		userStore:                sso.NewUserStore(db),
		oidcService:              sso.NewOIDCService(cookieKey),
		encryption:               sso.NewSecretEncryption(serverSecret),
		baseURL:                  baseURL,
		allowedHosts:             allowedHosts,
		devMode:                  devMode,
		ipExtractor:              ipExtractor,
		useProxy:                 useProxy,
		additionalProxies:        additionalProxies,
	}
}

// SetPluginManager supplies the plugin manager after its dependencies are initialized.
func (h *SSOHandler) SetPluginManager(pluginManager *plugins.Manager) {
	h.pluginManager = pluginManager
}

// GetStatus returns the public SSO status (no auth required)
func (h *SSOHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	providers, err := h.providerStore.ListEnabled()
	if err != nil || len(providers) == 0 {
		respondJSONOK(w, SSOStatusResponse{
			Enabled: false,
		})
		return
	}

	// Build providers list for the response
	providerInfos := make([]SSOStatusProviderInfo, len(providers))
	for i, p := range providers {
		providerInfos[i] = SSOStatusProviderInfo{
			Name:         p.Name,
			Slug:         p.Slug,
			ProviderType: p.ProviderType,
		}
	}

	// First provider is the default (ListEnabled orders by is_default DESC)
	defaultProvider := providers[0]

	respondJSONOK(w, SSOStatusResponse{
		Enabled:      true,
		ProviderName: defaultProvider.Name,
		ProviderSlug: defaultProvider.Slug,
		Providers:    providerInfos,
	})
}

// StartLogin initiates the SSO login flow
func (h *SSOHandler) StartLogin(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	provider, err := h.providerStore.GetBySlug(slug)
	if err != nil {
		respondNotFound(w, r, "SSO provider")
		return
	}

	if !provider.Enabled {
		respondBadRequest(w, r, "SSO provider is disabled")
		return
	}

	if provider.ProviderType != sso.ProviderTypeOIDC {
		respondBadRequest(w, r, "Provider type not supported")
		return
	}

	// Extract and validate redirect_uri (where to return after login)
	redirectAfterLogin := r.URL.Query().Get("redirect_uri")
	if redirectAfterLogin == "" {
		redirectAfterLogin = "/"
	}
	// The desktop/native client passes its custom-scheme callback
	// (windshift://oauth/callback); the web app passes a same-origin relative
	// path. Anything matching neither falls back to "/". The native value is
	// stored verbatim in the state row and detected again in Callback.
	if !isValidRedirectURI(redirectAfterLogin) && !isValidNativeRedirectURI(redirectAfterLogin) {
		redirectAfterLogin = "/"
	}

	rememberMe := ssoRememberMeFromRequest(r)
	slog.Info("starting OIDC login",
		slog.String("component", "sso"),
		slog.String("provider", provider.Slug),
		slog.Bool("remember_me", rememberMe),
	)

	// Decrypt client secret
	clientSecret, err := h.encryption.Decrypt(provider.ClientSecretEncrypted)
	if err != nil {
		slog.Error("failed to decrypt client secret", slog.String("component", "sso"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	// Determine redirect URI
	redirectURI := h.getRedirectURI(r, slug)

	// Create relying party
	ctx := r.Context()
	relyingParty, err := h.oidcService.CreateRelyingParty(ctx, provider, redirectURI, clientSecret)
	if err != nil {
		slog.Error("failed to create relying party", slog.String("component", "sso"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	// Generate state with random data
	state := generateRandomState()

	// Store state token with redirect_uri and remember_me
	storeErr := repository.NewSSOStateRepository(h.db).Store(provider.ID, state, "", redirectAfterLogin, rememberMe, time.Now().Add(ssoStateTokenTTL))
	if storeErr != nil {
		slog.Error("failed to store OIDC state token", "error", storeErr)
		respondInternalError(w, r, storeErr)
		return
	}

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

	// Reject missing, expired, replayed, or provider-mismatched application
	// state before doing OIDC discovery. The relying-party library still
	// validates its encrypted state cookie during code exchange; this early
	// application-level check keeps invalid callbacks fail-closed and gives the
	// browser a useful recovery message even when the IdP is unavailable.
	state := r.URL.Query().Get("state")
	if state == "" {
		h.redirectWithError(w, r, "Login session expired. Please try signing in again.")
		return
	}
	if _, stateErr := repository.NewSSOStateRepository(h.db).GetValid(state, provider.ID, time.Now()); stateErr != nil {
		slog.Warn("OIDC callback state token missing or expired",
			slog.String("component", "sso"),
			slog.String("provider", provider.Slug))
		h.redirectWithError(w, r, "Login session expired. Please try signing in again.")
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
	ctx := r.Context()
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
		// Retrieve stored state data (redirect_uri, remember_me).
		//
		// The library has already verified `state` matches its RP-bound value
		// before invoking this callback, so an arbitrary attacker can't fake
		// a valid state. The DB lookup is here for application-level metadata
		// (where to redirect, remember-me flag) and as a defense-in-depth layer
		// against state replay across deployments. If the row is missing —
		// most likely the state expiry elapsed — fail closed instead of
		// silently defaulting to "/", so the user gets a retry prompt and the
		// audit log carries a discrete signal.
		token, stateErr := repository.NewSSOStateRepository(h.db).GetValid(state, provider.ID, time.Now())
		if stateErr != nil {
			slog.Warn("OIDC state token missing or expired", slog.String("component", "sso"), slog.String("provider", provider.Slug))
			h.redirectWithError(w, r, "Login session expired. Please try signing in again.")
			return
		}
		_ = repository.NewSSOStateRepository(h.db).Delete(token.ID)
		storedRedirectURI := token.RedirectURI
		rememberMe := token.RememberMe
		slog.Info("restored OIDC login state",
			slog.String("component", "sso"),
			slog.String("provider", provider.Slug),
			slog.Bool("remember_me", rememberMe),
		)
		// A native (desktop/iOS) flow is identified by its custom-scheme
		// redirect target, stored in StartLogin. Web flows keep the existing
		// relative-path coercion; the native target is left intact so the
		// branch below can hand back a one-time code instead of a cookie.
		nativeFlow := isValidNativeRedirectURI(storedRedirectURI)
		if !nativeFlow && (storedRedirectURI == "" || !isValidRedirectURI(storedRedirectURI)) {
			storedRedirectURI = "/"
		}

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
			switch {
			case err == sso.ErrAutoProvisionDisabled:
				h.redirectWithError(w, r, "User account not found. Contact your administrator.")
			case errors.Is(err, sso.ErrAccountLinkingRequiresVerification):
				h.redirectWithError(w, r, "Cannot link to existing account: your identity provider must verify your email address first")
			default:
				h.redirectWithError(w, r, "Failed to process user account")
			}
			return
		}

		user := result.User

		// If IdP verified the email, update our DB to reflect that
		if !result.NeedsEmailVerification && !user.EmailVerified {
			if h.emailVerificationService != nil {
				if err := h.emailVerificationService.SetEmailVerified(user.ID, true); err != nil {
					slog.Warn("failed to set email verified from IdP", slog.String("component", "sso"), slog.Int("user_id", user.ID), slog.Any("error", err))
				} else {
					user.EmailVerified = true
				}
			}
		}

		// Check if user is active
		if !user.IsActive {
			h.redirectWithError(w, r, "Account is disabled")
			return
		}

		// Handle email verification if needed
		if result.NeedsEmailVerification && !user.EmailVerified {
			// User needs email verification - send verification email
			if h.emailVerificationService != nil {
				var token string
				token, err = h.emailVerificationService.GenerateVerificationToken(user.ID)
				if err != nil {
					slog.Warn("failed to generate verification token", slog.String("component", "sso"), slog.Int("user_id", user.ID), slog.Any("error", err))
					// Continue with login but log the error
				} else {
					if err = h.emailVerificationService.SendVerificationEmail(user, token); err != nil {
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
		slog.Info("creating OIDC session",
			slog.String("component", "sso"),
			slog.String("provider", provider.Slug),
			slog.Int("user_id", user.ID),
			slog.Bool("remember_me", rememberMe),
		)
		session, err := h.sessionManager.CreateSession(user.ID, ipAddress, r.UserAgent(), rememberMe)
		if err != nil {
			slog.Error("failed to create session", slog.String("component", "sso"), slog.Any("error", err))
			h.redirectWithError(w, r, "Failed to create session")
			return
		}
		slog.Debug("session created", slog.String("component", "sso"))

		// Native clients can't receive an HttpOnly cookie through a custom-scheme
		// redirect, so hand back a short-lived one-time code instead and let the
		// app redeem it for the encoded session cookie at /api/auth/native/exchange
		// (mirrors the ws-init browser→CLI handoff). No session token ever rides in
		// the URL — only the opaque code.
		if nativeFlow {
			code, codeErr := h.issueNativeAuthCode(session.Token, session.ExpiresAt)
			if codeErr != nil {
				slog.Error("failed to issue native auth code", slog.String("component", "sso"), slog.Any("error", codeErr))
				h.redirectWithError(w, r, "Failed to complete sign-in")
				return
			}
			target := nativeRedirectURI + "?code=" + url.QueryEscape(code)
			slog.Debug("redirecting to native callback", slog.String("component", "sso"))
			http.Redirect(w, r, target, http.StatusFound) // #nosec G710 -- fixed custom-scheme target; code is opaque + single-use
			return
		}

		// Set a browser session cookie and redirect only to a validated relative
		// path. Do not return session tokens in redirect fragments; the v1 REST API
		// accepts only scoped crw_* API tokens, not web session tokens.
		slog.Debug("setting session cookie", slog.String("component", "sso"))
		if err := h.sessionManager.SetSessionCookie(w, r, session.Token, rememberMe); err != nil {
			slog.Error("failed to set session cookie", slog.String("component", "sso"), slog.Any("error", err))
			h.redirectWithError(w, r, "Failed to set session")
			return
		}
		slog.Debug("session cookie set, redirecting", slog.String("component", "sso"))

		// Redirect based on email verification status
		if result.NeedsEmailVerification && !user.EmailVerified {
			http.Redirect(w, r, "/?verify_email=pending", http.StatusFound)
		} else {
			// storedRedirectURI is validated above by isValidRedirectURI (relative path only).
			http.Redirect(w, r, storedRedirectURI, http.StatusFound) // #nosec G710
		}
	})

	callbackHandler(w, r)
}

// nativeAuthCodeTTL bounds how long a native-client one-time code can sit
// waiting to be redeemed. Short window because the desktop app exchanges it
// within seconds of receiving the custom-scheme callback.
const nativeAuthCodeTTL = 2 * time.Minute

// issueNativeAuthCode persists a single-use code bound to a freshly created
// session and returns it. The desktop/native app redeems it at
// /api/auth/native/exchange for the encoded session cookie value.
func (h *SSOHandler) issueNativeAuthCode(sessionToken string, sessionExpiresAt time.Time) (string, error) {
	code, err := randomOpaqueCode()
	if err != nil {
		return "", err
	}
	expires := time.Now().Add(nativeAuthCodeTTL)
	if err := repository.NewNativeAuthRepository(h.db).Store(code, sessionToken, sessionExpiresAt, expires); err != nil {
		return "", err
	}
	return code, nil
}

// nativeExchangeRequest is the payload the native app POSTs to redeem its code.
type nativeExchangeRequest struct {
	Code string `json:"code"`
}

// nativeExchangeResponse hands the app everything it needs to write the session
// cookie into its own webview's cookie store. CookieValue is the
// securecookie-encoded blob — opaque and tamper-proof; the app cannot produce
// it itself (it has no server secret).
type nativeExchangeResponse struct {
	CookieName  string    `json:"cookie_name"`
	CookieValue string    `json:"cookie_value"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// NativeExchange redeems the one-time code minted by the native SSO callback
// and returns the encoded session cookie. Single-use: the code is consumed
// atomically, so a replay returns 400. Public (allowlisted) because the app
// has no credentials yet at this point in the flow.
func (h *SSOHandler) NativeExchange(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[nativeExchangeRequest](w, r)
	if !ok {
		return
	}
	if strings.TrimSpace(req.Code) == "" {
		respondBadRequest(w, r, "code is required")
		return
	}

	row, err := repository.NewNativeAuthRepository(h.db).Consume(req.Code, time.Now())
	if errors.Is(err, repository.ErrNotFound) {
		respondBadRequest(w, r, "invalid or expired code")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	value, err := h.sessionManager.EncodeSessionCookieValue(row.SessionToken)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	slog.Info("native auth code exchanged", slog.String("component", "sso"))
	respondJSONOK(w, nativeExchangeResponse{
		CookieName:  auth.SessionCookieName,
		CookieValue: value,
		ExpiresAt:   row.SessionExpiresAt,
	})
}

// ListProviders returns all SSO providers (admin only)
func (h *SSOHandler) ListProviders(w http.ResponseWriter, r *http.Request) {
	providers, err := h.providerStore.List()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	response := make([]*SSOProviderResponse, len(providers))
	for i, p := range providers {
		response[i] = h.providerToResponse(p)
	}

	respondJSONOK(w, response)
}

// GetProvider returns a specific provider (admin only)
func (h *SSOHandler) GetProvider(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	provider, err := h.providerStore.GetByID(id)
	if err != nil {
		if err == sso.ErrProviderNotFound {
			respondNotFound(w, r, "provider")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	respondJSONOK(w, h.providerToResponse(provider))
}

// CreateProvider creates a new SSO provider (admin only)
func (h *SSOHandler) CreateProvider(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[SSOProviderRequest](w, r)
	if !ok {
		return
	}
	if !sanitizeSSOProviderRequest(w, r, &req) {
		return
	}

	// Validate required fields
	if req.Slug == "" {
		respondValidationError(w, r, "Slug is required")
		return
	}
	if req.Name == "" {
		respondValidationError(w, r, "Name is required")
		return
	}
	if req.ProviderType == "" {
		req.ProviderType = sso.ProviderTypeOIDC
	}
	if req.ProviderType != sso.ProviderTypeOIDC && req.ProviderType != sso.ProviderTypeSAML {
		respondValidationError(w, r, "Provider type must be 'oidc' or 'saml'")
		return
	}

	// Validate type-specific fields
	var encryptedSecret string
	switch req.ProviderType {
	case sso.ProviderTypeOIDC:
		if req.IssuerURL == "" {
			respondValidationError(w, r, "Issuer URL is required for OIDC providers")
			return
		}
		if err := utils.ValidateExternalURL(req.IssuerURL); err != nil {
			respondValidationError(w, r, fmt.Sprintf("Issuer URL is not a valid public HTTPS endpoint: %s", err.Error()))
			return
		}
		if req.ClientID == "" {
			respondValidationError(w, r, "Client ID is required for OIDC providers")
			return
		}
		if req.ClientSecret == "" {
			respondValidationError(w, r, "Client secret is required for OIDC providers")
			return
		}
	case sso.ProviderTypeSAML:
		if req.SAMLIdPMetadataURL == "" && req.SAMLIdPSSOURL == "" {
			respondValidationError(w, r, "Either IdP metadata URL or IdP SSO URL is required for SAML providers")
			return
		}
		if req.SAMLIdPSSOURL != "" && req.SAMLIdPCertificate == "" && req.SAMLIdPMetadataURL == "" {
			respondValidationError(w, r, "IdP certificate is required when configuring SAML manually (without metadata URL)")
			return
		}
	}

	// Determine if this should be the default provider (first provider = default)
	count, err := h.providerStore.Count()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Gate: require sso.multi-provider capability to add more than one provider
	if count >= 1 && (h.pluginManager == nil || !h.pluginManager.HasCapability("sso.multi-provider")) {
		respondJSON(w, http.StatusForbidden, map[string]any{
			"error": "Multiple SSO providers require the sso.multi-provider capability",
			"code":  "CAPABILITY_REQUIRED",
		})
		return
	}

	isDefault := count == 0 || req.IsDefault

	// Encrypt client secret if provided (OIDC)
	if req.ClientSecret != "" {
		var encErr error
		encryptedSecret, encErr = h.encryption.Encrypt(req.ClientSecret)
		if encErr != nil {
			respondInternalError(w, r, encErr)
			return
		}
	}

	// Set default scopes if not provided (OIDC)
	if req.ProviderType == sso.ProviderTypeOIDC && req.Scopes == "" {
		req.Scopes = "openid email profile"
	}

	// Preserve the existing default: trust provider-managed email when the
	// provider omits verification status.
	trustProviderEmail := true
	if req.RequireVerifiedEmail != nil {
		trustProviderEmail = *req.RequireVerifiedEmail
	}

	provider := &sso.SSOProvider{
		Slug:                  req.Slug,
		Name:                  req.Name,
		ProviderType:          req.ProviderType,
		Enabled:               req.Enabled,
		IsDefault:             isDefault,
		IssuerURL:             req.IssuerURL,
		ClientID:              req.ClientID,
		ClientSecretEncrypted: encryptedSecret,
		Scopes:                req.Scopes,
		AutoProvisionUsers:    req.AutoProvisionUsers,
		RequireVerifiedEmail:  trustProviderEmail,
		AttributeMapping:      req.AttributeMapping,
		SAMLIdPMetadataURL:    req.SAMLIdPMetadataURL,
		SAMLIdPSSOURL:         req.SAMLIdPSSOURL,
		SAMLIdPCertificate:    req.SAMLIdPCertificate,
		SAMLSPEntityID:        req.SAMLSPEntityID,
		SAMLSignRequests:      req.SAMLSignRequests,
	}

	if err := h.providerStore.Create(provider); err != nil {
		if err == sso.ErrProviderExists {
			respondConflict(w, r, "A provider with this slug already exists")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	if provider.RequireVerifiedEmail {
		slog.Warn("SSO provider saved with provider-managed email trust enabled — a provider that lets users self-assert email addresses can link identities to existing accounts",
			slog.String("component", "sso"),
			slog.String("provider", provider.Slug),
			slog.Int("provider_id", provider.ID))
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionSSOProviderCreate,
			ResourceType: logger.ResourceSSOProvider,
			ResourceID:   &provider.ID,
			ResourceName: provider.Name,
			Success:      true,
		})
	}

	respondJSONCreated(w, h.providerToResponse(provider))
}

// UpdateProvider updates an existing provider (admin only)
func (h *SSOHandler) UpdateProvider(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get existing provider
	existing, err := h.providerStore.GetByID(id)
	if err != nil {
		if err == sso.ErrProviderNotFound {
			respondNotFound(w, r, "provider")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	req, ok := decodeJSON[SSOProviderRequest](w, r)
	if !ok {
		return
	}
	if !sanitizeSSOProviderRequest(w, r, &req) {
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
		if existing.ProviderType == sso.ProviderTypeOIDC {
			if err := utils.ValidateExternalURL(req.IssuerURL); err != nil {
				respondValidationError(w, r, fmt.Sprintf("Issuer URL is not a valid public HTTPS endpoint: %s", err.Error()))
				return
			}
		}
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
	if req.RequireVerifiedEmail != nil {
		existing.RequireVerifiedEmail = *req.RequireVerifiedEmail
	}
	if req.AttributeMapping != "" {
		existing.AttributeMapping = req.AttributeMapping
	}
	// Update SAML-specific fields
	if req.SAMLIdPMetadataURL != "" {
		existing.SAMLIdPMetadataURL = req.SAMLIdPMetadataURL
	}
	if req.SAMLIdPSSOURL != "" {
		existing.SAMLIdPSSOURL = req.SAMLIdPSSOURL
	}
	if req.SAMLIdPCertificate != "" {
		existing.SAMLIdPCertificate = req.SAMLIdPCertificate
	}
	if req.SAMLSPEntityID != "" {
		existing.SAMLSPEntityID = req.SAMLSPEntityID
	}
	existing.SAMLSignRequests = req.SAMLSignRequests

	if err := h.providerStore.Update(existing); err != nil {
		respondInternalError(w, r, err)
		return
	}

	if existing.RequireVerifiedEmail {
		slog.Warn("SSO provider saved with provider-managed email trust enabled — a provider that lets users self-assert email addresses can link identities to existing accounts",
			slog.String("component", "sso"),
			slog.String("provider", existing.Slug),
			slog.Int("provider_id", existing.ID))
	}

	// Update secret if provided
	if req.ClientSecret != "" {
		encryptedSecret, err := h.encryption.Encrypt(req.ClientSecret)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if err := h.providerStore.UpdateSecret(id, encryptedSecret); err != nil {
			respondInternalError(w, r, err)
			return
		}
		existing.ClientSecretEncrypted = encryptedSecret
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionSSOProviderUpdate,
			ResourceType: logger.ResourceSSOProvider,
			ResourceID:   &id,
			ResourceName: existing.Name,
			Success:      true,
		})
	}

	respondJSONOK(w, h.providerToResponse(existing))
}

// DeleteProvider deletes a provider (admin only)
func (h *SSOHandler) DeleteProvider(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Fetch provider name before deletion for audit logging
	provider, _ := h.providerStore.GetByID(id)

	if err := h.providerStore.Delete(id); err != nil {
		if err == sso.ErrProviderNotFound {
			respondNotFound(w, r, "provider")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		providerName := ""
		if provider != nil {
			providerName = provider.Name
		}
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionSSOProviderDelete,
			ResourceType: logger.ResourceSSOProvider,
			ResourceID:   &id,
			ResourceName: providerName,
			Success:      true,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

// TestProvider tests the connection to a provider (admin only)
func (h *SSOHandler) TestProvider(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	provider, err := h.providerStore.GetByID(id)
	if err != nil {
		if err == sso.ErrProviderNotFound {
			respondNotFound(w, r, "provider")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	// Decrypt client secret
	clientSecret, err := h.encryption.Decrypt(provider.ClientSecretEncrypted)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Test connection
	ctx := r.Context()
	if err := h.oidcService.TestConnection(ctx, provider, clientSecret); err != nil {
		// Log detailed error server-side for debugging
		slog.Error("OIDC test connection failed",
			slog.String("component", "sso"),
			slog.Int("provider_id", id),
			slog.Any("error", err))

		// Return a safe, generic error message to prevent information leakage
		// Raw errors may contain internal paths, IP addresses, or other sensitive info
		safeMessage := "Failed to connect to OIDC provider. Check issuer URL and client credentials."
		if errors.Is(err, sso.ErrOIDCDiscoveryFailed) {
			safeMessage = "OIDC discovery failed. Verify the issuer URL is correct and accessible."
		}

		respondJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"error":   safeMessage,
		})
		return
	}

	respondJSONOK(w, map[string]any{
		"success": true,
		"message": "Successfully connected to OIDC provider",
	})
}

// GetExternalAccounts returns the external accounts linked to the current user
func (h *SSOHandler) GetExternalAccounts(w http.ResponseWriter, r *http.Request) {
	session, ok := r.Context().Value(middleware.ContextKeySession).(*auth.Session)
	if !ok || session == nil {
		respondUnauthorized(w, r)
		return
	}

	accounts, err := h.userStore.GetExternalAccountsForUser(session.UserID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, accounts)
}

// UnlinkExternalAccount removes a linked external account
func (h *SSOHandler) UnlinkExternalAccount(w http.ResponseWriter, r *http.Request) {
	session, ok := r.Context().Value(middleware.ContextKeySession).(*auth.Session)
	if !ok || session == nil {
		respondUnauthorized(w, r)
		return
	}

	accountID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.userStore.UnlinkExternalAccount(accountID, session.UserID); err != nil {
		if err == sso.ErrExternalAccountNotFound {
			respondNotFound(w, r, "external account")
		} else {
			respondInternalError(w, r, err)
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

	// Get host from request
	host := r.Host

	// Only trust X-Forwarded-Host from trusted proxy IPs
	// This prevents header injection attacks from untrusted sources
	if h.isTrustedRequest(r) {
		if forwardedHost := r.Header.Get("X-Forwarded-Host"); forwardedHost != "" {
			host = forwardedHost
		}
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
			// Only trust X-Forwarded-Proto from trusted proxies
			if h.isTrustedRequest(r) {
				if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
					scheme = proto
				} else {
					scheme = "http"
				}
			} else {
				scheme = "http"
			}
		}
	} else {
		// Production: only trust X-Forwarded-Proto from trusted proxies
		// Otherwise, always use HTTPS (never fall back to HTTP)
		if h.isTrustedRequest(r) {
			if proto := r.Header.Get("X-Forwarded-Proto"); proto == "https" {
				scheme = "https"
			}
		}
		// Default remains HTTPS - never use HTTP in production
	}

	return fmt.Sprintf("%s://%s%s/api/sso/callback/%s", scheme, host, requestContextPrefix(r), slug)
}

// isTrustedRequest checks if the request comes from a trusted proxy
func (h *SSOHandler) isTrustedRequest(r *http.Request) bool {
	if !h.useProxy {
		return false // Proxy mode disabled - trust nothing
	}

	// Get the immediate client IP (could be proxy)
	remoteAddr := r.RemoteAddr
	if colonIndex := strings.LastIndex(remoteAddr, ":"); colonIndex != -1 {
		remoteAddr = remoteAddr[:colonIndex]
	}

	clientIP := net.ParseIP(remoteAddr)
	if clientIP == nil {
		return false
	}

	return utils.IsTrustedProxy(clientIP, h.useProxy, h.additionalProxies)
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
		RequireVerifiedEmail: p.RequireVerifiedEmail,
		AttributeMapping:     p.AttributeMapping,
		SAMLIdPMetadataURL:   p.SAMLIdPMetadataURL,
		SAMLIdPSSOURL:        p.SAMLIdPSSOURL,
		HasSAMLIdPCert:       p.SAMLIdPCertificate != "",
		SAMLSPEntityID:       p.SAMLSPEntityID,
		SAMLSignRequests:     p.SAMLSignRequests,
		CreatedAt:            p.CreatedAt,
		UpdatedAt:            p.UpdatedAt,
	}
}

func ssoRememberMeFromRequest(r *http.Request) bool {
	query := r.URL.Query()
	return query.Get("remember_me") == "true" || query.Get("remember") == "true"
}

// GetEncryption returns the encryption service (for reuse by LDAP handler).
func (h *SSOHandler) GetEncryption() *sso.SecretEncryption {
	return h.encryption
}

func generateRandomState() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}
