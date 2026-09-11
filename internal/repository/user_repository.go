package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// UserRepository is the canonical home for users-table reads and writes.
// Both the admin user-management endpoints (handlers/users.go) and the
// scattered user-metadata lookups elsewhere (leave-period substitute check,
// channel-manager audit details, etc.) route through here.
type UserRepository struct {
	db database.Database
}

// NewUserRepository creates a UserRepository.
func NewUserRepository(db database.Database) *UserRepository {
	return &UserRepository{db: db}
}

// Exists reports whether a user row with the given id exists. Used by the
// leave-period handler's substitute-user check (and any other "is this user
// real?" gate).
func (r *UserRepository) Exists(id int) (bool, error) {
	var ok bool
	if err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE id = ?)", id).Scan(&ok); err != nil {
		return false, fmt.Errorf("check user %d: %w", id, err)
	}
	return ok, nil
}

// ActiveExists reports whether an active user row with the given id exists.
// It is intentionally separate from Exists: administrative workflows may
// need to reference inactive users, while assigning an inactive user as a
// channel manager would leave the channel without an effective manager.
func (r *UserRepository) ActiveExists(id int) (bool, error) {
	var ok bool
	if err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE id = ? AND is_active = true)", id).Scan(&ok); err != nil {
		return false, fmt.Errorf("check active user %d: %w", id, err)
	}
	return ok, nil
}

// GetIDByEmail returns the user ID for an email address.
func (r *UserRepository) GetIDByEmail(email string) (int, error) {
	var id int
	err := r.db.QueryRow(`SELECT id FROM users WHERE email = ?`, email).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}

// FindByEmailCaseInsensitive returns the small identity projection used when
// matching users from external systems.
func (r *UserRepository) FindByEmailCaseInsensitive(email string) (id int, username string, err error) {
	err = r.db.QueryRow(
		"SELECT id, username FROM users WHERE LOWER(email) = LOWER(?)",
		email,
	).Scan(&id, &username)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", ErrNotFound
	}
	if err != nil {
		return 0, "", fmt.Errorf("find user by email: %w", err)
	}
	return id, username, nil
}

// UsernameExistsCaseInsensitive reports whether a username is already in use
// without relying on database collation.
func (r *UserRepository) UsernameExistsCaseInsensitive(username string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(
		"SELECT EXISTS(SELECT 1 FROM users WHERE LOWER(username) = LOWER(?))",
		username,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check imported username %q: %w", username, err)
	}
	return exists, nil
}

// GetFullName returns "first_name last_name" for a user. Used to enrich
// audit details on channel-manager add/remove and similar admin actions.
// Returns empty string + nil if the row is missing (caller treats that as
// "unknown user").
func (r *UserRepository) GetFullName(ctx context.Context, userID int) (string, error) {
	var firstName, lastName string
	err := r.db.QueryRowContext(ctx,
		"SELECT first_name, last_name FROM users WHERE id = ?",
		userID,
	).Scan(&firstName, &lastName)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get user %d full name: %w", userID, err)
	}
	return strings.TrimSpace(firstName + " " + lastName), nil
}

// AdminUserRow is the joined shape returned by ListAdmin: a full user record
// plus the agent-owner's name (when the user is an owned agent).
type AdminUserRow struct {
	models.User
	OwnerFirstName string
	OwnerLastName  string
	OwnerUsername  string
}

