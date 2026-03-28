package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"windshift/internal/services"
)

// InvitationHandler handles user invitation API requests
type InvitationHandler struct {
	invitationService *services.InvitationService
}

// NewInvitationHandler creates a new invitation handler
func NewInvitationHandler(invitationService *services.InvitationService) *InvitationHandler {
	return &InvitationHandler{
		invitationService: invitationService,
	}
}

// VerifyInvitation handles the verification of an invitation token
func (h *InvitationHandler) VerifyInvitation(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		// Try to get token from URL path if not in query
		pathParts := strings.Split(r.URL.Path, "/")
		if len(pathParts) > 0 {
			token = pathParts[len(pathParts)-1]
		}
	}

	if token == "" {
		respondBadRequest(w, r, "token is required")
		return
	}

	user, err := h.invitationService.VerifyInvitation(token)
	if err != nil {
		switch err {
		case services.ErrInvitationInvalid:
			respondBadRequest(w, r, "invalid invitation token")
		case services.ErrInvitationExpired:
			respondBadRequest(w, r, "invitation token has expired")
		case services.ErrInvitationAlreadyUsed:
			respondBadRequest(w, r, "invitation has already been used")
		default:
			respondInternalError(w, r, err)
		}
		return
	}

	respondJSONOK(w, user)
}

// AcceptInvitation handles setting the password for an invited user
func (h *InvitationHandler) AcceptInvitation(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondBadRequest(w, r, "invalid request body")
		return
	}

	if req.Token == "" {
		respondBadRequest(w, r, "token is required")
		return
	}

	if req.Password == "" {
		respondBadRequest(w, r, "password is required")
		return
	}

	// Validate password strength (optional, but recommended)
	if len(req.Password) < 8 {
		respondBadRequest(w, r, "password must be at least 8 characters long")
		return
	}

	err := h.invitationService.AcceptInvitation(req.Token, req.Password)
	if err != nil {
		switch err {
		case services.ErrInvitationInvalid:
			respondBadRequest(w, r, "invalid invitation token")
		case services.ErrInvitationExpired:
			respondBadRequest(w, r, "invitation token has expired")
		case services.ErrInvitationAlreadyUsed:
			respondBadRequest(w, r, "invitation has already been used")
		default:
			respondInternalError(w, r, err)
		}
		return
	}

	respondJSONOK(w, map[string]string{"status": "ok"})
}
