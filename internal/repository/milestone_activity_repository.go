package repository

import (
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
)

// MilestoneActivityRepository reads the stored sources of a milestone's
// Activity feed:
//
//   - milestone comments (milestone_comments)
//   - description, status, and target-date changes (milestone_history)
//   - comments on current member items (comments joined through item_milestones)
//   - status changes of current member items (item_history status_id rows,
//     creation rows excluded)
//   - items added to or removed from the milestone (item_history milestones
//     rows whose old and new milestone lists differ on this milestone),
//     including items that have since left it
//   - items created directly in the milestone before item creation recorded a
//     milestones history row: current members with no milestones
//     history at all, and items whose first milestones history row already
//     listed the milestone. Their actor and time are the item's creator and
//     creation time. Newer items start with a milestones row whose old value
//     is empty, so they match neither rule and get only the membership entry.
//
// Page links and unlinks (milestone_history page_link rows) are not read
// here: the service adds them after the page permission filter.
//
// Item rows are limited to items in the given workspaces, the same
// workspace-access rule the milestone progress view applies.
type MilestoneActivityRepository struct {
	db database.Database
}

// NewMilestoneActivityRepository creates a MilestoneActivityRepository.
func NewMilestoneActivityRepository(db database.Database) *MilestoneActivityRepository {
	return &MilestoneActivityRepository{db: db}
}

type milestoneActivitySourceKind int

// Source order doubles as the tie-break rank for entries with the same time.
const (
	activitySourceMilestoneHistory milestoneActivitySourceKind = iota
	activitySourceMilestoneComment
	activitySourceItemCreatedIn
	activitySourceItemMembership
	activitySourceItemStatus
	activitySourceItemComment
)

type milestoneActivitySource struct {
	kind  milestoneActivitySourceKind
	query string // SELECT … FROM … WHERE …, projected as milestoneActivityColumns
	order string
	args  []any
}

// Every source projects the same columns so one scanner reads them all:
// row_id, occurred_at, actor_kind, actor_user_id, actor_user_name,
// actor_customer_name, item_id, item_title, item_workspace_id,
// workspace_key, item_number, field_name, old_value, new_value.
const (
	activityUserName     = `COALESCE(NULLIF(TRIM(COALESCE(u.first_name, '') || ' ' || COALESCE(u.last_name, '')), ''), u.username, '')`
	activityCustomerName = `COALESCE(NULLIF(TRIM(pc.name), ''), pc.email, '')`
	activityItemColumns  = `i.id, i.title, i.workspace_id, w.key, i.workspace_item_number`
	activityNoItem       = `NULL, NULL, NULL, NULL, NULL`
)

type milestoneActivityRow struct {
	kind       milestoneActivitySourceKind
	rowID      int
	activity   models.MilestoneActivity
	fieldName  string
	statusFrom string
	statusTo   string
}

// List returns the newest limit entries across the stored sources, newest
// first. Each source reads at most limit rows, so callers paging with an
// offset pass offset+limit and slice; asking for one more row than a page
// needs tells them whether another page follows without counting the
// history tables.
func (r *MilestoneActivityRepository) List(milestoneID int, workspaceIDs []int, limit int) ([]models.MilestoneActivity, error) {
	if limit <= 0 {
		return []models.MilestoneActivity{}, nil
	}
	sources := milestoneActivitySources(milestoneID, workspaceIDs)
	rows := []milestoneActivityRow{}
	for _, source := range sources {
		sourceRows, err := r.readSource(source, milestoneID, limit)
		if err != nil {
			return nil, err
		}
		rows = append(rows, sourceRows...)
	}
	sort.SliceStable(rows, func(a, b int) bool {
		left, right := rows[a], rows[b]
		if !left.activity.OccurredAt.Equal(right.activity.OccurredAt) {
			return left.activity.OccurredAt.After(right.activity.OccurredAt)
		}
		if left.kind != right.kind {
			return left.kind < right.kind
		}
		return left.rowID > right.rowID
	})
	if len(rows) > limit {
		rows = rows[:limit]
	}
	if err := r.resolveItemStatusNames(rows); err != nil {
		return nil, err
	}
	result := make([]models.MilestoneActivity, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.activity)
	}
	return result, nil
}

