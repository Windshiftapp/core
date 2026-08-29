package repository

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"windshift/internal/cql"
	"windshift/internal/database"
	"windshift/internal/models"
)

// dbTimeLayouts are the datetime string layouts SQLite may emit when a value
// loses its column type affinity (e.g. through COALESCE/MAX), depending on
// whether it was written by Go (RFC3339) or by SQLite's CURRENT_TIMESTAMP.
var dbTimeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02 15:04:05.999999999 -0700 MST",
	"2006-01-02 15:04:05 -0700 MST",
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02 15:04:05.999999-07:00",
	"2006-01-02 15:04:05-07:00",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02 15:04:05",
}

// parseDBTime tolerantly parses a datetime string returned from SQLite.
func parseDBTime(s string) (time.Time, bool) {
	for _, layout := range dbTimeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// ItemListParams contains parameters for listing items
type ItemListParams struct {
	WorkspaceIDs     []int
	Filters          ItemFilters
	Pagination       PaginationParams
	SortBy           string
	SortAsc          bool
	OmitDescriptions bool
}

// ItemFilters contains optional filters for item queries
type ItemFilters struct {
	WorkspaceID   *int
	StatusID      *int
	StatusIDNot   *int
	PriorityID    *int
	AssigneeID    *int
	CreatorID     *int
	ItemTypeID    *int
	MilestoneID   *int
	IterationID   *int
	ParentID      *int    // nil = any, 0 = root items only
	ParentIDIsSet bool    // true if ParentID filter should be applied
	Level         *int    // Hierarchy level filter
	MaxLevel      *int    // Maximum hierarchy level filter
	CreatedSince  *string // ISO date string
	// CompletedSince constrains ONLY items in a completed status to those that
	// entered that status on/after this ISO date; non-completed items always
	// pass. Caps the indefinitely-growing "done" list on personal views.
	CompletedSince *string
	// CompletedActivitySince constrains completed items by their most recent
	// activity while leaving unfinished items untouched.
	CompletedActivitySince *time.Time
	QLQuery                string // Custom QL query
	QLArgs                 []any
	StatusIDs              []int  // Multi-value status filter (for backlog + search)
	StatusIDsNot           []int  // Multi-value negated status filter
	PriorityIDs            []int  // Multi-value priority filter
	ItemIDs                []int  // Multi-value item ID filter
	TextQuery              string // LIKE search on title/description
	ItemKeyQuery           string // Workspace key pattern match (e.g. "OK-40")
	ItemID                 *int   // Filter by specific item ID
}

func (f ItemFilters) hasScalarFilters() bool {
	return f.StatusID != nil || f.StatusIDNot != nil || f.PriorityID != nil ||
		f.AssigneeID != nil || f.CreatorID != nil || f.ItemTypeID != nil ||
		f.MilestoneID != nil || f.IterationID != nil || f.ParentIDIsSet ||
		f.Level != nil || f.MaxLevel != nil || f.CreatedSince != nil ||
		f.CompletedSince != nil || f.CompletedActivitySince != nil || f.ItemID != nil
}

func (f ItemFilters) hasListFilters() bool {
	return len(f.StatusIDs) != 0 || len(f.StatusIDsNot) != 0 || len(f.PriorityIDs) != 0 || len(f.ItemIDs) != 0
}

func (f ItemFilters) hasTextFilters() bool {
	return f.TextQuery != "" || f.ItemKeyQuery != ""
}

func (f ItemFilters) isUnfiltered() bool {
	return f.QLQuery == "" && !f.hasScalarFilters() && !f.hasListFilters() && !f.hasTextFilters()
}

// PaginationParams contains pagination parameters
type PaginationParams struct {
	Page   int
	Limit  int
	Offset int
	// Cursor is an opaque continuation token for the default-ranked workspace
	// list. CursorMode opts the first request into the continuation contract;
	// collection, QL, and filtered lists intentionally continue to use their
	// existing page/offset contract.
	Cursor     string
	CursorMode bool
}

// ItemListPage is the repository result for a list request. The legacy list
// methods below still return only items and total; callers that opt into the
// workspace cursor contract can also consume NextCursor.
type ItemListPage struct {
	Items      []models.Item
	Total      int
	NextCursor string
}

// ItemIDPage is the lightweight counterpart to ItemListPage for callers that
// need the selected item set without hydrating item details.
type ItemIDPage struct {
	IDs   []int
	Total int
}

// ErrInvalidItemListCursor identifies a malformed opaque list continuation.
// Handlers map it to a validation response instead of exposing a SQL error.
var ErrInvalidItemListCursor = errors.New("invalid item list cursor")

type itemListCursor struct {
	Rank string `json:"r"`
	ID   int    `json:"i"`
}

// ItemListFilterFromClause contains every table alias referenced by
// buildWhereClause, including aliases emitted by the CQL generator. Keep this
// shared by item-list and analytics queries so a valid filter cannot succeed
// in one path and fail in another.
func ItemListFilterFromClause() string {
	return `FROM items i
		JOIN workspaces w ON i.workspace_id = w.id
		LEFT JOIN item_types it ON i.item_type_id = it.id
		LEFT JOIN iterations iter ON i.iteration_id = iter.id
		LEFT JOIN time_projects proj ON i.project_id = proj.id
		LEFT JOIN time_projects tp ON i.time_project_id = tp.id
		LEFT JOIN statuses st ON i.status_id = st.id
		LEFT JOIN status_categories sc ON st.category_id = sc.id
		LEFT JOIN priorities pri ON i.priority_id = pri.id
	`
}

// itemListPagePlan keeps the count and page-selection SQL together so the
// unfiltered workspace fast path cannot drift from filtered/collection lists.
type itemListPagePlan struct {
	countQuery       string
	countArgs        []any
	pageFromClause   string
	pageWhereClause  string
	pageArgs         []any
	workspaceCountID int
}

// systemFieldSortColumns maps field identifiers to safe SQL column references for sorting.
// This is the single source of truth for which system fields support server-side sorting.
var systemFieldSortColumns = map[string]string{
	"key":            "i.workspace_item_number",
	"title":          "i.title",
	"status":         "i.status_id",
	"priority":       "i.priority_id",
	"assignee":       "i.assignee_id",
	"milestone":      "(SELECT MIN(milestone_id) FROM item_milestones WHERE item_id = i.id)",
	"iteration":      "i.iteration_id",
	"due_date":       "i.due_date",
	"start_date":     "i.start_date",
	"end_date":       "i.end_date",
	"created_at":     "i.created_at",
	"updated_at":     "i.updated_at",
	"last_active_at": "i.last_active_at",
	"project":        "i.project_id",
	"rank":           "i.rank",
	"frac_index":     "i.frac_index",
}

// unsortableCustomFieldTypes lists custom field types that cannot be meaningfully sorted.
var unsortableCustomFieldTypes = map[string]bool{
	"multiselect": true,
	"multi_user":  true,
	"linking":     true,
}

// numericCustomFieldSortTypes store scalar numbers or entity/option IDs.
var numericCustomFieldSortTypes = map[string]bool{
	"number":               true,
	"select":               true,
	"milestone":            true,
	"iteration":            true,
	"user":                 true,
	"asset":                true,
	"portalcustomer":       true,
	"customerorganisation": true, //nolint:misspell // matches the field type
}

// SystemSortableFieldKeys returns the list of system field identifiers that support sorting.
func SystemSortableFieldKeys() []string {
	keys := make([]string, 0, len(systemFieldSortColumns))
	for k := range systemFieldSortColumns {
		keys = append(keys, k)
	}
	return keys
}

// FindAllWithDetails retrieves items with all joined data, supporting filters and pagination
func (r *ItemRepository) FindAllWithDetails(params ItemListParams) ([]models.Item, int, error) {
	page, err := r.FindAllWithDetailsPageContext(context.Background(), params)
	return page.Items, page.Total, err
}

// FindAllWithDetailsContext retrieves a page of items while allowing request
// cancellation to stop both the count and data queries and release their pool
// connections promptly.
func (r *ItemRepository) FindAllWithDetailsContext(ctx context.Context, params ItemListParams) ([]models.Item, int, error) {
	page, err := r.FindAllWithDetailsPageContext(ctx, params)
	return page.Items, page.Total, err
}

// FindAllWithDetailsPageContext is the cursor-aware form of the item list
// query. Cursor mode is deliberately limited to the direct, unfiltered
// workspace path; collection and filtered lists continue through the same
// joined page plan and offset semantics as before.
func (r *ItemRepository) FindAllWithDetailsPageContext(ctx context.Context, params ItemListParams) (ItemListPage, error) {
	cursorMode := itemListCursorEligible(params) &&
		(params.Pagination.CursorMode || params.Pagination.Cursor != "")
	var cursor itemListCursor
	if cursorMode && params.Pagination.Cursor != "" {
		var err error
		cursor, err = decodeItemListCursor(params.Pagination.Cursor)
		if err != nil {
			return ItemListPage{}, err
		}
	}

	// Build the SELECT clause. Collection/list surfaces can opt out of the
	// heavy description payload; detail endpoints still fetch full descriptions.
	descriptionExpr := "i.description"
	if params.OmitDescriptions {
		descriptionExpr = "''"
	}
	selectClause := fmt.Sprintf(`SELECT
		i.id, i.workspace_id, i.workspace_item_number, i.item_type_id, i.title, %s, i.status_id, i.priority_id, i.due_date, i.start_date, i.end_date, i.is_task,
		i.iteration_id, i.project_id, i.inherit_project, i.time_project_id, i.assignee_id, i.creator_id, i.custom_field_values, i.calendar_data, i.parent_id,
		i.story_points, i.estimate_minutes, i.frac_index, i.created_at, i.updated_at, i.last_active_at,
		w.name as workspace_name, w.key as workspace_key, it.name as item_type_name, COALESCE(it.builtin_key, '') as item_type_builtin_key,
		p.title as parent_title, p.workspace_item_number as parent_workspace_item_number, iter.name as iteration_name, COALESCE(CAST(iter.end_date AS TEXT), '') as iteration_end_date, proj.name as project_name, tp.name as time_project_name,
		assignee.first_name || ' ' || assignee.last_name as assignee_name, assignee.email as assignee_email, assignee.avatar_url as assignee_avatar,
		creator.first_name || ' ' || creator.last_name as creator_name, creator.email as creator_email,
		st.name as status_name, COALESCE(st.builtin_key, '') as status_builtin_key, sc.color as status_color,
		pri.name as priority_name, COALESCE(pri.builtin_key, '') as priority_builtin_key, pri.icon as priority_icon, pri.color as priority_color,
		COALESCE(%s, i.created_at) as status_since
	`, descriptionExpr, cql.CurrentStatusTransitionAtExpr(""))

	fromClause := ItemListFilterFromClause() + `
		LEFT JOIN items p ON i.parent_id = p.id
		LEFT JOIN users assignee ON i.assignee_id = assignee.id
		LEFT JOIN users creator ON i.creator_id = creator.id
	`

	whereClause, args := r.buildWhereClause(params)

	// Keep display-only parent and user joins out of the count/page plan, but
	// retain every alias that buildWhereClause or generated QL can reference.
	countFromClause := ItemListFilterFromClause()
	pagePlan := r.buildItemListPagePlan(params, countFromClause, whereClause, args)
	var total int
	var err error
	if pagePlan.workspaceCountID != 0 {
		total, err = cachedItemListCount(ctx, r.db, pagePlan.workspaceCountID, pagePlan.countQuery, pagePlan.countArgs...)
		if err != nil {
			return ItemListPage{}, fmt.Errorf("failed to count items: %w", err)
		}
	} else if err := r.db.QueryRowContext(ctx, pagePlan.countQuery, pagePlan.countArgs...).Scan(&total); err != nil {
		return ItemListPage{}, fmt.Errorf("failed to count items: %w", err)
	}

	var pageTx database.Tx
	var pageQueryer itemListPageQueryer = r.db
	if cursorMode && params.Pagination.Cursor != "" {
		// Resolve the cursor row and select the continuation page in one database
		// transaction. The shared global-rank lock allows concurrent list calls and
		// ordinary mutations, while preventing a migration batch from changing the
		// anchor between these two reads. This remains correct when normalization
		// assigns a new fractional payload rather than merely changing its bucket.
		pageTx, err = beginItemListCursorTransaction(ctx, r.db)
		if err != nil {
			return ItemListPage{}, fmt.Errorf("begin item list cursor transaction: %w", err)
		}
		defer func() { _ = pageTx.Rollback() }()
		if err := acquireGlobalRankMutationLock(pageTx, r.db.GetDriverName()); err != nil {
			return ItemListPage{}, err
		}
		cursor, err = resolveItemListCursor(ctx, pageTx, pagePlan.workspaceCountID, cursor)
		if err != nil {
			return ItemListPage{}, err
		}
		cursorWhere, cursorArgs, err := itemListCursorWhere(cursor)
		if err != nil {
			return ItemListPage{}, err
		}
		pagePlan.pageWhereClause += cursorWhere
		pagePlan.pageArgs = append(append([]any{}, pagePlan.pageArgs...), cursorArgs...)
		pageQueryer = pageTx
	}

	orderByClause := r.buildOrderByClause(params.SortBy, params.SortAsc)

	limit := params.Pagination.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}
	offset := params.Pagination.Offset
	if offset < 0 {
		offset = 0
	}
	if cursorMode {
		// A cursor is a continuation of the sorted stream, not a second offset.
		offset = 0
	}
	fetchLimit := limit
	if cursorMode {
		// Fetch one sentinel row so the response can omit NextCursor on the last
		// page without issuing a second count query after the keyset boundary.
		fetchLimit++
	}

	// Execute the page selection against the narrow filtering joins first. The
	// old query hydrated every display join before OFFSET discarded deep-page
	// rows. Keeping the page IDs in a CTE means only the requested rows pay for
	// descriptions, hierarchy/status enrichment, and the correlated status
	// transition lookup.
	pageQuery := "WITH page_items AS (SELECT i.id " + pagePlan.pageFromClause + pagePlan.pageWhereClause + orderByClause + fmt.Sprintf(" LIMIT %d OFFSET %d", fetchLimit, offset) + ") "
	fullQuery := pageQuery + selectClause + fromClause + " JOIN page_items page ON page.id = i.id" + orderByClause
	rows, err := pageQueryer.QueryContext(ctx, fullQuery, pagePlan.pageArgs...)
	if err != nil {
		return ItemListPage{}, fmt.Errorf("failed to query items: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items, err := r.scanItemList(rows)
	if err != nil {
		return ItemListPage{}, err
	}
	if err := rows.Close(); err != nil {
		return ItemListPage{}, fmt.Errorf("close item list rows: %w", err)
	}
	if pageTx != nil {
		if err := pageTx.Commit(); err != nil {
			return ItemListPage{}, fmt.Errorf("commit item list cursor transaction: %w", err)
		}
	}

	page := ItemListPage{Items: items, Total: total}
	if cursorMode && len(page.Items) > limit {
		page.Items = page.Items[:limit]
		last := page.Items[len(page.Items)-1]
		if last.FracIndex != nil && *last.FracIndex != "" {
			page.NextCursor = encodeItemListCursor(itemListCursor{Rank: *last.FracIndex, ID: last.ID})
		}
	}
	return page, nil
}

// FindIDPageContext returns one ordered item-ID page using the same filters,
// access scope, count, and sorting contract as the full item list.
func (r *ItemRepository) FindIDPageContext(ctx context.Context, params ItemListParams) (ItemIDPage, error) {
	whereClause, args := r.buildWhereClause(params)
	fromClause := ItemListFilterFromClause()
	plan := r.buildItemListPagePlan(params, fromClause, whereClause, args)

	var total int
	if plan.workspaceCountID != 0 {
		var err error
		total, err = cachedItemListCount(ctx, r.db, plan.workspaceCountID, plan.countQuery, plan.countArgs...)
		if err != nil {
			return ItemIDPage{}, fmt.Errorf("failed to count item ids: %w", err)
		}
	} else if err := r.db.QueryRowContext(ctx, plan.countQuery, plan.countArgs...).Scan(&total); err != nil {
		return ItemIDPage{}, fmt.Errorf("failed to count item ids: %w", err)
	}

	limit := params.Pagination.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}
	offset := max(params.Pagination.Offset, 0)
	query := "SELECT i.id " + plan.pageFromClause + plan.pageWhereClause +
		r.buildOrderByClause(params.SortBy, params.SortAsc) +
		fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)

	rows, err := r.db.QueryContext(ctx, query, plan.pageArgs...)
	if err != nil {
		return ItemIDPage{}, fmt.Errorf("failed to query item ids: %w", err)
	}
	defer func() { _ = rows.Close() }()

	ids := make([]int, 0, min(limit, total))
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return ItemIDPage{}, fmt.Errorf("failed to scan item id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return ItemIDPage{}, fmt.Errorf("failed to iterate item ids: %w", err)
	}
	return ItemIDPage{IDs: ids, Total: total}, nil
}

