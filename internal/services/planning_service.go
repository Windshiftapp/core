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

// MilestoneReleaseResult represents a release record for a milestone.
type MilestoneReleaseResult struct {
	ID              int
	MilestoneID     int
	TagName         string
	Name            string
	Body            string
	IsDraft         bool
	IsPrerelease    bool
	TargetCommitish string
	SCMConnectionID *int
	SCMRepository   *string
	SCMReleaseID    *string
	SCMReleaseURL   *string
	CreatedBy       *int
	CreatedAt       string
}

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
	IsGlobal      bool
	WorkspaceID   *int
	WorkspaceName string
	LatestRelease *MilestoneReleaseResult
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// MilestoneListParams contains parameters for listing milestones.
type MilestoneListParams struct {
	Limit         int
	Offset        int
	WorkspaceID   *int   // Filter by workspace
	CategoryID    *int   // Filter by category
	Status        string // Filter by status
	IncludeGlobal bool   // Include global milestones
}

// ListMilestones retrieves milestones with pagination and filtering.
func (s *PlanningService) ListMilestones(params MilestoneListParams) ([]MilestoneResult, int, error) {
	query := `
		SELECT m.id, m.name, m.description, m.target_date, m.status, m.category_id,
		       mc.name as category_name, mc.color as category_color,
		       m.is_global, m.workspace_id, w.name as workspace_name,
		       mr.id, mr.tag_name, mr.name, mr.body, mr.is_draft, mr.is_prerelease,
		       mr.target_commitish, mr.scm_connection_id, mr.scm_repository,
		       mr.scm_release_id, mr.scm_release_url, mr.created_by, mr.created_at,
		       m.created_at, m.updated_at
		FROM milestones m
		LEFT JOIN milestone_categories mc ON m.category_id = mc.id
		LEFT JOIN workspaces w ON m.workspace_id = w.id
		LEFT JOIN (
			SELECT * FROM milestone_releases
			WHERE id IN (
				SELECT MAX(id) FROM milestone_releases GROUP BY milestone_id
			)
		) mr ON mr.milestone_id = m.id
		WHERE 1=1`

	countQuery := "SELECT COUNT(*) FROM milestones m WHERE 1=1"
	var args []interface{}
	var countArgs []interface{}

	// Filter by workspace - show local milestones for this workspace + optionally global milestones
	if params.WorkspaceID != nil {
		if params.IncludeGlobal {
			query += " AND (m.workspace_id = ? OR m.is_global = ?)"
			countQuery += " AND (m.workspace_id = ? OR m.is_global = ?)"
			args = append(args, *params.WorkspaceID, true)
			countArgs = append(countArgs, *params.WorkspaceID, true)
		} else {
			query += " AND m.workspace_id = ?"
			countQuery += " AND m.workspace_id = ?"
			args = append(args, *params.WorkspaceID)
			countArgs = append(countArgs, *params.WorkspaceID)
		}
	} else if params.IncludeGlobal {
		// If no workspace specified but include_global, only show global milestones
		query += " AND m.is_global = ?"
		countQuery += " AND m.is_global = ?"
		args = append(args, true)
		countArgs = append(countArgs, true)
	}

	// Filter by category
	if params.CategoryID != nil {
		if *params.CategoryID == 0 {
			query += " AND m.category_id IS NULL"
			countQuery += " AND m.category_id IS NULL"
		} else {
			query += " AND m.category_id = ?"
			countQuery += " AND m.category_id = ?"
			args = append(args, *params.CategoryID)
			countArgs = append(countArgs, *params.CategoryID)
		}
	}

	// Filter by status
	if params.Status != "" {
		query += " AND m.status = ?"
		countQuery += " AND m.status = ?"
		args = append(args, params.Status)
		countArgs = append(countArgs, params.Status)
	}

	query += " ORDER BY m.target_date, m.name"
	query += " LIMIT ? OFFSET ?"
	args = append(args, params.Limit, params.Offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list milestones: %w", err)
	}
	defer rows.Close()

	var milestones []MilestoneResult
	for rows.Next() {
		var m MilestoneResult
		var description, targetDate, categoryName, categoryColor, workspaceName sql.NullString
		var categoryID, workspaceID sql.NullInt64
		// Release columns
		var mrID, mrCreatedBy, mrSCMConnectionID sql.NullInt64
		var mrTagName, mrName, mrBody, mrTargetCommitish sql.NullString
		var mrSCMRepository, mrSCMReleaseID, mrSCMReleaseURL sql.NullString
		var mrIsDraft, mrIsPrerelease sql.NullBool
		var mrCreatedAt sql.NullString
		err := rows.Scan(&m.ID, &m.Name, &description, &targetDate, &m.Status, &categoryID,
			&categoryName, &categoryColor, &m.IsGlobal, &workspaceID, &workspaceName,
			&mrID, &mrTagName, &mrName, &mrBody, &mrIsDraft, &mrIsPrerelease,
			&mrTargetCommitish, &mrSCMConnectionID, &mrSCMRepository,
			&mrSCMReleaseID, &mrSCMReleaseURL, &mrCreatedBy, &mrCreatedAt,
			&m.CreatedAt, &m.UpdatedAt)
		if err != nil {
			continue
		}
		m.Description = description.String
		m.TargetDate = targetDate.String
		m.CategoryName = categoryName.String
		m.CategoryColor = categoryColor.String
		m.WorkspaceName = workspaceName.String
		if categoryID.Valid {
			id := int(categoryID.Int64)
			m.CategoryID = &id
		}
		if workspaceID.Valid {
			id := int(workspaceID.Int64)
			m.WorkspaceID = &id
		}
		if mrID.Valid {
			rel := &MilestoneReleaseResult{
				ID:          int(mrID.Int64),
				MilestoneID: m.ID,
				TagName:     mrTagName.String,
				Name:        mrName.String,
				Body:        mrBody.String,
				CreatedAt:   mrCreatedAt.String,
			}
			if mrIsDraft.Valid {
				rel.IsDraft = mrIsDraft.Bool
			}
			if mrIsPrerelease.Valid {
				rel.IsPrerelease = mrIsPrerelease.Bool
			}
			rel.TargetCommitish = mrTargetCommitish.String
			if mrSCMConnectionID.Valid {
				cid := int(mrSCMConnectionID.Int64)
				rel.SCMConnectionID = &cid
			}
			if mrSCMRepository.Valid {
				rel.SCMRepository = &mrSCMRepository.String
			}
			if mrSCMReleaseID.Valid {
				rel.SCMReleaseID = &mrSCMReleaseID.String
			}
			if mrSCMReleaseURL.Valid {
				rel.SCMReleaseURL = &mrSCMReleaseURL.String
			}
			if mrCreatedBy.Valid {
				cb := int(mrCreatedBy.Int64)
				rel.CreatedBy = &cb
			}
			m.LatestRelease = rel
		}
		milestones = append(milestones, m)
	}

	if milestones == nil {
		milestones = []MilestoneResult{}
	}

	var total int
	_ = s.db.QueryRow(countQuery, countArgs...).Scan(&total)

	return milestones, total, nil
}

