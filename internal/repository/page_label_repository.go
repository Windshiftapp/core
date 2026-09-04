package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// PageLabelRepository persists workspace page labels and the
// page↔page_label join (page_label_assignments). Mirrors LabelRepository
// in shape but operates against pages, not items.
type PageLabelRepository struct {
	db database.Database
}

// NewPageLabelRepository creates a PageLabelRepository.
func NewPageLabelRepository(db database.Database) *PageLabelRepository {
	return &PageLabelRepository{db: db}
}

const pageLabelColumns = "id, name, color, workspace_id, created_at, updated_at"

// ListByWorkspace returns all page labels in the given workspace, ordered by name.
func (r *PageLabelRepository) ListByWorkspace(workspaceID int) ([]models.PageLabel, error) {
	rows, err := r.db.Query(
		"SELECT "+pageLabelColumns+" FROM page_labels WHERE workspace_id = ? ORDER BY name",
		workspaceID,
	)
	if err != nil {
		return nil, fmt.Errorf("list page labels for workspace %d: %w", workspaceID, err)
	}
	defer func() { _ = rows.Close() }()

	return scanPageLabels(rows)
}

// GetByID loads a single page label by its primary key. Returns ErrNotFound when missing.
func (r *PageLabelRepository) GetByID(id int) (*models.PageLabel, error) {
	var label models.PageLabel
	err := r.db.QueryRow(
		"SELECT "+pageLabelColumns+" FROM page_labels WHERE id = ?",
		id,
	).Scan(&label.ID, &label.Name, &label.Color, &label.WorkspaceID, &label.CreatedAt, &label.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get page label %d: %w", id, err)
	}
	return &label, nil
}

// GetWorkspaceID returns the workspace_id for a page label or ErrNotFound when missing.
func (r *PageLabelRepository) GetWorkspaceID(id int) (int, error) {
	var workspaceID int
	err := r.db.QueryRow("SELECT workspace_id FROM page_labels WHERE id = ?", id).Scan(&workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("get page label %d workspace: %w", id, err)
	}
	return workspaceID, nil
}

// NameExistsInWorkspace reports whether a label with the given name already
// exists in the workspace. excludeID > 0 excludes that row from the check
// (so an Update doesn't collide with itself).
func (r *PageLabelRepository) NameExistsInWorkspace(workspaceID int, name string, excludeID int) (bool, error) {
	var count int
	var err error
	if excludeID > 0 {
		err = r.db.QueryRow(
			"SELECT COUNT(*) FROM page_labels WHERE name = ? AND workspace_id = ? AND id != ?",
			name, workspaceID, excludeID,
		).Scan(&count)
	} else {
		err = r.db.QueryRow(
			"SELECT COUNT(*) FROM page_labels WHERE name = ? AND workspace_id = ?",
			name, workspaceID,
		).Scan(&count)
	}
	if err != nil {
		return false, fmt.Errorf("check page label name %q in workspace %d: %w", name, workspaceID, err)
	}
	return count > 0, nil
}

// Create inserts a page label and returns the id + the stamped timestamp.
// Returns ErrDuplicateEntry when the workspace already owns a label with
// the same name. Handlers pre-check with NameExistsInWorkspace, but two
// concurrent creates can race past that check — without translating the
// resulting unique-violation here, the loser would surface as a 500
// instead of the 409 the pre-check path already returns.
func (r *PageLabelRepository) Create(name, color string, workspaceID int) (int, time.Time, error) {
	now := time.Now()
	var id int
	err := r.db.QueryRow(`
		INSERT INTO page_labels (name, color, workspace_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?) RETURNING id
	`, name, color, workspaceID, now, now).Scan(&id)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return 0, time.Time{}, ErrDuplicateEntry
		}
		return 0, time.Time{}, fmt.Errorf("create page label: %w", err)
	}
	return id, now, nil
}

// Update overwrites a page label's name and color. Returns ErrDuplicateEntry
// when the new name collides with a sibling label in the same workspace —
// the handler's pre-check is racy across concurrent renames, so the same
// translation Create uses applies here.
func (r *PageLabelRepository) Update(id int, name, color string) error {
	_, err := r.db.ExecWrite(
		"UPDATE page_labels SET name = ?, color = ?, updated_at = ? WHERE id = ?",
		name, color, time.Now(), id,
	)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return ErrDuplicateEntry
		}
		return fmt.Errorf("update page label %d: %w", id, err)
	}
	return nil
}

// Delete removes a page label row (cascading page_label_assignments via FK).
func (r *PageLabelRepository) Delete(id int) error {
	if _, err := r.db.ExecWrite("DELETE FROM page_labels WHERE id = ?", id); err != nil {
		return fmt.Errorf("delete page label %d: %w", id, err)
	}
	return nil
}

