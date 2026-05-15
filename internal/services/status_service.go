package services

import (
	"database/sql"
	"errors"
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate statuses: %w", err)
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

	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("status not found: %d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get status: %w", err)
	}

	status.Description = description.String
	return &status, nil
}

// GetTerminalStatuses returns all statuses reachable in the given workflow
// whose category is marked as completed (i.e. the "this status closes the
// item" set). A status is considered part of the workflow if it appears as
// either the source or destination of any workflow_transitions row.
//
// Returned in stable order (status name) so callers picking the "first
// terminal" as a fallback get a deterministic choice.
func (s *StatusService) GetTerminalStatuses(workflowID int) ([]StatusResult, error) {
	rows, err := s.db.Query(`
		SELECT s.id, s.name, s.description, s.category_id, s.is_default,
		       sc.name as category_name, sc.color as category_color, sc.is_completed,
		       s.created_at, s.updated_at
		FROM statuses s
		JOIN status_categories sc ON s.category_id = sc.id
		WHERE sc.is_completed = TRUE
		  AND s.id IN (
		      SELECT to_status_id FROM workflow_transitions WHERE workflow_id = ?
		      UNION
		      SELECT from_status_id FROM workflow_transitions
		      WHERE workflow_id = ? AND from_status_id IS NOT NULL
		  )
		ORDER BY s.name
	`, workflowID, workflowID)
	if err != nil {
		return nil, fmt.Errorf("failed to list terminal statuses: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var statuses []StatusResult
	for rows.Next() {
		var st StatusResult
		var description sql.NullString
		if err := rows.Scan(&st.ID, &st.Name, &description, &st.CategoryID, &st.IsDefault,
			&st.CategoryName, &st.CategoryColor, &st.IsCompleted,
			&st.CreatedAt, &st.UpdatedAt); err != nil {
			continue
		}
		st.Description = description.String
		statuses = append(statuses, st)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate terminal statuses: %w", err)
	}
	if statuses == nil {
		statuses = []StatusResult{}
	}
	return statuses, nil
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate categories: %w", err)
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

	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("category not found: %d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get category: %w", err)
	}

	c.Description = description.String
	return &c, nil
}
