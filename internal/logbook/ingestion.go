package logbook

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"path/filepath"
	"regexp"
	"strings"
	"windshift/internal/kreuzberg"
	"windshift/internal/llm"
	"windshift/internal/models"
)

// IngestionService orchestrates document ingestion: extract → chunk → store.
type IngestionService struct {
	repo          *Repository
	articleClient llm.Client
}

// NewIngestionService creates a new ingestion service.
func NewIngestionService(repo *Repository, articleClient llm.Client) *IngestionService {
	return &IngestionService{
		repo:          repo,
		articleClient: articleClient,
	}
}

// IngestFile processes an uploaded file: extract text, chunk, embed, store.
func (s *IngestionService) IngestFile(ctx context.Context, docID string) error {
	doc, err := s.repo.GetDocument(docID)
	if err != nil {
		return fmt.Errorf("failed to get document: %w", err)
	}
	if doc == nil {
		return fmt.Errorf("document not found: %s", docID)
	}

	// Update status to processing
	if err := s.repo.UpdateDocumentStatus(docID, models.LogbookDocStatusProcessing, ""); err != nil {
		return err
	}

	// Extract text from file
	result, err := kreuzberg.ExtractFile(doc.FilePath)
	if err != nil {
		_ = s.repo.UpdateDocumentStatus(docID, models.LogbookDocStatusError, fmt.Sprintf("extraction failed: %v", err))
		return fmt.Errorf("extraction failed: %w", err)
	}

	// Compute content hash
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(result.Content)))

	// Update document with extracted content
	if err := s.repo.UpdateDocumentContent(docID, result.Content, result.MimeType, hash); err != nil {
		_ = s.repo.UpdateDocumentStatus(docID, models.LogbookDocStatusError, fmt.Sprintf("content update failed: %v", err))
		return err
	}

	// Generate thumbnail (non-fatal)
	s.generateThumbnail(docID, doc.FilePath, result.MimeType)

	// No text content (e.g. image files) — skip LLM processing, go straight to ready
	if result.Content == "" {
		return s.chunkContent(ctx, docID, "")
	}

	// Classify and clean content
	contentType, cleanedContent := s.classifyAndClean(ctx, docID, doc.Title, result.Content, doc.FilePath, result.MimeType)

	// Generate KB article based on classification
	s.generateArticle(ctx, docID, doc.Title, cleanedContent, contentType)

	// Chunk cleaned content instead of raw
	return s.chunkContent(ctx, docID, cleanedContent)
}

// IngestNote processes a markdown note: chunk and store.
func (s *IngestionService) IngestNote(ctx context.Context, docID string) error {
	doc, err := s.repo.GetDocument(docID)
	if err != nil {
		return fmt.Errorf("failed to get document: %w", err)
	}
	if doc == nil {
		return fmt.Errorf("document not found: %s", docID)
	}

	// Update status to processing
	if err := s.repo.UpdateDocumentStatus(docID, models.LogbookDocStatusProcessing, ""); err != nil {
		return err
	}

	// Compute content hash
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(doc.RawContent)))
	if err := s.repo.UpdateDocumentContent(docID, doc.RawContent, "text/markdown", hash); err != nil {
		_ = s.repo.UpdateDocumentStatus(docID, models.LogbookDocStatusError, fmt.Sprintf("content update failed: %v", err))
		return err
	}

	// Notes are user-written content — skip classification and cleaning
	contentType := models.LogbookContentTypeKnowledge
	if err := s.repo.UpdateDocumentClassification(docID, contentType, doc.RawContent); err != nil {
		slog.Warn("failed to store classification", slog.String("doc_id", docID), slog.Any("error", err))
	}

	// Notes are user-authored — use raw content as the article directly (no LLM)
	if err := s.repo.UpdateDocumentArticle(docID, doc.RawContent); err != nil {
		slog.Warn("failed to set note article", slog.String("doc_id", docID), slog.Any("error", err))
	}

	return s.chunkContent(ctx, docID, doc.RawContent)
}

