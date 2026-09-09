package v2

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"windshift/internal/cql"
	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/services"
)

func searchItems(app *services.ItemApplicationService, timeout time.Duration) pageOperation[models.Item] {
	if timeout <= 0 {
		timeout = 12 * time.Second
	}
	return func(r *http.Request) ([]models.Item, Pagination, int, error) {
		user, err := principal(r)
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		page, request, err := parseItemSearch(r, user.ID)
		if err != nil {
			return nil, Pagination{}, 0, err
		}
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		result, err := app.List(ctx, request)
		if err != nil {
			database.ObserveRequestQueryError(err)
			if errors.Is(err, context.DeadlineExceeded) {
				return nil, page, 0, newError(http.StatusGatewayTimeout, "database_deadline_exceeded", "The database request timed out.")
			}
		}
		return result.Items, page, result.Total, itemError(err)
	}
}

func parseItemSearch(r *http.Request, userID int) (Pagination, services.ItemListRequest, error) {
	page, err := ParsePage(r)
	if err != nil {
		return Pagination{}, services.ItemListRequest{}, err
	}
	query := r.URL.Query()
	text := strings.TrimSpace(query.Get("q"))
	ql := strings.TrimSpace(query.Get("ql"))
	request := services.ItemListRequest{
		UserID: userID, QL: ql, SortBy: "updated_at", ExcludePersonal: excludePersonal(r),
		Pagination: services.PaginationParams{Limit: page.PageSize, Offset: page.Offset},
	}
	if len(text) > 500 {
		return page, request, invalidQuery("q")
	}
	if len(ql) > 500 {
		return page, request, invalidQuery("ql")
	}
	for _, filter := range []struct {
		name   string
		max    int
		target *[]int
	}{
		{"workspace_id", 50, &request.WorkspaceIDs},
		{"status", 20, &request.Filters.StatusIDs},
		{"priority", 10, &request.Filters.PriorityIDs},
	} {
		values := query[filter.name]
		if len(values) > filter.max {
			return page, request, invalidQuery(filter.name)
		}
		for _, value := range values {
			id, err := strconv.Atoi(value)
			if err != nil || id < 1 {
				return page, request, invalidQuery(filter.name)
			}
			*filter.target = append(*filter.target, id)
		}
	}
	if request.QL == "" {
		if cql.LooksLikeQuery(text) {
			request.QL = text
		} else if reference, err := parseItemReference(text); err == nil && reference.WorkspaceKey != "" {
			request.Filters.ItemKeyQuery = text
		} else {
			request.Filters.TextQuery = text
		}
	}
	return page, request, nil
}