// ListAdmin returns every user with the joined agent-owner display fields.
// Sorted by last_name, first_name. Used by the admin user list — non-admin
// listing routes through services.UserReadService.ListAll instead.
func (r *UserRepository) ListAdmin() ([]models.User, error) {
	rows, err := r.db.Query(`
		SELECT u.id, u.email, u.username, u.first_name, u.last_name, u.is_active, u.avatar_url,
			u.requires_password_reset, u.timezone, u.language, COALESCE(u.is_agent, false),
			u.agent_owner_user_id, o.first_name, o.last_name, o.username,
			u.created_at, u.updated_at
		FROM users u
		LEFT JOIN users o ON o.id = u.agent_owner_user_id
		ORDER BY u.last_name, u.first_name
	`)
	if err != nil {
		return nil, fmt.Errorf("list admin users: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var users []models.User
	for rows.Next() {
		var u models.User
		var avatarURL, timezone, language sql.NullString
		var requiresPasswordReset sql.NullBool
		var ownerID sql.NullInt64
		var ownerFirst, ownerLast, ownerUsername sql.NullString
		if err := rows.Scan(&u.ID, &u.Email, &u.Username, &u.FirstName, &u.LastName,
			&u.IsActive, &avatarURL, &requiresPasswordReset, &timezone, &language, &u.IsAgent,
			&ownerID, &ownerFirst, &ownerLast, &ownerUsername,
			&u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		u.AvatarURL = avatarURL.String
		u.RequiresPasswordReset = requiresPasswordReset.Bool
		u.Timezone = "UTC"
		if timezone.Valid {
			u.Timezone = timezone.String
		}
		u.Language = "en"
		if language.Valid {
			u.Language = language.String
		}
		if ownerID.Valid {
			id := int(ownerID.Int64)
			u.AgentOwnerUserID = &id
			name := strings.TrimSpace(ownerFirst.String + " " + ownerLast.String)
			if name == "" {
				name = ownerUsername.String
			}
			u.AgentOwnerName = name
		}
		u.FullName = strings.TrimSpace(u.FirstName + " " + u.LastName)
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate admin users: %w", err)
	}
	if users == nil {
		users = []models.User{}
	}
	return users, nil
}

// GetByEmailOrUsernameForAuth returns a user with password fields for login/change-password flows.
func (r *UserRepository) GetByEmailOrUsernameForAuth(emailOrUsername string) (*models.User, error) {
	var u models.User
	var avatarURL sql.NullString
	err := r.db.QueryRow(`
		SELECT id, email, username, first_name, last_name, is_active, avatar_url, password_hash, requires_password_reset, COALESCE(is_agent, false), created_at, updated_at
		FROM users
		WHERE email = ? OR username = ?
	`, emailOrUsername, emailOrUsername).Scan(
		&u.ID, &u.Email, &u.Username, &u.FirstName, &u.LastName,
		&u.IsActive, &avatarURL, &u.PasswordHash,
		&u.RequiresPasswordReset, &u.IsAgent, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, notFoundOrWrap(err, fmt.Sprintf("get auth user %q", emailOrUsername))
	}
	if avatarURL.Valid {
		u.AvatarURL = avatarURL.String
	}
	u.FullName = strings.TrimSpace(u.FirstName + " " + u.LastName)
	return &u, nil
}

// GetByID returns a user with the same column set as the admin Get endpoint
// surfaces (no agent-owner join — callers that want it use ListAdmin or
// fetch separately). Returns ErrNotFound when missing.
func (r *UserRepository) GetByID(id int) (*models.User, error) {
	var u models.User
	var avatarURL, timezone, language sql.NullString
	var requiresPasswordReset sql.NullBool
	var agentOwnerUserID sql.NullInt64
	err := r.db.QueryRow(`
		SELECT id, email, username, first_name, last_name, is_active, avatar_url, requires_password_reset, timezone, language, COALESCE(is_agent, false), agent_owner_user_id, created_at, updated_at
		FROM users WHERE id = ?
	`, id).Scan(&u.ID, &u.Email, &u.Username, &u.FirstName, &u.LastName,
		&u.IsActive, &avatarURL, &requiresPasswordReset, &timezone, &language, &u.IsAgent, &agentOwnerUserID, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, notFoundOrWrap(err, fmt.Sprintf("get user %d", id))
	}
	u.AvatarURL = avatarURL.String
	u.RequiresPasswordReset = requiresPasswordReset.Bool
	u.Timezone = "UTC"
	if timezone.Valid {
		u.Timezone = timezone.String
	}
	u.Language = "en"
	if language.Valid {
		u.Language = language.String
	}
	if agentOwnerUserID.Valid {
		ownerID := int(agentOwnerUserID.Int64)
		u.AgentOwnerUserID = &ownerID
	}
	u.FullName = strings.TrimSpace(u.FirstName + " " + u.LastName)
	return &u, nil
}

// AgentOwnerInfo carries just the fields the UI needs to attribute an agent
// row to its owner. Only populated for users where is_agent = TRUE and
// agent_owner_user_id is non-NULL.
type AgentOwnerInfo struct {
	UserID        int    `json:"user_id"`
	OwnerUserID   int    `json:"owner_user_id"`
	OwnerName     string `json:"owner_name"`
	OwnerUsername string `json:"owner_username,omitempty"`
}

// GetAgentOwner returns the owner attribution for an agent user, or
// ErrNotFound when the user isn't an agent or has no owner. Callers gate
// access on user.list / system admin — this method doesn't enforce that
// itself, it just returns the joined row.
func (r *UserRepository) GetAgentOwner(agentUserID int) (*AgentOwnerInfo, error) {
	var info AgentOwnerInfo
	var ownerFirst, ownerLast, ownerUsername sql.NullString
	err := r.db.QueryRow(`
		SELECT a.id, owner.id, owner.first_name, owner.last_name, owner.username
		FROM users a
		JOIN users owner ON owner.id = a.agent_owner_user_id
		WHERE a.id = ? AND COALESCE(a.is_agent, FALSE) = TRUE
	`, agentUserID).Scan(&info.UserID, &info.OwnerUserID, &ownerFirst, &ownerLast, &ownerUsername)
	if err != nil {
		return nil, notFoundOrWrap(err, fmt.Sprintf("get agent owner for user %d", agentUserID))
	}
	name := strings.TrimSpace(ownerFirst.String + " " + ownerLast.String)
	if name == "" {
		name = ownerUsername.String
	}
	info.OwnerName = name
	info.OwnerUsername = ownerUsername.String
	return &info, nil
}

// EmailExists / UsernameExists test for collisions on the unique columns.
// excludeID > 0 excludes that row from the check (so an Update doesn't
// collide with itself).
func (r *UserRepository) EmailExists(email string, excludeID int) (bool, error) {
	return r.uniqueCheck("email", email, excludeID)
}

func (r *UserRepository) UsernameExists(username string, excludeID int) (bool, error) {
	return r.uniqueCheck("username", username, excludeID)
}

func (r *UserRepository) uniqueCheck(column, value string, excludeID int) (bool, error) {
	// column is hardcoded by the two callers above (email/username) — fmt.Sprintf safe.
	var ok bool
	var err error
	if excludeID > 0 {
		err = r.db.QueryRow(
			fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM users WHERE %s = ? AND id != ?)", column),
			value, excludeID,
		).Scan(&ok)
	} else {
		err = r.db.QueryRow(
			fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM users WHERE %s = ?)", column),
			value,
		).Scan(&ok)
	}
	if err != nil {
		return false, fmt.Errorf("check user %s %q: %w", column, value, err)
	}
	return ok, nil
}

// CreateUserParams carries the fields needed to insert a new user.
// PasswordHash is optional (agent users and invited users have none).
// IsActive is honored as-is; callers default it to false to preserve the
// "require explicit activation" gate unless the admin opts in at create time.
type CreateUserParams struct {
	Email                 string
	Username              string
	FirstName             string
	LastName              string
	AvatarURL             string
	PasswordHash          *string
	RequiresPasswordReset bool
	IsActive              bool
	IsAgent               bool
	EmailVerified         bool
	// Agent provisioning columns; zero values insert NULL.
	AgentOwnerUserID *int
	AgentProvenance  string
	OAuthClientID    *int
}

// WorkspaceManagedAgentIdentityParams contains the identity fields created
// atomically with a workspace-managed agent profile.
type WorkspaceManagedAgentIdentityParams struct {
	Email           string
	Username        string
	Name            string
	AvatarURL       string
	WorkspaceID     int
	GrantedByUserID int
}

// CreateWorkspaceManagedAgentIdentity creates the agent user and grants Editor
// only when Everyone does not already inherit Editor access in the workspace.
func CreateWorkspaceManagedAgentIdentity(ctx context.Context, tx database.Tx, p WorkspaceManagedAgentIdentityParams) (int, error) {
	var id int
	err := tx.QueryRowContext(ctx, `
		INSERT INTO users
			(email, username, first_name, last_name, is_active, password_hash,
			 requires_password_reset, is_agent, agent_owner_user_id,
			 agent_provenance, avatar_url, email_verified)
		VALUES (?, ?, ?, '', true, NULL, false, true, NULL, 'user', ?, true)
		RETURNING id
	`, p.Email, p.Username, p.Name, nullableUserString(p.AvatarURL)).Scan(&id)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return 0, ErrDuplicateEntry
		}
		return 0, fmt.Errorf("create workspace-managed agent identity: %w", err)
	}

	var editorRoleID int
	if err := tx.QueryRowContext(ctx, `SELECT id FROM workspace_roles WHERE builtin_key = ?`, models.RoleBuiltinEditor).Scan(&editorRoleID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("load workspace-managed agent role %q: %w", models.RoleEditor, ErrNotFound)
		}
		return 0, fmt.Errorf("load workspace-managed agent role %q: %w", models.RoleEditor, err)
	}

	// Everyone inherits Editor while both Viewer and Editor have no explicit
	// user or group assignments. Preserve that open state by adding no row.
	var editorRestricted bool
	err = tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM user_workspace_roles uwr
			JOIN workspace_roles wr ON wr.id = uwr.role_id
			WHERE uwr.workspace_id = ? AND wr.builtin_key IN (?, ?)
			UNION ALL
			SELECT 1
			FROM group_workspace_roles gwr
			JOIN workspace_roles wr ON wr.id = gwr.role_id
			WHERE gwr.workspace_id = ? AND wr.builtin_key IN (?, ?)
		)
	`, p.WorkspaceID, models.RoleBuiltinViewer, models.RoleBuiltinEditor,
		p.WorkspaceID, models.RoleBuiltinViewer, models.RoleBuiltinEditor).Scan(&editorRestricted)
	if err != nil {
		return 0, fmt.Errorf("check workspace Editor restrictions: %w", err)
	}
	if !editorRestricted {
		return id, nil
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO user_workspace_roles
			(user_id, workspace_id, role_id, granted_by, granted_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
	`, id, p.WorkspaceID, editorRoleID, p.GrantedByUserID)
	if err != nil {
		return 0, fmt.Errorf("grant workspace-managed agent Editor role: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return 0, fmt.Errorf("grant workspace-managed agent role %q: %w", models.RoleEditor, ErrNotFound)
	}
	return id, nil
}