func milestoneActivitySources(milestoneID int, workspaceIDs []int) []milestoneActivitySource {
	sources := make([]milestoneActivitySource, 0, int(activitySourceItemComment)+1)
	sources = append(sources,
		milestoneActivitySource{
			kind: activitySourceMilestoneHistory,
			query: `SELECT mh.id, mh.changed_at,
				CASE WHEN mh.user_id IS NULL THEN 'system' ELSE 'user' END,
				mh.user_id, ` + activityUserName + `, '', ` + activityNoItem + `,
				mh.field_name, mh.old_value, mh.new_value
			FROM milestone_history mh
			LEFT JOIN users u ON u.id = mh.user_id
			WHERE mh.milestone_id = ? AND mh.field_name IN ('description', 'status', 'target_date')`,
			order: "mh.changed_at DESC, mh.id DESC",
			args:  []any{milestoneID},
		},
		milestoneActivitySource{
			kind: activitySourceMilestoneComment,
			query: `SELECT mc.id, mc.created_at, 'user', mc.author_id, ` + activityUserName + `, '', ` + activityNoItem + `,
				NULL, NULL, NULL
			FROM milestone_comments mc
			LEFT JOIN users u ON u.id = mc.author_id
			WHERE mc.milestone_id = ?`,
			order: "mc.created_at DESC, mc.id DESC",
			args:  []any{milestoneID},
		},
	)
	workspaceClause, workspaceArgs := inPlaceholders(workspaceIDs)
	if workspaceClause == "" {
		// No accessible workspace: no item is visible, so no item events.
		return sources
	}
	pattern := "%," + strconv.Itoa(milestoneID) + ",%"
	withWorkspaces := func(args ...any) []any {
		return append(args, workspaceArgs...)
	}
	sources = append(sources,
		milestoneActivitySource{
			kind: activitySourceItemCreatedIn,
			query: `SELECT i.id, i.created_at,
				CASE WHEN i.creator_id IS NOT NULL THEN 'user'
				     WHEN i.creator_portal_customer_id IS NOT NULL THEN 'portal_customer'
				     ELSE 'system' END,
				i.creator_id, ` + activityUserName + `, ` + activityCustomerName + `, ` + activityItemColumns + `,
				NULL, NULL, NULL
			FROM items i
			JOIN workspaces w ON w.id = i.workspace_id
			LEFT JOIN users u ON u.id = i.creator_id
			LEFT JOIN portal_customers pc ON pc.id = i.creator_portal_customer_id
			WHERE i.id IN (
				SELECT im.item_id FROM item_milestones im
				WHERE im.milestone_id = ?
				  AND NOT EXISTS (
					SELECT 1 FROM item_history mh
					WHERE mh.item_id = im.item_id AND mh.field_name = 'milestones'
				  )
				UNION
				SELECT fh.item_id FROM item_history fh
				WHERE fh.field_name = 'milestones'
				  AND (',' || COALESCE(fh.old_value, '') || ',') LIKE ?
				  AND NOT EXISTS (
					SELECT 1 FROM item_history eh
					WHERE eh.item_id = fh.item_id AND eh.field_name = 'milestones'
					  AND (eh.changed_at < fh.changed_at OR (eh.changed_at = fh.changed_at AND eh.id < fh.id))
				  )
			)
			AND i.workspace_id IN (` + workspaceClause + `)`,
			order: "i.created_at DESC, i.id DESC",
			args:  withWorkspaces(milestoneID, pattern),
		},
		milestoneActivitySource{
			kind: activitySourceItemMembership,
			query: `SELECT h.id, h.changed_at, h.actor_kind, h.user_id, ` + activityUserName + `, ` + activityCustomerName + `, ` + activityItemColumns + `,
				h.field_name, h.old_value, h.new_value
			FROM item_history h
			JOIN items i ON i.id = h.item_id
			JOIN workspaces w ON w.id = i.workspace_id
			LEFT JOIN users u ON u.id = h.user_id
			LEFT JOIN portal_customers pc ON pc.id = h.actor_portal_customer_id
			WHERE h.field_name = 'milestones'
			  AND ((',' || COALESCE(h.old_value, '') || ',') LIKE ?) <> ((',' || COALESCE(h.new_value, '') || ',') LIKE ?)
			  AND i.workspace_id IN (` + workspaceClause + `)`,
			order: "h.changed_at DESC, h.id DESC",
			args:  withWorkspaces(pattern, pattern),
		},
		milestoneActivitySource{
			kind: activitySourceItemStatus,
			query: `SELECT h.id, h.changed_at, h.actor_kind, h.user_id, ` + activityUserName + `, ` + activityCustomerName + `, ` + activityItemColumns + `,
				h.field_name, h.old_value, h.new_value
			FROM item_history h
			JOIN item_milestones im ON im.item_id = h.item_id AND im.milestone_id = ?
			JOIN items i ON i.id = h.item_id
			JOIN workspaces w ON w.id = i.workspace_id
			LEFT JOIN users u ON u.id = h.user_id
			LEFT JOIN portal_customers pc ON pc.id = h.actor_portal_customer_id
			WHERE h.field_name = 'status_id' AND COALESCE(h.old_value, '') <> ''
			  AND i.workspace_id IN (` + workspaceClause + `)`,
			order: "h.changed_at DESC, h.id DESC",
			args:  withWorkspaces(milestoneID),
		},
		milestoneActivitySource{
			kind: activitySourceItemComment,
			query: `SELECT c.id, c.created_at,
				CASE WHEN c.author_id IS NULL AND c.portal_customer_id IS NOT NULL THEN 'portal_customer' ELSE 'user' END,
				c.author_id, ` + activityUserName + `, ` + activityCustomerName + `, ` + activityItemColumns + `,
				NULL, NULL, NULL
			FROM comments c
			JOIN item_milestones im ON im.item_id = c.item_id AND im.milestone_id = ?
			JOIN items i ON i.id = c.item_id
			JOIN workspaces w ON w.id = i.workspace_id
			LEFT JOIN users u ON u.id = c.author_id
			LEFT JOIN portal_customers pc ON pc.id = c.portal_customer_id
			WHERE i.workspace_id IN (` + workspaceClause + `)`,
			order: "c.created_at DESC, c.id DESC",
			args:  withWorkspaces(milestoneID),
		},
	)
	return sources
}

