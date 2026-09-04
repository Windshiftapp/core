package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"windshift/internal/cql"
	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
)

var (
	ErrAssetForbidden = errors.New("asset access forbidden")
	ErrAssetConflict  = errors.New("asset conflict")
)

type AssetSetPatch struct {
	Name, Description *string
	IsDefault         *bool
}
type AssetTypePatch struct {
	Name, Description, Icon, Color *string
	DisplayOrder                   *int
	IsActive                       *bool
}
type AssetCategoryPatch struct{ Name, Description *string }
type AssetStatusPatch struct {
	Name, Color, Description *string
	IsDefault                *bool
	DisplayOrder             *int
}
type AssetMutationInput struct {
	AssetTypeID                  int
	CategoryID, StatusID         *int
	Title, Description, AssetTag string
	CustomFieldValues            map[string]any
}
type AssetPatchInput struct {
	AssetTypeID       *int
	CategoryID        *int
	CategoryIDSet     bool
	StatusID          *int
	StatusIDSet       bool
	Title             *string
	Description       *string
	AssetTag          *string
	CustomFieldValues *map[string]any
}

type AssetSetRoles struct {
	UserRoles    []models.UserAssetSetRole    `json:"user_roles"`
	GroupRoles   []models.GroupAssetSetRole   `json:"group_roles"`
	EveryoneRole *models.AssetSetEveryoneRole `json:"everyone_role"`
}

type AssetRoleAssignment struct {
	UserID, GroupID *int
	RoleID          int
}

type AssetApplicationService struct {
	db               database.Database
	repo             *repository.AssetRepository
	permissions      *PermissionService
	assetPermissions *AssetPermissionService
	assets           *AssetService
	links            *ItemLinkService
	attachmentPath   string
}

func NewAssetApplicationService(db database.Database, permissions *PermissionService, assets *AssetService, assetPermissions *AssetPermissionService) *AssetApplicationService {
	repo := repository.NewAssetRepository(db)
	if assets == nil {
		assets = NewAssetService(db, repo)
	}
	if assetPermissions == nil {
		assetPermissions = NewAssetPermissionService(repo, permissions)
	}
	return &AssetApplicationService{db: db, repo: repo, permissions: permissions, assetPermissions: assetPermissions, assets: assets}
}

func (s *AssetApplicationService) WithLinks(links *ItemLinkService) *AssetApplicationService {
	s.links = links
	return s
}

func (s *AssetApplicationService) require(userID, setID int, permission string) error {
	allowed, err := s.assetPermissions.HasAssetSetPermission(userID, setID, permission)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrAssetForbidden
	}
	return nil
}

func (s *AssetApplicationService) ListSets(userID int) ([]models.AssetManagementSet, error) {
	isAdmin, err := s.permissions.HasGlobalPermission(userID, "system.admin")
	if err != nil {
		return nil, err
	}
	sets, err := s.repo.ListSetsForUser(userID, isAdmin)
	if err != nil {
		return nil, err
	}
	for i := range sets {
		if isAdmin {
			sets[i].UserPermission = "Administrator"
			continue
		}
		role, roleErr := s.assetPermissions.GetUserSetRole(userID, sets[i].ID)
		if roleErr == nil && role != nil {
			sets[i].UserPermission = role.Name
		}
	}
	return sets, nil
}

func (s *AssetApplicationService) HasAccessibleAssetSets(userID int) (bool, error) {
	sets, err := s.ListSets(userID)
	return len(sets) > 0, err
}

func (s *AssetApplicationService) GetSet(userID, id int) (*models.AssetManagementSet, error) {
	if err := s.require(userID, id, AssetPermissionKeyView); err != nil {
		return nil, err
	}
	set, err := s.repo.GetSetByID(id)
	if err != nil {
		return nil, err
	}
	role, roleErr := s.assetPermissions.GetUserSetRole(userID, id)
	if roleErr == nil && role != nil {
		set.UserPermission = role.Name
	}
	return set, nil
}

func (s *AssetApplicationService) CreateSet(userID int, actor AuditActor, input models.AssetManagementSet) (*models.AssetManagementSet, error) {
	admin, err := s.permissions.HasGlobalPermission(userID, "system.admin")
	if err != nil {
		return nil, err
	}
	manage, err := s.permissions.HasGlobalPermission(userID, "asset.manage")
	if err != nil {
		return nil, err
	}
	if !admin && !manage {
		return nil, ErrAssetForbidden
	}
	input.Name = sanitize.PlainTextField.Sanitize(input.Name)
	input.Description = sanitize.RichText.Sanitize(input.Description)
	if strings.TrimSpace(input.Name) == "" {
		return nil, fmt.Errorf("name is required")
	}
	input.CreatedBy = &userID
	id, err := s.repo.CreateSetAndInitialize(&input, userID)
	if err != nil {
		return nil, err
	}
	result, err := s.repo.GetSetByID(id)
	if err != nil {
		return nil, err
	}
	result.UserPermission = "Administrator"
	emitServiceAudit(s.db, actor, logger.ActionAssetSetCreate, logger.ResourceAssetSet, &id, result.Name, nil)
	return result, nil
}

