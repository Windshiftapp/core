package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
)

var (
	ErrWorkspaceMutationForbidden = errors.New("workspace mutation forbidden")
	ErrWorkspaceMutationInvalid   = errors.New("workspace mutation invalid")
)

var workspaceKeyPattern = regexp.MustCompile(`^[A-Za-z0-9]+$`)

type WorkspaceMutationAccess interface {
	WorkspaceSourceAccess
	CanAdminWorkspace(int, int) (bool, error)
	HasGlobalPermission(int, string) (bool, error)
}

// WorkspaceApplicationService owns authorization and side effects for workspace mutations.
type WorkspaceApplicationService struct {
	db          database.Database
	workspaces  *WorkspaceService
	repository  *repository.WorkspaceRepository
	access      WorkspaceMutationAccess
	invalidator *AuthorizationCacheInvalidator
}

func NewWorkspaceApplicationService(db database.Database, access WorkspaceMutationAccess, invalidator *AuthorizationCacheInvalidator) *WorkspaceApplicationService {
	return &WorkspaceApplicationService{
		db: db, workspaces: NewWorkspaceServiceWithAccess(db, access),
		repository: repository.NewWorkspaceRepository(db), access: access, invalidator: invalidator,
	}
}

func (s *WorkspaceApplicationService) Create(ctx context.Context, actor AuditActor, params CreateWorkspaceParams) (*models.Workspace, error) {
	allowed, err := s.access.HasGlobalPermission(actor.UserID, models.PermissionWorkspaceCreate)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrWorkspaceMutationForbidden
	}
	params.CreatorID = actor.UserID
	params.Name = sanitize.PlainTextField.Sanitize(params.Name)
	params.Key = sanitize.ShortIdentifier.Sanitize(params.Key)
	params.Description = sanitize.RichText.Sanitize(params.Description)
	params.Icon = sanitize.ShortIdentifier.Sanitize(params.Icon)
	params.Color = sanitize.ShortIdentifier.Sanitize(params.Color)
	if err := validateWorkspaceCreate(params); err != nil {
		return nil, err
	}
	result, err := s.workspaces.Create(ctx, params)
	if err != nil {
		return nil, err
	}
	if err := s.invalidator.Apply(AuthorizationInvalidation{ResetPermissions: true, ActiveWorkspacesChanged: true, WorkspaceKeysChanged: true}); err != nil {
		return nil, err
	}
	if err := s.repository.CreateItemSequence(int64(result.Workspace.ID)); err != nil {
		slog.Warn("failed to create workspace item sequence", "workspace_id", result.Workspace.ID, "error", err)
	}
	action := logger.ActionWorkspaceCreate
	if params.TemplateWorkspaceID != nil {
		action = logger.ActionWorkspaceCreateFromTemplate
	}
	emitServiceAudit(s.db, actor, action, logger.ResourceWorkspace, &result.Workspace.ID, result.Workspace.Name, map[string]any{"key": result.Workspace.Key})
	return result.Workspace, nil
}

func (s *WorkspaceApplicationService) Update(actor AuditActor, params UpdateWorkspaceParams) (*models.Workspace, error) {
	allowed, err := s.access.CanAdminWorkspace(actor.UserID, params.ID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, repository.ErrNotFound
	}
	before, err := s.workspaces.GetByID(params.ID)
	if err != nil {
		return nil, err
	}
	sanitizeWorkspaceUpdate(&params)
	if err := validateWorkspaceUpdate(params); err != nil {
		return nil, err
	}
	workspace, err := s.workspaces.Update(params)
	if err != nil {
		return nil, err
	}
	permissionsChanged := before.Active != workspace.Active || before.IsPersonal != workspace.IsPersonal || !workspaceIntPointersEqual(before.OwnerID, workspace.OwnerID)
	if err := s.invalidator.Apply(AuthorizationInvalidation{
		ResetPermissions: permissionsChanged, ActiveWorkspacesChanged: before.Active != workspace.Active,
		WorkspaceKeysChanged: before.Key != workspace.Key,
	}); err != nil {
		return nil, err
	}
	emitServiceAudit(s.db, actor, logger.ActionWorkspaceUpdate, logger.ResourceWorkspace, &workspace.ID, workspace.Name, nil)
	return workspace, nil
}

func (s *WorkspaceApplicationService) Delete(actor AuditActor, workspaceID int) error {
	allowed, err := s.access.CanAdminWorkspace(actor.UserID, workspaceID)
	if err != nil {
		return err
	}
	if !allowed {
		return repository.ErrNotFound
	}
	workspace, err := s.workspaces.GetByID(workspaceID)
	if err != nil {
		return err
	}
	if err := s.workspaces.Delete(workspaceID); err != nil {
		return err
	}
	if err := s.invalidator.Apply(AuthorizationInvalidation{ResetPermissions: true, ActiveWorkspacesChanged: true, WorkspaceKeysChanged: true}); err != nil {
		return err
	}
	emitServiceAudit(s.db, actor, logger.ActionWorkspaceDelete, logger.ResourceWorkspace, &workspaceID, workspace.Name, map[string]any{"key": workspace.Key})
	return nil
}

func validateWorkspaceCreate(params CreateWorkspaceParams) error {
	if params.Name == "" || len(params.Name) > 100 {
		return fmt.Errorf("%w: name must contain 1 to 100 characters", ErrWorkspaceMutationInvalid)
	}
	if len(params.Key) < 2 || len(params.Key) > 10 || !workspaceKeyPattern.MatchString(params.Key) {
		return fmt.Errorf("%w: key must contain 2 to 10 alphanumeric characters", ErrWorkspaceMutationInvalid)
	}
	if len(params.Description) > 500 {
		return fmt.Errorf("%w: description must not exceed 500 characters", ErrWorkspaceMutationInvalid)
	}
	return nil
}

func validateWorkspaceUpdate(params UpdateWorkspaceParams) error {
	if params.Name != nil && (*params.Name == "" || len(*params.Name) > 100) {
		return fmt.Errorf("%w: name must contain 1 to 100 characters", ErrWorkspaceMutationInvalid)
	}
	if params.Key != nil && (len(*params.Key) < 2 || len(*params.Key) > 10 || !workspaceKeyPattern.MatchString(*params.Key)) {
		return fmt.Errorf("%w: key must contain 2 to 10 alphanumeric characters", ErrWorkspaceMutationInvalid)
	}
	if params.Description != nil && len(*params.Description) > 500 {
		return fmt.Errorf("%w: description must not exceed 500 characters", ErrWorkspaceMutationInvalid)
	}
	return nil
}

func sanitizeWorkspaceUpdate(params *UpdateWorkspaceParams) {
	params.Name = sanitizeOptionalWorkspaceField(params.Name, sanitize.PlainTextField.Sanitize)
	params.Key = sanitizeOptionalWorkspaceField(params.Key, func(value string) string { return strings.ToUpper(sanitize.ShortIdentifier.Sanitize(value)) })
	params.Description = sanitizeOptionalWorkspaceField(params.Description, sanitize.RichText.Sanitize)
	params.Icon = sanitizeOptionalWorkspaceField(params.Icon, sanitize.ShortIdentifier.Sanitize)
	params.Color = sanitizeOptionalWorkspaceField(params.Color, sanitize.ShortIdentifier.Sanitize)
}

func sanitizeOptionalWorkspaceField(value *string, policy func(string) string) *string {
	if value == nil {
		return nil
	}
	result := policy(*value)
	return &result
}

func workspaceIntPointersEqual(left, right *int) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}