// GetMilestone retrieves a milestone by ID.
func (s *PlanningService) GetMilestone(id int) (*MilestoneResult, error) {
	var m MilestoneResult
	var description, targetDate, categoryName, categoryColor, workspaceName sql.NullString
	var categoryID, workspaceID sql.NullInt64
	// Release columns
	var mrID, mrCreatedBy, mrSCMConnectionID sql.NullInt64
	var mrTagName, mrName, mrBody, mrTargetCommitish sql.NullString
	var mrSCMRepository, mrSCMReleaseID, mrSCMReleaseURL sql.NullString
	var mrIsDraft, mrIsPrerelease sql.NullBool
	var mrCreatedAt sql.NullString
	err := s.db.QueryRow(`
		SELECT m.id, m.name, m.description, m.target_date, m.status, m.category_id,
		       mc.name as category_name, mc.color as category_color,
		       m.is_global, m.workspace_id, w.name as workspace_name,
		       mr.id, mr.tag_name, mr.name, mr.body, mr.is_draft, mr.is_prerelease,
		       mr.target_commitish, mr.scm_connection_id, mr.scm_repository,
		       mr.scm_release_id, mr.scm_release_url, mr.created_by, mr.created_at,
		       m.created_at, m.updated_at
		FROM milestones m
		LEFT JOIN milestone_categories mc ON m.category_id = mc.id
		LEFT JOIN workspaces w ON m.workspace_id = w.id
		LEFT JOIN (
			SELECT * FROM milestone_releases
			WHERE id IN (
				SELECT MAX(id) FROM milestone_releases GROUP BY milestone_id
			)
		) mr ON mr.milestone_id = m.id
		WHERE m.id = ?
	`, id).Scan(&m.ID, &m.Name, &description, &targetDate, &m.Status, &categoryID,
		&categoryName, &categoryColor, &m.IsGlobal, &workspaceID, &workspaceName,
		&mrID, &mrTagName, &mrName, &mrBody, &mrIsDraft, &mrIsPrerelease,
		&mrTargetCommitish, &mrSCMConnectionID, &mrSCMRepository,
		&mrSCMReleaseID, &mrSCMReleaseURL, &mrCreatedBy, &mrCreatedAt,
		&m.CreatedAt, &m.UpdatedAt)

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
	m.WorkspaceName = workspaceName.String
	if categoryID.Valid {
		cid := int(categoryID.Int64)
		m.CategoryID = &cid
	}
	if workspaceID.Valid {
		wid := int(workspaceID.Int64)
		m.WorkspaceID = &wid
	}
	if mrID.Valid {
		rel := &MilestoneReleaseResult{
			ID:          int(mrID.Int64),
			MilestoneID: m.ID,
			TagName:     mrTagName.String,
			Name:        mrName.String,
			Body:        mrBody.String,
			CreatedAt:   mrCreatedAt.String,
		}
		if mrIsDraft.Valid {
			rel.IsDraft = mrIsDraft.Bool
		}
		if mrIsPrerelease.Valid {
			rel.IsPrerelease = mrIsPrerelease.Bool
		}
		rel.TargetCommitish = mrTargetCommitish.String
		if mrSCMConnectionID.Valid {
			cid := int(mrSCMConnectionID.Int64)
			rel.SCMConnectionID = &cid
		}
		if mrSCMRepository.Valid {
			rel.SCMRepository = &mrSCMRepository.String
		}
		if mrSCMReleaseID.Valid {
			rel.SCMReleaseID = &mrSCMReleaseID.String
		}
		if mrSCMReleaseURL.Valid {
			rel.SCMReleaseURL = &mrSCMReleaseURL.String
		}
		if mrCreatedBy.Valid {
			cb := int(mrCreatedBy.Int64)
			rel.CreatedBy = &cb
		}
		m.LatestRelease = rel
	}

	return &m, nil
}

// GetSCMConnectionWorkspaceID returns the workspace_id for a given SCM connection ID.
// Returns 0 and no error if the connection doesn't exist.
func (s *PlanningService) GetSCMConnectionWorkspaceID(connectionID int) (int, error) {
	var workspaceID int
	err := s.db.QueryRow(`SELECT workspace_id FROM workspace_scm_connections WHERE id = ?`, connectionID).Scan(&workspaceID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get SCM connection workspace: %w", err)
	}
	return workspaceID, nil
}

// CreateMilestoneParams contains parameters for creating a milestone.
type CreateMilestoneParams struct {
	Name        string
	Description string
	TargetDate  string
	Status      string
	CategoryID  *int
	IsGlobal    bool
	WorkspaceID *int
}

