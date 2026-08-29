package database

type defaultStatusCategory struct {
	builtinKey  string
	name        string
	color       string
	description string
	isDefault   bool
	isCompleted bool
}

var defaultStatusCategories = []defaultStatusCategory{
	{"to_do", "To Do", "#d1d5db", "Work that hasn't been started", false, false},
	{"in_progress", "In Progress", "#3b82f6", "Work that is actively being done", true, false},
	{"done", "Done", "#22c55e", "Work that has been completed", false, true},
}

type defaultStatus struct {
	builtinKey  string
	name        string
	description string
	category    string
	isDefault   bool
}

var defaultStatuses = []defaultStatus{
	{"open", "Open", "New work item, not yet started", "To Do", true},
	{"in_progress", "In Progress", "Currently being worked on", "In Progress", false},
	{"done", "Done", "Work has been completed", "Done", false},
}

type defaultTransition struct {
	from string
	to   string
}

var defaultTransitions = []defaultTransition{
	{"", "Open"},
	{"Open", "In Progress"},
	{"Open", "Done"},
	{"In Progress", "Done"},
}

type defaultScreenField struct {
	fieldType       string
	fieldIdentifier string
	displayOrder    int
	isRequired      bool
	fieldWidth      string
}

var defaultScreenFields = []defaultScreenField{
	{"system", "title", 1, true, "full"},
	{"system", "description", 2, false, "full"},
	{"system", "status", 3, true, "half"},
	{"system", "priority", 4, false, "half"},
	{"system", "assignee", 5, false, "half"},
	{"system", "due_date", 6, false, "half"},
	{"system", "milestone", 7, false, "half"},
	{"system", "iteration", 8, false, "half"},
	{"system", "start_date", 9, false, "half"},
	{"system", "end_date", 10, false, "half"},
	{"system", "labels", 11, false, "full"},
}

var defaultScreenContexts = []string{"create", "edit", "view"}

type defaultLinkType struct {
	builtinKey         string
	name               string
	description        string
	forwardLabel       string
	reverseLabel       string
	color              string
	isSystem           bool
	allowedEntityTypes *string
}

var defaultLinkTypes = []defaultLinkType{
	{"tests", "Tests", "Test case tests work item", "tests", "tested by", "#10b981", true, strPtr(`["item","test_case"]`)},
	{"implements", "Implements", "Work item implements another work item", "implements", "implemented by", "#3b82f6", true, nil},
	{"depends_on", "Depends On", "Work item depends on another work item", "depends on", "blocks", "#f59e0b", true, nil},
	{"relates_to", "Relates To", "General bidirectional relationship", "relates to", "relates to", "#6b7280", true, nil},
	{"links_to", "Links To", "General directional link", "links to", "linked from", "#64748b", true, nil},
	{"duplicates", "Duplicates", "Work item is a duplicate of another", "duplicates", "duplicated by", "#ef4444", true, nil},
	{"child_of", "Child Of", "Alternative hierarchy relationship", "child of", "parent of", "#8b5cf6", true, nil},
	{"page", "Page", "Work item references a knowledge page", "references page", "referenced by", "#0ea5e9", true, strPtr(`["item","page"]`)},
}

type defaultSystemSetting struct {
	key         string
	value       string
	valueType   string
	description string
	category    string
}

var defaultSystemSettings = []defaultSystemSetting{
	{"time_tracking_enabled", "true", "boolean", "Enable time tracking functionality", "modules"},
	{"test_management_enabled", "true", "boolean", "Enable test management functionality", "modules"},
	{"ai_chat_enabled", "true", "boolean", "Enable AI chat functionality", "modules"},
	{"ai_feature_config", "{}", "json", "Per-feature AI LLM configuration", "ai"},
	{"setup_completed", "false", "boolean", "Whether initial setup has been completed", "setup"},
	{"admin_user_created", "false", "boolean", "Whether admin user has been created", "setup"},
	{"calendar_feed_enabled", "true", "boolean", "Allow users to generate ICS calendar feed URLs", "security"},
	{"plugin_cli_exec_enabled", "false", "boolean", "Allow plugins to execute CLI commands", "security"},
	{"max_custom_field_indexes_per_table", "20", "integer", "Maximum number of custom field indexes per table", "performance"},
	{"recurrence_volume_diagnostic_enabled", "true", "boolean", "Enable recurrence rule volume warnings in system diagnostics", "diagnostics"},
	{"recurrence_volume_warning_threshold", "80", "integer", "Recurrence rules per workspace that trigger an administrator warning", "diagnostics"},
}

type defaultHierarchyLevel struct {
	builtinKey  string
	level       int
	name        string
	description string
}

var defaultHierarchyLevels = []defaultHierarchyLevel{
	{"initiative", 0, "Initiative", "High-level strategic work spanning multiple epics"},
	{"epic", 1, "Epic", "Large work item that can be broken down into stories"},
	{"story", 2, "Story", "User story or feature that delivers value"},
	{"task", 3, "Task", "Individual work item or technical task"},
	{"activity", 4, "Activity", "Discrete activity within a task"},
}

type defaultItemType struct {
	builtinKey     string
	name           string
	description    string
	icon           string
	color          string
	hierarchyLevel int
	sortOrder      int
}

var defaultItemTypes = []defaultItemType{
	{"initiative", "Initiative", "Strategic initiative spanning multiple teams", "Target", "#7c3aed", 0, 1},
	{"epic", "Epic", "Large feature or capability", "Zap", "#2563eb", 1, 1},
	{"story", "Story", "User story delivering value to end users", "BookOpen", "#059669", 2, 1},
	{"task", "Task", "Development or operational task", "CheckSquare", "#dc2626", 3, 1},
	{"bug", "Bug", "Software defect that needs fixing", "Bug", "#ea580c", 3, 2},
	{"subtask", "Sub-task", "Small work item below any regular hierarchy level", "Minus", "#6b7280", -1, 1},
}

var defaultItemTypeBindings = []string{"Epic", "Story", "Task", "Bug", "Sub-task"}

const defaultNotificationChannelConfig = `{
	"smtp_host": "",
	"smtp_port": 587,
	"smtp_username": "",
	"smtp_password": "",
	"smtp_from_email": "",
	"smtp_from_name": "Windshift",
	"smtp_encryption": "tls"
}`

type defaultTheme struct {
	builtinKey              string
	name                    string
	description             string
	isDefault               bool
	isActive                bool
	navBackgroundColorLight string
	navTextColorLight       string
	navBackgroundColorDark  string
	navTextColorDark        string
}

var defaultThemes = []defaultTheme{
	{"default", "Default", "Clean theme with standard navigation colors", true, true, "#ffffff", "#374151", "#1f2937", "#f3f4f6"},
	{"ocean", "Ocean", "Professional blue-tinted navigation theme", false, false, "#f0f9ff", "#0c4a6e", "#0c4a6e", "#e0f2fe"},
	{"forest", "Forest", "Nature-inspired green navigation theme", false, false, "#f0fdf4", "#14532d", "#14532d", "#dcfce7"},
}

type defaultNotificationEventRule struct {
	eventType             string
	notifyAssignee        bool
	notifyCreator         bool
	notifyWatchers        bool
	notifyWorkspaceAdmins bool
}

var defaultNotificationEventRules = []defaultNotificationEventRule{
	{"item.assigned", true, false, false, false},
	{"comment.created", true, true, true, false},
	{"status.changed", true, true, false, false},
}
