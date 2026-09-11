package models

import (
	"encoding/json"
	"time"
)

// Page is a workspace knowledge (wiki) page authored in Markdown.
// Pages form a per-workspace tree via parent_id. Permission evaluation
// uses the per-page ACL (page_permissions) with workspace-role fallback.
type Page struct {
	ID                 int             `json:"id" db:"id"`
	WorkspaceID        int             `json:"workspace_id" db:"workspace_id"`
	ParentID           *int            `json:"parent_id" db:"parent_id"`
	Title              string          `json:"title" db:"title"`
	Slug               string          `json:"slug" db:"slug"`
	Metadata           json.RawMessage `json:"metadata" db:"metadata"`
	Content            string          `json:"content" db:"content"`
	ContentHash        string          `json:"content_hash" db:"content_hash"`
	Excerpt            string          `json:"excerpt" db:"excerpt"`
	CreatedBy          int             `json:"created_by" db:"created_by"`
	UpdatedBy          *int            `json:"updated_by" db:"updated_by"`
	ArchivedBy         *int            `json:"archived_by" db:"archived_by"`
	IsHome             bool            `json:"is_home" db:"is_home"`
	InheritPermissions bool            `json:"inherit_permissions" db:"inherit_permissions"`
	Rank               *string         `json:"rank" db:"rank"`
	FracIndex          *string         `json:"frac_index" db:"frac_index"`
	Path               string          `json:"path" db:"path"`
	Depth              int             `json:"depth" db:"depth"`
	CreatedAt          time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at" db:"updated_at"`
	ArchivedAt         *time.Time      `json:"archived_at" db:"archived_at"`

	// Labels is populated by the page-label preload helpers
	// (PageLabelRepository.LoadForPages); always an empty slice in the JSON
	// response when no labels are attached so the frontend never has to
	// null-check.
	Labels []PageLabel `json:"labels" db:"-"`
}

// PageNode is a tree-rendering projection of Page with computed children.
// Used by internal tree projections; not persisted directly.
type PageNode struct {
	Page
	Children []*PageNode `json:"children,omitempty"`
}

// PageRevision is an immutable snapshot of a page produced on every
// content/title/parent/permission-impacting edit and on restore.
type PageRevision struct {
	ID             int                 `json:"id" db:"id"`
	PageID         int                 `json:"page_id" db:"page_id"`
	RevisionNumber int                 `json:"revision_number" db:"revision_number"`
	Title          string              `json:"title" db:"title"`
	Slug           string              `json:"slug" db:"slug"`
	Content        string              `json:"content" db:"content"`
	ContentHash    string              `json:"content_hash" db:"content_hash"`
	Excerpt        string              `json:"excerpt" db:"excerpt"`
	ParentID       *int                `json:"parent_id" db:"parent_id"`
	Path           string              `json:"path" db:"path"`
	Depth          int                 `json:"depth" db:"depth"`
	ChangeSummary  string              `json:"change_summary" db:"change_summary"`
	ChangeType     string              `json:"change_type" db:"change_type"`
	CreatedBy      int                 `json:"created_by" db:"created_by"`
	CreatedAt      time.Time           `json:"created_at" db:"created_at"`
	Author         *PageRevisionAuthor `json:"author,omitempty" db:"-"`
}

// PageRevisionAuthor is the small user projection needed by revision lists.
// IsActive is used by handlers to preserve user-directory visibility rules
// and is deliberately never serialized.
type PageRevisionAuthor struct {
	ID       int    `json:"id"`
	Name     string `json:"name,omitempty"`
	Username string `json:"username,omitempty"`
	IsActive bool   `json:"-"`
}

// PageRevisionChangeType enumerates valid values for PageRevision.ChangeType.
// Matches the CHECK constraint in pages.sql / pages_postgres.sql.
const (
	PageRevisionChangeTypeCreate      = "create"
	PageRevisionChangeTypeEdit        = "edit"
	PageRevisionChangeTypeMove        = "move"
	PageRevisionChangeTypePermissions = "permissions"
	PageRevisionChangeTypeRestore     = "restore"
	PageRevisionChangeTypeArchive     = "archive"
)

// PagePermission is a grant-only ACL row attached to a page. Phase 1 supports
// only grants; deny semantics are deferred to a later phase.
type PagePermission struct {
	ID              int       `json:"id" db:"id"`
	PageID          int       `json:"page_id" db:"page_id"`
	PrincipalType   string    `json:"principal_type" db:"principal_type"`
	PrincipalID     int       `json:"principal_id" db:"principal_id"`
	PermissionLevel string    `json:"permission_level" db:"permission_level"`
	GrantedBy       *int      `json:"granted_by" db:"granted_by"`
	GrantedAt       time.Time `json:"granted_at" db:"granted_at"`
}

// PagePrincipalType enumerates valid values for PagePermission.PrincipalType.
const (
	PagePrincipalTypeUser  = "user"
	PagePrincipalTypeGroup = "group"
	PagePrincipalTypeRole  = "role"
)

// PagePermissionLevel enumerates valid values for PagePermission.PermissionLevel.
const (
	PagePermissionLevelView  = "view"
	PagePermissionLevelEdit  = "edit"
	PagePermissionLevelAdmin = "admin"
)

// Page attachments are stored in the polymorphic `attachments` table
// (models.Attachment) with entity_type='page'. There is intentionally no
// dedicated PageAttachment model — uploads route through the existing
// AttachmentService.

// PageChunk is a search/RAG chunk derived from the current page content,
// rebuilt within the same transaction as any content change.
type PageChunk struct {
	ID             int       `json:"id" db:"id"`
	PageID         int       `json:"page_id" db:"page_id"`
	WorkspaceID    int       `json:"workspace_id" db:"workspace_id"`
	RevisionNumber int       `json:"revision_number" db:"revision_number"`
	Position       int       `json:"position" db:"position"`
	HeadingPath    string    `json:"heading_path" db:"heading_path"`
	Content        string    `json:"content" db:"content"`
	TokenCount     int       `json:"token_count" db:"token_count"`
	ByteStart      int       `json:"byte_start" db:"byte_start"`
	ByteEnd        int       `json:"byte_end" db:"byte_end"`
	ContentHash    string    `json:"content_hash" db:"content_hash"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
}
