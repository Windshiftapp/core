package services

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
)

type CatalogMutationService struct {
	db          database.Database
	permissions *PermissionService
	itemTypes   *repository.ItemTypeRepository
	priorities  *repository.PriorityRepository
	linkTypes   *repository.LinkTypeRepository
	statuses    *EnumService
	categories  *EnumService
	workflows   *repository.WorkflowRepository
	workflow    *WorkflowService
}

func NewCatalogMutationService(db database.Database, permissions *PermissionService, workflow *WorkflowService) *CatalogMutationService {
	return &CatalogMutationService{
		db: db, permissions: permissions, itemTypes: repository.NewItemTypeRepository(db),
		priorities: repository.NewPriorityRepository(db), linkTypes: repository.NewLinkTypeRepository(db),
		statuses: NewEnumService(db, NewStatusConfig()), categories: NewEnumService(db, NewStatusCategoryConfig()),
		workflows: repository.NewWorkflowRepository(db), workflow: workflow,
	}
}

type ItemTypePatch struct {
	Name, Description, Icon, Color *string
	IsDefault                      *bool
	HierarchyLevel, SortOrder      *int
	ConfigurationSetIDs            *[]int
}

type PriorityPatch struct {
	Name, Description, Icon, Color *string
	IsDefault                      *bool
	SortOrder                      *int
	ConfigurationSetIDs            *[]int
}

type StatusPatch struct {
	Name, Description *string
	CategoryID        *int
	IsDefault         *bool
}

type StatusCategoryPatch struct {
	Name, Description, Color *string
	IsDefault, IsCompleted   *bool
}

type WorkflowPatch struct {
	Name, Description *string
	IsDefault         *bool
}

type LinkTypePatch struct {
	Name, Description, ForwardLabel, ReverseLabel, Color *string
	Active                                               *bool
	AllowedEntityTypes                                   *[]string
}

func (s *CatalogMutationService) CreateItemType(actor AuditActor, item models.ItemType) (*models.ItemType, error) {
	if err := s.requireAdmin(actor.UserID); err != nil {
		return nil, err
	}
	if err := s.prepareItemType(&item); err != nil {
		return nil, err
	}
	id, err := s.itemTypes.Create(&item, item.ConfigurationSetIDs)
	if err != nil {
		return nil, catalogRepositoryError(err)
	}
	created, err := s.itemTypes.GetByID(id)
	if err == nil {
		emitServiceAudit(s.db, actor, logger.ActionItemTypeCreate, logger.ResourceItemType, &id, created.Name, nil)
	}
	return created, err
}

func (s *CatalogMutationService) PatchItemType(actor AuditActor, id int, patch ItemTypePatch) (*models.ItemType, error) {
	if err := s.requireAdmin(actor.UserID); err != nil {
		return nil, err
	}
	item, err := s.itemTypes.GetByID(id)
	if err != nil {
		return nil, err
	}
	applyItemTypePatch(item, patch)
	if err := s.prepareItemType(item); err != nil {
		return nil, err
	}
	if err := s.itemTypes.Update(id, item, item.ConfigurationSetIDs); err != nil {
		return nil, catalogRepositoryError(err)
	}
	updated, err := s.itemTypes.GetByID(id)
	if err == nil {
		emitServiceAudit(s.db, actor, logger.ActionItemTypeUpdate, logger.ResourceItemType, &id, updated.Name, nil)
	}
	return updated, err
}

func (s *CatalogMutationService) DeleteItemType(actor AuditActor, id int) error {
	if err := s.requireAdmin(actor.UserID); err != nil {
		return err
	}
	item, err := s.itemTypes.GetByID(id)
	if err != nil {
		return err
	}
	count, err := repository.NewItemRepository(s.db).CountByField("item_type_id", id)
	if err != nil {
		return err
	}
	if count > 0 {
		return NewServiceError(http.StatusConflict, fmt.Sprintf("item type is used by %d work items", count))
	}
	if err := s.itemTypes.Delete(id); err != nil {
		return err
	}
	emitServiceAudit(s.db, actor, logger.ActionItemTypeDelete, logger.ResourceItemType, &id, item.Name, nil)
	return nil
}

