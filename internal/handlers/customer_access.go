package handlers

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"windshift/internal/auth"
	"windshift/internal/logger"
	"windshift/internal/middleware"
	"windshift/internal/models"
	"windshift/internal/portalwebauthn"
	"windshift/internal/repository"
	"windshift/internal/services"
	"windshift/internal/sso"
	"windshift/internal/utils"
)

const customerSessionCookie = "windshift_customer_session"

type CustomerAccessHandler struct {
	repo           *repository.CustomerAccessRepository
	providers      *sso.ProviderStore
	customers      *repository.PortalCustomerRepository
	channels       *repository.ChannelRepository
	collections    *repository.CollectionRepository
	permissions    *services.PermissionService
	scope          *services.PublicBoardScopeService
	portalSessions *auth.PortalSessionManager
	portalAuth     *PortalAuthHandler
	passkeys       *PortalWebAuthnHandler
	magicLinks     *services.MagicLinkService
	ip             *utils.IPExtractor
	secure         bool
	limiter        interface{ AllowRequest(*http.Request) bool }
	publicBoards   services.PublicBoardAvailability
	sharing        *services.CollectionApplicationService
	boardViewer    *PublicBoardHandler
}

func (h *CustomerAccessHandler) SetLimiter(limiter interface{ AllowRequest(*http.Request) bool }) {
	h.limiter = limiter
}

func (h *CustomerAccessHandler) allowLogin(w http.ResponseWriter, r *http.Request) bool {
	if h.limiter != nil && !h.limiter.AllowRequest(r) {
		respondJSON(w, http.StatusTooManyRequests, map[string]any{"error": "Too many sign-in attempts", "code": "RATE_LIMIT_EXCEEDED"})
		return false
	}
	return true
}

type CustomerAccessDependencies struct {
	Access         *repository.CustomerAccessRepository
	Providers      *sso.ProviderStore
	Customers      *repository.PortalCustomerRepository
	Channels       *repository.ChannelRepository
	Collections    *repository.CollectionRepository
	Permissions    *services.PermissionService
	Scope          *services.PublicBoardScopeService
	PortalSessions *auth.PortalSessionManager
	PortalAuth     *PortalAuthHandler
	Passkeys       *PortalWebAuthnHandler
	MagicLinks     *services.MagicLinkService
	IP             *utils.IPExtractor
	Secure         bool
	PublicBoards   services.PublicBoardAvailability
	Sharing        *services.CollectionApplicationService
	BoardViewer    *PublicBoardHandler
}

func NewCustomerAccessHandler(deps CustomerAccessDependencies) *CustomerAccessHandler {
	return &CustomerAccessHandler{repo: deps.Access, providers: deps.Providers, customers: deps.Customers, channels: deps.Channels, collections: deps.Collections, permissions: deps.Permissions, scope: deps.Scope, portalSessions: deps.PortalSessions, portalAuth: deps.PortalAuth, passkeys: deps.Passkeys, magicLinks: deps.MagicLinks, ip: deps.IP, secure: deps.Secure, publicBoards: deps.PublicBoards, sharing: deps.Sharing, boardViewer: deps.BoardViewer}
}

func customerSurfacePath(surface models.CustomerSurface) string {
	return "/" + surface.Kind + "/" + url.PathEscape(surface.Slug)
}

func customerOpaqueToken() (string, error) {
	var data [32]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data[:]), nil
}

func (h *CustomerAccessHandler) resolve(w http.ResponseWriter, r *http.Request) (*models.CustomerSurface, bool) {
	if r.PathValue("kind") == "board" && h.publicBoards != nil && !h.publicBoards.PublicBoardsEnabled() {
		respondNotFound(w, r, "customer resource")
		return nil, false
	}
	surface, err := h.repo.Resolve(r.Context(), r.PathValue("kind"), r.PathValue("slug"))
	if err != nil {
		respondNotFound(w, r, "customer resource")
		return nil, false
	}
	return surface, true
}

