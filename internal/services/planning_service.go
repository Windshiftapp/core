package services

import (
	"database/sql"
	"fmt"
	"time"

	"windshift/internal/database"
)

// PlanningService encapsulates business logic for milestones, iterations, and projects.
type PlanningService struct {
	db database.Database
}

// NewPlanningService creates a new PlanningService.
func NewPlanningService(db database.Database) *PlanningService {
	return &PlanningService{db: db}
}

// ========================================
// Milestones
// ========================================

// MilestoneResult represents a milestone with category details.
type MilestoneResult struct {
	ID            int
	Name          string
	Description   string
	TargetDate    string
	Status        string
	CategoryID    *int
	CategoryName  string
	CategoryColor string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// MilestoneListParams contains parameters for listing milestones.
type MilestoneListParams struct {
	Limit  int
	Offset int
}

// ListMilestones retrieves milestones with pagination.
func (s *PlanningService) ListMilestones(params MilestoneListParams) ([]MilestoneResult, int, error) {
	rows, err := s.db.Query(`
		SELECT m.id, m.name, m.description, m.target_date, m.status, m.category_id,
		       mc.name as category_name, mc.color as category_color,
		       m.created_at, m.updated_at
		FROM milestones m
		LEFT JOIN milestone_categories mc ON m.category_id = mc.id
		ORDER BY m.target_date, m.name
		LIMIT ? OFFSET ?
	`, params.Limit, params.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list milestones: %w", err)
	}
	defer rows.Close()

	var milestones []MilestoneResult
	for rows.Next() {
		var m MilestoneResult
		var description, targetDate, categoryName, categoryColor sql.NullString
		var categoryID sql.NullInt64
		err := rows.Scan(&m.ID, &m.Name, &description, &targetDate, &m.Status, &categoryID,
			&categoryName, &categoryColor, &m.CreatedAt, &m.UpdatedAt)
		if err != nil {
			continue
		}
		m.Description = description.String
		m.TargetDate = targetDate.String
		m.CategoryName = categoryName.String
		m.CategoryColor = categoryColor.String
		if categoryID.Valid {
			id := int(categoryID.Int64)
			m.CategoryID = &id
		}
		milestones = append(milestones, m)
	}

	if milestones == nil {
		milestones = []MilestoneResult{}
	}

	var total int
	s.db.QueryRow("SELECT COUNT(*) FROM milestones").Scan(&total)

	return milestones, total, nil
}

// GetMilestone retrieves a milestone by ID.
func (s *PlanningService) GetMilestone(id int) (*MilestoneResult, error) {
	var m MilestoneResult
	var description, targetDate, categoryName, categoryColor sql.NullString
	var categoryID sql.NullInt64
	err := s.db.QueryRow(`
		SELECT m.id, m.name, m.description, m.target_date, m.status, m.category_id,
		       mc.name as category_name, mc.color as category_color,
		       m.created_at, m.updated_at
		FROM milestones m
		LEFT JOIN milestone_categories mc ON m.category_id = mc.id
		WHERE m.id = ?
	`, id).Scan(&m.ID, &m.Name, &description, &targetDate, &m.Status, &categoryID,
		&categoryName, &categoryColor, &m.CreatedAt, &m.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("milestone not found: %d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get milestone: %w", err)
	}

	m.Description = description.String
	m.TargetDate = targetDate.String
	m.CategoryName = categoryName.String
	m.CategoryColor = categoryColor.String
	if categoryID.Valid {
		cid := int(categoryID.Int64)
		m.CategoryID = &cid
	}

	return &m, nil
}

// CreateMilestoneParams contains parameters for creating a milestone.
type CreateMilestoneParams struct {
	Name        string
	Description string
	TargetDate  string
	Status      string
	CategoryID  *int
}

// CreateMilestone creates a new milestone.
func (s *PlanningService) CreateMilestone(params CreateMilestoneParams) (*MilestoneResult, error) {
	status := params.Status
	if status == "" {
		status = "planning"
	}

	result, err := s.db.ExecWrite(`
		INSERT INTO milestones (name, description, target_date, status, category_id)
		VALUES (?, ?, ?, ?, ?)
	`, params.Name, params.Description, params.TargetDate, status, params.CategoryID)
	if err != nil {
		return nil, fmt.Errorf("failed to create milestone: %w", err)
	}

	id, _ := result.LastInsertId()
	return s.GetMilestone(int(id))
}

// UpdateMilestoneParams contains parameters for updating a milestone.
type UpdateMilestoneParams struct {
	ID          int
	Name        string
	Description string
	TargetDate  string
	Status      string
	CategoryID  *int
}

// UpdateMilestone updates an existing milestone.
func (s *PlanningService) UpdateMilestone(params UpdateMilestoneParams) (*MilestoneResult, error) {
	_, err := s.db.ExecWrite(`
		UPDATE milestones SET name = ?, description = ?, target_date = ?, status = ?, category_id = ?,
		       updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, params.Name, params.Description, params.TargetDate, params.Status, params.CategoryID, params.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to update milestone: %w", err)
	}

	return s.GetMilestone(params.ID)
}

// DeleteMilestone deletes a milestone.
func (s *PlanningService) DeleteMilestone(id int) error {
	_, err := s.db.ExecWrite("DELETE FROM milestones WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete milestone: %w", err)
	}
	return nil
}

// ========================================
// Iterations
// ========================================

// IterationResult represents an iteration with type details.
type IterationResult struct {
	ID          int
	Name        string
	Description string
	StartDate   string
	EndDate     string
	Status      string
	TypeID      *int
	TypeName    string
	TypeColor   string
	IsGlobal    bool
	WorkspaceID *int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// IterationListParams contains parameters for listing iterations.
type IterationListParams struct {
	Limit  int
	Offset int
}

// ListIterations retrieves iterations with pagination.
func (s *PlanningService) ListIterations(params IterationListParams) ([]IterationResult, int, error) {
	rows, err := s.db.Query(`
		SELECT i.id, i.name, i.description, i.start_date, i.end_date, i.status,
		       i.type_id, it.name as type_name, it.color as type_color,
		       i.is_global, i.workspace_id, i.created_at, i.updated_at
		FROM iterations i
		LEFT JOIN iteration_types it ON i.type_id = it.id
		ORDER BY i.start_date DESC
		LIMIT ? OFFSET ?
	`, params.Limit, params.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list iterations: %w", err)
	}
	defer rows.Close()

	var iterations []IterationResult
	for rows.Next() {
		var iter IterationResult
		var description, typeName, typeColor sql.NullString
		var typeID, workspaceID sql.NullInt64
		err := rows.Scan(&iter.ID, &iter.Name, &description, &iter.StartDate, &iter.EndDate, &iter.Status,
			&typeID, &typeName, &typeColor, &iter.IsGlobal, &workspaceID, &iter.CreatedAt, &iter.UpdatedAt)
		if err != nil {
			continue
		}
		iter.Description = description.String
		iter.TypeName = typeName.String
		iter.TypeColor = typeColor.String
		if typeID.Valid {
			id := int(typeID.Int64)
			iter.TypeID = &id
		}
		if workspaceID.Valid {
			id := int(workspaceID.Int64)
			iter.WorkspaceID = &id
		}
		iterations = append(iterations, iter)
	}

	if iterations == nil {
		iterations = []IterationResult{}
	}

	var total int
	s.db.QueryRow("SELECT COUNT(*) FROM iterations").Scan(&total)

	return iterations, total, nil
}

// GetIteration retrieves an iteration by ID.
func (s *PlanningService) GetIteration(id int) (*IterationResult, error) {
	var iter IterationResult
	var description, typeName, typeColor sql.NullString
	var typeID, workspaceID sql.NullInt64
	err := s.db.QueryRow(`
		SELECT i.id, i.name, i.description, i.start_date, i.end_date, i.status,
		       i.type_id, it.name as type_name, it.color as type_color,
		       i.is_global, i.workspace_id, i.created_at, i.updated_at
		FROM iterations i
		LEFT JOIN iteration_types it ON i.type_id = it.id
		WHERE i.id = ?
	`, id).Scan(&iter.ID, &iter.Name, &description, &iter.StartDate, &iter.EndDate, &iter.Status,
		&typeID, &typeName, &typeColor, &iter.IsGlobal, &workspaceID, &iter.CreatedAt, &iter.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("iteration not found: %d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get iteration: %w", err)
	}

	iter.Description = description.String
	iter.TypeName = typeName.String
	iter.TypeColor = typeColor.String
	if typeID.Valid {
		tid := int(typeID.Int64)
		iter.TypeID = &tid
	}
	if workspaceID.Valid {
		wid := int(workspaceID.Int64)
		iter.WorkspaceID = &wid
	}

	return &iter, nil
}

// IsIterationGlobal checks if an iteration is global.
func (s *PlanningService) IsIterationGlobal(id int) (bool, error) {
	var isGlobal bool
	err := s.db.QueryRow("SELECT is_global FROM iterations WHERE id = ?", id).Scan(&isGlobal)
	if err == sql.ErrNoRows {
		return false, fmt.Errorf("iteration not found: %d", id)
	}
	if err != nil {
		return false, fmt.Errorf("failed to check iteration: %w", err)
	}
	return isGlobal, nil
}

// CreateIterationParams contains parameters for creating an iteration.
type CreateIterationParams struct {
	Name        string
	Description string
	StartDate   string
	EndDate     string
	Status      string
	TypeID      *int
	IsGlobal    bool
	WorkspaceID *int
}

// CreateIteration creates a new iteration.
func (s *PlanningService) CreateIteration(params CreateIterationParams) (*IterationResult, error) {
	status := params.Status
	if status == "" {
		status = "planned"
	}

	result, err := s.db.ExecWrite(`
		INSERT INTO iterations (name, description, start_date, end_date, status, type_id, is_global, workspace_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, params.Name, params.Description, params.StartDate, params.EndDate, status, params.TypeID, params.IsGlobal, params.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to create iteration: %w", err)
	}

	id, _ := result.LastInsertId()
	return s.GetIteration(int(id))
}

// UpdateIterationParams contains parameters for updating an iteration.
type UpdateIterationParams struct {
	ID          int
	Name        string
	Description string
	StartDate   string
	EndDate     string
	Status      string
	TypeID      *int
	IsGlobal    bool
	WorkspaceID *int
}

// UpdateIteration updates an existing iteration.
func (s *PlanningService) UpdateIteration(params UpdateIterationParams) (*IterationResult, error) {
	_, err := s.db.ExecWrite(`
		UPDATE iterations SET name = ?, description = ?, start_date = ?, end_date = ?,
		       status = ?, type_id = ?, is_global = ?, workspace_id = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, params.Name, params.Description, params.StartDate, params.EndDate, params.Status, params.TypeID, params.IsGlobal, params.WorkspaceID, params.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to update iteration: %w", err)
	}

	return s.GetIteration(params.ID)
}

// DeleteIteration deletes an iteration.
func (s *PlanningService) DeleteIteration(id int) error {
	_, err := s.db.ExecWrite("DELETE FROM iterations WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete iteration: %w", err)
	}
	return nil
}

// ========================================
// Projects
// ========================================

// ProjectResult represents a project with workspace details.
type ProjectResult struct {
	ID            int
	Name          string
	Description   string
	Active        bool
	WorkspaceID   *int
	WorkspaceName string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// ProjectListParams contains parameters for listing projects.
type ProjectListParams struct {
	Limit  int
	Offset int
}

// ListProjects retrieves projects with pagination.
func (s *PlanningService) ListProjects(params ProjectListParams) ([]ProjectResult, int, error) {
	rows, err := s.db.Query(`
		SELECT p.id, p.name, p.description, p.active, p.workspace_id,
		       w.name as workspace_name, p.created_at, p.updated_at
		FROM projects p
		LEFT JOIN workspaces w ON p.workspace_id = w.id
		ORDER BY p.name
		LIMIT ? OFFSET ?
	`, params.Limit, params.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list projects: %w", err)
	}
	defer rows.Close()

	var projects []ProjectResult
	for rows.Next() {
		var p ProjectResult
		var description, workspaceName sql.NullString
		var workspaceID sql.NullInt64
		err := rows.Scan(&p.ID, &p.Name, &description, &p.Active, &workspaceID, &workspaceName, &p.CreatedAt, &p.UpdatedAt)
		if err != nil {
			continue
		}
		p.Description = description.String
		p.WorkspaceName = workspaceName.String
		if workspaceID.Valid {
			id := int(workspaceID.Int64)
			p.WorkspaceID = &id
		}
		projects = append(projects, p)
	}

	if projects == nil {
		projects = []ProjectResult{}
	}

	var total int
	s.db.QueryRow("SELECT COUNT(*) FROM projects").Scan(&total)

	return projects, total, nil
}

// GetProject retrieves a project by ID.
func (s *PlanningService) GetProject(id int) (*ProjectResult, error) {
	var p ProjectResult
	var description, workspaceName sql.NullString
	var workspaceID sql.NullInt64
	err := s.db.QueryRow(`
		SELECT p.id, p.name, p.description, p.active, p.workspace_id,
		       w.name as workspace_name, p.created_at, p.updated_at
		FROM projects p
		LEFT JOIN workspaces w ON p.workspace_id = w.id
		WHERE p.id = ?
	`, id).Scan(&p.ID, &p.Name, &description, &p.Active, &workspaceID, &workspaceName, &p.CreatedAt, &p.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("project not found: %d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}

	p.Description = description.String
	p.WorkspaceName = workspaceName.String
	if workspaceID.Valid {
		wid := int(workspaceID.Int64)
		p.WorkspaceID = &wid
	}

	return &p, nil
}

// CreateProjectParams contains parameters for creating a project.
type CreateProjectParams struct {
	Name        string
	Description string
	WorkspaceID *int
	Active      bool
}

// CreateProject creates a new project.
func (s *PlanningService) CreateProject(params CreateProjectParams) (*ProjectResult, error) {
	result, err := s.db.ExecWrite(`
		INSERT INTO projects (name, description, workspace_id, active)
		VALUES (?, ?, ?, ?)
	`, params.Name, params.Description, params.WorkspaceID, params.Active)
	if err != nil {
		return nil, fmt.Errorf("failed to create project: %w", err)
	}

	id, _ := result.LastInsertId()
	return s.GetProject(int(id))
}

// UpdateProjectParams contains parameters for updating a project.
type UpdateProjectParams struct {
	ID          int
	Name        string
	Description string
	WorkspaceID *int
	Active      bool
}

// UpdateProject updates an existing project.
func (s *PlanningService) UpdateProject(params UpdateProjectParams) (*ProjectResult, error) {
	_, err := s.db.ExecWrite(`
		UPDATE projects SET name = ?, description = ?, workspace_id = ?, active = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, params.Name, params.Description, params.WorkspaceID, params.Active, params.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to update project: %w", err)
	}

	return s.GetProject(params.ID)
}

// DeleteProject deletes a project.
func (s *PlanningService) DeleteProject(id int) error {
	_, err := s.db.ExecWrite("DELETE FROM projects WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}
	return nil
}
