package handlers

import (
	"encoding/xml"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"windshift/internal/logger"
	"windshift/internal/repository"
	"windshift/internal/sso"
)

// SAMLMetadata serves the SAML SP metadata XML for a given provider.
// GET /api/sso/{slug}/saml/metadata
func (h *SSOHandler) SAMLMetadata(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if slug == "" {
		respondBadRequest(w, r, "Provider slug is required")
		return
	}

	provider, err := h.providerStore.GetBySlug(slug)
	if err != nil {
		respondNotFound(w, r, "provider")
		return
	}

	if provider.ProviderType != sso.ProviderTypeSAML {
		respondBadRequest(w, r, "Provider is not a SAML provider")
		return
	}

	baseURL := h.getBaseURL(r)
	sp, err := sso.NewSAMLServiceProvider(provider, baseURL)
	if err != nil {
		slog.Error("failed to create SAML SP", "error", err, "provider", slug)
		respondInternalError(w, r, err)
		return
	}

	metadata := sp.Metadata()
	xmlBytes, err := xml.MarshalIndent(metadata, "", "  ")
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/samlmetadata+xml")
	w.Header().Set("Content-Disposition", "attachment; filename=\"metadata.xml\"")
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(xmlBytes) //nolint:gosec // G705: server-generated SAML metadata XML, not user content
}

// requireSAMLProvider validates the slug path parameter, fetches the SSO provider,
// checks that it is enabled and of SAML type, and creates a SAMLServiceProvider.
// On failure it calls redirectWithError and returns false.
func (h *SSOHandler) requireSAMLProvider(w http.ResponseWriter, r *http.Request) (*sso.SSOProvider, *sso.SAMLServiceProvider, bool) {
	slug := r.PathValue("slug")
	if slug == "" {
		h.redirectWithError(w, r, "Provider slug is required")
		return nil, nil, false
	}

	provider, err := h.providerStore.GetBySlug(slug)
	if err != nil {
		h.redirectWithError(w, r, "SSO provider not found")
		return nil, nil, false
	}

	if !provider.Enabled {
		h.redirectWithError(w, r, "SSO provider is disabled")
		return nil, nil, false
	}

	if provider.ProviderType != sso.ProviderTypeSAML {
		h.redirectWithError(w, r, "Provider is not a SAML provider")
		return nil, nil, false
	}

	baseURL := h.getBaseURL(r)
	sp, err := sso.NewSAMLServiceProvider(provider, baseURL)
	if err != nil {
		slog.Error("failed to create SAML SP", "error", err, "provider", slug)
		h.redirectWithError(w, r, "SSO configuration error")
		return nil, nil, false
	}

	return provider, sp, true
}

// SAMLLogin initiates a SAML authentication flow by redirecting to the IdP.
// GET /api/sso/{slug}/saml/login
func (h *SSOHandler) SAMLLogin(w http.ResponseWriter, r *http.Request) {
	provider, sp, ok := h.requireSAMLProvider(w, r)
	if !ok {
		return
	}

	// Generate relay state containing a CSRF state token
	state := generateRandomState()
	rememberMe := ssoRememberMeFromRequest(r)
	slog.Info("starting SAML login",
		slog.String("component", "sso"),
		slog.String("provider", provider.Slug),
		slog.Bool("remember_me", rememberMe),
	)

	redirectURI := r.URL.Query().Get("redirect_uri")
	if redirectURI == "" {
		redirectURI = "/"
	}

	// Validate redirect_uri to prevent open redirect attacks - only allow relative paths
	if !isValidRedirectURI(redirectURI) {
		redirectURI = "/"
	}

	// Create AuthnRequest first so its ID can be bound to the state token; the
	// ACS handler requires the returned assertion's InResponseTo to match it.
	redirectURL, requestID, err := sp.MakeAuthenticationRequest(state)
	if err != nil {
		slog.Error("failed to create SAML AuthnRequest", "error", err, "provider", provider.Slug)
		h.redirectWithError(w, r, "Failed to create authentication request")
		return
	}

	// Store state token for CSRF protection, bound to the AuthnRequest ID.
	storeErr := repository.NewSSOStateRepository(h.db).Store(provider.ID, state, requestID, redirectURI, rememberMe, time.Now().Add(ssoStateTokenTTL))
	if storeErr != nil {
		slog.Error("failed to store SAML state token", "error", storeErr)
		h.redirectWithError(w, r, "Internal server error")
		return
	}

	// #nosec G710 -- SAML AuthnRequest redirect to the IdP; sp.MakeAuthenticationRequest builds the URL from the configured provider's SSO endpoint, not from the incoming request
	http.Redirect(w, r, redirectURL.String(), http.StatusFound)
}

