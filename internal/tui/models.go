package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// UserInfo contains authenticated SSH user information
type UserInfo struct {
	UserID         int
	CredentialID   int
	CredentialName string
	RemoteAddr     string
	Email          string
	Username       string
	FirstName      string
	LastName       string
}

// AppScreen represents different screens in the TUI
type AppScreen int

const (
	WorkspaceListScreen AppScreen = iota
	WorkItemListScreen
	WorkItemDetailScreen
	CreateWorkItemScreen
	CommentsScreen
	TimeLoggingScreen
	HelpScreen
	CommandPaletteScreen
)

// PickerType represents the type of picker being shown
type PickerType int

const (
	PickerNone PickerType = iota
	PickerStatus
	PickerPriority
	PickerProject
)

// PickerState holds the state for the popup picker
type PickerState struct {
	Active   bool
	Type     PickerType
	Selected int
}

// Model represents the main application model
type Model struct {
	// State
	currentScreen AppScreen
	workspaces    []Workspace
	workItems     []WorkItem
	comments      []Comment
	timeProjects  []TimeProject
	statuses      []Status   // Cached list of available statuses
	priorities    []Priority // Cached list of available priorities

	// Current selections
	currentWorkspace     *Workspace
	selectedWorkspaceIdx int
	selectedItemIdx      int

	// Forms
	editForm    WorkItemEditForm
	commentForm CommentForm
	createForm  CreateWorkItemForm
	timeForm    TimeLogForm

	// Picker state
	picker PickerState

	// UI state
	loading        bool
	errorMessage   string
	successMessage string

	// API client
	apiClient *APIClient

	// User information
	userInfo *UserInfo

	// Authentication
	sessionToken string

	// Window size
	width  int
	height int

	// Styles
	styles Styles
}

// WorkItemEditForm for editing work items
type WorkItemEditForm struct {
	title               string
	description         string         // Keep this for storing the raw value
	descriptionTextarea textarea.Model // The textarea component for editing
	statusID            *int           // ID-based status
	statusName          string         // For display
	statusColor         string         // Category color for display
	priorityID          *int           // ID-based priority
	priorityName        string         // For display
	priorityColor       string         // Priority color for display
	currentField        int
	editing             bool
	// Cursor positions for single-line fields
	titleCursor int
}

// CommentForm for creating comments
type CommentForm struct {
	content string
	editing bool
}

// CreateWorkItemForm for creating new work items
type CreateWorkItemForm struct {
	title         string
	description   string
	priorityID    *int   // ID-based priority
	priorityName  string // For display
	priorityColor string // Priority color for display
	currentField  int
	editing       bool
	// Cursor positions for single-line fields
	titleCursor int
}

// TimeLogForm for logging time
type TimeLogForm struct {
	description  string
	duration     string
	date         string
	startTime    string
	projectID    *int   // Selected project ID
	projectName  string // For display
	currentField int
	editing      bool
}

// Styles contains lipgloss styles for the TUI
type Styles struct {
	Title        lipgloss.Style
	Subtitle     lipgloss.Style
	SelectedItem lipgloss.Style
	NormalItem   lipgloss.Style
	ErrorMessage lipgloss.Style
	HelpText     lipgloss.Style
	Border       lipgloss.Style
	EditingField lipgloss.Style
	StatusBar    lipgloss.Style
}

// NewModel creates a new model instance
func NewModel(apiURL string) Model {
	return NewModelWithUser(apiURL, nil)
}

// NewModelWithUser creates a new model instance with user information
func NewModelWithUser(apiURL string, userInfo *UserInfo) Model {
	return NewModelWithUserAndToken(apiURL, userInfo, "")
}

