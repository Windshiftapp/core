package services

import (
	"context"
	"fmt"
	"time"

	"windshift/internal/models"
	"windshift/internal/repository"
)

// Agent presence values surfaced to assignment pickers (WI-272). They answer
// "if I assign this item to the agent, will anything pick it up?":
//   - online:  remote-pool binding with at least one live runner
//   - offline: remote-pool binding but no runner with a fresh heartbeat
//   - local:   binding runs on this server's in-process runtime
//   - unbound: agent user with no ready binding in the workspace; actionable
//     rosters exclude these users
const (
	AgentPresenceOnline  = "online"
	AgentPresenceOffline = "offline"
	AgentPresenceLocal   = "local"
	AgentPresenceUnbound = "unbound"
)

// AgentPresenceService derives per-agent availability from workspace
// bindings and runner-pool heartbeats. Read-only; one bindings query plus
// one live-count query per distinct pool.
type AgentPresenceService struct {
	bindings *repository.WorkspaceAgentBindingRepository
	runners  *repository.RunnerRepository
	now      func() time.Time
}

// NewAgentPresenceService constructs the service. Both repos are required.
func NewAgentPresenceService(bindings *repository.WorkspaceAgentBindingRepository, runners *repository.RunnerRepository) *AgentPresenceService {
	return &AgentPresenceService{
		bindings: bindings,
		runners:  runners,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

// ForWorkspace maps acting_user_id → presence for every ready binding in the
// workspace. Agent users absent from the map cannot currently act there.
func (s *AgentPresenceService) ForWorkspace(ctx context.Context, workspaceID int) (map[int]string, error) {
	bindings, err := s.bindings.ListForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("agent presence: %w", err)
	}
	freshSince := s.now().Add(-models.RunnerLivenessWindow)
	liveByPool := map[int]int{}
	out := make(map[int]string, len(bindings))
	for _, b := range bindings {
		if b.Lifecycle != models.AgentLifecycleReady {
			continue
		}
		if b.TargetPoolID == nil {
			out[b.ActingUserID] = AgentPresenceLocal
			continue
		}
		live, ok := liveByPool[*b.TargetPoolID]
		if !ok {
			live, err = s.runners.CountLiveInstancesForPool(ctx, *b.TargetPoolID, freshSince)
			if err != nil {
				return nil, fmt.Errorf("agent presence: count live runners for pool %d: %w", *b.TargetPoolID, err)
			}
			liveByPool[*b.TargetPoolID] = live
		}
		if live > 0 {
			out[b.ActingUserID] = AgentPresenceOnline
		} else {
			out[b.ActingUserID] = AgentPresenceOffline
		}
	}
	return out, nil
}
