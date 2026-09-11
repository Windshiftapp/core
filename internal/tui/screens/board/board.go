// Package board is the main split-pane view: a status-grouped work-item
// list on the left and a live detail panel on the right that follows the
// selection. It replaces the separate item-list screen.
package board

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"windshift/internal/tui/components/chip"
	"windshift/internal/tui/components/inputs"
	"windshift/internal/tui/core"
	"windshift/internal/tui/data"
	"windshift/internal/tui/dialog"
	"windshift/internal/tui/screens/timelog"
)

const (
	pickerStatusID   = "board.picker.status"
	pickerPriorityID = "board.picker.priority"
	pickerAssignID   = "board.picker.assign"
	formEditID       = "board.form.edit"
	formCreateID     = "board.form.create"
	formCommentID    = "board.form.comment"
)

const (
	defaultSplitRatio = 0.45
	minSplitRatio     = 0.25
	maxSplitRatio     = 0.75
	splitStep         = 0.05
	// narrowWidth is the terminal width below which the board collapses to a
	// single pane.
	narrowWidth = 80
	// commentsDebounce delays the lazy comment fetch while the user is still
	// moving the cursor.
	commentsDebounce = 300 * time.Millisecond
)

type paneFocus int

const (
	focusList paneFocus = iota
	focusDetail
)

// debounceMsg fires after the selection has been resting for a beat; stale
// sequence numbers are dropped.
type debounceMsg struct{ seq int }
type agentRefreshMsg struct {
	itemID    int
	requestID uint64
}

type loadState uint8

const (
	loadStale loadState = iota
	loadLoading
	loadLoaded
	loadFailed
)

// Model is the split-pane board screen.
type Model struct {
	ctx         *core.Ctx
	workspaceID int

	items        []data.WorkItem
	statuses     []data.Status
	priorities   []data.Priority
	timeProjects []data.TimeProject
	users        []data.User

	filter      Filter
	filterInput textinput.Model
	filtering   bool // filter input focused

	comments             map[int][]data.Comment
	commentState         map[int]loadState
	commentRequests      map[int]uint64
	agentRuns            map[int][]data.AgentRun
	runState             map[int]loadState
	runRequests          map[int]uint64
	detailsLoaded        map[int]bool
	detailRequests       map[int]uint64
	detailSeq            int
	pendingItemAction    string
	pendingItemID        int
	pendingTimeLogItemID int

	mutationRequests      map[int]uint64
	itemCreateRequests    map[uint64]bool
	commentCreateRequests map[uint64]int

	list   *listPane
	detail *detailPane

	collapsed  map[string]bool
	splitRatio float64
	focus      paneFocus
	// narrowDetail: in single-pane (narrow) mode, whether the detail pane is
	// the visible one.
	narrowDetail bool

	loading   bool
	truncated bool
	spinner   spinner.Model

	width         int
	height        int
	loadRequestID uint64
}

func New(ctx *core.Ctx) *Model {
	sp := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	sp.Style = lipgloss.NewStyle().Foreground(ctx.Styles.Palette.Primary)
	ratio := defaultSplitRatio
	if r := ctx.Prefs.SplitRatio; r != nil && *r >= minSplitRatio && *r <= maxSplitRatio {
		ratio = *r
	}
	m := &Model{
		ctx:                   ctx,
		comments:              map[int][]data.Comment{},
		commentState:          map[int]loadState{},
		commentRequests:       map[int]uint64{},
		agentRuns:             map[int][]data.AgentRun{},
		runState:              map[int]loadState{},
		runRequests:           map[int]uint64{},
		detailsLoaded:         map[int]bool{},
		detailRequests:        map[int]uint64{},
		mutationRequests:      map[int]uint64{},
		itemCreateRequests:    map[uint64]bool{},
		commentCreateRequests: map[uint64]int{},
		list:                  newListPane(ctx),
		detail:                newDetailPane(ctx),
		collapsed:             map[string]bool{},
		splitRatio:            ratio,
		loading:               true,
		spinner:               sp,
		filterInput:           inputs.New(ctx.Styles, "filter…", 100),
	}
	if ctx.Workspace != nil {
		m.workspaceID = ctx.Workspace.ID
	}
	return m
}

func (m *Model) Init() tea.Cmd {
	if m.workspaceID == 0 {
		return nil
	}
	m.loadRequestID = m.ctx.NextRequestID()
	m.loading = true
	return tea.Batch(
		data.LoadWorkItems(m.ctx.Client, m.workspaceID, m.loadRequestID),
		data.LoadStatuses(m.ctx.Client, m.workspaceID, m.loadRequestID),
		data.LoadPriorities(m.ctx.Client, m.workspaceID, m.loadRequestID),
		data.LoadTimeProjects(m.ctx.Client, m.workspaceID, m.loadRequestID),
		data.LoadAssignableUsers(m.ctx.Client, m.workspaceID, m.loadRequestID),
		m.spinner.Tick,
	)
}

func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
	if m.narrow() {
		m.list.setSize(max(1, width-2), m.listHeight())
		m.detail.setSize(max(1, width-2), max(1, m.paneHeight()-2))
		m.filterInput.SetWidth(max(1, width-6))
		return
	}
	listW, detailW := m.paneWidths()
	m.list.setSize(max(1, listW-2), m.listHeight())
	m.detail.setSize(max(1, detailW-2), max(1, m.paneHeight()-2))
	m.filterInput.SetWidth(max(1, listW-6))
}

