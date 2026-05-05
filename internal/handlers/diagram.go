package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/utils"
)

type DiagramHandler struct {
	repo              *repository.DiagramRepository
	itemRepo          *repository.ItemRepository
	permissionService *services.PermissionService
}

func NewDiagramHandler(
	repo *repository.DiagramRepository,
	itemRepo *repository.ItemRepository,
	permissionService *services.PermissionService,
) *DiagramHandler {
	return &DiagramHandler{
		repo:              repo,
		itemRepo:          itemRepo,
		permissionService: permissionService,
	}
}

// checkItemEditPermission checks if the current user can edit the given item
func (h *DiagramHandler) checkItemEditPermission(w http.ResponseWriter, r *http.Request, itemID int) bool {
	return CheckItemPermission(w, r, h.itemRepo, h.permissionService, itemID, models.PermissionItemEdit)
}

// decodeDiagramRequest decodes the JSON body, sanitizes the name, and validates
// that both name and diagram_data are non-empty. It writes an error response and
// returns ok=false on failure.
func decodeDiagramRequest(w http.ResponseWriter, r *http.Request) (name, diagramData string, ok bool) {
	var req struct {
		Name        string `json:"name"`
		DiagramData string `json:"diagram_data"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return "", "", false
	}

	req.Name = utils.SanitizeName(req.Name)

	if req.Name == "" {
		respondValidationError(w, r, "Diagram name is required")
		return "", "", false
	}

	if req.DiagramData == "" {
		respondValidationError(w, r, "Diagram data is required")
		return "", "", false
	}

	return req.Name, req.DiagramData, true
}

// Create creates a new diagram for an item
func (h *DiagramHandler) Create(w http.ResponseWriter, r *http.Request) {
	itemID, ok := requireIDParam(w, r, "itemId")
	if !ok {
		return
	}

	if !h.checkItemEditPermission(w, r, itemID) {
		return
	}

	name, diagramData, ok := decodeDiagramRequest(w, r)
	if !ok {
		return
	}

	// Get current user from context
	var createdBy *int
	if user := utils.GetCurrentUser(r); user != nil {
		createdBy = &user.ID
	}

	id, now, err := h.repo.Create(itemID, name, diagramData, createdBy)
	if err != nil {
		slog.Error("failed to create diagram", slog.String("component", "diagrams"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	diagram := &models.ItemDiagram{
		ID:          int(id),
		ItemID:      itemID,
		Name:        name,
		DiagramData: diagramData,
		CreatedBy:   createdBy,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// Record history for diagram creation
	if createdBy != nil {
		newValue := fmt.Sprintf("diagram:%d:%s", id, name)
		if err := h.repo.RecordHistory(itemID, *createdBy, "diagram_created", nil, newValue); err != nil {
			slog.Warn("failed to record diagram creation history", slog.String("component", "diagrams"), slog.Any("error", err))
		}
	}

	respondJSONOK(w, diagram)
}

// GetByItem retrieves all diagrams for an item
func (h *DiagramHandler) GetByItem(w http.ResponseWriter, r *http.Request) {
	itemID, ok := requireIDParam(w, r, "itemId")
	if !ok {
		return
	}

	if !CheckItemPermission(w, r, h.itemRepo, h.permissionService, itemID, models.PermissionItemView) {
		return
	}

	diagrams, err := h.repo.ListByItem(itemID)
	if err != nil {
		slog.Error("failed to query diagrams", slog.String("component", "diagrams"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, diagrams)
}

// Get retrieves a specific diagram by ID
func (h *DiagramHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	d, err := h.repo.GetByID(id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "diagram")
		return
	}
	if err != nil {
		slog.Error("failed to query diagram", slog.String("component", "diagrams"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	if !CheckItemPermission(w, r, h.itemRepo, h.permissionService, d.ItemID, models.PermissionItemView) {
		return
	}

	respondJSONOK(w, d)
}

// Update updates an existing diagram
func (h *DiagramHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	name, diagramData, ok := decodeDiagramRequest(w, r)
	if !ok {
		return
	}

	// Get user from context for history tracking
	var userID *int
	if user := utils.GetCurrentUser(r); user != nil {
		userID = &user.ID
	}

	// Get old diagram name and item_id before updating
	oldName, itemID, err := h.repo.GetNameAndItemID(id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "diagram")
		return
	}
	if err != nil {
		slog.Error("failed to get diagram details", slog.String("component", "diagrams"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	if !h.checkItemEditPermission(w, r, itemID) {
		return
	}

	if err := h.repo.Update(id, name, diagramData, userID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "diagram")
			return
		}
		slog.Error("failed to update diagram", slog.String("component", "diagrams"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	// Record history for diagram update
	if userID != nil {
		// Track update - show old name if it changed, otherwise show current name
		var historyOldName *string
		if oldName != name {
			historyOldName = &oldName
		}
		newValue := fmt.Sprintf("diagram:%d:%s", id, name)
		if err := h.repo.RecordHistory(itemID, *userID, "diagram_updated", historyOldName, newValue); err != nil {
			slog.Warn("failed to record diagram update history", slog.String("component", "diagrams"), slog.Any("error", err))
		}
	}

	// Retrieve the updated diagram
	d, err := h.repo.GetByID(id)
	if err != nil {
		slog.Error("failed to retrieve updated diagram", slog.String("component", "diagrams"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, d)
}

// Delete deletes a diagram
func (h *DiagramHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Get user from context for history tracking
	var userID *int
	if user := utils.GetCurrentUser(r); user != nil {
		userID = &user.ID
	}

	// Get diagram details before deletion (for history tracking)
	diagramName, itemID, err := h.repo.GetNameAndItemID(id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "diagram")
		return
	}
	if err != nil {
		slog.Error("failed to get diagram details", slog.String("component", "diagrams"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	if !h.checkItemEditPermission(w, r, itemID) {
		return
	}

	// Record history before deletion
	if userID != nil {
		if err := h.repo.RecordHistory(itemID, *userID, "diagram_deleted", &diagramName, diagramName); err != nil {
			slog.Warn("failed to record diagram deletion history", slog.String("component", "diagrams"), slog.Any("error", err))
		}
	}

	if err := h.repo.Delete(id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "diagram")
			return
		}
		slog.Error("failed to delete diagram", slog.String("component", "diagrams"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Diagram %d deleted successfully", id),
	})
}