// NewModelWithUserAndToken creates a new model instance with user information and session token
func NewModelWithUserAndToken(apiURL string, userInfo *UserInfo, sessionToken string) Model {
	styles := Styles{
		Title: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#64FFAA")).
			Bold(true).
			Padding(0, 1),
		Subtitle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#64B4FF")).
			Bold(true),
		SelectedItem: lipgloss.NewStyle().
			Background(lipgloss.Color("#326496")).
			Foreground(lipgloss.Color("#FFFFFF")).
			Bold(true).
			Padding(0, 1),
		NormalItem: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#DCDDDE")),
		ErrorMessage: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF6464")).
			Bold(true),
		HelpText: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#B4B4B4")),
		Border: lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#64B4FF")),
		EditingField: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFF64")).
			Bold(true),
		StatusBar: lipgloss.NewStyle().
			Background(lipgloss.Color("#1E1E1E")).
			Foreground(lipgloss.Color("#B4B4B4")).
			Padding(0, 1),
	}

	// Create API client with session token if provided
	apiClient := NewAPIClient(apiURL)
	if sessionToken != "" {
		apiClient.SetSessionToken(sessionToken)
	}

	return Model{
		currentScreen:        WorkspaceListScreen,
		workspaces:           []Workspace{},
		workItems:            []WorkItem{},
		comments:             []Comment{},
		timeProjects:         []TimeProject{},
		selectedWorkspaceIdx: 0,
		selectedItemIdx:      0,
		apiClient:            apiClient,
		userInfo:             userInfo,
		sessionToken:         sessionToken,
		styles:               styles,
		timeForm: TimeLogForm{
			date:      time.Now().Format("2006-01-02"),
			startTime: time.Now().Format("15:04"),
		},
	}
}

// Init implements tea.Model
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.loadWorkspaces(),
		tea.EnterAltScreen,
	)
}

// Update implements tea.Model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKeyPress(msg)

	case workspacesLoadedMsg:
		m.workspaces = msg.workspaces
		m.loading = false
		if len(m.workspaces) > 0 {
			m.selectedWorkspaceIdx = 0
		}
		return m, nil

	case workItemsLoadedMsg:
		m.workItems = msg.items
		m.loading = false
		if len(m.workItems) > 0 {
			m.selectedItemIdx = 0
		}
		return m, nil

	case commentsLoadedMsg:
		m.comments = msg.comments
		m.loading = false
		return m, nil

	case commentCreatedMsg:
		// Reset comment form and reload comments
		m.commentForm.content = ""
		m.commentForm.editing = false
		m.successMessage = "Comment added!"
		m.errorMessage = ""
		if len(m.workItems) > 0 && m.selectedItemIdx < len(m.workItems) {
			item := m.workItems[m.selectedItemIdx]
			return m, m.loadComments(item.ID)
		}
		return m, nil

	case workItemCreatedMsg:
		// Reset create form and reload work items
		m.createForm.title = ""
		m.createForm.description = ""
		m.createForm.priorityID = nil
		m.createForm.priorityName = ""
		m.createForm.editing = false
		m.currentScreen = WorkItemListScreen
		m.successMessage = "Work item created!"
		m.errorMessage = ""
		if m.currentWorkspace != nil {
			return m, m.loadWorkItems(m.currentWorkspace.ID)
		}
		return m, nil

	case statusesLoadedMsg:
		m.statuses = msg.statuses
		return m, nil

	case prioritiesLoadedMsg:
		m.priorities = msg.priorities
		return m, nil

	case timeProjectsLoadedMsg:
		m.timeProjects = msg.projects
		return m, nil

	case workItemUpdatedMsg:
		// Show success message and reload work items
		m.successMessage = "Work item saved successfully!"
		m.errorMessage = ""
		if m.currentWorkspace != nil {
			return m, m.loadWorkItems(m.currentWorkspace.ID)
		}
		return m, nil

	case timeLogCreatedMsg:
		// Reset time form and go back to work item detail
		m.timeForm.description = ""
		m.timeForm.duration = ""
		m.currentScreen = WorkItemDetailScreen
		m.successMessage = "Time logged!"
		m.errorMessage = ""
		return m, nil

	case errorMsg:
		m.errorMessage = msg.error
		m.loading = false
		return m, nil
	}

	return m, cmd
}

