package v2

import (
	"errors"
	"net/http"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

// registerSLAWarningThresholdRoutes publishes warning-threshold configuration
// on the canonical v2 surface. Thresholds drive sla.warning events consumed by
// the existing notification and automation rules; they are not goal fields.
func registerSLAWarningThresholdRoutes(builder *routeBuilder, deps Deps) {
	builder.Read("/workspaces/{workspace_id}/sla/warning-thresholds", AuthAuthenticated, []string{"workspaces:read"}, listSLAWarningThresholds(deps))
	builder.JSON(http.MethodPost, "/workspaces/{workspace_id}/sla/warning-thresholds", http.StatusCreated, false, AuthAuthenticated, []string{"workspaces:write"}, createSLAWarningThreshold(deps))
	builder.JSON(http.MethodPut, "/workspaces/{workspace_id}/sla/warning-thresholds/{threshold_id}", http.StatusOK, false, AuthAuthenticated, []string{"workspaces:write"}, updateSLAWarningThreshold(deps))
	builder.Command(http.MethodDelete, "/workspaces/{workspace_id}/sla/warning-thresholds/{threshold_id}", AuthAuthenticated, []string{"workspaces:write"}, deleteSLAWarningThreshold(deps))
}

type slaWarningThresholdRequest struct {
	Label    string `json:"label"`
	Percent  int    `json:"percent"`
	MetricID *int   `json:"metric_id"`
	IsActive *bool  `json:"is_active"`
}

func (request slaWarningThresholdRequest) input() services.SLAWarningThresholdInput {
	isActive := true
	if request.IsActive != nil {
		isActive = *request.IsActive
	}
	return services.SLAWarningThresholdInput{
		Label:    request.Label,
		Percent:  request.Percent,
		MetricID: request.MetricID,
		IsActive: isActive,
	}
}

func listSLAWarningThresholds(deps Deps) readOperation[[]models.SLAWarningThreshold] {
	return func(r *http.Request) ([]models.SLAWarningThreshold, error) {
		if deps.SLASettings == nil {
			return nil, newError(http.StatusNotFound, "not_found", "SLA warning thresholds are not available")
		}
		// Warning state renders on item lists and boards, so item viewers may
		// read it; only mutations need workspace administration (WI-1578).
		user, workspaceID, err := principalAndWorkspace(r)
		if err != nil {
			return nil, err
		}
		if err := requireWorkspace(deps.Access.CanViewWorkspace, user.ID, workspaceID); err != nil {
			return nil, err
		}
		service := deps.SLASettings
		thresholds, err := service.ListWarningThresholds(r.Context(), workspaceID)
		if err != nil {
			return nil, internalError(err)
		}
		return thresholds, nil
	}
}

func createSLAWarningThreshold(deps Deps) jsonOperation[slaWarningThresholdRequest, *models.SLAWarningThreshold] {
	return func(r *http.Request, request slaWarningThresholdRequest) (*models.SLAWarningThreshold, error) {
		service, workspaceID, err := requireSLAThresholdAdmin(r, deps)
		if err != nil {
			return nil, err
		}
		threshold, err := service.CreateWarningThreshold(r.Context(), workspaceID, request.input())
		if err != nil {
			return nil, slaThresholdError(err)
		}
		logger.Record(r, logger.ActionSLAWarningThresholdCreate, logger.ResourceSLAWarningThreshold, &threshold.ID, threshold.Label, map[string]any{"workspace_id": workspaceID})
		return threshold, nil
	}
}

func updateSLAWarningThreshold(deps Deps) jsonOperation[slaWarningThresholdRequest, *models.SLAWarningThreshold] {
	return func(r *http.Request, request slaWarningThresholdRequest) (*models.SLAWarningThreshold, error) {
		service, workspaceID, err := requireSLAThresholdAdmin(r, deps)
		if err != nil {
			return nil, err
		}
		thresholdID, err := pathID(r, "threshold_id")
		if err != nil {
			return nil, err
		}
		threshold, err := service.UpdateWarningThreshold(r.Context(), workspaceID, thresholdID, request.input())
		if err != nil {
			return nil, slaThresholdError(err)
		}
		logger.Record(r, logger.ActionSLAWarningThresholdUpdate, logger.ResourceSLAWarningThreshold, &threshold.ID, threshold.Label, map[string]any{"workspace_id": workspaceID})
		return threshold, nil
	}
}

func deleteSLAWarningThreshold(deps Deps) commandOperation {
	return func(r *http.Request) error {
		service, workspaceID, err := requireSLAThresholdAdmin(r, deps)
		if err != nil {
			return err
		}
		thresholdID, err := pathID(r, "threshold_id")
		if err != nil {
			return err
		}
		if err := service.DeleteWarningThreshold(r.Context(), workspaceID, thresholdID); err != nil {
			return slaThresholdError(err)
		}
		logger.Record(r, logger.ActionSLAWarningThresholdDelete, logger.ResourceSLAWarningThreshold, &thresholdID, "", map[string]any{"workspace_id": workspaceID})
		return nil
	}
}

func requireSLAThresholdAdmin(r *http.Request, deps Deps) (*services.SLASettingsService, int, error) {
	if deps.SLASettings == nil {
		return nil, 0, newError(http.StatusNotFound, "not_found", "SLA warning thresholds are not available")
	}
	user, workspaceID, err := principalAndWorkspace(r)
	if err != nil {
		return nil, 0, err
	}
	if err := requireWorkspace(deps.Access.CanAdminWorkspace, user.ID, workspaceID); err != nil {
		return nil, 0, err
	}
	return deps.SLASettings, workspaceID, nil
}

func slaThresholdError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, services.ErrSLAThresholdInvalid):
		return newError(http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, repository.ErrNotFound):
		return newError(http.StatusNotFound, "not_found", "Warning threshold was not found")
	default:
		return internalError(err)
	}
}
