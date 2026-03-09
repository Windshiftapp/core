package routes

import (
	"net/http"

	"windshift/internal/models"
)

// RegisterTestManagementRoutes registers test management routes.
func RegisterTestManagementRoutes(deps *Deps) {
	api := deps.API
	auth := deps.AuthMiddleware.RequireAuth
	pm := deps.PermissionMiddleware
	testView := pm.RequireWorkspacePermission(models.PermissionTestView)
	testManage := pm.RequireWorkspacePermission(models.PermissionTestManage)
	testExecute := pm.RequireWorkspacePermission(models.PermissionTestExecute)

	// Test folders
	api.HandleH("GET /workspaces/{workspaceId}/test-folders", auth(testView(http.HandlerFunc(deps.TestMgmt.Folder.GetAllFolders))))
	api.HandleH("POST /workspaces/{workspaceId}/test-folders", auth(testManage(http.HandlerFunc(deps.TestMgmt.Folder.CreateFolder))))
	api.HandleH("GET /workspaces/{workspaceId}/test-folders/{id}", auth(testView(http.HandlerFunc(deps.TestMgmt.Folder.GetFolder))))
	api.HandleH("PUT /workspaces/{workspaceId}/test-folders/{id}", auth(testManage(http.HandlerFunc(deps.TestMgmt.Folder.UpdateFolder))))
	api.HandleH("DELETE /workspaces/{workspaceId}/test-folders/{id}", auth(testManage(http.HandlerFunc(deps.TestMgmt.Folder.DeleteFolder))))
	api.HandleH("PUT /workspaces/{workspaceId}/test-folders/reorder", auth(testManage(http.HandlerFunc(deps.TestMgmt.Folder.ReorderFolders))))

	// Test cases
	api.HandleH("GET /workspaces/{workspaceId}/test-cases", auth(testView(http.HandlerFunc(deps.TestMgmt.Case.GetAllTestCases))))
	api.HandleH("POST /workspaces/{workspaceId}/test-cases", auth(testManage(http.HandlerFunc(deps.TestMgmt.Case.CreateTestCase))))
	api.HandleH("GET /workspaces/{workspaceId}/test-cases/{id}", auth(testView(http.HandlerFunc(deps.TestMgmt.Case.GetTestCase))))
	api.HandleH("PUT /workspaces/{workspaceId}/test-cases/{id}", auth(testManage(http.HandlerFunc(deps.TestMgmt.Case.UpdateTestCase))))
	api.HandleH("DELETE /workspaces/{workspaceId}/test-cases/{id}", auth(testManage(http.HandlerFunc(deps.TestMgmt.Case.DeleteTestCase))))
	api.HandleH("PUT /workspaces/{workspaceId}/test-cases/{id}/move", auth(testManage(http.HandlerFunc(deps.TestMgmt.Case.MoveTestCase))))
	api.HandleH("PUT /workspaces/{workspaceId}/test-cases/reorder", auth(testManage(http.HandlerFunc(deps.TestMgmt.Case.ReorderTestCases))))
	api.HandleH("GET /workspaces/{workspaceId}/test-cases/{id}/connections", auth(testView(http.HandlerFunc(deps.TestMgmt.Case.GetTestCaseConnections))))

	// Test case steps
	api.HandleH("GET /workspaces/{workspaceId}/test-cases/{testCaseId}/steps", auth(testView(http.HandlerFunc(deps.TestMgmt.Case.GetTestSteps))))
	api.HandleH("POST /workspaces/{workspaceId}/test-cases/{testCaseId}/steps", auth(testManage(http.HandlerFunc(deps.TestMgmt.Case.CreateTestStep))))
	api.HandleH("PUT /workspaces/{workspaceId}/test-cases/{testCaseId}/steps/{stepId}", auth(testManage(http.HandlerFunc(deps.TestMgmt.Case.UpdateTestStep))))
	api.HandleH("DELETE /workspaces/{workspaceId}/test-cases/{testCaseId}/steps/{stepId}", auth(testManage(http.HandlerFunc(deps.TestMgmt.Case.DeleteTestStep))))
	api.HandleH("PUT /workspaces/{workspaceId}/test-cases/{testCaseId}/steps/reorder", auth(testManage(http.HandlerFunc(deps.TestMgmt.Case.ReorderTestSteps))))

	// Test labels
	api.HandleH("GET /workspaces/{workspaceId}/test-labels", auth(testView(http.HandlerFunc(deps.TestMgmt.Case.GetAllTestLabels))))
	api.HandleH("POST /workspaces/{workspaceId}/test-labels", auth(testManage(http.HandlerFunc(deps.TestMgmt.Case.CreateTestLabel))))
	api.HandleH("PUT /workspaces/{workspaceId}/test-labels/{labelId}", auth(testManage(http.HandlerFunc(deps.TestMgmt.Case.UpdateTestLabel))))
	api.HandleH("DELETE /workspaces/{workspaceId}/test-labels/{labelId}", auth(testManage(http.HandlerFunc(deps.TestMgmt.Case.DeleteTestLabel))))

	// Test case labels
	api.HandleH("GET /workspaces/{workspaceId}/test-cases/{testCaseId}/labels", auth(testView(http.HandlerFunc(deps.TestMgmt.Case.GetTestCaseLabels))))
	api.HandleH("POST /workspaces/{workspaceId}/test-cases/{testCaseId}/labels", auth(testManage(http.HandlerFunc(deps.TestMgmt.Case.AddTestCaseLabel))))
	api.HandleH("DELETE /workspaces/{workspaceId}/test-cases/{testCaseId}/labels/{labelId}", auth(testManage(http.HandlerFunc(deps.TestMgmt.Case.RemoveTestCaseLabel))))

	// Test sets
	api.HandleH("GET /workspaces/{workspaceId}/test-sets", auth(testView(http.HandlerFunc(deps.TestMgmt.Set.GetAll))))
	api.HandleH("POST /workspaces/{workspaceId}/test-sets", auth(testManage(http.HandlerFunc(deps.TestMgmt.Set.Create))))
	api.HandleH("GET /workspaces/{workspaceId}/test-sets/{id}", auth(testView(http.HandlerFunc(deps.TestMgmt.Set.Get))))
	api.HandleH("PUT /workspaces/{workspaceId}/test-sets/{id}", auth(testManage(http.HandlerFunc(deps.TestMgmt.Set.Update))))
	api.HandleH("DELETE /workspaces/{workspaceId}/test-sets/{id}", auth(testManage(http.HandlerFunc(deps.TestMgmt.Set.Delete))))
	api.HandleH("GET /workspaces/{workspaceId}/test-sets/{id}/test-cases", auth(testView(http.HandlerFunc(deps.TestMgmt.Set.GetTestCases))))
	api.HandleH("POST /workspaces/{workspaceId}/test-sets/{id}/test-cases", auth(testManage(http.HandlerFunc(deps.TestMgmt.Set.AddTestCase))))
	api.HandleH("DELETE /workspaces/{workspaceId}/test-sets/{id}/test-cases/{testCaseId}", auth(testManage(http.HandlerFunc(deps.TestMgmt.Set.RemoveTestCase))))
	api.HandleH("GET /workspaces/{workspaceId}/test-sets/{id}/runs", auth(testView(http.HandlerFunc(deps.TestMgmt.Set.GetRuns))))

	// Test plans (alias for test sets)
	api.HandleH("GET /workspaces/{workspaceId}/test-plans", auth(testView(http.HandlerFunc(deps.TestMgmt.Set.GetAll))))
	api.HandleH("POST /workspaces/{workspaceId}/test-plans", auth(testManage(http.HandlerFunc(deps.TestMgmt.Set.Create))))
	api.HandleH("GET /workspaces/{workspaceId}/test-plans/{id}", auth(testView(http.HandlerFunc(deps.TestMgmt.Set.Get))))
	api.HandleH("PUT /workspaces/{workspaceId}/test-plans/{id}", auth(testManage(http.HandlerFunc(deps.TestMgmt.Set.Update))))
	api.HandleH("DELETE /workspaces/{workspaceId}/test-plans/{id}", auth(testManage(http.HandlerFunc(deps.TestMgmt.Set.Delete))))
	api.HandleH("GET /workspaces/{workspaceId}/test-plans/{id}/test-cases", auth(testView(http.HandlerFunc(deps.TestMgmt.Set.GetTestCases))))
	api.HandleH("POST /workspaces/{workspaceId}/test-plans/{id}/test-cases", auth(testManage(http.HandlerFunc(deps.TestMgmt.Set.AddTestCase))))
	api.HandleH("DELETE /workspaces/{workspaceId}/test-plans/{id}/test-cases/{testCaseId}", auth(testManage(http.HandlerFunc(deps.TestMgmt.Set.RemoveTestCase))))
	api.HandleH("GET /workspaces/{workspaceId}/test-plans/{id}/runs", auth(testView(http.HandlerFunc(deps.TestMgmt.Set.GetRuns))))

	// Test run templates
	api.HandleH("GET /workspaces/{workspaceId}/test-run-templates", auth(testView(http.HandlerFunc(deps.TestMgmt.RunTemplate.GetAll))))
	api.HandleH("POST /workspaces/{workspaceId}/test-run-templates", auth(testManage(http.HandlerFunc(deps.TestMgmt.RunTemplate.Create))))
	api.HandleH("GET /workspaces/{workspaceId}/test-run-templates/{id}", auth(testView(http.HandlerFunc(deps.TestMgmt.RunTemplate.Get))))
	api.HandleH("PUT /workspaces/{workspaceId}/test-run-templates/{id}", auth(testManage(http.HandlerFunc(deps.TestMgmt.RunTemplate.Update))))
	api.HandleH("DELETE /workspaces/{workspaceId}/test-run-templates/{id}", auth(testManage(http.HandlerFunc(deps.TestMgmt.RunTemplate.Delete))))
	api.HandleH("GET /workspaces/{workspaceId}/test-run-templates/{id}/executions", auth(testView(http.HandlerFunc(deps.TestMgmt.RunTemplate.GetExecutions))))
	api.HandleH("POST /workspaces/{workspaceId}/test-run-templates/{id}/execute", auth(testExecute(http.HandlerFunc(deps.TestMgmt.RunTemplate.Execute))))

	// Test runs
	api.HandleH("GET /workspaces/{workspaceId}/test-runs", auth(testView(http.HandlerFunc(deps.TestMgmt.Run.GetAll))))
	api.HandleH("POST /workspaces/{workspaceId}/test-runs", auth(testExecute(http.HandlerFunc(deps.TestMgmt.Run.Create))))
	api.HandleH("GET /workspaces/{workspaceId}/test-runs/{id}", auth(testView(http.HandlerFunc(deps.TestMgmt.Run.Get))))
	api.HandleH("PUT /workspaces/{workspaceId}/test-runs/{id}", auth(testExecute(http.HandlerFunc(deps.TestMgmt.Run.Update))))
	api.HandleH("DELETE /workspaces/{workspaceId}/test-runs/{id}", auth(testManage(http.HandlerFunc(deps.TestMgmt.Run.Delete))))
	api.HandleH("POST /workspaces/{workspaceId}/test-runs/{id}/end", auth(testExecute(http.HandlerFunc(deps.TestMgmt.Run.End))))
	api.HandleH("GET /workspaces/{workspaceId}/test-runs/{id}/results", auth(testView(http.HandlerFunc(deps.TestMgmt.Run.GetResults))))
	api.HandleH("PUT /workspaces/{workspaceId}/test-runs/{id}/results/{resultId}", auth(testExecute(http.HandlerFunc(deps.TestMgmt.Run.UpdateResult))))
	api.HandleH("GET /workspaces/{workspaceId}/test-runs/{id}/steps", auth(testView(http.HandlerFunc(deps.TestMgmt.Run.GetStepResults))))
	api.HandleH("PUT /workspaces/{workspaceId}/test-runs/{id}/steps/{stepId}", auth(testExecute(http.HandlerFunc(deps.TestMgmt.Run.UpdateStepResult))))
	api.HandleH("GET /workspaces/{workspaceId}/test-runs/{id}/summary", auth(testView(http.HandlerFunc(deps.TestMgmt.Summary.GetMarkdownSummary))))

	// Test reports dashboard endpoint
	api.HandleH("GET /workspaces/{workspaceId}/test-reports/summary", auth(testView(http.HandlerFunc(deps.TestMgmt.Summary.GetReportsSummary))))

	// Test result item linking
	api.HandleH("POST /workspaces/{workspaceId}/test-results/{resultId}/items", auth(testExecute(http.HandlerFunc(deps.TestMgmt.Run.LinkItemToTestResult))))
	api.HandleH("DELETE /workspaces/{workspaceId}/test-results/{resultId}/items/{itemId}", auth(testExecute(http.HandlerFunc(deps.TestMgmt.Run.UnlinkItemFromTestResult))))
	api.HandleH("GET /workspaces/{workspaceId}/test-results/{resultId}/items", auth(testView(http.HandlerFunc(deps.TestMgmt.Run.GetTestResultItems))))

	// Test case links (using itemLinkHandler)
	api.HandleH("GET /test-cases/{id}/links", auth(http.HandlerFunc(deps.Items.ItemLink.GetLinksForItem)))
}