// listHeight is the row budget inside the list pane's frame, minus the
// filter line when it's visible.
func (m *Model) listHeight() int {
	h := m.paneHeight() - 2 // pane border
	if m.filterVisible() {
		h--
	}
	if h < 1 {
		h = 1
	}
	return h
}

func (m *Model) filterVisible() bool { return m.filtering || m.filter.Active() }

func (m *Model) narrow() bool { return m.width < narrowWidth }

func (m *Model) paneHeight() int {
	h := m.height
	if m.truncated {
		h-- // one line for the truncation notice
	}
	if h < 1 {
		h = 1
	}
	return h
}

func (m *Model) paneWidths() (listW, detailW int) {
	listW = int(float64(m.width) * m.splitRatio)
	if listW < 28 {
		listW = 28
	}
	if maxList := m.width - 42; listW > maxList && maxList >= 28 {
		listW = maxList
	}
	detailW = m.width - listW - 1 // 1-col gap between framed panes
	if detailW < 10 {
		detailW = 10
	}
	return listW, detailW
}

// OnThemeChanged re-derives styles baked into retained components
// (core.ThemeAware).
func (m *Model) OnThemeChanged() {
	m.spinner.Style = lipgloss.NewStyle().Foreground(m.ctx.Styles.Palette.Primary)
	m.filterInput.SetStyles(inputs.Styles(m.ctx.Styles))
	m.detail.resetRenderer()
	m.detail.rebuild()
}

func (m *Model) Title() string { return "Board" }

func (m *Model) ShortHelp() []key.Binding {
	k := m.ctx.Keys
	if m.filtering {
		return []key.Binding{k.Enter, k.Back}
	}
	if m.focus == focusDetail {
		return []key.Binding{k.Up, k.Down, k.FocusToggle, k.Theme, k.Edit, k.Comments, k.Back}
	}
	return []key.Binding{k.Up, k.Down, k.Filter, k.Theme, k.Status, k.Priority, k.Assign, k.Edit, k.New, k.Help}
}

