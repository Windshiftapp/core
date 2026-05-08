package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"windshift/internal/auth"
	"windshift/internal/middleware"
	"windshift/internal/portalwebauthn"
	"windshift/internal/utils"
)

const (
	maxPortalCredentialsPerCustomer = 10
	maxPortalCredentialNameLen      = 100
)

// PortalWebAuthnHandler exposes passkey enrolment, listing, removal, and
// passwordless sign-in for portal customers. The handler is intentionally
// kept disjoint from WebAuthnHandler: portal credentials live in their own
// tables and are tied to portal_customers, not users.
//
// All DB access goes through the portalwebauthn package's stores; the
// database.Database handle is kept only to wire those stores at construction.
type PortalWebAuthnHandler struct {
	portalSessionManager *auth.PortalSessionManager
	config               *portalwebauthn.Config
	sessionStore         *portalwebauthn.SessionStore
	credentialStore      *portalwebauthn.CredentialStore
	lookupStore          *portalwebauthn.PortalLookupStore
	ipExtractor          *utils.IPExtractor
}

// NewPortalWebAuthnHandler wires the portal-passkey handler with the
// portalwebauthn package's repository-shaped stores.
func NewPortalWebAuthnHandler(
	portalSessionManager *auth.PortalSessionManager,
	config *portalwebauthn.Config,
	sessionStore *portalwebauthn.SessionStore,
	credentialStore *portalwebauthn.CredentialStore,
	lookupStore *portalwebauthn.PortalLookupStore,
	ipExtractor *utils.IPExtractor,
) *PortalWebAuthnHandler {
	return &PortalWebAuthnHandler{
		portalSessionManager: portalSessionManager,
		config:               config,
		sessionStore:         sessionStore,
		credentialStore:      credentialStore,
		lookupStore:          lookupStore,
		ipExtractor:          ipExtractor,
	}
}

// ----- helpers -----

// requirePortalCustomer extracts the authenticated portal customer ID from the
// request context. Returns 0,false if the request was authenticated as an
// internal user instead of a portal customer (the RequirePortalAuth middleware
// accepts both, but portal-passkey endpoints are customer-only).
func requirePortalCustomer(w http.ResponseWriter, r *http.Request) (int, bool) {
	if id, ok := r.Context().Value(middleware.ContextKeyPortalCustomerID).(int); ok && id > 0 {
		return id, true
	}
	respondBadRequest(w, r, "Portal passkeys must be managed while signed in as a portal customer.")
	return 0, false
}

// ----- registration -----

type portalRegistrationStartRequest struct {
	CredentialName string `json:"credential_name"`
}

type portalRegistrationCompleteRequest struct {
	SessionID      string      `json:"sessionId"`
	CredentialName string      `json:"credentialName"`
	Response       interface{} `json:"response"`
}

// StartPortalRegistration begins enrolment of a new passkey for the
// authenticated portal customer.
func (h *PortalWebAuthnHandler) StartPortalRegistration(w http.ResponseWriter, r *http.Request) {
	customerID, ok := requirePortalCustomer(w, r)
	if !ok {
		return
	}
	req, ok := decodeJSON[portalRegistrationStartRequest](w, r)
	if !ok {
		return
	}
	name := strings.TrimSpace(req.CredentialName)
	if name == "" {
		respondValidationError(w, r, "Credential name is required")
		return
	}
	if utf8.RuneCountInString(name) > maxPortalCredentialNameLen {
		respondValidationError(w, r, "Credential name is too long")
		return
	}

	customer, err := h.lookupStore.GetCustomer(customerID)
	if err != nil {
		if errors.Is(err, portalwebauthn.ErrPortalCustomerNotFound) {
			respondNotFound(w, r, "portal_customer")
			return
		}
		respondInternalError(w, r, err)
		return
	}

	subject := portalwebauthn.NewSubject(customer)
	existing, err := h.credentialStore.GetCustomerCredentials(customerID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if len(existing) >= maxPortalCredentialsPerCustomer {
		respondValidationError(w, r, "Maximum number of passkeys reached")
		return
	}
	subject.SetCredentials(existing)

	options, sessionData, err := h.config.WebAuthn().BeginRegistration(subject)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	sessionID, err := h.sessionStore.SaveRegistrationSession(customerID, sessionData)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, map[string]interface{}{
		"publicKey": options.Response,
		"sessionId": sessionID,
	})
}

