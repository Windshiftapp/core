package email

import (
	"fmt"
	"html"
	"log/slog"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/table"
	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	nethtml "golang.org/x/net/html"
)

const (
	maxEmailHTMLBytes = 1024 * 1024
	minFidelityRunes  = 100
	minHTMLFraction   = 4 // Fall back below one quarter of the plain-text length.
)

var (
	emailHTMLPolicy      = newEmailHTMLPolicy()
	plainMarkdownEscapes = strings.NewReplacer(
		`\`, `\\`, "`", "\\`", "*", `\*`, "_", `\_`,
		"[", `\[`, "]", `\]`, "<", `\<`, ">", `\>`, "#", `\#`, "|", `\|`, "~", `\~`,
	)
	plainBlockMarker   = regexp.MustCompile(`(?m)^([ \t]*)([-+])([ \t]+)`)
	plainOrderedMarker = regexp.MustCompile(`(?m)^([ \t]*\d{1,9})([.)])([ \t]+)`)
	plainDivider       = regexp.MustCompile(`(?m)^([ \t]*)([-=]{2,})([ \t]*)$`)
)

func newEmailHTMLPolicy() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowElements("p", "div", "span", "br", "strong", "b", "em", "i", "s", "del",
		"h1", "h2", "h3", "h4", "h5", "h6", "ul", "ol", "li", "blockquote", "pre", "code", "hr",
		"table", "thead", "tbody", "tfoot", "tr", "th", "td", "caption", "a")
	p.AllowAttrs("href").OnElements("a")
	p.AllowAttrs("start").Matching(regexp.MustCompile(`^\d{1,9}$`)).OnElements("ol")
	p.AllowAttrs("role").Matching(regexp.MustCompile(`^presentation$`)).OnElements("table")
	p.RequireParseableURLs(true)
	p.AllowURLSchemes("http", "https", "mailto", "tel")
	return p
}

// bodyMarkdown selects the MIME alternative before cleaning quotes or signatures.
func (e *ParsedEmail) bodyMarkdown(reply bool) string {
	if e.HTMLBody != "" {
		markdown, visible, err := emailHTMLMarkdown(e.HTMLBody)
		plainLength := normalizedBodyLength(e.PlainBody)
		htmlLength := normalizedBodyLength(visible)
		switch {
		case err != nil:
			slog.Warn("email HTML conversion failed, using text fallback", "message_id", e.MessageID, "error", err)
		case htmlLength == 0:
			// Image-only and tracking-only alternatives have no message text.
		case plainLength >= minFidelityRunes && htmlLength*minHTMLFraction < plainLength:
			slog.Debug("email HTML alternative is incomplete, using plain text", "message_id", e.MessageID)
		default:
			return cleanEmailMarkdown(markdown, reply)
		}
	}

	body := e.PlainBody
	if strings.TrimSpace(body) == "" && e.HTMLBody != "" {
		// Keep an oversized or otherwise unconvertible HTML-only message readable.
		body = html.UnescapeString(StripHTML(e.HTMLBody[:min(len(e.HTMLBody), maxEmailHTMLBytes)]))
	}
	if strings.TrimSpace(body) == "" {
		return ""
	}
	body = strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\r", "\n")
	if reply {
		body = ExtractReplyContent(body)
	}
	body = StripSignature(body)
	body = plainMarkdownEscapes.Replace(body)
	body = plainBlockMarker.ReplaceAllString(body, "$1\\$2$3")
	body = plainOrderedMarker.ReplaceAllString(body, "$1\\$2$3")
	body = plainDivider.ReplaceAllString(body, "$1\\$2$3")
	lines := strings.Split(body, "\n")
	for i := 0; i+1 < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != "" && strings.TrimSpace(lines[i+1]) != "" {
			lines[i] = strings.TrimRight(lines[i], " \t") + "  "
		}
	}
	return strings.Join(lines, "\n")
}

func normalizedBodyLength(body string) int {
	return utf8.RuneCountInString(strings.Join(strings.Fields(body), " "))
}

func emailHTMLMarkdown(body string) (markdown, visible string, err error) {
	if len(body) > maxEmailHTMLBytes {
		return "", "", fmt.Errorf("HTML body exceeds %d bytes", maxEmailHTMLBytes)
	}
	safeHTML := emailHTMLPolicy.Sanitize(body)
	visible = html.UnescapeString(StripHTML(safeHTML))
	doc, err := nethtml.Parse(strings.NewReader(safeHTML))
	if err != nil {
		return "", "", fmt.Errorf("parse email HTML: %w", err)
	}
	flattenEmailLayoutTables(doc)
	conv := converter.NewConverter(converter.WithPlugins(
		base.NewBasePlugin(), commonmark.NewCommonmarkPlugin(), table.NewTablePlugin(),
	))
	converted, err := conv.ConvertNode(doc)
	if err != nil {
		return "", "", fmt.Errorf("convert email HTML: %w", err)
	}
	return string(converted), visible, nil
}

// Only tables with explicit headers are treated as data tables.
func flattenEmailLayoutTables(node *nethtml.Node) {
	if node.Type == nethtml.ElementNode && node.Data == "table" {
		layout := !emailTableHasHeader(node)
		for _, attr := range node.Attr {
			if attr.Key == "role" && attr.Val == "presentation" {
				layout = true
			}
		}
		if layout {
			flattenEmailTable(node, true)
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		flattenEmailLayoutTables(child)
	}
}

func emailTableHasHeader(node *nethtml.Node) bool {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == nethtml.ElementNode && child.Data == "table" {
			continue
		}
		if child.Data == "th" || emailTableHasHeader(child) {
			return true
		}
	}
	return false
}

func flattenEmailTable(node *nethtml.Node, root bool) {
	if !root && node.Type == nethtml.ElementNode && node.Data == "table" {
		return
	}
	if node.Type == nethtml.ElementNode {
		switch node.Data {
		case "table", "thead", "tbody", "tfoot", "tr", "th", "td", "caption":
			node.Data = "div"
			node.DataAtom = 0
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		flattenEmailTable(child, false)
	}
}

// Hide code-block contents from the existing line-based email cleanup.
func cleanEmailMarkdown(body string, reply bool) string {
	source := []byte(body)
	doc := goldmark.DefaultParser().Parse(text.NewReader(source))
	type codeRange struct{ start, end int }
	var ranges []codeRange
	_ = ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering && (node.Kind() == ast.KindFencedCodeBlock || node.Kind() == ast.KindCodeBlock) && node.Lines().Len() > 0 {
			lines := node.Lines()
			ranges = append(ranges, codeRange{lines.At(0).Start, lines.At(lines.Len() - 1).Stop})
		}
		return ast.WalkContinue, nil
	})
	prefix := "WINDSHIFT_EMAIL_CODE_"
	for strings.Contains(body, prefix) {
		prefix += "_"
	}
	restore := make([]string, 0, len(ranges)*2)
	for i := len(ranges) - 1; i >= 0; i-- {
		r := ranges[i]
		placeholder := fmt.Sprintf("%s%d\n", prefix, i)
		restore = append(restore, placeholder, body[r.start:r.end])
		body = body[:r.start] + placeholder + body[r.end:]
	}
	if reply {
		body = ExtractReplyContent(body)
	}
	body = StripSignature(body)
	if len(restore) > 0 {
		body = strings.NewReplacer(restore...).Replace(body)
	}
	return body
}