// ReprocessDocument re-processes an existing document (delete old chunks, re-chunk).
func (s *IngestionService) ReprocessDocument(ctx context.Context, docID string) error {
	doc, err := s.repo.GetDocument(docID)
	if err != nil {
		return fmt.Errorf("failed to get document: %w", err)
	}
	if doc == nil {
		return fmt.Errorf("document not found: %s", docID)
	}

	// Update status to processing
	if err := s.repo.UpdateDocumentStatus(docID, models.LogbookDocStatusProcessing, "reprocessing"); err != nil {
		return err
	}

	// Delete existing chunks
	if err := s.repo.DeleteChunksByDocument(docID); err != nil {
		_ = s.repo.UpdateDocumentStatus(docID, models.LogbookDocStatusError, fmt.Sprintf("chunk deletion failed: %v", err))
		return err
	}

	content := doc.RawContent
	var filePath, mimeType string

	// For uploaded files, re-extract
	if doc.SourceType == models.LogbookSourceUpload && doc.FilePath != "" {
		result, err := kreuzberg.ExtractFile(doc.FilePath)
		if err != nil {
			_ = s.repo.UpdateDocumentStatus(docID, models.LogbookDocStatusError, fmt.Sprintf("re-extraction failed: %v", err))
			return fmt.Errorf("re-extraction failed: %w", err)
		}
		content = result.Content
		filePath = doc.FilePath
		mimeType = result.MimeType
		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
		if err := s.repo.UpdateDocumentContent(docID, content, result.MimeType, hash); err != nil {
			return err
		}

		// Regenerate thumbnail (non-fatal)
		s.generateThumbnail(docID, doc.FilePath, result.MimeType)
	}

	// No text content (e.g. image files) — skip LLM processing, go straight to ready
	if content == "" {
		return s.chunkContent(ctx, docID, "")
	}

	// Classify and clean content
	contentType, cleanedContent := s.classifyAndClean(ctx, docID, doc.Title, content, filePath, mimeType)

	// Re-generate KB article based on classification
	s.generateArticle(ctx, docID, doc.Title, cleanedContent, contentType)

	return s.chunkContent(ctx, docID, cleanedContent)
}

// chunkContent splits text into chunks and stores them.
func (s *IngestionService) chunkContent(ctx context.Context, docID, content string) error {
	// Chunk the text
	config := kreuzberg.DefaultChunkConfig()
	textChunks, err := kreuzberg.ChunkText(content, config)
	if err != nil {
		_ = s.repo.UpdateDocumentStatus(docID, models.LogbookDocStatusError, fmt.Sprintf("chunking failed: %v", err))
		return fmt.Errorf("chunking failed: %w", err)
	}

	if len(textChunks) == 0 {
		slog.Warn("no chunks produced from document", slog.String("doc_id", docID))
		return s.repo.UpdateDocumentStatus(docID, models.LogbookDocStatusReady, "no content to index")
	}

	// Build model chunks
	modelChunks := make([]models.LogbookChunk, len(textChunks))
	for i, tc := range textChunks {
		modelChunks[i] = models.LogbookChunk{
			DocumentID: docID,
			Position:   i,
			Content:    tc.Content,
			TokenCount: estimateTokens(tc.Content),
			ByteStart:  tc.ByteStart,
			ByteEnd:    tc.ByteEnd,
			FirstPage:  tc.FirstPage,
			LastPage:   tc.LastPage,
		}
	}

	// Store chunks
	if err := s.repo.CreateChunks(docID, modelChunks); err != nil {
		_ = s.repo.UpdateDocumentStatus(docID, models.LogbookDocStatusError, fmt.Sprintf("chunk storage failed: %v", err))
		return fmt.Errorf("chunk storage failed: %w", err)
	}

	// Mark document as ready
	if err := s.repo.UpdateDocumentStatus(docID, models.LogbookDocStatusReady, ""); err != nil {
		return err
	}

	slog.Info("document ingestion complete",
		slog.String("doc_id", docID),
		slog.Int("chunks", len(modelChunks)),
	)
	return nil
}

const maxArticleContentChars = 12000
const maxClassifyContentChars = 2000

