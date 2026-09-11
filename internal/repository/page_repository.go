// Package repository — page_repository persists workspace knowledge pages.
package repository

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// MaxPageDepth caps every recursive page-tree walk. Mirrors the items
// hierarchy ceiling so a stored cycle cannot loop the DB forever and the
// CTE-based ancestor/descendant traversals stay bounded.
const MaxPageDepth = 30

// PageRepository persists the pages table and its tree helpers (ancestors,
// descendants, children, and parent-walk for cycle detection).
type PageRepository struct {
	db database.Database
}

// NewPageRepository creates a PageRepository.
func NewPageRepository(db database.Database) *PageRepository {
	return &PageRepository{db: db}
}

// pageColumns lists every column of the pages table in the order used by
// scanPage. Centralized so SELECT and Scan stay in sync.
const pageColumns = `id, workspace_id, parent_id, title, slug, metadata, content, content_hash,
	excerpt, created_by, updated_by, archived_by, is_home, inherit_permissions,
	rank, frac_index, path, depth, created_at, updated_at, archived_at`

// pageTreeColumns is pageColumns minus the heavy body fields (content,
// content_hash, excerpt), in the order used by scanPageMeta. The tree/list
// endpoints render titles and hierarchy only, so projecting the body out of
// the SELECT means the large content column is never read off disk (nor
// de-TOASTed on Postgres) nor allocated into a Go string — the win that
// stripping the fields *after* the read can't give a workspace with
// thousands of pages. (WI-407.)
const pageTreeColumns = `id, workspace_id, parent_id, title, slug, metadata,
	created_by, updated_by, archived_by, is_home, inherit_permissions,
	rank, frac_index, path, depth, created_at, updated_at, archived_at`

// applyPageNullables folds the nullable columns shared by every page scan
// into the Page. Kept separate so scanPage and scanPageMeta stay in sync.
func applyPageNullables(p *models.Page, parentID, updatedBy, archivedBy sql.NullInt64, rank, fracIndex sql.NullString, archivedAt sql.NullTime) {
	if len(p.Metadata) == 0 || !json.Valid(p.Metadata) {
		p.Metadata = json.RawMessage(`{}`)
	}
	if parentID.Valid {
		v := int(parentID.Int64)
		p.ParentID = &v
	}
	if updatedBy.Valid {
		v := int(updatedBy.Int64)
		p.UpdatedBy = &v
	}
	if archivedBy.Valid {
		v := int(archivedBy.Int64)
		p.ArchivedBy = &v
	}
	if rank.Valid {
		p.Rank = &rank.String
	}
	if fracIndex.Valid {
		p.FracIndex = &fracIndex.String
	}
	if archivedAt.Valid {
		p.ArchivedAt = &archivedAt.Time
	}
}

// scanPage scans a single row into a Page using the package-local rowScanner
// abstraction (declared by custom_field_repository).
func scanPage(s rowScanner) (*models.Page, error) {
	var p models.Page
	var parentID, updatedBy, archivedBy sql.NullInt64
	var rank, fracIndex sql.NullString
	var archivedAt sql.NullTime
	// metadata is written as a bound Go string (TEXT storage class), which
	// database/sql will not scan into *json.RawMessage — go through []byte.
	var metadata []byte

	if err := s.Scan(
		&p.ID, &p.WorkspaceID, &parentID, &p.Title, &p.Slug, &metadata, &p.Content, &p.ContentHash,
		&p.Excerpt, &p.CreatedBy, &updatedBy, &archivedBy, &p.IsHome, &p.InheritPermissions,
		&rank, &fracIndex, &p.Path, &p.Depth, &p.CreatedAt, &p.UpdatedAt, &archivedAt,
	); err != nil {
		return nil, err
	}
	p.Metadata = json.RawMessage(metadata)

	applyPageNullables(&p, parentID, updatedBy, archivedBy, rank, fracIndex, archivedAt)
	return &p, nil
}

// scanPageMeta scans a row selected via pageTreeColumns. Every Page field is
// populated except the body (Content/ContentHash/Excerpt stay zero-valued —
// they aren't in the SELECT).
func scanPageMeta(s rowScanner) (*models.Page, error) {
	var p models.Page
	var parentID, updatedBy, archivedBy sql.NullInt64
	var rank, fracIndex sql.NullString
	var archivedAt sql.NullTime
	// See scanPage: string-typed TEXT values cannot scan into *json.RawMessage.
	var metadata []byte

	if err := s.Scan(
		&p.ID, &p.WorkspaceID, &parentID, &p.Title, &p.Slug, &metadata,
		&p.CreatedBy, &updatedBy, &archivedBy, &p.IsHome, &p.InheritPermissions,
		&rank, &fracIndex, &p.Path, &p.Depth, &p.CreatedAt, &p.UpdatedAt, &archivedAt,
	); err != nil {
		return nil, err
	}
	p.Metadata = json.RawMessage(metadata)

	applyPageNullables(&p, parentID, updatedBy, archivedBy, rank, fracIndex, archivedAt)
	return &p, nil
}

// CreateInput is the persisted shape of a new page. The service computes
// slug/content/hash/excerpt/path/depth/inheritance flags before calling.
type CreateInput struct {
	WorkspaceID        int
	ParentID           *int
	Title              string
	Slug               string
	Metadata           string
	Content            string
	ContentHash        string
	Excerpt            string
	CreatedBy          int
	IsHome             bool
	InheritPermissions bool
	Rank               *string
	FracIndex          *string
	Path               string
	Depth              int
}