// CountOwnedAgents returns the number of agent users owned by ownerID.
func (r *UserRepository) CountOwnedAgents(ownerID int) (int, error) {
	var count int
	if err := r.db.QueryRow("SELECT COUNT(*) FROM users WHERE agent_owner_user_id = ?", ownerID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count agents owned by user %d: %w", ownerID, err)
	}
	return count, nil
}

func scanOwnedAgent(scanner interface{ Scan(...any) error }, ownerID int) (*models.User, error) {
	var user models.User
	var avatarURL sql.NullString
	if err := scanner.Scan(&user.ID, &user.Email, &user.Username, &user.FirstName, &user.LastName,
		&user.IsActive, &avatarURL, &user.CreatedAt, &user.UpdatedAt); err != nil {
		return nil, err
	}
	user.IsAgent = true
	user.AgentOwnerUserID = &ownerID
	user.AvatarURL = avatarURL.String
	user.FullName = strings.TrimSpace(user.FirstName + " " + user.LastName)
	return &user, nil
}

const ownedAgentColumns = "id, email, username, first_name, last_name, is_active, avatar_url, created_at, updated_at"

// FindOwnedAgentByUsername returns nil when no owned agent matches.
func (r *UserRepository) FindOwnedAgentByUsername(ownerID int, username string) (*models.User, error) {
	user, err := scanOwnedAgent(r.db.QueryRow(`SELECT `+ownedAgentColumns+`
		FROM users WHERE username = ? AND agent_owner_user_id = ? AND COALESCE(is_agent, false) = true`, username, ownerID), ownerID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find owned agent %q: %w", username, err)
	}
	return user, nil
}

