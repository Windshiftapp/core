package utils

import (
	"html"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Pre-compiled regular expressions for performance
var (
	// Script tag removal regex
	scriptRegex = regexp.MustCompile(`(?i)<script[^>]*>.*?</script>`)

	// Dangerous HTML tags regex
	dangerousRegex = regexp.MustCompile(`(?i)<(script|object|embed|iframe|form|img|svg)[^>]*>`)

	// Email validation regex
	emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

	// Dangerous filename characters regex
	dangerousCharsRegex = regexp.MustCompile(`[<>:"|?*\x00-\x1f]`)

	// All HTML tags regex - matches opening, closing, and self-closing tags
	htmlTagRegex = regexp.MustCompile(`<[^>]*>`)

	// Safe br tag variations (used by Milkdown to preserve blank lines)
	brTagRegex = regexp.MustCompile(`<br\s*/?>`)
)

// SanitizeText removes potentially dangerous HTML/script content and limits length
func SanitizeText(input string, maxLength int) string {
	if input == "" {
		return input
	}
	
	// HTML escape to prevent script injection
	sanitized := html.EscapeString(input)

	// Remove any remaining script tags (belt and suspenders approach)
	sanitized = scriptRegex.ReplaceAllString(sanitized, "")

	// Remove other potentially dangerous tags
	sanitized = dangerousRegex.ReplaceAllString(sanitized, "")
	
	// Trim whitespace
	sanitized = strings.TrimSpace(sanitized)
	
	// Limit length to prevent excessive data
	if maxLength > 0 && utf8.RuneCountInString(sanitized) > maxLength {
		runes := []rune(sanitized)
		sanitized = string(runes[:maxLength])
	}
	
	return sanitized
}

// SanitizeTitle sanitizes titles with appropriate length limits
func SanitizeTitle(title string) string {
	return SanitizeText(title, 200)
}

// StripHTMLTags removes all HTML tags from input string.
// Use this for user-generated content stored as Markdown where HTML tags are not expected.
func StripHTMLTags(input string) string {
	if input == "" {
		return ""
	}
	return htmlTagRegex.ReplaceAllString(input, "")
}

// SanitizeDescription sanitizes descriptions by stripping HTML tags and limiting size.
// Content is stored as Markdown, so any HTML tags are injection attempts.
// Exception: <br /> tags are preserved as they're used by Milkdown to preserve blank lines.
func SanitizeDescription(description string) string {
	if description == "" || description == "null" {
		return ""
	}

	// Preserve <br> tags by replacing with placeholder before stripping HTML
	// These are used by Milkdown's remarkPreserveEmptyLinePlugin for blank lines
	const brPlaceholder = "\x00BR_PLACEHOLDER\x00"
	description = brTagRegex.ReplaceAllString(description, brPlaceholder)

	// Strip any other HTML tags - Markdown content shouldn't have any
	description = StripHTMLTags(description)

	// Restore <br /> tags
	description = strings.ReplaceAll(description, brPlaceholder, "<br />")

	// Limit size to prevent excessive data (10KB should be enough for rich text)
	maxLength := 10000
	if len(description) > maxLength {
		return description[:maxLength]
	}

	return description
}

// SanitizeName sanitizes names (workspace names, field names, etc.)
func SanitizeName(name string) string {
	return SanitizeText(name, 100)
}

// SanitizeJSON sanitizes JSON strings by limiting their size
func SanitizeJSON(jsonStr string) string {
	// Just limit the size for now, proper JSON validation should be done separately
	return SanitizeText(jsonStr, 10000)
}