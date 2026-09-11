package v2

import (
	"database/sql"
	"errors"
	"net/http"

	"windshift/internal/contextkeys"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

func registerLabelRoutes(builder *routeBuilder, deps Deps) {
	builder.JSON(http.MethodPost, "/workspaces/{workspace_id}/labels", http.StatusCreated, false, AuthAuthenticated, []string{"items:write"}, createLabel(deps))
	builder.JSON(http.MethodPatch, "/workspaces/{workspace_id}/labels/{label_id}", http.StatusOK, true, AuthAuthenticated, []string{"items:write"}, updateLabel(deps))
	builder.Command(http.MethodDelete, "/workspaces/{workspace_id}/labels/{label_id}", AuthAuthenticated, []string{"items:write"}, deleteLabel(deps))
	builder.Read("/items/{item_id}/labels", AuthAuthenticated, []string{"items:read"}, listItemLabels(deps))
	builder.JSON(http.MethodPut, "/items/{item_id}/labels", http.StatusOK, false, AuthAuthenticated, []string{"items:write"}, setItemLabels(deps))
	builder.JSON(http.MethodPost, "/items/{item_id}/labels", http.StatusOK, false, AuthAuthenticated, []string{"items:write"}, addItemLabel(deps))
	builder.Command(http.MethodDelete, "/items/{item_id}/labels/{label_id}", AuthAuthenticated, []string{"items:write"}, removeItemLabel(deps))
}

type labelCreateRequest struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

type labelPatchRequest struct {
	Name  Optional[string] `json:"name"`
	Color Optional[string] `json:"color"`
}

type labelIDsRequest struct {
	LabelIDs []int `json:"label_ids"`
}

type labelIDRequest struct {
	LabelID int `json:"label_id"`
}

func createLabel(deps Deps) jsonOperation[labelCreateRequest, labelDTO] {
	return func(r *http.Request, input labelCreateRequest) (labelDTO, error) {
		user, workspaceID, err := principalAndWorkspace(r)
		if err != nil {
			return labelDTO{}, err
		}
		if err := requireWorkspace(deps.Access.CanEditWorkspace, user.ID, workspaceID); err != nil {
			return labelDTO{}, err
		}
		label, err := deps.Labels.Create(auditActor(r, user), input.Name, input.Color)
		if err != nil {
			return labelDTO{}, labelMutationError(err)
		}
		return labelFromModel(label), nil
	}
}

func updateLabel(deps Deps) jsonOperation[labelPatchRequest, labelDTO] {
	return func(r *http.Request, input labelPatchRequest) (labelDTO, error) {
		user, workspaceID, err := principalAndWorkspace(r)
		if err != nil {
			return labelDTO{}, err
		}
		if err := requireWorkspace(deps.Access.CanAdminWorkspace, user.ID, workspaceID); err != nil {
			return labelDTO{}, err
		}
		labelID, err := pathID(r, "label_id")
		if err != nil {
			return labelDTO{}, err
		}
		if input.Name.Null || input.Color.Null {
			return labelDTO{}, newError(http.StatusBadRequest, "invalid_request", "Label fields cannot be null")
		}
		update := services.LabelUpdate{}
		if input.Name.Set {
			update.Name = &input.Name.Value
		}
		if input.Color.Set {
			update.Color = &input.Color.Value
		}
		label, err := deps.Labels.Update(auditActor(r, user), labelID, update)
		if err != nil {
			return labelDTO{}, labelMutationError(err)
		}
		return labelFromModel(label), nil
	}
}

func deleteLabel(deps Deps) commandOperation {
	return func(r *http.Request) error {
		user, workspaceID, err := principalAndWorkspace(r)
		if err != nil {
			return err
		}
		if err := requireWorkspace(deps.Access.CanAdminWorkspace, user.ID, workspaceID); err != nil {
			return err
		}
		labelID, err := pathID(r, "label_id")
		if err != nil {
			return err
		}
		return labelMutationError(deps.Labels.Delete(auditActor(r, user), labelID))
	}
}

func listItemLabels(deps Deps) readOperation[[]labelDTO] {
	return func(r *http.Request) ([]labelDTO, error) {
		item, err := requireItem(r, deps, deps.Access.CanViewWorkspace)
		if err != nil {
			return nil, err
		}
		labels, err := deps.Labels.ListForItem(item.ID)
		if err != nil {
			return nil, internalError(err)
		}
		result := make([]labelDTO, len(labels))
		for i := range labels {
			result[i] = labelFromModel(&labels[i])
		}
		return result, nil
	}
}

func setItemLabels(deps Deps) jsonOperation[labelIDsRequest, []labelDTO] {
	return func(r *http.Request, input labelIDsRequest) ([]labelDTO, error) {
		item, err := requireItem(r, deps, deps.Access.CanEditWorkspace)
		if err != nil {
			return nil, err
		}
		labels, err := deps.Labels.SetForItem(item.ID, input.LabelIDs)
		return mapLabelMutation(labels, err)
	}
}

func addItemLabel(deps Deps) jsonOperation[labelIDRequest, []labelDTO] {
	return func(r *http.Request, input labelIDRequest) ([]labelDTO, error) {
		item, err := requireItem(r, deps, deps.Access.CanEditWorkspace)
		if err != nil {
			return nil, err
		}
		labels, err := deps.Labels.AddToItem(item.ID, input.LabelID)
		return mapLabelMutation(labels, err)
	}
}

func removeItemLabel(deps Deps) commandOperation {
	return func(r *http.Request) error {
		item, err := requireItem(r, deps, deps.Access.CanEditWorkspace)
		if err != nil {
			return err
		}
		labelID, err := pathID(r, "label_id")
		if err != nil {
			return err
		}
		if err := deps.Labels.RemoveFromItem(item.ID, labelID); err != nil {
			return internalError(err)
		}
		return nil
	}
}

func requireItem(r *http.Request, deps Deps, allowed func(int, int) (bool, error)) (*models.Item, error) {
	itemID, err := pathID(r, "item_id")
	if err != nil {
		return nil, err
	}
	return requireItemID(r, deps, itemID, allowed)
}

func requireItemID(r *http.Request, deps Deps, itemID int, allowed func(int, int) (bool, error)) (*models.Item, error) {
	user, err := principal(r)
	if err != nil {
		return nil, err
	}
	item, err := deps.Items.FindByID(itemID)
	if err != nil {
		return nil, readError(err, "Item was not found")
	}
	ok, err := allowed(user.ID, item.WorkspaceID)
	if err != nil {
		return nil, internalError(err)
	}
	if !ok {
		return nil, newError(http.StatusNotFound, "not_found", "Item was not found")
	}
	return item, nil
}

func requireWorkspace(allowed func(int, int) (bool, error), userID, workspaceID int) error {
	ok, err := allowed(userID, workspaceID)
	if err != nil {
		return internalError(err)
	}
	if !ok {
		return newError(http.StatusNotFound, "not_found", "Workspace was not found")
	}
	return nil
}

func auditActor(r *http.Request, user *models.User) services.AuditActor {
	token, _ := r.Context().Value(contextkeys.APIToken).(*models.APIToken)
	authMethod, _ := r.Context().Value(contextkeys.AuthMethod).(string)
	return services.NewAuditActorFromRequest(r, user, token, authMethod)
}

func mapLabelMutation(labels []models.Label, err error) ([]labelDTO, error) {
	if err != nil {
		return nil, labelMutationError(err)
	}
	result := make([]labelDTO, len(labels))
	for i := range labels {
		result[i] = labelFromModel(&labels[i])
	}
	return result, nil
}

func labelMutationError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, repository.ErrNotFound), errors.Is(err, sql.ErrNoRows):
		return newError(http.StatusNotFound, "not_found", "Label was not found")
	case errors.Is(err, repository.ErrDuplicateEntry):
		return newError(http.StatusConflict, "conflict", "Label already exists")
	case errors.Is(err, services.ErrLabelNameRequired), errors.Is(err, services.ErrLabelIDRequired):
		return newError(http.StatusBadRequest, "invalid_request", err.Error())
	default:
		return internalError(err)
	}
}
