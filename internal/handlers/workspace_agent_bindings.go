package handlers

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"windshift/internal/aitools"
	"windshift/internal/auth"
	"windshift/internal/llm"
	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/restapi"
	"windshift/internal/sanitize"
	"windshift/internal/services"
)

// WorkspaceAgentBindingHandler exposes the workspace-admin CRUD for the
// coding-agent harness bindings (WI-88). Every mutation goes through
// services.BindingService so the WI-87 acting-identity chokepoint always
// runs at create time. The Candidates endpoint surfaces the picker
// contents to the UI so admins can't see ineligible options.
type WorkspaceAgentBindingHandler struct {
	bindings          *services.BindingService
	identity          *services.AgentActingIdentityService
	permissionService *services.PermissionService
	auditor           *logger.Auditor
	skills            *repository.WorkspaceAgentSkillRepository
	prompts           *llm.PromptStore
	catalog           llm.TemplateSource
	presence          *services.AgentPresenceService
	runnerRegistry    *services.RunnerRegistryService
	baseURL           string
	initialPrompt     string
}

// NewWorkspaceAgentBindingHandler constructs the handler.
func NewWorkspaceAgentBindingHandler(
	bindings *services.BindingService,
	identity *services.AgentActingIdentityService,
	permissionService *services.PermissionService,
	auditor *logger.Auditor,
) *WorkspaceAgentBindingHandler {
	return &WorkspaceAgentBindingHandler{
		bindings:          bindings,
		identity:          identity,
		permissionService: permissionService,
		auditor:           auditor,
	}
}

// ToolCapabilities returns the canonical Standard-agent capability catalog.
// Workspace admins are the only callers allowed to inspect full tool policy;
// the response is derived from the executable aitools registry and omits every
// destructive or access-administration entry.
func (h *WorkspaceAgentBindingHandler) ToolCapabilities(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	if !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionWorkspaceAdmin, h.permissionService) {
		return
	}
	respondJSON(w, http.StatusOK, aitools.StandardCapabilityGroups(aitools.Default))
}

// SetSkillsRepo wires the optional agent-skills repository (WI-258) so
// binding responses can include attached skill ids and the agent-config
// update endpoint can replace attachments.
func (h *WorkspaceAgentBindingHandler) SetSkillsRepo(repo *repository.WorkspaceAgentSkillRepository) {
	h.skills = repo
}

// SetPromptStore wires the effective embedded-or-overridden Agent Studio
// template catalog used by both the templates endpoint and Draft creation.
func (h *WorkspaceAgentBindingHandler) SetPromptStore(store *llm.PromptStore) {
	h.prompts = store
}

// SetTemplateCatalog wires the merged creation catalog (embedded defaults
// overlaid by system-admin DB overrides, WI-922). When set it takes
// precedence over the raw PromptStore for the templates endpoint.
func (h *WorkspaceAgentBindingHandler) SetTemplateCatalog(catalog llm.TemplateSource) {
	h.catalog = catalog
}

// SetPresenceService supplies the same runner-heartbeat contract used by
// assignment pickers so Agent Studio does not invent a second availability
// interpretation.
func (h *WorkspaceAgentBindingHandler) SetPresenceService(service *services.AgentPresenceService) {
	h.presence = service
}

// SetRunnerOnboarding wires the workspace-scoped subset of the runner
// registry used by Agent Studio. Pool authorization still goes through the
// binding service before any registry row is read or changed.
func (h *WorkspaceAgentBindingHandler) SetRunnerOnboarding(registry *services.RunnerRegistryService, baseURL string) {
	h.runnerRegistry = registry
	h.baseURL = baseURL
}

// Templates returns the approved creation templates and their effective
// instructions. Only workspace administrators can inspect the full prompts.
func (h *WorkspaceAgentBindingHandler) Templates(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	if !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionWorkspaceAdmin, h.permissionService) {
		return
	}
	if h.catalog != nil {
		respondJSON(w, http.StatusOK, h.catalog.AgentTemplates())
		return
	}
	if h.prompts == nil {
		respondServiceUnavailable(w, r, "Agent Studio templates are not configured")
		return
	}
	respondJSON(w, http.StatusOK, h.prompts.AgentTemplates())
}

// SetInitialPrompt wires the effective server-managed coding-agent prompt.
// It may differ from the embedded default when AI_PROMPTS_DIR overrides it.
func (h *WorkspaceAgentBindingHandler) SetInitialPrompt(prompt string) {
	h.initialPrompt = prompt
}

// InitialPrompt returns the effective standard prompt to workspace admins so
// they can understand what their binding-level instructions are appended to.
func (h *WorkspaceAgentBindingHandler) InitialPrompt(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	if !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionWorkspaceAdmin, h.permissionService) {
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"prompt": h.initialPrompt})
}

// Candidates returns the acting-identity options the workspace admin
// may pick for a binding in this workspace: owned agents + allowlisted
// centralized service users (when the WI-87 master flag is on). The
// chokepoint still re-validates at create time — this endpoint is a UX
// shortcut, not the security boundary.
func (h *WorkspaceAgentBindingHandler) Candidates(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	if !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionWorkspaceAdmin, h.permissionService) {
		return
	}
	candidates, err := h.identity.ListCandidatesForBinding(r.Context(), user.ID, workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if candidates == nil {
		candidates = []services.CandidateActingIdentity{}
	}
	respondJSON(w, http.StatusOK, candidates)
}

type bindingResponse struct {
	ID               int                       `json:"id"`
	WorkspaceID      int                       `json:"workspace_id"`
	ActingUserID     int                       `json:"acting_user_id"`
	ActingUserKind   string                    `json:"acting_user_kind"`
	ProfileType      models.AgentProfileType   `json:"profile_type"`
	Lifecycle        models.AgentLifecycle     `json:"lifecycle"`
	ProfileVersion   int                       `json:"profile_version"`
	IdentityClass    models.AgentIdentityClass `json:"identity_class"`
	Purpose          string                    `json:"purpose,omitempty"`
	CapabilityGroups []string                  `json:"capability_groups"`
	ArchivedAt       *time.Time                `json:"archived_at,omitempty"`
	ArchivedByUserID *int                      `json:"archived_by_user_id,omitempty"`
	LastKnownName    string                    `json:"last_known_name,omitempty"`
	LastKnownHandle  string                    `json:"last_known_handle,omitempty"`
	LastKnownAvatar  string                    `json:"last_known_avatar,omitempty"`
	Name             string                    `json:"name,omitempty"`
	Handle           string                    `json:"handle,omitempty"`
	AvatarURL        string                    `json:"avatar_url,omitempty"`
	RepoSlug         string                    `json:"repo_slug,omitempty"`
	RepoBaseRef      string                    `json:"repo_base_ref,omitempty"`
	LLMConnectionID  *int                      `json:"llm_connection_id,omitempty"`
	SCMConnectionID  *int                      `json:"scm_connection_id,omitempty"`
	TargetPoolID     *int                      `json:"target_pool_id,omitempty"`
	RunnerImage      string                    `json:"runner_image,omitempty"`
	TokenScopes      []string                  `json:"token_scopes,omitempty"`
	TokenTTLMinutes  int                       `json:"token_ttl_minutes"`
	MaxRunsPerDay    int                       `json:"max_runs_per_day"`
	Instructions     string                    `json:"instructions,omitempty"`
	SkillIDs         []int                     `json:"skill_ids,omitempty"`
	// Repos is the binding's bound repositories (WI-449). The legacy scalar
	// RepoSlug/RepoBaseRef/SCMConnectionID above mirror the primary repo.
	Repos []bindingRepoResponse `json:"repos,omitempty"`
}

