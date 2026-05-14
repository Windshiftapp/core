package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"windshift/internal/auth"
	"windshift/internal/database"
	"windshift/internal/middleware"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/restapi"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// Portal constants
const (
	defaultItemStatus = "open" // Default status for new portal submissions
)

// PortalHandler handles public portal submissions
type PortalHandler struct {
	db                   database.Database
	sessionManager       *auth.SessionManager
	portalSessionManager *auth.PortalSessionManager
	ipExtractor          *utils.IPExtractor
	portalService        *services.PortalService
	approvalService      *services.ApprovalService
	draftRepo            *repository.PortalDraftRepository
	attachmentPath       string
}

// SetApprovalService wires the approval service so portal customers can
// decide on approvals via /portal/{slug}/approvals/*. Optional — if unset,
// the approval routes return 503.
func (h *PortalHandler) SetApprovalService(s *services.ApprovalService) {
	h.approvalService = s
}

// getClientIP extracts the client IP with proxy validation
func (h *PortalHandler) getClientIP(r *http.Request) string {
	return h.ipExtractor.GetClientIP(r)
}

// getPortalCustomerID attempts to get the portal customer ID from either:
// 1. A direct portal customer session (magic link auth)
// 2. An internal user session with a linked portal customer (backward compatible)
// Returns the portal customer ID and an error if authentication fails
func (h *PortalHandler) getPortalCustomerID(ctx context.Context, r *http.Request) (*int, error) {
	clientIP := h.getClientIP(r)

	// First, try portal customer session (direct magic link auth)
	if h.portalSessionManager != nil {
		portalToken, err := h.portalSessionManager.GetPortalSessionFromRequest(r)
		if err == nil && portalToken != "" {
			portalSession, err := h.portalSessionManager.ValidatePortalSession(portalToken)
			if err == nil && portalSession != nil {
				slog.Debug("portal customer authenticated via portal session", slog.String("component", "portal"), slog.Int("portal_customer_id", portalSession.PortalCustomerID))
				return &portalSession.PortalCustomerID, nil
			}
		}
	}

	// Fall back to internal user session (backward compatible)
	sessionToken, err := h.sessionManager.GetSessionFromRequest(r)
	if err != nil {
		return nil, fmt.Errorf("authentication required")
	}

	session, err := h.sessionManager.ValidateSession(sessionToken, clientIP)
	if err != nil || session == nil {
		return nil, fmt.Errorf("invalid or expired session")
	}

	// Get portal customer ID from the user's internal session
	customerQuery := `SELECT id FROM portal_customers WHERE user_id = ?`
	var customerID int
	err = h.db.QueryRowContext(ctx, customerQuery, session.UserID).Scan(&customerID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("no portal customer found for this user")
	} else if err != nil {
		return nil, fmt.Errorf("failed to find portal customer: %w", err)
	}

	slog.Debug("portal customer authenticated via internal user session", slog.String("component", "portal"), slog.Int("portal_customer_id", customerID), slog.Int("user_id", session.UserID))
	return &customerID, nil
}

// getInternalUserGroupIDs returns the group IDs for an internal user
// Returns nil if not an internal user or if no groups found
func (h *PortalHandler) getInternalUserGroupIDs(ctx context.Context, r *http.Request) []int {
	clientIP := h.getClientIP(r)
	sessionToken, err := h.sessionManager.GetSessionFromRequest(r)
	if err != nil {
		return nil
	}

	session, err := h.sessionManager.ValidateSession(sessionToken, clientIP)
	if err != nil || session == nil {
		return nil
	}

	// Get user's group memberships
	rows, err := h.db.QueryContext(ctx, `SELECT group_id FROM group_members WHERE user_id = ?`, session.UserID)
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()

	var groupIDs []int
	for rows.Next() {
		var groupID int
		if err := rows.Scan(&groupID); err != nil {
			continue
		}
		groupIDs = append(groupIDs, groupID)
	}
	return groupIDs
}