// ListOwnedAgents returns newest agents first.
func (r *UserRepository) ListOwnedAgents(ownerID int) ([]models.User, error) {
	rows, err := r.db.Query(`SELECT `+ownedAgentColumns+`
		FROM users WHERE agent_owner_user_id = ? ORDER BY created_at DESC`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list agents owned by user %d: %w", ownerID, err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]models.User, 0)
	for rows.Next() {
		user, err := scanOwnedAgent(rows, ownerID)
		if err != nil {
			return nil, fmt.Errorf("scan owned agent: %w", err)
		}
		out = append(out, *user)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate owned agents: %w", err)
	}
	return out, nil
}

// UpdateOwnedAgentName updates and returns an owned agent or ErrNotFound.
func (r *UserRepository) UpdateOwnedAgentName(agentID, ownerID int, name string) (*models.User, error) {
	result, err := r.db.ExecWrite(`UPDATE users SET first_name = ?, last_name = '', updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND agent_owner_user_id = ? AND COALESCE(is_agent, false) = true`, name, agentID, ownerID)
	if err != nil {
		return nil, fmt.Errorf("update owned agent %d: %w", agentID, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("count updated owned agent rows: %w", err)
	}
	if rows == 0 {
		return nil, ErrNotFound
	}
	user, err := scanOwnedAgent(r.db.QueryRow("SELECT "+ownedAgentColumns+" FROM users WHERE id = ?", agentID), ownerID)
	if err != nil {
		return nil, fmt.Errorf("reload owned agent %d: %w", agentID, err)
	}
	return user, nil
}

// OwnedAgentTarget returns the owner and username used by the delete policy.
func (r *UserRepository) OwnedAgentTarget(agentID int) (ownerID *int, username string, err error) {
	var owner sql.NullInt64
	err = r.db.QueryRow("SELECT agent_owner_user_id, username FROM users WHERE id = ?", agentID).Scan(&owner, &username)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", fmt.Errorf("get owned agent target %d: %w", agentID, err)
	}
	if owner.Valid {
		value := int(owner.Int64)
		ownerID = &value
	}
	return ownerID, username, nil
}

// Delete removes a user row.
func (r *UserRepository) Delete(id int) error {
	if _, err := r.db.ExecWrite("DELETE FROM users WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete user %d: %w", id, err)
	}
	return nil
}