// agentCatalogEntry is the member-safe Agent Studio projection. Configuration
// secrets, instructions, grants, repository details, and dependency errors are
// deliberately absent; workspace administrators use the existing profile APIs
// for those fields.
type agentCatalogEntry struct {
	ID             int                       `json:"id"`
	WorkspaceID    int                       `json:"workspace_id"`
	Name           string                    `json:"name"`
	Handle         string                    `json:"handle"`
	AvatarURL      string                    `json:"avatar_url,omitempty"`
	Purpose        string                    `json:"purpose,omitempty"`
	ProfileType    models.AgentProfileType   `json:"profile_type"`
	Runtime        string                    `json:"runtime"`
	IdentityClass  models.AgentIdentityClass `json:"identity_class"`
	OwnerName      string                    `json:"owner_name,omitempty"`
	Lifecycle      models.AgentLifecycle     `json:"lifecycle"`
	Availability   string                    `json:"availability"`
	Available      bool                      `json:"available"`
	ModelSummary   string                    `json:"model_summary,omitempty"`
	ProfileVersion int                       `json:"profile_version"`
	UpdatedAt      time.Time                 `json:"updated_at"`
}

type bindingRepoResponse struct {
	RepoSlug        string `json:"repo_slug"`
	RepoBaseRef     string `json:"repo_base_ref,omitempty"`
	SCMConnectionID *int   `json:"scm_connection_id,omitempty"`
	IsPrimary       bool   `json:"is_primary"`
	Position        int    `json:"position"`
}

func catalogRuntime(profileType models.AgentProfileType) string {
	switch profileType {
	case models.AgentProfileStandard:
		return "windshift"
	case models.AgentProfileCoding:
		return "authorized_runner"
	default:
		return "legacy_local"
	}
}

func catalogAvailability(binding *models.WorkspaceAgentBinding, validation *services.ProfileValidationResult, presence string) (string, bool) {
	if binding.ArchivedAt != nil || binding.Lifecycle == models.AgentLifecycleArchived {
		return "archived", false
	}
	switch binding.Lifecycle {
	case models.AgentLifecycleDraft:
		return "draft", false
	case models.AgentLifecyclePaused:
		return "paused", false
	case models.AgentLifecycleReady:
		if validation != nil && validation.Ready {
			if binding.ProfileType == models.AgentProfileCoding && presence == services.AgentPresenceOffline {
				// Runner-backed work is durable: Offline is observable but
				// remains assignable so work can queue until a runner returns.
				return "offline", true
			}
			return "ready", true
		}
		return "needs_setup", false
	default:
		return "invalid", false
	}
}

func toBindingResponse(b *models.WorkspaceAgentBinding) bindingResponse {
	resp := bindingResponse{
		ID:               b.ID,
		WorkspaceID:      b.WorkspaceID,
		ActingUserID:     b.ActingUserID,
		ActingUserKind:   b.ActingUserKind,
		ProfileType:      b.ProfileType,
		Lifecycle:        b.Lifecycle,
		ProfileVersion:   b.ProfileVersion,
		IdentityClass:    b.IdentityClass,
		Purpose:          b.Purpose,
		CapabilityGroups: append([]string(nil), b.CapabilityGroups...),
		ArchivedAt:       b.ArchivedAt,
		ArchivedByUserID: b.ArchivedByUserID,
		LastKnownName:    b.LastKnownName,
		LastKnownHandle:  b.LastKnownHandle,
		LastKnownAvatar:  b.LastKnownAvatar,
		Name:             b.DisplayName,
		Handle:           b.Handle,
		AvatarURL:        b.AvatarURL,
		RepoSlug:         b.RepoSlug,
		RepoBaseRef:      b.RepoBaseRef,
		LLMConnectionID:  b.LLMConnectionID,
		SCMConnectionID:  b.SCMConnectionID,
		TargetPoolID:     b.TargetPoolID,
		RunnerImage:      b.RunnerImage,
		TokenScopes:      b.TokenScopes,
		TokenTTLMinutes:  b.TokenTTLMinutes,
		MaxRunsPerDay:    b.MaxRunsPerDay,
		Instructions:     b.Instructions,
	}
	for _, rp := range b.Repos {
		resp.Repos = append(resp.Repos, bindingRepoResponse{
			RepoSlug:        rp.RepoSlug,
			RepoBaseRef:     rp.RepoBaseRef,
			SCMConnectionID: rp.SCMConnectionID,
			IsPrimary:       rp.IsPrimary,
			Position:        rp.Position,
		})
	}
	return resp
}

// Catalog returns the safe Agent Studio card data visible to every workspace
// member. Full profile configuration remains workspace-admin only.
func (h *WorkspaceAgentBindingHandler) Catalog(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	if !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionItemView, h.permissionService) {
		return
	}
	bindings, err := h.bindings.ListForWorkspace(r.Context(), workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	out := make([]agentCatalogEntry, 0, len(bindings))
	presenceByAgent := map[int]string{}
	if h.presence != nil {
		if resolved, err := h.presence.ForWorkspace(r.Context(), workspaceID); err == nil {
			presenceByAgent = resolved
		}
	}
	includeOwnerAttribution := canViewAgentOwnerAttribution(h.permissionService, user.ID)
	for _, binding := range bindings {
		name := binding.DisplayName
		if name == "" {
			name = binding.LastKnownName
		}
		handle := binding.Handle
		if handle == "" {
			handle = binding.LastKnownHandle
		}
		avatarURL := binding.AvatarURL
		if avatarURL == "" {
			avatarURL = binding.LastKnownAvatar
		}
		var validation *services.ProfileValidationResult
		if binding.Lifecycle == models.AgentLifecycleReady && binding.ArchivedAt == nil {
			validation, _ = h.bindings.ValidateStudioProfile(r.Context(), workspaceID, binding.ID)
		}
		availability, available := catalogAvailability(binding, validation, presenceByAgent[binding.ActingUserID])
		ownerName := ""
		if includeOwnerAttribution {
			ownerName = h.bindings.AgentProfileOwnerName(r.Context(), binding)
		}
		out = append(out, agentCatalogEntry{
			ID:             binding.ID,
			WorkspaceID:    binding.WorkspaceID,
			Name:           name,
			Handle:         handle,
			AvatarURL:      avatarURL,
			Purpose:        binding.Purpose,
			ProfileType:    binding.ProfileType,
			Runtime:        catalogRuntime(binding.ProfileType),
			IdentityClass:  binding.IdentityClass,
			OwnerName:      ownerName,
			Lifecycle:      binding.Lifecycle,
			Availability:   availability,
			Available:      available,
			ModelSummary:   h.bindings.AgentProfileModelSummary(r.Context(), binding),
			ProfileVersion: binding.ProfileVersion,
			UpdatedAt:      binding.UpdatedAt,
		})
	}
	respondJSON(w, http.StatusOK, out)
}

