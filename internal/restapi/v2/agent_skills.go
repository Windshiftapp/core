package v2

import (
	"context"
	"errors"
	"net/http"

	"windshift/internal/contextkeys"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

type agentSkillReader interface {
	List(context.Context, int, int, int) ([]*models.WorkspaceAgentSkill, error)
	Get(context.Context, int, int, int, int) (*models.WorkspaceAgentSkill, error)
	Create(context.Context, services.AuditActor, int, services.AgentSkillInput) (*models.WorkspaceAgentSkill, error)
	Patch(context.Context, services.AuditActor, int, int, services.AgentSkillPatch) (*models.WorkspaceAgentSkill, error)
	Delete(context.Context, services.AuditActor, int, int) error
}

func registerAgentSkillRoutes(builder *routeBuilder, skills agentSkillReader) {
	builder.Read("/workspaces/{workspace_id}/agent-skills", AuthAuthenticated, []string{"agent-skills:read"}, listAgentSkills(skills))
	builder.JSON(http.MethodPost, "/workspaces/{workspace_id}/agent-skills", http.StatusCreated, false, AuthAuthenticated, []string{"agent-skills:write"}, createAgentSkill(skills))
	builder.Read("/workspaces/{workspace_id}/agent-skills/{skill_id}", AuthAuthenticated, []string{"agent-skills:read"}, getAgentSkill(skills))
	builder.JSON(http.MethodPatch, "/workspaces/{workspace_id}/agent-skills/{skill_id}", http.StatusOK, true, AuthAuthenticated, []string{"agent-skills:write"}, patchAgentSkill(skills))
	builder.Command(http.MethodDelete, "/workspaces/{workspace_id}/agent-skills/{skill_id}", AuthAuthenticated, []string{"agent-skills:write"}, deleteAgentSkill(skills))
}

type agentSkillCreateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Body        string `json:"body"`
	Enabled     *bool  `json:"enabled"`
	PageIDs     []int  `json:"page_ids"`
}

type agentSkillPatchRequest struct {
	Name        Optional[string] `json:"name"`
	Description Optional[string] `json:"description"`
	Body        Optional[string] `json:"body"`
	Enabled     Optional[bool]   `json:"enabled"`
	PageIDs     Optional[[]int]  `json:"page_ids"`
}

func listAgentSkills(skills agentSkillReader) readOperation[[]*models.WorkspaceAgentSkill] {
	return func(r *http.Request) ([]*models.WorkspaceAgentSkill, error) {
		user, tokenID, workspaceID, err := agentSkillAccess(r)
		if err != nil {
			return nil, err
		}
		items, err := skills.List(r.Context(), user.ID, tokenID, workspaceID)
		if err != nil {
			return nil, agentSkillError(err)
		}
		return items, nil
	}
}

func getAgentSkill(skills agentSkillReader) readOperation[models.WorkspaceAgentSkill] {
	return func(r *http.Request) (models.WorkspaceAgentSkill, error) {
		user, tokenID, workspaceID, err := agentSkillAccess(r)
		if err != nil {
			return models.WorkspaceAgentSkill{}, err
		}
		skillID, err := pathID(r, "skill_id")
		if err != nil {
			return models.WorkspaceAgentSkill{}, err
		}
		skill, err := skills.Get(r.Context(), user.ID, tokenID, workspaceID, skillID)
		if err != nil {
			return models.WorkspaceAgentSkill{}, agentSkillError(err)
		}
		return *skill, nil
	}
}

func createAgentSkill(skills agentSkillReader) jsonOperation[agentSkillCreateRequest, models.WorkspaceAgentSkill] {
	return func(r *http.Request, input agentSkillCreateRequest) (models.WorkspaceAgentSkill, error) {
		user, _, workspaceID, err := agentSkillAccess(r)
		if err != nil {
			return models.WorkspaceAgentSkill{}, err
		}
		enabled := true
		if input.Enabled != nil {
			enabled = *input.Enabled
		}
		item, err := skills.Create(r.Context(), auditActor(r, user), workspaceID, services.AgentSkillInput{
			Name: input.Name, Description: input.Description, Body: input.Body, Enabled: enabled, PageIDs: input.PageIDs,
		})
		if err != nil {
			return models.WorkspaceAgentSkill{}, agentSkillError(err)
		}
		return *item, nil
	}
}

func patchAgentSkill(skills agentSkillReader) jsonOperation[agentSkillPatchRequest, models.WorkspaceAgentSkill] {
	return func(r *http.Request, input agentSkillPatchRequest) (models.WorkspaceAgentSkill, error) {
		user, _, workspaceID, err := agentSkillAccess(r)
		if err != nil {
			return models.WorkspaceAgentSkill{}, err
		}
		skillID, err := pathID(r, "skill_id")
		if err != nil {
			return models.WorkspaceAgentSkill{}, err
		}
		if input.PageIDs.Set && input.PageIDs.Null {
			return models.WorkspaceAgentSkill{}, newError(http.StatusBadRequest, "invalid_request", "page_ids cannot be null")
		}
		var pageIDs *[]int
		if input.PageIDs.Set {
			pageIDs = &input.PageIDs.Value
		}
		item, err := skills.Patch(r.Context(), auditActor(r, user), workspaceID, skillID, services.AgentSkillPatch{
			Name: optionalValue(input.Name), Description: optionalValue(input.Description),
			Body: optionalValue(input.Body), Enabled: optionalValue(input.Enabled), PageIDs: pageIDs,
		})
		if err != nil {
			return models.WorkspaceAgentSkill{}, agentSkillError(err)
		}
		return *item, nil
	}
}

func deleteAgentSkill(skills agentSkillReader) commandOperation {
	return func(r *http.Request) error {
		user, _, workspaceID, err := agentSkillAccess(r)
		if err != nil {
			return err
		}
		skillID, err := pathID(r, "skill_id")
		if err != nil {
			return err
		}
		return agentSkillError(skills.Delete(r.Context(), auditActor(r, user), workspaceID, skillID))
	}
}

func agentSkillAccess(r *http.Request) (user *models.User, tokenID, workspaceID int, err error) {
	user, err = principal(r)
	if err != nil {
		return nil, 0, 0, err
	}
	workspaceID, err = pathID(r, "workspace_id")
	if err != nil {
		return nil, 0, 0, err
	}
	token, ok := r.Context().Value(contextkeys.APIToken).(*models.APIToken)
	if !ok || token == nil {
		return user, 0, workspaceID, nil
	}
	return user, token.ID, workspaceID, nil
}

func agentSkillError(err error) error {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		return newError(http.StatusNotFound, "not_found", "Agent skill was not found")
	case errors.Is(err, services.ErrAgentSkillActivationTooLarge):
		return newError(http.StatusUnprocessableEntity, "skill_activation_too_large", "The saved skill exceeds the supported activation budget")
	case errors.Is(err, services.ErrAgentSkillForbidden):
		return newError(http.StatusForbidden, "forbidden", "Workspace administration permission is required")
	case errors.Is(err, services.ErrAgentSkillValidation), errors.Is(err, repository.ErrSkillPageNotInWorkspace):
		return newError(http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, repository.ErrSkillDuplicateName):
		return newError(http.StatusConflict, "conflict", err.Error())
	default:
		return internalError(err)
	}
}
