package services

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// PlanningService encapsulates business logic for milestones, iterations, and projects.
type PlanningService struct {
	db       database.Database
	items    *repository.ItemRepository
	statuses *repository.StatusRepository
	scmRepos *repository.SCMWorkspaceRepository
}

// milestonePositionStep is the gap left between adjacent positions so that a
// single insert between two milestones rarely needs a full renormalization.
// Matches the test-folder reorder convention (1000).
const milestonePositionStep = 1000

var ErrInvalidMilestoneReorder = errors.New("invalid milestone reorder")
var ErrInvalidPlanningScope = errors.New("invalid planning scope")

func validatePlanningScope(isGlobal bool, workspaceID *int) error {
	if isGlobal && workspaceID != nil {
		return planningValidationErrorWithCause("workspace_id", "global planning objects cannot have a workspace", ErrInvalidPlanningScope)
	}
	if !isGlobal && workspaceID == nil {
		return planningValidationErrorWithCause("workspace_id", "local planning objects require a workspace", ErrInvalidPlanningScope)
	}
	return nil
}

// milestoneScopeClause builds the SQL WHERE fragment that pins a milestone
// to its reorder scope. Position is scoped per (is_global, workspace_id,
// category_id); COALESCE normalizes NULL workspace_id / category_id to 0 so
// every row in the same logical group compares equal. The fragment uses ?
// placeholders consumed by the scope's parameters (returned by
// milestoneScopeArgs) — callers must append those args in the same order.
func milestoneScopeClause() string {
	return "is_global = ? AND COALESCE(workspace_id, 0) = ? AND COALESCE(category_id, 0) = ?"
}

// milestoneScopeArgs returns the args for milestoneScopeClause for a scope.
func milestoneScopeArgs(isGlobal bool, workspaceID, categoryID *int) []any {
	ws := 0
	if workspaceID != nil {
		ws = *workspaceID
	}
	cat := 0
	if categoryID != nil {
		cat = *categoryID
	}
	return []any{isGlobal, ws, cat}
}

// SetSCMWorkspaceRepository wires the SCM persistence layer used by release
// lookups. Optional: when unset, calls build it lazily from db.
func (s *PlanningService) SetSCMWorkspaceRepository(repo *repository.SCMWorkspaceRepository) {
	s.scmRepos = repo
}

func (s *PlanningService) scmRepository() *repository.SCMWorkspaceRepository {
	if s.scmRepos == nil {
		s.scmRepos = repository.NewSCMWorkspaceRepository(s.db)
	}
	return s.scmRepos
}

// NewPlanningService creates a new PlanningService.
func NewPlanningService(db database.Database) *PlanningService {
	return &PlanningService{
		db:       db,
		items:    repository.NewItemRepository(db),
		statuses: repository.NewStatusRepository(db),
	}
}

// milestoneScanner is satisfied by both *sql.Row and *sql.Rows.
type milestoneScanner interface {
	Scan(dest ...any) error
}

// scanMilestoneRow scans a single milestone row (with LEFT JOIN release columns)
// into a MilestoneResult. The column order must match the standard milestone query.
func scanMilestoneRow(sc milestoneScanner) (MilestoneResult, error) {
	var m MilestoneResult
	var description, targetDate, categoryName, categoryColor, workspaceName, externalKey sql.NullString
	var categoryID, workspaceID sql.NullInt64
	// Release columns
	var mrID, mrCreatedBy, mrSCMConnectionID, mrWorkspaceRepositoryID sql.NullInt64
	var mrTagName, mrName, mrBody, mrTargetCommitish, mrTagURL sql.NullString
	var mrSCMRepository, mrSCMReleaseID, mrSCMReleaseURL sql.NullString
	var mrReleaseStatus, mrReleasedAt, mrAssetsJSON, mrLastSyncedAt sql.NullString
	var mrIsDraft, mrIsPrerelease sql.NullBool
	var mrCreatedAt sql.NullString

	var lastActivityAt sql.NullTime

	err := sc.Scan(&m.ID, &m.Name, &description, &targetDate, &m.Status, &categoryID,
		&categoryName, &categoryColor, &m.IsGlobal, &workspaceID, &workspaceName,
		&externalKey, &m.Position,
		&mrID, &mrTagName, &mrName, &mrBody, &mrIsDraft, &mrIsPrerelease,
		&mrTargetCommitish, &mrWorkspaceRepositoryID, &mrTagURL, &mrReleaseStatus,
		&mrReleasedAt, &mrAssetsJSON, &mrLastSyncedAt, &mrSCMConnectionID, &mrSCMRepository,
		&mrSCMReleaseID, &mrSCMReleaseURL, &mrCreatedBy, &mrCreatedAt,
		&m.CreatedAt, &m.UpdatedAt, &lastActivityAt)
	if err != nil {
		return m, err
	}

	if lastActivityAt.Valid {
		at := lastActivityAt.Time.UTC()
		m.LastActivityAt = &at
	}
	m.Description = description.String
	m.TargetDate = targetDate.String
	m.CategoryName = categoryName.String
	m.CategoryColor = categoryColor.String
	m.WorkspaceName = workspaceName.String
	if externalKey.Valid {
		ek := externalKey.String
		m.ExternalKey = &ek
	}
	if categoryID.Valid {
		id := int(categoryID.Int64)
		m.CategoryID = &id
	}
	if workspaceID.Valid {
		id := int(workspaceID.Int64)
		m.WorkspaceID = &id
	}
	m.LatestRelease = hydrateMilestoneRelease(m.ID,
		mrID, mrCreatedBy, mrSCMConnectionID, mrWorkspaceRepositoryID,
		mrTagName, mrName, mrBody, mrTargetCommitish,
		mrTagURL, mrReleaseStatus, mrReleasedAt, mrAssetsJSON, mrLastSyncedAt,
		mrSCMRepository, mrSCMReleaseID, mrSCMReleaseURL,
		mrIsDraft, mrIsPrerelease, mrCreatedAt,
	)

	return m, nil
}

