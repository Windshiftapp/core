// Package services — page_service owns the wiki-pages business rules:
// sanitization, slug derivation, path/depth bookkeeping, cycle prevention,
// and tree assembly. The HTTP handlers, AI tools, and knowledge retrieval
// service all go through PageService rather than touching the repository
// directly. Revisions and search chunks land in a follow-up slice.
package services

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"windshift/internal/database"
	"windshift/internal/itemevents"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
)

// PageService is the entry point for all page CRUD and tree operations.
type PageService struct {
	db         database.Database
	pages      *repository.PageRepository
	pageLabels *repository.PageLabelRepository
}

// NewPageService creates a PageService backed by the provided database.
func NewPageService(db database.Database) *PageService {
	return &PageService{
		db:    db,
		pages: repository.NewPageRepository(db),
	}
}

// SetPageLabelRepository wires the page-label repository for label preload
// on tree/detail responses. Optional — when unset, responses still serialize
// pages but omit the `labels` field (it remains nil/empty).
func (s *PageService) SetPageLabelRepository(repo *repository.PageLabelRepository) {
	s.pageLabels = repo
}

func (s *PageService) pageByIDTx(tx database.Tx, pageID int) (*models.Page, error) {
	page, err := s.pages.GetByIDTx(tx, pageID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrPageNotFound
	}
	return page, err
}

// PreloadLabels populates Labels on each page when a page-label repository
// is wired. Safe to call on an empty slice or when the repo is unset.
func (s *PageService) PreloadLabels(pages []models.Page) error {
	if s.pageLabels == nil {
		return nil
	}
	return s.pageLabels.LoadForPages(pages)
}

// PreloadLabelsForPage loads labels for a single page. No-op when no
// page-label repository is wired.
func (s *PageService) PreloadLabelsForPage(page *models.Page) error {
	if s.pageLabels == nil || page == nil {
		return nil
	}
	labels, err := s.pageLabels.ListForPage(page.ID)
	if err != nil {
		return err
	}
	if labels == nil {
		labels = []models.PageLabel{}
	}
	page.Labels = labels
	return nil
}

// Service-level errors. Wraps repository errors so the handler layer can
// map them to HTTP status codes without knowing repository internals.
var (
	ErrPageNotFound       = errors.New("page not found")
	ErrPageTitleRequired  = errors.New("page title is required")
	ErrPageParentMismatch = errors.New("parent page belongs to a different workspace")
	ErrPageCycle          = errors.New("move would create a cycle")
	ErrPageDepthExceeded  = errors.New("page tree depth limit exceeded")
	// ErrPageUniqueConflict covers any uniqueness rule the pages table still
	// enforces: one home page per workspace (idx_pages_workspace_home) and
	// one frac_index per sibling set (idx_pages_frac_index_scoped). Slugs
	// used to be in this set and no longer are.
	ErrPageUniqueConflict   = errors.New("page conflicts with an existing page")
	ErrPageContentConflict  = errors.New("page content changed since it was read")
	ErrPageRevisionMismatch = errors.New("revision does not belong to the target page")
	ErrPageMetadataInvalid  = errors.New("page metadata must be a JSON object")
)

// CreatePageInput is the request shape for Create. Permission inheritance
// is always true on create; the permissions dialog (Phase 2) lets an
// admin break inheritance later.
type CreatePageInput struct {
	WorkspaceID int
	ParentID    *int
	Title       string
	Metadata    json.RawMessage
	Content     string
	IsHome      bool
	Rank        *string
	FracIndex   *string
}

// Create inserts a new page after sanitizing inputs and computing derived
// columns. Returns the persisted page.
func (s *PageService) Create(actorID int, in CreatePageInput) (*models.Page, error) {
	title := sanitize.PlainTextField.Sanitize(in.Title)
	if title == "" {
		return nil, ErrPageTitleRequired
	}
	metadata, err := normalizePageMetadata(in.Metadata)
	if err != nil {
		return nil, err
	}

	content := sanitize.LongDocument.Sanitize(in.Content)
	excerpt := deriveExcerpt(content)
	hash := contentHash(content)

	// Creation always inherits permissions; admins can change this later.
	inherit := true

	return database.WithTxResult(s.db, func(tx database.Tx) (*models.Page, error) {
		parentID, parentPath, parentDepth, err := s.resolveParent(tx, in.WorkspaceID, in.ParentID)
		if err != nil {
			return nil, err
		}
		depth := parentDepth + 1
		if in.ParentID == nil {
			depth = 0
		}
		if depth >= repository.MaxPageDepth {
			return nil, ErrPageDepthExceeded
		}

		id, err := s.pages.CreateTx(tx, repository.CreateInput{
			WorkspaceID:        in.WorkspaceID,
			ParentID:           parentID,
			Title:              title,
			Slug:               makeSlug(title),
			Metadata:           metadata,
			Content:            content,
			ContentHash:        hash,
			Excerpt:            excerpt,
			CreatedBy:          actorID,
			IsHome:             in.IsHome,
			InheritPermissions: inherit,
			Rank:               in.Rank,
			FracIndex:          in.FracIndex,
			Path:               parentPath,
			Depth:              depth,
		})
		if err != nil {
			if errors.Is(err, repository.ErrDuplicateEntry) {
				return nil, ErrPageUniqueConflict
			}
			return nil, err
		}

		page, err := s.pages.GetByIDTx(tx, id)
		if err != nil {
			return nil, err
		}

		if err := s.snapshotAndRebuildChunks(tx, page, actorID, models.PageRevisionChangeTypeCreate, ""); err != nil {
			return nil, err
		}
		return page, nil
	})
}

// GetByID returns a single page, or ErrPageNotFound when no row matches.
// Workspace scoping is the caller's responsibility — the handler layer
// checks workspace membership and runs the page ACL evaluator.
func (s *PageService) GetByID(id int) (*models.Page, error) {
	page, err := s.pages.GetByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrPageNotFound
		}
		return nil, err
	}
	return page, nil
}

// UpdatePageInput is the request shape for Update. InheritPermissions is
// intentionally absent: inheritance changes go through SetInheritPermissions
// (PageOpAdmin) — accepting it here would let an editor flip the flag via
// a normal title/content save, bypassing the admin gate. Rank / FracIndex
// are absent for the same reason: reordering goes through Move /
// SetFracIndex so a normal save cannot clear an existing ordering.
type UpdatePageInput struct {
	ID                  int
	Title               string
	Content             string
	Metadata            *json.RawMessage
	ExpectedContentHash *string
}

