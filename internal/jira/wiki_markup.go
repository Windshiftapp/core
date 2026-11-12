package jira

import (
	"regexp"
	"strings"
)

// wiki_markup.go converts Jira Server/Data Center wiki markup to Markdown.
//
// Data Center's REST API returns rich-text fields as wiki markup strings while
// Cloud returns ADF documents. ConvertADFToMarkdown only understands ADF, so
// the importer previously stored wiki markup verbatim. This converter covers
// the constructs the importer can render faithfully and degrades anything it
// does not recognize to readable text rather than dropping it.

var (
	wikiHeadingPattern = regexp.MustCompile(`^h([1-6])\.\s*(.*)$`)
	wikiRulePattern    = regexp.MustCompile(`^-{4,}$`)
	wikiBulletPattern  = regexp.MustCompile(`^(\*+)\s+(.*)$`)
	wikiOrderedPattern = regexp.MustCompile(`^(#+)\s+(.*)$`)
	wikiBlockOpen      = regexp.MustCompile(`^\{(code|noformat|quote|panel|note|warning|info|tip)(?::([^}]*))?\}$`)

	wikiMonospacePattern = regexp.MustCompile(`\{\{([^{}]+)\}\}`)
	wikiBoldPattern      = regexp.MustCompile(`\*([^\s*](?:[^*]*[^\s*])?)\*`)
	wikiItalicPattern    = regexp.MustCompile(`_([^\s_](?:[^_]*[^\s_])?)_`)
	wikiStrikePattern    = regexp.MustCompile(`(^|[\s(\[])-([^\s-](?:[^-]*[^\s-])?)-([\s)\].,;:!?]|$)`)
	wikiNamedLinkPattern = regexp.MustCompile(`\[([^|\]\n]+)\|([^\]\n]+)\]`)
	wikiURLLinkPattern   = regexp.MustCompile(`\[((?:https?|ftp)://[^\]\n]+)\]`)
	wikiMentionPattern   = regexp.MustCompile(`\[~([^\]\n]+)\]`)
	// Data Center wiki bodies name attachments instead of carrying ADF media
	// ids: `!image.png!` embeds one and `[^file.pdf]` links it.
	wikiInlineMediaPattern    = regexp.MustCompile(`!([^!\n|]+)(?:\|[^!\n]*)?!`)
	wikiAttachmentLinkPattern = regexp.MustCompile(`\[\^([^\]\n|]+)(?:\|[^\]\n]*)?\]`)
	wikiImagePattern          = regexp.MustCompile(`!([^!\n]+)!`)
	wikiUnderlinePattern      = regexp.MustCompile(`(^|[\s(\[])\+([^\s+](?:[^+]*[^\s+])?)\+([\s)\].,;:!?]|$)`)
	wikiCitationPattern       = regexp.MustCompile(`(^|[\s(\[])\?\?([^\s?](?:[^?]*[^\s?])?)\?\?([\s)\].,;:!?]|$)`)
	wikiSuperscriptPattern    = regexp.MustCompile(`(^|[\s(\[])\^([^\s^](?:[^^]*[^\s^])?)\^([\s)\].,;:!?]|$)`)
	wikiSubscriptPattern      = regexp.MustCompile(`(^|[\s(\[])~([^\s~](?:[^~]*[^\s~])?)~([\s)\].,;:!?]|$)`)
)