// scanMilestones scans all rows from a milestone query into a slice. A scan or
// iteration failure is returned rather than silently dropping rows, so callers
// never mistake an incomplete read for a complete empty result.
func scanMilestones(rows *sql.Rows) ([]MilestoneResult, error) {
	var milestones []MilestoneResult
	for rows.Next() {
		m, err := scanMilestoneRow(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan milestone row: %w", err)
		}
		milestones = append(milestones, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate milestones: %w", err)
	}
	if milestones == nil {
		milestones = []MilestoneResult{}
	}
	return milestones, nil
}

// MilestoneReleaseResult represents a release record for a milestone.
type MilestoneReleaseResult struct {
	ID                    int
	MilestoneID           int
	WorkspaceRepositoryID *int
	TagName               string
	TagURL                *string
	ReleaseStatus         string
	ReleasedAt            *string
	Assets                []models.SCMReleaseAsset
	LastSyncedAt          *string
	Name                  string
	Body                  string
	IsDraft               bool
	IsPrerelease          bool
	TargetCommitish       string
	SCMConnectionID       *int
	SCMRepository         *string
	SCMReleaseID          *string
	SCMReleaseURL         *string
	CreatedBy             *int
	CreatedAt             string
}

var ErrSCMRepositoryNotLinked = errors.New("SCM repository is not linked to the selected connection")
var ErrMilestoneReleaseInProgress = errors.New("milestone release is already in progress")
var ErrMilestoneReleaseIdempotencyConflict = errors.New("milestone release idempotency key was already used with different parameters")

// LinkedSCMRepository is the canonical repository identity resolved from a
// workspace connection. Release callers use the stored name rather than a
// caller-provided owner/repository string.
type LinkedSCMRepository struct {
	ID             int
	RepositoryName string
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
	ExternalKey   *string
	Position      int
	LatestRelease *MilestoneReleaseResult
	Releases      []MilestoneReleaseResult
	CreatedAt     time.Time
	UpdatedAt     time.Time
	// LastActivityAt is the time of the newest Activity entry: a milestone
	// change, comment, or page link, or an item added, removed, changing
	// status, or commented on. Nil until the first activity.
	LastActivityAt *time.Time
}

// milestoneOrderByClause returns the ORDER BY clause for ListMilestones.
// With no client sort requested (SortBy empty), milestones use their manual
// drag-and-drop order (position), with name as a stable tiebreaker. When the
// client requests an explicit sort, that column wins; unknown sort keys fall
// back to the manual order to avoid injecting arbitrary SQL.
func milestoneOrderByClause(sortBy, sortOrder string) string {
	dir := "ASC"
	if strings.EqualFold(sortOrder, "desc") {
		dir = "DESC"
	}
	switch sortBy {
	case "position":
		return " ORDER BY m.position " + dir + ", m.name ASC, m.id ASC"
	case "name":
		return " ORDER BY m.name " + dir + ", m.position ASC, m.id ASC"
	case "target_date":
		// NULL dates sort last in ascending order on both backends via the
		// IS NULL tiebreaker, keeping behavior consistent.
		return " ORDER BY m.target_date IS NULL ASC, m.target_date " + dir + ", m.name ASC, m.position ASC, m.id ASC"
	case "status":
		return " ORDER BY m.status " + dir + ", m.position ASC, m.id ASC"
	case "created_at", "updated_at":
		return " ORDER BY m." + sortBy + " " + dir + ", m.position ASC, m.id ASC"
	default:
		return " ORDER BY m.position ASC, m.name ASC, m.id ASC"
	}
}

// MilestoneListParams contains parameters for listing milestones.
type MilestoneListParams struct {
	Limit         int
	Offset        int
	WorkspaceID   *int   // Filter by workspace
	WorkspaceIDs  []int  // Caller-visible local workspaces for an unscoped list
	CategoryID    *int   // Filter by category
	Status        string // Filter by status
	IncludeGlobal bool   // Include global milestones
	IsGlobal      bool   // Restricts an unscoped list to global milestones only
	// SortBy overrides the default manual-position ordering. When empty,
	// results are ordered by position then name (the drag-and-drop order).
	// When set (e.g. "name", "target_date", "status"), the client sort wins.
	SortBy string
	// SortOrder is "asc" or "desc"; defaults to "asc" when SortBy is set.
	SortOrder string
}

// ListMilestones retrieves milestones with pagination and filtering.
func (s *PlanningService) ListMilestones(params MilestoneListParams) ([]MilestoneResult, int, error) {
	params.Status = normalizePlanningStatus(params.Status)
	list := newPlanningListQuery(milestoneSelectQuery+"\nWHERE 1=1", "SELECT COUNT(*) FROM milestones m WHERE 1=1")
	list.addWorkspaceScope("m.workspace_id", "m.is_global", params.WorkspaceID, params.WorkspaceIDs, params.IncludeGlobal)
	list.addNullableIDFilter("m.category_id", params.CategoryID)
	list.addStringFilter("m.status", params.Status)
	query, args := list.paginated(milestoneOrderByClause(params.SortBy, params.SortOrder), params.Limit, params.Offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list milestones: %w", err)
	}
	defer rows.Close()

	milestones, err := scanMilestones(rows)
	if err != nil {
		return nil, 0, err
	}

	var total int
	if err := s.db.QueryRow(list.countQuery, list.countArgs...).Scan(&total); err != nil {
		slog.Warn("failed to get milestone pagination count", slog.Any("error", err))
	}

	return milestones, total, nil
}

// GetMilestone retrieves a milestone by ID.
func (s *PlanningService) GetMilestone(id int) (*MilestoneResult, error) {
	row := s.db.QueryRow(milestoneSelectQuery+"\nWHERE m.id = ?", id)

	m, err := scanMilestoneRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("milestone not found: %d: %w", id, repository.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get milestone: %w", err)
	}

	return &m, nil
}

// hydrateMilestoneRelease builds a MilestoneReleaseResult from nullable scan variables.
// Returns nil if mrID is not valid (no release row).
func hydrateMilestoneRelease(
	milestoneID int,
	mrID, mrCreatedBy, mrSCMConnectionID, mrWorkspaceRepositoryID sql.NullInt64,
	mrTagName, mrName, mrBody, mrTargetCommitish sql.NullString,
	mrTagURL, mrReleaseStatus, mrReleasedAt, mrAssetsJSON, mrLastSyncedAt sql.NullString,
	mrSCMRepository, mrSCMReleaseID, mrSCMReleaseURL sql.NullString,
	mrIsDraft, mrIsPrerelease sql.NullBool,
	mrCreatedAt sql.NullString,
) *MilestoneReleaseResult {
	if !mrID.Valid {
		return nil
	}
	rel := &MilestoneReleaseResult{
		ID:              int(mrID.Int64),
		MilestoneID:     milestoneID,
		TagName:         mrTagName.String,
		Name:            mrName.String,
		Body:            mrBody.String,
		CreatedAt:       mrCreatedAt.String,
		TargetCommitish: mrTargetCommitish.String,
	}
	if mrWorkspaceRepositoryID.Valid {
		id := int(mrWorkspaceRepositoryID.Int64)
		rel.WorkspaceRepositoryID = &id
	}
	if mrTagURL.Valid {
		rel.TagURL = &mrTagURL.String
	}
	rel.ReleaseStatus = mrReleaseStatus.String
	if mrReleasedAt.Valid {
		rel.ReleasedAt = &mrReleasedAt.String
	}
	if mrLastSyncedAt.Valid {
		rel.LastSyncedAt = &mrLastSyncedAt.String
	}
	if mrAssetsJSON.Valid && mrAssetsJSON.String != "" {
		_ = json.Unmarshal([]byte(mrAssetsJSON.String), &rel.Assets)
	}
	if mrIsDraft.Valid {
		rel.IsDraft = mrIsDraft.Bool
	}
	if mrIsPrerelease.Valid {
		rel.IsPrerelease = mrIsPrerelease.Bool
	}
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
	return rel
}

// FindMilestoneIDByName resolves a milestone by exact (case-insensitive) name,
// preferring the workspace's own milestone over a same-named global one.
// Returns nil when nothing matches.
func (s *PlanningService) FindMilestoneIDByName(workspaceID int, name string) (*int, error) {
	var id int
	err := s.db.QueryRow(`
		SELECT id FROM milestones
		WHERE LOWER(name) = LOWER(?) AND (workspace_id = ? OR is_global = true)
		ORDER BY CASE WHEN workspace_id = ? THEN 0 ELSE 1 END
		LIMIT 1
	`, name, workspaceID, workspaceID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("resolve milestone by name: %w", err)
	}
	return &id, nil
}

// GetSCMConnectionWorkspaceID returns the workspace_id for a given SCM connection ID.
// Returns 0 and no error if the connection doesn't exist.
func (s *PlanningService) GetSCMConnectionWorkspaceID(connectionID int) (int, error) {
	workspaceID, _, err := s.scmRepository().GetConnectionWorkspaceAndProvider(connectionID)
	if errors.Is(err, repository.ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get SCM connection workspace: %w", err)
	}
	return workspaceID, nil
}

// ResolveLinkedSCMRepository verifies that a requested release repository is
// linked to the selected workspace connection. RepositoryID is preferred; the
// name lookup remains for existing API clients. When both are supplied they
// must identify the same stored row.
func (s *PlanningService) ResolveLinkedSCMRepository(connectionID, repositoryID int, repositoryName string) (*LinkedSCMRepository, error) {
	repositoryName = strings.TrimSpace(repositoryName)
	linked, err := s.scmRepository().ResolveLinkedRepository(connectionID, repositoryID, repositoryName)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrSCMRepositoryNotLinked
	}
	if err != nil {
		return nil, fmt.Errorf("failed to resolve linked SCM repository: %w", err)
	}
	if repositoryName != "" && repositoryName != linked.RepositoryName {
		return nil, ErrSCMRepositoryNotLinked
	}
	return &LinkedSCMRepository{ID: linked.ID, RepositoryName: linked.RepositoryName}, nil
}