func (r *MilestoneActivityRepository) readSource(source milestoneActivitySource, milestoneID, limit int) ([]milestoneActivityRow, error) {
	args := append(append([]any{}, source.args...), limit)
	rows, err := r.db.Query(source.query+"\nORDER BY "+source.order+"\nLIMIT ?", args...)
	if err != nil {
		return nil, fmt.Errorf("read milestone activity source %d: %w", source.kind, err)
	}
	defer func() { _ = rows.Close() }()
	result := []milestoneActivityRow{}
	for rows.Next() {
		row, ok, err := scanMilestoneActivityRow(rows, source.kind, milestoneID)
		if err != nil {
			return nil, fmt.Errorf("scan milestone activity source %d: %w", source.kind, err)
		}
		if ok {
			result = append(result, row)
		}
	}
	return result, rows.Err()
}

func scanMilestoneActivityRow(rows *sql.Rows, kind milestoneActivitySourceKind, milestoneID int) (milestoneActivityRow, bool, error) {
	var (
		row                                  milestoneActivityRow
		occurredAt                           time.Time
		actorKind                            sql.NullString
		actorUserID, itemID, itemWorkspaceID sql.NullInt64
		itemNumber                           sql.NullInt64
		actorUserName, actorCustomerName     sql.NullString
		itemTitle, workspaceKey              sql.NullString
		fieldName, oldValue, newValue        sql.NullString
	)
	if err := rows.Scan(&row.rowID, &occurredAt, &actorKind, &actorUserID, &actorUserName, &actorCustomerName,
		&itemID, &itemTitle, &itemWorkspaceID, &workspaceKey, &itemNumber,
		&fieldName, &oldValue, &newValue); err != nil {
		return row, false, err
	}
	row.kind = kind
	row.fieldName = fieldName.String
	activity := &row.activity
	activity.OccurredAt = occurredAt.UTC()
	activity.ActorKind = actorKind.String
	switch activity.ActorKind {
	case models.MilestoneActivityActorUser:
		if actorUserID.Valid {
			id := int(actorUserID.Int64)
			activity.ActorID = &id
			activity.ActorName = actorUserName.String
		} else {
			activity.ActorKind = models.MilestoneActivityActorSystem
		}
	case models.MilestoneActivityActorPortalCustomer:
		activity.ActorName = actorCustomerName.String
	default:
		activity.ActorKind = models.MilestoneActivityActorSystem
	}
	if itemID.Valid {
		activity.Item = &models.MilestoneActivityItem{
			ID:          int(itemID.Int64),
			Key:         fmt.Sprintf("%s-%d", workspaceKey.String, itemNumber.Int64),
			Title:       itemTitle.String,
			WorkspaceID: int(itemWorkspaceID.Int64),
		}
	}

	switch kind {
	case activitySourceMilestoneHistory:
		activity.ID = "milestone_history:" + strconv.Itoa(row.rowID)
		switch row.fieldName {
		case MilestoneHistoryDescription:
			activity.Type = models.MilestoneActivityMilestoneDescriptionEdit
		case MilestoneHistoryStatus:
			activity.Type = models.MilestoneActivityMilestoneStatusChanged
			activity.OldValue, activity.NewValue = nullStringPtr(oldValue), nullStringPtr(newValue)
		case MilestoneHistoryTargetDate:
			activity.Type = models.MilestoneActivityMilestoneTargetDateChange
			activity.OldValue, activity.NewValue = nullStringPtr(oldValue), nullStringPtr(newValue)
		default:
			return row, false, nil
		}
	case activitySourceMilestoneComment:
		activity.ID = "milestone_comment:" + strconv.Itoa(row.rowID)
		activity.Type = models.MilestoneActivityMilestoneCommentAdded
		commentID := row.rowID
		activity.CommentID = &commentID
	case activitySourceItemCreatedIn:
		activity.ID = "item_created:" + strconv.Itoa(row.rowID)
		activity.Type = models.MilestoneActivityItemAdded
	case activitySourceItemMembership:
		activity.ID = "item_history:" + strconv.Itoa(row.rowID)
		if milestoneListContains(newValue.String, milestoneID) {
			activity.Type = models.MilestoneActivityItemAdded
		} else {
			activity.Type = models.MilestoneActivityItemRemoved
		}
	case activitySourceItemStatus:
		activity.ID = "item_history:" + strconv.Itoa(row.rowID)
		activity.Type = models.MilestoneActivityItemStatusChanged
		row.statusFrom, row.statusTo = oldValue.String, newValue.String
	case activitySourceItemComment:
		activity.ID = "item_comment:" + strconv.Itoa(row.rowID)
		activity.Type = models.MilestoneActivityItemCommentAdded
		commentID := row.rowID
		activity.CommentID = &commentID
	}
	return row, true, nil
}

