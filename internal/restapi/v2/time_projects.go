package v2

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

func registerTimeRoutes(builder *routeBuilder, deps Deps) {
	projects := deps.TimeProjects
	builder.Read("/time/project-categories", AuthAuthenticated, []string{"time:read"}, listTimeProjectCategories(projects))
	builder.JSON(http.MethodPost, "/time/project-categories", http.StatusCreated, false, AuthAuthenticated, []string{"time:write"}, createTimeProjectCategory(projects))
	builder.Read("/time/project-categories/{category_id}", AuthAuthenticated, []string{"time:read"}, getTimeProjectCategory(projects))
	builder.JSON(http.MethodPatch, "/time/project-categories/{category_id}", http.StatusOK, true, AuthAuthenticated, []string{"time:write"}, patchTimeProjectCategory(projects))
	builder.Command(http.MethodDelete, "/time/project-categories/{category_id}", AuthAuthenticated, []string{"time:write"}, deleteTimeProjectCategory(projects))
	builder.JSON(http.MethodPut, "/time/project-categories/order", http.StatusOK, false, AuthAuthenticated, []string{"time:write"}, reorderTimeProjectCategories(projects))
	builder.Read("/time/projects", AuthAuthenticated, []string{"time:read"}, listTimeProjects(projects))
	builder.JSON(http.MethodPost, "/time/projects", http.StatusCreated, false, AuthAuthenticated, []string{"time:write"}, createTimeProject(projects))
	builder.Read("/time/projects/{project_id}", AuthAuthenticated, []string{"time:read"}, getTimeProject(projects))
	builder.JSON(http.MethodPatch, "/time/projects/{project_id}", http.StatusOK, true, AuthAuthenticated, []string{"time:write"}, patchTimeProject(projects))
	builder.Command(http.MethodDelete, "/time/projects/{project_id}", AuthAuthenticated, []string{"time:write"}, deleteTimeProject(projects))
	builder.Read("/workspaces/{workspace_id}/time-projects", AuthAuthenticated, []string{"time:read"}, listWorkspaceTimeProjects(projects))
	builder.Read("/time/projects/{project_id}/managers", AuthAuthenticated, []string{"time:read"}, listTimeProjectManagers(projects))
	builder.JSON(http.MethodPost, "/time/projects/{project_id}/managers", http.StatusCreated, false, AuthAuthenticated, []string{"time:write"}, addTimeProjectManager(projects))
	builder.Command(http.MethodDelete, "/time/projects/{project_id}/managers/{assignment_id}", AuthAuthenticated, []string{"time:write"}, removeTimeProjectManager(projects))
	builder.Read("/time/projects/{project_id}/members", AuthAuthenticated, []string{"time:read"}, listTimeProjectMembers(projects))
	builder.JSON(http.MethodPost, "/time/projects/{project_id}/members", http.StatusCreated, false, AuthAuthenticated, []string{"time:write"}, addTimeProjectMember(projects))
	builder.Command(http.MethodDelete, "/time/projects/{project_id}/members/{assignment_id}", AuthAuthenticated, []string{"time:write"}, removeTimeProjectMember(projects))
	builder.JSON(http.MethodPost, "/time/timers", http.StatusCreated, false, AuthAuthenticated, []string{"time:write"}, startTimer(deps.Timers))
	builder.Read("/time/timers/active", AuthAuthenticated, []string{"time:read"}, getActiveTimer(deps.Timers))
	builder.Action(http.MethodPost, "/time/timers/active/stop", http.StatusOK, AuthAuthenticated, []string{"time:write"}, stopTimer(deps.Timers))
}

