package v2

import (
	"errors"
	"net/http"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

func registerPageDiagramRoutes(builder *routeBuilder, deps Deps) {
	const collection = "/workspaces/{workspace_id}/pages/{page_id}/diagrams"
	const detail = collection + "/{attachment_id}"
	builder.Read(collection, AuthAuthenticated, []string{"pages:read"}, listPageDiagrams(deps))
	builder.JSON(http.MethodPost, collection, http.StatusCreated, false, AuthAuthenticated, []string{"pages:write"}, createPageDiagram(deps))
	builder.Read(detail, AuthAuthenticated, []string{"pages:read"}, getPageDiagram(deps))
	builder.JSON(http.MethodPatch, detail, http.StatusOK, true, AuthAuthenticated, []string{"pages:write"}, updatePageDiagram(deps))
}

func listPageDiagrams(deps Deps) readOperation[[]services.PageDiagram] {
	return func(r *http.Request) ([]services.PageDiagram, error) {
		user, pageID, err := pageDiagramTarget(r, deps)
		if err != nil {
			return nil, err
		}
		diagrams, err := deps.PageDiagrams.List(auditActor(r, user), pageID)
		if err != nil {
			return nil, pageDiagramError(err)
		}
		if diagrams == nil {
			diagrams = []services.PageDiagram{}
		}
		return diagrams, nil
	}
}

func createPageDiagram(deps Deps) jsonOperation[services.CreatePageDiagramInput, services.PageDiagram] {
	return func(r *http.Request, input services.CreatePageDiagramInput) (services.PageDiagram, error) {
		user, pageID, err := pageDiagramTarget(r, deps)
		if err != nil {
			return services.PageDiagram{}, err
		}
		input.PageID = pageID
		diagram, err := deps.PageDiagrams.Create(auditActor(r, user), input)
		if err != nil {
			return services.PageDiagram{}, pageDiagramError(err)
		}
		return *diagram, nil
	}
}

func getPageDiagram(deps Deps) readOperation[services.PageDiagram] {
	return func(r *http.Request) (services.PageDiagram, error) {
		user, pageID, err := pageDiagramTarget(r, deps)
		if err != nil {
			return services.PageDiagram{}, err
		}
		attachmentID, err := pathID(r, "attachment_id")
		if err != nil {
			return services.PageDiagram{}, err
		}
		diagram, err := deps.PageDiagrams.Get(auditActor(r, user), pageID, attachmentID)
		if err != nil {
			return services.PageDiagram{}, pageDiagramError(err)
		}
		return *diagram, nil
	}
}

func updatePageDiagram(deps Deps) jsonOperation[services.UpdatePageDiagramInput, services.PageDiagram] {
	return func(r *http.Request, input services.UpdatePageDiagramInput) (services.PageDiagram, error) {
		user, pageID, err := pageDiagramTarget(r, deps)
		if err != nil {
			return services.PageDiagram{}, err
		}
		attachmentID, err := pathID(r, "attachment_id")
		if err != nil {
			return services.PageDiagram{}, err
		}
		input.PageID = pageID
		input.AttachmentID = attachmentID
		diagram, err := deps.PageDiagrams.Update(auditActor(r, user), input)
		if err != nil {
			return services.PageDiagram{}, pageDiagramError(err)
		}
		return *diagram, nil
	}
}

func pageDiagramTarget(r *http.Request, deps Deps) (*models.User, int, error) {
	user, workspaceID, err := principalAndWorkspace(r)
	if err != nil {
		return nil, 0, err
	}
	pageID, err := pathID(r, "page_id")
	if err != nil {
		return nil, 0, err
	}
	page, err := deps.Pages.GetByID(pageID)
	if errors.Is(err, repository.ErrNotFound) || (err == nil && (page == nil || page.WorkspaceID != workspaceID)) {
		return nil, 0, newError(http.StatusNotFound, "not_found", "Page was not found")
	}
	if err != nil {
		return nil, 0, internalError(err)
	}
	return user, pageID, nil
}

func pageDiagramError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, services.ErrPageDiagramNotFound),
		errors.Is(err, services.ErrPageNotFound),
		errors.Is(err, services.ErrPageAttachmentUploadNotFound),
		errors.Is(err, repository.ErrNotFound):
		return newError(http.StatusNotFound, "not_found", "Page diagram was not found")
	case errors.Is(err, services.ErrPageContentConflict),
		errors.Is(err, services.ErrPageDiagramReferenceConflict):
		return newError(http.StatusConflict, "conflict", err.Error())
	case errors.Is(err, services.ErrDiagramPayloadTooLarge):
		return newError(http.StatusRequestEntityTooLarge, "payload_too_large", err.Error())
	case errors.Is(err, services.ErrPageDiagramNameRequired),
		errors.Is(err, services.ErrPageDiagramPlacementInvalid),
		errors.Is(err, services.ErrDiagramPayloadRequired),
		errors.Is(err, services.ErrDiagramPayloadConflict),
		errors.Is(err, services.ErrDiagramPayloadInvalid),
		errors.Is(err, services.ErrPageAttachmentUploadInvalid):
		return newError(http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, services.ErrPageAttachmentUploadDisabled):
		return newError(http.StatusServiceUnavailable, "service_unavailable", "Page diagram storage is disabled")
	default:
		return internalError(err)
	}
}
