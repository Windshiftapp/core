package services

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// UserNotificationDeleter removes a user's notifications through the
// notification service/manager layer so caches are invalidated with the rows.
type UserNotificationDeleter interface {
	DeleteUserNotifications(userID int) error
}

// PendingRemoteRevocation carries the encrypted material needed to revoke one
// OAuth grant at its provider after the offboarding transaction has committed.
// The executor (server wiring) owns the decryption key and the provider
// clients; this service only collects rows before deleting them.
type PendingRemoteRevocation struct {
	// Kind is "scm" (user_scm_oauth_tokens) or "integration" (user_integration_tokens).
	Kind string
	// ProviderType is the provider slug, e.g. "github", "todoist", "notion".
	ProviderType string
	// EncryptedAccessToken is the user's at-rest OAuth access token.
	EncryptedAccessToken string
	// OAuthClientID is the provider's OAuth application client id.
	OAuthClientID string
	// EncryptedClientSecret is the provider's OAuth client secret (SCM only).
	EncryptedClientSecret string
	// BaseURL is the provider's API base URL (self-hosted SCM support).
	BaseURL string
}

// Remote revocation kinds.
const (
	RevocationKindSCM         = "scm"
	RevocationKindIntegration = "integration"
)

// OffboardResult reports the post-commit side effects of an offboarding so the
// caller can evict security caches and revoke provider-side grants.
type OffboardResult struct {
	// RevokedAPITokenIDs are the api_tokens row IDs removed during the
	// transaction. TokenManager's validation cache is keyed by SHA256 of the
	// raw token, which is not visible to this DB-only service, so the caller
	// must evict these entries itself.
	RevokedAPITokenIDs []int
	// RemoteRevocations lists OAuth grants whose local rows were deleted and
	// whose provider-side grant should now be revoked best-effort.
	RemoteRevocations []PendingRemoteRevocation
}

