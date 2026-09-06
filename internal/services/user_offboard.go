package services

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// UserNotificationDeleter removes a user's notifications through the
// notification service/manager layer so caches are invalidated with the rows.
type UserNotificationDeleter interface {
	DeleteUserNotifications(userID int) error
}

// ErrUserOffboardingHasProtectedIntegrationLinks prevents deleting a personal
// workspace while provider-managed links still need explicit cleanup.
var ErrUserOffboardingHasProtectedIntegrationLinks = errors.New("user has provider-managed integration links in personal workspace")

// OffboardUser deactivates a user and anonymizes their PII while preserving
// audit trails. The user row is kept (anonymized) so that FK references from
// item_history, comments, time_worklogs, etc. remain valid.
//
// Returns the IDs of api_tokens revoked during the transaction so the caller
// can evict them from TokenManager's validation cache (the cache key is a
// SHA256 of the raw token, not visible to this DB-only service).
func OffboardUser(db database.Database, userID int, notificationDeleter UserNotificationDeleter, invalidators ...*AuthorizationCacheInvalidator) (revokedTokenIDs []int, err error) {
	var cacheInvalidator *AuthorizationCacheInvalidator
	if len(invalidators) > 0 {
		cacheInvalidator = invalidators[0]
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Resolve the personal workspace and protect its external links before any
	// mutation in this transaction. Offboarding must not orphan linked tickets.
	var personalWsID *int
	var wsID int
	personalWorkspaceQuery := `SELECT id FROM workspaces WHERE is_personal = true AND owner_id = ?`
	if db.GetDriverName() == "postgres" {
		personalWorkspaceQuery += " FOR UPDATE"
	}
	row := tx.QueryRow(personalWorkspaceQuery, userID)
	if err := row.Scan(&wsID); err == nil {
		personalWsID = &wsID
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("failed to find personal workspace: %w", err)
	}
	itemRepo := repository.NewItemRepository(db)
	if personalWsID != nil {
		if err := itemRepo.LockWorkspaceItemsTx(tx, *personalWsID); err != nil {
			return nil, err
		}
		hasLinks, err := NewIntegrationLinkGuards(db).HasLinksForWorkspaceTx(tx, *personalWsID)
		if err != nil {
			return nil, fmt.Errorf("failed to check personal workspace integration links: %w", err)
		}
		if hasLinks {
			return nil, ErrUserOffboardingHasProtectedIntegrationLinks
		}
	}

	// a) Anonymize user record
	if _, err := tx.Exec(`
		UPDATE users SET
			email = 'deleted-' || CAST(id AS TEXT) || '@deleted.local',
			username = 'deleted-user-' || CAST(id AS TEXT),
			first_name = 'Deleted',
			last_name = 'User',
			avatar_url = NULL,
			password_hash = NULL,
			is_active = false,
			scim_external_id = NULL,
			scim_managed = false,
			timezone = NULL,
			email_verified = false,
			email_verification_token = NULL,
			email_verification_expires = NULL,
			requires_password_reset = false,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`, userID); err != nil {
		return nil, fmt.Errorf("failed to anonymize user: %w", err)
	}

	// b) Delete personal workspace (items cascade via FK)
	if personalWsID != nil {
		if err := itemRepo.DeleteByWorkspaceTx(tx, *personalWsID); err != nil {
			return nil, fmt.Errorf("failed to delete personal workspace items: %w", err)
		}
		if _, err := tx.Exec(`DELETE FROM workspaces WHERE id = ?`, *personalWsID); err != nil {
			return nil, fmt.Errorf("failed to delete personal workspace: %w", err)
		}
	}

	// c) Unassign from all items
	if err := itemRepo.ClearAssigneeForUserTx(tx, userID); err != nil {
		return nil, fmt.Errorf("failed to unassign items: %w", err)
	}

	// d) Invalidate sessions, credentials, and API tokens. Token IDs are
	//    collected before the delete so the caller can evict the validation
	//    cache (TokenManager keys cache entries by SHA256 of the raw token,
	//    so we cannot reconstruct keys from the user_id alone).
	tokenRows, err := tx.Query(`SELECT id FROM api_tokens WHERE user_id = ?`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to load api_tokens: %w", err)
	}
	for tokenRows.Next() {
		var id int
		if scanErr := tokenRows.Scan(&id); scanErr != nil {
			_ = tokenRows.Close()
			return nil, fmt.Errorf("failed to scan api_token: %w", scanErr)
		}
		revokedTokenIDs = append(revokedTokenIDs, id)
	}
	if err := tokenRows.Err(); err != nil {
		_ = tokenRows.Close()
		return nil, fmt.Errorf("failed to iterate api_tokens: %w", err)
	}
	_ = tokenRows.Close()

	for _, stmt := range []struct {
		query string
		desc  string
	}{
		{`DELETE FROM user_sessions WHERE user_id = ?`, "sessions"},
		{`DELETE FROM user_credentials WHERE user_id = ?`, "credentials"},
		{`DELETE FROM api_tokens WHERE user_id = ?`, "api tokens"},
	} {
		if _, err := tx.Exec(stmt.query, userID); err != nil {
			return nil, fmt.Errorf("failed to delete %s: %w", stmt.desc, err)
		}
	}

	// e) Remove group memberships
	if _, err := tx.Exec(`DELETE FROM group_members WHERE user_id = ?`, userID); err != nil {
		return nil, fmt.Errorf("failed to remove group memberships: %w", err)
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
		return nil, fmt.Errorf("failed to capture implicit access effect: %w", err)
	}

	// f) Remove workspace role assignments
	if _, err := tx.Exec(`DELETE FROM user_workspace_roles WHERE user_id = ?`, userID); err != nil {
		return nil, fmt.Errorf("failed to remove workspace roles: %w", err)
	}

	// g) Remove global permissions
	if _, err := tx.Exec(`DELETE FROM user_global_permissions WHERE user_id = ?`, userID); err != nil {
		return nil, fmt.Errorf("failed to remove global permissions: %w", err)
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
			return nil, fmt.Errorf("failed to delete %s: %w", stmt.desc, err)
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
			return nil, fmt.Errorf("failed to delete %s: %w", stmt.desc, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit offboarding transaction: %w", err)
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
		return revokedTokenIDs, fmt.Errorf("invalidate authorization caches after offboarding: %w", err)
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

	return revokedTokenIDs, nil
}
