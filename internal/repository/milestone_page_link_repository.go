package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	"windshift/internal/database"
	"windshift/internal/models"
)

// ErrPageLinkTypeUnavailable is returned when the built-in Page link type
// (builtin_key "page") is missing or inactive, so no page link can be stored.
var ErrPageLinkTypeUnavailable = errors.New("the built-in Page link type is missing or inactive")

// MilestonePageLinkRepository persists links from milestones to pages.
// They reuse the polymorphic item_links table: source_type
// "milestone", target_type "page", the built-in Page link type, and the
// table's created_by/created_at for who linked the page and when. The generic
// link endpoints ignore these rows; only the milestone page-link routes
// manage them. Linking and unlinking also write a milestone_history page_link
// row, so the Activity tab keeps both after the link is gone.
type MilestonePageLinkRepository struct {
	db database.Database
}

// NewMilestonePageLinkRepository creates a MilestonePageLinkRepository.
func NewMilestonePageLinkRepository(db database.Database) *MilestonePageLinkRepository {
	return &MilestonePageLinkRepository{db: db}
}

// The inner page join drops links whose page row no longer exists.
const milestonePageLinkSelect = `
	SELECT il.id, il.source_id, il.target_id, p.title, p.workspace_id, il.created_by, il.created_at,
	       COALESCE(NULLIF(TRIM(COALESCE(u.first_name, '') || ' ' || COALESCE(u.last_name, '')), ''), u.username, '') AS created_by_name
	FROM item_links il
	JOIN pages p ON p.id = il.target_id
	LEFT JOIN users u ON u.id = il.created_by
	WHERE il.source_type = 'milestone' AND il.target_type = 'page'`

// ListByMilestone returns a milestone's page links, oldest first.
func (r *MilestonePageLinkRepository) ListByMilestone(milestoneID int) ([]models.MilestonePageLink, error) {
	rows, err := r.db.Query(milestonePageLinkSelect+`
	  AND il.source_id = ?
	ORDER BY il.created_at, il.id`, milestoneID)
	if err != nil {
		return nil, fmt.Errorf("list page links for milestone %d: %w", milestoneID, err)
	}
	defer func() { _ = rows.Close() }()
	result := []models.MilestonePageLink{}
	for rows.Next() {
		link, err := scanMilestonePageLink(rows)
		if err != nil {
			return nil, fmt.Errorf("scan milestone page link: %w", err)
		}
		result = append(result, link)
	}
	return result, rows.Err()
}

// GetByID loads one milestone page link. Returns ErrNotFound when missing.
func (r *MilestonePageLinkRepository) GetByID(id int) (*models.MilestonePageLink, error) {
	link, err := scanMilestonePageLink(r.db.QueryRow(milestonePageLinkSelect+"\n\t  AND il.id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get milestone page link %d: %w", id, err)
	}
	return &link, nil
}

// Create links a page to a milestone and returns the link ID. It records the
// link in the milestone history in the same transaction, with
// createdBy as the actor. It returns ErrDuplicateEntry when the page is
// already linked and ErrPageLinkTypeUnavailable when the built-in Page link
// type is unusable.
func (r *MilestonePageLinkRepository) Create(milestoneID, pageID, createdBy int) (int, error) {
	var linkTypeID int
	err := r.db.QueryRow("SELECT id FROM link_types WHERE builtin_key = 'page' AND active = TRUE").Scan(&linkTypeID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrPageLinkTypeUnavailable
	}
	if err != nil {
		return 0, fmt.Errorf("load page link type: %w", err)
	}
	return database.WithTxResult(r.db, func(tx database.Tx) (int, error) {
		var id int
		err := tx.QueryRow(`
			INSERT INTO item_links (link_type_id, source_type, source_id, target_type, target_id, created_by, created_at)
			VALUES (?, 'milestone', ?, 'page', ?, ?, CURRENT_TIMESTAMP)
			ON CONFLICT DO NOTHING
			RETURNING id
		`, linkTypeID, milestoneID, pageID, createdBy).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrDuplicateEntry
		}
		if err != nil {
			return 0, fmt.Errorf("create milestone page link: %w", err)
		}
		if err := RecordMilestoneHistory(tx, MilestoneHistoryEntry{
			MilestoneID: milestoneID,
			UserID:      &createdBy,
			FieldName:   MilestoneHistoryPageLink,
			NewValue:    strconv.Itoa(pageID),
		}); err != nil {
			return 0, err
		}
		return id, nil
	})
}