type timeProjectApplication interface {
	List(int, string) ([]repository.TimeProjectDetail, error)
	ListForWorkspace(int, int, string) ([]repository.TimeProjectDetail, error)
	Get(int, int) (*repository.TimeProjectDetail, error)
	ListCategories() ([]models.TimeProjectCategory, error)
	GetCategory(int) (*models.TimeProjectCategory, error)
	CreateCategory(services.AuditActor, models.TimeProjectCategory) (*models.TimeProjectCategory, error)
	PatchCategory(services.AuditActor, int, services.TimeProjectCategoryPatch) (*models.TimeProjectCategory, error)
	DeleteCategory(services.AuditActor, int) error
	ReorderCategories(int, []services.TimeProjectOrder) ([]models.TimeProjectCategory, error)
	CreateProject(services.AuditActor, models.TimeProject) (*repository.TimeProjectDetail, error)
	PatchProject(services.AuditActor, int, services.TimeProjectPatch) (*repository.TimeProjectDetail, error)
	DeleteProject(services.AuditActor, int) error
	ListManagers(int, int) ([]models.TimeProjectManager, error)
	AddManager(services.AuditActor, int, models.TimeProjectManagerRequest) (*models.TimeProjectManager, error)
	RemoveManager(services.AuditActor, int, int) error
	ListMembers(int, int) ([]models.TimeProjectMember, error)
	AddMember(services.AuditActor, int, models.TimeProjectMemberRequest) (*models.TimeProjectMember, error)
	RemoveMember(services.AuditActor, int, int) error
}

type timerApplication interface {
	StartTimer(int, int, int, *int, string) (*models.ActiveTimer, error)
	GetActiveForUser(int) (*models.ActiveTimer, error)
	StopActiveForUser(int) (*services.StopResult, error)
}

type timeProjectDTO struct {
	ID            int            `json:"id"`
	CustomerID    *int           `json:"customer_id"`
	CategoryID    *int           `json:"category_id"`
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	Status        string         `json:"status"`
	Color         string         `json:"color"`
	HourlyRate    float64        `json:"hourly_rate"`
	Settings      map[string]any `json:"settings"`
	CustomerName  string         `json:"customer_name"`
	CategoryName  string         `json:"category_name"`
	CategoryColor string         `json:"category_color"`
	TotalHours    *float64       `json:"total_hours"`
	CreatedAt     string         `json:"created_at"`
	UpdatedAt     string         `json:"updated_at"`
}

type timeProjectCategoryDTO struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Color        string `json:"color"`
	DisplayOrder int    `json:"display_order"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

type timeProjectCreateRequest struct {
	CustomerID  int            `json:"customer_id"`
	CategoryID  *int           `json:"category_id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Status      string         `json:"status"`
	Color       string         `json:"color"`
	HourlyRate  float64        `json:"hourly_rate"`
	Settings    map[string]any `json:"settings"`
}

type timeProjectPatchRequest struct {
	CustomerID  Optional[int]            `json:"customer_id"`
	CategoryID  Optional[int]            `json:"category_id"`
	Name        Optional[string]         `json:"name"`
	Description Optional[string]         `json:"description"`
	Status      Optional[string]         `json:"status"`
	Color       Optional[string]         `json:"color"`
	HourlyRate  Optional[float64]        `json:"hourly_rate"`
	Settings    Optional[map[string]any] `json:"settings"`
}

type timeProjectCategoryCreateRequest struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Color        string `json:"color"`
	DisplayOrder int    `json:"display_order"`
}

type timeProjectCategoryPatchRequest struct {
	Name         Optional[string] `json:"name"`
	Description  Optional[string] `json:"description"`
	Color        Optional[string] `json:"color"`
	DisplayOrder Optional[int]    `json:"display_order"`
}

type timeProjectOrderRequest struct {
	Items []struct {
		ID           int `json:"id"`
		DisplayOrder int `json:"display_order"`
	} `json:"items"`
}

type timeProjectManagerDTO struct {
	ID            int    `json:"id"`
	ProjectID     int    `json:"project_id"`
	PrincipalType string `json:"principal_type"`
	PrincipalID   int    `json:"principal_id"`
	GrantedBy     *int   `json:"granted_by"`
	GrantedAt     string `json:"granted_at"`
	Name          string `json:"name"`
	Email         string `json:"email"`
}

type timeProjectMemberDTO struct {
	ID            int    `json:"id"`
	ProjectID     int    `json:"project_id"`
	PrincipalType string `json:"principal_type"`
	PrincipalID   int    `json:"principal_id"`
	GrantedBy     *int   `json:"granted_by"`
	GrantedAt     string `json:"granted_at"`
	Name          string `json:"name"`
	Email         string `json:"email"`
}

type timePrincipalRequest struct {
	PrincipalType string `json:"principal_type"`
	PrincipalID   int    `json:"principal_id"`
}

type startTimerRequest struct {
	WorkspaceID int    `json:"workspace_id"`
	ProjectID   int    `json:"project_id"`
	ItemID      *int   `json:"item_id"`
	Description string `json:"description"`
}

