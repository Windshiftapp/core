package v2

import (
	"net/http"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/services"
)

// registerSLATeamCalendarRoutes publishes team-owned service-hours calendars on
// the canonical v2 session surface. These routes stay session-only: a team's
// calendars are configured from the team page and no team-scoped bearer token
// scope exists yet.
func registerSLATeamCalendarRoutes(builder *routeBuilder, deps Deps) {
	builder.SessionRead("/teams/{team_id}/working-calendars", listTeamCalendars(deps))
	builder.SessionRead("/teams/{team_id}/working-calendars/{calendar_id}/impact", getTeamCalendarImpact(deps))
	builder.SessionJSON(http.MethodPost, "/teams/{team_id}/working-calendars", http.StatusCreated, false, createTeamCalendar(deps))
	builder.SessionJSON(http.MethodPut, "/teams/{team_id}/working-calendars/{calendar_id}", http.StatusOK, false, updateTeamCalendar(deps))
	builder.SessionCommand(http.MethodDelete, "/teams/{team_id}/working-calendars/{calendar_id}", deleteTeamCalendar(deps))
}

func listTeamCalendars(deps Deps) readOperation[[]models.WorkingCalendar] {
	return func(r *http.Request) ([]models.WorkingCalendar, error) {
		service, teamID, err := requireTeamCalendarAdmin(r, deps)
		if err != nil {
			return nil, err
		}
		calendars, err := service.ListTeam(r.Context(), teamID)
		if err != nil {
			return nil, internalError(err)
		}
		return calendars, nil
	}
}

func createTeamCalendar(deps Deps) jsonOperation[slaCalendarRequest, *models.WorkingCalendar] {
	return func(r *http.Request, request slaCalendarRequest) (*models.WorkingCalendar, error) {
		service, teamID, err := requireTeamCalendarAdmin(r, deps)
		if err != nil {
			return nil, err
		}
		input, err := request.input()
		if err != nil {
			return nil, err
		}
		calendar, err := service.CreateTeam(r.Context(), teamID, input)
		if err != nil {
			return nil, slaCalendarError(err)
		}
		logger.Record(r, logger.ActionSLACalendarCreate, logger.ResourceSLACalendar, &calendar.ID, calendar.Name, map[string]any{"team_id": teamID})
		return calendar, nil
	}
}

func updateTeamCalendar(deps Deps) jsonOperation[slaCalendarRequest, *models.WorkingCalendar] {
	return func(r *http.Request, request slaCalendarRequest) (*models.WorkingCalendar, error) {
		service, teamID, err := requireTeamCalendarAdmin(r, deps)
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
		calendar, err := service.UpdateTeam(r.Context(), teamID, calendarID, input)
		if err != nil {
			return nil, slaCalendarError(err)
		}
		logger.Record(r, logger.ActionSLACalendarUpdate, logger.ResourceSLACalendar, &calendar.ID, calendar.Name, map[string]any{"team_id": teamID})
		return calendar, nil
	}
}

func deleteTeamCalendar(deps Deps) commandOperation {
	return func(r *http.Request) error {
		service, teamID, err := requireTeamCalendarAdmin(r, deps)
		if err != nil {
			return err
		}
		calendarID, err := pathID(r, "calendar_id")
		if err != nil {
			return err
		}
		if err := service.DeleteTeam(r.Context(), teamID, calendarID); err != nil {
			return slaCalendarError(err)
		}
		logger.Record(r, logger.ActionSLACalendarDelete, logger.ResourceSLACalendar, &calendarID, "", map[string]any{"team_id": teamID})
		return nil
	}
}

// getTeamCalendarImpact previews the recalculation an edit to a team calendar
// would trigger across every bound workspace.
func getTeamCalendarImpact(deps Deps) readOperation[*models.SLACalendarImpact] {
	return func(r *http.Request) (*models.SLACalendarImpact, error) {
		service, teamID, err := requireTeamCalendarAdmin(r, deps)
		if err != nil {
			return nil, err
		}
		calendarID, err := pathID(r, "calendar_id")
		if err != nil {
			return nil, err
		}
		impact, err := service.TeamCalendarImpact(r.Context(), teamID, calendarID)
		if err != nil {
			return nil, slaCalendarError(err)
		}
		return impact, nil
	}
}

// requireTeamCalendarAdmin resolves the calendar service and enforces team
// administration: a global team manager or a direct team admin. It mirrors the
// cookie surface's authorization so both transports agree.
func requireTeamCalendarAdmin(r *http.Request, deps Deps) (*services.SLACalendarService, int, error) {
	if deps.SLACalendars == nil {
		return nil, 0, newError(http.StatusNotFound, "not_found", "SLA calendars are not available")
	}
	user, err := principal(r)
	if err != nil {
		return nil, 0, err
	}
	teamID, err := pathID(r, "team_id")
	if err != nil {
		return nil, 0, err
	}
	if deps.GlobalPermission != nil {
		hasGlobal, err := deps.GlobalPermission.HasGlobalPermission(user.ID, models.PermissionTeamsManage)
		if err != nil {
			return nil, 0, internalError(err)
		}
		if hasGlobal {
			return deps.SLACalendars, teamID, nil
		}
	}
	if deps.TeamAccess == nil {
		return nil, 0, newError(http.StatusNotFound, "not_found", "SLA calendars are not available")
	}
	isAdmin, err := deps.TeamAccess.IsTeamAdmin(teamID, user.ID)
	if err != nil {
		return nil, 0, internalError(err)
	}
	if !isAdmin {
		return nil, 0, newError(http.StatusForbidden, "insufficient_permission", "Team administrator permission is required")
	}
	return deps.SLACalendars, teamID, nil
}
