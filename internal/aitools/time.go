package aitools

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/utils"
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
	Duration        string `json:"duration,omitempty" jsonschema:"Duration string like '2h', '30m', '1h30m', '1d' (1d = 8h)"`
	DurationMinutes int    `json:"duration_minutes,omitempty" jsonschema:"Alternative to duration: minutes as integer"`
	StartTime       string `json:"start_time,omitempty" jsonschema:"HH:MM start time. Pair with end_time."`
	EndTime         string `json:"end_time,omitempty" jsonschema:"HH:MM end time. Pair with start_time."`
	ItemID          *int   `json:"item_id,omitempty" jsonschema:"Optional linked work item ID"`
}

type logTimeOut struct {
	ID              int64  `json:"id"`
	ProjectID       int    `json:"project_id"`
	ProjectName     string `json:"project_name,omitempty"`
	Date            string `json:"date"`
	DurationMinutes int    `json:"duration_minutes"`
	Description     string `json:"description"`
}

// ----------------------------------------------------------------------------
// start_timer / stop_timer
// ----------------------------------------------------------------------------

type startTimerArgs struct {
	ProjectID   int    `json:"project_id" jsonschema:"Time project ID"`
	WorkspaceID int    `json:"workspace_id" jsonschema:"Workspace ID"`
	Description string `json:"description" jsonschema:"Timer description"`
	ItemID      *int   `json:"item_id,omitempty" jsonschema:"Optional linked work item ID"`
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
	Register(Default, Tool[listTimeProjectsArgs]{
		Name:        "list_time_projects",
		Description: "List time tracking projects the user has access to.",
		Run: func(_ context.Context, env *Env, args listTimeProjectsArgs) (any, error) {
			accessibleIDs, err := env.TimePermService.GetAccessibleProjects(env.UserID)
			if err != nil {
				return nil, err
			}
			if accessibleIDs != nil && len(accessibleIDs) == 0 {
				return listTimeProjectsOut{Projects: []timeProjectDTO{}}, nil
			}
			query := `SELECT tp.id, tp.name, tp.status, COALESCE(tp.description, ''),
			       COALESCE(co.name, ''), COALESCE(tpc.name, ''), tp.hourly_rate
			FROM time_projects tp
			LEFT JOIN customer_organisations co ON tp.customer_id = co.id
			LEFT JOIN time_project_categories tpc ON tp.category_id = tpc.id
			WHERE 1=1`
			var qa []interface{}
			if accessibleIDs != nil {
				ph := make([]string, len(accessibleIDs))
				for i, id := range accessibleIDs {
					ph[i] = "?"
					qa = append(qa, id)
				}
				query += fmt.Sprintf(" AND tp.id IN (%s)", strings.Join(ph, ","))
			}
			if args.Status != "" {
				query += " AND tp.status = ?"
				qa = append(qa, args.Status)
			}
			query += " ORDER BY tp.name"
			rows, err := env.DB.Query(query, qa...)
			if err != nil {
				return nil, err
			}
			defer func() { _ = rows.Close() }()
			out := listTimeProjectsOut{Projects: []timeProjectDTO{}}
			for rows.Next() {
				var p timeProjectDTO
				if err := rows.Scan(&p.ID, &p.Name, &p.Status, &p.Description, &p.CustomerName, &p.CategoryName, &p.HourlyRate); err != nil {
					continue
				}
				out.Projects = append(out.Projects, p)
			}
			return out, nil
		},
	})

	Register(Default, Tool[listWorklogsArgs]{
		Name:        "list_worklogs",
		Description: "List the current user's time tracking worklogs with optional date and project filters.",
		Run: func(_ context.Context, env *Env, args listWorklogsArgs) (any, error) {
			limit := args.Limit
			if limit <= 0 || limit > 200 {
				limit = 50
			}
			query := `SELECT tw.id, tw.project_id, tp.name, COALESCE(co.name, ''), tw.description, tw.date,
			       tw.duration_minutes, COALESCE(tw.item_id, 0),
			       COALESCE(i.workspace_item_number, 0), COALESCE(w.key, ''), COALESCE(i.workspace_id, 0)
			FROM time_worklogs tw
			JOIN time_projects tp ON tw.project_id = tp.id
			LEFT JOIN customer_organisations co ON tw.customer_id = co.id
			LEFT JOIN items i ON tw.item_id = i.id
			LEFT JOIN workspaces w ON i.workspace_id = w.id
			WHERE tw.user_id = ?`
			qa := []interface{}{env.UserID}
			if args.DateFrom != "" {
				t, err := time.Parse("2006-01-02", args.DateFrom)
				if err != nil {
					return map[string]string{"error": "invalid date_from format, use YYYY-MM-DD"}, nil
				}
				query += " AND tw.date >= ?"
				qa = append(qa, t.Unix())
			}
			if args.DateTo != "" {
				t, err := time.Parse("2006-01-02", args.DateTo)
				if err != nil {
					return map[string]string{"error": "invalid date_to format, use YYYY-MM-DD"}, nil
				}
				query += " AND tw.date <= ?"
				qa = append(qa, t.Add(24*time.Hour-time.Second).Unix())
			}
			if args.ProjectID != nil && *args.ProjectID > 0 {
				query += " AND tw.project_id = ?"
				qa = append(qa, *args.ProjectID)
			}
			query += " ORDER BY tw.date DESC LIMIT ?"
			qa = append(qa, limit)

			rows, err := env.DB.Query(query, qa...)
			if err != nil {
				return nil, err
			}
			defer func() { _ = rows.Close() }()
			out := listWorklogsOut{Worklogs: []worklogDTO{}}
			for rows.Next() {
				var w worklogDTO
				var dateUnix int64
				var itemID, itemNumber, wsID int
				var wsKey string
				if err := rows.Scan(&w.ID, &w.ProjectID, &w.ProjectName, &w.CustomerName, &w.Description,
					&dateUnix, &w.DurationMinutes, &itemID, &itemNumber, &wsKey, &wsID); err != nil {
					continue
				}
				w.Date = time.Unix(dateUnix, 0).UTC().Format("2006-01-02")
				if itemID > 0 && wsKey != "" && env.HasWorkspaceAccess(wsID) {
					w.ItemKey = fmt.Sprintf("%s-%d", wsKey, itemNumber)
					id := itemID
					w.ItemID = &id
				}
				out.Worklogs = append(out.Worklogs, w)
			}
			return out, nil
		},
	})

	Register(Default, Tool[logTimeArgs]{
		Name:        "log_time",
		Description: "Log a time entry on a time tracking project. Provide duration (e.g. '2h', '30m', '1h30m', '1d') OR duration_minutes OR start_time + end_time (HH:MM).",
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
			var projectName, projectStatus string
			var customerID sql.NullInt64
			err = env.DB.QueryRow("SELECT name, status, customer_id FROM time_projects WHERE id = ?", args.ProjectID).
				Scan(&projectName, &projectStatus, &customerID)
			if err != nil {
				return map[string]string{"error": "project not found"}, nil //nolint:nilerr // surface as a tool error in JSON, not as a protocol error
			}
			if projectStatus != "Active" {
				return map[string]string{"error": fmt.Sprintf("project %q is not active (status: %s)", projectName, projectStatus)}, nil
			}
			if !customerID.Valid {
				return map[string]string{"error": "project has no customer assigned, cannot log time"}, nil
			}
			date, err := time.Parse("2006-01-02", args.Date)
			if err != nil {
				return map[string]string{"error": "invalid date format, use YYYY-MM-DD"}, nil //nolint:nilerr // surface as a tool error in JSON, not as a protocol error
			}
			var durationMins int
			var startUnix, endUnix int64
			switch {
			case args.Duration != "":
				dur, err := utils.ParseDuration(args.Duration)
				if err != nil {
					return map[string]string{"error": fmt.Sprintf("invalid duration: %s", err.Error())}, nil
				}
				durationMins = int(dur.Minutes())
				startUnix = date.Unix()
				endUnix = date.Add(dur).Unix()
			case args.DurationMinutes > 0:
				durationMins = args.DurationMinutes
				startUnix = date.Unix()
				endUnix = date.Add(time.Duration(args.DurationMinutes) * time.Minute).Unix()
			case args.StartTime != "" && args.EndTime != "":
				sp, ep := strings.SplitN(args.StartTime, ":", 2), strings.SplitN(args.EndTime, ":", 2)
				if len(sp) != 2 || len(ep) != 2 {
					return map[string]string{"error": "start_time and end_time must be in HH:MM format"}, nil
				}
				sh, e1 := strconv.Atoi(sp[0])
				sm, e2 := strconv.Atoi(sp[1])
				eh, e3 := strconv.Atoi(ep[0])
				em, e4 := strconv.Atoi(ep[1])
				if e1 != nil || e2 != nil || e3 != nil || e4 != nil {
					return map[string]string{"error": "start_time and end_time must be in HH:MM format"}, nil //nolint:nilerr // surface as a tool error in JSON, not as a protocol error
				}
				st := date.Add(time.Duration(sh)*time.Hour + time.Duration(sm)*time.Minute)
				et := date.Add(time.Duration(eh)*time.Hour + time.Duration(em)*time.Minute)
				if !et.After(st) {
					return map[string]string{"error": "end_time must be after start_time"}, nil
				}
				durationMins = int(et.Sub(st).Minutes())
				startUnix, endUnix = st.Unix(), et.Unix()
			default:
				return map[string]string{"error": "provide duration, duration_minutes, or start_time and end_time"}, nil
			}
			if durationMins <= 0 {
				return map[string]string{"error": "duration must be positive"}, nil
			}
			var itemIDVal interface{}
			if args.ItemID != nil && *args.ItemID > 0 {
				wsID, err := repository.NewItemRepository(env.DB).GetWorkspaceID(*args.ItemID)
				if err != nil {
					return map[string]string{"error": "item not found"}, nil //nolint:nilerr // surface as a tool error in JSON, not as a protocol error
				}
				if !env.HasWorkspaceAccess(wsID) {
					return map[string]string{"error": "item not found"}, nil
				}
				itemIDVal = *args.ItemID
			}
			now := time.Now().Unix()
			var id int64
			err = env.DB.QueryRow(`
				INSERT INTO time_worklogs (project_id, customer_id, user_id, item_id, description, date, start_time, end_time, duration_minutes, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
				args.ProjectID, customerID.Int64, env.UserID, itemIDVal, args.Description,
				date.Unix(), startUnix, endUnix, durationMins, now, now,
			).Scan(&id)
			if err != nil {
				return nil, err
			}
			return logTimeOut{
				ID:              id,
				ProjectID:       args.ProjectID,
				ProjectName:     projectName,
				Date:            args.Date,
				DurationMinutes: durationMins,
				Description:     args.Description,
			}, nil
		},
	})

	Register(Default, Tool[startTimerArgs]{
		Name:        "start_timer",
		Description: "Start a time tracking timer. Only one timer can be active at a time.",
		Run: func(_ context.Context, env *Env, args startTimerArgs) (any, error) {
			timer, err := env.TimerService.StartTimer(env.UserID, args.WorkspaceID, args.ProjectID, args.ItemID, args.Description)
			if err != nil {
				if msg, ok := timerErrToToolMessage(err); ok {
					return map[string]string{"error": msg}, nil
				}
				return nil, err
			}
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
		Description: "Stop the user's currently running timer and create a worklog entry.",
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
