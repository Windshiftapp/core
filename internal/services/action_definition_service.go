package services

import (
	"context"
	"errors"
	"fmt"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services/actioncatalog"
)

var ErrActionDefinitionInvalid = errors.New("action definition is invalid")

// ActionDefinitionService owns validation and graph persistence for action
// definitions shared by HTTP transports. Every successful mutation persists an
// immutable revision and its audit rows in the same transaction as the action
// row and graph, so an audit or revision failure rolls the mutation back.
type ActionDefinitionService struct {
	db        database.Database
	repo      *repository.ActionRepository
	revisions *repository.ActionRevisionRepository
	runtime   *ActionService
	asset     *AssetService
}

func NewActionDefinitionService(db database.Database, repo *repository.ActionRepository, runtime *ActionService) *ActionDefinitionService {
	return &ActionDefinitionService{
		db:        db,
		repo:      repo,
		revisions: repository.NewActionRevisionRepository(db),
		runtime:   runtime,
	}
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

// Create validates and persists a new action, its graph, its first immutable
// revision, and the caller's audit events in one transaction.
func (s *ActionDefinitionService) Create(action *models.Action, nodes []models.ActionNode, edges []models.ActionEdge, meta ActionWriteMeta) (*models.Action, actioncatalog.ValidationErrors, error) {
	definition := actioncatalog.ActionDefinition{
		Name: action.Name, Description: action.Description, TriggerType: action.TriggerType,
		TriggerConfig: action.TriggerConfig, Nodes: nodes, Edges: edges,
	}
	if validationErrors, err := s.Validate(action.WorkspaceID, definition); err != nil || len(validationErrors) > 0 {
		return nil, validationErrors, err
	}
	if meta.Source == "" {
		meta.Source = models.ActionRevisionSourceCreate
	}

	created, err := database.WithTxResult(s.db, func(tx database.Tx) (*models.Action, error) {
		actionID, err := s.repo.CreateTx(tx, action)
		if err != nil {
			return nil, err
		}
		action.ID = actionID
		if err := s.repo.SaveGraphTx(tx, actionID, nodes, edges); err != nil {
			return nil, fmt.Errorf("create action graph: %w", err)
		}
		persistedNodes, err := s.repo.GetNodesByActionIDTx(tx, actionID)
		if err != nil {
			return nil, err
		}
		persistedEdges, err := s.repo.GetEdgesByActionIDTx(tx, actionID)
		if err != nil {
			return nil, err
		}
		action.Nodes = persistedNodes
		action.Edges = persistedEdges
		if err := persistActionRevisionTx(context.Background(), tx, s.revisions, action, persistedNodes, persistedEdges, meta.Source, meta); err != nil {
			return nil, err
		}
		return action, nil
	})
	if err != nil {
		return nil, nil, err
	}
	s.invalidate(action.WorkspaceID)
	return created, nil, nil
}

// Save validates the effective definition and persists either a graph
// replacement or a metadata-only update, together with a new revision and the
// caller's audit events, in one transaction. actorChanged applies the actor
// override inside the same transaction.
func (s *ActionDefinitionService) Save(action *models.Action, nodes []models.ActionNode, edges []models.ActionEdge, replaceGraph, validate, actorChanged bool, meta ActionWriteMeta) (*models.Action, actioncatalog.ValidationErrors, error) {
	definition := actioncatalog.ActionDefinition{
		Name: action.Name, Description: action.Description, TriggerType: action.TriggerType,
		TriggerConfig: action.TriggerConfig, Nodes: nodes, Edges: edges,
	}
	if validate {
		if validationErrors, err := s.Validate(action.WorkspaceID, definition); err != nil || len(validationErrors) > 0 {
			return nil, validationErrors, err
		}
	}
	if meta.Source == "" {
		meta.Source = models.ActionRevisionSourceUpdate
	}

	updated, err := database.WithTxResult(s.db, func(tx database.Tx) (*models.Action, error) {
		if err := s.repo.UpdateTx(tx, action); err != nil {
			return nil, err
		}
		if actorChanged {
			if err := s.repo.SetActorTx(tx, action.ID, action.ActorUserID); err != nil {
				return nil, err
			}
		}
		if replaceGraph {
			if err := s.repo.SaveGraphTx(tx, action.ID, nodes, edges); err != nil {
				return nil, err
			}
		}
		persistedNodes, err := s.repo.GetNodesByActionIDTx(tx, action.ID)
		if err != nil {
			return nil, err
		}
		persistedEdges, err := s.repo.GetEdgesByActionIDTx(tx, action.ID)
		if err != nil {
			return nil, err
		}
		action.Nodes = persistedNodes
		action.Edges = persistedEdges
		if err := persistActionRevisionTx(context.Background(), tx, s.revisions, action, persistedNodes, persistedEdges, meta.Source, meta); err != nil {
			return nil, err
		}
		return action, nil
	})
	if err != nil {
		return nil, nil, err
	}
	s.invalidate(action.WorkspaceID)
	return updated, nil, nil
}

// Delete removes an action and writes the delete audit row in one transaction.
// Revisions are retained so an operator can still reconstruct the definition
// that ran before deletion.
func (s *ActionDefinitionService) Delete(action *models.Action, meta ActionWriteMeta) error {
	err := database.WithTx(s.db, func(tx database.Tx) error {
		if err := s.repo.DeleteTx(tx, action.ID); err != nil {
			return err
		}
		details := map[string]any{"workspace_id": action.WorkspaceID}
		if action.CurrentRevisionID != nil {
			details["revision_id"] = *action.CurrentRevisionID
		}
		for key, value := range meta.PrimaryDetails {
			details[key] = value
		}
		return writeActionAuditTx(context.Background(), tx, meta.Actor, meta.PrimaryActionType, action, details)
	})
	if err != nil {
		return err
	}
	s.invalidate(action.WorkspaceID)
	return nil
}

func (s *ActionDefinitionService) invalidate(workspaceID int) {
	if s.runtime != nil {
		s.runtime.InvalidateWorkspaceCache(workspaceID)
	}
}