func (h *CustomerAccessHandler) providerAllowed(ctx context.Context, p *sso.SSOProvider, surface *models.CustomerSurface) bool {
	if surface == nil || !p.Enabled || p.ProviderType != sso.ProviderTypeGitHub || p.AccountType != "customer" {
		return false
	}
	policy, err := h.repo.Policy(ctx, surface.Kind, surface.ID)
	if err != nil || (policy.Mode != "public" && (p.CustomerOrganisationID == nil || !slices.Contains(policy.OrganisationIDs, *p.CustomerOrganisationID))) {
		return false
	}
	return true
}

func (h *CustomerAccessHandler) customer(r *http.Request, surface models.CustomerSurface) int {
	if surface.Kind == "portal" && h.portalSessions != nil {
		if token, err := h.portalSessions.GetPortalSessionFromRequest(r); err == nil {
			if session, err := h.portalSessions.ValidatePortalSession(token, h.ip.GetClientIP(r)); err == nil && session.ChannelID != nil && *session.ChannelID == surface.ID {
				return session.PortalCustomerID
			}
		}
	}
	cookie, err := r.Cookie(customerSessionCookie)
	if err != nil {
		return 0
	}
	id, err := h.repo.SessionCustomer(r.Context(), cookie.Value, surface, time.Now())
	if err != nil {
		return 0
	}
	allowed, err := h.repo.Allowed(r.Context(), surface, id)
	if err != nil || !allowed {
		return 0
	}
	return id
}

func (h *CustomerAccessHandler) Status(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Add("Vary", "Cookie")
	surface, ok := h.resolve(w, r)
	if !ok {
		return
	}
	policy, err := h.repo.Policy(r.Context(), surface.Kind, surface.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	id := h.customer(r, *surface)
	result := models.CustomerAccessStatus{Restricted: policy.Mode == "organisations", Authenticated: id > 0, Allowed: policy.Mode == "public", Providers: []models.CustomerLoginProvider{}, Passkeys: h.passkeys != nil}
	if id > 0 {
		result.Allowed, err = h.repo.Allowed(r.Context(), *surface, id)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
	}
	providers, err := h.providers.ListEnabled()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	for _, p := range providers {
		if h.providerAllowed(r.Context(), p, surface) {
			result.Providers = append(result.Providers, models.CustomerLoginProvider{Name: p.Name, Slug: p.Slug, ProviderType: p.ProviderType})
		}
	}
	respondJSONOK(w, result)
}

func (h *CustomerAccessHandler) managementSurface(w http.ResponseWriter, r *http.Request) (*models.CustomerSurface, bool) {
	user := utils.GetCurrentUser(r)
	if user == nil {
		respondUnauthorized(w, r)
		return nil, false
	}
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return nil, false
	}
	surface, err := h.repo.Surface(r.Context(), r.PathValue("kind"), id)
	if err != nil {
		respondNotFound(w, r, "customer resource")
		return nil, false
	}
	admin, err := h.permissions.IsSystemAdmin(user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return nil, false
	}
	allowed := admin
	if !allowed && surface.Kind == "portal" {
		allowed, err = h.channels.UserCanManage(r.Context(), user.ID, id)
	}
	if !allowed && surface.Kind == "board" {
		ownerID, ownerErr := h.collections.GetOwnerID(id)
		err = ownerErr
		allowed = err == nil && ownerID != nil && *ownerID == user.ID
	}
	if err != nil || !allowed {
		respondForbidden(w, r)
		return nil, false
	}
	return surface, true
}

func (h *CustomerAccessHandler) GetPolicy(w http.ResponseWriter, r *http.Request) {
	surface, ok := h.managementSurface(w, r)
	if !ok {
		return
	}
	policy, err := h.repo.Policy(r.Context(), surface.Kind, surface.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, policy)
}