// CreateMilestone creates a new milestone.
func (s *PlanningService) CreateMilestone(params CreateMilestoneParams) (*MilestoneResult, error) {
	status := params.Status
	if status == "" {
		status = "planning"
	}

	var id int64
	err := s.db.QueryRow(`
		INSERT INTO milestones (name, description, target_date, status, category_id, is_global, workspace_id)
		VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, params.Name, params.Description, params.TargetDate, status, params.CategoryID, params.IsGlobal, params.WorkspaceID).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("failed to create milestone: %w", err)
	}

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
	IsGlobal    bool
	WorkspaceID *int
}

// UpdateMilestone updates an existing milestone.
func (s *PlanningService) UpdateMilestone(params UpdateMilestoneParams) (*MilestoneResult, error) {
	_, err := s.db.ExecWrite(`
		UPDATE milestones SET name = ?, description = ?, target_date = ?, status = ?, category_id = ?,
		       is_global = ?, workspace_id = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, params.Name, params.Description, params.TargetDate, params.Status, params.CategoryID,
		params.IsGlobal, params.WorkspaceID, params.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to update milestone: %w", err)
	}

	return s.GetMilestone(params.ID)
}

// ListMilestoneReleases fetches all releases for a given milestone, ordered by created_at DESC.
func (s *PlanningService) ListMilestoneReleases(milestoneID int) ([]MilestoneReleaseResult, error) {
	rows, err := s.db.Query(`
		SELECT id, milestone_id, tag_name, name, body, is_draft, is_prerelease,
		       target_commitish, scm_connection_id, scm_repository,
		       scm_release_id, scm_release_url, created_by, created_at
		FROM milestone_releases
		WHERE milestone_id = ?
		ORDER BY created_at DESC
	`, milestoneID)
	if err != nil {
		return nil, fmt.Errorf("failed to list milestone releases: %w", err)
	}
	defer rows.Close()

	var releases []MilestoneReleaseResult
	for rows.Next() {
		var r MilestoneReleaseResult
		var name, body, targetCommitish sql.NullString
		var scmConnectionID, createdBy sql.NullInt64
		var scmRepository, scmReleaseID, scmReleaseURL sql.NullString
		var isDraft, isPrerelease sql.NullBool

		if err := rows.Scan(&r.ID, &r.MilestoneID, &r.TagName, &name, &body,
			&isDraft, &isPrerelease, &targetCommitish, &scmConnectionID, &scmRepository,
			&scmReleaseID, &scmReleaseURL, &createdBy, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan milestone release: %w", err)
		}

		r.Name = name.String
		r.Body = body.String
		r.TargetCommitish = targetCommitish.String
		if isDraft.Valid {
			r.IsDraft = isDraft.Bool
		}
		if isPrerelease.Valid {
			r.IsPrerelease = isPrerelease.Bool
		}
		if scmConnectionID.Valid {
			cid := int(scmConnectionID.Int64)
			r.SCMConnectionID = &cid
		}
		if scmRepository.Valid {
			r.SCMRepository = &scmRepository.String
		}
		if scmReleaseID.Valid {
			r.SCMReleaseID = &scmReleaseID.String
		}
		if scmReleaseURL.Valid {
			r.SCMReleaseURL = &scmReleaseURL.String
		}
		if createdBy.Valid {
			cb := int(createdBy.Int64)
			r.CreatedBy = &cb
		}

		releases = append(releases, r)
	}

	return releases, nil
}

// ReleaseMilestoneParams contains parameters for releasing a milestone.
type ReleaseMilestoneParams struct {
	ID              int
	TagName         string
	Name            string
	Body            string
	IsDraft         bool
	IsPrerelease    bool
	TargetCommitish string
	SCMConnectionID *int
	SCMRepository   *string
	SCMReleaseID    *string
	SCMReleaseURL   *string
	CreatedBy       *int
}

