package logbook

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"

	"github.com/lib/pq"
)

// Repository handles data access for the logbook system.
type Repository struct {
	db database.Database
}

// NewRepository creates a new logbook repository.
func NewRepository(db database.Database) *Repository {
	return &Repository{db: db}
}

// --- Buckets ---

// ListBucketsForUser returns buckets the user has access to.
func (r *Repository) ListBucketsForUser(accessibleBucketIDs []string) ([]models.LogbookBucket, error) {
	if len(accessibleBucketIDs) == 0 {
		return []models.LogbookBucket{}, nil
	}

	placeholders, args := buildStringPlaceholders(accessibleBucketIDs)
	query := fmt.Sprintf(`
		SELECT b.id, b.name, b.description, b.workspace_id, b.created_by,
		       b.created_at, b.updated_at, b.max_age_days, b.approval_required,
		       b.portal_visible, b.email_address, b.default_authority,
		       (SELECT COUNT(*) FROM logbook_documents d WHERE d.bucket_id = b.id AND d.archived_at IS NULL) as document_count
		FROM logbook_buckets b
		WHERE b.id IN (%s)
		ORDER BY b.name ASC
	`, placeholders)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list buckets: %w", err)
	}
	defer rows.Close()

	var buckets []models.LogbookBucket
	for rows.Next() {
		var b models.LogbookBucket
		err := rows.Scan(
			&b.ID, &b.Name, &b.Description, &b.WorkspaceID, &b.CreatedBy,
			&b.CreatedAt, &b.UpdatedAt, &b.MaxAgeDays, &b.ApprovalRequired,
			&b.PortalVisible, &b.EmailAddress, &b.DefaultAuthority,
			&b.DocumentCount,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan bucket: %w", err)
		}
		buckets = append(buckets, b)
	}
	return buckets, rows.Err()
}

// GetBucket returns a single bucket by ID.
func (r *Repository) GetBucket(id string) (*models.LogbookBucket, error) {
	var b models.LogbookBucket
	err := r.db.QueryRow(`
		SELECT b.id, b.name, b.description, b.workspace_id, b.created_by,
		       b.created_at, b.updated_at, b.max_age_days, b.approval_required,
		       b.portal_visible, b.email_address, b.default_authority,
		       (SELECT COUNT(*) FROM logbook_documents d WHERE d.bucket_id = b.id AND d.archived_at IS NULL) as document_count
		FROM logbook_buckets b
		WHERE b.id = $1
	`, id).Scan(
		&b.ID, &b.Name, &b.Description, &b.WorkspaceID, &b.CreatedBy,
		&b.CreatedAt, &b.UpdatedAt, &b.MaxAgeDays, &b.ApprovalRequired,
		&b.PortalVisible, &b.EmailAddress, &b.DefaultAuthority,
		&b.DocumentCount,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get bucket: %w", err)
	}
	return &b, nil
}

// CreateBucket inserts a new bucket and returns it.
func (r *Repository) CreateBucket(req models.LogbookBucketCreateRequest, userID int) (*models.LogbookBucket, error) {
	var b models.LogbookBucket
	err := r.db.QueryRow(`
		INSERT INTO logbook_buckets (name, description, workspace_id, created_by, max_age_days,
		    approval_required, portal_visible, email_address, default_authority)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, name, description, workspace_id, created_by, created_at, updated_at,
		    max_age_days, approval_required, portal_visible, email_address, default_authority
	`, req.Name, req.Description, req.WorkspaceID, userID, req.MaxAgeDays,
		req.ApprovalRequired, req.PortalVisible, req.EmailAddress, req.DefaultAuthority,
	).Scan(
		&b.ID, &b.Name, &b.Description, &b.WorkspaceID, &b.CreatedBy,
		&b.CreatedAt, &b.UpdatedAt, &b.MaxAgeDays, &b.ApprovalRequired,
		&b.PortalVisible, &b.EmailAddress, &b.DefaultAuthority,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create bucket: %w", err)
	}
	return &b, nil
}

