package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"windshift/internal/auth"
	"windshift/internal/database"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// cliAuthCodeTTL bounds how long an approved code can sit waiting for the
// CLI to redeem it. Short window because the callback is a local redirect —
// anything longer is almost certainly a stale flow.
const cliAuthCodeTTL = 2 * time.Minute

// CLIAuthHandler backs the `ws init` onboarding flow. It combines the
// existing agent-creation and token-minting primitives so a user can click
// "Allow" in the browser and have a ws-cli-* agent + token materialize on
// the machine that started the flow.
type CLIAuthHandler struct {
	db                database.Database
	agent             *AgentHandler
	tokenManager      *auth.TokenManager
	apiToken          *APITokenHandler
	permissionService *services.PermissionService
}

// NewCLIAuthHandler wires the handler. All four deps must be non-nil — the
// flow refuses to register routes otherwise (see routes/users.go).
func NewCLIAuthHandler(db database.Database, agent *AgentHandler, tm *auth.TokenManager, apiToken *APITokenHandler, permService *services.PermissionService) *CLIAuthHandler {
	return &CLIAuthHandler{
		db:                db,
		agent:             agent,
		tokenManager:      tm,
		apiToken:          apiToken,
		permissionService: permService,
	}
}

// Capabilities returns the feature gates a CLI needs to decide between the
// automatic (agent-mint) and manual (personal-token) setup paths. Exposed
// unauthenticated because the CLI runs this before it has any credentials.
func (h *CLIAuthHandler) Capabilities(w http.ResponseWriter, r *http.Request) {
	agentsEnabled := false
	if h.agent != nil {
		agentsEnabled = h.agent.allowUserManagedAgents()
	}

	tokenPolicy := "all_users"
	if h.apiToken != nil {
		tokenPolicy = h.apiToken.getCreationPolicy()
	}
	tokensEnabled := tokenPolicy != "disabled"

	respondJSONOK(w, map[string]interface{}{
		"auto_onboarding_enabled": agentsEnabled && tokensEnabled,
		"manual_tokens_enabled":   tokensEnabled,
		"agents_enabled":          agentsEnabled,
		"token_policy":            tokenPolicy,
	})
}

// ApproveRequest is the payload the consent page POSTs to finalize the flow.
type ApproveRequest struct {
	CallbackURL string   `json:"callback_url"`
	State       string   `json:"state"`
	Hostname    string   `json:"hostname"`
	AgentName   string   `json:"agent_name"`
	FirstName   string   `json:"first_name,omitempty"`
	LastName    string   `json:"last_name,omitempty"`
	Scopes      []string `json:"scopes"`
}

