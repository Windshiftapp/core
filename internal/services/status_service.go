package services

import (
	"database/sql"
	"fmt"
	"time"

	"windshift/internal/database"
)

// StatusService encapsulates business logic for statuses and status categories.
type StatusService struct {
	db database.Database
}

// NewStatusService creates a new StatusService.
func NewStatusService(db database.Database) *StatusService {
	return &StatusService{db: db}
}

// StatusResult represents a status with category details.
type StatusResult struct {
	ID            int
	Name          string
	Description   string
	CategoryID    int
	CategoryName  string
	CategoryColor string
	IsDefault     bool
	IsCompleted   bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// ListStatuses retrieves all statuses with their category details.
func (s *StatusService) ListStatuses() ([]StatusResult, error) {
	rows, err := s.db.Query(`
		SELECT s.id, s.name, s.description, s.category_id, s.is_default,
		       sc.name as category_name, sc.color as category_color, sc.is_completed,
		       s.created_at, s.updated_at
		FROM statuses s
		JOIN status_categories sc ON s.category_id = sc.id
		ORDER BY sc.id, s.name
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list statuses: %w", err)
	}
	defer rows.Close()

	var statuses []StatusResult
	for rows.Next() {
		var s StatusResult
		var description sql.NullString
		err := rows.Scan(&s.ID, &s.Name, &description, &s.CategoryID, &s.IsDefault,
			&s.CategoryName, &s.CategoryColor, &s.IsCompleted,
			&s.CreatedAt, &s.UpdatedAt)
		if err != nil {
			continue
		}
		s.Description = description.String
		statuses = append(statuses, s)
	}

	if statuses == nil {
		statuses = []StatusResult{}
	}

	return statuses, nil
}

// GetStatus retrieves a status by ID.
func (s *StatusService) GetStatus(id int) (*StatusResult, error) {
	var status StatusResult
	var description sql.NullString
	err := s.db.QueryRow(`
		SELECT s.id, s.name, s.description, s.category_id, s.is_default,
		       sc.name as category_name, sc.color as category_color, sc.is_completed,
		       s.created_at, s.updated_at
		FROM statuses s
		JOIN status_categories sc ON s.category_id = sc.id
		WHERE s.id = ?
	`, id).Scan(&status.ID, &status.Name, &description, &status.CategoryID, &status.IsDefault,
		&status.CategoryName, &status.CategoryColor, &status.IsCompleted,
		&status.CreatedAt, &status.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("status not found: %d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get status: %w", err)
	}

	status.Description = description.String
	return &status, nil
}

// StatusCategoryResult represents a status category.
type StatusCategoryResult struct {
	ID          int
	Name        string
	Color       string
	Description string
	IsDefault   bool
	IsCompleted bool
}

// ListCategories retrieves all status categories.
func (s *StatusService) ListCategories() ([]StatusCategoryResult, error) {
	rows, err := s.db.Query(`
		SELECT id, name, color, description, is_default, is_completed
		FROM status_categories
		ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list categories: %w", err)
	}
	defer rows.Close()

	var categories []StatusCategoryResult
	for rows.Next() {
		var c StatusCategoryResult
		var description sql.NullString
		err := rows.Scan(&c.ID, &c.Name, &c.Color, &description, &c.IsDefault, &c.IsCompleted)
		if err != nil {
			continue
		}
		c.Description = description.String
		categories = append(categories, c)
	}

	if categories == nil {
		categories = []StatusCategoryResult{}
	}

	return categories, nil
}

// GetCategory retrieves a status category by ID.
func (s *StatusService) GetCategory(id int) (*StatusCategoryResult, error) {
	var c StatusCategoryResult
	var description sql.NullString
	err := s.db.QueryRow(`
		SELECT id, name, color, description, is_default, is_completed
		FROM status_categories WHERE id = ?
	`, id).Scan(&c.ID, &c.Name, &c.Color, &description, &c.IsDefault, &c.IsCompleted)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("category not found: %d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get category: %w", err)
	}

	c.Description = description.String
	return &c, nil
}
