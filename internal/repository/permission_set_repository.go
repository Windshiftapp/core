package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// PermissionSetRepository persists permission_sets and their three
// assignment join tables (role / group / user). Cache invalidation lives
// outside this layer (the handler calls services.PermissionService when a
// mutation lands).
type PermissionSetRepository struct {
	db database.Database
}

// NewPermissionSetRepository creates a PermissionSetRepository.
func NewPermissionSetRepository(db database.Database) *PermissionSetRepository {
	return &PermissionSetRepository{db: db}
}

const permissionSetSelectColumns = "id, name, description, is_system, created_by, created_at, updated_at"

// List returns all permission_sets ordered by is_system DESC then name ASC.
func (r *PermissionSetRepository) List() ([]models.PermissionSet, error) {
	rows, err := r.db.Query(
		"SELECT " + permissionSetSelectColumns + " FROM permission_sets ORDER BY is_system DESC, name ASC",
	)
	if err != nil {
		return nil, fmt.Errorf("list permission_sets: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var sets []models.PermissionSet
	for rows.Next() {
		ps, scanErr := scanPermissionSet(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan permission_set: %w", scanErr)
		}
		sets = append(sets, ps)
	}
	if sets == nil {
		sets = []models.PermissionSet{}
	}
	return sets, nil
}

// GetByID returns a permission_set's metadata only (no Permissions).
// Returns ErrNotFound when missing.
func (r *PermissionSetRepository) GetByID(id int) (*models.PermissionSet, error) {
	row := r.db.QueryRow(
		"SELECT "+permissionSetSelectColumns+" FROM permission_sets WHERE id = ?",
		id,
	)
	ps, err := scanPermissionSet(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get permission_set %d: %w", id, err)
	}
	return &ps, nil
}

// GetByIDWithPermissions loads the set plus its assigned permissions.
func (r *PermissionSetRepository) GetByIDWithPermissions(id int) (*models.PermissionSet, error) {
	ps, err := r.GetByID(id)
	if err != nil {
		return nil, err
	}

	rows, err := r.db.Query(`
		SELECT p.id, p.permission_key, p.permission_name, p.description, p.scope, p.is_system, p.created_at, p.updated_at
		FROM permissions p
		JOIN permission_set_permissions psp ON p.id = psp.permission_id
		WHERE psp.permission_set_id = ?
		ORDER BY p.scope, p.permission_name
	`, id)
	if err != nil {
		return nil, fmt.Errorf("list permissions for set %d: %w", id, err)
	}
	defer func() { _ = rows.Close() }()

	ps.Permissions = []models.Permission{}
	for rows.Next() {
		var perm models.Permission
		if err := rows.Scan(&perm.ID, &perm.PermissionKey, &perm.PermissionName,
			&perm.Description, &perm.Scope, &perm.IsSystem, &perm.CreatedAt, &perm.UpdatedAt); err != nil {
			continue
		}
		ps.Permissions = append(ps.Permissions, perm)
	}
	return ps, nil
}

// Exists reports whether a permission_set row with the given id exists.
func (r *PermissionSetRepository) Exists(id int) (bool, error) {
	var ok bool
	if err := r.db.QueryRow(
		"SELECT EXISTS(SELECT 1 FROM permission_sets WHERE id = ?)",
		id,
	).Scan(&ok); err != nil {
		return false, fmt.Errorf("check permission_set %d: %w", id, err)
	}
	return ok, nil
}

// CountConfigSetUsage returns how many configuration_sets reference the
// given permission_set. The handler refuses delete when this is > 0.
func (r *PermissionSetRepository) CountConfigSetUsage(id int) (int, error) {
	var n int
	if err := r.db.QueryRow(
		"SELECT COUNT(*) FROM configuration_sets WHERE permission_set_id = ?",
		id,
	).Scan(&n); err != nil {
		return 0, fmt.Errorf("count configuration_sets using permission_set %d: %w", id, err)
	}
	return n, nil
}

// Create inserts a new permission_set and the row→permission mappings in
// permission_set_permissions for each ID in permissionIDs. createdBy is the
// user the set was created on behalf of. Returns the new set ID.
func (r *PermissionSetRepository) Create(name, description string, createdBy int, permissionIDs []int) (int64, error) {
	now := time.Now()
	var permSetID int64
	err := r.db.QueryRow(`
		INSERT INTO permission_sets (name, description, is_system, created_by, created_at, updated_at)
		VALUES (?, ?, false, ?, ?, ?) RETURNING id
	`, name, description, createdBy, now, now).Scan(&permSetID)
	if err != nil {
		return 0, fmt.Errorf("create permission_set: %w", err)
	}

	for _, permID := range permissionIDs {
		if _, err := r.db.ExecWrite(`
			INSERT INTO permission_set_permissions (permission_set_id, permission_id, granted_by, granted_at)
			VALUES (?, ?, ?, ?)
		`, permSetID, permID, createdBy, now); err != nil {
			// Mirrors the prior handler behavior: best-effort, individual
			// permission inserts don't roll back the set creation.
			return permSetID, fmt.Errorf("add permission %d to set %d: %w", permID, permSetID, err)
		}
	}
	return permSetID, nil
}

// UpdateMetadata updates name and description of an existing permission_set.
func (r *PermissionSetRepository) UpdateMetadata(id int, name, description string) error {
	if _, err := r.db.ExecWrite(`
		UPDATE permission_sets
		SET name = ?, description = ?, updated_at = ?
		WHERE id = ?
	`, name, description, time.Now(), id); err != nil {
		return fmt.Errorf("update permission_set %d: %w", id, err)
	}
	return nil
}

// ReplacePermissions clears the permission_set_permissions rows for the set
// and re-inserts the given permissionIDs. Not transactional — matches the
// pre-existing behavior; repository.WithTx callers can wrap it if needed.
func (r *PermissionSetRepository) ReplacePermissions(setID int, permissionIDs []int, grantedBy int) error {
	if _, err := r.db.ExecWrite(
		"DELETE FROM permission_set_permissions WHERE permission_set_id = ?",
		setID,
	); err != nil {
		return fmt.Errorf("clear permissions for set %d: %w", setID, err)
	}

	now := time.Now()
	for _, permID := range permissionIDs {
		if _, err := r.db.ExecWrite(`
			INSERT INTO permission_set_permissions (permission_set_id, permission_id, granted_by, granted_at)
			VALUES (?, ?, ?, ?)
		`, setID, permID, grantedBy, now); err != nil {
			return fmt.Errorf("add permission %d to set %d: %w", permID, setID, err)
		}
	}
	return nil
}

// Delete removes a permission_set row. The permission_set_permissions and
// per-target assignment rows are FK-cascade deleted by the schema.
func (r *PermissionSetRepository) Delete(id int) error {
	if _, err := r.db.ExecWrite("DELETE FROM permission_sets WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete permission_set %d: %w", id, err)
	}
	return nil
}

// PermissionSetAssignments groups the three assignment-table reads that
// feed the FE assignment editor.
type PermissionSetAssignments struct {
	RoleAssignments  []models.PermissionSetRoleAssignment
	GroupAssignments []models.PermissionSetGroupAssignment
	UserAssignments  []models.PermissionSetUserAssignment
}

// ListAssignments returns the three assignment lists for the given set.
// Errors on individual rows are skipped (matches prior handler behavior of
// "scan-or-skip"); query-level errors propagate.
func (r *PermissionSetRepository) ListAssignments(setID int) (PermissionSetAssignments, error) {
	out := PermissionSetAssignments{
		RoleAssignments:  []models.PermissionSetRoleAssignment{},
		GroupAssignments: []models.PermissionSetGroupAssignment{},
		UserAssignments:  []models.PermissionSetUserAssignment{},
	}

	roleRows, err := r.db.Query(`
		SELECT ra.id, ra.permission_set_id, ra.permission_id, ra.role_id, ra.created_by, ra.created_at,
		       p.permission_key, p.permission_name, p.description,
		       r.name as role_name
		FROM permission_set_role_assignments ra
		JOIN permissions p ON ra.permission_id = p.id
		JOIN workspace_roles r ON ra.role_id = r.id
		WHERE ra.permission_set_id = ?
		ORDER BY p.permission_name, r.name
	`, setID)
	if err != nil {
		return out, fmt.Errorf("list role assignments for set %d: %w", setID, err)
	}
	defer func() { _ = roleRows.Close() }()
	for roleRows.Next() {
		var ra models.PermissionSetRoleAssignment
		var perm models.Permission
		var role models.WorkspaceRole
		if err := roleRows.Scan(&ra.ID, &ra.PermissionSetID, &ra.PermissionID, &ra.RoleID, &ra.CreatedBy, &ra.CreatedAt,
			&perm.PermissionKey, &perm.PermissionName, &perm.Description, &role.Name); err != nil {
			continue
		}
		perm.ID = ra.PermissionID
		role.ID = ra.RoleID
		ra.Permission = &perm
		ra.Role = &role
		out.RoleAssignments = append(out.RoleAssignments, ra)
	}

	groupRows, err := r.db.Query(`
		SELECT ga.id, ga.permission_set_id, ga.permission_id, ga.group_id, ga.created_by, ga.created_at,
		       p.permission_key, p.permission_name, p.description,
		       g.name as group_name
		FROM permission_set_group_assignments ga
		JOIN permissions p ON ga.permission_id = p.id
		JOIN groups g ON ga.group_id = g.id
		WHERE ga.permission_set_id = ?
		ORDER BY p.permission_name, g.name
	`, setID)
	if err != nil {
		return out, fmt.Errorf("list group assignments for set %d: %w", setID, err)
	}
	defer func() { _ = groupRows.Close() }()
	for groupRows.Next() {
		var ga models.PermissionSetGroupAssignment
		var perm models.Permission
		var group models.Group
		if err := groupRows.Scan(&ga.ID, &ga.PermissionSetID, &ga.PermissionID, &ga.GroupID, &ga.CreatedBy, &ga.CreatedAt,
			&perm.PermissionKey, &perm.PermissionName, &perm.Description, &group.GroupName); err != nil {
			continue
		}
		perm.ID = ga.PermissionID
		group.ID = ga.GroupID
		ga.Permission = &perm
		ga.Group = &group
		out.GroupAssignments = append(out.GroupAssignments, ga)
	}

	userRows, err := r.db.Query(`
		SELECT ua.id, ua.permission_set_id, ua.permission_id, ua.user_id, ua.created_by, ua.created_at,
		       p.permission_key, p.permission_name, p.description,
		       u.username, u.first_name, u.last_name
		FROM permission_set_user_assignments ua
		JOIN permissions p ON ua.permission_id = p.id
		JOIN users u ON ua.user_id = u.id
		WHERE ua.permission_set_id = ?
		ORDER BY p.permission_name, u.username
	`, setID)
	if err != nil {
		return out, fmt.Errorf("list user assignments for set %d: %w", setID, err)
	}
	defer func() { _ = userRows.Close() }()
	for userRows.Next() {
		var ua models.PermissionSetUserAssignment
		var perm models.Permission
		var user models.User
		if err := userRows.Scan(&ua.ID, &ua.PermissionSetID, &ua.PermissionID, &ua.UserID, &ua.CreatedBy, &ua.CreatedAt,
			&perm.PermissionKey, &perm.PermissionName, &perm.Description,
			&user.Username, &user.FirstName, &user.LastName); err != nil {
			continue
		}
		perm.ID = ua.PermissionID
		user.ID = ua.UserID
		ua.Permission = &perm
		ua.User = &user
		out.UserAssignments = append(out.UserAssignments, ua)
	}

	return out, nil
}

// AssignmentKind selects which assignment table CreateAssignment / DeleteAssignment
// targets. The repo validates against an allow-list internally.
type AssignmentKind string

const (
	AssignmentKindRole  AssignmentKind = "role"
	AssignmentKindGroup AssignmentKind = "group"
	AssignmentKindUser  AssignmentKind = "user"
)

var assignmentTables = map[AssignmentKind]string{
	AssignmentKindRole:  "permission_set_role_assignments",
	AssignmentKindGroup: "permission_set_group_assignments",
	AssignmentKindUser:  "permission_set_user_assignments",
}

var assignmentTargetColumns = map[AssignmentKind]string{
	AssignmentKindRole:  "role_id",
	AssignmentKindGroup: "group_id",
	AssignmentKindUser:  "user_id",
}

// CreateAssignment inserts an assignment row in the table selected by kind.
// Returns ErrDuplicateEntry when the (set, permission, target) tuple already
// exists.
func (r *PermissionSetRepository) CreateAssignment(setID, permissionID, targetID, createdBy int, kind AssignmentKind) error {
	table, ok := assignmentTables[kind]
	if !ok {
		return fmt.Errorf("permission_set assignment: unknown kind %q", kind)
	}
	column := assignmentTargetColumns[kind]

	// table + column come from the closed allow-list above; the fmt.Sprintf
	// cannot splice attacker-controlled input.
	query := fmt.Sprintf(
		"INSERT INTO %s (permission_set_id, permission_id, %s, created_by, created_at) VALUES (?, ?, ?, ?, ?)",
		table, column,
	)
	if _, err := r.db.ExecWrite(query, setID, permissionID, targetID, createdBy, time.Now()); err != nil {
		if database.IsUniqueConstraintError(err) {
			return ErrDuplicateEntry
		}
		return fmt.Errorf("create %s assignment for set %d: %w", kind, setID, err)
	}
	return nil
}

// DeleteAssignment removes an assignment. Returns ErrNotFound when no row
// matches (id wrong, or not in the named set).
func (r *PermissionSetRepository) DeleteAssignment(setID, assignmentID int, kind AssignmentKind) error {
	table, ok := assignmentTables[kind]
	if !ok {
		return fmt.Errorf("permission_set assignment: unknown kind %q", kind)
	}

	// table from the closed allow-list above; safe to splice.
	query := fmt.Sprintf("DELETE FROM %s WHERE id = ? AND permission_set_id = ?", table)
	result, err := r.db.ExecWrite(query, assignmentID, setID)
	if err != nil {
		return fmt.Errorf("delete %s assignment %d for set %d: %w", kind, assignmentID, setID, err)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func scanPermissionSet(scanner interface {
	Scan(dest ...any) error
}) (models.PermissionSet, error) {
	var ps models.PermissionSet
	if err := scanner.Scan(&ps.ID, &ps.Name, &ps.Description, &ps.IsSystem,
		&ps.CreatedBy, &ps.CreatedAt, &ps.UpdatedAt); err != nil {
		return ps, err
	}
	return ps, nil
}
