package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// ThemeRepository provides data access for application themes.
type ThemeRepository struct {
	db database.Database
}

// NewThemeRepository creates a new ThemeRepository.
func NewThemeRepository(db database.Database) *ThemeRepository {
	return &ThemeRepository{db: db}
}

const themeColumns = `id, name, description, is_default, is_active,
	nav_background_color_light, nav_text_color_light,
	nav_background_color_dark, nav_text_color_dark,
	created_at, updated_at`

type themeScanner interface {
	Scan(dest ...interface{}) error
}

func scanTheme(s themeScanner, t *models.Theme) error {
	return s.Scan(
		&t.ID, &t.Name, &t.Description,
		&t.IsDefault, &t.IsActive,
		&t.NavBackgroundColorLight, &t.NavTextColorLight,
		&t.NavBackgroundColorDark, &t.NavTextColorDark,
		&t.CreatedAt, &t.UpdatedAt,
	)
}

// List returns all themes ordered as the API expects.
func (r *ThemeRepository) List() ([]models.Theme, error) {
	rows, err := r.db.Query(`SELECT ` + themeColumns + ` FROM themes ORDER BY is_default DESC, name ASC`)
	if err != nil {
		return nil, fmt.Errorf("query themes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	themes := make([]models.Theme, 0)
	for rows.Next() {
		var theme models.Theme
		if err := scanTheme(rows, &theme); err != nil {
			return nil, fmt.Errorf("scan theme: %w", err)
		}
		themes = append(themes, theme)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate themes: %w", err)
	}
	return themes, nil
}

// GetByID returns a theme by id.
func (r *ThemeRepository) GetByID(id int) (models.Theme, error) {
	var theme models.Theme
	err := scanTheme(r.db.QueryRow(`SELECT `+themeColumns+` FROM themes WHERE id = ?`, id), &theme)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Theme{}, ErrNotFound
	}
	if err != nil {
		return models.Theme{}, fmt.Errorf("get theme %d: %w", id, err)
	}
	return theme, nil
}

// GetActive returns the currently active theme.
func (r *ThemeRepository) GetActive() (models.Theme, error) {
	var theme models.Theme
	err := scanTheme(r.db.QueryRow(`SELECT `+themeColumns+` FROM themes WHERE is_active = true ORDER BY is_default DESC LIMIT 1`), &theme)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Theme{}, ErrNotFound
	}
	if err != nil {
		return models.Theme{}, fmt.Errorf("get active theme: %w", err)
	}
	return theme, nil
}

// Create inserts a new theme and returns its id.
func (r *ThemeRepository) Create(req models.ThemeCreateRequest, now time.Time) (int, error) {
	query := `
		INSERT INTO themes (name, description, nav_background_color_light, nav_text_color_light, nav_background_color_dark, nav_text_color_dark, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`
	var id int
	if err := r.db.QueryRow(query, req.Name, req.Description, req.NavBackgroundColorLight, req.NavTextColorLight, req.NavBackgroundColorDark, req.NavTextColorDark, now, now).Scan(&id); err != nil {
		return 0, fmt.Errorf("create theme: %w", err)
	}
	return id, nil
}

// Update modifies a theme.
func (r *ThemeRepository) Update(id int, req models.ThemeUpdateRequest, now time.Time) error {
	query := `
		UPDATE themes
		SET name = ?, description = ?, nav_background_color_light = ?, nav_text_color_light = ?,
		    nav_background_color_dark = ?, nav_text_color_dark = ?, is_active = ?, updated_at = ?
		WHERE id = ?
	`
	res, err := r.db.Exec(query, req.Name, req.Description, req.NavBackgroundColorLight, req.NavTextColorLight, req.NavBackgroundColorDark, req.NavTextColorDark, req.IsActive, now, id)
	if err != nil {
		return fmt.Errorf("update theme %d: %w", id, err)
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
}

// DeactivateAllExcept deactivates every theme except id.
func (r *ThemeRepository) DeactivateAllExcept(id int) error {
	if _, err := r.db.Exec("UPDATE themes SET is_active = false WHERE id != ?", id); err != nil {
		return fmt.Errorf("deactivate other themes: %w", err)
	}
	return nil
}

// Delete removes a theme.
func (r *ThemeRepository) Delete(id int) error {
	res, err := r.db.Exec("DELETE FROM themes WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete theme %d: %w", id, err)
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
}

// DeactivateAll deactivates every theme.
func (r *ThemeRepository) DeactivateAll() error {
	if _, err := r.db.Exec("UPDATE themes SET is_active = false"); err != nil {
		return fmt.Errorf("deactivate themes: %w", err)
	}
	return nil
}

// Activate marks a theme active.
func (r *ThemeRepository) Activate(id int, now time.Time) error {
	res, err := r.db.Exec("UPDATE themes SET is_active = true, updated_at = ? WHERE id = ?", now, id)
	if err != nil {
		return fmt.Errorf("activate theme %d: %w", id, err)
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
}
