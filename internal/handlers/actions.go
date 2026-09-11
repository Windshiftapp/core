package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
	"windshift/internal/services"
	"windshift/internal/utils"
)

// ActionsHandler retains the session-only capability administration surface.
type ActionsHandler struct {
	repo           *repository.ActionRepository
	credentialRepo *repository.ActionCredentialRepository
	auditor        *logger.Auditor
	keyCache       *WorkspaceKeyCache
}

func NewActionsHandler(repo *repository.ActionRepository, credentialRepo *repository.ActionCredentialRepository, auditor *logger.Auditor, keyCache *WorkspaceKeyCache) *ActionsHandler {
	return &ActionsHandler{repo: repo, credentialRepo: credentialRepo, auditor: auditor, keyCache: keyCache}
}

func (h *ActionsHandler) requireCapability(w http.ResponseWriter, r *http.Request) (*models.ActionCapability, bool) {
	capID, ok := requireIDParam(w, r, "capabilityId")
	if !ok {
		return nil, false
	}
	capability, err := h.repo.GetCapabilityByID(capID)
	if err == repository.ErrNotFound {
		respondNotFound(w, r, "capability")
		return nil, false
	}
	if err != nil {
		respondInternalError(w, r, err)
		return nil, false
	}
	return capability, true
}

// GetWorkspaceLogs gets all execution logs for a workspace
func (h *ActionsHandler) GetWorkspaceLogs(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireWorkspaceIDParam(w, r, h.keyCache, "workspaceId")
	if !ok {
		return
	}

	// Parse pagination params
	limit, offset := parseOffsetPagination(r, 50, 100)

	logs, err := h.repo.GetExecutionLogsByWorkspaceID(workspaceID, limit, offset)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if logs == nil {
		logs = []*models.ActionExecutionLog{}
	}

	respondJSONOK(w, logs)
}

func (h *ActionsHandler) isEnabledLLMConnection(connectionID int) bool {
	return h.repo.IsEnabledLLMConnection(connectionID)
}

func (h *ActionsHandler) validateCapabilityConfig(w http.ResponseWriter, r *http.Request, capType models.CapabilityType, configStr string, appliesToAllWorkspaces bool, capabilityWorkspaceIDs []int) bool {
	switch capType {
	case models.CapabilityDockerEnvironment:
		var config models.DockerEnvironmentConfig
		if err := json.Unmarshal([]byte(configStr), &config); err != nil {
			respondValidationError(w, r, fmt.Sprintf("Invalid docker_environment config: %v", err))
			return false
		}
		if strings.TrimSpace(config.Image) == "" {
			respondValidationError(w, r, "Docker image is required")
			return false
		}
		if config.NetworkMode != "" {
			switch config.NetworkMode {
			case "none", "bridge", "host":
				// valid
			default:
				respondValidationError(w, r, fmt.Sprintf("Invalid Docker network mode: %s", config.NetworkMode))
				return false
			}
		}
	case models.CapabilityHTTPClient:
		var config models.HTTPClientConfig
		if err := json.Unmarshal([]byte(configStr), &config); err != nil {
			respondValidationError(w, r, fmt.Sprintf("Invalid http_client config: %v", err))
			return false
		}
		if len(config.AllowedURLPatterns) == 0 {
			respondValidationError(w, r, "At least one allowed URL pattern is required")
			return false
		}
		for _, pattern := range config.AllowedURLPatterns {
			if strings.TrimSpace(pattern) == "" {
				respondValidationError(w, r, "Allowed URL patterns cannot be blank")
				return false
			}
		}
		seenHeaders := make(map[string]string, len(config.DefaultHeaders))
		// default_headers must hold non-sensitive literals only. Auth tokens
		// live in the credential store; an inline Authorization header here
		// would be readable by anyone who can list workspace capabilities.
		for header := range config.DefaultHeaders {
			if !models.IsValidHTTPHeaderName(header) {
				respondValidationError(w, r, fmt.Sprintf("Invalid HTTP header name %q in default_headers", header))
				return false
			}
			normalized := strings.ToLower(strings.TrimSpace(header))
			if previous, exists := seenHeaders[normalized]; exists {
				respondValidationError(w, r, fmt.Sprintf("Headers %q and %q differ only by case or surrounding whitespace", previous, header))
				return false
			}
			seenHeaders[normalized] = header
			if models.IsSensitiveHeaderName(header) {
				respondValidationError(w, r, fmt.Sprintf("Header %q is sensitive — use auth/secret_header_refs to reference a credential instead of placing it in default_headers", header))
				return false
			}
		}
		if !h.validateHTTPAuthRefs(w, r, &config, appliesToAllWorkspaces, capabilityWorkspaceIDs) {
			return false
		}
	case models.CapabilityLLMConnection:
		var config models.LLMConnectionCapabilityConfig
		if err := json.Unmarshal([]byte(configStr), &config); err != nil {
			respondValidationError(w, r, fmt.Sprintf("Invalid llm_connection config: %v", err))
			return false
		}
		if config.ConnectionID <= 0 {
			respondValidationError(w, r, "LLM connection is required")
			return false
		}
		if !h.isEnabledLLMConnection(config.ConnectionID) {
			respondValidationError(w, r, fmt.Sprintf("LLM connection %d does not exist or is disabled", config.ConnectionID))
			return false
		}
	case models.CapabilityRunnerPool:
		var config models.RunnerPoolConfig
		if err := json.Unmarshal([]byte(configStr), &config); err != nil {
			respondValidationError(w, r, fmt.Sprintf("Invalid runner_pool config: %v", err))
			return false
		}
		if config.MaxConcurrentRuns < 0 {
			respondValidationError(w, r, "max_concurrent_runs cannot be negative (0 = unlimited)")
			return false
		}
	}
	return true
}

