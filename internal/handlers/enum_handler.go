package handlers

import (
	"encoding/json"
	"net/http"

	"windshift/internal/services"
)

// EnumHandler provides HTTP handlers for generic enum CRUD operations
type EnumHandler struct {
	service   *services.EnumService
	newEntity func() interface{} // Factory function to create new entity
}

// NewEnumHandler creates a new enum handler
func NewEnumHandler(service *services.EnumService, newEntity func() interface{}) *EnumHandler {
	return &EnumHandler{
		service:   service,
		newEntity: newEntity,
	}
}

// GetAll handles GET requests to list all entities
func (h *EnumHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	entities, err := h.service.GetAll()
	if err != nil {
		handleServiceError(w, r, err)
		return
	}
	respondJSONOK(w, entities)
}

// Get handles GET requests for a single entity by ID
func (h *EnumHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	entity, err := h.service.GetByID(id)
	if err != nil {
		handleServiceError(w, r, err)
		return
	}
	respondJSONOK(w, entity)
}

// Create handles POST requests to create a new entity
func (h *EnumHandler) Create(w http.ResponseWriter, r *http.Request) {
	entity := h.newEntity()
	if err := json.NewDecoder(r.Body).Decode(entity); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	created, err := h.service.Create(entity, r)
	if err != nil {
		handleServiceError(w, r, err)
		return
	}
	respondJSONCreated(w, created)
}

// Update handles PUT requests to update an existing entity
func (h *EnumHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	entity := h.newEntity()
	if err := json.NewDecoder(r.Body).Decode(entity); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	updated, err := h.service.Update(id, entity, r)
	if err != nil {
		handleServiceError(w, r, err)
		return
	}
	respondJSONOK(w, updated)
}

// Delete handles DELETE requests to delete an entity
func (h *EnumHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.service.Delete(id, r); err != nil {
		handleServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleServiceError converts service errors to HTTP responses
func handleServiceError(w http.ResponseWriter, r *http.Request, err error) {
	if se, ok := err.(*services.ServiceError); ok {
		switch se.StatusCode {
		case 400:
			respondBadRequest(w, r, se.Message)
		case 404:
			respondNotFound(w, r, se.Message)
		case 409:
			respondConflict(w, r, se.Message)
		default:
			respondBadRequest(w, r, se.Message)
		}
		return
	}
	respondInternalError(w, r, err)
}
