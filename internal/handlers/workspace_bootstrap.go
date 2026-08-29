package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

// WorkspaceBootstrapHandler composes the stable reference data required on
// workspace entry. It calls services and repositories directly so the browser
// pays for one authorized request instead of a fan-out of domain GETs.
type WorkspaceBootstrapHandler struct {
	workspace  *WorkspaceHandler
	users      *UserHandler
	milestones *MilestoneHandler
	iterations *IterationHandler
	projects   *TimeProjectHandler
}

type WorkspaceBootstrapResponse struct {
	Workspace              *models.Workspace              `json:"workspace"`
	HomepageLayout         models.WorkspaceHomepageLayout `json:"homepage_layout"`
	Statuses               []models.Status                `json:"statuses"`
	Users                  []models.User                  `json:"users"`
	Milestones             []models.Milestone             `json:"milestones"`
	Iterations             []models.Iteration             `json:"iterations"`
	Projects               []models.TimeProject           `json:"projects"`
	ItemTypes              []services.ItemTypeResult      `json:"item_types"`
	StatusCategories       []models.StatusCategory        `json:"status_categories"`
	Priorities             []models.Priority              `json:"priorities"`
	CustomFieldDefinitions []models.CustomFieldDefinition `json:"custom_field_definitions"`
}

func NewWorkspaceBootstrapHandler(
	workspace *WorkspaceHandler,
	users *UserHandler,
	milestones *MilestoneHandler,
	iterations *IterationHandler,
	projects *TimeProjectHandler,
) *WorkspaceBootstrapHandler {
	return &WorkspaceBootstrapHandler{
		workspace: workspace, users: users, milestones: milestones,
		iterations: iterations, projects: projects,
	}
}

func (h *WorkspaceBootstrapHandler) Get(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	workspaceID, ok := requireWorkspaceIDParam(w, r, h.workspace.keyCache, "id")
	if !ok {
		return
	}

	workspace, err := h.workspace.loadWorkspaceForUser(user, workspaceID)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "workspace")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	h.workspace.trackWorkspaceVisit(user.ID, workspaceID)
	response, err := h.load(r.Context(), user.ID, workspace)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, response)
}

func (h *WorkspaceBootstrapHandler) load(ctx context.Context, userID int, workspace *models.Workspace) (WorkspaceBootstrapResponse, error) {
	response := WorkspaceBootstrapResponse{
		Workspace:              workspace,
		HomepageLayout:         emptyHomepageLayout(),
		Statuses:               []models.Status{},
		Users:                  []models.User{},
		Milestones:             []models.Milestone{},
		Iterations:             []models.Iteration{},
		Projects:               []models.TimeProject{},
		ItemTypes:              []services.ItemTypeResult{},
		StatusCategories:       []models.StatusCategory{},
		Priorities:             []models.Priority{},
		CustomFieldDefinitions: []models.CustomFieldDefinition{},
	}

	var wait sync.WaitGroup
	errorsFound := make(chan error, 16)
	run := func(name string, load func() error) {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if err := load(); err != nil {
				errorsFound <- fmt.Errorf("%s: %w", name, err)
			}
		}()
	}

	// Appearance used to be a separate best-effort request. Preserve that
	// resilience while returning it in the successful aggregate snapshot.
	run("homepage layout", func() error {
		layout, err := h.workspace.loadHomepageLayout(workspace.ID)
		if err != nil {
			slog.Warn("workspace bootstrap: homepage layout unavailable", "workspace_id", workspace.ID, "error", err)
			return nil
		}
		response.HomepageLayout = layout
		return nil
	})
	run("statuses", func() error {
		statuses, err := h.workspace.loadWorkspaceStatuses(workspace.ID, nil)
		response.Statuses = statuses
		return err
	})
	run("assignable users", func() error {
		users, err := h.listAssignableUsers(ctx, workspace.ID)
		response.Users = users
		return err
	})
	run("milestones", func() error {
		milestones, err := h.listMilestones(userID, workspace.ID)
		response.Milestones = milestones
		return err
	})
	run("iterations", func() error {
		iterations, err := h.listIterations(workspace.ID)
		response.Iterations = iterations
		return err
	})
	run("projects", func() error {
		projects, err := h.listProjects(ctx, userID, workspace)
		response.Projects = projects
		return err
	})
	run("configuration", func() error {
		config := services.NewConfigReadService(h.workspace.db)
		itemTypes, err := services.NewWorkspaceService(h.workspace.db).GetItemTypes(workspace.ID)
		if err != nil {
			return err
		}
		priorities, err := config.ListPriorities()
		if err != nil {
			return err
		}
		response.ItemTypes = itemTypes
		response.Priorities = prioritiesToModels(priorities)
		return nil
	})
	run("status categories", func() error {
		categories, err := h.listStatusCategories()
		if err != nil {
			return err
		}
		response.StatusCategories = categories
		return nil
	})
	run("custom fields", func() error {
		fields, err := repository.NewCustomFieldRepository(h.workspace.db).List()
		if err != nil {
			slog.Warn("workspace bootstrap: custom fields unavailable", "workspace_id", workspace.ID, "error", err)
			return nil
		}
		response.CustomFieldDefinitions = fields
		return nil
	})

	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		return WorkspaceBootstrapResponse{}, err
	}
	return response, nil
}

