package services

import (
	"context"
	"errors"
	"strings"

	"windshift/internal/agentskills"
	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
)

const maxAgentSkillPages = 25

var (
	ErrAgentSkillForbidden          = errors.New("agent skill operation forbidden")
	ErrAgentSkillValidation         = errors.New("agent skill validation failed")
	ErrAgentSkillActivationTooLarge = errors.New("agent skill activation exceeds the supported budget")
)

type AgentSkillValidationError struct{ Message string }

func (e *AgentSkillValidationError) Error() string { return e.Message }

func (e *AgentSkillValidationError) Unwrap() error { return ErrAgentSkillValidation }

type AgentSkillInput struct {
	Name        string
	Description string
	Body        string
	Enabled     bool
	PageIDs     []int
}

type AgentSkillPatch struct {
	Name        *string
	Description *string
	Body        *string
	Enabled     *bool
	PageIDs     *[]int
}

// AgentSkillApplicationService serves workspace admins and immutable run grants through one contract.
type AgentSkillApplicationService struct {
	db          database.Database
	repository  *repository.WorkspaceAgentSkillRepository
	runs        *repository.AgentRunRepository
	permissions *PermissionService
}

func NewAgentSkillApplicationService(db database.Database, permissions *PermissionService) *AgentSkillApplicationService {
	return &AgentSkillApplicationService{
		db: db, repository: repository.NewWorkspaceAgentSkillRepository(db),
		runs: repository.NewAgentRunRepository(db), permissions: permissions,
	}
}

func (s *AgentSkillApplicationService) List(ctx context.Context, userID, tokenID, workspaceID int) ([]*models.WorkspaceAgentSkill, error) {
	if grants, run, err := s.runSkills(ctx, tokenID, workspaceID); err != nil {
		return nil, err
	} else if run {
		items := make([]*models.WorkspaceAgentSkill, len(grants))
		for i, grant := range grants {
			items[i] = &models.WorkspaceAgentSkill{ID: grant.ID, WorkspaceID: workspaceID, Name: grant.Name, Description: grant.Description, Enabled: true}
		}
		return items, nil
	}
	if err := s.requireAdmin(userID, workspaceID); err != nil {
		return nil, err
	}
	items, err := s.repository.ListForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []*models.WorkspaceAgentSkill{}
	}
	for _, item := range items {
		if err := s.populate(ctx, item); err != nil {
			return nil, err
		}
	}
	return items, nil
}

func (s *AgentSkillApplicationService) Get(ctx context.Context, userID, tokenID, workspaceID, skillID int) (*models.WorkspaceAgentSkill, error) {
	if grants, run, err := s.runSkills(ctx, tokenID, workspaceID); err != nil {
		return nil, err
	} else if run {
		for _, grant := range grants {
			if grant.ID == skillID {
				if grant.Error != "" {
					return nil, ErrAgentSkillActivationTooLarge
				}
				return &models.WorkspaceAgentSkill{ID: grant.ID, WorkspaceID: workspaceID, Name: grant.Name, Description: grant.Description, Body: grant.Body, Enabled: true}, nil
			}
		}
		return nil, repository.ErrNotFound
	}
	if err := s.requireAdmin(userID, workspaceID); err != nil {
		return nil, err
	}
	item, err := s.repository.Get(ctx, skillID, workspaceID)
	if err != nil {
		return nil, err
	}
	if err := s.populate(ctx, item); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *AgentSkillApplicationService) Create(ctx context.Context, actor AuditActor, workspaceID int, input AgentSkillInput) (*models.WorkspaceAgentSkill, error) {
	if err := s.requireAdmin(actor.UserID, workspaceID); err != nil {
		return nil, err
	}
	item, pages, err := s.prepare(ctx, workspaceID, input)
	if err != nil {
		return nil, err
	}
	item.CreatedByUserID = &actor.UserID
	id, err := s.repository.Insert(ctx, item)
	if err != nil {
		return nil, err
	}
	item.ID = id
	if err := s.repository.ReplaceSkillPageSnapshots(ctx, id, pages); err != nil {
		return nil, err
	}
	if err := s.populate(ctx, item); err != nil {
		return nil, err
	}
	emitServiceAudit(s.db, actor, "agent_skill.create", "workspace_agent_skill", &id, item.Name, map[string]any{"workspace_id": workspaceID})
	return item, nil
}

func (s *AgentSkillApplicationService) Patch(ctx context.Context, actor AuditActor, workspaceID, skillID int, patch AgentSkillPatch) (*models.WorkspaceAgentSkill, error) {
	if err := s.requireAdmin(actor.UserID, workspaceID); err != nil {
		return nil, err
	}
	current, err := s.repository.Get(ctx, skillID, workspaceID)
	if err != nil {
		return nil, err
	}
	if patch.Name != nil {
		current.Name = *patch.Name
	}
	if patch.Description != nil {
		current.Description = *patch.Description
	}
	if patch.Body != nil {
		current.Body = *patch.Body
	}
	if patch.Enabled != nil {
		current.Enabled = *patch.Enabled
	}
	pageIDs := []int{}
	if patch.PageIDs != nil {
		pageIDs = *patch.PageIDs
	} else {
		refs, err := s.repository.PageRefsForSkill(ctx, skillID)
		if err != nil {
			return nil, err
		}
		for _, ref := range refs {
			pageIDs = append(pageIDs, ref.ID)
		}
	}
	prepared, pages, err := s.prepare(ctx, workspaceID, AgentSkillInput{
		Name: current.Name, Description: current.Description, Body: current.Body,
		Enabled: current.Enabled, PageIDs: pageIDs,
	})
	if err != nil {
		return nil, err
	}
	prepared.ID = skillID
	if n, err := s.repository.Update(ctx, prepared); err != nil {
		return nil, err
	} else if n == 0 {
		return nil, repository.ErrNotFound
	}
	if err := s.repository.ReplaceSkillPageSnapshots(ctx, skillID, pages); err != nil {
		return nil, err
	}
	if err := s.populate(ctx, prepared); err != nil {
		return nil, err
	}
	emitServiceAudit(s.db, actor, "agent_skill.update", "workspace_agent_skill", &skillID, prepared.Name, map[string]any{"workspace_id": workspaceID})
	return prepared, nil
}

