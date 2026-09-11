package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/mail"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
)

// UserReadService provides read operations for users
type UserReadService struct {
	db database.Database
}

// AdminUserUpdate holds optional user fields for system-admin updates. Empty
// string values are ignored to preserve the existing REST v1 semantics.
type AdminUserUpdate struct {
	FirstName *string
	LastName  *string
	Email     *string
	Username  *string
	AvatarURL *string
	Timezone  *string
	Language  *string
	IsActive  *bool
}

// IsEmpty reports whether the update would touch no persisted fields.
func (u AdminUserUpdate) IsEmpty() bool {
	return (u.FirstName == nil || *u.FirstName == "") &&
		(u.LastName == nil || *u.LastName == "") &&
		(u.Email == nil || *u.Email == "") &&
		(u.Username == nil || *u.Username == "") &&
		u.AvatarURL == nil &&
		(u.Timezone == nil || *u.Timezone == "") &&
		(u.Language == nil || *u.Language == "") &&
		u.IsActive == nil
}

var (
	ErrUserManagedExternally = errors.New("user is managed externally")
	ErrUserEmailExists       = errors.New("email already exists")
	ErrUserUsernameExists    = errors.New("username already exists")
	ErrUserEmailInvalid      = errors.New("email is invalid")
)

// AdminUserUpdateResult carries the pre-image and updated user for transport
// audit projections.
type AdminUserUpdateResult struct {
	Before *repository.UpdateProfileSnapshot
	User   *models.User
}

// NewUserReadService creates a new user read service
func NewUserReadService(db database.Database) *UserReadService {
	return &UserReadService{db: db}
}

// hydrateUser populates nullable fields and the computed FullName on a User.
func hydrateUser(u *models.User, avatarURL, timezone, language sql.NullString) {
	u.FullName = u.FirstName + " " + u.LastName
	if avatarURL.Valid {
		u.AvatarURL = avatarURL.String
	}
	if timezone.Valid {
		u.Timezone = timezone.String
	}
	if language.Valid {
		u.Language = language.String
	}
}

// scanUserRow scans a single user row from the standard column set
// (id, email, username, first_name, last_name, is_active, avatar_url, timezone, language, is_agent, agent_owner_user_id, created_at)
// and returns a fully hydrated User.
func scanUserRow(scanner interface{ Scan(dest ...any) error }) (models.User, error) {
	var u models.User
	var avatarURL, timezone, language sql.NullString
	var agentOwnerUserID sql.NullInt64
	err := scanner.Scan(&u.ID, &u.Email, &u.Username, &u.FirstName, &u.LastName, &u.IsActive,
		&avatarURL, &timezone, &language, &u.IsAgent, &agentOwnerUserID, &u.CreatedAt)
	if err != nil {
		return u, err
	}
	hydrateUser(&u, avatarURL, timezone, language)
	if agentOwnerUserID.Valid {
		owner := int(agentOwnerUserID.Int64)
		u.AgentOwnerUserID = &owner
	}
	return u, nil
}

// List retrieves active users with pagination
func (s *UserReadService) List(pagination PaginationParams) ([]models.User, int, error) {
	rows, err := s.db.Query(`
		SELECT id, email, username, first_name, last_name, is_active, avatar_url, timezone, language, COALESCE(is_agent, false), agent_owner_user_id, created_at
		FROM users
		WHERE is_active = true
		ORDER BY first_name, last_name
		LIMIT ? OFFSET ?
	`, pagination.Limit, pagination.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list users: %w", err)
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		u, err := scanUserRow(rows)
		if err != nil {
			continue
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to iterate users: %w", err)
	}

	if users == nil {
		users = []models.User{}
	}

	// Get total count
	var total int
	err = s.db.QueryRow("SELECT COUNT(*) FROM users WHERE is_active = true").Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count users: %w", err)
	}

	return users, total, nil
}

// GetByID retrieves a user by ID
func (s *UserReadService) GetByID(id int) (*models.User, error) {
	return repository.NewUserRepository(s.db).GetByID(id)
}

// ListAdmin returns all users, including inactive users, with stable pagination.
func (s *UserReadService) ListAdmin(pagination PaginationParams) ([]models.User, int, error) {
	users, err := repository.NewUserRepository(s.db).ListAdmin()
	if err != nil {
		return nil, 0, err
	}
	total := len(users)
	start := pagination.Offset
	if start < 0 {
		start = 0
	}
	if start > total {
		start = total
	}
	end := total
	if pagination.Limit > 0 && start+pagination.Limit < end {
		end = start + pagination.Limit
	}
	return users[start:end], total, nil
}

