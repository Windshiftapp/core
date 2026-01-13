package tui

import (
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbletea"
)

// handleKeyPress handles key presses based on the current screen
func (m Model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	// Clear success message on any key press
	m.successMessage = ""

	// Handle picker keys if picker is active
	if m.picker.Active {
		return m.handlePickerKeys(msg)
	}

	// Global quit key
	if msg.String() == "q" && !m.isEditing() {
		return m, tea.Quit
	}

	// Global help key
	if (msg.String() == "h" || msg.String() == "f1") && !m.isEditing() {
		if m.currentScreen == HelpScreen {
			// Return to previous screen
			if m.currentWorkspace != nil {
				m.currentScreen = WorkItemListScreen
			} else {
				m.currentScreen = WorkspaceListScreen
			}
		} else {
			m.currentScreen = HelpScreen
		}
		return m, nil
	}

	switch m.currentScreen {
	case WorkspaceListScreen:
		return m.handleWorkspaceKeys(msg)
	case WorkItemListScreen:
		return m.handleWorkItemKeys(msg)
	case WorkItemDetailScreen:
		return m.handleWorkItemDetailKeys(msg)
	case CreateWorkItemScreen:
		return m.handleCreateWorkItemKeys(msg)
	case CommentsScreen:
		return m.handleCommentsKeys(msg)
	case TimeLoggingScreen:
		return m.handleTimeLoggingKeys(msg)
	case HelpScreen:
		return m.handleHelpKeys(msg)
	}
	
	return m, cmd
}

func (m Model) isEditing() bool {
	switch m.currentScreen {
	case WorkItemDetailScreen:
		return m.editForm.editing
	case CreateWorkItemScreen:
		return m.createForm.editing
	case CommentsScreen:
		return m.commentForm.editing
	case TimeLoggingScreen:
		return m.timeForm.editing
	}
	return false
}

func (m Model) handleWorkspaceKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.selectedWorkspaceIdx > 0 {
			m.selectedWorkspaceIdx--
		} else if len(m.workspaces) > 0 {
			m.selectedWorkspaceIdx = len(m.workspaces) - 1
		}
	case "down", "j":
		if len(m.workspaces) > 0 {
			m.selectedWorkspaceIdx = (m.selectedWorkspaceIdx + 1) % len(m.workspaces)
		}
	case "enter":
		if len(m.workspaces) > 0 && m.selectedWorkspaceIdx < len(m.workspaces) {
			m.currentWorkspace = &m.workspaces[m.selectedWorkspaceIdx]
			m.currentScreen = WorkItemListScreen
			// Load work items, statuses, priorities, and time projects
			return m, tea.Batch(
				m.loadWorkItems(m.currentWorkspace.ID),
				m.loadStatuses(),
				m.loadPriorities(),
				m.loadTimeProjects(),
			)
		}
	case "r":
		m.loading = true
		m.errorMessage = ""
		return m, m.loadWorkspaces()
	}
	return m, nil
}