// unfilteredWorkspaceListWorkspaceID identifies the direct workspace-list hot
// path. Its total is invariant under sorting and can be
// shared safely for the short cache window. Filtered/search lists retain an
// exact count query so their totals do not inherit a stale workspace total.
func unfilteredWorkspaceListWorkspaceID(params ItemListParams) (int, bool) {
	if len(params.WorkspaceIDs) == 0 || params.Filters.WorkspaceID == nil || !params.Filters.isUnfiltered() {
		return 0, false
	}
	workspaceID := *params.Filters.WorkspaceID
	for _, accessibleWorkspaceID := range params.WorkspaceIDs {
		if accessibleWorkspaceID == workspaceID {
			return workspaceID, true
		}
	}
	return 0, false
}

func itemListCursorEligible(params ItemListParams) bool {
	if _, ok := unfilteredWorkspaceListWorkspaceID(params); !ok {
		return false
	}
	// The keyset predicate below matches only the canonical default order. A
	// caller asking for another sort keeps the established offset contract.
	return params.SortBy == "" || (params.SortBy == "frac_index" && params.SortAsc)
}

func encodeItemListCursor(cursor itemListCursor) string {
	payload, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeItemListCursor(raw string) (itemListCursor, error) {
	payload, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return itemListCursor{}, fmt.Errorf("%w: malformed base64", ErrInvalidItemListCursor)
	}
	var cursor itemListCursor
	if err := json.Unmarshal(payload, &cursor); err != nil || cursor.Rank == "" || cursor.ID <= 0 {
		return itemListCursor{}, fmt.Errorf("%w: malformed payload", ErrInvalidItemListCursor)
	}
	if _, err := ParseGlobalRank(cursor.Rank); err != nil {
		return itemListCursor{}, fmt.Errorf("%w: invalid rank", ErrInvalidItemListCursor)
	}
	return cursor, nil
}

type itemListPageQueryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func beginItemListCursorTransaction(ctx context.Context, db database.Database) (database.Tx, error) {
	if !database.IsPostgresDriver(db.GetDriverName()) {
		// SQLiteDB.BeginTx intentionally uses the single dedicated writer. Cursor
		// continuations need only a stable read snapshot, so use the ordinary read
		// pool and avoid serializing unrelated list calls with writes.
		tx, err := db.GetDB().BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		if err != nil {
			return nil, err
		}
		return database.NewSQLiteTx(tx), nil
	}
	return db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
}

func resolveItemListCursor(ctx context.Context, queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}, workspaceID int, cursor itemListCursor) (itemListCursor, error) {
	if workspaceID <= 0 {
		return itemListCursor{}, fmt.Errorf("%w: cursor requires a workspace", ErrInvalidItemListCursor)
	}
	var currentRank string
	if err := queryer.QueryRowContext(ctx, `SELECT frac_index FROM items WHERE id = ? AND workspace_id = ?`, cursor.ID, workspaceID).Scan(&currentRank); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return itemListCursor{}, fmt.Errorf("%w: cursor item no longer exists", ErrInvalidItemListCursor)
		}
		return itemListCursor{}, fmt.Errorf("resolve item list cursor: %w", err)
	}
	if _, err := ParseGlobalRank(currentRank); err != nil {
		return itemListCursor{}, fmt.Errorf("%w: cursor item has invalid rank", ErrInvalidItemListCursor)
	}
	cursor.Rank = currentRank
	return cursor, nil
}