// Update overwrites a page's title/content and recomputes the derived
// columns. Inheritance, parent (Move), and archive each have their own
// admin-gated call so the audit trail and handler authorization paths
// stay distinct.
func (s *PageService) Update(actorID int, in UpdatePageInput) (*models.Page, error) {
	title := sanitize.PlainTextField.Sanitize(in.Title)
	if title == "" {
		return nil, ErrPageTitleRequired
	}
	content := sanitize.LongDocument.Sanitize(in.Content)
	excerpt := deriveExcerpt(content)
	hash := contentHash(content)
	var metadata *string
	if in.Metadata != nil {
		normalized, metaErr := normalizePageMetadata(*in.Metadata)
		if metaErr != nil {
			return nil, metaErr
		}
		metadata = &normalized
	}

	return database.WithTxResult(s.db, func(tx database.Tx) (*models.Page, error) {
		existing, err := s.pages.GetByIDTx(tx, in.ID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return nil, ErrPageNotFound
			}
			return nil, err
		}
		if in.ExpectedContentHash != nil && existing.ContentHash != *in.ExpectedContentHash {
			return nil, ErrPageContentConflict
		}

		newSlug := existing.Slug
		if !strings.EqualFold(title, existing.Title) {
			newSlug = makeSlug(title)
		}

		err = s.pages.UpdateTx(tx, repository.UpdateInput{
			ID:                 in.ID,
			Title:              title,
			Slug:               newSlug,
			Content:            content,
			ContentHash:        hash,
			Excerpt:            excerpt,
			InheritPermissions: existing.InheritPermissions,
			Metadata:           metadata,
			UpdatedBy:          actorID,
		})
		if err != nil {
			if errors.Is(err, repository.ErrDuplicateEntry) {
				return nil, ErrPageUniqueConflict
			}
			if errors.Is(err, repository.ErrNotFound) {
				return nil, ErrPageNotFound
			}
			return nil, err
		}

		updated, err := s.pages.GetByIDTx(tx, in.ID)
		if err != nil {
			return nil, err
		}
		if err := s.snapshotAndRebuildChunks(tx, updated, actorID, models.PageRevisionChangeTypeEdit, ""); err != nil {
			return nil, err
		}
		return updated, nil
	})
}

// Move reparents a page to a new parent. Cycle detection and depth check
// run inside the transaction; descendants' paths/depths are updated in the
// same pass so the tree stays consistent.
//
// prevSiblingID / nextSiblingID position the moved page within its new
// parent's children. Either may be nil to mean "start of list" / "end of
// list"; when both are nil the existing append-by-natural-order behavior
// is preserved (no frac_index write).
func (s *PageService) Move(actorID, pageID int, newParentID, prevSiblingID, nextSiblingID *int) (*models.Page, error) {
	return database.WithTxResult(s.db, func(tx database.Tx) (*models.Page, error) {
		page, err := s.pageByIDTx(tx, pageID)
		if err != nil {
			return nil, err
		}
		destination, err := s.resolvePageMoveDestinationTx(tx, pageID, page.WorkspaceID, newParentID, true)
		if err != nil {
			return nil, err
		}
		if err := s.validateStoredSubtreeMoveDepthTx(tx, page, destination.depth); err != nil {
			return nil, err
		}

		// Recompute the rank when siblings or the parent change, backfilling missing neighbor keys.
		parentChanged := !samePageParent(page.ParentID, newParentID)
		newFracIndex, err := s.resolveSiblingFracIndex(tx, page.WorkspaceID, newParentID, pageID, prevSiblingID, nextSiblingID, parentChanged, actorID)
		if err != nil {
			return nil, err
		}

		if err := s.pages.MoveTx(tx, pageID, newParentID, destination.path, destination.depth, actorID, newFracIndex); err != nil {
			return nil, pageMoveError(err)
		}

		oldPrefix := page.Path + fmt.Sprintf("%d/", page.ID)
		newPrefix := destination.path + fmt.Sprintf("%d/", pageID)
		depthShift := destination.depth - page.Depth
		if oldPrefix != newPrefix || depthShift != 0 {
			_, execErr := tx.Exec(`
				UPDATE pages
				SET path = ? || SUBSTR(path, ?),
				    depth = depth + ?
				WHERE workspace_id = ?
				  AND path LIKE ?
			`, newPrefix, len(oldPrefix)+1, depthShift, page.WorkspaceID, oldPrefix+"%")
			if execErr != nil {
				return nil, fmt.Errorf("rewrite descendant paths: %w", execErr)
			}
		}
		return s.finishPageMoveTx(tx, pageID, actorID, "")
	})
}

// MoveAcrossWorkspace moves a page and its complete subtree into another
// workspace. The hierarchy rewrite and every workspace-scoped relation change
// commit atomically. Revision rows and attachments intentionally stay attached
// to their page IDs: revisions are historical snapshots, while attachments are
// not workspace-scoped.
func (s *PageService) MoveAcrossWorkspace(actorID, pageID, destinationWorkspaceID int, newParentID, prevSiblingID, nextSiblingID *int) (*models.Page, error) {
	return database.WithTxResult(s.db, func(tx database.Tx) (*models.Page, error) {
		page, err := s.pageByIDTx(tx, pageID)
		if err != nil {
			return nil, err
		}
		if page.WorkspaceID == destinationWorkspaceID {
			return nil, fmt.Errorf("cross-workspace move requires a different destination workspace")
		}

		subtree, err := s.pages.ListSubtreeTx(tx, page, s.db.GetDriverName() == "postgres")
		if err != nil {
			return nil, err
		}
		if len(subtree) == 0 {
			return nil, ErrPageNotFound
		}

		destination, err := s.resolvePageMoveDestinationTx(tx, pageID, destinationWorkspaceID, newParentID, false)
		if err != nil {
			return nil, err
		}
		if subtreeMoveExceedsDepth(page.Depth, deepestPageDepth(page.Depth, subtree), destination.depth) {
			return nil, ErrPageDepthExceeded
		}

		newFracIndex, err := s.resolveSiblingFracIndex(tx, destinationWorkspaceID, newParentID, pageID, prevSiblingID, nextSiblingID, true, actorID)
		if err != nil {
			return nil, err
		}
		if err := s.pages.MoveAcrossWorkspaceTx(tx, pageID, destinationWorkspaceID, newParentID, destination.path, destination.depth, actorID, newFracIndex); err != nil {
			return nil, pageMoveError(err)
		}
		pageIDs, err := s.movePageDescendantsAcrossWorkspaceTx(tx, page, subtree, destinationWorkspaceID, destination, actorID)
		if err != nil {
			return nil, err
		}
		if err := s.rehomePageSubtreeRelationsTx(tx, pageIDs, destinationWorkspaceID, actorID); err != nil {
			return nil, err
		}
		return s.finishPageMoveTx(tx, pageID, actorID, "Moved to another workspace")
	})
}

