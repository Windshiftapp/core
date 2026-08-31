package handlers

import (
	"errors"
	"net/http"

	"windshift/internal/models"
	"windshift/internal/objecttranslation"
	"windshift/internal/sanitize"
	"windshift/internal/services"
)

// sanitizeEnumFields scrubs the shared Name/Color/Description shape of
// the enum entities. Name + Description render in the settings tables
// and pickers; Color is a hex code (identifier-shaped). Pass nil for
// fields the entity doesn't have.
func sanitizeEnumFields(name, color, description *string) {
	sanitize.ApplyAll(
		sanitize.Pair{Target: name, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: color, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: description, Policy: sanitize.PlainTextField},
	)
}

// sanitizeEnumEntity dispatches to the entity types this generic
// handler decodes (see the NewEnumHandler call sites in server.go).
func sanitizeEnumEntity(entity any) {
	switch e := entity.(type) {
	case *models.HierarchyLevel:
		sanitizeEnumFields(&e.Name, nil, &e.Description)
	case *models.StatusCategory:
		sanitizeEnumFields(&e.Name, &e.Color, &e.Description)
	case *models.Status:
		sanitizeEnumFields(&e.Name, nil, &e.Description)
	case *models.MilestoneCategory:
		sanitizeEnumFields(&e.Name, &e.Color, &e.Description)
	case *models.ChannelCategory:
		sanitizeEnumFields(&e.Name, &e.Color, &e.Description)
	case *models.CollectionCategory:
		sanitizeEnumFields(&e.Name, &e.Color, &e.Description)
	case *models.IterationType:
		sanitizeEnumFields(&e.Name, &e.Color, &e.Description)
	case *models.ContactRole:
		sanitizeEnumFields(&e.Name, nil, &e.Description)
	}
}

// EnumHandler provides HTTP handlers for generic enum CRUD operations
type EnumHandler struct {
	service            *services.EnumService
	newEntity          func() any // Factory function to create new entity
	permissionService  *services.PermissionService
	mutationPermission string
	translations       *objecttranslation.Service
	translationType    string
}

func (h *EnumHandler) WithObjectTranslations(service *objecttranslation.Service, objectType string) *EnumHandler {
	h.translations = service
	h.translationType = objectType
	return h
}

// WithGlobalMutationPermission adds a handler-level authorization boundary for
// global catalogs. Route middleware should still apply the same permission;
// this guard protects direct handler wiring and future routes from bypassing it.
func (h *EnumHandler) WithGlobalMutationPermission(permissionService *services.PermissionService, permission string) *EnumHandler {
	h.permissionService = permissionService
	h.mutationPermission = permission
	return h
}

func (h *EnumHandler) authorizeMutation(w http.ResponseWriter, r *http.Request) bool {
	if h.mutationPermission == "" {
		return true
	}
	user, ok := RequireAuth(w, r)
	if !ok {
		return false
	}
	if h.permissionService == nil {
		respondInternalError(w, r, errors.New("enum mutation permission service is not configured"))
		return false
	}
	allowed, err := h.permissionService.HasGlobalPermissionContext(r.Context(), user.ID, h.mutationPermission)
	if err != nil {
		respondInternalError(w, r, err)
		return false
	}
	if !allowed {
		respondForbidden(w, r)
		return false
	}
	return true
}

// NewEnumHandler creates a new enum handler
func NewEnumHandler(service *services.EnumService, newEntity func() any) *EnumHandler {
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
	if h.translationType != "" && !localizeObjectResponse(w, r, h.translations, h.translationType, entities) {
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
	if h.translationType != "" && !localizeObjectResponse(w, r, h.translations, h.translationType, entity) {
		return
	}
	respondJSONOK(w, entity)
}

// Create handles POST requests to create a new entity
func (h *EnumHandler) Create(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeMutation(w, r) {
		return
	}
	entity := h.newEntity()
	if err := newJSONDecoder(w, r).Decode(entity); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}
	sanitizeEnumEntity(entity)

	created, err := h.service.Create(entity, r)
	if err != nil {
		handleServiceError(w, r, err)
		return
	}
	if h.translationType != "" && !localizeObjectResponse(w, r, h.translations, h.translationType, created) {
		return
	}
	respondJSONCreated(w, created)
}

// Update handles PUT requests to update an existing entity
func (h *EnumHandler) Update(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeMutation(w, r) {
		return
	}
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	entity := h.newEntity()
	if err := newJSONDecoder(w, r).Decode(entity); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}
	sanitizeEnumEntity(entity)

	updated, err := h.service.Update(id, entity, r)
	if err != nil {
		handleServiceError(w, r, err)
		return
	}
	if h.translationType != "" && !localizeObjectResponse(w, r, h.translations, h.translationType, updated) {
		return
	}
	respondJSONOK(w, updated)
}

// Delete handles DELETE requests to delete an entity
func (h *EnumHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeMutation(w, r) {
		return
	}
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