// SAMLAssertionConsumerService handles the SAML response from the IdP.
// POST /api/sso/{slug}/saml/acs
func (h *SSOHandler) SAMLAssertionConsumerService(w http.ResponseWriter, r *http.Request) {
	provider, sp, ok := h.requireSAMLProvider(w, r)
	if !ok {
		return
	}

	// Reject IdP-initiated flows: every legitimate response originates from
	// SAMLLogin, which always sets RelayState to a freshly minted state token.
	// An empty RelayState means either an IdP-initiated flow we don't support
	// or a malformed request — fail closed before parsing the assertion.
	relayState := r.FormValue("RelayState") //nolint:gosec // G120: SAML payload size is bounded by the SAML library and reverse-proxy upload limit; FormValue here only reads the state token we minted in SAMLLogin.
	if relayState == "" {
		slog.Warn("SAML request rejected: empty RelayState (IdP-initiated flow not supported)", "provider", provider.Slug)
		h.redirectWithError(w, r, "Invalid authentication request. Please initiate login from the application.")
		return
	}

	// Validate the state token first so its recorded AuthnRequest ID can bind
	// the assertion. Looking it up before parsing also means a replayed
	// assertion whose state token was already consumed is rejected here.
	token, err := repository.NewSSOStateRepository(h.db).GetValid(relayState, provider.ID, time.Now())
	if err != nil {
		slog.Warn("SAML state token not found, rejecting request", "provider", provider.Slug)
		h.redirectWithError(w, r, "Invalid or expired authentication request. Please try again.")
		return
	}
	redirectURI := token.RedirectURI
	rememberMe := token.RememberMe
	slog.Info("restored SAML login state",
		slog.String("component", "sso"),
		slog.String("provider", provider.Slug),
		slog.Bool("remember_me", rememberMe),
	)
	// Delete used state token (single-use)
	_ = repository.NewSSOStateRepository(h.db).Delete(token.ID)

	// Parse and validate the SAML response, requiring the assertion's
	// InResponseTo to match the AuthnRequest we issued for this state token.
	assertionInfo, err := sp.ParseResponse(r, token.RequestID)
	if err != nil {
		slog.Error("SAML assertion validation failed", "error", err, "provider", provider.Slug)
		h.redirectWithError(w, r, "Authentication failed: invalid SAML response")
		return
	}

	// Convert SAML attributes to OIDCClaims for reuse of FindOrCreateUser
	claims := h.samlAssertionToClaims(assertionInfo, provider)

	if claims.Email == "" {
		slog.Error("no email in SAML assertion", "provider", provider.Slug, "nameID", assertionInfo.NameID)
		h.redirectWithError(w, r, "No email address found in SSO response")
		return
	}

	// Use the existing FindOrCreateUser flow
	result, err := h.userStore.FindOrCreateUser(provider, claims)
	if err != nil {
		slog.Error("SAML user lookup/creation failed", "error", err, "provider", provider.Slug, "email", claims.Email)
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

	if !user.IsActive {
		h.redirectWithError(w, r, "Account is disabled")
		return
	}

	// Handle email verification if needed
	if result.NeedsEmailVerification && !user.EmailVerified {
		if h.emailVerificationService != nil {
			token, tokenErr := h.emailVerificationService.GenerateVerificationToken(user.ID)
			if tokenErr == nil {
				if sendErr := h.emailVerificationService.SendVerificationEmail(user, token); sendErr != nil {
					slog.Warn("failed to send verification email", "user_id", user.ID, "error", sendErr)
				}
			}
		}
	}

	// Get client IP
	ipAddress := h.ipExtractor.GetClientIP(r)

	// Create session
	slog.Info("creating SAML session",
		slog.String("component", "sso"),
		slog.String("provider", provider.Slug),
		slog.Int("user_id", user.ID),
		slog.Bool("remember_me", rememberMe),
	)
	session, err := h.sessionManager.CreateSession(user.ID, ipAddress, r.UserAgent(), rememberMe)
	if err != nil {
		slog.Error("failed to create session after SAML login", "error", err, "user_id", user.ID)
		h.redirectWithError(w, r, "Failed to create session")
		return
	}

	// Set session cookie
	if err := h.sessionManager.SetSessionCookie(w, r, session.Token, rememberMe); err != nil {
		slog.Error("failed to set session cookie", "error", err)
		h.redirectWithError(w, r, "Failed to set session")
		return
	}

	// Audit log. Run synchronously: the previous fire-and-forget goroutine had
	// no recover, no timeout, and dropped the error, so events were silently
	// lost on shutdown or DB blips. The SAML callback is already a multi-DB
	// path, so one bounded write isn't user-visible.
	if err := logger.LogAudit(h.db, logger.AuditEvent{
		UserID:       user.ID,
		Username:     user.Username,
		IPAddress:    ipAddress,
		UserAgent:    r.UserAgent(),
		ActionType:   logger.ActionLoginSuccess,
		ResourceType: logger.ResourceUser,
		ResourceName: user.Email,
		Details: map[string]any{
			"provider":      provider.Slug,
			"provider_type": sso.ProviderTypeSAML,
			"method":        "saml",
		},
		Success: true,
	}); err != nil {
		slog.Warn("saml login audit log failed", "user_id", user.ID, "provider", provider.Slug, "err", err)
	}

	// Redirect - validate redirect URI before using it
	if result.NeedsEmailVerification && !user.EmailVerified {
		http.Redirect(w, r, "/?verify_email=pending", http.StatusFound)
	} else {
		target := redirectURI
		if target == "" || !isValidRedirectURI(target) {
			target = "/"
		}
		// target is validated by isValidRedirectURI (relative path only).
		http.Redirect(w, r, target, http.StatusFound) // #nosec G710
	}
}

// samlAssertionToClaims converts SAML assertion attributes to OIDCClaims
// so we can reuse the existing FindOrCreateUser flow.
func (h *SSOHandler) samlAssertionToClaims(info *sso.SAMLAssertionInfo, provider *sso.SSOProvider) *sso.OIDCClaims {
	attrMap, _ := provider.GetAttributeMap()
	if attrMap == nil {
		attrMap = &sso.AttributeMap{
			Email:         "email",
			EmailVerified: "email_verified",
			Name:          "name",
			GivenName:     "given_name",
			FamilyName:    "family_name",
			Username:      "preferred_username",
		}
	}

	claims := &sso.OIDCClaims{
		Subject: info.NameID,
		Raw:     make(map[string]any),
	}

	// Copy all attributes into Raw for profile_data
	for k, v := range info.Attributes {
		if len(v) == 1 {
			claims.Raw[k] = v[0]
		} else {
			claims.Raw[k] = v
		}
	}

	// Extract email
	claims.Email = getFirstSAMLAttribute(info, attrMap.Email,
		"http://schemas.xmlsoap.org/ws/2005/05/identity/claims/emailaddress",
		"urn:oid:0.9.2342.19200300.100.1.3",
		"email", "mail",
	)
	if claims.Email == "" && strings.Contains(info.NameID, "@") {
		claims.Email = info.NameID
	}

	// Extract display name
	claims.Name = getFirstSAMLAttribute(info, attrMap.Name,
		"http://schemas.xmlsoap.org/ws/2005/05/identity/claims/name",
		"urn:oid:2.16.840.1.113730.3.1.241",
		"displayName", "cn",
	)

	// Extract given name
	claims.GivenName = getFirstSAMLAttribute(info, attrMap.GivenName,
		"http://schemas.xmlsoap.org/ws/2005/05/identity/claims/givenname",
		"urn:oid:2.5.4.42",
		"givenName", "firstName",
	)

	// Extract family name
	claims.FamilyName = getFirstSAMLAttribute(info, attrMap.FamilyName,
		"http://schemas.xmlsoap.org/ws/2005/05/identity/claims/surname",
		"urn:oid:2.5.4.4",
		"sn", "lastName",
	)

	// Extract username
	claims.Username = getFirstSAMLAttribute(info, attrMap.Username,
		"http://schemas.xmlsoap.org/ws/2005/05/identity/claims/name",
		"uid", "sAMAccountName",
	)
	if claims.Username == "" && claims.Email != "" {
		claims.Username = strings.Split(claims.Email, "@")[0]
	}

	// SAML lacks a standard verified-email claim. If the IdP asserts the mapped
	// attribute, honor it; otherwise provider trust decides whether the
	// asserted email can auto-link an existing account.
	claims.EmailVerifiedProvided = true
	if attrMap.EmailVerified != "" {
		if v := info.GetAttribute(attrMap.EmailVerified); v != "" {
			// Any value that isn't a recognizable "true" means not verified.
			if verified, err := strconv.ParseBool(strings.TrimSpace(v)); err == nil {
				claims.EmailVerified = verified
			}
			return claims
		}
	}
	claims.EmailVerified = provider.RequireVerifiedEmail

	return claims
}

// getFirstSAMLAttribute tries multiple attribute names and returns the first non-empty value.
func getFirstSAMLAttribute(info *sso.SAMLAssertionInfo, names ...string) string {
	for _, name := range names {
		if name == "" {
			continue
		}
		if v := info.GetAttribute(name); v != "" {
			return v
		}
	}
	return ""
}

// nativeRedirectURI is the sole exact-match native callback. It receives only
// a one-time exchange code, never a session token.
const nativeRedirectURI = "windshift://oauth/callback"

// isValidNativeRedirectURI permits only the registered native callback.
func isValidNativeRedirectURI(uri string) bool {
	return uri == nativeRedirectURI
}

// isValidRedirectURI validates that a post-login redirect URI is safe.
// Only same-origin relative paths are accepted. This prevents open redirects
// and avoids handing web session tokens to native custom-scheme handlers.
func isValidRedirectURI(uri string) bool {
	if uri == "" {
		return false
	}
	// Must start with "/" and must not start with "//" or "/\" (protocol-relative or backslash-relative URL)
	if !strings.HasPrefix(uri, "/") || strings.HasPrefix(uri, "//") || strings.HasPrefix(uri, `/\`) {
		return false
	}
	// Reject backslash-based bypasses (e.g., "/\evil.com")
	if strings.Contains(uri, "\\") {
		return false
	}
	// Reject tab/newline characters (header injection / URL confusion)
	if strings.ContainsAny(uri, "\t\n\r") {
		return false
	}
	// Reject userinfo-based redirect confusion (e.g., "/@evil.com")
	if strings.Contains(uri, "@") {
		return false
	}
	return true
}

// getBaseURL returns the base URL, preferring the configured value.
func (h *SSOHandler) getBaseURL(r *http.Request) string {
	if h.baseURL != "" {
		return strings.TrimSuffix(h.baseURL, "/")
	}
	scheme := "https"
	if h.devMode {
		scheme = "http"
	}
	return scheme + "://" + r.Host + requestContextPrefix(r)
}