type pageMoveDestination struct {
	path  string
	depth int
}

func (s *PageService) resolvePageMoveDestinationTx(tx database.Tx, pageID, workspaceID int, parentID *int, preventCycle bool) (pageMoveDestination, error) {
	if parentID == nil {
		return pageMoveDestination{path: "/"}, nil
	}
	if preventCycle {
		cyclic, err := s.pages.WouldCreatePageCycleTx(tx, pageID, *parentID)
		if err != nil {
			return pageMoveDestination{}, err
		}
		if cyclic {
			return pageMoveDestination{}, ErrPageCycle
		}
	}
	parent, err := s.pageByIDTx(tx, *parentID)
	if err != nil {
		return pageMoveDestination{}, err
	}
	if parent.WorkspaceID != workspaceID {
		return pageMoveDestination{}, ErrPageParentMismatch
	}
	destination := pageMoveDestination{
		path:  parent.Path + fmt.Sprintf("%d/", parent.ID),
		depth: parent.Depth + 1,
	}
	if destination.depth >= repository.MaxPageDepth {
		return pageMoveDestination{}, ErrPageDepthExceeded
	}
	return destination, nil
}

func (s *PageService) validateStoredSubtreeMoveDepthTx(tx database.Tx, page *models.Page, newDepth int) error {
	prefix := page.Path + fmt.Sprintf("%d/", page.ID)
	var deepestDescendant sql.NullInt64
	if err := tx.QueryRow(
		`SELECT MAX(depth) FROM pages WHERE workspace_id = ? AND path LIKE ?`,
		page.WorkspaceID, prefix+"%",
	).Scan(&deepestDescendant); err != nil {
		return fmt.Errorf("measure subtree depth: %w", err)
	}
	if deepestDescendant.Valid && subtreeMoveExceedsDepth(page.Depth, int(deepestDescendant.Int64), newDepth) {
		return ErrPageDepthExceeded
	}
	return nil
}

func subtreeMoveExceedsDepth(oldRootDepth, deepestDepth, newRootDepth int) bool {
	return deepestDepth-oldRootDepth+newRootDepth >= repository.MaxPageDepth
}

func deepestPageDepth(rootDepth int, subtree []models.Page) int {
	deepest := rootDepth
	for i := range subtree {
		if subtree[i].Depth > deepest {
			deepest = subtree[i].Depth
		}
	}
	return deepest
}

func (s *PageService) movePageDescendantsAcrossWorkspaceTx(tx database.Tx, page *models.Page, subtree []models.Page, workspaceID int, destination pageMoveDestination, actorID int) ([]int, error) {
	oldPrefix := page.Path + fmt.Sprintf("%d/", page.ID)
	newPrefix := destination.path + fmt.Sprintf("%d/", page.ID)
	depthShift := destination.depth - page.Depth
	pageIDs := make([]int, 0, len(subtree))
	pageIDs = append(pageIDs, page.ID)
	for i := range subtree {
		descendant := &subtree[i]
		if descendant.ID == page.ID {
			continue
		}
		if !strings.HasPrefix(descendant.Path, oldPrefix) {
			return nil, fmt.Errorf("page %d is outside subtree prefix %q", descendant.ID, oldPrefix)
		}
		descendantPath := newPrefix + strings.TrimPrefix(descendant.Path, oldPrefix)
		if err := s.pages.MoveAcrossWorkspaceTx(tx, descendant.ID, workspaceID, descendant.ParentID, descendantPath, descendant.Depth+depthShift, actorID, descendant.FracIndex); err != nil {
			return nil, pageMoveError(err)
		}
		pageIDs = append(pageIDs, descendant.ID)
	}
	return pageIDs, nil
}

func (s *PageService) finishPageMoveTx(tx database.Tx, pageID, actorID int, summary string) (*models.Page, error) {
	moved, err := s.pageByIDTx(tx, pageID)
	if err != nil {
		return nil, err
	}
	if _, err := s.writeRevisionTx(tx, moved, actorID, models.PageRevisionChangeTypeMove, summary); err != nil {
		return nil, err
	}
	return moved, nil
}

func pageMoveError(err error) error {
	if errors.Is(err, repository.ErrNotFound) {
		return ErrPageNotFound
	}
	if errors.Is(err, repository.ErrDuplicateEntry) {
		return ErrPageUniqueConflict
	}
	return err
}