func (m *Model) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		if !m.loading {
			return nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return cmd

	case data.WorkItemsLoadedMsg:
		if msg.WorkspaceID != m.workspaceID || msg.RequestID != m.loadRequestID {
			return nil
		}
		m.items = msg.Items
		m.truncated = msg.Truncated
		m.loading = false
		m.SetSize(m.width, m.height) // truncation notice changes pane height
		m.rebuildRows()
		return m.syncDetail()

	case data.StatusesLoadedMsg:
		if msg.WorkspaceID != m.workspaceID || msg.RequestID != m.loadRequestID {
			return nil
		}
		m.statuses = msg.Statuses
		m.rebuildRows()
		return m.syncDetail()

	case data.PrioritiesLoadedMsg:
		if msg.WorkspaceID != m.workspaceID || msg.RequestID != m.loadRequestID {
			return nil
		}
		m.priorities = msg.Priorities
		m.rebuildRows()
		return nil

	case data.TimeProjectsLoadedMsg:
		if msg.WorkspaceID != m.workspaceID || msg.RequestID != m.loadRequestID {
			return nil
		}
		m.timeProjects = msg.Projects
		return nil

	case data.UsersLoadedMsg:
		if msg.WorkspaceID != m.workspaceID || msg.RequestID != m.loadRequestID {
			return nil
		}
		m.users = msg.Users
		return m.syncDetail()

	case data.WorkItemLoadedMsg:
		switch msg.Operation {
		case data.OpItemDetails:
			if msg.RequestID != m.detailRequests[msg.Item.ID] {
				return nil
			}
			m.detailsLoaded[msg.Item.ID] = true
		case data.OpItemStatus, data.OpItemPriority, data.OpItemAssignee, data.OpItemUpdate:
			if msg.RequestID != m.mutationRequests[msg.Item.ID] {
				return nil
			}
			delete(m.mutationRequests, msg.Item.ID)
		default:
			return nil
		}
		for i := range m.items {
			if m.items[i].ID == msg.Item.ID {
				m.items[i] = msg.Item
				break
			}
		}
		m.rebuildRows()
		cmds := []tea.Cmd{m.syncDetail()}
		if msg.Operation == data.OpItemDetails && m.pendingItemID == msg.Item.ID {
			action := m.pendingItemAction
			m.pendingItemAction = ""
			m.pendingItemID = 0
			switch action {
			case pickerStatusID:
				cmds = append(cmds, m.openStatusPicker())
			case formEditID:
				cmds = append(cmds, m.openEdit())
			}
		}
		if msg.Operation != data.OpItemDetails {
			cmds = append(cmds, core.NotifySuccess(m.itemKey(&msg.Item)+" updated"))
		}
		return tea.Batch(cmds...)

	case dialog.ResultMsg:
		return m.applyPickerResult(msg)

	case data.WorkItemCreatedMsg:
		if msg.WorkspaceID == m.workspaceID && m.itemCreateRequests[msg.RequestID] {
			delete(m.itemCreateRequests, msg.RequestID)
			return tea.Batch(core.NotifySuccess("Work item created"), m.reloadItems())
		}
		return nil

	case data.WorkItemUpdatedMsg:
		if msg.WorkspaceID == m.workspaceID {
			return tea.Batch(core.NotifySuccess("Saved"), m.reloadItems())
		}
		return nil

	case data.CommentCreatedMsg:
		if m.commentCreateRequests[msg.RequestID] != msg.ItemID {
			return nil
		}
		delete(m.commentCreateRequests, msg.RequestID)
		requestID := m.ctx.NextRequestID()
		m.commentState[msg.ItemID] = loadLoading
		m.commentRequests[msg.ItemID] = requestID
		return tea.Batch(core.NotifySuccess("Comment added to "+m.keyForItemID(msg.ItemID)), data.LoadComments(m.ctx.Client, msg.ItemID, requestID))

	case data.CommentsLoadedMsg:
		if msg.RequestID != m.commentRequests[msg.ItemID] {
			return nil
		}
		m.comments[msg.ItemID] = msg.Comments
		m.commentState[msg.ItemID] = loadLoaded
		if it := m.list.selectedItem(); it != nil && it.ID == msg.ItemID {
			m.detail.setComments(msg.Comments)
		}
		return nil

	case data.AgentRunsLoadedMsg:
		if msg.RequestID != m.runRequests[msg.ItemID] {
			return nil
		}
		m.agentRuns[msg.ItemID] = msg.Runs
		m.runState[msg.ItemID] = loadLoaded
		if it := m.list.selectedItem(); it != nil && it.ID == msg.ItemID {
			m.detail.setAgentRuns(msg.Runs)
		}
		if hasActiveRun(msg.Runs) {
			return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
				return agentRefreshMsg{itemID: msg.ItemID, requestID: msg.RequestID}
			})
		}
		return nil

	case agentRefreshMsg:
		if msg.requestID != m.runRequests[msg.itemID] || m.runState[msg.itemID] != loadLoaded {
			return nil
		}
		return m.loadAgentRuns(msg.itemID)

	case debounceMsg:
		if msg.seq != m.detailSeq {
			return nil // selection moved on
		}
		it := m.list.selectedItem()
		if it == nil {
			return nil
		}
		var cmds []tea.Cmd
		if m.commentState[it.ID] == loadStale {
			cmds = append(cmds, m.loadComments(it.ID))
		}
		if m.runState[it.ID] == loadStale {
			cmds = append(cmds, m.loadAgentRuns(it.ID))
		}
		if !m.detailsLoaded[it.ID] && m.detailRequests[it.ID] == 0 {
			cmds = append(cmds, m.loadItemDetails(it.ID))
		}
		return tea.Batch(cmds...)

	case data.PrefsLoadedMsg:
		// Prefs arrived after the board was built — apply the split late.
		if msg.OK {
			if r := msg.Prefs.SplitRatio; r != nil && *r >= minSplitRatio && *r <= maxSplitRatio {
				m.splitRatio = *r
				m.SetSize(m.width, m.height)
			}
		}
		return nil

	case data.CurrentUserLoadedMsg:
		if m.pendingTimeLogItemID == 0 {
			return nil
		}
		itemID := m.pendingTimeLogItemID
		m.pendingTimeLogItemID = 0
		if !msg.OK {
			return core.NotifyError("Could not load your timezone")
		}
		if item := m.itemByID(itemID); item != nil {
			return core.Push(timelog.New(m.ctx, *item, m.timeProjects))
		}
		return nil

	case data.ErrorMsg:
		matched := false
		if msg.Operation == data.OpWorkItems && msg.WorkspaceID == m.workspaceID && msg.RequestID == m.loadRequestID {
			m.loading = false
			matched = true
		}
		if msg.WorkspaceID == m.workspaceID && msg.RequestID == m.loadRequestID {
			switch msg.Operation {
			case data.OpStatuses, data.OpPriorities, data.OpTimeProjects, data.OpUsers:
				matched = true
			}
		}
		if msg.Operation == data.OpComments && msg.RequestID == m.commentRequests[msg.ItemID] {
			m.commentState[msg.ItemID] = loadFailed
			m.refreshSelectedDetail()
			matched = true
		}
		if msg.Operation == data.OpAgentRuns && msg.RequestID == m.runRequests[msg.ItemID] {
			m.runState[msg.ItemID] = loadFailed
			m.refreshSelectedDetail()
			matched = true
		}
		if msg.Operation == data.OpItemDetails && msg.RequestID == m.detailRequests[msg.ItemID] {
			delete(m.detailRequests, msg.ItemID)
			m.pendingItemAction = ""
			m.pendingItemID = 0
			matched = true
		}
		if msg.RequestID == m.mutationRequests[msg.ItemID] {
			delete(m.mutationRequests, msg.ItemID)
			m.detailsLoaded[msg.ItemID] = false
			delete(m.detailRequests, msg.ItemID)
			return tea.Batch(core.NotifyError(m.keyForItemID(msg.ItemID)+": "+msg.Err), m.loadItemDetails(msg.ItemID))
		}
		if msg.Operation == data.OpItemCreate && msg.WorkspaceID == m.workspaceID && m.itemCreateRequests[msg.RequestID] {
			delete(m.itemCreateRequests, msg.RequestID)
			matched = true
		}
		if msg.Operation == data.OpCommentCreate && m.commentCreateRequests[msg.RequestID] == msg.ItemID {
			delete(m.commentCreateRequests, msg.RequestID)
			matched = true
		}
		if matched {
			return core.NotifyError(msg.Err)
		}
		return nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	if m.filtering {
		var cmd tea.Cmd
		m.filterInput, cmd = m.filterInput.Update(msg)
		return cmd
	}
	return nil
}

