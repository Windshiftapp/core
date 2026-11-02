// Package markdown renders stored Markdown for browser display.
package markdown

import (
	"bytes"
	"encoding/base64"
	"html"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

var (
	commonURLSchemes    = []string{"http", "https"}
	linkOnlyURLSchemes  = []string{"mailto", "tel"}
	sanitizerURLSchemes = slices.Concat(commonURLSchemes, linkOnlyURLSchemes, []string{"page"})
	classPattern        = regexp.MustCompile(`^(?:language-[A-Za-z0-9_+-]+)$`)
	pageIDPattern       = regexp.MustCompile(`^\d+$`)
	rasterDataPrefix    = regexp.MustCompile(`(?i)^image/(?:png|jpeg|gif|webp);base64,`)
	mentionTokenPattern = regexp.MustCompile(`@\{([^}\n]*)#\d+\}`)
	markdownHTML        = goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(
			parser.WithInlineParsers(util.Prioritized(mentionInlineParser{}, 500)),
			parser.WithASTTransformers(util.Prioritized(linkPolicyTransformer{}, 500)),
		),
		goldmark.WithRendererOptions(renderer.WithNodeRenderers(
			util.Prioritized(literalHTMLRenderer{}, 500),
		)),
	)
	markdownPolicy = newPolicy()
)

// mentionInlineParser renders the stored @{Display Name#123} token as its
// display text. Inline parsers do not run inside code spans or fenced blocks,
// so literal examples survive unchanged.
type mentionInlineParser struct{}

func (mentionInlineParser) Trigger() []byte { return []byte{'@'} }

func (mentionInlineParser) Parse(parent ast.Node, block text.Reader, _ parser.Context) ast.Node {
	line, segment := block.PeekLine()
	match := mentionTokenPattern.FindSubmatchIndex(line)
	if len(match) == 0 || match[0] != 0 {
		return nil
	}
	// Mirror the mention parser's word boundary so a token glued to a word
	// (for example "foo@{A#1}") is not rewritten as a mention.
	if segment.Start > 0 && isMentionBoundaryChar(block.Source()[segment.Start-1]) {
		return nil
	}
	name := line[match[2]:match[3]]
	block.Advance(match[1])
	return ast.NewString(append([]byte("@"), name...))
}

func isMentionBoundaryChar(c byte) bool {
	return c == '.' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

type linkPolicyTransformer struct{}

func (linkPolicyTransformer) Transform(document *ast.Document, _ text.Reader, _ parser.Context) {
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := node.(type) {
		case *ast.Link:
			if !validDestination(string(n.Destination), false) {
				n.Destination = nil
			}
		case *ast.Image:
			if !validDestination(string(n.Destination), true) {
				n.Destination = nil
			}
		}
		return ast.WalkContinue, nil
	})
}

