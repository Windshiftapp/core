package planeimport

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Description conversion: Plane stores issue bodies as a TipTap (ProseMirror)
// JSON document. Windshift stores item descriptions as Markdown and renders
// HTML on read (internal/markdown). This converter turns the source document
// into the Markdown Windshift stores; the fallback path renders from Plane's
// own HTML so a node shape the converter does not know degrades to the source
// render instead of to nothing.

// tiptapNode is one ProseMirror node. Marks, attrs, and content are kept
// generic: the converter dispatches on Type and reads only the attrs each
// node type documents.
type tiptapNode struct {
	Type    string         `json:"type"`
	Attrs   map[string]any `json:"attrs,omitempty"`
	Marks   []tiptapMark   `json:"marks,omitempty"`
	Content []tiptapNode   `json:"content,omitempty"`
	Text    string         `json:"text,omitempty"`
}

type tiptapMark struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs,omitempty"`
}

// ConvertDescriptionJSON renders a TipTap document to Windshift Markdown.
// A document that fails to parse returns ok=false so callers fall back to
// Plane's HTML render; callers that see ok=false must report the fallback.
func ConvertDescriptionJSON(docJSON string) (markdown string, ok bool) {
	docJSON = strings.TrimSpace(docJSON)
	if docJSON == "" {
		return "", true
	}
	// Plane's DB also stores the document double-encoded as a JSON string in
	// some export paths; unwrap one level when the payload starts as a string.
	if strings.HasPrefix(docJSON, `"`) {
		var inner string
		if err := json.Unmarshal([]byte(docJSON), &inner); err == nil {
			docJSON = inner
		}
	}
	var doc tiptapNode
	if err := json.Unmarshal([]byte(docJSON), &doc); err != nil || doc.Type != "doc" {
		return "", false
	}
	var b strings.Builder
	for i, block := range doc.Content {
		rendered, ok := renderBlock(block, 0)
		if !ok {
			return "", false
		}
		if rendered == "" && block.Type != "paragraph" {
			continue // unknown empty block: nothing to emit
		}
		if i > 0 && rendered != "" {
			b.WriteString("\n\n")
		}
		b.WriteString(rendered)
	}
	out := strings.TrimSpace(b.String())
	// Plane wraps some documents as {"type":"doc",...,"content":[...]} with a
	// single editor-paragraph-block wrapper — flatten it when it is the whole doc.
	if out == "" && len(doc.Content) == 0 {
		return "", true
	}
	return normalizeMarkdownBlankLines(out), true
}