func (s *AssetApplicationService) UpdateSet(userID, id int, actor AuditActor, patch AssetSetPatch) (*models.AssetManagementSet, error) {
	if err := s.require(userID, id, AssetPermissionKeyAdmin); err != nil {
		return nil, err
	}
	current, err := s.repo.GetAssetSetCoreByID(id)
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
	current.Name = sanitize.PlainTextField.Sanitize(current.Name)
	current.Description = sanitize.RichText.Sanitize(current.Description)
	if strings.TrimSpace(current.Name) == "" {
		return nil, fmt.Errorf("name is required")
	}
	if err := s.repo.UpdateSetAndPromotion(current); err != nil {
		return nil, err
	}
	result, err := s.repo.GetSetByID(id)
	if err != nil {
		return nil, err
	}
	result.UserPermission = "Administrator"
	emitServiceAudit(s.db, actor, logger.ActionAssetSetUpdate, logger.ResourceAssetSet, &id, result.Name, nil)
	return result, nil
}

func (s *AssetApplicationService) DeleteSet(userID, id int, actor AuditActor) error {
	admin, err := s.permissions.HasGlobalPermission(userID, "system.admin")
	if err != nil {
		return err
	}
	if !admin {
		return ErrAssetForbidden
	}
	set, err := s.repo.GetAssetSetCoreByID(id)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteSet(id); err != nil {
		return err
	}
	emitServiceAudit(s.db, actor, logger.ActionAssetSetDelete, logger.ResourceAssetSet, &id, set.Name, nil)
	return nil
}

func (s *AssetApplicationService) ListRoles(userID int) ([]models.AssetRole, error) {
	roles, err := s.repo.ListAllRoles()
	if err != nil {
		return nil, err
	}
	visible, err := s.ListSets(userID)
	if err != nil || len(visible) == 0 {
		return nil, err
	}
	return roles, nil
}

func (s *AssetApplicationService) GetRole(userID, id int) (*models.AssetRole, error) {
	if _, err := s.ListRoles(userID); err != nil {
		return nil, err
	}
	role, err := s.repo.GetRoleByID(id)
	if err != nil {
		return nil, err
	}
	role.Permissions, err = s.repo.GetRolePermissions(id)
	return role, err
}

func (s *AssetApplicationService) SetRoles(userID, setID int) (AssetSetRoles, error) {
	if err := s.require(userID, setID, AssetPermissionKeyAdmin); err != nil {
		return AssetSetRoles{}, err
	}
	users, err := s.repo.FindSetUserRolesByGrantDate(setID)
	if err != nil {
		return AssetSetRoles{}, err
	}
	groups, err := s.repo.FindSetGroupRolesByGrantDate(setID)
	if err != nil {
		return AssetSetRoles{}, err
	}
	everyone, err := s.repo.GetEveryoneRoleDetailed(setID)
	return AssetSetRoles{UserRoles: users, GroupRoles: groups, EveryoneRole: everyone}, err
}

func (s *AssetApplicationService) AssignRole(userID, setID int, actor AuditActor, input AssetRoleAssignment) error {
	if err := s.require(userID, setID, AssetPermissionKeyAdmin); err != nil {
		return err
	}
	exists, err := s.repo.AssetRoleExists(input.RoleID)
	if err != nil {
		return err
	}
	if !exists {
		return repository.ErrNotFound
	}
	if (input.UserID == nil) == (input.GroupID == nil) {
		return fmt.Errorf("exactly one of user_id or group_id is required")
	}
	kind, principalID := "user", 0
	if input.UserID != nil {
		principalID = *input.UserID
	} else {
		kind, principalID = "group", *input.GroupID
	}
	if err := s.ensureAssignmentPreservesAdmin(setID, input.RoleID, kind, principalID); err != nil {
		return err
	}
	if kind == "user" {
		err = s.repo.AssignUserRole(setID, principalID, input.RoleID, userID)
	} else {
		err = s.repo.AssignGroupRole(setID, principalID, input.RoleID, userID)
	}
	if err == nil {
		emitServiceAudit(s.db, actor, logger.ActionAssetSetRoleAssign, logger.ResourceAssetSetRole, &setID, "", map[string]any{"role_id": input.RoleID})
	}
	return err
}