// CreateTx inserts a page within the given transaction. Returns the new id.
func (r *PageRepository) CreateTx(tx database.Tx, in CreateInput) (int, error) {
	now := time.Now().UTC()
	var id int
	err := tx.QueryRow(`
		INSERT INTO pages (
			workspace_id, parent_id, title, slug, metadata, content, content_hash, excerpt,
			created_by, updated_by, is_home, inherit_permissions,
			rank, frac_index, path, depth, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id
	`,
		in.WorkspaceID, nullInt(in.ParentID), in.Title, in.Slug, in.Metadata, in.Content, in.ContentHash, in.Excerpt,
		in.CreatedBy, in.CreatedBy, in.IsHome, in.InheritPermissions,
		nullString(in.Rank), nullString(in.FracIndex), in.Path, in.Depth, now, now,
	).Scan(&id)
	if err != nil {
		if isUniqueConstraintError(err) {
			return 0, ErrDuplicateEntry
		}
		return 0, fmt.Errorf("insert page: %w", err)
	}
	return id, nil
}

// GetByID loads a single page. Returns ErrNotFound when no row matches.
func (r *PageRepository) GetByID(id int) (*models.Page, error) {
	row := r.db.QueryRow("SELECT "+pageColumns+" FROM pages WHERE id = ?", id)
	page, err := scanPage(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get page %d: %w", id, err)
	}
	return page, nil
}

