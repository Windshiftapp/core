package handlers

import (
	"errors"
	"net/http"

	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/utils"
)

type LinkTypeHandler struct {
	db   database.Database // retained for audit logging via logAudit(h.db, ...)
	repo *repository.LinkTypeRepository
}

func NewLinkTypeHandler(db database.Database) *LinkTypeHandler {
	return &LinkTypeHandler{
		db:   db,
		repo: repository.NewLinkTypeRepository(db),
	}
}

func (h *LinkTypeHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	// Check if we should include inactive link types (admin only)
	includeInactive := r.URL.Query().Get("include_inactive") == "true"

	linkTypes, err := h.repo.List(includeInactive)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, linkTypes)
}

func (h *LinkTypeHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	lt, err := h.repo.GetByID(id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "link_type")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, lt)
}

func (h *LinkTypeHandler) Create(w http.ResponseWriter, r *http.Request) {
	lt, ok := decodeJSON[models.LinkType](w, r)
	if !ok {
		return
	}

	// Validate required fields
	if lt.Name == "" || lt.ForwardLabel == "" || lt.ReverseLabel == "" {
		respondValidationError(w, r, "Name, forward_label, and reverse_label are required")
		return
	}

	// Sanitize user input
	lt.Name = utils.SanitizeTitle(lt.Name)
	lt.Description = utils.SanitizeCommentContent(lt.Description)
	lt.ForwardLabel = utils.SanitizeTitle(lt.ForwardLabel)
	lt.ReverseLabel = utils.SanitizeTitle(lt.ReverseLabel)

	// Set defaults
	if lt.Color == "" {
		lt.Color = "#6b7280"
	}

	id, now, err := h.repo.Create(&lt)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	lt.ID = id
	lt.IsSystem = false
	lt.Active = true
	lt.CreatedAt = now
	lt.UpdatedAt = now

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logAudit(h.db, r, currentUser, logger.ActionLinkTypeCreate, logger.ResourceLinkType, &id, lt.Name)
	}

	respondJSONCreated(w, lt)
}

func (h *LinkTypeHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	lt, ok := decodeJSON[models.LinkType](w, r)
	if !ok {
		return
	}

	// Validate required fields
	if lt.Name == "" || lt.ForwardLabel == "" || lt.ReverseLabel == "" {
		respondValidationError(w, r, "Name, forward_label, and reverse_label are required")
		return
	}

	lt.Name = utils.SanitizeTitle(lt.Name)
	lt.Description = utils.SanitizeCommentContent(lt.Description)
	lt.ForwardLabel = utils.SanitizeTitle(lt.ForwardLabel)
	lt.ReverseLabel = utils.SanitizeTitle(lt.ReverseLabel)

	now, err := h.repo.Update(id, &lt)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	lt.ID = id
	lt.UpdatedAt = now

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logAudit(h.db, r, currentUser, logger.ActionLinkTypeUpdate, logger.ResourceLinkType, &id, lt.Name)
	}

	respondJSONOK(w, lt)
}

func (h *LinkTypeHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	// Check if it's a system link type (can't be deleted)
	existing, err := h.repo.GetByID(id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "link_type")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if existing.IsSystem {
		respondForbidden(w, r)
		return
	}

	if err := h.repo.Delete(id); err != nil {
		respondInternalError(w, r, err)
		return
	}

	currentUser := utils.GetCurrentUser(r)
	if currentUser != nil {
		logAudit(h.db, r, currentUser, logger.ActionLinkTypeDelete, logger.ResourceLinkType, &id, "")
	}

	w.WriteHeader(http.StatusNoContent)
}
