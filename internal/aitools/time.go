package aitools

import (
	"context"
	"errors"
	"fmt"
	"time"

	"windshift/internal/auth"
	"windshift/internal/repository"
	"windshift/internal/services"
)

// ----------------------------------------------------------------------------
// list_time_projects
// ----------------------------------------------------------------------------

type listTimeProjectsArgs struct {
	Status string `json:"status,omitempty" jsonschema:"Filter by project status (e.g. 'Active', 'Archived')"`
}

type timeProjectDTO struct {
	ID           int     `json:"id"`
	Name         string  `json:"name"`
	Status       string  `json:"status"`
	Description  string  `json:"description,omitempty"`
	CustomerName string  `json:"customer_name,omitempty"`
	CategoryName string  `json:"category_name,omitempty"`
	HourlyRate   float64 `json:"hourly_rate,omitempty"`
}

type listTimeProjectsOut struct {
	Projects []timeProjectDTO `json:"projects"`
}

// ----------------------------------------------------------------------------
// list_worklogs
// ----------------------------------------------------------------------------

type listWorklogsArgs struct {
	DateFrom  string `json:"date_from,omitempty" jsonschema:"Start date (YYYY-MM-DD)"`
	DateTo    string `json:"date_to,omitempty" jsonschema:"End date (YYYY-MM-DD)"`
	ProjectID *int   `json:"project_id,omitempty" jsonschema:"Filter by project ID"`
	Limit     int    `json:"limit,omitempty" jsonschema:"Max results (default 50)"`
}

type worklogDTO struct {
	ID              int    `json:"id"`
	ProjectID       int    `json:"project_id"`
	ProjectName     string `json:"project_name,omitempty"`
	CustomerName    string `json:"customer_name,omitempty"`
	Description     string `json:"description"`
	Date            string `json:"date"`
	DurationMinutes int    `json:"duration_minutes"`
	ItemKey         string `json:"item_key,omitempty"`
	ItemID          *int   `json:"item_id,omitempty"`
}

type listWorklogsOut struct {
	Worklogs []worklogDTO `json:"worklogs"`
}

type temporalContextArgs struct {
	Timezone string `json:"timezone,omitempty" jsonschema:"Optional IANA timezone, only when the user explicitly asks about a different zone; defaults to the acting user's timezone"`
}

type temporalContextOut struct {
	Timezone  string `json:"timezone"`
	LocalNow  string `json:"local_now"`
	LocalDate string `json:"local_date"`
	UTCNow    string `json:"utc_now"`
}

// ----------------------------------------------------------------------------
// log_time / create_worklog
// ----------------------------------------------------------------------------

// logTimeArgs supports both forms (string duration like "1h30m" via Duration,
// or explicit start_time/end_time, or numeric DurationMinutes). Exactly one
// of those three options must be provided.
type logTimeArgs struct {
	ProjectID       int    `json:"project_id" jsonschema:"Time project ID"`
	Description     string `json:"description" jsonschema:"Description of the work done"`
	Date            string `json:"date" jsonschema:"Date in YYYY-MM-DD format"`
	Duration        string `json:"duration,omitempty" jsonschema:"Duration string like '2h', '30m', '1h30m', '1d' (1d = 8h). Hours and minutes only; seconds and other units are rejected."`
	DurationMinutes int    `json:"duration_minutes,omitempty" jsonschema:"Alternative to duration: minutes as integer"`
	StartTime       string `json:"start_time,omitempty" jsonschema:"HH:MM start time. Pair with end_time."`
	EndTime         string `json:"end_time,omitempty" jsonschema:"HH:MM end time. Pair with start_time."`
	ItemID          *int   `json:"item_id,omitempty" jsonschema:"Optional linked work item ID (alternative: item_key)"`
	ItemKey         string `json:"item_key,omitempty" jsonschema:"Optional linked work item key like PROJ-42 (alternative to item_id)"`
	Timezone        string `json:"timezone,omitempty" jsonschema:"Optional IANA timezone only when the user explicitly named a different zone. Defaults to the acting user's timezone. Enter date and HH:MM exactly as local wall-clock values; never convert or pre-offset them."`
}

type logTimeOut struct {
	ID              int64  `json:"id"`
	ProjectID       int    `json:"project_id"`
	ProjectName     string `json:"project_name,omitempty"`
	Date            string `json:"date"`
	DurationMinutes int    `json:"duration_minutes"`
	Description     string `json:"description"`
	Timezone        string `json:"timezone"`
	StartTimeLocal  string `json:"start_time_local"`
	EndTimeLocal    string `json:"end_time_local"`
	StartAt         string `json:"start_at"`
	EndAt           string `json:"end_at"`
}

// ----------------------------------------------------------------------------
// start_timer / stop_timer
// ----------------------------------------------------------------------------