// OffboardUser deactivates a user, anonymizes their PII, and removes every
// user-owned credential, secret, membership, and private draft while preserving
// audit trails. The user row is kept (anonymized) so that FK references from
// item_history, comments, time_worklogs, etc. remain valid. Offboarding is
// irreversible: the row gets offboarded_at set and no code path clears it.
func OffboardUser(db database.Database, userID int, notificationDeleter UserNotificationDeleter, invalidators ...*AuthorizationCacheInvalidator) (OffboardResult, error) {
	var result OffboardResult
	var cacheInvalidator *AuthorizationCacheInvalidator
	if len(invalidators) > 0 {
		cacheInvalidator = invalidators[0]
	}
	tx, err := db.Begin()
	if err != nil {
		return result, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// a) Anonymize user record and mark the offboarding irreversibly.
	if _, err := tx.Exec(`
		UPDATE users SET
			email = 'deleted-' || CAST(id AS TEXT) || '@deleted.local',
			username = 'deleted-user-' || CAST(id AS TEXT),
			first_name = 'Deleted',
			last_name = 'User',
			avatar_url = NULL,
			password_hash = NULL,
			is_active = false,
			offboarded_at = CURRENT_TIMESTAMP,
			scim_external_id = NULL,
			scim_managed = false,
			timezone = NULL,
			email_verified = false,
			email_verification_token = NULL,
			email_verification_expires = NULL,
			requires_password_reset = false,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`, userID); err != nil {
		return result, fmt.Errorf("failed to anonymize user: %w", err)
	}

	// b) Delete personal workspace (items cascade via FK)
	var personalWsID *int
	row := tx.QueryRow(`SELECT id FROM workspaces WHERE is_personal = true AND owner_id = ?`, userID)
	var wsID int
	if err := row.Scan(&wsID); err == nil {
		personalWsID = &wsID
	} else if !errors.Is(err, sql.ErrNoRows) {
		return result, fmt.Errorf("failed to load personal workspace: %w", err)
	}
	itemRepo := repository.NewItemRepository(db)
	if personalWsID != nil {
		if err := itemRepo.DeleteByWorkspaceTx(tx, *personalWsID); err != nil {
			return result, fmt.Errorf("failed to delete personal workspace items: %w", err)
		}
		if _, err := tx.Exec(`DELETE FROM workspaces WHERE id = ?`, *personalWsID); err != nil {
			return result, fmt.Errorf("failed to delete personal workspace: %w", err)
		}
	}

	// c) Unassign from all items
	if err := itemRepo.ClearAssigneeForUserTx(tx, userID); err != nil {
		return result, fmt.Errorf("failed to unassign items: %w", err)
	}

	// d) Invalidate sessions, credentials, and API tokens. Token IDs are
	//    collected before the delete so the caller can evict the validation
	//    cache (TokenManager keys cache entries by SHA256 of the raw token,
	//    so we cannot reconstruct keys from the user_id alone).
	tokenRows, err := tx.Query(`SELECT id FROM api_tokens WHERE user_id = ?`, userID)
	if err != nil {
		return result, fmt.Errorf("failed to load api_tokens: %w", err)
	}
	for tokenRows.Next() {
		var id int
		if scanErr := tokenRows.Scan(&id); scanErr != nil {
			_ = tokenRows.Close()
			return result, fmt.Errorf("failed to scan api_token: %w", scanErr)
		}
		result.RevokedAPITokenIDs = append(result.RevokedAPITokenIDs, id)
	}
	if err := tokenRows.Err(); err != nil {
		_ = tokenRows.Close()
		return result, fmt.Errorf("failed to iterate api_tokens: %w", err)
	}
	_ = tokenRows.Close()

	for _, stmt := range []struct {
		query string
		desc  string
	}{
		{`DELETE FROM user_sessions WHERE user_id = ?`, "sessions"},
		{`DELETE FROM user_credentials WHERE user_id = ?`, "credentials"},
		{`DELETE FROM api_tokens WHERE user_id = ?`, "api tokens"},
		{`DELETE FROM user_invitations WHERE user_id = ?`, "pending invitations"},
	} {
		if _, err := tx.Exec(stmt.query, userID); err != nil {
			return result, fmt.Errorf("failed to delete %s: %w", stmt.desc, err)
		}
	}

	// d2) Collect OAuth grant material for post-commit provider-side
	//     revocation. Only OAuth connections with client credentials can be
	//     revoked remotely; the rest are just deleted locally below.
	revocationRows, err := tx.Query(`
		SELECT ut.oauth_access_token_encrypted,
		       sp.provider_type, sp.oauth_client_id, sp.oauth_client_secret_encrypted, COALESCE(sp.base_url, '')
		FROM user_scm_oauth_tokens ut
		JOIN scm_providers sp ON sp.id = ut.scm_provider_id
		WHERE ut.user_id = ?
		  AND sp.auth_method = 'oauth'
		  AND sp.oauth_client_id IS NOT NULL
		  AND sp.oauth_client_secret_encrypted IS NOT NULL
		  AND ut.oauth_access_token_encrypted IS NOT NULL
		  AND ut.oauth_access_token_encrypted != ''
	`, userID)
	if err != nil {
		return result, fmt.Errorf("failed to load SCM OAuth grants: %w", err)
	}
	for revocationRows.Next() {
		var rev PendingRemoteRevocation
		rev.Kind = RevocationKindSCM
		if scanErr := revocationRows.Scan(&rev.EncryptedAccessToken, &rev.ProviderType, &rev.OAuthClientID, &rev.EncryptedClientSecret, &rev.BaseURL); scanErr != nil {
			_ = revocationRows.Close()
			return result, fmt.Errorf("failed to scan SCM OAuth grant: %w", scanErr)
		}
		result.RemoteRevocations = append(result.RemoteRevocations, rev)
	}
	if err := revocationRows.Err(); err != nil {
		_ = revocationRows.Close()
		return result, fmt.Errorf("failed to iterate SCM OAuth grants: %w", err)
	}
	_ = revocationRows.Close()

	// user_id in the integration tables is TEXT; bind it the same way the
	// OAuth handlers do.
	userIDText := strconv.Itoa(userID)
	integrationRows, err := tx.Query(`
		SELECT ut.oauth_access_token_encrypted, ip.provider_type, COALESCE(ip.oauth_client_id, ''), COALESCE(ip.oauth_client_secret_encrypted, '')
		FROM user_integration_tokens ut
		JOIN integration_providers ip ON ip.id = ut.integration_provider_id
		WHERE ut.user_id = ?
	`, userIDText)
	if err != nil {
		return result, fmt.Errorf("failed to load integration OAuth grants: %w", err)
	}
	for integrationRows.Next() {
		var rev PendingRemoteRevocation
		rev.Kind = RevocationKindIntegration
		if scanErr := integrationRows.Scan(&rev.EncryptedAccessToken, &rev.ProviderType, &rev.OAuthClientID, &rev.EncryptedClientSecret); scanErr != nil {
			_ = integrationRows.Close()
			return result, fmt.Errorf("failed to scan integration OAuth grant: %w", scanErr)
		}
		result.RemoteRevocations = append(result.RemoteRevocations, rev)
	}
	if err := integrationRows.Err(); err != nil {
		_ = integrationRows.Close()
		return result, fmt.Errorf("failed to iterate integration OAuth grants: %w", err)
	}
	_ = integrationRows.Close()

	// d3) Delete user-owned integration secrets and sync state. The portal
	//     draft table also serves customer identities, so delete only the
	//     user-owned rows explicitly instead of relying on CASCADE.
	for _, stmt := range []struct {
		query string
		desc  string
		args  []any
	}{
		{`DELETE FROM user_integration_tokens WHERE user_id = ?`, "integration tokens", []any{userIDText}},
		{`DELETE FROM integration_oauth_state WHERE user_id = ?`, "integration OAuth state", []any{userIDText}},
		{`DELETE FROM todoist_sync_config WHERE user_id = ?`, "Todoist sync config", []any{userIDText}},
		{`DELETE FROM todoist_task_links WHERE user_id = ?`, "Todoist task links", []any{userIDText}},
		{`DELETE FROM calendar_feed_tokens WHERE user_id = ?`, "calendar feed tokens", []any{userID}},
		{`DELETE FROM portal_request_drafts WHERE user_id = ?`, "portal request drafts", []any{userID}},
		{`DELETE FROM team_members WHERE user_id = ?`, "team memberships", []any{userID}},
		{`DELETE FROM user_leave_periods WHERE user_id = ?`, "leave periods", []any{userID}},
		{`DELETE FROM on_call_schedule_layer_members WHERE user_id = ?`, "on-call layer memberships", []any{userID}},
		{`DELETE FROM on_call_schedule_overrides WHERE user_id = ? OR override_user_id = ?`, "on-call overrides", []any{userID, userID}},
		{`DELETE FROM user_asset_set_roles WHERE user_id = ?`, "asset set roles", []any{userID}},
	} {
		if _, err := tx.Exec(stmt.query, stmt.args...); err != nil {
			return result, fmt.Errorf("failed to delete %s: %w", stmt.desc, err)
		}
	}
	// Offboarding must also release substitute slots the user still holds for
	// other users' leave periods.
	if _, err := tx.Exec(`UPDATE user_leave_periods SET substitute_user_id = NULL, updated_at = CURRENT_TIMESTAMP WHERE substitute_user_id = ?`, userID); err != nil {
		return result, fmt.Errorf("failed to release leave substitutes: %w", err)
	}

	// e) Remove group memberships
	if _, err := tx.Exec(`DELETE FROM group_members WHERE user_id = ?`, userID); err != nil {
		return result, fmt.Errorf("failed to remove group memberships: %w", err)
	}

	var changesEveryone bool
	if err := tx.QueryRow(`
		SELECT EXISTS(
			SELECT 1
			FROM user_workspace_roles target
			JOIN workspace_roles wr ON wr.id = target.role_id
			WHERE target.user_id = ?
			  AND wr.builtin_key IN (?, ?, ?)
			  AND NOT EXISTS (
				SELECT 1 FROM user_workspace_roles other
				WHERE other.workspace_id = target.workspace_id
				  AND other.role_id = target.role_id
				  AND other.user_id <> target.user_id
			  )
			  AND NOT EXISTS (
				SELECT 1 FROM group_workspace_roles gwr
				WHERE gwr.workspace_id = target.workspace_id AND gwr.role_id = target.role_id
			  )
		)
	`, userID, models.RoleBuiltinViewer, models.RoleBuiltinEditor, models.RoleBuiltinTester).Scan(&changesEveryone); err != nil {
		return result, fmt.Errorf("failed to capture implicit access effect: %w", err)
	}

	// f) Remove workspace role assignments
	if _, err := tx.Exec(`DELETE FROM user_workspace_roles WHERE user_id = ?`, userID); err != nil {
		return result, fmt.Errorf("failed to remove workspace roles: %w", err)
	}

	// g) Remove global permissions
	if _, err := tx.Exec(`DELETE FROM user_global_permissions WHERE user_id = ?`, userID); err != nil {
		return result, fmt.Errorf("failed to remove global permissions: %w", err)
	}

	// h) Clean up user-specific data
	for _, stmt := range []struct {
		query string
		desc  string
	}{
		{`DELETE FROM user_preferences WHERE user_id = ?`, "preferences"},
		{`DELETE FROM personal_labels WHERE user_id = ?`, "personal labels"},
		{`DELETE FROM reviews WHERE user_id = ?`, "reviews"},
		{`DELETE FROM active_timers WHERE user_id = ?`, "active timers"},
		{`DELETE FROM item_watches WHERE user_id = ?`, "item watches"},
		{`DELETE FROM user_workspace_visits WHERE user_id = ?`, "workspace visits"},
		{`DELETE FROM user_item_activities WHERE user_id = ?`, "item activities"},
	} {
		if _, err := tx.Exec(stmt.query, userID); err != nil {
			return result, fmt.Errorf("failed to delete %s: %w", stmt.desc, err)
		}
	}

	// i) Remove SCM/SSO connections
	for _, stmt := range []struct {
		query string
		desc  string
	}{
		{`DELETE FROM user_scm_oauth_tokens WHERE user_id = ?`, "SCM tokens"},
		{`DELETE FROM user_external_accounts WHERE user_id = ?`, "SSO connections"},
		{`DELETE FROM ldap_user_mappings WHERE user_id = ?`, "LDAP mappings"},
		{`DELETE FROM webauthn_credentials WHERE user_id = ?`, "WebAuthn credentials"},
	} {
		if _, err := tx.Exec(stmt.query, userID); err != nil {
			return result, fmt.Errorf("failed to delete %s: %w", stmt.desc, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return OffboardResult{}, fmt.Errorf("failed to commit offboarding transaction: %w", err)
	}
	if personalWsID != nil {
		repository.InvalidateItemListCountCache(db, *personalWsID)
	}
	if err := cacheInvalidator.Apply(AuthorizationInvalidation{
		UserIDs:                 []int{userID},
		ResetPermissions:        changesEveryone,
		ActiveWorkspacesChanged: personalWsID != nil,
		WorkspaceKeysChanged:    personalWsID != nil,
	}); err != nil {
		return result, fmt.Errorf("invalidate authorization caches after offboarding: %w", err)
	}

	if notificationDeleter != nil {
		if err := notificationDeleter.DeleteUserNotifications(userID); err != nil {
			slog.Warn("failed to delete notifications during user offboarding",
				slog.String("component", "notifications"),
				slog.Int("user_id", userID),
				slog.Any("error", err),
			)
		}
	}

	return result, nil
}