// View implements tea.Model
func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	var content string

	switch m.currentScreen {
	case WorkspaceListScreen:
		content = m.renderWorkspaceList()
	case WorkItemListScreen:
		content = m.renderWorkItemList()
	case WorkItemDetailScreen:
		content = m.renderWorkItemDetail()
	case CreateWorkItemScreen:
		content = m.renderCreateWorkItem()
	case CommentsScreen:
		content = m.renderComments()
	case TimeLoggingScreen:
		content = m.renderTimeLogging()
	case HelpScreen:
		content = m.renderHelp()
	default:
		content = "Unknown screen"
	}

	// Add status bar
	statusBar := m.renderStatusBar()

	// Calculate available height for content (reserve space for status bar)
	contentHeight := m.height - 1 // Reserve 1 line for status bar

	// Ensure content fills available space
	contentLines := strings.Split(content, "\n")

	if len(contentLines) > contentHeight {
		// Truncate if too long
		content = strings.Join(contentLines[:contentHeight], "\n")
	} else if len(contentLines) < contentHeight {
		// Pad with empty lines to fill the space
		padding := contentHeight - len(contentLines)
		for i := 0; i < padding; i++ {
			content += "\n"
		}
	}

	// Style the main content area to use full width and height
	mainContent := lipgloss.NewStyle().
		Width(m.width).
		Height(contentHeight).
		Render(content)

	result := mainContent + "\n" + statusBar

	// Overlay picker if active
	if m.picker.Active {
		result = m.overlayPicker(result)
	}

	return result
}

// Helper methods for rendering different screens
func (m Model) renderWorkspaceList() string {
	title := m.styles.Title.Render("🏢 Windshift TUI - Workspaces")

	if m.loading {
		return title + "\n\nLoading workspaces..."
	}

	if len(m.workspaces) == 0 {
		return title + "\n\nNo workspaces found."
	}

	var items []string
	for i, workspace := range m.workspaces {
		line := fmt.Sprintf("%s - %s", workspace.Key, workspace.Name)
		if i == m.selectedWorkspaceIdx {
			line = m.styles.SelectedItem.Render("▶ " + line)
		} else {
			line = m.styles.NormalItem.Render("  " + line)
		}
		items = append(items, line)
	}

	content := title + "\n\n" + strings.Join(items, "\n")

	if m.errorMessage != "" {
		content += "\n\n" + m.styles.ErrorMessage.Render("Error: "+m.errorMessage)
	}

	return content
}

func (m Model) renderWorkItemList() string {
	workspaceName := "Unknown"
	if m.currentWorkspace != nil {
		workspaceName = m.currentWorkspace.Name
	}

	title := m.styles.Title.Render(fmt.Sprintf("📋 Work Items - %s", workspaceName))

	if m.loading {
		return title + "\n\nLoading work items..."
	}

	if len(m.workItems) == 0 {
		return title + "\n\nNo work items found."
	}

	// Calculate column widths based on available width
	availableWidth := m.width - 4                             // Account for padding
	keyWidth := 12                                            // For item keys like "FIRST-123"
	statusWidth := 15                                         // For status like "[in_progress]"
	titleWidth := availableWidth - keyWidth - statusWidth - 6 // Account for spacing and hierarchy indent
	if titleWidth < 20 {
		titleWidth = 20
	}

	// Create column header
	header := m.styles.Subtitle.Render(
		fmt.Sprintf("%-*s %-*s %s",
			keyWidth, "KEY",
			titleWidth, "TITLE",
			"STATUS"))

	items := []string{header, strings.Repeat("─", availableWidth)}

	for i, item := range m.workItems {
		// Add hierarchy indentation
		hierarchyIndent := strings.Repeat("  ", item.GetLevel())

		// Build item key
		workspaceKey := "WORK"
		if m.currentWorkspace != nil {
			workspaceKey = m.currentWorkspace.Key
		}
		itemKey := fmt.Sprintf("%s-%d", workspaceKey, item.ID)

		// Truncate title if too long
		title = item.Title
		if len(title) > titleWidth-len(hierarchyIndent)-3 {
			title = title[:titleWidth-len(hierarchyIndent)-6] + "..."
		}

		// Style status with color coding - prefer ID-based status name and color
		var status string
		if item.StatusName != "" {
			status = m.formatStatusWithColor(item.StatusName, item.StatusCategoryColor)
		} else {
			status = m.formatStatus(item.Status)
		}

		// Format the row
		row := fmt.Sprintf("%-*s %s%-*s %s",
			keyWidth, itemKey,
			hierarchyIndent,
			titleWidth-len(hierarchyIndent), title,
			status)

		// Apply selection styling
		if i == m.selectedItemIdx {
			row = m.styles.SelectedItem.Render("▶ " + row)
		} else {
			row = m.styles.NormalItem.Render("  " + row)
		}

		items = append(items, row)
	}

	content := title + "\n\n" + strings.Join(items, "\n")

	if m.errorMessage != "" {
		content += "\n\n" + m.styles.ErrorMessage.Render("Error: "+m.errorMessage)
	}

	return content
}