type stoppedTimerDTO struct {
	Stopped         bool   `json:"stopped"`
	TimerID         int    `json:"timer_id"`
	Description     string `json:"description"`
	DurationSeconds int64  `json:"duration_seconds"`
	DurationMinutes int    `json:"duration_minutes"`
	WorklogCreated  bool   `json:"worklog_created"`
}

func listTimeProjects(projects timeProjectApplication) readOperation[[]timeProjectDTO] {
	return func(r *http.Request) ([]timeProjectDTO, error) {
		user, err := principal(r)
		if err != nil {
			return nil, err
		}
		items, err := projects.List(user.ID, strings.TrimSpace(r.URL.Query().Get("status")))
		if err != nil {
			return nil, internalError(err)
		}
		result := make([]timeProjectDTO, len(items))
		for i := range items {
			result[i] = timeProjectFromRepository(items[i])
		}
		return result, nil
	}
}

func listWorkspaceTimeProjects(projects timeProjectApplication) readOperation[[]timeProjectDTO] {
	return func(r *http.Request) ([]timeProjectDTO, error) {
		user, err := principal(r)
		if err != nil {
			return nil, err
		}
		workspaceID, err := pathID(r, "workspace_id")
		if err != nil {
			return nil, err
		}
		items, err := projects.ListForWorkspace(user.ID, workspaceID, r.URL.Query().Get("status"))
		if err != nil {
			return nil, timeProjectError(err)
		}
		result := make([]timeProjectDTO, len(items))
		for i := range items {
			result[i] = timeProjectFromRepository(items[i])
		}
		return result, nil
	}
}

func getTimeProject(projects timeProjectApplication) readOperation[timeProjectDTO] {
	return func(r *http.Request) (timeProjectDTO, error) {
		user, err := principal(r)
		if err != nil {
			return timeProjectDTO{}, err
		}
		projectID, err := pathID(r, "project_id")
		if err != nil {
			return timeProjectDTO{}, err
		}
		project, err := projects.Get(user.ID, projectID)
		if err != nil {
			return timeProjectDTO{}, readError(err, "Time project was not found")
		}
		return timeProjectFromRepository(*project), nil
	}
}

func createTimeProject(projects timeProjectApplication) jsonOperation[timeProjectCreateRequest, timeProjectDTO] {
	return func(r *http.Request, input timeProjectCreateRequest) (timeProjectDTO, error) {
		user, err := principal(r)
		if err != nil {
			return timeProjectDTO{}, err
		}
		project, err := projects.CreateProject(auditActor(r, user), models.TimeProject{
			CustomerID: &input.CustomerID, CategoryID: input.CategoryID, Name: input.Name,
			Description: input.Description, Status: input.Status, Color: input.Color,
			HourlyRate: input.HourlyRate, Settings: input.Settings,
		})
		if err != nil {
			return timeProjectDTO{}, timeProjectError(err)
		}
		return timeProjectFromRepository(*project), nil
	}
}

func patchTimeProject(projects timeProjectApplication) jsonOperation[timeProjectPatchRequest, timeProjectDTO] {
	return func(r *http.Request, input timeProjectPatchRequest) (timeProjectDTO, error) {
		user, err := principal(r)
		if err != nil {
			return timeProjectDTO{}, err
		}
		id, err := pathID(r, "project_id")
		if err != nil {
			return timeProjectDTO{}, err
		}
		patch := services.TimeProjectPatch{
			CustomerIDSet: input.CustomerID.Set, CustomerID: optionalNullableInt(input.CustomerID),
			CategoryIDSet: input.CategoryID.Set, CategoryID: optionalNullableInt(input.CategoryID),
			Name: optionalValue(input.Name), Description: optionalValue(input.Description),
			Status: optionalValue(input.Status), Color: optionalValue(input.Color),
			HourlyRate: optionalValue(input.HourlyRate), SettingsSet: input.Settings.Set,
		}
		if input.Settings.Set && !input.Settings.Null {
			patch.Settings = input.Settings.Value
		}
		project, err := projects.PatchProject(auditActor(r, user), id, patch)
		if err != nil {
			return timeProjectDTO{}, timeProjectError(err)
		}
		return timeProjectFromRepository(*project), nil
	}
}