// withSkillIDs decorates a binding response with its attached skill ids.
// Best-effort: a skills lookup failure leaves the field empty rather than
// failing the listing.
func (h *WorkspaceAgentBindingHandler) withSkillIDs(r *http.Request, resp bindingResponse) bindingResponse {
	if h.skills == nil {
		return resp
	}
	ids, err := h.skills.SkillIDsForBinding(r.Context(), resp.ID)
	if err == nil {
		resp.SkillIDs = ids
	}
	return resp
}

type createBindingBody struct {
	ActingUserID int `json:"acting_user_id"`
	// Repos is the preferred multi-repo input (WI-449). When empty, the legacy
	// scalar repo_slug/repo_base_ref/scm_connection_id below are folded into a
	// single primary repo for old clients.
	Repos           []createBindingRepoBody `json:"repos,omitempty"`
	RepoSlug        string                  `json:"repo_slug,omitempty"`
	RepoBaseRef     string                  `json:"repo_base_ref,omitempty"`
	LLMConnectionID *int                    `json:"llm_connection_id,omitempty"`
	SCMConnectionID *int                    `json:"scm_connection_id,omitempty"`
	TargetPoolID    *int                    `json:"target_pool_id,omitempty"`
	RunnerImage     string                  `json:"runner_image,omitempty"`
	TokenScopes     []string                `json:"token_scopes,omitempty"`
	TokenTTLMinutes int                     `json:"token_ttl_minutes,omitempty"`
	MaxRunsPerDay   int                     `json:"max_runs_per_day,omitempty"`
	Instructions    string                  `json:"instructions,omitempty"`
	SkillIDs        []int                   `json:"skill_ids,omitempty"`
}

type createBindingRepoBody struct {
	RepoSlug        string `json:"repo_slug"`
	RepoBaseRef     string `json:"repo_base_ref,omitempty"`
	SCMConnectionID *int   `json:"scm_connection_id,omitempty"`
	IsPrimary       bool   `json:"is_primary,omitempty"`
}

type createStudioProfileBody struct {
	TemplateKey      string                  `json:"template_key"`
	ProfileType      models.AgentProfileType `json:"profile_type,omitempty"`
	ActingUserID     int                     `json:"acting_user_id,omitempty"`
	Name             string                  `json:"name,omitempty"`
	Handle           string                  `json:"handle,omitempty"`
	AvatarURL        string                  `json:"avatar_url,omitempty"`
	Purpose          string                  `json:"purpose,omitempty"`
	Instructions     *string                 `json:"instructions,omitempty"`
	CapabilityGroups []string                `json:"capability_groups,omitempty"`
	LLMConnectionID  *int                    `json:"llm_connection_id,omitempty"`
	Repos            []createBindingRepoBody `json:"repos,omitempty"`
	TargetPoolID     *int                    `json:"target_pool_id,omitempty"`
	RunnerImage      string                  `json:"runner_image,omitempty"`
	TokenScopes      []string                `json:"token_scopes,omitempty"`
	TokenTTLMinutes  int                     `json:"token_ttl_minutes,omitempty"`
	MaxRunsPerDay    int                     `json:"max_runs_per_day,omitempty"`
	SkillIDs         []int                   `json:"skill_ids,omitempty"`
}

type updateStudioProfileBody struct {
	ExpectedVersion int    `json:"expected_version"`
	Name            string `json:"name,omitempty"`
	Handle          string `json:"handle,omitempty"`
	AvatarURL       string `json:"avatar_url,omitempty"`
	Purpose         string `json:"purpose,omitempty"`
}

type migrateLegacyProfileBody struct {
	TargetPoolID int `json:"target_pool_id"`
}

type privateProfileTestBody struct {
	Prompt string `json:"prompt,omitempty"`
}

const maxPrivateProfileTestPromptRunes = 8000

// CreateProfile transactionally creates an Agent Studio Draft using either a
// new workspace-managed identity or an eligible centralized service identity,
// according to the shared central setting.
func (h *WorkspaceAgentBindingHandler) CreateProfile(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	if !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionWorkspaceAdmin, h.permissionService) {
		return
	}
	var body createStudioProfileBody
	if err := newJSONDecoder(w, r).Decode(&body); err != nil {
		respondBadRequest(w, r, "invalid request body")
		return
	}
	sanitize.ApplyAll(
		sanitize.Pair{Target: &body.TemplateKey, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: &body.Name, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &body.Handle, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: &body.Purpose, Policy: sanitize.PlainTextField},
	)
	if body.Instructions != nil {
		sanitize.Apply(body.Instructions, sanitize.RichText)
	}
	repos := make([]services.RepoInput, 0, len(body.Repos))
	for i := range body.Repos {
		sanitize.ApplyAll(
			sanitize.Pair{Target: &body.Repos[i].RepoSlug, Policy: sanitize.ShortIdentifier},
			sanitize.Pair{Target: &body.Repos[i].RepoBaseRef, Policy: sanitize.ShortIdentifier},
		)
		repos = append(repos, services.RepoInput{
			RepoSlug:        body.Repos[i].RepoSlug,
			RepoBaseRef:     body.Repos[i].RepoBaseRef,
			SCMConnectionID: body.Repos[i].SCMConnectionID,
			IsPrimary:       body.Repos[i].IsPrimary,
		})
	}
	profile, err := h.bindings.CreateStudioProfile(r.Context(), services.CreateStudioProfileRequest{
		WorkspaceID:     workspaceID,
		CreatedByUserID: user.ID,
		TemplateKey:     body.TemplateKey,
		ProfileType:     body.ProfileType,
		ActingUserID:    body.ActingUserID,
		Name:            body.Name,
		Handle:          body.Handle,
		AvatarURL:       strings.TrimSpace(body.AvatarURL),
		Purpose:         body.Purpose,
		Instructions:    body.Instructions,
		LLMConnectionID: body.LLMConnectionID,
		Repos:           repos,
		TargetPoolID:    body.TargetPoolID,
		RunnerImage:     body.RunnerImage,
		TokenScopes:     body.TokenScopes,
		TokenTTLMinutes: body.TokenTTLMinutes,
		MaxRunsPerDay:   body.MaxRunsPerDay,
		SkillIDs:        body.SkillIDs,
	})
	if err != nil {
		switch {
		case errors.Is(err, services.ErrAgentProfileServiceUnavailable):
			respondServiceUnavailable(w, r, err.Error())
		case errors.Is(err, services.ErrAgentProfileHandleTaken),
			errors.Is(err, repository.ErrBindingDuplicate):
			respondConflict(w, r, err.Error())
		case errors.Is(err, services.ErrAgentProfileInvalidTemplate),
			errors.Is(err, services.ErrAgentProfileInvalidType),
			errors.Is(err, services.ErrAgentProfileInvalidHandle),
			errors.Is(err, services.ErrAgentProfileNameRequired),
			errors.Is(err, services.ErrAgentProfileInvalidCapabilities),
			errors.Is(err, services.ErrAgentProfileStandardRuntimeOnly),
			errors.Is(err, services.ErrAgentProfileCodingTools),
			errors.Is(err, services.ErrAgentProfileTestManagement),
			errors.Is(err, services.ErrLLMConnectionRequired),
			errors.Is(err, services.ErrLLMConnectionInvalid),
			errors.Is(err, services.ErrBindingTokenTTLOverCap),
			errors.Is(err, services.ErrBindingRepoNeedsSCMConnection),
			errors.Is(err, services.ErrBindingInvalidRepoSlug),
			errors.Is(err, services.ErrBindingDuplicateRepoSlug),
			errors.Is(err, services.ErrBindingPrimaryRepoRequired),
			errors.Is(err, services.ErrBindingTooManyRepos),
			errors.Is(err, services.ErrBindingInvalidPool),
			errors.Is(err, services.ErrBindingRunnerImageRequiresPool),
			errors.Is(err, services.ErrBindingInvalidRunnerImage),
			errors.Is(err, services.ErrBindingInstructionsTooLong),
			isSkillAttachError(err),
			isAgentScopeError(err):
			respondBadRequest(w, r, err.Error())
		case errors.Is(err, services.ErrAgentProfileCentralizedRequired),
			isIdentityGateError(err):
			respondForbidden(w, r)
		default:
			respondInternalError(w, r, err)
		}
		return
	}
	h.auditor.LogWithDetails(r, user, logger.ActionAgentProfileCreate, logger.ResourceWorkspaceAgentBinding, &profile.ID, "", map[string]any{
		"workspace_id":   workspaceID,
		"acting_user_id": profile.ActingUserID,
		"profile_type":   profile.ProfileType,
		"identity_class": profile.IdentityClass,
		"lifecycle":      profile.Lifecycle,
	})
	respondJSON(w, http.StatusCreated, h.withSkillIDs(r, toBindingResponse(profile)))
}

