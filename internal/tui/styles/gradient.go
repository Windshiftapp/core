package styles

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// ApplyForegroundGrad colors each visible rune of s with a linearly
// interpolated color between from and to. Whitespace and ANSI sequences are
// passed through. Good for short labels (splash wordmark, header) — for body
// text the per-rune render cost adds up.
func ApplyForegroundGrad(base lipgloss.Style, s string, from, to color.Color) string {
	runes := []rune(s)
	visible := 0
	for _, r := range runes {
		if r != ' ' {
			visible++
		}
	}
	if visible == 0 {
		return s
	}

	fr, fg, fb, _ := from.RGBA()
	tr, tg, tb, _ := to.RGBA()

	var out strings.Builder
	idx := 0
	for _, r := range runes {
		if r == ' ' {
			out.WriteRune(r)
			continue
		}
		t := 0.0
		if visible > 1 {
			t = float64(idx) / float64(visible-1)
		}
		c := color.RGBA{
			R: uint8((float64(fr) + (float64(tr)-float64(fr))*t) / 257),
			G: uint8((float64(fg) + (float64(tg)-float64(fg))*t) / 257),
			B: uint8((float64(fb) + (float64(tb)-float64(fb))*t) / 257),
			A: 255,
		}
		out.WriteString(base.Foreground(c).Render(string(r)))
		idx++
	}
	return out.String()
}
