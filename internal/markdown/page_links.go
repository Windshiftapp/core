package markdown

import (
	"regexp"
	"strconv"
	"strings"
)

// pageLinkPattern matches the sanitized anchor form of a page link, i.e. the
// HTML produced by Render for `[text](page:123)`. Render's destination policy
// guarantees the href is exactly the page scheme with a numeric ID, and
// bluemonday escapes attribute values, so the strict shape below is the only
// shape that can occur.
var pageLinkPattern = regexp.MustCompile(`(?s)<a\b[^>]*\bhref="page:(\d+)"[^>]*>(.*?)</a>`)

// RewritePageLinks rewrites sanitized HTML anchors whose destination is a
// page link (href="page:<id>") using resolve. Resolvable links get their href
// replaced; unresolvable ones are stripped to their link text so no dead or
// existence-leaking anchor survives. Everything else passes through
// unchanged. Resolve may be called more than once per link.
func RewritePageLinks(html string, resolve func(pageID int) (href string, ok bool)) string {
	if html == "" || resolve == nil || !strings.Contains(html, `href="page:`) {
		return html
	}
	return pageLinkPattern.ReplaceAllStringFunc(html, func(match string) string {
		parts := pageLinkPattern.FindStringSubmatch(match)
		if parts == nil {
			return match
		}
		pageID, err := strconv.Atoi(parts[1])
		if err != nil {
			return parts[2]
		}
		href, ok := resolve(pageID)
		if !ok {
			return parts[2]
		}
		return `<a href="` + href + `">` + parts[2] + `</a>`
	})
}

// markdownPageLinkPattern matches the markdown link form `[text](page:<id>)`
// as authored through the editor's page-link picker.
var markdownPageLinkPattern = regexp.MustCompile(`\[([^\]\n]+)\]\(page:(\d+)\)`)

// RewritePageLinksInMarkdown rewrites markdown page links the same way
// RewritePageLinks rewrites their sanitized HTML form: resolvable links get
// their destination replaced, unresolvable ones degrade to plain link text so
// no dead or existence-leaking anchor survives. Markdown-level rewriting is
// what portal surfaces that render the raw markdown through the shared
// read-only editor need — the readonly pipeline renders markdown structure
// and treats raw HTML as inert text.
func RewritePageLinksInMarkdown(md string, resolve func(pageID int) (href string, ok bool)) string {
	if md == "" || resolve == nil || !strings.Contains(md, "](page:") {
		return md
	}
	return markdownPageLinkPattern.ReplaceAllStringFunc(md, func(match string) string {
		parts := markdownPageLinkPattern.FindStringSubmatch(match)
		if parts == nil {
			return match
		}
		pageID, err := strconv.Atoi(parts[2])
		if err != nil {
			return parts[1]
		}
		href, ok := resolve(pageID)
		if !ok {
			return parts[1]
		}
		return "[" + parts[1] + "](" + href + ")"
	})
}
