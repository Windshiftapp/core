package i18n

import (
	"fmt"
	"strings"
	"time"
)

// Translate renders the catalog value for key in locale and substitutes
// {name} placeholders from params. Fallback order is exact locale, base
// language, then Default. A key with no translation is returned verbatim so a
// missing string is visible instead of silently empty.
func Translate(locale, key string, params map[string]any) string {
	value, ok := lookup(locale, key)
	if !ok {
		return key
	}
	if len(params) == 0 {
		return value
	}
	return interpolate(value, params)
}

// TranslateOK is Translate and reports whether a translation existed, so
// callers can fall back to stored copy instead of showing a raw key.
func TranslateOK(locale, key string, params map[string]any) (string, bool) {
	value, ok := lookup(locale, key)
	if !ok {
		return "", false
	}
	if len(params) == 0 {
		return value, true
	}
	return interpolate(value, params), true
}

func lookup(locale, key string) (string, bool) {
	normalized := Normalize(locale)
	if entries, ok := catalog[normalized]; ok {
		if value, ok := entries[key]; ok {
			return value, true
		}
	}

	// Region-specific tag with a sparse table falls back to its base language
	// before English.
	base := strings.ToLower(strings.SplitN(normalized, "-", 2)[0])
	for tag, entries := range catalog {
		if !strings.EqualFold(strings.SplitN(tag, "-", 2)[0], base) {
			continue
		}
		if value, ok := entries[key]; ok {
			return value, true
		}
	}

	if normalized != Default {
		if value, ok := catalog[Default][key]; ok {
			return value, true
		}
	}
	return "", false
}

func interpolate(value string, params map[string]any) string {
	var out strings.Builder
	for {
		start := strings.IndexByte(value, '{')
		if start < 0 {
			out.WriteString(value)
			return out.String()
		}
		end := strings.IndexByte(value[start:], '}')
		if end < 0 {
			out.WriteString(value)
			return out.String()
		}
		out.WriteString(value[:start])
		name := value[start+1 : start+end]
		if param, ok := params[name]; ok {
			fmt.Fprint(&out, param)
		} else {
			out.WriteString(value[start : start+end+1])
		}
		value = value[start+end+1:]
	}
}

// HumanizeDuration renders an expiry window in the locale's words: "30
// minutes", "24 hours", "7 days". Non-round durations fall back to the Go
// duration string.
func HumanizeDuration(locale string, d time.Duration) string {
	switch {
	case d >= 24*time.Hour && d%(24*time.Hour) == 0:
		return durationUnit(locale, int(d/(24*time.Hour)), "day", "days")
	case d >= time.Hour && d%time.Hour == 0:
		return durationUnit(locale, int(d/time.Hour), "hour", "hours")
	case d >= time.Minute && d%time.Minute == 0:
		return durationUnit(locale, int(d/time.Minute), "minute", "minutes")
	default:
		return d.String()
	}
}

func durationUnit(locale string, count int, singular, plural string) string {
	key := "email.duration." + plural
	if count == 1 {
		key = "email.duration." + singular
	}
	return Translate(locale, key, map[string]any{"count": count})
}