func emptyHomepageLayout() models.WorkspaceHomepageLayout {
	return models.WorkspaceHomepageLayout{
		Sections: []models.WorkspaceHomepageSection{},
		Widgets:  []models.WorkspaceWidget{},
	}
}

func (h *WorkspaceBootstrapHandler) listAssignableUsers(ctx context.Context, workspaceID int) ([]models.User, error) {
	if h.users.workspaceUsers == nil {
		return nil, errors.New("workspace user resolver is not configured")
	}
	return h.users.workspaceUsers.List(ctx, workspaceID)
}

func (h *WorkspaceBootstrapHandler) listMilestones(userID, workspaceID int) ([]models.Milestone, error) {
	results, _, err := h.milestones.planningService.ListMilestones(services.MilestoneListParams{
		Limit: 1000, WorkspaceID: &workspaceID, IncludeGlobal: true,
	})
	if err != nil {
		return nil, err
	}
	milestones := make([]models.Milestone, 0, len(results))
	for i := range results {
		milestones = append(milestones, h.milestones.milestoneResultToModel(&results[i], userID))
	}
	return milestones, nil
}

func (h *WorkspaceBootstrapHandler) listIterations(workspaceID int) ([]models.Iteration, error) {
	results, _, err := h.iterations.planningService.ListIterations(services.IterationListParams{
		Limit: 1000, WorkspaceID: &workspaceID, IncludeGlobal: true,
	})
	if err != nil {
		return nil, err
	}
	iterations := make([]models.Iteration, 0, len(results))
	for i := range results {
		iterations = append(iterations, iterationResultToModel(&results[i]))
	}
	return iterations, nil
}

func (h *WorkspaceBootstrapHandler) listProjects(ctx context.Context, userID int, workspace *models.Workspace) ([]models.TimeProject, error) {
	accessibleIDs, err := h.projects.timePermissionService.GetAccessibleProjectsContext(ctx, userID)
	if err != nil {
		return nil, err
	}
	if accessibleIDs != nil && len(accessibleIDs) == 0 {
		return []models.TimeProject{}, nil
	}
	details, err := repository.NewTimeProjectRepository(h.workspace.db).ListDetails(accessibleIDs, "")
	if err != nil {
		return nil, err
	}

	// Re-read category restrictions as an authoritative project-access input.
	// The workspace detail treats this decoration as best-effort, but a failure
	// here must not accidentally broaden the project list.
	categoryIDs, err := h.workspace.repo.GetTimeProjectCategories(workspace.ID)
	if err != nil {
		return nil, err
	}
	allowedCategories := make(map[int]struct{}, len(categoryIDs))
	for _, categoryID := range categoryIDs {
		allowedCategories[categoryID] = struct{}{}
	}
	projects := make([]models.TimeProject, 0, len(details))
	for _, project := range details {
		if len(allowedCategories) > 0 {
			if project.CategoryID == nil {
				continue
			}
			if _, ok := allowedCategories[*project.CategoryID]; !ok {
				continue
			}
		}
		projects = append(projects, models.TimeProject{
			ID: project.ID, CustomerID: project.CustomerID, CategoryID: project.CategoryID,
			Name: project.Name, Description: project.Description, Status: project.Status,
			Color: project.Color, HourlyRate: project.HourlyRate, Settings: project.Settings,
			CreatedAt: project.CreatedAt, UpdatedAt: project.UpdatedAt,
			CustomerName: project.CustomerName, CategoryName: project.CategoryName,
			CategoryColor: project.CategoryColor, TotalHours: project.TotalHours,
		})
	}
	return projects, nil
}

func prioritiesToModels(results []services.PriorityResult) []models.Priority {
	priorities := make([]models.Priority, 0, len(results))
	for _, priority := range results {
		priorities = append(priorities, models.Priority{
			ID: priority.ID, BuiltinKey: priority.BuiltinKey, Name: priority.Name, Description: priority.Description,
			Icon: priority.Icon, Color: priority.Color, SortOrder: priority.SortOrder,
			IsDefault: priority.IsDefault,
		})
	}
	return priorities
}

func (h *WorkspaceBootstrapHandler) listStatusCategories() ([]models.StatusCategory, error) {
	entities, err := services.NewEnumService(h.workspace.db, services.NewStatusCategoryConfig()).GetAll()
	if err != nil {
		return nil, err
	}
	categories := make([]models.StatusCategory, 0, len(entities))
	for _, entity := range entities {
		category, ok := entity.(*models.StatusCategory)
		if !ok {
			return nil, fmt.Errorf("unexpected status category type %T", entity)
		}
		categories = append(categories, *category)
	}
	return categories, nil
}
