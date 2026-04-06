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

	// Customer association (set by logbook actions)
	CustomerOrganisationID *int `json:"customer_organisation_id,omitempty"`
	PortalCustomerID       *int `json:"portal_customer_id,omitempty"`

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
	RetrievalCount int       `json:"retrieval_count"`
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

// ===== Logbook Actions (Automations) =====

// LogbookActionTriggerType defines what triggers a logbook action
type LogbookActionTriggerType string

const (
	LogbookTriggerDocumentClassified LogbookActionTriggerType = "document_classified"
	LogbookTriggerContentKeyword     LogbookActionTriggerType = "content_keyword"
	LogbookTriggerMimeType           LogbookActionTriggerType = "mime_type"
	LogbookTriggerManual             LogbookActionTriggerType = "manual"
)

// LogbookActionNodeType defines the types of nodes in a logbook action flow
type LogbookActionNodeType string

const (
	LogbookNodeTrigger           LogbookActionNodeType = "trigger"
	LogbookNodeCreateItem        LogbookActionNodeType = "create_item"
	LogbookNodeCreateAsset       LogbookActionNodeType = "create_asset"
	LogbookNodeAssociateCustomer LogbookActionNodeType = "associate_customer"
	LogbookNodeCondition         LogbookActionNodeType = "condition"
)

// LogbookAction represents a bucket-scoped automation definition
type LogbookAction struct {
	ID            int                      `json:"id"`
	BucketID      string                   `json:"bucket_id"`
	Name          string                   `json:"name"`
	Description   string                   `json:"description,omitempty"`
	IsEnabled     bool                     `json:"is_enabled"`
	TriggerType   LogbookActionTriggerType `json:"trigger_type"`
	TriggerConfig string                   `json:"trigger_config,omitempty"`
	CreatedBy     *int                     `json:"created_by,omitempty"`
	CreatedAt     time.Time                `json:"created_at"`
	UpdatedAt     time.Time                `json:"updated_at"`

	// Joined fields
	CreatorName string              `json:"creator_name,omitempty"`
	Nodes       []LogbookActionNode `json:"nodes,omitempty"`
	Edges       []LogbookActionEdge `json:"edges,omitempty"`
}

// LogbookActionNode represents a step in a logbook action flow
type LogbookActionNode struct {
	ID         int                   `json:"id"`
	ActionID   int                   `json:"action_id"`
	NodeType   LogbookActionNodeType `json:"node_type"`
	NodeConfig string                `json:"node_config"`
	PositionX  float64               `json:"position_x"`
	PositionY  float64               `json:"position_y"`
	CreatedAt  time.Time             `json:"created_at"`
	UpdatedAt  time.Time             `json:"updated_at"`
}

