package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
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

// workspaceSelectBase is the common SELECT columns for workspace queries with counts.
const workspaceSelectBase = `SELECT w.id, w.name, w.key, w.description, w.active, w.time_project_id, w.is_personal, w.owner_id, w.icon, w.color, w.avatar_url, w.default_view, w.display_mode, w.internal_comments_enabled, w.created_at, w.updated_at,
       COUNT(p.id) as project_count,
       tp.name as time_project_name`

const workspaceFromJoinsBase = ` FROM workspaces w
LEFT JOIN projects p ON w.id = p.workspace_id
LEFT JOIN time_projects tp ON w.time_project_id = tp.id`

const workspaceGroupByBase = ` GROUP BY w.id, w.name, w.key, w.description, w.active, w.time_project_id, w.is_personal, w.owner_id, w.icon, w.color, w.avatar_url, w.default_view, w.display_mode, w.internal_comments_enabled, w.created_at, w.updated_at, tp.name`

// scanWorkspaceBase scans a standard workspace row (17 columns) and applies nullable fields.
func scanWorkspaceBase(s interface{ Scan(dest ...any) error }) (models.Workspace, error) {
	var ws models.Workspace
	var icon, color, defaultView, displayMode, timeProjectName sql.NullString
	err := s.Scan(&ws.ID, &ws.Name, &ws.Key, &ws.Description,
		&ws.Active, &ws.TimeProjectID, &ws.IsPersonal, &ws.OwnerID,
		&icon, &color, &ws.AvatarURL, &defaultView, &displayMode,
		&ws.InternalCommentsEnabled,
		&ws.CreatedAt, &ws.UpdatedAt, &ws.ProjectCount, &timeProjectName)
	if err != nil {
		return ws, err
	}
	ws.Icon = icon.String
	ws.Color = color.String
	ws.DefaultView = defaultView.String
	ws.TimeProjectName = timeProjectName.String
	return ws, nil
}

// IDKey is a (id, key) pair used by WorkspaceKeyCache to resolve URL path
// parameters that may be either numeric IDs or human-readable workspace keys.
type IDKey struct {
	ID  int
	Key string
}

// ListNameKeyToIDMap returns a lower-cased mapping of both each workspace's
// name and its key to the workspace ID. Used by the asset CQL evaluator to
// resolve workspace identifiers in user-authored queries; both name and key
// are accepted because reports were originally written against names but
// admins moved to keys for stability.
func (r *WorkspaceRepository) ListNameKeyToIDMap() (map[string]int, error) {
	rows, err := r.db.Query("SELECT id, name, key FROM workspaces")
	if err != nil {
		return nil, fmt.Errorf("list workspace name/key map: %w", err)
	}
	defer func() { _ = rows.Close() }()

	m := make(map[string]int)
	for rows.Next() {
		var id int
		var name, key string
		if err := rows.Scan(&id, &name, &key); err != nil {
			return nil, fmt.Errorf("scan workspace name/key row: %w", err)
		}
		m[strings.ToLower(name)] = id
		m[strings.ToLower(key)] = id
	}
	return m, nil
}