func (m *Model) reloadItems() tea.Cmd {
	m.loadRequestID = m.ctx.NextRequestID()
	m.loading = true
	return tea.Batch(
		data.LoadWorkItems(m.ctx.Client, m.workspaceID, m.loadRequestID),
		data.LoadStatuses(m.ctx.Client, m.workspaceID, m.loadRequestID),
		data.LoadPriorities(m.ctx.Client, m.workspaceID, m.loadRequestID),
		data.LoadTimeProjects(m.ctx.Client, m.workspaceID, m.loadRequestID),
		data.LoadAssignableUsers(m.ctx.Client, m.workspaceID, m.loadRequestID),
		m.spinner.Tick,
	)
}

// rebuildRows reflattens grouping state into the list pane.
func (m *Model) rebuildRows() {
	catByStatus := make(map[int]string, len(m.statuses))
	colorByCat := make(map[string]string, len(m.statuses))
	for _, s := range m.statuses {
		catByStatus[s.ID] = s.CategoryName
		if s.CategoryName != "" && s.CategoryColor != "" {
			colorByCat[s.CategoryName] = s.CategoryColor
		}
	}
	prioRank := make(map[int]int, len(m.priorities))
	for i, p := range m.priorities {
		prioRank[p.ID] = i
	}
	me := 0
	if m.ctx.User != nil {
		me = m.ctx.User.UserID
	}
	wsKey := ""
	if m.ctx.Workspace != nil {
		wsKey = m.ctx.Workspace.Key
	}
	m.list.setRows(BuildRows(m.items, Grouping{
		CategoryByStatusID: catByStatus,
		PriorityRank:       prioRank,
		MeUserID:           me,
		Collapsed:          m.collapsed,
		Filter:             m.filter,
		WorkspaceKey:       wsKey,
		ColorByCategory:    colorByCat,
	}))
}

// EditingText reports whether the filter input is focused (core.TextEditor)
// so global single-key bindings don't swallow typed filter characters.
func (m *Model) EditingText() bool { return m.filtering }

// applyPickerResult patches the selected item optimistically and fires the
// matching mutation + targeted refresh.
func (m *Model) applyPickerResult(msg dialog.ResultMsg) tea.Cmd {
	// Create doesn't need a selection.
	if msg.ID == formCreateID {
		form, ok := msg.Value.(dialog.FormResult)
		if !ok || m.ctx.Workspace == nil {
			return nil
		}
		title := strings.TrimSpace(form.Values["title"])
		if title == "" {
			return core.NotifyError("Title is required")
		}
		var priorityID *int
		if priority, ok := form.Choices["priority"].(data.Priority); ok && priority.ID > 0 {
			id := priority.ID
			priorityID = &id
		}
		requestID := m.ctx.NextRequestID()
		m.itemCreateRequests[requestID] = true
		return data.CreateWorkItem(m.ctx.Client, m.workspaceID, title, form.Values["description"], priorityID, requestID)
	}

	it := m.list.selectedItem()
	if it == nil {
		return nil
	}
	switch msg.ID {
	case formEditID:
		form, ok := msg.Value.(dialog.FormResult)
		if !ok {
			return nil
		}
		title := strings.TrimSpace(form.Values["title"])
		if title == "" {
			return core.NotifyError("Title is required")
		}
		var statusID, priorityID *int
		if status, ok := form.Choices["status"].(data.Status); ok && (it.StatusID == nil || *it.StatusID != status.ID) {
			id := status.ID
			statusID = &id
		}
		if priority, ok := form.Choices["priority"].(data.Priority); ok && (it.PriorityID == nil || *it.PriorityID != priority.ID) {
			id := priority.ID
			priorityID = &id
		}
		assigneeChanged := false
		assigneeID := 0
		if assignee, ok := form.Choices["assignee"].(data.User); ok {
			assigneeID = assignee.ID
			current := 0
			if it.AssigneeID != nil {
				current = *it.AssigneeID
			}
			if current != assigneeID {
				assigneeChanged = true
			}
		}
		return m.startMutation(it.ID, func(requestID uint64) tea.Cmd {
			return data.UpdateWorkItem(m.ctx.Client, m.workspaceID, it.ID, title, form.Values["description"], statusID, priorityID, assigneeChanged, assigneeID, requestID)
		})
	case formCommentID:
		form, ok := msg.Value.(dialog.FormResult)
		if !ok {
			return nil
		}
		body := strings.TrimSpace(form.Values["comment"])
		if body == "" {
			return nil
		}
		requestID := m.ctx.NextRequestID()
		m.commentCreateRequests[requestID] = it.ID
		return data.CreateComment(m.ctx.Client, it.ID, body, requestID)
	case pickerStatusID:
		s, ok := msg.Value.(data.Status)
		if !ok {
			return nil
		}
		if it.StatusID != nil && *it.StatusID == s.ID {
			return nil
		}
		return m.startMutation(it.ID, func(requestID uint64) tea.Cmd {
			return data.SetItemStatus(m.ctx.Client, it.ID, s.ID, requestID)
		})
	case pickerPriorityID:
		p, ok := msg.Value.(data.Priority)
		if !ok {
			return nil
		}
		if it.PriorityID != nil && *it.PriorityID == p.ID {
			return nil
		}
		return m.startMutation(it.ID, func(requestID uint64) tea.Cmd {
			return data.SetItemPriority(m.ctx.Client, it.ID, p.ID, requestID)
		})
	case pickerAssignID:
		u, ok := msg.Value.(data.User)
		if !ok {
			return nil
		}
		current := 0
		if it.AssigneeID != nil {
			current = *it.AssigneeID
		}
		if current == u.ID {
			return nil
		}
		return m.startMutation(it.ID, func(requestID uint64) tea.Cmd {
			return data.SetItemAssignee(m.ctx.Client, it.ID, u.ID, requestID)
		})
	}
	return nil
}