func (m Model) renderWorkItemDetail() string {
	if m.selectedItemIdx >= len(m.workItems) {
		return "No work item selected"
	}

	item := m.workItems[m.selectedItemIdx]
	workspaceKey := "WORK"
	if m.currentWorkspace != nil {
		workspaceKey = m.currentWorkspace.Key
	}
	itemKey := fmt.Sprintf("%s-%d", workspaceKey, item.ID)

	title := m.styles.Title.Render(fmt.Sprintf("✏️ Edit Work Item - %s", itemKey))

	var content []string
	content = append(content, title, "")

	// Title field (index 0)
	titleLabel := m.styles.Subtitle.Render("Title:")
	titleValue := m.editForm.title
	if m.editForm.currentField == 0 {
		if m.editForm.editing {
			cursorPos := m.editForm.titleCursor
			if cursorPos >= 0 && cursorPos <= len(titleValue) {
				titleValue = titleValue[:cursorPos] + "█" + titleValue[cursorPos:]
			} else {
				titleValue += "█"
			}
			titleValue = m.styles.EditingField.Render(titleValue)
		} else {
			titleValue = m.styles.SelectedItem.Render(titleValue)
		}
	}
	content = append(content, titleLabel, titleValue, "")

	// Description field (index 1)
	descLabel := m.styles.Subtitle.Render("Description:")
	if m.editForm.currentField == 1 && m.editForm.editing {
		content = append(content, descLabel, m.editForm.descriptionTextarea.View(), "")
	} else {
		descValue := m.editForm.description
		if m.editForm.currentField == 1 {
			descValue = m.styles.SelectedItem.Render(descValue)
		}
		content = append(content, descLabel, descValue, "")
	}

	// Status field (index 2) - uses picker
	statusLabel := m.styles.Subtitle.Render("Status:")
	if m.editForm.currentField == 2 {
		// Show with color badge
		statusDisplay := m.formatStatusWithColor(m.editForm.statusName, m.editForm.statusColor)
		content = append(content, statusLabel, m.styles.SelectedItem.Render(statusDisplay+" [Enter to change]"), "")
	} else {
		statusDisplay := m.formatStatusWithColor(m.editForm.statusName, m.editForm.statusColor)
		content = append(content, statusLabel, statusDisplay, "")
	}

	// Priority field (index 3) - uses picker
	priorityLabel := m.styles.Subtitle.Render("Priority:")
	if m.editForm.currentField == 3 {
		priorityDisplay := m.formatPriorityWithColor(m.editForm.priorityName, m.editForm.priorityColor)
		content = append(content, priorityLabel, m.styles.SelectedItem.Render(priorityDisplay+" [Enter to change]"), "")
	} else {
		priorityDisplay := m.formatPriorityWithColor(m.editForm.priorityName, m.editForm.priorityColor)
		content = append(content, priorityLabel, priorityDisplay, "")
	}

	if m.editForm.editing {
		if m.editForm.currentField == 1 {
			content = append(content, m.styles.HelpText.Render("ESC: Stop editing | Tab/Ctrl+Enter: Next field | Enter: New line | Type to input"))
		} else {
			content = append(content, m.styles.HelpText.Render("ESC: Stop editing | Enter: Next field | Type to input"))
		}
	} else {
		content = append(content, m.styles.HelpText.Render("Enter: Edit/Select | ↑↓/jk: Navigate | S: Save | L: Log time | C: Comments | ESC/Q: Back"))
	}

	return strings.Join(content, "\n")
}