// getAuthFromContext extracts auth info from context (set by RequirePortalAuth middleware)
// Returns (internalUserID, portalCustomerID) - one will be set, the other nil
func (h *PortalHandler) getAuthFromContext(r *http.Request) (userID, customerID *int) {
	ctx := r.Context()

	// Check for internal user (set by middleware)
	if session, ok := ctx.Value(middleware.ContextKeySession).(*auth.Session); ok && session != nil {
		return &session.UserID, nil
	}

	// Check for portal customer (set by middleware)
	if portalCustomerID, ok := ctx.Value(middleware.ContextKeyPortalCustomerID).(int); ok {
		return nil, &portalCustomerID
	}

	return nil, nil
}

// getPortalCustomerOrgID returns the customer organisation ID for a portal customer
// Returns nil if no organisation is associated
//
//nolint:misspell // organisation is used in database column names (customer_organisation_id)
func (h *PortalHandler) getPortalCustomerOrgID(ctx context.Context, portalCustomerID int) *int {
	var orgID sql.NullInt64
	err := h.db.QueryRowContext(ctx, `SELECT customer_organisation_id FROM portal_customers WHERE id = ?`, portalCustomerID).Scan(&orgID)
	if err != nil || !orgID.Valid {
		return nil
	}
	result := int(orgID.Int64)
	return &result
}

// portalVisibilityContext holds the computed visibility context used for filtering
// request types, asset reports, and other portal resources by visibility rules.
type portalVisibilityContext struct {
	userGroupIDs  []int
	customerOrgID *int
	isAdmin       bool
}

// getPortalVisibilityContext builds the visibility context needed for filtering
// portal resources. It resolves the user's group memberships, portal customer
// organisation ID, and admin status in a single call, eliminating the identical
// block of code previously duplicated in GetRequestTypes and GetAssetReports.
//
// Admin status is scoped to the specific portal channel: a user is an admin
// for this portal if they are a system admin OR a channel manager for this
// channel (via channel_managers). Before scoping to the channel, the check
// granted admin to anyone with channels.manage globally — a permission that
// does not exist — so the branch was effectively dead code beyond system
// admins, while intent was to allow channel managers to preview.
func (h *PortalHandler) getPortalVisibilityContext(ctx context.Context, r *http.Request, channelID int) portalVisibilityContext {
	vc := portalVisibilityContext{
		userGroupIDs: h.getInternalUserGroupIDs(ctx, r),
	}

	// Get portal customer org ID if authenticated as portal customer. Sessions
	// minted on a different portal are ignored so a cookie from portal A
	// cannot bias visibility filtering on portal B.
	if h.portalSessionManager != nil {
		portalToken, err := h.portalSessionManager.GetPortalSessionFromRequest(r)
		if err == nil && portalToken != "" {
			portalSession, err := h.portalSessionManager.ValidatePortalSession(portalToken)
			if err == nil && portalSession != nil && portalSession.ChannelID != nil && *portalSession.ChannelID == channelID {
				vc.customerOrgID = h.getPortalCustomerOrgID(ctx, portalSession.PortalCustomerID)
			}
		}
	}

	// Check if this is an admin viewing for customization (has internal session)
	if sessionToken, err := h.sessionManager.GetSessionFromRequest(r); err == nil {
		clientIP := h.getClientIP(r)
		if session, err := h.sessionManager.ValidateSession(sessionToken, clientIP); err == nil && session != nil {
			var isAdmin bool
			err := h.db.QueryRowContext(ctx, `
				SELECT EXISTS(
					SELECT 1 FROM user_global_permissions ugp
					JOIN permissions p ON ugp.permission_id = p.id
					WHERE ugp.user_id = ? AND p.permission_key = 'system.admin'
				) OR EXISTS(
					SELECT 1 FROM channel_managers cm
					WHERE cm.channel_id = ?
					AND ((cm.manager_type = 'user' AND cm.manager_id = ?)
						 OR (cm.manager_type = 'group' AND cm.manager_id IN (
							 SELECT group_id FROM group_members WHERE user_id = ?
						 )))
				)
			`, session.UserID, channelID, session.UserID, session.UserID).Scan(&isAdmin)
			if err == nil && isAdmin {
				vc.isAdmin = true
			}
		}
	}

	return vc
}

