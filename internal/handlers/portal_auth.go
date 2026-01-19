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
	magicLinkService     *services.MagicLinkService
	ipExtractor          *utils.IPExtractor
}

// NewPortalAuthHandler creates a new portal auth handler
func NewPortalAuthHandler(
	db database.Database,
	portalSessionManager *auth.PortalSessionManager,
	magicLinkService *services.MagicLinkService,
	ipExtractor *utils.IPExtractor,
) *PortalAuthHandler {
	return &PortalAuthHandler{
		db:                   db,
		portalSessionManager: portalSessionManager,
		magicLinkService:     magicLinkService,
		ipExtractor:          ipExtractor,
	}
}

// getClientIP extracts the client IP with proxy validation
func (h *PortalAuthHandler) getClientIP(r *http.Request) string {
	return h.ipExtractor.GetClientIP(r)
}

// findPortalBySlug finds a portal channel by its slug
func (h *PortalAuthHandler) findPortalBySlug(ctx context.Context, slug string) (*models.Channel, *models.ChannelConfig, error) {
	query := `
		SELECT id, name, type, config, status
		FROM channels
		WHERE type = 'portal'
		ORDER BY created_at DESC
	`

	rows, err := h.db.QueryContext(ctx, query)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var channel models.Channel
		if err := rows.Scan(&channel.ID, &channel.Name, &channel.Type, &channel.Config, &channel.Status); err != nil {
			continue
		}

		// Parse config to check slug
		var config models.ChannelConfig
		if channel.Config != "" {
			if err := json.Unmarshal([]byte(channel.Config), &config); err != nil {
				continue
			}
		}

		if config.PortalSlug == slug && config.PortalEnabled {
			return &channel, &config, nil
		}
	}

	return nil, nil, sql.ErrNoRows
}

// RequestMagicLink handles POST /portal/{slug}/auth/request
// Sends a magic link email to the portal customer
func (h *PortalAuthHandler) RequestMagicLink(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Find portal
	channel, _, err := h.findPortalBySlug(ctx, slug)
	if err != nil {
		// Always return success to prevent email enumeration
		slog.Debug("portal not found", slog.String("component", "portal_auth"), slog.String("slug", slug))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "If your email is registered, you will receive a sign-in link shortly.",
		})
		return
	}

	// Parse request body
	var request struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	email := strings.TrimSpace(strings.ToLower(request.Email))
	if email == "" {
		http.Error(w, "Email is required", http.StatusBadRequest)
		return
	}

	// Find portal customer by email
	customerID, customerName, err := h.magicLinkService.GetPortalCustomerByEmail(email)
	if err != nil {
		// Always return success to prevent email enumeration
		slog.Debug("portal customer not found", slog.String("component", "portal_auth"), slog.String("email", email))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "If your email is registered, you will receive a sign-in link shortly.",
		})
		return
	}

	// Generate magic link
	token, err := h.magicLinkService.GenerateMagicLink(customerID, &channel.ID)
	if err != nil {
		slog.Error("failed to generate magic link", slog.String("component", "portal_auth"), slog.Any("error", err))
		// Still return success to prevent enumeration
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
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
	_, _, err := h.findPortalBySlug(ctx, slug)
	if err != nil {
		http.Error(w, "Portal not found", http.StatusNotFound)
		return
	}

	if token == "" {
		http.Error(w, "Token is required", http.StatusBadRequest)
		return
	}

	// Validate magic link
	result, err := h.magicLinkService.ValidateMagicLink(token)
	if err != nil {
		slog.Warn("magic link validation failed", slog.String("component", "portal_auth"), slog.Any("error", err))

		var message string
		var statusCode int
		switch err {
		case services.ErrMagicLinkExpired:
			message = "This link has expired. Please request a new sign-in link."
			statusCode = http.StatusUnauthorized
		case services.ErrMagicLinkAlreadyUsed:
			message = "This link has already been used. Please request a new sign-in link."
			statusCode = http.StatusUnauthorized
		case services.ErrMagicLinkInvalid:
			message = "This link is invalid. Please request a new sign-in link."
			statusCode = http.StatusUnauthorized
		default:
			message = "Failed to verify link. Please try again."
			statusCode = http.StatusInternalServerError
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": message,
		})
		return
	}

	// Create portal session
	clientIP := h.getClientIP(r)
	userAgent := r.Header.Get("User-Agent")

	session, err := h.portalSessionManager.CreatePortalSession(result.PortalCustomerID, clientIP, userAgent)
	if err != nil {
		slog.Error("failed to create portal session", slog.String("component", "portal_auth"), slog.Any("error", err))
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	// Set session cookie
	if err := h.portalSessionManager.SetPortalSessionCookie(w, r, session.Token); err != nil {
		slog.Error("failed to set portal session cookie", slog.String("component", "portal_auth"), slog.Any("error", err))
		http.Error(w, "Failed to set session cookie", http.StatusInternalServerError)
		return
	}

	slog.Info("portal customer authenticated", slog.String("component", "portal_auth"), slog.Int("portal_customer_id", result.PortalCustomerID), slog.String("email", result.CustomerEmail))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
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
		http.Error(w, "Portal not found", http.StatusNotFound)
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Successfully logged out",
	})
}

// GetCurrentCustomer handles GET /portal/{slug}/auth/me
// Returns the current authenticated portal customer
func (h *PortalAuthHandler) GetCurrentCustomer(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Find portal
	_, _, err := h.findPortalBySlug(ctx, slug)
	if err != nil {
		http.Error(w, "Portal not found", http.StatusNotFound)
		return
	}

	// Get session token
	token, err := h.portalSessionManager.GetPortalSessionFromRequest(r)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"authenticated": false,
		})
		return
	}

	// Validate session
	session, err := h.portalSessionManager.ValidatePortalSession(token)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"authenticated": false,
			"error":         "Session expired or invalid",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"authenticated": true,
		"customer": map[string]interface{}{
			"id":    session.Customer.ID,
			"email": session.Customer.Email,
			"name":  session.Customer.Name,
		},
	})
}