// rehomePageSubtreeRelationsTx applies the explicit cross-workspace policy:
// labels map by exact name when the destination already has one; unmatched
// labels, explicit ACLs, item links, and workspace-agent skill references are
// removed. Search chunks follow the page. Attachments and revisions are not
// workspace-scoped and remain untouched.
func (s *PageService) rehomePageSubtreeRelationsTx(tx database.Tx, pageIDs []int, destinationWorkspaceID, actorID int) error {
	if len(pageIDs) == 0 {
		return nil
	}
	query := newPageIDQuery(pageIDs)
	assignments, err := loadPageLabelAssignmentsTx(tx, query.idList, query.args)
	if err != nil {
		return err
	}
	destinationLabels, err := loadPageLabelsByNameTx(tx, destinationWorkspaceID)
	if err != nil {
		return err
	}
	if err := remapPageLabelsTx(tx, query.idList, query.args, assignments, destinationLabels); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM page_permissions WHERE page_id IN (`+query.idList+`)`, query.args...); err != nil {
		return fmt.Errorf("clear page permissions during workspace move: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM workspace_agent_skill_pages WHERE page_id IN (`+query.idList+`)`, query.args...); err != nil {
		return fmt.Errorf("clear workspace skill page references during workspace move: %w", err)
	}
	linkArgs := append(append([]any{}, query.args...), query.args...)
	metadata := itemevents.User(actorID, "application")
	if err := itemevents.NewRecorder(s.db).RemovedLinks(context.Background(), tx, "page", pageIDs, metadata); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		DELETE FROM item_links
		WHERE (source_type = 'page' AND source_id IN (`+query.idList+`))
		   OR (target_type = 'page' AND target_id IN (`+query.idList+`))
	`, linkArgs...); err != nil {
		return fmt.Errorf("clear page links during workspace move: %w", err)
	}
	chunkArgs := append([]any{destinationWorkspaceID}, query.args...)
	if _, err := tx.Exec(`UPDATE page_chunks SET workspace_id = ? WHERE page_id IN (`+query.idList+`)`, chunkArgs...); err != nil {
		return fmt.Errorf("rehome page chunks during workspace move: %w", err)
	}
	return nil
}

type pageLabelAssignment struct {
	pageID int
	name   string
}

type pageIDQuery struct {
	idList string
	args   []any
}

func newPageIDQuery(pageIDs []int) pageIDQuery {
	placeholders := make([]string, len(pageIDs))
	args := make([]any, len(pageIDs))
	for i, pageID := range pageIDs {
		placeholders[i] = "?"
		args[i] = pageID
	}
	return pageIDQuery{idList: strings.Join(placeholders, ","), args: args}
}

func loadPageLabelAssignmentsTx(tx database.Tx, idList string, args []any) ([]pageLabelAssignment, error) {
	rows, err := tx.Query(`
		SELECT a.page_id, l.name
		FROM page_label_assignments a
		JOIN page_labels l ON l.id = a.page_label_id
		WHERE a.page_id IN (`+idList+`)
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("load page labels before workspace move: %w", err)
	}
	assignments := make([]pageLabelAssignment, 0)
	for rows.Next() {
		var assignment pageLabelAssignment
		if err := rows.Scan(&assignment.pageID, &assignment.name); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan page label before workspace move: %w", err)
		}
		assignments = append(assignments, assignment)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate page labels before workspace move: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close page label rows: %w", err)
	}
	return assignments, nil
}

func loadPageLabelsByNameTx(tx database.Tx, workspaceID int) (map[string]int, error) {
	rows, err := tx.Query(`SELECT id, name FROM page_labels WHERE workspace_id = ?`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("load destination page labels: %w", err)
	}
	labels := make(map[string]int)
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan destination page label: %w", err)
		}
		labels[name] = id
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate destination page labels: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close destination page label rows: %w", err)
	}
	return labels, nil
}

