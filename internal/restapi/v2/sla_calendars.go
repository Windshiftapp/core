package v2

import (
	"encoding/json"
	"errors"
	"net/http"

	"windshift/internal/businesstime"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

// registerSLACalendarRoutes publishes workspace working-calendar configuration
// on the canonical v2 surface. Creating a calendar is explicit: a workspace is
// never seeded with a default, so metrics can only reference a calendar an
// administrator created.
func registerSLACalendarRoutes(builder *routeBuilder, deps Deps) {
	builder.Read("/workspaces/{workspace_id}/sla/calendars", AuthAuthenticated, []string{"workspaces:read"}, listSLACalendars(deps))
	builder.Read("/workspaces/{workspace_id}/sla/available-calendars", AuthAuthenticated, []string{"workspaces:read"}, listAvailableSLACalendars(deps))
	builder.Read("/workspaces/{workspace_id}/sla/calendars/{calendar_id}/impact", AuthAuthenticated, []string{"workspaces:read"}, getSLACalendarImpact(deps))
	builder.Read("/workspaces/{workspace_id}/sla/coverage-preview", AuthAuthenticated, []string{"workspaces:read"}, getSLACoveragePreview(deps))
	builder.JSON(http.MethodPost, "/workspaces/{workspace_id}/sla/calendars", http.StatusCreated, false, AuthAuthenticated, []string{"workspaces:write"}, createSLACalendar(deps))
	builder.JSON(http.MethodPut, "/workspaces/{workspace_id}/sla/calendars/{calendar_id}", http.StatusOK, false, AuthAuthenticated, []string{"workspaces:write"}, updateSLACalendar(deps))
	builder.Command(http.MethodDelete, "/workspaces/{workspace_id}/sla/calendars/{calendar_id}", AuthAuthenticated, []string{"workspaces:write"}, deleteSLACalendar(deps))
}

type slaCalendarRequest struct {
	Name            string                                  `json:"name"`
	Description     string                                  `json:"description"`
	Timezone        string                                  `json:"timezone"`
	WeeklyIntervals map[string][]businesstime.ClockInterval `json:"weekly_intervals"`
	Holidays        []businesstime.Holiday                  `json:"holidays"`
	IsDefault       bool                                    `json:"is_default"`
	ApplyToOngoing  bool                                    `json:"apply_to_ongoing"`
}

func (request slaCalendarRequest) input() (services.SLACalendarInput, error) {
	input := services.SLACalendarInput{
		Name:           request.Name,
		Description:    request.Description,
		Timezone:       request.Timezone,
		IsDefault:      request.IsDefault,
		ApplyToOngoing: request.ApplyToOngoing,
	}
	if request.WeeklyIntervals != nil {
		encoded, err := json.Marshal(request.WeeklyIntervals)
		if err != nil {
			return input, newError(http.StatusBadRequest, "invalid_request", "weekly_intervals is invalid")
		}
		input.WeeklyIntervals = encoded
	}
	if request.Holidays != nil {
		encoded, err := json.Marshal(request.Holidays)
		if err != nil {
			return input, newError(http.StatusBadRequest, "invalid_request", "holidays is invalid")
		}
		input.Holidays = encoded
	}
	return input, nil
}

func listSLACalendars(deps Deps) readOperation[[]models.WorkingCalendar] {
	return func(r *http.Request) ([]models.WorkingCalendar, error) {
		service, err := requireSLACalendarAdmin(r, deps)
		if err != nil {
			return nil, err
		}
		workspaceID, err := pathID(r, "workspace_id")
		if err != nil {
			return nil, err
		}
		calendars, err := service.ListWorkspace(r.Context(), workspaceID)
		if err != nil {
			return nil, internalError(err)
		}
		return calendars, nil
	}
}

func listAvailableSLACalendars(deps Deps) readOperation[[]models.WorkingCalendar] {
	return func(r *http.Request) ([]models.WorkingCalendar, error) {
		service, err := requireSLACalendarAdmin(r, deps)
		if err != nil {
			return nil, err
		}
		workspaceID, err := pathID(r, "workspace_id")
		if err != nil {
			return nil, err
		}
		calendars, err := service.ListAvailable(r.Context(), workspaceID)
		if err != nil {
			return nil, internalError(err)
		}
		return calendars, nil
	}
}

// getSLACoveragePreview compares a candidate calendar against the workspace's
// bound team service hours. Informational only.
func getSLACoveragePreview(deps Deps) readOperation[*models.SLACoverage] {
	return func(r *http.Request) (*models.SLACoverage, error) {
		service, err := requireSLACalendarAdmin(r, deps)
		if err != nil {
			return nil, err
		}
		workspaceID, err := pathID(r, "workspace_id")
		if err != nil {
			return nil, err
		}
		calendarID, err := optionalPositiveQuery(r, "calendar_id")
		if err != nil {
			return nil, err
		}
		if calendarID == nil {
			return nil, newError(http.StatusBadRequest, "invalid_request", "calendar_id is required")
		}
		coverage, err := service.PreviewCoverage(r.Context(), workspaceID, *calendarID)
		if err != nil {
			return nil, slaCalendarError(err)
		}
		return coverage, nil
	}
}

// getSLACalendarImpact previews the recalculation an edit to a
// workspace-visible calendar would trigger, scoped to that workspace.
func getSLACalendarImpact(deps Deps) readOperation[*models.SLACalendarImpact] {
	return func(r *http.Request) (*models.SLACalendarImpact, error) {
		service, err := requireSLACalendarAdmin(r, deps)
		if err != nil {
			return nil, err
		}
		workspaceID, err := pathID(r, "workspace_id")
		if err != nil {
			return nil, err
		}
		calendarID, err := pathID(r, "calendar_id")
		if err != nil {
			return nil, err
		}
		impact, err := service.WorkspaceCalendarImpact(r.Context(), workspaceID, calendarID)
		if err != nil {
			return nil, slaCalendarError(err)
		}
		return impact, nil
	}
}

func createSLACalendar(deps Deps) jsonOperation[slaCalendarRequest, *models.WorkingCalendar] {
	return func(r *http.Request, request slaCalendarRequest) (*models.WorkingCalendar, error) {
		service, err := requireSLACalendarAdmin(r, deps)
		if err != nil {
			return nil, err
		}
		workspaceID, err := pathID(r, "workspace_id")
		if err != nil {
			return nil, err
		}
		input, err := request.input()
		if err != nil {
			return nil, err
		}
		calendar, err := service.CreateWorkspace(r.Context(), workspaceID, input)
		if err != nil {
			return nil, slaCalendarError(err)
		}
		logger.Record(r, logger.ActionSLACalendarCreate, logger.ResourceSLACalendar, &calendar.ID, calendar.Name, map[string]any{"workspace_id": workspaceID})
		return calendar, nil
	}
}

func updateSLACalendar(deps Deps) jsonOperation[slaCalendarRequest, *models.WorkingCalendar] {
	return func(r *http.Request, request slaCalendarRequest) (*models.WorkingCalendar, error) {
		service, err := requireSLACalendarAdmin(r, deps)
		if err != nil {
			return nil, err
		}
		workspaceID, err := pathID(r, "workspace_id")
		if err != nil {
			return nil, err
		}
		calendarID, err := pathID(r, "calendar_id")
		if err != nil {
			return nil, err
		}
		input, err := request.input()
		if err != nil {
			return nil, err
		}
		calendar, err := service.UpdateWorkspace(r.Context(), workspaceID, calendarID, input)
		if err != nil {
			return nil, slaCalendarError(err)
		}
		logger.Record(r, logger.ActionSLACalendarUpdate, logger.ResourceSLACalendar, &calendar.ID, calendar.Name, map[string]any{"workspace_id": workspaceID})
		return calendar, nil
	}
}

func deleteSLACalendar(deps Deps) commandOperation {
	return func(r *http.Request) error {
		service, err := requireSLACalendarAdmin(r, deps)
		if err != nil {
			return err
		}
		workspaceID, err := pathID(r, "workspace_id")
		if err != nil {
			return err
		}
		calendarID, err := pathID(r, "calendar_id")
		if err != nil {
			return err
		}
		if err := service.DeleteWorkspace(r.Context(), workspaceID, calendarID); err != nil {
			return slaCalendarError(err)
		}
		logger.Record(r, logger.ActionSLACalendarDelete, logger.ResourceSLACalendar, &calendarID, "", map[string]any{"workspace_id": workspaceID})
		return nil
	}
}

// requireSLACalendarAdmin resolves the service and enforces workspace-admin
// access, mirroring the cookie surface.
func requireSLACalendarAdmin(r *http.Request, deps Deps) (*services.SLACalendarService, error) {
	if deps.SLACalendars == nil {
		return nil, newError(http.StatusNotFound, "not_found", "SLA calendars are not available")
	}
	user, workspaceID, err := principalAndWorkspace(r)
	if err != nil {
		return nil, err
	}
	if err := requireWorkspace(deps.Access.CanAdminWorkspace, user.ID, workspaceID); err != nil {
		return nil, err
	}
	return deps.SLACalendars, nil
}

func slaCalendarError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, services.ErrSLACalendarInvalid):
		return newError(http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, repository.ErrNotFound):
		return newError(http.StatusNotFound, "not_found", "Calendar was not found")
	case errors.Is(err, repository.ErrSLAInUse):
		return newError(http.StatusConflict, "conflict", err.Error())
	default:
		return internalError(err)
	}
}