// Approve is called when the user clicks "Allow" on the consent page. It
// creates (or reuses) an agent owned by the current user, mints a token,
// and returns a one-time `code` the CLI can redeem at /exchange.
func (h *CLIAuthHandler) Approve(w http.ResponseWriter, r *http.Request) {
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	req, ok := decodeJSON[ApproveRequest](w, r)
	if !ok {
		return
	}

	if err := validateLoopbackCallback(req.CallbackURL); err != nil {
		respondBadRequest(w, r, err.Error())
		return
	}
	if strings.TrimSpace(req.State) == "" {
		respondBadRequest(w, r, "state is required")
		return
	}
	agentName := sanitizeAgentName(req.AgentName)
	if agentName == "" {
		respondBadRequest(w, r, "invalid agent_name")
		return
	}
	if len(req.Scopes) == 0 {
		respondBadRequest(w, r, "scopes are required")
		return
	}
	if err := auth.ValidateScopes(req.Scopes); err != nil {
		respondBadRequest(w, r, err.Error())
		return
	}
	// Auto-onboarding should never ask for admin-scoped tokens.
	for _, s := range req.Scopes {
		if auth.IsAdminScope(s) {
			respondBadRequest(w, r, "admin scopes are not permitted in CLI onboarding")
			return
		}
	}

	isAdmin, _ := h.permissionService.IsSystemAdmin(currentUser.ID)

	// Enforce token-creation policy before we touch any state.
	if err := h.apiToken.checkCreationPolicy(currentUser.ID); err != nil {
		h.auditApproveFailure(r, currentUser, agentName, "token_policy_disabled")
		respondForbidden(w, r)
		return
	}

	// Find-or-create the per-machine agent.
	existing, err := h.agent.FindOwnedAgentByUsername(currentUser.ID, agentName)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	var agent *models.User
	if existing != nil {
		agent = existing
	} else {
		firstName := strings.TrimSpace(req.FirstName)
		if firstName == "" {
			firstName = "CLI"
		}
		lastName := strings.TrimSpace(req.LastName)
		if lastName == "" {
			lastName = req.Hostname
			if lastName == "" {
				lastName = "agent"
			}
		}
		createReq := CreateAgentRequest{
			Username:  agentName,
			FirstName: firstName,
			LastName:  lastName,
		}
		created, createErr := h.agent.CreateOwnedAgent(currentUser.ID, isAdmin, createReq)
		if createErr != nil {
			switch {
			case errors.Is(createErr, ErrAgentsDisabled):
				h.auditApproveFailure(r, currentUser, agentName, "agents_disabled")
				respondForbidden(w, r)
			case errors.Is(createErr, ErrAgentLimitReached):
				h.auditApproveFailure(r, currentUser, agentName, "max_agents_reached")
				respondForbidden(w, r)
			case errors.Is(createErr, ErrAgentUsernameTaken), errors.Is(createErr, ErrAgentEmailTaken):
				// A username collision on the cli-agent path almost always means
				// another user owns the same username — tell the caller so they
				// can retry with --agent-name.
				respondConflict(w, r, createErr.Error())
			default:
				respondInternalError(w, r, createErr)
			}
			return
		}
		agent = created

		_ = logger.LogAudit(h.db, logger.AuditEvent{
			UserID:       currentUser.ID,
			Username:     currentUser.Username,
			IPAddress:    utils.GetClientIP(r),
			UserAgent:    r.UserAgent(),
			ActionType:   logger.ActionAgentCreate,
			ResourceType: logger.ResourceUser,
			ResourceID:   &agent.ID,
			ResourceName: agent.Username,
			Details: map[string]interface{}{
				"agent_kind":    "owned",
				"origin":        "cli_onboarding",
				"owner_user_id": currentUser.ID,
				"hostname":      req.Hostname,
			},
			Success: true,
		})
	}

	// Mint a token for the agent. Token name surfaces in the owner's Agents
	// tab so they can spot per-machine tokens at a glance.
	tokenName := fmt.Sprintf("ws-cli on %s", firstNonEmpty(req.Hostname, agentName))
	tokenResp, err := h.tokenManager.CreateToken(agent.ID, models.APITokenCreate{
		Name:        tokenName,
		Permissions: req.Scopes,
	})
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	// Persist an approved auth-code row so /exchange can hand the plaintext
	// back to the CLI exactly once.
	code, err := randomOpaqueCode()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	scopesCSV := strings.Join(req.Scopes, ",")
	expires := time.Now().Add(cliAuthCodeTTL)
	if _, err = h.db.Exec(`
		INSERT INTO cli_auth_codes (code, state, callback_url, hostname, agent_name, requested_scopes, status, approved_by_user_id, agent_id, token_id, token_plaintext, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, 'approved', ?, ?, ?, ?, ?)
	`, code, req.State, req.CallbackURL, req.Hostname, agentName, scopesCSV, currentUser.ID, agent.ID, tokenResp.APIToken.ID, tokenResp.Token, expires); err != nil {
		// Best-effort: revoke the just-minted token so we don't leave a
		// live credential stranded on the server.
		_ = h.tokenManager.AdminRevokeToken(tokenResp.APIToken.ID)
		respondInternalError(w, r, err)
		return
	}

	_ = logger.LogAudit(h.db, logger.AuditEvent{
		UserID:       currentUser.ID,
		Username:     currentUser.Username,
		IPAddress:    utils.GetClientIP(r),
		UserAgent:    r.UserAgent(),
		ActionType:   logger.ActionAPITokenCreate,
		ResourceType: logger.ResourceAPIToken,
		ResourceID:   &tokenResp.APIToken.ID,
		ResourceName: tokenResp.APIToken.Name,
		Details: map[string]interface{}{
			"origin":         "cli_onboarding",
			"target_user_id": agent.ID,
			"hostname":       req.Hostname,
			"token_prefix":   tokenResp.APIToken.TokenPrefix,
		},
		Success: true,
	})

	respondJSONOK(w, map[string]interface{}{
		"code":         code,
		"state":        req.State,
		"callback_url": req.CallbackURL,
		"agent": map[string]interface{}{
			"id":       agent.ID,
			"username": agent.Username,
		},
	})
}

// Deny is called when the user declines on the consent page. Purely for
// audit visibility — the browser has everything it needs to redirect back
// to the CLI with result=denied.
func (h *CLIAuthHandler) Deny(w http.ResponseWriter, r *http.Request) {
	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	req, _ := decodeJSON[ApproveRequest](w, r) // best-effort body, optional fields

	_ = logger.LogAudit(h.db, logger.AuditEvent{
		UserID:       currentUser.ID,
		Username:     currentUser.Username,
		IPAddress:    utils.GetClientIP(r),
		UserAgent:    r.UserAgent(),
		ActionType:   "cli_onboarding.deny",
		ResourceType: logger.ResourceUser,
		Details: map[string]interface{}{
			"hostname":   req.Hostname,
			"agent_name": sanitizeAgentName(req.AgentName),
		},
		Success: true,
	})

	w.WriteHeader(http.StatusNoContent)
}