// UpdateBucket updates a bucket.
func (r *Repository) UpdateBucket(id string, req models.LogbookBucketUpdateRequest) error {
	_, err := r.db.ExecWrite(`
		UPDATE logbook_buckets
		SET name = $1, description = $2, max_age_days = $3,
		    approval_required = COALESCE($4, approval_required),
		    portal_visible = COALESCE($5, portal_visible),
		    email_address = $6, default_authority = $7, updated_at = $8
		WHERE id = $9
	`, req.Name, req.Description, req.MaxAgeDays,
		req.ApprovalRequired, req.PortalVisible,
		req.EmailAddress, req.DefaultAuthority, time.Now(), id,
	)
	if err != nil {
		return fmt.Errorf("failed to update bucket: %w", err)
	}
	return nil
}

// DeleteBucket deletes a bucket (cascades to permissions, documents, chunks).
func (r *Repository) DeleteBucket(id string) error {
	_, err := r.db.ExecWrite(`DELETE FROM logbook_buckets WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete bucket: %w", err)
	}
	return nil
}

// --- Permissions ---

// ListBucketPermissions returns all permissions for a bucket.
func (r *Repository) ListBucketPermissions(bucketID string) ([]models.LogbookBucketPermission, error) {
	rows, err := r.db.Query(`
		SELECT p.id, p.bucket_id, p.principal_type, p.principal_id, p.permission
		FROM logbook_bucket_permissions p
		WHERE p.bucket_id = $1
		ORDER BY p.principal_type, p.principal_id
	`, bucketID)
	if err != nil {
		return nil, fmt.Errorf("failed to list bucket permissions: %w", err)
	}
	defer rows.Close()

	var perms []models.LogbookBucketPermission
	for rows.Next() {
		var p models.LogbookBucketPermission
		if err := rows.Scan(&p.ID, &p.BucketID, &p.PrincipalType, &p.PrincipalID, &p.Permission); err != nil {
			return nil, fmt.Errorf("failed to scan permission: %w", err)
		}
		perms = append(perms, p)
	}
	return perms, rows.Err()
}

// SetBucketPermissions replaces all permissions for a bucket.
func (r *Repository) SetBucketPermissions(bucketID string, perms []models.LogbookBucketPermission) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Delete existing permissions
	if _, err := tx.Exec(`DELETE FROM logbook_bucket_permissions WHERE bucket_id = $1`, bucketID); err != nil {
		return fmt.Errorf("failed to clear permissions: %w", err)
	}

	// Insert new permissions
	for _, p := range perms {
		if _, err := tx.Exec(`
			INSERT INTO logbook_bucket_permissions (bucket_id, principal_type, principal_id, permission)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (bucket_id, principal_type, principal_id, permission) DO NOTHING
		`, bucketID, p.PrincipalType, p.PrincipalID, p.Permission); err != nil {
			return fmt.Errorf("failed to insert permission: %w", err)
		}
	}

	return tx.Commit()
}

// HasBucketPermission checks if a user has a specific permission on a bucket.
// Resolution: direct user permission → group membership (using provided groupIDs).
func (r *Repository) HasBucketPermission(userID int, groupIDs []int, bucketID, permission string) (bool, error) {
	// Check direct user permission
	var count int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM logbook_bucket_permissions
		WHERE bucket_id = $1 AND principal_type = 'user' AND principal_id = $2 AND permission = $3
	`, bucketID, userID, permission).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check user permission: %w", err)
	}
	if count > 0 {
		return true, nil
	}

	// Check group permission using provided group IDs
	if len(groupIDs) > 0 {
		groupPlaceholders, groupArgs := buildIntPlaceholders(groupIDs, 2)
		args := make([]interface{}, 0, 2+len(groupArgs))
		args = append(args, bucketID, permission)
		args = append(args, groupArgs...)
		query := fmt.Sprintf(`
			SELECT COUNT(*) FROM logbook_bucket_permissions
			WHERE bucket_id = $1 AND principal_type = 'group'
			AND principal_id IN (%s)
			AND permission = $2
		`, groupPlaceholders)
		err = r.db.QueryRow(query, args...).Scan(&count)
		if err != nil {
			return false, fmt.Errorf("failed to check group permission: %w", err)
		}
		return count > 0, nil
	}

	return false, nil
}

