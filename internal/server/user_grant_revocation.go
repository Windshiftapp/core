package server

import (
	"context"
	"log/slog"
	"time"

	"windshift/internal/integrations/todoist"
	"windshift/internal/models"
	"windshift/internal/scm"
	"windshift/internal/services"
	"windshift/internal/sso"
)

// revokeUserRemoteGrants best-effort revokes the OAuth grants an offboarded
// user held at external providers (GitHub OAuth Apps, Todoist). The local
// token rows are already gone at this point; a failure here must never fail
// the offboarding, so every error is logged and swallowed. Providers without
// a revocation API (Gitea, Notion) are skipped — their access tokens simply
// expire unused at the provider.
func (s *Server) revokeUserRemoteGrants(revocations []services.PendingRemoteRevocation) {
	if len(revocations) == 0 {
		return
	}
	if s.secretEncryption == nil {
		slog.Warn("offboarding: no secret encryption configured; skipping remote OAuth revocation",
			slog.String("component", "offboarding"),
			slog.Int("grants", len(revocations)),
		)
		return
	}
	enc := s.secretEncryption

	for _, rev := range revocations {
		accessToken, err := enc.Decrypt(rev.EncryptedAccessToken)
		if err != nil {
			slog.Warn("offboarding: failed to decrypt access token; skipping remote revoke",
				slog.String("component", "offboarding"),
				slog.String("kind", rev.Kind),
				slog.String("provider", rev.ProviderType),
				slog.Any("error", err),
			)
			continue
		}

		switch rev.Kind {
		case services.RevocationKindSCM:
			revokeSCMGrant(enc, rev, accessToken)
		case services.RevocationKindIntegration:
			revokeIntegrationGrant(enc, rev, accessToken)
		default:
			slog.Warn("offboarding: unknown revocation kind; skipping",
				slog.String("component", "offboarding"),
				slog.String("kind", rev.Kind),
			)
		}
	}
}

// revokeSCMGrant asks an SCM provider to revoke the user's OAuth token via
// the provider's TokenRevoker capability (currently GitHub OAuth Apps).
func revokeSCMGrant(enc *sso.SecretEncryption, rev services.PendingRemoteRevocation, accessToken string) {
	clientSecret, err := enc.Decrypt(rev.EncryptedClientSecret)
	if err != nil {
		slog.Warn("offboarding: failed to decrypt client secret; skipping remote revoke",
			slog.String("component", "offboarding"),
			slog.String("provider", rev.ProviderType),
			slog.Any("error", err),
		)
		return
	}

	provider, err := scm.NewProvider(scm.ProviderConfig{
		ProviderType:      models.SCMProviderType(rev.ProviderType),
		AuthMethod:        models.SCMAuthMethodOAuth,
		BaseURL:           rev.BaseURL,
		OAuthClientID:     rev.OAuthClientID,
		OAuthClientSecret: clientSecret,
		OAuthAccessToken:  accessToken,
	})
	if err != nil {
		slog.Warn("offboarding: provider construction failed; skipping remote revoke",
			slog.String("component", "offboarding"),
			slog.String("provider", rev.ProviderType),
			slog.Any("error", err),
		)
		return
	}

	revoker, ok := provider.(scm.TokenRevoker)
	if !ok {
		// Provider has no revocation endpoint (e.g. Gitea).
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := revoker.RevokeToken(ctx, accessToken); err != nil {
		slog.Warn("offboarding: remote revoke failed",
			slog.String("component", "offboarding"),
			slog.String("provider", rev.ProviderType),
			slog.Any("error", err),
		)
		return
	}
	slog.Info("offboarding: remote OAuth token revoked",
		slog.String("component", "offboarding"),
		slog.String("provider", rev.ProviderType),
	)
}

// revokeIntegrationGrant revokes an integration-provider OAuth token. Only
// Todoist exposes a revocation endpoint today; Notion has none.
func revokeIntegrationGrant(enc *sso.SecretEncryption, rev services.PendingRemoteRevocation, accessToken string) {
	if rev.ProviderType != "todoist" {
		slog.Info("offboarding: provider does not support remote revocation; token expires unused",
			slog.String("component", "offboarding"),
			slog.String("provider", rev.ProviderType),
		)
		return
	}
	if rev.OAuthClientID == "" || rev.EncryptedClientSecret == "" {
		slog.Warn("offboarding: Todoist client credentials missing; skipping remote revoke",
			slog.String("component", "offboarding"),
			slog.String("provider", rev.ProviderType),
		)
		return
	}
	clientSecret, err := enc.Decrypt(rev.EncryptedClientSecret)
	if err != nil {
		slog.Warn("offboarding: failed to decrypt client secret; skipping remote revoke",
			slog.String("component", "offboarding"),
			slog.String("provider", rev.ProviderType),
			slog.Any("error", err),
		)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := todoist.RevokeToken(ctx, rev.OAuthClientID, clientSecret, accessToken); err != nil {
		slog.Warn("offboarding: Todoist token revoke failed",
			slog.String("component", "offboarding"),
			slog.String("provider", rev.ProviderType),
			slog.Any("error", err),
		)
		return
	}
	slog.Info("offboarding: remote OAuth token revoked",
		slog.String("component", "offboarding"),
		slog.String("provider", rev.ProviderType),
	)
}
