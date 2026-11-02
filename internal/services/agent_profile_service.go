package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"windshift/internal/agentstudio"
	"windshift/internal/auth"
	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

var (
	ErrAgentProfileServiceUnavailable  = errors.New("agent profile service is not configured")
	ErrAgentProfileInvalidTemplate     = errors.New("agent profile: unknown template")
	ErrAgentProfileInvalidType         = errors.New("agent profile: type must be standard or coding")
	ErrAgentProfileInvalidHandle       = errors.New("agent profile: handle must be 3-32 lowercase letters, numbers, dots, underscores, or hyphens")
	ErrAgentProfileHandleTaken         = errors.New("agent profile: handle already exists")
	ErrAgentProfileNameRequired        = errors.New("agent profile: name is required")
	ErrAgentProfileCentralizedRequired = errors.New("agent profile: select an eligible centralized service identity")
	ErrAgentProfileInvalidCapabilities = errors.New("agent profile: one or more capability groups are invalid")
	ErrAgentProfileStandardRuntimeOnly = errors.New("agent profile: Standard profiles cannot configure SCM, repositories, runners, or runner images")
	ErrAgentProfileCodingTools         = errors.New("agent profile: Coding profiles do not use Standard capability groups")
	ErrAgentProfileTestManagement      = errors.New("agent profile: Tests capability requires Test Management")
	ErrAgentProfileValidationFailed    = errors.New("agent profile: readiness validation failed")
	ErrAgentProfileIdentityImmutable   = errors.New("agent profile: centralized and user-owned identity fields are read-only")
	ErrAgentProfileVersionConflict     = errors.New("agent profile: the profile changed since it was loaded")
	ErrAgentProfileLegacyMigrationOnly = errors.New("agent profile: only a Legacy local profile can migrate to an authorized runner")
	ErrAgentProfileRunnerAlreadySet    = errors.New("agent profile: the Coding runner pool is already configured and cannot be reassigned")
)

var agentProfileHandleRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,31}$`)

// CreateStudioProfileRequest is the server-owned Agent Studio creation
// contract. Instructions is presence-aware: nil copies the template's current
// effective prompt, while a non-nil value stores the administrator's edit.
type CreateStudioProfileRequest struct {
	WorkspaceID      int
	CreatedByUserID  int
	TemplateKey      string
	ProfileType      models.AgentProfileType
	ActingUserID     int
	Name             string
	Handle           string
	AvatarURL        string
	Purpose          string
	Instructions     *string
	CapabilityGroups []string
	LLMConnectionID  *int
	Repos            []RepoInput
	TargetPoolID     *int
	RunnerImage      string
	TokenScopes      []string
	TokenTTLMinutes  int
	MaxRunsPerDay    int
	SkillIDs         []int
}

// ProfileValidationError is stable machine-readable readiness feedback.
type ProfileValidationError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Dependency string `json:"dependency,omitempty"`
	// Hint points administrators at the surface that resolves the error.
	Hint string `json:"hint,omitempty"`
}

type ProfileValidationResult struct {
	Ready  bool                     `json:"ready"`
	Errors []ProfileValidationError `json:"errors"`
	// ProfileVersion is the definition version the checks ran against.
	// Activation only succeeds while the stored version still matches it, so a
	// configuration change during validation cannot be marked Ready.
	ProfileVersion int `json:"profile_version"`
}

// UpdateStudioProfileRequest carries the mutable overview fields. Profile type
// and identity class are intentionally absent: both are immutable.
type UpdateStudioProfileRequest struct {
	WorkspaceID     int
	BindingID       int
	ExpectedVersion int
	Name            string
	Handle          string
	AvatarURL       string
	Purpose         string
}

// AgentProfileModelSummary resolves the non-secret model identifier selected
// for a profile. Catalog callers may show it to workspace members without
// exposing the connection id, provider credentials, base URL, or config.
func (s *BindingService) AgentProfileModelSummary(ctx context.Context, binding *models.WorkspaceAgentBinding) string {
	if binding == nil || binding.LLMConnectionID == nil || s.llmRuntime == nil {
		return ""
	}
	cfg, err := s.llmRuntime.ConnectionRuntime(ctx, *binding.LLMConnectionID)
	if err != nil || cfg == nil {
		return ""
	}
	return strings.TrimSpace(cfg.Model)
}

// AgentProfileOwnerName resolves the existing human-owner attribution for a
// grandfathered user-owned identity. Callers must apply the centralized
// user.list/system-admin visibility rule before invoking this method.
func (s *BindingService) AgentProfileOwnerName(ctx context.Context, binding *models.WorkspaceAgentBinding) string {
	if s.db == nil || binding == nil || binding.IdentityClass != models.AgentIdentityUserOwned {
		return ""
	}
	var ownerName string
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(
			NULLIF(TRIM(COALESCE(owner.first_name, '') || ' ' || COALESCE(owner.last_name, '')), ''),
			owner.username,
			''
		)
		FROM users agent
		JOIN users owner ON owner.id = agent.agent_owner_user_id
		WHERE agent.id = ? AND COALESCE(agent.is_agent, FALSE) = TRUE
	`, binding.ActingUserID).Scan(&ownerName)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(ownerName)
}

