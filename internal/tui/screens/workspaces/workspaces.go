// Package workspaces is the workspace picker — the first screen of a
// session.
package workspaces

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"windshift/internal/tui/components/splash"
	"windshift/internal/tui/core"
	"windshift/internal/tui/data"
	"windshift/internal/tui/screens/board"
)

// Model is the workspace picker screen.
type Model struct {
	ctx *core.Ctx

	workspaces []data.Workspace
	selected   int
	offset     int
	loading    bool
	loadedOnce bool
	// autoOpened guards the once-per-session jump to the persisted last
	// workspace — going back to the picker must not bounce the user again.
	autoOpened bool
	prefsKnown bool

	spinner   spinner.Model
	width     int
	height    int
	requestID uint64
}

func New(ctx *core.Ctx) *Model {
	sp := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	sp.Style = lipgloss.NewStyle().Foreground(ctx.Styles.Palette.Primary)
	return &Model{
		ctx:     ctx,
		loading: true,
		spinner: sp,
	}
}

func (m *Model) Init() tea.Cmd {
	m.requestID = m.ctx.NextRequestID()
	m.loading = true
	return tea.Batch(data.LoadWorkspaces(m.ctx.Client, m.requestID), m.spinner.Tick)
}

func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.ensureVisible()
}

// OnThemeChanged re-derives styles baked into retained components
// (core.ThemeAware).
func (m *Model) OnThemeChanged() {
	m.spinner.Style = lipgloss.NewStyle().Foreground(m.ctx.Styles.Palette.Primary)
}

func (m *Model) Title() string { return "Pick a workspace" }

func (m *Model) ShortHelp() []key.Binding {
	k := m.ctx.Keys
	return []key.Binding{k.Up, k.Down, k.Enter, k.Theme, k.Refresh, k.Help, k.Quit}
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

	case data.WorkspacesLoadedMsg:
		if msg.RequestID != m.requestID {
			return nil
		}
		m.workspaces = msg.Workspaces
		m.loading = false
		m.loadedOnce = true
		if len(m.workspaces) > 0 && m.selected >= len(m.workspaces) {
			m.selected = 0
		}
		m.ensureVisible()
		return m.maybeAutoOpen()

	case data.PrefsLoadedMsg:
		m.prefsKnown = true
		return m.maybeAutoOpen()

	case data.ErrorMsg:
		if msg.Operation == data.OpWorkspaces && msg.RequestID == m.requestID {
			m.loading = false
			return core.NotifyError(msg.Err)
		}
		return nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return nil
}

// maybeAutoOpen jumps straight to the persisted last workspace once both
// the workspace list and the preferences have arrived. Runs at most once
// per session and only while this screen is the sole (bottom) screen.
func (m *Model) maybeAutoOpen() tea.Cmd {
	if m.autoOpened || !m.prefsKnown || !m.loadedOnce || m.ctx.Workspace != nil {
		return nil
	}
	m.autoOpened = true
	target := m.ctx.Prefs.LastWorkspaceID
	if target == nil {
		return nil
	}
	for i := range m.workspaces {
		if m.workspaces[i].ID == *target {
			m.selected = i
			ws := m.workspaces[i]
			m.ctx.Workspace = &ws
			return core.Push(board.New(m.ctx))
		}
	}
	return nil // stale workspace id — stay on the picker
}

func (m *Model) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	k := m.ctx.Keys
	switch {
	case key.Matches(msg, k.Up):
		if m.selected > 0 {
			m.selected--
		} else if len(m.workspaces) > 0 {
			m.selected = len(m.workspaces) - 1
		}
		m.ensureVisible()
	case key.Matches(msg, k.Down):
		if len(m.workspaces) > 0 {
			m.selected = (m.selected + 1) % len(m.workspaces)
		}
		m.ensureVisible()
	case key.Matches(msg, k.Enter):
		if len(m.workspaces) > 0 && m.selected < len(m.workspaces) {
			ws := m.workspaces[m.selected]
			m.ctx.Workspace = &ws
			return tea.Batch(
				core.Push(board.New(m.ctx)),
				core.SaveLastWorkspace(ws.ID),
			)
		}
	case key.Matches(msg, k.Refresh):
		m.requestID = m.ctx.NextRequestID()
		m.loading = true
		return tea.Batch(data.LoadWorkspaces(m.ctx.Client, m.requestID), m.spinner.Tick)
	}
	return nil
}

func (m *Model) View() string {
	s := m.ctx.Styles

	// First load: full splash instead of a bare list heading.
	if m.loading && !m.loadedOnce && len(m.workspaces) == 0 {
		return splash.Render(s, m.width, m.height)
	}

	heading := s.Base.Heading.Render("Workspaces")
	count := s.List.Counter.Render(fmt.Sprintf("%d total", len(m.workspaces)))

	if m.loading {
		return heading + "\n\n" + m.spinner.View() + " " + s.Base.Hint.Render("Loading workspaces…")
	}
	if len(m.workspaces) == 0 {
		return heading + "\n\n" + s.List.Empty.Render("No workspaces available.")
	}

	rows := []string{heading + "   " + count, ""}
	visible := max(1, m.height-len(rows))
	m.ensureVisible()
	end := min(len(m.workspaces), m.offset+visible)
	for i := m.offset; i < end; i++ {
		w := m.workspaces[i]
		label := fmt.Sprintf("%s · %s", w.Key, w.Name)
		if i == m.selected {
			rows = append(rows, s.List.SelBar.Render("▎")+" "+s.List.ItemSelected.Render(label))
		} else {
			rows = append(rows, "  "+s.List.Item.Render(label))
		}
	}
	return strings.Join(rows, "\n")
}

func (m *Model) ensureVisible() {
	visible := max(1, m.height-2)
	if m.selected < m.offset {
		m.offset = m.selected
	}
	if m.selected >= m.offset+visible {
		m.offset = m.selected - visible + 1
	}
	maxOffset := max(0, len(m.workspaces)-visible)
	if m.offset > maxOffset {
		m.offset = maxOffset
	}
	if m.offset < 0 {
		m.offset = 0
	}
}
