package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
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

// AdminOAuthClientHandler backs the admin CRUD for `oauth_clients`. The OAuth
// 2.0 server itself (authorize/token endpoints) lives in oauth.go; this file
// only manages the registered-client records.
type AdminOAuthClientHandler struct {
	db                database.Database
	tokenManager      *auth.TokenManager
	permissionService *services.PermissionService
}

// NewAdminOAuthClientHandler wires the handler. tokenManager is required so
// that DeleteClient can cascade-revoke the access tokens issued by the client
// before the row goes away (CASCADE wipes the refresh-token rows; this
// handler revokes the api_tokens those refresh tokens point at).
func NewAdminOAuthClientHandler(db database.Database, tm *auth.TokenManager, ps *services.PermissionService) *AdminOAuthClientHandler {
	return &AdminOAuthClientHandler{
		db:                db,
		tokenManager:      tm,
		permissionService: ps,
	}
}

// OAuthClientResponse is the public shape returned to admin clients. Never
// contains the secret hash; HasSecret says only whether one exists.
type OAuthClientResponse struct {
	ID            int       `json:"id"`
	Slug          string    `json:"slug"`
	DisplayName   string    `json:"display_name"`
	ClientID      string    `json:"client_id"`
	ClientType    string    `json:"client_type"`
	HasSecret     bool      `json:"has_secret"`
	RedirectURIs  []string  `json:"redirect_uris"`
	AllowedScopes []string  `json:"allowed_scopes"`
	Enabled       bool      `json:"enabled"`
	CreatedBy     int       `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// OAuthClientCreateResponse adds the one-time secret to the standard shape.
// The plaintext secret is returned exactly once at creation/rotation; admins
// must copy it then because the server only stores the bcrypt hash.
type OAuthClientCreateResponse struct {
	OAuthClientResponse
	ClientSecret string `json:"client_secret"`
}

// OAuthClientCreateRequest is the admin-supplied registration payload.
type OAuthClientCreateRequest struct {
	Slug          string   `json:"slug"`
	DisplayName   string   `json:"display_name"`
	ClientType    string   `json:"client_type,omitempty"` // "confidential" (default) | "public"
	RedirectURIs  []string `json:"redirect_uris"`
	AllowedScopes []string `json:"allowed_scopes"`
	Enabled       *bool    `json:"enabled,omitempty"`
}

// OAuthClientUpdateRequest is partial: any nil/empty field is left alone.
// Slug, ClientID, and ClientType are immutable post-creation.
type OAuthClientUpdateRequest struct {
	DisplayName   string   `json:"display_name,omitempty"`
	RedirectURIs  []string `json:"redirect_uris,omitempty"`
	AllowedScopes []string `json:"allowed_scopes,omitempty"`
	Enabled       *bool    `json:"enabled,omitempty"`
}

// GetClients returns all registered OAuth clients (no secrets).
func (h *AdminOAuthClientHandler) GetClients(w http.ResponseWriter, r *http.Request) {
	if _, ok := RequireAuth(w, r); !ok {
		return
	}

	rows, err := h.db.Query(`
		SELECT id, slug, display_name, client_id, client_type, client_secret_hash,
			redirect_uris, allowed_scopes, enabled, created_by, created_at, updated_at
		FROM oauth_clients
		ORDER BY display_name
	`)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	clients := []OAuthClientResponse{}
	for rows.Next() {
		c, err := scanOAuthClient(rows)
		if err != nil {
			continue
		}
		clients = append(clients, c)
	}
	if err := rows.Err(); err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, clients)
}

// GetClient returns a single client by numeric id.
func (h *AdminOAuthClientHandler) GetClient(w http.ResponseWriter, r *http.Request) {
	if _, ok := RequireAuth(w, r); !ok {
		return
	}
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	row := h.queryClientByID(id)
	c, err := scanOAuthClient(row)
	if errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "oauth_client")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, c)
}

// CreateClient registers a new OAuth client. The client_id and client_secret
// are server-generated. The plaintext secret is returned exactly once; the
// server stores only the bcrypt hash.
//
// Public clients (client_type=public) get no secret at all and must use PKCE
// on every /token exchange.
func (h *AdminOAuthClientHandler) CreateClient(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	req, ok := decodeJSON[OAuthClientCreateRequest](w, r)
	if !ok {
		return
	}

	req.Slug = strings.TrimSpace(req.Slug)
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	if req.Slug == "" || req.DisplayName == "" {
		respondValidationError(w, r, "slug and display_name are required")
		return
	}
	clientType := strings.TrimSpace(req.ClientType)
	if clientType == "" {
		clientType = "confidential"
	}
	if clientType != "confidential" && clientType != "public" {
		respondValidationError(w, r, "client_type must be 'confidential' or 'public'")
		return
	}

	if err := validateRedirectURIs(req.RedirectURIs); err != nil {
		respondValidationError(w, r, err.Error())
		return
	}
	if err := auth.ValidateScopes(req.AllowedScopes); err != nil {
		respondValidationError(w, r, err.Error())
		return
	}
	for _, s := range req.AllowedScopes {
		if auth.IsAdminScope(s) {
			respondValidationError(w, r, "admin-scoped scopes cannot be granted to OAuth clients")
			return
		}
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	clientID, err := generateOAuthClientID()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	var (
		secretPlain string
		secretHash  sql.NullString
	)
	if clientType == "confidential" {
		secretPlain, err = generateOAuthClientSecret()
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		hash, hashErr := bcrypt.GenerateFromPassword([]byte(secretPlain), bcrypt.DefaultCost)
		if hashErr != nil {
			respondInternalError(w, r, hashErr)
			return
		}
		secretHash = sql.NullString{String: string(hash), Valid: true}
	}

	redirectsJSON, _ := json.Marshal(req.RedirectURIs)
	scopesJSON, _ := json.Marshal(req.AllowedScopes)

	_, err = h.db.Exec(`
		INSERT INTO oauth_clients (
			slug, display_name, client_id, client_secret_hash, client_type,
			redirect_uris, allowed_scopes, enabled, created_by
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, req.Slug, req.DisplayName, clientID, secretHash, clientType,
		string(redirectsJSON), string(scopesJSON), enabled, user.ID)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			respondConflict(w, r, "An OAuth client with this slug or client_id already exists")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	row := h.queryClientByClientID(clientID)
	created, err := scanOAuthClient(row)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	h.audit(r, user, "oauth_client.create", &created.ID, created.DisplayName, map[string]interface{}{
		"slug":           created.Slug,
		"client_id":      created.ClientID,
		"client_type":    created.ClientType,
		"redirect_uris":  created.RedirectURIs,
		"allowed_scopes": created.AllowedScopes,
	})

	respondJSONCreated(w, OAuthClientCreateResponse{
		OAuthClientResponse: created,
		ClientSecret:        secretPlain, // empty for public clients
	})
}

