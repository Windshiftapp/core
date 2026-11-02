package services

import (
	"math"
	"sort"
	"strconv"

	"windshift/internal/models"
)

// MilestoneActivityStore reads the stored Activity sources of a milestone.
// repository.MilestoneActivityRepository implements it.
type MilestoneActivityStore interface {
	List(milestoneID int, workspaceIDs []int, limit int) ([]models.MilestoneActivity, error)
}

// MilestoneActivityWorkspaces lists the workspaces whose items a user may
// view. PermissionService implements it; the milestone progress view uses the
// same list.
type MilestoneActivityWorkspaces interface {
	AccessibleWorkspaceIDs(userID int) ([]int, error)
}

// MilestoneActivityPageLinks lists the recorded page links and unlinks of a
// milestone for pages a user may view. MilestonePageLinkService implements it.
type MilestoneActivityPageLinks interface {
	History(userID, milestoneID int) ([]models.MilestonePageLinkEvent, error)
}

// MilestoneActivityService builds a milestone's Activity feed: one
// shared, newest-first feed of milestone events (comments; description,
// status, and target-date changes; pages linked and unlinked) and member-item events
// (comments, status changes, items added or removed). Every viewer sees the
// same feed, except that item events are limited to items in workspaces the
// viewer can access (the progress view's rule) and page link entries to pages
// the viewer may view. Page link entries come from the milestone history,
// so an unlinked page keeps its "linked" entry and gains an
// "unlinked" one; links made before that history existed have no entry.
type MilestoneActivityService struct {
	store      MilestoneActivityStore
	access     MilestoneReadAuthorizer
	workspaces MilestoneActivityWorkspaces
	pages      MilestoneActivityPageLinks
}

// NewMilestoneActivityService creates a MilestoneActivityService. A nil page
// lister leaves page-link entries out of the feed.
func NewMilestoneActivityService(store MilestoneActivityStore, access MilestoneReadAuthorizer, workspaces MilestoneActivityWorkspaces, pages MilestoneActivityPageLinks) *MilestoneActivityService {
	return &MilestoneActivityService{store: store, access: access, workspaces: workspaces, pages: pages}
}

// MilestoneActivityListParams selects one page of the feed.
type MilestoneActivityListParams struct {
	Limit  int
	Offset int
}

const defaultMilestoneActivityLimit = 50

// List returns one page of the milestone's Activity feed, newest first, and
// whether the user can see more entries after it.
func (s *MilestoneActivityService) List(userID, milestoneID int, params MilestoneActivityListParams) ([]models.MilestoneActivity, bool, error) {
	if _, err := s.access.AuthorizeMilestoneRead(userID, milestoneID); err != nil {
		return nil, false, err
	}
	limit := params.Limit
	if limit <= 0 {
		limit = defaultMilestoneActivityLimit
	}
	offset := max(params.Offset, 0)
	// One entry past the page shows whether another page follows.
	window := offset + limit + 1
	if window <= offset {
		window = math.MaxInt32
	}

	workspaceIDs, err := s.workspaces.AccessibleWorkspaceIDs(userID)
	if err != nil {
		return nil, false, err
	}
	entries, err := s.store.List(milestoneID, workspaceIDs, window)
	if err != nil {
		return nil, false, err
	}
	if s.pages != nil {
		events, err := s.pages.History(userID, milestoneID)
		if err != nil {
			return nil, false, err
		}
		// History is oldest first; append newest first so the stable sort
		// keeps a later event ahead of an earlier one with the same time.
		for i := len(events) - 1; i >= 0; i-- {
			entries = append(entries, pageLinkActivity(events[i]))
		}
		sort.SliceStable(entries, func(a, b int) bool {
			return entries[a].OccurredAt.After(entries[b].OccurredAt)
		})
	}

	if offset >= len(entries) {
		return []models.MilestoneActivity{}, false, nil
	}
	end := min(offset+limit, len(entries))
	return entries[offset:end], len(entries) > end, nil
}

func pageLinkActivity(event models.MilestonePageLinkEvent) models.MilestoneActivity {
	activity := models.MilestoneActivity{
		ID:         "milestone_history:" + strconv.Itoa(event.ID),
		Type:       models.MilestoneActivityMilestonePageLinked,
		OccurredAt: event.OccurredAt.UTC(),
		ActorKind:  models.MilestoneActivityActorSystem,
		Page: &models.MilestoneActivityPage{
			ID:          event.PageID,
			Title:       event.PageTitle,
			WorkspaceID: event.WorkspaceID,
		},
	}
	if event.Unlinked {
		activity.Type = models.MilestoneActivityMilestonePageUnlinked
	}
	if event.UserID != nil {
		id := *event.UserID
		activity.ActorKind = models.MilestoneActivityActorUser
		activity.ActorID = &id
		activity.ActorName = event.UserName
	}
	return activity
}
