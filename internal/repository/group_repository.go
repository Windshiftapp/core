package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/utils"
)

// GroupRepository owns persistence for the team-group surface (groups,
// group_members and the user lookups the membership flows need).
type GroupRepository struct {
	db database.Database
}

// NewGroupRepository creates a group repository.
func NewGroupRepository(db database.Database) *GroupRepository {
	return &GroupRepository{db: db}
}

// groupNullableFields holds the nullable scan targets shared by every group-row query.
type groupNullableFields struct {
	ldapDN        sql.NullString
	ldapCN        sql.NullString
	ldapLastSync  sql.NullTime
	createdBy     sql.NullInt64
	createdByName sql.NullString
}

// applyTo populates the corresponding fields on a TeamGroup from nullable scan values.
func (n *groupNullableFields) applyTo(g *models.TeamGroup) {
	g.LDAPDistinguishedName = n.ldapDN.String
	g.LDAPCommonName = n.ldapCN.String
	g.LDAPLastSyncAt = utils.NullTimeToPtr(n.ldapLastSync)
	g.CreatedBy = utils.NullInt64ToPtr(n.createdBy)
	g.CreatedByName = n.createdByName.String
}

// scanGroupMember scans a single group member row from *sql.Rows and returns the populated model.
func scanGroupMember(rows *sql.Rows) (models.TeamGroupMember, error) {
	var member models.TeamGroupMember
	var ldapLastSyncMember sql.NullTime
	var addedBy sql.NullInt64
	var addedByName sql.NullString

	err := rows.Scan(
		&member.ID, &member.GroupID, &member.UserID, &member.LDAPSyncEnabled, &ldapLastSyncMember,
		&addedBy, &member.AddedAt, &member.CreatedAt, &member.UpdatedAt,
		&member.UserEmail, &member.UserName, &member.UserUsername, &addedByName,
	)
	if err != nil {
		return member, err
	}

	member.LDAPLastSyncAt = utils.NullTimeToPtr(ldapLastSyncMember)
	member.AddedBy = utils.NullInt64ToPtr(addedBy)
	member.AddedByName = addedByName.String

	return member, nil
}

// ListAll returns every group with its member count, ordered by name.
func (r *GroupRepository) ListAll() ([]models.TeamGroup, error) {
	return r.list("")
}

// ListPage returns a bounded group page with its member counts.
func (r *GroupRepository) ListPage(limit, offset int) ([]models.TeamGroup, error) {
	return r.list(" LIMIT ? OFFSET ?", limit, offset)
}

func (r *GroupRepository) list(suffix string, args ...any) ([]models.TeamGroup, error) {
	rows, err := r.db.Query(`
		SELECT
			g.id, g.name, g.description, g.ldap_distinguished_name, g.ldap_common_name,
			g.ldap_sync_enabled, g.ldap_last_sync_at, g.is_system_group, g.is_active,
			g.created_by, g.created_at, g.updated_at,
			u.first_name || ' ' || u.last_name as created_by_name,
			(SELECT COUNT(*) FROM group_members gm WHERE gm.group_id = g.id) as member_count
		FROM groups g
		LEFT JOIN users u ON g.created_by = u.id
		ORDER BY g.name`+suffix, args...)
	if err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var groups []models.TeamGroup
	for rows.Next() {
		var group models.TeamGroup
		var nf groupNullableFields

		err := rows.Scan(
			&group.ID, &group.Name, &group.Description, &nf.ldapDN, &nf.ldapCN,
			&group.LDAPSyncEnabled, &nf.ldapLastSync, &group.IsSystemGroup, &group.IsActive,
			&nf.createdBy, &group.CreatedAt, &group.UpdatedAt,
			&nf.createdByName, &group.MemberCount,
		)
		if err != nil {
			return nil, fmt.Errorf("scan group: %w", err)
		}

		nf.applyTo(&group)
		groups = append(groups, group)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate groups: %w", err)
	}
	return groups, nil
}

