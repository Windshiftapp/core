package utils

import (
	"fmt"
	"strings"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
)

// ContentConverter handles conversion between HTML and Markdown (legacy support)
type ContentConverter struct {
}

// NewContentConverter creates a new content converter
func NewContentConverter() (*ContentConverter, error) {
	return &ContentConverter{}, nil
}

// HTMLToMarkdown converts HTML to Markdown (useful for legacy content)
func (c *ContentConverter) HTMLToMarkdown(html string) (string, error) {
	markdown, err := htmltomarkdown.ConvertString(html)
	if err != nil {
		return "", fmt.Errorf("failed to convert HTML to markdown: %w", err)
	}
	return strings.TrimSpace(markdown), nil
}

// StripHTML removes HTML tags and returns plain text (fallback option)
func StripHTML(html string) string {
	// Simple HTML stripping for fallback
	// This is a basic implementation - for production, consider using a proper HTML parser
	var result strings.Builder
	inTag := false

	for _, r := range html {
		if r == '<' {
			inTag = true
		} else if r == '>' {
			inTag = false
			result.WriteRune(' ')
		} else if !inTag {
			result.WriteRune(r)
		}
	}

	// Clean up multiple spaces
	text := result.String()
	text = strings.ReplaceAll(text, "  ", " ")
	text = strings.TrimSpace(text)

	return text
}
