package handlers

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

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
	redirectURI := r.URL.Query().Get("redirect_uri")
	if !isValidRedirectURI(redirectURI) && !isValidNativeRedirectURI(redirectURI) {
		redirectURI = "/"
	}
	state := generateRandomState()
	verifier := oauth2.GenerateVerifier()
	if err := repository.NewSSOStateRepository(h.db).StoreOIDC(provider.ID, state, verifier, redirectURI, ssoRememberMeFromRequest(r), time.Now().Add(ssoStateTokenTTL)); err != nil {
		respondInternalError(w, r, err)
		return
	}
	cookie := h.githubStateCookie(r, provider) // #nosec G124 -- Secure is disabled only for explicit HTTP development mode.
	cookie.Value = state
	cookie.MaxAge = int(ssoStateTokenTTL.Seconds())
	http.SetCookie(w, cookie)
	config := sso.GitHubOAuthConfig(provider, "", h.getRedirectURI(r, provider.Slug))
	http.Redirect(w, r, config.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier)), http.StatusFound) // #nosec G710 -- OAuth authorization endpoint is fixed to github.com.
}

func (h *SSOHandler) githubCallback(w http.ResponseWriter, r *http.Request, provider *sso.SSOProvider) {
	state := r.URL.Query().Get("state")
	cookie := h.githubStateCookie(r, provider) // #nosec G124 -- Secure is disabled only for explicit HTTP development mode.
	browserCookie, err := r.Cookie(cookie.Name)
	if err != nil || state == "" || subtle.ConstantTimeCompare([]byte(browserCookie.Value), []byte(state)) != 1 {
		h.redirectWithError(w, r, "Login session expired. Please try signing in again.")
		return
	}
	cookie.MaxAge = -1
	http.SetCookie(w, cookie)
	stored, err := repository.NewSSOStateRepository(h.db).Consume(state, provider.ID, time.Now())
	if err != nil || stored.Nonce == "" {
		h.redirectWithError(w, r, "Login session expired. Please try signing in again.")
		return
	}
	if r.URL.Query().Get("error") != "" {
		h.redirectWithError(w, r, "GitHub sign-in was denied. Please try again.")
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		h.redirectWithError(w, r, "GitHub did not return an authorization code")
		return
	}
	secret, err := h.encryption.Decrypt(provider.ClientSecretEncrypted)
	if err != nil {
		h.redirectWithError(w, r, "SSO configuration error")
		return
	}
	claims, err := h.githubService.Authenticate(r.Context(), provider, secret, h.getRedirectURI(r, provider.Slug), code, stored.Nonce)
	if err != nil {
		// Token-exchange errors may contain response bodies; never log credentials.
		slog.Warn("GitHub sign-in failed", "component", "sso", "provider", provider.Slug)
		switch {
		case errors.Is(err, sso.ErrGitHubOrganization):
			h.redirectWithError(w, r, "Access requires active membership in the configured GitHub organization")
		case errors.Is(err, sso.ErrGitHubVerifiedEmail):
			h.redirectWithError(w, r, "Your GitHub account must have a verified email address")
		default:
			h.redirectWithError(w, r, "GitHub authentication failed. Please try signing in again.")
		}
		return
	}
	h.completeSSOLogin(w, r, provider, claims, stored.RedirectURI, stored.RememberMe)
}
