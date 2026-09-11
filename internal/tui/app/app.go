// Package app holds the root Bubble Tea model: a thin router/compositor
// that owns the screen stack, the dialog overlay stack and the status-bar
// notice, and delegates everything else to the active screen.
package app

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"windshift/internal/tui/components/header"
	"windshift/internal/tui/components/statusbar"
	"windshift/internal/tui/core"
	"windshift/internal/tui/data"
	"windshift/internal/tui/dialog"
	"windshift/internal/tui/styles"
)

// chromeRows is what the header (bar + rule) and status bar occupy.
const chromeRows = 3

const themePickerID = "app.theme"

// Model is the root model. It implements tea.Model by value but all mutable
// state lives behind pointers (ctx, screens), so Update mutates in place.
type Model struct {
	ctx     *core.Ctx
	stack   []core.Screen
	dialogs []dialog.Dialog
	notice  statusbar.Notice

	prefsKnown            bool
	prefsThemeDirty       bool
	prefsSplitDirty       bool
	prefsWorkspaceDirty   bool
	prefsVersion          uint64
	prefsSavingVersion    uint64
	prefsCompletedVersion uint64
	prefsSaving           bool
}

// New builds the root model with root as the bottom (initial) screen.
func New(ctx *core.Ctx, root core.Screen) Model {
	return Model{
		ctx:   ctx,
		stack: []core.Screen{root},
	}
}

func (m Model) active() core.Screen { return m.stack[len(m.stack)-1] }

// Init starts the initial screen and kicks off the preferences load (which
// never blocks startup — failure just means defaults).
func (m Model) Init() tea.Cmd {
	return tea.Batch(data.LoadPrefs(m.ctx.Client), data.LoadCurrentUser(m.ctx.Client), m.active().Init())
}

// Update routes messages: sizes fan out to the whole stack, keys go to the
// top dialog or the active screen, navigation/dialog/notice messages mutate
// root state, and everything else broadcasts to every stacked screen.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.ctx.Width = msg.Width
		m.ctx.Height = msg.Height
		for _, s := range m.stack {
			s.SetSize(msg.Width, bodyHeight(msg.Height))
		}
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case core.PushMsg:
		msg.Screen.SetSize(m.ctx.Width, bodyHeight(m.ctx.Height))
		m.stack = append(m.stack, msg.Screen)
		return m, msg.Screen.Init()

	case core.PopMsg:
		if len(m.stack) > 1 {
			m.stack = m.stack[:len(m.stack)-1]
		}
		return m, nil

	case core.ReplaceMsg:
		msg.Screen.SetSize(m.ctx.Width, bodyHeight(m.ctx.Height))
		m.stack[len(m.stack)-1] = msg.Screen
		return m, msg.Screen.Init()

	case dialog.OpenMsg:
		m.dialogs = append(m.dialogs, msg.Dialog)
		return m, nil

	case core.NoticeMsg:
		switch msg.Kind {
		case core.NoticeSuccess:
			m.notice = statusbar.Notice{Kind: statusbar.Success, Text: msg.Text}
		case core.NoticeError:
			m.notice = statusbar.Notice{Kind: statusbar.Error, Text: msg.Text}
		default:
			m.notice = statusbar.Notice{}
		}
		return m, nil

	case core.PrefsChangeMsg:
		if msg.SetTheme {
			m.ctx.Prefs.Theme = msg.Theme
			m.prefsThemeDirty = true
		}
		if msg.SetSplitRatio {
			ratio := msg.SplitRatio
			m.ctx.Prefs.SplitRatio = &ratio
			m.prefsSplitDirty = true
		}
		if msg.SetLastWorkspaceID {
			workspaceID := msg.LastWorkspaceID
			m.ctx.Prefs.LastWorkspaceID = &workspaceID
			m.prefsWorkspaceDirty = true
		}
		m.prefsVersion++
		cmd := m.startPrefsSave()
		return m, cmd

	case data.PrefsLoadedMsg:
		m.prefsKnown = true
		if msg.OK {
			if !m.prefsThemeDirty {
				m.ctx.Prefs.Theme = msg.Prefs.Theme
			}
			if !m.prefsSplitDirty {
				m.ctx.Prefs.SplitRatio = msg.Prefs.SplitRatio
			}
			if !m.prefsWorkspaceDirty {
				m.ctx.Prefs.LastWorkspaceID = msg.Prefs.LastWorkspaceID
			}
			if m.ctx.Prefs.Theme != "" {
				resolved := styles.ByName(m.ctx.Prefs.Theme)
				if resolved.Name != m.ctx.Theme {
					m.applyTheme(resolved)
				}
				if resolved.Name != m.ctx.Prefs.Theme {
					m.ctx.Prefs.Theme = resolved.Name
					m.prefsVersion++
					m.prefsThemeDirty = true
				}
			}
		}
		merged := data.PrefsLoadedMsg{Prefs: m.ctx.Prefs, OK: msg.OK}
		routeCmd := m.routeActive(merged)
		saveCmd := m.startPrefsSave()
		return m, tea.Batch(routeCmd, saveCmd)

	case data.PrefsSavedMsg:
		if msg.Version != m.prefsSavingVersion {
			return m, nil
		}
		m.prefsSaving = false
		m.prefsCompletedVersion = msg.Version
		if msg.Err != "" {
			m.notice = statusbar.Notice{Kind: statusbar.Error, Text: "Saving preferences failed: " + msg.Err}
		}
		cmd := m.startPrefsSave()
		return m, cmd

	case data.CurrentUserLoadedMsg:
		if msg.OK {
			m.ctx.UserTimezoneKnown = true
			if m.ctx.User != nil {
				m.ctx.User.Timezone = msg.Timezone
			}
		}
		return m, m.routeActive(msg)

	case data.ErrorMsg:
		return m, m.routeActive(msg)
	}

	return m, m.routeActiveAndDialog(msg)
}

