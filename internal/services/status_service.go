package services

import (
	"database/sql"
	"fmt"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// StatusService encapsulates business logic for statuses and status categories.
type StatusService struct {
	db         database.Database
	statuses   *EnumService
	categories *EnumService
}

// NewStatusService creates a new StatusService.
func NewStatusService(db database.Database) *StatusService {
	return &StatusService{
		db:         db,
		statuses:   NewEnumService(db, NewStatusConfig()),
		categories: NewEnumService(db, NewStatusCategoryConfig()),
	}
}

// StatusResult represents a status with category details.
type StatusResult struct {
	ID                  int
	BuiltinKey          string
	Name                string
	Description         string
	CategoryID          int
	CategoryName        string
	CategoryBuiltinKey  string
	CategoryDescription string
	CategoryColor       string
	CategoryIsDefault   bool
	IsDefault           bool
	IsCompleted         bool
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// ListStatuses retrieves all statuses with their category details.
func (s *StatusService) ListStatuses() ([]StatusResult, error) {
	entities, err := s.statuses.GetAll()
	if err != nil {
		return nil, fmt.Errorf("list statuses: %w", err)
	}
	statuses := make([]StatusResult, 0, len(entities))
	for _, entity := range entities {
		status, ok := entity.(*models.Status)
		if !ok {
			return nil, fmt.Errorf("list statuses: unexpected entity %T", entity)
		}
		statuses = append(statuses, statusResult(*status))
	}
	return statuses, nil
}

// GetStatus retrieves a status by ID.
func (s *StatusService) GetStatus(id int) (*StatusResult, error) {
	entity, err := s.statuses.GetByID(id)
	if err != nil {
		return nil, fmt.Errorf("get status %d: %w", id, err)
	}
	status, ok := entity.(*models.Status)
	if !ok {
		return nil, fmt.Errorf("get status: unexpected entity %T", entity)
	}
	result := statusResult(*status)
	return &result, nil
}

func statusResult(status models.Status) StatusResult {
	return StatusResult{
		ID: status.ID, BuiltinKey: status.BuiltinKey, Name: status.Name,
		Description: status.Description, CategoryID: status.CategoryID,
		CategoryName: status.CategoryName, CategoryBuiltinKey: status.CategoryBuiltinKey,
		CategoryDescription: status.CategoryDescription, CategoryColor: status.CategoryColor,
		CategoryIsDefault: status.CategoryIsDefault, IsDefault: status.IsDefault,
		IsCompleted: status.IsCompleted, CreatedAt: status.CreatedAt, UpdatedAt: status.UpdatedAt,
	}
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
		SELECT s.id, COALESCE(s.builtin_key, ''), s.name, s.description, s.category_id, s.is_default,
		       sc.name as category_name, COALESCE(sc.builtin_key, ''), sc.description as category_description,
		       sc.color as category_color, sc.is_default as category_is_default, sc.is_completed,
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
		if err := rows.Scan(&st.ID, &st.BuiltinKey, &st.Name, &description, &st.CategoryID, &st.IsDefault,
			&st.CategoryName, &st.CategoryBuiltinKey, &st.CategoryDescription,
			&st.CategoryColor, &st.CategoryIsDefault, &st.IsCompleted,
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

// ListWorkflowStatuses returns statuses that participate in a workflow.
func (s *StatusService) ListWorkflowStatuses(workflowID int) ([]StatusResult, error) {
	rows, err := s.db.Query(`
		SELECT DISTINCT s.id, COALESCE(s.builtin_key, ''), s.name, s.description, s.category_id, s.is_default, s.created_at, s.updated_at,
		       sc.name as category_name, COALESCE(sc.builtin_key, ''), sc.description as category_description,
		       sc.color as category_color, sc.is_default as category_is_default, sc.is_completed
		FROM workflow_transitions wt
		JOIN statuses s ON s.id = wt.to_status_id OR (wt.from_status_id IS NOT NULL AND s.id = wt.from_status_id)
		LEFT JOIN status_categories sc ON s.category_id = sc.id
		WHERE wt.workflow_id = ?
		ORDER BY s.id
	`, workflowID)
	if err != nil {
		return nil, fmt.Errorf("failed to list workflow statuses: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var statuses []StatusResult
	for rows.Next() {
		var st StatusResult
		var categoryName, categoryDescription, categoryColor sql.NullString
		var isCompleted sql.NullBool
		if err := rows.Scan(
			&st.ID, &st.BuiltinKey, &st.Name, &st.Description, &st.CategoryID,
			&st.IsDefault, &st.CreatedAt, &st.UpdatedAt,
			&categoryName, &st.CategoryBuiltinKey, &categoryDescription,
			&categoryColor, &st.CategoryIsDefault, &isCompleted,
		); err != nil {
			return nil, fmt.Errorf("scan workflow status: %w", err)
		}
		st.CategoryName = categoryName.String
		st.CategoryDescription = categoryDescription.String
		st.CategoryColor = categoryColor.String
		st.IsCompleted = isCompleted.Bool
		statuses = append(statuses, st)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workflow statuses: %w", err)
	}
	if statuses == nil {
		statuses = []StatusResult{}
	}
	return statuses, nil
}

// StatusCategoryResult represents a status category.
type StatusCategoryResult struct {
	ID          int
	BuiltinKey  string
	Name        string
	Color       string
	Description string
	IsDefault   bool
	IsCompleted bool
}

// ListCategories retrieves all status categories.
func (s *StatusService) ListCategories() ([]StatusCategoryResult, error) {
	entities, err := s.categories.GetAll()
	if err != nil {
		return nil, fmt.Errorf("list status categories: %w", err)
	}
	categories := make([]StatusCategoryResult, 0, len(entities))
	for _, entity := range entities {
		category, ok := entity.(*models.StatusCategory)
		if !ok {
			return nil, fmt.Errorf("list status categories: unexpected entity %T", entity)
		}
		categories = append(categories, statusCategoryResult(*category))
	}
	return categories, nil
}

// GetCategory retrieves a status category by ID.
func (s *StatusService) GetCategory(id int) (*StatusCategoryResult, error) {
	entity, err := s.categories.GetByID(id)
	if err != nil {
		return nil, fmt.Errorf("get status category %d: %w", id, err)
	}
	category, ok := entity.(*models.StatusCategory)
	if !ok {
		return nil, fmt.Errorf("get status category: unexpected entity %T", entity)
	}
	result := statusCategoryResult(*category)
	return &result, nil
}

func statusCategoryResult(category models.StatusCategory) StatusCategoryResult {
	return StatusCategoryResult{
		ID: category.ID, BuiltinKey: category.BuiltinKey, Name: category.Name,
		Color: category.Color, Description: category.Description,
		IsDefault: category.IsDefault, IsCompleted: category.IsCompleted,
	}
}
