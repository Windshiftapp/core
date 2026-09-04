package v2

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

func registerWorklogRoutes(builder *routeBuilder, deps Deps) {
	builder.Page("/time/worklogs", AuthAuthenticated, []string{"time:read"}, listWorklogs(deps))
	builder.Read("/time/worklogs/{worklog_id}", AuthAuthenticated, []string{"time:read"}, getWorklog(deps))
	builder.JSON(http.MethodPost, "/time/worklogs", http.StatusCreated, false, AuthAuthenticated, []string{"time:write"}, createWorklog(deps))
	builder.JSON(http.MethodPatch, "/time/worklogs/{worklog_id}", http.StatusOK, true, AuthAuthenticated, []string{"time:write"}, updateWorklog(deps))
	builder.Command(http.MethodDelete, "/time/worklogs/{worklog_id}", AuthAuthenticated, []string{"time:delete"}, deleteWorklog(deps))
	builder.Page("/items/{item_id}/worklogs", AuthAuthenticated, []string{"time:read"}, listItemWorklogs(deps))
	builder.Page("/time/projects/{project_id}/worklogs", AuthAuthenticated, []string{"time:read"}, listProjectWorklogs(deps))
}

type worklogCreateRequest struct {
	ProjectID       int    `json:"project_id"`
	ItemID          *int   `json:"item_id"`
	Description     string `json:"description"`
	Date            string `json:"date"`
	Duration        string `json:"duration"`
	DurationMinutes int    `json:"duration_minutes"`
	StartTime       string `json:"start_time"`
	EndTime         string `json:"end_time"`
	Timezone        string `json:"timezone"`
}

type worklogPatchRequest struct {
	ProjectID       Optional[int]    `json:"project_id"`
	ItemID          Optional[int]    `json:"item_id"`
	Description     Optional[string] `json:"description"`
	Date            Optional[string] `json:"date"`
	Duration        Optional[string] `json:"duration"`
	DurationMinutes Optional[int]    `json:"duration_minutes"`
	StartTime       Optional[string] `json:"start_time"`
	EndTime         Optional[string] `json:"end_time"`
	Timezone        Optional[string] `json:"timezone"`
}

type worklogDTO struct {
	ID                  int      `json:"id"`
	ProjectID           int      `json:"project_id"`
	CustomerID          int      `json:"customer_id"`
	UserID              *int     `json:"user_id"`
	ItemID              *int     `json:"item_id"`
	Description         string   `json:"description"`
	Date                int64    `json:"date"`
	StartTime           int64    `json:"start_time"`
	EndTime             int64    `json:"end_time"`
	DurationMinutes     int      `json:"duration_minutes"`
	CreatedAt           int64    `json:"created_at"`
	UpdatedAt           int64    `json:"updated_at"`
	CustomerName        string   `json:"customer_name"`
	ProjectName         string   `json:"project_name"`
	UserName            string   `json:"user_name"`
	ItemTitle           string   `json:"item_title"`
	WorkspaceID         *int     `json:"workspace_id"`
	WorkspaceKey        string   `json:"workspace_key"`
	WorkspaceItemNumber int      `json:"workspace_item_number"`
	ProjectMaxHours     *float64 `json:"project_max_hours"`
	ProjectTotalHours   *float64 `json:"project_total_hours"`
	Warnings            []string `json:"warnings,omitempty"`
}

func listWorklogs(deps Deps) pageOperation[worklogDTO] {
	return func(r *http.Request) ([]worklogDTO, Pagination, int, error) {
		user, err := principal(r)
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		page, err := ParsePage(r)
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		filter := repository.WorklogListFilter{UserID: user.ID, Limit: page.PageSize, Offset: page.Offset}
		if err := applyWorklogFilters(r, &filter); err != nil {
			return nil, Pagination{}, 0, err
		}
		worklogs, total, err := deps.Worklogs.ListMine(filter)
		if err != nil {
			return nil, Pagination{}, 0, internalError(err)
		}
		worklogs = redactWorklogs(r, deps, worklogs)
		return mapWorklogs(worklogs), page, total, nil
	}
}