// Delete removes one milestone page link and records the unlink in the
// milestone history in the same transaction, with deletedBy as the
// actor.
func (r *MilestonePageLinkRepository) Delete(id, deletedBy int) error {
	return database.WithTx(r.db, func(tx database.Tx) error {
		var milestoneID, pageID int
		err := tx.QueryRow(`
			DELETE FROM item_links WHERE id = ? AND source_type = 'milestone' AND target_type = 'page'
			RETURNING source_id, target_id
		`, id).Scan(&milestoneID, &pageID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("delete milestone page link %d: %w", id, err)
		}
		return RecordMilestoneHistory(tx, MilestoneHistoryEntry{
			MilestoneID: milestoneID,
			UserID:      &deletedBy,
			FieldName:   MilestoneHistoryPageLink,
			OldValue:    strconv.Itoa(pageID),
		})
	})
}

// ListHistory returns a milestone's recorded page links and unlinks,
// oldest first, with each page's current title and workspace.
// Events for pages that no longer exist are dropped. Links made before the
// history was recorded have no event.
func (r *MilestonePageLinkRepository) ListHistory(milestoneID int) ([]models.MilestonePageLinkEvent, error) {
	rows, err := r.db.Query(`
		SELECT mh.id, mh.old_value, mh.new_value, mh.user_id, mh.changed_at,
		       COALESCE(NULLIF(TRIM(COALESCE(u.first_name, '') || ' ' || COALESCE(u.last_name, '')), ''), u.username, '') AS user_name
		FROM milestone_history mh
		LEFT JOIN users u ON u.id = mh.user_id
		WHERE mh.milestone_id = ? AND mh.field_name = ?
		ORDER BY mh.changed_at, mh.id`, milestoneID, MilestoneHistoryPageLink)
	if err != nil {
		return nil, fmt.Errorf("list page link history for milestone %d: %w", milestoneID, err)
	}
	defer func() { _ = rows.Close() }()
	events := []models.MilestonePageLinkEvent{}
	pageIDs := []int{}
	seen := map[int]bool{}
	for rows.Next() {
		var (
			event              models.MilestonePageLinkEvent
			oldValue, newValue sql.NullString
			userID             sql.NullInt64
			userName           sql.NullString
		)
		if err := rows.Scan(&event.ID, &oldValue, &newValue, &userID, &event.OccurredAt, &userName); err != nil {
			return nil, fmt.Errorf("scan milestone page link history: %w", err)
		}
		raw := newValue.String
		if !newValue.Valid || raw == "" {
			raw = oldValue.String
			event.Unlinked = true
		}
		pageID, err := strconv.Atoi(raw)
		if err != nil {
			continue
		}
		event.MilestoneID = milestoneID
		event.PageID = pageID
		assignNullableInt(&event.UserID, userID)
		event.UserName = userName.String
		event.OccurredAt = event.OccurredAt.UTC()
		events = append(events, event)
		if !seen[pageID] {
			seen[pageID] = true
			pageIDs = append(pageIDs, pageID)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate milestone page link history: %w", err)
	}
	clause, args := inPlaceholders(pageIDs)
	if clause == "" {
		return events, nil
	}
	type pageInfo struct {
		title       string
		workspaceID int
	}
	pages := map[int]pageInfo{}
	pageRows, err := r.db.Query("SELECT id, title, workspace_id FROM pages WHERE id IN ("+clause+")", args...)
	if err != nil {
		return nil, fmt.Errorf("load pages for milestone %d page link history: %w", milestoneID, err)
	}
	defer func() { _ = pageRows.Close() }()
	for pageRows.Next() {
		var id int
		var info pageInfo
		if err := pageRows.Scan(&id, &info.title, &info.workspaceID); err != nil {
			return nil, fmt.Errorf("scan page for milestone page link history: %w", err)
		}
		pages[id] = info
	}
	if err := pageRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pages for milestone page link history: %w", err)
	}
	result := make([]models.MilestonePageLinkEvent, 0, len(events))
	for _, event := range events {
		info, ok := pages[event.PageID]
		if !ok {
			continue
		}
		event.PageTitle = info.title
		event.WorkspaceID = info.workspaceID
		result = append(result, event)
	}
	return result, nil
}

// DeleteByMilestone removes every page link of a milestone inside tx. Call it
// when deleting the milestone so no links point at a missing milestone.
func (r *MilestonePageLinkRepository) DeleteByMilestone(tx database.Tx, milestoneID int) error {
	if _, err := tx.Exec("DELETE FROM item_links WHERE source_type = 'milestone' AND source_id = ?", milestoneID); err != nil {
		return fmt.Errorf("delete page links for milestone %d: %w", milestoneID, err)
	}
	return nil
}

type milestonePageLinkScanner interface {
	Scan(dest ...any) error
}

func scanMilestonePageLink(row milestonePageLinkScanner) (models.MilestonePageLink, error) {
	var link models.MilestonePageLink
	err := row.Scan(&link.ID, &link.MilestoneID, &link.PageID, &link.PageTitle, &link.WorkspaceID,
		&link.CreatedBy, &link.CreatedAt, &link.CreatedByName)
	return link, err
}
