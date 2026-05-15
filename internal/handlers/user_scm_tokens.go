package handlers

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/scm"
	"windshift/internal/sso"
)

// UserSCMTokenHandler handles user-level SCM OAuth token management
type UserSCMTokenHandler struct {
	db         database.Database
	encryption *sso.SecretEncryption
}

// UserSCMConnectionResponse represents a user's connected SCM account
type UserSCMConnectionResponse struct {
	ID           int                    `json:"id"`
	ProviderID   int                    `json:"provider_id"`
	ProviderName string                 `json:"provider_name"`
	ProviderType models.SCMProviderType `json:"provider_type"`
	ProviderSlug string                 `json:"provider_slug"`
	AuthMethod   models.SCMAuthMethod   `json:"auth_method"`
	SCMUsername  string                 `json:"scm_username,omitempty"`
	SCMAvatarURL string                 `json:"scm_avatar_url,omitempty"`
	ConnectedAt  time.Time              `json:"connected_at"`
	LastUsedAt   *time.Time             `json:"last_used_at,omitempty"`
}

// NewUserSCMTokenHandler creates a new user SCM token handler
func NewUserSCMTokenHandler(db database.Database, encryption *sso.SecretEncryption) *UserSCMTokenHandler {
	return &UserSCMTokenHandler{
		db:         db,
		encryption: encryption,
	}
}