func getWorklog(deps Deps) readOperation[worklogDTO] {
	return func(r *http.Request) (worklogDTO, error) {
		user, worklog, err := requireWorklog(r, deps)
		if err != nil {
			return worklogDTO{}, err
		}
		allowed, err := deps.TimeAccess.CanEditWorklog(user.ID, worklog.ID)
		if err != nil {
			return worklogDTO{}, internalError(err)
		}
		if !allowed {
			return worklogDTO{}, newError(http.StatusNotFound, "not_found", "Worklog was not found")
		}
		redacted := redactWorklogs(r, deps, []models.Worklog{*worklog})
		return worklogFromModel(&redacted[0], nil), nil
	}
}

func createWorklog(deps Deps) jsonOperation[worklogCreateRequest, worklogDTO] {
	return func(r *http.Request, input worklogCreateRequest) (worklogDTO, error) {
		user, err := principal(r)
		if err != nil {
			return worklogDTO{}, err
		}
		if err := requireWorklogProject(deps, user.ID, input.ProjectID, false); err != nil {
			return worklogDTO{}, err
		}
		if err := requireWorklogItem(r, deps, input.ItemID); err != nil {
			return worklogDTO{}, err
		}
		result, err := deps.Worklogs.Create(user.ID, worklogInput(input, user.Timezone))
		if err != nil {
			return worklogDTO{}, worklogError(err)
		}
		return worklogFromModel(result.Worklog, result.Warnings), nil
	}
}

func updateWorklog(deps Deps) jsonOperation[worklogPatchRequest, worklogDTO] {
	return func(r *http.Request, patch worklogPatchRequest) (worklogDTO, error) {
		if worklogPatchHasInvalidNull(patch) {
			return worklogDTO{}, newError(http.StatusBadRequest, "invalid_request", "Only item_id may be null")
		}
		user, current, err := requireWorklog(r, deps)
		if err != nil {
			return worklogDTO{}, err
		}
		allowed, err := deps.TimeAccess.CanEditWorklog(user.ID, current.ID)
		if err != nil {
			return worklogDTO{}, internalError(err)
		}
		if !allowed {
			return worklogDTO{}, newError(http.StatusNotFound, "not_found", "Worklog was not found")
		}
		input := mergeWorklogPatch(current, patch, user.Timezone)
		if err := requireWorklogProject(deps, user.ID, input.ProjectID, false); err != nil {
			return worklogDTO{}, err
		}
		if err := requireWorklogItem(r, deps, input.ItemID); err != nil {
			return worklogDTO{}, err
		}
		result, err := deps.Worklogs.Update(user.ID, current.ID, input)
		if err != nil {
			return worklogDTO{}, worklogError(err)
		}
		return worklogFromModel(result.Worklog, result.Warnings), nil
	}
}

func deleteWorklog(deps Deps) commandOperation {
	return func(r *http.Request) error {
		user, worklog, err := requireWorklog(r, deps)
		if err != nil {
			return err
		}
		allowed, err := deps.TimeAccess.CanEditWorklog(user.ID, worklog.ID)
		if err != nil {
			return internalError(err)
		}
		if !allowed {
			return newError(http.StatusNotFound, "not_found", "Worklog was not found")
		}
		return worklogError(deps.Worklogs.Delete(worklog.ID))
	}
}

func listItemWorklogs(deps Deps) pageOperation[worklogDTO] {
	return func(r *http.Request) ([]worklogDTO, Pagination, int, error) {
		item, err := requireItem(r, deps, deps.Access.CanViewWorkspace)
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		page, err := ParsePage(r)
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		user, err := principal(r)
		if err != nil {
			return nil, page, 0, err
		}
		projectIDs, err := deps.TimeAccess.AccessibleTimeProjectIDs(user.ID)
		if err != nil {
			return nil, page, 0, internalError(err)
		}
		worklogs, total, err := deps.Worklogs.ListPage(repository.WorklogDetailFilter{
			ItemID: &item.ID, AccessibleProjectIDs: projectIDs, Limit: page.PageSize, Offset: page.Offset,
		})
		if err != nil {
			return nil, page, 0, internalError(err)
		}
		return mapWorklogs(worklogs), page, total, nil
	}
}

