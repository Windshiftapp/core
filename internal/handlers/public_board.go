package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"windshift/internal/database"
	"windshift/internal/fileserve"
	"windshift/internal/markdown"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

const publicBoardItemLimit = 500

var publicBoardCardFieldAllowlist = map[string]struct{}{
	"key":          {},
	"title":        {},
	"status":       {},
	"priority":     {},
	"assignee":     {},
	"item_type":    {},
	"story_points": {},
	"due_date":     {},
	"labels":       {},
}

func filterPublicBoardCardFields(fields []models.ListColumn) []models.ListColumn {
	filtered := make([]models.ListColumn, 0, len(fields))
	for _, field := range fields {
		if field.FieldType != "system" {
			continue
		}
		if _, allowed := publicBoardCardFieldAllowlist[field.FieldIdentifier]; allowed {
			filtered = append(filtered, field)
		}
	}
	return filtered
}

func rewritePublicAttachmentURLs(content, slug string) string {
	return strings.ReplaceAll(
		content,
		"/api/attachments/",
		fmt.Sprintf("/api/public/board/%s/attachments/", slug),
	)
}

// PublicBoardHandler serves read-only public board views
type PublicBoardHandler struct {
	db                database.Database
	permissionService *services.PermissionService
	publicBoardScope  *services.PublicBoardScopeService
	attachmentPath    string
}

// NewPublicBoardHandler creates a new PublicBoardHandler
func NewPublicBoardHandler(db database.Database, permissionService *services.PermissionService, attachmentPath string) *PublicBoardHandler {
	return &PublicBoardHandler{
		db:                db,
		permissionService: permissionService,
		publicBoardScope:  services.NewPublicBoardScopeService(db, permissionService),
		attachmentPath:    attachmentPath,
	}
}

func isPublicBoardScopeError(err error) bool {
	return errors.Is(err, services.ErrPublicBoardWorkspaceScopeRequired) ||
		errors.Is(err, services.ErrPublicBoardWorkspaceNotFound)
}

// publicBoardCard is a stripped-down item for public display
type publicBoardCard struct {
	Key            string        `json:"key"`
	Title          string        `json:"title"`
	PriorityName   string        `json:"priority_name,omitempty"`
	PriorityIcon   string        `json:"priority_icon,omitempty"`
	PriorityColor  string        `json:"priority_color,omitempty"`
	AssigneeName   string        `json:"assignee_name,omitempty"`
	AssigneeAvatar string        `json:"assignee_avatar,omitempty"`
	Labels         []publicLabel `json:"labels,omitempty"`
	DueDate        string        `json:"due_date,omitempty"`
	StatusName     string        `json:"status_name,omitempty"`
	ItemTypeName   string        `json:"item_type_name,omitempty"`
	StoryPoints    *float64      `json:"story_points,omitempty"`
}

type publicLabel struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

type publicColumn struct {
	Name     string            `json:"name"`
	Color    string            `json:"color"`
	WIPLimit *int              `json:"wip_limit,omitempty"`
	Items    []publicBoardCard `json:"items"`
}

