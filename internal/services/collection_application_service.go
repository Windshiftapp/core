package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
)

var collectionSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$`)

var (
	ErrCollectionForbidden = errors.New("collection access forbidden")
	ErrCollectionConflict  = errors.New("collection conflict")
	ErrBoardConflict       = errors.New("board configuration conflict")
)

type CollectionValidationError struct{ Message string }

func (e *CollectionValidationError) Error() string { return e.Message }

type CollectionListParams struct {
	UserID      int
	Query       string
	WorkspaceID *int
	CategoryID  *int
	Limit       int
	Offset      int
}

type CollectionUpdate struct {
	NameSet        bool
	Name           string
	DescriptionSet bool
	Description    string
	QLQuerySet     bool
	QLQuery        string
	FilterStateSet bool
	FilterState    *string
	WorkspaceIDSet bool
	WorkspaceID    *int
	CategoryIDSet  bool
	CategoryID     *int
	IsPublicSet    bool
	IsPublic       bool
	IsPrivateSet   bool
	IsPrivate      bool
	PublicSlugSet  bool
	PublicSlug     *string
	// PublicMilestoneSwimlanesSet distinguishes an omitted patch field from false.
	PublicMilestoneSwimlanesSet bool
	PublicMilestoneSwimlanes    bool
}

type CollectionSharingUpdate struct {
	IsPublic                 bool
	PublicSlug               *string
	PublicMilestoneSwimlanes *bool
}

type CollectionCategoryPatch struct {
	Name, Color, Description *string
}

type BoardConfigurationScope struct {
	CollectionID *int
	WorkspaceID  *int
}

type BoardConfigurationCollection struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	QLQuery     string  `json:"ql_query"`
	IsPublic    bool    `json:"is_public"`
	WorkspaceID *int    `json:"workspace_id,omitempty"`
	CreatedBy   *int    `json:"created_by,omitempty"`
	PublicSlug  *string `json:"public_slug,omitempty"`
}

type BoardConfigurationBootstrap struct {
	Collection             *BoardConfigurationCollection `json:"collection,omitempty"`
	BoardConfiguration     *models.BoardConfiguration    `json:"board_configuration"`
	Statuses               []models.Status               `json:"statuses"`
	ReferencedWorkspaceIDs []int                         `json:"referenced_workspace_ids"`
}

type CollectionApplicationService struct {
	db          database.Database
	repository  *repository.CollectionRepository
	permissions *PermissionService
	publicScope *PublicBoardScopeService
	categories  *EnumService
	boards      *repository.BoardConfigurationRepository
	items       *ItemCRUDService
	workspaces  *WorkspaceService

	// beforePublicSharingWrite runs between scope authorization and the
	// conditional write. Tests use it to reproduce a concurrent query edit
	// deterministically; it is nil in production.
	beforePublicSharingWrite func()
}

func NewCollectionApplicationService(db database.Database, permissions *PermissionService) *CollectionApplicationService {
	return &CollectionApplicationService{
		db: db, repository: repository.NewCollectionRepository(db), permissions: permissions,
		publicScope: NewPublicBoardScopeService(db, permissions),
		categories:  NewEnumService(db, NewCollectionCategoryConfig()),
		boards:      repository.NewBoardConfigurationRepository(db),
		items:       NewItemCRUDService(db),
		workspaces:  NewWorkspaceService(db),
	}
}

func (s *CollectionApplicationService) ListCategories() ([]models.CollectionCategory, error) {
	entities, err := s.categories.GetAll()
	if err != nil {
		return nil, err
	}
	items := make([]models.CollectionCategory, len(entities))
	for i, entity := range entities {
		category, ok := entity.(*models.CollectionCategory)
		if !ok {
			return nil, fmt.Errorf("list collection categories returned %T", entity)
		}
		items[i] = *category
	}
	return items, nil
}

func (s *CollectionApplicationService) GetCategory(id int) (*models.CollectionCategory, error) {
	entity, err := s.categories.GetByID(id)
	if err != nil {
		return nil, err
	}
	category, ok := entity.(*models.CollectionCategory)
	if !ok {
		return nil, fmt.Errorf("get collection category returned %T", entity)
	}
	return category, nil
}

func (s *CollectionApplicationService) CreateCategory(actor AuditActor, input models.CollectionCategory) (*models.CollectionCategory, error) {
	sanitizeCollectionCategory(&input)
	entity, err := s.categories.Create(&input, nil)
	if err != nil {
		return nil, err
	}
	created, ok := entity.(*models.CollectionCategory)
	if !ok {
		return nil, fmt.Errorf("create collection category returned %T", entity)
	}
	emitServiceAudit(s.db, actor, logger.ActionCollectionCategoryCreate, logger.ResourceCollectionCategory, &created.ID, created.Name, nil)
	return created, nil
}

func (s *CollectionApplicationService) PatchCategory(actor AuditActor, id int, patch CollectionCategoryPatch) (*models.CollectionCategory, error) {
	current, err := s.GetCategory(id)
	if err != nil {
		return nil, err
	}
	if patch.Name != nil {
		current.Name = *patch.Name
	}
	if patch.Color != nil {
		current.Color = *patch.Color
	}
	if patch.Description != nil {
		current.Description = *patch.Description
	}
	sanitizeCollectionCategory(current)
	entity, err := s.categories.Update(id, current, nil)
	if err != nil {
		return nil, err
	}
	updated, ok := entity.(*models.CollectionCategory)
	if !ok {
		return nil, fmt.Errorf("update collection category returned %T", entity)
	}
	emitServiceAudit(s.db, actor, logger.ActionCollectionCategoryUpdate, logger.ResourceCollectionCategory, &id, updated.Name, nil)
	return updated, nil
}

func (s *CollectionApplicationService) DeleteCategory(actor AuditActor, id int) error {
	current, err := s.GetCategory(id)
	if err != nil {
		return err
	}
	if err := s.categories.Delete(id, nil); err != nil {
		return err
	}
	emitServiceAudit(s.db, actor, logger.ActionCollectionCategoryDelete, logger.ResourceCollectionCategory, &id, current.Name, nil)
	return nil
}

func (s *CollectionApplicationService) List(params CollectionListParams) ([]models.Collection, int, error) {
	collections, err := s.repository.ListVisibleModels(repository.CollectionListFilter{
		UserID: params.UserID, WorkspaceID: params.WorkspaceID, CategoryID: params.CategoryID,
	})
	if err != nil {
		return nil, 0, err
	}
	workspaceIDs, err := s.permissions.AccessibleWorkspaceIDs(params.UserID)
	if err != nil {
		return nil, 0, err
	}
	accessible := make(map[int]struct{}, len(workspaceIDs))
	for _, id := range workspaceIDs {
		accessible[id] = struct{}{}
	}
	query := strings.ToLower(strings.TrimSpace(params.Query))
	collections = slices.DeleteFunc(collections, func(collection models.Collection) bool {
		if !strings.Contains(strings.ToLower(collection.Name), query) {
			return true
		}
		if collection.WorkspaceID == nil {
			return false
		}
		_, ok := accessible[*collection.WorkspaceID]
		return !ok
	})
	total := len(collections)
	start := min(params.Offset, total)
	end := min(start+params.Limit, total)
	return collections[start:end], total, nil
}

func (s *CollectionApplicationService) Get(userID, id int) (*models.Collection, error) {
	collection, err := s.repository.GetVisibleModel(id, userID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrCollectionNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := s.requireWorkspaceAccess(userID, collection); err != nil {
		return nil, err
	}
	return collection, nil
}

func (s *CollectionApplicationService) GetBySlug(userID int, slug string) (*models.Collection, error) {
	collection, err := s.repository.GetBySlug(slug)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrCollectionNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.Get(userID, collection.ID)
}

func (s *CollectionApplicationService) Create(actor AuditActor, collection models.Collection) (*models.Collection, error) {
	if err := sanitizeCollectionModel(&collection); err != nil {
		return nil, err
	}
	if strings.TrimSpace(collection.Name) == "" {
		return nil, collectionValidation("name is required")
	}
	if err := s.validatePublicPermission(actor.UserID, collection.IsPublic || collection.PublicSlug != nil); err != nil {
		return nil, err
	}
	if err := s.validateSlug(collection.PublicSlug, collection.IsPublic); err != nil {
		return nil, err
	}
	if err := s.validateWorkspaceAndCategory(actor.UserID, collection.WorkspaceID, collection.CategoryID); err != nil {
		return nil, err
	}
	if collection.IsPublic {
		if err := s.authorizePublicScope(actor.UserID, collection.QLQuery); err != nil {
			return nil, err
		}
	}
	if err := s.repository.Create(&collection, actor.UserID); err != nil {
		if database.IsUniqueConstraintError(err) {
			return nil, ErrCollectionConflict
		}
		return nil, err
	}
	emitServiceAudit(s.db, actor, logger.ActionCollectionCreate, logger.ResourceCollection, &collection.ID, collection.Name, nil)
	return s.repository.GetModel(collection.ID)
}

func (s *CollectionApplicationService) Update(actor AuditActor, id int, update CollectionUpdate) (*models.Collection, error) {
	collection, err := s.requireOwner(actor.UserID, id)
	if err != nil {
		return nil, err
	}
	if err := s.requireWorkspaceAccess(actor.UserID, collection); err != nil {
		return nil, err
	}
	oldPublic := collection.IsPublic
	if update.NameSet {
		collection.Name = update.Name
	}
	if update.DescriptionSet {
		collection.Description = update.Description
	}
	if update.QLQuerySet {
		collection.QLQuery = update.QLQuery
	}
	if update.FilterStateSet {
		collection.FilterState = update.FilterState
	}
	if update.WorkspaceIDSet {
		collection.WorkspaceID = update.WorkspaceID
	}
	if update.CategoryIDSet {
		collection.CategoryID = update.CategoryID
	}
	if update.IsPublicSet {
		collection.IsPublic = update.IsPublic
	}
	if update.IsPrivateSet {
		collection.IsPrivate = update.IsPrivate
	}
	if update.PublicSlugSet {
		collection.PublicSlug = update.PublicSlug
	}
	if update.PublicMilestoneSwimlanesSet {
		collection.PublicMilestoneSwimlanes = update.PublicMilestoneSwimlanes
	}
	// Private overrides public sharing: a collection marked private must never
	// stay anonymously readable at its public slug.
	if collection.IsPrivate {
		collection.IsPublic = false
		collection.PublicSlug = nil
	}
	if err := sanitizeCollectionModel(collection); err != nil {
		return nil, err
	}
	if strings.TrimSpace(collection.Name) == "" {
		return nil, collectionValidation("name is required")
	}
	publicPolicyChanged := update.IsPublicSet || update.PublicSlugSet || (collection.IsPublic && (update.QLQuerySet || update.WorkspaceIDSet))
	if err := s.validatePublicPermission(actor.UserID, publicPolicyChanged); err != nil {
		return nil, err
	}
	if err := s.validateSlug(collection.PublicSlug, collection.IsPublic); err != nil {
		return nil, err
	}
	if err := s.validateWorkspaceAndCategory(actor.UserID, collection.WorkspaceID, collection.CategoryID); err != nil {
		return nil, err
	}
	if collection.IsPublic && (!oldPublic || update.QLQuerySet || update.WorkspaceIDSet) {
		if err := s.authorizePublicScope(actor.UserID, collection.QLQuery); err != nil {
			return nil, err
		}
	}
	if err := s.repository.Update(id, collection); err != nil {
		if errors.Is(err, repository.ErrCollectionQueuesRequireWorkspace) {
			return nil, collectionValidation("remove collection queues before clearing its workspace")
		}
		if database.IsUniqueConstraintError(err) {
			return nil, ErrCollectionConflict
		}
		return nil, err
	}
	emitServiceAudit(s.db, actor, logger.ActionCollectionUpdate, logger.ResourceCollection, &id, collection.Name, nil)
	return s.repository.GetModel(id)
}

func (s *CollectionApplicationService) UpdateSharing(actor AuditActor, id int, update CollectionSharingUpdate) (*models.Collection, error) {
	collection, err := s.requireOwner(actor.UserID, id)
	if err != nil {
		return nil, err
	}
	if err := s.requireWorkspaceAccess(actor.UserID, collection); err != nil {
		return nil, err
	}
	// Private always overrides public sharing, so a private collection cannot be
	// published. Reject the contradictory request instead of storing an is_public
	// flag the public board will never honor, and clear any slug left behind.
	if collection.IsPrivate {
		if update.IsPublic {
			return nil, collectionValidation("make the collection shared before publishing its board")
		}
		update.PublicSlug = nil
	}
	if update.PublicSlug != nil {
		value := sanitize.ShortIdentifier.Sanitize(*update.PublicSlug)
		update.PublicSlug = &value
	}
	if err := s.validatePublicPermission(actor.UserID, true); err != nil {
		return nil, err
	}
	if err := s.validateSlug(update.PublicSlug, update.IsPublic); err != nil {
		return nil, err
	}
	if update.IsPublic {
		if err := s.authorizePublicScope(actor.UserID, collection.QLQuery); err != nil {
			return nil, err
		}
	}
	if s.beforePublicSharingWrite != nil {
		s.beforePublicSharingWrite()
	}
	publicMilestoneSwimlanes := collection.PublicMilestoneSwimlanes
	if update.PublicMilestoneSwimlanes != nil {
		publicMilestoneSwimlanes = *update.PublicMilestoneSwimlanes
	}
	// Publishing is conditional on the authorized scope still being current, so
	// a query edit between authorization and this write cannot make an
	// unauthorized scope anonymously readable.
	updated, err := s.repository.UpdatePublicSharing(id, update.IsPublic, update.PublicSlug, publicMilestoneSwimlanes, collection.QLQuery, collection.WorkspaceID)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return nil, ErrCollectionConflict
		}
		return nil, err
	}
	if !updated {
		return nil, fmt.Errorf("%w: collection query changed before it could be published", ErrCollectionConflict)
	}
	emitServiceAudit(s.db, actor, logger.ActionCollectionUpdate, logger.ResourceCollection, &id, collection.Name, nil)
	return s.repository.GetModel(id)
}

func (s *CollectionApplicationService) Delete(actor AuditActor, id int) error {
	collection, err := s.requireOwner(actor.UserID, id)
	if err != nil {
		return err
	}
	if err := s.requireWorkspaceAccess(actor.UserID, collection); err != nil {
		return err
	}
	if err := s.repository.Delete(id); err != nil {
		return err
	}
	emitServiceAudit(s.db, actor, logger.ActionCollectionDelete, logger.ResourceCollection, &id, collection.Name, nil)
	return nil
}

func (s *CollectionApplicationService) requireOwner(userID, id int) (*models.Collection, error) {
	collection, err := s.repository.GetModel(id)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrCollectionNotFound
	}
	if err != nil {
		return nil, err
	}
	if collection.CreatedBy == nil || *collection.CreatedBy != userID {
		return nil, ErrCollectionForbidden
	}
	return collection, nil
}

func (s *CollectionApplicationService) requireWorkspaceAccess(userID int, collection *models.Collection) error {
	if collection.WorkspaceID == nil {
		return nil
	}
	allowed, err := s.permissions.HasWorkspacePermission(userID, *collection.WorkspaceID, models.PermissionItemView)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrCollectionNotFound
	}
	return nil
}

func (s *CollectionApplicationService) validateWorkspaceAndCategory(userID int, workspaceID, categoryID *int) error {
	if workspaceID != nil {
		allowed, err := s.permissions.HasWorkspacePermission(userID, *workspaceID, models.PermissionItemView)
		if err != nil {
			return err
		}
		if !allowed {
			return ErrCollectionNotFound
		}
	}
	if categoryID == nil {
		return nil
	}
	if workspaceID != nil {
		return collectionValidation("categories require a global collection")
	}
	exists, err := s.repository.CategoryExists(*categoryID)
	if err != nil {
		return err
	}
	if !exists {
		return collectionValidation("category was not found")
	}
	return nil
}

func (s *CollectionApplicationService) validatePublicPermission(userID int, required bool) error {
	if !required {
		return nil
	}
	admin, err := s.permissions.IsSystemAdmin(userID)
	if err != nil {
		return err
	}
	allowed, err := s.permissions.HasGlobalPermission(userID, models.PermissionPublicBoardManage)
	if err != nil {
		return err
	}
	if !admin && !allowed {
		return ErrCollectionForbidden
	}
	return nil
}

func (s *CollectionApplicationService) authorizePublicScope(userID int, query string) error {
	_, err := s.publicScope.AuthorizePublishing(userID, query)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrPublicBoardWorkspaceAdminRequired):
		return ErrCollectionForbidden
	case errors.Is(err, ErrPublicBoardWorkspaceScopeRequired):
		return collectionValidation("public collections require a workspace scope")
	case errors.Is(err, ErrPublicBoardWorkspaceNotFound):
		return collectionValidation("public collection query references an unknown workspace")
	default:
		return err
	}
}

func (s *CollectionApplicationService) validateSlug(slug *string, public bool) error {
	if public && (slug == nil || *slug == "") {
		return collectionValidation("public_slug is required when public sharing is enabled")
	}
	if slug != nil && *slug != "" && !collectionSlugPattern.MatchString(*slug) {
		return collectionValidation("public_slug must be 3-64 lowercase alphanumeric or hyphen characters")
	}
	return nil
}

func sanitizeCollectionModel(collection *models.Collection) error {
	collection.Name = sanitize.PlainTextField.Sanitize(collection.Name)
	collection.Description = sanitize.RichText.Sanitize(collection.Description)
	collection.QLQuery = sanitize.QueryText.Sanitize(collection.QLQuery)
	if collection.FilterState != nil {
		if err := sanitize.ValidateJSONPayload("filter_state", *collection.FilterState); err != nil {
			return collectionValidation(err.Error())
		}
	}
	return nil
}

func collectionValidation(message string) error {
	return &CollectionValidationError{Message: message}
}

func (s *CollectionApplicationService) GetBoardConfiguration(userID int, scope BoardConfigurationScope) (*models.BoardConfiguration, error) {
	if err := s.authorizeBoardRead(userID, scope); err != nil {
		return nil, err
	}
	config, err := s.loadBoardConfiguration(scope)
	switch {
	case errors.Is(err, repository.ErrNotFound) && scope.WorkspaceID != nil:
		return &models.BoardConfiguration{WorkspaceID: scope.WorkspaceID}, nil
	case errors.Is(err, repository.ErrNotFound) && scope.CollectionID != nil:
		// No row yet: report the effective (inherited) view settings so
		// navigation can render the scope without special-casing 404s.
		collection, getErr := s.repository.GetByID(*scope.CollectionID)
		if getErr != nil {
			return nil, boardRepositoryError(getErr)
		}
		empty := &models.BoardConfiguration{CollectionID: scope.CollectionID, WorkspaceID: collection.WorkspaceID}
		if err := s.applyEffectiveViewSettings(empty); err != nil {
			return nil, err
		}
		return empty, nil
	case err != nil:
		return nil, err
	}
	if scope.CollectionID != nil {
		if err := s.applyEffectiveViewSettings(config); err != nil {
			return nil, err
		}
	}
	config.Columns, err = s.boards.GetColumnsWithStatuses(config.ID)
	return config, err
}

func (s *CollectionApplicationService) GetBoardConfigurationBootstrap(ctx context.Context, userID int, scope BoardConfigurationScope, fallbackWorkspaceID *int) (*BoardConfigurationBootstrap, error) {
	if err := s.authorizeBoardRead(userID, scope); err != nil {
		return nil, err
	}
	config, err := s.loadBoardConfiguration(scope)
	switch {
	case errors.Is(err, repository.ErrNotFound):
		if scope.WorkspaceID != nil {
			config = &models.BoardConfiguration{WorkspaceID: scope.WorkspaceID}
		} else {
			// No row yet: report the effective (inherited) view settings so
			// navigation can render the scope without special-casing 404s.
			collection, getErr := s.repository.GetByID(*scope.CollectionID)
			if getErr != nil {
				return nil, boardRepositoryError(getErr)
			}
			config = &models.BoardConfiguration{CollectionID: scope.CollectionID, WorkspaceID: collection.WorkspaceID}
			if err := s.applyEffectiveViewSettings(config); err != nil {
				return nil, err
			}
		}
	case err != nil:
		return nil, err
	default:
		if scope.CollectionID != nil {
			if err := s.applyEffectiveViewSettings(config); err != nil {
				return nil, err
			}
		}
		if config.Columns, err = s.boards.GetColumnsWithStatuses(config.ID); err != nil {
			return nil, err
		}
	}

	if scope.WorkspaceID != nil {
		statuses, err := s.workspaces.GetStatusesForWorkspaces([]int{*scope.WorkspaceID})
		if err != nil {
			return nil, err
		}
		return &BoardConfigurationBootstrap{BoardConfiguration: config, Statuses: statuses, ReferencedWorkspaceIDs: []int{*scope.WorkspaceID}}, nil
	}

	collection, err := s.repository.GetByID(*scope.CollectionID)
	if err != nil {
		return nil, boardRepositoryError(err)
	}
	accessibleWorkspaceIDs, err := s.permissions.AccessibleWorkspaceIDs(userID)
	if err != nil {
		return nil, err
	}
	referencedWorkspaceIDs, err := s.items.ListDistinctWorkspaceIDsWithQLContext(ctx, collection.QLQuery, accessibleWorkspaceIDs, userID)
	if err != nil {
		slog.Warn("board configuration bootstrap: collection CQL workspace projection failed", "collection_id", collection.ID, "error", err)
		referencedWorkspaceIDs = []int{}
	}
	if len(referencedWorkspaceIDs) == 0 {
		candidate := fallbackWorkspaceID
		if candidate == nil {
			candidate = collection.WorkspaceID
		}
		if candidate != nil && slices.Contains(accessibleWorkspaceIDs, *candidate) {
			referencedWorkspaceIDs = []int{*candidate}
		}
	}
	statuses, err := s.workspaces.GetStatusesForWorkspaces(referencedWorkspaceIDs)
	if err != nil {
		return nil, err
	}
	return &BoardConfigurationBootstrap{
		Collection:             boardCollection(collection),
		BoardConfiguration:     config,
		Statuses:               statuses,
		ReferencedWorkspaceIDs: referencedWorkspaceIDs,
	}, nil
}

func (s *CollectionApplicationService) PutBoardConfiguration(actor AuditActor, scope BoardConfigurationScope, input models.BoardConfigurationRequest) (*models.BoardConfiguration, error) {
	if err := s.authorizeBoardWrite(actor.UserID, scope); err != nil {
		return nil, err
	}
	sanitizeBoardConfiguration(&input)
	if err := validateBoardConfiguration(input); err != nil {
		return nil, err
	}
	if err := validateViewSettings(input.ViewSettings, scope.CollectionID != nil); err != nil {
		return nil, err
	}
	if scope.WorkspaceID != nil {
		if err := s.validateWorkspaceDefaultView(*scope.WorkspaceID, input.ViewSettings); err != nil {
			return nil, err
		}
	}
	if scope.CollectionID != nil {
		collection, err := s.repository.GetByID(*scope.CollectionID)
		if err != nil {
			return nil, boardRepositoryError(err)
		}
		if collection.IsPublic {
			if err := ValidatePublicBoardCardFields(input.CardFields); err != nil {
				return nil, collectionValidation(err.Error())
			}
		}
	}

	current, err := s.loadBoardConfiguration(scope)
	action := logger.ActionBoardConfigUpdate
	switch {
	case errors.Is(err, repository.ErrNotFound):
		for _, column := range input.Columns {
			if column.ID != nil {
				return nil, collectionValidation("new board columns cannot include an id")
			}
		}
		_, err = s.boards.Create(scope.CollectionID, scope.WorkspaceID, &input)
		action = logger.ActionBoardConfigCreate
	case err == nil:
		existingColumns, loadErr := s.boards.GetColumns(current.ID)
		if loadErr != nil {
			return nil, loadErr
		}
		existingIDs := make(map[int]struct{}, len(existingColumns))
		for _, column := range existingColumns {
			existingIDs[column.ID] = struct{}{}
		}
		for _, column := range input.Columns {
			if column.ID != nil {
				if _, ok := existingIDs[*column.ID]; !ok {
					return nil, collectionValidation("board column does not belong to this configuration")
				}
			}
		}
		input.ViewSettings = mergeViewSettings(current.ViewSettings, input.ViewSettings)
		err = s.boards.Update(current.ID, &input)
	default:
		return nil, err
	}
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return nil, ErrBoardConflict
		}
		return nil, err
	}
	config, err := s.loadBoardConfiguration(scope)
	if err != nil {
		return nil, err
	}
	config.Columns, err = s.boards.GetColumnsWithStatuses(config.ID)
	if err != nil {
		return nil, err
	}
	emitServiceAudit(s.db, actor, action, logger.ResourceBoardConfiguration, &config.ID, "", nil)
	return config, nil
}

func (s *CollectionApplicationService) DeleteBoardConfiguration(actor AuditActor, scope BoardConfigurationScope) error {
	if err := s.authorizeBoardWrite(actor.UserID, scope); err != nil {
		return err
	}
	config, err := s.loadBoardConfiguration(scope)
	if errors.Is(err, repository.ErrNotFound) {
		return ErrCollectionNotFound
	}
	if err != nil {
		return err
	}
	if err := s.boards.Delete(config.ID); err != nil {
		return err
	}
	emitServiceAudit(s.db, actor, logger.ActionBoardConfigDelete, logger.ResourceBoardConfiguration, &config.ID, "", nil)
	return nil
}

func (s *CollectionApplicationService) loadBoardConfiguration(scope BoardConfigurationScope) (*models.BoardConfiguration, error) {
	var (
		config *models.BoardConfiguration
		err    error
	)
	switch {
	case scope.CollectionID != nil && scope.WorkspaceID == nil:
		config, err = s.boards.GetByCollectionID(*scope.CollectionID)
	case scope.WorkspaceID != nil && scope.CollectionID == nil:
		config, err = s.boards.GetByWorkspaceID(*scope.WorkspaceID)
	default:
		return nil, collectionValidation("exactly one board configuration scope is required")
	}
	if err != nil {
		return nil, err
	}
	normalizeStoredViewSettings(config.ViewSettings)
	return config, nil
}

func (s *CollectionApplicationService) authorizeBoardRead(userID int, scope BoardConfigurationScope) error {
	if scope.WorkspaceID != nil && scope.CollectionID == nil {
		allowed, err := s.permissions.HasWorkspacePermission(userID, *scope.WorkspaceID, models.PermissionItemView)
		if err != nil {
			return err
		}
		if !allowed {
			return ErrCollectionNotFound
		}
		return nil
	}
	if scope.CollectionID == nil || scope.WorkspaceID != nil {
		return collectionValidation("exactly one board configuration scope is required")
	}
	collection, err := s.repository.GetByID(*scope.CollectionID)
	if err != nil {
		return boardRepositoryError(err)
	}
	if collection.WorkspaceID != nil {
		allowed, err := s.permissions.HasWorkspacePermission(userID, *collection.WorkspaceID, models.PermissionItemView)
		if err != nil {
			return err
		}
		if !allowed {
			return ErrCollectionNotFound
		}
	}
	if collection.IsPrivate && (collection.CreatedBy == nil || *collection.CreatedBy != userID) {
		return ErrCollectionNotFound
	}
	return nil
}

func (s *CollectionApplicationService) authorizeBoardWrite(userID int, scope BoardConfigurationScope) error {
	if scope.WorkspaceID != nil && scope.CollectionID == nil {
		allowed, err := s.permissions.HasWorkspacePermission(userID, *scope.WorkspaceID, models.PermissionWorkspaceAdmin)
		if err != nil {
			return err
		}
		if !allowed {
			return ErrCollectionNotFound
		}
		return nil
	}
	if scope.CollectionID == nil || scope.WorkspaceID != nil {
		return collectionValidation("exactly one board configuration scope is required")
	}
	collection, err := s.repository.GetByID(*scope.CollectionID)
	if err != nil {
		return boardRepositoryError(err)
	}
	if collection.WorkspaceID != nil {
		allowed, err := s.permissions.HasWorkspacePermission(userID, *collection.WorkspaceID, models.PermissionItemView)
		if err != nil {
			return err
		}
		if !allowed {
			return ErrCollectionNotFound
		}
	}
	if collection.CreatedBy == nil || *collection.CreatedBy != userID {
		return ErrCollectionNotFound
	}
	return nil
}

func validateBoardConfiguration(input models.BoardConfigurationRequest) error {
	if input.CompletedItemRetentionDays == nil {
		return nil
	}
	if input.ShowRightmostColumnLast50 {
		return collectionValidation("show_rightmost_column_last_50 and completed_item_retention_days cannot both be enabled")
	}
	days := *input.CompletedItemRetentionDays
	if days < 1 || days > 3650 {
		return collectionValidation("completed_item_retention_days must be between 1 and 3650")
	}
	return nil
}

// normalizeStoredViewSettings drops enabled-views ids that are no longer
// known, so overrides stored before the toggleable set changed still read
// and round-trip cleanly instead of failing every subsequent save. A set
// reduced to nothing falls back to inherit. The write path stays strict:
// only reads are tolerant.
func normalizeStoredViewSettings(settings *models.ViewSettings) {
	if settings == nil || settings.EnabledViews == nil || len(*settings.EnabledViews) == 0 {
		return
	}
	known := make(map[string]struct{}, len(models.WorkspaceNavItemIDs))
	for _, id := range models.WorkspaceNavItemIDs {
		known[id] = struct{}{}
	}
	kept := make([]string, 0, len(*settings.EnabledViews))
	for _, id := range *settings.EnabledViews {
		if _, ok := known[id]; ok {
			kept = append(kept, id)
		}
	}
	if len(kept) == len(*settings.EnabledViews) {
		return
	}
	if len(kept) == 0 {
		settings.EnabledViews = nil
		return
	}
	settings.EnabledViews = &kept
}

// validateViewSettings checks an explicit enabled-views key: known nav
// ids for the scope, no duplicates, at least one entry. A null key
// (pointer to a nil slice) is the reset form and passes. Workspace-scope
// overrides may toggle any workspace nav item; collection-scope overrides
// are limited to the collection-scoped views because tools and
// test-management entries only render in the workspace default context.
func validateViewSettings(settings *models.ViewSettings, collectionScope bool) error {
	if settings == nil || settings.EnabledViews == nil {
		return nil
	}
	views := *settings.EnabledViews
	if views == nil {
		return nil
	}
	if len(views) == 0 {
		return collectionValidation("enabled_views must contain at least one view")
	}
	known := make(map[string]struct{}, len(models.WorkspaceNavItemIDs))
	for _, id := range models.WorkspaceNavItemIDs {
		known[id] = struct{}{}
	}
	seen := make(map[string]struct{}, len(views))
	for _, id := range views {
		if _, ok := known[id]; !ok {
			return collectionValidation(fmt.Sprintf("unknown view %q in enabled_views", id))
		}
		if collectionScope && !models.IsCollectionViewID(id) {
			return collectionValidation(fmt.Sprintf("view %q can only be toggled at the workspace scope", id))
		}
		if _, dup := seen[id]; dup {
			return collectionValidation(fmt.Sprintf("duplicate view %q in enabled_views", id))
		}
		seen[id] = struct{}{}
	}
	return nil
}

// validateWorkspaceDefaultView rejects a workspace-scope enabled-views
// override that disables the workspace's current default view. The caller
// changes the default view via the workspace API first.
func (s *CollectionApplicationService) validateWorkspaceDefaultView(workspaceID int, settings *models.ViewSettings) error {
	if !settings.HasEnabledViewsOverride() {
		return nil
	}
	workspace, err := s.workspaces.GetByID(workspaceID)
	if err != nil {
		return err
	}
	if !slices.Contains(models.BoardViewIDs, workspace.DefaultView) {
		return nil
	}
	if !slices.Contains(*settings.EnabledViews, workspace.DefaultView) {
		return collectionValidation(fmt.Sprintf("workspace default view %q must stay enabled; change the workspace default view first", workspace.DefaultView))
	}
	return nil
}

// mergeViewSettings applies the request's view settings on top of the stored
// ones so keys the request omits keep their stored value.
func mergeViewSettings(current, update *models.ViewSettings) *models.ViewSettings {
	if update == nil {
		return current
	}
	merged := models.ViewSettings{}
	if current != nil {
		merged = *current
	}
	if update.EnabledViews != nil {
		merged.EnabledViews = update.EnabledViews
	}
	if update.MilestoneSwimlanes != nil {
		merged.MilestoneSwimlanes = update.MilestoneSwimlanes
	}
	return &merged
}

// applyEffectiveViewSettings resolves the effective view settings for a
// collection-scoped configuration: the collection's own view override (with
// workspace-scope non-view ids inherited underneath), or the workspace
// effective set, or every collection view enabled. Collection-scoped rows
// carry no workspace id, so it is resolved from the collection record when
// needed.
func (s *CollectionApplicationService) applyEffectiveViewSettings(config *models.BoardConfiguration) error {
	normalizeStoredViewSettings(config.ViewSettings)
	// Milestone-swimlane preferences share the board-options JSON but are
	// independent of the enabled-views inherit/reset state: a collection's own
	// value wins, otherwise the workspace default is inherited.
	var ownMilestones *models.MilestoneSwimlaneConfig
	if config.ViewSettings != nil {
		ownMilestones = config.ViewSettings.MilestoneSwimlanes
	}
	workspaceID := config.WorkspaceID
	if workspaceID == nil && config.CollectionID != nil {
		collection, err := s.repository.GetByID(*config.CollectionID)
		if err != nil {
			return boardRepositoryError(err)
		}
		workspaceID = collection.WorkspaceID
	}
	wsConfig, wsErr := func() (*models.BoardConfiguration, error) {
		if workspaceID == nil {
			return nil, repository.ErrNotFound
		}
		row, err := s.boards.GetByWorkspaceID(*workspaceID)
		if err != nil {
			return nil, err
		}
		normalizeStoredViewSettings(row.ViewSettings)
		return row, nil
	}()
	resolvedMilestones := ownMilestones
	if resolvedMilestones == nil && wsErr == nil && wsConfig.ViewSettings != nil {
		resolvedMilestones = wsConfig.ViewSettings.MilestoneSwimlanes
	}
	settingsWith := func(views *[]string) *models.ViewSettings {
		return &models.ViewSettings{EnabledViews: views, MilestoneSwimlanes: resolvedMilestones}
	}
	allViews := func() *models.ViewSettings {
		views := slices.Clone(models.BoardViewIDs)
		return settingsWith(&views)
	}
	if config.CollectionID != nil && config.ViewSettings.HasEnabledViewsOverride() {
		// The collection override speaks only for the collection-scoped
		// views; workspace-only nav ids always inherit from the workspace
		// row. Merge both parts in canonical registry order.
		views := slices.Clone(*config.ViewSettings.EnabledViews)
		if wsErr == nil && wsConfig.ViewSettings.HasEnabledViewsOverride() {
			views = mergeNavIDs(views, *wsConfig.ViewSettings.EnabledViews)
		}
		config.ViewSettings = settingsWith(&views)
		return nil
	}
	if config.ViewSettings.HasEnabledViewsOverride() {
		config.ViewSettings.MilestoneSwimlanes = resolvedMilestones
		return nil
	}
	config.ViewSettingsInherited = true
	if wsErr != nil {
		if errors.Is(wsErr, repository.ErrNotFound) {
			config.ViewSettings = allViews()
			return nil
		}
		return wsErr
	}
	if wsConfig.ViewSettings.HasEnabledViewsOverride() {
		views := slices.Clone(*wsConfig.ViewSettings.EnabledViews)
		config.ViewSettings = settingsWith(&views)
		return nil
	}
	config.ViewSettings = allViews()
	return nil
}

// mergeNavIDs returns the union of override and inherited ids, ordered by
// the canonical WorkspaceNavItemIDs registry so API responses are stable.
func mergeNavIDs(override, inherited []string) []string {
	present := make(map[string]struct{}, len(override)+len(inherited))
	for _, id := range override {
		present[id] = struct{}{}
	}
	for _, id := range inherited {
		if !models.IsCollectionViewID(id) {
			present[id] = struct{}{}
		}
	}
	merged := make([]string, 0, len(present))
	for _, id := range models.WorkspaceNavItemIDs {
		if _, ok := present[id]; ok {
			merged = append(merged, id)
		}
	}
	return merged
}

func sanitizeBoardConfiguration(input *models.BoardConfigurationRequest) {
	for i := range input.Columns {
		sanitize.ApplyAll(
			sanitize.Pair{Target: &input.Columns[i].Name, Policy: sanitize.PlainTextField},
			sanitize.Pair{Target: &input.Columns[i].Color, Policy: sanitize.ShortIdentifier},
		)
	}
}

func sanitizeCollectionCategory(category *models.CollectionCategory) {
	sanitize.ApplyAll(
		sanitize.Pair{Target: &category.Name, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &category.Color, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: &category.Description, Policy: sanitize.PlainTextField},
	)
}

func ValidatePublicBoardCardFields(fields []models.ListColumn) error {
	allowed := map[string]struct{}{
		"key": {}, "title": {}, "status": {}, "priority": {}, "assignee": {},
		"item_type": {}, "story_points": {}, "due_date": {}, "labels": {},
	}
	for _, field := range fields {
		if field.FieldType != "system" {
			return fmt.Errorf("card field %q is not approved for public boards", field.FieldIdentifier)
		}
		if _, ok := allowed[field.FieldIdentifier]; !ok {
			return fmt.Errorf("card field %q is not supported on public boards", field.FieldIdentifier)
		}
	}
	return nil
}

func boardCollection(collection *repository.CollectionRecord) *BoardConfigurationCollection {
	result := &BoardConfigurationCollection{
		ID: collection.ID, Name: collection.Name, Description: collection.Description,
		QLQuery: collection.QLQuery, IsPublic: collection.IsPublic,
		WorkspaceID: collection.WorkspaceID, CreatedBy: collection.CreatedBy,
	}
	if collection.Slug != "" {
		result.PublicSlug = &collection.Slug
	}
	return result
}

func boardRepositoryError(err error) error {
	if errors.Is(err, repository.ErrNotFound) {
		return ErrCollectionNotFound
	}
	var serviceError *ServiceError
	if errors.As(err, &serviceError) && serviceError.StatusCode == http.StatusNotFound {
		return ErrCollectionNotFound
	}
	return err
}