func (m *Model) startMutation(itemID int, command func(uint64) tea.Cmd) tea.Cmd {
	if _, busy := m.mutationRequests[itemID]; busy {
		return core.NotifyError(m.keyForItemID(itemID) + " is already being updated")
	}
	requestID := m.ctx.NextRequestID()
	m.mutationRequests[itemID] = requestID
	return command(requestID)
}

// syncDetail points the detail pane at the current selection and returns the
// debounced fetch command when any lazy section's cache is cold.
func (m *Model) syncDetail() tea.Cmd {
	it := m.list.selectedItem()
	if it == nil {
		m.detail.setItem(nil, nil, loadStale, nil, loadStale, false)
		return nil
	}
	commentState := m.commentState[it.ID]
	runState := m.runState[it.ID]
	m.detail.setItem(it, m.comments[it.ID], commentState, m.agentRuns[it.ID], runState, m.assigneeIsAgent(it))
	if commentState != loadStale && runState != loadStale && (m.detailsLoaded[it.ID] || m.detailRequests[it.ID] != 0) {
		return nil
	}
	m.detailSeq++
	seq := m.detailSeq
	return tea.Tick(commentsDebounce, func(time.Time) tea.Msg { return debounceMsg{seq: seq} })
}

func (m *Model) refreshSelectedDetail() {
	if it := m.list.selectedItem(); it != nil {
		m.detail.setItem(it, m.comments[it.ID], m.commentState[it.ID], m.agentRuns[it.ID], m.runState[it.ID], m.assigneeIsAgent(it))
	}
}

func (m *Model) loadComments(itemID int) tea.Cmd {
	requestID := m.ctx.NextRequestID()
	m.commentState[itemID] = loadLoading
	m.commentRequests[itemID] = requestID
	return data.LoadComments(m.ctx.Client, itemID, requestID)
}

func (m *Model) loadAgentRuns(itemID int) tea.Cmd {
	requestID := m.ctx.NextRequestID()
	m.runState[itemID] = loadLoading
	m.runRequests[itemID] = requestID
	return data.LoadAgentRuns(m.ctx.Client, itemID, requestID)
}

func (m *Model) loadItemDetails(itemID int) tea.Cmd {
	requestID := m.ctx.NextRequestID()
	m.detailRequests[itemID] = requestID
	return data.ReloadWorkItem(m.ctx.Client, itemID, data.OpItemDetails, requestID)
}

func hasActiveRun(runs []data.AgentRun) bool {
	for _, run := range runs {
		if run.Status == "queued" || run.Status == "running" {
			return true
		}
	}
	return false
}

func (m *Model) keyForItemID(itemID int) string {
	if item := m.itemByID(itemID); item != nil {
		return m.itemKey(item)
	}
	return fmt.Sprintf("item %d", itemID)
}

func (m *Model) itemByID(itemID int) *data.WorkItem {
	for i := range m.items {
		if m.items[i].ID == itemID {
			return &m.items[i]
		}
	}
	return nil
}

// assigneeIsAgent resolves the item's assignee against the workspace user
// list to detect coding agents.
func (m *Model) assigneeIsAgent(it *data.WorkItem) bool {
	if it.AssigneeID == nil {
		return false
	}
	for _, u := range m.users {
		if u.ID == *it.AssigneeID {
			return u.IsAgent
		}
	}
	return false
}