// UpdateProfile edits the unified Agent Studio overview. Type and identity
// class remain immutable, and identity fields are accepted only for
// workspace-managed identities.
func (h *WorkspaceAgentBindingHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	workspaceID, id, user, ok := h.requireProfileAdmin(w, r)
	if !ok {
		return
	}
	var body updateStudioProfileBody
	if err := newJSONDecoder(w, r).Decode(&body); err != nil {
		respondBadRequest(w, r, "invalid request body")
		return
	}
	sanitize.ApplyAll(
		sanitize.Pair{Target: &body.Name, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &body.Handle, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: &body.Purpose, Policy: sanitize.PlainTextField},
	)
	profile, err := h.bindings.UpdateStudioProfile(r.Context(), services.UpdateStudioProfileRequest{
		WorkspaceID:     workspaceID,
		BindingID:       id,
		ExpectedVersion: body.ExpectedVersion,
		Name:            body.Name,
		Handle:          body.Handle,
		AvatarURL:       body.AvatarURL,
		Purpose:         body.Purpose,
	})
	if err != nil {
		switch {
		case errors.Is(err, services.ErrBindingNotFound):
			respondNotFound(w, r, "agent profile")
		case errors.Is(err, services.ErrBindingUnavailable),
			errors.Is(err, services.ErrAgentProfileVersionConflict):
			respondConflict(w, r, err.Error())
		case errors.Is(err, services.ErrAgentProfileInvalidHandle),
			errors.Is(err, services.ErrAgentProfileHandleTaken),
			errors.Is(err, services.ErrAgentProfileNameRequired),
			errors.Is(err, services.ErrAgentProfileIdentityImmutable):
			respondBadRequest(w, r, err.Error())
		default:
			respondInternalError(w, r, err)
		}
		return
	}
	h.auditor.LogWithDetails(r, user, logger.ActionAgentProfileUpdate, logger.ResourceWorkspaceAgentBinding, &profile.ID, "", map[string]any{
		"workspace_id": workspaceID,
	})
	respondJSON(w, http.StatusOK, h.withSkillIDs(r, toBindingResponse(profile)))
}

// MigrateLegacyProfile moves one grandfathered local profile to an authorized
// runner pool without replacing its identity, binding, attribution, or run
// history. The resulting Coding definition is Draft until revalidated.
func (h *WorkspaceAgentBindingHandler) MigrateLegacyProfile(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	if !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionWorkspaceAdmin, h.permissionService) {
		return
	}
	id, ok := requireIDParam(w, r, "id")
	if !ok {
		return
	}
	var body migrateLegacyProfileBody
	if err := newJSONDecoder(w, r).Decode(&body); err != nil {
		respondBadRequest(w, r, "invalid request body")
		return
	}
	if body.TargetPoolID <= 0 {
		respondBadRequest(w, r, "target_pool_id must be a positive integer")
		return
	}
	profile, err := h.bindings.MigrateLegacyToRunner(r.Context(), workspaceID, id, body.TargetPoolID)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrBindingNotFound):
			respondNotFound(w, r, "agent profile")
		case errors.Is(err, services.ErrBindingUnavailable),
			errors.Is(err, services.ErrAgentProfileLegacyMigrationOnly):
			respondConflict(w, r, err.Error())
		case errors.Is(err, services.ErrBindingInvalidPool):
			respondBadRequest(w, r, err.Error())
		default:
			respondInternalError(w, r, err)
		}
		return
	}
	h.auditor.LogWithDetails(r, user, logger.ActionAgentProfileMigrateRunner, logger.ResourceWorkspaceAgentBinding, &profile.ID, "", map[string]any{
		"workspace_id":   workspaceID,
		"target_pool_id": body.TargetPoolID,
		"profile_type":   profile.ProfileType,
	})
	respondJSON(w, http.StatusOK, h.withSkillIDs(r, toBindingResponse(profile)))
}

// ConnectCodingRunner completes runner authorization for a Coding Draft that
// was created before a pool was available. Existing assignments cannot be
// changed through this endpoint.
func (h *WorkspaceAgentBindingHandler) ConnectCodingRunner(w http.ResponseWriter, r *http.Request) {
	workspaceID, profileID, user, ok := h.requireProfileAdmin(w, r)
	if !ok {
		return
	}
	var body migrateLegacyProfileBody
	if err := newJSONDecoder(w, r).Decode(&body); err != nil {
		respondBadRequest(w, r, "invalid request body")
		return
	}
	if body.TargetPoolID <= 0 {
		respondBadRequest(w, r, "target_pool_id must be a positive integer")
		return
	}
	profile, err := h.bindings.ConnectCodingRunner(r.Context(), workspaceID, profileID, body.TargetPoolID)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrBindingNotFound):
			respondNotFound(w, r, "agent profile")
		case errors.Is(err, services.ErrBindingUnavailable),
			errors.Is(err, services.ErrAgentProfileRunnerAlreadySet):
			respondConflict(w, r, err.Error())
		case errors.Is(err, services.ErrBindingInvalidPool):
			respondBadRequest(w, r, err.Error())
		default:
			respondInternalError(w, r, err)
		}
		return
	}
	h.auditor.LogWithDetails(r, user, logger.ActionAgentProfileConnectRunner, logger.ResourceWorkspaceAgentBinding, &profile.ID, "", map[string]any{
		"workspace_id":   workspaceID,
		"target_pool_id": body.TargetPoolID,
	})
	respondJSON(w, http.StatusOK, h.withSkillIDs(r, toBindingResponse(profile)))
}

func (h *WorkspaceAgentBindingHandler) requireRunnerPoolAdmin(w http.ResponseWriter, r *http.Request) (workspaceID, poolID int, user *models.User, ok bool) {
	workspaceID, ok = requireIDParam(w, r, "workspaceId")
	if !ok {
		return 0, 0, nil, false
	}
	user, ok = RequireAuth(w, r)
	if !ok {
		return 0, 0, nil, false
	}
	if !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionWorkspaceAdmin, h.permissionService) {
		return 0, 0, nil, false
	}
	poolID, ok = requireIDParam(w, r, "poolId")
	if !ok {
		return 0, 0, nil, false
	}
	if h.runnerRegistry == nil {
		respondServiceUnavailable(w, r, "runner onboarding is not configured")
		return 0, 0, nil, false
	}
	if err := h.bindings.ValidateRunnerPool(workspaceID, poolID); err != nil {
		respondNotFound(w, r, "runner pool")
		return 0, 0, nil, false
	}
	return workspaceID, poolID, user, true
}

