package utils

import (
	"html"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/microcosm-cc/bluemonday"
)

// Bluemonday policies (safe for concurrent use after creation)
var (
	// strictPolicy strips ALL HTML tags
	strictPolicy = bluemonday.StrictPolicy()

	// brOnlyPolicy strips all HTML except <br> tags (used by Milkdown for blank lines)
	brOnlyPolicy = func() *bluemonday.Policy {
		p := bluemonday.StrictPolicy()
		p.AllowElements("br")
		return p
	}()
)

// Pre-compiled regular expressions for non-HTML patterns
var (
	// Dangerous URL schemes in Markdown links: [text](javascript:...) or ![alt](data:...)
	// Matches both link and image syntax, case-insensitive scheme names.
	// Handles one level of nested parens in the URL (e.g. alert(1)) before the closing Markdown paren.
	dangerousMarkdownURLRegex = regexp.MustCompile(`(?i)(!?\[[^\]]*\])\(\s*(javascript|vbscript|data)\s*:(?:[^()]*(?:\([^()]*\)[^()]*)*)\)`)
)

// SanitizeText removes potentially dangerous HTML/script content and limits length
func SanitizeText(input string, maxLength int) string {
	if input == "" {
		return input
	}

	// HTML escape to prevent script injection
	sanitized := html.EscapeString(input)

	// Strip any HTML tags that might remain (belt and suspenders after escaping)
	sanitized = strictPolicy.Sanitize(sanitized)

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
	return strictPolicy.Sanitize(input)
}

// SanitizeDescription sanitizes descriptions by stripping HTML tags and limiting size.
// Content is stored as Markdown, so any HTML tags are injection attempts.
// Exception: <br /> tags are preserved as they're used by Milkdown to preserve blank lines.
func SanitizeDescription(description string) string {
	if description == "" || description == "null" {
		return ""
	}

	// Use brOnlyPolicy to strip all HTML except <br> tags
	description = brOnlyPolicy.Sanitize(description)

	// Normalize <br/> (bluemonday output) back to <br /> for Milkdown compatibility
	description = strings.ReplaceAll(description, "<br/>", "<br />")

	// Sanitize dangerous Markdown URLs (javascript:, vbscript:, data: schemes)
	description = SanitizeMarkdownURLs(description)

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

// SanitizeMarkdownURLs replaces dangerous URL schemes in Markdown link/image syntax.
// This catches XSS vectors like [Click me](javascript:alert(1)) and ![img](data:text/html,<script>...)
// that survive HTML tag stripping because they contain no HTML tags.
func SanitizeMarkdownURLs(input string) string {
	if input == "" {
		return ""
	}
	return dangerousMarkdownURLRegex.ReplaceAllString(input, "${1}(#unsafe-link-removed)")
}

// SanitizeCommentContent sanitizes user-submitted comment content.
// It chains HTML tag stripping (for injected HTML) with Markdown URL sanitization
// (for javascript:/vbscript:/data: links that would be rendered by the Markdown editor).
func SanitizeCommentContent(input string) string {
	if input == "" {
		return ""
	}
	return SanitizeMarkdownURLs(StripHTMLTags(input))
}

// SanitizeJSON sanitizes JSON strings by limiting their size
func SanitizeJSON(jsonStr string) string {
	// Just limit the size for now, proper JSON validation should be done separately
	return SanitizeText(jsonStr, 10000)
}
