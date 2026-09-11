package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

var ErrWorkspaceHasProtectedIntegrationLinks = errors.New("workspace has provider-managed integration links")

// WorkspaceService encapsulates workspace business logic used by both HTTP handlers
// and other services.
type WorkspaceService struct {
	db                    database.Database
	repo                  *repository.WorkspaceRepository
	itemRepo              *repository.ItemRepository
	templates             *repository.WorkspaceTemplateRepository
	integrationLinkGuards *IntegrationLinkGuards
	access                WorkspaceSourceAccess
}

// NewWorkspaceService creates a new WorkspaceService.
func NewWorkspaceService(db database.Database) *WorkspaceService {
	return &WorkspaceService{
		db:                    db,
		repo:                  repository.NewWorkspaceRepository(db),
		itemRepo:              repository.NewItemRepository(db),
		templates:             repository.NewWorkspaceTemplateRepository(db),
		integrationLinkGuards: NewIntegrationLinkGuards(db),
	}
}

// NewWorkspaceServiceWithAccess creates a WorkspaceService whose template
// clones authorize the source workspace through the given access checker.
func NewWorkspaceServiceWithAccess(db database.Database, access WorkspaceSourceAccess) *WorkspaceService {
	service := NewWorkspaceService(db)
	service.access = access
	return service
}

// WorkspaceListParams contains the parameters for listing workspaces.
type WorkspaceListParams struct {
	WorkspaceIDs []int
	Limit        int
	Offset       int
}

// List retrieves a page from an already-authorized workspace ID snapshot.
func (s *WorkspaceService) List(params WorkspaceListParams) ([]models.Workspace, int, error) {
	if len(params.WorkspaceIDs) == 0 {
		return []models.Workspace{}, 0, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(params.WorkspaceIDs)), ",")
	workspaceArgs := make([]any, len(params.WorkspaceIDs))
	for i, workspaceID := range params.WorkspaceIDs {
		workspaceArgs[i] = workspaceID
	}
	listArgs := append(append([]any{}, workspaceArgs...), params.Limit, params.Offset)
	rows, err := s.db.Query(`
		SELECT w.id, w.name, w.key, w.description, w.active, w.is_template, w.is_personal,
		       w.icon, w.color, w.internal_comments_enabled, w.created_at, w.updated_at
		FROM workspaces w
		WHERE w.id IN (`+placeholders+`)
		ORDER BY w.name
		LIMIT ? OFFSET ?
	`, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list workspaces: %w", err)
	}
	defer rows.Close()

	var workspaces []models.Workspace
	for rows.Next() {
		var ws models.Workspace
		var icon, color sql.NullString
		err = rows.Scan(&ws.ID, &ws.Name, &ws.Key, &ws.Description, &ws.Active, &ws.IsTemplate, &ws.IsPersonal,
			&icon, &color, &ws.InternalCommentsEnabled, &ws.CreatedAt, &ws.UpdatedAt)
		if err != nil {
			continue
		}
		ws.Icon = icon.String
		ws.Color = color.String
		workspaces = append(workspaces, ws)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to iterate workspaces: %w", err)
	}

	if workspaces == nil {
		workspaces = []models.Workspace{}
	}

	var total int
	err = s.db.QueryRow("SELECT COUNT(*) FROM workspaces WHERE id IN ("+placeholders+")", workspaceArgs...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count workspaces: %w", err)
	}

	return workspaces, total, nil
}

// GetByID retrieves a workspace by ID.
func (s *WorkspaceService) GetByID(id int) (*models.Workspace, error) {
	ws, err := s.repo.FindByID(id)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, fmt.Errorf("workspace not found: %d: %w", id, repository.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}
	categories, err := s.repo.GetTimeProjectCategories(id)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace time project categories: %w", err)
	}
	ws.TimeProjectCategories = categories
	return ws, nil
}

