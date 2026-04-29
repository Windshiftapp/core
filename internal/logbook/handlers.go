package logbook

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"windshift/internal/models"
	"windshift/internal/restapi"

	"github.com/google/uuid"
)

// Handlers holds all HTTP handlers for the logbook system.
type Handlers struct {
	repo             *Repository
	permService      *PermissionService
	ingestionService *IngestionService
	storagePath      string

	// ingestCtx is the parent context threaded into async ingestion goroutines.
	// It is canceled on Shutdown so in-flight ingestion observes server stop.
	ingestCtx    context.Context
	ingestCancel context.CancelFunc
}

// NewHandlers creates a new set of logbook handlers.
func NewHandlers(repo *Repository, permService *PermissionService, ingestionService *IngestionService, storagePath string) *Handlers {
	ctx, cancel := context.WithCancel(context.Background())
	return &Handlers{
		repo:             repo,
		permService:      permService,
		ingestionService: ingestionService,
		storagePath:      storagePath,
		ingestCtx:        ctx,
		ingestCancel:     cancel,
	}
}

// Shutdown cancels background work started by the handlers (notably async
// ingestion). Safe to call more than once; subsequent calls are no-ops.
func (h *Handlers) Shutdown() {
	if h.ingestCancel != nil {
		h.ingestCancel()
	}
}