// UpdateStudioProfile updates purpose for every profile and identity fields
// only for a workspace-managed identity. The expected version makes concurrent
// administrator edits fail explicitly instead of silently overwriting.
func (s *BindingService) UpdateStudioProfile(ctx context.Context, req UpdateStudioProfileRequest) (*models.WorkspaceAgentBinding, error) {
	if s.db == nil {
		return nil, ErrAgentProfileServiceUnavailable
	}
	binding, err := s.repo.Get(ctx, req.BindingID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && binding.WorkspaceID != req.WorkspaceID) {
		return nil, ErrBindingNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load agent profile: %w", err)
	}
	if binding.Lifecycle == models.AgentLifecycleArchived {
		return nil, ErrBindingUnavailable
	}
	if req.ExpectedVersion <= 0 || req.ExpectedVersion != binding.ProfileVersion {
		return nil, ErrAgentProfileVersionConflict
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Handle = strings.ToLower(strings.TrimSpace(req.Handle))
	req.AvatarURL = strings.TrimSpace(req.AvatarURL)
	req.Purpose = strings.TrimSpace(req.Purpose)
	if binding.IdentityClass == models.AgentIdentityWorkspaceManaged {
		if req.Name == "" {
			return nil, ErrAgentProfileNameRequired
		}
		if !agentProfileHandleRE.MatchString(req.Handle) {
			return nil, ErrAgentProfileInvalidHandle
		}
	} else if (req.Name != "" && req.Name != binding.DisplayName) ||
		(req.Handle != "" && req.Handle != binding.Handle) ||
		(req.AvatarURL != "" && req.AvatarURL != binding.AvatarURL) {
		return nil, ErrAgentProfileIdentityImmutable
	}

	err = database.WithTx(s.db, func(tx database.Tx) error {
		result, err := tx.ExecContext(ctx, `
			UPDATE workspace_agent_bindings
			SET purpose = ?,
			    lifecycle = 'draft',
			    profile_version = profile_version + 1,
			    updated_at = CURRENT_TIMESTAMP
			WHERE id = ? AND workspace_id = ? AND profile_version = ? AND lifecycle <> 'archived'
		`, req.Purpose, req.BindingID, req.WorkspaceID, req.ExpectedVersion)
		if err != nil {
			return fmt.Errorf("update agent profile overview: %w", err)
		}
		rows, _ := result.RowsAffected()
		if rows != 1 {
			return ErrAgentProfileVersionConflict
		}
		if binding.IdentityClass != models.AgentIdentityWorkspaceManaged {
			return nil
		}
		result, err = tx.ExecContext(ctx, `
			UPDATE users
			SET username = ?, first_name = ?, last_name = '', avatar_url = ?,
			    updated_at = CURRENT_TIMESTAMP
			WHERE id = ? AND COALESCE(is_agent, FALSE) = TRUE
			  AND agent_owner_user_id IS NULL
		`, req.Handle, req.Name, nullProfileString(req.AvatarURL), binding.ActingUserID)
		if err != nil {
			if database.IsUniqueConstraintError(err) {
				return ErrAgentProfileHandleTaken
			}
			return fmt.Errorf("update workspace-managed agent identity: %w", err)
		}
		rows, _ = result.RowsAffected()
		if rows != 1 {
			return ErrAgentProfileIdentityImmutable
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, req.BindingID)
}

// MigrateLegacyToRunner preserves a grandfathered profile while replacing
// its local in-process runtime with an authorized remote pool. The migrated
// definition returns to Draft and must pass current Coding readiness before
// it can accept new work.
func (s *BindingService) MigrateLegacyToRunner(ctx context.Context, workspaceID, bindingID, poolID int) (*models.WorkspaceAgentBinding, error) {
	binding, err := s.repo.Get(ctx, bindingID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && binding.WorkspaceID != workspaceID) {
		return nil, ErrBindingNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load Legacy agent profile: %w", err)
	}
	if binding.Lifecycle == models.AgentLifecycleArchived {
		return nil, ErrBindingUnavailable
	}
	if binding.ProfileType != models.AgentProfileLegacy {
		return nil, ErrAgentProfileLegacyMigrationOnly
	}
	if err := s.validateTargetPool(workspaceID, poolID); err != nil {
		return nil, err
	}
	rows, err := s.repo.MigrateLegacyToRunner(ctx, bindingID, workspaceID, poolID)
	if err != nil {
		return nil, err
	}
	if rows != 1 {
		return nil, ErrAgentProfileLegacyMigrationOnly
	}
	return s.repo.Get(ctx, bindingID)
}

// ConnectCodingRunner authorizes the first runner pool for a Coding profile
// created as an incomplete Draft. Reassignment is deliberately excluded.
func (s *BindingService) ConnectCodingRunner(ctx context.Context, workspaceID, bindingID, poolID int) (*models.WorkspaceAgentBinding, error) {
	binding, err := s.repo.Get(ctx, bindingID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && binding.WorkspaceID != workspaceID) {
		return nil, ErrBindingNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load Coding agent profile: %w", err)
	}
	if binding.Lifecycle == models.AgentLifecycleArchived {
		return nil, ErrBindingUnavailable
	}
	if binding.ProfileType != models.AgentProfileCoding || binding.TargetPoolID != nil {
		return nil, ErrAgentProfileRunnerAlreadySet
	}
	if err := s.validateTargetPool(workspaceID, poolID); err != nil {
		return nil, err
	}
	rows, err := s.repo.ConnectCodingRunner(ctx, bindingID, workspaceID, poolID)
	if err != nil {
		return nil, err
	}
	if rows != 1 {
		return nil, ErrAgentProfileRunnerAlreadySet
	}
	return s.repo.Get(ctx, bindingID)
}

// ValidateRunnerPool exposes the same workspace-scoped authorization check
// used by profile creation and execution to runner-onboarding handlers.
func (s *BindingService) ValidateRunnerPool(workspaceID, poolID int) error {
	return s.validateTargetPool(workspaceID, poolID)
}

// CreateStudioProfile creates a Draft from one of the approved templates. In
// workspace-managed mode the user, conditional Editor role, binding,
// repositories, and skill attachments share one transaction. In fallback mode
// the same transaction creates the binding around an eligible centralized
// identity.
func (s *BindingService) CreateStudioProfile(ctx context.Context, req CreateStudioProfileRequest) (*models.WorkspaceAgentBinding, error) {
	if s.db == nil || s.prompts == nil {
		return nil, ErrAgentProfileServiceUnavailable
	}
	if req.WorkspaceID <= 0 || req.CreatedByUserID <= 0 {
		return nil, errors.New("agent profile: workspace and creator are required")
	}
	template, ok := s.prompts.AgentTemplate(strings.TrimSpace(req.TemplateKey))
	if !ok {
		return nil, ErrAgentProfileInvalidTemplate
	}
	if req.ProfileType == "" {
		req.ProfileType = template.DefaultType
	}
	if req.ProfileType != models.AgentProfileStandard && req.ProfileType != models.AgentProfileCoding {
		return nil, ErrAgentProfileInvalidType
	}
	if req.LLMConnectionID == nil {
		return nil, ErrLLMConnectionRequired
	}
	if s.llmRuntime != nil {
		if _, err := s.llmRuntime.ConnectionRuntime(ctx, *req.LLMConnectionID); err != nil {
			return nil, ErrLLMConnectionInvalid
		}
	}
	if len(req.TokenScopes) > 0 {
		if err := auth.ValidateAgentScopes(req.TokenScopes); err != nil {
			return nil, fmt.Errorf("binding service: %w", err)
		}
	}
	if req.TokenTTLMinutes > 0 && req.TokenTTLMinutes > int(MaxAgentTokenTTL.Minutes()) {
		return nil, ErrBindingTokenTTLOverCap
	}
	if req.Instructions == nil {
		value := template.Instructions
		req.Instructions = &value
	}
	if len(*req.Instructions) > maxBindingInstructionsLen {
		return nil, ErrBindingInstructionsTooLong
	}

	groups, err := s.normalizeStudioCapabilityGroups(ctx, req.ProfileType, req.CapabilityGroups)
	if err != nil {
		return nil, err
	}
	repos, err := normalizeRepoInputs(req.Repos)
	if err != nil {
		return nil, err
	}
	runnerImage, err := validateRunnerImage(req.RunnerImage)
	if err != nil {
		return nil, err
	}
	if req.ProfileType == models.AgentProfileStandard {
		if len(repos) > 0 || req.TargetPoolID != nil || runnerImage != "" {
			return nil, ErrAgentProfileStandardRuntimeOnly
		}
	} else {
		if len(groups) > 0 {
			return nil, ErrAgentProfileCodingTools
		}
		if req.TargetPoolID != nil {
			if err := s.validateTargetPool(req.WorkspaceID, *req.TargetPoolID); err != nil {
				return nil, err
			}
		}
		if runnerImage != "" && req.TargetPoolID == nil {
			return nil, ErrBindingRunnerImageRequiresPool
		}
	}

	managed, err := s.workspaceManagedAgentsEnabled(ctx)
	if err != nil {
		return nil, err
	}
	var centralized *ActingIdentity
	if !managed {
		centralized, err = s.identity.Resolve(ctx, req.CreatedByUserID, req.ActingUserID, req.WorkspaceID)
		if err != nil {
			return nil, err
		}
		if centralized.Kind != ActingIdentityKindCentralized {
			return nil, ErrAgentProfileCentralizedRequired
		}
	} else {
		req.Handle = strings.ToLower(strings.TrimSpace(req.Handle))
		req.Name = strings.TrimSpace(req.Name)
		if !agentProfileHandleRE.MatchString(req.Handle) {
			return nil, ErrAgentProfileInvalidHandle
		}
		if req.Name == "" {
			return nil, ErrAgentProfileNameRequired
		}
	}

	created, err := database.WithTxResult(s.db, func(tx database.Tx) (*models.WorkspaceAgentBinding, error) {
		var actingUserID int
		identityClass := models.AgentIdentityCentralized
		if managed {
			createdID, err := createWorkspaceManagedAgentIdentity(ctx, tx, req)
			if err != nil {
				return nil, err
			}
			actingUserID = createdID
			identityClass = models.AgentIdentityWorkspaceManaged
		} else {
			actingUserID = centralized.UserID
		}

		binding := &models.WorkspaceAgentBinding{
			WorkspaceID:      req.WorkspaceID,
			ActingUserID:     actingUserID,
			ActingUserKind:   ActingIdentityKindCentralized,
			ProfileType:      req.ProfileType,
			Lifecycle:        models.AgentLifecycleDraft,
			ProfileVersion:   1,
			IdentityClass:    identityClass,
			Purpose:          req.Purpose,
			CapabilityGroups: groups,
			LLMConnectionID:  req.LLMConnectionID,
			TargetPoolID:     req.TargetPoolID,
			RunnerImage:      runnerImage,
			TokenScopes:      append([]string(nil), req.TokenScopes...),
			TokenTTLMinutes:  req.TokenTTLMinutes,
			MaxRunsPerDay:    req.MaxRunsPerDay,
			Instructions:     *req.Instructions,
			CreatedByUserID:  req.CreatedByUserID,
			Repos:            repos,
		}
		txBindings := repository.NewWorkspaceAgentBindingRepositoryTx(tx)
		id, err := txBindings.Insert(ctx, binding)
		if err != nil {
			return nil, err
		}
		binding.ID = id
		if len(req.SkillIDs) > 0 {
			txSkills := repository.NewWorkspaceAgentSkillRepositoryTx(tx)
			if err := txSkills.ReplaceBindingSkills(ctx, id, req.WorkspaceID, req.SkillIDs); err != nil {
				return nil, fmt.Errorf("agent profile: attach knowledge: %w", err)
			}
		}
		return binding, nil
	})
	if err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, created.ID)
}

func (s *BindingService) workspaceManagedAgentsEnabled(ctx context.Context) (bool, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM system_settings WHERE key = 'workspace_managed_agents'`).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("read workspace-managed agents setting: %w", err)
	}
	return strings.EqualFold(value, "true"), nil
}

func createWorkspaceManagedAgentIdentity(ctx context.Context, tx database.Tx, req CreateStudioProfileRequest) (int, error) {
	email := fmt.Sprintf("agent-%s-w%d@agents.local", req.Handle, req.WorkspaceID)
	id, err := repository.CreateWorkspaceManagedAgentIdentity(ctx, tx, repository.WorkspaceManagedAgentIdentityParams{
		Email:           email,
		Username:        req.Handle,
		Name:            req.Name,
		AvatarURL:       req.AvatarURL,
		WorkspaceID:     req.WorkspaceID,
		GrantedByUserID: req.CreatedByUserID,
	})
	if err != nil {
		if errors.Is(err, repository.ErrDuplicateEntry) {
			return 0, ErrAgentProfileHandleTaken
		}
		return 0, err
	}
	return id, nil
}

func nullProfileString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.TrimSpace(value)
}

func (s *BindingService) normalizeStudioCapabilityGroups(ctx context.Context, profileType models.AgentProfileType, requested []string) ([]string, error) {
	if profileType == models.AgentProfileCoding {
		if len(requested) == 0 {
			return nil, nil
		}
		return nil, ErrAgentProfileCodingTools
	}
	groups := make([]string, 0, len(requested))
	for _, raw := range requested {
		group := strings.TrimSpace(raw)
		if group == "" || !s.standardCapabilityGroups[group] {
			return nil, ErrAgentProfileInvalidCapabilities
		}
		if group == string(agentstudio.CapabilityReadComment) {
			continue
		}
		if !slices.Contains(groups, group) {
			groups = append(groups, group)
		}
	}
	slices.Sort(groups)
	if slices.Contains(groups, string(agentstudio.CapabilityTests)) {
		enabled, err := s.systemSettingEnabled(ctx, "test_management_enabled", true)
		if err != nil {
			return nil, err
		}
		if !enabled {
			return nil, ErrAgentProfileTestManagement
		}
	}
	return groups, nil
}

func (s *BindingService) systemSettingEnabled(ctx context.Context, key string, fallback bool) (bool, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM system_settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return fallback, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s setting: %w", key, err)
	}
	return strings.EqualFold(value, "true"), nil
}

// ValidateStudioProfile computes readiness without mutating persisted
// lifecycle. The errors are intentionally dependency-specific so the creation
// journey can link administrators to the exact recovery surface.
func (s *BindingService) ValidateStudioProfile(ctx context.Context, workspaceID, bindingID int) (*ProfileValidationResult, error) {
	if s.db == nil || s.permissions == nil {
		return nil, ErrAgentProfileServiceUnavailable
	}
	binding, err := s.repo.Get(ctx, bindingID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && binding.WorkspaceID != workspaceID) {
		return nil, ErrBindingNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load agent profile: %w", err)
	}

	result := &ProfileValidationResult{Errors: []ProfileValidationError{}, ProfileVersion: binding.ProfileVersion}
	var active, isAgent bool
	if err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(is_active, false), COALESCE(is_agent, false)
		FROM users WHERE id = ?
	`, binding.ActingUserID).Scan(&active, &isAgent); err != nil || !active || !isAgent {
		result.Errors = append(result.Errors, ProfileValidationError{
			Code: "acting_identity_unavailable", Message: "The acting agent identity is inactive or missing.", Dependency: "identity",
		})
	}
	if binding.LLMConnectionID == nil {
		result.Errors = append(result.Errors, ProfileValidationError{
			Code: "llm_connection_required", Message: "Select an enabled LLM connection.", Dependency: "llm_connection",
		})
	} else if s.llmRuntime == nil {
		result.Errors = append(result.Errors, ProfileValidationError{
			Code: "llm_runtime_unavailable", Message: "LLM connections cannot be validated on this server.", Dependency: "llm_connection",
		})
	} else if _, err := s.llmRuntime.ConnectionRuntime(ctx, *binding.LLMConnectionID); err != nil {
		result.Errors = append(result.Errors, ProfileValidationError{
			Code: "llm_connection_unavailable", Message: "The selected LLM connection is missing or disabled.", Dependency: "llm_connection",
		})
	}
	s.validateProfilePermissions(binding, result)

	switch binding.ProfileType {
	case models.AgentProfileStandard:
		if s.standardRuns == nil {
			result.Errors = append(result.Errors, ProfileValidationError{
				Code: "standard_runtime_unavailable", Message: "The built-in Standard agent runtime is not configured on this server.", Dependency: "standard_runtime",
			})
		}
	case models.AgentProfileCoding:
		if len(binding.Repos) == 0 {
			result.Errors = append(result.Errors, ProfileValidationError{
				Code: "repository_required", Message: "Select at least one centrally configured SCM repository.", Dependency: "repositories",
			})
		}
		if binding.TargetPoolID == nil {
			result.Errors = append(result.Errors, ProfileValidationError{
				Code: "runner_authorization_required", Message: "Authorize a runner pool before making this Coding profile Ready.", Dependency: "runner",
			})
		} else if err := s.validateTargetPool(workspaceID, *binding.TargetPoolID); err != nil {
			result.Errors = append(result.Errors, ProfileValidationError{
				Code: "runner_pool_unavailable", Message: "The selected runner pool is missing, disabled, or unavailable to this workspace.", Dependency: "runner",
			})
		}
	case models.AgentProfileLegacy:
		if len(binding.Repos) == 0 {
			result.Errors = append(result.Errors, ProfileValidationError{
				Code: "repository_required", Message: "Select at least one centrally configured SCM repository.", Dependency: "repositories",
			})
		}
		if s.runs == nil || !s.runs.LocalExecutionEnabled() {
			result.Errors = append(result.Errors, ProfileValidationError{
				Code: "legacy_runtime_unavailable", Message: "The preserved Legacy local runtime is not available on this server.", Dependency: "legacy_runtime",
			})
		}
	default:
		result.Errors = append(result.Errors, ProfileValidationError{
			Code: "unsupported_profile_type", Message: "This profile runtime cannot be activated through Agent Studio.", Dependency: "profile_type",
		})
	}
	result.Ready = len(result.Errors) == 0
	return result, nil
}

func (s *BindingService) validateProfilePermissions(binding *models.WorkspaceAgentBinding, result *ProfileValidationResult) {
	required := []string{models.PermissionItemView, models.PermissionItemComment, models.PermissionItemEdit}
	if binding.ProfileType == models.AgentProfileStandard {
		for _, group := range binding.CapabilityGroups {
			switch agentstudio.CapabilityGroup(group) {
			case agentstudio.CapabilityIssueManagement:
				required = append(required, models.PermissionItemCreate)
			case agentstudio.CapabilityKnowledgeDiagrams:
				required = append(required, models.PermissionPageView, models.PermissionPageCreate, models.PermissionPageEdit)
			case agentstudio.CapabilityActions:
				required = append(required, models.PermissionActionManage)
			case agentstudio.CapabilityTests:
				required = append(required, models.PermissionTestView, models.PermissionTestExecute)
			}
		}
	}
	required = uniqueProfileStrings(required)
	grants, err := s.permissions.HasWorkspacePermissions(binding.ActingUserID, binding.WorkspaceID, required)
	if err != nil {
		result.Errors = append(result.Errors, ProfileValidationError{
			Code: "permission_check_failed", Message: "The acting identity's workspace permissions could not be validated.", Dependency: "permissions",
		})
		return
	}
	for _, permission := range required {
		if !grants[permission] {
			message := fmt.Sprintf("Grant the acting identity the %s permission.", permission)
			hint := fmt.Sprintf("Assign this agent a workspace role that includes %s in Workspace settings → Members.", permission)
			if permission == models.PermissionItemEdit {
				message = "Grant the acting identity Editor access."
				hint = "Assign this agent the Editor role in Workspace settings → Members."
			}
			result.Errors = append(result.Errors, ProfileValidationError{
				Code: "permission_missing", Message: message, Dependency: permission, Hint: hint,
			})
		}
	}
}

func uniqueProfileStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if !slices.Contains(out, value) {
			out = append(out, value)
		}
	}
	return out
}