func (h *CustomerAccessHandler) PutPolicy(w http.ResponseWriter, r *http.Request) {
	surface, ok := h.managementSurface(w, r)
	if !ok {
		return
	}
	policy, ok := decodeJSON[models.CustomerAccessPolicy](w, r)
	if !ok {
		return
	}
	if surface.Kind == "board" {
		user := utils.GetCurrentUser(r)
		service := h.sharing
		if service == nil {
			respondInternalError(w, r, errors.New("board sharing service is unavailable"))
			return
		}
		current, err := service.GetBoardSharing(user.ID, surface.ID)
		if err == nil {
			_, err = service.SaveBoardSharing(services.AuditActor{UserID: user.ID, Username: user.Username, IPAddress: utils.GetClientIP(r), UserAgent: r.UserAgent()}, surface.ID,
				services.BoardSharingSave{Revision: current.Revision, Enabled: current.Enabled, StandaloneEnabled: current.StandaloneEnabled, Slug: current.Slug, MilestoneSwimlanes: current.MilestoneSwimlanes, Policy: policy})
		}
		if err != nil {
			var validation *services.CollectionValidationError
			switch {
			case errors.Is(err, services.ErrCollectionForbidden):
				respondForbidden(w, r)
			case errors.Is(err, services.ErrCollectionConflict):
				respondJSON(w, http.StatusConflict, map[string]any{"error": "Sharing changed; reload before saving", "code": "CONFLICT"})
			case errors.As(err, &validation):
				respondValidationError(w, r, validation.Error())
			default:
				respondInternalError(w, r, err)
			}
			return
		}
		h.GetPolicy(w, r)
		return
	}
	if err := h.repo.SavePolicy(r.Context(), *surface, policy); err != nil {
		if errors.Is(err, repository.ErrCustomerAccessPolicyInvalid) {
			respondValidationError(w, r, err.Error())
		} else {
			respondInternalError(w, r, err)
		}
		return
	}
	action, resource := logger.ActionChannelUpdate, logger.ResourceChannel
	if surface.Kind == "board" {
		action, resource = logger.ActionCollectionUpdate, logger.ResourceCollection
	}
	logger.Record(r, action, resource, &surface.ID, surface.Kind, map[string]any{"customer_access": policy})
	h.GetPolicy(w, r)
}