// ConvertWikiMarkupToMarkdown converts Jira wiki markup to Markdown. The
// mention resolver is not used for wiki markup because Data Center mentions
// carry usernames directly (`[~username]`); the media resolver is consulted
// with the attachment filename so callers that index attachments by filename
// get a real download link, otherwise the image keeps a filename reference.
func ConvertWikiMarkupToMarkdown(value string, mediaResolver MediaResolver) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	var out strings.Builder

	for i := 0; i < len(lines); {
		trimmed := strings.TrimSpace(lines[i])

		if macro := wikiBlockOpen.FindStringSubmatch(trimmed); macro != nil {
			name := macro[1]
			if name == "code" || name == "noformat" {
				language := ""
				if name == "code" {
					language = strings.TrimSpace(macro[2])
				}
				body, next := wikiCollectBlock(lines, i+1, "{"+name+"}")
				out.WriteString("```" + language + "\n")
				out.WriteString(strings.Join(body, "\n"))
				out.WriteString("\n```\n\n")
				i = next
				continue
			}
			body, next := wikiCollectBlock(lines, i+1, "{"+name+"}")
			inner := ConvertWikiMarkupToMarkdown(strings.Join(body, "\n"), mediaResolver)
			label := ""
			switch name {
			case "note":
				label = "**Note:** "
			case "warning":
				label = "**Warning:** "
			case "info":
				label = "**Info:** "
			case "tip":
				label = "**Tip:** "
			}
			first := true
			for _, blockLine := range strings.Split(inner, "\n") {
				prefix := "> "
				if first && label != "" {
					prefix = "> " + label
				}
				out.WriteString(prefix + blockLine + "\n")
				first = false
			}
			out.WriteString("\n")
			i = next
			continue
		}

		if heading := wikiHeadingPattern.FindStringSubmatch(trimmed); heading != nil {
			level := int(heading[1][0] - '0')
			out.WriteString(strings.Repeat("#", level) + " " + convertWikiInline(heading[2], mediaResolver) + "\n\n")
			i++
			continue
		}

		if wikiRulePattern.MatchString(trimmed) {
			out.WriteString("---\n\n")
			i++
			continue
		}

		if strings.HasPrefix(trimmed, "||") || (strings.HasPrefix(trimmed, "|") && strings.HasSuffix(trimmed, "|")) {
			table, next := wikiConvertTable(lines, i, mediaResolver)
			out.WriteString(table)
			i = next
			continue
		}

		if item := wikiBulletPattern.FindStringSubmatch(trimmed); item != nil {
			depth := len(item[1]) - 1
			out.WriteString(strings.Repeat("  ", depth) + "- " + convertWikiInline(item[2], mediaResolver) + "\n")
			i++
			continue
		}

		if item := wikiOrderedPattern.FindStringSubmatch(trimmed); item != nil {
			depth := len(item[1]) - 1
			out.WriteString(strings.Repeat("  ", depth) + "1. " + convertWikiInline(item[2], mediaResolver) + "\n")
			i++
			continue
		}

		if trimmed == "" {
			out.WriteString("\n")
			i++
			continue
		}

		out.WriteString(convertWikiInline(trimmed, mediaResolver) + "\n")
		i++
	}

	return strings.TrimSpace(collapseWikiBlankLines(out.String()))
}

// WikiReferencedFilenames returns the attachment filenames a Jira wiki body
// references, so Data Center attachments can be classified without ADF media
// ids. Names are reported verbatim; callers decide whether one is unambiguous.
func WikiReferencedFilenames(body string) []string {
	if strings.TrimSpace(body) == "" {
		return nil
	}
	seen := make(map[string]struct{})
	var names []string
	for _, pattern := range []*regexp.Regexp{wikiInlineMediaPattern, wikiAttachmentLinkPattern} {
		for _, match := range pattern.FindAllStringSubmatch(body, -1) {
			name := strings.TrimSpace(match[1])
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			names = append(names, name)
		}
	}
	return names
}

// wikiCollectBlock gathers lines until the closing macro marker and returns
// the body plus the index just past the marker.
func wikiCollectBlock(lines []string, start int, closing string) (collected []string, next int) {
	for i := start; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == closing {
			return collected, i + 1
		}
		collected = append(collected, lines[i])
	}
	return collected, len(lines)
}