func validDestination(destination string, image bool) bool {
	if destination == "" || strings.HasPrefix(destination, "//") ||
		strings.Contains(destination, `\`) || strings.IndexFunc(destination, unicodeURLControl) >= 0 {
		return false
	}
	decoded, err := url.PathUnescape(destination)
	if err != nil || strings.Contains(decoded, `\`) {
		return false
	}
	if strings.HasPrefix(destination, "#") || strings.HasPrefix(destination, "/") {
		return true
	}

	parsed, err := url.Parse(destination)
	if err != nil {
		return false
	}
	if parsed.Scheme == "" {
		return true
	}

	scheme := strings.ToLower(parsed.Scheme)
	if slices.Contains(commonURLSchemes, scheme) {
		return true
	}
	if !image && slices.Contains(linkOnlyURLSchemes, scheme) {
		return true
	}

	switch scheme {
	case "page":
		return !image && pageIDPattern.MatchString(parsed.Opaque)
	case "data":
		if !image || parsed.RawQuery != "" || parsed.Fragment != "" {
			return false
		}
		prefix := rasterDataPrefix.FindString(parsed.Opaque)
		if prefix == "" {
			return false
		}
		_, err := base64.StdEncoding.DecodeString(parsed.Opaque[len(prefix):])
		return err == nil
	default:
		return false
	}
}

func unicodeURLControl(r rune) bool {
	return unicode.IsControl(r) || unicode.IsSpace(r)
}

// StripMentionIDs rewrites @{Display Name#123} tokens to their display text.
// Callers that cannot run the Markdown renderer (plain-text email bodies) use
// it so the embedded id is never exposed.
func StripMentionIDs(source string) string {
	return mentionTokenPattern.ReplaceAllString(source, "@$1")
}

// Render converts Markdown source to sanitized HTML. Source is never changed.
func Render(source string) (string, error) {
	if source == "" {
		return "", nil
	}

	var rendered bytes.Buffer
	if err := markdownHTML.Convert([]byte(source), &rendered); err != nil {
		return "", err
	}
	return markdownPolicy.Sanitize(rendered.String()), nil
}

func newPolicy() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowElements(
		"p", "br", "hr", "blockquote", "pre", "code",
		"h1", "h2", "h3", "h4", "h5", "h6",
		"ul", "ol", "li", "em", "strong", "del",
		"a", "img", "table", "thead", "tbody", "tr", "th", "td",
		"input",
	)
	// Goldmark destinations pass through validDestination before rendering.
	// Keep bluemonday focused on the final HTML allowlist and scheme defense
	// instead of maintaining a second copy of the destination grammar.
	p.AllowAttrs("href").OnElements("a")
	p.AllowAttrs("title").OnElements("a", "img")
	p.AllowAttrs("src").OnElements("img")
	p.AllowAttrs("alt").OnElements("img")
	p.AllowAttrs("class").Matching(classPattern).OnElements("code")
	p.AllowAttrs("align").Matching(regexp.MustCompile(`^(?:left|right|center)$`)).OnElements("th", "td")
	p.AllowAttrs("type").Matching(regexp.MustCompile(`^checkbox$`)).OnElements("input")
	p.AllowAttrs("checked", "disabled").Matching(regexp.MustCompile(`^$`)).OnElements("input")
	p.AllowRelativeURLs(true)
	p.AllowURLSchemes(sanitizerURLSchemes...)
	p.AllowURLSchemeWithCustomPolicy("data", func(parsed *url.URL) bool {
		if parsed.RawQuery != "" || parsed.Fragment != "" {
			return false
		}
		prefix := rasterDataPrefix.FindString(parsed.Opaque)
		if prefix == "" {
			return false
		}
		_, err := base64.StdEncoding.DecodeString(parsed.Opaque[len(prefix):])
		return err == nil
	})
	p.RequireNoFollowOnFullyQualifiedLinks(true)
	p.RequireNoReferrerOnFullyQualifiedLinks(true)
	return p
}

// literalHTMLRenderer makes raw HTML documentation visible instead of
// executing it. Milkdown's lowercase break spellings remain line breaks.
type literalHTMLRenderer struct{}

func (literalHTMLRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindRawHTML, renderLiteralInlineHTML)
	reg.Register(ast.KindHTMLBlock, renderLiteralBlockHTML)
}

func renderLiteralInlineHTML(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkSkipChildren, nil
	}
	n, ok := node.(*ast.RawHTML)
	if !ok {
		return ast.WalkContinue, nil
	}
	raw := string(n.Segments.Value(source))
	if isMilkdownBreak(raw) {
		_, _ = w.WriteString("<br>")
		return ast.WalkSkipChildren, nil
	}
	_, _ = w.WriteString(html.EscapeString(raw))
	return ast.WalkSkipChildren, nil
}

func renderLiteralBlockHTML(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkSkipChildren, nil
	}
	n, ok := node.(*ast.HTMLBlock)
	if !ok {
		return ast.WalkContinue, nil
	}
	var raw bytes.Buffer
	for i := range n.Lines().Len() {
		segment := n.Lines().At(i)
		raw.Write(segment.Value(source))
	}
	if n.HasClosure() {
		raw.Write(n.ClosureLine.Value(source))
	}
	value := raw.String()
	if isMilkdownBreak(strings.TrimSpace(value)) {
		_, _ = w.WriteString("<br>\n")
		return ast.WalkSkipChildren, nil
	}
	_, _ = w.WriteString("<p>")
	_, _ = w.WriteString(html.EscapeString(value))
	_, _ = w.WriteString("</p>\n")
	return ast.WalkSkipChildren, nil
}

func isMilkdownBreak(raw string) bool {
	return raw == "<br>" || raw == "<br/>" || raw == "<br />"
}
