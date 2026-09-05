// Package emailutil provides shared email template rendering utilities.
package emailutil

import (
	"bytes"
	"fmt"
	htmltemplate "html/template"
	texttemplate "text/template"
)

// RenderTemplates parses and executes an HTML and a plain-text Go template
// with the given data, returning the rendered strings.
func RenderTemplates(htmlTemplateSrc, textTemplateSrc string, data any) (html, text string, err error) {
	htmlTmpl, err := htmltemplate.New("html").Parse(htmlTemplateSrc)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse HTML template: %w", err)
	}

	var htmlBuf bytes.Buffer
	if err = htmlTmpl.Execute(&htmlBuf, data); err != nil {
		return "", "", fmt.Errorf("failed to execute HTML template: %w", err)
	}

	textTmpl, err := texttemplate.New("text").Parse(textTemplateSrc)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse text template: %w", err)
	}

	var textBuf bytes.Buffer
	if err = textTmpl.Execute(&textBuf, data); err != nil {
		return "", "", fmt.Errorf("failed to execute text template: %w", err)
	}

	return htmlBuf.String(), textBuf.String(), nil
}
