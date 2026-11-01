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

// GetReminderSettings returns the per-user due-date reminder opt-in. A missing
// preferences row means the user has not opted in, so it returns the defaults.
func (r *UserPreferencesRepository) GetReminderSettings(userID int) (enabled bool, days int, err error) {
	days = 3
	err = r.db.QueryRow(
		"SELECT due_date_reminders_enabled, due_date_reminder_days FROM user_preferences WHERE user_id = ?",
		userID,
	).Scan(&enabled, &days)
	if errors.Is(err, sql.ErrNoRows) {
		return false, 3, nil
	}
	if err != nil {
		return false, 0, fmt.Errorf("get reminder settings %d: %w", userID, err)
	}
	return enabled, days, nil
}

// UpsertReminderSettings writes the per-user due-date reminder opt-in. The
// preferences JSON is left untouched.
func (r *UserPreferencesRepository) UpsertReminderSettings(userID int, enabled bool, days int, now time.Time) error {
	res, err := r.db.ExecWrite(
		"UPDATE user_preferences SET due_date_reminders_enabled = ?, due_date_reminder_days = ?, updated_at = ? WHERE user_id = ?",
		enabled, days, now, userID,
	)
	if err != nil {
		return fmt.Errorf("update reminder settings %d: %w", userID, err)
	}
	if affected, _ := res.RowsAffected(); affected > 0 {
		return nil
	}
	_, err = r.db.ExecWrite(
		"INSERT INTO user_preferences (user_id, preferences, due_date_reminders_enabled, due_date_reminder_days, created_at, updated_at) VALUES (?, '{}', ?, ?, ?, ?)",
		userID, enabled, days, now, now,
	)
	if err != nil {
		return fmt.Errorf("insert reminder settings %d: %w", userID, err)
	}
	return nil
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
	res, err := r.db.ExecWrite(
		"UPDATE user_preferences SET preferences = ?, updated_at = ? WHERE user_id = ?",
		prefs, now, userID,
	)
	if err != nil {
		return fmt.Errorf("update user preferences %d: %w", userID, err)
	}
	if affected, _ := res.RowsAffected(); affected > 0 {
		return nil
	}

	_, err = r.db.ExecWrite(
		"INSERT INTO user_preferences (user_id, preferences, created_at, updated_at) VALUES (?, ?, ?, ?)",
		userID, prefs, now, now,
	)
	if err != nil {
		return fmt.Errorf("insert user preferences %d: %w", userID, err)
	}
	return nil
}