func itemListCursorWhere(cursor itemListCursor) (where string, args []any, err error) {
	if _, err := ParseGlobalRank(cursor.Rank); err != nil {
		return "", nil, fmt.Errorf("%w: invalid rank", ErrInvalidItemListCursor)
	}
	return " AND (i.frac_index > ? OR (i.frac_index = ? AND i.id > ?))", []any{cursor.Rank, cursor.Rank, cursor.ID}, nil
}

func (r *ItemRepository) buildItemListPagePlan(
	params ItemListParams,
	countFromClause, whereClause string,
	args []any,
) itemListPagePlan {
	plan := itemListPagePlan{
		countQuery:      "SELECT COUNT(*) " + countFromClause + whereClause,
		countArgs:       args,
		pageFromClause:  countFromClause,
		pageWhereClause: whereClause,
		pageArgs:        args,
	}
	if workspaceID, cacheable := unfilteredWorkspaceListWorkspaceID(params); cacheable {
		// The explicit workspace filter is safe to use directly because the
		// workspace ID must also be present in the caller's accessible set.
		// Collection/QL/filter requests never enter this branch.
		workspaceArgs := []any{workspaceID}
		plan.countQuery = "SELECT COUNT(*) FROM items WHERE workspace_id = ?"
		plan.countArgs = workspaceArgs
		plan.pageFromClause = "FROM items i "
		plan.pageWhereClause = "WHERE i.workspace_id = ?"
		plan.pageArgs = workspaceArgs
		plan.workspaceCountID = workspaceID
	}
	return plan
}

