package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"windshift/internal/models"
	"windshift/internal/sanitize"
	"windshift/internal/utils"
)

var ErrPlanningForbidden = errors.New("planning object forbidden")

// ExternalReleaseOptions is the provider-neutral release creation contract.
type ExternalReleaseOptions struct {
	TagName         string
	TargetCommitish string
	Name            string
	Body            string
	IsDraft         bool
	IsPrerelease    bool
}

type ExternalRelease struct {
	ID           string
	URL          string
	TagName      string
	TagURL       string
	Name         string
	Body         string
	Status       string
	Assets       []models.SCMReleaseAsset
	IsDraft      bool
	IsPrerelease bool
	ReleasedAt   *time.Time
}

type ExternalTag struct {
	Name string
	URL  string
}

type PlanningReleaseProvider interface {
	CreateRelease(context.Context, string, string, ExternalReleaseOptions) (*ExternalRelease, error)
	ListReleases(context.Context, string, string) ([]ExternalRelease, error)
	ListTags(context.Context, string, string) ([]ExternalTag, error)
}

type PlanningReleaseResolver func(context.Context, int, int) (PlanningReleaseProvider, error)

type PlanningApplicationService struct {
	planning   *PlanningService
	permission *PermissionService
	completion *IterationCompletionService
	releases   PlanningReleaseResolver
}

func NewPlanningApplicationService(planning *PlanningService, permission *PermissionService, completion *IterationCompletionService, releases PlanningReleaseResolver) *PlanningApplicationService {
	return &PlanningApplicationService{planning: planning, permission: permission, completion: completion, releases: releases}
}

func (s *PlanningApplicationService) ListMilestones(userID int, params MilestoneListParams) ([]MilestoneResult, int, error) {
	if err := s.requireListScope(userID, params.WorkspaceID); err != nil {
		return nil, 0, err
	}
	params.IncludeGlobal = params.WorkspaceID == nil || params.IncludeGlobal
	if params.IsGlobal {
		params.WorkspaceID = nil
		params.WorkspaceIDs = nil
	}
	if params.WorkspaceID == nil && !params.IsGlobal {
		workspaceIDs, err := s.permission.AccessibleWorkspaceIDs(userID)
		if err != nil {
			return nil, 0, err
		}
		params.WorkspaceIDs = workspaceIDs
	}
	return s.planning.ListMilestones(params)
}

func (s *PlanningApplicationService) GetMilestone(userID, id int) (*MilestoneResult, error) {
	result, err := s.planning.GetMilestone(id)
	if err != nil {
		return nil, err
	}
	if err := s.requireRead(userID, result.IsGlobal, result.WorkspaceID); err != nil {
		return nil, err
	}
	result.Releases, err = s.planning.ListMilestoneReleases(id)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *PlanningApplicationService) CreateMilestone(userID int, actor AuditActor, params CreateMilestoneParams) (*MilestoneResult, error) {
	if err := s.requireWrite(userID, params.IsGlobal, params.WorkspaceID, models.PermissionMilestoneCreate); err != nil {
		return nil, err
	}
	params.Name = sanitize.PlainTextField.Sanitize(params.Name)
	params.Description = sanitize.Comment.Sanitize(params.Description)
	params.AuditActor = &actor
	return s.planning.CreateMilestone(params)
}

func (s *PlanningApplicationService) UpdateMilestone(userID int, actor AuditActor, params UpdateMilestoneParams) (*MilestoneResult, error) {
	existing, err := s.planning.GetMilestone(params.ID)
	if err != nil {
		return nil, err
	}
	if err := s.requireWrite(userID, existing.IsGlobal, existing.WorkspaceID, models.PermissionMilestoneCreate); err != nil {
		return nil, err
	}
	params.WorkspaceID = existing.WorkspaceID
	params.Name = sanitize.PlainTextField.Sanitize(params.Name)
	params.Description = sanitize.Comment.Sanitize(params.Description)
	params.AuditActor = &actor
	return s.planning.UpdateMilestone(params)
}

func (s *PlanningApplicationService) DeleteMilestone(userID int, actor AuditActor, id int) error {
	existing, err := s.planning.GetMilestone(id)
	if err != nil {
		return err
	}
	if err := s.requireWrite(userID, existing.IsGlobal, existing.WorkspaceID, models.PermissionMilestoneCreate); err != nil {
		return err
	}
	return s.planning.DeleteMilestone(id, actor)
}

func (s *PlanningApplicationService) ReorderMilestones(userID int, actor AuditActor, scope MilestoneScope, ids []int) error {
	if err := s.requireWrite(userID, scope.IsGlobal, scope.WorkspaceID, models.PermissionMilestoneCreate); err != nil {
		return err
	}
	return s.planning.ReorderMilestones(scope, ids, actor)
}

