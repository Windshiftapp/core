package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"windshift/internal/auth"
	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// OAuth 2.0 server lifetimes. Hardcoded constants in v1; configurable via
// system_settings later if needed.
const (
	oauthCodeTTL     = 60 * time.Second
	oauthAccessTTL   = 1 * time.Hour
	oauthRefreshTTL  = 30 * 24 * time.Hour
	oauthAgentPrefix = "oauth-" // agent username = oauth-{slug}-{user_id}
)

// Standard RFC 6749 §5.2 error codes used in /token + redirect-back error
// query strings. Strings are part of the wire contract — don't rename.
const (
	oauthErrInvalidRequest          = "invalid_request"
	oauthErrInvalidClient           = "invalid_client"
	oauthErrInvalidGrant            = "invalid_grant"
	oauthErrUnauthorizedClient      = "unauthorized_client"
	oauthErrUnsupportedGrantType    = "unsupported_grant_type"
	oauthErrInvalidScope            = "invalid_scope"
	oauthErrAccessDenied            = "access_denied"
	oauthErrServerError             = "server_error"
	oauthErrUnsupportedResponseType = "unsupported_response_type"
)

// OAuthHandler implements the OAuth 2.0 authorization-code-with-PKCE server.
//
// Two browser-facing endpoints:
//
//   - GET  /api/oauth/authorize/info  — JSON describing the consenting flow
//     (display_name, scopes, validation errors). Called by the SPA consent
//     page after the browser lands at /oauth/authorize?...
//   - POST /api/oauth/authorize/{approve,deny} — user clicks Allow/Deny.
//
// Two machine-to-machine endpoints:
//
//   - POST /api/oauth/token — exchange authorization_code OR refresh_token
//     for a fresh access+refresh pair. Public + rate-limited.
//
// Token issuance flows through the same primitives as cli_auth.go: a
// per-(client, user) agent is found-or-created, a `crw_…` API token is
// minted for that agent with the granted scopes, and a refresh-token row
// (SHA-256-hashed) is issued alongside it.
type OAuthHandler struct {
	db                database.Database
	agent             *AgentHandler
	tokenManager      *auth.TokenManager
	apiToken          *APITokenHandler
	permissionService *services.PermissionService
}

// NewOAuthHandler wires the handler. All deps are required; the routes
// refuse to register otherwise (see routes/admin.go).
func NewOAuthHandler(db database.Database, ah *AgentHandler, tm *auth.TokenManager, ath *APITokenHandler, ps *services.PermissionService) *OAuthHandler {
	return &OAuthHandler{
		db:                db,
		agent:             ah,
		tokenManager:      tm,
		apiToken:          ath,
		permissionService: ps,
	}
}

// AuthorizeInfoResponse is the SPA consent page's view of an /authorize
// request: client identity, granted scopes, and any validation issue. The
// SPA fetches this on mount and renders the consent card from it.
type AuthorizeInfoResponse struct {
	ClientID            string   `json:"client_id"`
	ClientDisplayName   string   `json:"client_display_name"`
	RedirectURI         string   `json:"redirect_uri"`
	GrantedScopes       []string `json:"granted_scopes"`
	State               string   `json:"state"`
	CodeChallenge       string   `json:"code_challenge,omitempty"`
	CodeChallengeMethod string   `json:"code_challenge_method,omitempty"`
}

// AuthorizeInfo answers the consent page's "what is this request?" query.
// Returns 400 on a malformed or untrustworthy request so the page shows a
// "this app's request is malformed" error instead of going to the consent
// screen.
func (h *OAuthHandler) AuthorizeInfo(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	clientID := q.Get("client_id")
	redirectURI := q.Get("redirect_uri")
	responseType := q.Get("response_type")
	scopeStr := q.Get("scope")
	state := q.Get("state")
	codeChallenge := q.Get("code_challenge")
	codeChallengeMethod := q.Get("code_challenge_method")

	if responseType != "code" {
		respondBadRequest(w, r, "response_type must be 'code'")
		return
	}

	client, err := h.lookupEnabledClientByClientID(clientID)
	if err != nil {
		// Don't leak whether the client_id exists — the user shouldn't see
		// fingerprint-able errors here. The admin's audit log still records
		// the failed lookup if/when we plumb it.
		respondBadRequest(w, r, "Unknown or disabled client_id")
		return
	}

	if !redirectURIAllowed(client.RedirectURIs, redirectURI) {
		respondBadRequest(w, r, "redirect_uri does not match a registered URI for this client")
		return
	}

	requested := splitOAuthScopes(scopeStr)
	if len(requested) == 0 {
		respondBadRequest(w, r, "at least one scope is required")
		return
	}
	granted, err := intersectScopes(requested, client.AllowedScopes)
	if err != nil {
		respondBadRequest(w, r, err.Error())
		return
	}

	// Public clients must use PKCE. Confidential clients may use it.
	if client.ClientType == "public" && codeChallenge == "" {
		respondBadRequest(w, r, "code_challenge is required for public clients")
		return
	}
	if codeChallenge != "" {
		if codeChallengeMethod == "" {
			codeChallengeMethod = "plain" // RFC 7636 default
		}
		if codeChallengeMethod != "S256" && codeChallengeMethod != "plain" {
			respondBadRequest(w, r, "code_challenge_method must be 'S256' or 'plain'")
			return
		}
	}

	respondJSONOK(w, AuthorizeInfoResponse{
		ClientID:            client.ClientID,
		ClientDisplayName:   client.DisplayName,
		RedirectURI:         redirectURI,
		GrantedScopes:       granted,
		State:               state,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
	})
}

