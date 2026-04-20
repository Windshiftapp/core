package services

import (
	"database/sql"
	"fmt"

	"windshift/internal/database"
	"windshift/internal/models"
)

// UserReadService provides read operations for users
type UserReadService struct {
	db database.Database
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
	row := s.db.QueryRow(`
		SELECT id, email, username, first_name, last_name, is_active, avatar_url, timezone, language, COALESCE(is_agent, false), agent_owner_user_id, created_at
		FROM users WHERE id = ?
	`, id)

	u, err := scanUserRow(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found: %d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &u, nil
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