// Guard applies the same organisation gate to all legacy customer-facing reads and writes.
func (h *CustomerAccessHandler) Guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		kind, slug := "", ""
		if len(parts) >= 3 && parts[0] == "api" && parts[1] == "portal" {
			kind, slug = "portal", parts[2]
			if len(parts) >= 5 && parts[3] == "auth" && (parts[4] == "request" || parts[4] == "verify" || parts[4] == "logout" || parts[4] == "webauthn") {
				next.ServeHTTP(w, r)
				return
			}
		} else if len(parts) >= 4 && parts[0] == "api" && parts[1] == "public" && parts[2] == "board" {
			kind, slug = "board", parts[3]
		}
		if kind == "" {
			next.ServeHTTP(w, r)
			return
		}
		if kind == "board" && h.publicBoards != nil && !h.publicBoards.PublicBoardsEnabled() {
			respondNotFound(w, r, "customer resource")
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Add("Vary", "Cookie")
		surface, err := h.repo.Resolve(r.Context(), kind, slug)
		if err != nil {
			respondNotFound(w, r, "customer resource")
			return
		}
		if kind == "portal" {
			channel, err := h.channels.FindByID(r.Context(), surface.ID)
			if err != nil {
				respondInternalError(w, r, err)
				return
			}
			var config models.ChannelConfig
			if err := json.Unmarshal([]byte(channel.Config), &config); err != nil {
				respondInternalError(w, r, err)
				return
			}
			module := ""
			if len(parts) > 3 {
				switch parts[3] {
				case "my-requests", "requests":
					module = "requests"
					// Only item context is shared with approvals; the handler still
					// requires active approver access when requests are disabled.
					if parts[3] == "requests" && len(parts) == 5 && r.Method == http.MethodGet && config.PortalModuleEnabled("approvals") {
						module = ""
					}
				case "request-types", "submit", "custom-fields":
					module = "forms"
				case "drafts":
					module = "drafts"
				case "approvals":
					module = "approvals"
				case "knowledge-base":
					module = "knowledge-base"
				case "asset-reports", "assets":
					module = "asset-reports"
				case "credentials", "passkey-prompt":
					module = "profile"
				case "auth":
					if r.Method == http.MethodPut && len(parts) > 4 && parts[4] == "me" {
						module = "profile"
					}
				}
			}
			if module != "" && !config.PortalModuleEnabled(module) {
				respondForbidden(w, r)
				return
			}
		}
		policy, err := h.repo.Policy(r.Context(), kind, surface.ID)
		if err != nil {
			respondInternalError(w, r, err)
			return
		}
		if policy.Mode == "public" {
			next.ServeHTTP(w, r)
			return
		}
		id := h.customer(r, *surface)
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
		ctx := r.Context()
		if kind == "portal" {
			token, e := h.portalSessions.GetPortalSessionFromRequest(r)
			if e != nil {
				respondUnauthorized(w, r)
				return
			}
			session, e := h.portalSessions.ValidatePortalSession(token, h.ip.GetClientIP(r))
			if e != nil || session.ChannelID == nil || *session.ChannelID != surface.ID {
				respondUnauthorized(w, r)
				return
			}
			ctx = context.WithValue(ctx, middleware.ContextKeyUser, (*models.User)(nil))
			ctx = context.WithValue(ctx, middleware.ContextKeySession, (*auth.Session)(nil))
			ctx = context.WithValue(ctx, middleware.ContextKeyPortalCustomerID, id)
			ctx = context.WithValue(ctx, middleware.ContextKeyPortalSession, session)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (h *CustomerAccessHandler) issueSession(w http.ResponseWriter, r *http.Request, surface models.CustomerSurface, id int) error {
	allowed, err := h.repo.Allowed(r.Context(), surface, id)
	if err != nil {
		return err
	}
	if !allowed {
		return repository.ErrCustomerAccessDenied
	}
	if surface.Kind == "portal" {
		_, email, err := h.customers.GetIdentity(r.Context(), id)
		if err != nil {
			return err
		}
		if err := h.portalAdmission(r.Context(), surface, email); err != nil {
			return err
		}
		if _, err := h.customers.EnsureChannelAccess(id, surface.ID); err != nil {
			return err
		}
		session, err := h.portalSessions.CreatePortalSession(id, surface.ID, h.ip.GetClientIP(r), r.UserAgent())
		if err != nil {
			return err
		}
		return h.portalSessions.SetPortalSessionCookie(w, r, session.Token)
	}
	token, err := customerOpaqueToken()
	if err != nil {
		return err
	}
	if err := h.repo.StoreSession(r.Context(), token, surface, id, time.Now().Add(auth.PortalSessionDuration)); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: customerSessionCookie, Value: token, Path: "/", HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteLaxMode, MaxAge: int(auth.PortalSessionDuration.Seconds())}) // #nosec G124 -- Secure is enabled for HTTPS and configured HTTPS reverse proxies.
	return nil
}

func portalAdmissionRequired(config models.ChannelConfig) bool {
	return config.PortalRegistrationMode != "" && config.PortalRegistrationMode != "open" || len(config.PortalAllowedDomains) > 0
}

// Check current registration rules for customer sign-in and portal board reads.
func (h *CustomerAccessHandler) portalAdmission(ctx context.Context, surface models.CustomerSurface, email string) error {
	if surface.Kind != "portal" {
		return nil
	}
	channel, err := h.channels.FindByID(ctx, surface.ID)
	if err != nil {
		return err
	}
	var config models.ChannelConfig
	if err := json.Unmarshal([]byte(channel.Config), &config); err != nil {
		return err
	}
	if config.PortalRegistrationMode == "manual" {
		has, err := h.portalAuth.portalAuthRepo.CustomerEmailHasChannelAccess(ctx, email, surface.ID)
		if err != nil {
			return err
		}
		if !has {
			return repository.ErrCustomerAccessDenied
		}
	} else if config.PortalRegistrationMode != "" && config.PortalRegistrationMode != "open" {
		return repository.ErrCustomerAccessDenied
	}
	if len(config.PortalAllowedDomains) > 0 {
		at := strings.LastIndex(email, "@")
		if at < 0 || !slices.ContainsFunc(config.PortalAllowedDomains, func(domain string) bool { return strings.EqualFold(strings.TrimSpace(domain), email[at+1:]) }) {
			return repository.ErrCustomerAccessDenied
		}
	}
	return nil
}

