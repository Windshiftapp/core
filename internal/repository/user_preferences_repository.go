package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"windshift/internal/database"
)

// UserPreferencesRepository provides data access for user preference rows.
type UserPreferencesRepository struct {
	db database.Database
}

// NewUserPreferencesRepository creates a new UserPreferencesRepository.
func NewUserPreferencesRepository(db database.Database) *UserPreferencesRepository {
	return &UserPreferencesRepository{db: db}
}

// GetJSON returns the raw preferences JSON for a user.
func (r *UserPreferencesRepository) GetJSON(userID int) (string, error) {
	var prefs string
	err := r.db.QueryRow("SELECT preferences FROM user_preferences WHERE user_id = ?", userID).Scan(&prefs)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get user preferences %d: %w", userID, err)
	}
	return prefs, nil
}

// UpsertJSON creates or updates the raw preferences JSON for a user.
func (r *UserPreferencesRepository) UpsertJSON(userID int, prefs string, now time.Time) error {
	res, err := r.db.Exec(
		"UPDATE user_preferences SET preferences = ?, updated_at = ? WHERE user_id = ?",
		prefs, now, userID,
	)
	if err != nil {
		return fmt.Errorf("update user preferences %d: %w", userID, err)
	}
	if affected, _ := res.RowsAffected(); affected > 0 {
		return nil
	}

	_, err = r.db.Exec(
		"INSERT INTO user_preferences (user_id, preferences, created_at, updated_at) VALUES (?, ?, ?, ?)",
		userID, prefs, now, now,
	)
	if err != nil {
		return fmt.Errorf("insert user preferences %d: %w", userID, err)
	}
	return nil
}
