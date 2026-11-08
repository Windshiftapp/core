// Package i18n provides locale resolution and locale-aware formatting for
// backend-rendered content such as transactional and notification email.
//
// The shipped locale set mirrors frontend/src/lib/stores/i18n.svelte.js
// SUPPORTED_LOCALES. Unknown or missing locales fall back to English.
package i18n

import (
	"sort"
	"strings"

	"golang.org/x/text/language"
)

// Default is the locale used when a requested locale is unknown or empty.
const Default = "en"

// supported lists the shipped locales, derived from the generated date-format
// table so the set has a single source: frontend/src/lib/locales/dateFormats.js,
// which frontend/scripts/check-i18n.js requires for every locale directory.
var supported = supportedLocales()

func supportedLocales() []string {
	locales := make([]string, 0, len(dateFormats))
	for locale := range dateFormats {
		locales = append(locales, locale)
	}
	sort.Strings(locales)
	return locales
}

// Supported returns the shipped locale tags.
func Supported() []string {
	out := make([]string, len(supported))
	copy(out, supported)
	return out
}

// IsSupported reports whether tag resolves to a shipped locale.
func IsSupported(tag string) bool {
	_, ok := match(tag)
	return ok
}

// Normalize resolves a raw locale tag to a shipped locale, falling back to
// Default when nothing matches. Matching is case-insensitive and accepts
// underscore separators (pt_BR). A region-specific tag prefers an exact match,
// then the base language. This mirrors negotiateLocale in
// frontend/src/lib/stores/i18n-utils.js.
func Normalize(tag string) string {
	if locale, ok := match(tag); ok {
		return locale
	}
	return Default
}

// match resolves a raw tag to a shipped locale and reports whether it matched,
// so callers can distinguish a real match from the Default fallback.
func match(tag string) (string, bool) {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(tag), "_", "-"))
	if normalized == "" {
		return "", false
	}
	for _, locale := range supported {
		if strings.EqualFold(locale, normalized) {
			return locale, true
		}
	}
	base := normalized
	if i := strings.IndexByte(base, '-'); i >= 0 {
		base = base[:i]
	}
	for _, locale := range supported {
		if strings.EqualFold(strings.SplitN(locale, "-", 2)[0], base) {
			return locale, true
		}
	}
	return "", false
}

// NegotiateAcceptLanguage picks the best shipped locale from an Accept-Language
// header, honoring the client's quality ordering. It reports ok=false when no
// shipped locale matches, so callers can leave a preference unset rather than
// persisting an unsupported tag.
func NegotiateAcceptLanguage(header string) (string, bool) {
	tags, _, err := language.ParseAcceptLanguage(header)
	if err != nil {
		return "", false
	}
	for _, tag := range tags {
		if locale, ok := match(tag.String()); ok {
			return locale, true
		}
	}
	return "", false
}

// IsRTL reports whether the locale reads right to left.
func IsRTL(locale string) bool {
	return Normalize(locale) == "ar"
}