// UpdateAdmin applies the shared administrator profile mutation. Empty string
// pointer values retain the existing value, matching the bearer compatibility
// contract. Activation remains an independently projected field.
func (s *UserReadService) UpdateAdmin(id int, update AdminUserUpdate) (*AdminUserUpdateResult, error) {
	sanitizeOptional := func(value *string, policy sanitize.Policy) {
		if value != nil {
			sanitize.Apply(value, policy)
		}
	}
	sanitizeOptional(update.FirstName, sanitize.PlainTextField)
	sanitizeOptional(update.LastName, sanitize.PlainTextField)
	sanitizeOptional(update.Username, sanitize.ShortIdentifier)
	sanitizeOptional(update.Timezone, sanitize.ShortIdentifier)
	sanitizeOptional(update.Language, sanitize.ShortIdentifier)
	repo := repository.NewUserRepository(s.db)
	before, err := repo.GetUpdateProfileSnapshot(id)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	if before.SCIMManaged {
		return nil, ErrUserManagedExternally
	}

	value := func(candidate *string, current string) string {
		if candidate == nil || *candidate == "" {
			return current
		}
		return *candidate
	}
	avatarURL := before.AvatarURL.String
	if update.AvatarURL != nil {
		avatarURL = *update.AvatarURL
	}
	timezone := value(update.Timezone, before.Timezone.String)
	if timezone == "" {
		timezone = "UTC"
	}
	language := value(update.Language, before.Language.String)
	if language == "" {
		language = "en"
	}
	params := repository.UpdateProfileParams{
		Email:     value(update.Email, before.Email),
		Username:  value(update.Username, before.Username),
		FirstName: value(update.FirstName, before.FirstName),
		LastName:  value(update.LastName, before.LastName),
		AvatarURL: avatarURL,
		Timezone:  timezone,
		Language:  language,
	}
	parsedEmail, parseErr := mail.ParseAddress(params.Email)
	if parseErr != nil || parsedEmail.Address != params.Email {
		return nil, ErrUserEmailInvalid
	}
	if exists, existsErr := repo.EmailExists(params.Email, id); existsErr != nil {
		return nil, existsErr
	} else if exists {
		return nil, ErrUserEmailExists
	}
	if exists, existsErr := repo.UsernameExists(params.Username, id); existsErr != nil {
		return nil, existsErr
	} else if exists {
		return nil, ErrUserUsernameExists
	}
	if err := repo.UpdateProfile(id, params); err != nil {
		if errors.Is(err, repository.ErrDuplicateEntry) {
			return nil, ErrUserEmailExists
		}
		return nil, err
	}
	if update.IsActive != nil && *update.IsActive != before.IsActive {
		if err := repo.SetActive(id, *update.IsActive); err != nil {
			return nil, err
		}
	}
	user, err := repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	return &AdminUserUpdateResult{Before: before, User: user}, nil
}

// GetGroupIDs returns active group membership IDs for a user.
func (s *UserReadService) GetGroupIDs(userID int) ([]int, error) {
	rows, err := s.db.Query("SELECT group_id FROM group_members WHERE user_id = ?", userID)
	if err != nil {
		return nil, fmt.Errorf("list user groups: %w", err)
	}
	defer func() { _ = rows.Close() }()

	ids := []int{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan user group: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user groups: %w", err)
	}
	return ids, nil
}

// ListAll retrieves all active users without pagination.
func (s *UserReadService) ListAll() ([]models.User, error) {
	rows, err := s.db.Query(`
		SELECT id, email, username, first_name, last_name, is_active, avatar_url, timezone, language, COALESCE(is_agent, false), agent_owner_user_id, created_at
		FROM users
		WHERE is_active = true
		ORDER BY first_name, last_name
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		u, err := scanUserRow(rows)
		if err != nil {
			continue
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate users: %w", err)
	}

	if users == nil {
		users = []models.User{}
	}

	return users, nil
}

// CountActive returns the number of active users.
func (s *UserReadService) CountActive() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM users WHERE is_active = true").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count active users: %w", err)
	}
	return count, nil
}

// Exists checks if a user exists by ID
func (s *UserReadService) Exists(id int) (bool, error) {
	var exists bool
	err := s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE id = ?)", id).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check user existence: %w", err)
	}
	return exists, nil
}

// ListAllowlistedCentralizedServiceUsers returns the active, unowned
// agent users (is_agent + agent_owner_user_id IS NULL) that the WI-87
// global allowlist makes reachable from this workspace — either via a
// workspace-scoped grant or a workspace_id IS NULL "any workspace"
// grant. Does NOT consult the master flag; the caller is responsible
// for gating that.
func (s *UserReadService) ListAllowlistedCentralizedServiceUsers(ctx context.Context, workspaceID int) ([]models.User, error) {
	if workspaceID <= 0 {
		return []models.User{}, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.id, u.email, u.username, u.first_name, u.last_name, u.is_active,
		       u.avatar_url, u.timezone, u.language,
		       COALESCE(u.is_agent, false), u.agent_owner_user_id, u.created_at
		FROM users u
		INNER JOIN global_agent_acting_user_allowlist a ON a.user_id = u.id
		WHERE COALESCE(u.is_agent, false) = true
		  AND u.agent_owner_user_id IS NULL
		  AND COALESCE(u.is_active, true) = true
		  AND (a.workspace_id IS NULL OR a.workspace_id = ?)
		GROUP BY u.id, u.email, u.username, u.first_name, u.last_name, u.is_active,
		         u.avatar_url, u.timezone, u.language, u.is_agent, u.agent_owner_user_id, u.created_at
		ORDER BY u.first_name, u.last_name, u.username
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list allowlisted centralized service users: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []models.User
	for rows.Next() {
		u, err := scanUserRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []models.User{}
	}
	return out, nil
}

// IsCentralizedServiceUser reports whether the user row exists, is an
// agent identity (is_agent = true), and is *not* owned by anyone
// (agent_owner_user_id IS NULL). The WI-87 admin allowlist editor uses
// this to refuse non-service users at the boundary — owned agents reach
// bindings through the chokepoint directly without needing a grant, and
// regular humans must never be impersonated by the harness. Returns
// (false, nil) when the user row does not exist so callers can render
// a 400 without leaking row existence.
func (s *UserReadService) IsCentralizedServiceUser(ctx context.Context, userID int) (bool, error) {
	if userID <= 0 {
		return false, nil
	}
	var (
		isAgent  bool
		hasOwner bool
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(is_agent, false), agent_owner_user_id IS NOT NULL
		FROM users WHERE id = ?
	`, userID).Scan(&isAgent, &hasOwner)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read user for service-user check: %w", err)
	}
	return isAgent && !hasOwner, nil
}
