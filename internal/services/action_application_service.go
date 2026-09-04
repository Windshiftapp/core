package services

import (
	"context"
	"encoding/json"
	"errors"
	"sort"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
	"windshift/internal/services/actioncatalog"
	"windshift/internal/services/actiontemplates"
)

var ErrActionNotVisible = errors.New("action not visible")
var ErrActionDisabled = errors.New("action is disabled")

type ActionCatalogTrigger struct {
	Type         models.ActionTriggerType `json:"type"`
	Label        string                   `json:"label"`
	Description  string                   `json:"description"`
	ConfigSchema json.RawMessage          `json:"config_schema"`
}

type ActionCatalogNode struct {
	Type         models.ActionNodeType `json:"type"`
	Label        string                `json:"label"`
	Description  string                `json:"description"`
	Category     string                `json:"category"`
	ConfigSchema json.RawMessage       `json:"config_schema"`
	IsIterator   bool                  `json:"is_iterator"`
	Outputs      []string              `json:"outputs"`
}

type ActionCatalogCapability struct {
	ID             int                   `json:"id"`
	Name           string                `json:"name"`
	CapabilityType models.CapabilityType `json:"capability_type"`
}

type ActionCatalog struct {
	Scope        string                    `json:"scope"`
	Triggers     []ActionCatalogTrigger    `json:"triggers"`
	Nodes        []ActionCatalogNode       `json:"nodes"`
	Capabilities []ActionCatalogCapability `json:"capabilities"`
}

type ActionTemplateSummary struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category,omitempty"`
	TriggerType string `json:"trigger_type"`
	NodeCount   int    `json:"node_count"`
}

type ActionApplicationService struct {
	db          database.Database
	repo        *repository.ActionRepository
	items       *repository.ItemRepository
	permissions *PermissionService
	definitions *ActionDefinitionService
	runtime     *ActionService
	templates   *ActionTemplateService
}

func NewActionApplicationService(db database.Database, repo *repository.ActionRepository, items *repository.ItemRepository, permissions *PermissionService, runtime *ActionService, assets *AssetService) *ActionApplicationService {
	definitions := NewActionDefinitionService(repo, runtime)
	definitions.SetAssetService(assets)
	return &ActionApplicationService{db: db, repo: repo, items: items, permissions: permissions, definitions: definitions, runtime: runtime, templates: NewActionTemplateService(db)}
}

func (s *ActionApplicationService) ListTemplates() []ActionTemplateSummary {
	registry := actiontemplates.Registry()
	result := make([]ActionTemplateSummary, 0, len(registry))
	for _, template := range registry {
		result = append(result, ActionTemplateSummary{
			Key: template.Key, Name: template.Name, Description: template.Description,
			Category: template.Category, TriggerType: string(template.TriggerType), NodeCount: len(template.Nodes),
		})
	}
	return result
}

func (s *ActionApplicationService) ApplyTemplate(ctx context.Context, userID, workspaceID int, actor AuditActor, key string) (*ApplyToWorkspaceResult, error) {
	if err := s.requireManage(userID, workspaceID); err != nil {
		return nil, err
	}
	if _, ok := actiontemplates.Get(key); !ok {
		return nil, repository.ErrNotFound
	}
	result, err := s.templates.ApplyToWorkspace(ctx, key, workspaceID, userID)
	if err != nil {
		return nil, err
	}
	if s.runtime != nil {
		s.runtime.InvalidateWorkspaceCache(workspaceID)
	}
	emitServiceAudit(s.db, actor, "automation.create", "automation", &result.ActionID, result.Name, map[string]any{
		"template_key": result.TemplateKey, "context": "from_template",
	})
	return result, nil
}

func (s *ActionApplicationService) Catalog(userID, workspaceID int) (ActionCatalog, error) {
	if err := s.requireManage(userID, workspaceID); err != nil {
		return ActionCatalog{}, err
	}
	result := ActionCatalog{Scope: "workspace", Triggers: []ActionCatalogTrigger{}, Nodes: []ActionCatalogNode{}, Capabilities: []ActionCatalogCapability{}}
	catalog := actioncatalog.Default()
	for _, trigger := range catalog.Triggers() {
		schema, _ := json.Marshal(trigger.ConfigSchema)
		result.Triggers = append(result.Triggers, ActionCatalogTrigger{Type: trigger.Type, Label: trigger.Label, Description: trigger.Description, ConfigSchema: schema})
	}
	for _, node := range catalog.Nodes() {
		schema, _ := json.Marshal(node.ConfigSchema)
		result.Nodes = append(result.Nodes, ActionCatalogNode{Type: node.Type, Label: node.Label, Description: node.Description, Category: node.Category, ConfigSchema: schema, IsIterator: node.IsIterator, Outputs: node.Outputs})
	}
	capabilities, err := s.repo.ListCapabilitiesForWorkspace(workspaceID, "")
	if err != nil {
		return ActionCatalog{}, err
	}
	for _, capability := range capabilities {
		result.Capabilities = append(result.Capabilities, ActionCatalogCapability{ID: capability.ID, Name: capability.Name, CapabilityType: capability.CapabilityType})
	}
	return result, nil
}