// GetAccessibleBucketIDs returns IDs of all buckets the user can access at the given permission level.
func (r *Repository) GetAccessibleBucketIDs(userID int, groupIDs []int, permission string) ([]string, error) {
	if len(groupIDs) == 0 {
		// User-only query
		rows, err := r.db.Query(`
			SELECT DISTINCT bucket_id FROM logbook_bucket_permissions
			WHERE principal_type = 'user' AND principal_id = $1 AND permission = $2
		`, userID, permission)
		if err != nil {
			return nil, fmt.Errorf("failed to get accessible bucket IDs: %w", err)
		}
		defer rows.Close()
		return scanStringColumn(rows)
	}

	// User + group query
	groupPlaceholders, groupArgs := buildIntPlaceholders(groupIDs, 2)
	args := make([]interface{}, 0, 2+len(groupArgs))
	args = append(args, userID, permission)
	args = append(args, groupArgs...)
	query := fmt.Sprintf(`
		SELECT DISTINCT bucket_id FROM logbook_bucket_permissions
		WHERE (
			(principal_type = 'user' AND principal_id = $1)
			OR (principal_type = 'group' AND principal_id IN (%s))
		) AND permission = $2
	`, groupPlaceholders)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get accessible bucket IDs: %w", err)
	}
	defer rows.Close()
	return scanStringColumn(rows)
}

