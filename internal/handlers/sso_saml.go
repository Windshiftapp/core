package handlers

import (
	"encoding/xml"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"windshift/internal/logger"
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

// SAMLLogin initiates a SAML authentication flow by redirecting to the IdP.
// GET /api/sso/{slug}/saml/login
func (h *SSOHandler) SAMLLogin(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if slug == "" {
		h.redirectWithError(w, r, "Provider slug is required")
		return
	}

	provider, err := h.providerStore.GetBySlug(slug)
	if err != nil {
		h.redirectWithError(w, r, "SSO provider not found")
		return
	}

	if !provider.Enabled {
		h.redirectWithError(w, r, "SSO provider is disabled")
		return
	}

	if provider.ProviderType != sso.ProviderTypeSAML {
		h.redirectWithError(w, r, "Provider is not a SAML provider")
		return
	}

	baseURL := h.getBaseURL(r)
	sp, err := sso.NewSAMLServiceProvider(provider, baseURL)
	if err != nil {
		slog.Error("failed to create SAML SP", "error", err, "provider", slug)
		h.redirectWithError(w, r, "SSO configuration error")
		return
	}

	// Generate relay state containing a CSRF state token
	state := generateRandomState()
	rememberMe := r.URL.Query().Get("remember") == "true"

	redirectURI := r.URL.Query().Get("redirect_uri")
	if redirectURI == "" {
		redirectURI = "/"
	}

	// Validate redirect_uri to prevent open redirect attacks - only allow relative paths
	if !isValidRedirectURI(redirectURI) {
		redirectURI = "/"
	}

	// Store state token for CSRF protection
	_, storeErr := h.db.Exec(`
		INSERT INTO sso_state_tokens (provider_id, state, redirect_uri, remember_me, expires_at)
		VALUES (?, ?, ?, ?, ?)
	`, provider.ID, state, redirectURI, rememberMe, time.Now().Add(5*time.Minute))
	if storeErr != nil {
		slog.Error("failed to store SAML state token", "error", storeErr)
		h.redirectWithError(w, r, "Internal server error")
		return
	}

	// Create AuthnRequest and redirect to IdP
	redirectURL, err := sp.MakeAuthenticationRequest(state)
	if err != nil {
		slog.Error("failed to create SAML AuthnRequest", "error", err, "provider", slug)
		h.redirectWithError(w, r, "Failed to create authentication request")
		return
	}

	http.Redirect(w, r, redirectURL.String(), http.StatusFound)
}

// SAMLAssertionConsumerService handles the SAML response from the IdP.
// POST /api/sso/{slug}/saml/acs
func (h *SSOHandler) SAMLAssertionConsumerService(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if slug == "" {
		h.redirectWithError(w, r, "Provider slug is required")
		return
	}

	provider, err := h.providerStore.GetBySlug(slug)
	if err != nil {
		h.redirectWithError(w, r, "SSO provider not found")
		return
	}

	if !provider.Enabled {
		h.redirectWithError(w, r, "SSO provider is disabled")
		return
	}

	if provider.ProviderType != sso.ProviderTypeSAML {
		h.redirectWithError(w, r, "Provider is not a SAML provider")
		return
	}

	baseURL := h.getBaseURL(r)
	sp, err := sso.NewSAMLServiceProvider(provider, baseURL)
	if err != nil {
		slog.Error("failed to create SAML SP for ACS", "error", err, "provider", slug)
		h.redirectWithError(w, r, "SSO configuration error")
		return
	}

	// Parse and validate the SAML response
	assertionInfo, err := sp.ParseResponse(r)
	if err != nil {
		slog.Error("SAML assertion validation failed", "error", err, "provider", slug)
		h.redirectWithError(w, r, "Authentication failed: invalid SAML response")
		return
	}

	// Get relay state (contains our state token)
	relayState := r.FormValue("RelayState")

	// Validate state token
	var stateTokenID int
	var redirectURI string
	var rememberMe bool
	err = h.db.QueryRow(`
		SELECT id, redirect_uri, remember_me FROM sso_state_tokens
		WHERE state = ? AND provider_id = ? AND expires_at > ?
	`, relayState, provider.ID, time.Now()).Scan(&stateTokenID, &redirectURI, &rememberMe)
	if err != nil {
		slog.Warn("SAML state token not found, rejecting request", "provider", slug)
		h.redirectWithError(w, r, "Invalid or expired authentication request. Please try again.")
		return
	} else {
		// Delete used state token
		_, _ = h.db.Exec("DELETE FROM sso_state_tokens WHERE id = ?", stateTokenID)
	}

	// Convert SAML attributes to OIDCClaims for reuse of FindOrCreateUser
	claims := h.samlAssertionToClaims(assertionInfo, provider)

	if claims.Email == "" {
		slog.Error("no email in SAML assertion", "provider", slug, "nameID", assertionInfo.NameID)
		h.redirectWithError(w, r, "No email address found in SSO response")
		return
	}

	// Use the existing FindOrCreateUser flow
	result, err := h.userStore.FindOrCreateUser(provider, claims)
	if err != nil {
		slog.Error("SAML user lookup/creation failed", "error", err, "provider", slug, "email", claims.Email)
		switch {
		case err == sso.ErrAutoProvisionDisabled:
			h.redirectWithError(w, r, "User account not found. Contact your administrator.")
		case errors.Is(err, sso.ErrEmailNotVerified):
			h.redirectWithError(w, r, "Your email address has not been verified by the identity provider")
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

	// Audit log
	go func() {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       user.ID,
			Username:     user.Username,
			IPAddress:    ipAddress,
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionLoginSuccess,
			ResourceType: logger.ResourceUser,
			ResourceName: user.Email,
			Details: map[string]interface{}{
				"provider":      provider.Slug,
				"provider_type": sso.ProviderTypeSAML,
				"method":        "saml",
			},
			Success: true,
		})
	}()

	// Redirect - validate redirect URI before using it
	if result.NeedsEmailVerification && !user.EmailVerified {
		http.Redirect(w, r, "/?verify_email=pending", http.StatusFound)
	} else {
		target := redirectURI
		if target == "" || !isValidRedirectURI(target) {
			target = "/"
		}
		http.Redirect(w, r, target, http.StatusFound)
	}
}

// samlAssertionToClaims converts SAML assertion attributes to OIDCClaims
// so we can reuse the existing FindOrCreateUser flow.
func (h *SSOHandler) samlAssertionToClaims(info *sso.SAMLAssertionInfo, provider *sso.SSOProvider) *sso.OIDCClaims {
	attrMap, _ := provider.GetAttributeMap()
	if attrMap == nil {
		attrMap = &sso.AttributeMap{
			Email:      "email",
			Name:       "name",
			GivenName:  "given_name",
			FamilyName: "family_name",
			Username:   "preferred_username",
		}
	}

	claims := &sso.OIDCClaims{
		Subject: info.NameID,
		Raw:     make(map[string]interface{}),
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

	// SAML doesn't have a standard email_verified claim - treat as unverified
	// unless the provider is configured to trust email from IdP
	claims.EmailVerified = false
	claims.EmailVerifiedProvided = false

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

// isValidRedirectURI validates that a redirect URI is safe (relative path only).
// This prevents open redirect attacks by rejecting absolute URLs and protocol-relative URLs.
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
	return scheme + "://" + r.Host
}