// GetUserConnections returns all SCM providers the user has connected
func (h *UserSCMTokenHandler) GetUserConnections(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	rows, err := h.db.Query(`
		SELECT
			ut.id, ut.scm_provider_id, sp.name, sp.provider_type, sp.slug, sp.auth_method,
			ut.scm_username, ut.scm_avatar_url, ut.connected_at, ut.last_used_at
		FROM user_scm_oauth_tokens ut
		JOIN scm_providers sp ON sp.id = ut.scm_provider_id
		WHERE ut.user_id = ? AND sp.enabled = true
		ORDER BY ut.connected_at DESC
	`, user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	connections := []UserSCMConnectionResponse{}
	for rows.Next() {
		var conn UserSCMConnectionResponse
		var scmUsername, scmAvatarURL sql.NullString
		var lastUsedAt sql.NullTime

		err := rows.Scan(
			&conn.ID, &conn.ProviderID, &conn.ProviderName, &conn.ProviderType,
			&conn.ProviderSlug, &conn.AuthMethod,
			&scmUsername, &scmAvatarURL, &conn.ConnectedAt, &lastUsedAt,
		)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		conn.SCMUsername = scmUsername.String
		conn.SCMAvatarURL = scmAvatarURL.String
		if lastUsedAt.Valid {
			conn.LastUsedAt = &lastUsedAt.Time
		}

		connections = append(connections, conn)
	}
	if err := rows.Err(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, connections)
}

// GetConnectionStatus returns the user's connection status for a specific provider
func (h *UserSCMTokenHandler) GetConnectionStatus(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	providerID, ok := requireIDParam(w, r, "provider_id")
	if !ok {
		return
	}

	var err error

	var conn UserSCMConnectionResponse
	var scmUsername, scmAvatarURL sql.NullString
	var lastUsedAt sql.NullTime

	err = h.db.QueryRow(`
		SELECT
			ut.id, ut.scm_provider_id, sp.name, sp.provider_type, sp.slug, sp.auth_method,
			ut.scm_username, ut.scm_avatar_url, ut.connected_at, ut.last_used_at
		FROM user_scm_oauth_tokens ut
		JOIN scm_providers sp ON sp.id = ut.scm_provider_id
		WHERE ut.user_id = ? AND ut.scm_provider_id = ?
	`, user.ID, providerID).Scan(
		&conn.ID, &conn.ProviderID, &conn.ProviderName, &conn.ProviderType,
		&conn.ProviderSlug, &conn.AuthMethod,
		&scmUsername, &scmAvatarURL, &conn.ConnectedAt, &lastUsedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		// User not connected - return provider info without connection
		var providerName string
		var providerType models.SCMProviderType
		var providerSlug string
		var authMethod models.SCMAuthMethod

		err = h.db.QueryRow(`
			SELECT name, provider_type, slug, auth_method
			FROM scm_providers WHERE id = ? AND enabled = true
		`, providerID).Scan(&providerName, &providerType, &providerSlug, &authMethod)

		if errors.Is(err, sql.ErrNoRows) {
			respondNotFound(w, r, "provider")
			return
		}
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		respondJSONOK(w, map[string]interface{}{
			"connected":     false,
			"provider_id":   providerID,
			"provider_name": providerName,
			"provider_type": providerType,
			"provider_slug": providerSlug,
			"auth_method":   authMethod,
		})
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	conn.SCMUsername = scmUsername.String
	conn.SCMAvatarURL = scmAvatarURL.String
	if lastUsedAt.Valid {
		conn.LastUsedAt = &lastUsedAt.Time
	}

	respondJSONOK(w, map[string]interface{}{
		"connected":  true,
		"connection": conn,
	})
}

// DisconnectProvider removes the user's connection to an SCM provider.
//
// Before deleting the local token row we make a best-effort attempt to
// revoke the access token at the remote provider (currently GitHub
// OAuth Apps; Gitea has no standardized revocation endpoint). A failure
// to revoke remotely is logged but does NOT block the local delete —
// the user explicitly asked to disconnect, and on next login the
// provider's authorization will be invalidated naturally when the token
// expires or via the user's account settings.
func (h *UserSCMTokenHandler) DisconnectProvider(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	providerID, ok := requireIDParam(w, r, "provider_id")
	if !ok {
		return
	}

	h.attemptRemoteRevoke(r.Context(), user.ID, providerID)

	result, err := h.db.Exec(`
		DELETE FROM user_scm_oauth_tokens
		WHERE user_id = ? AND scm_provider_id = ?
	`, user.ID, providerID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		respondNotFound(w, r, "connection")
		return
	}

	respondJSONOK(w, map[string]interface{}{
		"success": true,
		"message": "SCM account disconnected",
	})
}

// attemptRemoteRevoke loads the user's access token plus the provider's
// OAuth client credentials and asks the provider to revoke the token.
// Errors are logged and swallowed — see the DisconnectProvider doc
// comment for why this is best-effort.
func (h *UserSCMTokenHandler) attemptRemoteRevoke(ctx context.Context, userID, providerID int) {
	var encAccessToken sql.NullString
	var providerType models.SCMProviderType
	var clientID, clientSecretEnc, baseURL sql.NullString
	err := h.db.QueryRowContext(ctx, `
		SELECT ut.oauth_access_token_encrypted,
		       sp.provider_type, sp.oauth_client_id, sp.oauth_client_secret_encrypted, sp.base_url
		FROM user_scm_oauth_tokens ut
		JOIN scm_providers sp ON sp.id = ut.scm_provider_id
		WHERE ut.user_id = ? AND ut.scm_provider_id = ?
	`, userID, providerID).Scan(&encAccessToken, &providerType, &clientID, &clientSecretEnc, &baseURL)
	if err != nil {
		// No row, no credentials, nothing to revoke. The downstream DELETE
		// will return 404 on its own.
		return
	}

	if !encAccessToken.Valid || encAccessToken.String == "" || !clientID.Valid || !clientSecretEnc.Valid {
		return
	}

	accessToken, err := h.encryption.Decrypt(encAccessToken.String)
	if err != nil {
		slog.Warn("disconnect: failed to decrypt access token; skipping remote revoke", slog.String("component", "scm"), slog.Int("user_id", userID), slog.Int("provider_id", providerID), slog.Any("error", err))
		return
	}
	clientSecret, err := h.encryption.Decrypt(clientSecretEnc.String)
	if err != nil {
		slog.Warn("disconnect: failed to decrypt client secret; skipping remote revoke", slog.String("component", "scm"), slog.Int("provider_id", providerID), slog.Any("error", err))
		return
	}

	provider, err := scm.NewProvider(scm.ProviderConfig{
		ProviderType:      providerType,
		AuthMethod:        models.SCMAuthMethodOAuth,
		BaseURL:           baseURL.String,
		OAuthClientID:     clientID.String,
		OAuthClientSecret: clientSecret,
		OAuthAccessToken:  accessToken,
	})
	if err != nil {
		slog.Warn("disconnect: provider construction failed; skipping remote revoke", slog.String("component", "scm"), slog.Int("provider_id", providerID), slog.Any("error", err))
		return
	}

	revoker, ok := provider.(scm.TokenRevoker)
	if !ok {
		// Provider doesn't support remote revocation (e.g. Gitea). The
		// local DELETE that follows is enough.
		return
	}

	revokeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := revoker.RevokeToken(revokeCtx, accessToken); err != nil {
		slog.Warn("disconnect: remote revoke failed; proceeding with local disconnect", slog.String("component", "scm"), slog.Int("user_id", userID), slog.Int("provider_id", providerID), slog.Any("error", err))
		return
	}
	slog.Info("disconnect: remote OAuth token revoked", slog.String("component", "scm"), slog.Int("user_id", userID), slog.Int("provider_id", providerID))
}

// GetAvailableProviders returns all OAuth SCM providers that the user can connect to
func (h *UserSCMTokenHandler) GetAvailableProviders(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Get all enabled OAuth providers and whether the user is connected.
	// Restricted providers are only listed when the user belongs to at least
	// one workspace on the provider's allowlist — otherwise the UI would
	// offer a "Connect" button that StartOAuth then 404s.
	rows, err := h.db.Query(`
		SELECT
			sp.id, sp.name, sp.provider_type, sp.slug, sp.auth_method,
			CASE WHEN ut.id IS NOT NULL THEN 1 ELSE 0 END as is_connected,
			ut.scm_username, ut.scm_avatar_url, ut.connected_at
		FROM scm_providers sp
		LEFT JOIN user_scm_oauth_tokens ut ON ut.scm_provider_id = sp.id AND ut.user_id = ?
		WHERE sp.enabled = true
		  AND sp.auth_method = 'oauth'
		  AND (
			COALESCE(sp.workspace_restriction_mode, 'unrestricted') = 'unrestricted'
			OR EXISTS (
				SELECT 1
				FROM scm_provider_workspace_allowlist al
				JOIN user_workspace_roles uwr
				  ON uwr.workspace_id = al.workspace_id
				 AND uwr.user_id = ?
				WHERE al.provider_id = sp.id
			)
		  )
		ORDER BY sp.name
	`, user.ID, user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	type ProviderWithStatus struct {
		ID           int                    `json:"id"`
		Name         string                 `json:"name"`
		ProviderType models.SCMProviderType `json:"provider_type"`
		Slug         string                 `json:"slug"`
		AuthMethod   models.SCMAuthMethod   `json:"auth_method"`
		IsConnected  bool                   `json:"is_connected"`
		SCMUsername  string                 `json:"scm_username,omitempty"`
		SCMAvatarURL string                 `json:"scm_avatar_url,omitempty"`
		ConnectedAt  *time.Time             `json:"connected_at,omitempty"`
	}

	providers := []ProviderWithStatus{}
	for rows.Next() {
		var p ProviderWithStatus
		var isConnected int
		var scmUsername, scmAvatarURL sql.NullString
		var connectedAt sql.NullTime

		err := rows.Scan(
			&p.ID, &p.Name, &p.ProviderType, &p.Slug, &p.AuthMethod,
			&isConnected, &scmUsername, &scmAvatarURL, &connectedAt,
		)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		p.IsConnected = isConnected == 1
		p.SCMUsername = scmUsername.String
		p.SCMAvatarURL = scmAvatarURL.String
		if connectedAt.Valid {
			p.ConnectedAt = &connectedAt.Time
		}

		providers = append(providers, p)
	}
	if err := rows.Err(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, providers)
}