type publicBoardResponse struct {
	Collection struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"collection"`
	Columns     []publicColumn      `json:"columns"`
	CardFields  []models.ListColumn `json:"card_fields,omitempty"`
	TotalItems  int                 `json:"total_items"`
	LoadedItems int                 `json:"loaded_items"`
	Truncated   bool                `json:"truncated"`
	ItemLimit   int                 `json:"item_limit"`
	UpdatedAt   string              `json:"updated_at"`
}

type publicComment struct {
	AuthorName   string `json:"author_name"`
	AuthorAvatar string `json:"author_avatar,omitempty"`
	Content      string `json:"content"`
	ContentHTML  string `json:"content_html,omitempty"`
	CreatedAt    string `json:"created_at"`
}

type publicItemDetail struct {
	Key             string          `json:"key"`
	Title           string          `json:"title"`
	Description     string          `json:"description"`
	DescriptionHTML string          `json:"description_html,omitempty"`
	StatusName      string          `json:"status_name,omitempty"`
	StatusColor     string          `json:"status_color,omitempty"`
	PriorityName    string          `json:"priority_name,omitempty"`
	PriorityIcon    string          `json:"priority_icon,omitempty"`
	PriorityColor   string          `json:"priority_color,omitempty"`
	ItemTypeName    string          `json:"item_type_name,omitempty"`
	ItemTypeIcon    string          `json:"item_type_icon,omitempty"`
	ItemTypeColor   string          `json:"item_type_color,omitempty"`
	AssigneeName    string          `json:"assignee_name,omitempty"`
	AssigneeAvatar  string          `json:"assignee_avatar,omitempty"`
	DueDate         string          `json:"due_date,omitempty"`
	Labels          []publicLabel   `json:"labels,omitempty"`
	StoryPoints     *float64        `json:"story_points,omitempty"`
	Comments        []publicComment `json:"comments"`
	CreatedAt       string          `json:"created_at"`
}

// GetPublicBoardItem serves a single item detail for a public collection
func (h *PublicBoardHandler) GetPublicBoardItem(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	key := r.PathValue("key")
	if slug == "" || key == "" {
		respondNotFound(w, r, "item")
		return
	}

	// Split key on last "-" → workspace key + item number
	lastDash := strings.LastIndex(key, "-")
	if lastDash <= 0 {
		respondNotFound(w, r, "item")
		return
	}
	workspaceKey := key[:lastDash]
	itemNumber, err := strconv.Atoi(key[lastDash+1:])
	if err != nil || itemNumber <= 0 {
		respondNotFound(w, r, "item")
		return
	}

	// Validate slug → public collection
	var collectionID int
	err = h.db.QueryRow(`
		SELECT id FROM collections
		WHERE public_slug = ? AND is_public = true AND public_slug IS NOT NULL
	`, slug).Scan(&collectionID)
	if errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "board")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	publicItem, err := repository.NewItemRepository(h.db).FindPublicItemByKeyAndNumber(workspaceKey, itemNumber)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "item")
			return
		}
		respondInternalError(w, r, err)
		return
	}
	itemID := publicItem.ID
	title := publicItem.Title
	description := publicItem.Description
	createdAt := publicItem.CreatedAt

	// Verify item belongs to this collection by running the collection query
	belongs, err := h.itemBelongsToCollection(itemID, collectionID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !belongs {
		respondNotFound(w, r, "item")
		return
	}

	// Load labels for this item
	labels := h.loadSingleItemLabels(itemID)

	// Load public comments (non-private only)
	comments, err := h.loadPublicComments(itemID, slug)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Rewrite attachment URLs for public access
	description = rewritePublicAttachmentURLs(description, slug)
	descriptionHTML, err := markdown.Render(description)
	if err != nil {
		respondInternalError(w, r, fmt.Errorf("render public item description: %w", err))
		return
	}
	for i := range comments {
		comments[i].ContentHTML, err = markdown.Render(comments[i].Content)
		if err != nil {
			respondInternalError(w, r, fmt.Errorf("render public comment: %w", err))
			return
		}
	}

	detail := publicItemDetail{
		Key:             key,
		Title:           title,
		Description:     description,
		DescriptionHTML: descriptionHTML,
		StatusName:      publicItem.StatusName,
		StatusColor:     publicItem.StatusColor,
		PriorityName:    publicItem.PriorityName,
		PriorityIcon:    publicItem.PriorityIcon,
		PriorityColor:   publicItem.PriorityColor,
		ItemTypeName:    publicItem.ItemTypeName,
		ItemTypeIcon:    publicItem.ItemTypeIcon,
		ItemTypeColor:   publicItem.ItemTypeColor,
		AssigneeName:    publicItem.AssigneeName,
		AssigneeAvatar:  publicItem.AssigneeAvatar,
		Labels:          labels,
		Comments:        comments,
		CreatedAt:       createdAt,
		DueDate:         publicItem.DueDate,
		StoryPoints:     publicItem.StoryPoints,
	}

	respondJSONOK(w, detail)
}

// GetPublicBoard serves a read-only board view for a public collection
func (h *PublicBoardHandler) GetPublicBoard(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if slug == "" {
		respondNotFound(w, r, "board")
		return
	}

	// Look up collection by public_slug where is_public = true
	var collectionID int
	var collectionName, collectionDescription, qlQuery, updatedAt string
	err := h.db.QueryRow(`
		SELECT id, name, COALESCE(description, ''), COALESCE(ql_query, ''), updated_at
		FROM collections
		WHERE public_slug = ? AND is_public = true AND public_slug IS NOT NULL
	`, slug).Scan(&collectionID, &collectionName, &collectionDescription, &qlQuery, &updatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "board")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Load board configuration for this collection
	var boardConfigID int
	var backlogStatusIDsJSON, cardFieldsJSON sql.NullString
	err = h.db.QueryRow(`
		SELECT id, backlog_status_ids, card_fields
		FROM board_configurations
		WHERE collection_id = ?
	`, collectionID).Scan(&boardConfigID, &backlogStatusIDsJSON, &cardFieldsJSON)

	var cardFields []models.ListColumn
	var backlogStatusSet map[int]bool
	var columns []boardColumnInfo

	switch {
	case errors.Is(err, sql.ErrNoRows):
		// No explicit board config — build default columns from status categories
		columns, err = h.buildDefaultColumns()
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		backlogStatusSet = make(map[int]bool)
	case err != nil:
		respondInternalError(w, r, err)
		return
	default:
		// Parse card_fields
		if cardFieldsJSON.Valid && cardFieldsJSON.String != "" {
			_ = json.Unmarshal([]byte(cardFieldsJSON.String), &cardFields)
			cardFields = filterPublicBoardCardFields(cardFields)
		}

		// Parse backlog status IDs (items in these statuses won't appear on the board)
		var backlogStatusIDs []int
		if backlogStatusIDsJSON.Valid && backlogStatusIDsJSON.String != "" {
			_ = json.Unmarshal([]byte(backlogStatusIDsJSON.String), &backlogStatusIDs)
		}
		backlogStatusSet = make(map[int]bool, len(backlogStatusIDs))
		for _, id := range backlogStatusIDs {
			backlogStatusSet[id] = true
		}

		// Load columns with status mappings
		columns, err = h.loadColumnsWithStatuses(boardConfigID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	// Build status-to-column mapping
	statusToColumn := make(map[int]int) // statusID -> column index
	for i, col := range columns {
		for _, sid := range col.statusIDs {
			statusToColumn[sid] = i
		}
	}

	// Resolve the mandatory query scope and narrow it further to the
	// collection's home workspace when one is set.
	scopedWorkspaceIDs, err := h.resolveCollectionWorkspaceIDs(collectionID)
	if err != nil {
		if isPublicBoardScopeError(err) {
			respondNotFound(w, r, "board")
			return
		}
		respondInternalError(w, r, err)
		return
	}
	if len(scopedWorkspaceIDs) == 0 {
		// No workspaces, return empty board
		resp := h.buildEmptyResponse(collectionName, collectionDescription, columns, cardFields, updatedAt)
		respondJSONOK(w, resp)
		return
	}

	// Execute the collection's QL query to get items
	crudService := services.NewItemCRUDService(h.db)
	items, totalItems, err := crudService.ListWithQL(services.ListWithQLParams{
		CollectionID: collectionID,
		WorkspaceIDs: scopedWorkspaceIDs,
		Pagination:   services.PaginationParams{Limit: publicBoardItemLimit},
		SortBy:       "created_at",
		SortAsc:      false,
	})
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Load labels for all items
	itemLabels := h.loadItemLabels(items)

	// Build response columns with grouped items
	responseColumns := make([]publicColumn, len(columns))
	for i, col := range columns {
		responseColumns[i] = publicColumn{
			Name:     col.name,
			Color:    col.color,
			WIPLimit: col.wipLimit,
			Items:    []publicBoardCard{},
		}
	}

	// Group items by column
	for _, item := range items {
		if item.StatusID == nil {
			continue
		}
		// Skip backlog items
		if backlogStatusSet[*item.StatusID] {
			continue
		}
		colIdx, found := statusToColumn[*item.StatusID]
		if !found {
			continue
		}

		card := publicBoardCard{
			Key:            item.WorkspaceKey + "-" + itoa(item.WorkspaceItemNumber),
			Title:          item.Title,
			PriorityName:   item.PriorityName,
			PriorityIcon:   item.PriorityIcon,
			PriorityColor:  item.PriorityColor,
			AssigneeName:   item.AssigneeName,
			AssigneeAvatar: item.AssigneeAvatar,
			StatusName:     item.StatusName,
			ItemTypeName:   item.ItemTypeName,
			StoryPoints:    item.StoryPoints,
		}

		if item.DueDate != nil {
			card.DueDate = item.DueDate.Format("2006-01-02")
		}

		// Attach labels
		if labels, ok := itemLabels[item.ID]; ok {
			card.Labels = labels
		}

		responseColumns[colIdx].Items = append(responseColumns[colIdx].Items, card)
	}

	resp := publicBoardResponse{
		Columns:     responseColumns,
		CardFields:  cardFields,
		TotalItems:  totalItems,
		LoadedItems: len(items),
		Truncated:   totalItems > len(items),
		ItemLimit:   publicBoardItemLimit,
		UpdatedAt:   updatedAt,
	}
	resp.Collection.Name = collectionName
	resp.Collection.Description = collectionDescription

	respondJSONOK(w, resp)
}

// boardColumnInfo holds column data loaded from the DB
type boardColumnInfo struct {
	name      string
	color     string
	wipLimit  *int
	statusIDs []int
}

func (h *PublicBoardHandler) loadColumnsWithStatuses(configID int) ([]boardColumnInfo, error) {
	rows, err := h.db.Query(`
		SELECT id, name, wip_limit, color
		FROM board_columns
		WHERE board_configuration_id = ?
		ORDER BY display_order
	`, configID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	type colRow struct {
		id       int
		name     string
		wipLimit *int
		color    string
	}
	var colRows []colRow
	for rows.Next() {
		var c colRow
		var wipLimit sql.NullInt64
		if err := rows.Scan(&c.id, &c.name, &wipLimit, &c.color); err != nil {
			return nil, err
		}
		if wipLimit.Valid {
			v := int(wipLimit.Int64)
			c.wipLimit = &v
		}
		colRows = append(colRows, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	columns := make([]boardColumnInfo, len(colRows))
	for i, cr := range colRows {
		columns[i] = boardColumnInfo{
			name:     cr.name,
			color:    cr.color,
			wipLimit: cr.wipLimit,
		}

		// Load status mappings
		srows, err := h.db.Query(`SELECT status_id FROM board_column_statuses WHERE board_column_id = ?`, cr.id)
		if err != nil {
			return nil, err
		}
		var statusIDs []int
		for srows.Next() {
			var sid int
			if err := srows.Scan(&sid); err != nil {
				_ = srows.Close()
				return nil, err
			}
			statusIDs = append(statusIDs, sid)
		}
		if err := srows.Err(); err != nil {
			_ = srows.Close()
			return nil, err
		}
		_ = srows.Close()
		columns[i].statusIDs = statusIDs
	}

	return columns, nil
}

func (h *PublicBoardHandler) buildDefaultColumns() ([]boardColumnInfo, error) {
	// Load status categories ordered: non-completed first, then completed
	catRows, err := h.db.Query(`SELECT id, name, color FROM status_categories ORDER BY is_completed ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = catRows.Close() }()

	type category struct {
		id    int
		name  string
		color string
	}
	var categories []category
	for catRows.Next() {
		var c category
		if err := catRows.Scan(&c.id, &c.name, &c.color); err != nil {
			return nil, err
		}
		categories = append(categories, c)
	}
	if err := catRows.Err(); err != nil {
		return nil, err
	}

	// Load all statuses grouped by category
	statusRows, err := h.db.Query(`SELECT id, category_id FROM statuses ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = statusRows.Close() }()

	statusesByCat := make(map[int][]int)
	for statusRows.Next() {
		var sid, catID int
		if err := statusRows.Scan(&sid, &catID); err != nil {
			return nil, err
		}
		statusesByCat[catID] = append(statusesByCat[catID], sid)
	}
	if err := statusRows.Err(); err != nil {
		return nil, err
	}

	// Build columns: one per category
	columns := make([]boardColumnInfo, 0, len(categories))
	for _, cat := range categories {
		columns = append(columns, boardColumnInfo{
			name:      cat.name,
			color:     cat.color,
			statusIDs: statusesByCat[cat.id],
		})
	}
	return columns, nil
}

// resolveCollectionWorkspaceIDs derives the anonymous read boundary from the
// collection query and intersects it with the optional home workspace.
func (h *PublicBoardHandler) resolveCollectionWorkspaceIDs(collectionID int) ([]int, error) {
	var wsID sql.NullInt64
	var qlQuery string
	err := h.db.QueryRow(`SELECT workspace_id, COALESCE(ql_query, '') FROM collections WHERE id = ?`, collectionID).Scan(&wsID, &qlQuery)
	if err != nil {
		return nil, err
	}
	workspaceIDs, err := h.publicBoardScope.ResolveWorkspaceIDs(qlQuery)
	if err != nil {
		return nil, err
	}
	if !wsID.Valid {
		return workspaceIDs, nil
	}
	for _, workspaceID := range workspaceIDs {
		if workspaceID == int(wsID.Int64) {
			return []int{workspaceID}, nil
		}
	}
	return []int{}, nil
}

func (h *PublicBoardHandler) loadItemLabels(items []models.Item) map[int][]publicLabel {
	if len(items) == 0 {
		return nil
	}

	result := make(map[int][]publicLabel)
	for _, item := range items {
		if len(item.Labels) > 0 {
			var labels []publicLabel
			for _, l := range item.Labels {
				labels = append(labels, publicLabel{Name: l.Name, Color: l.Color})
			}
			result[item.ID] = labels
		}
	}
	return result
}

func (h *PublicBoardHandler) buildEmptyResponse(name, desc string, columns []boardColumnInfo, cardFields []models.ListColumn, updatedAt string) publicBoardResponse {
	resp := publicBoardResponse{
		CardFields: cardFields,
		ItemLimit:  publicBoardItemLimit,
		UpdatedAt:  updatedAt,
	}
	resp.Collection.Name = name
	resp.Collection.Description = desc
	for _, col := range columns {
		resp.Columns = append(resp.Columns, publicColumn{
			Name:     col.name,
			Color:    col.color,
			WIPLimit: col.wipLimit,
			Items:    []publicBoardCard{},
		})
	}
	return resp
}

func (h *PublicBoardHandler) loadSingleItemLabels(itemID int) []publicLabel {
	rows, err := h.db.Query(`
		SELECT l.name, l.color
		FROM labels l
		JOIN item_labels il ON il.label_id = l.id
		WHERE il.item_id = ?
	`, itemID)
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()

	var labels []publicLabel
	for rows.Next() {
		var l publicLabel
		if err := rows.Scan(&l.Name, &l.Color); err != nil {
			return labels
		}
		labels = append(labels, l)
	}
	if err := rows.Err(); err != nil {
		return labels
	}
	return labels
}

func (h *PublicBoardHandler) loadPublicComments(itemID int, slug string) ([]publicComment, error) {
	rows, err := h.db.Query(`
		SELECT COALESCE(u.first_name || ' ' || u.last_name, pc.name, 'Unknown'),
		       COALESCE(u.avatar_url, ''),
		       c.content, c.created_at
		FROM comments c
		LEFT JOIN users u ON c.author_id = u.id
		LEFT JOIN portal_customers pc ON c.portal_customer_id = pc.id
		WHERE c.item_id = ? AND c.is_private = false
		ORDER BY c.created_at ASC
	`, itemID)
	if err != nil {
		return nil, fmt.Errorf("failed to load comments: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var comments []publicComment
	for rows.Next() {
		var c publicComment
		if err := rows.Scan(&c.AuthorName, &c.AuthorAvatar, &c.Content, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan comment: %w", err)
		}
		c.Content = rewritePublicAttachmentURLs(c.Content, slug)
		comments = append(comments, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if comments == nil {
		comments = []publicComment{}
	}
	return comments, nil
}

// DownloadAttachment serves an image attachment for a public board item
func (h *PublicBoardHandler) DownloadAttachment(w http.ResponseWriter, r *http.Request) {
	// Publication and membership are mutable authorization inputs; never cache.
	w.Header().Set("Cache-Control", "no-store")

	slug := r.PathValue("slug")
	attachmentIDStr := r.PathValue("id")
	if slug == "" || attachmentIDStr == "" {
		respondNotFound(w, r, "attachment")
		return
	}

	attachmentID, err := strconv.Atoi(attachmentIDStr)
	if err != nil {
		respondNotFound(w, r, "attachment")
		return
	}

	// Validate slug → public collection
	var collectionID int
	err = h.db.QueryRow(`
		SELECT id FROM collections
		WHERE public_slug = ? AND is_public = true AND public_slug IS NOT NULL
	`, slug).Scan(&collectionID)
	if err != nil {
		respondNotFound(w, r, "attachment")
		return
	}

	// Require an item attachment; legacy rows reuse item_id for other entities.
	var itemID sql.NullInt64
	var entityType sql.NullString
	var filePath, mimeType, originalFilename string
	var fileSize int64
	err = h.db.QueryRow(`
		SELECT item_id, COALESCE(entity_type, 'item'), file_path, mime_type, original_filename, file_size
		FROM attachments WHERE id = ?
	`, attachmentID).Scan(&itemID, &entityType, &filePath, &mimeType, &originalFilename, &fileSize)
	if err != nil {
		respondNotFound(w, r, "attachment")
		return
	}

	if !itemID.Valid || (entityType.String != "" && entityType.String != "item") {
		respondNotFound(w, r, "attachment")
		return
	}

	// SVG is excluded because it can execute scripts.
	if !strings.HasPrefix(mimeType, "image/") || mimeType == "image/svg+xml" {
		respondNotFound(w, r, "attachment")
		return
	}

	belongs, err := h.itemBelongsToCollection(int(itemID.Int64), collectionID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !belongs {
		respondNotFound(w, r, "attachment")
		return
	}

	// Confine reads to storage root and hide traversal or symlink failures as 404.
	file, err := fileserve.OpenUnderRoot(h.attachmentPath, filePath)
	if err != nil {
		respondNotFound(w, r, "attachment")
		return
	}
	defer func() { _ = file.Close() }()

	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Length", strconv.FormatInt(fileSize, 10))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Disposition", fileserve.ContentDisposition("inline", originalFilename))

	_, _ = io.Copy(w, file)
}

// itemBelongsToCollection checks the collection's workspace-scoped QL result.
func (h *PublicBoardHandler) itemBelongsToCollection(itemID, collectionID int) (bool, error) {
	scopedWorkspaceIDs, err := h.resolveCollectionWorkspaceIDs(collectionID)
	if err != nil {
		if isPublicBoardScopeError(err) {
			return false, nil
		}
		return false, err
	}

	crudService := services.NewItemCRUDService(h.db)
	items, _, err := crudService.ListWithQL(services.ListWithQLParams{
		CollectionID: collectionID,
		WorkspaceIDs: scopedWorkspaceIDs,
		Filters:      services.ItemFilters{ItemID: &itemID},
		Pagination:   services.PaginationParams{Limit: 1},
		SortBy:       "created_at",
		SortAsc:      false,
	})
	if err != nil {
		return false, err
	}

	return len(items) > 0, nil
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
