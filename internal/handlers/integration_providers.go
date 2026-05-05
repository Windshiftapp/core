package handlers

import (
	"errors"
	"net/http"
	"time"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sso"

	"github.com/google/uuid"
)

// IntegrationProviderHandler handles admin CRUD for integration providers
type IntegrationProviderHandler struct {
	repo       *repository.IntegrationProviderRepository
	encryption *sso.SecretEncryption
}

// IntegrationProviderResponse represents a provider for API responses (without secrets)
type IntegrationProviderResponse struct {
	ID                   string                         `json:"id"`
	Slug                 string                         `json:"slug"`
	Name                 string                         `json:"name"`
	ProviderType         models.IntegrationProviderType `json:"provider_type"`
	Enabled              bool                           `json:"enabled"`
	OAuthClientID        string                         `json:"oauth_client_id,omitempty"`
	HasOAuthClientSecret bool                           `json:"has_oauth_client_secret"`
	ProviderConfig       string                         `json:"provider_config,omitempty"`
	CreatedAt            time.Time                      `json:"created_at"`
	UpdatedAt            time.Time                      `json:"updated_at"`
}

// NewIntegrationProviderHandler creates a new integration provider handler
func NewIntegrationProviderHandler(repo *repository.IntegrationProviderRepository, encryption *sso.SecretEncryption) *IntegrationProviderHandler {
	return &IntegrationProviderHandler{
		repo:       repo,
		encryption: encryption,
	}
}

// GetProviders returns all integration providers
func (h *IntegrationProviderHandler) GetProviders(w http.ResponseWriter, r *http.Request) {
	providers, err := h.repo.List()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	responses := make([]IntegrationProviderResponse, 0, len(providers))
	for _, p := range providers {
		responses = append(responses, providerToResponse(p))
	}
	respondJSONOK(w, responses)
}

// GetProvider returns a single integration provider
func (h *IntegrationProviderHandler) GetProvider(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		respondBadRequest(w, r, "Missing provider ID")
		return
	}

	p, err := h.repo.GetByID(id)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "integration_provider")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, providerToResponse(*p))
}

// CreateProvider creates a new integration provider
func (h *IntegrationProviderHandler) CreateProvider(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[models.IntegrationProviderRequest](w, r)
	if !ok {
		return
	}

	if req.Slug == "" || req.Name == "" || req.ProviderType == "" {
		respondValidationError(w, r, "Missing required fields: slug, name, provider_type")
		return
	}

	// Validate provider type
	validTypes := map[string]bool{
		string(models.IntegrationProviderNotion): true,
	}
	if !validTypes[req.ProviderType] {
		respondBadRequest(w, r, "Invalid provider type. Supported: notion")
		return
	}

	// Encrypt secret if provided
	var secretEnc string
	if req.OAuthClientSecret != "" {
		enc, err := h.encryption.Encrypt(req.OAuthClientSecret)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		secretEnc = enc
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	id := uuid.New().String()
	insert := repository.IntegrationProviderInsert{
		ID:                         id,
		Slug:                       req.Slug,
		Name:                       req.Name,
		ProviderType:               req.ProviderType,
		Enabled:                    enabled,
		OAuthClientID:              req.OAuthClientID,
		OAuthClientSecretEncrypted: secretEnc,
		ProviderConfig:             req.ProviderConfig,
	}
	if err := h.repo.Create(insert); err != nil {
		if errors.Is(err, repository.ErrDuplicateEntry) {
			respondConflict(w, r, "Provider with this slug already exists")
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
	respondJSONCreated(w, providerToResponse(*created))
}

// UpdateProvider updates an existing integration provider
func (h *IntegrationProviderHandler) UpdateProvider(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		respondBadRequest(w, r, "Missing provider ID")
		return
	}

	req, ok := decodeJSON[models.IntegrationProviderRequest](w, r)
	if !ok {
		return
	}

	update := repository.IntegrationProviderUpdate{}
	if req.Slug != "" {
		update.Slug = &req.Slug
	}
	if req.Name != "" {
		update.Name = &req.Name
	}
	if req.Enabled != nil {
		update.Enabled = req.Enabled
	}
	if req.OAuthClientID != "" {
		update.OAuthClientID = &req.OAuthClientID
	}
	if req.OAuthClientSecret != "" {
		enc, err := h.encryption.Encrypt(req.OAuthClientSecret)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		update.OAuthClientSecretEncrypted = &enc
	}
	if req.ProviderConfig != "" {
		update.ProviderConfig = &req.ProviderConfig
	}

	if err := h.repo.Update(id, update); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "integration_provider")
			return
		}
		if errors.Is(err, repository.ErrDuplicateEntry) {
			respondConflict(w, r, "Provider with this slug already exists")
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
	respondJSONOK(w, providerToResponse(*updated))
}

// DeleteProvider deletes an integration provider
func (h *IntegrationProviderHandler) DeleteProvider(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		respondBadRequest(w, r, "Missing provider ID")
		return
	}

	if err := h.repo.Delete(id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			respondNotFound(w, r, "integration_provider")
			return
		}
		respondInternalError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func providerToResponse(p repository.IntegrationProvider) IntegrationProviderResponse {
	return IntegrationProviderResponse{
		ID:                   p.ID,
		Slug:                 p.Slug,
		Name:                 p.Name,
		ProviderType:         p.ProviderType,
		Enabled:              p.Enabled,
		OAuthClientID:        p.OAuthClientID,
		HasOAuthClientSecret: p.HasOAuthClientSecret,
		ProviderConfig:       p.ProviderConfig,
		CreatedAt:            p.CreatedAt,
		UpdatedAt:            p.UpdatedAt,
	}
}