func (s *PlanningApplicationService) GetMilestoneProgress(userID, id int) (*MilestoneProgressReport, error) {
	if _, err := s.GetMilestone(userID, id); err != nil {
		return nil, err
	}
	workspaceIDs, err := s.permission.AccessibleWorkspaceIDs(userID)
	if err != nil {
		return nil, err
	}
	return s.planning.GetMilestoneProgress(id, workspaceIDs)
}

func (s *PlanningApplicationService) GetMilestoneTestStatistics(userID, id int) (*MilestoneTestStats, error) {
	if _, err := s.GetMilestone(userID, id); err != nil {
		return nil, err
	}
	workspaceIDs, err := s.permission.AccessibleWorkspaceIDs(userID)
	if err != nil {
		return nil, err
	}
	return s.planning.GetMilestoneTestStatistics(id, workspaceIDs)
}

func (s *PlanningApplicationService) GetMilestoneTestStatisticsBatch(userID int, ids []int) (map[int]*MilestoneTestStats, error) {
	workspaceIDs, err := s.permission.AccessibleWorkspaceIDs(userID)
	if err != nil {
		return nil, err
	}
	return s.planning.GetMilestoneTestStatisticsBatch(ids, workspaceIDs)
}

type ReleaseMilestoneInput struct {
	Mode            string
	ConnectionID    int
	RepositoryID    int
	Repository      string
	IdempotencyKey  string
	TagName         string
	Name            string
	Body            string
	IsDraft         bool
	IsPrerelease    bool
	TargetCommitish string
}

func (s *PlanningApplicationService) ReleaseMilestone(ctx context.Context, userID int, actor AuditActor, id int, input ReleaseMilestoneInput) (*MilestoneResult, error) {
	existing, err := s.planning.GetMilestone(id)
	if err != nil {
		return nil, err
	}
	if err := s.requireWrite(userID, existing.IsGlobal, existing.WorkspaceID, models.PermissionMilestoneCreate); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.TagName) == "" {
		return nil, planningValidationError("tag_name", "tag_name is required")
	}
	input.Mode = strings.ToLower(strings.TrimSpace(input.Mode))
	if input.Mode == "" {
		input.Mode = "create"
	}
	if input.Mode != "create" && input.Mode != "attach" {
		return nil, planningValidationError("mode", "mode must be 'create' or 'attach'")
	}
	if len(input.IdempotencyKey) > 200 {
		return nil, planningValidationError("idempotency_key", "Idempotency-Key must be at most 200 characters")
	}

	var connectionID *int
	var workspaceRepositoryID *int
	var repositoryName *string
	var provider PlanningReleaseProvider
	var owner, repo string
	if input.ConnectionID > 0 {
		workspaceID, err := s.planning.GetSCMConnectionWorkspaceID(input.ConnectionID)
		if err != nil || workspaceID == 0 {
			return nil, planningValidationError("connection_id", "SCM connection not found")
		}
		allowed, err := s.permission.HasWorkspacePermission(userID, workspaceID, models.PermissionItemEdit)
		if err != nil {
			return nil, err
		}
		if !allowed {
			return nil, ErrPlanningForbidden
		}
		linked, err := s.planning.ResolveLinkedSCMRepository(input.ConnectionID, input.RepositoryID, input.Repository)
		if err != nil {
			return nil, err
		}
		var ok bool
		owner, repo, ok = utils.SplitRepositoryPath(linked.RepositoryName)
		if !ok {
			return nil, planningValidationError("repository", "repository must be in namespace/project format")
		}
		connectionID = &input.ConnectionID
		workspaceRepositoryID = &linked.ID
		repositoryName = &linked.RepositoryName
		if s.releases != nil {
			provider, err = s.releases(ctx, input.ConnectionID, userID)
			if err != nil {
				return nil, err
			}
		}
	}
	var existingRelease *ExternalRelease
	if input.Mode == "attach" {
		if provider == nil {
			return nil, planningValidationError("connection_id", "attach mode requires an SCM connection and linked repository")
		}
		releases, err := provider.ListReleases(ctx, owner, repo)
		if err != nil {
			return nil, err
		}
		for i := range releases {
			if releases[i].TagName == input.TagName {
				existingRelease = &releases[i]
				break
			}
		}
		if existingRelease == nil {
			tags, err := provider.ListTags(ctx, owner, repo)
			if err != nil {
				return nil, err
			}
			for _, tag := range tags {
				if tag.Name == input.TagName {
					existingRelease = &ExternalRelease{TagName: tag.Name, TagURL: tag.URL, Status: "tag_only"}
					break
				}
			}
		}
		if existingRelease == nil {
			return nil, planningValidationError("tag_name", "the selected tag or release no longer exists in the SCM repository")
		}
	}
	key := strings.TrimSpace(input.IdempotencyKey)
	if key == "" {
		key = planningReleaseFallbackKey(id, userID, input, repositoryName)
	}
	createdBy := userID
	params := ReleaseMilestoneParams{
		ID: id, IdempotencyKey: key, TagName: input.TagName, Name: input.Name, Body: input.Body,
		IsDraft: input.IsDraft, IsPrerelease: input.IsPrerelease, TargetCommitish: input.TargetCommitish,
		SCMConnectionID: connectionID, WorkspaceRepositoryID: workspaceRepositoryID,
		SCMRepository: repositoryName, CreatedBy: &createdBy,
	}
	attempt, err := s.planning.BeginMilestoneRelease(ctx, params)
	if err != nil {
		return nil, err
	}
	if attempt.AlreadyCreated {
		return s.planning.GetMilestone(id)
	}
	if provider != nil {
		release := existingRelease
		if attempt.NeedsReconcile {
			releases, err := provider.ListReleases(ctx, owner, repo)
			if err != nil {
				_ = s.planning.MarkMilestoneReleaseUncertain(ctx, attempt.ID, attempt.LeaseToken, err.Error())
				return nil, err
			}
			for i := range releases {
				if releases[i].TagName == input.TagName {
					release = &releases[i]
					break
				}
			}
		}
		if release == nil && input.Mode == "create" {
			release, err = provider.CreateRelease(ctx, owner, repo, ExternalReleaseOptions{
				TagName: input.TagName, TargetCommitish: input.TargetCommitish, Name: input.Name, Body: input.Body,
				IsDraft: input.IsDraft, IsPrerelease: input.IsPrerelease,
			})
			if err != nil {
				_ = s.planning.MarkMilestoneReleaseUncertain(ctx, attempt.ID, attempt.LeaseToken, err.Error())
				return nil, err
			}
		}
		if release.ID != "" {
			params.SCMReleaseID = &release.ID
		}
		if release.URL != "" {
			params.SCMReleaseURL = &release.URL
		}
		params.Name = release.Name
		params.Body = release.Body
		params.IsDraft = release.IsDraft
		params.IsPrerelease = release.IsPrerelease
		if release.TagURL != "" {
			params.TagURL = &release.TagURL
		}
		params.ReleaseStatus = release.Status
		params.Assets = release.Assets
		if release.ReleasedAt != nil {
			releasedAt := release.ReleasedAt.UTC().Format(time.RFC3339)
			params.ReleasedAt = &releasedAt
		}
	}
	result, err := s.planning.CompleteMilestoneRelease(ctx, attempt.ID, attempt.LeaseToken, params)
	if err != nil {
		_ = s.planning.MarkMilestoneReleaseUncertain(ctx, attempt.ID, attempt.LeaseToken, err.Error())
		return nil, err
	}
	emitServiceAudit(s.planning.db, actor, "milestone.release", "milestone", &id, result.Name, nil)
	return result, nil
}

