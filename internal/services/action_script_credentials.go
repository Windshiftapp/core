package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"windshift/internal/auth"
	"windshift/internal/models"
)

// ActionScriptTokenTTL bounds the lifetime of the temporary bearer token minted
// for one execute_script node execution. It only needs to outlive the node's
// runtime budget (a few seconds), so a short window keeps a leaked token's
// value near zero even though the token is revoked in cleanup.
const ActionScriptTokenTTL = 60 * time.Second

// ActionScriptScopes is the explicit, server-owned scope allowlist for tokens
// minted on behalf of a script executor. It deliberately omits every delete,
// admin, action-authoring, asset, test, agent-run, and MCP scope: scripts run
// with the executor's permissions but cannot delegate, author further
// automations, or reach surfaces that would let them widen their own reach.
// Scopes constrain the token, they never grant permissions the executor lacks.
var ActionScriptScopes = []string{
	auth.ScopeItemsRead, auth.ScopeItemsWrite,
	auth.ScopeLinksRead, auth.ScopeLinksWrite,
	auth.ScopeWorkspacesRead,
	auth.ScopeUsersRead,
	auth.ScopeStatusesRead,
	auth.ScopeWorkflowsRead,
	auth.ScopeItemTypesRead,
	auth.ScopePrioritiesRead,
	auth.ScopeCustomFieldsRead,
	auth.ScopeMilestonesRead, auth.ScopeMilestonesWrite,
	auth.ScopeIterationsRead, auth.ScopeIterationsWrite,
	auth.ScopePagesRead, auth.ScopePagesWrite,
	auth.ScopeTimeRead, auth.ScopeTimeWrite,
	auth.ScopeApprovalsRead,
}

// ActionScriptTokenManager is the narrow slice of *auth.TokenManager the
// execute_script executor needs. Keeping it an interface lets tests inject a
// fake that fails closed without standing up the token cache and database.
type ActionScriptTokenManager interface {
	CreateToken(userID int, request models.APITokenCreate) (*models.APITokenResponse, error)
	RevokeToken(tokenID, userID int) error
}

// ActionScriptCredential is one minted temporary credential. The raw Token
// value must never be written to logs, execution traces, node output, or
// persisted definitions; it lives only in the bridge closure for the duration
// of a single node execution.
type ActionScriptCredential struct {
	Token     string
	TokenID   int
	ExpiresAt time.Time
}

// MintActionScriptCredential issues a short-lived temporary token for the
// effective executor with the fixed action scope allowlist. It fails closed on
// an unidentified or inactive user (CreateToken rejects inactive users) so a
// script can never run as creator/admin/system.
func MintActionScriptCredential(_ context.Context, tm ActionScriptTokenManager, actorUserID int) (*ActionScriptCredential, error) {
	if tm == nil {
		return nil, errors.New("action script: token manager is required")
	}
	if actorUserID <= 0 {
		return nil, errors.New("action script: no effective executor identity")
	}
	if err := auth.ValidateScopes(ActionScriptScopes); err != nil {
		return nil, fmt.Errorf("action script: invalid scope allowlist: %w", err)
	}
	expiresAt := time.Now().UTC().Add(ActionScriptTokenTTL)
	resp, err := tm.CreateToken(actorUserID, models.APITokenCreate{
		Name:        fmt.Sprintf("action-script:%d", actorUserID),
		Permissions: ActionScriptScopes,
		ExpiresAt:   &expiresAt,
		IsTemporary: true,
	})
	if err != nil {
		return nil, fmt.Errorf("action script: mint executor credential: %w", err)
	}
	return &ActionScriptCredential{
		Token:     resp.Token,
		TokenID:   resp.APIToken.ID,
		ExpiresAt: expiresAt,
	}, nil
}

// RevokeActionScriptCredential removes a minted credential during node cleanup.
// Revocation failure is returned so the executor can log it, but the token's
// short expiry is the crash fallback: even a failed revoke cannot outlive
// ActionScriptTokenTTL.
func RevokeActionScriptCredential(tm ActionScriptTokenManager, cred *ActionScriptCredential, actorUserID int) error {
	if tm == nil || cred == nil {
		return nil
	}
	if err := tm.RevokeToken(cred.TokenID, actorUserID); err != nil {
		return fmt.Errorf("action script: revoke executor credential: %w", err)
	}
	return nil
}