func (s *CatalogMutationService) CreatePriority(actor AuditActor, item models.Priority) (*models.Priority, error) {
	if err := s.requireAdmin(actor.UserID); err != nil {
		return nil, err
	}
	if err := s.preparePriority(&item, 0); err != nil {
		return nil, err
	}
	id, err := s.priorities.Create(&item)
	if err != nil {
		return nil, catalogRepositoryError(err)
	}
	created, err := s.priorities.GetByID(id)
	if err == nil {
		emitServiceAudit(s.db, actor, "priority.create", "priority", &id, created.Name, nil)
	}
	return created, err
}

func (s *CatalogMutationService) PatchPriority(actor AuditActor, id int, patch PriorityPatch) (*models.Priority, error) {
	if err := s.requireAdmin(actor.UserID); err != nil {
		return nil, err
	}
	item, err := s.priorities.GetByID(id)
	if err != nil {
		return nil, err
	}
	applyPriorityPatch(item, patch)
	if err := s.preparePriority(item, id); err != nil {
		return nil, err
	}
	if err := s.priorities.Update(id, item); err != nil {
		return nil, catalogRepositoryError(err)
	}
	updated, err := s.priorities.GetByID(id)
	if err == nil {
		emitServiceAudit(s.db, actor, "priority.update", "priority", &id, updated.Name, nil)
	}
	return updated, err
}

func (s *CatalogMutationService) DeletePriority(actor AuditActor, id int) error {
	if err := s.requireAdmin(actor.UserID); err != nil {
		return err
	}
	item, err := s.priorities.GetByID(id)
	if err != nil {
		return err
	}
	count, err := repository.NewItemRepository(s.db).CountByField("priority_id", id)
	if err != nil {
		return err
	}
	if count > 0 {
		return NewServiceError(http.StatusConflict, fmt.Sprintf("priority is used by %d work items", count))
	}
	if err := s.priorities.Delete(id); err != nil {
		return err
	}
	emitServiceAudit(s.db, actor, "priority.delete", "priority", &id, item.Name, nil)
	return nil
}

func (s *CatalogMutationService) CreateStatus(actor AuditActor, item models.Status) (*models.Status, error) {
	if err := s.requireAdmin(actor.UserID); err != nil {
		return nil, err
	}
	sanitizeCatalogText(&item.Name, &item.Description)
	entity, err := s.statuses.Create(&item, nil)
	if err != nil {
		return nil, err
	}
	created, ok := entity.(*models.Status)
	if !ok {
		return nil, fmt.Errorf("create status returned %T", entity)
	}
	emitServiceAudit(s.db, actor, "status.create", "status", &created.ID, created.Name, nil)
	return created, nil
}

func (s *CatalogMutationService) PatchStatus(actor AuditActor, id int, patch StatusPatch) (*models.Status, error) {
	if err := s.requireAdmin(actor.UserID); err != nil {
		return nil, err
	}
	entity, err := s.statuses.GetByID(id)
	if err != nil {
		return nil, err
	}
	item, ok := entity.(*models.Status)
	if !ok {
		return nil, fmt.Errorf("get status returned %T", entity)
	}
	if patch.Name != nil {
		item.Name = *patch.Name
	}
	if patch.Description != nil {
		item.Description = *patch.Description
	}
	if patch.CategoryID != nil {
		item.CategoryID = *patch.CategoryID
	}
	if patch.IsDefault != nil {
		item.IsDefault = *patch.IsDefault
	}
	sanitizeCatalogText(&item.Name, &item.Description)
	entity, err = s.statuses.Update(id, item, nil)
	if err != nil {
		return nil, err
	}
	updated, ok := entity.(*models.Status)
	if !ok {
		return nil, fmt.Errorf("update status returned %T", entity)
	}
	emitServiceAudit(s.db, actor, "status.update", "status", &id, updated.Name, nil)
	return updated, nil
}