// routeActive sends async results only to the screen that currently owns the
// foreground. Hidden screens must not consume stale results or keep lifecycle
// commands alive.
func (m Model) routeActive(msg tea.Msg) tea.Cmd {
	return m.active().Update(msg)
}

func (m Model) routeActiveAndDialog(msg tea.Msg) tea.Cmd {
	cmds := []tea.Cmd{m.routeActive(msg)}
	if len(m.dialogs) > 0 {
		if handler, ok := m.dialogs[len(m.dialogs)-1].(dialog.MessageHandler); ok {
			cmds = append(cmds, handler.HandleMessage(msg))
		}
	}
	return tea.Batch(cmds...)
}

func (m *Model) startPrefsSave() tea.Cmd {
	if !m.prefsKnown || m.prefsSaving || m.prefsVersion <= m.prefsCompletedVersion {
		return nil
	}
	m.prefsSaving = true
	m.prefsSavingVersion = m.prefsVersion
	return data.SavePrefs(m.ctx.Client, m.ctx.Prefs, m.prefsSavingVersion)
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Success notices are transient — any key clears them. Errors stay until
	// something succeeds or explicitly clears.
	if m.notice.Kind == statusbar.Success {
		m.notice = statusbar.Notice{}
	}

	// Ctrl+C is always a terminal interrupt. Printable q remains available in
	// text editors and dialog filters.
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}

	// Dialogs eat keys first.
	if len(m.dialogs) > 0 {
		top := m.dialogs[len(m.dialogs)-1]
		action := top.HandleKey(msg)
		var cmds []tea.Cmd
		if action.Cmd != nil {
			cmds = append(cmds, action.Cmd)
		}
		if action.Close {
			m.dialogs = m.dialogs[:len(m.dialogs)-1]
			if action.Selected != nil {
				result := dialog.ResultMsg{ID: top.ID(), Value: action.Selected}
				if top.ID() == themePickerID {
					if theme, ok := action.Selected.(styles.Theme); ok {
						m.applyTheme(theme)
						m.ctx.Prefs.Theme = theme.Name
						cmds = append(cmds, core.NotifySuccess("Theme: "+theme.Label), core.SaveTheme(theme.Name))
					}
				} else if len(m.dialogs) > 0 {
					parent := m.dialogs[len(m.dialogs)-1]
					if handler, ok := parent.(dialog.ResultHandler); ok {
						if cmd := handler.HandleResult(result); cmd != nil {
							cmds = append(cmds, cmd)
						}
					} else if cmd := m.active().Update(result); cmd != nil {
						cmds = append(cmds, cmd)
					}
				} else if cmd := m.active().Update(result); cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
		}
		return m, tea.Batch(cmds...)
	}

	// Global keys — but only when the active screen isn't editing text,
	// otherwise they'd swallow typed characters that look like
	// single-letter bindings (e.g. 'q' in a comment).
	if !m.editingText() {
		if key.Matches(msg, m.ctx.Keys.Quit) {
			return m, tea.Quit
		}
		if key.Matches(msg, m.ctx.Keys.Help) {
			m.dialogs = append(m.dialogs, dialog.NewHelp(m.helpGroups(), m.ctx.Styles))
			return m, nil
		}
		if key.Matches(msg, m.ctx.Keys.Theme) {
			m.dialogs = append(m.dialogs, m.themePicker())
			return m, nil
		}
	}

	return m, m.active().Update(msg)
}

func (m Model) themePicker() dialog.Dialog {
	themes := styles.Themes()
	options := make([]dialog.Option, 0, len(themes))
	selected := 0
	for i, theme := range themes {
		marker := "  "
		if theme.Name == m.ctx.Theme {
			marker = "● "
			selected = i
		}
		label := marker + theme.Label + " · " + theme.Description
		options = append(options, dialog.Option{Label: label, Search: theme.Label + " " + theme.Description, Value: theme})
	}
	return dialog.NewPicker(themePickerID, "Choose theme", options, selected, m.ctx.Styles)
}

