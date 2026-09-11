package v2

import (
	"errors"
	"net/http"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

type agentRerunResponse struct {
	Started bool `json:"started"`
}

type agentRunCursorValue struct {
	ID int `json:"id"`
}

type agentRunPage struct {
	Runs       []*models.AgentRun `json:"runs"`
	NextCursor string             `json:"next_cursor,omitempty"`
	HasMore    bool               `json:"has_more"`
}

type agentRunEventPage struct {
	Events     []*models.AgentRunEvent `json:"events"`
	NextCursor string                  `json:"next_cursor,omitempty"`
	HasMore    bool                    `json:"has_more"`
}

func registerAgentRunRoutes(builder *routeBuilder, runs agentRunApplication) {
	builder.Read("/workspaces/{workspace_id}/agent-runs", AuthAuthenticated, []string{"items:read"}, listWorkspaceAgentRuns(runs))
	builder.Read("/items/{item_id}/agent-runs", AuthAuthenticated, []string{"items:read"}, listItemAgentRuns(runs))
	builder.Action(http.MethodPost, "/items/{item_id}/agent-runs", http.StatusOK, AuthAuthenticated, []string{"items:write"}, rerunAgent(runs))
	builder.Read("/agent-runs/{run_id}", AuthAuthenticated, []string{"items:read"}, getAgentRun(runs))
	builder.Read("/agent-runs/{run_id}/usage", AuthAuthenticated, []string{"items:read"}, getAgentRunUsage(runs))
	builder.Read("/agent-runs/{run_id}/events", AuthAuthenticated, []string{"items:read"}, listAgentRunEvents(runs))
	builder.Action(http.MethodPost, "/agent-runs/{run_id}/cancel", http.StatusOK, AuthAuthenticated, []string{"items:write"}, cancelAgentRun(runs))
}

func listWorkspaceAgentRuns(runs agentRunApplication) readOperation[agentRunPage] {
	return func(r *http.Request) (agentRunPage, error) {
		user, err := principal(r)
		if err != nil {
			return agentRunPage{}, err
		}
		workspaceID, err := pathID(r, "workspace_id")
		if err != nil {
			return agentRunPage{}, err
		}
		limit, beforeID, err := agentRunCursor(r)
		if err != nil {
			return agentRunPage{}, err
		}
		result, err := runs.ListForWorkspace(r.Context(), user.ID, workspaceID, limit+1, beforeID)
		return makeAgentRunPage(result, limit), agentRunError(err)
	}
}

func listItemAgentRuns(runs agentRunApplication) readOperation[agentRunPage] {
	return func(r *http.Request) (agentRunPage, error) {
		user, err := principal(r)
		if err != nil {
			return agentRunPage{}, err
		}
		itemID, err := pathID(r, "item_id")
		if err != nil {
			return agentRunPage{}, err
		}
		limit, beforeID, err := agentRunCursor(r)
		if err != nil {
			return agentRunPage{}, err
		}
		result, err := runs.ListForItem(r.Context(), user.ID, itemID, limit+1, beforeID)
		return makeAgentRunPage(result, limit), agentRunError(err)
	}
}

func rerunAgent(runs agentRunApplication) actionOperation[agentRerunResponse] {
	return func(r *http.Request) (agentRerunResponse, error) {
		user, err := principal(r)
		if err != nil {
			return agentRerunResponse{}, err
		}
		itemID, err := pathID(r, "item_id")
		if err != nil {
			return agentRerunResponse{}, err
		}
		started, err := runs.Rerun(r.Context(), user.ID, itemID)
		return agentRerunResponse{Started: started}, agentRunError(err)
	}
}

func getAgentRun(runs agentRunApplication) readOperation[*models.AgentRun] {
	return func(r *http.Request) (*models.AgentRun, error) {
		user, id, err := agentRunTarget(r)
		if err != nil {
			return nil, err
		}
		result, err := runs.Get(r.Context(), user.ID, id)
		return result, agentRunError(err)
	}
}

func getAgentRunUsage(runs agentRunApplication) readOperation[repository.RunUsageTotals] {
	return func(r *http.Request) (repository.RunUsageTotals, error) {
		user, id, err := agentRunTarget(r)
		if err != nil {
			return repository.RunUsageTotals{}, err
		}
		result, err := runs.Usage(r.Context(), user.ID, id)
		return result, agentRunError(err)
	}
}

func listAgentRunEvents(runs agentRunApplication) readOperation[agentRunEventPage] {
	return func(r *http.Request) (agentRunEventPage, error) {
		user, id, err := agentRunTarget(r)
		if err != nil {
			return agentRunEventPage{}, err
		}
		if r.URL.Query().Has("after_id") {
			return agentRunEventPage{}, newError(http.StatusBadRequest, "invalid_request", "Use the opaque cursor parameter")
		}
		afterID := 0
		if raw := r.URL.Query().Get("cursor"); raw != "" {
			var cursor agentRunCursorValue
			if err := decodeOpaqueCursor("agent-run-events", raw, &cursor); err != nil || cursor.ID <= 0 {
				return agentRunEventPage{}, newError(http.StatusBadRequest, "invalid_request", "cursor is invalid")
			}
			afterID = cursor.ID
		}
		limit, err := parsePositiveInt(r, "page_size", 200, 200)
		if err != nil {
			return agentRunEventPage{}, err
		}
		result, err := runs.Events(r.Context(), user.ID, id, afterID, limit+1)
		if err != nil {
			return agentRunEventPage{}, agentRunError(err)
		}
		hasMore := len(result) > limit
		if hasMore {
			result = result[:limit]
		}
		next := ""
		if len(result) > 0 {
			next = encodeOpaqueCursor("agent-run-events", agentRunCursorValue{ID: result[len(result)-1].ID})
		}
		return agentRunEventPage{Events: result, NextCursor: next, HasMore: hasMore}, nil
	}
}

func cancelAgentRun(runs agentRunApplication) actionOperation[services.AgentRunCancelResult] {
	return func(r *http.Request) (services.AgentRunCancelResult, error) {
		user, id, err := agentRunTarget(r)
		if err != nil {
			return services.AgentRunCancelResult{}, err
		}
		result, err := runs.Cancel(r.Context(), user.ID, id, r.URL.Query().Get("force") == "true")
		return result, agentRunError(err)
	}
}

func agentRunCursor(r *http.Request) (limit, beforeID int, err error) {
	limit, err = parsePositiveInt(r, "page_size", 50, 200)
	if err != nil {
		return 0, 0, err
	}
	if r.URL.Query().Has("before_id") {
		return 0, 0, newError(http.StatusBadRequest, "invalid_request", "Use the opaque cursor parameter")
	}
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		var cursor agentRunCursorValue
		if err := decodeOpaqueCursor("agent-runs", raw, &cursor); err != nil || cursor.ID <= 0 {
			return 0, 0, newError(http.StatusBadRequest, "invalid_request", "cursor is invalid")
		}
		beforeID = cursor.ID
	}
	return limit, beforeID, nil
}