// validateHTTPAuthRefs ensures the credentials referenced by Auth /
// SecretHeaderRefs exist and are in-scope for the capability:
//   - a global capability (appliesToAllWorkspaces=true) may reference global
//     credentials only;
//   - a workspace-scoped capability may reference globals OR credentials
//     scoped to one of the capability's workspaces.
//
// The header names used by Auth/SecretHeaderRefs must themselves be marked as
// sensitive — i.e. you don't quietly hide a credential reference inside a
// header that wouldn't otherwise be policed.
func (h *ActionsHandler) validateHTTPAuthRefs(w http.ResponseWriter, r *http.Request, config *models.HTTPClientConfig, appliesToAllWorkspaces bool, capabilityWorkspaceIDs []int) bool {
	checkCredScope := func(credentialID int, where string) bool {
		cred, err := h.credentialRepo.GetActionCredentialByID(credentialID)
		if err != nil {
			respondValidationError(w, r, fmt.Sprintf("%s references credential %d which does not exist", where, credentialID))
			return false
		}
		if !cred.IsEnabled {
			respondValidationError(w, r, fmt.Sprintf("%s references credential %d which is disabled", where, credentialID))
			return false
		}
		if appliesToAllWorkspaces {
			if !cred.AppliesToAllWorkspaces {
				respondValidationError(w, r, fmt.Sprintf("%s references workspace-scoped credential %d, but the capability applies to all workspaces — use a credential that applies to all workspaces too", where, credentialID))
				return false
			}
			return true
		}
		// Workspace-scoped capability: every workspace it runs in must also be
		// in the credential's allowlist, otherwise the capability would fail to
		// resolve in some of them.
		if services.CanCapabilityReference(cred, capabilityWorkspaceIDs) {
			return true
		}
		respondValidationError(w, r, fmt.Sprintf("%s references credential %d whose workspace allowlist does not cover every workspace the capability runs in", where, credentialID))
		return false
	}

	seenSecretHeaders := make(map[string]string, len(config.SecretHeaderRefs)+1)
	if config.Auth != nil {
		if config.Auth.CredentialID <= 0 {
			respondValidationError(w, r, "auth.credential_id is required when auth is set")
			return false
		}
		if strings.TrimSpace(config.Auth.HeaderName) == "" {
			respondValidationError(w, r, "auth.header_name is required when auth is set")
			return false
		}
		if !models.IsValidHTTPHeaderName(config.Auth.HeaderName) {
			respondValidationError(w, r, fmt.Sprintf("Invalid auth.header_name %q", config.Auth.HeaderName))
			return false
		}
		if !models.IsSensitiveHeaderName(config.Auth.HeaderName) {
			respondValidationError(w, r, fmt.Sprintf("auth.header_name %q is not in the sensitive-header allowlist; rename it to a known auth header (e.g. Authorization, X-API-Key) or use default_headers for non-secret literals", config.Auth.HeaderName))
			return false
		}
		normalized := strings.ToLower(strings.TrimSpace(config.Auth.HeaderName))
		seenSecretHeaders[normalized] = config.Auth.HeaderName
		if config.Auth.Placement != "" && config.Auth.Placement != "header" {
			respondValidationError(w, r, fmt.Sprintf("auth.placement %q is not supported (use \"header\")", config.Auth.Placement))
			return false
		}
		if !models.IsValidHTTPAuthScheme(config.Auth.Scheme) {
			respondValidationError(w, r, "auth.scheme must be a single HTTP auth-scheme token without spaces")
			return false
		}
		if !checkCredScope(config.Auth.CredentialID, "auth") {
			return false
		}
	}
	for headerName, credentialID := range config.SecretHeaderRefs {
		if strings.TrimSpace(headerName) == "" {
			respondValidationError(w, r, "secret_header_refs contains an empty header name")
			return false
		}
		if !models.IsValidHTTPHeaderName(headerName) {
			respondValidationError(w, r, fmt.Sprintf("Invalid secret_header_refs header name %q", headerName))
			return false
		}
		if !models.IsSensitiveHeaderName(headerName) {
			respondValidationError(w, r, fmt.Sprintf("secret_header_refs header %q is not in the sensitive-header allowlist; use default_headers for non-secret literals", headerName))
			return false
		}
		normalized := strings.ToLower(strings.TrimSpace(headerName))
		if previous, exists := seenSecretHeaders[normalized]; exists {
			respondValidationError(w, r, fmt.Sprintf("Secret headers %q and %q target the same HTTP header", previous, headerName))
			return false
		}
		seenSecretHeaders[normalized] = headerName
		if credentialID <= 0 {
			respondValidationError(w, r, fmt.Sprintf("secret_header_refs[%q] must reference a credential id > 0", headerName))
			return false
		}
		if !checkCredScope(credentialID, fmt.Sprintf("secret_header_refs[%q]", headerName)) {
			return false
		}
	}
	return true
}

