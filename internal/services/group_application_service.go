package services

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/sanitize"
)

var (
	ErrGroupNameRequired = errors.New("group name is required")
	ErrGroupDuplicate    = errors.New("group name already exists")
	ErrGroupManaged      = errors.New("managed group cannot be changed")
	ErrGroupSystem       = errors.New("system group cannot be deleted")
	ErrGroupNoFields     = errors.New("no group fields to update")
)

type GroupUpdateInput struct {
	Name        *string
	Description *string
	IsActive    *bool
	MemberIDs   *[]int
	ActorID     *int
}

type GroupMutationResult struct {
	Group    *models.TeamGroup
	Previous *models.TeamGroup
	Warnings []string
}

// GroupApplicationService owns group validation, persistence, and permission-cache invalidation.
type GroupApplicationService struct {
	repo        *repository.GroupRepository
	invalidator *AuthorizationCacheInvalidator
}

func NewGroupApplicationService(repo *repository.GroupRepository, invalidator *AuthorizationCacheInvalidator) *GroupApplicationService {
	return &GroupApplicationService{repo: repo, invalidator: invalidator}
}

func (s *GroupApplicationService) List() ([]models.TeamGroup, error) {
	groups, err := s.repo.ListAll()
	if groups == nil {
		groups = []models.TeamGroup{}
	}
	return groups, err
}

func (s *GroupApplicationService) ListPage(limit, offset int) ([]models.TeamGroup, int, error) {
	total, err := s.repo.Count()
	if err != nil {
		return nil, 0, err
	}
	groups, err := s.repo.ListPage(limit, offset)
	return groups, total, err
}

func (s *GroupApplicationService) Get(id int, withMembers bool) (*models.TeamGroup, error) {
	group, err := s.repo.GetByID(id)
	if err != nil || !withMembers {
		return group, err
	}
	members, err := s.repo.ListMembers(id)
	if err != nil {
		return nil, err
	}
	group.Members = members
	group.MemberCount = len(members)
	return group, nil
}

func (s *GroupApplicationService) Create(name, description string, actorID int) (*GroupMutationResult, error) {
	warnings := sanitize.ApplyAllWithWarnings(
		sanitize.Pair{Target: &name, Policy: sanitize.PlainTextField, Label: "Group name"},
		sanitize.Pair{Target: &description, Policy: sanitize.RichText, Label: "Description"},
	)
	if strings.TrimSpace(name) == "" {
		return nil, ErrGroupNameRequired
	}
	exists, err := s.repo.NameExists(name, 0)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrGroupDuplicate
	}
	now := time.Now()
	id, err := s.repo.Create(name, description, &actorID, now)
	if errors.Is(err, repository.ErrDuplicateEntry) {
		return nil, ErrGroupDuplicate
	}
	if err != nil {
		return nil, err
	}
	return &GroupMutationResult{Group: &models.TeamGroup{
		ID: int(id), Name: name, Description: description, IsActive: true,
		CreatedBy: &actorID, CreatedAt: now, UpdatedAt: now,
		Members: []models.TeamGroupMember{},
	}, Warnings: warnings}, nil
}

func (s *GroupApplicationService) Update(id int, input GroupUpdateInput) (*GroupMutationResult, error) {
	if input.Name == nil && input.Description == nil && input.IsActive == nil && input.MemberIDs == nil {
		return nil, ErrGroupNoFields
	}
	old, err := s.repo.GetUpdateSnapshot(id)
	if err != nil {
		return nil, err
	}
	if old.SCIMManaged {
		return nil, ErrGroupManaged
	}
	previousMembers, err := s.repo.ListMembers(id)
	if err != nil {
		return nil, err
	}
	name, description, active := old.Name, old.Description, old.IsActive
	if input.Name != nil {
		name = *input.Name
	}
	if input.Description != nil {
		description = *input.Description
	}
	if input.IsActive != nil {
		active = *input.IsActive
	}
	warnings := sanitize.ApplyAllWithWarnings(
		sanitize.Pair{Target: &name, Policy: sanitize.PlainTextField, Label: "Group name"},
		sanitize.Pair{Target: &description, Policy: sanitize.RichText, Label: "Description"},
	)
	if strings.TrimSpace(name) == "" {
		return nil, ErrGroupNameRequired
	}
	exists, err := s.repo.NameExists(name, id)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrGroupDuplicate
	}
	var invalidation AuthorizationInvalidation
	if old.IsActive != active {
		invalidation, err = s.invalidator.GroupPlan(id)
		if err != nil {
			return nil, err
		}
	}
	if err := s.repo.Update(id, name, description, active, time.Now()); errors.Is(err, repository.ErrDuplicateEntry) {
		return nil, ErrGroupDuplicate
	} else if err != nil {
		return nil, err
	}
	if old.IsActive != active {
		if err := s.invalidator.Apply(invalidation); err != nil {
			return nil, err
		}
	}
	if input.MemberIDs != nil {
		memberIDs := uniqueGroupMemberIDs(*input.MemberIDs)
		for _, memberID := range memberIDs {
			exists, err := s.repo.UserExists(memberID)
			if err != nil {
				return nil, err
			}
			if !exists {
				return nil, fmt.Errorf("%w: user %d", repository.ErrNotFound, memberID)
			}
		}
		for _, member := range previousMembers {
			if member.LDAPSyncEnabled && !slices.Contains(memberIDs, member.UserID) {
				return nil, ErrGroupManaged
			}
		}
		invalidation, err := s.invalidator.GroupPlan(id)
		if err != nil {
			return nil, err
		}
		if err := s.repo.ReplaceManualMembers(id, memberIDs, input.ActorID, time.Now()); err != nil {
			return nil, err
		}
		if err := s.invalidator.Apply(invalidation); err != nil {
			return nil, err
		}
	}
	updated, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	return &GroupMutationResult{Group: updated, Previous: old, Warnings: warnings}, nil
}

func uniqueGroupMemberIDs(values []int) []int {
	seen := make(map[int]struct{}, len(values))
	result := make([]int, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	slices.Sort(result)
	return result
}

func (s *GroupApplicationService) Delete(id int) (*repository.GroupDeleteSnapshot, error) {
	snapshot, err := s.repo.GetDeleteSnapshot(id)
	if err != nil {
		return nil, err
	}
	if snapshot.IsSystemGroup {
		return nil, ErrGroupSystem
	}
	if snapshot.SCIMManaged {
		return nil, ErrGroupManaged
	}
	invalidation, err := s.invalidator.GroupDeletePlan(id)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Delete(id); err != nil {
		return nil, err
	}
	if err := s.invalidator.Apply(invalidation); err != nil {
		return nil, err
	}
	return snapshot, nil
}