func (s *CatalogMutationService) DeleteStatus(actor AuditActor, id int) error {
	if err := s.requireAdmin(actor.UserID); err != nil {
		return err
	}
	entity, err := s.statuses.GetByID(id)
	if err != nil {
		return err
	}
	if err := s.statuses.Delete(id, nil); err != nil {
		return err
	}
	emitServiceAudit(s.db, actor, "status.delete", "status", &id, entity.GetName(), nil)
	return nil
}

func (s *CatalogMutationService) CreateStatusCategory(actor AuditActor, item models.StatusCategory) (*models.StatusCategory, error) {
	if err := s.requireAdmin(actor.UserID); err != nil {
		return nil, err
	}
	sanitizeCategory(&item)
	entity, err := s.categories.Create(&item, nil)
	if err != nil {
		return nil, err
	}
	created, ok := entity.(*models.StatusCategory)
	if !ok {
		return nil, fmt.Errorf("create status category returned %T", entity)
	}
	emitServiceAudit(s.db, actor, "status_category.create", "status_category", &created.ID, created.Name, nil)
	return created, nil
}

func (s *CatalogMutationService) PatchStatusCategory(actor AuditActor, id int, patch StatusCategoryPatch) (*models.StatusCategory, error) {
	if err := s.requireAdmin(actor.UserID); err != nil {
		return nil, err
	}
	entity, err := s.categories.GetByID(id)
	if err != nil {
		return nil, err
	}
	item, ok := entity.(*models.StatusCategory)
	if !ok {
		return nil, fmt.Errorf("get status category returned %T", entity)
	}
	if patch.Name != nil {
		item.Name = *patch.Name
	}
	if patch.Description != nil {
		item.Description = *patch.Description
	}
	if patch.Color != nil {
		item.Color = *patch.Color
	}
	if patch.IsDefault != nil {
		item.IsDefault = *patch.IsDefault
	}
	if patch.IsCompleted != nil {
		item.IsCompleted = *patch.IsCompleted
	}
	sanitizeCategory(item)
	entity, err = s.categories.Update(id, item, nil)
	if err != nil {
		return nil, err
	}
	updated, ok := entity.(*models.StatusCategory)
	if !ok {
		return nil, fmt.Errorf("update status category returned %T", entity)
	}
	emitServiceAudit(s.db, actor, "status_category.update", "status_category", &id, updated.Name, nil)
	return updated, nil
}

func (s *CatalogMutationService) DeleteStatusCategory(actor AuditActor, id int) error {
	if err := s.requireAdmin(actor.UserID); err != nil {
		return err
	}
	entity, err := s.categories.GetByID(id)
	if err != nil {
		return err
	}
	if err := s.categories.Delete(id, nil); err != nil {
		return err
	}
	emitServiceAudit(s.db, actor, "status_category.delete", "status_category", &id, entity.GetName(), nil)
	return nil
}

func (s *CatalogMutationService) NonDoneStatusIDs() ([]int, error) {
	return repository.NewStatusRepository(s.db).ListNonDoneIDs()
}

func (s *CatalogMutationService) CreateWorkflow(actor AuditActor, item models.Workflow) (*WorkflowResult, error) {
	if err := s.requireAdmin(actor.UserID); err != nil {
		return nil, err
	}
	if err := prepareWorkflow(&item); err != nil {
		return nil, err
	}
	exists, err := s.workflows.NameExists(item.Name)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, NewServiceError(http.StatusConflict, "workflow name already exists")
	}
	id, err := s.workflows.Create(item.Name, item.Description, item.IsDefault)
	if err != nil {
		return nil, err
	}
	s.workflow.InvalidateInitialStatusCache()
	created, err := s.workflow.GetByID(id)
	if err == nil {
		emitServiceAudit(s.db, actor, logger.ActionWorkflowCreate, logger.ResourceWorkflow, &id, item.Name, nil)
	}
	return created, err
}