// wikiConvertTable renders consecutive `||header||` / `|cell|` lines as a
// Markdown table and returns the text plus the index after the table.
func wikiConvertTable(lines []string, start int, mediaResolver MediaResolver) (rendered string, next int) {
	var out strings.Builder
	headerDone := false
	i := start
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i])
		isHeader := strings.HasPrefix(trimmed, "||")
		if !isHeader && (!strings.HasPrefix(trimmed, "|") || !strings.HasSuffix(trimmed, "|")) {
			break
		}
		var cells []string
		if isHeader {
			for _, cell := range strings.Split(strings.Trim(trimmed, "|"), "||") {
				cells = append(cells, convertWikiInline(strings.TrimSpace(cell), mediaResolver))
			}
		} else {
			for _, cell := range strings.Split(strings.Trim(trimmed, "|"), "|") {
				cells = append(cells, convertWikiInline(strings.TrimSpace(cell), mediaResolver))
			}
		}
		out.WriteString("| " + strings.Join(cells, " | ") + " |\n")
		if isHeader && !headerDone {
			separators := make([]string, len(cells))
			for index := range separators {
				separators[index] = "---"
			}
			out.WriteString("| " + strings.Join(separators, " | ") + " |\n")
			headerDone = true
		}
		i++
	}
	out.WriteString("\n")
	return out.String(), i
}

// convertWikiInline converts inline wiki markup within a single line.
func convertWikiInline(value string, mediaResolver MediaResolver) string {
	if value == "" {
		return ""
	}
	result := wikiMonospacePattern.ReplaceAllString(value, "`$1`")
	result = wikiImagePattern.ReplaceAllStringFunc(result, func(match string) string {
		filename := wikiMediaFilename(strings.Trim(match, "!"))
		if mediaResolver != nil {
			if resolution := mediaResolver(filename); resolution.Resolved && resolution.Markdown != "" {
				return resolution.Markdown
			}
		}
		return "![" + filename + "](" + filename + ")"
	})
	// `[^file.pdf]` links an attachment rather than embedding it.
	result = wikiAttachmentLinkPattern.ReplaceAllStringFunc(result, func(match string) string {
		filename := wikiMediaFilename(strings.TrimSuffix(strings.TrimPrefix(match, "[^"), "]"))
		if mediaResolver != nil {
			if resolution := mediaResolver(filename); resolution.Resolved && resolution.Markdown != "" {
				return resolution.Markdown
			}
		}
		return "[" + filename + "](" + filename + ")"
	})
	result = wikiNamedLinkPattern.ReplaceAllString(result, "[$1]($2)")
	result = wikiURLLinkPattern.ReplaceAllString(result, "<$1>")
	result = wikiMentionPattern.ReplaceAllString(result, "@$1")
	result = wikiBoldPattern.ReplaceAllString(result, "**$1**")
	result = wikiItalicPattern.ReplaceAllString(result, "*$1*")
	result = wikiStrikePattern.ReplaceAllString(result, "$1~~$2~~$3")
	result = wikiUnderlinePattern.ReplaceAllString(result, "$1<u>$2</u>$3")
	result = wikiCitationPattern.ReplaceAllString(result, "$1<cite>$2</cite>$3")
	result = wikiSuperscriptPattern.ReplaceAllString(result, "$1<sup>$2</sup>$3")
	result = wikiSubscriptPattern.ReplaceAllString(result, "$1<sub>$2</sub>$3")
	return result
}

// wikiMediaFilename strips the optional `|params` suffix Jira wiki media
// references accept (`!chart.png|width=200!`, `[^guide.pdf|thumbnail]`).
func wikiMediaFilename(value string) string {
	if index := strings.Index(value, "|"); index >= 0 {
		value = value[:index]
	}
	return strings.TrimSpace(value)
}

func collapseWikiBlankLines(value string) string {
	for strings.Contains(value, "\n\n\n") {
		value = strings.ReplaceAll(value, "\n\n\n", "\n\n")
	}
	return value
}