func deleteTimeProject(projects timeProjectApplication) commandOperation {
	return func(r *http.Request) error {
		user, err := principal(r)
		if err != nil {
			return err
		}
		id, err := pathID(r, "project_id")
		if err != nil {
			return err
		}
		return timeProjectError(projects.DeleteProject(auditActor(r, user), id))
	}
}

func listTimeProjectCategories(projects timeProjectApplication) readOperation[[]timeProjectCategoryDTO] {
	return func(_ *http.Request) ([]timeProjectCategoryDTO, error) {
		items, err := projects.ListCategories()
		if err != nil {
			return nil, internalError(err)
		}
		return timeProjectCategoriesFromModels(items), nil
	}
}

func getTimeProjectCategory(projects timeProjectApplication) readOperation[timeProjectCategoryDTO] {
	return func(r *http.Request) (timeProjectCategoryDTO, error) {
		id, err := pathID(r, "category_id")
		if err != nil {
			return timeProjectCategoryDTO{}, err
		}
		item, err := projects.GetCategory(id)
		if err != nil {
			return timeProjectCategoryDTO{}, timeProjectError(err)
		}
		return timeProjectCategoryFromModel(*item), nil
	}
}

func createTimeProjectCategory(projects timeProjectApplication) jsonOperation[timeProjectCategoryCreateRequest, timeProjectCategoryDTO] {
	return func(r *http.Request, input timeProjectCategoryCreateRequest) (timeProjectCategoryDTO, error) {
		user, err := principal(r)
		if err != nil {
			return timeProjectCategoryDTO{}, err
		}
		item, err := projects.CreateCategory(auditActor(r, user), models.TimeProjectCategory{
			Name: input.Name, Description: input.Description, Color: input.Color, DisplayOrder: input.DisplayOrder,
		})
		if err != nil {
			return timeProjectCategoryDTO{}, timeProjectError(err)
		}
		return timeProjectCategoryFromModel(*item), nil
	}
}

func patchTimeProjectCategory(projects timeProjectApplication) jsonOperation[timeProjectCategoryPatchRequest, timeProjectCategoryDTO] {
	return func(r *http.Request, input timeProjectCategoryPatchRequest) (timeProjectCategoryDTO, error) {
		user, err := principal(r)
		if err != nil {
			return timeProjectCategoryDTO{}, err
		}
		id, err := pathID(r, "category_id")
		if err != nil {
			return timeProjectCategoryDTO{}, err
		}
		item, err := projects.PatchCategory(auditActor(r, user), id, services.TimeProjectCategoryPatch{
			Name: optionalValue(input.Name), Description: optionalValue(input.Description),
			Color: optionalValue(input.Color), DisplayOrder: optionalValue(input.DisplayOrder),
		})
		if err != nil {
			return timeProjectCategoryDTO{}, timeProjectError(err)
		}
		return timeProjectCategoryFromModel(*item), nil
	}
}

func deleteTimeProjectCategory(projects timeProjectApplication) commandOperation {
	return func(r *http.Request) error {
		user, err := principal(r)
		if err != nil {
			return err
		}
		id, err := pathID(r, "category_id")
		if err != nil {
			return err
		}
		return timeProjectError(projects.DeleteCategory(auditActor(r, user), id))
	}
}

func reorderTimeProjectCategories(projects timeProjectApplication) jsonOperation[timeProjectOrderRequest, []timeProjectCategoryDTO] {
	return func(r *http.Request, input timeProjectOrderRequest) ([]timeProjectCategoryDTO, error) {
		user, err := principal(r)
		if err != nil {
			return nil, err
		}
		order := make([]services.TimeProjectOrder, len(input.Items))
		for i, item := range input.Items {
			order[i] = services.TimeProjectOrder{ID: item.ID, DisplayOrder: item.DisplayOrder}
		}
		items, err := projects.ReorderCategories(user.ID, order)
		if err != nil {
			return nil, timeProjectError(err)
		}
		return timeProjectCategoriesFromModels(items), nil
	}
}

func listTimeProjectManagers(projects timeProjectApplication) readOperation[[]timeProjectManagerDTO] {
	return func(r *http.Request) ([]timeProjectManagerDTO, error) {
		user, projectID, err := timeProjectPrincipalAndID(r)
		if err != nil {
			return nil, err
		}
		items, err := projects.ListManagers(user.ID, projectID)
		if err != nil {
			return nil, timeProjectError(err)
		}
		result := make([]timeProjectManagerDTO, len(items))
		for i, item := range items {
			result[i] = timeProjectManagerFromModel(item)
		}
		return result, nil
	}
}