func (m *Model) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	k := m.ctx.Keys

	if m.filtering {
		return m.handleFilterKey(msg)
	}

	if m.focus == focusDetail {
		switch {
		case key.Matches(msg, k.Up):
			m.detail.scroll(-1)
		case key.Matches(msg, k.Down):
			m.detail.scroll(1)
		case key.Matches(msg, k.HalfPageUp):
			m.detail.halfPage(-1)
		case key.Matches(msg, k.HalfPageDown):
			m.detail.halfPage(1)
		case key.Matches(msg, k.FocusToggle), key.Matches(msg, k.Back):
			m.focus = focusList
			m.narrowDetail = false
		case key.Matches(msg, k.Edit):
			return m.openEdit()
		case key.Matches(msg, k.Comments):
			return m.openComment()
		}
		return nil
	}

	switch {
	case key.Matches(msg, k.Up):
		m.list.move(-1)
		return m.syncDetail()
	case key.Matches(msg, k.Down):
		m.list.move(1)
		return m.syncDetail()
	case key.Matches(msg, k.PrevGroup):
		m.list.jumpGroup(-1)
		return m.syncDetail()
	case key.Matches(msg, k.NextGroup):
		m.list.jumpGroup(1)
		return m.syncDetail()
	case key.Matches(msg, k.HalfPageUp):
		m.list.halfPage(-1)
		return m.syncDetail()
	case key.Matches(msg, k.HalfPageDown):
		m.list.halfPage(1)
		return m.syncDetail()
	case key.Matches(msg, k.Collapse):
		if g := m.list.selectedGroupKey(); g != "" {
			m.collapsed[g] = !m.collapsed[g]
			m.rebuildRows()
			return m.syncDetail()
		}
	case key.Matches(msg, k.Enter), key.Matches(msg, k.FocusToggle):
		if m.list.selectedItem() != nil {
			m.focus = focusDetail
			m.narrowDetail = true
		}
	case key.Matches(msg, k.SplitNarrow):
		return m.adjustSplit(-splitStep)
	case key.Matches(msg, k.SplitWiden):
		return m.adjustSplit(splitStep)
	case key.Matches(msg, k.Filter):
		m.filtering = true
		m.filterInput.SetValue(m.filter.Query)
		m.filterInput.CursorEnd()
		m.SetSize(m.width, m.height) // reserve the filter line
		return m.filterInput.Focus()
	case key.Matches(msg, k.Status):
		return m.openStatusPicker()
	case key.Matches(msg, k.Priority):
		return m.openPriorityPicker()
	case key.Matches(msg, k.Assign):
		return m.openAssignPicker()
	case key.Matches(msg, k.Edit):
		return m.openEdit()
	case key.Matches(msg, k.New):
		return m.openCreate()
	case key.Matches(msg, k.Comments):
		return m.openComment()
	case key.Matches(msg, k.LogTime):
		if it := m.list.selectedItem(); it != nil {
			if !m.ctx.UserTimezoneKnown {
				m.pendingTimeLogItemID = it.ID
				return data.LoadCurrentUser(m.ctx.Client)
			}
			return core.Push(timelog.New(m.ctx, *it, m.timeProjects))
		}
	case key.Matches(msg, k.Refresh):
		var detailCmd tea.Cmd
		if it := m.list.selectedItem(); it != nil {
			m.commentState[it.ID] = loadStale
			m.runState[it.ID] = loadStale
			m.detailsLoaded[it.ID] = false
			delete(m.detailRequests, it.ID)
			detailCmd = m.syncDetail()
		}
		return tea.Batch(m.reloadItems(), detailCmd)
	case key.Matches(msg, k.Back):
		if m.filter.Active() {
			m.clearFilter()
			return m.syncDetail()
		}
		m.ctx.Workspace = nil
		return core.Pop()
	}
	return nil
}

// handleFilterKey routes keys while the filter input is focused: the filter
// applies live on every keystroke; enter keeps it and returns focus to the
// list; esc clears it.
func (m *Model) handleFilterKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		m.clearFilter()
		return m.syncDetail()
	case "enter":
		m.filtering = false
		m.filterInput.Blur()
		if !m.filter.Active() {
			m.SetSize(m.width, m.height) // release the filter line
		}
		return nil
	}
	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)
	if q := m.filterInput.Value(); q != m.filter.Query {
		m.filter.Query = q
		m.rebuildRows()
		return tea.Batch(cmd, m.syncDetail())
	}
	return cmd
}

func (m *Model) clearFilter() {
	m.filtering = false
	m.filter.Query = ""
	m.filterInput.SetValue("")
	m.filterInput.Blur()
	m.SetSize(m.width, m.height)
	m.rebuildRows()
}

func (m *Model) openStatusPicker() tea.Cmd {
	it := m.list.selectedItem()
	if it == nil {
		return nil
	}
	if !m.detailsLoaded[it.ID] {
		m.pendingItemAction = pickerStatusID
		m.pendingItemID = it.ID
		if m.detailRequests[it.ID] != 0 {
			return nil
		}
		return m.loadItemDetails(it.ID)
	}
	transitions := validTransitions(it)
	if len(transitions) == 0 {
		return core.NotifyError("No status transition is available for " + m.itemKey(it))
	}
	options := make([]dialog.Option, len(transitions))
	for i, s := range transitions {
		options[i] = dialog.Option{
			Label:  chip.Status(m.ctx.Styles, s.Name, s.CategoryColor),
			Search: s.Name,
			Value:  s,
		}
	}
	return dialog.Open(dialog.NewPicker(pickerStatusID, "Set status", options, 0, m.ctx.Styles))
}

func (m *Model) openPriorityPicker() tea.Cmd {
	it := m.list.selectedItem()
	if it == nil || len(m.priorities) == 0 {
		return nil
	}
	options := make([]dialog.Option, len(m.priorities))
	selectedIdx := 0
	for i, p := range m.priorities {
		options[i] = dialog.Option{
			Label:  chip.Priority(m.ctx.Styles, p.Name, p.Color),
			Search: p.Name,
			Value:  p,
		}
		if it.PriorityID != nil && *it.PriorityID == p.ID {
			selectedIdx = i
		}
	}
	return dialog.Open(dialog.NewPicker(pickerPriorityID, "Set priority", options, selectedIdx, m.ctx.Styles))
}