// MintRunnerSetupToken creates one short-lived, single-use registration token
// for an authorized workspace pool. The plaintext and install command are
// returned once and never persisted by Agent Studio.
func (h *WorkspaceAgentBindingHandler) MintRunnerSetupToken(w http.ResponseWriter, r *http.Request) {
	workspaceID, poolID, user, ok := h.requireRunnerPoolAdmin(w, r)
	if !ok {
		return
	}
	var req mintRunnerTokenRequest
	if err := newJSONDecoder(w, r).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		respondBadRequest(w, r, "invalid request body")
		return
	}
	sanitize.Apply(&req.Description, sanitize.PlainTextField)
	if req.TTLHours < 0 || req.TTLHours > defaultRunnerTokenTTLHours {
		respondBadRequest(w, r, "ttl_hours must be between 0 and 720")
		return
	}
	ttlHours := req.TTLHours
	if ttlHours == 0 {
		ttlHours = defaultRunnerTokenTTLHours
	}
	full, token, err := h.runnerRegistry.MintRegistrationToken(
		r.Context(),
		poolID,
		&user.ID,
		req.Description,
		time.Duration(ttlHours)*time.Hour,
	)
	if errors.Is(err, services.ErrRunnerPoolUnavailable) {
		respondConflict(w, r, "runner pool is disabled")
		return
	}
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	h.auditor.LogWithDetails(r, user, logger.ActionAgentRunnerSetupTokenMint, logger.ResourceRunnerRegistrationToken, &token.ID, "", map[string]any{
		"workspace_id": workspaceID,
		"pool_id":      poolID,
	})
	respondJSONCreated(w, mintRunnerTokenResponse{
		Token:                   full,
		InstallCommand:          runnerInstallCommand(apiBaseURLFor(h.baseURL, r), full),
		RunnerRegistrationToken: token,
	})
}

// ListRunnerSetupTokens exposes hash-only setup-token metadata so a restored
// browser draft can detect consumption, expiry, or cancellation.
func (h *WorkspaceAgentBindingHandler) ListRunnerSetupTokens(w http.ResponseWriter, r *http.Request) {
	_, poolID, _, ok := h.requireRunnerPoolAdmin(w, r)
	if !ok {
		return
	}
	tokens, err := h.runnerRegistry.ListRegistrationTokens(r.Context(), poolID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if tokens == nil {
		tokens = []*models.RunnerRegistrationToken{}
	}
	respondJSON(w, http.StatusOK, tokens)
}

// RevokeRunnerSetupToken cancels an unconsumed onboarding attempt. The token
// id is verified against the workspace-authorized pool before revocation.
func (h *WorkspaceAgentBindingHandler) RevokeRunnerSetupToken(w http.ResponseWriter, r *http.Request) {
	workspaceID, poolID, user, ok := h.requireRunnerPoolAdmin(w, r)
	if !ok {
		return
	}
	tokenID, ok := requireIDParam(w, r, "tokenId")
	if !ok {
		return
	}
	tokens, err := h.runnerRegistry.ListRegistrationTokens(r.Context(), poolID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if !containsID(tokenIDs(tokens), tokenID) {
		respondNotFound(w, r, "registration token")
		return
	}
	if err := h.runnerRegistry.RevokeRegistrationToken(r.Context(), tokenID); err != nil {
		respondInternalError(w, r, err)
		return
	}
	h.auditor.LogWithDetails(r, user, logger.ActionAgentRunnerSetupTokenRevoke, logger.ResourceRunnerRegistrationToken, &tokenID, "", map[string]any{
		"workspace_id": workspaceID,
		"pool_id":      poolID,
	})
	respondJSON(w, http.StatusOK, map[string]any{"id": tokenID, "revoked": true})
}

// ListRunnerSetupInstances supports bounded onboarding polling. Credentials
// are never part of the model or response.
func (h *WorkspaceAgentBindingHandler) ListRunnerSetupInstances(w http.ResponseWriter, r *http.Request) {
	_, poolID, _, ok := h.requireRunnerPoolAdmin(w, r)
	if !ok {
		return
	}
	instances, err := h.runnerRegistry.ListInstances(r.Context(), poolID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if instances == nil {
		instances = []*models.RunnerInstance{}
	}
	respondJSON(w, http.StatusOK, instances)
}

// TestProfile exercises the profile's actual runtime behind one private,
// workspace-admin-only contract. Standard tests return an immediate answer
// from a read-only tool surface; Coding and Legacy tests return a durable,
// cancelable ephemeral run that cannot push or invoke post-run mutations.
func (h *WorkspaceAgentBindingHandler) TestProfile(w http.ResponseWriter, r *http.Request) {
	workspaceID, profileID, user, ok := h.requireProfileAdmin(w, r)
	if !ok {
		return
	}
	var body privateProfileTestBody
	if r.Body != nil {
		if err := newJSONDecoder(w, r).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			respondBadRequest(w, r, "invalid request body")
			return
		}
	}
	sanitize.Apply(&body.Prompt, sanitize.RichText)
	if utf8.RuneCountInString(body.Prompt) > maxPrivateProfileTestPromptRunes {
		respondBadRequest(w, r, "prompt must be 8000 characters or fewer")
		return
	}
	result, err := h.bindings.RunPrivateProfileTest(
		r.Context(),
		workspaceID,
		profileID,
		user.ID,
		body.Prompt,
	)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrBindingNotFound):
			respondNotFound(w, r, "agent profile")
		case errors.Is(err, services.ErrBindingNoRepo):
			respondBadRequest(w, r, "this profile needs a repository before it can run a bounded verification")
		case errors.Is(err, services.ErrBindingUnavailable),
			errors.Is(err, services.ErrBindingRunnerNotConfigured),
			errors.Is(err, services.ErrStandardPrivateTestUnavailable),
			errors.Is(err, services.ErrTriggerUserSCMNotConnected):
			respondConflict(w, r, err.Error())
		case errors.Is(err, services.ErrBindingInvalidPool):
			respondBadRequest(w, r, err.Error())
		default:
			respondError(w, r, restapi.NewAPIError(
				http.StatusBadGateway,
				restapi.ErrCodeConnectionTestFailed,
				"agent profile test failed: "+err.Error(),
			))
		}
		return
	}
	h.auditor.LogWithDetails(r, user, logger.ActionAgentProfileTest, logger.ResourceWorkspaceAgentBinding, &profileID, "", map[string]any{
		"workspace_id": workspaceID,
		"mode":         result.Mode,
		"run_id":       result.RunID,
		"status":       result.Status,
	})
	respondJSON(w, http.StatusOK, result)
}