// FindDistinctWorkspaceIDsContext returns the workspace IDs represented by an
// item filter without loading the matching item rows. It uses the same joins
// and WHERE builder as the paginated item list so CQL aliases and access
// scoping remain identical while the result size stays bounded by workspace
// count rather than item count.
func (r *ItemRepository) FindDistinctWorkspaceIDsContext(ctx context.Context, params ItemListParams) ([]int, error) {
	whereClause, args := r.buildWhereClause(params)
	query := `SELECT DISTINCT i.workspace_id
		` + ItemListFilterFromClause() + whereClause + `
		ORDER BY i.workspace_id`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query distinct item workspaces: %w", err)
	}
	defer func() { _ = rows.Close() }()

	workspaceIDs := []int{}
	for rows.Next() {
		var workspaceID int
		if err := rows.Scan(&workspaceID); err != nil {
			return nil, fmt.Errorf("failed to scan distinct item workspace: %w", err)
		}
		workspaceIDs = append(workspaceIDs, workspaceID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate distinct item workspaces: %w", err)
	}
	return workspaceIDs, nil
}

// Search searches items by title and description with text matching.
// It delegates to FindAllWithDetails using TextQuery/ItemKeyQuery filters.
func (r *ItemRepository) Search(query string, workspaceIDs []int, pagination PaginationParams) ([]models.Item, int, error) {
	return r.SearchContext(context.Background(), query, workspaceIDs, pagination)
}

// SearchContext is the request-aware form of Search.
func (r *ItemRepository) SearchContext(ctx context.Context, query string, workspaceIDs []int, pagination PaginationParams) ([]models.Item, int, error) {
	if len(workspaceIDs) == 0 {
		return []models.Item{}, 0, nil
	}

	filters := ItemFilters{}
	parts := strings.Split(strings.ToUpper(query), "-")
	isKeyPattern := len(parts) == 2 && parts[0] != "" && parts[1] != ""
	if isKeyPattern {
		if _, err := strconv.Atoi(parts[1]); err == nil {
			filters.ItemKeyQuery = query
		} else {
			filters.TextQuery = query
		}
	} else {
		filters.TextQuery = query
	}

	return r.FindAllWithDetailsContext(ctx, ItemListParams{
		WorkspaceIDs: workspaceIDs,
		Filters:      filters,
		Pagination:   pagination,
		SortBy:       "updated_at",
	})
}