func (m Model) handleWorkItemKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.selectedItemIdx > 0 {
			m.selectedItemIdx--
		} else if len(m.workItems) > 0 {
			m.selectedItemIdx = len(m.workItems) - 1
		}
	case "down", "j":
		if len(m.workItems) > 0 {
			m.selectedItemIdx = (m.selectedItemIdx + 1) % len(m.workItems)
		}
	case "enter":
		if len(m.workItems) > 0 && m.selectedItemIdx < len(m.workItems) {
			item := m.workItems[m.selectedItemIdx]

			// Initialize the textarea for description
			ta := textarea.New()
			ta.SetValue(item.Description)
			ta.SetWidth(80)
			ta.SetHeight(10)
			ta.ShowLineNumbers = false
			ta.CharLimit = 5000
			ta.Placeholder = "Enter description..."

			m.editForm = WorkItemEditForm{
				title:               item.Title,
				description:         item.Description,
				descriptionTextarea: ta,
				statusID:            item.StatusID,
				statusName:          item.StatusName,
				statusColor:         item.StatusCategoryColor,
				priorityID:          item.PriorityID,
				priorityName:        item.PriorityName,
				priorityColor:       item.PriorityColor,
				titleCursor:         len(item.Title),
			}
			m.currentScreen = WorkItemDetailScreen
		}
	case "l":
		if len(m.workItems) > 0 {
			m.currentScreen = TimeLoggingScreen
			m.timeForm.description = ""
			m.timeForm.duration = ""
			m.timeForm.currentField = 0
			// Initialize project from workspace's time_project_id if set
			if m.currentWorkspace != nil && m.currentWorkspace.TimeProjectID != nil {
				m.timeForm.projectID = m.currentWorkspace.TimeProjectID
				// Find the project name
				for _, proj := range m.timeProjects {
					if int(proj.ID) == *m.currentWorkspace.TimeProjectID {
						m.timeForm.projectName = proj.Name
						break
					}
				}
			} else {
				m.timeForm.projectID = nil
				m.timeForm.projectName = ""
			}
		}
	case "c":
		if len(m.workItems) > 0 && m.selectedItemIdx < len(m.workItems) {
			item := m.workItems[m.selectedItemIdx]
			m.currentScreen = CommentsScreen
			return m, m.loadComments(item.ID)
		}
	case "n":
		if m.currentWorkspace != nil {
			m.createForm = CreateWorkItemForm{
				title:       "",
				description: "",
				priorityID:  nil,
				priorityName: "",
				titleCursor: 0,
			}
			m.currentScreen = CreateWorkItemScreen
		}
	case "r":
		if m.currentWorkspace != nil {
			m.loading = true
			m.errorMessage = ""
			return m, m.loadWorkItems(m.currentWorkspace.ID)
		}
	case "escape", "esc":
		m.currentScreen = WorkspaceListScreen
		m.currentWorkspace = nil
	}
	return m, nil
}

func (m Model) handleWorkItemDetailKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.editForm.editing {
		// Special handling for description field using textarea
		if m.editForm.currentField == 1 {
			// Check for ESC to stop editing (stay on current field)
			if msg.Type == tea.KeyEscape {
				// Save the content from textarea
				m.editForm.description = m.editForm.descriptionTextarea.Value()
				m.editForm.editing = false
				return m, nil
			}
			// Check for Tab or Ctrl+Enter to exit the field and move to next
			if msg.Type == tea.KeyTab || msg.String() == "ctrl+enter" {
				// Save the content from textarea
				m.editForm.description = m.editForm.descriptionTextarea.Value()
				m.editForm.editing = false
				if m.editForm.currentField < 3 {
					m.editForm.currentField++
				}
				return m, nil
			}
			// Pass all other key messages to the textarea
			var cmd tea.Cmd
			m.editForm.descriptionTextarea, cmd = m.editForm.descriptionTextarea.Update(msg)
			m.editForm.description = m.editForm.descriptionTextarea.Value()
			return m, cmd
		}

		// Handle title field (index 0) with text editing
		if m.editForm.currentField == 0 {
			switch msg.Type {
			case tea.KeyEscape:
				m.editForm.editing = false
				return m, nil
			case tea.KeyEnter, tea.KeyTab:
				m.editForm.editing = false
				m.editForm.currentField++
				return m, nil
			case tea.KeyBackspace, tea.KeyDelete:
				(&m).editFormBackspace()
				return m, nil
			case tea.KeyLeft:
				(&m).editFormMoveCursor(-1)
				return m, nil
			case tea.KeyRight:
				(&m).editFormMoveCursor(1)
				return m, nil
			}

			// Handle Ctrl+Enter separately
			if msg.String() == "ctrl+enter" {
				m.editForm.editing = false
				m.editForm.currentField++
				return m, nil
			}

			// Handle regular character input
			if len(msg.Runes) > 0 {
				(&m).editFormAddChar(msg.Runes[0])
			}
		}
	} else {
		switch msg.String() {
		case "up", "k":
			if m.editForm.currentField > 0 {
				m.editForm.currentField--
			}
		case "down", "j":
			if m.editForm.currentField < 3 {
				m.editForm.currentField++
			}
		case "enter":
			switch m.editForm.currentField {
			case 0:
				// Title - text editing
				m.editForm.editing = true
				m.editForm.titleCursor = len(m.editForm.title)
			case 1:
				// Description - textarea editing
				m.editForm.editing = true
				m.editForm.descriptionTextarea.Focus()
			case 2:
				// Status - open picker
				m.picker = PickerState{
					Active:   true,
					Type:     PickerStatus,
					Selected: m.findStatusIndex(m.editForm.statusID),
				}
			case 3:
				// Priority - open picker
				m.picker = PickerState{
					Active:   true,
					Type:     PickerPriority,
					Selected: m.findPriorityIndex(m.editForm.priorityID),
				}
			}
		case "s":
			if len(m.workItems) > 0 && m.selectedItemIdx < len(m.workItems) {
				item := m.workItems[m.selectedItemIdx]
				return m, m.updateWorkItem(item.ID, m.editForm.title, m.editForm.description, m.editForm.statusID, m.editForm.priorityID)
			}
		case "l":
			m.currentScreen = TimeLoggingScreen
			m.timeForm.description = ""
			m.timeForm.duration = ""
			m.timeForm.currentField = 0
			// Initialize project from workspace's time_project_id if set
			if m.currentWorkspace != nil && m.currentWorkspace.TimeProjectID != nil {
				m.timeForm.projectID = m.currentWorkspace.TimeProjectID
				// Find the project name
				for _, proj := range m.timeProjects {
					if int(proj.ID) == *m.currentWorkspace.TimeProjectID {
						m.timeForm.projectName = proj.Name
						break
					}
				}
			} else {
				m.timeForm.projectID = nil
				m.timeForm.projectName = ""
			}
		case "c":
			if len(m.workItems) > 0 && m.selectedItemIdx < len(m.workItems) {
				item := m.workItems[m.selectedItemIdx]
				m.currentScreen = CommentsScreen
				return m, m.loadComments(item.ID)
			}
		case "escape", "esc", "q":
			m.currentScreen = WorkItemListScreen
		}
	}
	return m, nil
}

