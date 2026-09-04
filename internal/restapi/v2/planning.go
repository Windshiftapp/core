package v2

import (
	"errors"
	"net/http"
	"strings"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

func registerPlanningRoutes(builder *routeBuilder, planning planningApplication) {
	builder.Page("/milestones", AuthAuthenticated, []string{"milestones:read"}, listMilestones(planning))
	builder.JSON(http.MethodPost, "/milestones", http.StatusCreated, false, AuthAuthenticated, []string{"milestones:write"}, createMilestone(planning))
	builder.Read("/milestones/{milestone_id}", AuthAuthenticated, []string{"milestones:read"}, getMilestone(planning))
	builder.JSON(http.MethodPatch, "/milestones/{milestone_id}", http.StatusOK, true, AuthAuthenticated, []string{"milestones:write"}, patchMilestone(planning))
	builder.Command(http.MethodDelete, "/milestones/{milestone_id}", AuthAuthenticated, []string{"milestones:delete"}, deleteMilestone(planning))
	builder.JSON(http.MethodPost, "/milestones/{milestone_id}/release", http.StatusOK, false, AuthAuthenticated, []string{"milestones:write"}, releaseMilestone(planning))
	builder.Read("/milestones/{milestone_id}/progress", AuthAuthenticated, []string{"milestones:read"}, milestoneProgress(planning))
	builder.Read("/milestones/{milestone_id}/test-statistics", AuthAuthenticated, []string{"milestones:read"}, milestoneTestStatistics(planning))
	builder.Read("/milestones/test-statistics", AuthAuthenticated, []string{"milestones:read"}, milestoneTestStatisticsBatch(planning))
	builder.JSON(http.MethodPost, "/milestones/reorder", http.StatusOK, false, AuthAuthenticated, []string{"milestones:write"}, reorderMilestones(planning))

	builder.Page("/iterations", AuthAuthenticated, []string{"iterations:read"}, listIterations(planning))
	builder.JSON(http.MethodPost, "/iterations", http.StatusCreated, false, AuthAuthenticated, []string{"iterations:write"}, createIteration(planning))
	builder.Read("/iterations/{iteration_id}", AuthAuthenticated, []string{"iterations:read"}, getIteration(planning))
	builder.JSON(http.MethodPatch, "/iterations/{iteration_id}", http.StatusOK, true, AuthAuthenticated, []string{"iterations:write"}, patchIteration(planning))
	builder.Command(http.MethodDelete, "/iterations/{iteration_id}", AuthAuthenticated, []string{"iterations:delete"}, deleteIteration(planning))
	builder.JSON(http.MethodPost, "/iterations/{iteration_id}/complete", http.StatusOK, false, AuthAuthenticated, []string{"iterations:write"}, completeIteration(planning))
	builder.Read("/iterations/{iteration_id}/progress", AuthAuthenticated, []string{"iterations:read"}, iterationProgress(planning))
	builder.Read("/iterations/{iteration_id}/burndown", AuthAuthenticated, []string{"iterations:read"}, iterationBurndown(planning))
	builder.Read("/iterations/progress", AuthAuthenticated, []string{"iterations:read"}, iterationProgressBatch(planning))
}

func milestoneTestStatistics(planning planningApplication) readOperation[*services.MilestoneTestStats] {
	return func(r *http.Request) (*services.MilestoneTestStats, error) {
		user, err := principal(r)
		if err != nil {
			return nil, err
		}
		id, err := pathID(r, "milestone_id")
		if err != nil {
			return nil, err
		}
		result, err := planning.GetMilestoneTestStatistics(user.ID, id)
		return result, planningError(err)
	}
}

func milestoneTestStatisticsBatch(planning planningApplication) readOperation[map[int]*services.MilestoneTestStats] {
	return func(r *http.Request) (map[int]*services.MilestoneTestStats, error) {
		user, err := principal(r)
		if err != nil {
			return nil, err
		}
		ids, err := itemIDList(r.URL.Query().Get("ids"))
		if err != nil {
			return nil, err
		}
		if len(ids) > maxItemBatch {
			return nil, newError(http.StatusBadRequest, "invalid_request", "ids supports at most 500 values")
		}
		result, err := planning.GetMilestoneTestStatisticsBatch(user.ID, ids)
		return result, planningError(err)
	}
}

func iterationProgressBatch(planning planningApplication) readOperation[map[int]*services.IterationProgressReport] {
	return func(r *http.Request) (map[int]*services.IterationProgressReport, error) {
		user, err := principal(r)
		if err != nil {
			return nil, err
		}
		ids, err := itemIDList(r.URL.Query().Get("ids"))
		if err != nil {
			return nil, err
		}
		if len(ids) > maxItemBatch {
			return nil, newError(http.StatusBadRequest, "invalid_request", "ids supports at most 500 values")
		}
		result, err := planning.GetIterationProgressBatch(user.ID, ids)
		return result, planningError(err)
	}
}

type planningScopeRequest struct {
	Scope       string `json:"scope"`
	WorkspaceID *int   `json:"workspace_id"`
}

type milestoneCreateRequest struct {
	planningScopeRequest
	Name        string  `json:"name"`
	Description string  `json:"description"`
	TargetDate  *string `json:"target_date"`
	Status      string  `json:"status"`
	CategoryID  *int    `json:"category_id"`
}

type milestoneReorderRequest struct {
	planningScopeRequest
	OrderedIDs []int `json:"ordered_ids"`
	CategoryID *int  `json:"category_id"`
}

type releaseMilestoneRequest struct {
	ConnectionID    int    `json:"connection_id"`
	RepositoryID    int    `json:"repository_id"`
	Repository      string `json:"repository"`
	TagName         string `json:"tag_name"`
	Name            string `json:"name"`
	Body            string `json:"body"`
	IsDraft         bool   `json:"is_draft"`
	IsPrerelease    bool   `json:"is_prerelease"`
	TargetCommitish string `json:"target_commitish"`
}

type iterationCreateRequest struct {
	planningScopeRequest
	Name        string `json:"name"`
	Description string `json:"description"`
	StartDate   string `json:"start_date"`
	EndDate     string `json:"end_date"`
	Status      string `json:"status"`
	TypeID      *int   `json:"type_id"`
}

type completeIterationRequest struct {
	TargetIterationID *int `json:"move_incomplete_to_iteration_id"`
}

func listMilestones(planning planningApplication) pageOperation[models.Milestone] {
	return func(r *http.Request) ([]models.Milestone, Pagination, int, error) {
		user, err := principal(r)
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		page, err := ParsePage(r)
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		workspaceID, err := queryPlanningScope(r)
		if err != nil {
			return nil, page, 0, err
		}
		categoryID, err := optionalPositiveQueryID(r, "category_id")
		if err != nil {
			return nil, page, 0, err
		}
		rows, total, err := planning.ListMilestones(user.ID, services.MilestoneListParams{
			Limit: page.PageSize, Offset: page.Offset, WorkspaceID: workspaceID, CategoryID: categoryID,
			Status: r.URL.Query().Get("status"), SortBy: r.URL.Query().Get("sort_by"), SortOrder: r.URL.Query().Get("sort_order"),
		})
		return mapMilestones(rows), page, total, planningError(err)
	}
}

func getMilestone(planning planningApplication) readOperation[models.Milestone] {
	return func(r *http.Request) (models.Milestone, error) {
		user, id, err := planningTarget(r, "milestone_id")
		if err != nil {
			return models.Milestone{}, err
		}
		result, err := planning.GetMilestone(user.ID, id)
		return milestoneModel(result), planningError(err)
	}
}

func createMilestone(planning planningApplication) jsonOperation[milestoneCreateRequest, models.Milestone] {
	return func(r *http.Request, input milestoneCreateRequest) (models.Milestone, error) {
		user, err := principal(r)
		if err != nil {
			return models.Milestone{}, err
		}
		global, workspaceID, err := bodyPlanningScope(input.planningScopeRequest)
		if err != nil {
			return models.Milestone{}, err
		}
		result, err := planning.CreateMilestone(user.ID, auditActor(r, user), services.CreateMilestoneParams{
			Name: input.Name, Description: input.Description, TargetDate: input.TargetDate, Status: input.Status,
			CategoryID: input.CategoryID, IsGlobal: global, WorkspaceID: workspaceID,
		})
		return milestoneModel(result), planningError(err)
	}
}

func patchMilestone(planning planningApplication) jsonOperation[models.MilestonePatch, models.Milestone] {
	return func(r *http.Request, patch models.MilestonePatch) (models.Milestone, error) {
		user, id, err := planningTarget(r, "milestone_id")
		if err != nil {
			return models.Milestone{}, err
		}
		existing, err := planning.GetMilestone(user.ID, id)
		if err != nil {
			return models.Milestone{}, planningError(err)
		}
		merged := patch.Apply(milestoneModel(existing))
		result, err := planning.UpdateMilestone(user.ID, auditActor(r, user), services.UpdateMilestoneParams{
			ID: id, Name: merged.Name, Description: merged.Description, TargetDate: merged.TargetDate,
			Status: merged.Status, CategoryID: merged.CategoryID,
		})
		return milestoneModel(result), planningError(err)
	}
}

func deleteMilestone(planning planningApplication) commandOperation {
	return func(r *http.Request) error {
		user, id, err := planningTarget(r, "milestone_id")
		if err != nil {
			return err
		}
		return planningError(planning.DeleteMilestone(user.ID, auditActor(r, user), id))
	}
}

func milestoneProgress(planning planningApplication) readOperation[*services.MilestoneProgressReport] {
	return func(r *http.Request) (*services.MilestoneProgressReport, error) {
		user, id, err := planningTarget(r, "milestone_id")
		if err != nil {
			return nil, err
		}
		result, err := planning.GetMilestoneProgress(user.ID, id)
		return result, planningError(err)
	}
}

func reorderMilestones(planning planningApplication) jsonOperation[milestoneReorderRequest, map[string]bool] {
	return func(r *http.Request, input milestoneReorderRequest) (map[string]bool, error) {
		user, err := principal(r)
		if err != nil {
			return nil, err
		}
		global, workspaceID, err := bodyPlanningScope(input.planningScopeRequest)
		if err != nil {
			return nil, err
		}
		if len(input.OrderedIDs) == 0 {
			return nil, newError(http.StatusBadRequest, "invalid_request", "ordered_ids is required")
		}
		err = planning.ReorderMilestones(user.ID, auditActor(r, user), services.MilestoneScope{IsGlobal: global, WorkspaceID: workspaceID, CategoryID: input.CategoryID}, input.OrderedIDs)
		return map[string]bool{"ok": err == nil}, planningError(err)
	}
}

func releaseMilestone(planning planningApplication) jsonOperation[releaseMilestoneRequest, models.Milestone] {
	return func(r *http.Request, input releaseMilestoneRequest) (models.Milestone, error) {
		user, id, err := planningTarget(r, "milestone_id")
		if err != nil {
			return models.Milestone{}, err
		}
		result, err := planning.ReleaseMilestone(r.Context(), user.ID, auditActor(r, user), id, services.ReleaseMilestoneInput{
			ConnectionID: input.ConnectionID, RepositoryID: input.RepositoryID, Repository: input.Repository,
			IdempotencyKey: r.Header.Get("Idempotency-Key"), TagName: input.TagName, Name: input.Name, Body: input.Body,
			IsDraft: input.IsDraft, IsPrerelease: input.IsPrerelease, TargetCommitish: input.TargetCommitish,
		})
		return milestoneModel(result), planningError(err)
	}
}

func listIterations(planning planningApplication) pageOperation[models.Iteration] {
	return func(r *http.Request) ([]models.Iteration, Pagination, int, error) {
		user, err := principal(r)
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		page, err := ParsePage(r)
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		workspaceID, err := queryPlanningScope(r)
		if err != nil {
			return nil, page, 0, err
		}
		typeID, err := optionalPositiveQueryID(r, "type_id")
		if err != nil {
			return nil, page, 0, err
		}
		rows, total, err := planning.ListIterations(user.ID, services.IterationListParams{
			Limit: page.PageSize, Offset: page.Offset, WorkspaceID: workspaceID, TypeID: typeID, Status: r.URL.Query().Get("status"),
		})
		return mapIterations(rows), page, total, planningError(err)
	}
}

func getIteration(planning planningApplication) readOperation[models.Iteration] {
	return func(r *http.Request) (models.Iteration, error) {
		user, id, err := planningTarget(r, "iteration_id")
		if err != nil {
			return models.Iteration{}, err
		}
		result, err := planning.GetIteration(user.ID, id)
		return iterationModel(result), planningError(err)
	}
}

func createIteration(planning planningApplication) jsonOperation[iterationCreateRequest, models.Iteration] {
	return func(r *http.Request, input iterationCreateRequest) (models.Iteration, error) {
		user, err := principal(r)
		if err != nil {
			return models.Iteration{}, err
		}
		global, workspaceID, err := bodyPlanningScope(input.planningScopeRequest)
		if err != nil {
			return models.Iteration{}, err
		}
		result, err := planning.CreateIteration(user.ID, auditActor(r, user), services.CreateIterationParams{
			Name: input.Name, Description: input.Description, StartDate: input.StartDate, EndDate: input.EndDate,
			Status: input.Status, TypeID: input.TypeID, IsGlobal: global, WorkspaceID: workspaceID,
		})
		return iterationModel(result), planningError(err)
	}
}

func patchIteration(planning planningApplication) jsonOperation[models.IterationPatch, models.Iteration] {
	return func(r *http.Request, patch models.IterationPatch) (models.Iteration, error) {
		user, id, err := planningTarget(r, "iteration_id")
		if err != nil {
			return models.Iteration{}, err
		}
		existing, err := planning.GetIteration(user.ID, id)
		if err != nil {
			return models.Iteration{}, planningError(err)
		}
		merged := patch.Apply(iterationModel(existing))
		result, err := planning.UpdateIteration(user.ID, auditActor(r, user), services.UpdateIterationParams{
			ID: id, Name: merged.Name, Description: merged.Description, StartDate: merged.StartDate,
			EndDate: merged.EndDate, Status: merged.Status, TypeID: merged.TypeID,
		})
		return iterationModel(result), planningError(err)
	}
}

func deleteIteration(planning planningApplication) commandOperation {
	return func(r *http.Request) error {
		user, id, err := planningTarget(r, "iteration_id")
		if err != nil {
			return err
		}
		return planningError(planning.DeleteIteration(user.ID, auditActor(r, user), id))
	}
}

func completeIteration(planning planningApplication) jsonOperation[completeIterationRequest, *services.CompleteIterationResult] {
	return func(r *http.Request, input completeIterationRequest) (*services.CompleteIterationResult, error) {
		user, id, err := planningTarget(r, "iteration_id")
		if err != nil {
			return nil, err
		}
		result, err := planning.CompleteIteration(r.Context(), user.ID, id, input.TargetIterationID)
		return result, planningError(err)
	}
}

func iterationProgress(planning planningApplication) readOperation[*services.IterationProgressReport] {
	return func(r *http.Request) (*services.IterationProgressReport, error) {
		user, id, err := planningTarget(r, "iteration_id")
		if err != nil {
			return nil, err
		}
		result, err := planning.GetIterationProgress(user.ID, id)
		return result, planningError(err)
	}
}

func iterationBurndown(planning planningApplication) readOperation[*services.IterationBurndownData] {
	return func(r *http.Request) (*services.IterationBurndownData, error) {
		user, id, err := planningTarget(r, "iteration_id")
		if err != nil {
			return nil, err
		}
		result, err := planning.GetIterationBurndown(user.ID, id)
		return result, planningError(err)
	}
}

func queryPlanningScope(r *http.Request) (*int, error) {
	scope := strings.TrimSpace(r.URL.Query().Get("scope"))
	workspaceID, err := optionalPositiveQueryID(r, "workspace_id")
	if err != nil {
		return nil, err
	}
	if scope == "global" && workspaceID == nil {
		return nil, nil
	}
	if scope == "" && workspaceID != nil {
		return workspaceID, nil
	}
	return nil, newError(http.StatusBadRequest, "invalid_request", "provide scope=global or workspace_id")
}

func bodyPlanningScope(input planningScopeRequest) (global bool, workspaceID *int, err error) {
	if input.Scope == "global" && input.WorkspaceID == nil {
		return true, nil, nil
	}
	if input.Scope == "" && input.WorkspaceID != nil && *input.WorkspaceID > 0 {
		return false, input.WorkspaceID, nil
	}
	return false, nil, newError(http.StatusBadRequest, "invalid_request", "provide scope=global or workspace_id")
}

func planningTarget(r *http.Request, name string) (*models.User, int, error) {
	user, err := principal(r)
	if err != nil {
		return nil, 0, err
	}
	id, err := pathID(r, name)
	return user, id, err
}

func milestoneModel(result *services.MilestoneResult) models.Milestone {
	if result == nil {
		return models.Milestone{}
	}
	var targetDate *string
	if result.TargetDate != "" {
		targetDate = &result.TargetDate
	}
	return models.Milestone{
		ID: result.ID, Name: result.Name, Description: result.Description, TargetDate: targetDate, Status: result.Status,
		CategoryID: result.CategoryID, CategoryName: result.CategoryName, CategoryColor: result.CategoryColor,
		IsGlobal: result.IsGlobal, WorkspaceID: result.WorkspaceID, WorkspaceName: result.WorkspaceName,
		ExternalKey: result.ExternalKey, Position: result.Position, CreatedAt: result.CreatedAt, UpdatedAt: result.UpdatedAt,
	}
}

func mapMilestones(rows []services.MilestoneResult) []models.Milestone {
	result := make([]models.Milestone, len(rows))
	for i := range rows {
		result[i] = milestoneModel(&rows[i])
	}
	return result
}

func iterationModel(result *services.IterationResult) models.Iteration {
	if result == nil {
		return models.Iteration{}
	}
	return models.Iteration{
		ID: result.ID, Name: result.Name, Description: result.Description, StartDate: result.StartDate, EndDate: result.EndDate,
		Status: result.Status, TypeID: result.TypeID, TypeName: result.TypeName, TypeColor: result.TypeColor,
		IsGlobal: result.IsGlobal, WorkspaceID: result.WorkspaceID, WorkspaceName: result.WorkspaceName,
		CreatedAt: result.CreatedAt, UpdatedAt: result.UpdatedAt,
	}
}

func mapIterations(rows []services.IterationResult) []models.Iteration {
	result := make([]models.Iteration, len(rows))
	for i := range rows {
		result[i] = iterationModel(&rows[i])
	}
	return result
}

func planningError(err error) error {
	if err == nil {
		return nil
	}
	if validation, ok := services.AsPlanningValidationError(err); ok {
		return newError(http.StatusBadRequest, "invalid_request", validation.Message)
	}
	switch {
	case errors.Is(err, repository.ErrNotFound), errors.Is(err, services.ErrIterationCompletionNotFound):
		return newError(http.StatusNotFound, "not_found", "Planning object was not found")
	case errors.Is(err, services.ErrPlanningForbidden), errors.Is(err, services.ErrIterationCompletionForbidden):
		return newError(http.StatusNotFound, "not_found", "Planning object was not found")
	case errors.Is(err, services.ErrIterationCompletionConflict), errors.Is(err, services.ErrIterationCompletionRequired), errors.Is(err, services.ErrIterationLifecycleConflict), errors.Is(err, services.ErrMilestoneReleaseIdempotencyConflict), errors.Is(err, services.ErrMilestoneReleaseInProgress):
		return newError(http.StatusConflict, "conflict", err.Error())
	default:
		return internalError(err)
	}
}
