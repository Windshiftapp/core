package logbook

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"windshift/internal/models"
	"windshift/internal/restapi"

	"github.com/google/uuid"
)

const maxUploadSize = 64 << 20 // 64 MB

// Handlers holds all HTTP handlers for the logbook system.
type Handlers struct {
	repo             *Repository
	permService      *PermissionService
	ingestionService *IngestionService
	storagePath      string
}

// NewHandlers creates a new set of logbook handlers.
func NewHandlers(repo *Repository, permService *PermissionService, ingestionService *IngestionService, storagePath string) *Handlers {
	return &Handlers{
		repo:             repo,
		permService:      permService,
		ingestionService: ingestionService,
		storagePath:      storagePath,
	}
}

// --- Auth helpers ---

func requireLogbookAuth(w http.ResponseWriter, r *http.Request) (*LogbookUser, bool) {
	lbUser := GetLogbookUser(r)
	if lbUser == nil {
		restapi.RespondError(w, r, restapi.ErrUnauthorized)
		return nil, false
	}
	return lbUser, true
}

func respondNotFound(w http.ResponseWriter, r *http.Request) {
	restapi.RespondError(w, r, restapi.ErrNotFound)
}

func respondInternalError(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("internal server error",
		slog.Any("error", err),
		slog.String("path", r.URL.Path),
		slog.String("method", r.Method),
	)
	restapi.RespondError(w, r, restapi.ErrInternalError)
}

// --- Bucket Handlers ---

// GetBuckets lists buckets the current user can view.
func (h *Handlers) GetBuckets(w http.ResponseWriter, r *http.Request) {
	lbUser, ok := requireLogbookAuth(w, r)
	if !ok {
		return
	}

	ids, err := h.permService.GetAccessibleBucketIDs(lbUser.ID, lbUser.IsAdmin, lbUser.GroupIDs)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	buckets, err := h.repo.ListBucketsForUser(ids)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	restapi.RespondOK(w, buckets)
}