// ExchangeRequest is the payload the CLI POSTs to claim the minted token.
type ExchangeRequest struct {
	Code  string `json:"code"`
	State string `json:"state"`
}

// Exchange redeems the one-time code the browser handed to the CLI. The
// plaintext token is returned exactly once and then cleared on the server.
func (h *CLIAuthHandler) Exchange(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[ExchangeRequest](w, r)
	if !ok {
		return
	}
	if strings.TrimSpace(req.Code) == "" || strings.TrimSpace(req.State) == "" {
		respondBadRequest(w, r, "code and state are required")
		return
	}

	var (
		rowID       int64
		state       string
		status      string
		tokenText   sql.NullString
		agentID     sql.NullInt64
		agentName   string
		hostname    string
		scopes      string
		expiresAt   time.Time
		consumedAt  sql.NullTime
		approvedBy  sql.NullInt64
		callbackURL string
	)
	err := h.db.QueryRow(`
		SELECT id, state, status, token_plaintext, agent_id, agent_name, hostname, requested_scopes, expires_at, consumed_at, approved_by_user_id, callback_url
		FROM cli_auth_codes
		WHERE code = ?
	`, req.Code).Scan(&rowID, &state, &status, &tokenText, &agentID, &agentName, &hostname, &scopes, &expiresAt, &consumedAt, &approvedBy, &callbackURL)
	if errors.Is(err, sql.ErrNoRows) {
		respondNotFound(w, r, "auth code")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if state != req.State {
		respondBadRequest(w, r, "state mismatch")
		return
	}
	if status != "approved" {
		respondBadRequest(w, r, fmt.Sprintf("auth code is %s", status))
		return
	}
	if consumedAt.Valid {
		respondBadRequest(w, r, "auth code already consumed")
		return
	}
	if time.Now().After(expiresAt) {
		// Mark expired so a later garbage-collection pass can clean up.
		_, _ = h.db.Exec(`UPDATE cli_auth_codes SET status = 'expired', token_plaintext = NULL WHERE id = ?`, rowID)
		respondBadRequest(w, r, "auth code expired")
		return
	}
	if !tokenText.Valid || tokenText.String == "" {
		respondInternalError(w, r, fmt.Errorf("auth code has no token payload"))
		return
	}

	// Consume atomically: only the first caller to flip status wins. The
	// UPDATE guard protects against a replay racing a second exchange.
	res, err := h.db.Exec(`
		UPDATE cli_auth_codes
		SET status = 'consumed', consumed_at = CURRENT_TIMESTAMP, token_plaintext = NULL
		WHERE id = ? AND status = 'approved' AND consumed_at IS NULL
	`, rowID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if rows, _ := res.RowsAffected(); rows != 1 {
		respondBadRequest(w, r, "auth code already consumed")
		return
	}

	respondJSONOK(w, map[string]interface{}{
		"token": tokenText.String,
		"agent": map[string]interface{}{
			"id":       agentID.Int64,
			"username": agentName,
		},
		"scopes":   strings.Split(scopes, ","),
		"hostname": hostname,
	})
}

// validateLoopbackCallback rejects anything that isn't http(s)://127.0.0.1
// or http(s)://localhost with a non-empty path — i.e. it keeps the minted
// token inside the user's machine. Preventing open redirect + exfiltration
// to an attacker-controlled URL is the whole point of this check.
func validateLoopbackCallback(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("callback_url is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("callback_url is not a valid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("callback_url must use http or https")
	}
	host := u.Hostname()
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return fmt.Errorf("callback_url must target a loopback address")
	}
	return nil
}

// sanitizeAgentName enforces the same character class as users.username. We
// keep the CLI input narrow so a malicious CLI can't slip control characters
// into the stored username.
func sanitizeAgentName(in string) string {
	in = strings.TrimSpace(in)
	if in == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range in {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-', r == '_', r == '.':
			b.WriteRune(r)
		}
	}
	s := b.String()
	if len(s) < 3 || len(s) > 32 {
		return ""
	}
	return s
}

func randomOpaqueCode() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func (h *CLIAuthHandler) auditApproveFailure(r *http.Request, user *models.User, agentName, reason string) {
	_ = logger.LogAudit(h.db, logger.AuditEvent{
		UserID:       user.ID,
		Username:     user.Username,
		IPAddress:    utils.GetClientIP(r),
		UserAgent:    r.UserAgent(),
		ActionType:   "cli_onboarding.approve",
		ResourceType: logger.ResourceUser,
		ResourceName: agentName,
		Details:      map[string]interface{}{"reason": reason},
		Success:      false,
		ErrorMessage: reason,
	})
}
