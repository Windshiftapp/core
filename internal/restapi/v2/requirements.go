package v2

import (
	"encoding/json"
	"errors"
	"net/http"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

func registerRequirementRoutes(builder *routeBuilder, deps Deps) {
	requirements := deps.RequirementApplication
	builder.Read("/workspaces/{workspace_id}/requirements", AuthAuthenticated, []string{"pages:read"}, listRequirements(requirements))
	builder.JSON(http.MethodPost, "/workspaces/{workspace_id}/requirements", http.StatusCreated, false, AuthAuthenticated, []string{"pages:write"}, createRequirement(requirements))
	builder.Read("/workspaces/{workspace_id}/requirements/{requirement_number}", AuthAuthenticated, []string{"pages:read"}, getRequirement(requirements))
	builder.JSON(http.MethodPatch, "/workspaces/{workspace_id}/requirements/{requirement_number}", http.StatusOK, true, AuthAuthenticated, []string{"pages:write"}, updateRequirement(requirements))
	builder.Read("/workspaces/{workspace_id}/requirements/{requirement_number}/history", AuthAuthenticated, []string{"pages:read"}, listRequirementHistory(requirements))
	builder.JSON(http.MethodPost, "/workspaces/{workspace_id}/pages/{page_id}/promote-to-requirement", http.StatusCreated, false, AuthAuthenticated, []string{"pages:write"}, promotePageToRequirement(requirements))
}

type createRequirementRequest struct {
	ParentID        *int            `json:"parent_id"`
	Title           string          `json:"title"`
	Content         string          `json:"content"`
	Metadata        json.RawMessage `json:"metadata"`
	RequirementType string          `json:"requirement_type"`
	Status          string          `json:"status"`
	OwnerID         *int            `json:"owner_id"`
}

type patchRequirementRequest struct {
	RequirementType Optional[string] `json:"requirement_type"`
	Status          Optional[string] `json:"status"`
	OwnerID         Optional[int]    `json:"owner_id"`
}

type promoteRequirementRequest struct {
	RequirementType string `json:"requirement_type"`
	Status          string `json:"status"`
	OwnerID         *int   `json:"owner_id"`
}

func listRequirements(requirements requirementApplication) readOperation[[]services.RequirementView] {
	return func(r *http.Request) ([]services.RequirementView, error) {
		user, workspaceID, err := principalAndWorkspace(r)
		if err != nil {
			return nil, err
		}
		limit, err := parsePositiveInt(r, "limit", 50, 100)
		if err != nil {
			return nil, err
		}
		offset, err := parseNonNegativeQueryInt(r, "offset", 0)
		if err != nil {
			return nil, err
		}
		ownerID, err := optionalPositiveQuery(r, "owner_id")
		if err != nil {
			return nil, err
		}
		result, err := requirements.List(user.ID, workspaceID, services.RequirementListFilter{
			Query:           r.URL.Query().Get("q"),
			RequirementType: r.URL.Query().Get("requirement_type"),
			Status:          r.URL.Query().Get("status"),
			OwnerID:         ownerID,
			Limit:           limit,
			Offset:          offset,
		})
		return result, requirementError(err)
	}
}

func createRequirement(requirements requirementApplication) jsonOperation[createRequirementRequest, services.RequirementView] {
	return func(r *http.Request, input createRequirementRequest) (services.RequirementView, error) {
		user, workspaceID, err := principalAndWorkspace(r)
		if err != nil {
			return services.RequirementView{}, err
		}
		view, err := requirements.Create(auditActor(r, user), services.CreateRequirementInput{
			WorkspaceID:     workspaceID,
			ParentID:        input.ParentID,
			Title:           input.Title,
			Content:         input.Content,
			Metadata:        input.Metadata,
			RequirementType: input.RequirementType,
			Status:          input.Status,
			OwnerID:         input.OwnerID,
		})
		return derefRequirementView(view), requirementError(err)
	}
}

func getRequirement(requirements requirementApplication) readOperation[services.RequirementView] {
	return func(r *http.Request) (services.RequirementView, error) {
		user, workspaceID, number, err := requirementTarget(r)
		if err != nil {
			return services.RequirementView{}, err
		}
		view, err := requirements.Get(user.ID, workspaceID, number)
		return derefRequirementView(view), requirementError(err)
	}
}

func updateRequirement(requirements requirementApplication) jsonOperation[patchRequirementRequest, services.RequirementView] {
	return func(r *http.Request, input patchRequirementRequest) (services.RequirementView, error) {
		if input.RequirementType.Null || input.Status.Null {
			return services.RequirementView{}, newError(http.StatusBadRequest, "invalid_request", "Requirement fields cannot be null")
		}
		user, workspaceID, number, err := requirementTarget(r)
		if err != nil {
			return services.RequirementView{}, err
		}
		patch := services.RequirementUpdateInput{
			RequirementType: optionalValue(input.RequirementType),
			Status:          optionalValue(input.Status),
		}
		if input.OwnerID.Set {
			patch.OwnerIDSet = true
			if !input.OwnerID.Null {
				patch.OwnerID = &input.OwnerID.Value
			}
		}
		view, err := requirements.Update(auditActor(r, user), workspaceID, number, patch)
		return derefRequirementView(view), requirementError(err)
	}
}

func listRequirementHistory(requirements requirementApplication) readOperation[[]models.RequirementHistoryEntry] {
	return func(r *http.Request) ([]models.RequirementHistoryEntry, error) {
		user, workspaceID, number, err := requirementTarget(r)
		if err != nil {
			return nil, err
		}
		result, err := requirements.ListHistory(user.ID, workspaceID, number)
		return result, requirementError(err)
	}
}

func promotePageToRequirement(requirements requirementApplication) jsonOperation[promoteRequirementRequest, services.RequirementView] {
	return func(r *http.Request, input promoteRequirementRequest) (services.RequirementView, error) {
		user, workspaceID, err := principalAndWorkspace(r)
		if err != nil {
			return services.RequirementView{}, err
		}
		pageID, err := pathID(r, "page_id")
		if err != nil {
			return services.RequirementView{}, err
		}
		view, err := requirements.Promote(auditActor(r, user), workspaceID, pageID, input.RequirementType, input.Status, input.OwnerID)
		return derefRequirementView(view), requirementError(err)
	}
}

func requirementTarget(r *http.Request) (user *models.User, workspaceID, number int, err error) {
	user, workspaceID, err = principalAndWorkspace(r)
	if err != nil {
		return nil, 0, 0, err
	}
	number, err = pathID(r, "requirement_number")
	return user, workspaceID, number, err
}

func derefRequirementView(view *services.RequirementView) services.RequirementView {
	if view == nil {
		return services.RequirementView{}
	}
	return *view
}

func requirementError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, services.ErrRequirementNotFound), errors.Is(err, services.ErrPageNotFound), errors.Is(err, repository.ErrNotFound):
		return newError(http.StatusNotFound, "not_found", "Requirement was not found")
	case errors.Is(err, services.ErrRequirementAlreadyExists):
		return newError(http.StatusConflict, "conflict", err.Error())
	case errors.Is(err, services.ErrRequirementTypeInvalid), errors.Is(err, services.ErrRequirementStatusInvalid), errors.Is(err, services.ErrRequirementNoChanges), errors.Is(err, services.ErrPageTitleRequired), errors.Is(err, services.ErrPageMetadataInvalid):
		return newError(http.StatusBadRequest, "invalid_request", err.Error())
	default:
		return internalError(err)
	}
}
