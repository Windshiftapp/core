package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"windshift/internal/database"
	"windshift/internal/models"
)

// SCIMRepository contains the persistence helpers used by the SCIM 2.0
// provisioning handler. Keeping the raw SQL here lets the handler stay free
// of direct database access while preserving the exact SCIM visibility
// scoping (scim_managed / is_agent guards) documented on each query.
type SCIMRepository struct {
	db database.Database
}

// NewSCIMRepository creates a new SCIM repository.
func NewSCIMRepository(db database.Database) *SCIMRepository {
	return &SCIMRepository{db: db}
}

// scimNullIfEmpty converts an empty string to nil (SQL NULL) so that partial
// unique indexes on scim_external_id (WHERE scim_external_id IS NOT NULL) are
// not violated when the field is omitted from the SCIM request.
func scimNullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// SCIMGroupMemberRow is one SCIM-managed group membership row joined with the
// user fields needed to render a SCIM member entry.
type SCIMGroupMemberRow struct {
	UserID    int
	FirstName string
	LastName  string
	Username  string
}

// =============================================================================
// Users
// =============================================================================

// ListUsersFiltered returns the SCIM-visible users matching an optional
// pre-built filter WHERE clause, plus the unpaginated total count.
//
// SCIM represents the IdP-provisioned surface. Agent users and locally
// managed humans must stay invisible here: if the IdP ever sees them in a
// GET /Users sweep it records their IDs in its shadow and then tries to
// DELETE them on the next sync tick, producing audit noise forever even
// after the write-side guard refuses every attempt.
func (r *SCIMRepository) ListUsersFiltered(whereClause string, filterArgs []any, count, offset int) ([]models.User, int, error) {
	baseQuery := `SELECT id, email, username, first_name, last_name, is_active,
	              COALESCE(scim_external_id, '') as scim_external_id, created_at, updated_at
	              FROM users WHERE is_agent = false AND scim_managed = true AND scim_deleted_at IS NULL`
	countQuery := `SELECT COUNT(*) FROM users WHERE is_agent = false AND scim_managed = true AND scim_deleted_at IS NULL`

	args := []any{}
	if whereClause != "" {
		baseQuery += " AND " + whereClause
		countQuery += " AND " + whereClause
		args = filterArgs
	}

	var totalResults int
	if err := r.db.QueryRow(countQuery, args...).Scan(&totalResults); err != nil {
		return nil, 0, fmt.Errorf("failed to count users: %w", err)
	}

	baseQuery += fmt.Sprintf(" ORDER BY id LIMIT %d OFFSET %d", count, offset)

	rows, err := r.db.Query(baseQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query users: %w", err)
	}
	defer func() { _ = rows.Close() }()

	users := make([]models.User, 0)
	for rows.Next() {
		var user models.User
		var scimExternalID string
		if err := rows.Scan(&user.ID, &user.Email, &user.Username, &user.FirstName,
			&user.LastName, &user.IsActive, &scimExternalID, &user.CreatedAt, &user.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan user row: %w", err)
		}
		user.SCIMExternalID = scimExternalID
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate users: %w", err)
	}

	return users, totalResults, nil
}

