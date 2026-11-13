package handlers

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sso"

	"golang.org/x/oauth2"
)

type githubAuthenticator interface {
	Authenticate(context.Context, *sso.SSOProvider, string, string, string, string) (*sso.OIDCClaims, error)
}

func githubOrganizationValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func validateGitHubProviderRequest(w http.ResponseWriter, r *http.Request, req *SSOProviderRequest) bool {
	if req.GitHubOrganization != nil {
		*req.GitHubOrganization = strings.TrimSpace(*req.GitHubOrganization)
		if req.ProviderType != sso.ProviderTypeGitHub && *req.GitHubOrganization != "" {
			respondValidationError(w, r, "Organization restrictions are only supported for GitHub providers")
			return false
		}
		if !sso.ValidateGitHubOrganization(*req.GitHubOrganization) {
			respondValidationError(w, r, "GitHub organization must be a valid organization login, not a URL")
			return false
		}
	}
	if req.ProviderType == sso.ProviderTypeGitHub {
		req.IssuerURL = ""
		req.AttributeMapping = "{}"
		req.Scopes = strings.Join(sso.GitHubScopes(githubOrganizationValue(req.GitHubOrganization)), " ")
	}
	return true
}

func (h *SSOHandler) githubStateCookie(r *http.Request, provider *sso.SSOProvider) *http.Cookie {
	secure := !h.devMode || strings.HasPrefix(h.getRedirectURI(r, provider.Slug), "https://")
	name := fmt.Sprintf("windshift-github-state-%d", provider.ID)
	if secure {
		name = "__Host-" + name
	}
	return &http.Cookie{Name: name, Path: "/", HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode} // #nosec G124 -- Secure is disabled only for explicit HTTP development mode.
}

func (h *SSOHandler) startGitHubLogin(w http.ResponseWriter, r *http.Request, provider *sso.SSOProvider) {
	var surface *models.CustomerSurface
	if provider.AccountType == "customer" {
		if h.customerAccess == nil {
			respondBadRequest(w, r, "Customer login is unavailable")
			return
		}
		var err error
		surface, err = h.customerAccess.repo.Resolve(r.Context(), r.URL.Query().Get("resource_kind"), r.URL.Query().Get("resource_slug"))
		if err != nil || !h.customerAccess.providerAllowed(r.Context(), provider, surface) {
			respondNotFound(w, r, "customer login")
			return
		}
	}
	redirectURI := r.URL.Query().Get("redirect_uri")
	if !isValidRedirectURI(redirectURI) && !isValidNativeRedirectURI(redirectURI) {
		redirectURI = "/"
	}
	if surface != nil {
		redirectURI = customerSurfacePath(*surface)
	}
	state := generateRandomState()
	verifier := oauth2.GenerateVerifier()
	if err := repository.NewSSOStateRepository(h.db).StoreOIDC(provider.ID, state, verifier, redirectURI, ssoRememberMeFromRequest(r), time.Now().Add(ssoStateTokenTTL)); err != nil {
		respondInternalError(w, r, err)
		return
	}
	if surface != nil {
		if err := repository.NewSSOStateRepository(h.db).BindCustomerResource(state, provider.ID, surface.Kind, surface.ID); err != nil {
			respondInternalError(w, r, err)
			return
		}
	}
	cookie := h.githubStateCookie(r, provider) // #nosec G124 -- Secure is disabled only for explicit HTTP development mode.
	cookie.Value = state
	cookie.MaxAge = int(ssoStateTokenTTL.Seconds())
	http.SetCookie(w, cookie)
	config := sso.GitHubOAuthConfig(provider, "", h.getRedirectURI(r, provider.Slug))
	http.Redirect(w, r, config.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier)), http.StatusFound) // #nosec G710 -- OAuth authorization endpoint is fixed to github.com.
}

