package services

import (
	"errors"
	"fmt"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/repository/actionutil"
	"windshift/internal/services/actioncatalog"
)

var ErrActionDefinitionInvalid = errors.New("action definition is invalid")

// ActionDefinitionService owns validation and graph persistence for action
// definitions shared by HTTP transports.
type ActionDefinitionService struct {
	repo    *repository.ActionRepository
	runtime *ActionService
	asset   *AssetService
}

func NewActionDefinitionService(repo *repository.ActionRepository, runtime *ActionService) *ActionDefinitionService {
	return &ActionDefinitionService{repo: repo, runtime: runtime}
}

func (s *ActionDefinitionService) SetAssetService(asset *AssetService) {
	s.asset = asset
}

func (s *ActionDefinitionService) HasCapability(workspaceID, capabilityID int) bool {
	capability, err := s.repo.GetCapabilityByID(capabilityID)
	if err != nil || capability == nil || !capability.IsEnabled {
		return false
	}
	scoped, err := s.repo.IsCapabilityScopedToWorkspace(capabilityID, workspaceID)
	return err == nil && scoped
}

func (s *ActionDefinitionService) HasCapabilityOfType(workspaceID, capabilityID int, capabilityType models.CapabilityType) bool {
	capability, err := s.repo.GetCapabilityByID(capabilityID)
	if err != nil || capability == nil || !capability.IsEnabled || capability.CapabilityType != capabilityType {
		return false
	}
	scoped, err := s.repo.IsCapabilityScopedToWorkspace(capabilityID, workspaceID)
	return err == nil && scoped
}

func (s *ActionDefinitionService) Validate(workspaceID int, definition actioncatalog.ActionDefinition) (actioncatalog.ValidationErrors, error) {
	if errs := actioncatalog.Validate(actioncatalog.Default(), definition, workspaceID, s); len(errs) > 0 {
		return errs, nil
	}
	if s.asset != nil {
		if err := s.asset.ValidateActionTaxonomyReferences(definition.Nodes); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrActionDefinitionInvalid, err)
		}
	}
	return nil, nil
}

func (s *ActionDefinitionService) Create(action *models.Action, nodes []models.ActionNode, edges []models.ActionEdge) (*models.Action, actioncatalog.ValidationErrors, error) {
	definition := actioncatalog.ActionDefinition{
		Name: action.Name, Description: action.Description, TriggerType: action.TriggerType,
		TriggerConfig: action.TriggerConfig, Nodes: nodes, Edges: edges,
	}
	if validationErrors, err := s.Validate(action.WorkspaceID, definition); err != nil || len(validationErrors) > 0 {
		return nil, validationErrors, err
	}

	actionID, err := s.repo.Create(action)
	if err != nil {
		return nil, nil, err
	}
	action.ID = actionID
	if err := actionutil.CreateFlowNodesAndEdges[
		models.ActionNode, *models.ActionNode,
		models.ActionEdge, *models.ActionEdge,
	](
		actionID, nodes, edges,
		func(node *models.ActionNode) (int, error) { return s.repo.CreateNode(node) },
		func(edge *models.ActionEdge) (int, error) { return s.repo.CreateEdge(edge) },
		func() { _ = s.repo.Delete(actionID) },
	); err != nil {
		return nil, nil, fmt.Errorf("create action graph: %w", err)
	}
	s.invalidate(action.WorkspaceID)
	created, err := s.repo.GetByID(actionID)
	return created, nil, err
}

// Save validates the effective definition and persists either a graph
// replacement or metadata-only update.
func (s *ActionDefinitionService) Save(action *models.Action, nodes []models.ActionNode, edges []models.ActionEdge, replaceGraph, validate bool) (*models.Action, actioncatalog.ValidationErrors, error) {
	definition := actioncatalog.ActionDefinition{
		Name: action.Name, Description: action.Description, TriggerType: action.TriggerType,
		TriggerConfig: action.TriggerConfig, Nodes: nodes, Edges: edges,
	}
	if validate {
		if validationErrors, err := s.Validate(action.WorkspaceID, definition); err != nil || len(validationErrors) > 0 {
			return nil, validationErrors, err
		}
	}
	var err error
	if replaceGraph {
		err = s.repo.SaveActionWithNodesAndEdges(action, nodes, edges)
	} else {
		err = s.repo.Update(action)
	}
	if err != nil {
		return nil, nil, err
	}
	s.invalidate(action.WorkspaceID)
	updated, err := s.repo.GetByID(action.ID)
	return updated, nil, err
}

func (s *ActionDefinitionService) Delete(actionID, workspaceID int) error {
	if err := s.repo.Delete(actionID); err != nil {
		return err
	}
	s.invalidate(workspaceID)
	return nil
}

func (s *ActionDefinitionService) invalidate(workspaceID int) {
	if s.runtime != nil {
		s.runtime.InvalidateWorkspaceCache(workspaceID)
	}
}
