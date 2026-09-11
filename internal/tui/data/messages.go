package data

// Message types emitted by the tea.Cmd constructors in commands.go and
// consumed by whichever screen (or the root model) cares about them.

type Operation string

const (
	OpWorkspaces    Operation = "workspaces"
	OpWorkItems     Operation = "work_items"
	OpStatuses      Operation = "statuses"
	OpPriorities    Operation = "priorities"
	OpTimeProjects  Operation = "time_projects"
	OpUsers         Operation = "users"
	OpComments      Operation = "comments"
	OpAgentRuns     Operation = "agent_runs"
	OpItemDetails   Operation = "item_details"
	OpItemCreate    Operation = "item_create"
	OpItemUpdate    Operation = "item_update"
	OpItemStatus    Operation = "item_status"
	OpItemPriority  Operation = "item_priority"
	OpItemAssignee  Operation = "item_assignee"
	OpCommentCreate Operation = "comment_create"
	OpTimeLogCreate Operation = "time_log_create"
)

type WorkspacesLoadedMsg struct {
	Workspaces []Workspace
	RequestID  uint64
}

type WorkItemsLoadedMsg struct {
	Items []WorkItem
	// Truncated is set when the workspace has more items than the client-side
	// page-accumulation cap; the board surfaces this instead of silently
	// showing a partial list.
	Truncated   bool
	WorkspaceID int
	RequestID   uint64
}

type CommentsLoadedMsg struct {
	// ItemID keys the result so late responses land in the right cache slot
	// even if the selection moved on.
	ItemID    int
	Comments  []Comment
	RequestID uint64
}

type WorkItemUpdatedMsg struct {
	WorkspaceID int
	ItemID      int
	RequestID   uint64
}

type WorkItemCreatedMsg struct {
	WorkspaceID int
	RequestID   uint64
}

type CommentCreatedMsg struct {
	ItemID    int
	RequestID uint64
}

type TimeLogCreatedMsg struct {
	ItemID    int
	RequestID uint64
}

type TimeProjectsLoadedMsg struct {
	Projects    []TimeProject
	WorkspaceID int
	RequestID   uint64
}

type StatusesLoadedMsg struct {
	Statuses    []Status
	WorkspaceID int
	RequestID   uint64
}

type PrioritiesLoadedMsg struct {
	Priorities  []Priority
	WorkspaceID int
	RequestID   uint64
}

type UsersLoadedMsg struct {
	Users       []User
	WorkspaceID int
	RequestID   uint64
}

// WorkItemLoadedMsg is a single-item refresh (after a quick-set mutation).
type WorkItemLoadedMsg struct {
	Item      WorkItem
	Operation Operation
	RequestID uint64
}

// AgentRunsLoadedMsg delivers an item's coding-agent run history, keyed by
// ItemID like CommentsLoadedMsg.
type AgentRunsLoadedMsg struct {
	ItemID    int
	Runs      []AgentRun
	RequestID uint64
}

// PrefsLoadedMsg delivers the persisted TUI preferences. OK is false when
// the load failed — startup proceeds with defaults, never blocks.
type PrefsLoadedMsg struct {
	Prefs Prefs
	OK    bool
}

type PrefsSavedMsg struct {
	Version uint64
	Err     string
}

type CurrentUserLoadedMsg struct {
	Timezone string
	OK       bool
}

// ErrorMsg carries a human-readable (already sanitized) error string from a
// failed loader/mutator.
type ErrorMsg struct {
	Err         string
	Operation   Operation
	WorkspaceID int
	ItemID      int
	RequestID   uint64
}