func (h *CustomerAccessHandler) completeGitHub(w http.ResponseWriter, r *http.Request, p *sso.SSOProvider, claims *sso.OIDCClaims, state *repository.SSOStateToken) {
	surface, err := h.repo.Surface(r.Context(), state.ResourceKind, state.ResourceID)
	if err == nil {
		surface, err = h.repo.Resolve(r.Context(), surface.Kind, surface.Slug)
	}
	if err != nil || surface.ID != state.ResourceID || !h.providerAllowed(r.Context(), p, surface) || !claims.EmailVerified || !claims.EmailVerifiedProvided || claims.Email == "" {
		respondForbidden(w, r)
		return
	}
	err = h.portalAdmission(r.Context(), *surface, claims.Email)
	var id int
	if err == nil {
		id, err = h.repo.GitHubCustomer(r.Context(), p.ID, claims.Subject, claims.Email, claims.Name, p.CustomerOrganisationID, p.AutoProvisionUsers)
	}
	if err == nil {
		err = h.issueSession(w, r, *surface, id)
	}
	if err != nil {
		http.Redirect(w, r, requestContextPrefix(r)+customerSurfacePath(*surface)+"?customer_login_error="+url.QueryEscape("Customer account is not permitted. Contact your administrator."), http.StatusFound) // #nosec G710 -- fixed local resource prefix and escaped database slug.
		return
	}
	http.Redirect(w, r, requestContextPrefix(r)+customerSurfacePath(*surface), http.StatusFound) // #nosec G710 -- fixed local resource prefix and escaped database slug.
}

func (h *CustomerAccessHandler) RequestMagicLink(w http.ResponseWriter, r *http.Request) {
	if !h.allowLogin(w, r) {
		return
	}
	surface, ok := h.resolve(w, r)
	if !ok {
		return
	}
	if surface.Kind == "portal" {
		h.portalAuth.RequestMagicLink(w, r)
		return
	}
	req, ok := decodeJSON[models.CustomerLoginRequest](w, r)
	if !ok {
		return
	}
	id, err := h.customers.FindIDByEmail(strings.TrimSpace(req.Email))
	if err == nil {
		allowed, e := h.repo.Allowed(r.Context(), *surface, id)
		if e == nil && allowed {
			token, e := customerOpaqueToken()
			if e == nil {
				e = h.repo.StoreToken(r.Context(), token, "magic", *surface, &id, time.Now().Add(services.MagicLinkExpiry))
			}
			if e == nil {
				name, email, e := h.customers.GetIdentity(r.Context(), id)
				if e == nil && h.magicLinks != nil {
					if e = h.magicLinks.SendBoardMagicLinkEmail(email, name, token, surface.Slug, "en"); e != nil {
						slog.Warn("customer sign-in email delivery failed", "component", "customer_access")
					}
				}
			}
		}
	}
	respondJSONOK(w, models.CustomerLoginResponse{Success: true, Message: "If your email is registered, you will receive a sign-in link shortly."})
}

func (h *CustomerAccessHandler) VerifyMagicLink(w http.ResponseWriter, r *http.Request) {
	if !h.allowLogin(w, r) {
		return
	}
	surface, ok := h.resolve(w, r)
	if !ok {
		return
	}
	if surface.Kind != "board" {
		respondNotFound(w, r, "customer login")
		return
	}
	req, ok := decodeJSON[models.CustomerLoginRequest](w, r)
	if !ok {
		return
	}
	id, err := h.repo.ConsumeToken(r.Context(), req.Token, "magic", *surface, time.Now())
	if err == nil {
		err = h.issueSession(w, r, *surface, id)
	}
	if err != nil {
		respondUnauthorized(w, r)
		return
	}
	respondJSONOK(w, models.CustomerLoginResponse{Success: true})
}

