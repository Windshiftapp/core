// Package inputs holds textinput construction and form-row rendering helpers
// shared by every form screen.
package inputs

import (
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"windshift/internal/tui/styles"
)

// New builds a styled single-line textinput configured for our forms.
// We override the default prompt ("> ") to be empty since the form labels
// already announce the field.
func New(s *styles.Styles, placeholder string, charLimit int) textinput.Model {
	in := textinput.New()
	in.Prompt = ""
	in.Placeholder = placeholder
	in.CharLimit = charLimit
	in.SetStyles(Styles(s))
	return in
}

// TextareaStyles keeps multiline editors in the same palette as the rest of
// the form. The bubbles defaults otherwise leak their own blue/grey theme into
// edit and comment dialogs.
func TextareaStyles(s *styles.Styles) textarea.Styles {
	st := textarea.DefaultDarkStyles()
	st.Focused.Base = lipgloss.NewStyle().Foreground(s.Palette.FgBase).Background(s.Palette.BgSurface)
	st.Focused.Text = lipgloss.NewStyle().Foreground(s.Palette.FgBase)
	st.Focused.Placeholder = lipgloss.NewStyle().Foreground(s.Palette.FgMuted)
	st.Focused.CursorLine = lipgloss.NewStyle().Background(s.Palette.BgSurface)
	st.Focused.EndOfBuffer = lipgloss.NewStyle().Foreground(s.Palette.BgSurface)
	st.Focused.Prompt = lipgloss.NewStyle().Foreground(s.Palette.Primary)
	st.Blurred.Base = lipgloss.NewStyle().Foreground(s.Palette.FgBase).Background(s.Palette.BgSurface)
	st.Blurred.Text = lipgloss.NewStyle().Foreground(s.Palette.FgBase)
	st.Blurred.Placeholder = lipgloss.NewStyle().Foreground(s.Palette.FgMuted)
	st.Blurred.CursorLine = lipgloss.NewStyle().Background(s.Palette.BgSurface)
	st.Blurred.EndOfBuffer = lipgloss.NewStyle().Foreground(s.Palette.BgSurface)
	st.Blurred.Prompt = lipgloss.NewStyle().Foreground(s.Palette.FgMuted)
	st.Cursor.Color = s.Palette.Primary
	st.Cursor.Shape = tea.CursorBar
	st.Cursor.Blink = true
	return st
}

// Styles configures textinput.Styles for both focused and blurred states.
// The cursor uses the theme accent; the frame marks the active field.
func Styles(s *styles.Styles) textinput.Styles {
	style := textinput.Styles{}
	style.Focused.Text = lipgloss.NewStyle().Foreground(s.Palette.FgBase)
	style.Focused.Placeholder = lipgloss.NewStyle().Foreground(s.Palette.FgMuted)
	style.Focused.Suggestion = lipgloss.NewStyle().Foreground(s.Palette.FgMuted)
	style.Focused.Prompt = lipgloss.NewStyle()

	style.Blurred.Text = lipgloss.NewStyle().Foreground(s.Palette.FgBase)
	style.Blurred.Placeholder = lipgloss.NewStyle().Foreground(s.Palette.FgMuted)
	style.Blurred.Suggestion = lipgloss.NewStyle().Foreground(s.Palette.FgMuted)
	style.Blurred.Prompt = lipgloss.NewStyle()

	style.Cursor.Color = s.Palette.Primary
	style.Cursor.Shape = tea.CursorBlock
	style.Cursor.Blink = true
	return style
}

// Render wraps a textinput's view in the form input frame. When the field is
// "selected but not editing" we still show the focus border so the
// user knows which row is active; the cursor disappears because
// textinput.Blur was called.
func Render(s *styles.Styles, in textinput.Model, selected, editing bool, width int) string {
	view := in.View()
	frame := s.Form.Input
	if editing || selected {
		frame = s.Form.InputFocused
	}
	if width > 0 {
		frame = frame.Width(width)
	}
	return frame.Render(view)
}

// RenderPickerCell draws a chip/string inside the form input frame (focused
// styling when selected). Used for the status/priority/project rows where
// the value comes from a picker dialog rather than an inline input.
func RenderPickerCell(s *styles.Styles, label string, selected bool) string {
	frame := s.Form.Input
	if selected {
		frame = s.Form.InputFocused
	}
	return frame.Render(label + "  " + lipgloss.NewStyle().Foreground(s.Palette.FgMuted).Render("[enter to change]"))
}

// Width picks a reasonable form-field width given the terminal width.
func Width(winW int) int {
	w := winW - 8
	if w < 1 {
		w = 1
	}
	if w > 80 {
		w = 80
	}
	return w
}