func contentPreview(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// classifyAndClean uses the LLM to classify a document and clean its content.
// Internally runs two focused LLM calls: one for classification, one for cleaning.
// Returns the content type and cleaned content. If no LLM is available, returns empty type and original content.
func (s *IngestionService) classifyAndClean(ctx context.Context, docID, title, content, filePath, mimeType string) (string, string) {
	if s.articleClient == nil || !s.articleClient.Available() {
		return "", content
	}

	// Step 1: Classify with a focused, few-shot prompt
	contentType := s.classify(ctx, docID, title, content, filePath, mimeType)

	// Step 2: Clean content (skip for records — they don't get articles anyway)
	cleanedContent := content
	if contentType != models.LogbookContentTypeRecord {
		cleanedContent = s.cleanContent(ctx, docID, title, content)
	}

	// Store classification result
	if err := s.repo.UpdateDocumentClassification(docID, contentType, cleanedContent); err != nil {
		slog.Warn("failed to store classification", slog.String("doc_id", docID), slog.Any("error", err))
	}

	slog.Info("document classified",
		slog.String("doc_id", docID),
		slog.String("content_type", contentType),
	)

	return contentType, cleanedContent
}

// classify runs a focused classification-only LLM call with few-shot examples.
// Returns a valid content type, defaulting to "record" on any failure.
func (s *IngestionService) classify(ctx context.Context, docID, title, content, filePath, mimeType string) string {
	// Truncate — classification only needs the beginning of the document
	truncated := content
	if len(truncated) > maxClassifyContentChars {
		truncated = truncated[:maxClassifyContentChars]
	}

	slog.Debug("classify: text-only path (no PDF attachment)",
		slog.String("doc_id", docID),
		slog.String("mime_type", mimeType),
		slog.Int("content_len", len(truncated)),
	)

	userContent := fmt.Sprintf("Document title: %s\n\nContent:\n%s", title, truncated)

	slog.Debug("classify prompt", slog.String("doc_id", docID), slog.String("title", title), slog.Int("content_len", len(truncated)), slog.String("content_preview", contentPreview(truncated, 200)))

	resp, err := s.articleClient.ChatCompletion(ctx, llm.ChatCompletionRequest{
		Messages: []llm.Message{
			{
				Role: "system",
				Content: `Classify this document into exactly one category. Reply with ONLY the category name.

knowledge
record
correspondence`,
			},
			{
				Role:    "user",
				Content: userContent,
			},
		},
		Temperature: 0.1,
		MaxTokens:   16,
	})
	if err != nil {
		slog.Warn("classification failed", slog.String("doc_id", docID), slog.Any("error", err))
		return models.LogbookContentTypeRecord
	}

	if len(resp.Choices) == 0 || resp.Choices[0].Message.Content == "" {
		slog.Warn("classification returned empty response", slog.String("doc_id", docID))
		return models.LogbookContentTypeRecord
	}

	raw := resp.Choices[0].Message.Content
	slog.Debug("classify response", slog.String("doc_id", docID), slog.String("raw", raw))
	result := parseClassificationType(raw)
	slog.Info("document classification result",
		slog.String("doc_id", docID),
		slog.String("raw_response", raw),
		slog.String("parsed_type", result),
	)
	return result
}

// cleanContent runs a focused cleaning-only LLM call.
// Returns the cleaned content, falling back to original on failure.
func (s *IngestionService) cleanContent(ctx context.Context, docID, title, content string) string {
	truncated := content
	if len(truncated) > maxArticleContentChars {
		truncated = truncated[:maxArticleContentChars] + "\n\n[content truncated]"
	}

	slog.Debug("clean content prompt", slog.String("doc_id", docID), slog.String("title", title), slog.Int("content_len", len(truncated)), slog.String("content_preview", contentPreview(truncated, 200)))

	resp, err := s.articleClient.ChatCompletion(ctx, llm.ChatCompletionRequest{
		Messages: []llm.Message{
			{
				Role: "system",
				Content: `Clean the document content by removing non-substantive elements. Output ONLY the cleaned content, nothing else.

Remove:
- Greeting and closing formulas (Dear X, Best regards, Sincerely, Mit freundlichen Grüßen, etc.)
- Email signatures and contact blocks
- Legal disclaimers and confidentiality notices
- Forwarding headers and reply chains

Preserve all substantive content, facts, data, and structure exactly as-is.`,
			},
			{
				Role:    "user",
				Content: fmt.Sprintf("Document title: %s\n\nContent:\n%s", title, truncated),
			},
		},
		Temperature: 0.1,
		MaxTokens:   4096,
	})
	if err != nil {
		slog.Warn("content cleaning failed", slog.String("doc_id", docID), slog.Any("error", err))
		return content
	}

	if len(resp.Choices) == 0 || resp.Choices[0].Message.Content == "" {
		slog.Warn("content cleaning returned empty response", slog.String("doc_id", docID))
		return content
	}

	cleaned := strings.TrimSpace(resp.Choices[0].Message.Content)
	slog.Debug("clean content response", slog.String("doc_id", docID), slog.Int("length", len(cleaned)))
	if cleaned == "" {
		return content
	}
	return cleaned
}

// parseClassificationType scans the LLM response for known classification keywords.
// Uses exact match first, then word-boundary matching to avoid substring false positives
// (e.g. "knowledge" inside "acknowledge"). Defaults to "record" on unknown input.
func parseClassificationType(response string) string {
	t := strings.ToLower(strings.TrimSpace(response))

	validTypes := []string{
		models.LogbookContentTypeRecord,
		models.LogbookContentTypeCorrespondence,
		models.LogbookContentTypeKnowledge,
	}

	// Fast path: exact match (ideal LLM response)
	for _, valid := range validTypes {
		if t == valid {
			return valid
		}
	}

	// Word-boundary match: find the earliest whole-word occurrence
	type match struct {
		pos  int
		kind string
	}
	var best *match
	for _, valid := range validTypes {
		re := regexp.MustCompile(`\b` + valid + `\b`)
		if loc := re.FindStringIndex(t); loc != nil {
			if best == nil || loc[0] < best.pos {
				best = &match{pos: loc[0], kind: valid}
			}
		}
	}

	if best != nil {
		return best.kind
	}
	slog.Warn("unknown classification response, defaulting to record", slog.String("raw", response))
	return models.LogbookContentTypeRecord
}

// generateArticle uses the direct LLM client to generate a structured KB article from content.
// Behavior depends on contentType:
//   - "knowledge": full KB article (default behavior)
//   - "correspondence": brief summary
//   - "record": skip article generation entirely
func (s *IngestionService) generateArticle(ctx context.Context, docID, title, content, contentType string) {
	// Records don't get articles
	if contentType == models.LogbookContentTypeRecord {
		return
	}

	if s.articleClient == nil || !s.articleClient.Available() {
		return
	}

	// Truncate content to fit context windows
	truncated := content
	if len(truncated) > maxArticleContentChars {
		truncated = truncated[:maxArticleContentChars] + "\n\n[content truncated]"
	}

	systemPrompt := "You are a knowledge base editor. Given raw document content, produce a clean, well-structured KB article in markdown. Preserve all important facts, procedures, and details. Use clear headings, bullet points, and concise language. Do not invent information that is not in the source material."
	if contentType == models.LogbookContentTypeCorrespondence {
		systemPrompt = "You are a knowledge base editor. Given correspondence content (email, letter, memo), produce a brief summary in markdown. Capture the key points, decisions, action items, and any important dates or commitments. Keep it concise — a few paragraphs at most. Do not invent information that is not in the source material."
	}

	slog.Debug("generate article prompt", slog.String("doc_id", docID), slog.String("title", title), slog.String("content_type", contentType), slog.Int("content_len", len(truncated)), slog.String("content_preview", contentPreview(truncated, 200)))

	resp, err := s.articleClient.ChatCompletion(ctx, llm.ChatCompletionRequest{
		Messages: []llm.Message{
			{
				Role:    "system",
				Content: systemPrompt,
			},
			{
				Role:    "user",
				Content: fmt.Sprintf("Document title: %s\n\nContent:\n%s", title, truncated),
			},
		},
		Temperature: 0.3,
		MaxTokens:   4096,
	})
	if err != nil {
		slog.Warn("article generation failed", slog.String("doc_id", docID), slog.Any("error", err))
		return
	}

	if len(resp.Choices) == 0 || resp.Choices[0].Message.Content == "" {
		slog.Warn("article generation returned empty response", slog.String("doc_id", docID))
		return
	}

	article := resp.Choices[0].Message.Content
	slog.Debug("generate article response", slog.String("doc_id", docID), slog.String("article", article))
	if err := s.repo.UpdateDocumentArticle(docID, article); err != nil {
		slog.Warn("failed to store generated article", slog.String("doc_id", docID), slog.Any("error", err))
		return
	}

	slog.Info("article generated", slog.String("doc_id", docID), slog.Int("length", len(article)))
}

// generateThumbnail attempts to create a thumbnail for the document. Failures are non-fatal.
func (s *IngestionService) generateThumbnail(docID, filePath, mimeType string) {
	thumbPath, err := GenerateThumbnail(docID, filePath, mimeType, filepath.Dir(filePath))
	if err != nil {
		slog.Warn("thumbnail generation failed", slog.String("doc_id", docID), slog.Any("error", err))
		return
	}
	if thumbPath == "" {
		return
	}
	if err := s.repo.UpdateDocumentThumbnail(docID, thumbPath); err != nil {
		slog.Warn("failed to store thumbnail path", slog.String("doc_id", docID), slog.Any("error", err))
	}
}

// estimateTokens provides a rough token count estimate (~4 chars per token).
func estimateTokens(text string) int {
	return len(text) / 4
}
