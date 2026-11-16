package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"

	"windshift/internal/middleware"
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
	id := h.customer(r, *portal)
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
	restricted := portalAdmissionRequired(config) || portalPolicy.Mode != "public" || boardPolicy.Mode != "public" || placement.Policy.Mode != "public"
	if restricted && id == 0 {
		respondJSON(w, http.StatusUnauthorized, map[string]any{"error": "Customer sign-in is required", "code": "CUSTOMER_LOGIN_REQUIRED"})
		return nil, false
	}
	if id > 0 {
		_, email, identityErr := h.customers.GetIdentity(r.Context(), id)
		if identityErr != nil || h.portalAdmission(r.Context(), *portal, email) != nil {
			respondForbidden(w, r)
			return nil, false
		}
		for _, surface := range []models.CustomerSurface{*portal, {Kind: "board", ID: collection.ID}} {
			allowed, err := h.repo.Allowed(r.Context(), surface, id)
			if err != nil {
				respondInternalError(w, r, err)
				return nil, false
			}
			if !allowed {
				respondForbidden(w, r)
				return nil, false
			}
		}
		if placement.Policy.Mode == "organisations" {
			organisationID, err := h.customers.OrganisationID(id)
			if err != nil || organisationID == nil || !slices.Contains(placement.Policy.OrganisationIDs, *organisationID) {
				respondForbidden(w, r)
				return nil, false
			}
		}
	}
	prefix := requestContextPrefix(r) + "/api/v2/portal/" + url.PathEscape(portal.Slug) + "/boards/" + url.PathEscape(placement.ID) + "/attachments/"
	ctx := context.WithValue(r.Context(), portalBoardContextKey{}, portalBoardContext{CollectionID: collection.ID, AttachmentPrefix: prefix})
	return r.WithContext(ctx), true
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
	id := h.customer(r, *surface)
	if id == 0 && portalAdmissionRequired(config) {
		respondJSON(w, http.StatusUnauthorized, map[string]any{"error": "Customer sign-in is required", "code": "CUSTOMER_LOGIN_REQUIRED"})
		return
	}
	if id > 0 {
		_, email, err := h.customers.GetIdentity(r.Context(), id)
		if err != nil || h.portalAdmission(r.Context(), *surface, email) != nil {
			respondForbidden(w, r)
			return
		}
	}
	policy, err := h.repo.Policy(r.Context(), "portal", surface.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if policy.Mode != "public" {
		if id == 0 {
			respondJSON(w, http.StatusUnauthorized, map[string]any{"error": "Customer sign-in is required", "code": "CUSTOMER_LOGIN_REQUIRED"})
			return
		}
		allowed, err := h.repo.Allowed(r.Context(), *surface, id)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if !allowed {
			respondForbidden(w, r)
			return
		}
	}
	result := models.CustomerExperience{Presentation: config.PortalPresentation, Title: config.PortalTitle, LogoURL: config.PortalLogoURL,
		Slug: surface.Slug, Modules: config.PortalModules, Navigation: config.PortalNavigation, DefaultModule: config.PortalDefaultModule,
		Boards: []models.CustomerExperienceBoard{}, Authenticated: id > 0, IdentityKey: id}
	// Internal authentication is useful to existing request modules, never board audience evidence.
	if id == 0 && policy.Mode == "public" {
		if user, ok := r.Context().Value(middleware.ContextKeyUser).(*models.User); ok && user != nil {
			result.Authenticated, result.IsInternal, result.IdentityKey = true, true, -user.ID
		}
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
			if id == 0 && (baseline.Mode != "public" || placement.Policy.Mode != "public") {
				continue
			}
			if id > 0 {
				allowed, err := h.repo.Allowed(r.Context(), models.CustomerSurface{Kind: "board", ID: collection.ID}, id)
				if err != nil {
					respondInternalError(w, r, err)
					return
				}
				if !allowed {
					continue
				}
				if placement.Policy.Mode == "organisations" {
					organisation, err := h.customers.OrganisationID(id)
					if err != nil {
						respondInternalError(w, r, err)
						return
					}
					if organisation == nil || !slices.Contains(placement.Policy.OrganisationIDs, *organisation) {
						continue
					}
				}
			}
			result.Boards = append(result.Boards, models.CustomerExperienceBoard{ID: placement.ID, Label: placement.Label, DisplayOrder: placement.DisplayOrder, PublicationRevision: collection.SharingRevision, PlacementRevision: placement.Revision})
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