func (s *ActionApplicationService) List(userID, workspaceID int) ([]*models.Action, error) {
	if err := s.requireManage(userID, workspaceID); err != nil {
		return nil, err
	}
	result, err := s.repo.ListByWorkspace(workspaceID)
	if result == nil {
		result = []*models.Action{}
	}
	return result, err
}

func (s *ActionApplicationService) ListManualActions(userID, workspaceID int) ([]*models.Action, error) {
	actions, err := s.repo.ListByWorkspace(workspaceID)
	if err != nil {
		return nil, err
	}
	result := make([]*models.Action, 0, len(actions))
	for _, action := range actions {
		if !action.IsEnabled || action.TriggerType != models.ActionTriggerManual {
			continue
		}
		allowed, err := s.canExecute(userID, workspaceID, action)
		if err != nil {
			return nil, err
		}
		if allowed {
			result = append(result, action)
		}
	}
	return result, nil
}

func (s *ActionApplicationService) Get(userID, workspaceID, actionID int) (*models.Action, error) {
	if err := s.requireManage(userID, workspaceID); err != nil {
		return nil, err
	}
	return s.actionInWorkspace(actionID, workspaceID)
}

func (s *ActionApplicationService) Create(userID, workspaceID int, actor AuditActor, input models.CreateActionRequest) (*models.Action, error) {
	if err := s.requireManage(userID, workspaceID); err != nil {
		return nil, err
	}
	if input.ActorUserID != nil {
		if err := s.requireSetActor(userID); err != nil {
			return nil, err
		}
	}
	roles, err := s.validateRoles(input.TriggerType, input.AllowedRoleIDs)
	if err != nil {
		return nil, err
	}
	input.Name = sanitize.PlainTextField.Sanitize(input.Name)
	input.Description = sanitize.RichText.Sanitize(input.Description)
	action := &models.Action{
		WorkspaceID: workspaceID, Name: input.Name, Description: input.Description, IsEnabled: true,
		TriggerType: input.TriggerType, TriggerConfig: input.TriggerConfig, CreatedBy: &userID,
		ActorUserID: input.ActorUserID, AllowedRoleIDs: roles,
	}
	created, validation, err := s.definitions.Create(action, input.Nodes, input.Edges)
	if len(validation) > 0 {
		return nil, &ActionValidationError{Errors: validation}
	}
	if err == nil {
		emitServiceAudit(s.db, actor, "automation.create", "automation", &created.ID, created.Name, nil)
	}
	return created, err
}

type ActionValidationError struct {
	Errors actioncatalog.ValidationErrors
}

func (e *ActionValidationError) Error() string {
	if len(e.Errors) == 0 {
		return "action definition is invalid"
	}
	return e.Errors[0].Message
}

func (s *ActionApplicationService) Update(userID, workspaceID, actionID int, actor AuditActor, input models.UpdateActionRequest) (*models.Action, error) {
	if err := s.requireManage(userID, workspaceID); err != nil {
		return nil, err
	}
	action, err := s.actionInWorkspace(actionID, workspaceID)
	if err != nil {
		return nil, err
	}
	previousActor := action.ActorUserID
	actorChanging := input.ActorUserID.Present && !sameIntPointer(input.ActorUserID.Value, previousActor)
	if actorChanging {
		if err := s.requireSetActor(userID); err != nil {
			return nil, err
		}
	}
	if input.Name != nil {
		*input.Name = sanitize.PlainTextField.Sanitize(*input.Name)
		action.Name = *input.Name
	}
	if input.Description != nil {
		*input.Description = sanitize.RichText.Sanitize(*input.Description)
		action.Description = *input.Description
	}
	if input.TriggerType != nil {
		action.TriggerType = *input.TriggerType
	}
	if input.TriggerConfig != nil {
		action.TriggerConfig = *input.TriggerConfig
	}
	if input.IsEnabled != nil {
		action.IsEnabled = *input.IsEnabled
	}
	if input.AllowedRoleIDs != nil {
		action.AllowedRoleIDs = input.AllowedRoleIDs
	} else if action.TriggerType != models.ActionTriggerManual {
		action.AllowedRoleIDs = []int{}
	}
	action.AllowedRoleIDs, err = s.validateRoles(action.TriggerType, action.AllowedRoleIDs)
	if err != nil {
		return nil, err
	}
	nodes, edges := action.Nodes, action.Edges
	if input.Nodes != nil {
		nodes, edges = input.Nodes, input.Edges
	}
	definitionTouched := input.Name != nil || input.TriggerType != nil || input.TriggerConfig != nil || input.Nodes != nil || (input.IsEnabled != nil && *input.IsEnabled)
	updated, validation, err := s.definitions.Save(action, nodes, edges, input.Nodes != nil, definitionTouched)
	if len(validation) > 0 {
		return nil, &ActionValidationError{Errors: validation}
	}
	if err != nil {
		return nil, err
	}
	if actorChanging {
		if err := s.repo.SetActor(actionID, input.ActorUserID.Value); err != nil {
			return nil, err
		}
		updated, err = s.repo.GetByID(actionID)
		if err != nil {
			return nil, err
		}
	}
	emitServiceAudit(s.db, actor, "automation.update", "automation", &updated.ID, updated.Name, nil)
	return updated, nil
}

