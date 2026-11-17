package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/scm"
	"windshift/internal/sso"
)

// UserSCMTokenHandler handles user-level SCM OAuth token management
type UserSCMTokenHandler struct {
	httpClients *scm.HTTPClientPool
	repo        *repository.UserSCMTokenRepository
	encryption  *sso.SecretEncryption
}

// NewUserSCMTokenHandler creates a new user SCM token handler
func NewUserSCMTokenHandler(repo *repository.UserSCMTokenRepository, encryption *sso.SecretEncryption, pools ...*scm.HTTPClientPool) *UserSCMTokenHandler {
	var pool *scm.HTTPClientPool
	if len(pools) > 0 {
		pool = pools[0]
	}
	if pool == nil {
		pool = scm.NewHTTPClientPool()
	}
	return &UserSCMTokenHandler{
		httpClients: pool,
		repo:        repo,
		encryption:  encryption,
	}
}

// GetUserConnections returns all SCM providers the user has connected
func (h *UserSCMTokenHandler) GetUserConnections(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	connections, err := h.repo.ListUserConnections(user.ID)
	if err != nil {
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

	conn, err := h.repo.GetUserConnection(user.ID, providerID)
	if errors.Is(err, repository.ErrNotFound) {
		// User not connected - return provider info without connection.
		provider, providerErr := h.repo.GetEnabledProviderInfo(providerID)
		if errors.Is(providerErr, repository.ErrNotFound) {
			respondNotFound(w, r, "provider")
			return
		}
		if providerErr != nil {
			respondInternalError(w, r, providerErr)
			return
		}

		respondJSONOK(w, map[string]any{
			"connected":     false,
			"provider_id":   providerID,
			"provider_name": provider.ProviderName,
			"provider_type": provider.ProviderType,
			"provider_slug": provider.ProviderSlug,
			"auth_method":   provider.AuthMethod,
		})
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, map[string]any{
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

	deleted, err := h.repo.DeleteUserConnection(user.ID, providerID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !deleted {
		respondNotFound(w, r, "connection")
		return
	}

	respondJSONOK(w, map[string]any{
		"success": true,
		"message": "SCM account disconnected",
	})
}

// attemptRemoteRevoke loads the user's access token plus the provider's
// OAuth client credentials and asks the provider to revoke the token.
// Errors are logged and swallowed — see the DisconnectProvider doc
// comment for why this is best-effort.
func (h *UserSCMTokenHandler) attemptRemoteRevoke(ctx context.Context, userID, providerID int) {
	info, err := h.repo.GetRemoteRevokeInfo(ctx, userID, providerID)
	if err != nil {
		// No row, no credentials, nothing to revoke. The downstream DELETE
		// will return 404 on its own.
		return
	}

	accessToken, err := h.encryption.Decrypt(info.EncryptedAccessToken)
	if err != nil {
		slog.Warn("disconnect: failed to decrypt access token; skipping remote revoke", slog.String("component", "scm"), slog.Int("user_id", userID), slog.Int("provider_id", providerID), slog.Any("error", err))
		return
	}
	clientSecret, err := h.encryption.Decrypt(info.EncryptedClientSecret)
	if err != nil {
		slog.Warn("disconnect: failed to decrypt client secret; skipping remote revoke", slog.String("component", "scm"), slog.Int("provider_id", providerID), slog.Any("error", err))
		return
	}

	provider, err := scm.NewProvider(scm.ProviderConfig{
		HTTPClientPool:    h.httpClients,
		ProviderType:      info.ProviderType,
		AuthMethod:        models.SCMAuthMethodOAuth,
		BaseURL:           info.BaseURL,
		OAuthClientID:     info.OAuthClientID,
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
	providers, err := h.repo.ListAvailableOAuthProviders(user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, providers)
}