// Count returns the total number of groups.
func (r *GroupRepository) Count() (int, error) {
	var total int
	if err := r.db.QueryRow("SELECT COUNT(*) FROM groups").Scan(&total); err != nil {
		return 0, fmt.Errorf("count groups: %w", err)
	}
	return total, nil
}

// GetByID returns a single group (without members). It returns ErrNotFound
// when the group does not exist.
func (r *GroupRepository) GetByID(id int) (*models.TeamGroup, error) {
	var group models.TeamGroup
	var nf groupNullableFields

	err := r.db.QueryRow(`
		SELECT
			g.id, g.name, g.description, g.ldap_distinguished_name, g.ldap_common_name,
			g.ldap_sync_enabled, g.ldap_last_sync_at, g.is_system_group, g.is_active,
			g.created_by, g.created_at, g.updated_at,
			u.first_name || ' ' || u.last_name as created_by_name
		FROM groups g
		LEFT JOIN users u ON g.created_by = u.id
		WHERE g.id = ?
	`, id).Scan(
		&group.ID, &group.Name, &group.Description, &nf.ldapDN, &nf.ldapCN,
		&group.LDAPSyncEnabled, &nf.ldapLastSync, &group.IsSystemGroup, &group.IsActive,
		&nf.createdBy, &group.CreatedAt, &group.UpdatedAt, &nf.createdByName,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get group: %w", err)
	}

	nf.applyTo(&group)
	return &group, nil
}

// ListMembers returns all members of a group with joined user display fields.
func (r *GroupRepository) ListMembers(groupID int) ([]models.TeamGroupMember, error) {
	rows, err := r.db.Query(`
		SELECT
			gm.id, gm.group_id, gm.user_id, gm.ldap_sync_enabled, gm.ldap_last_sync_at,
			gm.added_by, gm.added_at, gm.created_at, gm.updated_at,
			u.email, u.first_name || ' ' || u.last_name as user_name, u.username,
			adder.first_name || ' ' || adder.last_name as added_by_name
		FROM group_members gm
		JOIN users u ON gm.user_id = u.id
		LEFT JOIN users adder ON gm.added_by = adder.id
		WHERE gm.group_id = ?
		ORDER BY u.last_name, u.first_name
	`, groupID)
	if err != nil {
		return nil, fmt.Errorf("list group members: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var members []models.TeamGroupMember
	for rows.Next() {
		member, err := scanGroupMember(rows)
		if err != nil {
			return nil, fmt.Errorf("scan group member: %w", err)
		}
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate group members: %w", err)
	}
	return members, nil
}

// NameExists reports whether a group with the given name exists. When
// excludeID is non-zero that group is ignored (update uniqueness checks).
func (r *GroupRepository) NameExists(name string, excludeID int) (bool, error) {
	var exists bool
	var err error
	if excludeID != 0 {
		err = r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM groups WHERE name = ? AND id != ?)", name, excludeID).Scan(&exists)
	} else {
		err = r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM groups WHERE name = ?)", name).Scan(&exists)
	}
	if err != nil {
		return false, fmt.Errorf("check group name: %w", err)
	}
	return exists, nil
}