// GetByIDs loads multiple pages in a single query. Missing ids are simply
// absent from the result — the caller decides how to surface that. The slice
// is ordered as returned by the database (callers that need a specific order
// should sort by id themselves).
func (r *PageRepository) GetByIDs(ids []int) ([]models.Page, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	args := make([]any, len(ids))
	placeholders := ""
	for i, id := range ids {
		args[i] = id
		if i > 0 {
			placeholders += ", "
		}
		placeholders += "?"
	}
	rows, err := r.db.Query("SELECT "+pageColumns+" FROM pages WHERE id IN ("+placeholders+")", args...)
	if err != nil {
		return nil, fmt.Errorf("get pages by ids: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]models.Page, 0, len(ids))
	for rows.Next() {
		page, scanErr := scanPage(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan page: %w", scanErr)
		}
		out = append(out, *page)
	}
	return out, rows.Err()
}

// GetByIDTx loads a single page within a transaction.
func (r *PageRepository) GetByIDTx(tx database.Tx, id int) (*models.Page, error) {
	row := tx.QueryRow("SELECT "+pageColumns+" FROM pages WHERE id = ?", id)
	page, err := scanPage(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get page %d (tx): %w", id, err)
	}
	return page, nil
}

// GetParentIDTx returns the parent_id of a page within a transaction.
// Used by the cycle-detection walker. Returns ErrNotFound for unknown ids.
func (r *PageRepository) GetParentIDTx(tx database.Tx, id int) (*int, error) {
	var parentID sql.NullInt64
	err := tx.QueryRow("SELECT parent_id FROM pages WHERE id = ?", id).Scan(&parentID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get parent_id of page %d: %w", id, err)
	}
	if !parentID.Valid {
		return nil, nil
	}
	v := int(parentID.Int64)
	return &v, nil
}

// UpdateInput is the persisted shape of a page update. The service computes
// the derived columns (slug/content_hash/excerpt) before calling.
type UpdateInput struct {
	ID                 int
	Title              string
	Slug               string
	Content            string
	ContentHash        string
	Excerpt            string
	InheritPermissions bool
	Metadata           *string
	UpdatedBy          int
	// Unarchive clears archived_at/archived_by while applying the update.
	// Used by restore; normal title/content edits leave archive state alone.
	Unarchive bool
}

// UpdateTx applies a content/title/slug/inheritance edit within a transaction.
// rank and frac_index are deliberately not touched — sibling ordering is owned
// by MoveTx and SetFracIndexTx so a normal title/content save can't silently
// destroy a drag-and-drop ordering. Move and Archive are separate methods
// because they touch parent_id/path/depth and archived_* fields respectively.
func (r *PageRepository) UpdateTx(tx database.Tx, in UpdateInput) error {
	now := time.Now().UTC()
	query := `
		UPDATE pages
		SET title = ?,
		    slug = ?,
		    content = ?,
		    content_hash = ?,
		    excerpt = ?,
		    inherit_permissions = ?,
		    updated_by = ?,
		    updated_at = ?`
	args := make([]any, 0, 10)
	args = append(args, in.Title, in.Slug, in.Content, in.ContentHash, in.Excerpt, in.InheritPermissions, in.UpdatedBy, now)
	if in.Metadata != nil {
		query += `,
		    metadata = ?`
		args = append(args, *in.Metadata)
	}
	if in.Unarchive {
		query += `,
		    archived_at = NULL,
		    archived_by = NULL`
	}
	query += `
		WHERE id = ?`
	args = append(args, in.ID)
	res, err := tx.Exec(query, args...)
	if err != nil {
		if isUniqueConstraintError(err) {
			return ErrDuplicateEntry
		}
		return fmt.Errorf("update page %d: %w", in.ID, err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// MoveTx reparents a page and overwrites its path/depth. Cycle detection is
// the caller's responsibility (see WouldCreateCycleTx). When newFracIndex is
// non-nil the page's frac_index is rewritten in the same UPDATE so callers
// don't need a second round-trip for reorder-with-move.
func (r *PageRepository) MoveTx(tx database.Tx, pageID int, newParentID *int, newPath string, newDepth, updatedBy int, newFracIndex *string) error {
	now := time.Now().UTC()
	var (
		res sql.Result
		err error
	)
	if newFracIndex != nil {
		res, err = tx.Exec(`
			UPDATE pages
			SET parent_id = ?,
			    path = ?,
			    depth = ?,
			    frac_index = ?,
			    updated_by = ?,
			    updated_at = ?
			WHERE id = ?
		`, nullInt(newParentID), newPath, newDepth, *newFracIndex, updatedBy, now, pageID)
	} else {
		res, err = tx.Exec(`
			UPDATE pages
			SET parent_id = ?,
			    path = ?,
			    depth = ?,
			    updated_by = ?,
			    updated_at = ?
			WHERE id = ?
		`, nullInt(newParentID), newPath, newDepth, updatedBy, now, pageID)
	}
	if err != nil {
		if isUniqueConstraintError(err) {
			return ErrDuplicateEntry
		}
		return fmt.Errorf("move page %d: %w", pageID, err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// MoveAcrossWorkspaceTx rewrites the workspace and hierarchy columns for one
// row in a subtree move. Cross-workspace moves deliberately reset home status
// and explicit ACL inheritance; workspace-scoped relations are reconciled by
// PageService in the same transaction.
func (r *PageRepository) MoveAcrossWorkspaceTx(tx database.Tx, pageID, destinationWorkspaceID int, newParentID *int, newPath string, newDepth, updatedBy int, newFracIndex *string) error {
	now := time.Now().UTC()
	query := `
		UPDATE pages
		SET workspace_id = ?,
		    parent_id = ?,
		    path = ?,
		    depth = ?,
		    is_home = false,
		    inherit_permissions = true,
		    updated_by = ?,
		    updated_at = ?`
	args := []any{destinationWorkspaceID, nullInt(newParentID), newPath, newDepth, updatedBy, now}
	if newFracIndex != nil {
		query += `,
		    frac_index = ?`
		args = append(args, *newFracIndex)
	}
	query += ` WHERE id = ?`
	args = append(args, pageID)

	res, err := tx.Exec(query, args...)
	if err != nil {
		if isUniqueConstraintError(err) {
			return ErrDuplicateEntry
		}
		return fmt.Errorf("move page %d across workspaces: %w", pageID, err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// SetFracIndexTx writes a frac_index for a single page. Used for one-shot
// backfills when reordering exposes siblings whose frac_index is still NULL
// (pages predating drag-and-drop). The caller is responsible for picking a
// key that preserves the desired sibling order.
func (r *PageRepository) SetFracIndexTx(tx database.Tx, pageID int, fracIndex string, updatedBy int) error {
	now := time.Now().UTC()
	res, err := tx.Exec(`
		UPDATE pages
		SET frac_index = ?,
		    updated_by = ?,
		    updated_at = ?
		WHERE id = ?
	`, fracIndex, updatedBy, now, pageID)
	if err != nil {
		return fmt.Errorf("set frac_index for page %d: %w", pageID, err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// ClearFracIndexesTx temporarily removes pages from the scoped frac_index
// unique index before a sibling set is re-sequenced. Callers must assign final
// keys in the same transaction so the temporary NULL values are never
// committed.
func (r *PageRepository) ClearFracIndexesTx(tx database.Tx, pageIDs []int) error {
	if len(pageIDs) == 0 {
		return nil
	}

	placeholders := make([]string, len(pageIDs))
	args := make([]any, len(pageIDs))
	for i, pageID := range pageIDs {
		placeholders[i] = "?"
		args[i] = pageID
	}
	query := "UPDATE pages SET frac_index = NULL WHERE id IN (" + strings.Join(placeholders, ",") + ")"
	if _, err := tx.Exec(query, args...); err != nil {
		return fmt.Errorf("clear page frac_index values: %w", err)
	}
	return nil
}

// ArchiveTx flags a page (and only this row — descendants are archived by the
// service in a separate pass) as archived. Idempotent: re-archiving a page
// updates archived_at and archived_by to the latest call.
func (r *PageRepository) ArchiveTx(tx database.Tx, pageID, archivedBy int) error {
	now := time.Now().UTC()
	res, err := tx.Exec(`
		UPDATE pages
		SET archived_at = ?,
		    archived_by = ?,
		    updated_at = ?,
		    updated_by = ?
		WHERE id = ?
	`, now, archivedBy, now, archivedBy, pageID)
	if err != nil {
		return fmt.Errorf("archive page %d: %w", pageID, err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// SearchByKeyword returns non-archived pages in the workspace whose title or
// Markdown body contains query, case-insensitively. Title matches sort first.
// The caller must apply per-page ACLs before returning results to a user.
func (r *PageRepository) SearchByKeyword(workspaceID int, query string, limit int) ([]models.Page, error) {
	if limit <= 0 {
		limit = 20
	}
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}
	q = strings.ReplaceAll(q, `\`, `\\`)
	q = strings.ReplaceAll(q, `%`, `\%`)
	q = strings.ReplaceAll(q, `_`, `\_`)
	like := "%" + q + "%"
	rows, err := r.db.Query(`
		SELECT `+pageColumns+`
		FROM pages
		WHERE workspace_id = ?
		  AND archived_at IS NULL
		  AND (
		    LOWER(title) LIKE LOWER(?) ESCAPE '\'
		    OR LOWER(content) LIKE LOWER(?) ESCAPE '\'
		  )
		ORDER BY
		  CASE WHEN LOWER(title) LIKE LOWER(?) ESCAPE '\' THEN 0 ELSE 1 END,
		  title ASC,
		  id ASC
		LIMIT ?
	`, workspaceID, like, like, like, limit)
	if err != nil {
		return nil, fmt.Errorf("search pages by keyword: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []models.Page
	for rows.Next() {
		p, scanErr := scanPage(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan page: %w", scanErr)
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// ListWorkspaceTree returns every (non-archived unless includeArchived) page
// in a workspace, ordered by depth and then by frac_index/rank/title so
// callers can build the tree client-side with a single query.
func (r *PageRepository) ListWorkspaceTree(workspaceID int, includeArchived bool) ([]models.Page, error) {
	return r.listWorkspaceTree(workspaceID, includeArchived, pageColumns, scanPage)
}

// ListWorkspaceTreeMeta is ListWorkspaceTree without the page bodies: it
// selects pageTreeColumns so the heavy content column is never read or
// allocated. Use it for endpoints that render titles + hierarchy only
// (the sidebar tree, the move dialog, the v1 page list). (WI-407.)
func (r *PageRepository) ListWorkspaceTreeMeta(workspaceID int, includeArchived bool) ([]models.Page, error) {
	return r.listWorkspaceTree(workspaceID, includeArchived, pageTreeColumns, scanPageMeta)
}

// listWorkspaceTree is the shared body of the two public variants above; the
// only difference is which columns are selected (and the matching scan).
func (r *PageRepository) listWorkspaceTree(workspaceID int, includeArchived bool, columns string, scan func(rowScanner) (*models.Page, error)) ([]models.Page, error) {
	cond := "workspace_id = ? AND archived_at IS NULL"
	if includeArchived {
		cond = "workspace_id = ?"
	}
	rows, err := r.db.Query(`
		SELECT `+columns+`
		FROM pages
		WHERE `+cond+`
		ORDER BY depth ASC,
		         COALESCE(frac_index, '') ASC,
		         COALESCE(rank, '') ASC,
		         title ASC,
		         id ASC
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list workspace pages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []models.Page
	for rows.Next() {
		page, scanErr := scan(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan page: %w", scanErr)
		}
		out = append(out, *page)
	}
	return out, rows.Err()
}

// ListChildren returns direct children of a page (or root pages when
// parentID is nil), ordered the same way as ListWorkspaceTree.
func (r *PageRepository) ListChildren(workspaceID int, parentID *int) ([]models.Page, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if parentID == nil {
		rows, err = r.db.Query(`
			SELECT `+pageColumns+`
			FROM pages
			WHERE workspace_id = ? AND parent_id IS NULL AND archived_at IS NULL
			ORDER BY COALESCE(frac_index, '') ASC, COALESCE(rank, '') ASC, title ASC, id ASC
		`, workspaceID)
	} else {
		rows, err = r.db.Query(`
			SELECT `+pageColumns+`
			FROM pages
			WHERE workspace_id = ? AND parent_id = ? AND archived_at IS NULL
			ORDER BY COALESCE(frac_index, '') ASC, COALESCE(rank, '') ASC, title ASC, id ASC
		`, workspaceID, *parentID)
	}
	if err != nil {
		return nil, fmt.Errorf("list children: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []models.Page
	for rows.Next() {
		page, scanErr := scanPage(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan page child: %w", scanErr)
		}
		out = append(out, *page)
	}
	return out, rows.Err()
}

// ListChildrenTx mirrors ListChildren but reads inside the caller's
// transaction so a reorder pass sees the same neighbor state it is about
// to update. Returns rows in display order (frac_index, then rank, then
// title, then id) — the same order ListChildren uses.
func (r *PageRepository) ListChildrenTx(tx database.Tx, workspaceID int, parentID *int) ([]models.Page, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if parentID == nil {
		rows, err = tx.Query(`
			SELECT `+pageColumns+`
			FROM pages
			WHERE workspace_id = ? AND parent_id IS NULL AND archived_at IS NULL
			ORDER BY COALESCE(frac_index, '') ASC, COALESCE(rank, '') ASC, title ASC, id ASC
		`, workspaceID)
	} else {
		rows, err = tx.Query(`
			SELECT `+pageColumns+`
			FROM pages
			WHERE workspace_id = ? AND parent_id = ? AND archived_at IS NULL
			ORDER BY COALESCE(frac_index, '') ASC, COALESCE(rank, '') ASC, title ASC, id ASC
		`, workspaceID, *parentID)
	}
	if err != nil {
		return nil, fmt.Errorf("list children (tx): %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []models.Page
	for rows.Next() {
		page, scanErr := scanPage(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan page child: %w", scanErr)
		}
		out = append(out, *page)
	}
	return out, rows.Err()
}

// ListSubtreeTx returns the target page plus every descendant matched
// by the materialized-path prefix. When forUpdate is true (Postgres), rows are
// locked until the caller's transaction commits so permission checks performed
// by the service cannot race a concurrent insert/update in the archived subtree.
func (r *PageRepository) ListSubtreeTx(tx database.Tx, page *models.Page, forUpdate bool) ([]models.Page, error) {
	prefix := page.Path + fmt.Sprintf("%d/", page.ID)
	query := `
		SELECT ` + pageColumns + `
		FROM pages
		WHERE id = ? OR (workspace_id = ? AND path LIKE ?)
		ORDER BY depth ASC, id ASC`
	if forUpdate {
		query += " FOR UPDATE"
	}
	rows, err := tx.Query(query, page.ID, page.WorkspaceID, prefix+"%")
	if err != nil {
		return nil, fmt.Errorf("list page subtree: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []models.Page
	for rows.Next() {
		p, scanErr := scanPage(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan page subtree: %w", scanErr)
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// ListSubtreeForArchiveTx retains the archive-specific call site while the
// same locked subtree primitive is shared with cross-workspace moves.
func (r *PageRepository) ListSubtreeForArchiveTx(tx database.Tx, page *models.Page, forUpdate bool) ([]models.Page, error) {
	return r.ListSubtreeTx(tx, page, forUpdate)
}

// WouldCreatePageCycleTx reports whether reparenting page pageID under
// newParentID would create a cycle. Walks parent_id upward from
// newParentID; encountering pageID — or pageID == newParentID — means a
// cycle would result. If the walk exhausts MaxPageDepth without reaching a
// root, the hierarchy is either already cyclic or too deep; fail-closed
// and return (true, nil).
func (r *PageRepository) WouldCreatePageCycleTx(tx database.Tx, pageID, newParentID int) (bool, error) {
	current := newParentID
	for i := 0; i < MaxPageDepth; i++ {
		if current == pageID {
			return true, nil
		}
		parent, err := r.GetParentIDTx(tx, current)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return false, nil
			}
			return false, fmt.Errorf("walk page hierarchy: %w", err)
		}
		if parent == nil {
			return false, nil
		}
		current = *parent
	}
	return true, nil
}

// --- revisions ---

// pageRevisionColumns mirrors models.PageRevision field order.
const pageRevisionColumns = `id, page_id, revision_number, title, slug, content, content_hash,
	excerpt, parent_id, path, depth, change_summary, change_type, created_by, created_at`

const pageRevisionColumnsAliased = `pr.id, pr.page_id, pr.revision_number, pr.title, pr.slug, pr.content, pr.content_hash,
	pr.excerpt, pr.parent_id, pr.path, pr.depth, pr.change_summary, pr.change_type, pr.created_by, pr.created_at`

func scanPageRevision(s rowScanner) (*models.PageRevision, error) {
	var rev models.PageRevision
	var parentID sql.NullInt64
	if err := s.Scan(
		&rev.ID, &rev.PageID, &rev.RevisionNumber, &rev.Title, &rev.Slug, &rev.Content, &rev.ContentHash,
		&rev.Excerpt, &parentID, &rev.Path, &rev.Depth, &rev.ChangeSummary, &rev.ChangeType,
		&rev.CreatedBy, &rev.CreatedAt,
	); err != nil {
		return nil, err
	}
	if parentID.Valid {
		v := int(parentID.Int64)
		rev.ParentID = &v
	}
	return &rev, nil
}

func scanPageRevisionWithAuthor(s rowScanner) (*models.PageRevision, error) {
	var rev models.PageRevision
	var parentID, authorID sql.NullInt64
	var authorFirst, authorLast, authorUsername sql.NullString
	var authorActive sql.NullBool
	if err := s.Scan(
		&rev.ID, &rev.PageID, &rev.RevisionNumber, &rev.Title, &rev.Slug, &rev.Content, &rev.ContentHash,
		&rev.Excerpt, &parentID, &rev.Path, &rev.Depth, &rev.ChangeSummary, &rev.ChangeType,
		&rev.CreatedBy, &rev.CreatedAt,
		&authorID, &authorFirst, &authorLast, &authorUsername, &authorActive,
	); err != nil {
		return nil, err
	}
	if parentID.Valid {
		v := int(parentID.Int64)
		rev.ParentID = &v
	}
	if authorID.Valid {
		name := strings.TrimSpace(authorFirst.String + " " + authorLast.String)
		if name == "" {
			name = authorUsername.String
		}
		rev.Author = &models.PageRevisionAuthor{
			ID:       int(authorID.Int64),
			Name:     name,
			Username: authorUsername.String,
			IsActive: authorActive.Valid && authorActive.Bool,
		}
	}
	return &rev, nil
}

// NextRevisionNumberTx returns MAX(revision_number)+1 for the given page,
// or 1 when the page has no revisions yet. Run inside the same tx as the
// subsequent insert so revision_number stays unique under concurrent writes.
func (r *PageRepository) NextRevisionNumberTx(tx database.Tx, pageID int) (int, error) {
	var next int
	err := tx.QueryRow(
		"SELECT COALESCE(MAX(revision_number), 0) + 1 FROM page_revisions WHERE page_id = ?",
		pageID,
	).Scan(&next)
	if err != nil {
		return 0, fmt.Errorf("compute next revision number: %w", err)
	}
	return next, nil
}

// InsertRevisionTx persists an immutable snapshot inside an existing tx.
// Returns the new revision id.
func (r *PageRepository) InsertRevisionTx(tx database.Tx, rev models.PageRevision) (int, error) {
	var id int
	err := tx.QueryRow(`
		INSERT INTO page_revisions (
			page_id, revision_number, title, slug, content, content_hash, excerpt,
			parent_id, path, depth, change_summary, change_type, created_by
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		RETURNING id
	`,
		rev.PageID, rev.RevisionNumber, rev.Title, rev.Slug, rev.Content, rev.ContentHash, rev.Excerpt,
		nullInt(rev.ParentID), rev.Path, rev.Depth, rev.ChangeSummary, rev.ChangeType, rev.CreatedBy,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert revision: %w", err)
	}
	return id, nil
}

// GetRevisionByID loads a single revision. Returns ErrNotFound when no row
// matches.
func (r *PageRepository) GetRevisionByID(id int) (*models.PageRevision, error) {
	row := r.db.QueryRow("SELECT "+pageRevisionColumns+" FROM page_revisions WHERE id = ?", id)
	return scanRevisionRow(row, id, "")
}

// GetRevisionByIDTx loads a single revision inside the caller's transaction.
func (r *PageRepository) GetRevisionByIDTx(tx database.Tx, id int) (*models.PageRevision, error) {
	row := tx.QueryRow("SELECT "+pageRevisionColumns+" FROM page_revisions WHERE id = ?", id)
	return scanRevisionRow(row, id, " (tx)")
}

func scanRevisionRow(row rowScanner, id int, suffix string) (*models.PageRevision, error) {
	rev, err := scanPageRevision(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get revision %d%s: %w", id, suffix, err)
	}
	return rev, nil
}

// ListRevisions returns revisions for a page newest-first. limit <= 0
// returns up to 50; clients can paginate via offset for older history.
func (r *PageRepository) ListRevisions(pageID, limit, offset int) ([]models.PageRevision, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := r.db.Query(`
		SELECT `+pageRevisionColumnsAliased+`,
		       u.id, u.first_name, u.last_name, u.username, COALESCE(u.is_active, FALSE)
		FROM page_revisions pr
		LEFT JOIN users u ON u.id = pr.created_by
		WHERE pr.page_id = ?
		ORDER BY pr.revision_number DESC
		LIMIT ? OFFSET ?
	`, pageID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list revisions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []models.PageRevision
	for rows.Next() {
		rev, scanErr := scanPageRevisionWithAuthor(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan revision: %w", scanErr)
		}
		out = append(out, *rev)
	}
	return out, rows.Err()
}

func (r *PageRepository) CountRevisions(pageID int) (int, error) {
	var total int
	if err := r.db.QueryRow("SELECT COUNT(*) FROM page_revisions WHERE page_id = ?", pageID).Scan(&total); err != nil {
		return 0, fmt.Errorf("count page revisions: %w", err)
	}
	return total, nil
}

// --- ACL ---

// GrantPermissionTx inserts an ACL row inside an existing tx. Returns the
// new id. ErrDuplicateEntry on a duplicate (page, principal, level) row.
func (r *PageRepository) GrantPermissionTx(tx database.Tx, in models.PagePermission) (int, error) {
	var id int
	err := tx.QueryRow(`
		INSERT INTO page_permissions (page_id, principal_type, principal_id, permission_level, granted_by)
		VALUES (?, ?, ?, ?, ?)
		RETURNING id
	`, in.PageID, in.PrincipalType, in.PrincipalID, in.PermissionLevel, nullInt(in.GrantedBy)).Scan(&id)
	if err != nil {
		if isUniqueConstraintError(err) {
			return 0, ErrDuplicateEntry
		}
		return 0, fmt.Errorf("grant page permission: %w", err)
	}
	return id, nil
}

// RevokePermissionTx deletes an ACL row by id, but only if it belongs to
// the named page (so cross-page revoke attempts are caught here even if a
// caller composes a request maliciously).
func (r *PageRepository) RevokePermissionTx(tx database.Tx, pageID, permissionID int) error {
	res, err := tx.Exec(`DELETE FROM page_permissions WHERE id = ? AND page_id = ?`, permissionID, pageID)
	if err != nil {
		return fmt.Errorf("revoke page permission: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// SetInheritPermissionsTx flips the inherit_permissions flag on a page.
// Updates updated_at/updated_by so the audit trail stays accurate.
func (r *PageRepository) SetInheritPermissionsTx(tx database.Tx, pageID int, inherit bool, updatedBy int) error {
	res, err := tx.Exec(`
		UPDATE pages
		SET inherit_permissions = ?,
		    updated_by = ?,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, inherit, updatedBy, pageID)
	if err != nil {
		return fmt.Errorf("set inherit_permissions: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// GetPagePermissionByID loads a single ACL row. Returns ErrNotFound when
// no row matches. Used by the handler to verify (and 404) a revoke target
// before involving the service layer.
func (r *PageRepository) GetPagePermissionByID(id int) (*models.PagePermission, error) {
	row := r.db.QueryRow(`
		SELECT id, page_id, principal_type, principal_id, permission_level, granted_by, granted_at
		FROM page_permissions WHERE id = ?
	`, id)
	var p models.PagePermission
	var grantedBy sql.NullInt64
	if err := row.Scan(&p.ID, &p.PageID, &p.PrincipalType, &p.PrincipalID, &p.PermissionLevel, &grantedBy, &p.GrantedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get page permission %d: %w", id, err)
	}
	if grantedBy.Valid {
		v := int(grantedBy.Int64)
		p.GrantedBy = &v
	}
	return &p, nil
}

// ListACLForPage returns the rows stored directly against this page (no
// inheritance). The Phase 2 ACL UI will fetch inherited rows separately so
// admins can see exactly what's set vs. inherited.
func (r *PageRepository) ListACLForPage(pageID int) ([]models.PagePermission, error) {
	rows, err := r.db.Query(`
		SELECT id, page_id, principal_type, principal_id, permission_level, granted_by, granted_at
		FROM page_permissions
		WHERE page_id = ?
		ORDER BY id
	`, pageID)
	if err != nil {
		return nil, fmt.Errorf("list page ACL: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []models.PagePermission
	for rows.Next() {
		var p models.PagePermission
		var grantedBy sql.NullInt64
		if err := rows.Scan(&p.ID, &p.PageID, &p.PrincipalType, &p.PrincipalID, &p.PermissionLevel, &grantedBy, &p.GrantedAt); err != nil {
			return nil, fmt.Errorf("scan ACL row: %w", err)
		}
		if grantedBy.Valid {
			v := int(grantedBy.Int64)
			p.GrantedBy = &v
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// --- chunks ---

const pageChunkColumns = `id, page_id, workspace_id, revision_number, position, heading_path,
	content, token_count, byte_start, byte_end, content_hash, created_at`

func scanPageChunk(s rowScanner) (*models.PageChunk, error) {
	var c models.PageChunk
	if err := s.Scan(
		&c.ID, &c.PageID, &c.WorkspaceID, &c.RevisionNumber, &c.Position, &c.HeadingPath,
		&c.Content, &c.TokenCount, &c.ByteStart, &c.ByteEnd, &c.ContentHash, &c.CreatedAt,
	); err != nil {
		return nil, err
	}
	return &c, nil
}

// DeleteChunksForPageTx removes every chunk row for a page within a tx.
// Used before re-inserting freshly computed chunks.
func (r *PageRepository) DeleteChunksForPageTx(tx database.Tx, pageID int) error {
	_, err := tx.Exec("DELETE FROM page_chunks WHERE page_id = ?", pageID)
	if err != nil {
		return fmt.Errorf("delete page chunks: %w", err)
	}
	return nil
}

// DeleteChunksForSubtreeTx removes chunks for the root page and every
// descendant matched by the materialized-path prefix. Mirrors the WHERE
// clause used by Archive's cascade UPDATE so the chunk index stays in
// step with archived rows.
func (r *PageRepository) DeleteChunksForSubtreeTx(tx database.Tx, rootID, workspaceID int, pathLikePrefix string) error {
	_, err := tx.Exec(`
		DELETE FROM page_chunks
		WHERE page_id IN (
			SELECT id FROM pages
			WHERE id = ? OR (workspace_id = ? AND path LIKE ?)
		)
	`, rootID, workspaceID, pathLikePrefix)
	if err != nil {
		return fmt.Errorf("delete subtree page chunks: %w", err)
	}
	return nil
}

// InsertChunkTx persists a chunk inside the same tx as the page edit that
// produced it.
func (r *PageRepository) InsertChunkTx(tx database.Tx, c models.PageChunk) error {
	_, err := tx.Exec(`
		INSERT INTO page_chunks (
			page_id, workspace_id, revision_number, position, heading_path,
			content, token_count, byte_start, byte_end, content_hash
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, c.PageID, c.WorkspaceID, c.RevisionNumber, c.Position, c.HeadingPath,
		c.Content, c.TokenCount, c.ByteStart, c.ByteEnd, c.ContentHash)
	if err != nil {
		return fmt.Errorf("insert page chunk: %w", err)
	}
	return nil
}

// ListChunksForPage returns chunks in position order. Used by the search
// pipeline once the page passes the permission check.
func (r *PageRepository) ListChunksForPage(pageID int) ([]models.PageChunk, error) {
	rows, err := r.db.Query(`
		SELECT `+pageChunkColumns+`
		FROM page_chunks
		WHERE page_id = ?
		ORDER BY position ASC
	`, pageID)
	if err != nil {
		return nil, fmt.Errorf("list page chunks: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []models.PageChunk
	for rows.Next() {
		c, scanErr := scanPageChunk(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan chunk: %w", scanErr)
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// PageChunkSearchResult is a single ranked search hit. Score range depends
// on the backend (ts_rank floats on Postgres; substring-presence integer on
// SQLite) — callers should treat it as opaque except for sort ordering.
type PageChunkSearchResult struct {
	ChunkID     int
	PageID      int
	WorkspaceID int
	HeadingPath string
	Content     string
	Snippet     string
	Score       float64
}

// SearchChunks runs full-text search over page_chunks in the given
// workspace. The query is treated as a websearch-style phrase on Postgres
// and falls back to case-insensitive LIKE on SQLite. Permission filtering
// is the caller's responsibility (see KnowledgeRetrievalService.Search).
func (r *PageRepository) SearchChunks(workspaceID int, query string, limit int) ([]PageChunkSearchResult, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}

	switch r.db.GetDriverName() {
	case "postgres":
		return r.searchChunksPostgres(workspaceID, query, limit)
	default:
		return r.searchChunksSQLite(workspaceID, query, limit)
	}
}

func (r *PageRepository) searchChunksPostgres(workspaceID int, query string, limit int) ([]PageChunkSearchResult, error) {
	rows, err := r.db.Query(`
		SELECT c.id, c.page_id, c.workspace_id, c.heading_path, c.content,
		       ts_headline('english', c.content, websearch_to_tsquery('english', ?),
		           'MaxWords=40, MinWords=15, StartSel=<mark>, StopSel=</mark>') AS snippet,
		       ts_rank(to_tsvector('english', coalesce(c.heading_path, '') || ' ' || coalesce(c.content, '')),
		           websearch_to_tsquery('english', ?)) AS score
		FROM page_chunks c
		JOIN pages p ON p.id = c.page_id
		WHERE c.workspace_id = ? AND p.archived_at IS NULL
		AND to_tsvector('english', coalesce(c.heading_path, '') || ' ' || coalesce(c.content, ''))
		    @@ websearch_to_tsquery('english', ?)
		ORDER BY score DESC
		LIMIT ?
	`, query, query, workspaceID, query, limit)
	if err != nil {
		return nil, fmt.Errorf("page chunk search (postgres): %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanChunkSearch(rows)
}

func (r *PageRepository) searchChunksSQLite(workspaceID int, query string, limit int) ([]PageChunkSearchResult, error) {
	like := "%" + strings.ToLower(query) + "%"
	rows, err := r.db.Query(`
		SELECT c.id, c.page_id, c.workspace_id, c.heading_path, c.content,
		       '' AS snippet,
		       CAST(
		           (CASE WHEN LOWER(c.heading_path) LIKE ? THEN 2 ELSE 0 END) +
		           (CASE WHEN LOWER(c.content) LIKE ? THEN 1 ELSE 0 END)
		       AS REAL) AS score
		FROM page_chunks c
		JOIN pages p ON p.id = c.page_id
		WHERE c.workspace_id = ? AND p.archived_at IS NULL
		AND (LOWER(c.heading_path) LIKE ? OR LOWER(c.content) LIKE ?)
		ORDER BY score DESC, c.id ASC
		LIMIT ?
	`, like, like, workspaceID, like, like, limit)
	if err != nil {
		return nil, fmt.Errorf("page chunk search (sqlite): %w", err)
	}
	defer func() { _ = rows.Close() }()
	hits, err := scanChunkSearch(rows)
	if err != nil {
		return nil, err
	}
	for i := range hits {
		hits[i].Snippet = centeredSnippet(hits[i].Content, query, 240)
	}
	return hits, nil
}

// centeredSnippet returns up to maxRunes of content. When query occurs in
// content (case-insensitive), the window is shifted so the match lands roughly
// one third in. Rune-based slicing so multi-byte characters survive intact.
// The Postgres path uses ts_headline for an equivalent (better) result; this
// is the SQLite-only fallback.
func centeredSnippet(content, query string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(content)
	if len(runes) <= maxRunes {
		return content
	}

	start := 0
	if q := strings.TrimSpace(query); q != "" {
		idx := strings.Index(strings.ToLower(content), strings.ToLower(q))
		if idx >= 0 {
			// idx is a byte offset; translate to a rune offset.
			matchRune := len([]rune(content[:idx]))
			lead := maxRunes / 3
			start = matchRune - lead
			if start < 0 {
				start = 0
			}
			if start+maxRunes > len(runes) {
				start = len(runes) - maxRunes
			}
		}
	}
	return string(runes[start : start+maxRunes])
}

func scanChunkSearch(rows *sql.Rows) ([]PageChunkSearchResult, error) {
	var out []PageChunkSearchResult
	for rows.Next() {
		var hit PageChunkSearchResult
		if err := rows.Scan(&hit.ChunkID, &hit.PageID, &hit.WorkspaceID, &hit.HeadingPath, &hit.Content, &hit.Snippet, &hit.Score); err != nil {
			return nil, fmt.Errorf("scan chunk search result: %w", err)
		}
		out = append(out, hit)
	}
	return out, rows.Err()
}

// ArchivedPageRow is the joined shape returned by ListArchivedByWorkspace:
// the columns the admin UI needs to display + the archiver's resolved
// display name (empty when the user record is gone or had no name).
type ArchivedPageRow struct {
	ID             int
	Title          string
	Slug           string
	Path           string
	Depth          int
	ArchivedAt     time.Time
	ArchivedBy     *int
	ArchivedByName string
}

// ListArchivedByWorkspace returns every archived page in the workspace
// joined to the archiver's display name. Ordered by archived_at DESC then
// path ASC so the UI shows newest archives first; within an archive batch,
// ancestors precede descendants which lets users unarchive top-down.
//
// Permission filtering is the handler's responsibility (admin-only).
func (r *PageRepository) ListArchivedByWorkspace(workspaceID int) ([]ArchivedPageRow, error) {
	rows, err := r.db.Query(`
		SELECT p.id, p.title, p.slug, p.path, p.depth, p.archived_at, p.archived_by,
		       TRIM(COALESCE(u.first_name, '') || ' ' || COALESCE(u.last_name, '')) AS archived_by_name
		FROM pages p
		LEFT JOIN users u ON u.id = p.archived_by
		WHERE p.workspace_id = ? AND p.archived_at IS NOT NULL
		ORDER BY p.archived_at DESC, p.path ASC, p.id ASC
	`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list archived pages: %w", err)
	}
	defer rows.Close()

	var out []ArchivedPageRow
	for rows.Next() {
		var row ArchivedPageRow
		var archivedBy sql.NullInt64
		if err := rows.Scan(&row.ID, &row.Title, &row.Slug, &row.Path, &row.Depth, &row.ArchivedAt, &archivedBy, &row.ArchivedByName); err != nil {
			return nil, fmt.Errorf("scan archived page: %w", err)
		}
		if archivedBy.Valid {
			v := int(archivedBy.Int64)
			row.ArchivedBy = &v
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// CountWorkspacePages returns the number of non-archived pages in a
// workspace. Used by handlers to short-circuit empty trees.
func (r *PageRepository) CountWorkspacePages(workspaceID int) (int, error) {
	var n int
	err := r.db.QueryRow(
		"SELECT COUNT(*) FROM pages WHERE workspace_id = ? AND archived_at IS NULL",
		workspaceID,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count workspace pages: %w", err)
	}
	return n, nil
}

// --- helpers ---

func nullInt(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullString(v *string) any {
	if v == nil {
		return nil
	}
	return *v
}

// isUniqueConstraintError matches both SQLite and Postgres unique-violation
// errors without depending on driver-specific error types.
func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "unique_violation") ||
		strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "sqlite_constraint_unique") ||
		strings.Contains(msg, "constraint failed: unique")
}
