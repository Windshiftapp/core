package repository

import (
	"context"
	"database/sql"
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
	if err == sql.ErrNoRows {
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
	if users == nil {
		users = []models.User{}
	}
	return users, nil
}

// GetByID returns a user with the same column set as the admin Get endpoint
// surfaces (no agent-owner join — callers that want it use ListAdmin or
// fetch separately). Returns ErrNotFound when missing.
func (r *UserRepository) GetByID(id int) (*models.User, error) {
	var u models.User
	var avatarURL, timezone, language sql.NullString
	var requiresPasswordReset sql.NullBool
	err := r.db.QueryRow(`
		SELECT id, email, username, first_name, last_name, is_active, avatar_url, requires_password_reset, timezone, language, COALESCE(is_agent, false), created_at, updated_at
		FROM users WHERE id = ?
	`, id).Scan(&u.ID, &u.Email, &u.Username, &u.FirstName, &u.LastName,
		&u.IsActive, &avatarURL, &requiresPasswordReset, &timezone, &language, &u.IsAgent, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get user %d: %w", id, err)
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
	u.FullName = strings.TrimSpace(u.FirstName + " " + u.LastName)
	return &u, nil
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
type CreateUserParams struct {
	Email                 string
	Username              string
	FirstName             string
	LastName              string
	AvatarURL             string
	PasswordHash          *string
	RequiresPasswordReset bool
	IsAgent               bool
	EmailVerified         bool
}

// Create inserts a new user with is_active=false. Returns ErrDuplicateEntry
// when the unique (email/username) constraint trips.
func (r *UserRepository) Create(p CreateUserParams) (int64, error) {
	now := time.Now()
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO users (email, username, first_name, last_name, is_active, avatar_url, password_hash, requires_password_reset, is_agent, email_verified, created_at, updated_at)
		VALUES (?, ?, ?, ?, false, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, p.Email, p.Username, p.FirstName, p.LastName,
		nullableUserString(p.AvatarURL), nullableUserPtrString(p.PasswordHash),
		p.RequiresPasswordReset, p.IsAgent, p.EmailVerified, now, now,
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
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get user %d update snapshot: %w", id, err)
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
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get user %d regional snapshot: %w", id, err)
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
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get user %d delete snapshot: %w", id, err)
	}
	return &s, nil
}

// PasswordResetTarget is the small subset the password-reset audit needs.
type PasswordResetTarget struct {
	Username string
	Email    string
}

// GetPasswordResetTarget returns username+email for the reset audit.
// Returns ErrNotFound when missing.
func (r *UserRepository) GetPasswordResetTarget(id int) (*PasswordResetTarget, error) {
	var t PasswordResetTarget
	err := r.db.QueryRow(
		"SELECT username, email FROM users WHERE id = ?",
		id,
	).Scan(&t.Username, &t.Email)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get user %d for password reset: %w", id, err)
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
// audit + idempotence check.
type ActivationTarget struct {
	Username string
	Email    string
	IsActive bool
}

// GetActivationTarget reads the activate/deactivate audit fields.
// Returns ErrNotFound when missing.
func (r *UserRepository) GetActivationTarget(id int) (*ActivationTarget, error) {
	var t ActivationTarget
	err := r.db.QueryRow(
		"SELECT username, email, is_active FROM users WHERE id = ?",
		id,
	).Scan(&t.Username, &t.Email, &t.IsActive)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get user %d activation target: %w", id, err)
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