type startTimerArgs struct {
	ProjectID   int    `json:"project_id" jsonschema:"Time project ID"`
	WorkspaceID int    `json:"workspace_id" jsonschema:"Workspace ID"`
	Description string `json:"description" jsonschema:"Timer description"`
	ItemID      *int   `json:"item_id,omitempty" jsonschema:"Optional linked work item ID (alternative: item_key)"`
	ItemKey     string `json:"item_key,omitempty" jsonschema:"Optional linked work item key like PROJ-42 (alternative to item_id)"`
}

type startTimerOut struct {
	ID           int64  `json:"id"`
	Description  string `json:"description"`
	StartTimeUTC int64  `json:"start_time_utc"`
	Started      bool   `json:"started"`
}

type stopTimerArgs struct{}

type stopTimerOut struct {
	Stopped         bool   `json:"stopped"`
	TimerID         int    `json:"timer_id"`
	Description     string `json:"description"`
	DurationSeconds int64  `json:"duration_seconds"`
	DurationMinutes int    `json:"duration_minutes"`
	WorklogCreated  bool   `json:"worklog_created"`
}

func init() {
	Register(Default, Tool[temporalContextArgs]{
		Name:        "get_temporal_context",
		Group:       CapabilityTime,
		Access:      AccessRead,
		Risk:        RiskLow,
		Description: "Get authoritative current local and UTC time context for relative phrases such as today or yesterday. Defaults to the acting user's IANA timezone.",
		Scopes:      []string{auth.ScopeTimeRead},
		Run: func(_ context.Context, env *Env, args temporalContextArgs) (any, error) {
			timezone := env.Timezone
			if args.Timezone != "" {
				timezone = args.Timezone
			}
			resolved, location, err := services.ResolveTimezone(timezone)
			if err != nil {
				return map[string]string{"error": err.Error()}, nil //nolint:nilerr // tool validation errors are returned in the JSON result
			}
			now := time.Now()
			local := now.In(location)
			return temporalContextOut{
				Timezone:  resolved,
				LocalNow:  local.Format(time.RFC3339),
				LocalDate: local.Format(time.DateOnly),
				UTCNow:    now.UTC().Format(time.RFC3339),
			}, nil
		},
	})

	Register(Default, Tool[listTimeProjectsArgs]{
		Name:        "list_time_projects",
		Group:       CapabilityTime,
		Access:      AccessRead,
		Risk:        RiskLow,
		Description: "List time tracking projects the user has access to.",
		Scopes:      []string{auth.ScopeTimeRead},
		Run: func(_ context.Context, env *Env, args listTimeProjectsArgs) (any, error) {
			accessibleIDs, err := env.TimePermService.GetAccessibleProjects(env.UserID)
			if err != nil {
				return nil, err
			}
			if accessibleIDs != nil && len(accessibleIDs) == 0 {
				return listTimeProjectsOut{Projects: []timeProjectDTO{}}, nil
			}
			projects, err := repository.NewTimeProjectRepository(env.DB).ListDetails(accessibleIDs, args.Status)
			if err != nil {
				return nil, err
			}
			out := listTimeProjectsOut{Projects: []timeProjectDTO{}}
			for _, project := range projects {
				out.Projects = append(out.Projects, timeProjectDTO{
					ID: project.ID, Name: project.Name, Status: project.Status,
					Description: project.Description, CustomerName: project.CustomerName,
					CategoryName: project.CategoryName, HourlyRate: project.HourlyRate,
				})
			}
			return out, nil
		},
	})

	Register(Default, Tool[listWorklogsArgs]{
		Name:        "list_worklogs",
		Group:       CapabilityTime,
		Access:      AccessRead,
		Risk:        RiskLow,
		Description: "List the current user's time tracking worklogs with optional date and project filters.",
		Scopes:      []string{auth.ScopeTimeRead},
		Run: func(_ context.Context, env *Env, args listWorklogsArgs) (any, error) {
			limit := args.Limit
			if limit <= 0 || limit > 200 {
				limit = 50
			}
			filter := repository.WorklogListFilter{UserID: env.UserID, ProjectID: args.ProjectID, Limit: limit}
			if args.DateFrom != "" {
				start, _, err := services.CivilDateRangeUTC(args.DateFrom, args.DateFrom, time.UTC)
				if err != nil {
					return map[string]string{"error": "invalid date_from format, use YYYY-MM-DD"}, nil //nolint:nilerr // Tool validation errors use response payloads.
				}
				from := start.Unix()
				filter.DateFromUnix = &from
			}
			if args.DateTo != "" {
				_, endExclusive, err := services.CivilDateRangeUTC(args.DateTo, args.DateTo, time.UTC)
				if err != nil {
					return map[string]string{"error": "invalid date_to format, use YYYY-MM-DD"}, nil //nolint:nilerr // Tool validation errors use response payloads.
				}
				to := endExclusive.Unix()
				filter.DateToExclusiveUnix = &to
			}
			worklogs, _, err := repository.NewTimeWorklogRepository(env.DB).ListForUser(filter)
			if err != nil {
				return nil, err
			}
			out := listWorklogsOut{Worklogs: []worklogDTO{}}
			for _, worklog := range worklogs {
				result := worklogDTO{
					ID: worklog.ID, ProjectID: worklog.ProjectID, ProjectName: worklog.ProjectName,
					CustomerName: worklog.CustomerName, Description: worklog.Description,
					Date: time.Unix(worklog.Date, 0).UTC().Format("2006-01-02"), DurationMinutes: worklog.DurationMins,
				}
				if worklog.ItemID != nil && worklog.WorkspaceID != nil && worklog.WorkspaceKey != "" && env.HasWorkspaceAccess(*worklog.WorkspaceID) {
					result.ItemKey = fmt.Sprintf("%s-%d", worklog.WorkspaceKey, worklog.WorkspaceItemNumber)
					result.ItemID = worklog.ItemID
				}
				out.Worklogs = append(out.Worklogs, result)
			}
			return out, nil
		},
	})

	Register(Default, Tool[logTimeArgs]{
		Name:        "log_time",
		Group:       CapabilityTime,
		Access:      AccessWrite,
		Risk:        RiskMedium,
		Description: "Log a time entry on a time tracking project. Date and HH:MM are exact local wall-clock values in the acting user's IANA timezone: never convert or pre-offset them. Provide duration OR duration_minutes OR start_time + end_time; when clocks and a duration are both sent the duration must match, and an end clock at or before the start means the next day. Use timezone only when the user explicitly names a different zone.",
		Scopes:      []string{auth.ScopeTimeWrite},
		Run: func(_ context.Context, env *Env, args logTimeArgs) (any, error) {
			if args.ProjectID == 0 || args.Description == "" || args.Date == "" {
				return map[string]string{"error": "project_id, description, and date are required"}, nil
			}
			canBook, err := env.TimePermService.CanBookTimeOnProject(env.UserID, args.ProjectID)
			if err != nil {
				return nil, err
			}
			if !canBook {
				return map[string]string{"error": "no permission to book time on this project"}, nil
			}
			project, err := repository.NewTimeProjectRepository(env.DB).GetBookingInfo(args.ProjectID)
			if err != nil {
				return map[string]string{"error": "project not found"}, nil //nolint:nilerr // surface as a tool error in JSON, not as a protocol error
			}
			if project.Status != "Active" {
				return map[string]string{"error": fmt.Sprintf("project %q is not active (status: %s)", project.Name, project.Status)}, nil
			}
			if project.CustomerID == nil {
				return map[string]string{"error": "project has no customer assigned, cannot log time"}, nil
			}
			timezone := env.Timezone
			if args.Timezone != "" {
				timezone = args.Timezone
			}
			resolvedTimezone, location, err := services.ResolveTimezone(timezone)
			if err != nil {
				return map[string]string{"error": err.Error()}, nil //nolint:nilerr // tool validation errors are returned in the JSON result
			}
			date, err := services.ParseCivilDate(args.Date, location)
			if err != nil {
				return map[string]string{"error": err.Error()}, nil //nolint:nilerr // surface as a tool error in JSON, not as a protocol error
			}
			durationMins, startUnix, endUnix, err := services.ParseWorklogTimes(date, services.WorklogTimeInput{
				Duration:        args.Duration,
				DurationMinutes: args.DurationMinutes,
				StartTime:       args.StartTime,
				EndTime:         args.EndTime,
			})
			if err != nil {
				return map[string]string{"error": err.Error()}, nil //nolint:nilerr // tool validation errors are returned in the JSON result
			}
			itemID, toolErr := resolveOptionalItemRef(env, args.ItemID, args.ItemKey)
			if toolErr != nil {
				return toolErr, nil
			}
			id, err := repository.NewTimeWorklogRepository(env.DB).Create(repository.NewWorklog{
				ProjectID:       args.ProjectID,
				CustomerID:      *project.CustomerID,
				UserID:          env.UserID,
				ItemID:          itemID,
				Description:     args.Description,
				DateUnix:        services.WorklogDateUnix(date),
				StartTimeUnix:   startUnix,
				EndTimeUnix:     endUnix,
				DurationMinutes: durationMins,
			})
			if err != nil {
				return nil, err
			}
			env.AuditWrite(resourceTimeWorklog, int(id), "log_time", args.Description)
			return logTimeOut{
				ID:              id,
				ProjectID:       args.ProjectID,
				ProjectName:     project.Name,
				Date:            args.Date,
				DurationMinutes: durationMins,
				Description:     args.Description,
				Timezone:        resolvedTimezone,
				StartTimeLocal:  time.Unix(startUnix, 0).In(location).Format("15:04"),
				EndTimeLocal:    time.Unix(endUnix, 0).In(location).Format("15:04"),
				StartAt:         time.Unix(startUnix, 0).UTC().Format(time.RFC3339),
				EndAt:           time.Unix(endUnix, 0).UTC().Format(time.RFC3339),
			}, nil
		},
	})

	Register(Default, Tool[startTimerArgs]{
		Name:        "start_timer",
		Group:       CapabilityTime,
		Access:      AccessWrite,
		Risk:        RiskMedium,
		Description: "Start a time tracking timer. Only one timer can be active at a time. An optional work item can be linked by numeric ID or key (e.g. PROJ-42).",
		Scopes:      []string{auth.ScopeTimeWrite},
		Run: func(_ context.Context, env *Env, args startTimerArgs) (any, error) {
			itemID, toolErr := resolveOptionalItemRef(env, args.ItemID, args.ItemKey)
			if toolErr != nil {
				return toolErr, nil
			}
			timer, err := env.TimerService.StartTimer(env.UserID, args.WorkspaceID, args.ProjectID, itemID, args.Description)
			if err != nil {
				if msg, ok := timerErrToToolMessage(err); ok {
					return map[string]string{"error": msg}, nil
				}
				return nil, err
			}
			env.AuditWrite(resourceTimer, timer.ID, "start_timer", timer.Description)
			return startTimerOut{
				ID:           int64(timer.ID),
				Description:  timer.Description,
				StartTimeUTC: timer.StartTimeUTC,
				Started:      true,
			}, nil
		},
	})

	Register(Default, Tool[stopTimerArgs]{
		Name:        "stop_timer",
		Group:       CapabilityTime,
		Access:      AccessWrite,
		Risk:        RiskMedium,
		Description: "Stop the user's currently running timer and create a worklog entry.",
		Scopes:      []string{auth.ScopeTimeWrite},
		Run: func(_ context.Context, env *Env, _ stopTimerArgs) (any, error) {
			res, err := env.TimerService.StopActiveForUser(env.UserID)
			if err != nil {
				if errors.Is(err, services.ErrTimerNotFound) {
					return map[string]string{"error": "no active timer running"}, nil
				}
				if msg, ok := timerErrToToolMessage(err); ok {
					return map[string]string{"error": msg}, nil
				}
				return nil, err
			}
			env.AuditWrite(resourceTimer, res.TimerID, "stop_timer", res.Description)
			return stopTimerOut{
				Stopped:         true,
				TimerID:         res.TimerID,
				Description:     res.Description,
				DurationSeconds: res.DurationSeconds,
				DurationMinutes: res.DurationMinutes,
				WorklogCreated:  res.WorklogCreated,
			}, nil
		},
	})
}

