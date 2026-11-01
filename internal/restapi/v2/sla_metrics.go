package v2

import (
	"errors"
	"net/http"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

// registerSLAMetricRoutes publishes SLA metric configuration on the canonical
// v2 surface. A metric carries its conditions, ordered goals, and priority or
// fallback targets; every write bumps the configuration generation and
// schedules recalculation.
func registerSLAMetricRoutes(builder *routeBuilder, deps Deps) {
	builder.Read("/workspaces/{workspace_id}/sla/metrics", AuthAuthenticated, []string{"workspaces:read"}, listSLAMetrics(deps))
	builder.Read("/workspaces/{workspace_id}/sla/metrics/{metric_id}", AuthAuthenticated, []string{"workspaces:read"}, getSLAMetric(deps))
	builder.JSON(http.MethodPost, "/workspaces/{workspace_id}/sla/metrics", http.StatusCreated, false, AuthAuthenticated, []string{"workspaces:write"}, createSLAMetric(deps))
	builder.JSON(http.MethodPut, "/workspaces/{workspace_id}/sla/metrics/{metric_id}", http.StatusOK, false, AuthAuthenticated, []string{"workspaces:write"}, updateSLAMetric(deps))
	builder.Command(http.MethodDelete, "/workspaces/{workspace_id}/sla/metrics/{metric_id}", AuthAuthenticated, []string{"workspaces:write"}, deleteSLAMetric(deps))
}

type slaMetricRequest struct {
	Name          string                `json:"name"`
	DisplayFormat string                `json:"display_format"`
	Position      int                   `json:"position"`
	IsActive      *bool                 `json:"is_active"`
	ImportStatus  string                `json:"import_status"`
	Conditions    []models.SLACondition `json:"conditions"`
	Goals         []models.SLAGoal      `json:"goals"`
}

func (request slaMetricRequest) input() services.SLAMetricInput {
	return services.SLAMetricInput{
		Name:          request.Name,
		DisplayFormat: request.DisplayFormat,
		Position:      request.Position,
		IsActive:      request.IsActive,
		ImportStatus:  request.ImportStatus,
		Conditions:    request.Conditions,
		Goals:         request.Goals,
	}
}

func listSLAMetrics(deps Deps) readOperation[[]models.SLAMetric] {
	return func(r *http.Request) ([]models.SLAMetric, error) {
		service, workspaceID, err := requireSLAMetricAdmin(r, deps)
		if err != nil {
			return nil, err
		}
		metrics, err := service.List(r.Context(), workspaceID)
		if err != nil {
			return nil, internalError(err)
		}
		return metrics, nil
	}
}

func getSLAMetric(deps Deps) readOperation[*models.SLAMetric] {
	return func(r *http.Request) (*models.SLAMetric, error) {
		service, workspaceID, err := requireSLAMetricAdmin(r, deps)
		if err != nil {
			return nil, err
		}
		metricID, err := pathID(r, "metric_id")
		if err != nil {
			return nil, err
		}
		metric, err := service.Get(r.Context(), workspaceID, metricID)
		if err != nil {
			return nil, slaMetricError(err)
		}
		return metric, nil
	}
}

func createSLAMetric(deps Deps) jsonOperation[slaMetricRequest, *models.SLAMetric] {
	return func(r *http.Request, request slaMetricRequest) (*models.SLAMetric, error) {
		service, workspaceID, err := requireSLAMetricAdmin(r, deps)
		if err != nil {
			return nil, err
		}
		metric, err := service.Create(r.Context(), workspaceID, request.input())
		if err != nil {
			return nil, slaMetricError(err)
		}
		logger.Record(r, logger.ActionSLAMetricCreate, logger.ResourceSLAMetric, &metric.ID, metric.Name, map[string]any{"workspace_id": workspaceID})
		return metric, nil
	}
}

func updateSLAMetric(deps Deps) jsonOperation[slaMetricRequest, *models.SLAMetric] {
	return func(r *http.Request, request slaMetricRequest) (*models.SLAMetric, error) {
		service, workspaceID, err := requireSLAMetricAdmin(r, deps)
		if err != nil {
			return nil, err
		}
		metricID, err := pathID(r, "metric_id")
		if err != nil {
			return nil, err
		}
		metric, err := service.Update(r.Context(), workspaceID, metricID, request.input())
		if err != nil {
			return nil, slaMetricError(err)
		}
		logger.Record(r, logger.ActionSLAMetricUpdate, logger.ResourceSLAMetric, &metric.ID, metric.Name, map[string]any{"workspace_id": workspaceID})
		return metric, nil
	}
}

func deleteSLAMetric(deps Deps) commandOperation {
	return func(r *http.Request) error {
		service, workspaceID, err := requireSLAMetricAdmin(r, deps)
		if err != nil {
			return err
		}
		metricID, err := pathID(r, "metric_id")
		if err != nil {
			return err
		}
		if err := service.Delete(r.Context(), workspaceID, metricID); err != nil {
			return slaMetricError(err)
		}
		logger.Record(r, logger.ActionSLAMetricDelete, logger.ResourceSLAMetric, &metricID, "", map[string]any{"workspace_id": workspaceID})
		return nil
	}
}

// requireSLAMetricAdmin resolves the service and enforces workspace-admin
// access, mirroring the cookie surface.
func requireSLAMetricAdmin(r *http.Request, deps Deps) (*services.SLAMetricService, int, error) {
	if deps.SLAMetrics == nil {
		return nil, 0, newError(http.StatusNotFound, "not_found", "SLA metrics are not available")
	}
	user, workspaceID, err := principalAndWorkspace(r)
	if err != nil {
		return nil, 0, err
	}
	if err := requireWorkspace(deps.Access.CanAdminWorkspace, user.ID, workspaceID); err != nil {
		return nil, 0, err
	}
	return deps.SLAMetrics, workspaceID, nil
}

func slaMetricError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, services.ErrSLAMetricInvalid):
		return newError(http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, repository.ErrNotFound):
		return newError(http.StatusNotFound, "not_found", "Metric was not found")
	default:
		return internalError(err)
	}
}