func (h *CustomerAccessHandler) StartPasskey(w http.ResponseWriter, r *http.Request) {
	if !h.allowLogin(w, r) {
		return
	}
	surface, ok := h.resolve(w, r)
	if !ok {
		return
	}
	if h.passkeys == nil {
		respondNotFound(w, r, "passkey login")
		return
	}
	if surface.Kind == "portal" {
		h.passkeys.StartPortalLogin(w, r)
		return
	}
	options, data, err := h.passkeys.config.WebAuthn().BeginDiscoverableLogin()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	raw, err := json.Marshal(data)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	id, err := customerOpaqueToken()
	if err == nil {
		err = h.repo.StoreChallenge(r.Context(), id, *surface, string(raw), time.Now().Add(5*time.Minute))
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	respondJSONOK(w, models.CustomerLoginResponse{Success: true, PublicKey: options.Response, SessionID: id})
}

func (h *CustomerAccessHandler) CompletePasskey(w http.ResponseWriter, r *http.Request) {
	if !h.allowLogin(w, r) {
		return
	}
	surface, ok := h.resolve(w, r)
	if !ok {
		return
	}
	if h.passkeys == nil {
		respondNotFound(w, r, "passkey login")
		return
	}
	if surface.Kind == "portal" {
		h.passkeys.CompletePortalLogin(w, r)
		return
	}
	req, ok := decodeJSON[models.CustomerLoginRequest](w, r)
	if !ok {
		return
	}
	payload, err := h.repo.ConsumeChallenge(r.Context(), req.SessionID, *surface, time.Now())
	if err != nil {
		respondUnauthorized(w, r)
		return
	}
	var data webauthn.SessionData
	if err := json.Unmarshal([]byte(payload), &data); err != nil {
		respondInternalError(w, r, err)
		return
	}
	raw, err := json.Marshal(req.Response)
	if err != nil {
		respondValidationError(w, r, "Invalid credential response")
		return
	}
	parsed, err := protocol.ParseCredentialRequestResponseBody(bytes.NewReader(raw))
	if err != nil {
		respondValidationError(w, r, "Invalid credential response")
		return
	}
	var customerID int
	resolver := func(_ []byte, handle []byte) (webauthn.User, error) {
		id, err := strconv.Atoi(string(handle))
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("invalid customer identity")
		}
		allowed, err := h.repo.Allowed(r.Context(), *surface, id)
		if err != nil || !allowed {
			return nil, repository.ErrCustomerAccessDenied
		}
		customer, err := h.passkeys.lookupStore.GetCustomer(id)
		if err != nil {
			return nil, err
		}
		credentials, err := h.passkeys.credentialStore.GetCustomerCredentials(id)
		if err != nil {
			return nil, err
		}
		subject := portalwebauthn.NewSubject(customer)
		subject.SetCredentials(credentials)
		customerID = id
		return subject, nil
	}
	credential, err := h.passkeys.config.WebAuthn().ValidateDiscoverableLogin(resolver, data, parsed)
	if err != nil || customerID <= 0 {
		respondUnauthorized(w, r)
		return
	}
	if err := h.passkeys.credentialStore.UpdateCredentialCounter(credential.ID, credential.Authenticator.SignCount, credential.Authenticator.CloneWarning); err != nil {
		respondInternalError(w, r, err)
		return
	}
	if credential.Authenticator.CloneWarning {
		respondUnauthorized(w, r)
		return
	}
	if err := h.issueSession(w, r, *surface, customerID); err != nil {
		respondUnauthorized(w, r)
		return
	}
	respondJSONOK(w, models.CustomerLoginResponse{Success: true})
}

func (h *CustomerAccessHandler) Logout(w http.ResponseWriter, r *http.Request) {
	surface, ok := h.resolve(w, r)
	if !ok {
		return
	}
	if surface.Kind == "portal" {
		h.portalAuth.Logout(w, r)
		return
	}
	if cookie, err := r.Cookie(customerSessionCookie); err == nil {
		_ = h.repo.DeleteSession(r.Context(), cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: customerSessionCookie, Path: "/", HttpOnly: true, Secure: h.secure, SameSite: http.SameSiteLaxMode, MaxAge: -1}) // #nosec G124 -- clears the cookie with the same HTTPS configuration used on issuance.
	respondJSONOK(w, models.CustomerLoginResponse{Success: true})
}
