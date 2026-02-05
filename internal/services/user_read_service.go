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

// List retrieves active users with pagination
func (s *UserReadService) List(pagination PaginationParams) ([]models.User, int, error) {
	rows, err := s.db.Query(`
		SELECT id, email, username, first_name, last_name, is_active, avatar_url, timezone, language, created_at
		FROM users
		WHERE is_active = 1
		ORDER BY first_name, last_name
		LIMIT ? OFFSET ?
	`, pagination.Limit, pagination.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list users: %w", err)
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var u models.User
		var avatarURL, timezone, language sql.NullString
		err = rows.Scan(&u.ID, &u.Email, &u.Username, &u.FirstName, &u.LastName, &u.IsActive,
			&avatarURL, &timezone, &language, &u.CreatedAt)
		if err != nil {
			continue
		}
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
		users = append(users, u)
	}

	if users == nil {
		users = []models.User{}
	}

	// Get total count
	var total int
	err = s.db.QueryRow("SELECT COUNT(*) FROM users WHERE is_active = 1").Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count users: %w", err)
	}

	return users, total, nil
}

// GetByID retrieves a user by ID
func (s *UserReadService) GetByID(id int) (*models.User, error) {
	var u models.User
	var avatarURL, timezone, language sql.NullString

	err := s.db.QueryRow(`
		SELECT id, email, username, first_name, last_name, is_active, avatar_url, timezone, language, created_at
		FROM users WHERE id = ?
	`, id).Scan(&u.ID, &u.Email, &u.Username, &u.FirstName, &u.LastName, &u.IsActive,
		&avatarURL, &timezone, &language, &u.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found: %d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

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

	return &u, nil
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
