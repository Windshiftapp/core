package services

import (
	"context"
	"errors"
	"fmt"

	"windshift/internal/database"
	"windshift/internal/logger"
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
// caller's audit events, in one transaction. The actor patch and its
// authorization are resolved against the locked action row so a concurrent
// actor change cannot be overwritten by a stale in-memory copy.
func (s *ActionDefinitionService) Save(action *models.Action, nodes []models.ActionNode, edges []models.ActionEdge, replaceGraph, validate bool, actorPatch models.ActionActorUpdate, meta ActionWriteMeta) (*models.Action, actioncatalog.ValidationErrors, error) {
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
		// The UPDATE locks the row (PostgreSQL) or holds the single write
		// connection (SQLite), so the actor read below is authoritative.
		if err := s.repo.UpdateTx(tx, action); err != nil {
			return nil, err
		}
		currentActor, err := s.repo.GetActorUserIDTx(tx, action.ID)
		if err != nil {
			return nil, err
		}
		effectiveActor := currentActor
		if actorPatch.Present {
			effectiveActor = actorPatch.Value
		}
		actorChanging := !sameIntPointer(effectiveActor, currentActor)
		action.ActorUserID = effectiveActor
		if meta.AuthorizeActor != nil {
			if err := meta.AuthorizeActor(currentActor, effectiveActor, actorChanging); err != nil {
				return nil, err
			}
		}
		if actorChanging {
			if err := s.repo.SetActorTx(tx, action.ID, effectiveActor); err != nil {
				return nil, err
			}
			oldActor := 0
			if currentActor != nil {
				oldActor = *currentActor
			}
			newActor := 0
			if effectiveActor != nil {
				newActor = *effectiveActor
			}
			meta.ExtraAudits = append(meta.ExtraAudits, PendingAudit{
				ActionType: logger.ActionAutomationSetActor,
				Details:    map[string]any{"old_actor_user_id": oldActor, "new_actor_user_id": newActor},
			})
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
		// The denied-audit write must happen after the mutation transaction has
		// rolled back: LogAudit uses the write connection that the rolled-back
		// transaction still holds on SQLite.
		var denied *PrivilegedActionEditDenied
		if errors.As(err, &denied) {
			s.auditDeniedPrivilegedEdit(meta.Actor, action, denied)
		}
		return nil, nil, err
	}
	s.invalidate(action.WorkspaceID)
	return updated, nil, nil
}

// auditDeniedPrivilegedEdit records a rejected attempt to change the executable
// configuration of a privileged action without action.set_actor. It stores only
// safe metadata: which fields were attempted, never the definition itself. The
// write is independent of the rolled-back mutation transaction.
func (s *ActionDefinitionService) auditDeniedPrivilegedEdit(actor AuditActor, action *models.Action, denied *PrivilegedActionEditDenied) {
	required := denied.RequiredPermission
	if required == "" {
		required = models.PermissionActionSetActor
	}
	_ = logger.LogAudit(s.db, logger.AuditEvent{
		UserID:       actor.UserID,
		Username:     actor.Username,
		IPAddress:    actor.IPAddress,
		UserAgent:    actor.UserAgent,
		Source:       actor.Source,
		ActionType:   logger.ActionAccessDenied,
		ResourceType: logger.ResourceAutomation,
		ResourceID:   &action.ID,
		ResourceName: action.Name,
		Success:      false,
		ErrorMessage: denied.Error(),
		Details: mergeAuditDetails(map[string]any{
			"workspace_id":        action.WorkspaceID,
			"required_permission": required,
			"attempted_fields":    denied.AttemptedFields,
			"actor_user_id":       action.ActorUserID,
		}, actor),
	})
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