// CompletePortalRegistration verifies the authenticator response and stores
// the new credential.
func (h *PortalWebAuthnHandler) CompletePortalRegistration(w http.ResponseWriter, r *http.Request) {
	customerID, ok := requirePortalCustomer(w, r)
	if !ok {
		return
	}
	req, ok := decodeJSON[portalRegistrationCompleteRequest](w, r)
	if !ok {
		return
	}
	name := strings.TrimSpace(req.CredentialName)
	if name == "" {
		respondValidationError(w, r, "Credential name is required")
		return
	}
	if utf8.RuneCountInString(name) > maxPortalCredentialNameLen {
		respondValidationError(w, r, "Credential name is too long")
		return
	}

	customer, err := h.lookupStore.GetCustomer(customerID)
	if err != nil {
		respondNotFound(w, r, "portal_customer")
		return
	}
	subject := portalwebauthn.NewSubject(customer)

	sessionData, err := h.sessionStore.GetRegistrationSession(req.SessionID, customerID)
	if err != nil {
		respondValidationError(w, r, "Invalid or expired session")
		return
	}

	credentialJSON, err := json.Marshal(req.Response)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(credentialJSON))

	credential, err := h.config.WebAuthn().FinishRegistration(subject, *sessionData, r)
	if err != nil {
		respondValidationError(w, r, "Registration verification failed: "+err.Error())
		return
	}

	exists, err := h.credentialStore.CheckCredentialExists(credential.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if exists {
		respondConflict(w, r, "Credential already registered")
		return
	}

	if err := h.credentialStore.SaveCredential(customerID, name, credential); err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONCreated(w, map[string]interface{}{
		"status":  "success",
		"message": "Passkey registered successfully",
		"credential": map[string]interface{}{
			"id":              credential.ID,
			"name":            name,
			"attestationType": credential.AttestationType,
			"transport":       credential.Transport,
		},
	})
}

// ----- discoverable login -----

type portalLoginCompleteRequest struct {
	SessionID string      `json:"sessionId"`
	Response  interface{} `json:"response"`
}

// StartPortalLogin issues a discoverable-login challenge. The slug must
// resolve to an enabled portal channel; the customer is identified at finish
// time from the userHandle returned by the authenticator.
func (h *PortalWebAuthnHandler) StartPortalLogin(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	_, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if _, err := h.lookupStore.FindEnabledPortalChannelBySlug(slug); err != nil {
		respondNotFound(w, r, "portal")
		return
	}

	options, sessionData, err := h.config.WebAuthn().BeginDiscoverableLogin()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	sessionID, err := h.sessionStore.SaveAuthenticationSession(nil, sessionData)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	respondJSONOK(w, map[string]interface{}{
		"publicKey": options.Response,
		"sessionId": sessionID,
	})
}