// CreateWorkspaceParams contains the parameters for creating a workspace.
// TemplateWorkspaceID, when set, clones the referenced template workspace's
// configuration-set assignment, work-item templates, and seed items into the
// new workspace inside one transaction.
type CreateWorkspaceParams struct {
	Name        string
	Key         string
	Description string
	Icon        string
	Color       string
	CreatorID   int

	Active        *bool
	TimeProjectID *int
	IsPersonal    bool
	OwnerID       *int
	AvatarURL     *string
	DefaultView   string

	TemplateWorkspaceID *int
}

// CreateWorkspaceResult contains the result of creating a workspace. The
// copy counts stay zero for blank creation.
type CreateWorkspaceResult struct {
	Workspace                *models.Workspace
	SourceWorkspaceID        int
	ConfigSetAttached        bool
	TemplatesCopied          int
	ItemsCopied              int
	OmittedCustomFieldValues int
}

// Create creates a new workspace and grants the Administrator role to the
// creator. When a template source is supplied, the clone runs in the same
// transaction so the workspace, role grant, and copied data commit atomically.
// The whole transaction is retried on PostgreSQL serialization aborts and
// rare rank collisions; validation and authorization errors never retry.
func (s *WorkspaceService) Create(ctx context.Context, params CreateWorkspaceParams) (*CreateWorkspaceResult, error) {
	if params.Name == "" || params.Key == "" {
		return nil, fmt.Errorf("workspace name and key are required")
	}
	if params.IsPersonal && params.TemplateWorkspaceID != nil {
		return nil, fmt.Errorf("%w: personal workspaces cannot be created from a template", ErrInvalidWorkspaceTemplate)
	}

	key := strings.ToUpper(params.Key)
	if params.DefaultView == "" {
		params.DefaultView = "board"
	}

	started := time.Now()
	var lastErr error
	for attempt := 0; attempt < workspaceCloneMaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("workspace creation canceled: %w", err)
		}

		var txOpts *sql.TxOptions
		if s.db.GetDriverName() == "postgres" {
			// All source reads form one point-in-time snapshot.
			txOpts = &sql.TxOptions{Isolation: sql.LevelRepeatableRead}
		}
		tx, err := s.db.BeginTx(ctx, txOpts)
		if err != nil {
			return nil, fmt.Errorf("begin workspace creation transaction: %w", err)
		}

		result, err := s.createWorkspaceTx(ctx, tx, params, key)
		if err != nil {
			_ = tx.Rollback()
			if ctx.Err() != nil {
				return nil, fmt.Errorf("workspace creation canceled: %w", ctx.Err())
			}
			if isWorkspaceCloneRetryable(err) && attempt < workspaceCloneMaxAttempts-1 {
				lastErr = err
				continue
			}
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			if isWorkspaceCloneRetryable(err) && attempt < workspaceCloneMaxAttempts-1 {
				lastErr = err
				continue
			}
			return nil, fmt.Errorf("commit workspace creation: %w", err)
		}

		if result.ItemsCopied > 0 || result.TemplatesCopied > 0 || result.ConfigSetAttached {
			repository.InvalidateItemListCountCache(s.db, result.Workspace.ID)
			logWorkspaceCloneResult(result, time.Since(started))
		}
		return result, nil
	}
	return nil, fmt.Errorf("workspace creation failed after retries: %w", lastErr)
}

// NullableUpdate distinguishes an omitted field from an explicit null.
type NullableUpdate[T any] struct {
	Present bool
	Value   *T
}

// UpdateWorkspaceParams contains the fields to update on a workspace.
type UpdateWorkspaceParams struct {
	ID                      int
	Name                    *string
	Key                     *string
	Description             *string
	Active                  *bool
	TimeProjectID           NullableUpdate[int]
	IsPersonal              *bool
	OwnerID                 NullableUpdate[int]
	Icon                    *string
	Color                   *string
	AvatarURL               NullableUpdate[string]
	DefaultView             *string
	InternalCommentsEnabled *bool
	TimeProjectCategories   *[]int
	IsTemplate              *bool
}

