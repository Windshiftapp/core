package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"

	"windshift/internal/models"
	"windshift/internal/repository"
)

func (h *CustomerAccessHandler) portalBoard(w http.ResponseWriter, r *http.Request) (*http.Request, bool) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Add("Vary", "Cookie")
	if h.boardViewer == nil || h.publicBoards != nil && !h.publicBoards.PublicBoardsEnabled() {
		respondNotFound(w, r, "board")
		return nil, false
	}
	portal, err := h.repo.Resolve(r.Context(), "portal", r.PathValue("slug"))
	if err != nil {
		respondNotFound(w, r, "board")
		return nil, false
	}
	channel, err := h.channels.FindByID(r.Context(), portal.ID)
	if err != nil {
		respondNotFound(w, r, "board")
		return nil, false
	}
	var config models.ChannelConfig
	if err := json.Unmarshal([]byte(channel.Config), &config); err != nil {
		respondInternalError(w, r, err)
		return nil, false
	}
	if !config.PortalModuleEnabled("boards") {
		respondNotFound(w, r, "board")
		return nil, false
	}
	placement, err := repository.NewBoardPlacementRepository(h.boardViewer.db).Get(r.Context(), portal.ID, r.PathValue("placement_id"))
	if err != nil || !placement.Enabled {
		respondNotFound(w, r, "board")
		return nil, false
	}
	collection, err := h.collections.GetModel(placement.CollectionID)
	if err != nil || !collection.BoardEnabled || collection.IsPrivate || collection.PublicRevokedAt != nil {
		respondNotFound(w, r, "board")
		return nil, false
	}
	access, err := h.resolvePortalAccess(r, *portal, config)
	if err != nil {
		respondInternalError(w, r, err)
		return nil, false
	}
	portalPolicy, err := h.repo.Policy(r.Context(), "portal", portal.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return nil, false
	}
	boardPolicy, err := h.repo.Policy(r.Context(), "board", collection.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return nil, false
	}
	status, restricted, err := h.boardAccess(r, config, portalPolicy, access, *placement, collection.ID, boardPolicy)
	if err != nil {
		respondInternalError(w, r, err)
		return nil, false
	}
	switch status {
	case http.StatusOK:
	case http.StatusNotFound:
		respondNotFound(w, r, "board")
		return nil, false
	case http.StatusForbidden:
		respondForbidden(w, r)
		return nil, false
	default:
		respondJSON(w, http.StatusUnauthorized, map[string]any{"error": "Customer sign-in is required", "code": "CUSTOMER_LOGIN_REQUIRED"})
		return nil, false
	}
	prefix := requestContextPrefix(r) + "/api/v2/portal/" + url.PathEscape(portal.Slug) + "/boards/" + url.PathEscape(placement.ID) + "/attachments/"
	ctx := context.WithValue(r.Context(), portalBoardContextKey{}, portalBoardContext{CollectionID: collection.ID, AttachmentPrefix: prefix})
	if restricted && access.InternalID > 0 {
		ctx = context.WithValue(ctx, staffBoardFilterKey{}, true)
	}
	return r.WithContext(ctx), true
}

// boardAccess returns the effective status for reading a board placement: 200
// allowed, 401 customer sign-in required, 403 signed-in customer without
// access, 404 staff without internal collection access. It applies the portal
// admission/audience rules for customers and the internal collection rule for
// staff, so discovery and embedded reads share one decision. The bool reports
// whether the board is restricted rather than fully public.
func (h *CustomerAccessHandler) boardAccess(r *http.Request, config models.ChannelConfig, portalPolicy models.CustomerAccessPolicy, access portalAccess, placement models.BoardPlacement, collectionID int, baseline models.CustomerAccessPolicy) (status int, restricted bool, err error) {
	if !portalAdmissionRequired(config) && portalPolicy.Mode == "public" && baseline.Mode == "public" && placement.Policy.Mode == "public" {
		return http.StatusOK, false, nil
	}
	if access.InternalID > 0 {
		if h.staffBoardAllowed(access.InternalID, collectionID) {
			return http.StatusOK, true, nil
		}
		return http.StatusNotFound, true, nil
	}
	if access.CustomerID == 0 {
		return http.StatusUnauthorized, true, nil
	}
	if !access.ContentAllowed {
		return http.StatusForbidden, true, nil
	}
	allowed, err := h.repo.Allowed(r.Context(), models.CustomerSurface{Kind: "board", ID: collectionID}, access.CustomerID)
	if err != nil {
		return 0, true, err
	}
	if !allowed {
		return http.StatusForbidden, true, nil
	}
	if placement.Policy.Mode == "organisations" {
		organisation, err := h.customers.OrganisationID(access.CustomerID)
		if err != nil {
			return 0, true, err
		}
		if organisation == nil || !slices.Contains(placement.Policy.OrganisationIDs, *organisation) {
			return http.StatusForbidden, true, nil
		}
	}
	return http.StatusOK, true, nil
}