// CreateMilestoneParams contains parameters for creating a milestone.
// ExternalKey is a stable upsert key used by automation (e.g. the
// create_milestone action's `{{ref.short}}`); a non-empty value is unique
// per workspace (see uq_milestones_workspace_external_key).
type CreateMilestoneParams struct {
	Name        string
	Description string
	TargetDate  *string
	Status      string
	CategoryID  *int
	IsGlobal    bool
	WorkspaceID *int
	ExternalKey *string
	AuditActor  *AuditActor
}

// CreateMilestone creates a new milestone.
func (s *PlanningService) CreateMilestone(params CreateMilestoneParams) (*MilestoneResult, error) {
	if params.Status == "" {
		params.Status = "planning"
	}
	params.Status = normalizePlanningStatus(params.Status)
	if err := s.validateMilestoneMutation(params); err != nil {
		return nil, err
	}

	// New milestones land at the end of their scope's manual order. Position
	// is scoped per (is_global, workspace_id, category_id); MaxMilestonePosition
	// returns the current max for this scope, and we step by 1000 to leave
	// gaps for future inserts (mirrors the test-folder reorder convention).
	position, err := s.MaxMilestonePosition(params.IsGlobal, params.WorkspaceID, params.CategoryID)
	if err != nil {
		return nil, fmt.Errorf("failed to compute milestone position: %w", err)
	}
	position += milestonePositionStep

	var id int64
	err = s.db.QueryRow(`
		INSERT INTO milestones (name, description, target_date, status, category_id, is_global, workspace_id, external_key, position)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, params.Name, params.Description, params.TargetDate, params.Status, params.CategoryID, params.IsGlobal, params.WorkspaceID, params.ExternalKey, position).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("failed to create milestone: %w", err)
	}

	created, err := s.GetMilestone(int(id))
	if err != nil {
		return nil, err
	}
	if params.AuditActor != nil {
		resourceID := created.ID
		emitServiceAudit(s.db, *params.AuditActor, logger.ActionMilestoneCreate, logger.ResourceMilestone, &resourceID, created.Name, nil)
	}
	return created, nil
}

// FindMilestoneByExternalKey returns the milestone with the given
// (workspace_id, external_key). Returns (nil, nil) when no row matches —
// the executor treats that as "create a new one." Errors are reserved
// for actual DB failures.
func (s *PlanningService) FindMilestoneByExternalKey(workspaceID int, externalKey string) (*MilestoneResult, error) {
	if externalKey == "" {
		return nil, nil
	}
	var id int
	err := s.db.QueryRow(`
		SELECT id FROM milestones
		WHERE workspace_id = ? AND external_key = ?
	`, workspaceID, externalKey).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find milestone by external_key: %w", err)
	}
	return s.GetMilestone(id)
}

// FindMilestoneByName returns a workspace-scoped milestone with the exact
// name, or (nil, nil) when no row matches.
func (s *PlanningService) FindMilestoneByName(workspaceID int, name string) (*MilestoneResult, error) {
	var id int
	err := s.db.QueryRow(`
		SELECT id FROM milestones WHERE workspace_id = ? AND name = ?
	`, workspaceID, name).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find milestone by name: %w", err)
	}
	return s.GetMilestone(id)
}

// SetMilestoneStatus updates only the status column on a milestone scoped
// to the given workspace. Used by automation to promote a "planning"
// milestone to "in-progress" or "completed" without disturbing the other
// fields. Returns a "not found" error when no row matches the scope.
func (s *PlanningService) SetMilestoneStatus(milestoneID, workspaceID int, status string) error {
	status = normalizePlanningStatus(status)
	if !validMilestoneStatus(status) {
		return planningValidationError("status", milestoneStatusValidationMessage)
	}
	return database.WithTx(s.db, func(tx database.Tx) error {
		n, err := recordMilestoneStatusTransition(tx, milestoneID, "AND workspace_id = ?", []any{workspaceID}, status, nil)
		if err != nil {
			return fmt.Errorf("failed to set milestone status: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("milestone not found in workspace: %d: %w", milestoneID, repository.ErrNotFound)
		}
		return nil
	})
}

// AttachRelease inserts a milestone_releases row without flipping the
// parent milestone's status. The standalone ReleaseMilestone (above) sets
// status to 'completed' atomically with the insert; AttachRelease is the
// version automation uses when it wants to keep status under explicit
// control (e.g. "tag promotes branch milestone to in-progress, not
// completed").
func (s *PlanningService) AttachRelease(params ReleaseMilestoneParams) error {
	assetsJSON := releaseAssetsJSON(params.Assets)
	_, err := s.db.ExecWrite(`
		INSERT INTO milestone_releases (
			milestone_id, tag_name, name, body, is_draft, is_prerelease,
			target_commitish, workspace_repository_id, tag_url, release_status, released_at, assets_json,
			scm_connection_id, scm_repository, scm_release_id,
			scm_release_url, created_by
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING
	`, params.ID, params.TagName, params.Name, params.Body, params.IsDraft, params.IsPrerelease,
		params.TargetCommitish, params.WorkspaceRepositoryID, params.TagURL, releaseStatusOrDefault(params.ReleaseStatus), params.ReleasedAt, assetsJSON,
		params.SCMConnectionID, params.SCMRepository, params.SCMReleaseID,
		params.SCMReleaseURL, params.CreatedBy)
	if err != nil {
		return fmt.Errorf("failed to insert milestone release: %w", err)
	}
	return nil
}

// UpdateMilestoneParams contains parameters for updating a milestone.
// The scope (WorkspaceID) determines which rows the UPDATE may touch:
// nil means a global milestone (WHERE is_global = TRUE), non-nil scopes the
// UPDATE to that workspace. Cross-scope updates are impossible — the WHERE
// clause filters out milestones owned by another workspace, returning 0 rows
// affected, which surfaces as a "not found" error.
type UpdateMilestoneParams struct {
	ID          int
	Name        string
	Description string
	TargetDate  *string
	Status      string
	CategoryID  *int
	WorkspaceID *int // nil = global milestone
	AuditActor  *AuditActor
}

// UpdateMilestone updates an existing milestone within its declared scope.
// is_global / workspace_id cannot be changed via this method.
func (s *PlanningService) UpdateMilestone(params UpdateMilestoneParams) (*MilestoneResult, error) {
	params.Status = normalizePlanningStatus(params.Status)
	if err := s.validateMilestoneMutation(CreateMilestoneParams{
		Name:        params.Name,
		Description: params.Description,
		TargetDate:  params.TargetDate,
		Status:      params.Status,
		CategoryID:  params.CategoryID,
		IsGlobal:    params.WorkspaceID == nil,
		WorkspaceID: params.WorkspaceID,
	}); err != nil {
		return nil, err
	}
	scope := "is_global = true"
	scopeArgs := []any{params.ID}
	if params.WorkspaceID != nil {
		scope = "workspace_id = ? AND is_global = false"
		scopeArgs = append(scopeArgs, *params.WorkspaceID)
	}
	var actorID *int
	if params.AuditActor != nil && params.AuditActor.UserID > 0 {
		id := params.AuditActor.UserID
		actorID = &id
	}
	err := database.WithTx(s.db, func(tx database.Tx) error {
		// Read the current values in the same transaction so the milestone
		// history records exactly what this update changed.
		var oldDescription, oldTargetDate sql.NullString
		var oldStatus string
		err := tx.QueryRow(`SELECT description, target_date, status FROM milestones WHERE id = ? AND `+scope, scopeArgs...).
			Scan(&oldDescription, &oldTargetDate, &oldStatus)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("milestone not found: %d: %w", params.ID, repository.ErrNotFound)
		}
		if err != nil {
			return fmt.Errorf("failed to read milestone: %w", err)
		}
		args := append([]any{params.Name, params.Description, params.TargetDate, params.Status, params.CategoryID}, scopeArgs...)
		res, err := tx.ExecWrite(`
			UPDATE milestones SET name = ?, description = ?, target_date = ?, status = ?, category_id = ?,
			       updated_at = CURRENT_TIMESTAMP
			WHERE id = ? AND `+scope, args...)
		if err != nil {
			return fmt.Errorf("failed to update milestone: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to read update result: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("milestone not found: %d: %w", params.ID, repository.ErrNotFound)
		}
		newTargetDate := ""
		if params.TargetDate != nil {
			newTargetDate = *params.TargetDate
		}
		return repository.RecordMilestoneHistory(tx, milestoneHistoryChanges(params.ID, actorID, []milestoneFieldChange{
			{repository.MilestoneHistoryDescription, oldDescription.String, params.Description},
			{repository.MilestoneHistoryStatus, oldStatus, params.Status},
			{repository.MilestoneHistoryTargetDate, milestoneDateValue(oldTargetDate.String), milestoneDateValue(newTargetDate)},
		})...)
	})
	if err != nil {
		return nil, err
	}
	updated, err := s.GetMilestone(params.ID)
	if err != nil {
		return nil, err
	}
	if params.AuditActor != nil {
		resourceID := updated.ID
		emitServiceAudit(s.db, *params.AuditActor, logger.ActionMilestoneUpdate, logger.ResourceMilestone, &resourceID, updated.Name, nil)
	}
	return updated, nil
}

// milestoneFieldChange is one field's old and new value in a milestone update.
type milestoneFieldChange struct {
	field    string
	oldValue string
	newValue string
}

// milestoneHistoryChanges keeps the fields whose value changed, as milestone
// history rows.
func milestoneHistoryChanges(milestoneID int, actorID *int, changes []milestoneFieldChange) []repository.MilestoneHistoryEntry {
	now := time.Now().UTC()
	entries := []repository.MilestoneHistoryEntry{}
	for _, change := range changes {
		if change.oldValue == change.newValue {
			continue
		}
		entries = append(entries, repository.MilestoneHistoryEntry{
			MilestoneID: milestoneID,
			UserID:      actorID,
			FieldName:   change.field,
			OldValue:    change.oldValue,
			NewValue:    change.newValue,
			ChangedAt:   now,
		})
	}
	return entries
}

// milestoneDateValue normalizes a stored or submitted target date to
// YYYY-MM-DD. Drivers return DATE columns as "2026-10-30" or as a timestamp
// string such as "2026-10-30T00:00:00Z".
func milestoneDateValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 10 {
		return value[:10]
	}
	return value
}

// recordMilestoneStatusTransition sets a milestone's status inside tx and
// records the change in milestone history when the status actually changes.
// actorID nil records a system change (automation, release flows without a
// user).
func recordMilestoneStatusTransition(tx database.Tx, milestoneID int, scope string, scopeArgs []any, status string, actorID *int) (int64, error) {
	var oldStatus string
	err := tx.QueryRow(`SELECT status FROM milestones WHERE id = ? `+scope, append([]any{milestoneID}, scopeArgs...)...).Scan(&oldStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to read milestone status: %w", err)
	}
	res, err := tx.ExecWrite(`
		UPDATE milestones SET status = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? `+scope, append([]any{status, milestoneID}, scopeArgs...)...)
	if err != nil {
		return 0, fmt.Errorf("failed to update milestone status: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to inspect milestone status update: %w", err)
	}
	if n == 0 {
		return 0, nil
	}
	if err := repository.RecordMilestoneHistory(tx, milestoneHistoryChanges(milestoneID, actorID, []milestoneFieldChange{
		{repository.MilestoneHistoryStatus, oldStatus, status},
	})...); err != nil {
		return 0, err
	}
	return n, nil
}

// ListMilestoneReleases fetches all releases for a given milestone, ordered by created_at DESC.
func (s *PlanningService) ListMilestoneReleases(milestoneID int) ([]MilestoneReleaseResult, error) {
	rows, err := s.db.Query(`
		SELECT id, milestone_id, tag_name, name, body, is_draft, is_prerelease,
		       target_commitish, workspace_repository_id, tag_url, release_status,
		       CAST(released_at AS TEXT), CAST(assets_json AS TEXT), CAST(last_synced_at AS TEXT),
		       scm_connection_id, scm_repository,
		       scm_release_id, scm_release_url, created_by, created_at
		FROM milestone_releases
		WHERE milestone_id = ? AND state = 'created'
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
		var scmConnectionID, createdBy, workspaceRepositoryID sql.NullInt64
		var scmRepository, scmReleaseID, scmReleaseURL, tagURL sql.NullString
		var releaseStatus, releasedAt, assetsJSON, lastSyncedAt sql.NullString
		var isDraft, isPrerelease sql.NullBool

		if err := rows.Scan(&r.ID, &r.MilestoneID, &r.TagName, &name, &body,
			&isDraft, &isPrerelease, &targetCommitish, &workspaceRepositoryID, &tagURL,
			&releaseStatus, &releasedAt, &assetsJSON, &lastSyncedAt, &scmConnectionID, &scmRepository,
			&scmReleaseID, &scmReleaseURL, &createdBy, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan milestone release: %w", err)
		}

		r.Name = name.String
		r.Body = body.String
		r.TargetCommitish = targetCommitish.String
		if workspaceRepositoryID.Valid {
			id := int(workspaceRepositoryID.Int64)
			r.WorkspaceRepositoryID = &id
		}
		if tagURL.Valid {
			r.TagURL = &tagURL.String
		}
		r.ReleaseStatus = releaseStatus.String
		if releasedAt.Valid {
			r.ReleasedAt = &releasedAt.String
		}
		if lastSyncedAt.Valid {
			r.LastSyncedAt = &lastSyncedAt.String
		}
		if assetsJSON.Valid && assetsJSON.String != "" {
			_ = json.Unmarshal([]byte(assetsJSON.String), &r.Assets)
		}
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate milestone releases: %w", err)
	}

	return releases, nil
}

// ReleaseMilestoneParams contains parameters for releasing a milestone.
type ReleaseMilestoneParams struct {
	ID                    int
	IdempotencyKey        string
	WorkspaceRepositoryID *int
	TagName               string
	TagURL                *string
	ReleaseStatus         string
	ReleasedAt            *string
	Assets                []models.SCMReleaseAsset
	Name                  string
	Body                  string
	IsDraft               bool
	IsPrerelease          bool
	TargetCommitish       string
	SCMConnectionID       *int
	SCMRepository         *string
	SCMReleaseID          *string
	SCMReleaseURL         *string
	CreatedBy             *int
}

// MilestoneReleaseAttempt is a durable claim for one idempotent release
// request. New attempts may call CreateRelease immediately. Reclaimed attempts
// must first list the provider's releases and reconcile by tag because a prior
// request may have timed out after the provider accepted it.
type MilestoneReleaseAttempt struct {
	ID             int
	LeaseToken     string
	AlreadyCreated bool
	NeedsReconcile bool
}

type storedMilestoneReleaseRequest struct {
	TagName               string
	Name                  sql.NullString
	Body                  sql.NullString
	IsDraft               bool
	IsPrerelease          bool
	TargetCommitish       sql.NullString
	WorkspaceRepositoryID sql.NullInt64
	SCMConnectionID       sql.NullInt64
	SCMRepository         sql.NullString
	CreatedBy             sql.NullInt64
}

func milestoneReleaseRequestMatches(params ReleaseMilestoneParams, stored storedMilestoneReleaseRequest) bool {
	return params.TagName == stored.TagName &&
		stored.Name.Valid && params.Name == stored.Name.String &&
		stored.Body.Valid && params.Body == stored.Body.String &&
		params.IsDraft == stored.IsDraft &&
		params.IsPrerelease == stored.IsPrerelease &&
		stored.TargetCommitish.Valid && params.TargetCommitish == stored.TargetCommitish.String &&
		nullableIntMatches(params.SCMConnectionID, stored.SCMConnectionID) &&
		nullableIntMatches(params.WorkspaceRepositoryID, stored.WorkspaceRepositoryID) &&
		nullableStringMatches(params.SCMRepository, stored.SCMRepository) &&
		nullableIntMatches(params.CreatedBy, stored.CreatedBy)
}

func nullableIntMatches(value *int, stored sql.NullInt64) bool {
	if value == nil {
		return !stored.Valid
	}
	return stored.Valid && int(stored.Int64) == *value
}

func nullableStringMatches(value *string, stored sql.NullString) bool {
	if value == nil {
		return !stored.Valid
	}
	return stored.Valid && stored.String == *value
}

func newMilestoneReleaseLeaseToken() (string, error) {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", fmt.Errorf("generate milestone release lease token: %w", err)
	}
	return hex.EncodeToString(token[:]), nil
}

