package i18n

import (
	"strings"
	"time"
)

//go:generate bun ../../scripts/generate-i18n-catalog.mjs

// dateFormat is the numeric layout set for one locale. The layouts are Go
// reference-time strings (https://pkg.go.dev/time#pkg-constants) and never
// contain month or weekday names, so no CLDR symbol tables are required.
type dateFormat struct {
	Date     string
	DateTime string
	Time     string
}

func layoutFor(locale string) dateFormat {
	if format, ok := dateFormats[Normalize(locale)]; ok {
		return format
	}
	return dateFormats[Default]
}

// Location resolves an IANA timezone name. Empty or unknown names fall back to
// UTC so a bad recipient preference never breaks delivery.
func Location(name string) *time.Location {
	name = strings.TrimSpace(name)
	if name == "" {
		return time.UTC
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return loc
}

// ZoneName returns a stable, non-translated label for the zone used to render a
// timestamp, e.g. "UTC" or "America/New_York".
func ZoneName(loc *time.Location) string {
	if loc == nil {
		return time.UTC.String()
	}
	return loc.String()
}

func inZone(t time.Time, loc *time.Location) time.Time {
	if loc == nil {
		return t.In(time.UTC)
	}
	return t.In(loc)
}

// FormatDate renders t in loc using the locale's numeric date layout.
func FormatDate(locale string, t time.Time, loc *time.Location) string {
	return inZone(t, loc).Format(layoutFor(locale).Date)
}

// FormatTime renders t in loc using the locale's numeric time layout.
func FormatTime(locale string, t time.Time, loc *time.Location) string {
	return inZone(t, loc).Format(layoutFor(locale).Time)
}

// FormatDateTime renders t in loc using the locale's combined date/time layout.
func FormatDateTime(locale string, t time.Time, loc *time.Location) string {
	return inZone(t, loc).Format(layoutFor(locale).DateTime)
}

// FormatDateTimeZ renders t in loc and appends the zone label, so a reader in a
// different timezone is never shown an unlabeled time.
func FormatDateTimeZ(locale string, t time.Time, loc *time.Location) string {
	return FormatDateTime(locale, t, loc) + " " + ZoneName(loc)
}
