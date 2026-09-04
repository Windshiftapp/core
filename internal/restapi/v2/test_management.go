package v2

import (
	"errors"
	"net/http"
	"strconv"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

type workspaceCRUD[Model, Create, Patch any] struct {
	list   func(*http.Request, int, int, Pagination) ([]Model, int, error)
	get    func(*http.Request, int, int, int) (Model, error)
	create func(*http.Request, int, int, Create) (Model, error)
	patch  func(*http.Request, int, int, int, Patch) (Model, error)
	delete func(*http.Request, int, int, int) error
}

// WorkspaceCRUD uses Go 1.27 generic methods to register the common route
// shape without repeating decoding, identity, pagination, and path plumbing.
func (b *routeBuilder) WorkspaceCRUD[Model, Create, Patch any](path, idParam, readScope, writeScope string, operations workspaceCRUD[Model, Create, Patch]) {
	b.Page(path, AuthAuthenticated, []string{readScope}, func(r *http.Request) ([]Model, Pagination, int, error) {
		user, workspaceID, _, err := testTarget(r, "")
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		page, err := ParsePage(r)
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		items, total, err := operations.list(r, user.ID, workspaceID, page)
		return items, page, total, testManagementError(err)
	})
	b.JSON(http.MethodPost, path, http.StatusCreated, false, AuthAuthenticated, []string{writeScope}, func(r *http.Request, input Create) (Model, error) {
		user, workspaceID, _, err := testTarget(r, "")
		if err != nil {
			var zero Model
			return zero, err
		}
		result, err := operations.create(r, user.ID, workspaceID, input)
		return result, testManagementError(err)
	})
	itemPath := path + "/{" + idParam + "}"
	b.Read(itemPath, AuthAuthenticated, []string{readScope}, func(r *http.Request) (Model, error) {
		user, workspaceID, id, err := testTarget(r, idParam)
		if err != nil {
			var zero Model
			return zero, err
		}
		result, err := operations.get(r, user.ID, workspaceID, id)
		return result, testManagementError(err)
	})
	b.JSON(http.MethodPatch, itemPath, http.StatusOK, true, AuthAuthenticated, []string{writeScope}, func(r *http.Request, input Patch) (Model, error) {
		user, workspaceID, id, err := testTarget(r, idParam)
		if err != nil {
			var zero Model
			return zero, err
		}
		result, err := operations.patch(r, user.ID, workspaceID, id, input)
		return result, testManagementError(err)
	})
	b.Command(http.MethodDelete, itemPath, AuthAuthenticated, []string{writeScope}, func(r *http.Request) error {
		user, workspaceID, id, err := testTarget(r, idParam)
		if err != nil {
			return err
		}
		return testManagementError(operations.delete(r, user.ID, workspaceID, id))
	})
}

func registerTestManagementRoutes(builder *routeBuilder, application *services.TestManagementApplicationService) {
	registerTestFolderRoutes(builder, application)
	registerTestCaseRoutes(builder, application)
	registerTestPlanRoutes(builder, application)
	registerTestRunTemplateRoutes(builder, application)
	registerTestRunRoutes(builder, application)
	registerTestReportRoutes(builder, application)
	registerTestCoverageRoutes(builder, application)
}

type testFolderCreate struct {
	ParentID    *int   `json:"parent_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type testFolderPatch struct {
	ParentID    Optional[int] `json:"parent_id"`
	Name        *string       `json:"name"`
	Description *string       `json:"description"`
	SortOrder   *int          `json:"sort_order"`
}

type reorderRequest struct {
	IDs []int `json:"ids"`
}

func registerTestFolderRoutes(builder *routeBuilder, app *services.TestManagementApplicationService) {
	path := "/workspaces/{workspace_id}/test-folders"
	builder.WorkspaceCRUD(path, "folder_id", "tests:read", "tests:write", workspaceCRUD[*models.TestFolder, testFolderCreate, testFolderPatch]{
		list: func(_ *http.Request, userID, workspaceID int, page Pagination) ([]*models.TestFolder, int, error) {
			items, err := app.ListFolders(userID, workspaceID)
			return pagePointers(items, page), len(items), err
		},
		get: func(_ *http.Request, userID, workspaceID, id int) (*models.TestFolder, error) {
			return app.GetFolder(userID, workspaceID, id)
		},
		create: func(r *http.Request, userID, workspaceID int, input testFolderCreate) (*models.TestFolder, error) {
			return app.CreateFolder(userID, workspaceID, auditActor(r, mustPrincipal(r)), models.TestFolder{ParentID: input.ParentID, Name: input.Name, Description: input.Description})
		},
		patch: func(r *http.Request, userID, workspaceID, id int, input testFolderPatch) (*models.TestFolder, error) {
			return app.UpdateFolder(userID, workspaceID, id, auditActor(r, mustPrincipal(r)), services.TestFolderPatch{
				Name: input.Name, Description: input.Description, ParentID: optionalInt(input.ParentID), ParentIDSet: input.ParentID.Set,
				SortOrder: valueOrZero(input.SortOrder), SortOrderSet: input.SortOrder != nil,
			})
		},
		delete: func(r *http.Request, userID, workspaceID, id int) error {
			return app.DeleteFolder(userID, workspaceID, id, auditActor(r, mustPrincipal(r)))
		},
	})
	builder.JSON(http.MethodPost, path+"/reorder", http.StatusOK, false, AuthAuthenticated, []string{"tests:write"}, func(r *http.Request, input reorderRequest) (map[string]bool, error) {
		user, workspaceID, _, err := testTarget(r, "")
		if err != nil {
			return nil, err
		}
		return map[string]bool{"reordered": true}, testManagementError(app.ReorderFolders(user.ID, workspaceID, input.IDs))
	})
}

type testCaseCreate struct {
	Title             string `json:"title"`
	Preconditions     string `json:"preconditions"`
	Priority          string `json:"priority"`
	Status            string `json:"status"`
	EstimatedDuration int    `json:"estimated_duration"`
	FolderID          *int   `json:"folder_id"`
}

type testCasePatch struct {
	Title             *string       `json:"title"`
	Preconditions     *string       `json:"preconditions"`
	Priority          *string       `json:"priority"`
	Status            *string       `json:"status"`
	EstimatedDuration *int          `json:"estimated_duration"`
	FolderID          Optional[int] `json:"folder_id"`
	SortOrder         *int          `json:"sort_order"`
}

type moveTestCaseRequest struct {
	FolderID  *int `json:"folder_id"`
	SortOrder int  `json:"sort_order"`
}

type testStepCreate struct {
	Action   string `json:"action"`
	Data     string `json:"data"`
	Expected string `json:"expected"`
}

type testStepPatch struct {
	Action   *string `json:"action"`
	Data     *string `json:"data"`
	Expected *string `json:"expected"`
}

type testLabelInput struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

type testLabelPatch struct {
	Name        *string `json:"name"`
	Color       *string `json:"color"`
	Description *string `json:"description"`
}

type labelReference struct {
	LabelID int `json:"label_id"`
}

func registerTestCaseRoutes(builder *routeBuilder, app *services.TestManagementApplicationService) {
	path := "/workspaces/{workspace_id}/test-cases"
	builder.WorkspaceCRUD(path, "test_case_id", "tests:read", "tests:write", workspaceCRUD[*models.TestCase, testCaseCreate, testCasePatch]{
		list: func(r *http.Request, userID, workspaceID int, page Pagination) ([]*models.TestCase, int, error) {
			folderID, all, err := testCaseFolder(r)
			if err != nil {
				return nil, 0, err
			}
			labelID, err := optionalPositiveQueryInt(r, "label_id")
			if err != nil {
				return nil, 0, err
			}
			items, total, err := app.ListCases(userID, services.TestCaseListParams{
				WorkspaceID: workspaceID, FolderID: folderID, All: all, Limit: page.PageSize,
				Offset: page.Offset, Search: r.URL.Query().Get("q"), LabelID: labelID,
			})
			return pointerSlice(items), total, err
		},
		get: func(_ *http.Request, userID, workspaceID, id int) (*models.TestCase, error) {
			return app.GetCase(userID, workspaceID, id)
		},
		create: func(r *http.Request, userID, workspaceID int, input testCaseCreate) (*models.TestCase, error) {
			return app.CreateCase(userID, workspaceID, auditActor(r, mustPrincipal(r)), services.TestCaseCreateRequest{
				Title: input.Title, Preconditions: input.Preconditions, Priority: input.Priority, Status: input.Status,
				EstimatedDuration: input.EstimatedDuration, FolderID: input.FolderID,
			})
		},
		patch: func(r *http.Request, userID, workspaceID, id int, input testCasePatch) (*models.TestCase, error) {
			return app.UpdateCase(userID, workspaceID, id, auditActor(r, mustPrincipal(r)), services.TestCasePatch{
				Title: input.Title, Preconditions: input.Preconditions, Priority: input.Priority, Status: input.Status,
				EstimatedDuration: input.EstimatedDuration, FolderID: optionalInt(input.FolderID), FolderIDSet: input.FolderID.Set, SortOrder: input.SortOrder,
			})
		},
		delete: func(r *http.Request, userID, workspaceID, id int) error {
			return app.DeleteCase(userID, workspaceID, id, auditActor(r, mustPrincipal(r)))
		},
	})
	builder.JSON(http.MethodPost, path+"/{test_case_id}/move", http.StatusOK, false, AuthAuthenticated, []string{"tests:write"}, func(r *http.Request, input moveTestCaseRequest) (map[string]bool, error) {
		user, workspaceID, id, err := testTarget(r, "test_case_id")
		if err != nil {
			return nil, err
		}
		return map[string]bool{"moved": true}, testManagementError(app.MoveCase(user.ID, workspaceID, id, input.FolderID, input.SortOrder))
	})
	builder.JSON(http.MethodPost, path+"/reorder", http.StatusOK, false, AuthAuthenticated, []string{"tests:write"}, func(r *http.Request, input reorderRequest) (map[string]bool, error) {
		user, workspaceID, _, err := testTarget(r, "")
		if err != nil {
			return nil, err
		}
		return map[string]bool{"reordered": true}, testManagementError(app.ReorderCases(user.ID, workspaceID, input.IDs))
	})
	builder.Read(path+"/{test_case_id}/connections", AuthAuthenticated, []string{"tests:read"}, func(r *http.Request) (any, error) {
		user, workspaceID, id, err := testTarget(r, "test_case_id")
		if err != nil {
			return nil, err
		}
		result, err := app.CaseConnections(user.ID, workspaceID, id)
		return result, testManagementError(err)
	})
	registerTestStepRoutes(builder, app, path+"/{test_case_id}/steps")
	registerTestLabelRoutes(builder, app, path)
}

func registerTestStepRoutes(builder *routeBuilder, app *services.TestManagementApplicationService, path string) {
	builder.Read(path, AuthAuthenticated, []string{"tests:read"}, func(r *http.Request) ([]models.TestStep, error) {
		user, workspaceID, caseID, err := testTarget(r, "test_case_id")
		if err != nil {
			return nil, err
		}
		result, err := app.ListSteps(user.ID, workspaceID, caseID)
		return result, testManagementError(err)
	})
	builder.JSON(http.MethodPost, path, http.StatusCreated, false, AuthAuthenticated, []string{"tests:write"}, func(r *http.Request, input testStepCreate) (*models.TestStep, error) {
		user, workspaceID, caseID, err := testTarget(r, "test_case_id")
		if err != nil {
			return nil, err
		}
		result, err := app.CreateStep(user.ID, workspaceID, caseID, services.TestStepCreateRequest{Action: input.Action, Data: input.Data, Expected: input.Expected})
		return result, testManagementError(err)
	})
	builder.JSON(http.MethodPatch, path+"/{step_id}", http.StatusOK, true, AuthAuthenticated, []string{"tests:write"}, func(r *http.Request, input testStepPatch) (*models.TestStep, error) {
		user, workspaceID, caseID, err := testTarget(r, "test_case_id")
		if err != nil {
			return nil, err
		}
		stepID, err := pathID(r, "step_id")
		if err != nil {
			return nil, err
		}
		result, err := app.UpdateStep(user.ID, workspaceID, caseID, stepID, services.TestStepPatch{Action: input.Action, Data: input.Data, Expected: input.Expected})
		return result, testManagementError(err)
	})
	builder.Command(http.MethodDelete, path+"/{step_id}", AuthAuthenticated, []string{"tests:write"}, func(r *http.Request) error {
		user, workspaceID, caseID, err := testTarget(r, "test_case_id")
		if err != nil {
			return err
		}
		stepID, err := pathID(r, "step_id")
		if err != nil {
			return err
		}
		return testManagementError(app.DeleteStep(user.ID, workspaceID, caseID, stepID))
	})
	builder.JSON(http.MethodPost, path+"/reorder", http.StatusOK, false, AuthAuthenticated, []string{"tests:write"}, func(r *http.Request, input reorderRequest) (map[string]bool, error) {
		user, workspaceID, caseID, err := testTarget(r, "test_case_id")
		if err != nil {
			return nil, err
		}
		return map[string]bool{"reordered": true}, testManagementError(app.ReorderSteps(user.ID, workspaceID, caseID, input.IDs))
	})
}

func registerTestLabelRoutes(builder *routeBuilder, app *services.TestManagementApplicationService, casePath string) {
	path := "/workspaces/{workspace_id}/test-labels"
	builder.Read(path, AuthAuthenticated, []string{"tests:read"}, func(r *http.Request) ([]models.TestLabel, error) {
		user, workspaceID, _, err := testTarget(r, "")
		if err != nil {
			return nil, err
		}
		result, err := app.ListLabels(user.ID, workspaceID)
		return result, testManagementError(err)
	})
	builder.JSON(http.MethodPost, path, http.StatusCreated, false, AuthAuthenticated, []string{"tests:write"}, func(r *http.Request, input testLabelInput) (*models.TestLabel, error) {
		user, workspaceID, _, err := testTarget(r, "")
		if err != nil {
			return nil, err
		}
		result, err := app.CreateLabel(user.ID, workspaceID, services.TestLabelCreateRequest{Name: input.Name, Color: input.Color, Description: input.Description})
		return result, testManagementError(err)
	})
	builder.JSON(http.MethodPatch, path+"/{label_id}", http.StatusOK, true, AuthAuthenticated, []string{"tests:write"}, func(r *http.Request, input testLabelPatch) (*models.TestLabel, error) {
		user, workspaceID, id, err := testTarget(r, "label_id")
		if err != nil {
			return nil, err
		}
		result, err := app.UpdateLabel(user.ID, workspaceID, id, services.TestLabelPatch{Name: input.Name, Color: input.Color, Description: input.Description})
		return result, testManagementError(err)
	})
	builder.Command(http.MethodDelete, path+"/{label_id}", AuthAuthenticated, []string{"tests:write"}, func(r *http.Request) error {
		user, workspaceID, id, err := testTarget(r, "label_id")
		if err != nil {
			return err
		}
		return testManagementError(app.DeleteLabel(user.ID, workspaceID, id))
	})
	labelsPath := casePath + "/{test_case_id}/labels"
	builder.Read(labelsPath, AuthAuthenticated, []string{"tests:read"}, func(r *http.Request) ([]models.TestLabel, error) {
		user, workspaceID, caseID, err := testTarget(r, "test_case_id")
		if err != nil {
			return nil, err
		}
		result, err := app.ListCaseLabels(user.ID, workspaceID, caseID)
		return result, testManagementError(err)
	})
	builder.JSON(http.MethodPost, labelsPath, http.StatusOK, false, AuthAuthenticated, []string{"tests:write"}, func(r *http.Request, input labelReference) (map[string]bool, error) {
		user, workspaceID, caseID, err := testTarget(r, "test_case_id")
		if err != nil {
			return nil, err
		}
		return map[string]bool{"linked": true}, testManagementError(app.AddCaseLabel(user.ID, workspaceID, caseID, input.LabelID))
	})
	builder.Command(http.MethodDelete, labelsPath+"/{label_id}", AuthAuthenticated, []string{"tests:write"}, func(r *http.Request) error {
		user, workspaceID, caseID, err := testTarget(r, "test_case_id")
		if err != nil {
			return err
		}
		labelID, err := pathID(r, "label_id")
		if err != nil {
			return err
		}
		return testManagementError(app.RemoveCaseLabel(user.ID, workspaceID, caseID, labelID))
	})
}

type testPlanCreate struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	MilestoneID *int   `json:"milestone_id"`
}
type testPlanPatch struct {
	Name        *string       `json:"name"`
	Description *string       `json:"description"`
	MilestoneID Optional[int] `json:"milestone_id"`
}
type testCaseReference struct {
	TestCaseID int `json:"test_case_id"`
}

func registerTestPlanRoutes(builder *routeBuilder, app *services.TestManagementApplicationService) {
	path := "/workspaces/{workspace_id}/test-plans"
	builder.WorkspaceCRUD(path, "plan_id", "tests:read", "tests:write", workspaceCRUD[*models.TestSet, testPlanCreate, testPlanPatch]{
		list: func(_ *http.Request, userID, workspaceID int, page Pagination) ([]*models.TestSet, int, error) {
			items, err := app.ListSets(userID, workspaceID)
			return pagePointers(items, page), len(items), err
		},
		get: func(_ *http.Request, userID, workspaceID, id int) (*models.TestSet, error) {
			return app.GetSet(userID, workspaceID, id)
		},
		create: func(r *http.Request, userID, workspaceID int, input testPlanCreate) (*models.TestSet, error) {
			return app.CreateSet(userID, workspaceID, auditActor(r, mustPrincipal(r)), models.TestSet{Name: input.Name, Description: input.Description, MilestoneID: input.MilestoneID})
		},
		patch: func(r *http.Request, userID, workspaceID, id int, input testPlanPatch) (*models.TestSet, error) {
			return app.UpdateSet(userID, workspaceID, id, auditActor(r, mustPrincipal(r)), services.TestSetPatch{Name: input.Name, Description: input.Description, MilestoneID: optionalInt(input.MilestoneID), MilestoneIDSet: input.MilestoneID.Set})
		},
		delete: func(r *http.Request, userID, workspaceID, id int) error {
			return app.DeleteSet(userID, workspaceID, id, auditActor(r, mustPrincipal(r)))
		},
	})
	relation := path + "/{plan_id}"
	builder.Read(relation+"/test-cases", AuthAuthenticated, []string{"tests:read"}, func(r *http.Request) ([]models.TestCase, error) {
		user, ws, id, err := testTarget(r, "plan_id")
		if err != nil {
			return nil, err
		}
		result, err := app.ListSetCases(user.ID, ws, id)
		return result, testManagementError(err)
	})
	builder.JSON(http.MethodPost, relation+"/test-cases", http.StatusOK, false, AuthAuthenticated, []string{"tests:write"}, func(r *http.Request, input testCaseReference) (map[string]bool, error) {
		user, ws, id, err := testTarget(r, "plan_id")
		if err != nil {
			return nil, err
		}
		return map[string]bool{"linked": true}, testManagementError(app.AddSetCase(user.ID, ws, id, input.TestCaseID))
	})
	builder.Command(http.MethodDelete, relation+"/test-cases/{test_case_id}", AuthAuthenticated, []string{"tests:write"}, func(r *http.Request) error {
		user, ws, id, err := testTarget(r, "plan_id")
		if err != nil {
			return err
		}
		caseID, err := pathID(r, "test_case_id")
		if err != nil {
			return err
		}
		return testManagementError(app.RemoveSetCase(user.ID, ws, id, caseID))
	})
	builder.Read(relation+"/runs", AuthAuthenticated, []string{"tests:read"}, func(r *http.Request) ([]models.TestRun, error) {
		user, ws, id, err := testTarget(r, "plan_id")
		if err != nil {
			return nil, err
		}
		result, err := app.ListSetRuns(user.ID, ws, id)
		return result, testManagementError(err)
	})
}

type testRunTemplateCreate struct {
	SetID       int    `json:"set_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}
type testRunTemplatePatch struct {
	SetID       *int    `json:"set_id"`
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

func registerTestRunTemplateRoutes(builder *routeBuilder, app *services.TestManagementApplicationService) {
	path := "/workspaces/{workspace_id}/test-run-templates"
	builder.WorkspaceCRUD(path, "template_id", "tests:read", "tests:write", workspaceCRUD[*models.TestRunTemplate, testRunTemplateCreate, testRunTemplatePatch]{
		list: func(_ *http.Request, userID, workspaceID int, page Pagination) ([]*models.TestRunTemplate, int, error) {
			items, err := app.ListTemplates(userID, workspaceID)
			return pagePointers(items, page), len(items), err
		},
		get: func(_ *http.Request, userID, workspaceID, id int) (*models.TestRunTemplate, error) {
			return app.GetTemplate(userID, workspaceID, id)
		},
		create: func(_ *http.Request, userID, workspaceID int, input testRunTemplateCreate) (*models.TestRunTemplate, error) {
			return app.CreateTemplate(userID, workspaceID, models.TestRunTemplate{SetID: input.SetID, Name: input.Name, Description: input.Description})
		},
		patch: func(_ *http.Request, userID, workspaceID, id int, input testRunTemplatePatch) (*models.TestRunTemplate, error) {
			return app.UpdateTemplate(userID, workspaceID, id, services.TestRunTemplatePatch{SetID: input.SetID, Name: input.Name, Description: input.Description})
		},
		delete: func(_ *http.Request, userID, workspaceID, id int) error {
			return app.DeleteTemplate(userID, workspaceID, id)
		},
	})
	item := path + "/{template_id}"
	builder.Read(item+"/executions", AuthAuthenticated, []string{"tests:read"}, func(r *http.Request) ([]models.TestRun, error) {
		user, ws, id, err := testTarget(r, "template_id")
		if err != nil {
			return nil, err
		}
		result, err := app.ListTemplateExecutions(user.ID, ws, id)
		return result, testManagementError(err)
	})
	builder.Action(http.MethodPost, item+"/execute", http.StatusCreated, AuthAuthenticated, []string{"tests:write"}, func(r *http.Request) (*models.TestRun, error) {
		user, ws, id, err := testTarget(r, "template_id")
		if err != nil {
			return nil, err
		}
		result, err := app.ExecuteTemplate(user.ID, ws, id)
		return result, testManagementError(err)
	})
}

type testRunCreate struct {
	Name       string `json:"name"`
	TemplateID int    `json:"template_id"`
	SetID      int    `json:"set_id"`
	AssigneeID *int   `json:"assignee_id"`
}
type testRunPatch struct {
	Name       *string       `json:"name"`
	AssigneeID Optional[int] `json:"assignee_id"`
}
type testResultPatch struct {
	Status       string `json:"status"`
	ActualResult string `json:"actual_result"`
	Notes        string `json:"notes"`
}
type testStepResultPatch struct {
	Status       string `json:"status"`
	ActualResult string `json:"actual_result"`
	Notes        string `json:"notes"`
	ItemID       *int   `json:"item_id"`
}
type itemReference struct {
	ItemID int `json:"item_id"`
}

func registerTestRunRoutes(builder *routeBuilder, app *services.TestManagementApplicationService) {
	path := "/workspaces/{workspace_id}/test-runs"
	builder.WorkspaceCRUD(path, "run_id", "tests:read", "tests:write", workspaceCRUD[*models.TestRun, testRunCreate, testRunPatch]{
		list: func(r *http.Request, userID, workspaceID int, page Pagination) ([]*models.TestRun, int, error) {
			filters, err := testRunFilters(r)
			if err != nil {
				return nil, 0, err
			}
			items, err := app.ListRuns(userID, workspaceID, filters)
			return pagePointers(items, page), len(items), err
		},
		get: func(_ *http.Request, userID, workspaceID, id int) (*models.TestRun, error) {
			return app.GetRun(userID, workspaceID, id)
		},
		create: func(r *http.Request, userID, workspaceID int, input testRunCreate) (*models.TestRun, error) {
			return app.CreateRun(userID, workspaceID, auditActor(r, mustPrincipal(r)), services.TestRunCreateRequest{Name: input.Name, TemplateID: input.TemplateID, SetID: input.SetID, AssigneeID: input.AssigneeID})
		},
		patch: func(r *http.Request, userID, workspaceID, id int, input testRunPatch) (*models.TestRun, error) {
			return app.UpdateRun(userID, workspaceID, id, auditActor(r, mustPrincipal(r)), services.TestRunPatch{Name: input.Name, AssigneeID: optionalInt(input.AssigneeID), AssigneeIDSet: input.AssigneeID.Set})
		},
		delete: func(r *http.Request, userID, workspaceID, id int) error {
			return app.DeleteRun(userID, workspaceID, id, auditActor(r, mustPrincipal(r)))
		},
	})
	item := path + "/{run_id}"
	builder.Read(item+"/detail", AuthAuthenticated, []string{"tests:read"}, func(r *http.Request) (*services.TestRunDetail, error) {
		user, ws, id, err := testTarget(r, "run_id")
		if err != nil {
			return nil, err
		}
		result, err := app.GetRunDetail(user.ID, ws, id)
		return result, testManagementError(err)
	})
	builder.Action(http.MethodPost, item+"/end", http.StatusOK, AuthAuthenticated, []string{"tests:write"}, func(r *http.Request) (map[string]bool, error) {
		user, ws, id, err := testTarget(r, "run_id")
		if err != nil {
			return nil, err
		}
		return map[string]bool{"ended": true}, testManagementError(app.EndRun(user.ID, ws, id))
	})
	builder.Read(item+"/results", AuthAuthenticated, []string{"tests:read"}, func(r *http.Request) ([]services.TestRunResultWithCaseTitle, error) {
		user, ws, id, err := testTarget(r, "run_id")
		if err != nil {
			return nil, err
		}
		result, err := app.ListResults(user.ID, ws, id)
		return result, testManagementError(err)
	})
	builder.JSON(http.MethodPatch, item+"/results/{result_id}", http.StatusOK, true, AuthAuthenticated, []string{"tests:write"}, func(r *http.Request, input testResultPatch) (*models.TestResult, error) {
		user, ws, runID, err := testTarget(r, "run_id")
		if err != nil {
			return nil, err
		}
		resultID, err := pathID(r, "result_id")
		if err != nil {
			return nil, err
		}
		result, err := app.UpdateResult(user.ID, ws, runID, resultID, services.TestResultUpdateRequest{Status: input.Status, ActualResult: input.ActualResult, Notes: input.Notes})
		return result, testManagementError(err)
	})
	builder.Read(item+"/steps", AuthAuthenticated, []string{"tests:read"}, func(r *http.Request) (map[string]services.TestRunStepResult, error) {
		user, ws, id, err := testTarget(r, "run_id")
		if err != nil {
			return nil, err
		}
		result, err := app.ListStepResults(user.ID, ws, id)
		return result, testManagementError(err)
	})
	builder.Read(item+"/summary", AuthAuthenticated, []string{"tests:read"}, func(r *http.Request) (map[string]string, error) {
		user, ws, id, err := testTarget(r, "run_id")
		if err != nil {
			return nil, err
		}
		result, err := app.RunMarkdownSummary(user.ID, ws, id)
		return result, testManagementError(err)
	})
	builder.JSON(http.MethodPatch, item+"/steps/{step_id}", http.StatusOK, true, AuthAuthenticated, []string{"tests:write"}, func(r *http.Request, input testStepResultPatch) (map[string]bool, error) {
		user, ws, runID, err := testTarget(r, "run_id")
		if err != nil {
			return nil, err
		}
		stepID, err := pathID(r, "step_id")
		if err != nil {
			return nil, err
		}
		err = app.UpdateStepResult(user.ID, ws, runID, stepID, services.TestStepResultUpdateRequest{Status: input.Status, ActualResult: input.ActualResult, Notes: input.Notes, ItemID: input.ItemID})
		return map[string]bool{"updated": true}, testManagementError(err)
	})
	resultPath := "/workspaces/{workspace_id}/test-results/{result_id}/items"
	builder.Read(resultPath, AuthAuthenticated, []string{"tests:read"}, func(r *http.Request) ([]models.Item, error) {
		user, ws, id, err := testTarget(r, "result_id")
		if err != nil {
			return nil, err
		}
		result, err := app.ListResultItems(user.ID, ws, id)
		return result, testManagementError(err)
	})
	builder.JSON(http.MethodPost, resultPath, http.StatusOK, false, AuthAuthenticated, []string{"tests:write"}, func(r *http.Request, input itemReference) (map[string]bool, error) {
		user, ws, id, err := testTarget(r, "result_id")
		if err != nil {
			return nil, err
		}
		return map[string]bool{"linked": true}, testManagementError(app.LinkResultItem(user.ID, ws, id, input.ItemID))
	})
	builder.Command(http.MethodDelete, resultPath+"/{item_id}", AuthAuthenticated, []string{"tests:write"}, func(r *http.Request) error {
		user, ws, id, err := testTarget(r, "result_id")
		if err != nil {
			return err
		}
		itemID, err := pathID(r, "item_id")
		if err != nil {
			return err
		}
		return testManagementError(app.UnlinkResultItem(user.ID, ws, id, itemID))
	})
}

func registerTestReportRoutes(builder *routeBuilder, app *services.TestManagementApplicationService) {
	builder.Read("/workspaces/{workspace_id}/test-reports/summary", AuthAuthenticated, []string{"tests:read"}, func(r *http.Request) (map[string]any, error) {
		user, workspaceID, _, err := testTarget(r, "")
		if err != nil {
			return nil, err
		}
		milestoneID, err := optionalPositiveQueryInt(r, "milestone_id")
		if err != nil {
			return nil, err
		}
		days, err := parsePositiveInt(r, "days", 30, 365)
		if err != nil {
			return nil, err
		}
		result, err := app.ReportsSummary(user.ID, workspaceID, milestoneID, days)
		return result, testManagementError(err)
	})
}

type coverageConfigRequest struct {
	RequirementItemTypeIDs []int `json:"requirement_item_type_ids"`
}

type coverageMeta struct {
	Summary models.TestCoverageSummary `json:"summary"`
}

func registerTestCoverageRoutes(builder *routeBuilder, app *services.TestManagementApplicationService) {
	registerTestCoverageScope(builder, app, "/workspaces/{workspace_id}/test-coverage", func(r *http.Request) (services.TestCoverageScope, error) {
		workspaceID, err := pathID(r, "workspace_id")
		return services.TestCoverageScope{WorkspaceID: workspaceID}, err
	})
	registerTestCoverageScope(builder, app, "/collections/{collection_id}/test-coverage", func(r *http.Request) (services.TestCoverageScope, error) {
		collectionID, err := pathID(r, "collection_id")
		return services.TestCoverageScope{CollectionID: &collectionID}, err
	})
}

func registerTestCoverageScope(builder *routeBuilder, app *services.TestManagementApplicationService, path string, scope func(*http.Request) (services.TestCoverageScope, error)) {
	builder.Read(path+"/config", AuthAuthenticated, []string{"tests:read"}, func(r *http.Request) (*models.TestCoverageConfiguration, error) {
		user, err := principal(r)
		if err != nil {
			return nil, err
		}
		target, err := scope(r)
		if err != nil {
			return nil, err
		}
		result, err := app.CoverageConfig(user.ID, target)
		return result, testManagementError(err)
	})
	builder.JSON(http.MethodPost, path+"/config", http.StatusCreated, false, AuthAuthenticated, []string{"tests:write"}, func(r *http.Request, input coverageConfigRequest) (*models.TestCoverageConfiguration, error) {
		user, err := principal(r)
		if err != nil {
			return nil, err
		}
		target, err := scope(r)
		if err != nil {
			return nil, err
		}
		result, err := app.CreateCoverageConfig(user.ID, target, input.RequirementItemTypeIDs)
		return result, testManagementError(err)
	})
	builder.JSON(http.MethodPatch, path+"/config/{config_id}", http.StatusOK, true, AuthAuthenticated, []string{"tests:write"}, func(r *http.Request, input coverageConfigRequest) (*models.TestCoverageConfiguration, error) {
		user, err := principal(r)
		if err != nil {
			return nil, err
		}
		target, err := scope(r)
		if err != nil {
			return nil, err
		}
		configID, err := pathID(r, "config_id")
		if err != nil {
			return nil, err
		}
		result, err := app.UpdateCoverageConfig(user.ID, target, configID, input.RequirementItemTypeIDs)
		return result, testManagementError(err)
	})
	builder.Command(http.MethodDelete, path+"/config/{config_id}", AuthAuthenticated, []string{"tests:write"}, func(r *http.Request) error {
		user, err := principal(r)
		if err != nil {
			return err
		}
		target, err := scope(r)
		if err != nil {
			return err
		}
		configID, err := pathID(r, "config_id")
		if err != nil {
			return err
		}
		return testManagementError(app.DeleteCoverageConfig(user.ID, target, configID))
	})
	builder.Read(path+"/summary", AuthAuthenticated, []string{"tests:read"}, func(r *http.Request) (models.TestCoverageSummary, error) {
		user, err := principal(r)
		if err != nil {
			return models.TestCoverageSummary{}, err
		}
		target, err := scope(r)
		if err != nil {
			return models.TestCoverageSummary{}, err
		}
		result, err := app.CoverageSummary(user.ID, target)
		return result, testManagementError(err)
	})
	builder.PageMetadata(path+"/requirements", AuthAuthenticated, []string{"tests:read"}, func(r *http.Request) ([]models.RequirementCoverageItem, Pagination, int, coverageMeta, error) {
		user, err := principal(r)
		if err != nil {
			return nil, Pagination{}, 0, coverageMeta{}, err
		}
		target, err := scope(r)
		if err != nil {
			return nil, Pagination{}, 0, coverageMeta{}, err
		}
		page, err := ParsePage(r)
		if err != nil {
			return nil, Pagination{}, 0, coverageMeta{}, err
		}
		itemTypeID, err := optionalPositiveQueryInt(r, "item_type_id")
		if err != nil {
			return nil, Pagination{}, 0, coverageMeta{}, err
		}
		items, total, summary, err := app.CoverageRequirements(user.ID, target, services.TestCoverageRequirementsFilter{
			Covered: r.URL.Query().Get("covered"), ItemTypeID: itemTypeID, Search: r.URL.Query().Get("search"), Limit: page.PageSize, Offset: page.Offset,
		})
		return items, page, total, coverageMeta{Summary: summary}, testManagementError(err)
	})
}

func testTarget(r *http.Request, idParam string) (user *models.User, workspaceID, id int, err error) {
	user, err = principal(r)
	if err != nil {
		return nil, 0, 0, err
	}
	workspaceID, err = pathID(r, "workspace_id")
	if err != nil {
		return nil, 0, 0, err
	}
	if idParam == "" {
		return user, workspaceID, 0, nil
	}
	id, err = pathID(r, idParam)
	return user, workspaceID, id, err
}

func mustPrincipal(r *http.Request) *models.User { user, _ := principal(r); return user }

func pagePointers[T any](items []T, page Pagination) []*T {
	return pointerSlice(pageValues(items, page))
}
func pointerSlice[T any](items []T) []*T {
	result := make([]*T, len(items))
	for i := range items {
		result[i] = &items[i]
	}
	return result
}
func pageValues[T any](items []T, page Pagination) []T {
	start := min(page.Offset, len(items))
	end := min(start+page.PageSize, len(items))
	return items[start:end]
}
func valueOrZero(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func testCaseFolder(r *http.Request) (folderID *int, all bool, err error) {
	if r.URL.Query().Get("all") == "true" {
		return nil, true, nil
	}
	raw, exists := r.URL.Query()["folder_id"]
	if !exists || len(raw) == 0 || raw[0] == "null" {
		return nil, false, nil
	}
	id, err := strconv.Atoi(raw[0])
	if err != nil || id <= 0 {
		return nil, false, newError(http.StatusBadRequest, "invalid_request", "folder_id must be a positive integer or null")
	}
	return &id, false, nil
}

func optionalPositiveQueryInt(r *http.Request, name string) (*int, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return nil, nil
	}
	id, err := strconv.Atoi(raw)
	if err != nil || id <= 0 {
		return nil, newError(http.StatusBadRequest, "invalid_request", name+" must be a positive integer")
	}
	return &id, nil
}

func testRunFilters(r *http.Request) (services.TestRunListFilters, error) {
	assigneeID, err := optionalPositiveQueryInt(r, "assignee_id")
	if err != nil {
		return services.TestRunListFilters{}, err
	}
	templateID, err := optionalPositiveQueryInt(r, "template_id")
	if err != nil {
		return services.TestRunListFilters{}, err
	}
	setID, err := optionalPositiveQueryInt(r, "set_id")
	if err != nil {
		return services.TestRunListFilters{}, err
	}
	return services.TestRunListFilters{AssigneeID: assigneeID, Unassigned: r.URL.Query().Get("unassigned") == "true", TemplateID: templateID, SetID: setID, IncludeEnded: r.URL.Query().Get("include_ended") == "true"}, nil
}

func testManagementError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, services.ErrTestManagementForbidden):
		return newError(http.StatusNotFound, "not_found", "Test resource was not found")
	case errors.Is(err, repository.ErrNotFound), errors.Is(err, services.ErrTestRunItemNotFound), errors.Is(err, services.ErrTestSetCaseNotFound), errors.Is(err, services.ErrTestSetMilestoneNotFound), errors.Is(err, services.ErrTestRunTemplateSetNotFound):
		return newError(http.StatusNotFound, "not_found", "Test resource was not found")
	default:
		return newError(http.StatusBadRequest, "invalid_request", err.Error())
	}
}