// AuthorizeApproveRequest is the body the consent page POSTs when the user
// clicks Allow. Same query params as the /info call, echoed back so the
// approval is bound to a specific consent context (and we don't re-trust
// the URL bar).
type AuthorizeApproveRequest struct {
	ClientID            string `json:"client_id"`
	RedirectURI         string `json:"redirect_uri"`
	ResponseType        string `json:"response_type"`
	Scope               string `json:"scope"`
	State               string `json:"state"`
	CodeChallenge       string `json:"code_challenge,omitempty"`
	CodeChallengeMethod string `json:"code_challenge_method,omitempty"`
}

// AuthorizeApproveResponse tells the SPA where to send the browser next.
type AuthorizeApproveResponse struct {
	RedirectTo string `json:"redirect_to"`
}

// AuthorizeApprove is called when the user clicks Allow. Mints (or finds) a
// per-(client, user) agent, inserts a row into oauth_authorization_codes,
// and tells the SPA to redirect the browser to redirect_uri?code=…&state=…
func (h *OAuthHandler) AuthorizeApprove(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	req, ok := decodeJSON[AuthorizeApproveRequest](w, r)
	if !ok {
		return
	}

	if req.ResponseType != "code" {
		respondBadRequest(w, r, "response_type must be 'code'")
		return
	}

	client, err := h.lookupEnabledClientByClientID(req.ClientID)
	if err != nil {
		respondBadRequest(w, r, "Unknown or disabled client_id")
		return
	}
	if !redirectURIAllowed(client.RedirectURIs, req.RedirectURI) {
		respondBadRequest(w, r, "redirect_uri does not match a registered URI for this client")
		return
	}

	requested := splitOAuthScopes(req.Scope)
	granted, err := intersectScopes(requested, client.AllowedScopes)
	if err != nil {
		respondBadRequest(w, r, err.Error())
		return
	}

	if client.ClientType == "public" && req.CodeChallenge == "" {
		respondBadRequest(w, r, "code_challenge is required for public clients")
		return
	}
	if req.CodeChallenge != "" {
		if req.CodeChallengeMethod == "" {
			req.CodeChallengeMethod = "plain"
		}
		if req.CodeChallengeMethod != "S256" && req.CodeChallengeMethod != "plain" {
			respondBadRequest(w, r, "code_challenge_method must be 'S256' or 'plain'")
			return
		}
	}

	// Same gate the CLI flow uses — refuses to mint a token for a user
	// whose api_key_creation_policy is disabled.
	if err := h.apiToken.checkCreationPolicy(user.ID); err != nil {
		h.audit(r, user, "oauth.approve", &client.ID, client.DisplayName, false,
			map[string]interface{}{"reason": "token_policy_disabled"})
		respondForbidden(w, r)
		return
	}

	agent, err := h.findOrCreateClientAgent(user, client)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	code, err := randomOpaqueCode()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	scopesJSON, _ := json.Marshal(granted)
	expires := time.Now().Add(oauthCodeTTL)
	codeChallenge := nullStringOrEmpty(req.CodeChallenge)
	codeChallengeMethod := nullStringOrEmpty(req.CodeChallengeMethod)

	if _, err := h.db.Exec(`
		INSERT INTO oauth_authorization_codes (
			code, client_id, user_id, agent_id, redirect_uri, scopes,
			code_challenge, code_challenge_method, state, expires_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, code, client.ClientID, user.ID, agent.ID, req.RedirectURI, string(scopesJSON),
		codeChallenge, codeChallengeMethod, req.State, expires); err != nil {
		respondInternalError(w, r, err)
		return
	}

	h.audit(r, user, "oauth.approve", &client.ID, client.DisplayName, true, map[string]interface{}{
		"client_id": client.ClientID,
		"scopes":    granted,
		"agent_id":  agent.ID,
	})

	respondJSONOK(w, AuthorizeApproveResponse{
		RedirectTo: appendQuery(req.RedirectURI, map[string]string{
			"code":  code,
			"state": req.State,
		}),
	})
}

// AuthorizeDeny is called when the user clicks Deny. Returns a redirect URL
// with `error=access_denied` per RFC 6749 §4.1.2.1.
func (h *OAuthHandler) AuthorizeDeny(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	req, ok := decodeJSON[AuthorizeApproveRequest](w, r)
	if !ok {
		return
	}

	// Best-effort lookup so we can reject obviously bad redirect URIs even
	// on deny. If the client doesn't exist, return without redirecting —
	// a malformed deny shouldn't bounce to an attacker URL.
	client, err := h.lookupEnabledClientByClientID(req.ClientID)
	if err != nil {
		respondBadRequest(w, r, "Unknown or disabled client_id")
		return
	}
	if !redirectURIAllowed(client.RedirectURIs, req.RedirectURI) {
		respondBadRequest(w, r, "redirect_uri does not match a registered URI for this client")
		return
	}

	h.audit(r, user, "oauth.deny", &client.ID, client.DisplayName, true, map[string]interface{}{
		"client_id": client.ClientID,
	})

	respondJSONOK(w, AuthorizeApproveResponse{
		RedirectTo: appendQuery(req.RedirectURI, map[string]string{
			"error": oauthErrAccessDenied,
			"state": req.State,
		}),
	})
}

// Userinfo is the OIDC-compatible identity endpoint OAuth clients call after
// token exchange to learn who the access token represents. The auth
// middleware has already resolved the Bearer token to a user (the
// per-(client, user) agent), so we just project the relevant fields.
//
// Returns the OIDC-conventional shape: `sub` (stable identifier), `email`,
// `name`, plus a Windshift-specific `username` for display. Used by Omni's
// generic OAuth dispatcher to populate `service_credentials.principal_email`.
func (h *OAuthHandler) Userinfo(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	respondJSONOK(w, map[string]interface{}{
		"sub":      fmt.Sprintf("%d", user.ID),
		"email":    user.Email,
		"name":     firstNonEmpty(user.FullName, user.Username),
		"username": user.Username,
	})
}

// Token implements RFC 6749 §3.2 — the token endpoint. Accepts
// application/x-www-form-urlencoded or application/json bodies. Two grant
// types are supported: authorization_code and refresh_token.
func (h *OAuthHandler) Token(w http.ResponseWriter, r *http.Request) {
	// RFC 6749 §2.3.1 mandates that confidential clients NOT pass credentials
	// in the URL. Reject query-string client_secret outright — it would leak
	// into webserver/proxy access logs and browser history if a misbehaving
	// client put it there.
	if r.URL.Query().Get("client_secret") != "" {
		writeOAuthTokenError(w, http.StatusBadRequest, oauthErrInvalidRequest,
			"client_secret must not be sent in the URL — use the request body or HTTP Basic auth")
		return
	}

	params, err := parseTokenRequest(r)
	if err != nil {
		writeOAuthTokenError(w, http.StatusBadRequest, oauthErrInvalidRequest, err.Error())
		return
	}

	switch params.Get("grant_type") {
	case "authorization_code":
		h.tokenAuthorizationCode(w, r, params)
	case "refresh_token":
		h.tokenRefreshToken(w, r, params)
	case "":
		writeOAuthTokenError(w, http.StatusBadRequest, oauthErrInvalidRequest, "grant_type is required")
	default:
		writeOAuthTokenError(w, http.StatusBadRequest, oauthErrUnsupportedGrantType,
			fmt.Sprintf("unsupported grant_type: %s", params.Get("grant_type")))
	}
}

// tokenAuthorizationCode handles grant_type=authorization_code. Validates
// the client (secret or PKCE), looks up the code, validates redirect_uri
// match and PKCE verifier, marks the code consumed, and mints a new
// access+refresh pair.
func (h *OAuthHandler) tokenAuthorizationCode(w http.ResponseWriter, r *http.Request, params url.Values) {
	clientID := params.Get("client_id")
	clientSecret := params.Get("client_secret")
	code := params.Get("code")
	redirectURI := params.Get("redirect_uri")
	codeVerifier := params.Get("code_verifier")

	if clientID == "" || code == "" || redirectURI == "" {
		writeOAuthTokenError(w, http.StatusBadRequest, oauthErrInvalidRequest,
			"client_id, code, and redirect_uri are required")
		return
	}

	client, err := h.authenticateClient(clientID, clientSecret)
	if err != nil {
		writeOAuthTokenError(w, http.StatusUnauthorized, oauthErrInvalidClient, err.Error())
		return
	}

	authCode, err := h.consumeAuthorizationCode(code)
	if err != nil {
		writeOAuthTokenError(w, http.StatusBadRequest, oauthErrInvalidGrant, err.Error())
		return
	}

	if authCode.ClientID != client.ClientID {
		writeOAuthTokenError(w, http.StatusBadRequest, oauthErrInvalidGrant,
			"authorization code was issued to a different client")
		return
	}
	if authCode.RedirectURI != redirectURI {
		writeOAuthTokenError(w, http.StatusBadRequest, oauthErrInvalidGrant,
			"redirect_uri does not match the value used in /authorize")
		return
	}

	// PKCE verifier check. Required for public clients (authenticateClient
	// allowed an empty secret only if PKCE is in play); optional but
	// honored for confidential clients that set a code_challenge.
	if authCode.CodeChallenge != "" {
		if codeVerifier == "" {
			writeOAuthTokenError(w, http.StatusBadRequest, oauthErrInvalidGrant,
				"code_verifier is required when code_challenge was provided in /authorize")
			return
		}
		if err := verifyPKCE(authCode.CodeChallenge, authCode.CodeChallengeMethod, codeVerifier); err != nil {
			writeOAuthTokenError(w, http.StatusBadRequest, oauthErrInvalidGrant, err.Error())
			return
		}
	} else if client.ClientType == "public" {
		// Public client without PKCE. Should have been caught at /authorize
		// but enforce here too as a safety net.
		writeOAuthTokenError(w, http.StatusBadRequest, oauthErrInvalidGrant,
			"PKCE is required for public clients")
		return
	}

	scopes, err := unmarshalScopes(authCode.Scopes)
	if err != nil {
		writeOAuthTokenError(w, http.StatusInternalServerError, oauthErrServerError, "invalid scopes on stored code")
		return
	}

	resp, err := h.mintAccessAndRefresh(client, authCode.UserID, authCode.AgentID, scopes)
	if err != nil {
		writeOAuthTokenError(w, http.StatusInternalServerError, oauthErrServerError, err.Error())
		return
	}

	h.auditClient(r, client, "oauth.token.issue", map[string]interface{}{
		"grant_type": "authorization_code",
		"user_id":    authCode.UserID,
		"agent_id":   authCode.AgentID,
		"scopes":     scopes,
	})

	respondJSONOK(w, resp)
}

// tokenRefreshToken handles grant_type=refresh_token. Validates the client,
// looks up the refresh row by hash, checks not expired/revoked, mints a new
// pair, and rotates the chain. Replay of a revoked token cascades revoke
// over `rotated_to_id` to invalidate any subsequent rows the attacker holds.
func (h *OAuthHandler) tokenRefreshToken(w http.ResponseWriter, r *http.Request, params url.Values) {
	clientID := params.Get("client_id")
	clientSecret := params.Get("client_secret")
	refreshPlain := params.Get("refresh_token")

	if clientID == "" || refreshPlain == "" {
		writeOAuthTokenError(w, http.StatusBadRequest, oauthErrInvalidRequest,
			"client_id and refresh_token are required")
		return
	}

	client, err := h.authenticateClient(clientID, clientSecret)
	if err != nil {
		writeOAuthTokenError(w, http.StatusUnauthorized, oauthErrInvalidClient, err.Error())
		return
	}

	row, err := h.lookupRefreshToken(refreshPlain)
	if errors.Is(err, sql.ErrNoRows) {
		writeOAuthTokenError(w, http.StatusBadRequest, oauthErrInvalidGrant, "unknown refresh_token")
		return
	}
	if err != nil {
		writeOAuthTokenError(w, http.StatusInternalServerError, oauthErrServerError, err.Error())
		return
	}

	if row.ClientID != client.ClientID {
		writeOAuthTokenError(w, http.StatusBadRequest, oauthErrInvalidGrant,
			"refresh_token was issued to a different client")
		return
	}

	// Replay detection — an already-revoked token presented again means the
	// chain is compromised. Cascade-revoke everything reachable via
	// rotated_to_id from this row, including the new tokens the attacker
	// might already hold.
	if row.RevokedAt.Valid {
		h.cascadeRevokeRefreshChain(row.ID)
		h.auditClient(r, client, "oauth.token.refresh_replay", map[string]interface{}{
			"refresh_id": row.ID,
		})
		writeOAuthTokenError(w, http.StatusBadRequest, oauthErrInvalidGrant,
			"refresh_token has been revoked")
		return
	}

	if time.Now().After(row.ExpiresAt) {
		writeOAuthTokenError(w, http.StatusBadRequest, oauthErrInvalidGrant, "refresh_token expired")
		return
	}

	scopes, err := unmarshalScopes(row.Scopes)
	if err != nil {
		writeOAuthTokenError(w, http.StatusInternalServerError, oauthErrServerError, "invalid scopes on stored refresh token")
		return
	}

	resp, err := h.mintAccessAndRefresh(client, row.UserID, row.AgentID, scopes)
	if err != nil {
		writeOAuthTokenError(w, http.StatusInternalServerError, oauthErrServerError, err.Error())
		return
	}

	// Rotate: the just-presented refresh is marked revoked + rotated_to the
	// new id. The new id was returned by mintAccessAndRefresh; we need to
	// look it up to write rotated_to_id (small extra query — could be
	// folded into mintAccessAndRefresh later).
	newRefreshHash := hashRefreshToken(resp.RefreshToken)
	var newID int
	if err := h.db.QueryRow(
		`SELECT id FROM oauth_refresh_tokens WHERE token_hash = ?`, newRefreshHash,
	).Scan(&newID); err == nil {
		_, _ = h.db.Exec(
			`UPDATE oauth_refresh_tokens SET revoked_at = CURRENT_TIMESTAMP, rotated_to_id = ? WHERE id = ?`,
			newID, row.ID,
		)
	}

	// Revoke the old access token alongside the rotation so a leaked old
	// access token doesn't keep working past its natural 1h.
	_ = h.tokenManager.AdminRevokeToken(row.APITokenID)

	h.auditClient(r, client, "oauth.token.refresh", map[string]interface{}{
		"old_refresh_id": row.ID,
		"new_refresh_id": newID,
		"user_id":        row.UserID,
		"agent_id":       row.AgentID,
	})

	respondJSONOK(w, resp)
}

// findOrCreateClientAgent returns the per-(client, user) agent that all
// OAuth-issued tokens for this pair are bound to. Username convention is
// `oauth-{slug}-{user_id}` — collision-free across clients and users.
func (h *OAuthHandler) findOrCreateClientAgent(user *models.User, client *oauthClientRow) (*models.User, error) {
	username := fmt.Sprintf("%s%s-%d", oauthAgentPrefix, client.Slug, user.ID)
	existing, err := h.agent.FindOwnedAgentByUsername(user.ID, username)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	createReq := CreateAgentRequest{
		Username:  username,
		FirstName: client.DisplayName,
		LastName:  fmt.Sprintf("for %s", firstNonEmpty(user.Username, fmt.Sprintf("user-%d", user.ID))),
	}
	// CreateOAuthAgent (vs CreateOwnedAgent) carves OAuth-issued agents out
	// of the user-managed agents policy: 'allow_user_managed_agents' gates
	// the human-facing "create agent" UI, NOT third-party OAuth integrations.
	// Conflating them meant non-admin OAuth would 4xx on instances where the
	// policy is off, while admins silently bypassed it — privilege-dependent
	// behavior that defeated the policy's intent. CreateOAuthAgent persists
	// agent_provenance='oauth' + oauth_client_id, which the schema CHECK
	// constraints require to coexist.
	created, createErr := h.agent.CreateOAuthAgent(user.ID, client.ID, createReq)
	if createErr != nil {
		switch {
		case errors.Is(createErr, ErrOAuthClientDisabledOrMissing),
			errors.Is(createErr, ErrAgentUsernameTaken),
			errors.Is(createErr, ErrAgentEmailTaken):
			return nil, createErr
		default:
			return nil, createErr
		}
	}
	return created, nil
}

// TokenResponse is the RFC 6749 §5.1 successful response.
type oauthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"` // always "Bearer"
	ExpiresIn    int    `json:"expires_in"` // seconds
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// mintAccessAndRefresh creates an access token (1h) and a refresh token
// (30d), inserting a row into oauth_refresh_tokens that points at the
// access token's api_tokens row. Used by both the authorization-code and
// refresh-token grant paths.
func (h *OAuthHandler) mintAccessAndRefresh(client *oauthClientRow, userID, agentID int, scopes []string) (*oauthTokenResponse, error) {
	// Mint the `crw_…` access token. Bound to the agent (not the human
	// user) so audit logs and Windshift's per-actor accounting attribute
	// the actions to the OAuth client identity, not the human directly.
	expiresAt := time.Now().Add(oauthAccessTTL)
	tokenName := fmt.Sprintf("oauth-%s", client.Slug)
	tr, err := h.tokenManager.CreateToken(agentID, models.APITokenCreate{
		Name:        tokenName,
		Permissions: scopes,
		ExpiresAt:   &expiresAt,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to mint access token: %w", err)
	}

	// Mint the refresh token, hash it, and store the row. Hash is SHA-256
	// (not bcrypt) — the token itself is opaque random bytes, so the only
	// thing bcrypt would buy is slow lookup, and refresh tokens are checked
	// often enough for that to hurt.
	refreshPlain, err := generateOAuthRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}
	refreshHash := hashRefreshToken(refreshPlain)
	scopesJSON, _ := json.Marshal(scopes)

	if _, err := h.db.Exec(`
		INSERT INTO oauth_refresh_tokens (
			token_hash, api_token_id, client_id, user_id, agent_id,
			scopes, expires_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, refreshHash, tr.APIToken.ID, client.ClientID, userID, agentID,
		string(scopesJSON), time.Now().Add(oauthRefreshTTL)); err != nil {
		// Best-effort: revoke the just-minted access token so we don't
		// leave a live credential stranded if the refresh insert fails.
		_ = h.tokenManager.AdminRevokeToken(tr.APIToken.ID)
		return nil, fmt.Errorf("failed to record refresh token: %w", err)
	}

	return &oauthTokenResponse{
		AccessToken:  tr.Token,
		TokenType:    "Bearer",
		ExpiresIn:    int(oauthAccessTTL.Seconds()),
		RefreshToken: refreshPlain,
		Scope:        strings.Join(scopes, " "),
	}, nil
}

// authenticateClient validates a presented client_id + (optional) secret.
// Confidential clients require a matching secret; public clients pass with
// an empty secret (they're authenticated by PKCE on the token endpoint).
func (h *OAuthHandler) authenticateClient(clientID, clientSecret string) (*oauthClientRow, error) {
	client, err := h.lookupEnabledClientByClientID(clientID)
	if err != nil {
		return nil, fmt.Errorf("unknown or disabled client_id")
	}

	if client.ClientType == "public" {
		// Public clients have no secret — empty value is fine. PKCE is the
		// per-request authentication mechanism (enforced by the
		// authorization_code grant).
		return client, nil
	}

	// Confidential client: secret must match the stored bcrypt hash.
	if clientSecret == "" {
		return nil, fmt.Errorf("client_secret is required for confidential clients")
	}
	if !client.HasSecret {
		// Misconfiguration: confidential client with no secret hash. Refuse.
		return nil, fmt.Errorf("client has no configured secret")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(client.ClientSecretHash), []byte(clientSecret)); err != nil {
		return nil, fmt.Errorf("invalid client_secret")
	}
	return client, nil
}

// consumeAuthorizationCode atomically marks an authorization code consumed
// (via UPDATE … WHERE consumed_at IS NULL) and returns its full row. Returns
// errors for: not found, expired, already consumed.
func (h *OAuthHandler) consumeAuthorizationCode(code string) (*oauthAuthCodeRow, error) {
	row := &oauthAuthCodeRow{}
	var (
		codeChallenge       sql.NullString
		codeChallengeMethod sql.NullString
		consumedAt          sql.NullTime
		state               sql.NullString
	)
	err := h.db.QueryRow(`
		SELECT id, code, client_id, user_id, agent_id, redirect_uri, scopes,
			code_challenge, code_challenge_method, state, expires_at, consumed_at
		FROM oauth_authorization_codes
		WHERE code = ?
	`, code).Scan(&row.ID, &row.Code, &row.ClientID, &row.UserID, &row.AgentID,
		&row.RedirectURI, &row.Scopes, &codeChallenge, &codeChallengeMethod,
		&state, &row.ExpiresAt, &consumedAt)
	if err != nil {
		return nil, err
	}
	row.CodeChallenge = codeChallenge.String
	row.CodeChallengeMethod = codeChallengeMethod.String
	row.State = state.String

	if consumedAt.Valid {
		return nil, fmt.Errorf("authorization code has already been consumed")
	}
	if time.Now().After(row.ExpiresAt) {
		return nil, fmt.Errorf("authorization code has expired")
	}

	res, err := h.db.Exec(`
		UPDATE oauth_authorization_codes
		SET consumed_at = CURRENT_TIMESTAMP
		WHERE id = ? AND consumed_at IS NULL
	`, row.ID)
	if err != nil {
		return nil, err
	}
	affected, _ := res.RowsAffected()
	if affected != 1 {
		return nil, fmt.Errorf("authorization code race: already consumed")
	}
	return row, nil
}

// lookupRefreshToken finds a refresh-token row by SHA-256 hash of the plain
// token. Returns sql.ErrNoRows if there's no match.
func (h *OAuthHandler) lookupRefreshToken(plain string) (*oauthRefreshTokenRow, error) {
	hashHex := hashRefreshToken(plain)
	row := &oauthRefreshTokenRow{}
	var revokedAt sql.NullTime
	var rotatedToID sql.NullInt64
	err := h.db.QueryRow(`
		SELECT id, token_hash, api_token_id, client_id, user_id, agent_id,
			scopes, expires_at, revoked_at, rotated_to_id
		FROM oauth_refresh_tokens
		WHERE token_hash = ?
	`, hashHex).Scan(&row.ID, &row.TokenHash, &row.APITokenID, &row.ClientID,
		&row.UserID, &row.AgentID, &row.Scopes, &row.ExpiresAt, &revokedAt, &rotatedToID)
	if err != nil {
		return nil, err
	}
	row.RevokedAt = revokedAt
	if rotatedToID.Valid {
		v := int(rotatedToID.Int64)
		row.RotatedToID = &v
	}
	return row, nil
}

// cascadeRevokeRefreshChain marks the given refresh-token row and every row
// reachable via rotated_to_id (forward chain) as revoked. Called when a
// revoked token is replayed — the safe assumption is that whoever held the
// old token also holds the rotation chain that came after it.
//
// Also revokes the api_tokens that those refresh rows point at, so leaked
// access tokens get cut off at the same time.
func (h *OAuthHandler) cascadeRevokeRefreshChain(startID int) {
	// Walk the chain forward, collecting ids + api_token_ids.
	var (
		visited     = map[int]struct{}{}
		ids         = []int{}
		apiTokenIDs = []int{}
		queue       = []int{startID}
	)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if _, ok := visited[current]; ok {
			continue
		}
		visited[current] = struct{}{}
		ids = append(ids, current)

		var apiTokenID int
		var rotatedTo sql.NullInt64
		err := h.db.QueryRow(
			`SELECT api_token_id, rotated_to_id FROM oauth_refresh_tokens WHERE id = ?`,
			current,
		).Scan(&apiTokenID, &rotatedTo)
		if err != nil {
			continue
		}
		apiTokenIDs = append(apiTokenIDs, apiTokenID)
		if rotatedTo.Valid {
			queue = append(queue, int(rotatedTo.Int64))
		}
	}

	for _, id := range ids {
		_, _ = h.db.Exec(
			`UPDATE oauth_refresh_tokens SET revoked_at = CURRENT_TIMESTAMP WHERE id = ? AND revoked_at IS NULL`, id,
		)
	}
	for _, tid := range apiTokenIDs {
		_ = h.tokenManager.AdminRevokeToken(tid)
	}
}

// oauthClientRow is the in-memory shape we use for client lookups inside
// the OAuth handler. Distinct from the admin handler's response type
// because we need the secret hash for authenticate calls.
type oauthClientRow struct {
	ID               int
	Slug             string
	DisplayName      string
	ClientID         string
	ClientType       string
	HasSecret        bool
	ClientSecretHash string
	RedirectURIs     []string
	AllowedScopes    []string
	Enabled          bool
}

// lookupEnabledClientByClientID returns the client row by client_id, or an
// error if the row doesn't exist or the client is disabled.
func (h *OAuthHandler) lookupEnabledClientByClientID(clientID string) (*oauthClientRow, error) {
	if clientID == "" {
		return nil, fmt.Errorf("client_id is required")
	}
	c := &oauthClientRow{}
	var secretHash sql.NullString
	var redirectsJSON, scopesJSON string
	err := h.db.QueryRow(`
		SELECT id, slug, display_name, client_id, client_type, client_secret_hash,
			redirect_uris, allowed_scopes, enabled
		FROM oauth_clients
		WHERE client_id = ?
	`, clientID).Scan(&c.ID, &c.Slug, &c.DisplayName, &c.ClientID, &c.ClientType,
		&secretHash, &redirectsJSON, &scopesJSON, &c.Enabled)
	if err != nil {
		return nil, err
	}
	if !c.Enabled {
		return nil, fmt.Errorf("client is disabled")
	}
	c.HasSecret = secretHash.Valid && secretHash.String != ""
	c.ClientSecretHash = secretHash.String
	if redirectsJSON != "" {
		_ = json.Unmarshal([]byte(redirectsJSON), &c.RedirectURIs)
	}
	if scopesJSON != "" {
		_ = json.Unmarshal([]byte(scopesJSON), &c.AllowedScopes)
	}
	return c, nil
}

// oauthAuthCodeRow is the in-memory shape of an authorization-code row.
type oauthAuthCodeRow struct {
	ID                  int
	Code                string
	ClientID            string
	UserID              int
	AgentID             int
	RedirectURI         string
	Scopes              string // JSON
	CodeChallenge       string
	CodeChallengeMethod string
	State               string
	ExpiresAt           time.Time
}

// oauthRefreshTokenRow is the in-memory shape of a refresh-token row.
type oauthRefreshTokenRow struct {
	ID          int
	TokenHash   string
	APITokenID  int
	ClientID    string
	UserID      int
	AgentID     int
	Scopes      string // JSON
	ExpiresAt   time.Time
	RevokedAt   sql.NullTime
	RotatedToID *int
}

// audit emits a single audit-log entry for an OAuth event from the user side.
func (h *OAuthHandler) audit(r *http.Request, user *models.User, action string, resourceID *int, resourceName string, success bool, details map[string]interface{}) {
	_ = logger.LogAudit(h.db, logger.AuditEvent{
		UserID:       user.ID,
		Username:     user.Username,
		IPAddress:    utils.GetClientIP(r),
		UserAgent:    r.UserAgent(),
		ActionType:   action,
		ResourceType: "oauth_client",
		ResourceID:   resourceID,
		ResourceName: resourceName,
		Details:      details,
		Success:      success,
	})
}

// auditClient emits an audit entry for a /token-side event where there's no
// authenticated user — only the OAuth client and (sometimes) a user_id in
// the details bag.
func (h *OAuthHandler) auditClient(r *http.Request, client *oauthClientRow, action string, details map[string]interface{}) {
	_ = logger.LogAudit(h.db, logger.AuditEvent{
		IPAddress:    utils.GetClientIP(r),
		UserAgent:    r.UserAgent(),
		ActionType:   action,
		ResourceType: "oauth_client",
		ResourceID:   &client.ID,
		ResourceName: client.DisplayName,
		Details:      details,
		Success:      true,
	})
}

// ---- helpers ----

// parseTokenRequest reads the /token body. RFC 6749 §3.2 mandates form
// encoding, but we also accept JSON for clients that prefer it. Returns a
// url.Values either way so callers don't branch on content-type.
func parseTokenRequest(r *http.Request) (url.Values, error) {
	ct := r.Header.Get("Content-Type")
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = ct[:i]
	}
	ct = strings.TrimSpace(ct)
	if ct == "application/json" {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, fmt.Errorf("invalid JSON body: %w", err)
		}
		v := url.Values{}
		for k, val := range body {
			v.Set(k, val)
		}
		return v, nil
	}
	if err := r.ParseForm(); err != nil {
		return nil, fmt.Errorf("invalid form body: %w", err)
	}
	return r.PostForm, nil
}

// writeOAuthTokenError writes an RFC 6749 §5.2 error response.
func writeOAuthTokenError(w http.ResponseWriter, status int, code, description string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(status)
	body, _ := json.Marshal(map[string]string{
		"error":             code,
		"error_description": description,
	})
	_, _ = w.Write(body)
}

// splitOAuthScopes parses a space-separated scope string per RFC 6749. Empty
// entries are dropped; whitespace runs are tolerated.
func splitOAuthScopes(scopeStr string) []string {
	if strings.TrimSpace(scopeStr) == "" {
		return nil
	}
	return strings.Fields(scopeStr)
}

// intersectScopes returns the requested scopes that are in the client's
// allowed_scopes list. Validates each requested scope against the global
// scope catalog and rejects admin-class scopes outright.
func intersectScopes(requested, allowed []string) ([]string, error) {
	if err := auth.ValidateScopes(requested); err != nil {
		return nil, err
	}
	allowedSet := map[string]struct{}{}
	for _, s := range allowed {
		allowedSet[s] = struct{}{}
	}
	out := make([]string, 0, len(requested))
	for _, s := range requested {
		if auth.IsAdminScope(s) {
			return nil, fmt.Errorf("scope %q is not grantable via OAuth", s)
		}
		if _, ok := allowedSet[s]; !ok {
			return nil, fmt.Errorf("scope %q is not allowed for this client", s)
		}
		out = append(out, s)
	}
	return out, nil
}

// unmarshalScopes decodes a JSON-stored scope array back to a string slice.
func unmarshalScopes(raw string) ([]string, error) {
	if raw == "" {
		return nil, nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// redirectURIAllowed checks the candidate against the client's exact-match
// whitelist. RFC 6749 §3.1.2 mandates exact comparison; we don't allow
// pattern matching or scheme/host coercion.
func redirectURIAllowed(allowed []string, candidate string) bool {
	for _, a := range allowed {
		if a == candidate {
			return true
		}
	}
	return false
}

// verifyPKCE compares a code_verifier against a stored code_challenge per
// RFC 7636. Constant-time comparison to avoid leaking match-prefix info.
func verifyPKCE(challenge, method, verifier string) error {
	switch method {
	case "S256":
		sum := sha256.Sum256([]byte(verifier))
		computed := base64.RawURLEncoding.EncodeToString(sum[:])
		if subtle.ConstantTimeCompare([]byte(computed), []byte(challenge)) != 1 {
			return fmt.Errorf("PKCE verification failed")
		}
		return nil
	case "plain", "":
		if subtle.ConstantTimeCompare([]byte(verifier), []byte(challenge)) != 1 {
			return fmt.Errorf("PKCE verification failed")
		}
		return nil
	default:
		return fmt.Errorf("unsupported code_challenge_method: %s", method)
	}
}

// appendQuery returns base with `?k=v` (or `&k=v` if base already has a
// query string) for each entry in params. Values are URL-encoded.
func appendQuery(base string, params map[string]string) string {
	if len(params) == 0 {
		return base
	}
	v := url.Values{}
	for k, val := range params {
		if val != "" {
			v.Set(k, val)
		}
	}
	if v.Encode() == "" {
		return base
	}
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	return base + sep + v.Encode()
}

// generateOAuthRefreshToken returns a 64-char hex refresh token prefixed
// with `wsrt_` (windshift refresh token). Stored only as SHA-256 hex.
func generateOAuthRefreshToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "wsrt_" + hex.EncodeToString(buf), nil
}

// nullStringOrEmpty turns "" into a NULL-safe value for sql.Exec where the
// column is nullable. Plain empty string would be stored as ”; we want NULL
// so the consumed_at-style UPDATE-where-NULL guard works.
func nullStringOrEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