// GetAllBucketIDs returns all bucket IDs (for system admins).
func (r *Repository) GetAllBucketIDs() ([]string, error) {
	rows, err := r.db.Query(`SELECT id FROM logbook_buckets ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("failed to get all bucket IDs: %w", err)
	}
	defer rows.Close()
	return scanStringColumn(rows)
}

// --- Documents ---

// CreateDocument inserts a new document.
func (r *Repository) CreateDocument(doc *models.LogbookDocument) error {
	err := r.db.QueryRow(`
		INSERT INTO logbook_documents (bucket_id, title, source_type, source_ref, content_hash,
		    raw_content, mime_type, file_path, author, status, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, created_at, updated_at
	`, doc.BucketID, doc.Title, doc.SourceType, doc.SourceRef, doc.ContentHash,
		doc.RawContent, doc.MimeType, doc.FilePath, doc.Author, doc.Status, doc.CreatedBy,
	).Scan(&doc.ID, &doc.CreatedAt, &doc.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to create document: %w", err)
	}
	return nil
}

// GetDocument returns a single document by ID.
func (r *Repository) GetDocument(id string) (*models.LogbookDocument, error) {
	var d models.LogbookDocument
	err := r.db.QueryRow(`
		SELECT d.id, d.bucket_id, d.title, d.source_type, d.source_ref, d.content_hash,
		       d.raw_content, d.article, d.content_type, d.cleaned_content,
		       d.mime_type, d.file_path, d.author, d.status, d.status_message,
		       d.retrieval_count, d.created_by, d.created_at, d.updated_at, d.archived_at,
		       d.reviewed_at, d.reviewed_by,
		       d.has_thumbnail, d.thumbnail_path,
		       COALESCE(b.name, '') as bucket_name,
		       (SELECT COUNT(*) FROM logbook_chunks c WHERE c.document_id = d.id) as chunk_count,
		       (d.article != '') as has_article,
		       b.max_age_days
		FROM logbook_documents d
		LEFT JOIN logbook_buckets b ON d.bucket_id = b.id
		WHERE d.id = $1
	`, id).Scan(
		&d.ID, &d.BucketID, &d.Title, &d.SourceType, &d.SourceRef, &d.ContentHash,
		&d.RawContent, &d.Article, &d.ContentType, &d.CleanedContent,
		&d.MimeType, &d.FilePath, &d.Author, &d.Status, &d.StatusMessage,
		&d.RetrievalCount, &d.CreatedBy, &d.CreatedAt, &d.UpdatedAt, &d.ArchivedAt,
		&d.ReviewedAt, &d.ReviewedBy,
		&d.HasThumbnail, &d.ThumbnailPath,
		&d.BucketName, &d.ChunkCount,
		&d.HasArticle, &d.MaxAgeDays,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get document: %w", err)
	}
	return &d, nil
}

// UpdateDocument updates a document's title and content.
func (r *Repository) UpdateDocument(id, title, content string) error {
	_, err := r.db.ExecWrite(`
		UPDATE logbook_documents
		SET title = $1, raw_content = $2, status = 'pending', updated_at = $3
		WHERE id = $4
	`, title, content, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update document: %w", err)
	}
	return nil
}

// UpdateDocumentStatus updates the processing status of a document.
func (r *Repository) UpdateDocumentStatus(id, status, statusMessage string) error {
	_, err := r.db.ExecWrite(`
		UPDATE logbook_documents SET status = $1, status_message = $2, updated_at = $3 WHERE id = $4
	`, status, statusMessage, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update document status: %w", err)
	}
	return nil
}

// UpdateDocumentArticle updates the generated KB article for a document.
func (r *Repository) UpdateDocumentArticle(docID, article string) error {
	_, err := r.db.ExecWrite(`
		UPDATE logbook_documents SET article = $1, updated_at = $2 WHERE id = $3
	`, article, time.Now(), docID)
	if err != nil {
		return fmt.Errorf("failed to update document article: %w", err)
	}
	return nil
}

// SaveNoteDirectly updates a note's title and article without triggering reprocessing.
func (r *Repository) SaveNoteDirectly(docID, title, article string) error {
	_, err := r.db.ExecWrite(`
		UPDATE logbook_documents
		SET title = $1, article = $2, status = 'ready', updated_at = $3
		WHERE id = $4
	`, title, article, time.Now(), docID)
	if err != nil {
		return fmt.Errorf("failed to save note directly: %w", err)
	}
	return nil
}

// UpdateDocumentContent updates raw content, mime type, and hash during ingestion.
func (r *Repository) UpdateDocumentContent(id, rawContent, mimeType, contentHash string) error {
	_, err := r.db.ExecWrite(`
		UPDATE logbook_documents
		SET raw_content = $1, mime_type = $2, content_hash = $3, updated_at = $4
		WHERE id = $5
	`, rawContent, mimeType, contentHash, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update document content: %w", err)
	}
	return nil
}

// UpdateDocumentThumbnail sets the thumbnail path and marks the document as having a thumbnail.
func (r *Repository) UpdateDocumentThumbnail(docID, thumbnailPath string) error {
	_, err := r.db.ExecWrite(`
		UPDATE logbook_documents SET has_thumbnail = true, thumbnail_path = $1, updated_at = $2 WHERE id = $3
	`, thumbnailPath, time.Now(), docID)
	if err != nil {
		return fmt.Errorf("failed to update document thumbnail: %w", err)
	}
	return nil
}

// UpdateDocumentClassification updates the content type and cleaned content for a document.
func (r *Repository) UpdateDocumentClassification(docID, contentType, cleanedContent string) error {
	_, err := r.db.ExecWrite(`
		UPDATE logbook_documents SET content_type = $1, cleaned_content = $2, updated_at = $3 WHERE id = $4
	`, contentType, cleanedContent, time.Now(), docID)
	if err != nil {
		return fmt.Errorf("failed to update document classification: %w", err)
	}
	return nil
}

// ArchiveDocument soft-deletes a document.
func (r *Repository) ArchiveDocument(id string) error {
	_, err := r.db.ExecWrite(`
		UPDATE logbook_documents SET archived_at = $1, updated_at = $1 WHERE id = $2
	`, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to archive document: %w", err)
	}
	return nil
}

// ListDocuments returns paginated documents for a bucket.
func (r *Repository) ListDocuments(bucketID string, limit, offset int) ([]models.LogbookDocument, int, error) {
	// Get total count
	var total int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM logbook_documents WHERE bucket_id = $1 AND archived_at IS NULL
	`, bucketID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count documents: %w", err)
	}

	rows, err := r.db.Query(`
		SELECT d.id, d.bucket_id, d.title, d.source_type, d.source_ref, d.content_hash,
		       '', '', d.content_type, '',
		       d.mime_type, d.file_path, d.author, d.status, d.status_message,
		       d.retrieval_count, d.created_by, d.created_at, d.updated_at, d.archived_at,
		       d.reviewed_at, d.reviewed_by,
		       d.has_thumbnail, d.thumbnail_path,
		       COALESCE(b.name, '') as bucket_name,
		       (SELECT COUNT(*) FROM logbook_chunks c WHERE c.document_id = d.id) as chunk_count,
		       (d.article != '') as has_article,
		       b.max_age_days
		FROM logbook_documents d
		LEFT JOIN logbook_buckets b ON d.bucket_id = b.id
		WHERE d.bucket_id = $1 AND d.archived_at IS NULL
		ORDER BY d.created_at DESC
		LIMIT $2 OFFSET $3
	`, bucketID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list documents: %w", err)
	}
	defer rows.Close()

	var docs []models.LogbookDocument
	for rows.Next() {
		var d models.LogbookDocument
		if err := rows.Scan(
			&d.ID, &d.BucketID, &d.Title, &d.SourceType, &d.SourceRef, &d.ContentHash,
			&d.RawContent, &d.Article, &d.ContentType, &d.CleanedContent,
			&d.MimeType, &d.FilePath, &d.Author, &d.Status, &d.StatusMessage,
			&d.RetrievalCount, &d.CreatedBy, &d.CreatedAt, &d.UpdatedAt, &d.ArchivedAt,
			&d.ReviewedAt, &d.ReviewedBy,
			&d.HasThumbnail, &d.ThumbnailPath,
			&d.BucketName, &d.ChunkCount,
			&d.HasArticle, &d.MaxAgeDays,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan document: %w", err)
		}
		docs = append(docs, d)
	}
	return docs, total, rows.Err()
}

// ListAllDocuments returns paginated documents across multiple accessible buckets.
func (r *Repository) ListAllDocuments(accessibleBucketIDs []string, limit, offset int) ([]models.LogbookDocument, int, error) {
	if len(accessibleBucketIDs) == 0 {
		return []models.LogbookDocument{}, 0, nil
	}

	placeholders, args := buildStringPlaceholders(accessibleBucketIDs)
	argOffset := len(args)

	// Get total count
	var total int
	countQuery := fmt.Sprintf(`
		SELECT COUNT(*) FROM logbook_documents WHERE bucket_id IN (%s) AND archived_at IS NULL
	`, placeholders)
	if err := r.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count documents: %w", err)
	}

	args = append(args, limit, offset)
	query := fmt.Sprintf(`
		SELECT d.id, d.bucket_id, d.title, d.source_type, d.source_ref, d.content_hash,
		       '', '', d.content_type, '',
		       d.mime_type, d.file_path, d.author, d.status, d.status_message,
		       d.retrieval_count, d.created_by, d.created_at, d.updated_at, d.archived_at,
		       d.reviewed_at, d.reviewed_by,
		       d.has_thumbnail, d.thumbnail_path,
		       COALESCE(b.name, '') as bucket_name,
		       (SELECT COUNT(*) FROM logbook_chunks c WHERE c.document_id = d.id) as chunk_count,
		       (d.article != '') as has_article,
		       b.max_age_days
		FROM logbook_documents d
		LEFT JOIN logbook_buckets b ON d.bucket_id = b.id
		WHERE d.bucket_id IN (%s) AND d.archived_at IS NULL
		ORDER BY d.created_at DESC
		LIMIT $%d OFFSET $%d
	`, placeholders, argOffset+1, argOffset+2)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list all documents: %w", err)
	}
	defer rows.Close()

	var docs []models.LogbookDocument
	for rows.Next() {
		var d models.LogbookDocument
		if err := rows.Scan(
			&d.ID, &d.BucketID, &d.Title, &d.SourceType, &d.SourceRef, &d.ContentHash,
			&d.RawContent, &d.Article, &d.ContentType, &d.CleanedContent,
			&d.MimeType, &d.FilePath, &d.Author, &d.Status, &d.StatusMessage,
			&d.RetrievalCount, &d.CreatedBy, &d.CreatedAt, &d.UpdatedAt, &d.ArchivedAt,
			&d.ReviewedAt, &d.ReviewedBy,
			&d.HasThumbnail, &d.ThumbnailPath,
			&d.BucketName, &d.ChunkCount,
			&d.HasArticle, &d.MaxAgeDays,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan document: %w", err)
		}
		docs = append(docs, d)
	}
	return docs, total, rows.Err()
}