func planningReleaseFallbackKey(id, userID int, input ReleaseMilestoneInput, repositoryName *string) string {
	repo := ""
	if repositoryName != nil {
		repo = *repositoryName
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d\n%d\n%s\n%d\n%s\n%s\n%s\n%s\n%t\n%t\n%s", id, userID, input.Mode, input.ConnectionID, repo, input.TagName, input.Name, input.Body, input.IsDraft, input.IsPrerelease, input.TargetCommitish)))
	return "auto-" + hex.EncodeToString(sum[:])
}

func (s *PlanningApplicationService) ListIterations(userID int, params IterationListParams) ([]IterationResult, int, error) {
	if err := s.requireListScope(userID, params.WorkspaceID); err != nil {
		return nil, 0, err
	}
	params.IncludeGlobal = params.WorkspaceID == nil || params.IncludeGlobal
	if params.IsGlobal {
		params.WorkspaceID = nil
		params.WorkspaceIDs = nil
	}
	if params.WorkspaceID == nil && !params.IsGlobal {
		workspaceIDs, err := s.permission.AccessibleWorkspaceIDs(userID)
		if err != nil {
			return nil, 0, err
		}
		params.WorkspaceIDs = workspaceIDs
	}
	return s.planning.ListIterations(params)
}

func (s *PlanningApplicationService) GetIteration(userID, id int) (*IterationResult, error) {
	result, err := s.planning.GetIteration(id)
	if err != nil {
		return nil, err
	}
	if err := s.requireRead(userID, result.IsGlobal, result.WorkspaceID); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *PlanningApplicationService) CreateIteration(userID int, actor AuditActor, params CreateIterationParams) (*IterationResult, error) {
	if err := s.requireWrite(userID, params.IsGlobal, params.WorkspaceID, models.PermissionIterationManage); err != nil {
		return nil, err
	}
	params.Name = sanitize.PlainTextField.Sanitize(params.Name)
	params.Description = sanitize.Comment.Sanitize(params.Description)
	params.AuditActor = &actor
	return s.planning.CreateIteration(params)
}

func (s *PlanningApplicationService) UpdateIteration(userID int, actor AuditActor, params UpdateIterationParams) (*IterationResult, error) {
	existing, err := s.planning.GetIteration(params.ID)
	if err != nil {
		return nil, err
	}
	if err := s.requireWrite(userID, existing.IsGlobal, existing.WorkspaceID, models.PermissionIterationManage); err != nil {
		return nil, err
	}
	params.WorkspaceID = existing.WorkspaceID
	params.Name = sanitize.PlainTextField.Sanitize(params.Name)
	params.Description = sanitize.Comment.Sanitize(params.Description)
	params.AuditActor = &actor
	return s.planning.UpdateIteration(params)
}

func (s *PlanningApplicationService) DeleteIteration(userID int, actor AuditActor, id int) error {
	existing, err := s.planning.GetIteration(id)
	if err != nil {
		return err
	}
	if err := s.requireWrite(userID, existing.IsGlobal, existing.WorkspaceID, models.PermissionIterationManage); err != nil {
		return err
	}
	return s.planning.DeleteIteration(id, actor)
}

func (s *PlanningApplicationService) CompleteIteration(ctx context.Context, userID, id int, target *int) (*CompleteIterationResult, error) {
	return s.completion.Complete(ctx, CompleteIterationRequest{
		IterationID: id, TargetIterationID: target, UserID: userID,
		AuthorizeWorkspace: func(workspaceID int) (bool, error) {
			return s.permission.HasWorkspacePermission(userID, workspaceID, models.PermissionItemEdit)
		},
		AuthorizeGlobal: func() (bool, error) {
			return s.permission.HasGlobalPermission(userID, models.PermissionIterationManage)
		},
	})
}

func (s *PlanningApplicationService) GetIterationProgress(userID, id int) (*IterationProgressReport, error) {
	if _, err := s.GetIteration(userID, id); err != nil {
		return nil, err
	}
	workspaceIDs, err := s.permission.AccessibleWorkspaceIDs(userID)
	if err != nil {
		return nil, err
	}
	return s.planning.GetIterationProgress(id, workspaceIDs)
}

func (s *PlanningApplicationService) GetIterationBurndown(userID, id int) (*IterationBurndownData, error) {
	if _, err := s.GetIteration(userID, id); err != nil {
		return nil, err
	}
	workspaceIDs, err := s.permission.AccessibleWorkspaceIDs(userID)
	if err != nil {
		return nil, err
	}
	return s.planning.GetIterationBurndown(id, workspaceIDs)
}

func (s *PlanningApplicationService) GetIterationProgressBatch(userID int, ids []int) (map[int]*IterationProgressReport, error) {
	workspaceIDs, err := s.permission.AccessibleWorkspaceIDs(userID)
	if err != nil {
		return nil, err
	}
	result := make(map[int]*IterationProgressReport, len(ids))
	for _, id := range ids {
		iteration, err := s.GetIteration(userID, id)
		if err != nil || iteration == nil {
			continue
		}
		report, err := s.planning.GetIterationProgress(id, workspaceIDs)
		if err == nil {
			result[id] = report
		}
	}
	return result, nil
}

func (s *PlanningApplicationService) requireListScope(userID int, workspaceID *int) error {
	if workspaceID == nil {
		return nil
	}
	return s.requireWorkspace(userID, *workspaceID, models.PermissionItemView)
}

func (s *PlanningApplicationService) requireRead(userID int, global bool, workspaceID *int) error {
	if global {
		return nil
	}
	if workspaceID == nil {
		return ErrPlanningForbidden
	}
	return s.requireWorkspace(userID, *workspaceID, models.PermissionItemView)
}

func (s *PlanningApplicationService) requireWrite(userID int, global bool, workspaceID *int, globalPermission string) error {
	if global {
		allowed, err := s.permission.HasGlobalPermission(userID, globalPermission)
		if err != nil {
			return err
		}
		if !allowed {
			return ErrPlanningForbidden
		}
		return nil
	}
	if workspaceID == nil {
		return ErrPlanningForbidden
	}
	return s.requireWorkspace(userID, *workspaceID, models.PermissionItemEdit)
}

func (s *PlanningApplicationService) requireWorkspace(userID, workspaceID int, permission string) error {
	allowed, err := s.permission.HasWorkspacePermission(userID, workspaceID, permission)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrPlanningForbidden
	}
	return nil
}
