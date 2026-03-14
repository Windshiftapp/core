// Package kreuzberg provides text extraction and chunking utilities.
package kreuzberg

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"unicode/utf8"
)

// ExtractionResult holds the output of text extraction from a file.
type ExtractionResult struct {
	Content  string
	MimeType string
	Pages    int
}

// ChunkConfig configures the text chunking behavior.
type ChunkConfig struct {
	MaxTokens int // Maximum tokens per chunk (default: 512)
	Overlap   int // Overlap tokens between chunks (default: 64)
}

// Chunk represents a segment of text with position metadata.
type Chunk struct {
	Content   string
	ByteStart int
	ByteEnd   int
	FirstPage *int
	LastPage  *int
}

// DefaultChunkConfig returns sensible defaults for chunking.
func DefaultChunkConfig() ChunkConfig {
	return ChunkConfig{
		MaxTokens: 512,
		Overlap:   64,
	}
}

// ExtractFile extracts text content from a file using the kreuzberg CLI.
// Supports PDF, Office formats, plain text, images, and 75+ other formats.
func ExtractFile(filePath string) (*ExtractionResult, error) {
	out, err := exec.Command("kreuzberg", "extract", filePath, "--format", "json").Output() //nolint:gosec // G204: command path from application config, not user input
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			return nil, fmt.Errorf("kreuzberg extraction failed: %w: %s", err, string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("kreuzberg extraction failed: %w", err)
	}
	var result struct {
		Content  string `json:"content"`
		MimeType string `json:"mime_type"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("failed to parse kreuzberg output: %w", err)
	}
	return &ExtractionResult{
		Content:  result.Content,
		MimeType: result.MimeType,
	}, nil
}

// ChunkText splits text into semantic chunks based on the given configuration.
// Uses paragraph boundaries and sentence boundaries to create natural chunks.
func ChunkText(content string, config ChunkConfig) ([]Chunk, error) {
	if content == "" {
		return nil, nil
	}

	if config.MaxTokens <= 0 {
		config.MaxTokens = 512
	}
	if config.Overlap < 0 {
		config.Overlap = 0
	}

	// Approximate tokens as words (rough 1:1.3 ratio for English)
	maxChars := config.MaxTokens * 4 // ~4 chars per token average
	overlapChars := config.Overlap * 4

	paragraphs := splitParagraphs(content)
	var chunks []Chunk
	var current strings.Builder
	currentStart := 0

	for _, para := range paragraphs {
		paraBytes := len(para)

		// If adding this paragraph exceeds max, flush current chunk
		if current.Len()+paraBytes > maxChars && current.Len() > 0 {
			chunkContent := current.String()
			chunks = append(chunks, Chunk{
				Content:   chunkContent,
				ByteStart: currentStart,
				ByteEnd:   currentStart + len(chunkContent),
			})

			// Start new chunk with overlap
			if overlapChars > 0 && len(chunkContent) > overlapChars {
				overlap := chunkContent[len(chunkContent)-overlapChars:]
				current.Reset()
				current.WriteString(overlap)
				currentStart += len(chunkContent) - overlapChars
			} else {
				current.Reset()
				currentStart += len(chunkContent)
			}
		}

		// If a single paragraph exceeds max, split it
		if paraBytes > maxChars {
			// Flush any current content first
			if current.Len() > 0 {
				chunkContent := current.String()
				chunks = append(chunks, Chunk{
					Content:   chunkContent,
					ByteStart: currentStart,
					ByteEnd:   currentStart + len(chunkContent),
				})
				current.Reset()
				currentStart += len(chunkContent)
			}

			// Split large paragraph by sentences/words
			offset := 0
			for offset < len(para) {
				end := offset + maxChars
				if end > len(para) {
					end = len(para)
				} else {
					// Try to break at a sentence or word boundary
					for end > offset+maxChars/2 {
						if para[end] == ' ' || para[end] == '.' || para[end] == '\n' {
							end++
							break
						}
						end--
					}
					if end <= offset+maxChars/2 {
						end = offset + maxChars
						if end > len(para) {
							end = len(para)
						}
					}
				}

				// Ensure we don't split mid-rune
				for end < len(para) && !utf8.RuneStart(para[end]) {
					end++
				}

				chunkContent := para[offset:end]
				chunks = append(chunks, Chunk{
					Content:   chunkContent,
					ByteStart: currentStart + offset,
					ByteEnd:   currentStart + end,
				})
				offset = end
			}
			currentStart += len(para)
			continue
		}

		if current.Len() > 0 {
			current.WriteString("\n\n")
		}
		current.WriteString(para)
	}

	// Flush remaining content
	if current.Len() > 0 {
		chunkContent := current.String()
		chunks = append(chunks, Chunk{
			Content:   chunkContent,
			ByteStart: currentStart,
			ByteEnd:   currentStart + len(chunkContent),
		})
	}

	return chunks, nil
}

// splitParagraphs splits text on double newlines, preserving non-empty paragraphs.
func splitParagraphs(text string) []string {
	raw := strings.Split(text, "\n\n")
	var result []string
	for _, p := range raw {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
