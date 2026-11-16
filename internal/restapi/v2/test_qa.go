package v2

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"windshift/internal/repository"
	"windshift/internal/services"
)

func registerTestQARoutes(builder *routeBuilder, app *services.TestManagementApplicationService) {
	builder.JSON(http.MethodPost, "/workspaces/{workspace_id}/test-cases/bulk", http.StatusOK, false, AuthAuthenticated, []string{"tests:write"}, func(r *http.Request, input services.TestCaseBulkRequest) (services.TestCaseBulkResult, error) {
		user, workspaceID, _, err := testTarget(r, "")
		if err != nil {
			return services.TestCaseBulkResult{}, err
		}
		result, err := app.BulkCases(user.ID, workspaceID, auditActor(r, user), input)
		return result, testQAError(err)
	})
	builder.Read("/workspaces/{workspace_id}/test-coverage/items/{item_id}", AuthAuthenticated, []string{"tests:read"}, func(r *http.Request) (services.ItemTestCoverage, error) {
		user, workspaceID, itemID, err := testTarget(r, "item_id")
		if err != nil {
			return services.ItemTestCoverage{}, err
		}
		result, err := app.ItemCoverage(user.ID, workspaceID, itemID)
		return result, testQAError(err)
	})
	builder.Read("/workspaces/{workspace_id}/test-reports/effectiveness", AuthAuthenticated, []string{"tests:read"}, func(r *http.Request) (services.TestEffectivenessReport, error) {
		user, workspaceID, _, err := testTarget(r, "")
		if err != nil {
			return services.TestEffectivenessReport{}, err
		}
		filter, err := testEffectivenessFilter(r, workspaceID)
		if err != nil {
			return services.TestEffectivenessReport{}, err
		}
		result, err := app.Effectiveness(user.ID, filter)
		return result, testQAError(err)
	})
}

func testQAError(err error) error {
	if errors.Is(err, services.ErrTestManagementForbidden) || errors.Is(err, repository.ErrNotFound) {
		return newError(http.StatusNotFound, "not_found", "Resource not found")
	}
	return testManagementError(err)
}

func testEffectivenessFilter(r *http.Request, workspaceID int) (repository.TestEffectivenessFilter, error) {
	filter := repository.TestEffectivenessFilter{WorkspaceID: workspaceID}
	var err error
	filter.MilestoneID, err = optionalPositiveQueryInt(r, "milestone_id")
	if err != nil {
		return filter, err
	}
	for name, target := range map[string]*time.Time{"since": &filter.Since, "until": &filter.Until} {
		if raw := r.URL.Query().Get(name); raw != "" {
			*target, err = time.Parse(time.RFC3339, raw)
			if err != nil {
				return filter, newError(http.StatusBadRequest, "invalid_request", name+" must be RFC3339")
			}
		}
	}
	for name, target := range map[string]*int{"limit": &filter.Limit, "offset": &filter.Offset} {
		if raw := r.URL.Query().Get(name); raw != "" {
			*target, err = strconv.Atoi(raw)
			if err != nil || (name == "limit" && *target <= 0) || (name == "offset" && *target < 0) {
				return filter, newError(http.StatusBadRequest, "invalid_request", name+" is outside its valid range")
			}
		}
	}
	return filter, nil
}
