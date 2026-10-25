// Package planeimport imports issues exported from Plane (makeplane/plane)
// into Windshift workspaces, preserving authors, timestamps, and per-workspace
// item numbers.
//
// The import source is a JSONL contract (see ContractRecord): one meta record
// naming the source project, then one record per Plane issue and comment. The
// contract is produced from Plane's own database — Plane's HTTP export is
// session-auth only and cannot be fetched headlessly — but the loader is
// source-agnostic: any producer that emits the same records can drive it.
//
// Fidelity contract, enforced by the loader:
//   - workspace_item_number is taken from the source sequence_id (gaps in the
//     source numbering are gaps in Windshift too; the counter continues above
//     the maximum),
//   - creator_id and created_at are the source values, never import time,
//   - reruns are idempotent via plane_import_mappings keyed on the source uuid,
//   - soft-deleted source issues are skipped but reported, so their numbers
//     stay reserved as gaps rather than being reused by later creates.
package planeimport

import "time"

// ContractSchemaVersion is the contract format this package reads. A producer
// emits it in the meta record; the loader refuses a mismatch instead of
// silently mis-parsing rows.
const ContractSchemaVersion = 1

// ContractRecord is one JSONL line of the import contract. Issue, comment,
// and user records carry their payload in Issue/Comment/User; meta records
// name the source project. All fields not relevant to a record type are zero
// and omitted from the JSON.
type ContractRecord struct {
	Type    string         `json:"type"` // "meta" | "issue" | "comment" | "user"
	Meta    *Meta          `json:"meta,omitempty"`
	Issue   *Issue         `json:"issue,omitempty"`
	Comment *Comment       `json:"comment,omitempty"`
	User    *UserRow       `json:"user,omitempty"`
	Counts  map[string]int `json:"counts,omitempty"` // per-source-field drop counts reported by the producer
}

// Meta names the import source. It must be the first record of a contract.
type Meta struct {
	SchemaVersion int    `json:"schema_version"`
	Source        string `json:"source"` // "plane"
	WorkspaceSlug string `json:"workspace_slug"`
	ProjectID     string `json:"project_id"`
	ProjectKey    string `json:"project_key"`
	DumpedAt      string `json:"dumped_at"`
}

// Issue is one Plane issue (issues-table row) in contract shape. Timestamps are
// RFC3339. DescriptionJSON is the canonical Tiptap editor document;
// DescriptionHTML is Plane's own render, kept as the converter's fallback.
type Issue struct {
	ID              string     `json:"id"` // Plane uuid
	SequenceID      int        `json:"sequence_id"`
	Name            string     `json:"name"`
	DescriptionJSON string     `json:"description_json"`
	DescriptionHTML string     `json:"description_html"`
	StateID         string     `json:"state_id"`
	StateName       string     `json:"state_name"` // Plane state name at dump time; mapped via StatusMap
	Priority        string     `json:"priority"`
	EstimatePoint   *string    `json:"estimate_point,omitempty"` // raw Plane estimate value
	StartDate       *time.Time `json:"start_date,omitempty"`
	TargetDate      *time.Time `json:"target_date,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	ArchivedAt      *time.Time `json:"archived_at,omitempty"`
	DeletedAt       *time.Time `json:"deleted_at,omitempty"` // soft-delete: skipped, number stays a gap
	IsDraft         bool       `json:"is_draft"`
	ParentID        *string    `json:"parent_id,omitempty"`
	ParentSequence  int        `json:"parent_sequence_id,omitempty"` // sequence_id of the parent, for in-pass linking
	CreatedByID     string     `json:"created_by_id"`
	UpdatedByID     string     `json:"updated_by_id"`
}

// Comment is one Plane issue comment in contract shape. DeltaJSON is the
// TipTap document the comment stores; DeltaHTML is the fallback render.
// Comment is one Plane comment in contract shape. CommentJSON is the editor
// document the comment stores (issue_comments.comment_json); CommentHTML is
// Plane's render (comment_html); Text is the stripped plain text
// (comment_stripped) used as last-resort content. Access is Plane's
// visibility enum as stored ('INTERNAL' observed fleet-wide).
type Comment struct {
	ID          string     `json:"id"`
	IssueID     string     `json:"issue_id"`
	CommentJSON string     `json:"comment_json"`
	CommentHTML string     `json:"comment_html"`
	Text        string     `json:"text"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty"`
	CreatedByID string     `json:"created_by_id"`
	Access      string     `json:"access"`
}

// UserRow is one Plane users-table row carried inside the contract so the
// loader can map authors without reaching back into the source database.
type UserRow struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}