// buildWhereClause constructs the WHERE clause and arguments for item queries
func (r *ItemRepository) buildWhereClause(params ItemListParams) (whereClause string, args []any) {
	whereClause = "WHERE 1=1"

	if len(params.WorkspaceIDs) > 0 {
		placeholders := make([]string, len(params.WorkspaceIDs))
		for i, id := range params.WorkspaceIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		whereClause += " AND i.workspace_id IN (" + strings.Join(placeholders, ",") + ")"
	}

	if params.Filters.QLQuery != "" {
		whereClause += " AND (" + params.Filters.QLQuery + ")"
		args = append(args, params.Filters.QLArgs...)
	}

	if params.Filters.WorkspaceID != nil {
		whereClause += " AND i.workspace_id = ?"
		args = append(args, *params.Filters.WorkspaceID)
	}

	if params.Filters.StatusID != nil {
		whereClause += " AND i.status_id = ?"
		args = append(args, *params.Filters.StatusID)
	}

	if params.Filters.StatusIDNot != nil {
		whereClause += " AND i.status_id != ?"
		args = append(args, *params.Filters.StatusIDNot)
	}

	if params.Filters.PriorityID != nil {
		whereClause += " AND i.priority_id = ?"
		args = append(args, *params.Filters.PriorityID)
	}

	if params.Filters.AssigneeID != nil {
		whereClause += " AND i.assignee_id = ?"
		args = append(args, *params.Filters.AssigneeID)
	}

	if params.Filters.CreatorID != nil {
		whereClause += " AND i.creator_id = ?"
		args = append(args, *params.Filters.CreatorID)
	}

	if params.Filters.ItemTypeID != nil {
		whereClause += " AND i.item_type_id = ?"
		args = append(args, *params.Filters.ItemTypeID)
	}

	if params.Filters.MilestoneID != nil {
		whereClause += " AND EXISTS (SELECT 1 FROM item_milestones im WHERE im.item_id = i.id AND im.milestone_id = ?)"
		args = append(args, *params.Filters.MilestoneID)
	}

	if params.Filters.IterationID != nil {
		whereClause += " AND i.iteration_id = ?"
		args = append(args, *params.Filters.IterationID)
	}

	if params.Filters.ParentIDIsSet {
		if params.Filters.ParentID == nil || *params.Filters.ParentID == 0 {
			whereClause += " AND i.parent_id IS NULL"
		} else {
			whereClause += " AND i.parent_id = ?"
			args = append(args, *params.Filters.ParentID)
		}
	}

	if params.Filters.Level != nil {
		whereClause += " AND COALESCE(it.hierarchy_level, 0) = ?"
		args = append(args, *params.Filters.Level)
	}

	if params.Filters.MaxLevel != nil {
		whereClause += " AND COALESCE(it.hierarchy_level, 0) >= 0 AND COALESCE(it.hierarchy_level, 0) <= ?"
		args = append(args, *params.Filters.MaxLevel)
	}

	if params.Filters.CreatedSince != nil {
		whereClause += " AND i.created_at >= ?"
		args = append(args, *params.Filters.CreatedSince)
	}

	// Cap completed items by when they entered their (completed) status.
	// Non-completed items always pass; only items whose current status is in a
	// completed category are constrained. The completion time mirrors the
	// status_since SELECT expression (last item_history transition into the
	// current status, falling back to created_at).
	if params.Filters.CompletedSince != nil {
		whereClause += ` AND (
			COALESCE(sc.is_completed, FALSE) = FALSE
			OR ` + cql.CurrentCompletedAtExpr("") + ` >= ?
		)`
		args = append(args, *params.Filters.CompletedSince)
	}

	if params.Filters.CompletedActivitySince != nil {
		whereClause += ` AND (
			COALESCE(sc.is_completed, FALSE) = FALSE
			OR COALESCE(i.last_active_at, i.updated_at, i.created_at) >= ?
		)`
		args = append(args, *params.Filters.CompletedActivitySince)
	}

	if len(params.Filters.StatusIDs) > 0 {
		placeholders := make([]string, len(params.Filters.StatusIDs))
		for i, id := range params.Filters.StatusIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		whereClause += " AND i.status_id IN (" + strings.Join(placeholders, ",") + ")"
	}

	if len(params.Filters.StatusIDsNot) > 0 {
		placeholders := make([]string, len(params.Filters.StatusIDsNot))
		for i, id := range params.Filters.StatusIDsNot {
			placeholders[i] = "?"
			args = append(args, id)
		}
		whereClause += " AND i.status_id NOT IN (" + strings.Join(placeholders, ",") + ")"
	}

	if len(params.Filters.PriorityIDs) > 0 {
		placeholders := make([]string, len(params.Filters.PriorityIDs))
		for i, id := range params.Filters.PriorityIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		whereClause += " AND i.priority_id IN (" + strings.Join(placeholders, ",") + ")"
	}

	if len(params.Filters.ItemIDs) > 0 {
		placeholders := make([]string, len(params.Filters.ItemIDs))
		for i, id := range params.Filters.ItemIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		whereClause += " AND i.id IN (" + strings.Join(placeholders, ",") + ")"
	}

	if params.Filters.TextQuery != "" {
		whereClause += " AND (LOWER(i.title) LIKE LOWER(?) OR LOWER(i.description) LIKE LOWER(?))"
		searchPattern := "%" + params.Filters.TextQuery + "%"
		args = append(args, searchPattern, searchPattern)
	}

	if params.Filters.ItemKeyQuery != "" {
		parts := strings.Split(strings.ToUpper(params.Filters.ItemKeyQuery), "-")
		if len(parts) == 2 {
			if num, err := strconv.Atoi(parts[1]); err == nil && num > 0 {
				whereClause += " AND (LOWER(w.key) = LOWER(?) AND i.workspace_item_number = ?)"
				args = append(args, parts[0], num)
			}
		}
	}

	if params.Filters.ItemID != nil {
		whereClause += " AND i.id = ?"
		args = append(args, *params.Filters.ItemID)
	}

	return whereClause, args
}