func (m *Model) openAssignPicker() tea.Cmd {
	it := m.list.selectedItem()
	if it == nil || len(m.users) == 0 {
		return nil
	}
	s := m.ctx.Styles
	options := make([]dialog.Option, 0, len(m.users)+1)
	options = append(options, dialog.Option{
		Label:  s.Base.Hint.Render("(unassign)"),
		Search: "unassign",
		Value:  data.User{},
	})
	selectedIdx := 0
	for _, u := range m.users {
		label := u.FullName
		if label == "" {
			label = u.Username
		}
		if u.IsAgent {
			label += " " + s.List.Muted.Render("· agent")
		}
		if m.ctx.User != nil && u.ID == m.ctx.User.UserID {
			label += " " + s.List.Muted.Render("· me")
		}
		options = append(options, dialog.Option{Label: label, Search: u.FullName + " " + u.Username, Value: u})
		if it.AssigneeID != nil && *it.AssigneeID == u.ID {
			selectedIdx = len(options) - 1
		}
	}
	return dialog.Open(dialog.NewPicker(pickerAssignID, "Assign to", options, selectedIdx, m.ctx.Styles))
}

func (m *Model) statusFormOptions(it *data.WorkItem) (options []dialog.Option, selected any) {
	if it == nil {
		return nil, nil
	}
	allowed := make([]data.Status, 0, len(it.Transitions)+1)
	if it.StatusID != nil {
		for _, status := range m.statuses {
			if status.ID == *it.StatusID {
				allowed = append(allowed, status)
				break
			}
		}
	}
	allowed = append(allowed, validTransitions(it)...)
	options = make([]dialog.Option, 0, len(allowed))
	for _, status := range allowed {
		options = append(options, dialog.Option{
			Label: chip.Status(m.ctx.Styles, status.Name, status.CategoryColor), Search: status.Name, Value: status,
		})
		if it.StatusID != nil && *it.StatusID == status.ID {
			selected = status
		}
	}
	return options, selected
}

func validTransitions(it *data.WorkItem) []data.Status {
	seen := make(map[int]bool, len(it.Transitions))
	if it.StatusID != nil {
		seen[*it.StatusID] = true
	}
	transitions := make([]data.Status, 0, len(it.Transitions))
	for _, status := range it.Transitions {
		if seen[status.ID] {
			continue
		}
		seen[status.ID] = true
		transitions = append(transitions, status)
	}
	return transitions
}

func (m *Model) priorityFormOptions(it *data.WorkItem) (options []dialog.Option, selected any) {
	options = make([]dialog.Option, 0, len(m.priorities))
	for _, priority := range m.priorities {
		options = append(options, dialog.Option{
			Label: chip.Priority(m.ctx.Styles, priority.Name, priority.Color), Search: priority.Name, Value: priority,
		})
		if it != nil && it.PriorityID != nil && *it.PriorityID == priority.ID {
			selected = priority
		}
	}
	return options, selected
}

func (m *Model) assigneeFormOptions(it *data.WorkItem) (options []dialog.Option, selected any) {
	s := m.ctx.Styles
	unassigned := data.User{}
	options = make([]dialog.Option, 0, len(m.users)+1)
	options = append(options, dialog.Option{Label: s.Base.Hint.Render("(unassigned)"), Search: "unassigned", Value: unassigned})
	selected = unassigned
	for _, user := range m.users {
		label := user.FullName
		if label == "" {
			label = user.Username
		}
		if user.IsAgent {
			label += " " + s.List.Muted.Render("· agent")
		}
		if m.ctx.User != nil && user.ID == m.ctx.User.UserID {
			label += " " + s.List.Muted.Render("· me")
		}
		options = append(options, dialog.Option{Label: label, Search: user.FullName + " " + user.Username, Value: user})
		if it != nil && it.AssigneeID != nil && *it.AssigneeID == user.ID {
			selected = user
		}
	}
	return options, selected
}

func (m *Model) adjustSplit(delta float64) tea.Cmd {
	m.splitRatio += delta
	if m.splitRatio < minSplitRatio {
		m.splitRatio = minSplitRatio
	}
	if m.splitRatio > maxSplitRatio {
		m.splitRatio = maxSplitRatio
	}
	m.SetSize(m.width, m.height)

	r := m.splitRatio
	m.ctx.Prefs.SplitRatio = &r
	return core.SaveSplitRatio(r)
}

// formWidth sizes overlay-form fields to the terminal.
func (m *Model) formWidth() int {
	w := m.width - 16
	if w > 70 {
		w = 70
	}
	if w < 1 {
		w = 1
	}
	return w
}

func (m *Model) newFormTextarea(value string, height int) textarea.Model {
	ta := textarea.New()
	ta.SetStyles(inputs.TextareaStyles(m.ctx.Styles))
	ta.SetValue(value)
	ta.SetWidth(m.formWidth())
	ta.SetHeight(height)
	ta.ShowLineNumbers = false
	ta.CharLimit = 5000
	ta.Placeholder = "Markdown supported…"
	return ta
}

