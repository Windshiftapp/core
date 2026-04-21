package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/utils"
)

type TimeProjectCategoryHandler struct {
	db   database.Database
	repo *repository.TimeProjectCategoryRepository
}

func NewTimeProjectCategoryHandler(db database.Database) *TimeProjectCategoryHandler {
	return &TimeProjectCategoryHandler{
		db:   db,
		repo: repository.NewTimeProjectCategoryRepository(db),
	}
}

// GetCategories retrieves all time project categories
func (h *TimeProjectCategoryHandler) GetCategories(w http.ResponseWriter, r *http.Request) {
	categories, err := h.repo.List()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, categories)
}

// GetCategory retrieves a single time project category by ID
func (h *TimeProjectCategoryHandler) GetCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	c, err := h.repo.FindByID(id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "category")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, c)
}

// CreateCategory creates a new time project category
func (h *TimeProjectCategoryHandler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	c, ok := decodeJSON[models.TimeProjectCategory](w, r)
	if !ok {
		return
	}

	if c.Name == "" {
		respondValidationError(w, r, "Category name is required")
		return
	}

	if err := h.repo.Create(&c); err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		categoryID := c.ID
		logAudit(h.db, r, currentUser, logger.ActionTimeCategoryCreate, logger.ResourceTimeCategory, &categoryID, c.Name)
	}

	respondJSONCreated(w, c)
}

// UpdateCategory updates an existing time project category
func (h *TimeProjectCategoryHandler) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	c, ok := decodeJSON[models.TimeProjectCategory](w, r)
	if !ok {
		return
	}

	if c.Name == "" {
		respondValidationError(w, r, "Category name is required")
		return
	}

	exists, err := h.repo.Exists(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !exists {
		respondNotFound(w, r, "category")
		return
	}

	if err := h.repo.Update(id, &c); err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logAudit(h.db, r, currentUser, logger.ActionTimeCategoryUpdate, logger.ResourceTimeCategory, &id, c.Name)
	}

	respondJSONOK(w, c)
}

// DeleteCategory deletes a time project category
func (h *TimeProjectCategoryHandler) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	projectCount, err := h.repo.CountProjectsUsing(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if projectCount > 0 {
		respondConflict(w, r, "Cannot delete category: it is used by "+strconv.Itoa(projectCount)+" project(s)")
		return
	}

	rowsAffected, err := h.repo.Delete(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if rowsAffected == 0 {
		respondNotFound(w, r, "category")
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logAudit(h.db, r, currentUser, logger.ActionTimeCategoryDelete, logger.ResourceTimeCategory, &id, "")
	}

	w.WriteHeader(http.StatusNoContent)
}

// ReorderCategories updates the display order of multiple categories
func (h *TimeProjectCategoryHandler) ReorderCategories(w http.ResponseWriter, r *http.Request) {
	var orderUpdates []struct {
		ID           int `json:"id"`
		DisplayOrder int `json:"display_order"`
	}

	if err := json.NewDecoder(r.Body).Decode(&orderUpdates); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	now := time.Now()
	for _, update := range orderUpdates {
		if err := h.repo.UpdateDisplayOrder(update.ID, update.DisplayOrder, now); err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	respondJSONOK(w, map[string]string{"message": "Category order updated successfully"})
}
