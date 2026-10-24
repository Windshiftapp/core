package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"windshift/internal/auth"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// PortalAuthHandler handles portal customer authentication
type PortalAuthHandler struct {
	portalAuthRepo       *repository.PortalAuthRepository
	portalSessionManager *auth.PortalSessionManager
	sessionManager       *auth.SessionManager // internal session manager
	magicLinkService     *services.MagicLinkService
	ipExtractor          *utils.IPExtractor
}

// NewPortalAuthHandler creates a new portal auth handler
func NewPortalAuthHandler(
	portalAuthRepo *repository.PortalAuthRepository,
	portalSessionManager *auth.PortalSessionManager,
	sessionManager *auth.SessionManager,
	magicLinkService *services.MagicLinkService,
	ipExtractor *utils.IPExtractor,
) *PortalAuthHandler {
	return &PortalAuthHandler{
		portalAuthRepo:       portalAuthRepo,
		portalSessionManager: portalSessionManager,
		sessionManager:       sessionManager,
		magicLinkService:     magicLinkService,
		ipExtractor:          ipExtractor,
	}
}

// getClientIP extracts the client IP with proxy validation
func (h *PortalAuthHandler) getClientIP(r *http.Request) string {
	return h.ipExtractor.GetClientIP(r)
}

// findPortalBySlug resolves a portal channel by its public slug. Thin wrapper
// over the shared findChannelBySlug helper so PortalAuthHandler benefits from
// the same error-logging and rows.Err() handling FormHandler and PortalHandler
// already get.
func (h *PortalAuthHandler) findPortalBySlug(ctx context.Context, slug string) (*models.Channel, *models.ChannelConfig, error) {
	return h.portalAuthRepo.FindPortalBySlug(ctx, slug)
}

// resolvePortalChannel resolves the portal for auth endpoints: a bounded
// context plus the channel lookup, writing a 404 when the portal is unknown.
// Callers defer the returned cancel.
func (h *PortalAuthHandler) resolvePortalChannel(w http.ResponseWriter, r *http.Request) (context.Context, context.CancelFunc, *models.Channel, bool) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	channel, _, err := h.findPortalBySlug(ctx, r.PathValue("slug"))
	if err != nil {
		cancel()
		respondNotFound(w, r, "portal")
		return nil, func() {}, nil, false
	}
	return ctx, cancel, channel, true
}

// RequestMagicLink handles POST /portal/{slug}/auth/request
// Sends a magic link email to the portal customer
func (h *PortalAuthHandler) RequestMagicLink(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Find portal
	channel, config, err := h.findPortalBySlug(ctx, slug)
	if err != nil {
		// Always return success to prevent email enumeration
		slog.Debug("portal not found", slog.String("component", "portal_auth"), slog.String("slug", slug))
		respondJSONOK(w, map[string]any{
			"success": true,
			"message": "If your email is registered, you will receive a sign-in link shortly.",
		})
		return
	}

	// Parse request body
	var request struct {
		Email string `json:"email" validate:"required,email,max=255"`
		Next  string `json:"next"`
	}
	if !decodeChannelRequest(w, r, &request, false) {
		return
	}
	sanitize.Apply(&request.Email, sanitize.ShortIdentifier)

	// A deep link the customer followed into the portal; echoed back through
	// the emailed verify URL so sign-in resumes the same page.
	next := services.PortalNextPath(request.Next, slug)

	email := strings.TrimSpace(strings.ToLower(request.Email))
	if email == "" {
		respondValidationError(w, r, "Email is required")
		return
	}
	request.Email = email
	if err = utils.Validate(request); err != nil {
		respondValidationError(w, r, err.Error())
		return
	}

	// Domain allow-list: if configured, reject emails outside the allowed domains.
	// Explicit error is returned so legitimate users who typo their email get a
	// clear signal; the allowed domains themselves are not disclosed.
	if len(config.PortalAllowedDomains) > 0 {
		at := strings.LastIndex(email, "@")
		if at < 0 || at == len(email)-1 {
			respondValidationError(w, r, "Invalid email address")
			return
		}
		domain := email[at+1:]
		allowed := false
		for _, d := range config.PortalAllowedDomains {
			if strings.EqualFold(strings.TrimSpace(d), domain) {
				allowed = true
				break
			}
		}
		if !allowed {
			respondValidationError(w, r, "This email domain is not permitted for this portal.")
			return
		}
	}

	// Unknown modes fail closed. An empty value is the legacy spelling of
	// "open"; anything else must not silently turn a typo into open signup.
	if config.PortalRegistrationMode != "" && config.PortalRegistrationMode != "open" && config.PortalRegistrationMode != "manual" {
		respondJSONOK(w, map[string]any{
			"success": true,
			"message": "If your email is registered, you will receive a sign-in link shortly.",
		})
		return
	}

	// Manual registration mode: only admin-managed customers with existing channel
	// access can sign in. Return the generic success response for unknown emails
	// to avoid leaking who is a customer.
	if config.PortalRegistrationMode == "manual" {
		hasAccess, err := h.portalAuthRepo.CustomerEmailHasChannelAccess(ctx, email, channel.ID)
		if err != nil || !hasAccess {
			if err != nil {
				slog.Error("failed to check portal customer access", slog.String("component", "portal_auth"), slog.Any("error", err))
			}
			respondJSONOK(w, map[string]any{
				"success": true,
				"message": "If your email is registered, you will receive a sign-in link shortly.",
			})
			return
		}
	}

	// Find or create portal customer by email
	customerID, err := h.magicLinkService.FindOrCreatePortalCustomer(email, "", channel.ID)
	if err != nil {
		slog.Error("failed to find or create portal customer", slog.String("component", "portal_auth"), slog.String("email", email), slog.Any("error", err))
		// Still return success to prevent email enumeration
		respondJSONOK(w, map[string]any{
			"success": true,
			"message": "If your email is registered, you will receive a sign-in link shortly.",
		})
		return
	}

	// Get customer name for email personalization (may be empty for new customers)
	_, customerName, _ := h.magicLinkService.GetPortalCustomerByEmail(email)

	// Generate magic link
	token, err := h.magicLinkService.GenerateMagicLink(customerID, &channel.ID)
	if err != nil {
		slog.Error("failed to generate magic link", slog.String("component", "portal_auth"), slog.Any("error", err))
		// Still return success to prevent enumeration
		respondJSONOK(w, map[string]any{
			"success": true,
			"message": "If your email is registered, you will receive a sign-in link shortly.",
		})
		return
	}

	// Send magic link email
	err = h.magicLinkService.SendMagicLinkEmail(email, customerName, token, slug, next)
	if err != nil {
		slog.Error("failed to send magic link email", slog.String("component", "portal_auth"), slog.Any("error", err))
		// Still return success to prevent enumeration
	} else {
		slog.Info("magic link email sent", slog.String("component", "portal_auth"), slog.String("email", email), slog.String("portal", slug))
	}

	respondJSONOK(w, map[string]any{
		"success": true,
		"message": "If your email is registered, you will receive a sign-in link shortly.",
	})
}