func (m Model) handleCreateWorkItemKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.createForm.editing {
		// Handle title and description with text editing
		switch msg.Type {
		case tea.KeyEscape:
			m.createForm.editing = false
			return m, nil
		case tea.KeyEnter, tea.KeyTab:
			m.createForm.editing = false
			if m.createForm.currentField < 2 {
				m.createForm.currentField++
			}
			return m, nil
		case tea.KeyBackspace, tea.KeyDelete:
			(&m).createFormBackspace()
			return m, nil
		case tea.KeyLeft:
			(&m).createFormMoveCursor(-1)
			return m, nil
		case tea.KeyRight:
			(&m).createFormMoveCursor(1)
			return m, nil
		}

		// Handle regular character input
		if len(msg.Runes) > 0 {
			(&m).createFormAddChar(msg.Runes[0])
		}
	} else {
		switch msg.String() {
		case "up", "k":
			if m.createForm.currentField > 0 {
				m.createForm.currentField--
			}
		case "down", "j":
			if m.createForm.currentField < 2 {
				m.createForm.currentField++
			}
		case "enter":
			switch m.createForm.currentField {
			case 0, 1:
				// Title, Description - text editing
				m.createForm.editing = true
				if m.createForm.currentField == 0 {
					m.createForm.titleCursor = len(m.createForm.title)
				}
			case 2:
				// Priority - open picker
				m.picker = PickerState{
					Active:   true,
					Type:     PickerPriority,
					Selected: m.findPriorityIndex(m.createForm.priorityID),
				}
			}
		case "s":
			if m.currentWorkspace != nil {
				return m, m.createWorkItem(m.currentWorkspace.ID, m.createForm.title, m.createForm.description, m.createForm.priorityID)
			}
		case "escape", "esc":
			m.currentScreen = WorkItemListScreen
		}
	}
	return m, nil
}

func (m Model) handleCommentsKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.commentForm.editing {
		switch msg.String() {
		case "escape", "esc":
			m.commentForm.editing = false
		case "enter":
			if m.commentForm.content != "" && len(m.workItems) > 0 && m.selectedItemIdx < len(m.workItems) {
				item := m.workItems[m.selectedItemIdx]
				return m, m.createComment(item.ID, m.commentForm.content)
			}
			m.commentForm.editing = false
		case "backspace":
			if len(m.commentForm.content) > 0 {
				m.commentForm.content = m.commentForm.content[:len(m.commentForm.content)-1]
			}
		default:
			if len(msg.Runes) == 1 {
				m.commentForm.content += string(msg.Runes[0])
			}
		}
	} else {
		switch msg.String() {
		case "n":
			m.commentForm.content = ""
			m.commentForm.editing = true
		case "r":
			if len(m.workItems) > 0 && m.selectedItemIdx < len(m.workItems) {
				item := m.workItems[m.selectedItemIdx]
				return m, m.loadComments(item.ID)
			}
		case "escape", "esc":
			m.currentScreen = WorkItemListScreen
		}
	}
	return m, nil
}

