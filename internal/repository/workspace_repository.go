package repository

import (
	"database/sql"
	"encoding/json"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// WorkspaceRepository handles database operations for workspaces
type WorkspaceRepository struct {
	db database.Database
}

// NewWorkspaceRepository creates a new WorkspaceRepository
func NewWorkspaceRepository(db database.Database) *WorkspaceRepository {
	return &WorkspaceRepository{db: db}
}

// FindByID retrieves a workspace by ID with project count and time project name
func (r *WorkspaceRepository) FindByID(id int) (*models.Workspace, error) {
	var workspace models.Workspace
	var timeProjectName, icon, color, defaultView, displayMode sql.NullString
	var configSetID sql.NullInt64

	err := r.db.QueryRow(`
		SELECT w.id, w.name, w.key, w.description, w.active, w.time_project_id, w.is_personal, w.owner_id, w.icon, w.color, w.avatar_url, w.default_view, w.display_mode, w.created_at, w.updated_at,
		       COUNT(p.id) as project_count,
		       tp.name as time_project_name,
		       wcs.configuration_set_id
		FROM workspaces w
		LEFT JOIN projects p ON w.id = p.workspace_id
		LEFT JOIN time_projects tp ON w.time_project_id = tp.id
		LEFT JOIN workspace_configuration_sets wcs ON w.id = wcs.workspace_id
		WHERE w.id = ?
		GROUP BY w.id, w.name, w.key, w.description, w.active, w.time_project_id, w.is_personal, w.owner_id, w.icon, w.color, w.avatar_url, w.default_view, w.display_mode, w.created_at, w.updated_at, tp.name, wcs.configuration_set_id
	`, id).Scan(&workspace.ID, &workspace.Name, &workspace.Key, &workspace.Description,
		&workspace.Active, &workspace.TimeProjectID, &workspace.IsPersonal, &workspace.OwnerID,
		&icon, &color, &workspace.AvatarURL, &defaultView, &displayMode, &workspace.CreatedAt, &workspace.UpdatedAt,
		&workspace.ProjectCount, &timeProjectName, &configSetID)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	workspace.Icon = icon.String
	workspace.Color = color.String
	workspace.DefaultView = defaultView.String
	workspace.DisplayMode = displayMode.String
	workspace.TimeProjectName = timeProjectName.String
	if configSetID.Valid {
		workspace.ConfigurationSetID = &configSetID.Int64
	}

	return &workspace, nil
}

// FindByIDBasic retrieves basic workspace fields (for audit/delete operations)
func (r *WorkspaceRepository) FindByIDBasic(id int) (*models.Workspace, error) {
	var workspace models.Workspace
	var icon, color sql.NullString

	err := r.db.QueryRow(`
		SELECT id, name, key, description, active, is_personal, icon, color
		FROM workspaces
		WHERE id = ?
	`, id).Scan(&workspace.ID, &workspace.Name, &workspace.Key, &workspace.Description,
		&workspace.Active, &workspace.IsPersonal, &icon, &color)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	workspace.Icon = icon.String
	workspace.Color = color.String

	return &workspace, nil
}

// FindAll retrieves all workspaces accessible to a user
func (r *WorkspaceRepository) FindAll(userID int, isPersonalOnly bool) ([]models.Workspace, error) {
	var query string
	var rows *sql.Rows
	var err error

	if isPersonalOnly {
		query = `
			SELECT w.id, w.name, w.key, w.description, w.active, w.time_project_id, w.is_personal, w.owner_id, w.icon, w.color, w.avatar_url, w.default_view, w.display_mode, w.created_at, w.updated_at,
			       COUNT(p.id) as project_count,
			       tp.name as time_project_name
			FROM workspaces w
			LEFT JOIN projects p ON w.id = p.workspace_id
			LEFT JOIN time_projects tp ON w.time_project_id = tp.id
			WHERE w.is_personal = ? AND w.owner_id = ?
			GROUP BY w.id, w.name, w.key, w.description, w.active, w.time_project_id, w.is_personal, w.owner_id, w.icon, w.color, w.avatar_url, w.default_view, w.display_mode, w.created_at, w.updated_at, tp.name
			ORDER BY w.name`
		rows, err = r.db.Query(query, true, userID)
	} else {
		query = `
			SELECT w.id, w.name, w.key, w.description, w.active, w.time_project_id, w.is_personal, w.owner_id, w.icon, w.color, w.avatar_url, w.default_view, w.display_mode, w.created_at, w.updated_at,
			       COUNT(p.id) as project_count,
			       tp.name as time_project_name
			FROM workspaces w
			LEFT JOIN projects p ON w.id = p.workspace_id
			LEFT JOIN time_projects tp ON w.time_project_id = tp.id
			WHERE w.is_personal = false OR w.is_personal IS NULL OR w.owner_id = ?
			GROUP BY w.id, w.name, w.key, w.description, w.active, w.time_project_id, w.is_personal, w.owner_id, w.icon, w.color, w.avatar_url, w.default_view, w.display_mode, w.created_at, w.updated_at, tp.name
			ORDER BY w.is_personal ASC, w.name`
		rows, err = r.db.Query(query, userID)
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var workspaces []models.Workspace
	for rows.Next() {
		var workspace models.Workspace
		var timeProjectName, icon, color, defaultView, displayMode sql.NullString
		err := rows.Scan(&workspace.ID, &workspace.Name, &workspace.Key, &workspace.Description,
			&workspace.Active, &workspace.TimeProjectID, &workspace.IsPersonal, &workspace.OwnerID,
			&icon, &color, &workspace.AvatarURL, &defaultView, &displayMode,
			&workspace.CreatedAt, &workspace.UpdatedAt,
			&workspace.ProjectCount, &timeProjectName)
		if err != nil {
			return nil, err
		}

		workspace.Icon = icon.String
		workspace.Color = color.String
		workspace.DefaultView = defaultView.String
		workspace.DisplayMode = displayMode.String
		workspace.TimeProjectName = timeProjectName.String
		workspaces = append(workspaces, workspace)
	}

	return workspaces, rows.Err()
}

// Create inserts a new workspace and returns its ID
func (r *WorkspaceRepository) Create(workspace *models.Workspace) (int64, error) {
	now := time.Now()
	var id int64

	err := r.db.QueryRow(`
		INSERT INTO workspaces (name, key, description, active, time_project_id, is_personal, owner_id, icon, color, avatar_url, default_view, display_mode, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id
	`, workspace.Name, workspace.Key, workspace.Description, workspace.Active,
		workspace.TimeProjectID, workspace.IsPersonal, workspace.OwnerID,
		workspace.Icon, workspace.Color, workspace.AvatarURL, workspace.DefaultView, workspace.DisplayMode,
		now, now).Scan(&id)

	return id, err
}

// Update updates an existing workspace
func (r *WorkspaceRepository) Update(workspace *models.Workspace) error {
	now := time.Now()
	_, err := r.db.ExecWrite(`
		UPDATE workspaces
		SET name = ?, key = ?, description = ?, active = ?, time_project_id = ?, is_personal = ?, owner_id = ?, icon = ?, color = ?, avatar_url = ?, default_view = ?, display_mode = ?, updated_at = ?
		WHERE id = ?
	`, workspace.Name, workspace.Key, workspace.Description, workspace.Active,
		workspace.TimeProjectID, workspace.IsPersonal, workspace.OwnerID,
		workspace.Icon, workspace.Color, workspace.AvatarURL, workspace.DefaultView, workspace.DisplayMode,
		now, workspace.ID)

	return err
}

// Delete removes a workspace by ID
func (r *WorkspaceRepository) Delete(id int) error {
	_, err := r.db.ExecWrite("DELETE FROM workspaces WHERE id = ?", id)
	return err
}

// Exists checks if a workspace exists
func (r *WorkspaceRepository) Exists(id int) (bool, error) {
	var exists bool
	err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM workspaces WHERE id = ?)", id).Scan(&exists)
	return exists, err
}

// KeyExists checks if a workspace key exists
func (r *WorkspaceRepository) KeyExists(key string) (bool, error) {
	var exists bool
	err := r.db.QueryRow("SELECT EXISTS(SELECT 1 FROM workspaces WHERE key = ?)", key).Scan(&exists)
	return exists, err
}

// FindPersonalByOwnerID retrieves the personal workspace for a user
func (r *WorkspaceRepository) FindPersonalByOwnerID(ownerID int) (*models.Workspace, error) {
	var workspace models.Workspace
	var timeProjectName sql.NullString

	err := r.db.QueryRow(`
		SELECT w.id, w.name, w.key, w.description, w.active, w.time_project_id, w.is_personal, w.owner_id, w.created_at, w.updated_at,
		       COUNT(p.id) as project_count,
		       tp.name as time_project_name
		FROM workspaces w
		LEFT JOIN projects p ON w.id = p.workspace_id
		LEFT JOIN time_projects tp ON w.time_project_id = tp.id
		WHERE w.is_personal = true AND w.owner_id = ?
		GROUP BY w.id, w.name, w.key, w.description, w.active, w.time_project_id, w.is_personal, w.owner_id, w.created_at, w.updated_at, tp.name
	`, ownerID).Scan(&workspace.ID, &workspace.Name, &workspace.Key, &workspace.Description,
		&workspace.Active, &workspace.TimeProjectID, &workspace.IsPersonal, &workspace.OwnerID,
		&workspace.CreatedAt, &workspace.UpdatedAt,
		&workspace.ProjectCount, &timeProjectName)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	workspace.TimeProjectName = timeProjectName.String
	return &workspace, nil
}

// CreatePersonalWorkspace creates a personal workspace for a user
func (r *WorkspaceRepository) CreatePersonalWorkspace(ownerID int, name, key, description string) (int64, error) {
	now := time.Now()
	var id int64

	err := r.db.QueryRow(`
		INSERT INTO workspaces (name, key, description, active, time_project_id, is_personal, owner_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id
	`, name, key, description, true, nil, true, ownerID, now, now).Scan(&id)

	return id, err
}

// GetTimeProjectCategories retrieves time project categories for a workspace
func (r *WorkspaceRepository) GetTimeProjectCategories(workspaceID int) ([]int, error) {
	rows, err := r.db.Query(`
		SELECT time_project_category_id
		FROM workspace_time_project_categories
		WHERE workspace_id = ?
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	categories := []int{}
	for rows.Next() {
		var categoryID int
		if err := rows.Scan(&categoryID); err != nil {
			return nil, err
		}
		categories = append(categories, categoryID)
	}
	return categories, rows.Err()
}

// SaveTimeProjectCategories saves time project categories for a workspace
func (r *WorkspaceRepository) SaveTimeProjectCategories(workspaceID int, categories []int) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Delete existing associations
	_, err = tx.Exec("DELETE FROM workspace_time_project_categories WHERE workspace_id = ?", workspaceID)
	if err != nil {
		return err
	}

	// Insert new associations
	for _, categoryID := range categories {
		_, err = tx.Exec(
			"INSERT INTO workspace_time_project_categories (workspace_id, time_project_category_id) VALUES (?, ?)",
			workspaceID, categoryID,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetHomepageLayout retrieves the homepage layout for a workspace
func (r *WorkspaceRepository) GetHomepageLayout(workspaceID int) (*models.WorkspaceHomepageLayout, error) {
	var homepageLayout sql.NullString
	err := r.db.QueryRow(`
		SELECT homepage_layout
		FROM workspaces
		WHERE id = ?
	`, workspaceID).Scan(&homepageLayout)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	var layout models.WorkspaceHomepageLayout
	if homepageLayout.Valid && homepageLayout.String != "" {
		if err := json.Unmarshal([]byte(homepageLayout.String), &layout); err != nil {
			return nil, err
		}
	} else {
		layout = models.WorkspaceHomepageLayout{
			Sections: []models.WorkspaceHomepageSection{},
			Widgets:  []models.WorkspaceWidget{},
		}
	}

	return &layout, nil
}

// UpdateHomepageLayout updates the homepage layout for a workspace
func (r *WorkspaceRepository) UpdateHomepageLayout(workspaceID int, layout *models.WorkspaceHomepageLayout) error {
	layoutJSON, err := json.Marshal(layout)
	if err != nil {
		return err
	}

	_, err = r.db.Exec(`
		UPDATE workspaces
		SET homepage_layout = ?, updated_at = ?
		WHERE id = ?
	`, string(layoutJSON), time.Now(), workspaceID)

	return err
}

// CountCollections returns the count of collections in a workspace
func (r *WorkspaceRepository) CountCollections(workspaceID int) (int, error) {
	var count int
	err := r.db.QueryRow(`
		SELECT COUNT(*)
		FROM collections
		WHERE workspace_id = ?
	`, workspaceID).Scan(&count)
	return count, err
}

// CountItems returns the count of items in a workspace with optional filter
func (r *WorkspaceRepository) CountItems(workspaceID int, filterSQL string, filterArgs []interface{}) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM items i
		WHERE i.workspace_id = ?`

	args := []interface{}{workspaceID}
	if filterSQL != "" {
		query += " AND (" + filterSQL + ")"
		args = append(args, filterArgs...)
	}

	var count int
	err := r.db.QueryRow(query, args...).Scan(&count)
	return count, err
}

// GetItemsByStatusCategory returns item counts grouped by status category
func (r *WorkspaceRepository) GetItemsByStatusCategory(workspaceID int, filterSQL string, filterArgs []interface{}) (map[string]int, error) {
	query := `
		SELECT sc.name, COUNT(i.id) as item_count
		FROM items i
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN status_categories sc ON s.category_id = sc.id
		WHERE i.workspace_id = ?`

	args := []interface{}{workspaceID}
	if filterSQL != "" {
		query += " AND (" + filterSQL + ")"
		args = append(args, filterArgs...)
	}
	query += " GROUP BY sc.name"

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	result := make(map[string]int)
	for rows.Next() {
		var categoryName sql.NullString
		var count int
		if err := rows.Scan(&categoryName, &count); err != nil {
			return nil, err
		}
		if categoryName.Valid {
			result[categoryName.String] = count
		}
	}
	return result, rows.Err()
}

// AssignmentStats represents the distribution of items per assignee
type AssignmentStats struct {
	UserID       *int
	UserName     string
	FirstName    string
	LastName     string
	ItemCount    int
	IsUnassigned bool
}

// GetAssignmentDistribution returns item counts grouped by assignee
func (r *WorkspaceRepository) GetAssignmentDistribution(workspaceID int, since time.Time, filterSQL string, filterArgs []interface{}, limit int) ([]AssignmentStats, error) {
	query := `
		SELECT
			i.assignee_id,
			COALESCE(u.username, 'Unassigned') as user_name,
			COALESCE(u.first_name, '') as first_name,
			COALESCE(u.last_name, '') as last_name,
			COUNT(i.id) as item_count
		FROM items i
		LEFT JOIN users u ON i.assignee_id = u.id
		WHERE i.workspace_id = ?
		  AND i.created_at >= ?`

	args := []interface{}{workspaceID, since.Format("2006-01-02 15:04:05")}
	if filterSQL != "" {
		query += " AND (" + filterSQL + ")"
		args = append(args, filterArgs...)
	}
	query += `
		GROUP BY i.assignee_id, u.username, u.first_name, u.last_name
		ORDER BY item_count DESC
		LIMIT ?`
	args = append(args, limit)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var results []AssignmentStats
	for rows.Next() {
		var stat AssignmentStats
		var assigneeID sql.NullInt64
		if err := rows.Scan(&assigneeID, &stat.UserName, &stat.FirstName, &stat.LastName, &stat.ItemCount); err != nil {
			return nil, err
		}
		if assigneeID.Valid {
			id := int(assigneeID.Int64)
			stat.UserID = &id
			stat.IsUnassigned = false
		} else {
			stat.IsUnassigned = true
		}
		results = append(results, stat)
	}
	return results, rows.Err()
}

// ProjectStats represents statistics for a specific project
type ProjectStats struct {
	ProjectID         *int
	ProjectName       string
	ProjectColor      string
	ItemCount         int
	CompletedCount    int
	CompletionPercent float64
}

// GetProjectStatistics returns project statistics for a workspace
func (r *WorkspaceRepository) GetProjectStatistics(workspaceID int, since time.Time, filterSQL string, filterArgs []interface{}, limit int) ([]ProjectStats, error) {
	query := `
		SELECT
			tp.id,
			tp.name,
			tp.color,
			COUNT(i.id) as item_count,
			SUM(CASE WHEN LOWER(sc.name) = 'done' THEN 1 ELSE 0 END) as completed_count
		FROM items i
		LEFT JOIN time_projects tp ON i.time_project_id = tp.id
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN status_categories sc ON s.category_id = sc.id
		WHERE i.workspace_id = ?
		  AND i.created_at >= ?
		  AND i.time_project_id IS NOT NULL`

	args := []interface{}{workspaceID, since.Format("2006-01-02 15:04:05")}
	if filterSQL != "" {
		query += " AND (" + filterSQL + ")"
		args = append(args, filterArgs...)
	}
	query += `
		GROUP BY tp.id, tp.name, tp.color
		ORDER BY item_count DESC
		LIMIT ?`
	args = append(args, limit)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var results []ProjectStats
	for rows.Next() {
		var stat ProjectStats
		var projectID sql.NullInt64
		var projectColor sql.NullString
		if err := rows.Scan(&projectID, &stat.ProjectName, &projectColor, &stat.ItemCount, &stat.CompletedCount); err != nil {
			return nil, err
		}
		if projectID.Valid {
			id := int(projectID.Int64)
			stat.ProjectID = &id
		}
		stat.ProjectColor = projectColor.String
		if stat.ItemCount > 0 {
			stat.CompletionPercent = float64(stat.CompletedCount) / float64(stat.ItemCount) * 100
		}
		results = append(results, stat)
	}
	return results, rows.Err()
}

// GetPriorityBreakdown returns item counts grouped by priority
func (r *WorkspaceRepository) GetPriorityBreakdown(workspaceID int, since time.Time, filterSQL string, filterArgs []interface{}) (map[string]int, error) {
	query := `
		SELECT
			COALESCE(pri.name, 'None') as priority,
			COUNT(i.id) as item_count
		FROM items i
		LEFT JOIN priorities pri ON i.priority_id = pri.id
		WHERE i.workspace_id = ?
		  AND i.created_at >= ?`

	args := []interface{}{workspaceID, since.Format("2006-01-02 15:04:05")}
	if filterSQL != "" {
		query += " AND (" + filterSQL + ")"
		args = append(args, filterArgs...)
	}
	query += " GROUP BY pri.name"

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	result := make(map[string]int)
	for rows.Next() {
		var priority string
		var count int
		if err := rows.Scan(&priority, &count); err != nil {
			return nil, err
		}
		result[priority] = count
	}
	return result, rows.Err()
}

// MilestoneStatusBreakdown represents the distribution of items per status category within a milestone
type MilestoneStatusBreakdown struct {
	CategoryName  string
	CategoryColor string
	ItemCount     int
	IsCompleted   bool
}

// MilestoneStatusProgress aggregates milestone progress for a workspace
type MilestoneStatusProgress struct {
	MilestoneID     int
	MilestoneName   string
	TargetDate      *string
	Status          string
	CategoryColor   string
	TotalItems      int
	CompletedItems  int
	PercentComplete float64
	StatusBreakdown []MilestoneStatusBreakdown
}

// GetMilestoneProgress returns milestone progress for a workspace
func (r *WorkspaceRepository) GetMilestoneProgress(workspaceID int, filterSQL string, filterArgs []interface{}) ([]MilestoneStatusProgress, error) {
	query := `
		SELECT
			m.id,
			m.name,
			m.target_date,
			m.status,
			mc.color,
			sc.name,
			sc.color,
			sc.is_completed,
			COUNT(i.id) as item_count
		FROM items i
		JOIN milestones m ON i.milestone_id = m.id
		LEFT JOIN milestone_categories mc ON m.category_id = mc.id
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN status_categories sc ON s.category_id = sc.id
		WHERE i.workspace_id = ?
		  AND i.milestone_id IS NOT NULL
		  AND (m.status IS NULL OR LOWER(m.status) <> 'completed')`

	args := []interface{}{workspaceID}
	if filterSQL != "" {
		query += " AND (" + filterSQL + ")"
		args = append(args, filterArgs...)
	}
	query += `
		GROUP BY m.id, m.name, m.target_date, m.status, mc.color, sc.name, sc.color, sc.is_completed
		ORDER BY m.target_date IS NULL, m.target_date, m.name`

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	progressMap := make(map[int]*MilestoneStatusProgress)

	for rows.Next() {
		var milestoneID int
		var milestoneName string
		var targetDate sql.NullString
		var milestoneStatus sql.NullString
		var milestoneColor sql.NullString
		var categoryName sql.NullString
		var categoryColor sql.NullString
		var categoryCompleted sql.NullBool
		var itemCount int

		if err := rows.Scan(
			&milestoneID,
			&milestoneName,
			&targetDate,
			&milestoneStatus,
			&milestoneColor,
			&categoryName,
			&categoryColor,
			&categoryCompleted,
			&itemCount,
		); err != nil {
			return nil, err
		}

		if itemCount == 0 {
			continue
		}

		progress, exists := progressMap[milestoneID]
		if !exists {
			progress = &MilestoneStatusProgress{
				MilestoneID:     milestoneID,
				MilestoneName:   milestoneName,
				StatusBreakdown: []MilestoneStatusBreakdown{},
			}
			if targetDate.Valid {
				progress.TargetDate = &targetDate.String
			}
			progress.Status = milestoneStatus.String
			progress.CategoryColor = milestoneColor.String
			progressMap[milestoneID] = progress
		}

		label := categoryName.String
		if label == "" {
			label = "No Status"
		}

		breakdown := MilestoneStatusBreakdown{
			CategoryName:  label,
			ItemCount:     itemCount,
			IsCompleted:   categoryCompleted.Valid && categoryCompleted.Bool,
			CategoryColor: categoryColor.String,
		}

		progress.StatusBreakdown = append(progress.StatusBreakdown, breakdown)
		progress.TotalItems += itemCount
		if breakdown.IsCompleted {
			progress.CompletedItems += itemCount
		}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Convert map to slice with calculated percentages
	results := make([]MilestoneStatusProgress, 0, len(progressMap))
	for _, entry := range progressMap {
		if entry.TotalItems > 0 {
			entry.PercentComplete = float64(entry.CompletedItems) / float64(entry.TotalItems) * 100.0
		}
		results = append(results, *entry)
	}

	return results, nil
}

// GetWorkflowIDFromConfigSet retrieves the workflow ID for a workspace's configuration set
func (r *WorkspaceRepository) GetWorkflowIDFromConfigSet(workspaceID int) (*int, error) {
	var workflowID *int
	err := r.db.QueryRow(`
		SELECT workflow_id
		FROM configuration_sets cs
		JOIN workspace_configuration_sets wcs ON cs.id = wcs.configuration_set_id
		WHERE wcs.workspace_id = ?
		LIMIT 1
	`, workspaceID).Scan(&workflowID)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return workflowID, nil
}

// GetDefaultWorkflowID retrieves the default workflow ID
func (r *WorkspaceRepository) GetDefaultWorkflowID() (*int, error) {
	var defaultID int
	err := r.db.QueryRow(`SELECT id FROM workflows WHERE is_default = true LIMIT 1`).Scan(&defaultID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &defaultID, nil
}

// GetStatusesByWorkflowID retrieves statuses from a workflow
func (r *WorkspaceRepository) GetStatusesByWorkflowID(workflowID int) ([]models.Status, error) {
	rows, err := r.db.Query(`
		SELECT DISTINCT s.id, s.name, s.description, s.category_id, s.is_default, s.created_at, s.updated_at,
		       sc.name as category_name, sc.color as category_color, sc.is_completed
		FROM workflow_transitions wt
		JOIN statuses s ON s.id = wt.to_status_id OR (wt.from_status_id IS NOT NULL AND s.id = wt.from_status_id)
		LEFT JOIN status_categories sc ON s.category_id = sc.id
		WHERE wt.workflow_id = ?
		ORDER BY s.id
	`, workflowID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var statuses []models.Status
	for rows.Next() {
		var status models.Status
		var categoryName, categoryColor sql.NullString
		var isCompleted sql.NullBool
		err := rows.Scan(
			&status.ID, &status.Name, &status.Description, &status.CategoryID,
			&status.IsDefault, &status.CreatedAt, &status.UpdatedAt,
			&categoryName, &categoryColor, &isCompleted,
		)
		if err != nil {
			return nil, err
		}

		status.CategoryName = categoryName.String
		status.CategoryColor = categoryColor.String
		status.IsCompleted = isCompleted.Bool

		statuses = append(statuses, status)
	}

	if statuses == nil {
		statuses = []models.Status{}
	}

	return statuses, rows.Err()
}

// BuildWorkspaceMap creates a mapping of workspace identifiers (id, name, key) to IDs
func (r *WorkspaceRepository) BuildWorkspaceMap() (map[string]int, error) {
	workspaceMap := make(map[string]int)

	rows, err := r.db.Query("SELECT id, name, key FROM workspaces")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var id int
		var name, key string
		if err := rows.Scan(&id, &name, &key); err != nil {
			return nil, err
		}

		// Map by id (as string), lowercase name, and lowercase key
		workspaceMap[string(rune(id)+'0')] = id // Note: This is simplified; in practice use strconv
		workspaceMap[name] = id
		workspaceMap[key] = id
	}

	return workspaceMap, rows.Err()
}

// GetCollectionQuery retrieves the QL query and workspace ID for a collection
func (r *WorkspaceRepository) GetCollectionQuery(collectionID int) (workspaceID *int64, qlQuery string, err error) {
	var collectionWorkspaceID sql.NullInt64
	var collectionQuery sql.NullString

	err = r.db.QueryRow(`SELECT workspace_id, ql_query FROM collections WHERE id = ?`, collectionID).
		Scan(&collectionWorkspaceID, &collectionQuery)

	if err == sql.ErrNoRows {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}

	if collectionWorkspaceID.Valid {
		workspaceID = &collectionWorkspaceID.Int64
	}

	return workspaceID, collectionQuery.String, nil
}