func (s *AssetApplicationService) RevokeRole(userID, setID, assignmentID int, kind string, actor AuditActor) error {
	if err := s.require(userID, setID, AssetPermissionKeyAdmin); err != nil {
		return err
	}
	if kind != "group" {
		kind = "user"
	}
	if err := s.ensureRevocationPreservesAdmin(setID, assignmentID, kind); err != nil {
		return err
	}
	var err error
	if kind == "group" {
		err = s.repo.DeleteGroupRoleAssignment(assignmentID, setID)
	} else {
		err = s.repo.DeleteUserRoleAssignment(assignmentID, setID)
	}
	if err == nil {
		emitServiceAudit(s.db, actor, logger.ActionAssetSetRoleRevoke, logger.ResourceAssetSetRole, &setID, "", map[string]any{"role_assignment_id": assignmentID})
	}
	return err
}

func (s *AssetApplicationService) EveryoneRole(userID, setID int) (*models.AssetSetEveryoneRole, error) {
	if err := s.require(userID, setID, AssetPermissionKeyAdmin); err != nil {
		return nil, err
	}
	return s.repo.GetEveryoneRoleDetailed(setID)
}

func (s *AssetApplicationService) SetEveryoneRole(userID, setID int, roleID *int, actor AuditActor) (*models.AssetSetEveryoneRole, error) {
	if err := s.require(userID, setID, AssetPermissionKeyAdmin); err != nil {
		return nil, err
	}
	if roleID != nil {
		exists, err := s.repo.AssetRoleExists(*roleID)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, repository.ErrNotFound
		}
	}
	if err := s.ensureEveryonePreservesAdmin(setID, roleID); err != nil {
		return nil, err
	}
	if err := s.repo.SetEveryoneRole(setID, roleID, userID); err != nil {
		return nil, err
	}
	action := logger.ActionAssetSetRoleRevoke
	if roleID != nil {
		action = logger.ActionAssetSetRoleAssign
	}
	emitServiceAudit(s.db, actor, action, logger.ResourceAssetSetRole, &setID, "", nil)
	return s.repo.GetEveryoneRoleDetailed(setID)
}

func (s *AssetApplicationService) ensureAssignmentPreservesAdmin(setID, newRoleID int, kind string, principalID int) error {
	adminID, err := s.repo.GetAssetRoleIDByName("Administrator")
	if errors.Is(err, repository.ErrNotFound) {
		return nil
	}
	if err != nil || newRoleID == adminID {
		return err
	}
	current, err := s.repo.GetPrincipalDirectRoleID(setID, kind, principalID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil
	}
	if err != nil || current != adminID {
		return err
	}
	everyone, err := s.repo.GetEveryoneRoleIDValueForSet(setID)
	if err != nil {
		return err
	}
	if everyone.Valid && int(everyone.Int64) == adminID {
		return nil
	}
	remaining, err := s.repo.CountAdminAssignmentsExcludingPrincipal(setID, adminID, kind, principalID)
	if err != nil {
		return err
	}
	if remaining == 0 {
		return ErrAssetConflict
	}
	return nil
}