func makeAgentRunPage(runs []*models.AgentRun, limit int) agentRunPage {
	hasMore := len(runs) > limit
	if hasMore {
		runs = runs[:limit]
	}
	next := ""
	if hasMore && len(runs) > 0 {
		next = encodeOpaqueCursor("agent-runs", agentRunCursorValue{ID: runs[len(runs)-1].ID})
	}
	return agentRunPage{Runs: runs, NextCursor: next, HasMore: hasMore}
}

func agentRunTarget(r *http.Request) (*models.User, int, error) {
	user, err := principal(r)
	if err != nil {
		return nil, 0, err
	}
	id, err := pathID(r, "run_id")
	return user, id, err
}

func agentRunError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, services.ErrAgentRunItemNotVisible), errors.Is(err, services.ErrAgentRunNotVisible):
		return newError(http.StatusNotFound, "not_found", "Agent run was not found")
	case errors.Is(err, services.ErrAgentRunUnavailable):
		return newError(http.StatusServiceUnavailable, "service_unavailable", err.Error())
	case errors.Is(err, services.ErrRerunNoPriorRun), errors.Is(err, services.ErrRerunNoBinding), errors.Is(err, services.ErrBindingBudgetExceeded):
		return newError(http.StatusConflict, "conflict", err.Error())
	default:
		return internalError(err)
	}
}
