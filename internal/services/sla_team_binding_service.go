package services

import (
	"context"
	"errors"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// ErrSLABindingForbidden marks a team-workspace binding mutation that lacks
// consent from both the team and the workspace.
var ErrSLABindingForbidden = errors.New("team-workspace binding requires team admin and workspace admin")

// SLATeamBindingService owns team-to-workspace consent bindings. Creating or
// deleting a binding requires both team administration authority and workspace
// administration; using an existing binding requires neither.
type SLATeamBindingService struct {
	repo        *repository.SLARepository
	teams       *repository.TeamRepository
	permissions *PermissionService
}

// NewSLATeamBindingService constructs the binding service.
func NewSLATeamBindingService(db database.Database, permissions *PermissionService) *SLATeamBindingService {
	return &SLATeamBindingService{
		repo:        repository.NewSLARepository(db),
		teams:       repository.NewTeamRepository(db),
		permissions: permissions,
	}
}

// ListForWorkspace returns every binding whose workspace is workspaceID.
func (s *SLATeamBindingService) ListForWorkspace(ctx context.Context, workspaceID int) ([]models.TeamWorkspaceBinding, error) {
	return s.repo.ListTeamWorkspaceBindings(ctx, workspaceID)
}

// ListForTeam returns every binding whose team is teamID.
func (s *SLATeamBindingService) ListForTeam(ctx context.Context, teamID int) ([]models.TeamWorkspaceBinding, error) {
	return s.repo.ListTeamWorkspaceBindingsForTeam(ctx, teamID)
}

// Create binds a team and workspace after both sides consent.
func (s *SLATeamBindingService) Create(ctx context.Context, actorID, teamID, workspaceID int) (*models.TeamWorkspaceBinding, error) {
	if err := s.requireConsent(actorID, teamID, workspaceID); err != nil {
		return nil, err
	}
	id, err := s.repo.CreateTeamWorkspaceBinding(ctx, &models.TeamWorkspaceBinding{
		TeamID:      teamID,
		WorkspaceID: workspaceID,
		CreatedBy:   &actorID,
	})
	if err != nil {
		return nil, err
	}
	return s.repo.GetTeamWorkspaceBinding(ctx, id)
}

// DeleteForWorkspace removes a binding scoped to its workspace after both
// sides consent.
func (s *SLATeamBindingService) DeleteForWorkspace(ctx context.Context, actorID, workspaceID, bindingID int) error {
	binding, err := s.repo.GetTeamWorkspaceBinding(ctx, bindingID)
	if err != nil {
		return err
	}
	if binding.WorkspaceID != workspaceID {
		return repository.ErrNotFound
	}
	return s.delete(ctx, actorID, binding)
}

// DeleteForTeam removes a binding scoped to its team after both sides consent.
func (s *SLATeamBindingService) DeleteForTeam(ctx context.Context, actorID, teamID, bindingID int) error {
	binding, err := s.repo.GetTeamWorkspaceBinding(ctx, bindingID)
	if err != nil {
		return err
	}
	if binding.TeamID != teamID {
		return repository.ErrNotFound
	}
	return s.delete(ctx, actorID, binding)
}

func (s *SLATeamBindingService) delete(ctx context.Context, actorID int, binding *models.TeamWorkspaceBinding) error {
	if err := s.requireConsent(actorID, binding.TeamID, binding.WorkspaceID); err != nil {
		return err
	}
	return s.repo.DeleteTeamWorkspaceBinding(ctx, binding.WorkspaceID, binding.ID)
}

func (s *SLATeamBindingService) requireConsent(userID, teamID, workspaceID int) error {
	teamAuthority, err := s.hasTeamAuthority(userID, teamID)
	if err != nil {
		return err
	}
	workspaceAdmin, err := s.permissions.HasWorkspacePermission(userID, workspaceID, models.PermissionWorkspaceAdmin)
	if err != nil {
		return err
	}
	if !teamAuthority || !workspaceAdmin {
		return ErrSLABindingForbidden
	}
	return nil
}

// hasTeamAuthority accepts a holder of the global teams.manage permission
// (which includes system administrators) or a direct team admin.
func (s *SLATeamBindingService) hasTeamAuthority(userID, teamID int) (bool, error) {
	global, err := s.permissions.HasGlobalPermission(userID, models.PermissionTeamsManage)
	if err != nil {
		return false, err
	}
	if global {
		return true, nil
	}
	return s.teams.IsTeamAdmin(teamID, userID)
}