func (m Model) handleTimeLoggingKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.timeForm.editing {
		switch msg.String() {
		case "escape", "esc":
			m.timeForm.editing = false
		case "enter":
			m.timeForm.editing = false
			if m.timeForm.currentField < 3 {
				m.timeForm.currentField++
				m.timeForm.editing = true
			} else if m.timeForm.currentField == 3 {
				// Move to project field but don't start editing (it uses a picker)
				m.timeForm.currentField++
			}
		case "backspace":
			(&m).timeFormBackspace()
		default:
			if len(msg.Runes) == 1 {
				(&m).timeFormAddChar(msg.Runes[0])
			}
		}
	} else {
		switch msg.String() {
		case "up", "k":
			if m.timeForm.currentField > 0 {
				m.timeForm.currentField--
			}
		case "down", "j":
			if m.timeForm.currentField < 4 {
				m.timeForm.currentField++
			}
		case "enter":
			if m.timeForm.currentField == 4 {
				// Project field - open picker
				m.picker = PickerState{
					Active:   true,
					Type:     PickerProject,
					Selected: m.findTimeProjectIndex(m.timeForm.projectID),
				}
			} else {
				// Text fields - start editing
				m.timeForm.editing = true
			}
		case "s":
			if len(m.workItems) > 0 && m.selectedItemIdx < len(m.workItems) {
				// Validate project is selected
				if m.timeForm.projectID == nil {
					m.errorMessage = "Please select a project"
					return m, nil
				}
				item := m.workItems[m.selectedItemIdx]
				return m, m.createTimeLog(item.ID, *m.timeForm.projectID, m.timeForm.description, m.timeForm.duration, m.timeForm.date, m.timeForm.startTime)
			}
		case "escape", "esc":
			m.currentScreen = WorkItemListScreen
		}
	}
	return m, nil
}

func (m Model) handleHelpKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "escape", "h", "f1":
		if m.currentWorkspace != nil {
			m.currentScreen = WorkItemListScreen
		} else {
			m.currentScreen = WorkspaceListScreen
		}
	}
	return m, nil
}

// Helper methods for form editing
func (m *Model) editFormBackspace() {
	// Only title field (0) uses text editing in edit form
	if m.editForm.currentField == 0 {
		if m.editForm.titleCursor > 0 {
			m.editForm.title = m.editForm.title[:m.editForm.titleCursor-1] + m.editForm.title[m.editForm.titleCursor:]
			m.editForm.titleCursor--
		}
	}
}

func (m *Model) editFormAddChar(r rune) {
	// Only title field (0) uses text editing in edit form
	if m.editForm.currentField == 0 {
		m.editForm.title = m.editForm.title[:m.editForm.titleCursor] + string(r) + m.editForm.title[m.editForm.titleCursor:]
		m.editForm.titleCursor++
	}
}

func (m *Model) editFormMoveCursor(delta int) {
	// Only title field (0) uses text editing in edit form
	if m.editForm.currentField == 0 {
		newPos := m.editForm.titleCursor + delta
		if newPos >= 0 && newPos <= len(m.editForm.title) {
			m.editForm.titleCursor = newPos
		}
	}
}

func (m *Model) createFormBackspace() {
	switch m.createForm.currentField {
	case 0:
		if m.createForm.titleCursor > 0 {
			m.createForm.title = m.createForm.title[:m.createForm.titleCursor-1] + m.createForm.title[m.createForm.titleCursor:]
			m.createForm.titleCursor--
		}
	case 1:
		if len(m.createForm.description) > 0 {
			m.createForm.description = m.createForm.description[:len(m.createForm.description)-1]
		}
	}
}

func (m *Model) createFormAddChar(r rune) {
	switch m.createForm.currentField {
	case 0:
		m.createForm.title = m.createForm.title[:m.createForm.titleCursor] + string(r) + m.createForm.title[m.createForm.titleCursor:]
		m.createForm.titleCursor++
	case 1:
		m.createForm.description += string(r)
	}
}

