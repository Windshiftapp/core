package integrations

import (
	"context"

	"windshift/internal/models"
)

// SystemOAuthCallbackResult is the provider-neutral audit context returned by
// a system-owned OAuth flow.
type SystemOAuthCallbackResult struct {
	ProviderID   string
	ProviderName string
	Initiator    *models.User
	Generation   int64
}

// SystemOAuthFlow extends the shared OAuth handler for administrator-managed
// connections. Provider-specific token exchange and concurrency guarantees
// remain behind this interface.
type SystemOAuthFlow interface {
	StartOAuth(ctx context.Context, providerID string, actorID int, publicBaseURL string) (string, error)
	CompleteOAuth(ctx context.Context, state, code, publicBaseURL string) (*SystemOAuthCallbackResult, error)
	ConsumeFailedOAuthCallback(state string) (*SystemOAuthCallbackResult, error)
}