func (s *ActionApplicationService) Delete(userID, workspaceID, actionID int, actor AuditActor) error {
	if err := s.requireManage(userID, workspaceID); err != nil {
		return err
	}
	action, err := s.actionInWorkspace(actionID, workspaceID)
	if err != nil {
		return err
	}
	if err := s.definitions.Delete(action.ID, workspaceID); err != nil {
		return err
	}
	emitServiceAudit(s.db, actor, "automation.delete", "automation", &action.ID, action.Name, nil)
	return nil
}

func (s *ActionApplicationService) Logs(userID, workspaceID, actionID, limit, offset int) ([]*models.ActionExecutionLog, int, error) {
	if _, err := s.Get(userID, workspaceID, actionID); err != nil {
		return nil, 0, err
	}
	result, err := s.repo.GetExecutionLogsByActionID(actionID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	var total int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM action_execution_logs WHERE action_id = ?", actionID).Scan(&total); err != nil {
		return nil, 0, err
	}
	if result == nil {
		result = []*models.ActionExecutionLog{}
	}
	return result, total, nil
}

func (s *ActionApplicationService) Execute(userID, workspaceID, actionID, itemID int) (models.ActionExecutionStatus, error) {
	action, err := s.actionInWorkspace(actionID, workspaceID)
	if err != nil {
		return "", err
	}
	allowed, err := s.canExecute(userID, workspaceID, action)
	if err != nil || !allowed {
		return "", ErrActionNotVisible
	}
	if !action.IsEnabled {
		return "", ErrActionDisabled
	}
	itemWorkspaceID, err := s.items.GetWorkspaceID(itemID)
	if err != nil || itemWorkspaceID != workspaceID {
		return "", ErrActionNotVisible
	}
	permission := models.PermissionItemEdit
	if action.TriggerType == models.ActionTriggerManual && len(action.AllowedRoleIDs) > 0 {
		permission = models.PermissionItemView
	}
	if err := s.requireWorkspace(userID, workspaceID, permission); err != nil {
		return "", ErrActionNotVisible
	}
	if s.runtime == nil {
		return "", errors.New("action runtime unavailable")
	}
	err = s.runtime.ExecuteActionManually(action, itemID, userID)
	if errors.Is(err, ErrActionCompletedWithFailedSteps) {
		return models.ActionStatusFailed, nil
	}
	return models.ActionStatusCompleted, err
}

func (s *ActionApplicationService) actionInWorkspace(actionID, workspaceID int) (*models.Action, error) {
	action, err := s.repo.GetByID(actionID)
	if err != nil || action.WorkspaceID != workspaceID {
		return nil, ErrActionNotVisible
	}
	return action, nil
}

func (s *ActionApplicationService) requireManage(userID, workspaceID int) error {
	return s.requireWorkspace(userID, workspaceID, models.PermissionActionManage)
}

func (s *ActionApplicationService) requireWorkspace(userID, workspaceID int, permission string) error {
	allowed, err := s.permissions.HasWorkspacePermission(userID, workspaceID, permission)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrActionNotVisible
	}
	return nil
}

func (s *ActionApplicationService) requireSetActor(userID int) error {
	allowed, err := s.permissions.HasGlobalPermission(userID, models.PermissionActionSetActor)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrActionNotVisible
	}
	return nil
}

func (s *ActionApplicationService) validateRoles(trigger models.ActionTriggerType, roleIDs []int) ([]int, error) {
	if trigger != models.ActionTriggerManual {
		if len(roleIDs) > 0 {
			return nil, errors.New("allowed_role_ids can only be set on manual actions")
		}
		return []int{}, nil
	}
	seen := map[int]struct{}{}
	result := make([]int, 0, len(roleIDs))
	for _, id := range roleIDs {
		if id <= 0 {
			return nil, errors.New("allowed_role_ids must contain positive role IDs")
		}
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			result = append(result, id)
		}
	}
	sort.Ints(result)
	exists, err := s.repo.AllowedRoleIDsExist(result)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.New("allowed_role_ids contains an unknown workspace role")
	}
	return result, nil
}

func (s *ActionApplicationService) canExecute(userID, workspaceID int, action *models.Action) (bool, error) {
	manage, err := s.permissions.HasWorkspacePermission(userID, workspaceID, models.PermissionActionManage)
	if err != nil || manage {
		return manage, err
	}
	if action.TriggerType != models.ActionTriggerManual {
		return false, nil
	}
	permission := models.PermissionItemEdit
	if len(action.AllowedRoleIDs) > 0 {
		permission = models.PermissionItemView
	}
	allowed, err := s.permissions.HasWorkspacePermission(userID, workspaceID, permission)
	if err != nil || !allowed || len(action.AllowedRoleIDs) == 0 {
		return allowed, err
	}
	return s.repo.UserHasAllowedRole(action.ID, userID, workspaceID)
}

func sameIntPointer(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
