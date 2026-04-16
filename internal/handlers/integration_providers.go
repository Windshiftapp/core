package handlers

import (
	"database/sql"
	"net/http"
	"time"

	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/sso"

	"github.com/google/uuid"
)

// IntegrationProviderHandler handles admin CRUD for integration providers
type IntegrationProviderHandler struct {
	db         database.Database
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
func NewIntegrationProviderHandler(db database.Database, encryption *sso.SecretEncryption) *IntegrationProviderHandler {
	return &IntegrationProviderHandler{
		db:         db,
		encryption: encryption,
	}
}

// GetProviders returns all integration providers
func (h *IntegrationProviderHandler) GetProviders(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(`
		SELECT id, slug, name, provider_type, enabled,
			oauth_client_id, oauth_client_secret_encrypted,
			provider_config, created_at, updated_at
		FROM integration_providers
		ORDER BY name
	`)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	providers := []IntegrationProviderResponse{}
	for rows.Next() {
		resp, err := h.scanProviderRow(rows)
		if err != nil {
			continue
		}
		providers = append(providers, resp)
	}

	respondJSONOK(w, providers)
}

// GetProvider returns a single integration provider
func (h *IntegrationProviderHandler) GetProvider(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		respondBadRequest(w, r, "Missing provider ID")
		return
	}

	resp, err := h.getProviderByID(id)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "integration_provider")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	respondJSONOK(w, resp)
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
		var err error
		secretEnc, err = h.encryption.Encrypt(req.OAuthClientSecret)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	id := uuid.New().String()
	_, err := h.db.Exec(`
		INSERT INTO integration_providers (
			id, slug, name, provider_type, enabled,
			oauth_client_id, oauth_client_secret_encrypted, provider_config
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, id, req.Slug, req.Name, req.ProviderType, enabled,
		nullString(req.OAuthClientID), nullString(secretEnc), nullString(req.ProviderConfig))
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			respondConflict(w, r, "Provider with this slug already exists")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	resp, err := h.getProviderByID(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONCreated(w, resp)
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

	// Check provider exists
	var existingID string
	err := h.db.QueryRow("SELECT id FROM integration_providers WHERE id = ?", id).Scan(&existingID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "integration_provider")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	// Build update
	var secretEnc *string
	if req.OAuthClientSecret != "" {
		enc, err := h.encryption.Encrypt(req.OAuthClientSecret)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		secretEnc = &enc
	}

	if req.Slug != "" {
		if _, err := h.db.Exec("UPDATE integration_providers SET slug = ? WHERE id = ?", req.Slug, id); err != nil {
			if database.IsUniqueConstraintError(err) {
				respondConflict(w, r, "Provider with this slug already exists")
				return
			}
			respondInternalError(w, r, err)
			return
		}
	}
	if req.Name != "" {
		if _, err := h.db.Exec("UPDATE integration_providers SET name = ? WHERE id = ?", req.Name, id); err != nil {
			respondInternalError(w, r, err)
			return
		}
	}
	if req.Enabled != nil {
		if _, err := h.db.Exec("UPDATE integration_providers SET enabled = ? WHERE id = ?", *req.Enabled, id); err != nil {
			respondInternalError(w, r, err)
			return
		}
	}
	if req.OAuthClientID != "" {
		if _, err := h.db.Exec("UPDATE integration_providers SET oauth_client_id = ? WHERE id = ?", req.OAuthClientID, id); err != nil {
			respondInternalError(w, r, err)
			return
		}
	}
	if secretEnc != nil {
		if _, err := h.db.Exec("UPDATE integration_providers SET oauth_client_secret_encrypted = ? WHERE id = ?", *secretEnc, id); err != nil {
			respondInternalError(w, r, err)
			return
		}
	}
	if req.ProviderConfig != "" {
		if _, err := h.db.Exec("UPDATE integration_providers SET provider_config = ? WHERE id = ?", req.ProviderConfig, id); err != nil {
			respondInternalError(w, r, err)
			return
		}
	}

	// Update timestamp
	_, _ = h.db.Exec("UPDATE integration_providers SET updated_at = CURRENT_TIMESTAMP WHERE id = ?", id)

	resp, err := h.getProviderByID(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, resp)
}

// DeleteProvider deletes an integration provider
func (h *IntegrationProviderHandler) DeleteProvider(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		respondBadRequest(w, r, "Missing provider ID")
		return
	}

	result, err := h.db.Exec("DELETE FROM integration_providers WHERE id = ?", id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		respondNotFound(w, r, "integration_provider")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Helper methods

func (h *IntegrationProviderHandler) getProviderByID(id string) (IntegrationProviderResponse, error) {
	row := h.db.QueryRow(`
		SELECT id, slug, name, provider_type, enabled,
			oauth_client_id, oauth_client_secret_encrypted,
			provider_config, created_at, updated_at
		FROM integration_providers WHERE id = ?
	`, id)
	return h.scanProviderSingleRow(row)
}

func (h *IntegrationProviderHandler) scanProviderRow(rows *sql.Rows) (IntegrationProviderResponse, error) {
	var resp IntegrationProviderResponse
	var clientID, secretEnc, config sql.NullString

	err := rows.Scan(
		&resp.ID, &resp.Slug, &resp.Name, &resp.ProviderType, &resp.Enabled,
		&clientID, &secretEnc,
		&config, &resp.CreatedAt, &resp.UpdatedAt,
	)
	if err != nil {
		return resp, err
	}

	if clientID.Valid {
		resp.OAuthClientID = clientID.String
	}
	resp.HasOAuthClientSecret = secretEnc.Valid && secretEnc.String != ""
	if config.Valid {
		resp.ProviderConfig = config.String
	}

	return resp, nil
}

func (h *IntegrationProviderHandler) scanProviderSingleRow(row *sql.Row) (IntegrationProviderResponse, error) {
	var resp IntegrationProviderResponse
	var clientID, secretEnc, config sql.NullString

	err := row.Scan(
		&resp.ID, &resp.Slug, &resp.Name, &resp.ProviderType, &resp.Enabled,
		&clientID, &secretEnc,
		&config, &resp.CreatedAt, &resp.UpdatedAt,
	)
	if err != nil {
		return resp, err
	}

	if clientID.Valid {
		resp.OAuthClientID = clientID.String
	}
	resp.HasOAuthClientSecret = secretEnc.Valid && secretEnc.String != ""
	if config.Valid {
		resp.ProviderConfig = config.String
	}

	return resp, nil
}