func listProjectWorklogs(deps Deps) pageOperation[worklogDTO] {
	return func(r *http.Request) ([]worklogDTO, Pagination, int, error) {
		user, err := principal(r)
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		projectID, err := pathID(r, "project_id")
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		if err := requireWorklogProject(deps, user.ID, projectID, true); err != nil {
			return nil, Pagination{}, 0, err
		}
		page, err := ParsePage(r)
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		filter := repository.WorklogDetailFilter{ProjectID: &projectID, Limit: page.PageSize, Offset: page.Offset}
		if err := applyWorklogDetailDates(r, &filter); err != nil {
			return nil, Pagination{}, 0, err
		}
		worklogs, total, err := deps.Worklogs.ListPage(filter)
		if err != nil {
			return nil, Pagination{}, 0, internalError(err)
		}
		return mapWorklogs(worklogs), page, total, nil
	}
}

func requireWorklog(r *http.Request, deps Deps) (*models.User, *models.Worklog, error) {
	user, err := principal(r)
	if err != nil {
		return nil, nil, err
	}
	id, err := pathID(r, "worklog_id")
	if err != nil {
		return nil, nil, err
	}
	worklog, err := deps.Worklogs.Get(id)
	if err != nil {
		return nil, nil, worklogError(err)
	}
	return user, worklog, nil
}

func requireWorklogProject(deps Deps, userID, projectID int, manager bool) error {
	if projectID <= 0 {
		return newError(http.StatusBadRequest, "invalid_request", "project_id is required")
	}
	var allowed bool
	var err error
	if manager {
		allowed, err = deps.TimeAccess.IsTimeProjectManager(userID, projectID)
	} else {
		allowed, err = deps.TimeAccess.CanBookTimeOnProject(userID, projectID)
	}
	if err != nil {
		return internalError(err)
	}
	if !allowed {
		return newError(http.StatusNotFound, "not_found", "Time project was not found")
	}
	return nil
}

func requireWorklogItem(r *http.Request, deps Deps, itemID *int) error {
	if itemID == nil {
		return nil
	}
	if *itemID <= 0 {
		return newError(http.StatusBadRequest, "invalid_request", "item_id is invalid")
	}
	_, err := requireItemID(r, deps, *itemID, deps.Access.CanViewWorkspace)
	return err
}

func applyWorklogFilters(r *http.Request, filter *repository.WorklogListFilter) error {
	if err := applyWorklogDateRange(r, &filter.DateFromUnix, &filter.DateToExclusiveUnix); err != nil {
		return err
	}
	if raw := r.URL.Query().Get("project_id"); raw != "" {
		id, err := strconv.Atoi(raw)
		if err != nil || id <= 0 {
			return newError(http.StatusBadRequest, "invalid_request", "project_id is invalid")
		}
		filter.ProjectID = &id
	}
	return nil
}

func applyWorklogDetailDates(r *http.Request, filter *repository.WorklogDetailFilter) error {
	return applyWorklogDateRange(r, &filter.DateFromUnix, &filter.DateToExclusiveUnix)
}

func applyWorklogDateRange(r *http.Request, from, to **int64) error {
	if raw := r.URL.Query().Get("from"); raw != "" {
		start, _, err := services.CivilDateRangeUTC(raw, raw, time.UTC)
		if err != nil {
			return newError(http.StatusBadRequest, "invalid_request", "from must use YYYY-MM-DD")
		}
		value := start.Unix()
		*from = &value
	}
	if raw := r.URL.Query().Get("to"); raw != "" {
		_, end, err := services.CivilDateRangeUTC(raw, raw, time.UTC)
		if err != nil {
			return newError(http.StatusBadRequest, "invalid_request", "to must use YYYY-MM-DD")
		}
		value := end.Unix()
		*to = &value
	}
	return nil
}

func worklogInput(input worklogCreateRequest, userTimezone string) services.WorklogMutationInput {
	return services.WorklogMutationInput{
		ProjectID: input.ProjectID, ItemID: input.ItemID, Description: input.Description,
		Date: input.Date, Duration: input.Duration, DurationMinutes: input.DurationMinutes,
		StartTime: input.StartTime, EndTime: input.EndTime, Timezone: input.Timezone,
		UserTimezone: userTimezone,
	}
}

