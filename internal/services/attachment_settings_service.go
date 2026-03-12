package services

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"windshift/internal/database"
	"windshift/internal/models"
)

// AttachmentSettingsService encapsulates business logic for attachment settings.
type AttachmentSettingsService struct {
	db database.Database
}

// NewAttachmentSettingsService creates a new AttachmentSettingsService.
func NewAttachmentSettingsService(db database.Database) *AttachmentSettingsService {
	return &AttachmentSettingsService{db: db}
}

// Initialize creates initial attachment settings in the database or updates the path if changed.
func (s *AttachmentSettingsService) Initialize(attachmentPath string) error {
	// Check if settings already exist
	var exists bool
	err := s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM attachment_settings)").Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check attachment settings existence: %w", err)
	}

	if !exists {
		// Create initial settings
		_, err = s.db.Exec(`
			INSERT INTO attachment_settings (max_file_size, allowed_mime_types, attachment_path, enabled)
			VALUES (52428800, '[]', ?, true)
		`, attachmentPath)
		if err != nil {
			return fmt.Errorf("failed to create initial attachment settings: %w", err)
		}
		return nil
	}

	// Update attachment path if it has changed
	_, err = s.db.Exec(`
		UPDATE attachment_settings
		SET attachment_path = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = (SELECT MIN(id) FROM attachment_settings)
	`, attachmentPath)
	if err != nil {
		return fmt.Errorf("failed to update attachment path: %w", err)
	}
	return nil
}

// Get retrieves the current attachment settings.
func (s *AttachmentSettingsService) Get() (*models.AttachmentSettings, error) {
	settings := &models.AttachmentSettings{
		ID:               1,
		MaxFileSize:      52428800, // 50MB default
		AllowedMimeTypes: "[]",     // Empty array by default (all types allowed)
		AttachmentPath:   "",
		Enabled:          false, // Disabled by default if no path is set
	}

	err := s.db.QueryRow(`
		SELECT id, max_file_size, allowed_mime_types, attachment_path, enabled, created_at, updated_at
		FROM attachment_settings ORDER BY id DESC LIMIT 1
	`).Scan(
		&settings.ID, &settings.MaxFileSize, &settings.AllowedMimeTypes,
		&settings.AttachmentPath, &settings.Enabled, &settings.CreatedAt, &settings.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		// No settings in database, return defaults
		return settings, nil
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get attachment settings: %w", err)
	}

	return settings, nil
}

// Update modifies attachment settings by ID.
func (s *AttachmentSettingsService) Update(settingsID int, req *models.AttachmentSettingsRequest) (*models.AttachmentSettings, error) {
	// Validate max file size
	if req.MaxFileSize <= 0 {
		return nil, fmt.Errorf("max file size must be greater than 0")
	}

	// Convert allowed MIME types to JSON string
	allowedMimeTypesJSON, err := json.Marshal(req.AllowedMimeTypes)
	if err != nil {
		return nil, fmt.Errorf("invalid allowed MIME types: %w", err)
	}

	// Check if settings record exists
	var exists bool
	err = s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM attachment_settings WHERE id = ?)", settingsID).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("failed to check settings existence: %w", err)
	}

	if exists {
		// Update existing settings
		_, err = s.db.Exec(`
			UPDATE attachment_settings
			SET max_file_size = ?, allowed_mime_types = ?, enabled = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, req.MaxFileSize, string(allowedMimeTypesJSON), req.Enabled, settingsID)
	} else {
		// Create new settings record
		_, err = s.db.Exec(`
			INSERT INTO attachment_settings (id, max_file_size, allowed_mime_types, attachment_path, enabled)
			VALUES (?, ?, ?, ?, ?)
		`, settingsID, req.MaxFileSize, string(allowedMimeTypesJSON), "", req.Enabled)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to save settings: %w", err)
	}

	return s.Get()
}

// UpdatePath updates just the attachment path.
func (s *AttachmentSettingsService) UpdatePath(newPath string) error {
	_, err := s.db.Exec(`
		UPDATE attachment_settings
		SET attachment_path = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = (SELECT MIN(id) FROM attachment_settings)
	`, newPath)
	if err != nil {
		return fmt.Errorf("failed to update attachment path: %w", err)
	}
	return nil
}

// AttachmentStatus represents the status of the attachment system.
type AttachmentStatus struct {
	Enabled        bool   `json:"enabled"`
	AttachmentPath string `json:"attachment_path"`
	Writable       bool   `json:"writable"`
}

// GetStatus returns the attachment system status (enabled/disabled, path info).
func (s *AttachmentSettingsService) GetStatus() (*AttachmentStatus, error) {
	status := &AttachmentStatus{
		Enabled:        false,
		AttachmentPath: "",
		Writable:       false,
	}

	var enabled bool
	var attachmentPath string
	err := s.db.QueryRow(`
		SELECT enabled, attachment_path FROM attachment_settings ORDER BY id DESC LIMIT 1
	`).Scan(&enabled, &attachmentPath)

	if err == sql.ErrNoRows {
		// No settings in database
		return status, nil
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get attachment status: %w", err)
	}

	status.Enabled = enabled
	status.AttachmentPath = attachmentPath

	// Check if path is writable (if it exists)
	if attachmentPath != "" {
		info, err := os.Stat(attachmentPath)
		if err == nil && info.IsDir() {
			// Try to create a test file to verify write permissions
			testFile := filepath.Join(attachmentPath, ".write-test")
			if f, err := os.Create(testFile); err == nil { //nolint:gosec // G304 — testFile from controlled filepath.Join
				_ = f.Close()
				_ = os.Remove(testFile)
				status.Writable = true
			}
		}
	}

	return status, nil
}