// BeginMilestoneRelease persists or reclaims a release attempt before any
// remote side effect. The unique idempotency key suppresses duplicate requests,
// while the lease prevents concurrent retries from both reaching the provider.
func (s *PlanningService) BeginMilestoneRelease(ctx context.Context, params ReleaseMilestoneParams) (*MilestoneReleaseAttempt, error) {
	if strings.TrimSpace(params.IdempotencyKey) == "" {
		return nil, planningValidationError("idempotency_key", "idempotency key is required")
	}
	leaseToken, err := newMilestoneReleaseLeaseToken()
	if err != nil {
		return nil, err
	}
	leaseExpiresAt := time.Now().UTC().Add(5 * time.Minute)

	return database.WithTxResult(s.db, func(tx database.Tx) (*MilestoneReleaseAttempt, error) {
		assetsJSON := releaseAssetsJSON(params.Assets)
		result, err := tx.ExecWriteContext(ctx, `
			INSERT INTO milestone_releases (
				milestone_id, idempotency_key, state, lease_token, lease_expires_at,
				tag_name, name, body, is_draft, is_prerelease, target_commitish,
				workspace_repository_id, tag_url, release_status, released_at, assets_json,
				scm_connection_id, scm_repository, created_by
			) VALUES (?, ?, 'pending', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT DO NOTHING
		`, params.ID, params.IdempotencyKey, leaseToken, leaseExpiresAt,
			params.TagName, params.Name, params.Body, params.IsDraft, params.IsPrerelease,
			params.TargetCommitish, params.WorkspaceRepositoryID, params.TagURL, releaseStatusOrDefault(params.ReleaseStatus), params.ReleasedAt, assetsJSON,
			params.SCMConnectionID, params.SCMRepository, params.CreatedBy)
		if err != nil {
			return nil, fmt.Errorf("failed to persist pending milestone release: %w", err)
		}
		inserted, err := result.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("failed to inspect pending milestone release: %w", err)
		}

		var attemptID int
		var state string
		var storedRequest storedMilestoneReleaseRequest
		if err := tx.QueryRowContext(ctx, `
				SELECT id, state, tag_name, name, body, is_draft, is_prerelease,
				       target_commitish, workspace_repository_id, scm_connection_id, scm_repository, created_by
				FROM milestone_releases
				WHERE milestone_id = ? AND idempotency_key = ?
			`, params.ID, params.IdempotencyKey).Scan(
			&attemptID,
			&state,
			&storedRequest.TagName,
			&storedRequest.Name,
			&storedRequest.Body,
			&storedRequest.IsDraft,
			&storedRequest.IsPrerelease,
			&storedRequest.TargetCommitish,
			&storedRequest.WorkspaceRepositoryID,
			&storedRequest.SCMConnectionID,
			&storedRequest.SCMRepository,
			&storedRequest.CreatedBy,
		); err != nil {
			return nil, fmt.Errorf("failed to load milestone release attempt: %w", err)
		}
		if inserted == 0 && !milestoneReleaseRequestMatches(params, storedRequest) {
			return nil, fmt.Errorf(
				"%w: milestone %d and key %q",
				ErrMilestoneReleaseIdempotencyConflict,
				params.ID,
				params.IdempotencyKey,
			)
		}
		if state == "created" {
			return &MilestoneReleaseAttempt{ID: attemptID, AlreadyCreated: true}, nil
		}
		if inserted > 0 {
			return &MilestoneReleaseAttempt{ID: attemptID, LeaseToken: leaseToken}, nil
		}

		claim, err := tx.ExecWriteContext(ctx, `
			UPDATE milestone_releases
			SET state = 'pending', last_error = NULL, lease_token = ?, lease_expires_at = ?,
			    updated_at = CURRENT_TIMESTAMP
			WHERE id = ? AND state <> 'created'
			  AND (lease_token IS NULL OR lease_expires_at IS NULL OR lease_expires_at <= CURRENT_TIMESTAMP)
		`, leaseToken, leaseExpiresAt, attemptID)
		if err != nil {
			return nil, fmt.Errorf("failed to reclaim milestone release attempt: %w", err)
		}
		claimed, err := claim.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("failed to inspect milestone release claim: %w", err)
		}
		if claimed == 0 {
			return nil, ErrMilestoneReleaseInProgress
		}
		return &MilestoneReleaseAttempt{
			ID:             attemptID,
			LeaseToken:     leaseToken,
			NeedsReconcile: true,
		}, nil
	})
}