// applyTheme replaces ctx.Styles wholesale — anything reading it at render
// time restyles for free; screens with baked styles are told via ThemeAware.
func (m Model) applyTheme(t styles.Theme) {
	m.ctx.Theme = t.Name
	m.ctx.Styles = styles.New(t.Palette)
	for _, s := range m.stack {
		if ta, ok := s.(core.ThemeAware); ok {
			ta.OnThemeChanged()
		}
	}
	for _, d := range m.dialogs {
		if themed, ok := d.(dialog.ThemeAware); ok {
			themed.OnThemeChanged(m.ctx.Styles)
		}
	}
}

func (m Model) editingText() bool {
	if ed, ok := m.active().(core.TextEditor); ok {
		return ed.EditingText()
	}
	return false
}

func (m Model) helpGroups() []dialog.HelpGroup {
	k := m.ctx.Keys
	return []dialog.HelpGroup{
		{Title: "Global", Binds: []key.Binding{k.Theme, k.Help, k.Quit}},
		{Title: "Navigation", Binds: []key.Binding{k.Up, k.Down, k.Enter, k.Back}},
		{Title: "Item actions", Binds: []key.Binding{k.New, k.Save, k.LogTime, k.Comments, k.Refresh}},
		{Title: "Editing", Binds: []key.Binding{k.NextField, k.PrevField}},
	}
}

// View composes the screen: header on top, body in the middle, status bar at
// the bottom. Active dialog overlays everything via lipgloss.Place.
func (m Model) View() tea.View {
	v := tea.NewView("")
	v.AltScreen = true
	v.MouseMode = tea.MouseModeNone
	v.WindowTitle = m.windowTitle()
	v.BackgroundColor = m.ctx.Styles.Palette.BgBase

	if m.ctx.Width == 0 || m.ctx.Height == 0 {
		v.SetContent("")
		return v
	}
	if m.ctx.Width < 20 || m.ctx.Height < 6 {
		content := lipgloss.NewStyle().
			MaxWidth(m.ctx.Width).
			MaxHeight(m.ctx.Height).
			Foreground(m.ctx.Styles.Palette.FgMuted).
			Render("Terminal too small")
		v.SetContent(content)
		return v
	}

	body := m.active().View()
	body = lipgloss.NewStyle().
		Width(m.ctx.Width).
		Height(bodyHeight(m.ctx.Height)).
		Background(m.ctx.Styles.Palette.BgBase).
		Foreground(m.ctx.Styles.Palette.FgBase).
		Render(body)

	content := header.Render(m.ctx.Styles, m.ctx.Width, m.workspaceLabel(), m.ctx.User) +
		"\n" + body + "\n" +
		statusbar.Render(m.ctx.Styles, m.ctx.Width, m.notice, m.active().Title(), m.active().ShortHelp())

	if len(m.dialogs) > 0 {
		content = m.overlayDialog(content, m.dialogs[len(m.dialogs)-1])
	}

	v.SetContent(content)
	return v
}

func (m Model) workspaceLabel() string {
	if m.ctx.Workspace != nil {
		return m.ctx.Workspace.Key + " · " + m.ctx.Workspace.Name
	}
	return ""
}

// windowTitle is what we set on tea.View.WindowTitle. Terminals that
// support OSC 0 will pick it up.
func (m Model) windowTitle() string {
	if m.ctx.Workspace != nil {
		return "Windshift · " + m.ctx.Workspace.Key
	}
	return "Windshift"
}

// overlayDialog retains the board around the opaque dialog, preserving the
// context of the action instead of replacing the whole screen with a blank
// backdrop.
func (m Model) overlayDialog(content string, d dialog.Dialog) string {
	s := m.ctx.Styles

	width := 40
	if sized, ok := d.(interface{ PreferredWidth() int }); ok {
		width = sized.PreferredWidth()
	}
	availableWidth := max(1, m.ctx.Width-8)
	if width > availableWidth {
		width = availableWidth
	}

	footerText := "↑↓ select · enter confirm · esc cancel"
	if withFooter, ok := d.(interface{ Footer() string }); ok {
		footerText = withFooter.Footer()
	}

	titleLine := s.Dialog.Title.Render(d.Title())
	stacked := titleLine + "\n" + d.View(width, max(1, m.ctx.Height-8))
	if footerText != "" {
		stacked += "\n" + s.Dialog.Footer.Render(footerText)
	}
	frame := s.Dialog.Frame.Render(stacked)
	x := (m.ctx.Width - lipgloss.Width(frame)) / 2
	y := (m.ctx.Height - lipgloss.Height(frame)) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	base := lipgloss.NewLayer(content).X(0).Y(0).Z(0)
	overlay := lipgloss.NewLayer(frame).X(x).Y(y).Z(1)
	return lipgloss.NewCompositor(base, overlay).Render()
}

// bodyHeight subtracts the header (2 rows incl. the rule) and status bar
// (1 row) from the terminal height.
func bodyHeight(termHeight int) int {
	h := termHeight - chromeRows
	if h < 1 {
		return 1
	}
	return h
}