// UpdateClient mutates the client. Slug, client_id, and client_type are
// immutable: changing them would invalidate every token already issued and is
// usually a sign you wanted a new client instead.
func (h *AdminOAuthClientHandler) UpdateClient(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := decodeJSON[OAuthClientUpdateRequest](w, r)
	if !ok {
		return
	}

	existing, err := scanOAuthClient(h.queryClientByID(id))
	if errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "oauth_client")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	changes := map[string]interface{}{}

	if dn := strings.TrimSpace(req.DisplayName); dn != "" && dn != existing.DisplayName {
		if _, err := h.db.Exec(`UPDATE oauth_clients SET display_name = ? WHERE id = ?`, dn, id); err != nil {
			respondInternalError(w, r, err)
			return
		}
		changes["display_name"] = dn
	}
	if req.RedirectURIs != nil {
		if err := validateRedirectURIs(req.RedirectURIs); err != nil {
			respondValidationError(w, r, err.Error())
			return
		}
		j, _ := json.Marshal(req.RedirectURIs)
		if _, err := h.db.Exec(`UPDATE oauth_clients SET redirect_uris = ? WHERE id = ?`, string(j), id); err != nil {
			respondInternalError(w, r, err)
			return
		}
		changes["redirect_uris"] = req.RedirectURIs
	}
	if req.AllowedScopes != nil {
		if err := auth.ValidateScopes(req.AllowedScopes); err != nil {
			respondValidationError(w, r, err.Error())
			return
		}
		for _, s := range req.AllowedScopes {
			if auth.IsAdminScope(s) {
				respondValidationError(w, r, "admin-scoped scopes cannot be granted to OAuth clients")
				return
			}
		}
		j, _ := json.Marshal(req.AllowedScopes)
		if _, err := h.db.Exec(`UPDATE oauth_clients SET allowed_scopes = ? WHERE id = ?`, string(j), id); err != nil {
			respondInternalError(w, r, err)
			return
		}
		changes["allowed_scopes"] = req.AllowedScopes
	}
	if req.Enabled != nil && *req.Enabled != existing.Enabled {
		if _, err := h.db.Exec(`UPDATE oauth_clients SET enabled = ? WHERE id = ?`, *req.Enabled, id); err != nil {
			respondInternalError(w, r, err)
			return
		}
		changes["enabled"] = *req.Enabled
	}

	if _, err := h.db.Exec(`UPDATE oauth_clients SET updated_at = CURRENT_TIMESTAMP WHERE id = ?`, id); err != nil {
		respondInternalError(w, r, err)
		return
	}

	updated, err := scanOAuthClient(h.queryClientByID(id))
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if len(changes) > 0 {
		h.audit(r, user, "oauth_client.update", &updated.ID, updated.DisplayName, map[string]interface{}{
			"changes": changes,
		})
	}

	respondJSONOK(w, updated)
}

