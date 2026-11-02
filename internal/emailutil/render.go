// Package emailutil provides shared email template rendering utilities.
package emailutil

import (
	"bytes"
	"fmt"
	stdhtml "html"
	htmltemplate "html/template"
	texttemplate "text/template"

	"windshift/internal/markdown"
)

// templateFuncs are available to every email template. `markdown` renders a
// Markdown field to sanitized HTML so formatting, links, and tables survive
// into the HTML part of the email.
var templateFuncs = htmltemplate.FuncMap{
	"markdown":      renderMarkdown,
	"plainMentions": plainMentions,
}

// plainMentions strips the embedded id from @{Display Name#123} tokens for the
// plain-text email part, which cannot run the Markdown renderer. The HTML part
// goes through markdown.Render, which handles the token there.
func plainMentions(source string) string {
	return markdown.StripMentionIDs(source)
}

// renderMarkdown converts stored Markdown to sanitized HTML for the HTML email
// part. It is not used by the plain-text part, which keeps the raw Markdown.
func renderMarkdown(source string) htmltemplate.HTML {
	rendered, err := markdown.Render(source)
	if err != nil {
		// Never let a render error drop the customer's words.
		return htmltemplate.HTML(stdhtml.EscapeString(source)) //nolint:gosec // escaped fallback
	}
	return htmltemplate.HTML(rendered) //nolint:gosec // markdown.Render sanitizes
}

// RenderTemplates parses and executes an HTML and a plain-text Go template
// with the given data, returning the rendered strings.
func RenderTemplates(htmlTemplateSrc, textTemplateSrc string, data any) (html, text string, err error) {
	htmlTmpl, err := htmltemplate.New("html").Funcs(templateFuncs).Parse(htmlTemplateSrc)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse HTML template: %w", err)
	}

	var htmlBuf bytes.Buffer
	if err = htmlTmpl.Execute(&htmlBuf, data); err != nil {
		return "", "", fmt.Errorf("failed to execute HTML template: %w", err)
	}

	textTmpl, err := texttemplate.New("text").Funcs(templateFuncs).Parse(textTemplateSrc)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse text template: %w", err)
	}

	var textBuf bytes.Buffer
	if err = textTmpl.Execute(&textBuf, data); err != nil {
		return "", "", fmt.Errorf("failed to execute text template: %w", err)
	}

	return htmlBuf.String(), textBuf.String(), nil
}