// Update changes only the supplied workspace fields. Personal workspaces and
// templates are mutually exclusive in both directions.
func (s *WorkspaceService) Update(params UpdateWorkspaceParams) (*models.Workspace, error) {
	if params.IsPersonal != nil || params.IsTemplate != nil {
		current, err := s.repo.FindByIDBasic(params.ID)
		if errors.Is(err, repository.ErrNotFound) {
			return nil, fmt.Errorf("workspace not found: %d: %w", params.ID, repository.ErrNotFound)
		}
		if err != nil {
			return nil, err
		}
		nextIsPersonal := current.IsPersonal
		if params.IsPersonal != nil {
			nextIsPersonal = *params.IsPersonal
		}
		nextIsTemplate := current.IsTemplate
		if params.IsTemplate != nil {
			nextIsTemplate = *params.IsTemplate
		}
		if nextIsPersonal && nextIsTemplate {
			return nil, ErrPersonalWorkspaceTemplate
		}
	}

	sets := make([]string, 0, 12)
	args := make([]any, 0, 13)
	appendField := func(column string, value any) {
		sets = append(sets, column+" = ?")
		args = append(args, value)
	}

	if params.Name != nil {
		appendField("name", *params.Name)
	}
	if params.Key != nil {
		appendField("key", *params.Key)
	}
	if params.Description != nil {
		appendField("description", *params.Description)
	}
	if params.Active != nil {
		appendField("active", *params.Active)
	}
	if params.TimeProjectID.Present {
		appendField("time_project_id", nullableUpdateValue(params.TimeProjectID))
	}
	if params.IsPersonal != nil {
		appendField("is_personal", *params.IsPersonal)
	}
	if params.OwnerID.Present {
		appendField("owner_id", nullableUpdateValue(params.OwnerID))
	}
	if params.Icon != nil {
		appendField("icon", *params.Icon)
	}
	if params.Color != nil {
		appendField("color", *params.Color)
	}
	if params.AvatarURL.Present {
		appendField("avatar_url", nullableUpdateValue(params.AvatarURL))
	}
	if params.DefaultView != nil {
		appendField("default_view", *params.DefaultView)
	}
	if params.InternalCommentsEnabled != nil {
		appendField("internal_comments_enabled", *params.InternalCommentsEnabled)
	}
	if params.IsTemplate != nil {
		appendField("is_template", *params.IsTemplate)
	}

	if len(sets) > 0 {
		sets = append(sets, "updated_at = CURRENT_TIMESTAMP")
		args = append(args, params.ID)
		result, err := s.db.ExecWrite(
			"UPDATE workspaces SET "+strings.Join(sets, ", ")+" WHERE id = ?",
			args...,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to update workspace: %w", err)
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("failed to read workspace update result: %w", err)
		}
		if rows == 0 {
			return nil, fmt.Errorf("workspace not found: %d: %w", params.ID, repository.ErrNotFound)
		}
	}

	if params.TimeProjectCategories != nil {
		if err := s.repo.SaveTimeProjectCategories(params.ID, *params.TimeProjectCategories); err != nil {
			return nil, fmt.Errorf("failed to update workspace time project categories: %w", err)
		}
	}

	return s.GetByID(params.ID)
}

func nullableUpdateValue[T any](update NullableUpdate[T]) any {
	if update.Value == nil {
		return nil
	}
	return *update.Value
}