// CreateBucket creates a new bucket. Requires system admin.
func (h *Handlers) CreateBucket(w http.ResponseWriter, r *http.Request) {
	lbUser, ok := requireLogbookAuth(w, r)
	if !ok {
		return
	}

	if !lbUser.IsAdmin {
		restapi.RespondError(w, r, restapi.ErrAdminRequired)
		return
	}

	var req models.LogbookBucketCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		restapi.RespondErrorWithMessage(w, r, http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid request body")
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		restapi.RespondErrorWithMessage(w, r, http.StatusBadRequest, restapi.ErrCodeValidationFailed, "Bucket name is required")
		return
	}

	bucket, err := h.repo.CreateBucket(req, lbUser.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	restapi.RespondCreated(w, bucket)
}

// GetBucket returns a single bucket. Returns 404 on unauthorized (security policy).
func (h *Handlers) GetBucket(w http.ResponseWriter, r *http.Request) {
	lbUser, ok := requireLogbookAuth(w, r)
	if !ok {
		return
	}

	bucketID := r.PathValue("bucketID")
	if !isValidUUID(bucketID) {
		respondNotFound(w, r)
		return
	}

	has, err := h.permService.HasBucketPermission(lbUser.ID, lbUser.IsAdmin, lbUser.GroupIDs, bucketID, models.LogbookPermissionBucketView)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !has {
		respondNotFound(w, r) // 404 not 403 — prevents bucket existence leakage
		return
	}

	bucket, err := h.repo.GetBucket(bucketID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if bucket == nil {
		respondNotFound(w, r)
		return
	}

	restapi.RespondOK(w, bucket)
}

// UpdateBucket updates a bucket. Requires bucket.admin.
func (h *Handlers) UpdateBucket(w http.ResponseWriter, r *http.Request) {
	lbUser, ok := requireLogbookAuth(w, r)
	if !ok {
		return
	}

	bucketID := r.PathValue("bucketID")
	if !isValidUUID(bucketID) {
		respondNotFound(w, r)
		return
	}

	has, err := h.permService.HasBucketPermission(lbUser.ID, lbUser.IsAdmin, lbUser.GroupIDs, bucketID, models.LogbookPermissionBucketAdmin)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !has {
		respondNotFound(w, r)
		return
	}

	var req models.LogbookBucketUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		restapi.RespondErrorWithMessage(w, r, http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid request body")
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		restapi.RespondErrorWithMessage(w, r, http.StatusBadRequest, restapi.ErrCodeValidationFailed, "Bucket name is required")
		return
	}

	if err := h.repo.UpdateBucket(bucketID, req); err != nil {
		respondInternalError(w, r, err)
		return
	}

	bucket, err := h.repo.GetBucket(bucketID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	restapi.RespondOK(w, bucket)
}

// DeleteBucket deletes a bucket. Requires bucket.admin.
func (h *Handlers) DeleteBucket(w http.ResponseWriter, r *http.Request) {
	lbUser, ok := requireLogbookAuth(w, r)
	if !ok {
		return
	}

	bucketID := r.PathValue("bucketID")
	if !isValidUUID(bucketID) {
		respondNotFound(w, r)
		return
	}

	has, err := h.permService.HasBucketPermission(lbUser.ID, lbUser.IsAdmin, lbUser.GroupIDs, bucketID, models.LogbookPermissionBucketAdmin)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !has {
		respondNotFound(w, r)
		return
	}

	if err := h.repo.DeleteBucket(bucketID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	restapi.RespondNoContent(w)
}

// --- Bucket Permission Handlers ---

// GetBucketPermissions lists permissions for a bucket. Requires bucket.admin.
func (h *Handlers) GetBucketPermissions(w http.ResponseWriter, r *http.Request) {
	lbUser, ok := requireLogbookAuth(w, r)
	if !ok {
		return
	}

	bucketID := r.PathValue("bucketID")
	if !isValidUUID(bucketID) {
		respondNotFound(w, r)
		return
	}

	has, err := h.permService.HasBucketPermission(lbUser.ID, lbUser.IsAdmin, lbUser.GroupIDs, bucketID, models.LogbookPermissionBucketAdmin)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !has {
		respondNotFound(w, r)
		return
	}

	perms, err := h.repo.ListBucketPermissions(bucketID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	restapi.RespondOK(w, perms)
}

// SetBucketPermissions replaces all permissions for a bucket. Requires bucket.admin.
func (h *Handlers) SetBucketPermissions(w http.ResponseWriter, r *http.Request) {
	lbUser, ok := requireLogbookAuth(w, r)
	if !ok {
		return
	}

	bucketID := r.PathValue("bucketID")
	if !isValidUUID(bucketID) {
		respondNotFound(w, r)
		return
	}

	has, err := h.permService.HasBucketPermission(lbUser.ID, lbUser.IsAdmin, lbUser.GroupIDs, bucketID, models.LogbookPermissionBucketAdmin)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !has {
		respondNotFound(w, r)
		return
	}

	var req models.LogbookSetPermissionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		restapi.RespondErrorWithMessage(w, r, http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid request body")
		return
	}

	// Validate permissions
	for _, p := range req.Permissions {
		if p.PrincipalType != "user" && p.PrincipalType != "group" {
			restapi.RespondErrorWithMessage(w, r, http.StatusBadRequest, restapi.ErrCodeValidationFailed, "principal_type must be 'user' or 'group'")
			return
		}
		switch p.Permission {
		case models.LogbookPermissionBucketView, models.LogbookPermissionBucketEdit, models.LogbookPermissionBucketAdmin:
			// valid
		default:
			restapi.RespondErrorWithMessage(w, r, http.StatusBadRequest, restapi.ErrCodeValidationFailed, "Invalid permission: "+p.Permission)
			return
		}
	}

	if err := h.repo.SetBucketPermissions(bucketID, req.Permissions); err != nil {
		respondInternalError(w, r, err)
		return
	}

	perms, err := h.repo.ListBucketPermissions(bucketID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	restapi.RespondOK(w, perms)
}

// --- Document Handlers ---

// UploadDocument handles multipart file upload. Returns 202 Accepted.
func (h *Handlers) UploadDocument(w http.ResponseWriter, r *http.Request) {
	lbUser, ok := requireLogbookAuth(w, r)
	if !ok {
		return
	}

	bucketID := r.PathValue("bucketID")
	if !isValidUUID(bucketID) {
		respondNotFound(w, r)
		return
	}

	has, err := h.permService.HasBucketPermission(lbUser.ID, lbUser.IsAdmin, lbUser.GroupIDs, bucketID, models.LogbookPermissionBucketEdit)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !has {
		respondNotFound(w, r)
		return
	}

	// Parse multipart form
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		restapi.RespondErrorWithMessage(w, r, http.StatusBadRequest, restapi.ErrCodeInvalidInput, "File too large or invalid form data")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		restapi.RespondErrorWithMessage(w, r, http.StatusBadRequest, restapi.ErrCodeInvalidInput, "File is required")
		return
	}
	defer file.Close()

	// Read file content for hashing
	content, err := io.ReadAll(file)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Compute content hash for deduplication
	hash := fmt.Sprintf("%x", sha256.Sum256(content))
	existing, err := h.repo.FindByContentHash(bucketID, hash)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if existing != nil {
		restapi.RespondJSON(w, http.StatusOK, existing)
		return
	}

	// Generate document ID for storage path
	docID := uuid.New().String()

	// Save file to disk
	storagePath := filepath.Join(h.storagePath, bucketID, docID)
	if err := os.MkdirAll(storagePath, 0o750); err != nil { //nolint:gosec // G703: path components are validated UUIDs
		respondInternalError(w, r, err)
		return
	}
	filePath := filepath.Join(storagePath, filepath.Base(header.Filename))
	if err := os.WriteFile(filePath, content, 0o600); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Determine title
	title := r.FormValue("title")
	if title == "" {
		title = header.Filename
	}

	doc := &models.LogbookDocument{
		BucketID:   bucketID,
		Title:      title,
		SourceType: models.LogbookSourceUpload,
		FilePath:   filePath,
		Author:     r.FormValue("author"),
		Status:     models.LogbookDocStatusPending,
		CreatedBy:  lbUser.ID,
	}

	if err := h.repo.CreateDocument(doc); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Async ingestion
	go func() {
		ctx := context.Background()
		if err := h.ingestionService.IngestFile(ctx, doc.ID); err != nil {
			slog.Error("async file ingestion failed",
				slog.String("doc_id", doc.ID),
				slog.Any("error", err),
			)
		}
	}()

	restapi.RespondJSON(w, http.StatusAccepted, doc)
}

// CreateNote creates a text note document. Returns 202 Accepted.
func (h *Handlers) CreateNote(w http.ResponseWriter, r *http.Request) {
	lbUser, ok := requireLogbookAuth(w, r)
	if !ok {
		return
	}

	bucketID := r.PathValue("bucketID")
	if !isValidUUID(bucketID) {
		respondNotFound(w, r)
		return
	}

	has, err := h.permService.HasBucketPermission(lbUser.ID, lbUser.IsAdmin, lbUser.GroupIDs, bucketID, models.LogbookPermissionBucketEdit)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !has {
		respondNotFound(w, r)
		return
	}

	var req models.LogbookNoteCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		restapi.RespondErrorWithMessage(w, r, http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid request body")
		return
	}

	if strings.TrimSpace(req.Title) == "" {
		restapi.RespondErrorWithMessage(w, r, http.StatusBadRequest, restapi.ErrCodeValidationFailed, "Title is required")
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		restapi.RespondErrorWithMessage(w, r, http.StatusBadRequest, restapi.ErrCodeValidationFailed, "Content is required")
		return
	}

	doc := &models.LogbookDocument{
		BucketID:   bucketID,
		Title:      req.Title,
		SourceType: models.LogbookSourceNote,
		RawContent: req.Content,
		Author:     req.Author,
		Status:     models.LogbookDocStatusPending,
		CreatedBy:  lbUser.ID,
	}

	if err := h.repo.CreateDocument(doc); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Async ingestion
	go func() {
		ctx := context.Background()
		if err := h.ingestionService.IngestNote(ctx, doc.ID); err != nil {
			slog.Error("async note ingestion failed",
				slog.String("doc_id", doc.ID),
				slog.Any("error", err),
			)
		}
	}()

	restapi.RespondJSON(w, http.StatusAccepted, doc)
}

// GetDocument returns a single document. Returns 404 on unauthorized.
func (h *Handlers) GetDocument(w http.ResponseWriter, r *http.Request) {
	lbUser, ok := requireLogbookAuth(w, r)
	if !ok {
		return
	}

	docID := r.PathValue("documentID")
	if !isValidUUID(docID) {
		respondNotFound(w, r)
		return
	}

	doc, err := h.repo.GetDocument(docID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if doc == nil {
		respondNotFound(w, r)
		return
	}

	has, err := h.permService.HasBucketPermission(lbUser.ID, lbUser.IsAdmin, lbUser.GroupIDs, doc.BucketID, models.LogbookPermissionBucketView)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !has {
		respondNotFound(w, r) // 404 not 403
		return
	}

	restapi.RespondOK(w, doc)
}

// UpdateDocument updates a document and triggers reprocessing.
func (h *Handlers) UpdateDocument(w http.ResponseWriter, r *http.Request) {
	lbUser, ok := requireLogbookAuth(w, r)
	if !ok {
		return
	}

	docID := r.PathValue("documentID")
	if !isValidUUID(docID) {
		respondNotFound(w, r)
		return
	}

	doc, err := h.repo.GetDocument(docID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if doc == nil {
		respondNotFound(w, r)
		return
	}

	has, err := h.permService.HasBucketPermission(lbUser.ID, lbUser.IsAdmin, lbUser.GroupIDs, doc.BucketID, models.LogbookPermissionBucketEdit)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !has {
		respondNotFound(w, r)
		return
	}

	var req models.LogbookDocumentUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		restapi.RespondErrorWithMessage(w, r, http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid request body")
		return
	}

	if strings.TrimSpace(req.Title) == "" {
		restapi.RespondErrorWithMessage(w, r, http.StatusBadRequest, restapi.ErrCodeValidationFailed, "Title is required")
		return
	}

	// Direct save path for notes: when article is provided, save directly without reprocessing
	if req.Article != "" {
		if err := h.repo.SaveNoteDirectly(docID, req.Title, req.Article); err != nil {
			respondInternalError(w, r, err)
			return
		}
		updated, _ := h.repo.GetDocument(docID)
		restapi.RespondOK(w, updated)
		return
	}

	content := req.Content
	if content == "" {
		content = doc.RawContent
	}

	if err := h.repo.UpdateDocument(docID, req.Title, content); err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Async reprocessing
	go func() {
		ctx := context.Background()
		if err := h.ingestionService.ReprocessDocument(ctx, docID); err != nil {
			slog.Error("async document reprocessing failed",
				slog.String("doc_id", docID),
				slog.Any("error", err),
			)
		}
	}()

	updated, _ := h.repo.GetDocument(docID)
	restapi.RespondOK(w, updated)
}

// ArchiveDocument soft-deletes a document. Requires bucket.edit.
func (h *Handlers) ArchiveDocument(w http.ResponseWriter, r *http.Request) {
	lbUser, ok := requireLogbookAuth(w, r)
	if !ok {
		return
	}

	docID := r.PathValue("documentID")
	if !isValidUUID(docID) {
		respondNotFound(w, r)
		return
	}

	doc, err := h.repo.GetDocument(docID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if doc == nil {
		respondNotFound(w, r)
		return
	}

	has, err := h.permService.HasBucketPermission(lbUser.ID, lbUser.IsAdmin, lbUser.GroupIDs, doc.BucketID, models.LogbookPermissionBucketEdit)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !has {
		respondNotFound(w, r)
		return
	}

	if err := h.repo.ArchiveDocument(docID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	restapi.RespondNoContent(w)
}

// ListDocuments returns paginated documents for a bucket.
func (h *Handlers) ListDocuments(w http.ResponseWriter, r *http.Request) {
	lbUser, ok := requireLogbookAuth(w, r)
	if !ok {
		return
	}

	bucketID := r.PathValue("bucketID")
	if !isValidUUID(bucketID) {
		respondNotFound(w, r)
		return
	}

	has, err := h.permService.HasBucketPermission(lbUser.ID, lbUser.IsAdmin, lbUser.GroupIDs, bucketID, models.LogbookPermissionBucketView)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !has {
		respondNotFound(w, r)
		return
	}

	params := restapi.ParsePaginationParams(r)
	docs, total, err := h.repo.ListDocuments(bucketID, params.Limit, params.Offset)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	pagination := restapi.NewPaginationMeta(params, total)
	restapi.RespondPaginated(w, docs, pagination)
}

// ListAllDocuments returns paginated documents across all accessible buckets.
func (h *Handlers) ListAllDocuments(w http.ResponseWriter, r *http.Request) {
	lbUser, ok := requireLogbookAuth(w, r)
	if !ok {
		return
	}

	ids, err := h.permService.GetAccessibleBucketIDs(lbUser.ID, lbUser.IsAdmin, lbUser.GroupIDs)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	params := restapi.ParsePaginationParams(r)
	docs, total, err := h.repo.ListAllDocuments(ids, params.Limit, params.Offset)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	pagination := restapi.NewPaginationMeta(params, total)
	restapi.RespondPaginated(w, docs, pagination)
}

// --- Search Handlers ---

// KeywordSearch performs full-text search across accessible buckets.
func (h *Handlers) KeywordSearch(w http.ResponseWriter, r *http.Request) {
	lbUser, ok := requireLogbookAuth(w, r)
	if !ok {
		return
	}

	query := r.URL.Query().Get("q")
	if strings.TrimSpace(query) == "" {
		restapi.RespondErrorWithMessage(w, r, http.StatusBadRequest, restapi.ErrCodeValidationFailed, "Search query 'q' is required")
		return
	}

	ids, err := h.permService.GetAccessibleBucketIDs(lbUser.ID, lbUser.IsAdmin, lbUser.GroupIDs)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	params := restapi.ParsePaginationParams(r)
	results, total, err := h.repo.KeywordSearch(query, ids, params.Limit, params.Offset)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	pagination := restapi.NewPaginationMeta(params, total)
	restapi.RespondPaginated(w, results, pagination)
}

// --- Thumbnail Handler ---

// GetDocumentThumbnail serves the thumbnail image for a document.
func (h *Handlers) GetDocumentThumbnail(w http.ResponseWriter, r *http.Request) {
	lbUser, ok := requireLogbookAuth(w, r)
	if !ok {
		return
	}

	docID := r.PathValue("documentID")
	if !isValidUUID(docID) {
		respondNotFound(w, r)
		return
	}

	doc, err := h.repo.GetDocument(docID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if doc == nil {
		respondNotFound(w, r)
		return
	}

	has, err := h.permService.HasBucketPermission(lbUser.ID, lbUser.IsAdmin, lbUser.GroupIDs, doc.BucketID, models.LogbookPermissionBucketView)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !has {
		respondNotFound(w, r) // 404 not 403
		return
	}

	if !doc.HasThumbnail || doc.ThumbnailPath == "" {
		respondNotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=31536000")
	http.ServeFile(w, r, doc.ThumbnailPath)
}

// --- File Download Handler ---

// GetDocumentFile serves the original uploaded file for a document.
func (h *Handlers) GetDocumentFile(w http.ResponseWriter, r *http.Request) {
	lbUser, ok := requireLogbookAuth(w, r)
	if !ok {
		return
	}

	docID := r.PathValue("documentID")
	if !isValidUUID(docID) {
		respondNotFound(w, r)
		return
	}

	doc, err := h.repo.GetDocument(docID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if doc == nil {
		respondNotFound(w, r)
		return
	}

	has, err := h.permService.HasBucketPermission(lbUser.ID, lbUser.IsAdmin, lbUser.GroupIDs, doc.BucketID, models.LogbookPermissionBucketView)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !has {
		respondNotFound(w, r) // 404 not 403
		return
	}

	if doc.FilePath == "" {
		respondNotFound(w, r)
		return
	}

	// Determine content type
	contentType := doc.MimeType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Determine filename from title + extension
	filename := doc.Title
	if ext := filepath.Ext(doc.FilePath); ext != "" && filepath.Ext(filename) == "" {
		filename += ext
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", filename))
	http.ServeFile(w, r, doc.FilePath)
}

// --- Attachment Handlers ---

// UploadAttachment handles file upload for a logbook document.
func (h *Handlers) UploadAttachment(w http.ResponseWriter, r *http.Request) {
	lbUser, ok := requireLogbookAuth(w, r)
	if !ok {
		return
	}

	docID := r.PathValue("documentID")
	if !isValidUUID(docID) {
		respondNotFound(w, r)
		return
	}

	doc, err := h.repo.GetDocument(docID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if doc == nil {
		respondNotFound(w, r)
		return
	}

	has, err := h.permService.HasBucketPermission(lbUser.ID, lbUser.IsAdmin, lbUser.GroupIDs, doc.BucketID, models.LogbookPermissionBucketEdit)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !has {
		respondNotFound(w, r)
		return
	}

	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		restapi.RespondErrorWithMessage(w, r, http.StatusBadRequest, restapi.ErrCodeInvalidInput, "File too large or invalid form data")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		restapi.RespondErrorWithMessage(w, r, http.StatusBadRequest, restapi.ErrCodeInvalidInput, "File is required")
		return
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Generate UUID-based filename preserving extension
	ext := filepath.Ext(header.Filename)
	storedFilename := uuid.New().String() + ext

	// Store at {storagePath}/{bucketID}/{documentID}/{filename}
	dir := filepath.Join(h.storagePath, doc.BucketID, docID)
	if err := os.MkdirAll(dir, 0o750); err != nil { //nolint:gosec // G703: filename is uuid-generated, not user input
		respondInternalError(w, r, err)
		return
	}
	filePath := filepath.Join(dir, storedFilename)
	if err := os.WriteFile(filePath, content, 0o600); err != nil {
		respondInternalError(w, r, err)
		return
	}

	att := &models.LogbookAttachment{
		DocumentID:       docID,
		BucketID:         doc.BucketID,
		Filename:         storedFilename,
		OriginalFilename: header.Filename,
		FilePath:         filePath,
		MimeType:         header.Header.Get("Content-Type"),
		FileSize:         int64(len(content)),
		UploadedBy:       lbUser.ID,
	}

	attID, err := h.repo.CreateAttachment(att)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	att.ID = attID
	att.DownloadURL = fmt.Sprintf("/api/logbook/attachments/%s/download", attID)

	restapi.RespondJSON(w, http.StatusCreated, map[string]interface{}{
		"success":    true,
		"attachment": att,
	})
}

// DownloadAttachment serves a logbook attachment file.
func (h *Handlers) DownloadAttachment(w http.ResponseWriter, r *http.Request) {
	lbUser, ok := requireLogbookAuth(w, r)
	if !ok {
		return
	}

	attID := r.PathValue("attachmentID")
	if !isValidUUID(attID) {
		respondNotFound(w, r)
		return
	}

	att, err := h.repo.GetAttachment(attID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if att == nil {
		respondNotFound(w, r)
		return
	}

	has, err := h.permService.HasBucketPermission(lbUser.ID, lbUser.IsAdmin, lbUser.GroupIDs, att.BucketID, models.LogbookPermissionBucketView)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !has {
		respondNotFound(w, r)
		return
	}

	// Path traversal protection: ensure file is within storage directory
	absFilePath, err := filepath.Abs(att.FilePath)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	absStoragePath, err := filepath.Abs(h.storagePath)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !strings.HasPrefix(absFilePath, absStoragePath+string(filepath.Separator)) {
		respondNotFound(w, r)
		return
	}

	contentType := att.MimeType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", att.OriginalFilename))
	w.Header().Set("Cache-Control", "public, max-age=31536000")
	http.ServeFile(w, r, att.FilePath)
}

// --- Helpers ---

func isValidUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}