func (s *CatalogMutationService) PatchWorkflow(actor AuditActor, id int, patch WorkflowPatch) (*WorkflowResult, error) {
	if err := s.requireAdmin(actor.UserID); err != nil {
		return nil, err
	}
	current, err := s.workflows.Get(id)
	if err != nil {
		return nil, err
	}
	if patch.Name != nil {
		current.Name = *patch.Name
	}
	if patch.Description != nil {
		current.Description = *patch.Description
	}
	if patch.IsDefault != nil {
		current.IsDefault = *patch.IsDefault
	}
	if err := prepareWorkflow(current); err != nil {
		return nil, err
	}
	exists, err := s.workflows.NameExistsExcluding(current.Name, id)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, NewServiceError(http.StatusConflict, "workflow name already exists")
	}
	if err := s.workflows.Update(id, current.Name, current.Description, current.IsDefault); err != nil {
		return nil, err
	}
	s.workflow.InvalidateInitialStatusCache()
	updated, err := s.workflow.GetByID(id)
	if err == nil {
		emitServiceAudit(s.db, actor, logger.ActionWorkflowUpdate, logger.ResourceWorkflow, &id, updated.Name, nil)
	}
	return updated, err
}

func (s *CatalogMutationService) DeleteWorkflow(actor AuditActor, id int) error {
	if err := s.requireAdmin(actor.UserID); err != nil {
		return err
	}
	current, err := s.workflows.Get(id)
	if err != nil {
		return err
	}
	count, err := s.workflows.ConfigurationSetCount(id)
	if err != nil {
		return err
	}
	if count > 0 {
		return NewServiceError(http.StatusConflict, "workflow is used by configuration sets")
	}
	canceled, err := s.workflows.Delete(id)
	if err != nil {
		return err
	}
	s.workflow.InvalidateInitialStatusCache()
	emitServiceAudit(s.db, actor, logger.ActionWorkflowDelete, logger.ResourceWorkflow, &id, current.Name, map[string]any{"canceled_approval_request_ids": canceled})
	return nil
}

func (s *CatalogMutationService) ReplaceWorkflowTransitions(actor AuditActor, workflowID int, transitions []models.WorkflowTransition) ([]WorkflowTransitionResult, error) {
	if err := s.requireAdmin(actor.UserID); err != nil {
		return nil, err
	}
	if _, err := s.workflows.Get(workflowID); err != nil {
		return nil, err
	}
	canceled, err := s.workflows.ReplaceTransitions(workflowID, transitions)
	if err != nil {
		return nil, err
	}
	s.workflow.InvalidateInitialStatusCache()
	emitServiceAudit(s.db, actor, logger.ActionWorkflowUpdate, logger.ResourceWorkflow, &workflowID, "", map[string]any{"update_type": "transitions", "canceled_approval_request_ids": canceled})
	return s.workflow.GetTransitions(workflowID)
}

func (s *CatalogMutationService) ListLinkTypes(includeInactive bool) ([]models.LinkType, error) {
	return s.linkTypes.List(includeInactive)
}

func (s *CatalogMutationService) GetLinkType(id int) (*models.LinkType, error) {
	return s.linkTypes.GetByID(id)
}

func (s *CatalogMutationService) CreateLinkType(actor AuditActor, item models.LinkType) (*models.LinkType, error) {
	if err := s.requireAdmin(actor.UserID); err != nil {
		return nil, err
	}
	prepareLinkType(&item)
	if err := validateLinkType(item); err != nil {
		return nil, err
	}
	id, _, err := s.linkTypes.Create(&item)
	if err != nil {
		return nil, catalogRepositoryError(err)
	}
	created, err := s.linkTypes.GetByID(id)
	if err == nil {
		emitServiceAudit(s.db, actor, logger.ActionLinkTypeCreate, logger.ResourceLinkType, &id, created.Name, nil)
	}
	return created, err
}

