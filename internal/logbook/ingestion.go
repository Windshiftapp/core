package logbook

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"windshift/internal/kreuzberg"
	"windshift/internal/llm"
	"windshift/internal/models"
)

// IngestionService orchestrates document ingestion: extract → chunk → store.
type IngestionService struct {
	repo          *Repository
	articleClient llm.Client
	actionService *LogbookActionService

	// docLocks serializes Ingest/Reprocess calls on the same document. Two
	// concurrent reprocess requests would otherwise race on DeleteChunks /
	// CreateChunks and leave a corrupt chunk set behind. The map is guarded
	// by docLocksMu; entries are ref-counted and removed once the last
	// holder releases, so the map's size tracks in-flight docs rather than
	// the total distinct docs ever ingested.
	docLocksMu sync.Mutex
	docLocks   map[string]*docLockEntry
}

// docLockEntry is the per-document lock plus a refcount so we can delete the
// map entry once the last holder releases. The refcount is guarded by
// IngestionService.docLocksMu (not by entry.mu).
type docLockEntry struct {
	mu   sync.Mutex
	refs int
}

// NewIngestionService creates a new ingestion service.
func NewIngestionService(repo *Repository, articleClient llm.Client, actionService *LogbookActionService) *IngestionService {
	return &IngestionService{
		repo:          repo,
		articleClient: articleClient,
		actionService: actionService,
		docLocks:      make(map[string]*docLockEntry),
	}
}

// abortIfCanceled returns ctx.Err() if the ingestion context has been
// canceled (e.g. by Handlers.Shutdown), and marks the document as errored
// with a "canceled" reason so it doesn't sit in 'processing' forever. The
// status write uses context.Background so the cancel itself can't prevent
// the doc from leaving the processing state.
func (s *IngestionService) abortIfCanceled(ctx context.Context, docID, stage string) error {
	if err := ctx.Err(); err != nil {
		_ = s.repo.UpdateDocumentStatus(docID, models.LogbookDocStatusError, fmt.Sprintf("canceled at %s: %v", stage, err))
		return err
	}
	return nil
}

// lockDoc takes (and returns a release for) the per-document lock. The caller
// must invoke the returned release function when done; when the last holder
// releases, the entry is removed from the map so it doesn't accumulate one
// mutex per docID ever seen.
func (s *IngestionService) lockDoc(docID string) func() {
	s.docLocksMu.Lock()
	entry, ok := s.docLocks[docID]
	if !ok {
		entry = &docLockEntry{}
		s.docLocks[docID] = entry
	}
	entry.refs++
	s.docLocksMu.Unlock()

	entry.mu.Lock()

	return func() {
		entry.mu.Unlock()
		s.docLocksMu.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(s.docLocks, docID)
		}
		s.docLocksMu.Unlock()
	}
}

// IngestFile processes an uploaded file: extract text, chunk, embed, store.
func (s *IngestionService) IngestFile(ctx context.Context, docID string) error {
	release := s.lockDoc(docID)
	defer release()

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
		return s.chunkContent(docID, "")
	}

	if err := s.abortIfCanceled(ctx, docID, "before classify"); err != nil {
		return err
	}

	// Classify and clean content
	contentType, cleanedContent := s.classifyAndClean(ctx, docID, doc.Title, result.Content, result.MimeType)

	if err := s.abortIfCanceled(ctx, docID, "before article"); err != nil {
		return err
	}

	// Generate KB article based on classification
	s.generateArticle(ctx, docID, doc.Title, cleanedContent, contentType)

	if err := s.abortIfCanceled(ctx, docID, "before chunk"); err != nil {
		return err
	}

	// Chunk cleaned content instead of raw
	if err := s.chunkContent(docID, cleanedContent); err != nil {
		return err
	}

	// Emit event for action processing
	s.emitDocumentEvent(doc, contentType, result.MimeType)
	return nil
}

// IngestNote processes a markdown note: chunk and store.
func (s *IngestionService) IngestNote(ctx context.Context, docID string) error {
	release := s.lockDoc(docID)
	defer release()

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

	if err := s.chunkContent(docID, doc.RawContent); err != nil {
		return err
	}

	// Emit event for action processing
	s.emitDocumentEvent(doc, contentType, "text/markdown")
	return nil
}

// ReprocessDocument re-processes an existing document (delete old chunks, re-chunk).
func (s *IngestionService) ReprocessDocument(ctx context.Context, docID string) error {
	release := s.lockDoc(docID)
	defer release()

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
	var mimeType string

	// For uploaded files, re-extract
	if doc.SourceType == models.LogbookSourceUpload && doc.FilePath != "" {
		result, err := kreuzberg.ExtractFile(doc.FilePath)
		if err != nil {
			_ = s.repo.UpdateDocumentStatus(docID, models.LogbookDocStatusError, fmt.Sprintf("re-extraction failed: %v", err))
			return fmt.Errorf("re-extraction failed: %w", err)
		}
		content = result.Content
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
		return s.chunkContent(docID, "")
	}

	if err := s.abortIfCanceled(ctx, docID, "before classify"); err != nil {
		return err
	}

	// Classify and clean content
	contentType, cleanedContent := s.classifyAndClean(ctx, docID, doc.Title, content, mimeType)

	if err := s.abortIfCanceled(ctx, docID, "before article"); err != nil {
		return err
	}

	// Re-generate KB article based on classification
	s.generateArticle(ctx, docID, doc.Title, cleanedContent, contentType)

	if err := s.abortIfCanceled(ctx, docID, "before chunk"); err != nil {
		return err
	}

	return s.chunkContent(docID, cleanedContent)
}

