// Package template implements the {{var}} placeholder syntax shared
// between asset actions, logbook actions, and portal title templates.
// Unknown variables are left as the literal placeholder so admins notice
// typos at runtime instead of silently producing empty strings.
package template

import "regexp"

var placeholderRE = regexp.MustCompile(`\{\{([^}]+)\}\}`)

// Substitute replaces every {{name}} occurrence in template with vars[name].
// Whitespace around name is trimmed. When the lookup misses, the original
// {{name}} text is preserved.
func Substitute(template string, vars map[string]string) string {
	if template == "" {
		return ""
	}
	return placeholderRE.ReplaceAllStringFunc(template, func(match string) string {
		name := match[2 : len(match)-2]
		// Trim ASCII whitespace; matches strings.TrimSpace behavior for the
		// common case without pulling in the strings dependency.
		start, end := 0, len(name)
		for start < end && isSpace(name[start]) {
			start++
		}
		for end > start && isSpace(name[end-1]) {
			end--
		}
		key := name[start:end]
		if val, ok := vars[key]; ok {
			return val
		}
		return match
	})
}

func isSpace(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r':
		return true
	}
	return false
}