func (m Model) renderCreateWorkItem() string {
	workspaceName := "Unknown"
	if m.currentWorkspace != nil {
		workspaceName = m.currentWorkspace.Name
	}

	title := m.styles.Title.Render(fmt.Sprintf("➕ Create Work Item - %s", workspaceName))

	var content []string
	content = append(content, title, "")

	// Title field (index 0)
	titleLabel := m.styles.Subtitle.Render("Title:")
	titleValue := m.createForm.title
	if m.createForm.currentField == 0 {
		if m.createForm.editing {
			cursorPos := m.createForm.titleCursor
			if cursorPos >= 0 && cursorPos <= len(titleValue) {
				titleValue = titleValue[:cursorPos] + "█" + titleValue[cursorPos:]
			} else {
				titleValue += "█"
			}
			titleValue = m.styles.EditingField.Render(titleValue)
		} else {
			titleValue = m.styles.SelectedItem.Render(titleValue)
		}
	}
	content = append(content, titleLabel, titleValue, "")

	// Description field (index 1)
	descLabel := m.styles.Subtitle.Render("Description:")
	descValue := m.createForm.description
	if m.createForm.currentField == 1 {
		if m.createForm.editing {
			descValue = m.styles.EditingField.Render(descValue + "█")
		} else {
			descValue = m.styles.SelectedItem.Render(descValue)
		}
	}
	content = append(content, descLabel, descValue, "")

	// Priority field (index 2) - uses picker
	priorityLabel := m.styles.Subtitle.Render("Priority:")
	if m.createForm.currentField == 2 {
		priorityDisplay := m.formatPriorityWithColor(m.createForm.priorityName, m.createForm.priorityColor)
		content = append(content, priorityLabel, m.styles.SelectedItem.Render(priorityDisplay+" [Enter to select]"), "")
	} else {
		priorityDisplay := m.formatPriorityWithColor(m.createForm.priorityName, m.createForm.priorityColor)
		content = append(content, priorityLabel, priorityDisplay, "")
	}

	if m.createForm.editing {
		content = append(content, m.styles.HelpText.Render("ESC: Stop editing | Enter: Next field | Type to input"))
	} else {
		content = append(content, m.styles.HelpText.Render("Enter: Edit/Select | ↑↓/jk: Navigate | S: Create work item | ESC: Cancel"))
	}

	return strings.Join(content, "\n")
}

func (m Model) renderComments() string {
	if m.selectedItemIdx >= len(m.workItems) {
		return "No work item selected"
	}

	item := m.workItems[m.selectedItemIdx]
	title := m.styles.Title.Render(fmt.Sprintf("💬 Comments - %s", item.Title))

	var content []string
	content = append(content, title, "")

	if len(m.comments) == 0 {
		content = append(content, "No comments yet. Press 'n' to add a comment.")
	} else {
		for _, comment := range m.comments {
			authorName := "Unknown"
			if comment.AuthorName != nil {
				authorName = *comment.AuthorName
			}

			content = append(content, m.styles.Subtitle.Render(fmt.Sprintf("👤 %s - %s", authorName, comment.CreatedAt)), comment.Content, "")
		}
	}

	// New comment input
	content = append(content, m.styles.Subtitle.Render("New Comment:"))
	if m.commentForm.editing {
		content = append(content, m.styles.EditingField.Render(m.commentForm.content+"█"), "", m.styles.HelpText.Render("Type your comment | Enter: Post comment | ESC: Cancel"))
	} else {
		content = append(content, m.commentForm.content, "", m.styles.HelpText.Render("N: New comment | R: Refresh | ESC: Back to items"))
	}

	return strings.Join(content, "\n")
}

func (m Model) renderTimeLogging() string {
	if m.selectedItemIdx >= len(m.workItems) {
		return "No work item selected"
	}

	item := m.workItems[m.selectedItemIdx]
	title := m.styles.Title.Render(fmt.Sprintf("⏱️ Log Time - %s", item.Title))

	fields := []struct {
		label string
		value string
		idx   int
	}{
		{"Description", m.timeForm.description, 0},
		{"Duration (e.g., 2h, 30m, 1h30m)", m.timeForm.duration, 1},
		{"Date (YYYY-MM-DD)", m.timeForm.date, 2},
		{"Start Time (HH:MM)", m.timeForm.startTime, 3},
	}

	content := []string{title, ""}

	for _, field := range fields {
		label := m.styles.Subtitle.Render(field.label + ":")
		value := field.value

		if field.idx == m.timeForm.currentField {
			if m.timeForm.editing {
				value = m.styles.EditingField.Render(value + "█") // Cursor
			} else {
				value = m.styles.SelectedItem.Render(value)
			}
		}

		content = append(content, label, value, "")
	}

	// Project field (index 4) - uses picker
	projectLabel := m.styles.Subtitle.Render("Project:")
	projectValue := m.timeForm.projectName
	if projectValue == "" {
		projectValue = "(select project)"
	}
	if m.timeForm.currentField == 4 {
		content = append(content, projectLabel, m.styles.SelectedItem.Render(projectValue+" [Enter to change]"), "")
	} else {
		content = append(content, projectLabel, projectValue, "")
	}

	if m.timeForm.editing {
		content = append(content, m.styles.HelpText.Render("ESC: Stop editing | Enter: Next field | Type to input"))
	} else {
		content = append(content, m.styles.HelpText.Render("Enter: Edit/Select | ↑↓/jk: Navigate | S: Submit | ESC: Back"))
	}

	return strings.Join(content, "\n")
}