// ListActiveIDs returns the IDs of every workspace where active = true.
//
// Callers that need a per-user permission filter (e.g. item.view) should
// pair this with permission_service.HasWorkspacePermission per ID.
func (r *WorkspaceRepository) ListActiveIDs() ([]int, error) {
	rows, err := r.db.Query("SELECT id FROM workspaces WHERE active = true")
	if err != nil {
		return nil, fmt.Errorf("list active workspace ids: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan workspace id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ListActiveIDKeys returns active workspace id+key pairs.
func (r *WorkspaceRepository) ListActiveIDKeys() ([]IDKey, error) {
	rows, err := r.db.Query("SELECT id, key FROM workspaces WHERE active = true")
	if err != nil {
		return nil, fmt.Errorf("list active workspace id+keys: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var pairs []IDKey
	for rows.Next() {
		var p IDKey
		if err := rows.Scan(&p.ID, &p.Key); err != nil {
			return nil, fmt.Errorf("scan active workspace id+key: %w", err)
		}
		pairs = append(pairs, p)
	}
	return pairs, rows.Err()
}

// ListIDKeys returns every workspace's id+key pair, regardless of user
// scope. Used by the workspace key cache to populate its in-memory map.
// GetKey returns a workspace key by id.
func (r *WorkspaceRepository) GetKey(id int) (string, error) {
	var key string
	err := r.db.QueryRow(`SELECT key FROM workspaces WHERE id = ?`, id).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return key, err
}

func (r *WorkspaceRepository) ListIDKeys() ([]IDKey, error) {
	rows, err := r.db.Query("SELECT id, key FROM workspaces")
	if err != nil {
		return nil, fmt.Errorf("list workspace id+keys: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var pairs []IDKey
	for rows.Next() {
		var p IDKey
		if err := rows.Scan(&p.ID, &p.Key); err != nil {
			return nil, fmt.Errorf("scan workspace id+key: %w", err)
		}
		pairs = append(pairs, p)
	}
	return pairs, nil
}

// FindByID retrieves a workspace by ID with project count and time project name
func (r *WorkspaceRepository) FindByID(id int) (*models.Workspace, error) {
	var workspace models.Workspace
	var timeProjectName, icon, color, defaultView, displayMode sql.NullString
	var configSetID sql.NullInt64

	err := r.db.QueryRow(workspaceSelectBase+`,
		       wcs.configuration_set_id`+workspaceFromJoinsBase+`
		LEFT JOIN workspace_configuration_sets wcs ON w.id = wcs.workspace_id
		WHERE w.id = ?`+workspaceGroupByBase+`, wcs.configuration_set_id
	`, id).Scan(&workspace.ID, &workspace.Name, &workspace.Key, &workspace.Description,
		&workspace.Active, &workspace.TimeProjectID, &workspace.IsPersonal, &workspace.OwnerID,
		&icon, &color, &workspace.AvatarURL, &defaultView, &displayMode,
		&workspace.InternalCommentsEnabled,
		&workspace.CreatedAt, &workspace.UpdatedAt,
		&workspace.ProjectCount, &timeProjectName, &configSetID)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	workspace.Icon = icon.String
	workspace.Color = color.String
	workspace.DefaultView = defaultView.String
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
		SELECT id, name, key, description, active, is_personal, icon, color, internal_comments_enabled
		FROM workspaces
		WHERE id = ?
	`, id).Scan(&workspace.ID, &workspace.Name, &workspace.Key, &workspace.Description,
		&workspace.Active, &workspace.IsPersonal, &icon, &color, &workspace.InternalCommentsEnabled)

	if errors.Is(err, sql.ErrNoRows) {
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
		query = workspaceSelectBase + workspaceFromJoinsBase +
			` WHERE w.is_personal = ? AND w.owner_id = ?` + workspaceGroupByBase +
			` ORDER BY w.name`
		rows, err = r.db.Query(query, true, userID)
	} else {
		query = workspaceSelectBase + workspaceFromJoinsBase +
			` WHERE w.is_personal = false OR w.is_personal IS NULL OR w.owner_id = ?` + workspaceGroupByBase +
			` ORDER BY w.is_personal ASC, w.name`
		rows, err = r.db.Query(query, userID)
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var workspaces []models.Workspace
	for rows.Next() {
		workspace, scanErr := scanWorkspaceBase(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		workspaces = append(workspaces, workspace)
	}

	return workspaces, rows.Err()
}

// CreateTx inserts a new workspace within the given transaction and returns its ID.
// GrantAdministratorRoleTx grants the Administrator role on a workspace to a user within a transaction.
func (r *WorkspaceRepository) GrantAdministratorRoleTx(tx database.Tx, workspaceID int64, userID int) error {
	result, err := tx.Exec(`
		INSERT INTO user_workspace_roles (workspace_id, user_id, role_id, granted_by, granted_at)
		SELECT ?, ?, id, ?, CURRENT_TIMESTAMP FROM workspace_roles WHERE name = 'Administrator'
	`, workspaceID, userID, userID)
	if err != nil {
		return fmt.Errorf("failed to grant admin role to workspace creator: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("administrator role not found; workspace creation aborted")
	}
	return nil
}

func (r *WorkspaceRepository) CreateTx(tx database.Tx, workspace *models.Workspace) (int64, error) {
	now := time.Now()
	var id int64

	err := tx.QueryRow(`
		INSERT INTO workspaces (name, key, description, active, time_project_id, is_personal, owner_id, icon, color, avatar_url, default_view, display_mode, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id
	`, workspace.Name, workspace.Key, workspace.Description, workspace.Active,
		workspace.TimeProjectID, workspace.IsPersonal, workspace.OwnerID,
		workspace.Icon, workspace.Color, workspace.AvatarURL, workspace.DefaultView, "default",
		now, now).Scan(&id)

	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return 0, ErrDuplicateEntry
		}
		return 0, fmt.Errorf("failed to create workspace: %w", err)
	}

	return id, nil
}

// Update updates an existing workspace
func (r *WorkspaceRepository) Update(workspace *models.Workspace) error {
	now := time.Now()
	_, err := r.db.ExecWrite(`
		UPDATE workspaces
		SET name = ?, key = ?, description = ?, active = ?, time_project_id = ?, is_personal = ?, owner_id = ?, icon = ?, color = ?, avatar_url = ?, default_view = ?, internal_comments_enabled = ?, updated_at = ?
		WHERE id = ?
	`, workspace.Name, workspace.Key, workspace.Description, workspace.Active,
		workspace.TimeProjectID, workspace.IsPersonal, workspace.OwnerID,
		workspace.Icon, workspace.Color, workspace.AvatarURL, workspace.DefaultView,
		workspace.InternalCommentsEnabled,
		now, workspace.ID)

	return err
}

// Delete removes a workspace by ID
func (r *WorkspaceRepository) Delete(id int) error {
	_, err := r.db.ExecWrite("DELETE FROM workspaces WHERE id = ?", id)
	return err
}

// Exists checks if a workspace exists
// FindMissingOrPersonal accepts a set of workspace IDs and returns those that
// don't exist or are flagged as personal — i.e. invalid as portal/form
// submission targets. Used by UpdateChannelConfig to reject bogus IDs before
// they end up in the routing config. Returns nil/empty when all IDs are
// valid non-personal workspaces.
func (r *WorkspaceRepository) FindMissingOrPersonal(ids []int) ([]int, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	query := `SELECT id, is_personal FROM workspaces WHERE id IN (` + strings.Join(placeholders, ",") + `)`
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query workspace eligibility: %w", err)
	}
	defer func() { _ = rows.Close() }()

	usable := make(map[int]bool, len(ids))
	for rows.Next() {
		var (
			id         int
			isPersonal bool
		)
		if err := rows.Scan(&id, &isPersonal); err != nil {
			return nil, fmt.Errorf("scan workspace eligibility: %w", err)
		}
		if !isPersonal {
			usable[id] = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspace eligibility: %w", err)
	}

	var bad []int
	for _, id := range ids {
		if !usable[id] {
			bad = append(bad, id)
		}
	}
	return bad, nil
}

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

// ListActivePersonalWorkspaceIDs returns active personal workspace ids owned by userID.
func (r *WorkspaceRepository) ListActivePersonalWorkspaceIDs(userID int) ([]int, error) {
	rows, err := r.db.Query("SELECT id FROM workspaces WHERE is_personal = true AND owner_id = ? AND active = true", userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	ids := []int{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// GetActivePersonalWorkspaceID returns the active personal workspace owned by userID.
func (r *WorkspaceRepository) GetActivePersonalWorkspaceID(userID int) (int, error) {
	var id int
	err := r.db.QueryRow(`
		SELECT id FROM workspaces
		WHERE is_personal = ? AND owner_id = ? AND active = ?
	`, true, userID, true).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("get active personal workspace for user %d: %w", userID, err)
	}
	return id, nil
}

// CountNonPersonal returns the number of non-personal workspaces.
// Rows where is_personal is NULL are treated as non-personal.
func (r *WorkspaceRepository) CountNonPersonal() (int, error) {
	var count int
	err := r.db.QueryRow(
		`SELECT COUNT(*) FROM workspaces WHERE is_personal = false OR is_personal IS NULL`,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count non-personal workspaces: %w", err)
	}
	return count, nil
}

// WorkspaceBasic carries the minimal workspace fields needed for activity
// widgets (id, name, key, icon, color). Use FindBasicsByIDs to load many at
// once.
type WorkspaceBasic struct {
	ID    int
	Name  string
	Key   string
	Icon  string
	Color string
}

// FindBasicsByIDs returns basic workspace metadata for the given IDs.
// Inactive workspaces and missing IDs are silently omitted (this method
// powers activity widgets where inactives shouldn't surface). Order is
// not guaranteed — callers should index by ID.
func (r *WorkspaceRepository) FindBasicsByIDs(ids []int) ([]WorkspaceBasic, error) {
	if len(ids) == 0 {
		return []WorkspaceBasic{}, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	query := `SELECT id, name, key, icon, color FROM workspaces WHERE active = true AND id IN (` +
		strings.Join(placeholders, ",") + `)`
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query workspace basics: %w", err)
	}
	defer func() { _ = rows.Close() }()

	results := make([]WorkspaceBasic, 0, len(ids))
	for rows.Next() {
		var wb WorkspaceBasic
		var icon, color sql.NullString
		if err := rows.Scan(&wb.ID, &wb.Name, &wb.Key, &icon, &color); err != nil {
			return nil, fmt.Errorf("scan workspace basic: %w", err)
		}
		wb.Icon = icon.String
		wb.Color = color.String
		results = append(results, wb)
	}
	return results, rows.Err()
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

// ProjectStats represents statistics for a specific project
type ProjectStats struct {
	ProjectID         *int
	ProjectName       string
	ProjectColor      string
	ItemCount         int
	CompletedCount    int
	CompletionPercent float64
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
		JOIN item_milestones im ON im.item_id = i.id
		JOIN milestones m ON m.id = im.milestone_id
		LEFT JOIN milestone_categories mc ON m.category_id = mc.id
		LEFT JOIN statuses s ON i.status_id = s.id
		LEFT JOIN status_categories sc ON s.category_id = sc.id
		WHERE i.workspace_id = ?
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

		label := strings.TrimSpace(categoryName.String)
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

	if len(progressMap) == 0 {
		return []MilestoneStatusProgress{}, nil
	}

	// Build a deterministic order: upcoming target date first, then name
	keys := make([]int, 0, len(progressMap))
	for id := range progressMap {
		keys = append(keys, id)
	}
	sort.Slice(keys, func(i, j int) bool {
		left := progressMap[keys[i]]
		right := progressMap[keys[j]]

		if left.TargetDate == nil && right.TargetDate != nil {
			return false
		}
		if left.TargetDate != nil && right.TargetDate == nil {
			return true
		}
		if left.TargetDate != nil && right.TargetDate != nil && *left.TargetDate != *right.TargetDate {
			return *left.TargetDate < *right.TargetDate
		}
		return strings.ToLower(left.MilestoneName) < strings.ToLower(right.MilestoneName)
	})

	results := make([]MilestoneStatusProgress, 0, len(progressMap))
	for _, id := range keys {
		entry := progressMap[id]
		if entry.TotalItems > 0 {
			entry.PercentComplete = float64(entry.CompletedItems) / float64(entry.TotalItems) * 100.0
		}

		// Order breakdown by count desc
		sort.SliceStable(entry.StatusBreakdown, func(i, j int) bool {
			if entry.StatusBreakdown[i].ItemCount == entry.StatusBreakdown[j].ItemCount {
				return strings.ToLower(entry.StatusBreakdown[i].CategoryName) < strings.ToLower(entry.StatusBreakdown[j].CategoryName)
			}
			return entry.StatusBreakdown[i].ItemCount > entry.StatusBreakdown[j].ItemCount
		})

		results = append(results, *entry)
	}

	return results, nil
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
		workspaceMap[strconv.Itoa(id)] = id
		workspaceMap[name] = id
		workspaceMap[key] = id
	}

	return workspaceMap, rows.Err()
}

// GetHomepageLayoutJSON returns a workspace homepage layout JSON blob.
func (r *WorkspaceRepository) GetHomepageLayoutJSON(workspaceID int) (string, error) {
	var homepageLayout sql.NullString
	err := r.db.QueryRow(`
		SELECT homepage_layout
		FROM workspaces
		WHERE id = ?
	`, workspaceID).Scan(&homepageLayout)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get homepage layout for workspace %d: %w", workspaceID, err)
	}
	if !homepageLayout.Valid {
		return "", nil
	}
	return homepageLayout.String, nil
}

// UpdateHomepageLayoutJSON updates a workspace homepage layout JSON blob.
func (r *WorkspaceRepository) UpdateHomepageLayoutJSON(workspaceID int, layoutJSON string, updatedAt time.Time) error {
	res, err := r.db.Exec(`
		UPDATE workspaces
		SET homepage_layout = ?, updated_at = ?
		WHERE id = ?
	`, layoutJSON, updatedAt, workspaceID)
	if err != nil {
		return fmt.Errorf("update homepage layout for workspace %d: %w", workspaceID, err)
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return ErrNotFound
	}
	return nil
}

// CountCollections returns the number of collections scoped to a workspace.
func (r *WorkspaceRepository) CountCollections(workspaceID int) (int, error) {
	var count int
	if err := r.db.QueryRow(`
		SELECT COUNT(*)
		FROM collections
		WHERE workspace_id = ?
	`, workspaceID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count collections for workspace %d: %w", workspaceID, err)
	}
	return count, nil
}

// GetCollectionQuery retrieves the QL query and workspace ID for a collection
func (r *WorkspaceRepository) GetCollectionQuery(collectionID int) (workspaceID *int64, qlQuery string, err error) {
	var collectionWorkspaceID sql.NullInt64
	var collectionQuery sql.NullString

	err = r.db.QueryRow(`SELECT workspace_id, ql_query FROM collections WHERE id = ?`, collectionID).
		Scan(&collectionWorkspaceID, &collectionQuery)

	if errors.Is(err, sql.ErrNoRows) {
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

// CreateItemSequence ensures a per-workspace item-number sequence exists.
// PostgreSQL uses a real sequence so nextval() can produce gap-free numbers
// concurrently; SQLite falls back to MAX(workspace_item_number)+1 in
// ItemRepository.GetNextWorkspaceItemNumber, so this is a no-op there.
func (r *WorkspaceRepository) CreateItemSequence(workspaceID int64) error {
	if r.db.GetDriverName() != "postgres" {
		return nil
	}
	seqName := fmt.Sprintf("workspace_%d_item_seq", workspaceID)
	// Sequence names contain only digits (workspace ID); pq.QuoteIdentifier
	// is the canonical sanitizer but quoting a digits-only name is a no-op,
	// so a plain interpolation is safe here.
	_, err := r.db.Exec(fmt.Sprintf(`CREATE SEQUENCE IF NOT EXISTS %q START 1`, seqName))
	return err
}

// DropItemSequence removes the per-workspace sequence on workspace deletion.
// No-op on SQLite.
func (r *WorkspaceRepository) DropItemSequence(workspaceID int64) error {
	if r.db.GetDriverName() != "postgres" {
		return nil
	}
	seqName := fmt.Sprintf("workspace_%d_item_seq", workspaceID)
	_, err := r.db.Exec(fmt.Sprintf(`DROP SEQUENCE IF EXISTS %q`, seqName))
	return err
}