// renderBlock renders one block node to one Markdown block (no separating
// blank lines — the caller joins blocks). ok=false means "node shape not
// understood", which bubbles up to the HTML fallback.
func renderBlock(n tiptapNode, indent int) (string, bool) {
	pad := strings.Repeat("  ", indent)
	switch n.Type {
	case "paragraph":
		text, ok := renderInline(n.Content, n.Marks)
		if !ok {
			// Paragraphs whose children are blocks (nested wrappers some Plane
			// exports produce) render through their children instead of dying.
			var inner []string
			for _, child := range n.Content {
				rendered, childOK := renderBlock(child, indent)
				if !childOK {
					return "", false
				}
				if rendered != "" {
					inner = append(inner, rendered)
				}
			}
			if len(inner) == 0 {
				return "", true
			}
			return strings.Join(inner, "\n\n"), true
		}
		return strings.TrimSpace(text), true
	case "heading":
		level := attrInt(n.Attrs, "level", 1)
		if level < 1 {
			level = 1
		} else if level > 6 {
			level = 6
		}
		text, ok := renderInline(n.Content, n.Marks)
		if !ok {
			return "", false
		}
		return strings.Repeat("#", level) + " " + strings.TrimSpace(text), true
	case "bulletList", "orderedList":
		ordered := n.Type == "orderedList"
		var lines []string
		for i, li := range n.Content {
			if li.Type != "listItem" {
				return "", false
			}
			var inner []string
			for _, block := range li.Content {
				text, ok := renderBlock(block, indent+1)
				if !ok {
					return "", false
				}
				if text == "" {
					continue
				}
				inner = append(inner, text)
			}
			if len(inner) == 0 {
				inner = []string{""}
			}
			marker := "- "
			if ordered {
				marker = strconv.Itoa(i+1) + ". "
			}
			// First line carries the marker; deeper blocks keep their own
			// relative indent from renderBlock at indent+1.
			first := true
			var sb strings.Builder
			for _, line := range strings.Split(strings.Join(inner, "\n\n"), "\n") {
				if first {
					sb.WriteString(pad + marker + line)
					first = false
				} else {
					sb.WriteString(pad + "  " + strings.TrimPrefix(line, pad+"  "))
				}
				sb.WriteString("\n")
			}
			lines = append(lines, strings.TrimRight(sb.String(), "\n"))
		}
		return strings.Join(lines, "\n"), true
	case "taskList":
		var lines []string
		for _, li := range n.Content {
			if li.Type != "taskItem" {
				return "", false
			}
			checked := attrBool(n.Attrs, "checked") || attrBool(li.Attrs, "checked")
			var text string
			for _, block := range li.Content {
				rendered, ok := renderBlock(block, 0)
				if !ok {
					return "", false
				}
				if rendered != "" {
					text = rendered
					break
				}
			}
			mark := "[ ]"
			if checked {
				mark = "[x]"
			}
			lines = append(lines, "- "+mark+" "+text)
		}
		return strings.Join(lines, "\n"), true
	case "blockquote":
		var inner []string
		for _, block := range n.Content {
			text, ok := renderBlock(block, 0)
			if !ok {
				return "", false
			}
			inner = append(inner, text)
		}
		joined := strings.Join(inner, "\n")
		lines := strings.Split(joined, "\n")
		for i, line := range lines {
			lines[i] = "> " + line
		}
		return strings.Join(lines, "\n"), true
	case "codeBlock":
		lang, _ := n.Attrs["language"].(string)
		code := nodeText(n)
		fence := pickFence(code)
		return fence + lang + "\n" + code + "\n" + fence, true
	case "horizontalRule":
		return "---", true
	case "table":
		return renderTable(n)
	case "image":
		src, _ := n.Attrs["src"].(string)
		if src == "" {
			return "", true
		}
		alt, _ := n.Attrs["alt"].(string)
		return "![" + alt + "](" + src + ")", true
	case "editor-paragraph-block", "editorParagraphBlock", "editor-bullet-list", "editorBulletList", "editor-ordered-list", "editorOrderedList", "editor-code-block", "editorCodeBlock", "editor-blockquote", "editorBlockquote", "editor-table", "editorTable", "editor-image", "editorImage", "editor-horizontal-rule", "editorHorizontalRule", "editor-heading", "editorHeading", "editor-list-item", "editorListItem", "editor-task-list", "editorTaskList", "editor-task-item", "editorTaskItem":
		return renderBlock(unwrapEditorNode(n), indent)
	case "blockMath":
		return "```\n" + nodeText(n) + "\n```", true
	default:
		return "", false
	}
}

// renderInline renders inline content (text/mention nodes) to Markdown,
// applying marks. ok=false signals an unknown inline node shape.
func renderInline(nodes []tiptapNode, outerMarks []tiptapMark) (string, bool) {
	var b strings.Builder
	for _, n := range nodes {
		switch n.Type {
		case "text":
			segment := escapeMarkdownText(n.Text)
			b.WriteString(applyMarks(segment, append(outerMarks, n.Marks...)))
		case "mention":
			label, _ := n.Attrs["label"].(string)
			if label == "" {
				label = nodeText(n)
			}
			b.WriteString(applyMarks("@ "+label, append(outerMarks, n.Marks...)))
			b.WriteString(" ")
		case "hardBreak":
			b.WriteString("\\\n")
		default:
			return "", false
		}
	}
	return b.String(), true
}

func applyMarks(text string, marks []tiptapMark) string {
	if text == "" {
		return text
	}
	var link string
	for _, m := range marks {
		switch m.Type {
		case "bold":
			text = "**" + text + "**"
		case "italic":
			text = "*" + text + "*"
		case "strike":
			text = "~~" + text + "~~"
		case "code":
			text = "`" + text + "`"
		case "underline":
			text = "<u>" + text + "</u>"
		case "link":
			href, _ := m.Attrs["href"].(string)
			if href != "" {
				link = href
			}
		}
	}
	if link != "" {
		text = "[" + text + "](" + link + ")"
	}
	return text
}

