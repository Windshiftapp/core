package v2

import (
	"errors"
	"net/http"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

// registerSLATeamBindingRoutes publishes workspace-scoped team bindings on the
// canonical v2 surface. A binding authorizes a workspace to reference a team's
// service calendars; creating or deleting one requires consent from both the
// team and the workspace, so the team-admin direction stays on the internal
// cookie surface.
func registerSLATeamBindingRoutes(builder *routeBuilder, deps Deps) {
	builder.Read("/workspaces/{workspace_id}/team-bindings", AuthAuthenticated, []string{"workspaces:read"}, listSLATeamBindings(deps))
	builder.JSON(http.MethodPost, "/workspaces/{workspace_id}/team-bindings", http.StatusCreated, false, AuthAuthenticated, []string{"workspaces:write"}, createSLATeamBinding(deps))
	builder.Command(http.MethodDelete, "/workspaces/{workspace_id}/team-bindings/{binding_id}", AuthAuthenticated, []string{"workspaces:write"}, deleteSLATeamBinding(deps))
}

type slaTeamBindingRequest struct {
	TeamID int `json:"team_id"`
}

func listSLATeamBindings(deps Deps) readOperation[[]models.TeamWorkspaceBinding] {
	return func(r *http.Request) ([]models.TeamWorkspaceBinding, error) {
		service, workspaceID, err := requireSLATeamBindingAdmin(r, deps)
		if err != nil {
			return nil, err
		}
		bindings, err := service.ListForWorkspace(r.Context(), workspaceID)
		if err != nil {
			return nil, internalError(err)
		}
		return bindings, nil
	}
}

func createSLATeamBinding(deps Deps) jsonOperation[slaTeamBindingRequest, *models.TeamWorkspaceBinding] {
	return func(r *http.Request, request slaTeamBindingRequest) (*models.TeamWorkspaceBinding, error) {
		service, workspaceID, err := requireSLATeamBindingAdmin(r, deps)
		if err != nil {
			return nil, err
		}
		user, err := principal(r)
		if err != nil {
			return nil, err
		}
		binding, err := service.Create(r.Context(), user.ID, request.TeamID, workspaceID)
		if err != nil {
			return nil, slaTeamBindingError(err)
		}
		logger.Record(r, logger.ActionSLATeamBindingCreate, logger.ResourceSLATeamBinding, &binding.ID, "", map[string]any{"workspace_id": workspaceID, "team_id": request.TeamID})
		return binding, nil
	}
}

func deleteSLATeamBinding(deps Deps) commandOperation {
	return func(r *http.Request) error {
		service, workspaceID, err := requireSLATeamBindingAdmin(r, deps)
		if err != nil {
			return err
		}
		user, err := principal(r)
		if err != nil {
			return err
		}
		bindingID, err := pathID(r, "binding_id")
		if err != nil {
			return err
		}
		if err := service.DeleteForWorkspace(r.Context(), user.ID, workspaceID, bindingID); err != nil {
			return slaTeamBindingError(err)
		}
		logger.Record(r, logger.ActionSLATeamBindingDelete, logger.ResourceSLATeamBinding, &bindingID, "", map[string]any{"workspace_id": workspaceID})
		return nil
	}
}

// requireSLATeamBindingAdmin resolves the service and enforces workspace-admin
// access. The service enforces the team-admin half of the consent.
func requireSLATeamBindingAdmin(r *http.Request, deps Deps) (*services.SLATeamBindingService, int, error) {
	if deps.SLATeamBindings == nil {
		return nil, 0, newError(http.StatusNotFound, "not_found", "SLA team bindings are not available")
	}
	user, workspaceID, err := principalAndWorkspace(r)
	if err != nil {
		return nil, 0, err
	}
	if err := requireWorkspace(deps.Access.CanAdminWorkspace, user.ID, workspaceID); err != nil {
		return nil, 0, err
	}
	return deps.SLATeamBindings, workspaceID, nil
}

func slaTeamBindingError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, services.ErrSLABindingForbidden):
		return newError(http.StatusForbidden, "forbidden", err.Error())
	case errors.Is(err, repository.ErrDuplicateEntry):
		return newError(http.StatusConflict, "conflict", "team is already bound to this workspace")
	case errors.Is(err, repository.ErrNotFound):
		return newError(http.StatusNotFound, "not_found", "Binding was not found")
	case errors.Is(err, repository.ErrSLAInUse):
		return newError(http.StatusConflict, "conflict", err.Error())
	default:
		return internalError(err)
	}
}
