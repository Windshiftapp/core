package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/utils"
)

type PersonalLabelHandler struct {
	db                database.Database
	permissionService *services.PermissionService
}

func NewPersonalLabelHandler(db database.Database, permissionService *services.PermissionService) *PersonalLabelHandler {
	return &PersonalLabelHandler{db: db, permissionService: permissionService}
}

// hexColorRE accepts #RGB or #RRGGBB. Anything else falls back to the default color.
var hexColorRE = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

const defaultPersonalLabelColor = "#3B82F6"

// GetAll lists personal labels visible to the current user.
//
// Default (no ?user_id query): returns the union of the caller's own labels
// (user_id = self) and shared labels (user_id IS NULL). This is what the
// unified picker and the Profile manager need.
//
// Backwards-compat: an explicit ?user_id=<id> still filters as before.
// ?user_id=null or ?user_id=0 returns only shared labels.
func (h *PersonalLabelHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	userIDParam := r.URL.Query().Get("user_id")
	query := `
		SELECT id, name, color, user_id, created_at, updated_at
		FROM personal_labels
		WHERE `
	var args []interface{}

	switch userIDParam {
	case "":
		// Union: my labels + shared.
		query += "(user_id = ? OR user_id IS NULL)"
		args = append(args, user.ID)
	case "null", "0":
		query += "user_id IS NULL"
	default:
		// Explicit user_id: that user's labels OR shared labels.
		id, err := strconv.Atoi(userIDParam)
		if err != nil {
			respondValidationError(w, r, "Invalid user_id")
			return
		}
		query += "(user_id = ? OR user_id IS NULL)"
		args = append(args, id)
	}

	query += " ORDER BY name"

	rows, err := h.db.Query(query, args...)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	labels := []models.PersonalLabel{}
	for rows.Next() {
		label, err := scanPersonalLabel(rows)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		labels = append(labels, label)
	}
	if err := rows.Err(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, labels)
}

// validatePersonalLabel validates name + color, sanitizes name, and checks
// uniqueness within the appropriate scope. excludeID is 0 on create, the
// row's id on update.
func (h *PersonalLabelHandler) validatePersonalLabel(w http.ResponseWriter, r *http.Request, label *models.PersonalLabel, excludeID int) bool {
	if strings.TrimSpace(label.Name) == "" {
		respondValidationError(w, r, "Label name is required")
		return false
	}

	// Label names are stored as comma-separated values in custom field blobs;
	// a comma in the name itself would corrupt that encoding on decode.
	if strings.Contains(label.Name, ",") {
		respondValidationError(w, r, "Label name cannot contain a comma")
		return false
	}

	label.Name = utils.SanitizeTitle(label.Name)

	// Preserve the caller's color when valid; default only when missing/invalid.
	if strings.TrimSpace(label.Color) == "" || !hexColorRE.MatchString(label.Color) {
		label.Color = defaultPersonalLabelColor
	}

	var existingCount int
	if label.UserID != nil {
		query := "SELECT COUNT(*) FROM personal_labels WHERE name = ? AND user_id = ?"
		args := []interface{}{label.Name, *label.UserID}
		if excludeID > 0 {
			query += " AND id != ?"
			args = append(args, excludeID)
		}
		if err := h.db.QueryRow(query, args...).Scan(&existingCount); err != nil {
			respondInternalError(w, r, err)
			return false
		}
	} else {
		query := "SELECT COUNT(*) FROM personal_labels WHERE name = ? AND user_id IS NULL"
		args := []interface{}{label.Name}
		if excludeID > 0 {
			query += " AND id != ?"
			args = append(args, excludeID)
		}
		if err := h.db.QueryRow(query, args...).Scan(&existingCount); err != nil {
			respondInternalError(w, r, err)
			return false
		}
	}

	if existingCount > 0 {
		respondConflict(w, r, "A label with this name already exists")
		return false
	}

	return true
}

func (h *PersonalLabelHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	label, ok := h.loadPersonalLabel(w, r, id)
	if !ok {
		return
	}

	respondJSONOK(w, label)
}