// FindByContentHash returns an existing document with the same content hash in the same bucket.
func (r *Repository) FindByContentHash(bucketID, hash string) (*models.LogbookDocument, error) {
	var d models.LogbookDocument
	err := r.db.QueryRow(`
		SELECT id, bucket_id, title, status FROM logbook_documents
		WHERE bucket_id = $1 AND content_hash = $2 AND archived_at IS NULL
		LIMIT 1
	`, bucketID, hash).Scan(&d.ID, &d.BucketID, &d.Title, &d.Status)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find by content hash: %w", err)
	}
	return &d, nil
}

// --- Chunks ---

// CreateChunks inserts chunks in bulk.
func (r *Repository) CreateChunks(documentID string, chunks []models.LogbookChunk) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	for i := range chunks {
		c := &chunks[i]

		tags := c.Tags
		if tags == nil {
			tags = []string{}
		}

		_, err := tx.Exec(`
			INSERT INTO logbook_chunks (document_id, position, content, token_count,
			    byte_start, byte_end, first_page, last_page, summary, tags)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`, documentID, c.Position, c.Content, c.TokenCount,
			c.ByteStart, c.ByteEnd, c.FirstPage, c.LastPage,
			c.Summary, pq.Array(tags),
		)
		if err != nil {
			return fmt.Errorf("failed to insert chunk %d: %w", i, err)
		}
	}

	return tx.Commit()
}