func remapPageLabelsTx(tx database.Tx, idList string, args []any, assignments []pageLabelAssignment, destinationLabels map[string]int) error {
	if _, err := tx.Exec(`DELETE FROM page_label_assignments WHERE page_id IN (`+idList+`)`, args...); err != nil {
		return fmt.Errorf("clear page labels during workspace move: %w", err)
	}
	for _, assignment := range assignments {
		destinationLabelID, ok := destinationLabels[assignment.name]
		if !ok {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO page_label_assignments (page_id, page_label_id) VALUES (?, ?)`, assignment.pageID, destinationLabelID); err != nil {
			return fmt.Errorf("remap page label %q during workspace move: %w", assignment.name, err)
		}
	}
	return nil
}

// Archive flags a page (and its entire subtree) as archived. Archive is
// reversible by restoring an explicit revision, which unarchives only the
// addressed page. Use ArchiveChecked from HTTP handlers so descendant ACL
// checks run inside the archive transaction.
//
// deadcode-keep: called by core-tests/internal/services/page_service_test.go;
// production handlers use ArchiveChecked.
func (s *PageService) Archive(actorID, pageID int) error {
	return s.ArchiveChecked(actorID, pageID, nil)
}

// ArchiveChecked locks the page subtree, runs an optional authorization check
// over the exact rows that will be archived, then archives the same locked set.
// This closes the handler-level TOCTOU where a restricted descendant could be
// inserted between an out-of-transaction descendant scan and the archive UPDATE.
func (s *PageService) ArchiveChecked(actorID, pageID int, authorize func([]models.Page) error) error {
	return database.WithTx(s.db, func(tx database.Tx) error {
		page, err := s.pageByIDTx(tx, pageID)
		if err != nil {
			return err
		}

		prefix := page.Path + fmt.Sprintf("%d/", page.ID)
		pathLike := prefix + "%"
		subtree, err := s.pages.ListSubtreeForArchiveTx(tx, page, s.db.GetDriverName() == "postgres")
		if err != nil {
			return err
		}
		if len(subtree) == 0 {
			return ErrPageNotFound
		}

		// Only pages that are not already archived actually transition state.
		// Re-touching an already-archived descendant would (a) fail the caller's
		// admin authorization — archived pages are frozen to every op but
		// view/restore, so the check returns false and surfaces as a misleading
		// 404 — and (b) needlessly reset its archived_at and append a spurious
		// "archived with subtree" revision. So scope authorization and the write
		// to the live subset.
		live := make([]models.Page, 0, len(subtree))
		for i := range subtree {
			if subtree[i].ArchivedAt == nil {
				live = append(live, subtree[i])
			}
		}
		if len(live) == 0 {
			// The whole subtree is already archived; nothing to do.
			return nil
		}
		if authorize != nil {
			if err := authorize(live); err != nil {
				return err
			}
		}

		// Archive the page and every not-yet-archived descendant by
		// materialized-path prefix. A single statement keeps the operation
		// atomic and targets the same locked subtree rows authorized above; the
		// archived_at IS NULL guard leaves already-archived rows untouched.
		if _, err := tx.Exec(`
			UPDATE pages
			SET archived_at = CURRENT_TIMESTAMP,
			    archived_by = ?,
			    updated_at = CURRENT_TIMESTAMP,
			    updated_by = ?
			WHERE (id = ? OR (workspace_id = ? AND path LIKE ?)) AND archived_at IS NULL
		`, actorID, actorID, pageID, page.WorkspaceID, pathLike); err != nil {
			return fmt.Errorf("archive subtree: %w", err)
		}

		// Drop the now-stale chunks for the archived subtree so search and
		// AI tools cannot surface content from a hidden page even before
		// the permission filter runs.
		if err := s.pages.DeleteChunksForSubtreeTx(tx, page.ID, page.WorkspaceID, pathLike); err != nil {
			return err
		}

		// Every newly archived row gets its own revision entry so descendants
		// have a local audit trail explaining why/when they disappeared.
		// Already-archived rows are skipped — they kept their original archive
		// revision and weren't re-touched above.
		for i := range live {
			if _, err := s.writeRevisionTx(tx, &live[i], actorID, models.PageRevisionChangeTypeArchive, "archived with subtree"); err != nil {
				return err
			}
		}
		return nil
	})
}

// GetRevision returns a single revision by id, or ErrPageNotFound when no
// row matches.
func (s *PageService) GetRevision(id int) (*models.PageRevision, error) {
	rev, err := s.pages.GetRevisionByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrPageNotFound
		}
		return nil, err
	}
	return rev, nil
}

// ListRevisions returns the revision history for a page, newest first.
func (s *PageService) ListRevisions(pageID, limit, offset int) ([]models.PageRevision, error) {
	return s.pages.ListRevisions(pageID, limit, offset)
}

func (s *PageService) CountRevisions(pageID int) (int, error) {
	return s.pages.CountRevisions(pageID)
}

// Restore overwrites a page's live content/title with the snapshot stored
// in the given revision and records a new revision of change_type
// 'restore'. The revision must belong to the same page; cross-page
// restores return ErrPageRevisionMismatch.
func (s *PageService) Restore(actorID, pageID, revisionID int) (*models.Page, error) {
	return database.WithTxResult(s.db, func(tx database.Tx) (*models.Page, error) {
		page, err := s.pageByIDTx(tx, pageID)
		if err != nil {
			return nil, err
		}
		rev, err := s.pages.GetRevisionByIDTx(tx, revisionID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return nil, ErrPageNotFound
			}
			return nil, err
		}
		if rev.PageID != pageID {
			return nil, ErrPageRevisionMismatch
		}

		// Restore overwrites title/slug/content/excerpt/hash on the live row.
		// parent/path/depth/rank/frac_index are deliberately not restored —
		// moving and reordering a page are separate explicit actions, and
		// UpdateTx no longer touches those columns. If a user wants to undo
		// a move, they should run Move explicitly.
		if err := s.pages.UpdateTx(tx, repository.UpdateInput{
			ID:                 page.ID,
			Title:              rev.Title,
			Slug:               rev.Slug,
			Content:            rev.Content,
			ContentHash:        rev.ContentHash,
			Excerpt:            rev.Excerpt,
			InheritPermissions: page.InheritPermissions,
			UpdatedBy:          actorID,
			Unarchive:          page.ArchivedAt != nil,
		}); err != nil {
			if errors.Is(err, repository.ErrDuplicateEntry) {
				return nil, ErrPageUniqueConflict
			}
			return nil, err
		}

		restored, err := s.pages.GetByIDTx(tx, page.ID)
		if err != nil {
			return nil, err
		}
		if err := s.snapshotAndRebuildChunks(tx, restored, actorID, models.PageRevisionChangeTypeRestore, fmt.Sprintf("restored from revision %d", rev.RevisionNumber)); err != nil {
			return nil, err
		}
		return restored, nil
	})
}

// ListArchived returns every archived page in the workspace plus the
// archiver's resolved display name. Caller must enforce admin access.
func (s *PageService) ListArchived(workspaceID int) ([]repository.ArchivedPageRow, error) {
	return s.pages.ListArchivedByWorkspace(workspaceID)
}

// Unarchive flips a single page back to active by clearing archived_at /
// archived_by, without overwriting the page's content. A new revision is
// recorded (change_type=restore, summary="unarchived") so the history
// reflects the action. No-op when the page wasn't archived.
//
// Single-page only by design — Archive cascades via materialized path,
// but unarchiving is symmetric with Restore: each addressed page is
// opted into explicitly. A page whose parent remains archived stays
// hidden from the tree until the ancestor is also unarchived, matching
// existing Restore semantics.
func (s *PageService) Unarchive(actorID, pageID int) (*models.Page, error) {
	return database.WithTxResult(s.db, func(tx database.Tx) (*models.Page, error) {
		page, err := s.pageByIDTx(tx, pageID)
		if err != nil {
			return nil, err
		}
		if page.ArchivedAt == nil {
			return page, nil
		}
		// While archived the page held no slot in the live frac_index key
		// space (idx_pages_frac_index_scoped excludes archived rows), so a
		// live sibling may have since minted the key this page still owns.
		// Clear it now, while the row is still archived and out of the
		// index, so re-entering the index below can't hit a unique
		// collision. A NULL frac_index sorts to the end and is backfilled
		// on the next reorder, matching how legacy pre-drag-and-drop pages
		// are treated.
		if err := s.pages.ClearFracIndexesTx(tx, []int{page.ID}); err != nil {
			return nil, fmt.Errorf("clear frac_index on unarchive: %w", err)
		}
		if err := s.pages.UpdateTx(tx, repository.UpdateInput{
			ID:                 page.ID,
			Title:              page.Title,
			Slug:               page.Slug,
			Content:            page.Content,
			ContentHash:        page.ContentHash,
			Excerpt:            page.Excerpt,
			InheritPermissions: page.InheritPermissions,
			UpdatedBy:          actorID,
			Unarchive:          true,
		}); err != nil {
			return nil, err
		}
		fresh, err := s.pages.GetByIDTx(tx, page.ID)
		if err != nil {
			return nil, err
		}
		if _, err := s.writeRevisionTx(tx, fresh, actorID, models.PageRevisionChangeTypeRestore, "unarchived"); err != nil {
			return nil, err
		}
		return fresh, nil
	})
}

// writeRevisionTx persists a revision row for the given page snapshot.
// Used by every page-mutating operation so the history is always complete.
// Returns the revision_number that was just written.
func (s *PageService) writeRevisionTx(tx database.Tx, page *models.Page, actorID int, changeType, summary string) (int, error) {
	next, err := s.pages.NextRevisionNumberTx(tx, page.ID)
	if err != nil {
		return 0, err
	}
	if _, err := s.pages.InsertRevisionTx(tx, models.PageRevision{
		PageID:         page.ID,
		RevisionNumber: next,
		Title:          page.Title,
		Slug:           page.Slug,
		Content:        page.Content,
		ContentHash:    page.ContentHash,
		Excerpt:        page.Excerpt,
		ParentID:       page.ParentID,
		Path:           page.Path,
		Depth:          page.Depth,
		ChangeSummary:  summary,
		ChangeType:     changeType,
		CreatedBy:      actorID,
	}); err != nil {
		return 0, err
	}
	return next, nil
}

// snapshotAndRebuildChunks persists a revision and rebuilds the page chunk
// table in a single transaction. Use this on every content-affecting
// operation (create, edit, restore). Move uses writeRevisionTx directly
// because the content hasn't changed.
func (s *PageService) snapshotAndRebuildChunks(tx database.Tx, page *models.Page, actorID int, changeType, summary string) error {
	revisionNumber, err := s.writeRevisionTx(tx, page, actorID, changeType, summary)
	if err != nil {
		return err
	}
	if err := s.pages.DeleteChunksForPageTx(tx, page.ID); err != nil {
		return err
	}
	if page.Content == "" {
		return nil
	}
	specs := chunkPageMarkdown(page.Content)
	chunks := buildPageChunks(page, revisionNumber, specs)
	for _, chunk := range chunks {
		if err := s.pages.InsertChunkTx(tx, chunk); err != nil {
			return err
		}
	}
	return nil
}

// ListTree returns every non-archived page in a workspace. Rows are
// grouped by depth (all roots, then all depth-1 children, …) and
// within each depth-band sorted by frac_index, rank, title, id —
// i.e. breadth-by-depth, not depth-first. Callers that care about
// rendering order rebuild the tree via BuildPageTree, which is
// id-based and order-insensitive.
func (s *PageService) ListTree(workspaceID int, includeArchived bool) ([]models.Page, error) {
	return s.pages.ListWorkspaceTree(workspaceID, includeArchived)
}

// ListTreeMeta is ListTree without the page bodies. The tree/list endpoints
// render titles + hierarchy only, so the heavy content column is projected
// out of the query — never read off disk or allocated — rather than loaded
// and then discarded. Prefer this over ListTree wherever the body is unused.
// (WI-407.)
func (s *PageService) ListTreeMeta(workspaceID int, includeArchived bool) ([]models.Page, error) {
	return s.pages.ListWorkspaceTreeMeta(workspaceID, includeArchived)
}

// SearchByKeyword delegates to the repository's title-and-body substring
// search. Permission filtering happens at the handler layer.
func (s *PageService) SearchByKeyword(workspaceID int, query string, limit int) ([]models.Page, error) {
	return s.pages.SearchByKeyword(workspaceID, query, limit)
}

// BuildPageTree turns a flat ordered page list (typically from ListTree)
// into a nested PageNode tree suitable for direct rendering in internal tools.
func BuildPageTree(pages []models.Page) []*models.PageNode {
	byID := make(map[int]*models.PageNode, len(pages))
	for i := range pages {
		node := &models.PageNode{Page: pages[i]}
		byID[pages[i].ID] = node
	}
	var roots []*models.PageNode
	for i := range pages {
		node := byID[pages[i].ID]
		if pages[i].ParentID == nil {
			roots = append(roots, node)
			continue
		}
		parent, ok := byID[*pages[i].ParentID]
		if !ok {
			// Promote orphans so callers do not silently drop visible pages.
			roots = append(roots, node)
			continue
		}
		parent.Children = append(parent.Children, node)
	}
	return roots
}

// ListChildren returns direct children of the given parent (root pages when
// parentID is nil), ordered by frac_index/rank/title.
func (s *PageService) ListChildren(workspaceID int, parentID *int) ([]models.Page, error) {
	return s.pages.ListChildren(workspaceID, parentID)
}

// ListOwnACL returns the ACL rows stored directly against a page (no
// inheritance). Used by the read-only permissions endpoint in Phase 1; the
// Phase 2 dialog will use this plus a separate inheritance-walk endpoint.
func (s *PageService) ListOwnACL(pageID int) ([]models.PagePermission, error) {
	return s.pages.ListACLForPage(pageID)
}

// --- ACL writes (Phase 2) ---

// ErrPageInvalidPrincipal is returned when a Grant call names a principal
// type the data model doesn't accept (anything outside user/group/role).
var ErrPageInvalidPrincipal = errors.New("invalid principal_type")

// ErrPageInvalidLevel is returned when a Grant call names a permission
// level the data model doesn't accept.
var ErrPageInvalidLevel = errors.New("invalid permission_level")

// ErrPagePermissionDuplicate is returned when the (page, principal, level)
// tuple already exists. The caller can ignore this as a no-op or surface
// it to the user.
var ErrPagePermissionDuplicate = errors.New("permission already granted")

// ErrPageGrantPrincipalNotFound is returned when GrantPermission is asked
// to attach an ACL row to a user/group/role that does not exist or is
// disabled. The runtime evaluator already requires workspace membership
// for the match to count, but rejecting unknown principals at write time
// prevents stale-id grants from sitting in the audit log forever.
var ErrPageGrantPrincipalNotFound = errors.New("principal does not exist")

// GrantPermission attaches an ACL row to a page. Writes a 'permissions'
// revision so the audit trail captures the change. Returns the persisted
// row.
func (s *PageService) GrantPermission(actorID, pageID int, principalType string, principalID int, level string) (*models.PagePermission, error) {
	if !isValidPrincipalType(principalType) {
		return nil, ErrPageInvalidPrincipal
	}
	if !isValidPermissionLevel(level) {
		return nil, ErrPageInvalidLevel
	}

	return database.WithTxResult(s.db, func(tx database.Tx) (*models.PagePermission, error) {
		page, err := s.pageByIDTx(tx, pageID)
		if err != nil {
			return nil, err
		}

		if err := s.validateGrantPrincipal(tx, principalType, principalID); err != nil {
			return nil, err
		}

		actor := actorID
		insert := models.PagePermission{
			PageID:          pageID,
			PrincipalType:   principalType,
			PrincipalID:     principalID,
			PermissionLevel: level,
			GrantedBy:       &actor,
		}
		id, err := s.pages.GrantPermissionTx(tx, insert)
		if err != nil {
			if errors.Is(err, repository.ErrDuplicateEntry) {
				return nil, ErrPagePermissionDuplicate
			}
			return nil, err
		}

		if _, err := s.writeRevisionTx(tx, page, actorID, models.PageRevisionChangeTypePermissions, "granted "+level+" to "+principalType); err != nil {
			return nil, err
		}

		// Synthesize the persisted row from the insert input rather than
		// re-querying — re-reading through the read pool while the write
		// tx still holds the row deadlocks under SQLite's single-writer
		// model. granted_at is filled in by the DB; we surface the request
		// time which is close enough for the audit echo back to the
		// caller. Clients that need the canonical timestamp can re-fetch.
		insert.ID = id
		insert.GrantedAt = time.Now().UTC()
		return &insert, nil
	})
}

// RevokePermission removes a single ACL row from a page. The repository
// enforces the (permissionID, pageID) pairing so callers can't revoke a
// row belonging to a different page even if they construct a request by
// hand.
func (s *PageService) RevokePermission(actorID, pageID, permissionID int) error {
	return database.WithTx(s.db, func(tx database.Tx) error {
		page, err := s.pageByIDTx(tx, pageID)
		if err != nil {
			return err
		}
		if err := s.pages.RevokePermissionTx(tx, pageID, permissionID); err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return ErrPageNotFound
			}
			return err
		}
		_, err = s.writeRevisionTx(tx, page, actorID, models.PageRevisionChangeTypePermissions, "revoked permission")
		return err
	})
}

// SetInheritPermissions flips the inherit_permissions flag on a page and
// records a 'permissions' revision. Toggling has no UI cascade in Phase 2
// — descendants always inherit through the walker until they themselves
// flip the flag.
func (s *PageService) SetInheritPermissions(actorID, pageID int, inherit bool) (*models.Page, error) {
	return database.WithTxResult(s.db, func(tx database.Tx) (*models.Page, error) {
		page, err := s.pageByIDTx(tx, pageID)
		if err != nil {
			return nil, err
		}
		if page.InheritPermissions == inherit {
			// No-op; return the current page without writing a revision so
			// the audit trail isn't polluted with churn from idempotent UI
			// calls.
			return page, nil
		}
		if err := s.pages.SetInheritPermissionsTx(tx, pageID, inherit, actorID); err != nil {
			return nil, err
		}
		updated, err := s.pages.GetByIDTx(tx, pageID)
		if err != nil {
			return nil, err
		}
		summary := "broke permission inheritance"
		if inherit {
			summary = "restored permission inheritance"
		}
		if _, err := s.writeRevisionTx(tx, updated, actorID, models.PageRevisionChangeTypePermissions, summary); err != nil {
			return nil, err
		}
		return updated, nil
	})
}

// validateGrantPrincipal verifies active principals in the grant transaction; runtime evaluation enforces membership.
func (s *PageService) validateGrantPrincipal(tx database.Tx, principalType string, principalID int) error {
	var query string
	switch principalType {
	case models.PagePrincipalTypeUser:
		query = "SELECT EXISTS(SELECT 1 FROM users WHERE id = ? AND is_active = TRUE)"
	case models.PagePrincipalTypeGroup:
		query = "SELECT EXISTS(SELECT 1 FROM groups WHERE id = ? AND is_active = TRUE)"
	case models.PagePrincipalTypeRole:
		query = "SELECT EXISTS(SELECT 1 FROM workspace_roles WHERE id = ?)"
	default:
		return ErrPageInvalidPrincipal
	}
	var exists bool
	if err := tx.QueryRow(query, principalID).Scan(&exists); err != nil {
		return fmt.Errorf("validate grant principal %s/%d: %w", principalType, principalID, err)
	}
	if !exists {
		return ErrPageGrantPrincipalNotFound
	}
	return nil
}

func isValidPrincipalType(t string) bool {
	return t == models.PagePrincipalTypeUser ||
		t == models.PagePrincipalTypeGroup ||
		t == models.PagePrincipalTypeRole
}

func isValidPermissionLevel(l string) bool {
	return l == models.PagePermissionLevelView ||
		l == models.PagePermissionLevelEdit ||
		l == models.PagePermissionLevelAdmin
}

// --- helpers ---

// samePageParent treats nil as "workspace root" and compares two parent
// pointers structurally. Used by Move to decide whether the page is
// crossing into a different sibling set (which requires a fresh
// frac_index) or just reordering within its current one.
func samePageParent(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func (s *PageService) resolveParent(tx database.Tx, workspaceID int, parentID *int) (resolvedParentID *int, childPath string, parentDepth int, err error) {
	if parentID == nil {
		return nil, "/", -1, nil
	}
	parent, err := s.pages.GetByIDTx(tx, *parentID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, "", 0, ErrPageNotFound
		}
		return nil, "", 0, err
	}
	if parent.WorkspaceID != workspaceID {
		return nil, "", 0, ErrPageParentMismatch
	}
	parentPath := parent.Path + fmt.Sprintf("%d/", parent.ID)
	return &parent.ID, parentPath, parent.Depth, nil
}

// resolveSiblingFracIndex computes a moved page's sibling ordering key. Unanchored
// same-parent moves preserve order; parent changes append. Legacy null keys are
// backfilled before reordering.
func (s *PageService) resolveSiblingFracIndex(
	tx database.Tx,
	workspaceID int,
	newParentID *int,
	movedPageID int,
	prevSiblingID, nextSiblingID *int,
	parentChanged bool,
	actorID int,
) (*string, error) {
	if prevSiblingID == nil && nextSiblingID == nil && !parentChanged {
		return nil, nil
	}

	siblings, err := s.pages.ListChildrenTx(tx, workspaceID, newParentID)
	if err != nil {
		return nil, fmt.Errorf("list new parent children: %w", err)
	}

	// A page cannot anchor its own move.
	if prevSiblingID != nil && *prevSiblingID == movedPageID {
		prevSiblingID = nil
	}
	if nextSiblingID != nil && *nextSiblingID == movedPageID {
		nextSiblingID = nil
	}

	siblingByID := make(map[int]*models.Page, len(siblings))
	for i := range siblings {
		if siblings[i].ID == movedPageID {
			continue
		}
		siblingByID[siblings[i].ID] = &siblings[i]
	}
	for _, id := range []*int{prevSiblingID, nextSiblingID} {
		if id == nil {
			continue
		}
		if _, ok := siblingByID[*id]; !ok {
			return nil, fmt.Errorf("sibling %d is not a child of the target parent", *id)
		}
	}

	// Unanchored parent changes append after the current last sibling.
	if prevSiblingID == nil && nextSiblingID == nil {
		// A lone child gets a deterministic starting key.
		if len(siblingByID) == 0 {
			key, kerr := repository.KeyBetween("", "")
			if kerr != nil {
				return nil, fmt.Errorf("compute initial frac_index for empty parent: %w", kerr)
			}
			return &key, nil
		}
		// Anchor after the last sibling in display order. ListChildrenTx
		// returns siblings sorted by COALESCE(frac_index,''), rank, title,
		// id — so the final non-moved entry is the visual end of the list.
		var lastSibling *models.Page
		for i := len(siblings) - 1; i >= 0; i-- {
			if siblings[i].ID == movedPageID {
				continue
			}
			lastSibling = &siblings[i]
			break
		}
		// lastSibling can't be nil here because len(siblingByID) > 0.
		prevSiblingID = &lastSibling.ID
	}

	needsBackfill := false
	for _, id := range []*int{prevSiblingID, nextSiblingID} {
		if id == nil {
			continue
		}
		sib := siblingByID[*id]
		if sib.FracIndex == nil || *sib.FracIndex == "" {
			needsBackfill = true
			break
		}
	}

	if needsBackfill {
		// Re-sequence ALL siblings (overwriting any existing frac_index
		// values too) in their current display order. Mixed NULL +
		// non-NULL groups can interleave in ways that would collide with
		// freshly minted keys, so a full rewrite is the only safe option.
		//
		// Clear the whole active sibling set first. The scoped unique
		// index is checked after every UPDATE, so assigning a0 to a legacy
		// NULL row would otherwise collide with a later sibling that still
		// owns its old a0 key. Include the moving page when this is an
		// in-parent reorder; MoveTx assigns its final key below.
		siblingIDs := make([]int, 0, len(siblings))
		for i := range siblings {
			siblingIDs = append(siblingIDs, siblings[i].ID)
		}
		if err := s.pages.ClearFracIndexesTx(tx, siblingIDs); err != nil {
			return nil, fmt.Errorf("clear sibling frac_index values before backfill: %w", err)
		}

		var lastKey string
		for i := range siblings {
			if siblings[i].ID == movedPageID {
				continue
			}
			fresh, kerr := repository.KeyBetween(lastKey, "")
			if kerr != nil {
				return nil, fmt.Errorf("backfill frac_index for sibling %d: %w", siblings[i].ID, kerr)
			}
			if err := s.pages.SetFracIndexTx(tx, siblings[i].ID, fresh, actorID); err != nil {
				return nil, fmt.Errorf("persist backfilled frac_index for sibling %d: %w", siblings[i].ID, err)
			}
			siblings[i].FracIndex = &fresh
			siblingByID[siblings[i].ID] = &siblings[i]
			lastKey = fresh
		}
	}

	prevKey := ""
	if prevSiblingID != nil {
		if p := siblingByID[*prevSiblingID].FracIndex; p != nil {
			prevKey = *p
		}
	}
	nextKey := ""
	if nextSiblingID != nil {
		if n := siblingByID[*nextSiblingID].FracIndex; n != nil {
			nextKey = *n
		}
	}

	newKey, err := repository.KeyBetween(prevKey, nextKey)
	if err != nil {
		return nil, fmt.Errorf("compute frac_index between %q and %q: %w", prevKey, nextKey, err)
	}
	return &newKey, nil
}

var slugSpaceRe = regexp.MustCompile(`-+`)

// makeSlug derives a page's display slug from its title. Slugs are not
// unique and nothing resolves a page by one, so this is a pure function of
// the title with no database round-trip. A title with no alphanumerics
// (an emoji, say) reduces to "page" rather than the empty string, which
// the NOT NULL column would accept but no caller wants to render.
func makeSlug(title string) string {
	var b strings.Builder
	b.Grow(len(title))
	for _, r := range title {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
		case unicode.IsSpace(r), r == '-', r == '_', r == '/', r == '\\':
			b.WriteByte('-')
		}
	}
	out := slugSpaceRe.ReplaceAllString(b.String(), "-")
	out = strings.Trim(out, "-")
	if len(out) > 80 {
		out = strings.TrimRight(out[:80], "-")
	}
	if out == "" {
		return "page"
	}
	return out
}

func normalizePageMetadata(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "{}", nil
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
		return "", ErrPageMetadataInvalid
	}
	clean, err := json.Marshal(obj)
	if err != nil {
		return "", ErrPageMetadataInvalid
	}
	return string(clean), nil
}

// deriveExcerpt produces a short plain-text excerpt by stripping common
// Markdown syntax. Not a full Markdown parser — good enough for snippets.
func deriveExcerpt(content string) string {
	if content == "" {
		return ""
	}
	text := content
	text = strings.ReplaceAll(text, "\r", "")
	text = excerptCodeFence.ReplaceAllString(text, " ")
	text = excerptHeadingMark.ReplaceAllString(text, "")
	text = excerptListMark.ReplaceAllString(text, "")
	text = excerptInlineMark.ReplaceAllString(text, "")
	text = excerptLinkMark.ReplaceAllString(text, "$1")
	text = excerptHTML.ReplaceAllString(text, "")
	text = strings.Join(strings.Fields(text), " ")
	const maxLen = 240
	if len(text) > maxLen {
		text = strings.TrimRight(text[:maxLen], " ") + "…"
	}
	return text
}

var (
	excerptCodeFence   = regexp.MustCompile("(?s)```.*?```")
	excerptHeadingMark = regexp.MustCompile(`(?m)^#{1,6}\s+`)
	excerptListMark    = regexp.MustCompile(`(?m)^(\s*)([-*+]|\d+\.)\s+`)
	excerptInlineMark  = regexp.MustCompile("[`*_~>]")
	excerptLinkMark    = regexp.MustCompile(`!?\[([^\]]*)\]\([^)]*\)`)
	excerptHTML        = regexp.MustCompile(`(?s)<[^>]+>`)
)

func contentHash(content string) string {
	if content == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}