// milestoneListContains reports whether a milestones history value ("3,50")
// lists the milestone.
func milestoneListContains(list string, milestoneID int) bool {
	want := strconv.Itoa(milestoneID)
	for _, part := range strings.Split(list, ",") {
		if strings.TrimSpace(part) == want {
			return true
		}
	}
	return false
}

// resolveItemStatusNames replaces the status IDs that item_history stores
// with status names. An unknown status keeps its ID.
func (r *MilestoneActivityRepository) resolveItemStatusNames(rows []milestoneActivityRow) error {
	ids := []int{}
	seen := map[int]bool{}
	for _, row := range rows {
		if row.kind != activitySourceItemStatus {
			continue
		}
		for _, raw := range []string{row.statusFrom, row.statusTo} {
			if id, err := strconv.Atoi(raw); err == nil && !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	names := map[string]string{}
	if clause, args := inPlaceholders(ids); clause != "" {
		statusRows, err := r.db.Query("SELECT id, name FROM statuses WHERE id IN ("+clause+")", args...)
		if err != nil {
			return fmt.Errorf("resolve item status names: %w", err)
		}
		defer func() { _ = statusRows.Close() }()
		for statusRows.Next() {
			var id int
			var name string
			if err := statusRows.Scan(&id, &name); err != nil {
				return fmt.Errorf("scan item status name: %w", err)
			}
			names[strconv.Itoa(id)] = name
		}
		if err := statusRows.Err(); err != nil {
			return fmt.Errorf("iterate item status names: %w", err)
		}
	}
	for i := range rows {
		if rows[i].kind != activitySourceItemStatus {
			continue
		}
		rows[i].activity.OldValue = statusNameValue(names, rows[i].statusFrom)
		rows[i].activity.NewValue = statusNameValue(names, rows[i].statusTo)
	}
	return nil
}

func statusNameValue(names map[string]string, raw string) *string {
	if raw == "" {
		return nil
	}
	if name, ok := names[raw]; ok {
		return &name
	}
	return &raw
}

func nullStringPtr(value sql.NullString) *string {
	if !value.Valid || value.String == "" {
		return nil
	}
	v := value.String
	return &v
}