func addTimeProjectManager(projects timeProjectApplication) jsonOperation[timePrincipalRequest, timeProjectManagerDTO] {
	return func(r *http.Request, input timePrincipalRequest) (timeProjectManagerDTO, error) {
		user, projectID, err := timeProjectPrincipalAndID(r)
		if err != nil {
			return timeProjectManagerDTO{}, err
		}
		item, err := projects.AddManager(auditActor(r, user), projectID, models.TimeProjectManagerRequest{ManagerType: input.PrincipalType, ManagerID: input.PrincipalID})
		if err != nil {
			return timeProjectManagerDTO{}, timeProjectError(err)
		}
		return timeProjectManagerFromModel(*item), nil
	}
}

func removeTimeProjectManager(projects timeProjectApplication) commandOperation {
	return func(r *http.Request) error {
		user, projectID, err := timeProjectPrincipalAndID(r)
		if err != nil {
			return err
		}
		assignmentID, err := pathID(r, "assignment_id")
		if err != nil {
			return err
		}
		return timeProjectError(projects.RemoveManager(auditActor(r, user), projectID, assignmentID))
	}
}

func listTimeProjectMembers(projects timeProjectApplication) readOperation[[]timeProjectMemberDTO] {
	return func(r *http.Request) ([]timeProjectMemberDTO, error) {
		user, projectID, err := timeProjectPrincipalAndID(r)
		if err != nil {
			return nil, err
		}
		items, err := projects.ListMembers(user.ID, projectID)
		if err != nil {
			return nil, timeProjectError(err)
		}
		result := make([]timeProjectMemberDTO, len(items))
		for i, item := range items {
			result[i] = timeProjectMemberFromModel(item)
		}
		return result, nil
	}
}

func addTimeProjectMember(projects timeProjectApplication) jsonOperation[timePrincipalRequest, timeProjectMemberDTO] {
	return func(r *http.Request, input timePrincipalRequest) (timeProjectMemberDTO, error) {
		user, projectID, err := timeProjectPrincipalAndID(r)
		if err != nil {
			return timeProjectMemberDTO{}, err
		}
		item, err := projects.AddMember(auditActor(r, user), projectID, models.TimeProjectMemberRequest{MemberType: input.PrincipalType, MemberID: input.PrincipalID})
		if err != nil {
			return timeProjectMemberDTO{}, timeProjectError(err)
		}
		return timeProjectMemberFromModel(*item), nil
	}
}

func removeTimeProjectMember(projects timeProjectApplication) commandOperation {
	return func(r *http.Request) error {
		user, projectID, err := timeProjectPrincipalAndID(r)
		if err != nil {
			return err
		}
		assignmentID, err := pathID(r, "assignment_id")
		if err != nil {
			return err
		}
		return timeProjectError(projects.RemoveMember(auditActor(r, user), projectID, assignmentID))
	}
}

func startTimer(timers timerApplication) jsonOperation[startTimerRequest, *models.ActiveTimer] {
	return func(r *http.Request, input startTimerRequest) (*models.ActiveTimer, error) {
		user, err := principal(r)
		if err != nil {
			return nil, err
		}
		timer, err := timers.StartTimer(user.ID, input.WorkspaceID, input.ProjectID, input.ItemID, input.Description)
		if err != nil {
			return nil, timerError(err)
		}
		return timer, nil
	}
}

func getActiveTimer(timers timerApplication) readOperation[*models.ActiveTimer] {
	return func(r *http.Request) (*models.ActiveTimer, error) {
		user, err := principal(r)
		if err != nil {
			return nil, err
		}
		timer, err := timers.GetActiveForUser(user.ID)
		if errors.Is(err, repository.ErrNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, internalError(err)
		}
		return timer, nil
	}
}

func stopTimer(timers timerApplication) actionOperation[stoppedTimerDTO] {
	return func(r *http.Request) (stoppedTimerDTO, error) {
		user, err := principal(r)
		if err != nil {
			return stoppedTimerDTO{}, err
		}
		result, err := timers.StopActiveForUser(user.ID)
		if err != nil {
			return stoppedTimerDTO{}, timerError(err)
		}
		return stoppedTimerDTO{
			Stopped: true, TimerID: result.TimerID, Description: result.Description,
			DurationSeconds: result.DurationSeconds, DurationMinutes: result.DurationMinutes,
			WorklogCreated: result.WorklogCreated,
		}, nil
	}
}