func (m *Model) createFormMoveCursor(delta int) {
	if m.createForm.currentField == 0 {
		newPos := m.createForm.titleCursor + delta
		if newPos >= 0 && newPos <= len(m.createForm.title) {
			m.createForm.titleCursor = newPos
		}
	}
}

func (m *Model) timeFormBackspace() {
	switch m.timeForm.currentField {
	case 0:
		if len(m.timeForm.description) > 0 {
			m.timeForm.description = m.timeForm.description[:len(m.timeForm.description)-1]
		}
	case 1:
		if len(m.timeForm.duration) > 0 {
			m.timeForm.duration = m.timeForm.duration[:len(m.timeForm.duration)-1]
		}
	case 2:
		if len(m.timeForm.date) > 0 {
			m.timeForm.date = m.timeForm.date[:len(m.timeForm.date)-1]
		}
	case 3:
		if len(m.timeForm.startTime) > 0 {
			m.timeForm.startTime = m.timeForm.startTime[:len(m.timeForm.startTime)-1]
		}
	}
}

func (m *Model) timeFormAddChar(r rune) {
	switch m.timeForm.currentField {
	case 0:
		m.timeForm.description += string(r)
	case 1:
		m.timeForm.duration += string(r)
	case 2:
		m.timeForm.date += string(r)
	case 3:
		m.timeForm.startTime += string(r)
	}
}

// handlePickerKeys handles key presses when the picker is active
func (m Model) handlePickerKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.picker.Selected > 0 {
			m.picker.Selected--
		}
	case "down", "j":
		maxIdx := 0
		switch m.picker.Type {
		case PickerStatus:
			maxIdx = len(m.statuses) - 1
		case PickerPriority:
			maxIdx = len(m.priorities) - 1
		case PickerProject:
			maxIdx = len(m.timeProjects) - 1
		}
		if m.picker.Selected < maxIdx {
			m.picker.Selected++
		}
	case "enter":
		// Apply the selection
		switch m.picker.Type {
		case PickerStatus:
			if m.picker.Selected >= 0 && m.picker.Selected < len(m.statuses) {
				status := m.statuses[m.picker.Selected]
				if m.currentScreen == WorkItemDetailScreen {
					m.editForm.statusID = &status.ID
					m.editForm.statusName = status.Name
					m.editForm.statusColor = status.CategoryColor
				}
			}
		case PickerPriority:
			if m.picker.Selected >= 0 && m.picker.Selected < len(m.priorities) {
				priority := m.priorities[m.picker.Selected]
				if m.currentScreen == WorkItemDetailScreen {
					m.editForm.priorityID = &priority.ID
					m.editForm.priorityName = priority.Name
					m.editForm.priorityColor = priority.Color
				} else if m.currentScreen == CreateWorkItemScreen {
					m.createForm.priorityID = &priority.ID
					m.createForm.priorityName = priority.Name
					m.createForm.priorityColor = priority.Color
				}
			}
		case PickerProject:
			if m.picker.Selected >= 0 && m.picker.Selected < len(m.timeProjects) {
				project := m.timeProjects[m.picker.Selected]
				if m.currentScreen == TimeLoggingScreen {
					projectID := int(project.ID)
					m.timeForm.projectID = &projectID
					m.timeForm.projectName = project.Name
				}
			}
		}
		// Close the picker
		m.picker = PickerState{}
	case "escape", "esc":
		// Cancel - close picker without changes
		m.picker = PickerState{}
	}
	return m, nil
}

// findStatusIndex finds the index of a status ID in the statuses slice
func (m Model) findStatusIndex(statusID *int) int {
	if statusID == nil {
		return 0
	}
	for i, status := range m.statuses {
		if status.ID == *statusID {
			return i
		}
	}
	return 0
}

// findPriorityIndex finds the index of a priority ID in the priorities slice
func (m Model) findPriorityIndex(priorityID *int) int {
	if priorityID == nil {
		return 0
	}
	for i, priority := range m.priorities {
		if priority.ID == *priorityID {
			return i
		}
	}
	return 0
}

// findTimeProjectIndex finds the index of a project ID in the timeProjects slice
func (m Model) findTimeProjectIndex(projectID *int) int {
	if projectID == nil {
		return 0
	}
	for i, project := range m.timeProjects {
		if int(project.ID) == *projectID {
			return i
		}
	}
	return 0
}