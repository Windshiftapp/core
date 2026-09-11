package styles

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Styles is the per-component style sheet derived from a Palette. Components
// hold a *Styles and read whichever sub-struct they need; nothing inside the
// renderers reaches back into the Palette directly except through the
// Palette field below (for cases like per-row chip backgrounds that mix a
// brand color with a category color from the API).
type Styles struct {
	Palette Palette
	Base    BaseStyles
	Header  HeaderStyles
	Status  StatusStyles
	List    ListStyles
	Form    FormStyles
	Chip    ChipStyles
	Dialog  DialogStyles
	Splash  SplashStyles
}

type BaseStyles struct {
	Background lipgloss.Style
	Body       lipgloss.Style
	Hint       lipgloss.Style
	Muted      lipgloss.Style
	Heading    lipgloss.Style
}

type HeaderStyles struct {
	Bar        lipgloss.Style
	Logo       lipgloss.Style
	Workspace  lipgloss.Style
	User       lipgloss.Style
	Divider    lipgloss.Style
	GradFrom   color.Color
	GradTo     color.Color
	BottomEdge lipgloss.Style
}

type StatusStyles struct {
	Bar      lipgloss.Style
	Notice   lipgloss.Style
	Error    lipgloss.Style
	Success  lipgloss.Style
	Warning  lipgloss.Style
	Info     lipgloss.Style
	Hint     lipgloss.Style
	KeyChord lipgloss.Style
	KeyLabel lipgloss.Style
}

type ListStyles struct {
	Item         lipgloss.Style
	ItemSelected lipgloss.Style
	SelBar       lipgloss.Style // left-edge bar on selected row
	Muted        lipgloss.Style
	Counter      lipgloss.Style
	Empty        lipgloss.Style
	Header       lipgloss.Style // table column header
	Rule         lipgloss.Style // horizontal rule under header
}

type FormStyles struct {
	Label        lipgloss.Style
	Section      lipgloss.Style
	Input        lipgloss.Style
	InputFocused lipgloss.Style
	Hint         lipgloss.Style
	Error        lipgloss.Style
}

type ChipStyles struct {
	Base lipgloss.Style // foreground/background applied per-render w/ category color
	Dot  lipgloss.Style // small leading dot
}

type DialogStyles struct {
	Frame    lipgloss.Style
	Title    lipgloss.Style
	Body     lipgloss.Style
	Footer   lipgloss.Style
	Backdrop lipgloss.Style
}

type SplashStyles struct {
	Logo     lipgloss.Style
	Wordmark lipgloss.Style
	Tagline  lipgloss.Style
	Loading  lipgloss.Style
}

// New builds a Styles from the given Palette.
func New(p Palette) *Styles {
	s := &Styles{Palette: p}

	s.Base = BaseStyles{
		Background: lipgloss.NewStyle().Background(p.BgBase),
		Body:       lipgloss.NewStyle().Foreground(p.FgBase),
		Hint:       lipgloss.NewStyle().Foreground(p.FgMuted),
		Muted:      lipgloss.NewStyle().Foreground(p.FgSubtle),
		Heading:    lipgloss.NewStyle().Foreground(p.FgBase).Bold(true),
	}

	s.Header = HeaderStyles{
		Bar:        lipgloss.NewStyle().Background(p.BgSurface).Foreground(p.FgBase),
		Logo:       lipgloss.NewStyle().Bold(true),
		Workspace:  lipgloss.NewStyle().Foreground(p.FgBase).Bold(true),
		User:       lipgloss.NewStyle().Foreground(p.FgSubtle),
		Divider:    lipgloss.NewStyle().Foreground(p.BorderSubtle),
		GradFrom:   p.GradFrom,
		GradTo:     p.GradTo,
		BottomEdge: lipgloss.NewStyle().Foreground(p.Border),
	}

	s.Status = StatusStyles{
		Bar:      lipgloss.NewStyle().Background(p.BgSurface).Foreground(p.FgSubtle).Padding(0, 1),
		Notice:   lipgloss.NewStyle().Foreground(p.FgBase),
		Error:    lipgloss.NewStyle().Foreground(p.Danger).Bold(true),
		Success:  lipgloss.NewStyle().Foreground(p.Success).Bold(true),
		Warning:  lipgloss.NewStyle().Foreground(p.Warning).Bold(true),
		Info:     lipgloss.NewStyle().Foreground(p.Info).Bold(true),
		Hint:     lipgloss.NewStyle().Foreground(p.FgMuted),
		KeyChord: lipgloss.NewStyle().Foreground(p.FgBase).Bold(true),
		KeyLabel: lipgloss.NewStyle().Foreground(p.FgSubtle),
	}

	s.List = ListStyles{
		Item: lipgloss.NewStyle().Foreground(p.FgBase).Padding(0, 1),
		ItemSelected: lipgloss.NewStyle().
			Foreground(p.OnPrimary).
			Background(p.Selected).
			Padding(0, 1),
		SelBar:  lipgloss.NewStyle().Foreground(p.Primary).Bold(true),
		Muted:   lipgloss.NewStyle().Foreground(p.FgMuted),
		Counter: lipgloss.NewStyle().Foreground(p.FgSubtle),
		Empty:   lipgloss.NewStyle().Foreground(p.FgMuted).Italic(true),
		Header:  lipgloss.NewStyle().Foreground(p.FgSubtle).Bold(true).Padding(0, 1),
		Rule:    lipgloss.NewStyle().Foreground(p.BorderSubtle),
	}

	s.Form = FormStyles{
		Label:   lipgloss.NewStyle().Foreground(p.FgSubtle).Bold(true),
		Section: lipgloss.NewStyle().Foreground(p.FgMuted).Bold(true),
		Input: lipgloss.NewStyle().
			Foreground(p.FgBase).
			Background(p.BgSurface).
			Padding(0, 1),
		InputFocused: lipgloss.NewStyle().
			Foreground(p.FgBase).
			Background(p.BgSurface).
			Padding(0, 1).
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(p.BorderFocus),
		Hint:  lipgloss.NewStyle().Foreground(p.FgMuted),
		Error: lipgloss.NewStyle().Foreground(p.Danger),
	}

	s.Chip = ChipStyles{
		Base: lipgloss.NewStyle().Bold(true).Padding(0, 1),
		Dot:  lipgloss.NewStyle().Bold(true),
	}

	s.Dialog = DialogStyles{
		Frame: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(p.Border).
			Background(p.BgOverlay).
			Foreground(p.FgBase).
			Padding(0, 1),
		Title:  lipgloss.NewStyle().Foreground(p.FgBase).Bold(true),
		Body:   lipgloss.NewStyle().Foreground(p.FgBase),
		Footer: lipgloss.NewStyle().Foreground(p.FgMuted).MarginTop(1),
		Backdrop: lipgloss.NewStyle().
			Foreground(p.BgBase).
			Background(p.BgBase),
	}

	s.Splash = SplashStyles{
		Logo:     lipgloss.NewStyle().Bold(true),
		Wordmark: lipgloss.NewStyle().Foreground(p.FgBase).Bold(true),
		Tagline:  lipgloss.NewStyle().Foreground(p.FgSubtle).Italic(true),
		Loading:  lipgloss.NewStyle().Foreground(p.FgMuted),
	}

	return s
}