func (s *CatalogMutationService) PatchLinkType(actor AuditActor, id int, patch LinkTypePatch) (*models.LinkType, error) {
	if err := s.requireAdmin(actor.UserID); err != nil {
		return nil, err
	}
	item, err := s.linkTypes.GetByID(id)
	if err != nil {
		return nil, err
	}
	if item.IsSystem {
		return nil, ErrCatalogForbidden
	}
	if patch.Name != nil {
		item.Name = *patch.Name
	}
	if patch.Description != nil {
		item.Description = *patch.Description
	}
	if patch.ForwardLabel != nil {
		item.ForwardLabel = *patch.ForwardLabel
	}
	if patch.ReverseLabel != nil {
		item.ReverseLabel = *patch.ReverseLabel
	}
	if patch.Color != nil {
		item.Color = *patch.Color
	}
	if patch.Active != nil {
		item.Active = *patch.Active
	}
	if patch.AllowedEntityTypes != nil {
		item.AllowedEntityTypes = *patch.AllowedEntityTypes
	}
	prepareLinkType(item)
	if err := validateLinkType(*item); err != nil {
		return nil, err
	}
	if _, err := s.linkTypes.Update(id, item); err != nil {
		return nil, catalogRepositoryError(err)
	}
	updated, err := s.linkTypes.GetByID(id)
	if err == nil {
		emitServiceAudit(s.db, actor, logger.ActionLinkTypeUpdate, logger.ResourceLinkType, &id, updated.Name, nil)
	}
	return updated, err
}

func (s *CatalogMutationService) DeleteLinkType(actor AuditActor, id int) error {
	if err := s.requireAdmin(actor.UserID); err != nil {
		return err
	}
	item, err := s.linkTypes.GetByID(id)
	if err != nil {
		return err
	}
	if item.IsSystem {
		return ErrCatalogForbidden
	}
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM item_links WHERE link_type_id = ?", id).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return NewServiceError(http.StatusConflict, "link type is in use")
	}
	if err := s.linkTypes.Delete(id); err != nil {
		return err
	}
	emitServiceAudit(s.db, actor, logger.ActionLinkTypeDelete, logger.ResourceLinkType, &id, item.Name, nil)
	return nil
}

func (s *CatalogMutationService) requireAdmin(userID int) error {
	allowed, err := s.permissions.IsSystemAdmin(userID)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrCatalogForbidden
	}
	return nil
}

func (s *CatalogMutationService) prepareItemType(item *models.ItemType) error {
	sanitize.ApplyAll(sanitize.Pair{Target: &item.Name, Policy: sanitize.PlainTextField}, sanitize.Pair{Target: &item.Description, Policy: sanitize.RichText}, sanitize.Pair{Target: &item.Icon, Policy: sanitize.ShortIdentifier}, sanitize.Pair{Target: &item.Color, Policy: sanitize.ShortIdentifier})
	if strings.TrimSpace(item.Name) == "" {
		return NewServiceError(http.StatusBadRequest, "item type name is required")
	}
	for _, id := range item.ConfigurationSetIDs {
		exists, err := s.itemTypes.ConfigurationSetExists(id)
		if err != nil {
			return err
		}
		if !exists {
			return NewServiceError(http.StatusBadRequest, fmt.Sprintf("configuration set %d was not found", id))
		}
	}
	exists, err := s.itemTypes.NameExists(item.Name, item.ID)
	if err != nil {
		return err
	}
	if exists {
		return NewServiceError(http.StatusConflict, "item type name already exists")
	}
	return nil
}