// ValidateProfile returns current dependency and permission readiness without
// mutating the saved lifecycle.
func (h *WorkspaceAgentBindingHandler) ValidateProfile(w http.ResponseWriter, r *http.Request) {
	workspaceID, profileID, user, ok := h.requireProfileAdmin(w, r)
	if !ok {
		return
	}
	_ = user
	validation, err := h.bindings.ValidateStudioProfile(r.Context(), workspaceID, profileID)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrBindingNotFound):
			respondNotFound(w, r, "agent profile")
		case errors.Is(err, services.ErrAgentProfileServiceUnavailable):
			respondServiceUnavailable(w, r, err.Error())
		default:
			respondInternalError(w, r, err)
		}
		return
	}
	respondJSON(w, http.StatusOK, validation)
}

// ActivateProfile makes a Draft/Paused profile Ready only after current
// dependencies and configured permissions validate.
func (h *WorkspaceAgentBindingHandler) ActivateProfile(w http.ResponseWriter, r *http.Request) {
	workspaceID, profileID, user, ok := h.requireProfileAdmin(w, r)
	if !ok {
		return
	}
	profile, validation, err := h.bindings.ActivateStudioProfile(r.Context(), workspaceID, profileID)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrAgentProfileValidationFailed):
			respondJSON(w, http.StatusUnprocessableEntity, validation)
		case errors.Is(err, services.ErrBindingNotFound):
			respondNotFound(w, r, "agent profile")
		case errors.Is(err, services.ErrBindingUnavailable):
			respondConflict(w, r, err.Error())
		case errors.Is(err, services.ErrAgentProfileServiceUnavailable):
			respondServiceUnavailable(w, r, err.Error())
		default:
			respondInternalError(w, r, err)
		}
		return
	}
	h.auditor.LogWithDetails(r, user, logger.ActionAgentProfileReady, logger.ResourceWorkspaceAgentBinding, &profile.ID, "", map[string]any{
		"workspace_id":    workspaceID,
		"profile_version": profile.ProfileVersion,
	})
	respondJSON(w, http.StatusOK, h.withSkillIDs(r, toBindingResponse(profile)))
}

func (h *WorkspaceAgentBindingHandler) requireProfileAdmin(w http.ResponseWriter, r *http.Request) (workspaceID, profileID int, user *models.User, ok bool) {
	return h.requireBindingAdmin(w, r)
}

// requireBindingAdmin resolves the shared workspace-admin prologue for
// per-binding routes: workspace id, authenticated user, workspace admin
// permission, and positive binding id path param.
func (h *WorkspaceAgentBindingHandler) requireBindingAdmin(w http.ResponseWriter, r *http.Request) (workspaceID, bindingID int, user *models.User, ok bool) {
	workspaceID, ok = requireIDParam(w, r, "workspaceId")
	if !ok {
		return 0, 0, nil, false
	}
	user, ok = RequireAuth(w, r)
	if !ok {
		return 0, 0, nil, false
	}
	if !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionWorkspaceAdmin, h.permissionService) {
		return 0, 0, nil, false
	}
	bindingID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || bindingID <= 0 {
		respondBadRequest(w, r, "id path param must be a positive integer")
		return 0, 0, nil, false
	}
	return workspaceID, bindingID, user, true
}

// List returns every binding configured in the workspace.
func (h *WorkspaceAgentBindingHandler) List(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	if !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionWorkspaceAdmin, h.permissionService) {
		return
	}
	bindings, err := h.bindings.ListForWorkspace(r.Context(), workspaceID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	out := make([]bindingResponse, 0, len(bindings))
	for _, b := range bindings {
		out = append(out, h.withSkillIDs(r, toBindingResponse(b)))
	}
	respondJSON(w, http.StatusOK, out)
}

// Create persists a binding after validating the acting identity through
// the WI-87 chokepoint. Returns 409 Conflict when a binding already
// exists for (workspace, acting_user); the same surface (ApprError on a
// rejected identity) returns 403.
func (h *WorkspaceAgentBindingHandler) Create(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := requireIDParam(w, r, "workspaceId")
	if !ok {
		return
	}
	user, ok := RequireAuth(w, r)
	if !ok {
		return
	}
	if !RequireWorkspacePermission(w, r, user.ID, workspaceID, models.PermissionWorkspaceAdmin, h.permissionService) {
		return
	}
	var body createBindingBody
	if err := newJSONDecoder(w, r).Decode(&body); err != nil {
		respondBadRequest(w, r, "invalid request body")
		return
	}
	if body.ActingUserID <= 0 {
		respondBadRequest(w, r, "acting_user_id is required")
		return
	}
	// RepoSlug/RepoBaseRef are identifier-shaped (owner/repo, git ref);
	// Instructions is free-form persona text rendered in the binding editor.
	sanitize.ApplyAll(
		sanitize.Pair{Target: &body.RepoSlug, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: &body.RepoBaseRef, Policy: sanitize.ShortIdentifier},
		sanitize.Pair{Target: &body.Instructions, Policy: sanitize.RichText},
	)
	repos := sanitizeRepoInputs(body.Repos)

	binding, err := h.bindings.Create(r.Context(), services.CreateBindingRequest{
		WorkspaceID:     workspaceID,
		ActingUserID:    body.ActingUserID,
		Repos:           repos,
		RepoSlug:        body.RepoSlug,
		RepoBaseRef:     body.RepoBaseRef,
		LLMConnectionID: body.LLMConnectionID,
		SCMConnectionID: body.SCMConnectionID,
		TargetPoolID:    body.TargetPoolID,
		RunnerImage:     body.RunnerImage,
		TokenScopes:     body.TokenScopes,
		TokenTTLMinutes: body.TokenTTLMinutes,
		MaxRunsPerDay:   body.MaxRunsPerDay,
		Instructions:    body.Instructions,
		SkillIDs:        body.SkillIDs,
		CreatedByUserID: user.ID,
	})
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrBindingDuplicate):
			respondConflict(w, r, err.Error())
		case errors.Is(err, services.ErrLLMConnectionRequired),
			errors.Is(err, services.ErrLLMConnectionInvalid):
			respondBadRequest(w, r, err.Error())
		case errors.Is(err, services.ErrBindingTokenTTLOverCap),
			errors.Is(err, services.ErrBindingRepoNeedsSCMConnection),
			errors.Is(err, services.ErrBindingInvalidRepoSlug),
			errors.Is(err, services.ErrBindingDuplicateRepoSlug),
			errors.Is(err, services.ErrBindingPrimaryRepoRequired),
			errors.Is(err, services.ErrBindingTooManyRepos),
			errors.Is(err, services.ErrBindingInvalidPool),
			errors.Is(err, services.ErrBindingRunnerImageRequiresPool),
			errors.Is(err, services.ErrBindingInvalidRunnerImage),
			errors.Is(err, services.ErrBindingInstructionsTooLong),
			isSkillAttachError(err):
			respondBadRequest(w, r, err.Error())
		case isAgentScopeError(err):
			respondBadRequest(w, r, err.Error())
		case isIdentityGateError(err):
			respondForbidden(w, r)
		default:
			respondInternalError(w, r, err)
		}
		return
	}
	h.auditor.LogWithDetails(r, user, logger.ActionAgentBindingCreate, logger.ResourceWorkspaceAgentBinding, &binding.ID, "", map[string]any{
		"workspace_id":     workspaceID,
		"acting_user_id":   binding.ActingUserID,
		"acting_user_kind": binding.ActingUserKind,
	})
	respondJSON(w, http.StatusCreated, h.withSkillIDs(r, toBindingResponse(binding)))
}