func (m Model) renderHelp() string {
	title := m.styles.Title.Render("📚 Windshift TUI Help")

	helpText := []string{
		title,
		"",
		m.styles.Subtitle.Render("Global Controls:"),
		"  q         - Quit application",
		"  h, F1     - Show/hide help",
		"",
		m.styles.Subtitle.Render("Workspace List:"),
		"  ↑/↓, j/k  - Navigate list",
		"  Enter     - Select workspace",
		"  r         - Refresh list",
		"",
		m.styles.Subtitle.Render("Work Items List:"),
		"  ↑/↓, j/k  - Navigate list",
		"  Enter     - View/edit item details",
		"  l         - Log time for item",
		"  c         - View/add comments",
		"  n         - Create new work item",
		"  ESC       - Back to workspaces",
		"  r         - Refresh list",
		"",
		m.styles.Subtitle.Render("Work Item Detail:"),
		"  ↑/↓, j/k  - Navigate fields",
		"  Enter     - Edit field",
		"  s         - Save changes",
		"  l         - Log time",
		"  c         - View comments",
		"  ESC       - Back to items",
	}

	return strings.Join(helpText, "\n")
}

func (m Model) renderStatusBar() string {
	var status string

	switch {
	case m.loading:
		status = "⏳ Loading..."
	case m.errorMessage != "":
		status = "❌ " + m.errorMessage
	case m.successMessage != "":
		status = "✅ " + m.successMessage
	default:
		switch m.currentScreen {
		case WorkspaceListScreen:
			status = "Select a workspace"
		case WorkItemListScreen:
			status = "Navigate items ↑↓/jk | Enter=edit | L=log time | C=comments | N=create"
		case WorkItemDetailScreen:
			status = "Edit work item details (s=save, l=log time, c=comments)"
		case CreateWorkItemScreen:
			status = "Create new work item (s=save, ESC=cancel)"
		case CommentsScreen:
			status = "View/add comments (n=new comment, r=refresh)"
		case TimeLoggingScreen:
			status = "Fill in time logging details"
		case HelpScreen:
			status = "Help - Press ESC to return"
		}
	}

	leftSide := m.styles.StatusBar.Render("🌀 Windshift TUI")

	// Add user information to the right side of the status bar
	var rightSide string
	if m.userInfo != nil {
		// Create a display name from available user information
		var displayName string

		// Prefer first name + last name, fall back to username, then email
		switch {
		case m.userInfo.FirstName != "" && m.userInfo.LastName != "":
			displayName = fmt.Sprintf("%s %s", m.userInfo.FirstName, m.userInfo.LastName)
		case m.userInfo.Username != "":
			displayName = m.userInfo.Username
		case m.userInfo.Email != "":
			// Use the part before @ in email as display name
			if atIndex := strings.Index(m.userInfo.Email, "@"); atIndex > 0 {
				displayName = m.userInfo.Email[:atIndex]
			} else {
				displayName = m.userInfo.Email
			}
		default:
			// Fall back to credential name (clean it up)
			displayName = m.userInfo.CredentialName
			if strings.Contains(displayName, " Key") {
				if strings.HasSuffix(displayName, " SSH Key") {
					displayName = strings.TrimSuffix(displayName, " SSH Key")
				} else if strings.HasSuffix(displayName, " Key") {
					displayName = strings.TrimSuffix(displayName, " Key")
				}
			}
		}

		userDisplay := fmt.Sprintf("👤 %s | %s", displayName, status)
		rightSide = m.styles.StatusBar.Render(userDisplay)
	} else {
		rightSide = m.styles.StatusBar.Render(status)
	}

	// Calculate padding to fill the width
	padding := m.width - lipgloss.Width(leftSide) - lipgloss.Width(rightSide)
	if padding < 0 {
		padding = 0
	}

	return leftSide + strings.Repeat(" ", padding) + rightSide
}

