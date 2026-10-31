package v2

import (
	"net/http"

	"windshift/internal/models"
	"windshift/internal/services"
)

// registerMilestoneActivityRoutes publishes a milestone's Activity feed:
// milestone comments, description/status/target-date changes,
// linked pages, and member-item comments, status changes, and membership
// changes, newest first. The same route serves global and local milestones;
// item entries are limited to items the viewer can access.
func registerMilestoneActivityRoutes(builder *routeBuilder, deps Deps) {
	builder.Read("/milestones/{milestone_id}/activity", AuthAuthenticated, []string{"milestones:read"}, listMilestoneActivity(deps.MilestoneActivity))
}

// milestoneActivityPage is one page of the feed. The feed reports whether
// another page follows instead of a total, so reading a page never counts the
// full item history.
type milestoneActivityPage struct {
	Entries []models.MilestoneActivity `json:"entries"`
	HasMore bool                       `json:"has_more"`
}

func listMilestoneActivity(activity milestoneActivityApplication) readOperation[milestoneActivityPage] {
	return func(r *http.Request) (milestoneActivityPage, error) {
		user, milestoneID, err := planningTarget(r, "milestone_id")
		if err != nil {
			return milestoneActivityPage{}, err
		}
		page, err := ParsePage(r)
		if err != nil {
			return milestoneActivityPage{}, err
		}
		entries, hasMore, err := activity.List(user.ID, milestoneID, services.MilestoneActivityListParams{
			Limit: page.PageSize, Offset: page.Offset,
		})
		if err != nil {
			return milestoneActivityPage{}, planningError(err)
		}
		return milestoneActivityPage{Entries: entries, HasMore: hasMore}, nil
	}
}