// chunkContent splits text into chunks and stores them.
func (s *IngestionService) chunkContent(docID, content string) error {
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

// maxGeneratedArticleChars caps how much LLM-generated article text we store
// per document. MaxTokens on the LLM request is advisory and not enforced by
// every provider; a malicious or misbehaving endpoint could otherwise bloat
// the DB with arbitrary content. 64 KiB is generous for a KB article.
const maxGeneratedArticleChars = 64 * 1024

// maxCleanedContentChars caps the cleaned-content column. Input is already
// truncated to maxArticleContentChars before the LLM call, but the response
// is not trusted to honor any length contract.
const maxCleanedContentChars = 64 * 1024

func contentPreview(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// classifyAndClean uses the LLM to classify a document and clean its content.
// Internally runs two focused LLM calls: one for classification, one for cleaning.
// Returns the content type and cleaned content. If no LLM is available, returns empty type and original content.
func (s *IngestionService) classifyAndClean(ctx context.Context, docID, title, content, mimeType string) (contentType, cleanedContent string) {
	if s.articleClient == nil || !s.articleClient.Available() {
		return "", content
	}

	// Step 1: Classify with a focused, few-shot prompt
	contentType = s.classify(ctx, docID, title, content, mimeType)

	// Step 2: Clean content (skip for records — they don't get articles anyway)
	cleanedContent = content
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
func (s *IngestionService) classify(ctx context.Context, docID, title, content, mimeType string) string {
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

knowledge — documents containing substantive information: reports, guides, documentation, policies, specifications, analyses, procedures, standards, white papers, research, reference material
record — purely transactional or archival items with no informational value: invoices, receipts, purchase orders, shipping notifications, payment confirmations, automated system alerts
correspondence — person-to-person communication: emails, letters, memos, chat transcripts`,
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
		return models.LogbookContentTypeKnowledge
	}

	if len(resp.Choices) == 0 || resp.Choices[0].Message.Content == "" {
		slog.Warn("classification returned empty response", slog.String("doc_id", docID))
		return models.LogbookContentTypeKnowledge
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
	if len(cleaned) > maxCleanedContentChars {
		cleaned = cleaned[:maxCleanedContentChars]
	}
	slog.Debug("clean content response", slog.String("doc_id", docID), slog.Int("length", len(cleaned)))
	if cleaned == "" {
		return content
	}
	return cleaned
}

// parseClassificationType scans the LLM response for known classification keywords.
// Uses exact match first, then word-boundary matching to avoid substring false positives
// (e.g. "knowledge" inside "acknowledge"). Defaults to "knowledge" on unknown input.
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
	slog.Warn("unknown classification response, defaulting to knowledge", slog.String("raw", response))
	return models.LogbookContentTypeKnowledge
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
	if len(article) > maxGeneratedArticleChars {
		slog.Warn("LLM article exceeded cap, truncating",
			slog.String("doc_id", docID),
			slog.Int("got", len(article)),
			slog.Int("cap", maxGeneratedArticleChars),
		)
		article = article[:maxGeneratedArticleChars]
	}
	slog.Debug("generate article response", slog.String("doc_id", docID), slog.String("article", article))
	if err := s.repo.UpdateDocumentArticle(docID, article); err != nil {
		slog.Warn("failed to store generated article", slog.String("doc_id", docID), slog.Any("error", err))
		return
	}

	slog.Info("article generated", slog.String("doc_id", docID), slog.Int("length", len(article)))
}

// generateThumbnail attempts to create a thumbnail (600px) and preview (1200px) for the
// document. Failures are non-fatal.
func (s *IngestionService) generateThumbnail(docID, filePath, mimeType string) {
	thumbPath, previewPath, err := GenerateThumbnailAndPreview(docID, filePath, mimeType, filepath.Dir(filePath))
	if err != nil {
		slog.Warn("thumbnail generation failed", slog.String("doc_id", docID), slog.Any("error", err))
		return
	}
	if thumbPath == "" {
		return
	}
	if err := s.repo.UpdateDocumentThumbnailAndPreview(docID, thumbPath, previewPath); err != nil {
		slog.Warn("failed to store thumbnail/preview paths", slog.String("doc_id", docID), slog.Any("error", err))
	}
}

// estimateTokens provides a rough token count estimate (~4 chars per token).
func estimateTokens(text string) int {
	return len(text) / 4
}

// emitDocumentEvent sends a document event directly to the action service.
func (s *IngestionService) emitDocumentEvent(doc *models.LogbookDocument, contentType, mimeType string) {
	if s.actionService == nil {
		return
	}

	s.actionService.EmitEvent(&models.LogbookActionEvent{
		EventType:   models.LogbookTriggerDocumentClassified,
		BucketID:    doc.BucketID,
		DocumentID:  doc.ID,
		ActorUserID: doc.CreatedBy,
		ContentType: contentType,
		MimeType:    mimeType,
		Title:       doc.Title,
		SourceType:  doc.SourceType,
		Author:      doc.Author,
		RawContent:  contentPreview(doc.RawContent, 2000),
	})
}