// runIngestion runs an ingestion function on a background goroutine with
// panic recovery. A panic in any ingestion path would otherwise kill the
// entire sidecar process, since a panic in a goroutine spawned from an HTTP
// handler is not caught by the server's recovery middleware. On panic the
// document is flagged as errored so it doesn't sit in 'processing' forever.
func (h *Handlers) runIngestion(docID, label string, fn func(ctx context.Context) error) {
	go func() {
		defer func() {
			if p := recover(); p != nil {
				slog.Error("async ingestion panicked",
					slog.String("stage", label),
					slog.String("doc_id", docID),
					slog.Any("panic", p),
				)
				_ = h.repo.UpdateDocumentStatus(docID, models.LogbookDocStatusError, fmt.Sprintf("%s panicked: %v", label, p))
			}
		}()
		if err := fn(h.ingestCtx); err != nil {
			slog.Error("async ingestion failed",
				slog.String("stage", label),
				slog.String("doc_id", docID),
				slog.Any("error", err),
			)
		}
	}()
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
	slog.Error("internal server error", //nolint:gosec // logging internal error for debugging
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

	// Fetch attachment settings and enforce enabled state
	settings, err := h.repo.GetAttachmentSettings()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !settings.Enabled {
		restapi.RespondErrorWithMessage(w, r, http.StatusServiceUnavailable, restapi.ErrCodeServiceUnavailable, "Attachments are disabled")
		return
	}

	// Limit request body size using configured max size and stream parts to
	// temp files at the multipart layer. The small memory threshold means
	// large files never land in RAM — they go straight through io.Copy into
	// our storage directory.
	r.Body = http.MaxBytesReader(w, r.Body, settings.MaxFileSize)
	if err := r.ParseMultipartForm(multipartMemoryThreshold); err != nil {
		restapi.RespondErrorWithMessage(w, r, http.StatusBadRequest, restapi.ErrCodeInvalidInput, "File too large or invalid form data")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		restapi.RespondErrorWithMessage(w, r, http.StatusBadRequest, restapi.ErrCodeInvalidInput, "File is required")
		return
	}
	defer file.Close()

	// Prepare destination directory up front so writeUploadToStorage can
	// rename into place without racing mkdir.
	docID := uuid.New().String()
	dstDir := filepath.Join(h.storagePath, bucketID, docID)
	if err := os.MkdirAll(dstDir, 0o750); err != nil { //nolint:gosec // G703: path components are validated UUIDs
		respondInternalError(w, r, err)
		return
	}

	stored, err := writeUploadToStorage(file, header.Filename, dstDir, settings)
	if err != nil {
		// Clean up the (empty) per-doc dir so a rejected upload doesn't
		// leave orphan directories behind.
		_ = os.Remove(dstDir) //nolint:gosec // G304/G703: dstDir is storagePath+validated-UUIDs
		respondUploadError(w, r, err)
		return
	}

	// Deduplicate after writing (cheap: at most MaxFileSize of disk per dup,
	// freed immediately). Hashing during stream means we can't look up
	// before opening the file, but the bytes are already on disk so we just
	// remove the duplicate copy.
	existing, err := h.repo.FindByContentHash(bucketID, stored.Hash)
	if err != nil {
		_ = os.Remove(stored.Path) //nolint:gosec // G703: stored.Path = dstDir + hex-random + validated-ext (see writeUploadToStorage)
		_ = os.Remove(dstDir)      //nolint:gosec // G304/G703: dstDir is storagePath+validated-UUIDs
		respondInternalError(w, r, err)
		return
	}
	if existing != nil {
		_ = os.Remove(stored.Path) //nolint:gosec // G703: stored.Path = dstDir + hex-random + validated-ext (see writeUploadToStorage)
		_ = os.Remove(dstDir)      //nolint:gosec // G304/G703: dstDir is storagePath+validated-UUIDs
		restapi.RespondJSON(w, http.StatusOK, existing)
		return
	}

	// Determine title
	title := r.FormValue("title")
	if title == "" {
		title = header.Filename
	}

	doc := &models.LogbookDocument{
		BucketID:    bucketID,
		Title:       title,
		SourceType:  models.LogbookSourceUpload,
		FilePath:    stored.Path,
		ContentHash: stored.Hash,
		Author:      r.FormValue("author"),
		Status:      models.LogbookDocStatusPending,
		CreatedBy:   lbUser.ID,
	}

	// CreateDocument can fail (constraint violation, connection drop, etc.).
	// Without cleanup, the on-disk file is orphaned — nothing else references
	// it since the DB row never existed. Remove the file + per-doc dir
	// before returning.
	if err := h.repo.CreateDocument(doc); err != nil {
		_ = os.Remove(stored.Path) //nolint:gosec // G703: stored.Path = dstDir + hex-random + validated-ext (see writeUploadToStorage)
		_ = os.Remove(dstDir)      //nolint:gosec // G304/G703: dstDir is storagePath+validated-UUIDs
		respondInternalError(w, r, err)
		return
	}

	h.runIngestion(doc.ID, "file", func(ctx context.Context) error {
		return h.ingestionService.IngestFile(ctx, doc.ID)
	})

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

	h.runIngestion(doc.ID, "note", func(ctx context.Context) error {
		return h.ingestionService.IngestNote(ctx, doc.ID)
	})

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

	h.runIngestion(docID, "reprocess", func(ctx context.Context) error {
		return h.ingestionService.ReprocessDocument(ctx, docID)
	})

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

	// Remove on-disk file artifacts so archived documents don't linger on
	// the volume (GDPR "right to be forgotten" and general hygiene). The DB
	// row is kept with archived_at set for audit, but the binary content
	// does not need to persist. Best-effort: an error here is logged, not
	// surfaced, because the archive itself succeeded.
	h.purgeDocumentFiles(doc)

	restapi.RespondNoContent(w)
}

// purgeDocumentFiles best-effort removes the per-document storage directory,
// which holds the file, thumbnail, preview, and any attachments.
func (h *Handlers) purgeDocumentFiles(doc *models.LogbookDocument) {
	if doc == nil || doc.BucketID == "" || doc.ID == "" {
		return
	}
	if !isValidUUID(doc.BucketID) || !isValidUUID(doc.ID) {
		// Defensive: refuse to recurse into something that doesn't match the
		// known layout, so corrupted rows can never widen the removal.
		return
	}
	dir := filepath.Join(h.storagePath, doc.BucketID, doc.ID)
	within, err := h.isWithinStorage(dir)
	if err != nil || !within {
		return
	}
	if err := os.RemoveAll(dir); err != nil { //nolint:gosec // G703: dir = storagePath + validated UUIDs, isWithinStorage-checked above
		slog.Warn("failed to purge archived document files",
			slog.String("doc_id", doc.ID),
			slog.Any("error", err),
		)
	}
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

	// Optional filter by customer organisation
	if custOrgStr := r.URL.Query().Get("customer_organisation_id"); custOrgStr != "" {
		custOrgID, err := strconv.Atoi(custOrgStr)
		if err != nil {
			restapi.RespondErrorWithMessage(w, r, http.StatusBadRequest, restapi.ErrCodeInvalidInput, "Invalid customer_organisation_id")
			return
		}
		docs, total, err := h.repo.ListDocumentsByCustomerOrg(ids, custOrgID, params.Limit, params.Offset)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		pagination := restapi.NewPaginationMeta(params, total)
		restapi.RespondPaginated(w, docs, pagination)
		return
	}

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

	ok, err = h.isWithinStorage(doc.ThumbnailPath)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !ok {
		respondNotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=31536000")
	http.ServeFile(w, r, doc.ThumbnailPath)
}

// GetDocumentPreview serves the larger (1200px) preview image for a document.
func (h *Handlers) GetDocumentPreview(w http.ResponseWriter, r *http.Request) {
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

	if !doc.HasPreview || doc.PreviewPath == "" {
		respondNotFound(w, r)
		return
	}

	ok, err = h.isWithinStorage(doc.PreviewPath)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !ok {
		respondNotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=31536000")
	http.ServeFile(w, r, doc.PreviewPath)
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

	ok, err = h.isWithinStorage(doc.FilePath)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !ok {
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

	// Force download (attachment) rather than inline render. Even with the
	// upload allowlist, serving user content inline on the same origin would
	// mean any bypass ends in an XSS; attachment disposition removes that
	// entire class.
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("X-Content-Type-Options", "nosniff")
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

	// Fetch attachment settings and enforce enabled state
	settings, err := h.repo.GetAttachmentSettings()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !settings.Enabled {
		restapi.RespondErrorWithMessage(w, r, http.StatusServiceUnavailable, restapi.ErrCodeServiceUnavailable, "Attachments are disabled")
		return
	}

	// Stream parts through; see UploadDocument for rationale on the memory threshold.
	r.Body = http.MaxBytesReader(w, r.Body, settings.MaxFileSize)
	if err := r.ParseMultipartForm(multipartMemoryThreshold); err != nil {
		restapi.RespondErrorWithMessage(w, r, http.StatusBadRequest, restapi.ErrCodeInvalidInput, "File too large or invalid form data")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		restapi.RespondErrorWithMessage(w, r, http.StatusBadRequest, restapi.ErrCodeInvalidInput, "File is required")
		return
	}
	defer file.Close()

	dir := filepath.Join(h.storagePath, doc.BucketID, docID)
	if err := os.MkdirAll(dir, 0o750); err != nil { //nolint:gosec // G703: path components are UUID-generated
		respondInternalError(w, r, err)
		return
	}

	stored, err := writeUploadToStorage(file, header.Filename, dir, settings)
	if err != nil {
		respondUploadError(w, r, err)
		return
	}

	att := &models.LogbookAttachment{
		DocumentID:       docID,
		BucketID:         doc.BucketID,
		Filename:         filepath.Base(stored.Path),
		OriginalFilename: header.Filename,
		FilePath:         stored.Path,
		MimeType:         stored.MimeType,
		FileSize:         stored.Size,
		UploadedBy:       lbUser.ID,
	}

	attID, err := h.repo.CreateAttachment(att)
	if err != nil {
		// DB insert failed — remove the file we just wrote so it doesn't
		// linger as an unreachable blob. The parent dir (shared with the
		// document) is left alone.
		_ = os.Remove(stored.Path) //nolint:gosec // G703: stored.Path = attachment dir + hex-random + validated-ext (see writeUploadToStorage)
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

	within, err := h.isWithinStorage(att.FilePath)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !within {
		respondNotFound(w, r)
		return
	}

	contentType := att.MimeType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", att.OriginalFilename))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=31536000")
	http.ServeFile(w, r, att.FilePath)
}

// --- Helpers ---

func isValidUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

// isWithinStorage reports whether filePath resolves to a location under the
// configured storage root. Used before http.ServeFile to ensure a corrupted
// or restored DB row can't be turned into arbitrary file read. Returns an
// internal error on filesystem failure so callers can distinguish that from
// a simple mismatch.
func (h *Handlers) isWithinStorage(filePath string) (ok bool, err error) {
	absFilePath, err := filepath.Abs(filePath)
	if err != nil {
		return false, err
	}
	absStoragePath, err := filepath.Abs(h.storagePath)
	if err != nil {
		return false, err
	}
	return strings.HasPrefix(absFilePath, absStoragePath+string(filepath.Separator)), nil
}
