// Package splash renders the centered wordmark + tagline shown while the very
// first workspace list loads.
package splash

import (
	"charm.land/lipgloss/v2"

	"windshift/internal/tui/styles"
)

// Render centers the wordmark + tagline + a loading line in a width×height box.
func Render(s *styles.Styles, width, height int) string {
	wordmark := styles.ApplyForegroundGrad(
		s.Splash.Wordmark,
		"WINDSHIFT",
		s.Header.GradFrom,
		s.Header.GradTo,
	)
	body := wordmark + "\n" + s.Splash.Tagline.Render("self-hostable work tracking")

	loadLine := s.Splash.Loading.Render("Loading workspaces…")
	stacked := body + "\n\n" + loadLine

	return lipgloss.Place(
		width,
		height,
		lipgloss.Center,
		lipgloss.Center,
		stacked,
	)
}