func (h *ActionsHandler) filterUsableWorkspaceCapabilities(caps []*models.ActionCapability, capType string) []*models.ActionCapability {
	if capType != string(models.CapabilityLLMConnection) {
		return caps
	}
	usable := make([]*models.ActionCapability, 0, len(caps))
	for _, cap := range caps {
		var config models.LLMConnectionCapabilityConfig
		if err := json.Unmarshal([]byte(cap.Config), &config); err != nil {
			continue
		}
		if h.isEnabledLLMConnection(config.ConnectionID) {
			usable = append(usable, cap)
		}
	}
	return usable
}

// ListCapabilities lists all action capabilities
func (h *ActionsHandler) ListCapabilities(w http.ResponseWriter, r *http.Request) {
	caps, err := h.repo.ListCapabilities()
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if caps == nil {
		caps = []*models.ActionCapability{}
	}

	respondJSONOK(w, caps)
}

// GetCapability gets a single capability by ID
func (h *ActionsHandler) GetCapability(w http.ResponseWriter, r *http.Request) {
	capability, ok := h.requireCapability(w, r)
	if !ok {
		return
	}
	respondJSONOK(w, capability)
}

// CreateCapability creates a new action capability
func (h *ActionsHandler) CreateCapability(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeJSON[models.CreateCapabilityRequest](w, r)
	if !ok {
		return
	}
	sanitize.Apply(&req.Name, sanitize.PlainTextField)

	if req.Name == "" {
		respondValidationError(w, r, "Name is required")
		return
	}
	if req.CapabilityType == "" {
		respondValidationError(w, r, "Capability type is required")
		return
	}
	// Validate capability type
	switch req.CapabilityType {
	case models.CapabilityDockerEnvironment, models.CapabilityHTTPClient, models.CapabilityLLMConnection, models.CapabilityRunnerPool:
		// valid
	default:
		respondValidationError(w, r, fmt.Sprintf("Invalid capability type: %s", req.CapabilityType))
		return
	}
	if req.Config == "" {
		respondValidationError(w, r, "Config is required")
		return
	}
	// Default applies_to_all_workspaces to TRUE when the field is omitted —
	// matches the legacy "global" behavior so old clients still get a usable
	// capability. If a client explicitly restricts scope, at least one workspace
	// must be supplied.
	appliesAll := true
	if req.AppliesToAllWorkspaces != nil {
		appliesAll = *req.AppliesToAllWorkspaces
	}
	workspaceIDs, err := normalizeCapabilityWorkspaceIDs(req.WorkspaceIDs)
	if err != nil {
		respondValidationError(w, r, err.Error())
		return
	}
	if !appliesAll && len(workspaceIDs) == 0 {
		respondValidationError(w, r, "At least one workspace is required when restricting capability scope")
		return
	}
	if appliesAll {
		workspaceIDs = nil
	}
	if !h.validateCapabilityConfig(w, r, req.CapabilityType, req.Config, appliesAll, workspaceIDs) {
		return
	}

	currentUser, ok := RequireAuth(w, r)
	if !ok {
		return
	}

	isEnabled := true
	if req.IsEnabled != nil {
		isEnabled = *req.IsEnabled
	}

	capability := &models.ActionCapability{
		Name:                   req.Name,
		CapabilityType:         req.CapabilityType,
		Config:                 req.Config,
		IsEnabled:              isEnabled,
		AppliesToAllWorkspaces: appliesAll,
		WorkspaceIDs:           workspaceIDs,
		CreatedBy:              &currentUser.ID,
	}

	id, err := h.repo.CreateCapabilityWithWorkspaces(capability, workspaceIDs)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	created, err := h.repo.GetCapabilityByID(id)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	h.auditCapability(r, currentUser, logger.ActionAutomationCapabilityCreate, created)
	respondJSONCreated(w, created)
}

