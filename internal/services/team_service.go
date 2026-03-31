package services

import (
	"errors"
	"fmt"
	"sort"

	"windshift/internal/database"
	"windshift/internal/repository"
)

type TeamService struct {
	db        database.Database
	teamRepo  *repository.TeamRepository
	leaveRepo *repository.LeaveRepository
}

func NewTeamService(db database.Database, teamRepo *repository.TeamRepository, leaveRepo *repository.LeaveRepository) *TeamService {
	return &TeamService{
		db:        db,
		teamRepo:  teamRepo,
		leaveRepo: leaveRepo,
	}
}

// GetResolvedMembersForAssignment returns sorted unique user IDs eligible for assignment,
// optionally skipping users on leave and substituting them with their designated substitutes.
func (s *TeamService) GetResolvedMembersForAssignment(teamID int, skipOnLeave bool, useSubstitutes bool) ([]int, error) {
	members, err := s.teamRepo.GetResolvedMembers(teamID)
	if err != nil {
		return nil, err
	}

	if len(members) == 0 {
		return nil, fmt.Errorf("team has no active members")
	}

	// Sort members by user_id for deterministic ordering
	sort.Slice(members, func(i, j int) bool {
		return members[i].UserID < members[j].UserID
	})

	var result []int

	for _, member := range members {
		if !member.IsOnLeave {
			result = append(result, member.UserID)
			continue
		}

		// Member is on leave
		if !skipOnLeave {
			result = append(result, member.UserID)
			continue
		}

		// skipOnLeave is true — try substitution
		if useSubstitutes && member.SubstituteID != nil {
			substituteID := *member.SubstituteID
			// Check if the substitute is also on leave (max 1 level deep)
			subOnLeave, _, err := s.leaveRepo.IsUserOnLeave(substituteID)
			if err != nil {
				return nil, fmt.Errorf("failed to check substitute leave status: %w", err)
			}
			if !subOnLeave {
				result = append(result, substituteID)
			}
			// If substitute is also on leave, skip both
		}
		// If not using substitutes or no substitute set, skip the member
	}

	return uniqueSortedInts(result), nil
}

// GetNextRoundRobinAssignee determines the next user to assign to based on round-robin
// rotation among eligible team members.
func (s *TeamService) GetNextRoundRobinAssignee(actionNodeID, teamID int, skipOnLeave, useSubstitutes bool) (int, error) {
	eligible, err := s.GetResolvedMembersForAssignment(teamID, skipOnLeave, useSubstitutes)
	if err != nil {
		return 0, err
	}

	if len(eligible) == 0 {
		return 0, fmt.Errorf("no eligible team members for assignment")
	}

	state, err := s.teamRepo.GetRoundRobinState(actionNodeID, teamID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			// No prior state — assign to first member
			selectedUserID := eligible[0]
			if updateErr := s.teamRepo.UpdateRoundRobinState(actionNodeID, teamID, selectedUserID); updateErr != nil {
				return 0, fmt.Errorf("failed to update round robin state: %w", updateErr)
			}
			return selectedUserID, nil
		}
		return 0, err
	}

	var selectedUserID int

	if state.LastAssignedUserID != nil {
		lastID := *state.LastAssignedUserID
		// Find the index of the last assigned user in the eligible list
		idx := -1
		for i, id := range eligible {
			if id == lastID {
				idx = i
				break
			}
		}

		if idx >= 0 {
			// Pick the next member, wrapping around
			selectedUserID = eligible[(idx+1)%len(eligible)]
		} else {
			// Last assigned member is no longer eligible — assign to first
			selectedUserID = eligible[0]
		}
	} else {
		// No last assigned user — assign to first
		selectedUserID = eligible[0]
	}

	if updateErr := s.teamRepo.UpdateRoundRobinState(actionNodeID, teamID, selectedUserID); updateErr != nil {
		return 0, fmt.Errorf("failed to update round robin state: %w", updateErr)
	}

	return selectedUserID, nil
}

// uniqueSortedInts sorts and deduplicates an int slice.
func uniqueSortedInts(ids []int) []int {
	if len(ids) == 0 {
		return ids
	}

	sort.Ints(ids)

	result := ids[:1]
	for i := 1; i < len(ids); i++ {
		if ids[i] != ids[i-1] {
			result = append(result, ids[i])
		}
	}

	return result
}
