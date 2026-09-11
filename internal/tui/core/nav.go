package core

import tea "charm.land/bubbletea/v2"

// Navigation messages. Screens never touch the stack directly — they return
// one of these from Update and the root performs the mutation (sizing and
// Init-ing incoming screens).

// PushMsg pushes Screen onto the navigation stack.
type PushMsg struct{ Screen Screen }

// PopMsg pops the top screen. Ignored when only one screen remains.
type PopMsg struct{}

// ReplaceMsg swaps the top screen for Screen.
type ReplaceMsg struct{ Screen Screen }

// Push returns a command that pushes s.
func Push(s Screen) tea.Cmd {
	return func() tea.Msg { return PushMsg{Screen: s} }
}

// Pop returns a command that pops the top screen.
func Pop() tea.Cmd {
	return func() tea.Msg { return PopMsg{} }
}

// Replace returns a command that replaces the top screen with s.
func Replace(s Screen) tea.Cmd {
	return func() tea.Msg { return ReplaceMsg{Screen: s} }
}

// NoticeKind classifies a transient status-bar notice.
type NoticeKind int

const (
	NoticeNone NoticeKind = iota
	NoticeSuccess
	NoticeError
)

// NoticeMsg sets the transient status-bar notice. Success notices clear on
// the next key press; error notices clear on the next success or explicit
// NoticeNone.
type NoticeMsg struct {
	Kind NoticeKind
	Text string
}

// NotifySuccess returns a command that shows a success notice.
func NotifySuccess(text string) tea.Cmd {
	return func() tea.Msg { return NoticeMsg{Kind: NoticeSuccess, Text: text} }
}

// NotifyError returns a command that shows an error notice.
func NotifyError(text string) tea.Cmd {
	return func() tea.Msg { return NoticeMsg{Kind: NoticeError, Text: text} }
}

// PrefsChangeMsg applies one preference field. The root merges changes with a
// late initial load and serializes the resulting full-snapshot writes.
type PrefsChangeMsg struct {
	Theme              string
	SetTheme           bool
	SplitRatio         float64
	SetSplitRatio      bool
	LastWorkspaceID    int
	SetLastWorkspaceID bool
}

func SaveTheme(theme string) tea.Cmd {
	return func() tea.Msg { return PrefsChangeMsg{Theme: theme, SetTheme: true} }
}

func SaveSplitRatio(ratio float64) tea.Cmd {
	return func() tea.Msg { return PrefsChangeMsg{SplitRatio: ratio, SetSplitRatio: true} }
}

func SaveLastWorkspace(id int) tea.Cmd {
	return func() tea.Msg { return PrefsChangeMsg{LastWorkspaceID: id, SetLastWorkspaceID: true} }
}
