package models

import "time"

// LogbookBucket represents a knowledge bucket in the logbook system
type LogbookBucket struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	WorkspaceID *int      `json:"workspace_id,omitempty"`
	CreatedBy   int       `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// Configuration
	MaxAgeDays       *int   `json:"max_age_days,omitempty"`
	ApprovalRequired bool   `json:"approval_required"`
	PortalVisible    bool   `json:"portal_visible"`
	EmailAddress     string `json:"email_address,omitempty"`
	DefaultAuthority string `json:"default_authority,omitempty"`

	// Joined fields
	CreatedByName string `json:"created_by_name,omitempty"`
	DocumentCount int    `json:"document_count,omitempty"`
}

// LogbookBucketPermission represents a permission assignment for a bucket
type LogbookBucketPermission struct {
	ID            string `json:"id"`
	BucketID      string `json:"bucket_id"`
	PrincipalType string `json:"principal_type"` // "user" or "group"
	PrincipalID   int    `json:"principal_id"`
	Permission    string `json:"permission"` // bucket.view, bucket.edit, bucket.admin

	// Joined fields
	PrincipalName string `json:"principal_name,omitempty"`
}

// LogbookDocument represents a document in a bucket
type LogbookDocument struct {
	ID             string     `json:"id"`
	BucketID       string     `json:"bucket_id"`
	Title          string     `json:"title"`
	SourceType     string     `json:"source_type"` // "upload", "note", "email"
	SourceRef      string     `json:"source_ref,omitempty"`
	ContentHash    string     `json:"content_hash,omitempty"`
	RawContent     string     `json:"raw_content,omitempty"`
	Article        string     `json:"article,omitempty"`
	ContentType    string     `json:"content_type,omitempty"`
	CleanedContent string     `json:"cleaned_content,omitempty"`
	MimeType       string     `json:"mime_type,omitempty"`
	FilePath       string     `json:"-"` // Never expose file path to client
	Author         string     `json:"author,omitempty"`
	Status         string     `json:"status"` // "pending", "processing", "ready", "error"
	StatusMessage  string     `json:"status_message,omitempty"`
	RetrievalCount int        `json:"retrieval_count"`
	CreatedBy      int        `json:"created_by"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	ArchivedAt     *time.Time `json:"archived_at,omitempty"`
	ReviewedAt     *time.Time `json:"reviewed_at,omitempty"`
	ReviewedBy     *int       `json:"reviewed_by,omitempty"`

	// Thumbnail
	HasThumbnail  bool   `json:"has_thumbnail"`
	ThumbnailPath string `json:"-"` // Never expose to client

	// Joined fields
	CreatedByName string `json:"created_by_name,omitempty"`
	BucketName    string `json:"bucket_name,omitempty"`
	ChunkCount    int    `json:"chunk_count,omitempty"`
	HasArticle    bool   `json:"has_article,omitempty"`
	MaxAgeDays    *int   `json:"max_age_days,omitempty"`
}

// LogbookChunk represents a chunk of a document
type LogbookChunk struct {
	ID             string    `json:"id"`
	DocumentID     string    `json:"document_id"`
	Position       int       `json:"position"`
	Content        string    `json:"content"`
	TokenCount     int       `json:"token_count"`
	ByteStart      int       `json:"byte_start"`
	ByteEnd        int       `json:"byte_end"`
	FirstPage      *int      `json:"first_page,omitempty"`
	LastPage       *int      `json:"last_page,omitempty"`
	Summary        string    `json:"summary,omitempty"`
	Tags           []string  `json:"tags,omitempty"`
	RetrievalCount int `json:"retrieval_count"`
	CreatedAt      time.Time `json:"created_at"`
}

// LogbookSearchResult represents a search result with relevance scoring
type LogbookSearchResult struct {
	DocumentID    string  `json:"document_id"`
	ChunkID       string  `json:"chunk_id,omitempty"`
	Title         string  `json:"title"`
	Content       string  `json:"content"`
	Score         float64 `json:"score"`
	BucketID      string  `json:"bucket_id"`
	BucketName    string  `json:"bucket_name"`
	SourceType    string  `json:"source_type"`
	Author        string  `json:"author,omitempty"`
	Highlight     string  `json:"highlight,omitempty"`
	FirstPage     *int    `json:"first_page,omitempty"`
	LastPage      *int    `json:"last_page,omitempty"`
	CreatedAt     string  `json:"created_at"`
	CreatedByName string  `json:"created_by_name,omitempty"`
}

// Logbook permission constants
const (
	LogbookPermissionBucketView  = "bucket.view"
	LogbookPermissionBucketEdit  = "bucket.edit"
	LogbookPermissionBucketAdmin = "bucket.admin"
)

// Logbook content type constants
const (
	LogbookContentTypeKnowledge      = "knowledge"
	LogbookContentTypeRecord         = "record"
	LogbookContentTypeCorrespondence = "correspondence"
)

// Logbook document status constants
const (
	LogbookDocStatusPending    = "pending"
	LogbookDocStatusProcessing = "processing"
	LogbookDocStatusReady      = "ready"
	LogbookDocStatusError      = "error"
)

// Logbook source type constants
const (
	LogbookSourceUpload = "upload"
	LogbookSourceNote   = "note"
	LogbookSourceEmail  = "email"
)

// LogbookBucketCreateRequest represents the payload for creating a bucket
type LogbookBucketCreateRequest struct {
	Name             string `json:"name"`
	Description      string `json:"description"`
	WorkspaceID      *int   `json:"workspace_id,omitempty"`
	MaxAgeDays       *int   `json:"max_age_days,omitempty"`
	ApprovalRequired bool   `json:"approval_required"`
	PortalVisible    bool   `json:"portal_visible"`
	EmailAddress     string `json:"email_address,omitempty"`
	DefaultAuthority string `json:"default_authority,omitempty"`
}

// LogbookBucketUpdateRequest represents the payload for updating a bucket
type LogbookBucketUpdateRequest struct {
	Name             string `json:"name"`
	Description      string `json:"description"`
	MaxAgeDays       *int   `json:"max_age_days,omitempty"`
	ApprovalRequired *bool  `json:"approval_required,omitempty"`
	PortalVisible    *bool  `json:"portal_visible,omitempty"`
	EmailAddress     string `json:"email_address,omitempty"`
	DefaultAuthority string `json:"default_authority,omitempty"`
}

// LogbookNoteCreateRequest represents the payload for creating a note
type LogbookNoteCreateRequest struct {
	Title   string `json:"title"`
	Content string `json:"content"`
	Author  string `json:"author,omitempty"`
}

// LogbookDocumentUpdateRequest represents the payload for updating a document
type LogbookDocumentUpdateRequest struct {
	Title   string `json:"title"`
	Content string `json:"content,omitempty"`
	Article string `json:"article,omitempty"`
}

// LogbookAttachment represents a file attachment on a logbook document.
type LogbookAttachment struct {
	ID               string    `json:"id"`
	DocumentID       string    `json:"document_id"`
	BucketID         string    `json:"bucket_id"`
	Filename         string    `json:"filename"`
	OriginalFilename string    `json:"original_filename"`
	FilePath         string    `json:"-"`
	MimeType         string    `json:"mime_type"`
	FileSize         int64     `json:"file_size"`
	UploadedBy       int       `json:"uploaded_by"`
	CreatedAt        time.Time `json:"created_at"`
	DownloadURL      string    `json:"download_url,omitempty"`
}

// LogbookSetPermissionsRequest represents the payload for setting bucket permissions
type LogbookSetPermissionsRequest struct {
	Permissions []LogbookBucketPermission `json:"permissions"`
}