// buildOrderByClause constructs the ORDER BY clause.
// It supports system field identifiers (from systemFieldSortColumns) and custom field IDs
// (which sort via JSON extraction from i.custom_field_values).
func (r *ItemRepository) buildOrderByClause(sortBy string, sortAsc bool) string {
	if sortBy == "" {
		return r.defaultOrderBy()
	}

	direction := "DESC"
	if sortAsc {
		direction = "ASC"
	}

	// Preserve the canonical rank order for ascending fractional indexes.
	if sortBy == "frac_index" && sortAsc {
		return r.defaultOrderBy()
	}
	if sortBy == "last_active_at" {
		// Bubble Mode is paginated, so its ordering must be both null-safe and
		// deterministic. Older rows can lack last_active_at; use the same
		// fallback the API model exposes and break timestamp ties by item ID so
		// page boundaries cannot shuffle between requests.
		return fmt.Sprintf(
			" ORDER BY COALESCE(i.last_active_at, i.updated_at, i.created_at) %s, i.id %s",
			direction,
			direction,
		)
	}
	if col, ok := systemFieldSortColumns[sortBy]; ok {
		return fmt.Sprintf(" ORDER BY %s %s, i.id ASC", col, direction)
	}

	// Numeric IDs select custom fields; reject other input before building SQL.
	if _, err := strconv.Atoi(sortBy); err != nil {
		return r.defaultOrderBy()
	}

	var fieldType string
	err := r.db.QueryRow("SELECT field_type FROM custom_field_definitions WHERE id = ?", sortBy).Scan(&fieldType)
	if err != nil || unsortableCustomFieldTypes[fieldType] {
		return r.defaultOrderBy()
	}

	// Use the JSON extraction syntax supported by the active database.
	var expr string
	if database.IsPostgresDriver(r.db.GetDriverName()) {
		expr = fmt.Sprintf("(i.custom_field_values->>'%s')", sortBy)
	} else {
		expr = fmt.Sprintf(`NULLIF(i.custom_field_values, '') ->> '$.%q'`, sortBy)
	}

	if numericCustomFieldSortTypes[fieldType] {
		expr = fmt.Sprintf("CAST(%s AS NUMERIC)", expr)
	}

	return fmt.Sprintf(" ORDER BY %s %s, i.id ASC", expr, direction)
}

func (r *ItemRepository) defaultOrderBy() string {
	// frac_index is non-null and globally unique, so it fully determines order
	// while allowing workspace lists to seek through the composite rank index.
	return ` ORDER BY i.frac_index ASC`
}