func (h *SSOHandler) githubCallback(w http.ResponseWriter, r *http.Request, provider *sso.SSOProvider) {
	expired := func() {
		if provider.AccountType == "customer" {
			respondUnauthorized(w, r)
			return
		}
		h.redirectWithError(w, r, "Login session expired. Please try signing in again.")
	}
	state := r.URL.Query().Get("state")
	cookie := h.githubStateCookie(r, provider) // #nosec G124 -- Secure is disabled only for explicit HTTP development mode.
	browserCookie, err := r.Cookie(cookie.Name)
	if err != nil || state == "" || subtle.ConstantTimeCompare([]byte(browserCookie.Value), []byte(state)) != 1 {
		expired()
		return
	}
	cookie.MaxAge = -1
	http.SetCookie(w, cookie)
	stored, err := repository.NewSSOStateRepository(h.db).Consume(state, provider.ID, time.Now())
	if err != nil || stored.Nonce == "" {
		expired()
		return
	}
	fail := func(message string) {
		if provider.AccountType != "customer" {
			h.redirectWithError(w, r, message)
			return
		}
		if h.customerAccess != nil {
			surface, err := h.customerAccess.repo.Surface(r.Context(), stored.ResourceKind, stored.ResourceID)
			if err == nil {
				http.Redirect(w, r, requestContextPrefix(r)+customerSurfacePath(*surface)+"?customer_login_error="+url.QueryEscape(message), http.StatusFound) // #nosec G710 -- fixed local resource prefix and escaped database slug.
				return
			}
		}
		respondForbidden(w, r)
	}
	if r.URL.Query().Get("error") != "" {
		fail("GitHub sign-in was denied. Please try again.")
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		fail("GitHub did not return an authorization code")
		return
	}
	secret, err := h.encryption.Decrypt(provider.ClientSecretEncrypted)
	if err != nil {
		fail("SSO configuration error")
		return
	}
	claims, err := h.githubService.Authenticate(r.Context(), provider, secret, h.getRedirectURI(r, provider.Slug), code, stored.Nonce)
	if err != nil {
		// Log only API operation/status; raw OAuth errors can contain credentials.
		attributes := []any{"component", "sso", "provider", provider.Slug}
		var apiError *sso.GitHubAPIError
		if errors.As(err, &apiError) {
			attributes = append(attributes, "operation", apiError.Operation, "status_code", apiError.StatusCode)
		}
		slog.Warn("GitHub sign-in failed", attributes...)
		switch {
		case errors.Is(err, sso.ErrGitHubOrganizationUnavailable):
			fail("GitHub organization membership could not be checked. Please try again or ask your administrator to check GitHub application access.")
		case errors.Is(err, sso.ErrGitHubOrganization):
			fail("Access requires active membership in the configured GitHub organization")
		case errors.Is(err, sso.ErrGitHubVerifiedEmail):
			fail("Your GitHub account must have a verified email address")
		default:
			fail("GitHub authentication failed. Please try signing in again.")
		}
		return
	}
	if provider.AccountType == "customer" {
		if h.customerAccess == nil {
			fail("Customer login is unavailable")
			return
		}
		h.customerAccess.completeGitHub(w, r, provider, claims, stored)
		return
	}
	if stored.ResourceKind != "" || stored.ResourceID != 0 {
		h.redirectWithError(w, r, "Invalid login destination")
		return
	}
	h.completeSSOLogin(w, r, provider, claims, stored.RedirectURI, stored.RememberMe)
}

func (h *SSOHandler) validateCustomerProvider(w http.ResponseWriter, r *http.Request, req *SSOProviderRequest) bool {
	if req.AccountType != "internal" && req.AccountType != "customer" {
		respondValidationError(w, r, "Account type must be internal or customer")
		return false
	}
	if req.AccountType == "internal" {
		if req.CustomerOrganisationID != nil {
			respondValidationError(w, r, "Customer organisation is only valid for customer login")
			return false
		}
		return true
	}
	if req.ProviderType != sso.ProviderTypeGitHub {
		respondValidationError(w, r, "Customer login currently requires a GitHub provider")
		return false
	}
	if req.CustomerOrganisationID == nil || *req.CustomerOrganisationID <= 0 {
		respondValidationError(w, r, "A customer organisation is required for customer login")
		return false
	}
	exists, err := repository.NewCustomerAccessRepository(h.db).OrganisationExists(r.Context(), *req.CustomerOrganisationID)
	if err != nil {
		respondInternalError(w, r, err)
		return false
	}
	if !exists {
		respondValidationError(w, r, "Customer organisation does not exist")
		return false
	}
	return true
}

func (h *SSOHandler) SetCustomerAccess(handler *CustomerAccessHandler) { h.customerAccess = handler }