type UserNameMatch struct {
	ID       int
	FullName string
}

// FindIDsByFullName returns exact case-insensitive full-name matches.
func (r *UserRepository) FindIDsByFullName(name string) ([]UserNameMatch, error) {
	rows, err := r.db.Query(`SELECT id, first_name || ' ' || last_name
		FROM users WHERE LOWER(first_name || ' ' || last_name) = LOWER(?) ORDER BY id`, name)
	if err != nil {
		return nil, fmt.Errorf("find users by full name: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]UserNameMatch, 0)
	for rows.Next() {
		var match UserNameMatch
		if err := rows.Scan(&match.ID, &match.FullName); err != nil {
			return nil, fmt.Errorf("scan user full-name match: %w", err)
		}
		out = append(out, match)
	}
	return out, rows.Err()
}

// Create inserts a new user with the supplied is_active value. Returns
// ErrDuplicateEntry when the unique (email/username) constraint trips.
func (r *UserRepository) Create(p CreateUserParams) (int64, error) {
	now := time.Now()
	// Non-agent rows carry the column default provenance ("user"); agent rows
	// supply their own (e.g. "oauth").
	provenance := p.AgentProvenance
	if provenance == "" {
		provenance = "user"
	}
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO users (email, username, first_name, last_name, is_active, avatar_url, password_hash, requires_password_reset, is_agent, email_verified, agent_owner_user_id, agent_provenance, oauth_client_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, p.Email, p.Username, p.FirstName, p.LastName, p.IsActive,
		nullableUserString(p.AvatarURL), nullableUserPtrString(p.PasswordHash),
		p.RequiresPasswordReset, p.IsAgent, p.EmailVerified,
		nullableUserPtrInt(p.AgentOwnerUserID), provenance, nullableUserPtrInt(p.OAuthClientID),
		now, now,
	).Scan(&id)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return 0, ErrDuplicateEntry
		}
		return 0, fmt.Errorf("create user: %w", err)
	}
	return id, nil
}