// scanItemList scans rows into a slice of items
func (r *ItemRepository) scanItemList(rows *sql.Rows) ([]models.Item, error) {
	var items []models.Item

	for rows.Next() {
		var item models.Item
		var customFieldValuesJSON, calendarDataJSON sql.NullString
		var itemTypeID, parentID, parentWorkspaceItemNumber, iterationID, projectID, timeProjectID, assigneeID, creatorID, statusID, priorityID sql.NullInt64
		var dueDate, startDate, endDate sql.NullTime
		var statusSince sql.NullString
		var itemTypeName, itemTypeBuiltinKey, parentTitle, iterationName, iterationEndDate, projectName, timeProjectName sql.NullString
		var assigneeName, assigneeEmail, assigneeAvatar, creatorName, creatorEmail, statusName, statusColor sql.NullString
		var statusBuiltinKey, priorityName, priorityBuiltinKey, priorityIcon, priorityColor sql.NullString
		var fracIndex sql.NullString
		var storyPoints sql.NullFloat64
		var estimateMinutes sql.NullInt64
		var inheritProject bool
		// last_active_at can be NULL on rows created before it was consistently
		// populated, so scan through a nullable and fall back to updated_at.
		var lastActiveAt sql.NullTime

		err := rows.Scan(
			&item.ID, &item.WorkspaceID, &item.WorkspaceItemNumber, &itemTypeID, &item.Title, &item.Description,
			&statusID, &priorityID, &dueDate, &startDate, &endDate, &item.IsTask, &iterationID, &projectID, &inheritProject, &timeProjectID, &assigneeID, &creatorID, &customFieldValuesJSON, &calendarDataJSON, &parentID,
			&storyPoints, &estimateMinutes, &fracIndex, &item.CreatedAt, &item.UpdatedAt, &lastActiveAt, &item.WorkspaceName, &item.WorkspaceKey, &itemTypeName, &itemTypeBuiltinKey, &parentTitle, &parentWorkspaceItemNumber, &iterationName, &iterationEndDate, &projectName, &timeProjectName,
			&assigneeName, &assigneeEmail, &assigneeAvatar, &creatorName, &creatorEmail, &statusName, &statusBuiltinKey, &statusColor, &priorityName, &priorityBuiltinKey, &priorityIcon, &priorityColor,
			&statusSince,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan item: %w", err)
		}

		if lastActiveAt.Valid {
			item.LastActiveAt = lastActiveAt.Time
		} else {
			item.LastActiveAt = item.UpdatedAt
		}

		assignNullableInt(&item.ItemTypeID, itemTypeID)
		assignNullableInt(&item.ParentID, parentID)
		assignNullableInt(&item.ParentWorkspaceItemNumber, parentWorkspaceItemNumber)
		assignNullableInt(&item.IterationID, iterationID)
		assignNullableInt(&item.StatusID, statusID)
		assignNullableInt(&item.ProjectID, projectID)
		assignNullableInt(&item.TimeProjectID, timeProjectID)
		assignNullableInt(&item.PriorityID, priorityID)
		assignNullableInt(&item.AssigneeID, assigneeID)
		assignNullableInt(&item.CreatorID, creatorID)

		if dueDate.Valid {
			item.DueDate = &dueDate.Time
		}
		if startDate.Valid {
			item.StartDate = &startDate.Time
		}
		if endDate.Valid {
			item.EndDate = &endDate.Time
		}
		if statusSince.Valid {
			if t, ok := parseDBTime(statusSince.String); ok {
				item.StatusSince = &t
			}
		}
		if storyPoints.Valid {
			item.StoryPoints = &storyPoints.Float64
		}
		assignNullableInt(&item.EstimateMinutes, estimateMinutes)

		item.InheritProject = inheritProject
		assignNullableString(&item.ItemTypeName, itemTypeName)
		assignNullableString(&item.ItemTypeBuiltinKey, itemTypeBuiltinKey)
		assignNullableString(&item.ParentTitle, parentTitle)
		assignNullableString(&item.IterationName, iterationName)
		assignNullableString(&item.IterationEndDate, iterationEndDate)
		assignNullableString(&item.StatusName, statusName)
		assignNullableString(&item.StatusBuiltinKey, statusBuiltinKey)
		assignNullableString(&item.StatusColor, statusColor)
		assignNullableString(&item.ProjectName, projectName)
		assignNullableString(&item.TimeProjectName, timeProjectName)
		assignNullableString(&item.PriorityName, priorityName)
		assignNullableString(&item.PriorityBuiltinKey, priorityBuiltinKey)
		assignNullableString(&item.PriorityIcon, priorityIcon)
		assignNullableString(&item.PriorityColor, priorityColor)
		assignNullableString(&item.AssigneeName, assigneeName)
		assignNullableString(&item.AssigneeEmail, assigneeEmail)
		assignNullableString(&item.AssigneeAvatar, assigneeAvatar)
		assignNullableString(&item.CreatorName, creatorName)
		assignNullableString(&item.CreatorEmail, creatorEmail)

		if fracIndex.Valid {
			item.FracIndex = &fracIndex.String
		}

		if customFieldValuesJSON.Valid && customFieldValuesJSON.String != "" {
			if err := json.Unmarshal([]byte(customFieldValuesJSON.String), &item.CustomFieldValues); err != nil {
				item.CustomFieldValues = make(map[string]any)
			}
		} else {
			item.CustomFieldValues = make(map[string]any)
		}

		if calendarDataJSON.Valid && calendarDataJSON.String != "" {
			if err := json.Unmarshal([]byte(calendarDataJSON.String), &item.CalendarData); err != nil {
				item.CalendarData = []models.CalendarScheduleEntry{}
			}
		} else {
			item.CalendarData = []models.CalendarScheduleEntry{}
		}

		items = append(items, item)
	}

	if items == nil {
		items = []models.Item{}
	}

	return items, nil
}

// GetBacklogStatusIDs returns status IDs for backlog items.
// It first checks board_configurations for the workspace, then falls back to non-completed statuses.
func (r *ItemRepository) GetBacklogStatusIDs(workspaceID int) ([]int, error) {
	return r.GetBacklogStatusIDsContext(context.Background(), workspaceID)
}

// GetBacklogStatusIDsContext is the request-aware form of GetBacklogStatusIDs.
func (r *ItemRepository) GetBacklogStatusIDsContext(ctx context.Context, workspaceID int) ([]int, error) {
	// First, check if there's a board configuration with backlog_status_ids
	if workspaceID > 0 {
		var backlogStatusIDsJSON sql.NullString
		err := r.db.QueryRowContext(ctx, `
			SELECT backlog_status_ids
			FROM board_configurations
			WHERE workspace_id = ?`, workspaceID).Scan(&backlogStatusIDsJSON)

		if err == nil && backlogStatusIDsJSON.Valid && backlogStatusIDsJSON.String != "" {
			var statusIDs []int
			if err := json.Unmarshal([]byte(backlogStatusIDsJSON.String), &statusIDs); err != nil {
				return nil, fmt.Errorf("failed to parse backlog configuration: %w", err)
			}
			if len(statusIDs) > 0 {
				return statusIDs, nil
			}
		}
	}

	// Fall back to global "Open" statuses only (not "In Progress")
	rows, err := r.db.QueryContext(ctx, `
		SELECT DISTINCT s.id
		FROM statuses s
		JOIN status_categories sc ON s.category_id = sc.id
		WHERE COALESCE(sc.is_completed, FALSE) = FALSE
		AND COALESCE(sc.is_default, FALSE) = FALSE`)
	if err != nil {
		return nil, fmt.Errorf("failed to query backlog statuses: %w", err)
	}
	defer rows.Close()

	var statusIDs []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan backlog status: %w", err)
		}
		statusIDs = append(statusIDs, id)
	}

	return statusIDs, rows.Err()
}
