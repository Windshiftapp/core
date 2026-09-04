package v2

import (
	"errors"
	"net/http"
	"strings"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

func registerActionRoutes(builder *routeBuilder, actions actionApplication) {
	builder.Read("/action-templates", AuthAuthenticated, []string{"actions:read"}, listActionTemplates(actions))
	builder.Action(http.MethodPost, "/workspaces/{workspace_id}/action-templates/{template_key}/apply", http.StatusCreated, AuthAuthenticated, []string{"actions:write"}, applyActionTemplate(actions))
	builder.Read("/workspaces/{workspace_id}/action-catalog", AuthAuthenticated, []string{"actions:read"}, actionCatalog(actions))
	builder.Read("/workspaces/{workspace_id}/actions", AuthAuthenticated, []string{"actions:read"}, listActions(actions))
	builder.JSON(http.MethodPost, "/workspaces/{workspace_id}/actions", http.StatusCreated, false, AuthAuthenticated, []string{"actions:write"}, createAction(actions))
	builder.Read("/workspaces/{workspace_id}/actions/{action_id}", AuthAuthenticated, []string{"actions:read"}, getAction(actions))
	builder.JSON(http.MethodPatch, "/workspaces/{workspace_id}/actions/{action_id}", http.StatusOK, true, AuthAuthenticated, []string{"actions:write"}, updateAction(actions))
	builder.Command(http.MethodDelete, "/workspaces/{workspace_id}/actions/{action_id}", AuthAuthenticated, []string{"actions:write"}, deleteAction(actions))
	builder.JSON(http.MethodPost, "/workspaces/{workspace_id}/actions/{action_id}/execute", http.StatusOK, false, AuthAuthenticated, []string{"actions:write"}, executeAction(actions))
	builder.Page("/workspaces/{workspace_id}/actions/{action_id}/logs", AuthAuthenticated, []string{"actions:read"}, actionLogs(actions))
}

func listActionTemplates(actions actionApplication) readOperation[[]services.ActionTemplateSummary] {
	return func(_ *http.Request) ([]services.ActionTemplateSummary, error) {
		return actions.ListTemplates(), nil
	}
}

func applyActionTemplate(actions actionApplication) actionOperation[*services.ApplyToWorkspaceResult] {
	return func(r *http.Request) (*services.ApplyToWorkspaceResult, error) {
		user, workspaceID, _, err := actionTarget(r, false)
		if err != nil {
			return nil, err
		}
		key := strings.TrimSpace(r.PathValue("template_key"))
		if key == "" {
			return nil, newError(http.StatusBadRequest, "invalid_request", "template_key is required")
		}
		result, err := actions.ApplyTemplate(r.Context(), user.ID, workspaceID, auditActor(r, user), key)
		return result, actionError(err)
	}
}

type actionExecuteRequest struct {
	ItemID int `json:"item_id"`
}

type actionExecuteResponse struct {
	Status models.ActionExecutionStatus `json:"status"`
}

func actionCatalog(actions actionApplication) readOperation[services.ActionCatalog] {
	return func(r *http.Request) (services.ActionCatalog, error) {
		user, workspaceID, _, err := actionTarget(r, false)
		if err != nil {
			return services.ActionCatalog{}, err
		}
		result, err := actions.Catalog(user.ID, workspaceID)
		return result, actionError(err)
	}
}

func listActions(actions actionApplication) readOperation[[]*models.Action] {
	return func(r *http.Request) ([]*models.Action, error) {
		user, workspaceID, _, err := actionTarget(r, false)
		if err != nil {
			return nil, err
		}
		result, err := actions.List(user.ID, workspaceID)
		return result, actionError(err)
	}
}

func getAction(actions actionApplication) readOperation[*models.Action] {
	return func(r *http.Request) (*models.Action, error) {
		user, workspaceID, actionID, err := actionTarget(r, true)
		if err != nil {
			return nil, err
		}
		result, err := actions.Get(user.ID, workspaceID, actionID)
		return result, actionError(err)
	}
}

func createAction(actions actionApplication) jsonOperation[models.CreateActionRequest, *models.Action] {
	return func(r *http.Request, input models.CreateActionRequest) (*models.Action, error) {
		user, workspaceID, _, err := actionTarget(r, false)
		if err != nil {
			return nil, err
		}
		result, err := actions.Create(user.ID, workspaceID, auditActor(r, user), input)
		return result, actionError(err)
	}
}

func updateAction(actions actionApplication) jsonOperation[models.UpdateActionRequest, *models.Action] {
	return func(r *http.Request, input models.UpdateActionRequest) (*models.Action, error) {
		user, workspaceID, actionID, err := actionTarget(r, true)
		if err != nil {
			return nil, err
		}
		result, err := actions.Update(user.ID, workspaceID, actionID, auditActor(r, user), input)
		return result, actionError(err)
	}
}

func deleteAction(actions actionApplication) commandOperation {
	return func(r *http.Request) error {
		user, workspaceID, actionID, err := actionTarget(r, true)
		if err != nil {
			return err
		}
		return actionError(actions.Delete(user.ID, workspaceID, actionID, auditActor(r, user)))
	}
}

func executeAction(actions actionApplication) jsonOperation[actionExecuteRequest, actionExecuteResponse] {
	return func(r *http.Request, input actionExecuteRequest) (actionExecuteResponse, error) {
		user, workspaceID, actionID, err := actionTarget(r, true)
		if err != nil {
			return actionExecuteResponse{}, err
		}
		if input.ItemID <= 0 {
			return actionExecuteResponse{}, newError(http.StatusBadRequest, "invalid_request", "item_id is required")
		}
		status, err := actions.Execute(user.ID, workspaceID, actionID, input.ItemID)
		return actionExecuteResponse{Status: status}, actionError(err)
	}
}

func actionLogs(actions actionApplication) pageOperation[*models.ActionExecutionLog] {
	return func(r *http.Request) ([]*models.ActionExecutionLog, Pagination, int, error) {
		user, workspaceID, actionID, err := actionTarget(r, true)
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		page, err := ParsePage(r)
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		result, total, err := actions.Logs(user.ID, workspaceID, actionID, page.PageSize, page.Offset)
		return result, page, total, actionError(err)
	}
}

func actionTarget(r *http.Request, withAction bool) (user *models.User, workspaceID, actionID int, err error) {
	user, err = principal(r)
	if err != nil {
		return nil, 0, 0, err
	}
	workspaceID, err = pathID(r, "workspace_id")
	if err != nil {
		return nil, 0, 0, err
	}
	if !withAction {
		return user, workspaceID, 0, nil
	}
	actionID, err = pathID(r, "action_id")
	return user, workspaceID, actionID, err
}

func actionError(err error) error {
	if err == nil {
		return nil
	}
	var validation *services.ActionValidationError
	switch {
	case errors.Is(err, services.ErrActionNotVisible), errors.Is(err, repository.ErrNotFound):
		return newError(http.StatusNotFound, "not_found", "Action was not found")
	case errors.Is(err, services.ErrActionDisabled), errors.Is(err, services.ErrActionDefinitionInvalid), errors.As(err, &validation), strings.Contains(err.Error(), "allowed_role_ids"):
		return newError(http.StatusBadRequest, "invalid_request", err.Error())
	default:
		return internalError(err)
	}
}