// Delete removes a workspace by ID.
func (s *WorkspaceService) Delete(id int) error {
	if err := database.WithTx(s.db, func(tx database.Tx) error {
		exists, err := s.repo.LockForDeleteTx(tx, id)
		if err != nil {
			return fmt.Errorf("check workspace existence: %w", err)
		}
		if !exists {
			return fmt.Errorf("workspace not found: %d: %w", id, repository.ErrNotFound)
		}
		if err := s.itemRepo.LockWorkspaceItemsTx(tx, id); err != nil {
			return err
		}

		hasProtectedLinks, err := s.integrationLinkGuards.HasLinksForWorkspaceTx(tx, id)
		if err != nil {
			return err
		}
		if hasProtectedLinks {
			return ErrWorkspaceHasProtectedIntegrationLinks
		}

		if err := s.repo.DeleteTx(tx, id); err != nil {
			return fmt.Errorf("delete workspace: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	// The sequence is auxiliary cleanup. Run it after the workspace deletion
	// commits so a failed DROP cannot roll back an otherwise valid deletion or
	// leave a PostgreSQL transaction permanently aborted.
	if err := s.repo.DropItemSequence(int64(id)); err != nil {
		slog.Warn("failed to drop item sequence for workspace", "workspace_id", id, "error", err)
	}

	repository.InvalidateItemListCountCache(s.db, id)

	return nil
}

// Exists checks if a workspace exists.
// deadcode-keep: called by core-tests/internal/services/workspace_service_test.go
func (s *WorkspaceService) Exists(id int) (bool, error) {
	return s.repo.Exists(id)
}

// KeyExists checks if a workspace key exists.
func (s *WorkspaceService) KeyExists(key string) (bool, error) {
	return s.repo.KeyExists(strings.ToUpper(key))
}

// GetStatuses retrieves statuses available through the workspace's effective
// workflows. This follows the same fallback chain used for item transitions:
// item-type override, configuration-set workflow, then the global default
// workflow. A status is returned only when at least one applicable workflow
// references it. Personal workspaces are not workflow-bound and retain access
// to the full status catalog.
func (s *WorkspaceService) GetStatuses(workspaceID int) ([]models.Status, error) {
	statuses, err := s.GetStatusesForWorkspaces([]int{workspaceID})
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace statuses: %w", err)
	}
	return statuses, nil
}

// GetStatusesForWorkspaces returns the union of statuses available to any of
// the supplied workspaces in one query. An empty workspace list means global
// status context and therefore returns the complete status catalog.
func (s *WorkspaceService) GetStatusesForWorkspaces(workspaceIDs []int) ([]models.Status, error) {
	return repository.NewStatusRepository(s.db).ListForWorkspaces(workspaceIDs)
}

// ListTemplateSummaries returns every structurally eligible template
// (active, non-personal, marked as a template) with picker metadata.
// Callers must still filter the result to templates visible to the user.
func (s *WorkspaceService) ListTemplateSummaries(ctx context.Context) ([]models.WorkspaceTemplateSummary, error) {
	return s.templates.ListTemplateSummaries(ctx)
}

// GetItemTypes retrieves item types available for a workspace via its configuration set.
// If the workspace has a config set with item types defined, only those are returned.
// If no config set exists, all item types are returned.
func (s *WorkspaceService) GetItemTypes(workspaceID int) ([]ItemTypeResult, error) {
	rows, err := repository.NewItemTypeRepository(s.db).ListForWorkspace(workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace item types: %w", err)
	}
	out := make([]ItemTypeResult, 0, len(rows))
	for _, row := range rows {
		out = append(out, ItemTypeResult{ID: row.ID, BuiltinKey: row.BuiltinKey, Name: row.Name, Description: row.Description,
			Icon: row.Icon, Color: row.Color, HierarchyLevel: row.HierarchyLevel,
			SortOrder: row.SortOrder, IsDefault: row.IsDefault})
	}
	return out, nil
}

// GetPriorities returns the priorities enabled for a workspace's configuration
// set. When the workspace has no configuration set (or no priorities mapped to
// it), all priorities are returned — mirroring GetItemTypes/GetStatuses.
func (s *WorkspaceService) GetPriorities(workspaceID int) ([]PriorityResult, error) {
	rows, err := repository.NewPriorityRepository(s.db).ListForWorkspace(workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace priorities: %w", err)
	}
	out := make([]PriorityResult, 0, len(rows))
	for _, row := range rows {
		out = append(out, PriorityResult{ID: row.ID, BuiltinKey: row.BuiltinKey, Name: row.Name, Description: row.Description,
			Icon: row.Icon, Color: row.Color, SortOrder: row.SortOrder, IsDefault: row.IsDefault})
	}
	return out, nil
}
