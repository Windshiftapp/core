package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"windshift/internal/auth"
	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// PortalAuthHandler handles portal customer authentication
type PortalAuthHandler struct {
	db                   database.Database
	portalSessionManager *auth.PortalSessionManager
	sessionManager       *auth.SessionManager // internal session manager
	magicLinkService     *services.MagicLinkService
	ipExtractor          *utils.IPExtractor
}

// NewPortalAuthHandler creates a new portal auth handler
func NewPortalAuthHandler(
	db database.Database,
	portalSessionManager *auth.PortalSessionManager,
	sessionManager *auth.SessionManager,
	magicLinkService *services.MagicLinkService,
	ipExtractor *utils.IPExtractor,
) *PortalAuthHandler {
	return &PortalAuthHandler{
		db:                   db,
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
	res, err := findChannelBySlug(ctx, h.db, "portal", slug, func(c *models.ChannelConfig) string { return c.PortalSlug })
	if err != nil {
		return nil, nil, err
	}
	channel := res.channel
	config := res.config
	return &channel, &config, nil
}

// RequestMagicLink handles POST /portal/{slug}/auth/request
// Sends a magic link email to the portal customer
func (h *PortalAuthHandler) RequestMagicLink(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Find portal
	channel, config, err := h.findPortalBySlug(ctx, slug)
	if err != nil {
		// Always return success to prevent email enumeration
		slog.Debug("portal not found", slog.String("component", "portal_auth"), slog.String("slug", slug))
		respondJSONOK(w, map[string]interface{}{
			"success": true,
			"message": "If your email is registered, you will receive a sign-in link shortly.",
		})
		return
	}

	// Parse request body
	var request struct {
		Email string `json:"email"`
	}
	if err = json.NewDecoder(r.Body).Decode(&request); err != nil {
		respondBadRequest(w, r, "Invalid request body")
		return
	}

	email := strings.TrimSpace(strings.ToLower(request.Email))
	if email == "" {
		respondValidationError(w, r, "Email is required")
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

	// Manual registration mode: only admin-managed customers with existing channel
	// access can sign in. Return the generic success response for unknown emails
	// to avoid leaking who is a customer.
	if config.PortalRegistrationMode == "manual" {
		var hasAccess bool
		err := h.db.QueryRowContext(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM portal_customer_channels pcc
				JOIN portal_customers pc ON pc.id = pcc.portal_customer_id
				WHERE pc.email = ? AND pcc.channel_id = ?
			)
		`, email, channel.ID).Scan(&hasAccess)
		if err != nil || !hasAccess {
			if err != nil {
				slog.Error("failed to check portal customer access", slog.String("component", "portal_auth"), slog.Any("error", err))
			}
			respondJSONOK(w, map[string]interface{}{
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
		respondJSONOK(w, map[string]interface{}{
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
		respondJSONOK(w, map[string]interface{}{
			"success": true,
			"message": "If your email is registered, you will receive a sign-in link shortly.",
		})
		return
	}

	// Send magic link email
	err = h.magicLinkService.SendMagicLinkEmail(email, customerName, token, slug)
	if err != nil {
		slog.Error("failed to send magic link email", slog.String("component", "portal_auth"), slog.Any("error", err))
		// Still return success to prevent enumeration
	} else {
		slog.Info("magic link email sent", slog.String("component", "portal_auth"), slog.String("email", email), slog.String("portal", slug))
	}

	respondJSONOK(w, map[string]interface{}{
		"success": true,
		"message": "If your email is registered, you will receive a sign-in link shortly.",
	})
}

// VerifyMagicLink handles GET /portal/{slug}/auth/verify
// Verifies the magic link token and creates a session
func (h *PortalAuthHandler) VerifyMagicLink(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	token := r.URL.Query().Get("token")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Find portal
	channel, _, err := h.findPortalBySlug(ctx, slug)
	if err != nil {
		respondNotFound(w, r, "portal")
		return
	}

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

		body := map[string]interface{}{
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

	respondJSONOK(w, map[string]interface{}{
		"success": true,
		"message": "Successfully signed in",
		"customer": map[string]interface{}{
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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

	respondJSONOK(w, map[string]interface{}{
		"success": true,
		"message": "Successfully logged out",
	})
}

// GetCurrentCustomer handles GET /portal/{slug}/auth/me
// Returns the current authenticated portal customer or internal user
func (h *PortalAuthHandler) GetCurrentCustomer(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Find portal
	channel, _, err := h.findPortalBySlug(ctx, slug)
	if err != nil {
		respondNotFound(w, r, "portal")
		return
	}

	// Try portal session first. Sessions minted on a different portal are
	// ignored so the cookie cannot be used to introspect identity on a portal
	// the customer did not authenticate to.
	token, err := h.portalSessionManager.GetPortalSessionFromRequest(r)
	if err == nil {
		session, err := h.portalSessionManager.ValidatePortalSession(token)
		if err == nil && session.ChannelID != nil && *session.ChannelID == channel.ID {
			// Look up passkey state used by the frontend to drive both the
			// "set up a passkey" banner and the login modal's passkey button.
			var passkeyCount int
			_ = h.db.QueryRowContext(ctx, `
				SELECT COUNT(*) FROM portal_webauthn_credentials WHERE portal_customer_id = ?
			`, session.Customer.ID).Scan(&passkeyCount)

			var dismissedAt sql.NullTime
			_ = h.db.QueryRowContext(ctx, `
				SELECT dismissed_passkey_prompt_at FROM portal_customers WHERE id = ?
			`, session.Customer.ID).Scan(&dismissedAt)

			customerPayload := map[string]interface{}{
				"id":            session.Customer.ID,
				"email":         session.Customer.Email,
				"name":          session.Customer.Name,
				"passkey_count": passkeyCount,
			}
			if dismissedAt.Valid {
				customerPayload["dismissed_passkey_prompt_at"] = dismissedAt.Time.Format(time.RFC3339)
			} else {
				customerPayload["dismissed_passkey_prompt_at"] = nil
			}

			respondJSONOK(w, map[string]interface{}{
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
			session, err := h.sessionManager.ValidateSession(internalToken, clientIP)
			if err == nil && session.User != nil {
				// Internal user authenticated
				respondJSONOK(w, map[string]interface{}{
					"authenticated": true,
					"is_internal":   true,
					"user": map[string]interface{}{
						"id":         session.User.ID,
						"email":      session.User.Email,
						"name":       session.User.FirstName + " " + session.User.LastName,
						"first_name": session.User.FirstName,
						"last_name":  session.User.LastName,
					},
				})
				return
			}
		}
	}

	// No valid session found
	respondJSON(w, http.StatusUnauthorized, map[string]interface{}{
		"authenticated": false,
	})
}