// updateBindingBody is the editable subset of a binding (WI-450). The acting
// service user, its kind, the workspace, and the target pool are fixed at
// create and intentionally absent here.
type updateBindingBody struct {
	Repos            []createBindingRepoBody `json:"repos,omitempty"`
	LLMConnectionID  *int                    `json:"llm_connection_id,omitempty"`
	TokenScopes      []string                `json:"token_scopes,omitempty"`
	TokenTTLMinutes  int                     `json:"token_ttl_minutes,omitempty"`
	MaxRunsPerDay    int                     `json:"max_runs_per_day,omitempty"`
	Instructions     string                  `json:"instructions,omitempty"`
	CapabilityGroups *[]string               `json:"capability_groups,omitempty"`
	// RunnerImage is presence-aware (WI-450): nil leaves the current image
	// untouched; a present value (incl. "") sets/clears it.
	RunnerImage *string `json:"runner_image"`
	SkillIDs    []int   `json:"skill_ids,omitempty"`
}

// Update edits an existing binding's mutable configuration (WI-450). Identity
// and target pool are immutable; see updateBindingBody.
func (h *WorkspaceAgentBindingHandler) Update(w http.ResponseWriter, r *http.Request) {
	workspaceID, id, user, ok := h.requireBindingAdmin(w, r)
	if !ok {
		return
	}
	var body updateBindingBody
	if err := newJSONDecoder(w, r).Decode(&body); err != nil {
		respondBadRequest(w, r, "invalid request body")
		return
	}
	sanitize.Apply(&body.Instructions, sanitize.RichText)
	repos := sanitizeRepoInputs(body.Repos)

	binding, err := h.bindings.UpdateBinding(r.Context(), services.UpdateBindingRequest{
		WorkspaceID:      workspaceID,
		BindingID:        id,
		Repos:            repos,
		LLMConnectionID:  body.LLMConnectionID,
		TokenScopes:      body.TokenScopes,
		TokenTTLMinutes:  body.TokenTTLMinutes,
		MaxRunsPerDay:    body.MaxRunsPerDay,
		Instructions:     body.Instructions,
		CapabilityGroups: body.CapabilityGroups,
		RunnerImage:      body.RunnerImage,
		SkillIDs:         body.SkillIDs,
	})
	if err != nil {
		switch {
		case errors.Is(err, services.ErrBindingNotFound):
			respondNotFound(w, r, "agent binding")
		case errors.Is(err, services.ErrBindingUnavailable):
			respondConflict(w, r, err.Error())
		case errors.Is(err, services.ErrLLMConnectionRequired),
			errors.Is(err, services.ErrLLMConnectionInvalid),
			errors.Is(err, services.ErrBindingTokenTTLOverCap),
			errors.Is(err, services.ErrBindingRepoNeedsSCMConnection),
			errors.Is(err, services.ErrBindingInvalidRepoSlug),
			errors.Is(err, services.ErrBindingDuplicateRepoSlug),
			errors.Is(err, services.ErrBindingPrimaryRepoRequired),
			errors.Is(err, services.ErrBindingTooManyRepos),
			errors.Is(err, services.ErrBindingRunnerImageRequiresPool),
			errors.Is(err, services.ErrBindingInvalidRunnerImage),
			errors.Is(err, services.ErrBindingInstructionsTooLong),
			errors.Is(err, services.ErrAgentProfileInvalidCapabilities),
			errors.Is(err, services.ErrAgentProfileCodingTools),
			errors.Is(err, services.ErrAgentProfileTestManagement),
			isSkillAttachError(err),
			isAgentScopeError(err):
			respondBadRequest(w, r, err.Error())
		default:
			respondInternalError(w, r, err)
		}
		return
	}
	h.auditor.LogWithDetails(r, user, logger.ActionAgentBindingUpdate, logger.ResourceWorkspaceAgentBinding, &binding.ID, "", map[string]any{
		"workspace_id": workspaceID,
	})
	respondJSON(w, http.StatusOK, h.withSkillIDs(r, toBindingResponse(binding)))
}

// sanitizeRepoInputs sanitizes repo slugs and refs from the request body and
// maps them to the service-layer input shape.
func sanitizeRepoInputs(repos []createBindingRepoBody) []services.RepoInput {
	inputs := make([]services.RepoInput, 0, len(repos))
	for i := range repos {
		sanitize.ApplyAll(
			sanitize.Pair{Target: &repos[i].RepoSlug, Policy: sanitize.ShortIdentifier},
			sanitize.Pair{Target: &repos[i].RepoBaseRef, Policy: sanitize.ShortIdentifier},
		)
		inputs = append(inputs, services.RepoInput{
			RepoSlug:        repos[i].RepoSlug,
			RepoBaseRef:     repos[i].RepoBaseRef,
			SCMConnectionID: repos[i].SCMConnectionID,
			IsPrimary:       repos[i].IsPrimary,
		})
	}
	return inputs
}

// isSkillAttachError reports whether the error came from skill-id
// validation during binding create/update (bad or foreign ids → 400).
func isSkillAttachError(err error) bool {
	return errors.Is(err, repository.ErrBindingSkillNotInWorkspace) ||
		errors.Is(err, services.ErrBindingSkillsUnavailable)
}

type updateAgentConfigBody struct {
	Instructions string `json:"instructions"`
	// RunnerImage is presence-aware (WI-450): nil (key absent) leaves the
	// binding's current image untouched, so an older client that PUTs only
	// instructions + skill_ids does not silently clear it; a present value
	// (including "") sets/clears it.
	RunnerImage *string `json:"runner_image"`
	SkillIDs    []int   `json:"skill_ids"`
	// LLMConnectionID is presence-aware: nil leaves the binding's model
	// connection untouched; a present value retargets the profile at another
	// enabled connection and returns it to Draft for re-validation.
	LLMConnectionID *int `json:"llm_connection_id"`
}

// UpdateAgentConfig rewrites the binding's prompt-shaping configuration —
// custom instructions, skill attachments (WI-258), and the model connection.
// Bindings stay create/delete-only for everything else; this narrow update lets
// admins iterate on personas and models without recreating the binding.
func (h *WorkspaceAgentBindingHandler) UpdateAgentConfig(w http.ResponseWriter, r *http.Request) {
	workspaceID, id, user, ok := h.requireBindingAdmin(w, r)
	if !ok {
		return
	}
	var body updateAgentConfigBody
	if err := newJSONDecoder(w, r).Decode(&body); err != nil {
		respondBadRequest(w, r, "invalid request body")
		return
	}
	sanitize.Apply(&body.Instructions, sanitize.RichText)
	if err := h.bindings.UpdateAgentConfig(r.Context(), workspaceID, id, body.Instructions, body.RunnerImage, body.SkillIDs, body.LLMConnectionID); err != nil {
		switch {
		case errors.Is(err, services.ErrBindingNotFound):
			respondNotFound(w, r, "agent binding")
		case errors.Is(err, services.ErrBindingUnavailable):
			respondConflict(w, r, err.Error())
		case errors.Is(err, services.ErrBindingInstructionsTooLong),
			errors.Is(err, services.ErrBindingRunnerImageRequiresPool),
			errors.Is(err, services.ErrBindingInvalidRunnerImage),
			errors.Is(err, services.ErrLLMConnectionInvalid),
			isSkillAttachError(err):
			respondBadRequest(w, r, err.Error())
		default:
			respondInternalError(w, r, err)
		}
		return
	}
	h.auditor.LogWithDetails(r, user, logger.ActionAgentBindingUpdateConfig, logger.ResourceWorkspaceAgentBinding, &id, "", map[string]any{
		"workspace_id": workspaceID,
		"skill_count":  len(body.SkillIDs),
	})
	respondJSON(w, http.StatusOK, map[string]any{"updated": true})
}