func applyItemTypePatch(item *models.ItemType, patch ItemTypePatch) {
	if patch.Name != nil {
		item.Name = *patch.Name
	}
	if patch.Description != nil {
		item.Description = *patch.Description
	}
	if patch.Icon != nil {
		item.Icon = *patch.Icon
	}
	if patch.Color != nil {
		item.Color = *patch.Color
	}
	if patch.IsDefault != nil {
		item.IsDefault = *patch.IsDefault
	}
	if patch.HierarchyLevel != nil {
		item.HierarchyLevel = *patch.HierarchyLevel
	}
	if patch.SortOrder != nil {
		item.SortOrder = *patch.SortOrder
	}
	if patch.ConfigurationSetIDs != nil {
		item.ConfigurationSetIDs = *patch.ConfigurationSetIDs
	}
}

func (s *CatalogMutationService) preparePriority(item *models.Priority, excludeID int) error {
	sanitizeCatalogText(&item.Name, &item.Description)
	if strings.TrimSpace(item.Name) == "" {
		return NewServiceError(http.StatusBadRequest, "priority name is required")
	}
	for _, id := range item.ConfigurationSetIDs {
		exists, err := s.priorities.ConfigurationSetExists(id)
		if err != nil {
			return err
		}
		if !exists {
			return NewServiceError(http.StatusBadRequest, fmt.Sprintf("configuration set %d was not found", id))
		}
	}
	exists, err := s.priorities.NameExists(item.Name, excludeID)
	if err != nil {
		return err
	}
	if exists {
		return NewServiceError(http.StatusConflict, "priority name already exists")
	}
	if item.IsDefault {
		return s.priorities.ClearOtherDefaults(excludeID)
	}
	return nil
}

func applyPriorityPatch(item *models.Priority, patch PriorityPatch) {
	if patch.Name != nil {
		item.Name = *patch.Name
	}
	if patch.Description != nil {
		item.Description = *patch.Description
	}
	if patch.Icon != nil {
		item.Icon = *patch.Icon
	}
	if patch.Color != nil {
		item.Color = *patch.Color
	}
	if patch.IsDefault != nil {
		item.IsDefault = *patch.IsDefault
	}
	if patch.SortOrder != nil {
		item.SortOrder = *patch.SortOrder
	}
	if patch.ConfigurationSetIDs != nil {
		item.ConfigurationSetIDs = *patch.ConfigurationSetIDs
	}
}

func sanitizeCatalogText(name, description *string) {
	sanitize.ApplyAll(sanitize.Pair{Target: name, Policy: sanitize.PlainTextField}, sanitize.Pair{Target: description, Policy: sanitize.RichText})
}
func sanitizeCategory(item *models.StatusCategory) {
	sanitizeCatalogText(&item.Name, &item.Description)
	item.Color = sanitize.ShortIdentifier.Sanitize(item.Color)
}
func prepareWorkflow(item *models.Workflow) error {
	sanitizeCatalogText(&item.Name, &item.Description)
	if strings.TrimSpace(item.Name) == "" {
		return NewServiceError(http.StatusBadRequest, "workflow name is required")
	}
	return nil
}
func prepareLinkType(item *models.LinkType) {
	sanitizeCatalogText(&item.Name, &item.Description)
	item.ForwardLabel = sanitize.PlainTextField.Sanitize(item.ForwardLabel)
	item.ReverseLabel = sanitize.PlainTextField.Sanitize(item.ReverseLabel)
	item.Color = sanitize.ShortIdentifier.Sanitize(item.Color)
	if item.Color == "" {
		item.Color = "#6b7280"
	}
}
func validateLinkType(item models.LinkType) error {
	if item.Name == "" || item.ForwardLabel == "" || item.ReverseLabel == "" {
		return NewServiceError(http.StatusBadRequest, "name, forward_label, and reverse_label are required")
	}
	return nil
}
func catalogRepositoryError(err error) error {
	if errors.Is(err, repository.ErrDuplicateEntry) || database.IsUniqueConstraintError(err) {
		return NewServiceError(http.StatusConflict, "catalog name already exists")
	}
	return err
}