// getRequestTypeWithVisibility loads a request type and deserializes its visibility fields
func (h *PortalHandler) getRequestTypeWithVisibility(ctx context.Context, requestTypeID int) (*models.RequestType, error) {
	var rt models.RequestType
	var visibilityGroupIDs, visibilityOrgIDs sql.NullString
	err := h.db.QueryRowContext(ctx, `
		SELECT id, channel_id, name, description, item_type_id, icon, color, display_order, is_active,
		       visibility_group_ids, visibility_org_ids, title_template, created_at, updated_at
		FROM request_types WHERE id = ? AND is_active = true
	`, requestTypeID).Scan(
		&rt.ID, &rt.ChannelID, &rt.Name, &rt.Description, &rt.ItemTypeID, &rt.Icon, &rt.Color,
		&rt.DisplayOrder, &rt.IsActive, &visibilityGroupIDs, &visibilityOrgIDs, &rt.TitleTemplate,
		&rt.CreatedAt, &rt.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	applyRequestTypeVisibility(&rt, visibilityGroupIDs, visibilityOrgIDs)
	return &rt, nil
}

// unmarshalIntIDs decodes a JSON-encoded []int stored in a nullable string
// column, returning nil if the column is NULL, empty, or malformed.
func unmarshalIntIDs(n sql.NullString) []int {
	if !n.Valid || n.String == "" {
		return nil
	}
	var ids []int
	if err := json.Unmarshal([]byte(n.String), &ids); err != nil {
		return nil
	}
	return ids
}

// applyRequestTypeVisibility populates rt.VisibilityGroupIDs / VisibilityOrgIDs
// from the JSON-encoded nullable string columns that the schema uses for
// per-row visibility lists.
func applyRequestTypeVisibility(rt *models.RequestType, groups, orgs sql.NullString) {
	if ids := unmarshalIntIDs(groups); ids != nil {
		rt.VisibilityGroupIDs = ids
	}
	if ids := unmarshalIntIDs(orgs); ids != nil {
		rt.VisibilityOrgIDs = ids
	}
}

// NewPortalHandler creates a new portal handler
func NewPortalHandler(db database.Database, sessionManager *auth.SessionManager, portalSessionManager *auth.PortalSessionManager, ipExtractor *utils.IPExtractor, attachmentPath string) *PortalHandler {
	return &PortalHandler{
		db:                   db,
		sessionManager:       sessionManager,
		portalSessionManager: portalSessionManager,
		ipExtractor:          ipExtractor,
		portalService:        services.NewPortalService(db),
		draftRepo:            repository.NewPortalDraftRepository(db),
		attachmentPath:       attachmentPath,
	}
}

// findChannelByPortalSlug finds and validates a portal channel by slug.
func (h *PortalHandler) findChannelByPortalSlug(ctx context.Context, slug string) (*channelResult, error) {
	return findChannelBySlug(ctx, h.db, "portal", slug, func(c *models.ChannelConfig) string { return c.PortalSlug })
}

// grantChannelAccess grants a portal customer access to a channel if not already granted
func (h *PortalHandler) grantChannelAccess(ctx context.Context, customerID, channelID int) {
	var accessID int
	err := h.db.QueryRowContext(ctx, `SELECT id FROM portal_customer_channels WHERE portal_customer_id = ? AND channel_id = ?`, customerID, channelID).Scan(&accessID)

	if errors.Is(err, sql.ErrNoRows) {
		if _, err := h.db.ExecWriteContext(ctx, `
			INSERT INTO portal_customer_channels (portal_customer_id, channel_id, created_at)
			VALUES (?, ?, ?)
		`, customerID, channelID, time.Now()); err != nil {
			slog.Warn("failed to grant channel access to portal customer", slog.String("component", "portal"), slog.Int("customer_id", customerID), slog.Int("channel_id", channelID), slog.Any("error", err))
		}
	}
}

// customerHasChannelAccess returns true if the portal customer already has an
// access row for the given channel. Used by SubmitToPortal in manual-
// registration mode to refuse silent auto-grants for customers who have not
// been pre-provisioned.
func (h *PortalHandler) customerHasChannelAccess(ctx context.Context, customerID, channelID int) bool {
	var exists bool
	if err := h.db.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM portal_customer_channels WHERE portal_customer_id = ? AND channel_id = ?)
	`, customerID, channelID).Scan(&exists); err != nil {
		slog.Error("failed to check portal customer channel access", slog.String("component", "portal"), slog.Int("customer_id", customerID), slog.Int("channel_id", channelID), slog.Any("error", err))
		return false
	}
	return exists
}

// verifyPortalSessionBinding writes 401 and returns false if a portal session
// is in the request context but binds to a different channel than the one
// being accessed. Returns true when no portal session is present or when the
// session binds to the resolved channel. The cookie is shared across all
// portals on this domain, so without the binding check a customer signed in
// to portal A and browsing portal B would be treated as authenticated on B.
func (h *PortalHandler) verifyPortalSessionBinding(w http.ResponseWriter, r *http.Request, channelID int) bool {
	portalSession, ok := r.Context().Value(middleware.ContextKeyPortalSession).(*auth.PortalSession)
	if !ok || portalSession == nil {
		return true
	}
	if portalSession.ChannelID != nil && *portalSession.ChannelID == channelID {
		return true
	}
	var sessionChannel int
	if portalSession.ChannelID != nil {
		sessionChannel = *portalSession.ChannelID
	}
	slog.Warn("portal session channel binding mismatch",
		slog.String("component", "portal"),
		slog.Int("portal_customer_id", portalSession.PortalCustomerID),
		slog.Int("session_channel_id", sessionChannel),
		slog.Int("request_channel_id", channelID),
	)
	respondUnauthorized(w, r)
	return false
}

// resolvePortalBySlug resolves the path slug to a portal channel+config and
// returns a bounded context for downstream DB calls. It writes a 404 if the
// portal is not found. Callers always defer the returned cancel (a no-op on
// failure).
func (h *PortalHandler) resolvePortalBySlug(w http.ResponseWriter, r *http.Request) (context.Context, context.CancelFunc, models.Channel, models.ChannelConfig, bool) {
	slug := r.PathValue("slug")

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)

	portalResult, err := h.findChannelByPortalSlug(ctx, slug)
	if err != nil {
		cancel()
		respondNotFound(w, r, "portal")
		return nil, func() {}, models.Channel{}, models.ChannelConfig{}, false
	}
	if !h.verifyPortalSessionBinding(w, r, portalResult.channel.ID) {
		cancel()
		return nil, func() {}, models.Channel{}, models.ChannelConfig{}, false
	}
	return ctx, cancel, portalResult.channel, portalResult.config, true
}

// GetPortal returns the portal configuration for public display
func (h *PortalHandler) GetPortal(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, channel, config, ok := h.resolvePortalBySlug(w, r)
	if !ok {
		return
	}
	defer cancel()

	// Get workspace info (use first workspace for backward compatibility)
	var workspace models.Workspace
	var workspaceID int
	if len(config.PortalWorkspaceIDs) > 0 {
		workspaceID = config.PortalWorkspaceIDs[0]
	}

	if workspaceID > 0 {
		if err := h.db.QueryRowContext(ctx, `SELECT id, name, key FROM workspaces WHERE id = ?`, workspaceID).Scan(
			&workspace.ID, &workspace.Name, &workspace.Key,
		); err != nil {
			respondNotFound(w, r, "workspace")
			return
		}
	}

	// Get hub logo as fallback (for portals without their own logo)
	var hubLogoURL string
	var hubConfigJSON string
	if err := h.db.QueryRowContext(ctx, `SELECT value FROM system_settings WHERE key = 'portal_hub_config'`).Scan(&hubConfigJSON); err == nil && hubConfigJSON != "" {
		var hubConfig models.PortalHubConfig
		if err := json.Unmarshal([]byte(hubConfigJSON), &hubConfig); err == nil {
			hubLogoURL = hubConfig.LogoURL
		}
	}

	// Return portal info with customization settings
	response := map[string]interface{}{
		"channel_id":    channel.ID,
		"slug":          config.PortalSlug,
		"title":         config.PortalTitle,
		"description":   config.PortalDescription,
		"workspace_ids": config.PortalWorkspaceIDs,
		"workspace_id":  workspaceID, // First workspace for backward compatibility
		"workspace":     workspace,
		// Customization fields
		"gradient":                  config.PortalGradient,
		"theme":                     config.PortalTheme,
		"search_placeholder":        config.PortalSearchPlaceholder,
		"search_hint":               config.PortalSearchHint,
		"footer_columns":            config.PortalFooterColumns,
		"sections":                  config.PortalSections,
		"knowledge_base_share_link": config.KnowledgeBaseShareLink,
		"knowledge_base_url":        config.KnowledgeBaseURL,
		"knowledge_base_share_id":   config.KnowledgeBaseShareID,
		"background_image_url":      config.PortalBackgroundImageURL,
		"logo_url":                  config.PortalLogoURL,
		"hub_logo_url":              hubLogoURL,
	}

	respondJSONOK(w, response)
}

// GetRequestTypes returns request types for a portal, filtered by visibility
// For admin users viewing in customize mode, returns all request types
// For portal customers and regular users, filters by visibility rules
func (h *PortalHandler) GetRequestTypes(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Find channel by portal slug
	portalResult, err := h.findChannelByPortalSlug(ctx, slug)
	if err != nil {
		respondNotFound(w, r, "portal")
		return
	}
	channel := portalResult.channel

	// Query all request types for this channel
	query := `
		SELECT rt.id, rt.channel_id, rt.name, rt.description, rt.item_type_id,
		       rt.icon, rt.color, rt.display_order, rt.is_active,
		       rt.visibility_group_ids, rt.visibility_org_ids,
		       rt.created_at, rt.updated_at,
		       it.name as item_type_name
		FROM request_types rt
		LEFT JOIN item_types it ON rt.item_type_id = it.id
		WHERE rt.channel_id = ? AND rt.is_active = true
		ORDER BY rt.display_order, rt.name`

	rows, err := h.db.QueryContext(ctx, query, channel.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	defer func() { _ = rows.Close() }()

	// Get visibility context for filtering
	vc := h.getPortalVisibilityContext(ctx, r, channel.ID)

	var requestTypes []models.RequestType
	for rows.Next() {
		var rt models.RequestType
		var visibilityGroupIDs, visibilityOrgIDs sql.NullString
		err := rows.Scan(&rt.ID, &rt.ChannelID, &rt.Name, &rt.Description, &rt.ItemTypeID,
			&rt.Icon, &rt.Color, &rt.DisplayOrder, &rt.IsActive,
			&visibilityGroupIDs, &visibilityOrgIDs,
			&rt.CreatedAt, &rt.UpdatedAt,
			&rt.ItemTypeName)
		if err != nil {
			continue
		}

		applyRequestTypeVisibility(&rt, visibilityGroupIDs, visibilityOrgIDs)

		// Admin users see all request types; others see only visible ones
		if vc.isAdmin || rt.IsVisibleTo(vc.userGroupIDs, vc.customerOrgID) {
			requestTypes = append(requestTypes, rt)
		}
	}

	if requestTypes == nil {
		requestTypes = []models.RequestType{}
	}

	respondJSONOK(w, requestTypes)
}

// SubmitToPortal handles portal item submissions (requires authentication)
func (h *PortalHandler) SubmitToPortal(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, channel, config, ok := h.resolvePortalBySlug(w, r)
	if !ok {
		return
	}
	defer cancel()

	// Parse submission
	var submission struct {
		RequestTypeID *int                   `json:"request_type_id"`
		Title         string                 `json:"title"`
		Description   string                 `json:"description"`
		CustomFields  map[string]interface{} `json:"custom_fields"`
	}

	if err := json.NewDecoder(r.Body).Decode(&submission); err != nil {
		respondBadRequest(w, r, "Invalid submission")
		return
	}

	// Sanitize user input to prevent XSS
	submission.Title = utils.StripHTMLTags(submission.Title)
	submission.Description = utils.SanitizeCommentContent(submission.Description)

	// Get auth info from context (middleware already validated)
	authenticatedUserID, portalCustomerID := h.getAuthFromContext(r)

	// For portal customers, grant channel access. In manual-registration
	// mode the auto-grant is suppressed — only admin-managed customers with
	// pre-existing access can submit. Without this gate, a customer with
	// any portal session who reached this route (e.g. via cross-portal
	// session reuse before the binding check was added, or via a future
	// regression) would silently gain access to a manual-mode portal.
	// Internal users don't need portal customer records - they're tracked via user_id.
	if portalCustomerID != nil {
		if config.PortalRegistrationMode == "manual" {
			if !h.customerHasChannelAccess(ctx, *portalCustomerID, channel.ID) {
				slog.Warn("manual-mode portal blocked submit from customer without channel access",
					slog.String("component", "portal"),
					slog.Int("portal_customer_id", *portalCustomerID),
					slog.Int("channel_id", channel.ID),
				)
				respondUnauthorized(w, r)
				return
			}
		} else {
			h.grantChannelAccess(ctx, *portalCustomerID, channel.ID)
		}
	}

	// Validate request type visibility (security check). The resolved
	// request type is reused below to render the title template when the
	// title field is hidden from the form.
	var requestType *models.RequestType
	if submission.RequestTypeID != nil {
		var err error
		requestType, err = h.getRequestTypeWithVisibility(ctx, *submission.RequestTypeID)
		if err != nil {
			respondError(w, r, restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeNotFound, "Request type not found"))
			return
		}

		// Verify the request type belongs to this channel
		if requestType.ChannelID != channel.ID {
			respondError(w, r, restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeNotFound, "Request type not found"))
			return
		}

		// Get user context for visibility check
		userGroupIDs := h.getInternalUserGroupIDs(ctx, r)
		var customerOrgID *int
		if portalCustomerID != nil {
			customerOrgID = h.getPortalCustomerOrgID(ctx, *portalCustomerID)
		}

		// Check visibility — return 404 (not 403) to avoid leaking the
		// existence of request types the user can't see, matching the
		// not-found / wrong-channel branches above.
		if !requestType.IsVisibleTo(userGroupIDs, customerOrgID) {
			respondError(w, r, restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeNotFound, "Request type not found"))
			return
		}
	}

	// Validate and separate fields
	validationResult, err := validateAndSeparateFields(ctx, h.db, submission.RequestTypeID, submission.Title, submission.Description, submission.CustomFields)
	if err != nil {
		respondValidationError(w, r, err.Error())
		return
	}

	// Title fallback: when the request type hides the title field from the
	// form, render its title_template. Items have a NOT NULL title, so we
	// reject when the template is missing or renders to empty.
	if requestType != nil && !validationResult.titleFieldInForm {
		rendered := h.renderPortalTitle(ctx, requestType, submission.Description, submission.CustomFields, authenticatedUserID, portalCustomerID)
		if rendered == "" {
			respondValidationError(w, r, "request type is misconfigured: title field is hidden but no title template is set")
			return
		}
		submission.Title = utils.StripHTMLTags(rendered)
	}

	// Get target workspace (use first workspace for submission)
	if len(config.PortalWorkspaceIDs) == 0 {
		respondInternalError(w, r, fmt.Errorf("portal has no configured workspaces"))
		return
	}
	targetWorkspaceID := config.PortalWorkspaceIDs[0]

	// Determine initial status from workflow if item type is specified
	initialStatus := defaultItemStatus // Default fallback status
	if validationResult.itemTypeID != nil {
		var status string
		status, err = services.GetInitialStatusForItemType(h.db, *validationResult.itemTypeID)
		if err != nil {
			slog.Warn("could not determine initial status for item type", slog.String("component", "portal"), slog.Int("item_type_id", *validationResult.itemTypeID), slog.Any("error", err))
		} else {
			initialStatus = status
		}
	}

	// Create item using centralized service
	itemID, err := services.CreateItem(h.db, services.ItemCreationParams{
		WorkspaceID:             targetWorkspaceID,
		Title:                   submission.Title,
		Description:             submission.Description,
		Status:                  initialStatus,
		ItemTypeID:              validationResult.itemTypeID,
		Priority:                "medium",
		CreatorID:               authenticatedUserID,
		CreatorPortalCustomerID: portalCustomerID, // nil for internal users, set for portal customers
		ChannelID:               &channel.ID,
		RequestTypeID:           submission.RequestTypeID,
	})
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Store custom and virtual field values
	storeCustomFieldValues(ctx, h.db, "portal", itemID, validationResult.customFieldValues)
	storeVirtualFieldValues(ctx, h.db, "portal", itemID, validationResult.virtualFieldValues)

	// Update channel last activity
	if _, err := h.db.ExecWriteContext(ctx, `UPDATE channels SET last_activity = ? WHERE id = ?`, time.Now(), channel.ID); err != nil {
		slog.Warn("failed to update channel last_activity", slog.String("component", "portal"), slog.Int("channel_id", channel.ID), slog.Any("error", err))
	}

	// Drop any in-progress draft for this request type — the user just
	// submitted, so the saved state is no longer interesting. Best-effort:
	// failure here doesn't affect the successful submission.
	if submission.RequestTypeID != nil {
		h.deleteDraftAfterSubmit(ctx, *submission.RequestTypeID, repository.DraftIdentity{
			PortalCustomerID: portalCustomerID,
			UserID:           authenticatedUserID,
		})
	}

	// Return success response
	respondJSONCreated(w, map[string]interface{}{
		"success": true,
		"item_id": itemID,
		"message": "Submission received successfully",
	})
}

// SearchKnowledgeBase proxies knowledge base search requests to Docmost
func (h *PortalHandler) SearchKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Find channel by portal slug
	portalResult, err := h.findChannelByPortalSlug(ctx, slug)
	if err != nil {
		respondNotFound(w, r, "portal")
		return
	}
	config := portalResult.config

	// Check if knowledge base is configured
	if config.KnowledgeBaseURL == "" || config.KnowledgeBaseShareID == "" {
		respondError(w, r, restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeNotFound, "Knowledge base not configured for this portal"))
		return
	}

	// Parse search request
	var searchRequest struct {
		Query string `json:"query"`
	}

	if err = json.NewDecoder(r.Body).Decode(&searchRequest); err != nil {
		respondBadRequest(w, r, "Invalid search request")
		return
	}

	if searchRequest.Query == "" {
		respondValidationError(w, r, "Search query is required")
		return
	}
	// Cap query length: prevents pathological inputs and abuse of the proxy.
	if len(searchRequest.Query) > 256 {
		respondValidationError(w, r, "Search query is too long")
		return
	}

	// Defense in depth: re-validate the URL before making the request
	if err := utils.ValidateExternalURL(config.KnowledgeBaseURL); err != nil {
		respondError(w, r, restapi.NewAPIError(http.StatusBadGateway, "BAD_GATEWAY", "Failed to connect to knowledge base"))
		return
	}

	// Prepare Docmost API request
	docmostURL := fmt.Sprintf("%s/api/search/share-search", config.KnowledgeBaseURL)
	requestBody, err := json.Marshal(map[string]string{
		"query":   searchRequest.Query,
		"shareId": config.KnowledgeBaseShareID,
	})
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Make request to Docmost
	req, err := http.NewRequestWithContext(ctx, "POST", docmostURL, bytes.NewBuffer(requestBody))
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := utils.NewSSRFSafeHTTPClient(10 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		respondError(w, r, restapi.NewAPIError(http.StatusBadGateway, "BAD_GATEWAY", "Failed to connect to knowledge base"))
		return
	}
	defer func() { _ = resp.Body.Close() }()

	// Cap proxied response size at 2 MiB — Docmost search responses are JSON
	// snippets; anything larger is either misconfigured or an attempted
	// memory-exhaustion vector via this public, unauthenticated endpoint.
	const maxKBResponseBytes = 2 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxKBResponseBytes))
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Check response status
	if resp.StatusCode != http.StatusOK {
		respondError(w, r, restapi.NewAPIError(http.StatusBadGateway, "BAD_GATEWAY", "Knowledge base search failed"))
		return
	}

	// Forward response to client
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// DownloadPortalAttachment serves portal branding attachments (logos, backgrounds) without authentication
func (h *PortalHandler) DownloadPortalAttachment(w http.ResponseWriter, r *http.Request) {
	attachmentIDStr := r.PathValue("id")
	attachmentID, err := strconv.Atoi(attachmentIDStr)
	if err != nil {
		respondInvalidID(w, r, "id")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Get attachment info including category
	var filePath, mimeType, originalFilename, category string
	var fileSize int64
	err = h.db.QueryRowContext(ctx, `
		SELECT file_path, mime_type, original_filename, file_size, COALESCE(category, '') as category
		FROM attachments WHERE id = ?
	`, attachmentID).Scan(&filePath, &mimeType, &originalFilename, &fileSize, &category)

	if errors.Is(err, sql.ErrNoRows) {
		respondError(w, r, restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeNotFound, "Attachment not found"))
		return
	}
	if err != nil {
		slog.Error("failed to query attachment", slog.String("component", "portal"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	// Security check: Only allow portal branding attachments (logos, backgrounds)
	allowedCategories := map[string]bool{
		"portal_logo":       true,
		"portal_background": true,
		"hub_logo":          true,
	}

	if !allowedCategories[category] {
		// Return 404 to prevent enumeration of non-portal attachments
		respondError(w, r, restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeNotFound, "Attachment not found"))
		return
	}

	// Validate file path is within attachment directory (prevent path traversal)
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		slog.Error("failed to resolve file path", slog.String("component", "portal"), slog.Any("error", err))
		respondError(w, r, restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeNotFound, "File not found"))
		return
	}
	absBasePath, err := filepath.Abs(h.attachmentPath)
	if err != nil {
		slog.Error("failed to resolve attachment base path", slog.String("component", "portal"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}
	if !strings.HasPrefix(absPath, absBasePath+string(os.PathSeparator)) {
		slog.Warn("path traversal attempt detected", slog.String("component", "portal"), slog.String("file_path", filePath))
		respondError(w, r, restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeNotFound, "File not found"))
		return
	}

	// Open and serve the file
	file, err := os.Open(filePath) //nolint:gosec // G304 — filePath validated via filepath.Abs prefix check above
	if err != nil {
		slog.Error("failed to open attachment file", slog.String("component", "portal"), slog.String("path", filePath), slog.Any("error", err))
		respondError(w, r, restapi.NewAPIError(http.StatusNotFound, restapi.ErrCodeNotFound, "File not found"))
		return
	}
	defer func() { _ = file.Close() }()

	// Set headers
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Length", strconv.FormatInt(fileSize, 10))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Cache-Control", "public, max-age=86400") // Cache for 1 day
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", originalFilename))

	// Serve file
	_, _ = io.Copy(w, file)
}