func (h *PersonalLabelHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	label, ok := decodeJSON[models.PersonalLabel](w, r)
	if !ok {
		return
	}

	// A user may create their own label or a shared (user_id NULL) label.
	// Disallow creating labels owned by another user.
	if label.UserID != nil && *label.UserID != user.ID {
		respondValidationError(w, r, "Cannot create a label owned by another user")
		return
	}

	if !h.validatePersonalLabel(w, r, &label, 0) {
		return
	}

	now := time.Now()
	var id int64
	err := h.db.QueryRow(`
		INSERT INTO personal_labels (name, color, user_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?) RETURNING id
	`, label.Name, label.Color, label.UserID, now, now).Scan(&id)

	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	created, ok := h.loadPersonalLabel(w, r, int(id))
	if !ok {
		return
	}

	respondJSONCreated(w, created)
}

func (h *PersonalLabelHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	existing, ok := h.loadPersonalLabel(w, r, id)
	if !ok {
		return
	}

	if !h.canMutatePersonalLabel(w, r, existing, user.ID) {
		return
	}

	label, ok := decodeJSON[models.PersonalLabel](w, r)
	if !ok {
		return
	}

	// A user may update their own label, or a shared label, or toggle ownership
	// between "mine" and "shared". They may not transfer a label to another user.
	if label.UserID != nil && *label.UserID != user.ID {
		respondValidationError(w, r, "Cannot transfer a label to another user")
		return
	}

	if !h.validatePersonalLabel(w, r, &label, id) {
		return
	}

	now := time.Now()
	if _, err := h.db.ExecWrite(`
		UPDATE personal_labels
		SET name = ?, color = ?, user_id = ?, updated_at = ?
		WHERE id = ?
	`, label.Name, label.Color, label.UserID, now, id); err != nil {
		respondInternalError(w, r, err)
		return
	}

	updated, ok := h.loadPersonalLabel(w, r, id)
	if !ok {
		return
	}

	respondJSONOK(w, updated)
}

func (h *PersonalLabelHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	existing, ok := h.loadPersonalLabel(w, r, id)
	if !ok {
		return
	}

	if !h.canMutatePersonalLabel(w, r, existing, user.ID) {
		return
	}

	if _, err := h.db.ExecWrite("DELETE FROM personal_labels WHERE id = ?", id); err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// loadPersonalLabel fetches a single row by id, writing 404 if missing.
func (h *PersonalLabelHandler) loadPersonalLabel(w http.ResponseWriter, r *http.Request, id int) (models.PersonalLabel, bool) {
	row := h.db.QueryRow(`
		SELECT id, name, color, user_id, created_at, updated_at
		FROM personal_labels
		WHERE id = ?
	`, id)

	label, err := scanPersonalLabel(row)
	if errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "Personal label")
		return models.PersonalLabel{}, false
	}
	if err != nil {
		respondInternalError(w, r, err)
		return models.PersonalLabel{}, false
	}
	return label, true
}

// canMutatePersonalLabel enforces the ownership rules for Update and Delete:
// owners can mutate their own labels; any authenticated user can mutate a
// shared label (user_id IS NULL). Returns 404 (not 403) on denial, consistent
// with the no-leak policy used elsewhere in the codebase.
func (h *PersonalLabelHandler) canMutatePersonalLabel(w http.ResponseWriter, r *http.Request, label models.PersonalLabel, currentUserID int) bool {
	if label.UserID == nil {
		return true
	}
	if *label.UserID == currentUserID {
		return true
	}
	respondNotFound(w, r, "Personal label")
	return false
}

// scanPersonalLabel reads one row into a models.PersonalLabel, mapping
// SQL NULL user_id to a nil *int.
func scanPersonalLabel(s rowScanner) (models.PersonalLabel, error) {
	var label models.PersonalLabel
	var userID sql.NullInt64
	if err := s.Scan(&label.ID, &label.Name, &label.Color, &userID,
		&label.CreatedAt, &label.UpdatedAt); err != nil {
		return models.PersonalLabel{}, err
	}
	if userID.Valid {
		v := int(userID.Int64)
		label.UserID = &v
	}
	return label, nil
}

