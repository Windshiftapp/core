package handlers

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"windshift/internal/database"
	"windshift/internal/integrations/notion"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/sso"

	"github.com/google/uuid"
)

// IntegrationItemLinksHandler handles item integration link endpoints
type IntegrationItemLinksHandler struct {
	db                database.Database
	encryption        *sso.SecretEncryption
	permissionService *services.PermissionService
}

// ItemIntegrationLinkResponse represents an integration link for API responses
type ItemIntegrationLinkResponse struct {
	ID                    string    `json:"id"`
	ItemID                string    `json:"item_id"`
	IntegrationProviderID string    `json:"integration_provider_id"`
	ExternalID            string    `json:"external_id"`
	ExternalURL           string    `json:"external_url"`
	Title                 string    `json:"title"`
	Icon                  string    `json:"icon,omitempty"`
	LinkType              string    `json:"link_type"`
	LinkMetadata          string    `json:"link_metadata,omitempty"`
	LinkedBy              string    `json:"linked_by"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
	ProviderName          string    `json:"provider_name,omitempty"`
	ProviderType          string    `json:"provider_type,omitempty"`
}

// CreateItemIntegrationLinkRequest represents a request to link an external page
type CreateItemIntegrationLinkRequest struct {
	ProviderID  string `json:"provider_id"`
	ExternalID  string `json:"external_id"`
	ExternalURL string `json:"external_url"`
	Title       string `json:"title"`
	Icon        string `json:"icon,omitempty"`
	LinkType    string `json:"link_type"`
}

// NewIntegrationItemLinksHandler creates a new integration item links handler
func NewIntegrationItemLinksHandler(db database.Database, encryption *sso.SecretEncryption, permissionService *services.PermissionService) *IntegrationItemLinksHandler {
	return &IntegrationItemLinksHandler{
		db:                db,
		encryption:        encryption,
		permissionService: permissionService,
	}
}

// GetItemLinks returns all integration links for an item
func (h *IntegrationItemLinksHandler) GetItemLinks(w http.ResponseWriter, r *http.Request) {
	itemID, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}

	if !CheckItemPermission(w, r, h.db, h.permissionService, itemID, models.PermissionItemView) {
		return
	}

	rows, err := h.db.Query(`
		SELECT
			iil.id, iil.item_id, iil.integration_provider_id,
			iil.external_id, iil.external_url, iil.title, iil.icon,
			iil.link_type, iil.link_metadata, iil.linked_by,
			iil.created_at, iil.updated_at,
			ip.name AS provider_name, ip.provider_type
		FROM item_integration_links iil
		JOIN integration_providers ip ON ip.id = iil.integration_provider_id
		WHERE iil.item_id = ?
		ORDER BY iil.created_at DESC
	`, itemID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	links := []ItemIntegrationLinkResponse{}
	for rows.Next() {
		var link ItemIntegrationLinkResponse
		var icon, metadata sql.NullString

		err := rows.Scan(
			&link.ID, &link.ItemID, &link.IntegrationProviderID,
			&link.ExternalID, &link.ExternalURL, &link.Title, &icon,
			&link.LinkType, &metadata, &link.LinkedBy,
			&link.CreatedAt, &link.UpdatedAt,
			&link.ProviderName, &link.ProviderType,
		)
		if err != nil {
			slog.Error("failed to scan integration link", slog.String("component", "integration_links"), slog.Any("error", err))
			continue
		}
		if icon.Valid {
			link.Icon = icon.String
		}
		if metadata.Valid {
			link.LinkMetadata = metadata.String
		}
		links = append(links, link)
	}

	respondJSONOK(w, links)
}

// requireItemEditAuth parses the item ID from the URL, enforces item edit
// permission, and pulls the current user from the auth context. Returns
// (0, nil, false) after writing the appropriate HTTP response on any failure.
func (h *IntegrationItemLinksHandler) requireItemEditAuth(w http.ResponseWriter, r *http.Request) (int, *models.User, bool) {
	itemID, ok := requireIDParam(w, r, "id")
	if !ok {
		return 0, nil, false
	}
	if !CheckItemPermission(w, r, h.db, h.permissionService, itemID, models.PermissionItemEdit) {
		return 0, nil, false
	}
	user, ok := RequireAuth(w, r)
	if !ok {
		return 0, nil, false
	}
	return itemID, user, true
}

// CreateItemLink creates a new integration link for an item
func (h *IntegrationItemLinksHandler) CreateItemLink(w http.ResponseWriter, r *http.Request) {
	itemID, user, ok := h.requireItemEditAuth(w, r)
	if !ok {
		return
	}

	req, ok := decodeJSON[CreateItemIntegrationLinkRequest](w, r)
	if !ok {
		return
	}

	if req.ProviderID == "" || req.ExternalID == "" || req.Title == "" || req.LinkType == "" {
		respondValidationError(w, r, "Missing required fields: provider_id, external_id, title, link_type")
		return
	}

	// Verify provider exists and is enabled
	var providerExists bool
	err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM integration_providers WHERE id = ? AND enabled = true)", req.ProviderID).Scan(&providerExists)
	if err != nil || !providerExists {
		respondBadRequest(w, r, "Integration provider not found or disabled")
		return
	}

	id := uuid.New().String()
	_, err = h.db.Exec(`
		INSERT INTO item_integration_links (
			id, item_id, integration_provider_id,
			external_id, external_url, title, icon,
			link_type, linked_by
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, itemID, req.ProviderID,
		req.ExternalID, req.ExternalURL, req.Title, nullString(req.Icon),
		req.LinkType, fmt.Sprintf("%d", user.ID))
	if err != nil {
		if database.IsUniqueConstraintError(err) {
			respondConflict(w, r, "This page is already linked to this item")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	// Fetch the created link with joins
	link, err := h.getLinkByID(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONCreated(w, link)
}

// DeleteItemLink removes an integration link
func (h *IntegrationItemLinksHandler) DeleteItemLink(w http.ResponseWriter, r *http.Request) {
	linkID := r.PathValue("linkId")
	if linkID == "" {
		respondBadRequest(w, r, "Missing link ID")
		return
	}

	// Get the link to find the item
	var itemID int
	err := h.db.QueryRow("SELECT item_id FROM item_integration_links WHERE id = ?", linkID).Scan(&itemID)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "integration_link")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	if !CheckItemPermission(w, r, h.db, h.permissionService, itemID, models.PermissionItemEdit) {
		return
	}

	_, err = h.db.Exec("DELETE FROM item_integration_links WHERE id = ?", linkID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// RefreshItemLink refreshes the title and icon of a linked page from the provider
func (h *IntegrationItemLinksHandler) RefreshItemLink(w http.ResponseWriter, r *http.Request) {
	linkID := r.PathValue("linkId")
	if linkID == "" {
		respondBadRequest(w, r, "Missing link ID")
		return
	}

	// Get link details
	var itemID int
	var externalID, providerID string
	var providerType models.IntegrationProviderType
	err := h.db.QueryRow(`
		SELECT iil.item_id, iil.external_id, iil.integration_provider_id, ip.provider_type
		FROM item_integration_links iil
		JOIN integration_providers ip ON ip.id = iil.integration_provider_id
		WHERE iil.id = ?
	`, linkID).Scan(&itemID, &externalID, &providerID, &providerType)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "integration_link")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	if !CheckItemPermission(w, r, h.db, h.permissionService, itemID, models.PermissionItemView) {
		return
	}

	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	// Get user's token for this provider
	accessToken, err := h.getUserAccessToken(user.ID, providerID)
	if err != nil {
		respondBadRequest(w, r, "Not connected to this integration provider")
		return
	}

	// Fetch page info based on provider type
	switch providerType {
	case models.IntegrationProviderNotion:
		page, err := notion.GetPage(accessToken, externalID)
		if err != nil {
			slog.Error("failed to refresh Notion page", slog.String("component", "integration_links"), slog.Any("error", err))
			respondBadRequest(w, r, "Failed to fetch page from Notion")
			return
		}
		_, err = h.db.Exec(`
			UPDATE item_integration_links
			SET title = ?, icon = ?, external_url = ?, updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, page.Title, nullString(page.Icon), page.URL, linkID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
	default:
		respondBadRequest(w, r, "Refresh not supported for this provider type")
		return
	}

	link, err := h.getLinkByID(linkID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, link)
}

// SearchPages searches for pages in the integration provider
func (h *IntegrationItemLinksHandler) SearchPages(w http.ResponseWriter, r *http.Request) {
	_, user, ok := h.requireItemEditAuth(w, r)
	if !ok {
		return
	}

	query := r.URL.Query().Get("q")
	providerID := r.URL.Query().Get("provider_id")

	if providerID == "" {
		respondValidationError(w, r, "Missing required parameter: provider_id")
		return
	}

	// Get provider type
	var providerType models.IntegrationProviderType
	err := h.db.QueryRow("SELECT provider_type FROM integration_providers WHERE id = ? AND enabled = true", providerID).Scan(&providerType)
	if err != nil {
		if err == sql.ErrNoRows {
			respondNotFound(w, r, "integration_provider")
		} else {
			respondInternalError(w, r, err)
		}
		return
	}

	// Get user's token
	accessToken, err := h.getUserAccessToken(user.ID, providerID)
	if err != nil {
		respondBadRequest(w, r, "Not connected to this integration provider. Please connect your account first.")
		return
	}

	// Search based on provider type
	switch providerType {
	case models.IntegrationProviderNotion:
		pages, err := notion.SearchPages(accessToken, query)
		if err != nil {
			slog.Error("failed to search Notion pages", slog.String("component", "integration_links"), slog.Any("error", err))
			respondBadRequest(w, r, "Failed to search pages in Notion")
			return
		}

		// Convert to response format
		type SearchResult struct {
			ExternalID  string `json:"external_id"`
			ExternalURL string `json:"external_url"`
			Title       string `json:"title"`
			Icon        string `json:"icon,omitempty"`
			PageType    string `json:"page_type"`
		}

		results := make([]SearchResult, 0, len(pages))
		for _, p := range pages {
			results = append(results, SearchResult{
				ExternalID:  p.ID,
				ExternalURL: p.URL,
				Title:       p.Title,
				Icon:        p.Icon,
				PageType:    p.PageType,
			})
		}

		respondJSONOK(w, results)
	default:
		respondBadRequest(w, r, "Search not supported for this provider type")
	}
}

// Helper methods

func (h *IntegrationItemLinksHandler) getLinkByID(id string) (ItemIntegrationLinkResponse, error) {
	var link ItemIntegrationLinkResponse
	var icon, metadata sql.NullString

	err := h.db.QueryRow(`
		SELECT
			iil.id, iil.item_id, iil.integration_provider_id,
			iil.external_id, iil.external_url, iil.title, iil.icon,
			iil.link_type, iil.link_metadata, iil.linked_by,
			iil.created_at, iil.updated_at,
			ip.name AS provider_name, ip.provider_type
		FROM item_integration_links iil
		JOIN integration_providers ip ON ip.id = iil.integration_provider_id
		WHERE iil.id = ?
	`, id).Scan(
		&link.ID, &link.ItemID, &link.IntegrationProviderID,
		&link.ExternalID, &link.ExternalURL, &link.Title, &icon,
		&link.LinkType, &metadata, &link.LinkedBy,
		&link.CreatedAt, &link.UpdatedAt,
		&link.ProviderName, &link.ProviderType,
	)
	if err != nil {
		return link, err
	}
	if icon.Valid {
		link.Icon = icon.String
	}
	if metadata.Valid {
		link.LinkMetadata = metadata.String
	}
	return link, nil
}

func (h *IntegrationItemLinksHandler) getUserAccessToken(userID int, providerID string) (string, error) {
	var encToken string
	err := h.db.QueryRow(`
		SELECT oauth_access_token_encrypted
		FROM user_integration_tokens
		WHERE user_id = ? AND integration_provider_id = ?
	`, fmt.Sprintf("%d", userID), providerID).Scan(&encToken)
	if err != nil {
		return "", err
	}

	token, err := h.encryption.Decrypt(encToken)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt token: %w", err)
	}

	return token, nil
}