// resolveOptionalItemRef resolves the optional work-item reference time tools
// accept (numeric item_id or item_key like PROJ-42). Returns (nil, nil) when
// neither is provided. Resolution failures and items outside the caller's
// accessible workspaces both surface as the generic "item not found" tool
// error so item existence is never leaked.
func resolveOptionalItemRef(env *Env, itemID *int, itemKey string) (resolvedID *int, resolvedValue any) {
	if (itemID == nil || *itemID <= 0) && itemKey == "" {
		return nil, nil
	}
	rawID := 0
	if itemID != nil {
		rawID = *itemID
	}
	id, err := services.ResolveAccessibleWorklogItem(env.DB, rawID, itemKey, func(workspaceID int) (bool, error) {
		return env.HasWorkspaceAccess(workspaceID), nil
	})
	if err != nil {
		return nil, map[string]string{"error": "item not found"}
	}
	return &id, nil
}

// timerErrToToolMessage maps TimerService sentinel errors to the
// human-readable strings these AI tools have always returned. Returns
// (msg, true) when the error is one of the known sentinels.
func timerErrToToolMessage(err error) (string, bool) {
	switch {
	case errors.Is(err, services.ErrTimerValidation):
		return err.Error(), true
	case errors.Is(err, services.ErrTimerForbidden):
		return "no permission to book time on this project", true
	case errors.Is(err, services.ErrTimerNotFound):
		// Could be project, workspace, or item — the wrapped message
		// already names which one ("timer: not found: workspace").
		return err.Error(), true
	case errors.Is(err, services.ErrTimerProjectInactive):
		return "project is not active", true
	case errors.Is(err, services.ErrTimerAlreadyRunning):
		return "a timer is already running - stop it first", true
	}
	return "", false
}
