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

func registerOverviewChartRoutes(builder *routeBuilder, deps Deps) {
	builder.Read("/workspaces/{workspace_id}/overview-charts", AuthAuthenticated, []string{"items:read"}, overviewCharts(deps.ItemApplication))
}

func overviewCharts(app *services.ItemApplicationService) readOperation[*models.OverviewCharts] {
	return func(r *http.Request) (*models.OverviewCharts, error) {
		user, workspaceID, err := principalAndWorkspace(r)
		if err != nil {
			return nil, err
		}
		q := services.OverviewChartsQuery{UserID: user.ID, WorkspaceID: workspaceID}
		q.From, err = time.Parse(time.DateOnly, r.URL.Query().Get("from"))
		if err != nil {
			return nil, newError(http.StatusBadRequest, "invalid_request", "from is required and must use YYYY-MM-DD")
		}
		to, err := time.Parse(time.DateOnly, r.URL.Query().Get("to"))
		if err != nil {
			return nil, newError(http.StatusBadRequest, "invalid_request", "to is required and must use YYYY-MM-DD")
		}
		q.To = to.AddDate(0, 0, 1)
		q.CollectionID, err = optionalPositiveQueryID(r, "collection_id")
		if err != nil {
			return nil, err
		}
		series := r.URL.Query().Get("series")
		if !r.URL.Query().Has("series") {
			series = "created,completed"
		}
		for _, name := range strings.Split(series, ",") {
			switch name {
			case "created":
				q.Created = true
			case "completed":
				q.Completed = true
			default:
				return nil, newError(http.StatusBadRequest, "invalid_request", "series must contain created and/or completed")
			}
		}
		result, err := app.OverviewCharts(r.Context(), q)
		switch {
		case errors.Is(err, services.ErrOverviewChartRange):
			return nil, newError(http.StatusBadRequest, "invalid_request", err.Error())
		case errors.Is(err, repository.ErrNotFound):
			return nil, newError(http.StatusNotFound, "not_found", "Overview scope not found")
		case errors.Is(err, services.ErrQLQuery):
			return nil, newError(http.StatusBadRequest, "invalid_request", "Collection query is invalid")
		case err != nil:
			return nil, internalError(err)
		default:
			return result, nil
		}
	}
}