// ActivateStudioProfile validates current dependencies and transitions the
// saved Draft/Paused definition to Ready only when every check succeeds.
func (s *BindingService) ActivateStudioProfile(ctx context.Context, workspaceID, bindingID int) (*models.WorkspaceAgentBinding, *ProfileValidationResult, error) {
	validation, err := s.ValidateStudioProfile(ctx, workspaceID, bindingID)
	if err != nil {
		return nil, nil, err
	}
	return s.activateStudioProfile(ctx, workspaceID, bindingID, validation)
}

// activateStudioProfile transitions a validated definition to Ready, but only
// while the stored profile version still matches the one the validation ran
// against. A concurrent configuration change bumps the version, so this refuses
// to mark a changed definition Ready using validation of an older one.
func (s *BindingService) activateStudioProfile(ctx context.Context, workspaceID, bindingID int, validation *ProfileValidationResult) (*models.WorkspaceAgentBinding, *ProfileValidationResult, error) {
	if !validation.Ready {
		return nil, validation, ErrAgentProfileValidationFailed
	}
	rows, err := s.repo.SetLifecycleIfVersion(ctx, bindingID, workspaceID, models.AgentLifecycleReady, validation.ProfileVersion)
	if err != nil {
		return nil, validation, err
	}
	if rows == 0 {
		return nil, validation, ErrAgentProfileVersionConflict
	}
	binding, err := s.repo.Get(ctx, bindingID)
	if err != nil {
		return nil, validation, err
	}
	return binding, validation, nil
}