func renderTable(n tiptapNode) (string, bool) {
	rowsAttr := attrInt(n.Attrs, "columnWidths", 0)
	_ = rowsAttr
	var rows [][]string
	headerRow := -1
	for ri, row := range n.Content {
		if row.Type != "tableRow" {
			return "", false
		}
		var cells []string
		for ci, cell := range row.Content {
			if cell.Type != "tableHeader" && cell.Type != "tableCell" {
				return "", false
			}
			var cellText []string
			for _, block := range cell.Content {
				text, ok := renderBlock(block, 0)
				if !ok {
					return "", false
				}
				if text != "" {
					cellText = append(cellText, strings.ReplaceAll(text, "\n", " "))
				}
			}
			cells = append(cells, strings.Join(cellText, " "))
			_ = ci
		}
		if row.Content[0].Type == "tableHeader" && headerRow == -1 {
			headerRow = ri
		}
		rows = append(rows, cells)
	}
	if len(rows) == 0 {
		return "", true
	}
	width := 0
	for _, r := range rows {
		if len(r) > width {
			width = len(r)
		}
	}
	var lines []string
	for ri, r := range rows {
		for len(r) < width {
			r = append(r, "")
		}
		lines = append(lines, "| "+strings.Join(r, " | ")+" |")
		if ri == 0 || (headerRow == -1 && ri == 0) {
			seps := make([]string, width)
			for i := range seps {
				seps[i] = "---"
			}
			lines = append(lines, "|"+strings.Join(seps, "|")+"|")
		}
	}
	return strings.Join(lines, "\n"), true
}

// unwrapEditorNode converts Plane's editor-* wrapper node types to their
// plain TipTap equivalents. The hyphenated variants keep their hyphen as the
// word boundary; the camelCase variants only lowercase the first letter.
func unwrapEditorNode(n tiptapNode) tiptapNode {
	plain := n.Type
	if plain == "editor-paragraph-block" {
		plain = "paragraph"
	} else if strings.HasPrefix(plain, "editor-") {
		plain = strings.TrimPrefix(plain, "editor-")
	} else if strings.HasPrefix(plain, "editor") {
		rest := strings.TrimPrefix(plain, "editor")
		plain = strings.ToLower(rest[:1]) + rest[1:]
	}
	return tiptapNode{Type: plain, Attrs: n.Attrs, Marks: n.Marks, Content: n.Content, Text: n.Text}
}

// nodeText flattens all text under a node (code blocks, math).
func nodeText(n tiptapNode) string {
	var b strings.Builder
	collectText(n, &b)
	return b.String()
}

func collectText(n tiptapNode, b *strings.Builder) {
	if n.Text != "" {
		b.WriteString(n.Text)
	}
	for _, c := range n.Content {
		collectText(c, b)
	}
}

// pickFence chooses a fence longer than any backtick run inside the code so
// embedded fences cannot break the block.
func pickFence(code string) string {
	fence := "```"
	for strings.Contains(code, fence) {
		fence += "`"
	}
	return fence
}

func attrInt(attrs map[string]any, key string, def int) int {
	if v, ok := attrs[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		case string:
			if p, err := strconv.Atoi(n); err == nil {
				return p
			}
		}
	}
	return def
}

func attrBool(attrs map[string]any, key string) bool {
	v, _ := attrs[key].(bool)
	return v
}

// escapeMarkdownText protects characters that change Markdown meaning at the
// start of a text run. Inside paragraphs Plane text is otherwise literal and
// Windshift's renderer passes it through.
func escapeMarkdownText(s string) string {
	r := strings.NewReplacer(
		"\\", `\\`,
		`*`, `\*`,
		`_`, `\_`,
		"]", `\]`,
		"[", `\[`,
	)
	out := r.Replace(s)
	// Line starts: protect list markers and block intent.
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		switch {
		case strings.HasPrefix(trimmed, "#"), strings.HasPrefix(trimmed, ">"),
			strings.HasPrefix(trimmed, "- "), strings.HasPrefix(trimmed, "+ "),
			strings.HasPrefix(trimmed, "---"), strings.HasPrefix(trimmed, "|"):
			lines[i] = strings.Replace(line, trimmed, `\`+trimmed, 1)
		}
		if m := orderedMarker(trimmed); m != "" {
			lines[i] = strings.Replace(line, m, `\`+m, 1)
		}
	}
	return strings.Join(lines, "\n")
}

func orderedMarker(trimmed string) string {
	for i, c := range trimmed {
		if c == '.' && i > 0 && i <= 2 {
			head := trimmed[:i]
			if allDigits(head) {
				return trimmed[:i+1]
			}
		}
		if c == ' ' || c > '9' || c < '0' {
			return ""
		}
	}
	return ""
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// normalizeMarkdownBlankLines collapses 3+ consecutive newlines to one block
// gap (exactly one blank line), the shape Windshift's renderer expects.
func normalizeMarkdownBlankLines(s string) string {
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return s
}

// ConvertDescriptionFromHTML is the fallback path: when the TipTap document
// is missing or unparseable, Plane's own HTML render is carried through.
// Windshift stores Markdown but renders HTML on read only from Markdown, so a
// raw HTML body stays readable as HTML source; the reconciliation report marks
// every issue that took this path.
func ConvertDescriptionFromHTML(html string) string {
	return strings.TrimSpace(html)
}

var _ = fmt.Sprintf // keep fmt for future errors without churn
