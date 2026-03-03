package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"windshift/internal/database"
	"windshift/internal/middleware"
	"windshift/internal/models"
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
	user, ok := r.Context().Value(middleware.ContextKeyUser).(*models.User)
	if !ok {
		respondUnauthorized(w, r)
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

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(connections)
}

// GetConnectionStatus returns the user's connection status for a specific provider
func (h *UserSCMTokenHandler) GetConnectionStatus(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value(middleware.ContextKeyUser).(*models.User)
	if !ok {
		respondUnauthorized(w, r)
		return
	}

	providerID, err := strconv.Atoi(r.PathValue("provider_id"))
	if err != nil {
		respondInvalidID(w, r, "provider_id")
		return
	}

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

	if err == sql.ErrNoRows {
		// User not connected - return provider info without connection
		var providerName string
		var providerType models.SCMProviderType
		var providerSlug string
		var authMethod models.SCMAuthMethod

		err = h.db.QueryRow(`
			SELECT name, provider_type, slug, auth_method
			FROM scm_providers WHERE id = ? AND enabled = true
		`, providerID).Scan(&providerName, &providerType, &providerSlug, &authMethod)

		if err == sql.ErrNoRows {
			respondNotFound(w, r, "provider")
			return
		}
		if err != nil {
			respondInternalError(w, r, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
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

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"connected":  true,
		"connection": conn,
	})
}

// DisconnectProvider removes the user's connection to an SCM provider
func (h *UserSCMTokenHandler) DisconnectProvider(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value(middleware.ContextKeyUser).(*models.User)
	if !ok {
		respondUnauthorized(w, r)
		return
	}

	providerID, err := strconv.Atoi(r.PathValue("provider_id"))
	if err != nil {
		respondInvalidID(w, r, "provider_id")
		return
	}

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

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "SCM account disconnected",
	})
}

// GetAvailableProviders returns all OAuth SCM providers that the user can connect to
func (h *UserSCMTokenHandler) GetAvailableProviders(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value(middleware.ContextKeyUser).(*models.User)
	if !ok {
		respondUnauthorized(w, r)
		return
	}

	// Get all enabled OAuth providers and whether the user is connected
	rows, err := h.db.Query(`
		SELECT
			sp.id, sp.name, sp.provider_type, sp.slug, sp.auth_method,
			CASE WHEN ut.id IS NOT NULL THEN 1 ELSE 0 END as is_connected,
			ut.scm_username, ut.scm_avatar_url, ut.connected_at
		FROM scm_providers sp
		LEFT JOIN user_scm_oauth_tokens ut ON ut.scm_provider_id = sp.id AND ut.user_id = ?
		WHERE sp.enabled = true AND sp.auth_method = 'oauth'
		ORDER BY sp.name
	`, user.ID)
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

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(providers)
}