func (m *Model) openEdit() tea.Cmd {
	it := m.list.selectedItem()
	if it == nil {
		return nil
	}
	if !m.detailsLoaded[it.ID] {
		m.pendingItemAction = formEditID
		m.pendingItemID = it.ID
		if m.detailRequests[it.ID] != 0 {
			return nil
		}
		return m.loadItemDetails(it.ID)
	}
	title := inputs.New(m.ctx.Styles, "Title", 200)
	title.SetValue(it.Title)
	fields := []dialog.FormField{
		{Key: "title", Label: "Title", Input: title},
		{Key: "description", Label: "Description", Multiline: true, Area: m.newFormTextarea(it.Description, 10)},
	}
	if options, value := m.statusFormOptions(it); len(options) > 0 {
		fields = append(fields, dialog.FormField{Key: "status", Label: "Status", Choice: &dialog.FormChoice{
			PickerID: pickerStatusID, PickerTitle: "Set status", Options: options, Value: value,
		}})
	}
	if options, value := m.priorityFormOptions(it); len(options) > 0 {
		fields = append(fields, dialog.FormField{Key: "priority", Label: "Priority", Choice: &dialog.FormChoice{
			PickerID: pickerPriorityID, PickerTitle: "Set priority", Options: options, Value: value,
		}})
	}
	if options, value := m.assigneeFormOptions(it); len(options) > 0 {
		fields = append(fields, dialog.FormField{Key: "assignee", Label: "Assignee", Choice: &dialog.FormChoice{
			PickerID: pickerAssignID, PickerTitle: "Assign to", Options: options, Value: value,
		}})
	}
	return dialog.Open(dialog.NewForm(formEditID, "Edit · "+m.itemKey(it), fields, m.ctx.Styles, m.formWidth()))
}

func (m *Model) openCreate() tea.Cmd {
	if m.ctx.Workspace == nil {
		return nil
	}
	title := inputs.New(m.ctx.Styles, "Title", 200)
	fields := []dialog.FormField{
		{Key: "title", Label: "Title", Input: title},
		{Key: "description", Label: "Description (optional)", Multiline: true, Area: m.newFormTextarea("", 6)},
	}
	if options, value := m.priorityFormOptions(nil); len(options) > 0 {
		fields = append(fields, dialog.FormField{Key: "priority", Label: "Priority", Choice: &dialog.FormChoice{
			PickerID: pickerPriorityID, PickerTitle: "Set priority", Options: options, Value: value,
		}})
	}
	return dialog.Open(dialog.NewForm(formCreateID, "New work item · "+m.ctx.Workspace.Name, fields, m.ctx.Styles, m.formWidth()))
}

func (m *Model) openComment() tea.Cmd {
	it := m.list.selectedItem()
	if it == nil {
		return nil
	}
	fields := []dialog.FormField{
		{Key: "comment", Label: "Comment", Multiline: true, Area: m.newFormTextarea("", 6)},
	}
	return dialog.Open(dialog.NewForm(formCommentID, "Comment on "+m.itemKey(it), fields, m.ctx.Styles, m.formWidth()))
}

func (m *Model) itemKey(it *data.WorkItem) string {
	prefix := ""
	if prefix == "" && m.ctx.Workspace != nil {
		prefix = m.ctx.Workspace.Key
	}
	return it.DisplayKey(prefix)
}

func (m *Model) View() string {
	s := m.ctx.Styles

	if m.loading && len(m.items) == 0 {
		return m.spinner.View() + " " + s.Base.Hint.Render("Loading work items…")
	}

	var notice string
	if m.truncated {
		notice = s.Base.Hint.Render("Showing the first "+strconv.Itoa(len(m.items))+" items — refine in the web UI for more.") + "\n"
	}

	h := m.paneHeight()

	if m.narrow() {
		if m.narrowDetail {
			return notice + framePane(s, m.detailTitle(), m.detail.view(), m.width, h, true)
		}
		return notice + framePane(s, m.listTitle(), m.listColumn(), m.width, h, true)
	}

	listW, detailW := m.paneWidths()

	listBlock := framePane(s, m.listTitle(), m.listColumn(), listW, h, m.focus == focusList)
	detailBlock := framePane(s, m.detailTitle(), m.detail.view(), detailW, h, m.focus == focusDetail)

	return notice + lipgloss.JoinHorizontal(lipgloss.Top, listBlock, " ", detailBlock)
}

// listTitle labels the list pane's frame with the workspace and item count.
func (m *Model) listTitle() string {
	name := "Work items"
	if m.ctx.Workspace != nil {
		name = m.ctx.Workspace.Name
	}
	return fmt.Sprintf("%s · %d", name, len(m.items))
}

// detailTitle labels the detail pane's frame with the selected item's key.
func (m *Model) detailTitle() string {
	if it := m.list.selectedItem(); it != nil {
		return m.itemKey(it)
	}
	return "Details"
}

// listColumn stacks the list rows and, when active, the filter input line.
func (m *Model) listColumn() string {
	out := m.list.view()
	if m.filterVisible() {
		out += "\n" + m.ctx.Styles.Status.KeyChord.Render("/") + " " + m.filterInput.View()
	}
	return out
}