// UpdateProfileSnapshot is the read-side data the Update audit needs to
// detect what fields changed and to enforce the SCIM-managed gate.
type UpdateProfileSnapshot struct {
	Email       string
	Username    string
	FirstName   string
	LastName    string
	IsActive    bool
	AvatarURL   sql.NullString
	Timezone    sql.NullString
	Language    sql.NullString
	SCIMManaged bool
}

// GetUpdateProfileSnapshot reads the columns the Update audit + SCIM gate
// need. Returns ErrNotFound when missing.
func (r *UserRepository) GetUpdateProfileSnapshot(id int) (*UpdateProfileSnapshot, error) {
	var s UpdateProfileSnapshot
	err := r.db.QueryRow(`
		SELECT email, username, first_name, last_name, is_active, avatar_url, timezone, language,
		       COALESCE(scim_managed, false)
		FROM users WHERE id = ?
	`, id).Scan(&s.Email, &s.Username, &s.FirstName, &s.LastName, &s.IsActive,
		&s.AvatarURL, &s.Timezone, &s.Language, &s.SCIMManaged)
	if err != nil {
		return nil, notFoundOrWrap(err, fmt.Sprintf("get user %d update snapshot", id))
	}
	return &s, nil
}

// UpdateProfileParams is the editable subset of a user record carried by
// PUT /api/users/{id}.
type UpdateProfileParams struct {
	Email     string
	Username  string
	FirstName string
	LastName  string
	AvatarURL string
	Timezone  string
	Language  string
}

// UpdateProfile writes the editable fields. Returns ErrDuplicateEntry on
// unique-constraint trip.
func (r *UserRepository) UpdateProfile(id int, p UpdateProfileParams) error {
	_, err := r.db.ExecWrite(`
		UPDATE users
		SET email = ?, username = ?, first_name = ?, last_name = ?, avatar_url = ?, timezone = ?, language = ?, updated_at = ?
		WHERE id = ?
	`, p.Email, p.Username, p.FirstName, p.LastName,
		nullableUserString(p.AvatarURL), p.Timezone, p.Language, time.Now(), id)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return ErrDuplicateEntry
		}
		return fmt.Errorf("update user %d: %w", id, err)
	}
	return nil
}

// UpdateAvatar writes only the avatar_url column.
func (r *UserRepository) UpdateAvatar(id int, avatarURL string) error {
	if _, err := r.db.ExecWrite(
		"UPDATE users SET avatar_url = ?, updated_at = ? WHERE id = ?",
		avatarURL, time.Now(), id,
	); err != nil {
		return fmt.Errorf("update user %d avatar: %w", id, err)
	}
	return nil
}

// RegionalSnapshot carries the small subset the regional-settings update
// needs for change-tracking audit (plus the username for the audit row).
type RegionalSnapshot struct {
	Username string
	Timezone sql.NullString
	Language sql.NullString
}

// GetRegionalSnapshot reads the timezone/language for an audit pre-image.
// Returns ErrNotFound when missing.
func (r *UserRepository) GetRegionalSnapshot(id int) (*RegionalSnapshot, error) {
	var s RegionalSnapshot
	err := r.db.QueryRow(
		"SELECT username, timezone, language FROM users WHERE id = ?",
		id,
	).Scan(&s.Username, &s.Timezone, &s.Language)
	if err != nil {
		return nil, notFoundOrWrap(err, fmt.Sprintf("get user %d regional snapshot", id))
	}
	return &s, nil
}

