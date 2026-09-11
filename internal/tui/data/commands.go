package data

import tea "charm.land/bubbletea/v2"

// tea.Cmd constructors. Each wraps one Client call and emits either its
// typed *Msg or an ErrorMsg. They are package functions (not methods on a
// model) so any screen can fire them with just a *Client.

func LoadWorkspaces(c *Client, requestID uint64) tea.Cmd {
	return func() tea.Msg {
		workspaces, err := c.getWorkspaces()
		if err != nil {
			return ErrorMsg{Err: err.Error(), Operation: OpWorkspaces, RequestID: requestID}
		}
		return WorkspacesLoadedMsg{Workspaces: workspaces, RequestID: requestID}
	}
}

func LoadWorkItems(c *Client, workspaceID int, requestID uint64) tea.Cmd {
	return func() tea.Msg {
		items, truncated, err := c.getWorkItems(workspaceID)
		if err != nil {
			return ErrorMsg{Err: err.Error(), Operation: OpWorkItems, WorkspaceID: workspaceID, RequestID: requestID}
		}
		return WorkItemsLoadedMsg{Items: items, Truncated: truncated, WorkspaceID: workspaceID, RequestID: requestID}
	}
}

func LoadComments(c *Client, itemID int, requestID uint64) tea.Cmd {
	return func() tea.Msg {
		comments, err := c.getComments(itemID)
		if err != nil {
			return ErrorMsg{Err: err.Error(), Operation: OpComments, ItemID: itemID, RequestID: requestID}
		}
		return CommentsLoadedMsg{ItemID: itemID, Comments: comments, RequestID: requestID}
	}
}

func LoadStatuses(c *Client, workspaceID int, requestID uint64) tea.Cmd {
	return func() tea.Msg {
		statuses, err := c.getStatuses(workspaceID)
		if err != nil {
			return ErrorMsg{Err: err.Error(), Operation: OpStatuses, WorkspaceID: workspaceID, RequestID: requestID}
		}
		return StatusesLoadedMsg{Statuses: statuses, WorkspaceID: workspaceID, RequestID: requestID}
	}
}

func LoadPriorities(c *Client, workspaceID int, requestID uint64) tea.Cmd {
	return func() tea.Msg {
		priorities, err := c.getPriorities()
		if err != nil {
			return ErrorMsg{Err: err.Error(), Operation: OpPriorities, WorkspaceID: workspaceID, RequestID: requestID}
		}
		return PrioritiesLoadedMsg{Priorities: priorities, WorkspaceID: workspaceID, RequestID: requestID}
	}
}

func LoadTimeProjects(c *Client, workspaceID int, requestID uint64) tea.Cmd {
	return func() tea.Msg {
		projects, err := c.getTimeProjects()
		if err != nil {
			return ErrorMsg{Err: err.Error(), Operation: OpTimeProjects, WorkspaceID: workspaceID, RequestID: requestID}
		}
		return TimeProjectsLoadedMsg{Projects: projects, WorkspaceID: workspaceID, RequestID: requestID}
	}
}

func LoadAssignableUsers(c *Client, workspaceID int, requestID uint64) tea.Cmd {
	return func() tea.Msg {
		users, err := c.getAssignableUsers(workspaceID)
		if err != nil {
			return ErrorMsg{Err: err.Error(), Operation: OpUsers, WorkspaceID: workspaceID, RequestID: requestID}
		}
		return UsersLoadedMsg{Users: users, WorkspaceID: workspaceID, RequestID: requestID}
	}
}

// ReloadWorkItem refreshes one item after a mutation.
func ReloadWorkItem(c *Client, itemID int, operation Operation, requestID uint64) tea.Cmd {
	return func() tea.Msg {
		item, err := c.getWorkItem(itemID)
		if err != nil {
			return ErrorMsg{Err: err.Error(), Operation: operation, ItemID: itemID, RequestID: requestID}
		}
		return WorkItemLoadedMsg{Item: item, Operation: operation, RequestID: requestID}
	}
}

// SetItemStatus transitions an item, then refreshes it.
func SetItemStatus(c *Client, itemID, statusID int, requestID uint64) tea.Cmd {
	return func() tea.Msg {
		if err := c.setItemStatus(itemID, statusID); err != nil {
			return ErrorMsg{Err: err.Error(), Operation: OpItemStatus, ItemID: itemID, RequestID: requestID}
		}
		return ReloadWorkItem(c, itemID, OpItemStatus, requestID)()
	}
}