// formatStatus applies color coding to status values (fallback for legacy text-based status)
func (m Model) formatStatus(status string) string {
	statusStyle := lipgloss.NewStyle().
		Bold(true).
		Padding(0, 1).
		Margin(0, 1)

	switch strings.ToLower(status) {
	case "open", "to_do", "todo":
		return statusStyle.
			Background(lipgloss.Color("#4A90E2")).
			Foreground(lipgloss.Color("#FFFFFF")).
			Render("OPEN")
	case "in_progress", "in progress", "progress":
		return statusStyle.
			Background(lipgloss.Color("#F5A623")).
			Foreground(lipgloss.Color("#FFFFFF")).
			Render("IN PROGRESS")
	case "completed", "done", "closed":
		return statusStyle.
			Background(lipgloss.Color("#7ED321")).
			Foreground(lipgloss.Color("#FFFFFF")).
			Render("DONE")
	case "cancelled", "canceled": //nolint:misspell // intentionally handles both British and American spellings
		return statusStyle.
			Background(lipgloss.Color("#D0021B")).
			Foreground(lipgloss.Color("#FFFFFF")).
			Render("CANCELED")
	default:
		return statusStyle.
			Background(lipgloss.Color("#9B9B9B")).
			Foreground(lipgloss.Color("#FFFFFF")).
			Render(strings.ToUpper(status))
	}
}

// formatStatusWithColor applies color coding using API-provided color
func (m Model) formatStatusWithColor(name, color string) string {
	if name == "" {
		return "(not set)"
	}

	statusStyle := lipgloss.NewStyle().
		Bold(true).
		Padding(0, 1).
		Margin(0, 1)

	bgColor := color
	if bgColor == "" {
		bgColor = "#9B9B9B"
	}

	return statusStyle.
		Background(lipgloss.Color(bgColor)).
		Foreground(lipgloss.Color("#FFFFFF")).
		Render(strings.ToUpper(name))
}

// formatPriorityWithColor applies color coding for priority
func (m Model) formatPriorityWithColor(name, color string) string {
	if name == "" {
		return "(not set)"
	}

	priorityStyle := lipgloss.NewStyle().
		Bold(true).
		Padding(0, 1).
		Margin(0, 1)

	bgColor := color
	if bgColor == "" {
		bgColor = "#6B7280"
	}

	return priorityStyle.
		Background(lipgloss.Color(bgColor)).
		Foreground(lipgloss.Color("#FFFFFF")).
		Render(name)
}

// overlayPicker renders the picker as an overlay on top of the current view
func (m Model) overlayPicker(_ string) string {
	var title string
	var items []string

	switch m.picker.Type {
	case PickerStatus:
		title = "Select Status"
		for i, status := range m.statuses {
			line := m.formatStatusWithColor(status.Name, status.CategoryColor)
			if i == m.picker.Selected {
				line = "▶ " + line
			} else {
				line = "  " + line
			}
			items = append(items, line)
		}
	case PickerPriority:
		title = "Select Priority"
		for i, priority := range m.priorities {
			line := m.formatPriorityWithColor(priority.Name, priority.Color)
			if i == m.picker.Selected {
				line = "▶ " + line
			} else {
				line = "  " + line
			}
			items = append(items, line)
		}
	case PickerProject:
		title = "Select Project"
		for i, project := range m.timeProjects {
			line := project.Name
			if project.CustomerName != nil && *project.CustomerName != "" {
				line += " (" + *project.CustomerName + ")"
			}
			if i == m.picker.Selected {
				line = "▶ " + line
			} else {
				line = "  " + line
			}
			items = append(items, line)
		}
	}

	// Build the picker box
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#64B4FF")).
		Padding(1, 2).
		Background(lipgloss.Color("#1E1E2E"))

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#64FFAA")).
		MarginBottom(1)

	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#B4B4B4")).
		MarginTop(1)

	pickerContent := titleStyle.Render(title) + "\n\n"
	pickerContent += strings.Join(items, "\n")
	pickerContent += "\n\n" + helpStyle.Render("↑↓/jk: Navigate | Enter: Select | ESC: Cancel")

	picker := borderStyle.Render(pickerContent)

	// Use lipgloss.Place to center the picker overlay
	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		picker,
		lipgloss.WithWhitespaceChars(" "),
		lipgloss.WithWhitespaceForeground(lipgloss.Color("#1E1E1E")),
	)
}