// CompletePortalLogin verifies the discoverable-login response, looks up the
// owning portal customer, enforces channel access, and creates a portal session.
func (h *PortalWebAuthnHandler) CompletePortalLogin(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	_, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	channel, err := h.lookupStore.FindEnabledPortalChannelBySlug(slug)
	if err != nil {
		respondNotFound(w, r, "portal")
		return
	}

	req, ok := decodeJSON[portalLoginCompleteRequest](w, r)
	if !ok {
		return
	}

	sessionData, err := h.sessionStore.GetAuthenticationSession(req.SessionID)
	if err != nil {
		respondValidationError(w, r, "Invalid or expired session")
		return
	}

	credentialJSON, err := json.Marshal(req.Response)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	parsed, err := protocol.ParseCredentialRequestResponseBody(bytes.NewReader(credentialJSON))
	if err != nil {
		respondValidationError(w, r, "Invalid credential response")
		return
	}

	// Resolver: look up the owning portal customer from the userHandle the
	// authenticator presented. We then verify channel access for this slug
	// before letting the library finalize the assertion.
	var resolvedCustomer *auth.PortalCustomer
	resolver := func(rawID, userHandle []byte) (webauthn.User, error) {
		if len(userHandle) == 0 {
			return nil, fmt.Errorf("missing user handle")
		}
		customerID, err := strconv.Atoi(string(userHandle))
		if err != nil {
			return nil, fmt.Errorf("invalid user handle: %w", err)
		}
		customer, err := h.lookupStore.GetCustomer(customerID)
		if err != nil {
			return nil, fmt.Errorf("customer not found: %w", err)
		}
		hasAccess, err := h.lookupStore.CustomerHasChannelAccess(customerID, channel.ID)
		if err != nil {
			return nil, fmt.Errorf("channel access check failed: %w", err)
		}
		if !hasAccess {
			return nil, fmt.Errorf("customer lacks access to this portal")
		}
		resolvedCustomer = customer
		subject := portalwebauthn.NewSubject(customer)
		creds, err := h.credentialStore.GetCustomerCredentials(customerID)
		if err != nil {
			return nil, fmt.Errorf("failed to load credentials: %w", err)
		}
		subject.SetCredentials(creds)
		return subject, nil
	}

	credential, err := h.config.WebAuthn().ValidateDiscoverableLogin(resolver, *sessionData, parsed)
	if err != nil {
		slog.Warn("portal passkey login failed",
			slog.String("component", "portal_webauthn"),
			slog.String("slug", slug),
			slog.Any("error", err))
		respondUnauthorized(w, r)
		return
	}

	// Persist sign count and clone flag — needed even when we reject below.
	if err := h.credentialStore.UpdateCredentialCounter(
		credential.ID,
		credential.Authenticator.SignCount,
		credential.Authenticator.CloneWarning,
	); err != nil {
		slog.Warn("failed to update portal credential counter",
			slog.String("component", "portal_webauthn"), slog.Any("error", err))
	}
	if credential.Authenticator.CloneWarning {
		slog.Warn("rejecting portal passkey login due to clone warning",
			slog.String("component", "portal_webauthn"),
			slog.Int("portal_customer_id", resolvedCustomer.ID))
		respondUnauthorized(w, r)
		return
	}
	if resolvedCustomer == nil {
		respondInternalError(w, r, fmt.Errorf("resolver did not populate customer"))
		return
	}

	clientIP := h.ipExtractor.GetClientIP(r)
	session, err := h.portalSessionManager.CreatePortalSession(resolvedCustomer.ID, clientIP, r.UserAgent())
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if err := h.portalSessionManager.SetPortalSessionCookie(w, r, session.Token); err != nil {
		_ = h.portalSessionManager.DeletePortalSession(session.Token)
		respondInternalError(w, r, err)
		return
	}

	slog.Info("portal customer authenticated via passkey",
		slog.String("component", "portal_webauthn"),
		slog.Int("portal_customer_id", resolvedCustomer.ID),
		slog.String("portal", slug))

	respondJSONOK(w, map[string]interface{}{
		"success": true,
		"message": "Successfully signed in",
		"customer": map[string]interface{}{
			"id":    resolvedCustomer.ID,
			"email": resolvedCustomer.Email,
			"name":  resolvedCustomer.Name,
		},
	})
}

// ----- credential management -----

// GetPortalCredentials lists the authenticated customer's passkeys.
func (h *PortalWebAuthnHandler) GetPortalCredentials(w http.ResponseWriter, r *http.Request) {
	customerID, ok := requirePortalCustomer(w, r)
	if !ok {
		return
	}
	creds, err := h.credentialStore.GetCustomerCredentialsList(customerID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if creds == nil {
		creds = []portalwebauthn.Credential{}
	}
	respondJSONOK(w, creds)
}

// RemovePortalCredential deletes a passkey. The credential must belong to the
// authenticated customer.
func (h *PortalWebAuthnHandler) RemovePortalCredential(w http.ResponseWriter, r *http.Request) {
	customerID, ok := requirePortalCustomer(w, r)
	if !ok {
		return
	}
	credentialID := r.PathValue("credentialId")
	if credentialID == "" {
		respondValidationError(w, r, "Credential ID is required")
		return
	}

	ownerID, err := h.lookupStore.GetCredentialOwner(credentialID)
	if errors.Is(err, portalwebauthn.ErrCredentialNotFound) {
		respondNotFound(w, r, "credential")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if ownerID != customerID {
		respondNotFound(w, r, "credential")
		return
	}

	if err := h.credentialStore.DeleteCredential(credentialID); err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, map[string]string{
		"status":  "success",
		"message": "Passkey deleted successfully",
	})
}

// DismissPasskeyPrompt records that the customer has dismissed the post-login
// "set up a passkey" banner so it doesn't reappear next session.
func (h *PortalWebAuthnHandler) DismissPasskeyPrompt(w http.ResponseWriter, r *http.Request) {
	customerID, ok := requirePortalCustomer(w, r)
	if !ok {
		return
	}
	if err := h.lookupStore.DismissPasskeyPrompt(customerID, time.Now()); err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, map[string]string{"status": "success"})
}