// RunPrivateProfileTest exercises the profile's real runtime without making
// testing a readiness prerequisite. Standard tests execute synchronously in
// the built-in agent loop; Coding and Legacy tests return a durable ephemeral
// run id that the UI can observe and cancel.
func (s *BindingService) RunPrivateProfileTest(ctx context.Context, workspaceID, bindingID, triggeredByUserID int, prompt string) (*PrivateProfileTestResult, error) {
	binding, err := s.repo.Get(ctx, bindingID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && binding.WorkspaceID != workspaceID) {
		return nil, ErrBindingNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load profile for private test: %w", err)
	}
	if binding.Lifecycle == models.AgentLifecycleArchived {
		return nil, ErrBindingUnavailable
	}
	switch binding.ProfileType {
	case models.AgentProfileStandard:
		tester, ok := s.standardRuns.(StandardPrivateTester)
		if !ok {
			return nil, ErrStandardPrivateTestUnavailable
		}
		result, err := tester.RunPrivateTest(ctx, binding, workspaceID, triggeredByUserID, prompt)
		if err != nil {
			return nil, err
		}
		return &PrivateProfileTestResult{
			Mode:       "standard",
			Status:     models.AgentRunStatusSucceeded,
			Answer:     result.Answer,
			Iterations: result.Iterations,
			ToolCalls:  result.ToolCalls,
		}, nil
	case models.AgentProfileCoding, models.AgentProfileLegacy:
		runID, err := s.startTestRun(ctx, bindingID, workspaceID, triggeredByUserID, prompt)
		if err != nil {
			return nil, err
		}
		return &PrivateProfileTestResult{
			Mode:   "coding",
			RunID:  runID,
			Status: models.AgentRunStatusQueued,
		}, nil
	default:
		return nil, ErrBindingUnavailable
	}
}
