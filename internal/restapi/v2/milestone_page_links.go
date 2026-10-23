package v2

import (
	"errors"
	"net/http"

	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

// registerMilestonePageLinkRoutes publishes page links on workspace (local)
// milestones. Anyone who can view the milestone lists the linked
// pages they may view; users with edit rights on the milestone link pages
// from its workspace and unlink them. Global milestones have no page links.
func registerMilestonePageLinkRoutes(builder *routeBuilder, deps Deps) {
	const links = "/milestones/{milestone_id}/page-links"
	const link = links + "/{link_id}"
	builder.Read(links, AuthAuthenticated, []string{"milestones:read"}, listMilestonePageLinks(deps.MilestonePageLinks))
	builder.JSON(http.MethodPost, links, http.StatusCreated, false, AuthAuthenticated, []string{"milestones:write"}, createMilestonePageLink(deps.MilestonePageLinks))
	builder.Command(http.MethodDelete, link, AuthAuthenticated, []string{"milestones:write"}, deleteMilestonePageLink(deps.MilestonePageLinks))
}

type milestonePageLinkCreateRequest struct {
	PageID int `json:"page_id"`
}

func listMilestonePageLinks(links milestonePageLinkApplication) readOperation[[]models.MilestonePageLink] {
	return func(r *http.Request) ([]models.MilestonePageLink, error) {
		user, milestoneID, err := planningTarget(r, "milestone_id")
		if err != nil {
			return nil, err
		}
		rows, err := links.List(user.ID, milestoneID)
		if err != nil {
			return nil, milestonePageLinkError(err)
		}
		return rows, nil
	}
}

func createMilestonePageLink(links milestonePageLinkApplication) jsonOperation[milestonePageLinkCreateRequest, models.MilestonePageLink] {
	return func(r *http.Request, input milestonePageLinkCreateRequest) (models.MilestonePageLink, error) {
		user, milestoneID, err := planningTarget(r, "milestone_id")
		if err != nil {
			return models.MilestonePageLink{}, err
		}
		if input.PageID <= 0 {
			return models.MilestonePageLink{}, newError(http.StatusBadRequest, "invalid_request", "page_id is required")
		}
		created, err := links.Create(user.ID, milestoneID, input.PageID)
		if err != nil {
			return models.MilestonePageLink{}, milestonePageLinkError(err)
		}
		return *created, nil
	}
}

func deleteMilestonePageLink(links milestonePageLinkApplication) commandOperation {
	return func(r *http.Request) error {
		user, milestoneID, err := planningTarget(r, "milestone_id")
		if err != nil {
			return err
		}
		linkID, err := pathID(r, "link_id")
		if err != nil {
			return err
		}
		return milestonePageLinkError(links.Delete(user.ID, milestoneID, linkID))
	}
}

// milestonePageLinkError maps service errors. Unreadable milestones, missing
// edit rights, and links on other milestones read as not found
// (planningError), like the generic link routes.
func milestonePageLinkError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, services.ErrMilestonePageLinksGlobal):
		return newError(http.StatusBadRequest, "invalid_request", "Page links are only available on workspace milestones")
	case errors.Is(err, services.ErrMilestonePageNotFound):
		return newError(http.StatusNotFound, "not_found", "Page was not found in the milestone's workspace")
	case errors.Is(err, repository.ErrDuplicateEntry):
		return newError(http.StatusConflict, "conflict", "Page is already linked to this milestone")
	case errors.Is(err, repository.ErrPageLinkTypeUnavailable):
		return newError(http.StatusConflict, "conflict", "The built-in Page link type is missing or inactive")
	default:
		return planningError(err)
	}
}
