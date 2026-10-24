package services

import (
	"context"
	"errors"
	"fmt"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

// WorkspaceUserResolver is the canonical source for users who can act on a
// workspace item. Humans need item-view access. Agents additionally need a
// ready binding in that workspace.
type WorkspaceUserResolver struct {
	users       *UserReadService
	permissions *PermissionService
	presence    *AgentPresenceService
}

// NewWorkspaceUserResolver constructs a workspace-scoped user resolver.
func NewWorkspaceUserResolver(db database.Database, permissions *PermissionService) *WorkspaceUserResolver {
	return &WorkspaceUserResolver{
		users:       NewUserReadService(db),
		permissions: permissions,
		presence: NewAgentPresenceService(
			repository.NewWorkspaceAgentBindingRepository(db),
			repository.NewRunnerRepository(db),
		),
	}
}

// List returns active actionable users with fields limited for picker use.
func (r *WorkspaceUserResolver) List(ctx context.Context, workspaceID int) ([]models.User, error) {
	if workspaceID <= 0 {
		return nil, fmt.Errorf("workspace user resolver: invalid workspace id %d", workspaceID)
	}
	if r == nil || r.users == nil || r.permissions == nil || r.presence == nil {
		return nil, errors.New("workspace user resolver is not configured")
	}

	users, err := r.users.ListAll()
	if err != nil {
		return nil, fmt.Errorf("workspace user resolver: list users: %w", err)
	}
	presence, err := r.presence.ForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("workspace user resolver: %w", err)
	}

	resolved := make([]models.User, 0, len(users))
	for _, user := range users {
		agentPresence, readyAgent := presence[user.ID]
		if user.IsAgent && !readyAgent {
			continue
		}
		allowed, err := r.permissions.HasWorkspacePermission(user.ID, workspaceID, models.PermissionItemView)
		if err != nil {
			return nil, fmt.Errorf("workspace user resolver: check access for user %d: %w", user.ID, err)
		}
		if !allowed {
			continue
		}

		user.Email = ""
		user.Timezone = ""
		user.Language = ""
		if user.IsAgent {
			user.AgentPresence = agentPresence
		}
		resolved = append(resolved, user)
	}
	return resolved, nil
}

// CanAct reports whether an active user belongs in the workspace picker
// roster. It uses the same permission and binding rules as List.
func (r *WorkspaceUserResolver) CanAct(ctx context.Context, userID, workspaceID int) (bool, error) {
	if userID <= 0 || workspaceID <= 0 {
		return false, nil
	}
	if r == nil || r.users == nil || r.permissions == nil || r.presence == nil {
		return false, errors.New("workspace user resolver is not configured")
	}

	user, err := r.users.GetByID(userID)
	if errors.Is(err, repository.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("workspace user resolver: get user %d: %w", userID, err)
	}
	if !user.IsActive {
		return false, nil
	}
	if user.IsAgent {
		presence, err := r.presence.ForWorkspace(ctx, workspaceID)
		if err != nil {
			return false, fmt.Errorf("workspace user resolver: %w", err)
		}
		if _, ready := presence[userID]; !ready {
			return false, nil
		}
	}

	allowed, err := r.permissions.HasWorkspacePermission(userID, workspaceID, models.PermissionItemView)
	if err != nil {
		return false, fmt.Errorf("workspace user resolver: check access for user %d: %w", userID, err)
	}
	return allowed, nil
}

// CanActInWorkspace adapts CanAct to synchronous validation interfaces.
func (r *WorkspaceUserResolver) CanActInWorkspace(userID, workspaceID int) (bool, error) {
	return r.CanAct(context.Background(), userID, workspaceID)
}