// DeleteChunksByDocument removes all chunks for a document.
func (r *Repository) DeleteChunksByDocument(documentID string) error {
	_, err := r.db.ExecWrite(`DELETE FROM logbook_chunks WHERE document_id = $1`, documentID)
	if err != nil {
		return fmt.Errorf("failed to delete chunks: %w", err)
	}
	return nil
}

// --- Search ---

// KeywordSearch performs full-text search across accessible buckets.
func (r *Repository) KeywordSearch(query string, accessibleBucketIDs []string, limit, offset int) ([]models.LogbookSearchResult, int, error) {
	if len(accessibleBucketIDs) == 0 {
		return []models.LogbookSearchResult{}, 0, nil
	}

	placeholders, args := buildStringPlaceholders(accessibleBucketIDs)
	// Add FTS query and pagination args
	tsQuery := strings.ReplaceAll(query, " ", " & ")
	argOffset := len(args)
	args = append(args, tsQuery)

	countQuery := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM logbook_documents d
		WHERE d.bucket_id IN (%s) AND d.archived_at IS NULL
		AND to_tsvector('english', coalesce(d.title, '') || ' ' || coalesce(d.raw_content, '') || ' ' || coalesce(d.article, ''))
		    @@ to_tsquery('english', $%d)
	`, placeholders, argOffset+1)

	var total int
	if err := r.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count search results: %w", err)
	}

	args = append(args, limit, offset)
	searchQuery := fmt.Sprintf(`
		SELECT d.id, d.title,
		       ts_headline('english', d.raw_content, to_tsquery('english', $%d),
		           'MaxWords=40, MinWords=20, StartSel=<mark>, StopSel=</mark>') as highlight,
		       ts_rank(to_tsvector('english', coalesce(d.title, '') || ' ' || coalesce(d.raw_content, '') || ' ' || coalesce(d.article, '')),
		           to_tsquery('english', $%d)) as score,
		       d.bucket_id, COALESCE(b.name, '') as bucket_name,
		       d.source_type, d.author, d.created_at
		FROM logbook_documents d
		LEFT JOIN logbook_buckets b ON d.bucket_id = b.id
		WHERE d.bucket_id IN (%s) AND d.archived_at IS NULL
		AND to_tsvector('english', coalesce(d.title, '') || ' ' || coalesce(d.raw_content, '') || ' ' || coalesce(d.article, ''))
		    @@ to_tsquery('english', $%d)
		ORDER BY score DESC
		LIMIT $%d OFFSET $%d
	`, argOffset+1, argOffset+1, placeholders, argOffset+1, argOffset+2, argOffset+3)

	rows, err := r.db.Query(searchQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to execute keyword search: %w", err)
	}
	defer rows.Close()

	var results []models.LogbookSearchResult
	for rows.Next() {
		var sr models.LogbookSearchResult
		if err := rows.Scan(
			&sr.DocumentID, &sr.Title, &sr.Highlight, &sr.Score,
			&sr.BucketID, &sr.BucketName, &sr.SourceType, &sr.Author,
			&sr.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan search result: %w", err)
		}
		results = append(results, sr)
	}
	return results, total, rows.Err()
}

// IncrementRetrievalCount bumps the retrieval count for a document and its chunks.
func (r *Repository) IncrementRetrievalCount(documentID string, chunkIDs []string) error {
	_, err := r.db.ExecWrite(`
		UPDATE logbook_documents SET retrieval_count = retrieval_count + 1 WHERE id = $1
	`, documentID)
	if err != nil {
		return fmt.Errorf("failed to increment document retrieval count: %w", err)
	}

	if len(chunkIDs) > 0 {
		placeholders, args := buildStringPlaceholders(chunkIDs)
		_, err = r.db.ExecWrite(fmt.Sprintf(`
			UPDATE logbook_chunks SET retrieval_count = retrieval_count + 1 WHERE id IN (%s)
		`, placeholders), args...)
		if err != nil {
			return fmt.Errorf("failed to increment chunk retrieval counts: %w", err)
		}
	}
	return nil
}

// --- Attachments ---

// CreateAttachment inserts a new logbook attachment and returns the generated ID.
func (r *Repository) CreateAttachment(att *models.LogbookAttachment) (string, error) {
	var id string
	err := r.db.QueryRow(`
		INSERT INTO logbook_attachments (document_id, bucket_id, filename, original_filename, file_path, mime_type, file_size, uploaded_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id
	`, att.DocumentID, att.BucketID, att.Filename, att.OriginalFilename, att.FilePath, att.MimeType, att.FileSize, att.UploadedBy,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("failed to create attachment: %w", err)
	}
	return id, nil
}

// GetAttachment returns a single attachment by ID.
func (r *Repository) GetAttachment(id string) (*models.LogbookAttachment, error) {
	var a models.LogbookAttachment
	err := r.db.QueryRow(`
		SELECT id, document_id, bucket_id, filename, original_filename, file_path, mime_type, file_size, uploaded_by, created_at
		FROM logbook_attachments WHERE id = $1
	`, id).Scan(
		&a.ID, &a.DocumentID, &a.BucketID, &a.Filename, &a.OriginalFilename, &a.FilePath, &a.MimeType, &a.FileSize, &a.UploadedBy, &a.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get attachment: %w", err)
	}
	return &a, nil
}

// DeleteAttachment removes an attachment by ID.
func (r *Repository) DeleteAttachment(id string) error {
	_, err := r.db.ExecWrite(`DELETE FROM logbook_attachments WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete attachment: %w", err)
	}
	return nil
}

// --- Helpers ---

// buildStringPlaceholders creates PostgreSQL $N placeholders for string slices.
func buildStringPlaceholders(ids []string) (placeholders string, args []interface{}) {
	ph := make([]string, len(ids))
	args = make([]interface{}, len(ids))
	for i, id := range ids {
		ph[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	placeholders = strings.Join(ph, ", ")
	return placeholders, args
}

// buildIntPlaceholders creates PostgreSQL $N placeholders for int slices,
// starting at the given parameter offset (1-based).
func buildIntPlaceholders(ids []int, offset int) (placeholders string, args []interface{}) {
	ph := make([]string, len(ids))
	args = make([]interface{}, len(ids))
	for i, id := range ids {
		ph[i] = fmt.Sprintf("$%d", offset+i+1)
		args[i] = id
	}
	placeholders = strings.Join(ph, ", ")
	return placeholders, args
}

// scanStringColumn scans a single string column from rows into a slice.
func scanStringColumn(rows *sql.Rows) ([]string, error) {
	var result []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	return result, rows.Err()
}
