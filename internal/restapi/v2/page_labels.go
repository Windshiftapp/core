package v2

import (
	"errors"
	"net/http"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

func registerPageLabelRoutes(builder *routeBuilder, deps Deps) {
	const labels = "/workspaces/{workspace_id}/page-labels"
	const label = labels + "/{label_id}"
	const assignments = "/workspaces/{workspace_id}/pages/{page_id}/labels"
	builder.Read(labels, AuthAuthenticated, []string{"pages:read"}, listPageLabels(deps))
	builder.JSON(http.MethodPost, labels, http.StatusCreated, false, AuthAuthenticated, []string{"pages:write"}, createPageLabel(deps))
	builder.Read(label, AuthAuthenticated, []string{"pages:read"}, getPageLabel(deps))
	builder.JSON(http.MethodPatch, label, http.StatusOK, true, AuthAuthenticated, []string{"pages:write"}, updatePageLabel(deps))
	builder.Command(http.MethodDelete, label, AuthAuthenticated, []string{"pages:write"}, deletePageLabel(deps))
	builder.Read(assignments, AuthAuthenticated, []string{"pages:read"}, listLabelsForPage(deps))
	builder.JSON(http.MethodPut, assignments, http.StatusOK, false, AuthAuthenticated, []string{"pages:write"}, setLabelsForPage(deps))
	builder.JSON(http.MethodPost, assignments, http.StatusOK, false, AuthAuthenticated, []string{"pages:write"}, addLabelToPage(deps))
	builder.Command(http.MethodDelete, assignments+"/{label_id}", AuthAuthenticated, []string{"pages:write"}, removeLabelFromPage(deps))
}

type pageLabelCreateRequest struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

type pageLabelPatchRequest struct {
	Name  Optional[string] `json:"name"`
	Color Optional[string] `json:"color"`
}

func listPageLabels(deps Deps) readOperation[[]models.PageLabel] {
	return func(r *http.Request) ([]models.PageLabel, error) {
		_, workspaceID, err := requirePageWorkspace(r, deps, models.PermissionPageView)
		if err != nil {
			return nil, err
		}
		labels, err := deps.PageLabels.List(workspaceID)
		if err != nil {
			return nil, internalError(err)
		}
		if labels == nil {
			labels = []models.PageLabel{}
		}
		return labels, nil
	}
}

func getPageLabel(deps Deps) readOperation[models.PageLabel] {
	return func(r *http.Request) (models.PageLabel, error) {
		_, workspaceID, err := requirePageWorkspace(r, deps, models.PermissionPageView)
		if err != nil {
			return models.PageLabel{}, err
		}
		labelID, err := pathID(r, "label_id")
		if err != nil {
			return models.PageLabel{}, err
		}
		label, err := deps.PageLabels.Get(workspaceID, labelID)
		if err != nil {
			return models.PageLabel{}, pageLabelError(err)
		}
		return *label, nil
	}
}

func createPageLabel(deps Deps) jsonOperation[pageLabelCreateRequest, models.PageLabel] {
	return func(r *http.Request, input pageLabelCreateRequest) (models.PageLabel, error) {
		user, workspaceID, err := requirePageWorkspace(r, deps, models.PermissionPageEdit)
		if err != nil {
			return models.PageLabel{}, err
		}
		label, err := deps.PageLabels.Create(workspaceID, input.Name, input.Color, auditActor(r, user))
		if err != nil {
			return models.PageLabel{}, pageLabelError(err)
		}
		return *label, nil
	}
}

func updatePageLabel(deps Deps) jsonOperation[pageLabelPatchRequest, models.PageLabel] {
	return func(r *http.Request, input pageLabelPatchRequest) (models.PageLabel, error) {
		if input.Name.Null || input.Color.Null {
			return models.PageLabel{}, newError(http.StatusBadRequest, "invalid_request", "Page label fields cannot be null")
		}
		user, workspaceID, err := requirePageWorkspace(r, deps, models.PermissionPageEdit)
		if err != nil {
			return models.PageLabel{}, err
		}
		labelID, err := pathID(r, "label_id")
		if err != nil {
			return models.PageLabel{}, err
		}
		label, err := deps.PageLabels.Update(workspaceID, labelID, services.PageLabelUpdate{
			Name: optionalValue(input.Name), Color: optionalValue(input.Color),
		}, auditActor(r, user))
		if err != nil {
			return models.PageLabel{}, pageLabelError(err)
		}
		return *label, nil
	}
}

func deletePageLabel(deps Deps) commandOperation {
	return func(r *http.Request) error {
		user, workspaceID, err := requirePageWorkspace(r, deps, models.PermissionPageEdit)
		if err != nil {
			return err
		}
		labelID, err := pathID(r, "label_id")
		if err != nil {
			return err
		}
		_, err = deps.PageLabels.Delete(workspaceID, labelID, auditActor(r, user))
		return pageLabelError(err)
	}
}

func listLabelsForPage(deps Deps) readOperation[[]models.PageLabel] {
	return func(r *http.Request) ([]models.PageLabel, error) {
		_, workspaceID, pageID, err := requirePage(r, deps, services.PageOpView)
		if err != nil {
			return nil, err
		}
		labels, err := deps.PageLabels.ListForPage(pageID)
		if err != nil {
			return nil, pageLabelError(err)
		}
		for _, label := range labels {
			if label.WorkspaceID != workspaceID {
				return nil, internalError(errors.New("page label assignment crosses workspaces"))
			}
		}
		if labels == nil {
			labels = []models.PageLabel{}
		}
		return labels, nil
	}
}

func setLabelsForPage(deps Deps) jsonOperation[labelIDsRequest, []models.PageLabel] {
	return func(r *http.Request, input labelIDsRequest) ([]models.PageLabel, error) {
		_, workspaceID, pageID, err := requirePage(r, deps, services.PageOpEdit)
		if err != nil {
			return nil, err
		}
		labels, err := deps.PageLabels.SetForPage(workspaceID, pageID, input.LabelIDs)
		if err != nil {
			return nil, pageLabelError(err)
		}
		return labels, nil
	}
}

func addLabelToPage(deps Deps) jsonOperation[labelIDRequest, []models.PageLabel] {
	return func(r *http.Request, input labelIDRequest) ([]models.PageLabel, error) {
		_, workspaceID, pageID, err := requirePage(r, deps, services.PageOpEdit)
		if err != nil {
			return nil, err
		}
		labels, err := deps.PageLabels.AddToPage(workspaceID, pageID, input.LabelID)
		if err != nil {
			return nil, pageLabelError(err)
		}
		return labels, nil
	}
}

func removeLabelFromPage(deps Deps) commandOperation {
	return func(r *http.Request) error {
		_, _, pageID, err := requirePage(r, deps, services.PageOpEdit)
		if err != nil {
			return err
		}
		labelID, err := pathID(r, "label_id")
		if err != nil {
			return err
		}
		return pageLabelError(deps.PageLabels.RemoveFromPage(pageID, labelID))
	}
}

func requirePageWorkspace(r *http.Request, deps Deps, permission string) (*models.User, int, error) {
	user, workspaceID, err := principalAndWorkspace(r)
	if err != nil {
		return nil, 0, err
	}
	allowed, err := deps.PageAccess.HasWorkspacePermissionFor(user.ID, workspaceID, permission)
	if err != nil {
		return nil, 0, internalError(err)
	}
	if !allowed {
		return nil, 0, newError(http.StatusNotFound, "not_found", "Page labels were not found")
	}
	return user, workspaceID, nil
}

func requirePage(r *http.Request, deps Deps, operation string) (user *models.User, workspaceID, pageID int, err error) {
	user, workspaceID, err = principalAndWorkspace(r)
	if err != nil {
		return nil, 0, 0, err
	}
	pageID, err = pathID(r, "page_id")
	if err != nil {
		return nil, 0, 0, err
	}
	page, err := deps.Pages.GetByID(pageID)
	if errors.Is(err, repository.ErrNotFound) || errors.Is(err, services.ErrPageNotFound) || (err == nil && (page == nil || page.WorkspaceID != workspaceID)) {
		return nil, 0, 0, newError(http.StatusNotFound, "not_found", "Page was not found")
	}
	if err != nil {
		return nil, 0, 0, internalError(err)
	}
	allowed, err := deps.PageAccess.Can(user.ID, workspaceID, pageID, operation)
	if err != nil {
		return nil, 0, 0, internalError(err)
	}
	if !allowed {
		return nil, 0, 0, newError(http.StatusNotFound, "not_found", "Page was not found")
	}
	return user, workspaceID, pageID, nil
}

func pageLabelError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, repository.ErrNotFound), errors.Is(err, services.ErrPageLabelWorkspaceMismatch):
		return newError(http.StatusNotFound, "not_found", "Page label was not found")
	case errors.Is(err, services.ErrPageLabelNameRequired), errors.Is(err, services.ErrPageLabelIDRequired):
		return newError(http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, repository.ErrDuplicateEntry):
		return newError(http.StatusConflict, "conflict", "Page label already exists or is already assigned")
	default:
		return internalError(err)
	}
}