// ReleaseMilestone inserts a release record and marks the milestone as completed.
func (s *PlanningService) ReleaseMilestone(params ReleaseMilestoneParams) (*MilestoneResult, error) {
	_, err := s.db.ExecWrite(`
		INSERT INTO milestone_releases (
			milestone_id, tag_name, name, body, is_draft, is_prerelease,
			target_commitish, scm_connection_id, scm_repository, scm_release_id,
			scm_release_url, created_by
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, params.ID, params.TagName, params.Name, params.Body, params.IsDraft, params.IsPrerelease,
		params.TargetCommitish, params.SCMConnectionID, params.SCMRepository, params.SCMReleaseID,
		params.SCMReleaseURL, params.CreatedBy)
	if err != nil {
		return nil, fmt.Errorf("failed to insert milestone release: %w", err)
	}

	_, err = s.db.ExecWrite(`
		UPDATE milestones SET status = 'completed', updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, params.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to update milestone status: %w", err)
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
	ID            int
	Name          string
	Description   string
	StartDate     string
	EndDate       string
	Status        string
	TypeID        *int
	TypeName      string
	TypeColor     string
	IsGlobal      bool
	WorkspaceID   *int
	WorkspaceName string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// IterationListParams contains parameters for listing iterations.
type IterationListParams struct {
	Limit         int
	Offset        int
	WorkspaceID   *int   // Filter by workspace
	TypeID        *int   // Filter by type
	Status        string // Filter by status
	IncludeGlobal bool   // Include global iterations
}

// ListIterations retrieves iterations with pagination and filtering.
func (s *PlanningService) ListIterations(params IterationListParams) ([]IterationResult, int, error) {
	query := `
		SELECT i.id, i.name, i.description, i.start_date, i.end_date, i.status,
		       i.type_id, it.name as type_name, it.color as type_color,
		       i.is_global, i.workspace_id, w.name as workspace_name,
		       i.created_at, i.updated_at
		FROM iterations i
		LEFT JOIN iteration_types it ON i.type_id = it.id
		LEFT JOIN workspaces w ON i.workspace_id = w.id
		WHERE 1=1`

	countQuery := "SELECT COUNT(*) FROM iterations i WHERE 1=1"
	var args []interface{}
	var countArgs []interface{}

	// Filter by workspace - show local iterations for this workspace + optionally global iterations
	if params.WorkspaceID != nil {
		if params.IncludeGlobal {
			query += " AND (i.workspace_id = ? OR i.is_global = ?)"
			countQuery += " AND (i.workspace_id = ? OR i.is_global = ?)"
			args = append(args, *params.WorkspaceID, true)
			countArgs = append(countArgs, *params.WorkspaceID, true)
		} else {
			query += " AND i.workspace_id = ?"
			countQuery += " AND i.workspace_id = ?"
			args = append(args, *params.WorkspaceID)
			countArgs = append(countArgs, *params.WorkspaceID)
		}
	} else if params.IncludeGlobal {
		// If no workspace specified but include_global, only show global iterations
		query += " AND i.is_global = ?"
		countQuery += " AND i.is_global = ?"
		args = append(args, true)
		countArgs = append(countArgs, true)
	}

	// Filter by type
	if params.TypeID != nil {
		if *params.TypeID == 0 {
			query += " AND i.type_id IS NULL"
			countQuery += " AND i.type_id IS NULL"
		} else {
			query += " AND i.type_id = ?"
			countQuery += " AND i.type_id = ?"
			args = append(args, *params.TypeID)
			countArgs = append(countArgs, *params.TypeID)
		}
	}

	// Filter by status
	if params.Status != "" {
		query += " AND i.status = ?"
		countQuery += " AND i.status = ?"
		args = append(args, params.Status)
		countArgs = append(countArgs, params.Status)
	}

	query += " ORDER BY i.start_date DESC, i.name"
	query += " LIMIT ? OFFSET ?"
	args = append(args, params.Limit, params.Offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list iterations: %w", err)
	}
	defer rows.Close()

	var iterations []IterationResult
	for rows.Next() {
		var iter IterationResult
		var description, typeName, typeColor, workspaceName sql.NullString
		var typeID, workspaceID sql.NullInt64
		err := rows.Scan(&iter.ID, &iter.Name, &description, &iter.StartDate, &iter.EndDate, &iter.Status,
			&typeID, &typeName, &typeColor, &iter.IsGlobal, &workspaceID, &workspaceName,
			&iter.CreatedAt, &iter.UpdatedAt)
		if err != nil {
			continue
		}
		iter.Description = description.String
		iter.TypeName = typeName.String
		iter.TypeColor = typeColor.String
		iter.WorkspaceName = workspaceName.String
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
	_ = s.db.QueryRow(countQuery, countArgs...).Scan(&total)

	return iterations, total, nil
}

// GetIteration retrieves an iteration by ID.
func (s *PlanningService) GetIteration(id int) (*IterationResult, error) {
	var iter IterationResult
	var description, typeName, typeColor, workspaceName sql.NullString
	var typeID, workspaceID sql.NullInt64
	err := s.db.QueryRow(`
		SELECT i.id, i.name, i.description, i.start_date, i.end_date, i.status,
		       i.type_id, it.name as type_name, it.color as type_color,
		       i.is_global, i.workspace_id, w.name as workspace_name,
		       i.created_at, i.updated_at
		FROM iterations i
		LEFT JOIN iteration_types it ON i.type_id = it.id
		LEFT JOIN workspaces w ON i.workspace_id = w.id
		WHERE i.id = ?
	`, id).Scan(&iter.ID, &iter.Name, &description, &iter.StartDate, &iter.EndDate, &iter.Status,
		&typeID, &typeName, &typeColor, &iter.IsGlobal, &workspaceID, &workspaceName,
		&iter.CreatedAt, &iter.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("iteration not found: %d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get iteration: %w", err)
	}

	iter.Description = description.String
	iter.TypeName = typeName.String
	iter.TypeColor = typeColor.String
	iter.WorkspaceName = workspaceName.String
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

// IsIterationGlobal checks if an iteration is global and returns its workspace_id.
func (s *PlanningService) IsIterationGlobal(id int) (isGlobal bool, workspaceID *int, err error) {
	var wsID sql.NullInt64
	err = s.db.QueryRow("SELECT is_global, workspace_id FROM iterations WHERE id = ?", id).Scan(&isGlobal, &wsID)
	if err == sql.ErrNoRows {
		return false, nil, fmt.Errorf("iteration not found: %d", id)
	}
	if err != nil {
		return false, nil, fmt.Errorf("failed to check iteration: %w", err)
	}
	if wsID.Valid {
		wid := int(wsID.Int64)
		workspaceID = &wid
	}
	return isGlobal, workspaceID, nil
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

	var id int64
	err := s.db.QueryRow(`
		INSERT INTO iterations (name, description, start_date, end_date, status, type_id, is_global, workspace_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, params.Name, params.Description, params.StartDate, params.EndDate, status, params.TypeID, params.IsGlobal, params.WorkspaceID).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("failed to create iteration: %w", err)
	}

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
	Limit       int
	Offset      int
	WorkspaceID *int // Filter by workspace
}

// ListProjects retrieves projects with pagination and filtering.
func (s *PlanningService) ListProjects(params ProjectListParams) ([]ProjectResult, int, error) {
	query := `
		SELECT p.id, p.name, p.description, p.active, p.workspace_id,
		       w.name as workspace_name, p.created_at, p.updated_at
		FROM projects p
		LEFT JOIN workspaces w ON p.workspace_id = w.id
		WHERE 1=1`
	countQuery := "SELECT COUNT(*) FROM projects p WHERE 1=1"

	var args []interface{}
	var countArgs []interface{}

	// Filter by workspace
	if params.WorkspaceID != nil {
		query += " AND p.workspace_id = ?"
		countQuery += " AND p.workspace_id = ?"
		args = append(args, *params.WorkspaceID)
		countArgs = append(countArgs, *params.WorkspaceID)
	}

	query += " ORDER BY p.name"
	query += " LIMIT ? OFFSET ?"
	args = append(args, params.Limit, params.Offset)

	rows, err := s.db.Query(query, args...)
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
	_ = s.db.QueryRow(countQuery, countArgs...).Scan(&total)

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
	var id int64
	err := s.db.QueryRow(`
		INSERT INTO projects (name, description, workspace_id, active)
		VALUES (?, ?, ?, ?) RETURNING id
	`, params.Name, params.Description, params.WorkspaceID, params.Active).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("failed to create project: %w", err)
	}

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

// GetProjectWorkspaceID returns the workspace_id for a project (for permission checks).
func (s *PlanningService) GetProjectWorkspaceID(id int) (*int, error) {
	var workspaceID sql.NullInt64
	err := s.db.QueryRow("SELECT workspace_id FROM projects WHERE id = ?", id).Scan(&workspaceID)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("project not found: %d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get project workspace: %w", err)
	}
	if workspaceID.Valid {
		wid := int(workspaceID.Int64)
		return &wid, nil
	}
	return nil, nil
}

// ========================================
// Milestone Statistics & Progress
// ========================================

// MilestoneTestStats contains test plan statistics for a milestone.
type MilestoneTestStats struct {
	TotalTestPlans     int `json:"total_test_plans"`
	TotalTestRuns      int `json:"total_test_runs"`
	SuccessfulTestRuns int `json:"successful_test_runs"`
	FailedTestRuns     int `json:"failed_test_runs"`
	InProgressTestRuns int `json:"in_progress_test_runs"`
	TotalTestCases     int `json:"total_test_cases"`
}

// GetMilestoneTestStatistics retrieves test plan statistics for a milestone.
func (s *PlanningService) GetMilestoneTestStatistics(milestoneID int) (*MilestoneTestStats, error) {
	var stats MilestoneTestStats

	err := s.db.QueryRow(`
		SELECT
			COUNT(DISTINCT ts.id) as total_test_plans,
			COALESCE(SUM(run_stats.total_runs), 0) as total_test_runs,
			COALESCE(SUM(run_stats.successful_runs), 0) as successful_test_runs,
			COALESCE(SUM(run_stats.failed_runs), 0) as failed_test_runs,
			COALESCE(SUM(run_stats.in_progress_runs), 0) as in_progress_test_runs,
			COALESCE(SUM(tc_counts.test_case_count), 0) as total_test_cases
		FROM test_sets ts
		LEFT JOIN (
			SELECT
				set_id,
				COUNT(*) as total_runs,
				SUM(CASE WHEN ended_at IS NOT NULL THEN 1 ELSE 0 END) as successful_runs,
				SUM(CASE WHEN ended_at IS NULL THEN 1 ELSE 0 END) as failed_runs,
				0 as in_progress_runs
			FROM test_runs
			GROUP BY set_id
		) run_stats ON ts.id = run_stats.set_id
		LEFT JOIN (
			SELECT
				stc.set_id,
				COUNT(stc.test_case_id) as test_case_count
			FROM set_test_cases stc
			GROUP BY stc.set_id
		) tc_counts ON ts.id = tc_counts.set_id
		WHERE ts.milestone_id = ?
	`, milestoneID).Scan(
		&stats.TotalTestPlans,
		&stats.TotalTestRuns,
		&stats.SuccessfulTestRuns,
		&stats.FailedTestRuns,
		&stats.InProgressTestRuns,
		&stats.TotalTestCases,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get milestone test statistics: %w", err)
	}

	return &stats, nil
}

// MilestoneStatusBreakdown represents item counts by status category for a milestone.
type MilestoneStatusBreakdown struct {
	CategoryName  string `json:"category_name"`
	CategoryColor string `json:"category_color,omitempty"`
	ItemCount     int    `json:"item_count"`
	IsCompleted   bool   `json:"is_completed"`
}

// MilestoneProgressItem represents a work item in the milestone progress report.
type MilestoneProgressItem struct {
	ID             int    `json:"id"`
	Title          string `json:"title"`
	WorkspaceID    int    `json:"workspace_id"`
	WorkspaceKey   string `json:"workspace_key"`
	ItemNumber     int    `json:"item_number"`
	StatusName     string `json:"status_name,omitempty"`
	StatusColor    string `json:"status_color,omitempty"`
	PriorityName   string `json:"priority_name,omitempty"`
	PriorityColor  string `json:"priority_color,omitempty"`
	AssigneeName   string `json:"assignee_name,omitempty"`
	AssigneeAvatar string `json:"assignee_avatar,omitempty"`
}

// MilestoneProgressReport represents the full milestone progress data.
type MilestoneProgressReport struct {
	MilestoneID     int                                `json:"milestone_id"`
	MilestoneName   string                             `json:"milestone_name"`
	Description     string                             `json:"description,omitempty"`
	TargetDate      *string                            `json:"target_date,omitempty"`
	Status          string                             `json:"status"`
	CategoryColor   string                             `json:"category_color,omitempty"`
	TotalItems      int                                `json:"total_items"`
	CompletedItems  int                                `json:"completed_items"`
	PercentComplete float64                            `json:"percent_complete"`
	StatusBreakdown []MilestoneStatusBreakdown         `json:"status_breakdown"`
	ItemsByCategory map[string][]MilestoneProgressItem `json:"items_by_category"`
}

// GetMilestoneProgress retrieves progress report for a milestone.
func (s *PlanningService) GetMilestoneProgress(milestoneID int) (*MilestoneProgressReport, error) {
	var report MilestoneProgressReport
	report.MilestoneID = milestoneID
	report.ItemsByCategory = make(map[string][]MilestoneProgressItem)

	// Get milestone details
	var description, targetDate, categoryColor sql.NullString
	err := s.db.QueryRow(`
		SELECT m.name, m.description, m.target_date, m.status, mc.color
		FROM milestones m
		LEFT JOIN milestone_categories mc ON m.category_id = mc.id
		WHERE m.id = ?
	`, milestoneID).Scan(&report.MilestoneName, &description, &targetDate, &report.Status, &categoryColor)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("milestone not found: %d", milestoneID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get milestone: %w", err)
	}

	report.Description = description.String
	if targetDate.Valid && targetDate.String != "" {
		report.TargetDate = &targetDate.String
	}
	report.CategoryColor = categoryColor.String

	// Get status breakdown and items grouped by status category
	rows, err := s.db.Query(`
		SELECT
			i.id, i.title, i.workspace_id, w.key as workspace_key, i.workspace_item_number,
			COALESCE(sc.name, 'No Status') as category_name,
			COALESCE(sc.color, '#9ca3af') as category_color,
			COALESCE(sc.is_completed, false) as is_completed,
			COALESCE(st.name, '') as status_name,
			COALESCE(sc.color, '') as status_color,
			COALESCE(p.name, '') as priority_name,
			COALESCE(p.color, '') as priority_color,
			COALESCE(u.first_name || ' ' || u.last_name, '') as assignee_name,
			COALESCE(u.avatar_url, '') as assignee_avatar
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN statuses st ON i.status_id = st.id
		LEFT JOIN status_categories sc ON st.category_id = sc.id
		LEFT JOIN priorities p ON i.priority_id = p.id
		LEFT JOIN users u ON i.assignee_id = u.id
		WHERE i.milestone_id = ?
		ORDER BY sc.name, i.workspace_item_number
	`, milestoneID)

	if err != nil {
		return nil, fmt.Errorf("failed to get milestone items: %w", err)
	}
	defer rows.Close()

	// Track status breakdown counts
	breakdownMap := make(map[string]*MilestoneStatusBreakdown)

	for rows.Next() {
		var item MilestoneProgressItem
		var categoryName string
		var categoryColorVal string
		var isCompleted bool
		var statusColor, priorityColor sql.NullString

		err := rows.Scan(
			&item.ID, &item.Title, &item.WorkspaceID, &item.WorkspaceKey, &item.ItemNumber,
			&categoryName, &categoryColorVal, &isCompleted,
			&item.StatusName, &statusColor,
			&item.PriorityName, &priorityColor,
			&item.AssigneeName, &item.AssigneeAvatar,
		)
		if err != nil {
			continue
		}

		item.StatusColor = statusColor.String
		item.PriorityColor = priorityColor.String

		// Update breakdown counts
		if _, exists := breakdownMap[categoryName]; !exists {
			breakdownMap[categoryName] = &MilestoneStatusBreakdown{
				CategoryName:  categoryName,
				CategoryColor: categoryColorVal,
				IsCompleted:   isCompleted,
				ItemCount:     0,
			}
		}
		breakdownMap[categoryName].ItemCount++

		// Add item to category group
		report.ItemsByCategory[categoryName] = append(report.ItemsByCategory[categoryName], item)

		// Update totals
		report.TotalItems++
		if isCompleted {
			report.CompletedItems++
		}
	}

	// Convert breakdown map to slice
	report.StatusBreakdown = make([]MilestoneStatusBreakdown, 0, len(breakdownMap))
	for _, breakdown := range breakdownMap {
		report.StatusBreakdown = append(report.StatusBreakdown, *breakdown)
	}

	// Calculate percentage
	if report.TotalItems > 0 {
		report.PercentComplete = float64(report.CompletedItems) / float64(report.TotalItems) * 100.0
	}

	return &report, nil
}

// IsMilestoneGlobal checks if a milestone is global.
func (s *PlanningService) IsMilestoneGlobal(id int) (isGlobal bool, workspaceID *int, err error) {
	var wsID sql.NullInt64
	err = s.db.QueryRow("SELECT is_global, workspace_id FROM milestones WHERE id = ?", id).Scan(&isGlobal, &wsID)
	if err == sql.ErrNoRows {
		return false, nil, fmt.Errorf("milestone not found: %d", id)
	}
	if err != nil {
		return false, nil, fmt.Errorf("failed to check milestone: %w", err)
	}
	if wsID.Valid {
		wid := int(wsID.Int64)
		workspaceID = &wid
	}
	return isGlobal, workspaceID, nil
}

// ========================================
// Iteration Statistics & Progress
// ========================================

// IterationStatusBreakdown represents item counts by status category for an iteration.
type IterationStatusBreakdown struct {
	CategoryName  string `json:"category_name"`
	CategoryColor string `json:"category_color,omitempty"`
	ItemCount     int    `json:"item_count"`
	IsCompleted   bool   `json:"is_completed"`
}

// IterationProgressItem represents a work item in the iteration progress report.
type IterationProgressItem struct {
	ID             int    `json:"id"`
	Title          string `json:"title"`
	WorkspaceID    int    `json:"workspace_id"`
	WorkspaceKey   string `json:"workspace_key"`
	ItemNumber     int    `json:"item_number"`
	StatusName     string `json:"status_name,omitempty"`
	StatusColor    string `json:"status_color,omitempty"`
	PriorityName   string `json:"priority_name,omitempty"`
	PriorityColor  string `json:"priority_color,omitempty"`
	AssigneeName   string `json:"assignee_name,omitempty"`
	AssigneeAvatar string `json:"assignee_avatar,omitempty"`
}

// IterationProgressReport represents the full iteration progress data.
type IterationProgressReport struct {
	IterationID     int                                `json:"iteration_id"`
	IterationName   string                             `json:"iteration_name"`
	Description     string                             `json:"description,omitempty"`
	StartDate       string                             `json:"start_date"`
	EndDate         string                             `json:"end_date"`
	Status          string                             `json:"status"`
	TypeColor       string                             `json:"type_color,omitempty"`
	TotalItems      int                                `json:"total_items"`
	CompletedItems  int                                `json:"completed_items"`
	PercentComplete float64                            `json:"percent_complete"`
	StatusBreakdown []IterationStatusBreakdown         `json:"status_breakdown"`
	ItemsByCategory map[string][]IterationProgressItem `json:"items_by_category"`
}

// GetIterationProgress retrieves progress report for an iteration.
func (s *PlanningService) GetIterationProgress(iterationID int) (*IterationProgressReport, error) {
	var report IterationProgressReport
	report.IterationID = iterationID
	report.ItemsByCategory = make(map[string][]IterationProgressItem)

	// Get iteration details
	var description, typeColor sql.NullString
	err := s.db.QueryRow(`
		SELECT i.name, i.description, i.start_date, i.end_date, i.status, it.color
		FROM iterations i
		LEFT JOIN iteration_types it ON i.type_id = it.id
		WHERE i.id = ?
	`, iterationID).Scan(&report.IterationName, &description, &report.StartDate, &report.EndDate, &report.Status, &typeColor)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("iteration not found: %d", iterationID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get iteration: %w", err)
	}

	report.Description = description.String
	report.TypeColor = typeColor.String

	// Get status breakdown and items grouped by status category
	rows, err := s.db.Query(`
		SELECT
			i.id, i.title, i.workspace_id, w.key as workspace_key, i.workspace_item_number,
			COALESCE(sc.name, 'No Status') as category_name,
			COALESCE(sc.color, '#9ca3af') as category_color,
			COALESCE(sc.is_completed, false) as is_completed,
			COALESCE(st.name, '') as status_name,
			COALESCE(sc.color, '') as status_color,
			COALESCE(p.name, '') as priority_name,
			COALESCE(p.color, '') as priority_color,
			COALESCE(u.first_name || ' ' || u.last_name, '') as assignee_name,
			COALESCE(u.avatar_url, '') as assignee_avatar
		FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN statuses st ON i.status_id = st.id
		LEFT JOIN status_categories sc ON st.category_id = sc.id
		LEFT JOIN priorities p ON i.priority_id = p.id
		LEFT JOIN users u ON i.assignee_id = u.id
		WHERE i.iteration_id = ?
		ORDER BY sc.name, i.workspace_item_number
	`, iterationID)

	if err != nil {
		return nil, fmt.Errorf("failed to get iteration items: %w", err)
	}
	defer rows.Close()

	// Track status breakdown counts
	breakdownMap := make(map[string]*IterationStatusBreakdown)

	for rows.Next() {
		var item IterationProgressItem
		var categoryName string
		var categoryColorVal string
		var isCompleted bool
		var statusColor, priorityColor sql.NullString

		err := rows.Scan(
			&item.ID, &item.Title, &item.WorkspaceID, &item.WorkspaceKey, &item.ItemNumber,
			&categoryName, &categoryColorVal, &isCompleted,
			&item.StatusName, &statusColor,
			&item.PriorityName, &priorityColor,
			&item.AssigneeName, &item.AssigneeAvatar,
		)
		if err != nil {
			continue
		}

		item.StatusColor = statusColor.String
		item.PriorityColor = priorityColor.String

		// Update breakdown counts
		if _, exists := breakdownMap[categoryName]; !exists {
			breakdownMap[categoryName] = &IterationStatusBreakdown{
				CategoryName:  categoryName,
				CategoryColor: categoryColorVal,
				IsCompleted:   isCompleted,
				ItemCount:     0,
			}
		}
		breakdownMap[categoryName].ItemCount++

		// Add item to category group
		report.ItemsByCategory[categoryName] = append(report.ItemsByCategory[categoryName], item)

		// Update totals
		report.TotalItems++
		if isCompleted {
			report.CompletedItems++
		}
	}

	// Convert breakdown map to slice
	report.StatusBreakdown = make([]IterationStatusBreakdown, 0, len(breakdownMap))
	for _, breakdown := range breakdownMap {
		report.StatusBreakdown = append(report.StatusBreakdown, *breakdown)
	}

	// Calculate percentage
	if report.TotalItems > 0 {
		report.PercentComplete = float64(report.CompletedItems) / float64(report.TotalItems) * 100.0
	}

	return &report, nil
}

// ========================================
// Iteration Burndown Chart
// ========================================

// BurndownDataPoint represents a single day's burndown data.
type BurndownDataPoint struct {
	Date      string `json:"date"`
	Remaining int    `json:"remaining"`
	Completed int    `json:"completed"`
	Ideal     int    `json:"ideal"`
}

// IterationBurndownData represents the full burndown chart data.
type IterationBurndownData struct {
	IterationID int                 `json:"iteration_id"`
	StartDate   string              `json:"start_date"`
	EndDate     string              `json:"end_date"`
	TotalItems  int                 `json:"total_items"`
	DataPoints  []BurndownDataPoint `json:"data_points"`
}

// GetIterationBurndown calculates burndown data for an iteration by replaying item history.
func (s *PlanningService) GetIterationBurndown(iterationID int) (*IterationBurndownData, error) {
	// Get iteration details
	iter, err := s.GetIteration(iterationID)
	if err != nil {
		return nil, err
	}

	// Parse dates
	startDate, err := time.Parse("2006-01-02", iter.StartDate)
	if err != nil {
		return nil, fmt.Errorf("invalid start date: %w", err)
	}
	endDate, err := time.Parse("2006-01-02", iter.EndDate)
	if err != nil {
		return nil, fmt.Errorf("invalid end date: %w", err)
	}

	// Get all items in this iteration with their current status category
	rows, err := s.db.Query(`
		SELECT i.id, COALESCE(sc.is_completed, false) as is_completed
		FROM items i
		LEFT JOIN statuses st ON i.status_id = st.id
		LEFT JOIN status_categories sc ON st.category_id = sc.id
		WHERE i.iteration_id = ?
	`, iterationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get iteration items: %w", err)
	}
	defer rows.Close()

	// Build map of item IDs to their current completed state
	itemStates := make(map[int]bool) // itemID -> isCompleted
	for rows.Next() {
		var itemID int
		var isCompleted bool
		if err = rows.Scan(&itemID, &isCompleted); err != nil {
			continue
		}
		itemStates[itemID] = isCompleted
	}

	totalItems := len(itemStates)
	if totalItems == 0 {
		// Return empty data if no items
		return &IterationBurndownData{
			IterationID: iterationID,
			StartDate:   iter.StartDate,
			EndDate:     iter.EndDate,
			TotalItems:  0,
			DataPoints:  []BurndownDataPoint{},
		}, nil
	}

	// Get all status changes for items in this iteration within the date range
	// We need to work backwards from current state using history
	historyRows, err := s.db.Query(`
		SELECT ih.item_id, ih.changed_at, ih.old_value, ih.new_value
		FROM item_history ih
		JOIN items i ON ih.item_id = i.id
		WHERE i.iteration_id = ?
		  AND ih.field_name = 'status_id'
		  AND ih.changed_at >= ?
		ORDER BY ih.changed_at DESC
	`, iterationID, startDate.Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("failed to get item history: %w", err)
	}
	defer historyRows.Close()

	// Collect all status change events
	type statusChange struct {
		ItemID    int
		ChangedAt time.Time
		OldValue  sql.NullString
		NewValue  sql.NullString
	}
	var changes []statusChange

	for historyRows.Next() {
		var c statusChange
		var changedAtStr string
		if err = historyRows.Scan(&c.ItemID, &changedAtStr, &c.OldValue, &c.NewValue); err != nil {
			continue
		}
		// Parse the datetime
		c.ChangedAt, _ = time.Parse("2006-01-02 15:04:05", changedAtStr)
		if c.ChangedAt.IsZero() {
			c.ChangedAt, _ = time.Parse(time.RFC3339, changedAtStr)
		}
		changes = append(changes, c)
	}

	// Get status_id -> is_completed mapping
	statusCompletedMap := make(map[int]bool)
	statusRows, err := s.db.Query(`
		SELECT s.id, COALESCE(sc.is_completed, false)
		FROM statuses s
		LEFT JOIN status_categories sc ON s.category_id = sc.id
	`)
	if err == nil {
		defer statusRows.Close()
		for statusRows.Next() {
			var statusID int
			var isCompleted bool
			if err := statusRows.Scan(&statusID, &isCompleted); err == nil {
				statusCompletedMap[statusID] = isCompleted
			}
		}
	}

	// Helper to check if a status_id string represents a completed status
	isStatusCompleted := func(statusIDStr string) bool {
		if statusIDStr == "" {
			return false
		}
		var statusID int
		if _, err := fmt.Sscanf(statusIDStr, "%d", &statusID); err != nil {
			return false
		}
		return statusCompletedMap[statusID]
	}

	// Build daily data points
	var dataPoints []BurndownDataPoint
	today := time.Now().Truncate(24 * time.Hour)
	effectiveEndDate := endDate
	if today.Before(endDate) {
		effectiveEndDate = today
	}

	totalDays := int(endDate.Sub(startDate).Hours()/24) + 1

	// Start with current state and work backwards through history to build daily snapshots
	// Clone current state for simulation
	dayStates := make(map[int]bool)
	for id, completed := range itemStates {
		dayStates[id] = completed
	}

	// Build data for each day from end to start
	type dayData struct {
		date      string
		remaining int
		completed int
	}
	var dailyData []dayData

	for d := effectiveEndDate; !d.Before(startDate); d = d.AddDate(0, 0, -1) {
		dateStr := d.Format("2006-01-02")

		// Apply any history changes that happened after this day (reverse them)
		for _, c := range changes {
			changeDate := c.ChangedAt.Truncate(24 * time.Hour)
			if changeDate.Equal(d.AddDate(0, 0, 1)) || changeDate.After(d.AddDate(0, 0, 1)) {
				// This change happened after our current day, so reverse it
				// (set the item to its old state)
				if _, exists := dayStates[c.ItemID]; exists {
					dayStates[c.ItemID] = isStatusCompleted(c.OldValue.String)
				}
			}
		}

		// Filter changes to only those not yet processed
		var remainingChanges []statusChange
		for _, c := range changes {
			changeDate := c.ChangedAt.Truncate(24 * time.Hour)
			if changeDate.Before(d.AddDate(0, 0, 1)) {
				remainingChanges = append(remainingChanges, c)
			}
		}
		changes = remainingChanges

		// Count completed and remaining
		completed := 0
		for _, isCompleted := range dayStates {
			if isCompleted {
				completed++
			}
		}
		remaining := totalItems - completed

		dailyData = append(dailyData, dayData{
			date:      dateStr,
			remaining: remaining,
			completed: completed,
		})
	}

	// Reverse to get chronological order
	for i := len(dailyData) - 1; i >= 0; i-- {
		dd := dailyData[i]
		dayIndex := 0
		d, _ := time.Parse("2006-01-02", dd.date)
		dayIndex = int(d.Sub(startDate).Hours() / 24)

		// Calculate ideal remaining for this day
		ideal := totalItems
		if totalDays > 1 {
			ideal = totalItems - (dayIndex * totalItems / (totalDays - 1))
			if ideal < 0 {
				ideal = 0
			}
		}

		dataPoints = append(dataPoints, BurndownDataPoint{
			Date:      dd.date,
			Remaining: dd.remaining,
			Completed: dd.completed,
			Ideal:     ideal,
		})
	}

	return &IterationBurndownData{
		IterationID: iterationID,
		StartDate:   iter.StartDate,
		EndDate:     iter.EndDate,
		TotalItems:  totalItems,
		DataPoints:  dataPoints,
	}, nil
}

// ========================================
// Validation Helpers
// ========================================

// CategoryExists checks if a milestone category exists.
func (s *PlanningService) CategoryExists(categoryID int) (bool, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM milestone_categories WHERE id = ?", categoryID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check category: %w", err)
	}
	return count > 0, nil
}

// WorkspaceExists checks if a workspace exists.
func (s *PlanningService) WorkspaceExists(workspaceID int) (bool, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM workspaces WHERE id = ?", workspaceID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check workspace: %w", err)
	}
	return count > 0, nil
}

// IterationTypeExists checks if an iteration type exists.
func (s *PlanningService) IterationTypeExists(typeID int) (bool, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM iteration_types WHERE id = ?", typeID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check iteration type: %w", err)
	}
	return count > 0, nil
}

// ========================================
// Project Milestone Categories
// ========================================

// LoadProjectMilestoneCategories loads milestone categories for a project.
func (s *PlanningService) LoadProjectMilestoneCategories(projectID int) ([]int, error) {
	var categories []int
	rows, err := s.db.Query(`
		SELECT category_id FROM project_milestone_categories WHERE project_id = ?
	`, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to load milestone categories: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var categoryID int
		if err := rows.Scan(&categoryID); err != nil {
			return nil, fmt.Errorf("failed to scan category ID: %w", err)
		}
		categories = append(categories, categoryID)
	}
	return categories, nil
}

// SaveProjectMilestoneCategories saves milestone categories for a project.
func (s *PlanningService) SaveProjectMilestoneCategories(projectID int, categories []int) error {
	// Start transaction
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Delete existing associations
	_, err = tx.Exec("DELETE FROM project_milestone_categories WHERE project_id = ?", projectID)
	if err != nil {
		return fmt.Errorf("failed to delete existing categories: %w", err)
	}

	// Insert new associations
	for _, categoryID := range categories {
		_, err = tx.Exec(`
			INSERT INTO project_milestone_categories (project_id, category_id) VALUES (?, ?)
		`, projectID, categoryID)
		if err != nil {
			return fmt.Errorf("failed to insert category: %w", err)
		}
	}

	return tx.Commit()
}