// ListForPage returns the labels currently attached to a page, ordered by name.
func (r *PageLabelRepository) ListForPage(pageID int) ([]models.PageLabel, error) {
	rows, err := r.db.Query(`
		SELECT l.id, l.name, l.color, l.workspace_id, l.created_at, l.updated_at
		FROM page_label_assignments a
		JOIN page_labels l ON a.page_label_id = l.id
		WHERE a.page_id = ?
		ORDER BY l.name
	`, pageID)
	if err != nil {
		return nil, fmt.Errorf("list labels for page %d: %w", pageID, err)
	}
	defer func() { _ = rows.Close() }()

	return scanPageLabels(rows)
}

// ReplaceAssignments swaps the label set for a page atomically. Duplicate
// IDs in the input are deduplicated rather than rejected: callers (the
// HTTP handlers in particular) accept arbitrary client payloads, and a
// {label_ids: [1,1]} request that surfaced as a 500 because of the
// junction's UNIQUE(page_id, page_label_id) constraint is brittle. Treat
// the assignment set as a set.
func (r *PageLabelRepository) ReplaceAssignments(pageID int, labelIDs []int) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin replace page label assignments: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec("DELETE FROM page_label_assignments WHERE page_id = ?", pageID); err != nil {
		return fmt.Errorf("delete existing assignments for page %d: %w", pageID, err)
	}

	now := time.Now()
	seen := make(map[int]struct{}, len(labelIDs))
	for _, labelID := range labelIDs {
		if _, dup := seen[labelID]; dup {
			continue
		}
		seen[labelID] = struct{}{}
		if _, err := tx.Exec(
			"INSERT INTO page_label_assignments (page_id, page_label_id, created_at) VALUES (?, ?, ?)",
			pageID, labelID, now,
		); err != nil {
			return fmt.Errorf("assign label %d to page %d: %w", labelID, pageID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit replace page label assignments: %w", err)
	}
	return nil
}

// AddAssignment attaches a single label to a page. Returns ErrDuplicateEntry
// when the pair already exists (unique constraint on the junction).
func (r *PageLabelRepository) AddAssignment(pageID, labelID int) error {
	_, err := r.db.ExecWrite(
		"INSERT INTO page_label_assignments (page_id, page_label_id, created_at) VALUES (?, ?, ?)",
		pageID, labelID, time.Now(),
	)
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			return ErrDuplicateEntry
		}
		return fmt.Errorf("assign label %d to page %d: %w", labelID, pageID, err)
	}
	return nil
}

// RemoveAssignment detaches a label from a page. No-ops silently when the
// pair isn't there.
func (r *PageLabelRepository) RemoveAssignment(pageID, labelID int) error {
	if _, err := r.db.ExecWrite(
		"DELETE FROM page_label_assignments WHERE page_id = ? AND page_label_id = ?",
		pageID, labelID,
	); err != nil {
		return fmt.Errorf("remove label %d from page %d: %w", labelID, pageID, err)
	}
	return nil
}

// LoadForPages bulk-loads label rows for a slice of pages and attaches them
// to each page's Labels field. Used by the page list and detail endpoints to
// avoid an N+1 lookup. Always leaves Labels non-nil so the JSON response is
// `"labels": []` rather than `"labels": null` for pages without labels.
//
// Call this before BuildPageTree so each PageNode copy inherits the Labels
// slice header.
func (r *PageLabelRepository) LoadForPages(pages []models.Page) error {
	for i := range pages {
		if pages[i].Labels == nil {
			pages[i].Labels = []models.PageLabel{}
		}
	}
	if len(pages) == 0 {
		return nil
	}

	pageIDs := make([]any, len(pages))
	placeholders := make([]string, len(pages))
	for i, page := range pages {
		pageIDs[i] = page.ID
		placeholders[i] = "?"
	}

	query := fmt.Sprintf(`
		SELECT a.page_id, l.id, l.name, l.color, l.workspace_id, l.created_at, l.updated_at
		FROM page_label_assignments a
		JOIN page_labels l ON a.page_label_id = l.id
		WHERE a.page_id IN (%s)
		ORDER BY l.name
	`, strings.Join(placeholders, ","))

	rows, err := r.db.Query(query, pageIDs...)
	if err != nil {
		return fmt.Errorf("load labels for pages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	labelMap := make(map[int][]models.PageLabel)
	for rows.Next() {
		var pageID int
		var label models.PageLabel
		if err := rows.Scan(&pageID, &label.ID, &label.Name, &label.Color, &label.WorkspaceID,
			&label.CreatedAt, &label.UpdatedAt); err != nil {
			return fmt.Errorf("scan page label: %w", err)
		}
		labelMap[pageID] = append(labelMap[pageID], label)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate page labels: %w", err)
	}

	for i := range pages {
		if labels, ok := labelMap[pages[i].ID]; ok {
			pages[i].Labels = labels
		}
	}
	return nil
}

func scanPageLabels(rows *sql.Rows) ([]models.PageLabel, error) {
	labels := []models.PageLabel{}
	for rows.Next() {
		var label models.PageLabel
		if err := rows.Scan(&label.ID, &label.Name, &label.Color, &label.WorkspaceID,
			&label.CreatedAt, &label.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan page label: %w", err)
		}
		labels = append(labels, label)
	}
	return labels, nil
}
