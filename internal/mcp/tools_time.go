package mcp

import (
	"context"
	"database/sql"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (ms *MCPServer) registerTimeTools() {
	type listTimeProjectsInput struct{}
	mcp.AddTool(ms.server, &mcp.Tool{
		Name:        "list_time_projects",
		Description: "List time tracking projects the user has access to.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args listTimeProjectsInput) (*mcp.CallToolResult, any, error) {
		user := userFromContext(ctx)
		if user == nil {
			return errNoAuth()
		}

		// Get accessible project IDs (nil means all access)
		accessibleIDs, err := ms.deps.TimePermissionService.GetAccessibleProjects(user.ID)
		if err != nil {
			return errInternal("list time projects", err)
		}

		var rows *sql.Rows
		switch {
		case accessibleIDs == nil:
			// Full access - get all active projects
			rows, err = ms.deps.DB.Query(`
				SELECT tp.id, tp.name, tp.description, tp.status, tp.hourly_rate,
				       COALESCE(co.name, '') as customer_name
				FROM time_projects tp
				LEFT JOIN customer_organisations co ON tp.customer_id = co.id
				WHERE tp.status = 'Active'
				ORDER BY tp.name
			`)
		case len(accessibleIDs) == 0:
			// No access
			return toolJSON(map[string]any{"projects": []any{}})
		default:
			// Build IN clause
			query := `
				SELECT tp.id, tp.name, tp.description, tp.status, tp.hourly_rate,
				       COALESCE(co.name, '') as customer_name
				FROM time_projects tp
				LEFT JOIN customer_organisations co ON tp.customer_id = co.id
				WHERE tp.status = 'Active' AND tp.id IN (`
			args2 := make([]interface{}, len(accessibleIDs))
			for i, id := range accessibleIDs {
				if i > 0 {
					query += ","
				}
				query += "?"
				args2[i] = id
			}
			query += `) ORDER BY tp.name`
			rows, err = ms.deps.DB.Query(query, args2...)
		}
		if err != nil {
			return errInternal("list time projects", err)
		}
		defer rows.Close()

		var projects []map[string]any
		for rows.Next() {
			var id int
			var name, description, status, customerName string
			var hourlyRate float64
			if err := rows.Scan(&id, &name, &description, &status, &hourlyRate, &customerName); err != nil {
				continue
			}
			p := map[string]any{"id": id, "name": name, "status": status}
			if description != "" {
				p["description"] = description
			}
			if hourlyRate > 0 {
				p["hourly_rate"] = hourlyRate
			}
			if customerName != "" {
				p["customer_name"] = customerName
			}
			projects = append(projects, p)
		}
		if projects == nil {
			projects = []map[string]any{}
		}

		return toolJSON(map[string]any{"projects": projects})
	})

	type listWorklogsInput struct {
		ProjectID *int   `json:"project_id,omitempty" jsonschema:"Filter by project ID"`
		DateFrom  string `json:"date_from,omitempty" jsonschema:"Start date filter (YYYY-MM-DD)"`
		DateTo    string `json:"date_to,omitempty" jsonschema:"End date filter (YYYY-MM-DD)"`
		Limit     int    `json:"limit,omitempty" jsonschema:"Max results (default 50)"`
	}
	mcp.AddTool(ms.server, &mcp.Tool{
		Name:        "list_worklogs",
		Description: "List the authenticated user's time tracking worklogs.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args listWorklogsInput) (*mcp.CallToolResult, any, error) {
		user := userFromContext(ctx)
		if user == nil {
			return errNoAuth()
		}

		limit := args.Limit
		if limit <= 0 || limit > 200 {
			limit = 50
		}

		query := `
			SELECT w.id, w.project_id, w.description, w.date, w.start_time, w.end_time,
			       w.duration_minutes, w.created_at,
			       COALESCE(tp.name, '') as project_name,
			       w.item_id,
			       COALESCE(i.title, '') as item_title
			FROM time_worklogs w
			LEFT JOIN time_projects tp ON w.project_id = tp.id
			LEFT JOIN items i ON w.item_id = i.id
			WHERE w.user_id = ?`
		queryArgs := []interface{}{user.ID}

		if args.ProjectID != nil {
			query += " AND w.project_id = ?"
			queryArgs = append(queryArgs, *args.ProjectID)
		}
		if args.DateFrom != "" {
			t, err := time.Parse("2006-01-02", args.DateFrom)
			if err == nil {
				query += " AND w.date >= ?"
				queryArgs = append(queryArgs, t.Unix())
			}
		}
		if args.DateTo != "" {
			t, err := time.Parse("2006-01-02", args.DateTo)
			if err == nil {
				// End of day
				query += " AND w.date <= ?"
				queryArgs = append(queryArgs, t.Add(24*time.Hour-time.Second).Unix())
			}
		}

		query += " ORDER BY w.date DESC, w.start_time DESC LIMIT ?"
		queryArgs = append(queryArgs, limit)

		rows, err := ms.deps.DB.Query(query, queryArgs...)
		if err != nil {
			return errInternal("list worklogs", err)
		}
		defer rows.Close()

		var worklogs []map[string]any
		for rows.Next() {
			var id, projectID, durationMins int
			var dateUnix, startTimeUnix, endTimeUnix, createdAtUnix int64
			var description, projectName, itemTitle string
			var itemID sql.NullInt64

			if err := rows.Scan(&id, &projectID, &description, &dateUnix, &startTimeUnix, &endTimeUnix,
				&durationMins, &createdAtUnix, &projectName, &itemID, &itemTitle); err != nil {
				continue
			}

			wl := map[string]any{
				"id":               id,
				"project_id":       projectID,
				"description":      description,
				"date":             time.Unix(dateUnix, 0).UTC().Format("2006-01-02"),
				"duration_minutes": durationMins,
			}
			if projectName != "" {
				wl["project_name"] = projectName
			}
			if itemID.Valid {
				wl["item_id"] = itemID.Int64
			}
			if itemTitle != "" {
				wl["item_title"] = itemTitle
			}
			worklogs = append(worklogs, wl)
		}
		if worklogs == nil {
			worklogs = []map[string]any{}
		}

		return toolJSON(map[string]any{"worklogs": worklogs})
	})

	type createWorklogInput struct {
		ProjectID   int    `json:"project_id" jsonschema:"Time project ID"`
		Description string `json:"description,omitempty" jsonschema:"Worklog description"`
		Date        string `json:"date" jsonschema:"Date in YYYY-MM-DD format"`
		Duration    int    `json:"duration" jsonschema:"Duration in minutes"`
		ItemID      *int   `json:"item_id,omitempty" jsonschema:"Optional linked work item ID"`
	}
	mcp.AddTool(ms.server, &mcp.Tool{
		Name:        "create_worklog",
		Description: "Create a time tracking worklog entry.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args createWorklogInput) (*mcp.CallToolResult, any, error) {
		user := userFromContext(ctx)
		if user == nil {
			return errNoAuth()
		}

		if args.Duration <= 0 {
			return toolErrorResult("duration must be positive")
		}

		date, err := time.Parse("2006-01-02", args.Date)
		if err != nil {
			return toolErrorResult("invalid date format, use YYYY-MM-DD")
		}

		// Check project booking permission
		canBook, err := ms.deps.TimePermissionService.CanBookTimeOnProject(user.ID, args.ProjectID)
		if err != nil {
			return errInternal("check permission", err)
		}
		if !canBook {
			return toolErrorResult("no permission to book time on this project")
		}

		// Get customer_id from project
		var customerID int
		err = ms.deps.DB.QueryRow("SELECT COALESCE(customer_id, 0) FROM time_projects WHERE id = ?", args.ProjectID).Scan(&customerID)
		if err == sql.ErrNoRows {
			return toolErrorResult("project not found")
		}
		if err != nil {
			return errInternal("get project", err)
		}

		dateUnix := date.Unix()
		startTimeUnix := dateUnix // start at beginning of day
		endTimeUnix := startTimeUnix + int64(args.Duration*60)
		nowUnix := time.Now().UTC().Unix()

		result, err := ms.deps.DB.ExecWrite(`
			INSERT INTO time_worklogs (project_id, customer_id, user_id, item_id, description, date, start_time, end_time, duration_minutes, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			args.ProjectID, customerID, user.ID, args.ItemID, args.Description,
			dateUnix, startTimeUnix, endTimeUnix, args.Duration, nowUnix, nowUnix,
		)
		if err != nil {
			return errInternal("create worklog", err)
		}

		id, _ := result.LastInsertId()
		return toolJSON(map[string]any{
			"id":               id,
			"project_id":       args.ProjectID,
			"date":             args.Date,
			"duration_minutes": args.Duration,
			"created":          true,
		})
	})

	type startTimerInput struct {
		ProjectID   int    `json:"project_id" jsonschema:"Time project ID"`
		WorkspaceID int    `json:"workspace_id" jsonschema:"Workspace ID"`
		Description string `json:"description" jsonschema:"Timer description"`
		ItemID      *int   `json:"item_id,omitempty" jsonschema:"Optional linked work item ID"`
	}
	mcp.AddTool(ms.server, &mcp.Tool{
		Name:        "start_timer",
		Description: "Start a time tracking timer. Only one timer can be active at a time.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args startTimerInput) (*mcp.CallToolResult, any, error) {
		user := userFromContext(ctx)
		if user == nil {
			return errNoAuth()
		}

		if args.Description == "" {
			return toolErrorResult("description is required")
		}

		// Check project booking permission
		canBook, err := ms.deps.TimePermissionService.CanBookTimeOnProject(user.ID, args.ProjectID)
		if err != nil {
			return errInternal("check permission", err)
		}
		if !canBook {
			return toolErrorResult("no permission to book time on this project")
		}

		// Check project is active
		var projectStatus string
		err = ms.deps.DB.QueryRow("SELECT status FROM time_projects WHERE id = ?", args.ProjectID).Scan(&projectStatus)
		if err == sql.ErrNoRows {
			return toolErrorResult("project not found")
		}
		if err != nil {
			return errInternal("check project", err)
		}
		if projectStatus != "Active" {
			return toolErrorResult("project is not active")
		}

		// Check no existing timer
		var existingID int
		err = ms.deps.DB.QueryRow("SELECT id FROM active_timers WHERE user_id = ? LIMIT 1", user.ID).Scan(&existingID)
		if err != sql.ErrNoRows {
			if err != nil {
				return errInternal("check timer", err)
			}
			return toolErrorResult("a timer is already running - stop it first")
		}

		now := time.Now().UTC().Unix()
		var id int64
		err = ms.deps.DB.QueryRow(`
			INSERT INTO active_timers (workspace_id, item_id, project_id, user_id, description, start_time_utc, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id`,
			args.WorkspaceID, args.ItemID, args.ProjectID, user.ID, args.Description, now, now,
		).Scan(&id)
		if err != nil {
			return errInternal("start timer", err)
		}

		return toolJSON(map[string]any{
			"id":             id,
			"description":    args.Description,
			"start_time_utc": now,
			"started":        true,
		})
	})

	type stopTimerInput struct{}
	mcp.AddTool(ms.server, &mcp.Tool{
		Name:        "stop_timer",
		Description: "Stop the user's currently running timer and create a worklog entry.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args stopTimerInput) (*mcp.CallToolResult, any, error) {
		user := userFromContext(ctx)
		if user == nil {
			return errNoAuth()
		}

		// Find active timer
		var timerID, projectID, workspaceID int
		var description string
		var startTimeUTC int64
		var itemID sql.NullInt64

		err := ms.deps.DB.QueryRow(`
			SELECT id, workspace_id, item_id, project_id, description, start_time_utc
			FROM active_timers WHERE user_id = ? LIMIT 1`,
			user.ID,
		).Scan(&timerID, &workspaceID, &itemID, &projectID, &description, &startTimeUTC)
		if err == sql.ErrNoRows {
			return toolErrorResult("no active timer running")
		}
		if err != nil {
			return errInternal("find timer", err)
		}

		// Calculate duration
		endTimeUTC := time.Now().UTC().Unix()
		durationSeconds := endTimeUTC - startTimeUTC
		durationMinutes := int(durationSeconds / 60)

		// Get customer_id from project
		var customerID int
		err = ms.deps.DB.QueryRow("SELECT COALESCE(customer_id, 0) FROM time_projects WHERE id = ?", projectID).Scan(&customerID)
		if err != nil {
			return errInternal("get project", err)
		}

		// Create worklog
		startTime := time.Unix(startTimeUTC, 0).UTC()
		dateUnix := startTime.Truncate(24 * time.Hour).Unix()
		nowUnix := time.Now().UTC().Unix()

		var itemIDVal interface{}
		if itemID.Valid {
			itemIDVal = itemID.Int64
		}

		_, err = ms.deps.DB.ExecWrite(`
			INSERT INTO time_worklogs (project_id, customer_id, user_id, item_id, description, date, start_time, end_time, duration_minutes, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			projectID, customerID, user.ID, itemIDVal, description,
			dateUnix, startTimeUTC, endTimeUTC, durationMinutes, nowUnix, nowUnix,
		)
		if err != nil {
			return errInternal("create worklog", err)
		}

		// Delete the active timer
		_, err = ms.deps.DB.ExecWrite("DELETE FROM active_timers WHERE id = ?", timerID)
		if err != nil {
			return errInternal("delete timer", err)
		}

		return toolJSON(map[string]any{
			"stopped":          true,
			"timer_id":         timerID,
			"description":      description,
			"duration_seconds": durationSeconds,
			"duration_minutes": durationMinutes,
			"worklog_created":  true,
		})
	})
}