// Delete preserves the compatibility route while archiving the profile.
// Stable binding, acting-user, run, and attribution references survive.
func (h *WorkspaceAgentBindingHandler) Delete(w http.ResponseWriter, r *http.Request) {
	workspaceID, id, user, ok := h.requireBindingAdmin(w, r)
	if !ok {
		return
	}
	n, err := h.bindings.Delete(r.Context(), id, workspaceID, user.ID)
	if err != nil {
		respondInternalError(w, r, err)
		return
	}
	if n == 0 {
		respondNotFound(w, r, "agent binding")
		return
	}
	h.auditor.LogWithDetails(r, user, logger.ActionAgentBindingArchive, logger.ResourceWorkspaceAgentBinding, &id, "", map[string]any{
		"workspace_id": workspaceID,
	})
	w.WriteHeader(http.StatusNoContent)
}

// Restore reopens an archived profile as Draft with its stable identifiers.
func (h *WorkspaceAgentBindingHandler) Restore(w http.ResponseWriter, r *http.Request) {
	workspaceID, id, user, ok := h.requireBindingAdmin(w, r)
	if !ok {
		return
	}
	binding, err := h.bindings.Restore(r.Context(), id, workspaceID)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrBindingNotFound):
			respondNotFound(w, r, "agent binding")
		case errors.Is(err, services.ErrBindingNotArchived):
			respondConflict(w, r, err.Error())
		default:
			respondInternalError(w, r, err)
		}
		return
	}
	h.auditor.LogWithDetails(r, user, logger.ActionAgentBindingRestore, logger.ResourceWorkspaceAgentBinding, &id, "", map[string]any{
		"workspace_id": workspaceID,
	})
	respondJSON(w, http.StatusOK, h.withSkillIDs(r, toBindingResponse(binding)))
}

// testLLMRequest is the optional body for TestLLM. A blank/absent prompt
// falls back to the service default.
type testLLMRequest struct {
	Prompt string `json:"prompt,omitempty"`
}

// testLLMResponse carries the model's reply back to the admin. It proves only
// that the binding's LLM connection is reachable; the full chain (repo checked
// out, agent can read its files) is exercised by the heavier TestRun.
type testLLMResponse struct {
	Prompt string `json:"prompt"`
	Answer string `json:"answer"`
}

// TestLLM round-trips a prompt through a binding's LLM connection and returns
// the model's reply, so a workspace admin can confirm the agent's model is
// reachable before assigning real work. Workspace-admin gated, like the other
// mutations. A provider/connection failure is surfaced as 502 so the admin
// sees the upstream message rather than an opaque 500.
func (h *WorkspaceAgentBindingHandler) TestLLM(w http.ResponseWriter, r *http.Request) {
	workspaceID, id, _, ok := h.requireBindingAdmin(w, r)
	if !ok {
		return
	}
	body, ok := decodeOptionalJSON[testLLMRequest](w, r)
	if !ok {
		return
	}
	// Prompt is echoed back verbatim in the response.
	sanitize.Apply(&body.Prompt, sanitize.RichText)
	answer, err := h.bindings.TestLLM(r.Context(), id, workspaceID, body.Prompt)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrBindingNotFound):
			respondNotFound(w, r, "agent binding")
		case errors.Is(err, services.ErrLLMConnectionRequired):
			respondBadRequest(w, r, "this binding has no LLM connection — edit it to choose one")
		default:
			respondError(w, r, restapi.NewAPIError(http.StatusBadGateway, restapi.ErrCodeConnectionTestFailed,
				"LLM test failed: "+err.Error()))
		}
		return
	}
	prompt := body.Prompt
	if strings.TrimSpace(prompt) == "" {
		prompt = services.DefaultLLMTestPrompt
	}
	respondJSONOK(w, testLLMResponse{Prompt: prompt, Answer: answer})
}

// testRunResponse returns the id of the provisioned test run so the UI can
// watch it via the agent-runs events endpoints.
type testRunResponse struct {
	RunID int `json:"run_id"`
}

// TestRun provisions a real, ephemeral coding-agent container run for the
// binding (no work item, read-only prompt) so a workspace admin can confirm the
// full chain end-to-end: the model is reachable, the repo clones into a
// worktree, and the agent can read its files. Workspace-admin gated. The run
// executes asynchronously; the response carries its id for event polling.
//
// 404 when the binding is absent, 400 when it has no repo configured, and 409
// when the coding-agent runner isn't configured on this server or the binding
// targets a remote runner pool (test runs are local-runtime only).
func (h *WorkspaceAgentBindingHandler) TestRun(w http.ResponseWriter, r *http.Request) {
	workspaceID, id, user, ok := h.requireBindingAdmin(w, r)
	if !ok {
		return
	}
	runID, err := h.bindings.StartTestRun(r.Context(), id, workspaceID, user.ID)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrBindingNotFound):
			respondNotFound(w, r, "agent binding")
		case errors.Is(err, services.ErrBindingNoRepo):
			respondBadRequest(w, r, "this binding has no repo configured — a test run needs a repo to check out")
		case errors.Is(err, services.ErrBindingRunnerNotConfigured):
			respondConflict(w, r, "the coding-agent runner is not configured on this server")
		case errors.Is(err, services.ErrBindingTestRunRemotePool):
			respondConflict(w, r, "test runs execute on this server's local runtime and are not supported for bindings that target a remote runner pool — assign a real work item to verify the pool instead")
		case errors.Is(err, services.ErrTriggerUserSCMNotConnected):
			respondConflict(w, r, "you have no connected SCM account for this binding's OAuth connection — connect your GitHub/Gitea account under profile settings first")
		default:
			respondError(w, r, restapi.NewAPIError(http.StatusBadGateway, restapi.ErrCodeConnectionTestFailed,
				"failed to start test run: "+err.Error()))
		}
		return
	}
	h.auditor.LogWithDetails(r, user, logger.ActionAgentBindingTestRun, logger.ResourceWorkspaceAgentBinding, &id, "", map[string]any{
		"workspace_id": workspaceID,
		"run_id":       runID,
	})
	respondJSONOK(w, testRunResponse{RunID: runID})
}

// isIdentityGateError reports whether the error came from the WI-87
// chokepoint. The handler maps all of them to 403 so a workspace admin
// cannot tell the difference between "user does not exist", "not your
// agent", and "centralized service users are gated" — the design plan
// calls this out specifically.
func isIdentityGateError(err error) bool {
	return errors.Is(err, services.ErrActingIdentityNotFound) ||
		errors.Is(err, services.ErrActingIdentityNotAgent) ||
		errors.Is(err, services.ErrActingIdentityInactive) ||
		errors.Is(err, services.ErrActingIdentityNotOwned) ||
		errors.Is(err, services.ErrActingIdentityCentralizedGated) ||
		errors.Is(err, services.ErrActingIdentityNotInAllowlist)
}

// isAgentScopeError reports whether the wrapped error came from
// auth.ValidateAgentScopes.
func isAgentScopeError(err error) bool {
	return errors.Is(err, auth.ErrAgentScopesNotPermitted)
}