func mergeWorklogPatch(current *models.Worklog, patch worklogPatchRequest, userTimezone string) services.WorklogMutationInput {
	input := services.WorklogMutationInput{
		ProjectID: current.ProjectID, ItemID: current.ItemID, Description: current.Description,
		Date:            time.Unix(current.Date, 0).UTC().Format("2006-01-02"),
		DurationMinutes: current.DurationMins, UserTimezone: userTimezone,
	}
	if patch.ProjectID.Set {
		input.ProjectID = patch.ProjectID.Value
	}
	if patch.ItemID.Set {
		input.ItemID = optionalNullableValue(patch.ItemID)
	}
	if patch.Description.Set {
		input.Description = patch.Description.Value
	}
	if patch.Date.Set {
		input.Date = patch.Date.Value
	}
	if patch.Timezone.Set {
		input.Timezone = patch.Timezone.Value
	}
	if patch.Duration.Set || patch.DurationMinutes.Set {
		input.DurationMinutes = 0
		if patch.Duration.Set {
			input.Duration = patch.Duration.Value
		} else {
			input.DurationMinutes = patch.DurationMinutes.Value
		}
	} else if patch.StartTime.Set || patch.EndTime.Set {
		location, err := time.LoadLocation(userTimezone)
		if err != nil {
			location = time.UTC
		}
		input.DurationMinutes = 0
		input.StartTime = time.Unix(current.StartTime, 0).In(location).Format("15:04")
		input.EndTime = time.Unix(current.EndTime, 0).In(location).Format("15:04")
		if patch.StartTime.Set {
			input.StartTime = patch.StartTime.Value
		}
		if patch.EndTime.Set {
			input.EndTime = patch.EndTime.Value
		}
	}
	return input
}

func worklogPatchHasInvalidNull(patch worklogPatchRequest) bool {
	return patch.ProjectID.Null || patch.Description.Null || patch.Date.Null || patch.Duration.Null ||
		patch.DurationMinutes.Null || patch.StartTime.Null || patch.EndTime.Null || patch.Timezone.Null
}

func redactWorklogs(r *http.Request, deps Deps, worklogs []models.Worklog) []models.Worklog {
	user, _ := principal(r)
	return services.RedactInaccessibleWorklogItems(worklogs, func(workspaceID int) (bool, error) {
		return deps.Access.CanViewWorkspace(user.ID, workspaceID)
	})
}

func mapWorklogs(worklogs []models.Worklog) []worklogDTO {
	result := make([]worklogDTO, len(worklogs))
	for i := range worklogs {
		result[i] = worklogFromModel(&worklogs[i], nil)
	}
	return result
}

func worklogFromModel(worklog *models.Worklog, warnings []string) worklogDTO {
	return worklogDTO{
		ID: worklog.ID, ProjectID: worklog.ProjectID, CustomerID: worklog.CustomerID,
		UserID: worklog.UserID, ItemID: worklog.ItemID, Description: worklog.Description,
		Date: worklog.Date, StartTime: worklog.StartTime, EndTime: worklog.EndTime,
		DurationMinutes: worklog.DurationMins, CreatedAt: worklog.CreatedAt, UpdatedAt: worklog.UpdatedAt,
		CustomerName: worklog.CustomerName, ProjectName: worklog.ProjectName, UserName: worklog.UserName,
		ItemTitle: worklog.ItemTitle, WorkspaceID: worklog.WorkspaceID, WorkspaceKey: worklog.WorkspaceKey,
		WorkspaceItemNumber: worklog.WorkspaceItemNumber, ProjectMaxHours: worklog.ProjectMaxHours,
		ProjectTotalHours: worklog.ProjectTotalHours, Warnings: warnings,
	}
}

func worklogError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, repository.ErrNotFound):
		return newError(http.StatusNotFound, "not_found", "Worklog was not found")
	case errors.Is(err, services.ErrWorklogProjectNotFound):
		return newError(http.StatusNotFound, "not_found", "Time project was not found")
	case errors.Is(err, services.ErrWorklogProjectInactive),
		errors.Is(err, services.ErrWorklogCustomerMissing),
		errors.Is(err, services.ErrWorklogInvalidInput):
		return newError(http.StatusBadRequest, "invalid_request", err.Error())
	default:
		return internalError(err)
	}
}