// LogbookActionEdge represents a connection between nodes in a logbook action flow
type LogbookActionEdge struct {
	ID           int       `json:"id"`
	ActionID     int       `json:"action_id"`
	SourceNodeID int       `json:"source_node_id"`
	TargetNodeID int       `json:"target_node_id"`
	EdgeType     string    `json:"edge_type"`
	SourceHandle string    `json:"source_handle,omitempty"`
	TargetHandle string    `json:"target_handle,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// LogbookActionExecutionLog represents the audit trail for logbook action executions
type LogbookActionExecutionLog struct {
	ID             int                   `json:"id"`
	ActionID       int                   `json:"action_id"`
	DocumentID     *string               `json:"document_id,omitempty"`
	TriggerEvent   string                `json:"trigger_event"`
	Status         ActionExecutionStatus `json:"status"`
	StartedAt      time.Time             `json:"started_at"`
	CompletedAt    *time.Time            `json:"completed_at,omitempty"`
	ErrorMessage   string                `json:"error_message,omitempty"`
	ExecutionTrace string                `json:"execution_trace,omitempty"`

	// Joined fields
	ActionName    string `json:"action_name,omitempty"`
	DocumentTitle string `json:"document_title,omitempty"`
}

// LogbookTriggerConfig holds trigger-specific configuration for logbook actions
type LogbookTriggerConfig struct {
	ContentTypes []string `json:"content_types,omitempty"` // e.g. ["knowledge", "record", "correspondence"]
	Keywords     []string `json:"keywords,omitempty"`      // Keywords to match in document content
	KeywordMode  string   `json:"keyword_mode,omitempty"`  // "any" (default) or "all"
	MimeTypes    []string `json:"mime_types,omitempty"`    // MIME type patterns (supports wildcards like "image/*")
}

// CreateItemNodeConfig configures a create_item logbook action node
type CreateItemNodeConfig struct {
	WorkspaceID int    `json:"workspace_id"`
	ItemTypeID  int    `json:"item_type_id"`
	Title       string `json:"title"`       // Template string: {{doc.title}}, etc.
	Description string `json:"description"` // Template string
}

// AssociateCustomerNodeConfig configures an associate_customer logbook action node
type AssociateCustomerNodeConfig struct {
	CustomerOrganisationID *int `json:"customer_organisation_id,omitempty"`
	PortalCustomerID       *int `json:"portal_customer_id,omitempty"`
}

// LogbookActionEvent represents a document event that may trigger logbook actions
type LogbookActionEvent struct {
	EventType   LogbookActionTriggerType `json:"event_type"`
	BucketID    string                   `json:"bucket_id"`
	DocumentID  string                   `json:"document_id"`
	ActorUserID int                      `json:"actor_user_id"`
	ContentType string                   `json:"content_type,omitempty"`
	MimeType    string                   `json:"mime_type,omitempty"`
	Title       string                   `json:"title,omitempty"`
	SourceType  string                   `json:"source_type,omitempty"`
	Author      string                   `json:"author,omitempty"`
	RawContent  string                   `json:"raw_content,omitempty"`
	// Loop prevention fields for cross-application cascade tracking
	TriggeredByAction bool   `json:"triggered_by_action,omitempty"`
	ExecutionChainID  string `json:"execution_chain_id,omitempty"`
	CascadeDepth      int    `json:"cascade_depth,omitempty"`
	SourceApplication string `json:"source_application,omitempty"`
}

// CreateLogbookActionRequest represents the API request to create a logbook action
type CreateLogbookActionRequest struct {
	Name          string                   `json:"name"`
	Description   string                   `json:"description,omitempty"`
	TriggerType   LogbookActionTriggerType `json:"trigger_type"`
	TriggerConfig string                   `json:"trigger_config,omitempty"`
	Nodes         []LogbookActionNode      `json:"nodes,omitempty"`
	Edges         []LogbookActionEdge      `json:"edges,omitempty"`
}

// UpdateLogbookActionRequest represents the API request to update a logbook action
type UpdateLogbookActionRequest struct {
	Name          *string                   `json:"name,omitempty"`
	Description   *string                   `json:"description,omitempty"`
	TriggerType   *LogbookActionTriggerType `json:"trigger_type,omitempty"`
	TriggerConfig *string                   `json:"trigger_config,omitempty"`
	IsEnabled     *bool                     `json:"is_enabled,omitempty"`
	Nodes         []LogbookActionNode       `json:"nodes,omitempty"`
	Edges         []LogbookActionEdge       `json:"edges,omitempty"`
}

// NodeExecutionRequest is sent from the sidecar to the main server to execute
// SQLite-dependent nodes (create_item, create_asset).
type NodeExecutionRequest struct {
	NodeType   string             `json:"node_type"`
	NodeConfig string             `json:"node_config"`
	Event      LogbookActionEvent `json:"event"`
	// Loop prevention fields for cross-application cascade tracking
	TriggeredByAction bool   `json:"triggered_by_action,omitempty"`
	ExecutionChainID  string `json:"execution_chain_id,omitempty"`
	CascadeDepth      int    `json:"cascade_depth,omitempty"`
	SourceApplication string `json:"source_application,omitempty"`
}

// NodeExecutionResponse is returned by the main server after executing a node.
type NodeExecutionResponse struct {
	Output map[string]interface{} `json:"output,omitempty"`
	Error  string                 `json:"error,omitempty"`
}
