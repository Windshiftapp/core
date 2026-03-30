package llm

import "encoding/json"

// BuiltinTools returns the tool definitions available to the agentic chat.
func BuiltinTools() []ToolDefinition {
	return []ToolDefinition{
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "list_workspaces",
				Description: "List all workspaces the user has access to. Returns workspace ID, name, key, and description.",
				Parameters:  json.RawMessage(`{"type":"object","properties":{}}`),
			},
		},
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "get_workspace",
				Description: "Get details of a specific workspace by its ID.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"workspace_id": {"type": "integer", "description": "The workspace ID"}
					},
					"required": ["workspace_id"]
				}`),
			},
		},
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "list_items",
				Description: "List items (tasks, issues, etc.) in one or all workspaces. Returns item key, title, status, priority, assignee, due date, milestone, and iteration.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"workspace_id": {"type": "integer", "description": "Workspace ID. If omitted, queries all accessible workspaces."},
						"status": {"type": "string", "description": "Filter by status name (optional)"},
						"assignee_id": {"type": "integer", "description": "Filter by assignee user ID (optional)"},
						"limit": {"type": "integer", "description": "Max items to return (default 20, max 50)"},
						"filter": {"type": "string", "description": "CQL filter expression for advanced filtering. Supported fields: status, priority, assignee, creator, due_date, created, updated, label, milestone, milestonename, iteration, iterationname, project, itemtype, cf_<name>, custom.<name>. Operators: =, !=, <, <=, >, >=, ~ (contains), IN, NOT IN. Logical: AND, OR, NOT. Functions: currentUser(), now(), startOfDay(), endOfDay(). Examples: 'status = \"Open\" AND milestonename = \"v2.0\"', 'priority IN (\"high\", \"critical\") AND cf_team ~ \"platform\"', 'assignee = currentUser() AND due_date < now()'. Use list_custom_fields to discover available custom field names."}
					}
				}`),
			},
		},
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "get_item",
				Description: "Get details of a specific item by its numeric ID or item key (e.g. 'PROJ-42').",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"item_id": {"type": "integer", "description": "The item numeric ID"},
						"item_key": {"type": "string", "description": "The item key like PROJ-42"}
					}
				}`),
			},
		},
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "list_milestones",
				Description: "List milestones. Returns ID, name, status, target_date, category, and workspace info.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"workspace_id": {"type": "integer", "description": "Filter to a specific workspace (optional)"},
						"status": {"type": "string", "description": "Filter by status: planning, in-progress, completed, canceled (optional)"},
						"include_global": {"type": "boolean", "description": "Include cross-workspace milestones (default true)"}
					}
				}`),
			},
		},
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "list_iterations",
				Description: "List iterations (sprints, PIs, releases). Returns ID, name, status, start_date, end_date, type, and workspace info.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"workspace_id": {"type": "integer", "description": "Filter to a specific workspace (optional)"},
						"status": {"type": "string", "description": "Filter by status: planned, active, completed, canceled (optional)"},
						"include_global": {"type": "boolean", "description": "Include cross-workspace iterations (default true)"}
					}
				}`),
			},
		},
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "list_custom_fields",
				Description: "List available custom field definitions. Use this to discover what custom fields exist before filtering items with cf_<name> in the filter parameter of list_items.",
				Parameters:  json.RawMessage(`{"type":"object","properties":{}}`),
			},
		},
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "search_items",
				Description: "Search for items by text query across accessible workspaces. Searches title and description.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"query": {"type": "string", "description": "Search text"},
						"workspace_ids": {
							"type": "array",
							"items": {"type": "integer"},
							"description": "Optional list of workspace IDs to search in. If empty, searches all accessible workspaces."
						}
					},
					"required": ["query"]
				}`),
			},
		},
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "list_time_projects",
				Description: "List time tracking projects the user has access to. Returns project ID, name, status, customer name, category, and description.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"status": {"type": "string", "description": "Filter by project status, e.g. 'Active', 'Archived' (optional)"}
					}
				}`),
			},
		},
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "list_worklogs",
				Description: "List the current user's time tracking worklogs. Returns worklog ID, project name, customer, description, date, duration in minutes, and linked item key if visible.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"date_from": {"type": "string", "description": "Start date filter in YYYY-MM-DD format (optional)"},
						"date_to": {"type": "string", "description": "End date filter in YYYY-MM-DD format (optional)"},
						"project_id": {"type": "integer", "description": "Filter by time project ID (optional)"}
					}
				}`),
			},
		},
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "list_recent_activity",
				Description: "List recent changes and comments across accessible workspaces. Useful for understanding what happened recently, such as 'what changed yesterday?'.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"since_date": {"type": "string", "description": "Start date in YYYY-MM-DD format (defaults to yesterday)"},
						"workspace_id": {"type": "integer", "description": "Filter to a specific workspace (optional)"},
						"limit": {"type": "integer", "description": "Max items to return (default 50, max 100)"}
					}
				}`),
			},
		},
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "update_item",
				Description: "Update an item's fields. Identify the item by item_id or item_key. For status, priority, milestone, iteration, and assignee you can pass either the numeric _id or a _name which will be resolved automatically. Pass null to clear optional fields like due_date, milestone, iteration, project, or parent.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"item_id": {"type": "integer", "description": "The item numeric ID"},
						"item_key": {"type": "string", "description": "The item key like PROJ-42"},
						"title": {"type": "string", "description": "New title for the item"},
						"description": {"type": "string", "description": "New description for the item"},
						"status_id": {"type": "integer", "description": "Status ID to set"},
						"status_name": {"type": "string", "description": "Status name to resolve (e.g. 'Done', 'In Progress')"},
						"priority_id": {"type": "integer", "description": "Priority ID to set"},
						"priority_name": {"type": "string", "description": "Priority name to resolve (e.g. 'High', 'Critical')"},
						"assignee_id": {"type": "integer", "description": "Assignee user ID to set"},
						"assignee_name": {"type": "string", "description": "Assignee full name to resolve (e.g. 'John Doe')"},
						"due_date": {"type": ["string", "null"], "description": "Due date in YYYY-MM-DD format, or null to clear"},
						"milestone_id": {"type": ["integer", "null"], "description": "Milestone ID to set, or null to clear"},
						"milestone_name": {"type": "string", "description": "Milestone name to resolve (e.g. 'v2.0')"},
						"iteration_id": {"type": ["integer", "null"], "description": "Iteration ID to set, or null to clear"},
						"iteration_name": {"type": "string", "description": "Iteration name to resolve (e.g. 'Sprint 3')"},
						"project_id": {"type": ["integer", "null"], "description": "Time project ID to set, or null to clear"},
						"parent_id": {"type": ["integer", "null"], "description": "Parent item ID to set, or null to clear"},
						"custom_field_values": {"type": "object", "description": "Custom field values as key-value pairs"}
					}
				}`),
			},
		},
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "log_time",
				Description: "Log a time entry on a time tracking project. Requires project_id, description, and date. Provide either duration (e.g. '2h', '30m', '1h30m', '1d') OR start_time and end_time (HH:MM format). Optionally link to an item by item_id.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"project_id": {"type": "integer", "description": "The time project ID to log time on"},
						"description": {"type": "string", "description": "Description of the work done"},
						"date": {"type": "string", "description": "Date of the worklog in YYYY-MM-DD format"},
						"duration": {"type": "string", "description": "Duration string like '2h', '30m', '1h30m', '1d' (1d = 8h). Required if start_time/end_time not provided."},
						"start_time": {"type": "string", "description": "Start time in HH:MM format. Must be used together with end_time."},
						"end_time": {"type": "string", "description": "End time in HH:MM format. Must be used together with start_time."},
						"item_id": {"type": "integer", "description": "Optional item ID to link this worklog to"}
					},
					"required": ["project_id", "description", "date"]
				}`),
			},
		},
	}
}