func (h *CustomerAccessHandler) GetPortalBoard(w http.ResponseWriter, r *http.Request) {
	if request, ok := h.portalBoard(w, r); ok {
		h.boardViewer.GetPublicBoard(w, request)
	}
}

func (h *CustomerAccessHandler) GetPortalBoardItem(w http.ResponseWriter, r *http.Request) {
	if request, ok := h.portalBoard(w, r); ok {
		h.boardViewer.GetPublicBoardItem(w, request)
	}
}

func (h *CustomerAccessHandler) DownloadPortalBoardAttachment(w http.ResponseWriter, r *http.Request) {
	if request, ok := h.portalBoard(w, r); ok {
		h.boardViewer.DownloadAttachment(w, request)
	}
}

func (h *CustomerAccessHandler) GetPortalExperience(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Add("Vary", "Cookie")
	surface, err := h.repo.Resolve(r.Context(), "portal", r.PathValue("slug"))
	if err != nil {
		respondNotFound(w, r, "portal")
		return
	}
	channel, err := h.channels.FindByID(r.Context(), surface.ID)
	if err != nil {
		respondNotFound(w, r, "portal")
		return
	}
	var config models.ChannelConfig
	if err := json.Unmarshal([]byte(channel.Config), &config); err != nil {
		respondInternalError(w, r, err)
		return
	}
	access, err := h.resolvePortalAccess(r, *surface, config)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	policy, err := h.repo.Policy(r.Context(), "portal", surface.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	result := models.CustomerExperience{Presentation: config.PortalPresentation, Title: config.PortalTitle, LogoURL: config.PortalLogoURL,
		Slug: surface.Slug, Modules: config.PortalModules, Navigation: config.PortalNavigation, DefaultModule: config.PortalDefaultModule,
		Boards: []models.CustomerExperienceBoard{}, Authenticated: access.CustomerID > 0 || access.InternalID > 0, IdentityKey: access.CustomerID}
	if access.InternalID > 0 {
		result.IsInternal, result.IdentityKey = true, -access.InternalID
	}
	if result.Presentation == "" {
		result.Presentation = "classic"
	}
	if result.Modules == nil {
		result.Modules = slices.Clone(models.PortalModuleKinds)
	}
	if result.Presentation == "compact" {
		result.Modules = slices.DeleteFunc(slices.Clone(result.Modules), func(module string) bool {
			if module == "knowledge-base" {
				return config.KnowledgeBaseShareLink == "" && len(config.KnowledgeBasePageSources) == 0
			}
			if module == "forms" {
				return len(config.PortalWorkspaceIDs) == 0
			}
			if module == "asset-reports" {
				return !slices.ContainsFunc(config.PortalSections, func(section models.PortalSection) bool { return len(section.AssetReportIDs) > 0 })
			}
			return false
		})
	}
	if h.boardViewer != nil && config.PortalModuleEnabled("boards") && (h.publicBoards == nil || h.publicBoards.PublicBoardsEnabled()) {
		placements, err := repository.NewBoardPlacementRepository(h.boardViewer.db).List(r.Context(), surface.ID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		for _, placement := range placements {
			if !placement.Enabled {
				continue
			}
			collection, err := h.collections.GetModel(placement.CollectionID)
			if err != nil || !collection.BoardEnabled || collection.IsPrivate || collection.PublicRevokedAt != nil {
				continue
			}
			baseline, err := h.repo.Policy(r.Context(), "board", collection.ID)
			if err != nil {
				respondInternalError(w, r, err)
				return
			}
			status, _, err := h.boardAccess(r, config, policy, access, placement, collection.ID, baseline)
			if err != nil {
				respondInternalError(w, r, err)
				return
			}
			if status != http.StatusOK {
				continue
			}
			result.Boards = append(result.Boards, models.CustomerExperienceBoard{ID: placement.ID, Label: placement.Label, DisplayOrder: placement.DisplayOrder, PublicationRevision: collection.SharingRevision, PlacementRevision: placement.Revision})
		}
	}
	if len(result.Boards) == 0 {
		result.Navigation = slices.DeleteFunc(slices.Clone(result.Navigation), func(module string) bool { return module == "boards" })
		if result.DefaultModule == "boards" {
			result.DefaultModule = ""
		}
	}
	revision, err := json.Marshal(struct {
		Config string
		Policy models.CustomerAccessPolicy
		Boards []models.CustomerExperienceBoard
	}{channel.Config, policy, result.Boards})
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	digest := sha256.Sum256(revision)
	result.Revision = hex.EncodeToString(digest[:])
	respondJSONOK(w, result)
}
