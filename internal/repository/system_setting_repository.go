package repository

import (
	"database/sql"
	"fmt"

	"windshift/internal/database"
)

// SystemSettingRepository provides data access for the system_settings table.
type SystemSettingRepository struct {
	db database.Database
}

// NewSystemSettingRepository creates a new SystemSettingRepository.
func NewSystemSettingRepository(db database.Database) *SystemSettingRepository {
	return &SystemSettingRepository{db: db}
}

// GetValue returns the value for the given setting key. The second return
// value indicates whether the key exists and has a non-NULL value.
func (r *SystemSettingRepository) GetValue(key string) (value string, ok bool, err error) {
	var val sql.NullString
	err = r.db.QueryRow("SELECT value FROM system_settings WHERE key = ?", key).Scan(&val)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get system setting %q: %w", key, err)
	}
	if !val.Valid {
		return "", false, nil
	}
	return val.String, true, nil
}

// Upsert writes value for the given key, inserting a new row when the key
// doesn't already exist. valueType, description, and category are only used
// for a fresh insert.
func (r *SystemSettingRepository) Upsert(key, value, valueType, description, category string) error {
	result, err := r.db.ExecWrite(`
		UPDATE system_settings SET value = ?, updated_at = CURRENT_TIMESTAMP
		WHERE key = ?
	`, value, key)
	if err != nil {
		return fmt.Errorf("upsert system setting (update): %w", err)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		return nil
	}

	_, err = r.db.ExecWrite(`
		INSERT INTO system_settings (key, value, value_type, description, category, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, key, value, valueType, description, category)
	if err != nil {
		return fmt.Errorf("upsert system setting (insert): %w", err)
	}
	return nil
}