// SetItemPriority sets priority_id only, then refreshes the item.
func SetItemPriority(c *Client, itemID, priorityID int, requestID uint64) tea.Cmd {
	return func() tea.Msg {
		if err := c.setItemField(itemID, "priority_id", priorityID); err != nil {
			return ErrorMsg{Err: err.Error(), Operation: OpItemPriority, ItemID: itemID, RequestID: requestID}
		}
		return ReloadWorkItem(c, itemID, OpItemPriority, requestID)()
	}
}

// SetItemAssignee sets assignee_id only (0 unassigns), then refreshes.
func SetItemAssignee(c *Client, itemID, assigneeID int, requestID uint64) tea.Cmd {
	return func() tea.Msg {
		var v any
		if assigneeID > 0 {
			v = assigneeID
		} else {
			v = nil
		}
		if err := c.setItemField(itemID, "assignee_id", v); err != nil {
			return ErrorMsg{Err: err.Error(), Operation: OpItemAssignee, ItemID: itemID, RequestID: requestID}
		}
		return ReloadWorkItem(c, itemID, OpItemAssignee, requestID)()
	}
}

// LoadAgentRuns fetches an item's coding-agent run history. Failures are
// silent — the agent panel is informational, an error toast on every
// selection would be noise.
func LoadAgentRuns(c *Client, itemID int, requestID uint64) tea.Cmd {
	return func() tea.Msg {
		runs, err := c.getAgentRuns(itemID)
		if err != nil {
			return ErrorMsg{Err: err.Error(), Operation: OpAgentRuns, ItemID: itemID, RequestID: requestID}
		}
		return AgentRunsLoadedMsg{ItemID: itemID, Runs: runs, RequestID: requestID}
	}
}

// LoadPrefs fetches the persisted TUI preferences. Preferences are optional,
// so failure leaves the session on its local defaults.
func LoadPrefs(c *Client) tea.Cmd {
	return func() tea.Msg {
		p, err := c.getPrefs()
		if err != nil {
			return PrefsLoadedMsg{OK: false}
		}
		return PrefsLoadedMsg{Prefs: p, OK: true}
	}
}

func LoadCurrentUser(c *Client) tea.Cmd {
	return func() tea.Msg {
		timezone, err := c.getCurrentUserTimezone()
		if err != nil {
			return CurrentUserLoadedMsg{OK: false}
		}
		return CurrentUserLoadedMsg{Timezone: timezone, OK: true}
	}
}

// SavePrefs reports completion so the root can serialize full-snapshot writes.
func SavePrefs(c *Client, p Prefs, version uint64) tea.Cmd {
	return func() tea.Msg {
		if err := c.putPrefs(p); err != nil {
			return PrefsSavedMsg{Version: version, Err: err.Error()}
		}
		return PrefsSavedMsg{Version: version}
	}
}

func UpdateWorkItem(c *Client, workspaceID, itemID int, title, description string, statusID, priorityID *int, assigneeSet bool, assigneeID int, requestID uint64) tea.Cmd {
	return func() tea.Msg {
		if err := c.updateWorkItem(itemID, title, description, statusID, priorityID, assigneeSet, assigneeID); err != nil {
			return ErrorMsg{Err: err.Error(), Operation: OpItemUpdate, WorkspaceID: workspaceID, ItemID: itemID, RequestID: requestID}
		}
		return ReloadWorkItem(c, itemID, OpItemUpdate, requestID)()
	}
}

func CreateWorkItem(c *Client, workspaceID int, title, description string, priorityID *int, requestID uint64) tea.Cmd {
	return func() tea.Msg {
		if err := c.createWorkItem(workspaceID, title, description, priorityID); err != nil {
			return ErrorMsg{Err: err.Error(), Operation: OpItemCreate, WorkspaceID: workspaceID, RequestID: requestID}
		}
		return WorkItemCreatedMsg{WorkspaceID: workspaceID, RequestID: requestID}
	}
}

func CreateComment(c *Client, itemID int, content string, requestID uint64) tea.Cmd {
	return func() tea.Msg {
		if err := c.createComment(itemID, content); err != nil {
			return ErrorMsg{Err: err.Error(), Operation: OpCommentCreate, ItemID: itemID, RequestID: requestID}
		}
		return CommentCreatedMsg{ItemID: itemID, RequestID: requestID}
	}
}

func CreateTimeLog(c *Client, itemID, projectID int, description, duration, date, startTime, timezone string, requestID uint64) tea.Cmd {
	return func() tea.Msg {
		if err := c.createTimeLog(itemID, projectID, description, duration, date, startTime, timezone); err != nil {
			return ErrorMsg{Err: err.Error(), Operation: OpTimeLogCreate, ItemID: itemID, RequestID: requestID}
		}
		return TimeLogCreatedMsg{ItemID: itemID, RequestID: requestID}
	}
}