func timeProjectFromRepository(project repository.TimeProjectDetail) timeProjectDTO {
	return timeProjectDTO{
		ID: project.ID, CustomerID: project.CustomerID, CategoryID: project.CategoryID,
		Name: project.Name, Description: project.Description, Status: project.Status,
		Color: project.Color, HourlyRate: project.HourlyRate, Settings: project.Settings,
		CustomerName: project.CustomerName, CategoryName: project.CategoryName,
		CategoryColor: project.CategoryColor, TotalHours: project.TotalHours,
		CreatedAt: project.CreatedAt.UTC().Format(time.RFC3339), UpdatedAt: project.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func timeProjectPrincipalAndID(r *http.Request) (*models.User, int, error) {
	user, err := principal(r)
	if err != nil {
		return nil, 0, err
	}
	projectID, err := pathID(r, "project_id")
	if err != nil {
		return nil, 0, err
	}
	return user, projectID, nil
}

func timeProjectCategoriesFromModels(items []models.TimeProjectCategory) []timeProjectCategoryDTO {
	result := make([]timeProjectCategoryDTO, len(items))
	for i, item := range items {
		result[i] = timeProjectCategoryFromModel(item)
	}
	return result
}

func timeProjectCategoryFromModel(item models.TimeProjectCategory) timeProjectCategoryDTO {
	return timeProjectCategoryDTO{
		ID: item.ID, Name: item.Name, Description: item.Description, Color: item.Color,
		DisplayOrder: item.DisplayOrder, CreatedAt: item.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: item.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func timeProjectManagerFromModel(item models.TimeProjectManager) timeProjectManagerDTO {
	return timeProjectManagerDTO{
		ID: item.ID, ProjectID: item.ProjectID, PrincipalType: item.ManagerType,
		PrincipalID: item.ManagerID, GrantedBy: item.GrantedBy,
		GrantedAt: item.GrantedAt.UTC().Format(time.RFC3339), Name: item.ManagerName, Email: item.ManagerEmail,
	}
}

func timeProjectMemberFromModel(item models.TimeProjectMember) timeProjectMemberDTO {
	return timeProjectMemberDTO{
		ID: item.ID, ProjectID: item.ProjectID, PrincipalType: item.MemberType,
		PrincipalID: item.MemberID, GrantedBy: item.GrantedBy,
		GrantedAt: item.GrantedAt.UTC().Format(time.RFC3339), Name: item.MemberName, Email: item.MemberEmail,
	}
}

func optionalNullableInt(value Optional[int]) *int {
	if !value.Set || value.Null {
		return nil
	}
	return &value.Value
}

func timeProjectError(err error) error {
	if err == nil {
		return nil
	}
	var validation *services.TimeProjectValidationError
	switch {
	case errors.Is(err, repository.ErrNotFound):
		return newError(http.StatusNotFound, "not_found", "Time project resource was not found")
	case errors.Is(err, services.ErrTimeProjectForbidden):
		return newError(http.StatusForbidden, "forbidden", "Time project operation is not permitted")
	case errors.Is(err, services.ErrTimeProjectConflict):
		return newError(http.StatusConflict, "conflict", err.Error())
	case errors.As(err, &validation):
		return newError(http.StatusBadRequest, "invalid_request", validation.Message)
	default:
		return internalError(err)
	}
}

func timerError(err error) error {
	switch {
	case errors.Is(err, services.ErrTimerValidation):
		return newError(http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, services.ErrTimerNotFound):
		return newError(http.StatusNotFound, "not_found", "Timer was not found")
	case errors.Is(err, services.ErrTimerForbidden):
		return newError(http.StatusForbidden, "forbidden", "Timer operation is not permitted")
	case errors.Is(err, services.ErrTimerProjectInactive):
		return newError(http.StatusBadRequest, "invalid_request", "Cannot start a timer on an inactive project")
	case errors.Is(err, services.ErrTimerAlreadyRunning):
		return newError(http.StatusConflict, "conflict", "An active timer is already running")
	default:
		return internalError(err)
	}
}