func (s *AgentSkillApplicationService) Delete(ctx context.Context, actor AuditActor, workspaceID, skillID int) error {
	if err := s.requireAdmin(actor.UserID, workspaceID); err != nil {
		return err
	}
	n, err := s.repository.Delete(ctx, skillID, workspaceID)
	if err != nil {
		return err
	}
	if n == 0 {
		return repository.ErrNotFound
	}
	emitServiceAudit(s.db, actor, "agent_skill.delete", "workspace_agent_skill", &skillID, "", map[string]any{"workspace_id": workspaceID})
	return nil
}

func (s *AgentSkillApplicationService) runSkills(ctx context.Context, tokenID, workspaceID int) ([]models.SkillGrant, bool, error) {
	if tokenID == 0 {
		return nil, false, nil
	}
	_, runWorkspaceID, grants, status, err := s.runs.GetRunByTokenID(ctx, tokenID)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if runWorkspaceID != workspaceID || status != models.AgentRunStatusRunning || grants == nil {
		return nil, true, repository.ErrNotFound
	}
	return grants.Skills, true, nil
}

func (s *AgentSkillApplicationService) requireAdmin(userID, workspaceID int) error {
	allowed, err := s.permissions.HasWorkspacePermission(userID, workspaceID, models.PermissionWorkspaceAdmin)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrAgentSkillForbidden
	}
	return nil
}

func (s *AgentSkillApplicationService) prepare(ctx context.Context, workspaceID int, input AgentSkillInput) (*models.WorkspaceAgentSkill, []models.SkillPageReference, error) {
	sanitize.ApplyAll(
		sanitize.Pair{Target: &input.Name, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &input.Description, Policy: sanitize.PlainTextField},
		sanitize.Pair{Target: &input.Body, Policy: sanitize.LongDocument},
	)
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	if err := agentskills.ValidateMetadata(input.Name, input.Description); err != nil {
		return nil, nil, fmtAgentSkillValidation(err.Error())
	}
	if len(input.Name) > 120 {
		return nil, nil, fmtAgentSkillValidation("name must be at most 120 characters")
	}
	if len(input.Description) > 500 {
		return nil, nil, fmtAgentSkillValidation("description must be at most 500 characters")
	}
	if len(input.Body) > agentskills.MaxBodyBytes {
		return nil, nil, fmtAgentSkillValidation("body must be at most 64 KiB")
	}
	if len(input.PageIDs) > maxAgentSkillPages {
		return nil, nil, fmtAgentSkillValidation("a skill may reference at most 25 pages")
	}
	pages, err := s.repository.ResolveSkillPageSnapshots(ctx, workspaceID, input.PageIDs)
	if err != nil {
		return nil, nil, err
	}
	_, usage, err := agentskills.RenderActivation(input.Body, pages)
	if errors.Is(err, agentskills.ErrActivationTooLarge) {
		return nil, nil, ErrAgentSkillActivationTooLarge
	}
	if err != nil {
		return nil, nil, err
	}
	return &models.WorkspaceAgentSkill{
		WorkspaceID: workspaceID, Name: input.Name, Description: input.Description,
		Body: input.Body, Enabled: input.Enabled, Usage: agentSkillUsageModel(usage),
	}, pages, nil
}

func (s *AgentSkillApplicationService) populate(ctx context.Context, item *models.WorkspaceAgentSkill) error {
	refs, err := s.repository.PageRefsForSkill(ctx, item.ID)
	if err != nil {
		return err
	}
	item.Pages = refs
	_, usage, _ := agentskills.RenderActivation(item.Body, refs)
	item.Usage = agentSkillUsageModel(usage)
	for i := range refs {
		bytes, runes, prefixBytes, prefixRunes := agentskills.PageSnapshotUsage(refs[i])
		refs[i].ActivationBytes = bytes
		refs[i].ActivationRunes = runes
		item.Usage.PagePrefixBytes = prefixBytes
		item.Usage.PagePrefixRunes = prefixRunes
	}
	return nil
}

func agentSkillUsageModel(usage agentskills.Usage) *models.SkillActivationUsage {
	return &models.SkillActivationUsage{
		Bytes: usage.Bytes, EstimatedTokens: usage.EstimatedTokens,
		MaxBytes: usage.MaxBytes, MaxTokens: usage.MaxTokens,
	}
}

func fmtAgentSkillValidation(message string) error {
	return &AgentSkillValidationError{Message: message}
}