// MarkMilestoneReleaseUncertain records that the provider may have accepted
// the request and releases the local lease. The next retry reconciles by tag
// before attempting another remote create.
func (s *PlanningService) MarkMilestoneReleaseUncertain(ctx context.Context, attemptID int, leaseToken, message string) error {
	_, err := s.db.ExecWriteContext(ctx, `
		UPDATE milestone_releases
		SET state = 'reconciliation-required', last_error = ?,
		    lease_token = NULL, lease_expires_at = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND lease_token = ? AND state <> 'created'
	`, message, attemptID, leaseToken)
	if err != nil {
		return fmt.Errorf("failed to mark milestone release for reconciliation: %w", err)
	}
	return nil
}

// CompleteMilestoneRelease atomically records the remote identity and marks
// the milestone completed. Either both local effects commit or neither does.
func (s *PlanningService) CompleteMilestoneRelease(ctx context.Context, attemptID int, leaseToken string, params ReleaseMilestoneParams) (*MilestoneResult, error) {
	assetsJSON := releaseAssetsJSON(params.Assets)
	err := database.WithTx(s.db, func(tx database.Tx) error {
		result, err := tx.ExecWriteContext(ctx, `
			UPDATE milestone_releases
			SET state = 'created', name = ?, body = ?, is_draft = ?, is_prerelease = ?,
			    scm_release_id = ?, scm_release_url = ?, tag_url = ?,
			    release_status = ?, released_at = ?, assets_json = ?, last_synced_at = CURRENT_TIMESTAMP,
			    last_error = NULL, lease_token = NULL, lease_expires_at = NULL,
			    updated_at = CURRENT_TIMESTAMP
			WHERE id = ? AND lease_token = ? AND state = 'pending'
		`, params.Name, params.Body, params.IsDraft, params.IsPrerelease,
			params.SCMReleaseID, params.SCMReleaseURL, params.TagURL, releaseStatusOrDefault(params.ReleaseStatus), params.ReleasedAt, assetsJSON, attemptID, leaseToken)
		if err != nil {
			return fmt.Errorf("failed to finalize milestone release: %w", err)
		}
		updated, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to inspect milestone release finalization: %w", err)
		}
		if updated == 0 {
			return fmt.Errorf("milestone release attempt is no longer owned")
		}
		updated, err = recordMilestoneStatusTransition(tx, params.ID, "", nil, "completed", params.CreatedBy)
		if err != nil {
			return err
		}
		if updated == 0 {
			return fmt.Errorf("milestone not found: %d: %w", params.ID, repository.ErrNotFound)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.GetMilestone(params.ID)
}

// ReleaseMilestone is retained for in-process callers that already possess
// release metadata. Its local insert and milestone status transition are
// transactional.
func (s *PlanningService) ReleaseMilestone(params ReleaseMilestoneParams) (*MilestoneResult, error) {
	assetsJSON := releaseAssetsJSON(params.Assets)
	err := database.WithTx(s.db, func(tx database.Tx) error {
		_, err := tx.ExecWrite(`
			INSERT INTO milestone_releases (
				milestone_id, idempotency_key, state, tag_name, name, body,
				is_draft, is_prerelease, target_commitish, workspace_repository_id,
				tag_url, release_status, released_at, assets_json, scm_connection_id,
				scm_repository, scm_release_id, scm_release_url, created_by
			) VALUES (?, ?, 'created', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, params.ID, nullablePlanningString(params.IdempotencyKey), params.TagName, params.Name,
			params.Body, params.IsDraft, params.IsPrerelease, params.TargetCommitish,
			params.WorkspaceRepositoryID, params.TagURL, releaseStatusOrDefault(params.ReleaseStatus), params.ReleasedAt, assetsJSON,
			params.SCMConnectionID, params.SCMRepository, params.SCMReleaseID,
			params.SCMReleaseURL, params.CreatedBy)
		if err != nil {
			return fmt.Errorf("failed to insert milestone release: %w", err)
		}
		_, err = recordMilestoneStatusTransition(tx, params.ID, "", nil, "completed", params.CreatedBy)
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.GetMilestone(params.ID)
}

func nullablePlanningString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func releaseStatusOrDefault(status string) string {
	if strings.TrimSpace(status) == "" {
		return "tag_only"
	}
	return status
}

func releaseAssetsJSON(assets []models.SCMReleaseAsset) string {
	if len(assets) == 0 {
		return "[]"
	}
	raw, err := json.Marshal(assets)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

// DeleteMilestone deletes a milestone. HTTP adapters may supply an audit actor;
// internal automation/import callers omit it and rely on their own execution
// trail instead of manufacturing a user-driven audit event.
func (s *PlanningService) DeleteMilestone(id int, auditActors ...AuditActor) error {
	var resourceName string
	if len(auditActors) > 0 {
		existing, err := s.GetMilestone(id)
		if err != nil {
			return err
		}
		resourceName = existing.Name
	}
	err := database.WithTx(s.db, func(tx database.Tx) error {
		// item_links has no foreign keys, so remove the milestone's page links
		// with it rather than leave them pointing at nothing.
		if err := repository.NewMilestonePageLinkRepository(s.db).DeleteByMilestone(tx, id); err != nil {
			return err
		}
		if _, err := tx.Exec("DELETE FROM milestones WHERE id = ?", id); err != nil {
			return fmt.Errorf("failed to delete milestone: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.enqueueMilestoneReferenceCleanup(id)
	if actor := optionalAuditActor(auditActors); actor != nil {
		emitServiceAudit(s.db, *actor, logger.ActionMilestoneDelete, logger.ResourceMilestone, &id, resourceName, nil)
	}
	return nil
}

// enqueueMilestoneReferenceCleanup strips a deleted milestone from single- and
// multi-value milestone custom fields. Cleanup runs asynchronously so a large
// item population does not block deletion.
func (s *PlanningService) enqueueMilestoneReferenceCleanup(milestoneID int) {
	rows, err := s.db.Query(`SELECT id, field_type FROM custom_field_definitions WHERE field_type IN ('milestone', 'multi_milestone')`)
	if err != nil {
		slog.Warn("failed to load milestone custom fields for cleanup", slog.Int("milestone_id", milestoneID), slog.Any("error", err))
		return
	}
	type fieldRef struct {
		id        int
		fieldType string
	}
	var fields []fieldRef
	for rows.Next() {
		var field fieldRef
		if err := rows.Scan(&field.id, &field.fieldType); err != nil {
			slog.Warn("failed to read milestone custom field for cleanup", slog.Int("milestone_id", milestoneID), slog.Any("error", err))
			break
		}
		fields = append(fields, field)
	}
	if err := rows.Err(); err != nil {
		slog.Warn("failed to list milestone custom fields for cleanup", slog.Int("milestone_id", milestoneID), slog.Any("error", err))
	}
	_ = rows.Close()

	for _, field := range fields {
		payload, err := json.Marshal(milestoneCleanupPayload{
			FieldID:    field.id,
			FieldType:  field.fieldType,
			RemovedIDs: []int{milestoneID},
		})
		if err != nil {
			slog.Warn("failed to encode milestone reference cleanup", slog.Int("milestone_id", milestoneID), slog.Int("field_id", field.id), slog.Any("error", err))
			continue
		}
		// The payload matches scheduler.optionRemovalPayload; importing the
		// scheduler here would create an import cycle through email/services.
		if _, err := s.db.ExecWrite(
			`INSERT INTO pending_custom_field_cleanups (field_id, job_type, payload, status, created_at)
			 VALUES (?, 'option_removal', ?, 'pending', ?)`,
			field.id, string(payload), time.Now(),
		); err != nil {
			slog.Warn("failed to enqueue milestone reference cleanup", slog.Int("milestone_id", milestoneID), slog.Int("field_id", field.id), slog.Any("error", err))
		}
	}
}

type milestoneCleanupPayload struct {
	FieldID    int    `json:"field_id"`
	FieldType  string `json:"field_type"`
	RemovedIDs []int  `json:"removed_ids"`
}

// MilestoneTestStats aggregates test-plan activity for a milestone.
type MilestoneTestStats = models.MilestoneTestStats

// GetMilestoneTestStatistics retrieves test plan statistics for a milestone.
func (s *PlanningService) GetMilestoneTestStatistics(milestoneID int, workspaceIDs []int) (*MilestoneTestStats, error) {
	statsByID, err := s.GetMilestoneTestStatisticsBatch([]int{milestoneID}, workspaceIDs)
	if err != nil {
		return nil, err
	}
	if stats, ok := statsByID[milestoneID]; ok {
		return stats, nil
	}
	return &MilestoneTestStats{}, nil
}

// GetMilestoneTestStatisticsBatch returns test statistics keyed by milestone
// ID in one read. Global milestones are visible to every authenticated caller;
// local milestones and every test-set contribution remain restricted to the
// caller's accessible workspace IDs.
func (s *PlanningService) GetMilestoneTestStatisticsBatch(milestoneIDs, workspaceIDs []int) (map[int]*MilestoneTestStats, error) {
	statsByID, err := repository.NewTestSetRepository(s.db).FindStatsByMilestoneIDs(milestoneIDs, workspaceIDs)
	if err != nil {
		return nil, err
	}
	result := make(map[int]*MilestoneTestStats, len(statsByID))
	for milestoneID, stats := range statsByID {
		statsCopy := stats
		result[milestoneID] = &statsCopy
	}
	return result, nil
}

// MilestoneProgressReport represents the full milestone progress data.
type MilestoneProgressReport struct {
	MilestoneID     int                       `json:"milestone_id"`
	MilestoneName   string                    `json:"milestone_name"`
	Description     string                    `json:"description,omitempty"`
	TargetDate      *string                   `json:"target_date,omitempty"`
	Status          string                    `json:"status"`
	CategoryColor   string                    `json:"category_color,omitempty"`
	TotalItems      int                       `json:"total_items"`
	CompletedItems  int                       `json:"completed_items"`
	PercentComplete float64                   `json:"percent_complete"`
	StatusBreakdown []StatusBreakdown         `json:"status_breakdown"`
	ItemsByCategory map[string][]ProgressItem `json:"items_by_category"`
}

// GetMilestoneProgress retrieves progress report for a milestone.
func (s *PlanningService) GetMilestoneProgress(milestoneID int, workspaceIDs []int) (*MilestoneProgressReport, error) {
	var report MilestoneProgressReport
	report.MilestoneID = milestoneID
	// Get milestone details
	var description, targetDate, categoryColor sql.NullString
	err := s.db.QueryRow(`
		SELECT m.name, m.description, m.target_date, m.status, mc.color
		FROM milestones m
		LEFT JOIN milestone_categories mc ON m.category_id = mc.id
		WHERE m.id = ?
	`, milestoneID).Scan(&report.MilestoneName, &description, &targetDate, &report.Status, &categoryColor)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("milestone not found: %d: %w", milestoneID, repository.ErrNotFound)
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
	acc, err := s.buildProgressReport(repository.ItemFilters{MilestoneID: &milestoneID}, workspaceIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get milestone progress: %w", err)
	}

	report.TotalItems = acc.TotalItems
	report.CompletedItems = acc.CompletedItems
	report.PercentComplete = acc.PercentComplete
	report.StatusBreakdown = acc.StatusBreakdown
	report.ItemsByCategory = acc.ItemsByCategory

	return &report, nil
}

// IsMilestoneGlobal checks if a milestone is global.
func (s *PlanningService) IsMilestoneGlobal(id int) (isGlobal bool, workspaceID *int, err error) {
	var wsID sql.NullInt64
	err = s.db.QueryRow("SELECT is_global, workspace_id FROM milestones WHERE id = ?", id).Scan(&isGlobal, &wsID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil, fmt.Errorf("milestone not found: %d: %w", id, repository.ErrNotFound)
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

// MaxMilestonePosition returns the current maximum position for milestones
// in the given scope, or 0 when the scope is empty. Used by CreateMilestone
// to place a new milestone at the end of its group.
func (s *PlanningService) MaxMilestonePosition(isGlobal bool, workspaceID, categoryID *int) (int, error) {
	var maxPos sql.NullInt64
	err := s.db.QueryRow(
		"SELECT MAX(position) FROM milestones WHERE "+milestoneScopeClause(),
		milestoneScopeArgs(isGlobal, workspaceID, categoryID)...,
	).Scan(&maxPos)
	if err != nil {
		return 0, fmt.Errorf("failed to get max milestone position: %w", err)
	}
	if !maxPos.Valid {
		return 0, nil
	}
	return int(maxPos.Int64), nil
}

// MilestoneScope identifies the (is_global, workspace_id, category_id) group
// a reorder operation applies to. Drag-and-drop reorders milestones within a
// single scope only; cross-scope moves are out of scope and rejected.
type MilestoneScope struct {
	IsGlobal    bool
	WorkspaceID *int
	CategoryID  *int
}

// ReorderMilestones reassigns position for the milestones identified by
// orderedIDs, all of which must belong to scope. Positions are normalized to
// (index+1)*milestonePositionStep in a single transaction, leaving gaps for
// future inserts. Any id in orderedIDs that isn't in scope is ignored (its
// UPDATE matches 0 rows); the caller is responsible for supplying a complete,
// in-scope ordering. Mirrors the test-folder Reorder pattern.
func (s *PlanningService) ReorderMilestones(scope MilestoneScope, orderedIDs []int, auditActors ...AuditActor) error {
	if len(orderedIDs) == 0 {
		return nil
	}

	scopeClause := milestoneScopeClause()
	scopeArgs := milestoneScopeArgs(scope.IsGlobal, scope.WorkspaceID, scope.CategoryID)

	seen := make(map[int]struct{}, len(orderedIDs))
	for _, id := range orderedIDs {
		if id <= 0 {
			return fmt.Errorf("%w: ordered_ids must contain positive ids", ErrInvalidMilestoneReorder)
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("%w: ordered_ids contains duplicate id %d", ErrInvalidMilestoneReorder, id)
		}
		seen[id] = struct{}{}
	}

	rows, err := s.db.Query("SELECT id FROM milestones WHERE "+scopeClause, scopeArgs...)
	if err != nil {
		return fmt.Errorf("failed to load milestone reorder scope: %w", err)
	}
	defer func() { _ = rows.Close() }()

	scopeIDs := make(map[int]struct{}, len(orderedIDs))
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("failed to scan milestone reorder scope: %w", err)
		}
		scopeIDs[id] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to iterate milestone reorder scope: %w", err)
	}
	if len(scopeIDs) != len(orderedIDs) {
		return fmt.Errorf("%w: ordered_ids must include every milestone in the selected scope", ErrInvalidMilestoneReorder)
	}
	for id := range seen {
		if _, ok := scopeIDs[id]; !ok {
			return fmt.Errorf("%w: milestone %d is not in the selected scope", ErrInvalidMilestoneReorder, id)
		}
	}

	if err := database.WithTx(s.db, func(tx database.Tx) error {
		// Only position changes: a reorder is not an edit, so updated_at stays
		// put and the Updated sort doesn't jump the renumbered milestones.
		for i, id := range orderedIDs {
			position := (i + 1) * milestonePositionStep
			updateArgs := append([]any{position, id}, scopeArgs...)
			if _, err := tx.Exec(
				"UPDATE milestones SET position = ? WHERE id = ? AND "+scopeClause,
				updateArgs...,
			); err != nil {
				return fmt.Errorf("failed to reorder milestone %d: %w", id, err)
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if actor := optionalAuditActor(auditActors); actor != nil {
		emitServiceAudit(s.db, *actor, logger.ActionMilestoneReorder, logger.ResourceMilestone, nil, "", map[string]any{
			"ordered_ids":  orderedIDs,
			"is_global":    scope.IsGlobal,
			"workspace_id": scope.WorkspaceID,
			"category_id":  scope.CategoryID,
		})
	}
	return nil
}