// RotateSecret generates a new client_secret and returns the plaintext exactly
// once. Existing access/refresh tokens issued under the old secret keep
// working — only future /token exchanges that present the old secret start
// failing.
func (h *AdminOAuthClientHandler) RotateSecret(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	existing, err := scanOAuthClient(h.queryClientByID(id))
	if errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "oauth_client")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if existing.ClientType != "confidential" {
		respondValidationError(w, r, "Public clients have no secret to rotate")
		return
	}

	secret, err := generateOAuthClientSecret()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if _, err := h.db.Exec(
		`UPDATE oauth_clients SET client_secret_hash = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		string(hash), id,
	); err != nil {
		respondInternalError(w, r, err)
		return
	}

	h.audit(r, user, "oauth_client.rotate_secret", &existing.ID, existing.DisplayName, nil)

	respondJSONOK(w, OAuthClientCreateResponse{
		OAuthClientResponse: existing,
		ClientSecret:        secret,
	})
}

// DeleteClient removes the OAuth client and revokes every token issued by it.
//
// Cascade: ON DELETE CASCADE on oauth_authorization_codes and
// oauth_refresh_tokens wipes those rows automatically. Before the row is
// deleted, this handler revokes the api_tokens that refresh tokens pointed at
// — once the refresh rows go away the link is gone, so the api_tokens have to
// be revoked first or they stay live until natural expiry.
func (h *AdminOAuthClientHandler) DeleteClient(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	existing, err := scanOAuthClient(h.queryClientByID(id))
	if errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "oauth_client")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	revoked, err := h.cascadeRevokeTokensForClient(existing.ClientID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if _, err := h.db.Exec(`DELETE FROM oauth_clients WHERE id = ?`, id); err != nil {
		respondInternalError(w, r, err)
		return
	}

	h.audit(r, user, "oauth_client.delete", &existing.ID, existing.DisplayName, map[string]interface{}{
		"client_id":      existing.ClientID,
		"tokens_revoked": revoked,
	})

	w.WriteHeader(http.StatusNoContent)
}

// cascadeRevokeTokensForClient revokes every api_token that has a refresh
// token pointing at the given client. Returns the count for audit logging.
// The CASCADE on oauth_refresh_tokens.client_id wipes the refresh rows when
// the client itself is deleted; this just handles the access-token side.
func (h *AdminOAuthClientHandler) cascadeRevokeTokensForClient(clientID string) (int, error) {
	rows, err := h.db.Query(
		`SELECT DISTINCT api_token_id FROM oauth_refresh_tokens WHERE client_id = ?`,
		clientID,
	)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()

	var tokenIDs []int
	for rows.Next() {
		var tid int
		if err := rows.Scan(&tid); err != nil {
			return 0, err
		}
		tokenIDs = append(tokenIDs, tid)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	revoked := 0
	for _, tid := range tokenIDs {
		// Best-effort: a token may already be revoked or expired. Skip
		// errors so a single stale row doesn't block the whole delete.
		if err := h.tokenManager.AdminRevokeToken(tid); err == nil {
			revoked++
		}
	}
	return revoked, nil
}

// audit emits a single audit-log entry for an oauth_client.* action. Errors
// are swallowed because audit-log failures shouldn't break admin actions.
func (h *AdminOAuthClientHandler) audit(r *http.Request, user *models.User, action string, resourceID *int, resourceName string, details map[string]interface{}) {
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
		Success:      true,
	})
}

// queryClientByID returns a row for the SELECT-everything query, by numeric id.
func (h *AdminOAuthClientHandler) queryClientByID(id int) *sql.Row {
	return h.db.QueryRow(`
		SELECT id, slug, display_name, client_id, client_type, client_secret_hash,
			redirect_uris, allowed_scopes, enabled, created_by, created_at, updated_at
		FROM oauth_clients WHERE id = ?
	`, id)
}

// queryClientByClientID returns the same shape, by the OAuth-public client_id.
func (h *AdminOAuthClientHandler) queryClientByClientID(clientID string) *sql.Row {
	return h.db.QueryRow(`
		SELECT id, slug, display_name, client_id, client_type, client_secret_hash,
			redirect_uris, allowed_scopes, enabled, created_by, created_at, updated_at
		FROM oauth_clients WHERE client_id = ?
	`, clientID)
}

// oauthClientScanner unifies sql.Row and sql.Rows so scanOAuthClient backs
// both the list and the single-row paths.
type oauthClientScanner interface {
	Scan(dest ...interface{}) error
}

func scanOAuthClient(s oauthClientScanner) (OAuthClientResponse, error) {
	var (
		c          OAuthClientResponse
		secretHash sql.NullString
		redirects  string
		scopes     string
	)
	if err := s.Scan(
		&c.ID, &c.Slug, &c.DisplayName, &c.ClientID, &c.ClientType, &secretHash,
		&redirects, &scopes, &c.Enabled, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt,
	); err != nil {
		return c, err
	}
	c.HasSecret = secretHash.Valid && secretHash.String != ""
	if redirects != "" {
		_ = json.Unmarshal([]byte(redirects), &c.RedirectURIs)
	}
	if scopes != "" {
		_ = json.Unmarshal([]byte(scopes), &c.AllowedScopes)
	}
	if c.RedirectURIs == nil {
		c.RedirectURIs = []string{}
	}
	if c.AllowedScopes == nil {
		c.AllowedScopes = []string{}
	}
	return c, nil
}

// validateRedirectURIs enforces basic shape: non-empty, parseable, and either
// http/https or a custom scheme. Loopback constraints from cli_auth aren't
// applied — OAuth third-party apps can register any URL the admin trusts.
func validateRedirectURIs(uris []string) error {
	if len(uris) == 0 {
		return errInvalidf("at least one redirect_uri is required")
	}
	for _, u := range uris {
		u = strings.TrimSpace(u)
		if u == "" {
			return errInvalidf("redirect_uris must not contain empty entries")
		}
	}
	return nil
}

// generateOAuthClientID returns a 32-char hex identifier prefixed with
// `wsoc_` (windshift OAuth client). Opaque, server-generated, non-secret.
func generateOAuthClientID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "wsoc_" + hex.EncodeToString(buf), nil
}

// generateOAuthClientSecret returns a 64-char hex secret prefixed with
// `wsos_` (windshift OAuth secret). Returned to the admin exactly once;
// stored as a bcrypt hash.
func generateOAuthClientSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "wsos_" + hex.EncodeToString(buf), nil
}

// hashRefreshToken is a SHA-256 hex digest. Used by the OAuth server (oauth.go)
// to store and verify refresh tokens at rest. Lives here next to the other
// token helpers so the admin handler and the OAuth server share one
// definition. SHA-256 (not bcrypt) because refresh tokens are checked too
// often for bcrypt's cost — they're already opaque random tokens, so the
// extra entropy a slow hash buys you is unnecessary.
func hashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// errInvalidf builds a tiny inline error for validation paths that don't
// warrant their own sentinel. Avoids pulling in fmt for one-line messages.
func errInvalidf(msg string) error {
	return &validationStringError{msg: msg}
}

type validationStringError struct{ msg string }

func (e *validationStringError) Error() string { return e.msg }
