package core

import (
	"sync/atomic"
	"time"

	"windshift/internal/tui/data"
	"windshift/internal/tui/styles"
)

// Ctx is the shared per-SSH-session context. One *Ctx is constructed per
// connection and handed to every screen, dialog and component; screens read
// Styles at render time so a theme switch (which replaces Styles wholesale)
// propagates without plumbing.
type Ctx struct {
	// Styles is replaced atomically on theme switch. Read it at render
	// time; never copy it into retained state without implementing
	// ThemeAware.
	Styles *styles.Styles
	// Theme is the active theme name.
	Theme string

	Client            *data.Client
	User              *data.UserInfo
	UserTimezoneKnown bool
	Keys              KeyMap

	// Prefs is the canonical current preference state. Mutation sites
	// (theme cycle, split adjust, workspace entry) update it and fire
	// data.SavePrefs; the app seeds it from PrefsLoadedMsg on connect.
	Prefs data.Prefs

	// Workspace is the workspace the user has entered, nil while on the
	// workspace picker. Set by the workspaces screen on selection.
	Workspace *data.Workspace

	// Width and Height are the full terminal dimensions. Screens receive
	// their body-region size via SetSize; these are for overlays that
	// center against the whole terminal.
	Width  int
	Height int

	Clock      func() time.Time
	requestSeq atomic.Uint64
}

func (c *Ctx) NextRequestID() uint64 { return c.requestSeq.Add(1) }

func (c *Ctx) CurrentTime() time.Time {
	if c.Clock != nil {
		return c.Clock()
	}
	return time.Now()
}