// VerifyMagicLink handles GET /portal/{slug}/auth/verify
// Verifies the magic link token and creates a session
func (h *PortalAuthHandler) VerifyMagicLink(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")

	_, cancel, channel, ok := h.resolvePortalChannel(w, r)
	if !ok {
		return
	}
	defer cancel()

	if token == "" {
		respondValidationError(w, r, "Token is required")
		return
	}

	// Validate magic link. Expired/used/channel-mismatched tokens return a
	// populated result alongside the sentinel error so we can hand the
	// customer's email back to the frontend for a smooth recovery flow.
	// Channel-mismatched tokens are not consumed — the customer can still
	// redeem the link at the correct portal.
	result, err := h.magicLinkService.ValidateMagicLink(token, channel.ID)
	if err != nil {
		slog.Warn("magic link validation failed", slog.String("component", "portal_auth"), slog.Any("error", err))

		var message, code string
		var statusCode int
		switch err {
		case services.ErrMagicLinkExpired:
			message = "This link has expired. Please request a new sign-in link."
			code = "expired"
			statusCode = http.StatusUnauthorized
		case services.ErrMagicLinkAlreadyUsed:
			message = "This link has already been used. Please request a new sign-in link."
			code = "used"
			statusCode = http.StatusUnauthorized
		case services.ErrMagicLinkInvalid, services.ErrMagicLinkChannelMismatch:
			message = "This link is invalid. Please request a new sign-in link."
			code = "invalid"
			statusCode = http.StatusUnauthorized
		default:
			message = "Failed to verify link. Please try again."
			code = "error"
			statusCode = http.StatusInternalServerError
		}

		body := map[string]any{
			"success": false,
			"message": message,
			"code":    code,
		}
		// Possessing the (now-dead) token implies the customer received the
		// email, so returning the email back is not enumeration — it lets
		// the recovery UX prefill the sign-in form.
		if result != nil && result.CustomerEmail != "" {
			body["email"] = result.CustomerEmail
		}
		respondJSON(w, statusCode, body)
		return
	}

	// Create portal session
	clientIP := h.getClientIP(r)
	userAgent := r.Header.Get("User-Agent")

	session, err := h.portalSessionManager.CreatePortalSession(result.PortalCustomerID, channel.ID, clientIP, userAgent)
	if err != nil {
		// A deactivated customer can hold nothing against a valid link they
		// received, so a clear refusal beats a generic 500 (WI-1554).
		if errors.Is(err, auth.ErrPortalCustomerDeactivated) {
			slog.Warn("magic link redemption refused: customer deactivated",
				slog.String("component", "portal_auth"), slog.Int("portal_customer_id", result.PortalCustomerID))
			respondJSON(w, http.StatusUnauthorized, map[string]any{
				"success": false,
				"message": "This account has been deactivated. Please contact support if you believe this is a mistake.",
				"code":    "deactivated",
			})
			return
		}
		slog.Error("failed to create portal session", slog.String("component", "portal_auth"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	// Set session cookie
	if err := h.portalSessionManager.SetPortalSessionCookie(w, r, session.Token); err != nil {
		slog.Error("failed to set portal session cookie", slog.String("component", "portal_auth"), slog.Any("error", err))
		respondInternalError(w, r, err)
		return
	}

	slog.Info("portal customer authenticated", slog.String("component", "portal_auth"), slog.Int("portal_customer_id", result.PortalCustomerID), slog.String("email", result.CustomerEmail))

	respondJSONOK(w, map[string]any{
		"success": true,
		"message": "Successfully signed in",
		"customer": map[string]any{
			"id":    result.PortalCustomerID,
			"email": result.CustomerEmail,
			"name":  result.CustomerName,
		},
	})
}

// Logout handles POST /portal/{slug}/auth/logout
// Logs out the current portal customer
func (h *PortalAuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Find portal
	_, _, err := h.findPortalBySlug(ctx, slug)
	if err != nil {
		respondNotFound(w, r, "portal")
		return
	}

	// Get session token
	token, err := h.portalSessionManager.GetPortalSessionFromRequest(r)
	if err == nil && token != "" {
		// Delete the session from database
		if err := h.portalSessionManager.DeletePortalSession(token); err != nil {
			slog.Warn("failed to delete portal session", slog.String("component", "portal_auth"), slog.Any("error", err))
		}
	}

	// Clear the session cookie
	h.portalSessionManager.ClearPortalSessionCookie(w, r)

	slog.Debug("portal customer logged out", slog.String("component", "portal_auth"), slog.String("portal", slug))

	respondJSONOK(w, map[string]any{
		"success": true,
		"message": "Successfully logged out",
	})
}

// GetCurrentCustomer handles GET /portal/{slug}/auth/me
// Returns the current authenticated portal customer or internal user
func (h *PortalAuthHandler) GetCurrentCustomer(w http.ResponseWriter, r *http.Request) {
	ctx, cancel, channel, ok := h.resolvePortalChannel(w, r)
	if !ok {
		return
	}
	defer cancel()

	// Try portal session first. Sessions minted on a different portal are
	// ignored so the cookie cannot be used to introspect identity on a portal
	// the customer did not authenticate to.
	token, err := h.portalSessionManager.GetPortalSessionFromRequest(r)
	if err == nil {
		clientIP := h.getClientIP(r)
		session, err := h.portalSessionManager.ValidatePortalSession(token, clientIP)
		if err == nil && session.ChannelID != nil && *session.ChannelID == channel.ID {
			// Look up passkey state used by the frontend to drive both the
			// "set up a passkey" banner and the login modal's passkey button.
			info, err := h.portalAuthRepo.GetCustomerSessionInfo(ctx, session.Customer.ID)
			if err != nil {
				slog.Warn("failed to load portal customer session info", slog.String("component", "portal_auth"), slog.Any("error", err))
				info = &repository.PortalCustomerSessionInfo{}
			}

			customerPayload := map[string]any{
				"id":            session.Customer.ID,
				"email":         session.Customer.Email,
				"name":          session.Customer.Name,
				"passkey_count": info.PasskeyCount,
			}
			if info.DismissedPasskeyPromptAt != nil {
				customerPayload["dismissed_passkey_prompt_at"] = info.DismissedPasskeyPromptAt.Format(time.RFC3339)
			} else {
				customerPayload["dismissed_passkey_prompt_at"] = nil
			}

			respondJSONOK(w, map[string]any{
				"authenticated": true,
				"is_internal":   false,
				"customer":      customerPayload,
			})
			return
		}
	}

	// Fallback: Check for internal session
	if h.sessionManager != nil {
		internalToken, err := h.sessionManager.GetSessionFromRequest(r)
		if err == nil {
			clientIP := h.getClientIP(r)
			session, err := h.sessionManager.ValidateSessionContext(r.Context(), internalToken, clientIP)
			if err == nil && session.User != nil {
				// Internal user authenticated
				respondJSONOK(w, map[string]any{
					"authenticated": true,
					"is_internal":   true,
					"user": map[string]any{
						"id":         session.User.ID,
						"email":      session.User.Email,
						"name":       session.User.FirstName + " " + session.User.LastName,
						"first_name": session.User.FirstName,
						"last_name":  session.User.LastName,
						"avatar_url": session.User.AvatarURL,
					},
				})
				return
			}
		}
	}

	// No valid session found
	respondJSON(w, http.StatusUnauthorized, map[string]any{
		"authenticated": false,
	})
}