// scanPersonalLabel uses the package-shared rowScanner interface (defined in
// theme.go) so it works for both *sql.Row and *sql.Rows.

// ===== Item-personal-label association =====

// GetItemPersonalLabels returns the personal labels visible to the current
// user that are attached to the given item.
func (h *PersonalLabelHandler) GetItemPersonalLabels(w http.ResponseWriter, r *http.Request) {
	itemID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	if !CheckItemPermission(w, r, repository.NewItemRepository(h.db), h.permissionService, itemID, models.PermissionItemView) {
		return
	}

	h.respondItemPersonalLabels(w, r, itemID, user.ID)
}

// SetItemPersonalLabels replaces the caller's visible labels on the item with
// the supplied set. Crucially, the DELETE half is restricted to rows the
// caller is allowed to see, so we never wipe another user's personal-label
// assignments. Shared labels are touchable by anyone with edit permission.
func (h *PersonalLabelHandler) SetItemPersonalLabels(w http.ResponseWriter, r *http.Request) {
	itemID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	if !CheckItemPermission(w, r, repository.NewItemRepository(h.db), h.permissionService, itemID, models.PermissionItemEdit) {
		return
	}

	var input struct {
		LabelIDs []int `json:"label_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondBadRequest(w, r, err.Error())
		return
	}

	// Validate every supplied label is one the user is allowed to attach.
	for _, lid := range input.LabelIDs {
		if !h.userMayUseLabel(lid, user.ID) {
			respondNotFound(w, r, "Personal label")
			return
		}
	}

	tx, err := h.db.Begin()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	// Delete only rows visible to this user. SQL: join personal_labels, filter
	// by (user_id IS NULL OR user_id = ?). Wrapping in a sub-select keeps the
	// statement portable across SQLite and Postgres.
	if _, err := tx.Exec(`
		DELETE FROM personal_item_labels
		WHERE item_id = ?
		  AND personal_label_id IN (
		      SELECT id FROM personal_labels
		      WHERE user_id IS NULL OR user_id = ?
		  )
	`, itemID, user.ID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	now := time.Now()
	for _, labelID := range input.LabelIDs {
		if _, err := tx.Exec(
			"INSERT INTO personal_item_labels (item_id, personal_label_id, created_at) VALUES (?, ?, ?)",
			itemID, labelID, now,
		); err != nil {
			respondInternalError(w, r, fmt.Errorf("failed to add personal label %d: %w", labelID, err))
			return
		}
	}

	if err := tx.Commit(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	h.respondItemPersonalLabels(w, r, itemID, user.ID)
}

// AddItemPersonalLabel attaches a single label to an item.
func (h *PersonalLabelHandler) AddItemPersonalLabel(w http.ResponseWriter, r *http.Request) {
	itemID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	if !CheckItemPermission(w, r, repository.NewItemRepository(h.db), h.permissionService, itemID, models.PermissionItemEdit) {
		return
	}

	var input struct {
		LabelID int `json:"label_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		respondBadRequest(w, r, err.Error())
		return
	}
	if input.LabelID == 0 {
		respondValidationError(w, r, "label_id is required")
		return
	}

	if !h.userMayUseLabel(input.LabelID, user.ID) {
		respondNotFound(w, r, "Personal label")
		return
	}

	now := time.Now()
	if _, err := h.db.ExecWrite(
		"INSERT INTO personal_item_labels (item_id, personal_label_id, created_at) VALUES (?, ?, ?)",
		itemID, input.LabelID, now,
	); err != nil {
		if database.IsUniqueConstraintError(err) {
			respondConflict(w, r, "Label is already assigned to this item")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	h.respondItemPersonalLabels(w, r, itemID, user.ID)
}

// RemoveItemPersonalLabel detaches a single label from an item. Refusing to
// touch labels the caller cannot see prevents a user from unattaching another
// user's personal label by guessing IDs.
func (h *PersonalLabelHandler) RemoveItemPersonalLabel(w http.ResponseWriter, r *http.Request) {
	itemID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	if !CheckItemPermission(w, r, repository.NewItemRepository(h.db), h.permissionService, itemID, models.PermissionItemEdit) {
		return
	}

	labelID, ok := requireIDParam(w, r, "labelId")
	if !ok {
		return
	}

	if !h.userMayUseLabel(labelID, user.ID) {
		respondNotFound(w, r, "Personal label")
		return
	}

	if _, err := h.db.ExecWrite(
		"DELETE FROM personal_item_labels WHERE item_id = ? AND personal_label_id = ?",
		itemID, labelID,
	); err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// userMayUseLabel returns true iff the label is visible (and therefore
// usable) by the given user: it must be either shared or owned by them.
// A missing label returns false — callers translate to 404.
func (h *PersonalLabelHandler) userMayUseLabel(labelID, userID int) bool {
	var ownerID sql.NullInt64
	err := h.db.QueryRow("SELECT user_id FROM personal_labels WHERE id = ?", labelID).Scan(&ownerID)
	if err != nil {
		return false
	}
	if !ownerID.Valid {
		return true // shared
	}
	return int(ownerID.Int64) == userID
}

// respondItemPersonalLabels writes the user-visible labels for the item.
func (h *PersonalLabelHandler) respondItemPersonalLabels(w http.ResponseWriter, r *http.Request, itemID, viewingUserID int) {
	rows, err := h.db.Query(`
		SELECT pl.id, pl.name, pl.color, pl.user_id, pl.created_at, pl.updated_at
		FROM personal_item_labels pil
		JOIN personal_labels pl ON pil.personal_label_id = pl.id
		WHERE pil.item_id = ?
		  AND (pl.user_id IS NULL OR pl.user_id = ?)
		ORDER BY pl.name
	`, itemID, viewingUserID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	labels := []models.PersonalLabel{}
	for rows.Next() {
		label, err := scanPersonalLabel(rows)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		labels = append(labels, label)
	}
	if err := rows.Err(); err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, labels)
}

// LoadPersonalLabelsForItems bulk-loads personal labels for a slice of items
// in a single query, attaching them to each item's PersonalLabels field.
// The viewing user determines visibility: a user only sees their own personal
// labels plus any shared (user_id IS NULL) labels.
func LoadPersonalLabelsForItems(db database.Database, items []models.Item, viewingUserID int) error {
	if len(items) == 0 {
		return nil
	}

	itemIDs := make([]interface{}, 0, len(items)+1)
	placeholders := make([]string, 0, len(items))
	for _, item := range items {
		itemIDs = append(itemIDs, item.ID)
		placeholders = append(placeholders, "?")
	}
	itemIDs = append(itemIDs, viewingUserID)

	query := fmt.Sprintf(`
		SELECT pil.item_id, pl.id, pl.name, pl.color, pl.user_id, pl.created_at, pl.updated_at
		FROM personal_item_labels pil
		JOIN personal_labels pl ON pil.personal_label_id = pl.id
		WHERE pil.item_id IN (%s)
		  AND (pl.user_id IS NULL OR pl.user_id = ?)
		ORDER BY pl.name
	`, strings.Join(placeholders, ","))

	rows, err := db.Query(query, itemIDs...)
	if err != nil {
		return fmt.Errorf("failed to load personal labels for items: %w", err)
	}
	defer func() { _ = rows.Close() }()

	labelMap := make(map[int][]models.PersonalLabel)
	for rows.Next() {
		var itemID int
		var label models.PersonalLabel
		var userID sql.NullInt64
		if err := rows.Scan(&itemID, &label.ID, &label.Name, &label.Color, &userID,
			&label.CreatedAt, &label.UpdatedAt); err != nil {
			return fmt.Errorf("failed to scan personal label: %w", err)
		}
		if userID.Valid {
			v := int(userID.Int64)
			label.UserID = &v
		}
		labelMap[itemID] = append(labelMap[itemID], label)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate personal labels: %w", err)
	}

	for i := range items {
		if labels, ok := labelMap[items[i].ID]; ok {
			items[i].PersonalLabels = labels
		}
	}

	return nil
}