// GetUserByID loads a single user with the SCIM-relevant flags.
// Deprovisioned (tombstoned) users are excluded so every SCIM operation on
// them — GET, PUT, PATCH, DELETE — reports 404 per RFC 7644 §3.6.
func (r *SCIMRepository) GetUserByID(id int) (*models.User, error) {
	var user models.User
	var scimExternalID sql.NullString
	err := r.db.QueryRow(`
		SELECT id, email, username, first_name, last_name, is_active,
		       scim_external_id, COALESCE(scim_managed, false), COALESCE(is_agent, false),
		       created_at, updated_at
		FROM users WHERE id = ? AND scim_deleted_at IS NULL
	`, id).Scan(&user.ID, &user.Email, &user.Username, &user.FirstName, &user.LastName,
		&user.IsActive, &scimExternalID, &user.SCIMManaged, &user.IsAgent,
		&user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if scimExternalID.Valid {
		user.SCIMExternalID = scimExternalID.String
	}
	return &user, nil
}

// GetUserByEmail loads a user by email for the SCIM create/adopt flow.
func (r *SCIMRepository) GetUserByEmail(email string) (*models.User, error) {
	var user models.User
	err := r.db.QueryRow(`
		SELECT id, email, username, first_name, last_name, is_active,
		       COALESCE(scim_managed, false), created_at, updated_at
		FROM users WHERE email = ?
	`, email).Scan(&user.ID, &user.Email, &user.Username,
		&user.FirstName, &user.LastName, &user.IsActive,
		&user.SCIMManaged, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// UsernameExists reports whether any user row already holds the username.
// It deliberately mirrors the handler's historical `err == nil` semantics:
// lookup errors (including not-found) read as "no collision".
func (r *SCIMRepository) UsernameExists(username string) bool {
	var collidingID int
	err := r.db.QueryRow(`SELECT id FROM users WHERE username = ?`, username).Scan(&collidingID)
	return err == nil
}

// AdoptUser links an existing local user to SCIM management.
func (r *SCIMRepository) AdoptUser(id int, username, externalID string, isActive bool) error {
	_, err := r.db.ExecWrite(`
		UPDATE users SET username = ?, scim_managed = true, scim_external_id = ?,
		                 is_active = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, username, scimNullIfEmpty(externalID), isActive, id)
	return err
}

// CreateUser inserts a new SCIM-managed user and returns its ID.
func (r *SCIMRepository) CreateUser(email, username, firstName, lastName string, isActive bool, externalID string) (int, error) {
	var userID int64
	err := r.db.QueryRow(`
		INSERT INTO users (email, username, first_name, last_name, is_active,
		                   scim_external_id, scim_managed, email_verified)
		VALUES (?, ?, ?, ?, ?, ?, true, true) RETURNING id
	`, email, username, firstName, lastName, isActive, scimNullIfEmpty(externalID)).Scan(&userID)
	if err != nil {
		return 0, err
	}
	return int(userID), nil
}

// ReplaceUser fully replaces the SCIM-managed attributes of a user.
func (r *SCIMRepository) ReplaceUser(id int, email, username, firstName, lastName string, isActive bool, externalID string) (DeprovisionCascade, error) {
	var cascade DeprovisionCascade
	err := database.WithTx(r.db, func(tx database.Tx) error {
		var wasActive bool
		if err := tx.QueryRow(`SELECT COALESCE(is_active, false) FROM users WHERE id = ?`, id).Scan(&wasActive); err != nil {
			return fmt.Errorf("failed to load user state: %w", err)
		}
		if _, err := tx.Exec(`
			UPDATE users SET email = ?, username = ?, first_name = ?, last_name = ?,
			                 is_active = ?, scim_external_id = ?, scim_managed = true,
			                 updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, email, username, firstName, lastName, isActive, scimNullIfEmpty(externalID), id); err != nil {
			return fmt.Errorf("failed to replace user: %w", err)
		}
		if wasActive && !isActive {
			var err error
			cascade, err = DeactivateOwnedAgentsAndTokensTx(tx, id)
			return err
		}
		return nil
	})
	return cascade, err
}

// SCIMUserPatch carries the final column writes for one PATCH request. Fields
// are nil when the request leaves the attribute unchanged; the handler
// resolves multiple operations down to one value per column before the
// transaction runs, so a partial application is impossible.
type SCIMUserPatch struct {
	SetActive       *bool
	SetUsername     *string
	SetFirstName    *string
	SetLastName     *string
	SetExternalID   *string
	ClearExternalID bool
}

// ApplyUserPatch applies the resolved patch columns in one transaction. When
// the patch deactivates the user, the owned-agent and API-token cascade
// commits with it, so a deactivation can never succeed with live credentials
// left behind. Returns the cascade executed, if any.
func (r *SCIMRepository) ApplyUserPatch(id int, patch SCIMUserPatch) (DeprovisionCascade, error) {
	var cascade DeprovisionCascade
	err := database.WithTx(r.db, func(tx database.Tx) error {
		var wasActive bool
		if err := tx.QueryRow(`SELECT COALESCE(is_active, false) FROM users WHERE id = ?`, id).Scan(&wasActive); err != nil {
			return fmt.Errorf("failed to load user state: %w", err)
		}

		sets := []string{"updated_at = CURRENT_TIMESTAMP"}
		args := []any{}
		addColumn := func(column string, value any) {
			sets = append(sets, column+" = ?")
			args = append(args, value)
		}
		if patch.SetActive != nil {
			addColumn("is_active", *patch.SetActive)
		}
		if patch.SetUsername != nil {
			addColumn("username", *patch.SetUsername)
		}
		if patch.SetFirstName != nil {
			addColumn("first_name", *patch.SetFirstName)
		}
		if patch.SetLastName != nil {
			addColumn("last_name", *patch.SetLastName)
		}
		if patch.SetExternalID != nil {
			addColumn("scim_external_id", scimNullIfEmpty(*patch.SetExternalID))
		}
		if patch.ClearExternalID {
			addColumn("scim_external_id", nil)
		}

		args = append(args, id)
		if _, err := tx.Exec(`UPDATE users SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...); err != nil {
			return fmt.Errorf("failed to apply user patch: %w", err)
		}

		if wasActive && patch.SetActive != nil && !*patch.SetActive {
			var err error
			cascade, err = DeactivateOwnedAgentsAndTokensTx(tx, id)
			return err
		}
		return nil
	})
	return cascade, err
}

// TombstoneUser applies an RFC 7644 §3.6 SCIM DELETE: the user is deactivated
// and marked deprovisioned so every SCIM query hides the resource, while the
// row itself is retained for historical references. The full security cascade
// commits in the same transaction — SCIM-managed group memberships are
// removed, owned agents are deactivated, and their API tokens are revoked —
// so a deprovisioned account can never leave active agents or live tokens
// behind. Idempotent: repeated calls make no further changes.
func (r *SCIMRepository) TombstoneUser(id int) (DeprovisionCascade, error) {
	var cascade DeprovisionCascade
	err := database.WithTx(r.db, func(tx database.Tx) error {
		res, err := tx.Exec(`
			UPDATE users
			SET is_active = false, scim_deleted_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
			WHERE id = ? AND scim_deleted_at IS NULL
		`, id)
		if err != nil {
			return fmt.Errorf("failed to tombstone user: %w", err)
		}
		if affected, err := res.RowsAffected(); err == nil && affected == 0 {
			// Already tombstoned — nothing left to do.
			return nil
		}
		if _, err := tx.Exec(`
			DELETE FROM group_members WHERE user_id = ? AND scim_managed = true
		`, id); err != nil {
			return fmt.Errorf("failed to remove SCIM group memberships: %w", err)
		}
		cascade, err = DeactivateOwnedAgentsAndTokensTx(tx, id)
		return err
	})
	return cascade, err
}

// IsUserSCIMDeleted reports whether the user row carries the SCIM
// deprovisioning tombstone. Used by the create/adopt flow so a re-provisioning
// request for a deleted account conflicts instead of resurrecting it.
func (r *SCIMRepository) IsUserSCIMDeleted(id int) bool {
	var deleted bool
	err := r.db.QueryRow(
		`SELECT scim_deleted_at IS NOT NULL FROM users WHERE id = ?`, id,
	).Scan(&deleted)
	return err == nil && deleted
}

// IsUserSCIMVisible reports whether a user ID can be referenced as a SCIM
// group member: must exist, be SCIM-managed, and not be an agent. Mirrors
// the ListUsersFiltered scope so SCIM only ever wires up users it can also
// see via GET /Users. Without this check, a SCIM client could attach
// arbitrary local/admin/service users to SCIM-managed groups by guessing IDs.
func (r *SCIMRepository) IsUserSCIMVisible(userID int) bool {
	var ok bool
	err := r.db.QueryRow(`
		SELECT COALESCE(scim_managed, false) = true AND COALESCE(is_agent, false) = false
		FROM users WHERE id = ? AND scim_deleted_at IS NULL
	`, userID).Scan(&ok)
	return err == nil && ok
}

// =============================================================================
// Groups
// =============================================================================

// ListGroupsFiltered returns the SCIM-visible groups matching an optional
// pre-built filter WHERE clause, plus the unpaginated total count.
//
// Mirrors ListUsersFiltered: SCIM only sees what the IdP provisioned.
// Returning locally-managed groups here would let a SCIM client enumerate
// them, then take them over via PUT/PATCH or destroy them via DELETE.
func (r *SCIMRepository) ListGroupsFiltered(whereClause string, filterArgs []any, count, offset int) ([]models.TeamGroup, int, error) {
	baseQuery := `SELECT id, name, description, COALESCE(scim_external_id, '') as scim_external_id,
	              created_at, updated_at FROM groups WHERE scim_managed = true`
	countQuery := `SELECT COUNT(*) FROM groups WHERE scim_managed = true`

	args := []any{}
	if whereClause != "" {
		baseQuery += " AND " + whereClause
		countQuery += " AND " + whereClause
		args = filterArgs
	}

	var totalResults int
	if err := r.db.QueryRow(countQuery, args...).Scan(&totalResults); err != nil {
		return nil, 0, fmt.Errorf("failed to count groups: %w", err)
	}

	baseQuery += fmt.Sprintf(" ORDER BY id LIMIT %d OFFSET %d", count, offset)

	rows, err := r.db.Query(baseQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query groups: %w", err)
	}
	defer func() { _ = rows.Close() }()

	groups := make([]models.TeamGroup, 0)
	for rows.Next() {
		var group models.TeamGroup
		var scimExternalID string
		if err := rows.Scan(&group.ID, &group.Name, &group.Description, &scimExternalID,
			&group.CreatedAt, &group.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan group row: %w", err)
		}
		group.SCIMExternalID = scimExternalID
		groups = append(groups, group)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate groups: %w", err)
	}

	return groups, totalResults, nil
}

// GetGroupByID loads a single group with the SCIM-relevant flags.
func (r *SCIMRepository) GetGroupByID(id int) (*models.TeamGroup, error) {
	var group models.TeamGroup
	var scimExternalID sql.NullString
	err := r.db.QueryRow(`
		SELECT id, name, description, scim_external_id, COALESCE(scim_managed, false),
		       created_at, updated_at
		FROM groups WHERE id = ?
	`, id).Scan(&group.ID, &group.Name, &group.Description, &scimExternalID,
		&group.SCIMManaged, &group.CreatedAt, &group.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if scimExternalID.Valid {
		group.SCIMExternalID = scimExternalID.String
	}
	return &group, nil
}

// GroupNameExists reports whether a group with the given name already exists.
// Mirrors the handler's historical `err == nil` semantics (see UsernameExists).
func (r *SCIMRepository) GroupNameExists(name string) bool {
	var existingID int
	err := r.db.QueryRow(`SELECT id FROM groups WHERE name = ?`, name).Scan(&existingID)
	return err == nil
}

// CreateGroup inserts a new SCIM-managed group and returns its ID.
func (r *SCIMRepository) CreateGroup(name, externalID string) (int, error) {
	var groupID int64
	err := r.db.QueryRow(`
		INSERT INTO groups (name, description, scim_external_id, scim_managed, is_active)
		VALUES (?, '', ?, true, true) RETURNING id
	`, name, scimNullIfEmpty(externalID)).Scan(&groupID)
	if err != nil {
		return 0, err
	}
	return int(groupID), nil
}

// GetGroupMembers returns only SCIM-managed memberships. A locally-added
// member of an otherwise SCIM-managed group must stay invisible to the IdP;
// otherwise it'll record the ID in its shadow and try to remove it on the
// next sync.
func (r *SCIMRepository) GetGroupMembers(groupID int) ([]SCIMGroupMemberRow, error) {
	rows, err := r.db.Query(`
		SELECT u.id, u.first_name, u.last_name, u.username
		FROM group_members gm
		JOIN users u ON gm.user_id = u.id
		WHERE gm.group_id = ? AND gm.scim_managed = true AND u.scim_deleted_at IS NULL
	`, groupID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var members []SCIMGroupMemberRow
	for rows.Next() {
		var m SCIMGroupMemberRow
		if err := rows.Scan(&m.UserID, &m.FirstName, &m.LastName, &m.Username); err != nil {
			return nil, fmt.Errorf("scan group member row: %w", err)
		}
		members = append(members, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate group members: %w", err)
	}
	return members, nil
}

// AddGroupMember inserts a SCIM-managed membership (no upsert; a duplicate
// surfaces as a constraint error the caller audits, matching CreateGroup's
// historical behavior).
func (r *SCIMRepository) AddGroupMember(groupID, userID int) error {
	_, err := r.db.ExecWrite(`
		INSERT INTO group_members (group_id, user_id, scim_managed, added_at)
		VALUES (?, ?, true, CURRENT_TIMESTAMP)
	`, groupID, userID)
	return err
}

// UpsertGroupMember inserts a SCIM-managed membership, flipping an existing
// local membership into scim_managed state on conflict (PATCH add semantics).
func (r *SCIMRepository) UpsertGroupMember(groupID, userID int) error {
	_, err := r.db.ExecWrite(`
		INSERT INTO group_members (group_id, user_id, scim_managed, added_at)
		VALUES (?, ?, true, CURRENT_TIMESTAMP)
		ON CONFLICT(group_id, user_id) DO UPDATE SET scim_managed = true
	`, groupID, userID)
	return err
}

// RemoveGroupMember deletes a membership, scoped to SCIM-managed rows so a
// SCIM PATCH can't wipe a locally-added row. Matches the bulk DELETE inside
// ReplaceGroup.
func (r *SCIMRepository) RemoveGroupMember(groupID, userID int) error {
	_, err := r.db.ExecWrite(`DELETE FROM group_members WHERE group_id = ? AND user_id = ? AND scim_managed = true`, groupID, userID)
	return err
}

// UpdateGroupName applies a SCIM PATCH to displayName.
func (r *SCIMRepository) UpdateGroupName(id int, name string) error {
	_, err := r.db.ExecWrite(`UPDATE groups SET name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, name, id)
	return err
}

// UpdateGroupExternalID applies a SCIM PATCH to externalId.
func (r *SCIMRepository) UpdateGroupExternalID(id int, externalID string) error {
	_, err := r.db.ExecWrite(`UPDATE groups SET scim_external_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, externalID, id)
	return err
}

// DeleteGroup deletes a group row.
func (r *SCIMRepository) DeleteGroup(id int) error {
	_, err := r.db.ExecWrite(`DELETE FROM groups WHERE id = ?`, id)
	return err
}

// ReplaceGroup wraps the rename + member-set rewrite in a single transaction
// so a failure mid-flight cannot leave the group renamed-but-empty (or with
// only some of the new members applied). It returns the SCIM-managed member
// IDs that were present before the rewrite so the caller can emit a remove
// audit entry per departing user; the snapshot is taken inside the tx so it
// reflects the same state the DELETE then removes. On error the transaction
// rolls back and the caller observes no externally visible change.
func (r *SCIMRepository) ReplaceGroup(ctx context.Context, id int, name, externalID string, memberIDs []int) ([]int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err = tx.Exec(`
		UPDATE groups SET name = ?, scim_external_id = ?, scim_managed = true,
		                  updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, name, scimNullIfEmpty(externalID), id); err != nil {
		return nil, err
	}

	var priorMemberIDs []int
	priorRows, selErr := tx.Query(`SELECT user_id FROM group_members WHERE group_id = ? AND scim_managed = true`, id)
	if selErr != nil {
		return nil, selErr
	}
	for priorRows.Next() {
		var uid int
		if scanErr := priorRows.Scan(&uid); scanErr != nil {
			_ = priorRows.Close()
			return nil, scanErr
		}
		priorMemberIDs = append(priorMemberIDs, uid)
	}
	if iterErr := priorRows.Err(); iterErr != nil {
		_ = priorRows.Close()
		return nil, iterErr
	}
	_ = priorRows.Close()

	if _, err = tx.Exec(`DELETE FROM group_members WHERE group_id = ? AND scim_managed = true`, id); err != nil {
		return nil, err
	}

	for _, memberID := range memberIDs {
		if _, err = tx.Exec(`
			INSERT INTO group_members (group_id, user_id, scim_managed, added_at)
			VALUES (?, ?, true, CURRENT_TIMESTAMP)
			ON CONFLICT(group_id, user_id) DO UPDATE SET scim_managed = true
		`, id, memberID); err != nil {
			return nil, err
		}
	}

	if commitErr := tx.Commit(); commitErr != nil {
		return nil, commitErr
	}

	return priorMemberIDs, nil
}