// UpdateRegional writes only timezone + language.
func (r *UserRepository) UpdateRegional(id int, timezone, language string) error {
	if _, err := r.db.ExecWrite(`
		UPDATE users SET timezone = ?, language = ?, updated_at = ? WHERE id = ?
	`, timezone, language, time.Now(), id); err != nil {
		return fmt.Errorf("update user %d regional: %w", id, err)
	}
	return nil
}

// DeleteSnapshot is the small read-side payload Delete audits with before
// the row is anonymized via services.OffboardUser.
type DeleteSnapshot struct {
	Username    string
	Email       string
	FirstName   string
	LastName    string
	SCIMManaged bool
}

// GetDeleteSnapshot reads the columns the Delete audit needs (and the SCIM
// gate). Returns ErrNotFound when missing.
func (r *UserRepository) GetDeleteSnapshot(id int) (*DeleteSnapshot, error) {
	var s DeleteSnapshot
	err := r.db.QueryRow(`
		SELECT username, email, first_name, last_name, COALESCE(scim_managed, false)
		FROM users WHERE id = ?
	`, id).Scan(&s.Username, &s.Email, &s.FirstName, &s.LastName, &s.SCIMManaged)
	if err != nil {
		return nil, notFoundOrWrap(err, fmt.Sprintf("get user %d delete snapshot", id))
	}
	return &s, nil
}

// PasswordResetTarget is the small subset the password-reset audit needs.
type PasswordResetTarget struct {
	Username   string
	Email      string
	Offboarded bool
}

// GetPasswordResetTarget returns username+email for the reset audit.
// Returns ErrNotFound when missing.
func (r *UserRepository) GetPasswordResetTarget(id int) (*PasswordResetTarget, error) {
	var t PasswordResetTarget
	err := r.db.QueryRow(
		"SELECT username, email, offboarded_at IS NOT NULL FROM users WHERE id = ?",
		id,
	).Scan(&t.Username, &t.Email, &t.Offboarded)
	if err != nil {
		return nil, notFoundOrWrap(err, fmt.Sprintf("get user %d for password reset", id))
	}
	return &t, nil
}

// SetPassword writes a new password hash and updates requires_password_reset.
func (r *UserRepository) SetPassword(id int, passwordHash string, requiresReset bool) error {
	if _, err := r.db.ExecWrite(`
		UPDATE users SET password_hash = ?, requires_password_reset = ?, updated_at = ? WHERE id = ?
	`, passwordHash, requiresReset, time.Now(), id); err != nil {
		return fmt.Errorf("set user %d password: %w", id, err)
	}
	return nil
}

// ActivationTarget carries username/email/is_active for the activate/deactivate
// audit + idempotence check, plus the irreversible offboarding state.
type ActivationTarget struct {
	Username   string
	Email      string
	IsActive   bool
	Offboarded bool
}

// GetActivationTarget reads the activate/deactivate audit fields.
// Returns ErrNotFound when missing.
func (r *UserRepository) GetActivationTarget(id int) (*ActivationTarget, error) {
	var t ActivationTarget
	err := r.db.QueryRow(
		"SELECT username, email, is_active, offboarded_at IS NOT NULL FROM users WHERE id = ?",
		id,
	).Scan(&t.Username, &t.Email, &t.IsActive, &t.Offboarded)
	if err != nil {
		return nil, notFoundOrWrap(err, fmt.Sprintf("get user %d activation target", id))
	}
	return &t, nil
}

// SetActive flips the is_active column.
func (r *UserRepository) SetActive(id int, active bool) error {
	if _, err := r.db.ExecWrite(
		"UPDATE users SET is_active = ?, updated_at = ? WHERE id = ?",
		active, time.Now(), id,
	); err != nil {
		return fmt.Errorf("set user %d active=%t: %w", id, active, err)
	}
	return nil
}

func nullableUserString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullableUserPtrString(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

func nullableUserPtrInt(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}