func (s *AssetApplicationService) ensureRevocationPreservesAdmin(setID, assignmentID int, kind string) error {
	adminID, err := s.repo.GetAssetRoleIDByName("Administrator")
	if errors.Is(err, repository.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	roleID, err := s.repo.GetAssignmentRoleID(setID, assignmentID, kind)
	if errors.Is(err, repository.ErrNotFound) {
		return nil
	}
	if err != nil || roleID != adminID {
		return err
	}
	everyone, err := s.repo.GetEveryoneRoleIDValueForSet(setID)
	if err != nil {
		return err
	}
	if everyone.Valid && int(everyone.Int64) == adminID {
		return nil
	}
	remaining, err := s.repo.CountAdminAssignmentsExcluding(setID, adminID, assignmentID, kind)
	if err != nil {
		return err
	}
	if remaining == 0 {
		return ErrAssetConflict
	}
	return nil
}

func (s *AssetApplicationService) ensureEveryonePreservesAdmin(setID int, newRoleID *int) error {
	adminID, err := s.repo.GetAssetRoleIDByName("Administrator")
	if errors.Is(err, repository.ErrNotFound) {
		return nil
	}
	if err != nil || newRoleID != nil && *newRoleID == adminID {
		return err
	}
	current, err := s.repo.GetEveryoneRoleIDValueForSet(setID)
	if err != nil {
		return err
	}
	if !current.Valid || int(current.Int64) != adminID {
		return nil
	}
	remaining, err := s.repo.CountAdminAssignments(setID, adminID)
	if err != nil {
		return err
	}
	if remaining == 0 {
		return ErrAssetConflict
	}
	return nil
}

func (s *AssetApplicationService) ListTypes(userID, setID int) ([]models.AssetType, error) {
	if err := s.require(userID, setID, AssetPermissionKeyView); err != nil {
		return nil, err
	}
	return s.repo.FindAssetTypesForSet(setID)
}

func (s *AssetApplicationService) GetType(userID, id int) (*models.AssetType, error) {
	setID, err := s.repo.GetAssetTypeSetID(id)
	if err != nil {
		return nil, err
	}
	if err := s.require(userID, setID, AssetPermissionKeyView); err != nil {
		return nil, err
	}
	result, err := s.repo.FindAssetTypeByID(id)
	if err != nil {
		return nil, err
	}
	result.Fields, err = s.repo.FindAssetTypeFields(id)
	return result, err
}

func (s *AssetApplicationService) CreateType(userID, setID int, actor AuditActor, input models.AssetType) (*models.AssetType, error) {
	if err := s.require(userID, setID, AssetPermissionKeyAdmin); err != nil {
		return nil, err
	}
	input.SetID = setID
	sanitizeAssetType(&input)
	if input.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if input.Icon == "" {
		input.Icon = "Box"
	}
	if input.Color == "" {
		input.Color = "#6b7280"
	}
	if input.CreatedAt.IsZero() {
		input.CreatedAt = time.Now()
		input.UpdatedAt = input.CreatedAt
	}
	id, err := s.repo.CreateAssetType(&input)
	if err != nil {
		return nil, err
	}
	input.ID = id
	emitServiceAudit(s.db, actor, logger.ActionAssetTypeCreate, logger.ResourceAssetType, &id, input.Name, nil)
	return &input, nil
}

func (s *AssetApplicationService) UpdateType(userID, id int, actor AuditActor, patch AssetTypePatch) (*models.AssetType, error) {
	current, err := s.GetType(userID, id)
	if err != nil {
		return nil, err
	}
	if err := s.require(userID, current.SetID, AssetPermissionKeyAdmin); err != nil {
		return nil, err
	}
	if patch.Name != nil {
		current.Name = *patch.Name
	}
	if patch.Description != nil {
		current.Description = *patch.Description
	}
	if patch.Icon != nil {
		current.Icon = *patch.Icon
	}
	if patch.Color != nil {
		current.Color = *patch.Color
	}
	sanitizeAssetType(current)
	if current.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if err := s.repo.UpdateAssetType(id, repository.AssetTypeUpdate{Name: current.Name, Description: current.Description, Icon: current.Icon, Color: current.Color, DisplayOrder: patch.DisplayOrder, IsActive: patch.IsActive}); err != nil {
		return nil, err
	}
	result, err := s.repo.FindAssetTypeByID(id)
	if err != nil {
		return nil, err
	}
	emitServiceAudit(s.db, actor, logger.ActionAssetTypeUpdate, logger.ResourceAssetType, &id, result.Name, nil)
	return result, nil
}

func (s *AssetApplicationService) DeleteType(userID, id int, actor AuditActor) error {
	setID, count, err := s.repo.GetAssetTypeSetAndCount(id)
	if err != nil {
		return err
	}
	if err := s.require(userID, setID, AssetPermissionKeyAdmin); err != nil {
		return err
	}
	if count > 0 {
		return ErrAssetConflict
	}
	if err := s.repo.DeleteAssetType(id); err != nil {
		return err
	}
	emitServiceAudit(s.db, actor, logger.ActionAssetTypeDelete, logger.ResourceAssetType, &id, "", nil)
	return nil
}

func (s *AssetApplicationService) TypeFields(userID, id int) ([]models.AssetTypeField, error) {
	setID, err := s.repo.GetAssetTypeSetID(id)
	if err != nil {
		return nil, err
	}
	if err := s.require(userID, setID, AssetPermissionKeyView); err != nil {
		return nil, err
	}
	return s.repo.FindAssetTypeFields(id)
}

func (s *AssetApplicationService) ReplaceTypeFields(userID, id int, fields []repository.AssetTypeFieldAssignment) ([]models.AssetTypeField, error) {
	setID, err := s.repo.GetAssetTypeSetID(id)
	if err != nil {
		return nil, err
	}
	if err := s.require(userID, setID, AssetPermissionKeyAdmin); err != nil {
		return nil, err
	}
	if err := s.repo.ReplaceAssetTypeFields(id, fields); err != nil {
		return nil, err
	}
	return s.repo.FindAssetTypeFields(id)
}

func sanitizeAssetType(input *models.AssetType) {
	input.Name = sanitize.PlainTextField.Sanitize(input.Name)
	input.Description = sanitize.RichText.Sanitize(input.Description)
	input.Icon = sanitize.ShortIdentifier.Sanitize(input.Icon)
	input.Color = sanitize.ShortIdentifier.Sanitize(input.Color)
}

func (s *AssetApplicationService) ListCategories(userID, setID int, tree bool) ([]models.AssetCategory, error) {
	if err := s.require(userID, setID, AssetPermissionKeyView); err != nil {
		return nil, err
	}
	items, err := s.repo.FindAssetCategoriesForSet(setID)
	if err != nil || !tree {
		return items, err
	}
	return assetCategoryTree(items), nil
}

func (s *AssetApplicationService) GetCategory(userID, id int) (*models.AssetCategory, error) {
	setID, err := s.repo.GetAssetCategorySetID(id)
	if err != nil {
		return nil, err
	}
	if err := s.require(userID, setID, AssetPermissionKeyView); err != nil {
		return nil, err
	}
	return s.repo.FindAssetCategoryByID(id)
}

func (s *AssetApplicationService) CreateCategory(userID, setID int, actor AuditActor, input models.AssetCategory) (*models.AssetCategory, error) {
	if err := s.require(userID, setID, AssetPermissionKeyEdit); err != nil {
		return nil, err
	}
	input.Name = sanitize.PlainTextField.Sanitize(input.Name)
	input.Description = sanitize.RichText.Sanitize(input.Description)
	if input.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if input.ParentID != nil {
		parentSet, err := s.repo.GetAssetCategorySetID(*input.ParentID)
		if err != nil || parentSet != setID {
			return nil, fmt.Errorf("parent category must belong to same set")
		}
	}
	id, createdAt, err := s.repo.CreateAssetCategory(repository.CreateAssetCategoryInput{SetID: setID, Name: input.Name, Description: input.Description, ParentID: input.ParentID})
	if err != nil {
		return nil, err
	}
	input.ID = id
	input.SetID = setID
	input.CreatedAt = createdAt
	input.UpdatedAt = createdAt
	emitServiceAudit(s.db, actor, logger.ActionAssetCategoryCreate, logger.ResourceAssetCategory, &id, input.Name, nil)
	return &input, nil
}

func (s *AssetApplicationService) UpdateCategory(userID, id int, actor AuditActor, patch AssetCategoryPatch) (*models.AssetCategory, error) {
	current, err := s.GetCategory(userID, id)
	if err != nil {
		return nil, err
	}
	if err := s.require(userID, current.SetID, AssetPermissionKeyEdit); err != nil {
		return nil, err
	}
	if patch.Name != nil {
		current.Name = *patch.Name
	}
	if patch.Description != nil {
		current.Description = *patch.Description
	}
	current.Name = sanitize.PlainTextField.Sanitize(current.Name)
	current.Description = sanitize.RichText.Sanitize(current.Description)
	if current.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if err := s.repo.UpdateAssetCategoryNameDescription(id, current.Name, current.Description); err != nil {
		return nil, err
	}
	result, err := s.repo.GetAssetCategoryCoreByID(id)
	if err != nil {
		return nil, err
	}
	emitServiceAudit(s.db, actor, logger.ActionAssetCategoryUpdate, logger.ResourceAssetCategory, &id, result.Name, nil)
	return result, nil
}

func (s *AssetApplicationService) DeleteCategory(userID, id int, actor AuditActor) error {
	setID, children, parentID, count, err := s.repo.GetAssetCategoryDeletionInfo(id)
	if err != nil {
		return err
	}
	if err := s.require(userID, setID, AssetPermissionKeyEdit); err != nil {
		return err
	}
	if children || count > 0 {
		return ErrAssetConflict
	}
	if err := s.repo.DeleteAssetCategory(id, parentID); err != nil {
		return err
	}
	emitServiceAudit(s.db, actor, logger.ActionAssetCategoryDelete, logger.ResourceAssetCategory, &id, "", nil)
	return nil
}

func (s *AssetApplicationService) MoveCategory(userID, id int, parentID *int) (*models.AssetCategory, error) {
	setID, err := s.repo.GetAssetCategorySetID(id)
	if err != nil {
		return nil, err
	}
	if err := s.require(userID, setID, AssetPermissionKeyEdit); err != nil {
		return nil, err
	}
	oldParent, err := s.repo.GetAssetCategoryParentID(id)
	if err != nil {
		return nil, err
	}
	if parentID != nil {
		if *parentID == id {
			return nil, fmt.Errorf("category cannot be its own parent")
		}
		parentSet, err := s.repo.GetAssetCategorySetID(*parentID)
		if err != nil || parentSet != setID {
			return nil, fmt.Errorf("parent category must belong to same set")
		}
		descendant, err := s.repo.IsAssetCategoryDescendantOf(*parentID, id)
		if err != nil {
			return nil, err
		}
		if descendant {
			return nil, fmt.Errorf("category cannot move below its descendant")
		}
	}
	if err := s.repo.MoveAssetCategory(id, oldParent, parentID); err != nil {
		return nil, err
	}
	return s.repo.GetAssetCategoryCoreByID(id)
}

func assetCategoryTree(items []models.AssetCategory) []models.AssetCategory {
	byID := make(map[int]*models.AssetCategory, len(items))
	children := make(map[int][]int)
	for i := range items {
		items[i].Children = []models.AssetCategory{}
		byID[items[i].ID] = &items[i]
		if items[i].ParentID != nil {
			children[*items[i].ParentID] = append(children[*items[i].ParentID], items[i].ID)
		}
	}
	var build func(int) models.AssetCategory
	build = func(id int) models.AssetCategory {
		item := *byID[id]
		item.Children = []models.AssetCategory{}
		for _, child := range children[id] {
			item.Children = append(item.Children, build(child))
		}
		return item
	}
	result := []models.AssetCategory{}
	for i := range items {
		if items[i].ParentID == nil {
			result = append(result, build(items[i].ID))
		}
	}
	return result
}

func (s *AssetApplicationService) ListStatuses(userID, setID int) ([]models.AssetStatus, error) {
	if err := s.require(userID, setID, AssetPermissionKeyView); err != nil {
		return nil, err
	}
	return s.repo.FindAssetStatusesForSet(setID)
}
func (s *AssetApplicationService) GetStatus(userID, id int) (*models.AssetStatus, error) {
	setID, err := s.repo.GetAssetStatusSetID(id)
	if err != nil {
		return nil, err
	}
	if err := s.require(userID, setID, AssetPermissionKeyView); err != nil {
		return nil, err
	}
	return s.repo.FindAssetStatusByID(id)
}
func (s *AssetApplicationService) CreateStatus(userID, setID int, actor AuditActor, input models.AssetStatus) (*models.AssetStatus, error) {
	if err := s.require(userID, setID, AssetPermissionKeyAdmin); err != nil {
		return nil, err
	}
	input.SetID = setID
	sanitizeAssetStatus(&input)
	if input.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if input.Color == "" {
		input.Color = "#6b7280"
	}
	input.CreatedAt = time.Now()
	input.UpdatedAt = input.CreatedAt
	id, err := s.repo.CreateAssetStatusTransactional(&input)
	if err != nil {
		return nil, err
	}
	input.ID = id
	emitServiceAudit(s.db, actor, logger.ActionAssetStatusCreate, logger.ResourceAssetStatus, &id, input.Name, nil)
	return &input, nil
}
func (s *AssetApplicationService) UpdateStatus(userID, id int, actor AuditActor, patch AssetStatusPatch) (*models.AssetStatus, error) {
	current, err := s.GetStatus(userID, id)
	if err != nil {
		return nil, err
	}
	if err := s.require(userID, current.SetID, AssetPermissionKeyAdmin); err != nil {
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
	sanitizeAssetStatus(current)
	if current.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	display := current.DisplayOrder
	if patch.DisplayOrder != nil {
		display = *patch.DisplayOrder
	}
	if err := s.repo.UpdateAssetStatusTransactional(id, repository.AssetStatusUpdate{Name: current.Name, Color: current.Color, Description: current.Description, DisplayOrder: display, IsDefault: patch.IsDefault}, current.SetID); err != nil {
		return nil, err
	}
	result, err := s.repo.FindAssetStatusByID(id)
	if err != nil {
		return nil, err
	}
	emitServiceAudit(s.db, actor, logger.ActionAssetStatusUpdate, logger.ResourceAssetStatus, &id, result.Name, nil)
	return result, nil
}
func (s *AssetApplicationService) DeleteStatus(userID, id int, actor AuditActor) error {
	current, err := s.GetStatus(userID, id)
	if err != nil {
		return err
	}
	if err := s.require(userID, current.SetID, AssetPermissionKeyAdmin); err != nil {
		return err
	}
	count, err := s.repo.CountAssetsUsingStatus(id)
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrAssetConflict
	}
	if err := s.repo.DeleteAssetStatus(id); err != nil {
		return err
	}
	emitServiceAudit(s.db, actor, logger.ActionAssetStatusDelete, logger.ResourceAssetStatus, &id, current.Name, nil)
	return nil
}
func sanitizeAssetStatus(input *models.AssetStatus) {
	input.Name = sanitize.PlainTextField.Sanitize(input.Name)
	input.Color = sanitize.ShortIdentifier.Sanitize(input.Color)
	input.Description = sanitize.RichText.Sanitize(input.Description)
}

func (s *AssetApplicationService) ListAssets(userID, setID int, filter repository.AssetListFilter, ql string) ([]models.Asset, int, error) {
	if err := s.require(userID, setID, AssetPermissionKeyView); err != nil {
		return nil, 0, err
	}
	filter.SetID = setID
	if ql != "" {
		setMap, err := s.repo.GetCQLSetMap()
		if err != nil {
			return nil, 0, err
		}
		workspaceMap, err := repository.NewWorkspaceRepository(s.db).ListNameKeyToIDMap()
		if err != nil {
			return nil, 0, err
		}
		assetFields, err := s.repo.GetCQLCustomFieldMap(setID)
		if err != nil {
			return nil, 0, err
		}
		itemFields, err := repository.NewItemRepository(s.db).GetCQLCustomFieldMap()
		if err != nil {
			return nil, 0, err
		}
		evaluator := cql.NewAssetEvaluator(setMap, workspaceMap, assetFields, itemFields, s.db.GetDriverName())
		resolved := cql.SubstituteFunctions(ql, cql.UserContext(userID))
		filter.CQLSQL, filter.CQLArgs, err = evaluator.EvaluateToSQL(resolved)
		if err != nil {
			return nil, 0, fmt.Errorf("CQL query error: %w", err)
		}
	}
	total, err := s.repo.CountAssets(filter)
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.repo.ListAssets(filter)
	if err != nil {
		return nil, 0, err
	}
	result := make([]models.Asset, len(rows))
	for i := range rows {
		result[i] = repository.AssetRowToModel(rows[i])
	}
	return result, total, nil
}
func (s *AssetApplicationService) GetAsset(userID, id int) (*models.Asset, error) {
	setID, err := s.repo.GetAssetSetID(id)
	if err != nil {
		return nil, err
	}
	if err := s.require(userID, setID, AssetPermissionKeyView); err != nil {
		return nil, err
	}
	row, err := s.repo.FindAssetFullByID(id)
	if err != nil {
		return nil, err
	}
	result := repository.AssetRowToModel(*row)
	return &result, nil
}
func (s *AssetApplicationService) CreateAsset(userID, setID int, actor AuditActor, input AssetMutationInput) (*models.Asset, error) {
	if err := s.require(userID, setID, AssetPermissionKeyCreate); err != nil {
		return nil, err
	}
	if input.StatusID == nil {
		input.StatusID, _ = s.repo.GetDefaultStatus(setID)
	}
	if err := s.validateAssetTaxonomy(setID, input); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(input.CustomFieldValues)
	if err != nil {
		return nil, err
	}
	encodedString := string(encoded)
	return s.assets.CreateAsset(actor, repository.CreateAssetInput{SetID: setID, AssetTypeID: input.AssetTypeID, CategoryID: input.CategoryID, StatusID: input.StatusID, Title: input.Title, Description: input.Description, AssetTag: input.AssetTag, CustomFieldValuesJSON: &encodedString, CreatedBy: userID, CreatedAt: time.Now()}, input.CustomFieldValues)
}
func (s *AssetApplicationService) UpdateAsset(userID, id int, actor AuditActor, patch AssetPatchInput) (*models.Asset, error) {
	snapshot, err := s.repo.GetAssetUpdateSnapshot(id)
	if err != nil {
		return nil, err
	}
	if err := s.require(userID, snapshot.SetID, AssetPermissionKeyEdit); err != nil {
		return nil, err
	}
	row, err := s.repo.FindAssetFullByID(id)
	if err != nil {
		return nil, err
	}
	current := repository.AssetRowToModel(*row)
	input := AssetMutationInput{AssetTypeID: current.AssetTypeID, CategoryID: current.CategoryID, StatusID: current.StatusID, Title: current.Title, Description: current.Description, AssetTag: current.AssetTag, CustomFieldValues: current.CustomFieldValues}
	if patch.AssetTypeID != nil {
		input.AssetTypeID = *patch.AssetTypeID
	}
	if patch.CategoryIDSet {
		input.CategoryID = patch.CategoryID
	}
	if patch.StatusIDSet {
		input.StatusID = patch.StatusID
	}
	if patch.Title != nil {
		input.Title = *patch.Title
	}
	if patch.Description != nil {
		input.Description = *patch.Description
	}
	if patch.AssetTag != nil {
		input.AssetTag = *patch.AssetTag
	}
	if patch.CustomFieldValues != nil {
		input.CustomFieldValues = *patch.CustomFieldValues
	}
	if err := s.validateAssetTaxonomy(snapshot.SetID, input); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(input.CustomFieldValues)
	if err != nil {
		return nil, err
	}
	encodedString := string(encoded)
	return s.assets.UpdateAsset(actor, id, *snapshot, repository.UpdateAssetInput{AssetTypeID: input.AssetTypeID, CategoryID: input.CategoryID, StatusID: input.StatusID, Title: input.Title, Description: input.Description, AssetTag: input.AssetTag, CustomFieldValuesJSON: &encodedString}, input.CustomFieldValues)
}
func (s *AssetApplicationService) DeleteAsset(userID, id int, actor AuditActor) error {
	setID, err := s.repo.GetAssetSetID(id)
	if err != nil {
		return err
	}
	if err := s.require(userID, setID, AssetPermissionKeyDelete); err != nil {
		return err
	}
	return s.assets.DeleteAsset(actor, id)
}
func (s *AssetApplicationService) AssetSummaries(userID int, ids []int) ([]models.AssetSummary, error) {
	if len(ids) == 0 {
		return []models.AssetSummary{}, nil
	}
	if len(ids) > 500 {
		return nil, fmt.Errorf("ids must contain at most 500 values")
	}
	items, err := s.repo.FindAssetSummariesByIDs(ids)
	if err != nil {
		return nil, err
	}
	visible := make(map[int]models.AssetSummary, len(items))
	access := map[int]bool{}
	for _, item := range items {
		allowed, ok := access[item.SetID]
		if !ok {
			allowed = s.require(userID, item.SetID, AssetPermissionKeyView) == nil
			access[item.SetID] = allowed
		}
		if allowed {
			visible[item.ID] = item
		}
	}
	result := make([]models.AssetSummary, 0, len(visible))
	for _, id := range ids {
		if item, ok := visible[id]; ok {
			result = append(result, item)
		}
	}
	return result, nil
}

func (s *AssetApplicationService) AssetLinks(userID, assetID int) (EntityLinks, error) {
	if s.links == nil {
		return EntityLinks{}, errors.New("asset links are unavailable")
	}
	setID, err := s.repo.GetAssetSetID(assetID)
	if err != nil {
		return EntityLinks{}, err
	}
	if err := s.require(userID, setID, AssetPermissionKeyView); err != nil {
		return EntityLinks{}, err
	}
	outgoing, incoming, err := s.links.ListLinksForEntityWithChecks(userID, "asset", assetID)
	return EntityLinks{Outgoing: outgoing, Incoming: incoming}, err
}

func (s *AssetApplicationService) CreateAssetLink(userID, assetID int, input CreateItemLinkParams) (*models.ItemLink, error) {
	if s.links == nil {
		return nil, errors.New("asset links are unavailable")
	}
	setID, err := s.repo.GetAssetSetID(assetID)
	if err != nil {
		return nil, err
	}
	if err := s.require(userID, setID, AssetPermissionKeyEdit); err != nil {
		return nil, err
	}
	input.SourceType, input.SourceID = "asset", assetID
	return s.links.CreateManagedLink(userID, input)
}
func (s *AssetApplicationService) LinkedToItem(userID, itemID int) ([]models.LinkedAsset, error) {
	workspaceID, err := repository.NewItemRepository(s.db).GetWorkspaceID(itemID)
	if err != nil {
		return nil, err
	}
	allowed, err := s.permissions.HasWorkspacePermission(userID, workspaceID, models.PermissionItemView)
	if err != nil || !allowed {
		return nil, ErrAssetForbidden
	}
	items, err := s.repo.ListLinkedToItem(itemID)
	if err != nil {
		return nil, err
	}
	result := items[:0]
	for _, item := range items {
		if s.require(userID, item.SetID, AssetPermissionKeyView) == nil {
			result = append(result, item)
		}
	}
	return result, nil
}
func (s *AssetApplicationService) validateAssetTaxonomy(setID int, input AssetMutationInput) error {
	if input.AssetTypeID <= 0 {
		return fmt.Errorf("asset_type_id is required")
	}
	if ok, err := s.repo.AssetTypeBelongsToSet(input.AssetTypeID, setID); err != nil || !ok {
		return fmt.Errorf("asset type must belong to set")
	}
	if input.CategoryID != nil {
		if ok, err := s.repo.CategoryBelongsToSet(*input.CategoryID, setID); err != nil || !ok {
			return fmt.Errorf("category must belong to set")
		}
	}
	if input.StatusID != nil {
		if ok, err := s.repo.StatusBelongsToSet(*input.StatusID, setID); err != nil || !ok {
			return fmt.Errorf("status must belong to set")
		}
	}
	return nil
}
