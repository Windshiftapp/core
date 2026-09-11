package services

import (
	"context"
	"errors"
	"time"

	"windshift/internal/models"
	"windshift/internal/repository"
)

// ErrAgentRunItemNotVisible hides whether an item is missing or inaccessible.
var ErrAgentRunItemNotVisible = errors.New("agent run item not visible")
var ErrAgentRunNotVisible = errors.New("agent run not visible")
var ErrAgentRunUnavailable = errors.New("agent run service unavailable")

// AgentRunReadService owns item-scoped agent-run visibility and listing.
type AgentRunReadService struct {
	items       *repository.ItemRepository
	runs        *repository.AgentRunRepository
	permissions *PermissionService
	runtime     *RunService
	bindings    *BindingService
	usage       *repository.LLMUsageRepository
}

func (s *AgentRunReadService) WithRuntime(runtime *RunService, bindings *BindingService) *AgentRunReadService {
	s.runtime, s.bindings = runtime, bindings
	return s
}

func (s *AgentRunReadService) WithUsage(usage *repository.LLMUsageRepository) *AgentRunReadService {
	s.usage = usage
	return s
}

func (s *AgentRunReadService) ListForWorkspace(ctx context.Context, userID, workspaceID, limit, beforeID int) ([]*models.AgentRun, error) {
	if err := s.requireWorkspace(userID, workspaceID, models.PermissionItemView); err != nil {
		return nil, err
	}
	return s.runs.ListForWorkspace(ctx, workspaceID, limit, beforeID)
}

func NewAgentRunReadService(items *repository.ItemRepository, runs *repository.AgentRunRepository, permissions *PermissionService) *AgentRunReadService {
	return &AgentRunReadService{items: items, runs: runs, permissions: permissions}
}

// ListForItem returns newest-first runs when the actor may view the item.
func (s *AgentRunReadService) ListForItem(ctx context.Context, userID, itemID, limit, beforeID int) ([]*models.AgentRun, error) {
	workspaceID, err := s.items.GetWorkspaceID(itemID)
	if err != nil {
		return nil, ErrAgentRunItemNotVisible
	}
	allowed, err := s.permissions.HasWorkspacePermission(userID, workspaceID, models.PermissionItemView)
	if err != nil || !allowed {
		return nil, ErrAgentRunItemNotVisible
	}
	return s.runs.ListForItem(ctx, itemID, limit, beforeID)
}

func (s *AgentRunReadService) Get(ctx context.Context, userID, runID int) (*models.AgentRun, error) {
	run, err := s.runs.Get(ctx, runID)
	if err != nil {
		return nil, ErrAgentRunNotVisible
	}
	permission := models.PermissionItemView
	if run.IsEphemeral {
		permission = models.PermissionWorkspaceAdmin
	}
	if err := s.requireWorkspace(userID, run.WorkspaceID, permission); err != nil {
		return nil, ErrAgentRunNotVisible
	}
	return run, nil
}

func (s *AgentRunReadService) Usage(ctx context.Context, userID, runID int) (repository.RunUsageTotals, error) {
	run, err := s.Get(ctx, userID, runID)
	if err != nil {
		return repository.RunUsageTotals{}, err
	}
	if s.usage == nil {
		return repository.RunUsageTotals{}, nil
	}
	return s.usage.TotalsForRun(ctx, run.ID)
}

func (s *AgentRunReadService) Events(ctx context.Context, userID, runID, afterID, limit int) ([]*models.AgentRunEvent, error) {
	run, err := s.Get(ctx, userID, runID)
	if err != nil {
		return nil, err
	}
	return s.runs.ListEventsAfter(ctx, run.ID, afterID, limit)
}

func (s *AgentRunReadService) Rerun(ctx context.Context, userID, itemID int) (bool, error) {
	workspaceID, err := s.items.GetWorkspaceID(itemID)
	if err != nil {
		return false, ErrAgentRunItemNotVisible
	}
	if err := s.requireWorkspace(userID, workspaceID, models.PermissionItemEdit); err != nil {
		return false, ErrAgentRunItemNotVisible
	}
	if s.bindings == nil {
		return false, ErrAgentRunUnavailable
	}
	return s.bindings.RerunForItem(ctx, itemID, userID)
}

type AgentRunCancelResult struct {
	Canceled    bool `json:"canceled"`
	AlreadyDone bool `json:"already_done"`
	Remote      bool `json:"remote"`
	Forced      bool `json:"forced"`
}

func (s *AgentRunReadService) Cancel(ctx context.Context, userID, runID int, force bool) (AgentRunCancelResult, error) {
	run, err := s.runs.Get(ctx, runID)
	if err != nil {
		return AgentRunCancelResult{}, ErrAgentRunNotVisible
	}
	if err := s.requireWorkspace(userID, run.WorkspaceID, models.PermissionWorkspaceAdmin); err != nil {
		return AgentRunCancelResult{}, ErrAgentRunNotVisible
	}
	now := time.Now().UTC()
	if run.Status == models.AgentRunStatusQueued {
		transitioned, err := s.runs.CancelQueued(ctx, runID, now)
		if err != nil {
			return AgentRunCancelResult{}, err
		}
		if transitioned {
			_ = s.runs.AppendEvent(ctx, runID, "lifecycle", `{"phase":"canceled","reason":"canceled while queued"}`)
			return AgentRunCancelResult{Canceled: true}, nil
		}
		run, err = s.runs.Get(ctx, runID)
		if err != nil {
			return AgentRunCancelResult{}, ErrAgentRunNotVisible
		}
	}
	if run.RunnerID != nil {
		if err := s.runs.RequestCancel(ctx, runID, now); err != nil {
			return AgentRunCancelResult{}, err
		}
		forced := false
		if force {
			forced, err = s.runs.ForceCancelRunning(ctx, runID, now)
			if err != nil {
				return AgentRunCancelResult{}, err
			}
			if forced {
				_ = s.runs.AppendEvent(ctx, runID, "lifecycle", `{"phase":"canceled","reason":"force-canceled by admin"}`)
			}
		}
		return AgentRunCancelResult{Canceled: true, Remote: true, Forced: forced}, nil
	}
	if s.runtime == nil {
		return AgentRunCancelResult{}, ErrAgentRunUnavailable
	}
	canceled := s.runtime.Cancel(runID)
	forced := false
	if force && !canceled {
		forced, err = s.runs.ForceCancelRunning(ctx, runID, now)
		if err != nil {
			return AgentRunCancelResult{}, err
		}
		if forced {
			_ = s.runs.AppendEvent(ctx, runID, "lifecycle", `{"phase":"canceled","reason":"force-canceled by admin"}`)
		}
	}
	return AgentRunCancelResult{Canceled: canceled || forced, AlreadyDone: !canceled && !forced, Forced: forced}, nil
}

func (s *AgentRunReadService) requireWorkspace(userID, workspaceID int, permission string) error {
	allowed, err := s.permissions.HasWorkspacePermission(userID, workspaceID, permission)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrAgentRunNotVisible
	}
	return nil
}
