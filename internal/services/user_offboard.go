package services

import (
	"fmt"

	"windshift/internal/database"
)

// OffboardUser deactivates a user and anonymizes their PII while preserving
// audit trails. The user row is kept (anonymized) so that FK references from
// item_history, comments, time_worklogs, etc. remain valid.
func OffboardUser(db database.Database, userID int) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

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
		return fmt.Errorf("failed to anonymize user: %w", err)
	}

	// b) Delete personal workspace (items cascade via FK)
	var personalWsID *int
	row := tx.QueryRow(`SELECT id FROM workspaces WHERE is_personal = true AND owner_id = ?`, userID)
	var wsID int
	if err := row.Scan(&wsID); err == nil {
		personalWsID = &wsID
	}
	if personalWsID != nil {
		if _, err := tx.Exec(`DELETE FROM items WHERE workspace_id = ?`, *personalWsID); err != nil {
			return fmt.Errorf("failed to delete personal workspace items: %w", err)
		}
		if _, err := tx.Exec(`DELETE FROM workspaces WHERE id = ?`, *personalWsID); err != nil {
			return fmt.Errorf("failed to delete personal workspace: %w", err)
		}
	}

	// c) Unassign from all items
	if _, err := tx.Exec(`UPDATE items SET assignee_id = NULL WHERE assignee_id = ?`, userID); err != nil {
		return fmt.Errorf("failed to unassign items: %w", err)
	}

	// d) Invalidate sessions and tokens
	for _, stmt := range []struct {
		query string
		desc  string
	}{
		{`DELETE FROM user_sessions WHERE user_id = ?`, "sessions"},
		{`DELETE FROM user_app_tokens WHERE user_id = ?`, "app tokens"},
		{`DELETE FROM user_credentials WHERE user_id = ?`, "credentials"},
	} {
		if _, err := tx.Exec(stmt.query, userID); err != nil {
			return fmt.Errorf("failed to delete %s: %w", stmt.desc, err)
		}
	}

	// e) Remove group memberships
	if _, err := tx.Exec(`DELETE FROM group_members WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("failed to remove group memberships: %w", err)
	}

	// f) Remove workspace role assignments
	if _, err := tx.Exec(`DELETE FROM user_workspace_roles WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("failed to remove workspace roles: %w", err)
	}

	// g) Remove global permissions
	if _, err := tx.Exec(`DELETE FROM user_global_permissions WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("failed to remove global permissions: %w", err)
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
		{`DELETE FROM notifications WHERE user_id = ?`, "notifications"},
	} {
		if _, err := tx.Exec(stmt.query, userID); err != nil {
			return fmt.Errorf("failed to delete %s: %w", stmt.desc, err)
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
			return fmt.Errorf("failed to delete %s: %w", stmt.desc, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit offboarding transaction: %w", err)
	}

	return nil
}