// UpdateCapability updates an existing capability
func (h *ActionsHandler) UpdateCapability(w http.ResponseWriter, r *http.Request) {
	capability, ok := h.requireCapability(w, r)
	if !ok {
		return
	}

	req, ok := decodeJSON[models.UpdateCapabilityRequest](w, r)
	if !ok {
		return
	}
	sanitize.Apply(req.Name, sanitize.PlainTextField)

	if req.Name != nil {
		if *req.Name == "" {
			respondValidationError(w, r, "Name is required")
			return
		}
		capability.Name = *req.Name
	}
	if req.IsEnabled != nil {
		capability.IsEnabled = *req.IsEnabled
	}
	if req.AppliesToAllWorkspaces != nil {
		capability.AppliesToAllWorkspaces = *req.AppliesToAllWorkspaces
	}
	// Resolve the effective workspace allowlist for the updated capability so
	// validateCapabilityConfig can check credential refs against the same
	// scope that will be persisted moments later.
	effectiveWorkspaceIDs := capability.WorkspaceIDs
	if !capability.AppliesToAllWorkspaces {
		if req.WorkspaceIDs != nil {
			var err error
			effectiveWorkspaceIDs, err = normalizeCapabilityWorkspaceIDs(*req.WorkspaceIDs)
			if err != nil {
				respondValidationError(w, r, err.Error())
				return
			}
		}
		if len(effectiveWorkspaceIDs) == 0 {
			respondValidationError(w, r, "At least one workspace is required when restricting capability scope")
			return
		}
	} else {
		effectiveWorkspaceIDs = nil
	}
	candidateConfig := capability.Config
	if req.Config != nil {
		candidateConfig = *req.Config
	}
	// Config references and workspace scope form one authorization invariant.
	// Revalidate the existing config when only the scope changes; otherwise a
	// workspace-only credential can be stranded behind a newly-global HTTP
	// capability without the update endpoint noticing.
	if req.Config != nil || req.AppliesToAllWorkspaces != nil || req.WorkspaceIDs != nil || (req.IsEnabled != nil && *req.IsEnabled) {
		if !h.validateCapabilityConfig(w, r, capability.CapabilityType, candidateConfig, capability.AppliesToAllWorkspaces, effectiveWorkspaceIDs) {
			return
		}
	}
	capability.Config = candidateConfig
	capability.WorkspaceIDs = effectiveWorkspaceIDs

	if err := h.repo.UpdateCapabilityWithWorkspaces(capability, effectiveWorkspaceIDs); err != nil {
		respondInternalError(w, r, err)
		return
	}

	updated, err := h.repo.GetCapabilityByID(capability.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if currentUser := utils.GetCurrentUser(r); currentUser != nil {
		h.auditCapability(r, currentUser, logger.ActionAutomationCapabilityUpdate, updated)
	}
	respondJSONOK(w, updated)
}

// ListWorkspaceCapabilities returns the capabilities a workspace's actions may
// reference: every enabled capability with applies_to_all_workspaces=true PLUS
// any explicitly scoped to this workspace. Optional ?type= filter narrows the
// list (used by node editors that only care about one capability type).
func (h *ActionsHandler) ListWorkspaceCapabilities(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}

	capType := r.URL.Query().Get("type")
	if capType != "" {
		switch models.CapabilityType(capType) {
		case models.CapabilityDockerEnvironment, models.CapabilityHTTPClient, models.CapabilityLLMConnection, models.CapabilityRunnerPool:
			// valid
		default:
			respondValidationError(w, r, fmt.Sprintf("Invalid capability type: %s", capType))
			return
		}
	}

	caps, err := h.repo.ListCapabilitiesForWorkspace(workspaceID, capType)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	caps = h.filterUsableWorkspaceCapabilities(caps, capType)
	if caps == nil {
		caps = []*models.ActionCapability{}
	}

	respondJSONOK(w, sanitizeCapabilitiesForWorkspace(caps))
}