// Create inserts an active group and returns its ID. It returns
// ErrDuplicateEntry when the name violates the unique constraint.
func (r *GroupRepository) Create(name, description string, createdBy *int, now time.Time) (int64, error) {
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO groups (name, description, is_active, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?) RETURNING id
	`, name, description, true, createdBy, now, now).Scan(&id)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return 0, ErrDuplicateEntry
		}
		return 0, fmt.Errorf("create group: %w", err)
	}
	return id, nil
}

// GetUpdateSnapshot returns the fields needed to validate and audit a group
// update. It returns ErrNotFound when the group does not exist.
func (r *GroupRepository) GetUpdateSnapshot(id int) (*models.TeamGroup, error) {
	var group models.TeamGroup
	err := r.db.QueryRow(`
		SELECT id, name, description, is_active, COALESCE(scim_managed, false)
		FROM groups
		WHERE id = ?
	`, id).Scan(&group.ID, &group.Name, &group.Description, &group.IsActive, &group.SCIMManaged)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get group update snapshot: %w", err)
	}
	return &group, nil
}

// Update overwrites the mutable group fields. It returns ErrDuplicateEntry
// when the new name violates the unique constraint.
func (r *GroupRepository) Update(id int, name, description string, isActive bool, now time.Time) error {
	_, err := r.db.ExecWrite(`
		UPDATE groups
		SET name = ?, description = ?, is_active = ?, updated_at = ?
		WHERE id = ?
	`, name, description, isActive, now, id)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return ErrDuplicateEntry
		}
		return fmt.Errorf("update group: %w", err)
	}
	return nil
}

// GroupDeleteSnapshot carries the fields needed to authorize and audit a
// group deletion.
type GroupDeleteSnapshot struct {
	Name          string
	Description   string
	IsSystemGroup bool
	IsActive      bool
	SCIMManaged   bool
}

// GetDeleteSnapshot returns the delete-authorization snapshot for a group.
// It returns ErrNotFound when the group does not exist.
func (r *GroupRepository) GetDeleteSnapshot(id int) (*GroupDeleteSnapshot, error) {
	var snap GroupDeleteSnapshot
	err := r.db.QueryRow(`
		SELECT name, description, is_system_group, is_active, COALESCE(scim_managed, false)
		FROM groups
		WHERE id = ?
	`, id).Scan(&snap.Name, &snap.Description, &snap.IsSystemGroup, &snap.IsActive, &snap.SCIMManaged)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get group delete snapshot: %w", err)
	}
	return &snap, nil
}

// Delete removes a group row.
func (r *GroupRepository) Delete(id int) error {
	if _, err := r.db.ExecWrite("DELETE FROM groups WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete group: %w", err)
	}
	return nil
}

// Exists reports whether a group exists.
func (r *GroupRepository) Exists(id int) (bool, error) {
	var exists bool
	if err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM groups WHERE id = ?)", id).Scan(&exists); err != nil {
		return false, fmt.Errorf("check group exists: %w", err)
	}
	return exists, nil
}

// GetName returns a group's name. It returns ErrNotFound when the group does
// not exist.
func (r *GroupRepository) GetName(id int) (string, error) {
	var name string
	err := r.db.QueryRow("SELECT name FROM groups WHERE id = ?", id).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get group name: %w", err)
	}
	return name, nil
}

// UserExists reports whether a user exists.
func (r *GroupRepository) UserExists(userID int) (bool, error) {
	var exists bool
	if err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE id = ?)", userID).Scan(&exists); err != nil {
		return false, fmt.Errorf("check user exists: %w", err)
	}
	return exists, nil
}

// MembershipExists reports whether a user is already a member of a group.
func (r *GroupRepository) MembershipExists(groupID, userID int) (bool, error) {
	var exists bool
	if err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM group_members WHERE group_id = ? AND user_id = ?)", groupID, userID).Scan(&exists); err != nil {
		return false, fmt.Errorf("check group membership: %w", err)
	}
	return exists, nil
}

// AddMember inserts a group membership and returns its ID.
func (r *GroupRepository) AddMember(groupID, userID int, addedBy *int, now time.Time) (int64, error) {
	var membershipID int64
	err := r.db.QueryRow(`
		INSERT INTO group_members (group_id, user_id, added_by, added_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?) RETURNING id
	`, groupID, userID, addedBy, now, now, now).Scan(&membershipID)
	if err != nil {
		return 0, fmt.Errorf("add group member: %w", err)
	}
	return membershipID, nil
}

// GetUserDisplay returns the email, full name and username for a user.
func (r *GroupRepository) GetUserDisplay(userID int) (email, name, username string, err error) {
	err = r.db.QueryRow("SELECT email, first_name || ' ' || last_name, username FROM users WHERE id = ?", userID).Scan(&email, &name, &username)
	if err != nil {
		return "", "", "", fmt.Errorf("get user display: %w", err)
	}
	return email, name, username, nil
}

// GetUsername returns a user's username.
func (r *GroupRepository) GetUsername(userID int) (string, error) {
	var username string
	if err := r.db.QueryRow("SELECT username FROM users WHERE id = ?", userID).Scan(&username); err != nil {
		return "", fmt.Errorf("get username: %w", err)
	}
	return username, nil
}

// RemoveMember deletes a group membership and reports how many rows were
// removed (0 when the user was not a member).
func (r *GroupRepository) RemoveMember(groupID, userID int) (int64, error) {
	result, err := r.db.ExecWrite("DELETE FROM group_members WHERE group_id = ? AND user_id = ?", groupID, userID)
	if err != nil {
		return 0, fmt.Errorf("remove group member: %w", err)
	}
	rows, _ := result.RowsAffected()
	return rows, nil
}

// ReplaceManualMembers atomically replaces the non-directory-managed members
// of a group. Directory-managed memberships are preserved.
func (r *GroupRepository) ReplaceManualMembers(groupID int, userIDs []int, addedBy *int, now time.Time) error {
	tx, err := r.db.BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("begin group membership replacement: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecWrite(`DELETE FROM group_members WHERE group_id = ? AND COALESCE(ldap_sync_enabled, false) = false`, groupID); err != nil {
		return fmt.Errorf("clear manual group memberships: %w", err)
	}
	for _, userID := range userIDs {
		if _, err := tx.ExecWrite(`
			INSERT INTO group_members (group_id, user_id, added_by, added_at, created_at, updated_at)
			SELECT ?, ?, ?, ?, ?, ?
			WHERE NOT EXISTS (
				SELECT 1 FROM group_members WHERE group_id = ? AND user_id = ?
			)
		`, groupID, userID, addedBy, now, now, now, groupID, userID); err != nil {
			return fmt.Errorf("add replacement group member %d: %w", userID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit group membership replacement: %w", err)
	}
	return nil
}

// ListUserMemberships returns the active groups a user belongs to, ordered by
// name. Note: each group's CreatedAt carries the membership added_at (the
// legacy query reused that field for member added_at).
func (r *GroupRepository) ListUserMemberships(userID int) ([]models.TeamGroup, error) {
	rows, err := r.db.Query(`
		SELECT
			g.id, g.name, g.description, g.ldap_distinguished_name, g.ldap_common_name,
			g.ldap_sync_enabled, g.ldap_last_sync_at, g.is_system_group, g.is_active,
			g.created_by, g.created_at, g.updated_at,
			u.first_name || ' ' || u.last_name as created_by_name,
			gm.added_at, gm.ldap_sync_enabled as member_ldap_sync
		FROM groups g
		JOIN group_members gm ON g.id = gm.group_id
		LEFT JOIN users u ON g.created_by = u.id
		WHERE gm.user_id = ? AND g.is_active = true
		ORDER BY g.name
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list user memberships: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var groups []models.TeamGroup
	for rows.Next() {
		var group models.TeamGroup
		var nf groupNullableFields
		var memberLdapSync bool

		err := rows.Scan(
			&group.ID, &group.Name, &group.Description, &nf.ldapDN, &nf.ldapCN,
			&group.LDAPSyncEnabled, &nf.ldapLastSync, &group.IsSystemGroup, &group.IsActive,
			&nf.createdBy, &group.CreatedAt, &group.UpdatedAt, &nf.createdByName,
			&group.CreatedAt, &memberLdapSync, // Reusing CreatedAt field for member added_at
		)
		if err != nil {
			return nil, fmt.Errorf("scan user membership: %w", err)
		}

		nf.applyTo(&group)
		groups = append(groups, group)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user memberships: %w", err)
	}
	return groups, nil
}
