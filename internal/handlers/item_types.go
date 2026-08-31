package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/objecttranslation"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
	"windshift/internal/utils"
)

// sanitizeItemTypeRequest scrubs the user-facing fields on an item-type
// payload. Name renders in type pickers + board cards, Description in
// the type editor; Icon (Lucide icon name) and Color (hex) are
// identifier-shaped.
func sanitizeItemTypeRequest(it *models.ItemType) {
	sanitize.ApplyAll(
		sanitize.Pair{Target: &it.Name, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &it.Description, Policy: sanitize.RichText},
		sanitize.Pair{Target: &it.Icon, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: &it.Color, Policy: sanitize.ShortIdentifier},
	)
}

type ItemTypeHandler struct {
	db           database.Database
	repo         *repository.ItemTypeRepository
	translations *objecttranslation.Service
}

func (h *ItemTypeHandler) WithObjectTranslations(service *objecttranslation.Service) *ItemTypeHandler {
	h.translations = service
	return h
}

func NewItemTypeHandler(db database.Database) *ItemTypeHandler {
	return &ItemTypeHandler{db: db, repo: repository.NewItemTypeRepository(db)}
}

func (h *ItemTypeHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	var filter *int
	if cs := r.URL.Query().Get("configuration_set_id"); cs != "" {
		n, err := strconv.Atoi(cs)
		if err != nil {
			respondValidationError(w, r, "Invalid configuration_set_id")
			return
		}
		filter = &n
	}

	itemTypes, err := h.repo.List(filter)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !localizeObjectResponse(w, r, h.translations, "item_type", itemTypes) {
		return
	}
	respondJSONOK(w, itemTypes)
}

func (h *ItemTypeHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	it, err := h.repo.GetByID(id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "item_type")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !localizeObjectResponse(w, r, h.translations, "item_type", it) {
		return
	}
	respondJSONOK(w, it)
}

func (h *ItemTypeHandler) Create(w http.ResponseWriter, r *http.Request) {
	it, ok := decodeJSON[models.ItemType](w, r)
	if !ok {
		return
	}
	sanitizeItemTypeRequest(&it)

	if strings.TrimSpace(it.Name) == "" {
		respondValidationError(w, r, "Item type name is required")
		return
	}

	// Support both old (single) and new (multiple) configuration set IDs.
	configSetIDs := it.ConfigurationSetIDs
	if len(configSetIDs) == 0 && it.ConfigurationSetID != 0 {
		configSetIDs = []int{it.ConfigurationSetID}
	}
	if !h.configurationSetsExist(w, r, configSetIDs) {
		return
	}

	nameExists, err := h.repo.NameExists(it.Name, 0)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if nameExists {
		respondConflict(w, r, "Item type with this name already exists")
		return
	}

	id, err := h.repo.Create(&it, configSetIDs)
	if err != nil {
		if errors.Is(err, repository.ErrDuplicateEntry) {
			respondConflict(w, r, "Item type with this name already exists")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	created, err := h.repo.GetByID(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionItemTypeCreate,
			ResourceType: logger.ResourceItemType,
			ResourceID:   &created.ID,
			ResourceName: created.Name,
			Details: map[string]any{
				"icon":                    created.Icon,
				"color":                   created.Color,
				"hierarchy_level":         created.HierarchyLevel,
				"configuration_set_ids":   created.ConfigurationSetIDs,
				"configuration_set_names": created.ConfigurationSetNames,
			},
			Success: true,
		})
	}

	if !localizeObjectResponse(w, r, h.translations, "item_type", created) {
		return
	}
	respondJSONCreated(w, created)
}

func (h *ItemTypeHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Load the current row for the not-found check and audit diff.
	old, err := h.repo.GetByID(id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "item_type")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	it, fields, ok := decodeJSONWithFields[models.ItemType](w, r)
	if !ok {
		return
	}
	if _, provided := fields["is_default"]; !provided {
		it.IsDefault = old.IsDefault
	}
	sanitizeItemTypeRequest(&it)

	if strings.TrimSpace(it.Name) == "" {
		respondValidationError(w, r, "Item type name is required")
		return
	}

	nameExists, err := h.repo.NameExists(it.Name, id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if nameExists {
		respondConflict(w, r, "Item type with this name already exists")
		return
	}

	// Validate supplied configuration sets before mutating anything.
	if !h.configurationSetsExist(w, r, it.ConfigurationSetIDs) {
		return
	}

	if err := h.repo.Update(id, &it, it.ConfigurationSetIDs); err != nil {
		if errors.Is(err, repository.ErrDuplicateEntry) {
			respondConflict(w, r, "Item type with this name already exists")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	updated, err := h.repo.GetByID(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		details := make(map[string]any)
		if old.Name != updated.Name {
			details["name_changed"] = map[string]any{"old": old.Name, "new": updated.Name}
		}
		if old.Icon != updated.Icon {
			details["icon_changed"] = map[string]any{"old": old.Icon, "new": updated.Icon}
		}
		if old.Color != updated.Color {
			details["color_changed"] = map[string]any{"old": old.Color, "new": updated.Color}
		}
		if old.HierarchyLevel != updated.HierarchyLevel {
			details["hierarchy_level_changed"] = map[string]any{"old": old.HierarchyLevel, "new": updated.HierarchyLevel}
		}
		if old.SortOrder != updated.SortOrder {
			details["sort_order_changed"] = map[string]any{"old": old.SortOrder, "new": updated.SortOrder}
		}
		if len(updated.ConfigurationSetIDs) > 0 {
			details["configuration_sets"] = updated.ConfigurationSetNames
		}

		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionItemTypeUpdate,
			ResourceType: logger.ResourceItemType,
			ResourceID:   &updated.ID,
			ResourceName: updated.Name,
			Details:      details,
			Success:      true,
		})
	}

	if !localizeObjectResponse(w, r, h.translations, "item_type", updated) {
		return
	}
	respondJSONOK(w, updated)
}

func (h *ItemTypeHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	existing, err := h.repo.GetByID(id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "item_type")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Guard against deleting an item type that is still in use: existing items
	// must be re-typed (or the type kept) before the type can be removed.
	// Without this, the items.item_type_id FK is ON DELETE SET NULL, so the
	// items would silently become untyped and lose type-scoped
	// workflow/screen/approval behavior.
	itemCount, err := repository.NewItemRepository(h.db).CountByField("item_type_id", id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if itemCount > 0 {
		respondConflict(w, r, fmt.Sprintf("Cannot delete item type: it is used by %d item(s). Change the type of those items first.", itemCount))
		return
	}

	if err := h.repo.Delete(id); err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionItemTypeDelete,
			ResourceType: logger.ResourceItemType,
			ResourceID:   &id,
			ResourceName: existing.Name,
			Details: map[string]any{
				"icon":  existing.Icon,
				"color": existing.Color,
			},
			Success: true,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

// configurationSetsExist validates that every supplied configuration-set id
// exists, writing a 400 and returning false on the first miss (or a lookup
// error). An empty slice passes.
func (h *ItemTypeHandler) configurationSetsExist(w http.ResponseWriter, r *http.Request, ids []int) bool {
	for _, csID := range ids {
		exists, err := h.repo.ConfigurationSetExists(csID)
		if err != nil || !exists {
			respondValidationError(w, r, fmt.Sprintf("Configuration set %d not found", csID))
			return false
		}
	}
	return true
}
