// Package dialog defines a small overlay-dialog interface used by the TUI.
//
// Each open dialog is one entry on Model.dialogs []Dialog. The top of the
// stack receives key events; everything paints from bottom up so the most
// recent dialog appears on top. A dialog signals dismissal (and optional
// follow-up Cmd / selection) via Action.
package dialog

import (
	tea "charm.land/bubbletea/v2"

	"windshift/internal/tui/styles"
)

// Dialog is the contract every overlay implements.
type Dialog interface {
	// ID is a stable identifier used to find / replace a dialog on the
	// stack. Not displayed.
	ID() string
	// HandleKey processes a key press while this dialog is on top. Returning
	// Action.Close pops it.
	HandleKey(msg tea.KeyPressMsg) Action
	// View renders the dialog body — the framing (border, padding,
	// centering) is added by the caller via the styles.Dialog.Frame style.
	View(width, height int) string
	// Title returns the title shown above the body.
	Title() string
}

// ThemeAware is implemented by dialogs that retain component styles. Open
// dialogs receive theme changes immediately instead of becoming a patchwork
// of the previous and current palettes.
type ThemeAware interface {
	OnThemeChanged(*styles.Styles)
}

// ResultHandler lets a dialog consume the result of a child dialog. This is
// used by forms with picker-backed fields: the picker closes back into the
// still-open form instead of sending its selection to the screen behind it.
type ResultHandler interface {
	HandleResult(ResultMsg) tea.Cmd
}

// MessageHandler receives non-key Bubble Tea messages while the dialog is on
// top. Forms use it for cursor blink and other retained component lifecycle
// messages.
type MessageHandler interface {
	HandleMessage(tea.Msg) tea.Cmd
}

// Action is the result of a key press. Selected is type-asserted by the
// caller based on which dialog it opened.
type Action struct {
	Close    bool
	Selected any
	Cmd      tea.Cmd
}

// OpenMsg asks the root model to push Dialog onto the overlay stack.
type OpenMsg struct{ Dialog Dialog }

// Open returns a command that opens d.
func Open(d Dialog) tea.Cmd {
	return func() tea.Msg { return OpenMsg{Dialog: d} }
}

// ResultMsg is forwarded to the active screen when a dialog closes with a
// selection. ID is the dialog's ID; Value is what the dialog selected. The
// screen that opened the dialog is by construction the active screen when it
// closes, so it consumes its own result.
type ResultMsg struct {
	ID    string
	Value any
}