// sanitizeCapabilitiesForWorkspace redacts environment values, sensitive
// headers, and credential IDs from workspace views. System-admin endpoints
// retain the full configuration for credential management.
func sanitizeCapabilitiesForWorkspace(caps []*models.ActionCapability) []*models.ActionCapability {
	if len(caps) == 0 {
		return caps
	}
	out := make([]*models.ActionCapability, 0, len(caps))
	for _, c := range caps {
		if c.CapabilityType == models.CapabilityDockerEnvironment {
			var cfg models.DockerEnvironmentConfig
			if err := json.Unmarshal([]byte(c.Config), &cfg); err != nil {
				cp := *c
				cp.Config = "{}"
				out = append(out, &cp)
				continue
			}
			for key := range cfg.EnvVars {
				cfg.EnvVars[key] = "[REDACTED]"
			}
			newBytes, err := json.Marshal(cfg)
			if err != nil {
				cp := *c
				cp.Config = "{}"
				out = append(out, &cp)
				continue
			}
			cp := *c
			cp.Config = string(newBytes)
			out = append(out, &cp)
			continue
		}
		if c.CapabilityType != models.CapabilityHTTPClient {
			out = append(out, c)
			continue
		}
		var cfg models.HTTPClientConfig
		if err := json.Unmarshal([]byte(c.Config), &cfg); err != nil {
			// A malformed config cannot be safely scrubbed.
			cp := *c
			cp.Config = "{}"
			out = append(out, &cp)
			continue
		}
		if len(cfg.DefaultHeaders) > 0 {
			cleaned := make(map[string]string, len(cfg.DefaultHeaders))
			for k := range cfg.DefaultHeaders {
				if models.IsSensitiveHeaderName(k) {
					continue
				}
				// Expose header names but never their literal values.
				cleaned[k] = "[REDACTED]"
			}
			cfg.DefaultHeaders = cleaned
		}
		if len(cfg.SecretHeaderRefs) > 0 {
			redacted := make(map[string]int, len(cfg.SecretHeaderRefs))
			for k := range cfg.SecretHeaderRefs {
				redacted[k] = 1 // presence indicator, not the credential id
			}
			cfg.SecretHeaderRefs = redacted
		}
		if cfg.Auth != nil {
			// Hide the credential ID from workspace view — a workspace admin
			// doesn't need it to use the capability, and exposing it would
			// invite cross-workspace fishing.
			cfg.Auth = &models.HTTPAuthRef{
				CredentialID: 0,
				Placement:    cfg.Auth.Placement,
				HeaderName:   cfg.Auth.HeaderName,
				Scheme:       "",
			}
		}
		newBytes, err := json.Marshal(cfg)
		if err != nil {
			cp := *c
			cp.Config = "{}"
			out = append(out, &cp)
			continue
		}
		cp := *c
		cp.Config = string(newBytes)
		out = append(out, &cp)
	}
	return out
}

func normalizeCapabilityWorkspaceIDs(ids []int) ([]int, error) {
	seen := make(map[int]struct{}, len(ids))
	out := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, fmt.Errorf("workspace_ids must contain positive workspace IDs")
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out, nil
}

// DeleteCapability deletes a capability
func (h *ActionsHandler) DeleteCapability(w http.ResponseWriter, r *http.Request) {
	capID, ok := requireIDParam(w, r, "capabilityId")
	if !ok {
		return
	}

	capability, err := h.repo.GetCapabilityByID(capID)
	if err == repository.ErrNotFound {
		respondNotFound(w, r, "capability")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}

	if err := h.repo.DeleteCapability(capID); err != nil {
		respondInternalError(w, r, err)
		return
	}

	if currentUser := utils.GetCurrentUser(r); currentUser != nil {
		h.auditCapability(r, currentUser, logger.ActionAutomationCapabilityDelete, capability)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *ActionsHandler) auditCapability(r *http.Request, user *models.User, action string, capability *models.ActionCapability) {
	if user == nil || capability == nil {
		return
	}
	h.auditor.LogWithDetails(r, user, action, logger.ResourceAutomationCapability, &capability.ID, capability.Name, map[string]any{
		"capability_type":           capability.CapabilityType,
		"is_enabled":                capability.IsEnabled,
		"applies_to_all_workspaces": capability.AppliesToAllWorkspaces,
		"workspace_ids":             capability.WorkspaceIDs,
	})
}
