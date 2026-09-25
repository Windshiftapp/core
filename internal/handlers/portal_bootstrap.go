package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"windshift/internal/auth"
	"windshift/internal/middleware"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

type PortalBootstrapResponse struct {
	Portal       map[string]any             `json:"portal"`
	RequestTypes []models.RequestType       `json:"request_types"`
	AssetReports []models.PublicAssetReport `json:"asset_reports"`
}

type PortalUserBootstrapResponse struct {
	Authenticated bool                            `json:"authenticated"`
	IsInternal    bool                            `json:"is_internal"`
	User          map[string]any                  `json:"user,omitempty"`
	Customer      map[string]any                  `json:"customer,omitempty"`
	MyRequests    []services.PortalRequestSummary `json:"my_requests"`
	MyApprovals   []*models.ApprovalRequest       `json:"my_approvals"`
}

// GetBootstrap returns a branded sign-in shell to anonymous or unauthorized
// callers. Authorized callers also receive the portal configuration and its
// visibility-filtered catalogs.
func (h *PortalHandler) GetBootstrap(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, channel, config, ok := h.resolvePortalEntryBySlugTimeout(w, r, 10*time.Second)
	if !ok {
		return
	}
	defer cancel()

	response := PortalBootstrapResponse{
		Portal:       h.loadPortalEntryData(ctx, config),
		RequestTypes: []models.RequestType{},
		AssetReports: []models.PublicAssetReport{},
	}
	allowed, err := h.portalChannelAccessAllowed(ctx, r, channel.ID, config)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !allowed {
		respondJSONOK(w, response)
		return
	}

	portal, err := h.loadPortalData(ctx, r, channel, config)
	if errors.Is(err, repository.ErrNotFound) {
		respondNotFound(w, r, "workspace")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	response.Portal = portal
	vc := h.getPortalVisibilityContext(ctx, r, channel.ID)
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		requestTypes, err := h.loadPortalRequestTypes(ctx, channel.ID, vc)
		if err != nil {
			slog.Warn("portal bootstrap: request types unavailable", "channel_id", channel.ID, "error", err)
			return
		}
		response.RequestTypes = requestTypes
	}()
	go func() {
		defer wait.Done()
		assetReports, err := h.loadPortalAssetReports(channel, config, vc)
		if err != nil {
			slog.Warn("portal bootstrap: asset reports unavailable", "channel_id", channel.ID, "error", err)
			return
		}
		response.AssetReports = assetReports
	}()
	wait.Wait()
	respondJSONOK(w, response)
}

// GetUserBootstrap composes the optional identity and the two badge datasets
// loaded on every signed-in portal entry. Anonymous probes return a stable
// unauthenticated snapshot rather than a noisy 401 resource error.
func (h *PortalHandler) GetUserBootstrap(w http.ResponseWriter, r *http.Request) {
	internalUserID, portalCustomerID := h.getAuthFromContext(r)
	if internalUserID == nil && portalCustomerID == nil {
		_, cancel, _, _, ok := h.resolvePortalEntryBySlugTimeout(w, r, 10*time.Second)
		if !ok {
			return
		}
		defer cancel()
		respondJSONOK(w, PortalUserBootstrapResponse{
			Authenticated: false,
			MyRequests:    []services.PortalRequestSummary{},
			MyApprovals:   []*models.ApprovalRequest{},
		})
		return
	}

	ctx, cancel, channel, _, ok := h.resolvePortalBySlug(w, r)
	if !ok {
		return
	}
	defer cancel()

	response, err := h.portalAuthSnapshot(ctx, r)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		requests, err := h.loadMyPortalRequests(ctx, r, channel.ID)
		if err != nil {
			slog.Warn("portal user bootstrap: requests unavailable", "channel_id", channel.ID, "error", err)
			return
		}
		response.MyRequests = requests
	}()
	go func() {
		defer wait.Done()
		if h.approvalService == nil {
			return
		}
		actor, err := h.portalApprovalActorFromRequest(r)
		if err != nil {
			slog.Warn("portal user bootstrap: approval actor unavailable", "channel_id", channel.ID, "error", err)
			return
		}
		approvals, err := h.getApprovalsForPortalActor(ctx, actor, "pending", channel.ID)
		if err != nil {
			slog.Warn("portal user bootstrap: approvals unavailable", "channel_id", channel.ID, "error", err)
			return
		}
		response.MyApprovals = approvals
	}()
	wait.Wait()
	respondJSONOK(w, response)
}

func (h *PortalHandler) portalAuthSnapshot(ctx context.Context, r *http.Request) (PortalUserBootstrapResponse, error) {
	response := PortalUserBootstrapResponse{
		Authenticated: true,
		MyRequests:    []services.PortalRequestSummary{},
		MyApprovals:   []*models.ApprovalRequest{},
	}
	if session, ok := r.Context().Value(middleware.ContextKeySession).(*auth.Session); ok && session != nil && session.User != nil {
		response.IsInternal = true
		response.User = map[string]any{
			"id":         session.User.ID,
			"email":      session.User.Email,
			"name":       session.User.FirstName + " " + session.User.LastName,
			"first_name": session.User.FirstName,
			"last_name":  session.User.LastName,
			"avatar_url": session.User.AvatarURL,
		}
		return response, nil
	}

	portalSession, ok := r.Context().Value(middleware.ContextKeyPortalSession).(*auth.PortalSession)
	if !ok || portalSession == nil || portalSession.Customer == nil {
		return PortalUserBootstrapResponse{}, fmt.Errorf("authenticated portal session missing customer")
	}
	info, err := h.portalAuthRepo.GetCustomerSessionInfo(ctx, portalSession.Customer.ID)
	if err != nil {
		slog.Warn("portal user bootstrap: passkey state unavailable", "portal_customer_id", portalSession.Customer.ID, "error", err)
		info = &repository.PortalCustomerSessionInfo{}
	}
	response.Customer = map[string]any{
		"id":                          portalSession.Customer.ID,
		"email":                       portalSession.Customer.Email,
		"name":                        portalSession.Customer.Name,
		"passkey_count":               info.PasskeyCount,
		"dismissed_passkey_prompt_at": info.DismissedPasskeyPromptAt,
	}
	return response, nil
}

func (h *PortalHandler) loadMyPortalRequests(ctx context.Context, r *http.Request, channelID int) ([]services.PortalRequestSummary, error) {
	internalUserID, portalCustomerID := h.getAuthFromContext(r)
	var (
		requests []services.PortalRequestSummary
		err      error
	)
	switch {
	case internalUserID != nil:
		requests, err = h.portalService.GetRequestsByCreatorID(ctx, *internalUserID, channelID)
	case portalCustomerID != nil:
		requests, err = h.portalService.GetRequestsByPortalCustomerID(ctx, *portalCustomerID, channelID)
	default:
		return nil, errPortalApprovalActorUnauthorized
	}
	if requests == nil {
		requests = []services.PortalRequestSummary{}
	}
	return requests, err
}
