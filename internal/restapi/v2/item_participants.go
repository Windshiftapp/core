package v2

import (
	"errors"
	"net/http"

	"windshift/internal/logger"
	"windshift/internal/models"
	"windshift/internal/repository"
	"windshift/internal/services"
)

// registerItemParticipantRoutes exposes external request participants on a
// work item (WI-1136). Participants are portal customers, never internal
// users; writes require item edit permission and are audit-logged.
func registerItemParticipantRoutes(builder *routeBuilder, deps Deps) {
	builder.Read("/items/{item_id}/participants", AuthAuthenticated, []string{"items:read"}, listItemParticipants(deps))
	builder.JSON(http.MethodPost, "/items/{item_id}/participants", http.StatusOK, false, AuthAuthenticated, []string{"items:write"}, addItemParticipant(deps))
	builder.Command(http.MethodDelete, "/items/{item_id}/participants/{customer_id}", AuthAuthenticated, []string{"items:write"}, removeItemParticipant(deps))
}

type itemParticipantRequest struct {
	Email            string `json:"email"`
	Name             string `json:"name"`
	PortalCustomerID *int   `json:"portal_customer_id"`
}

func listItemParticipants(deps Deps) readOperation[[]models.ItemParticipant] {
	return func(r *http.Request) ([]models.ItemParticipant, error) {
		item, err := requireItem(r, deps, deps.Access.CanViewWorkspace)
		if err != nil {
			return nil, err
		}
		participants, err := deps.Participants.List(item.ID)
		if err != nil {
			return nil, internalError(err)
		}
		return participants, nil
	}
}

func addItemParticipant(deps Deps) jsonOperation[itemParticipantRequest, []models.ItemParticipant] {
	return func(r *http.Request, input itemParticipantRequest) ([]models.ItemParticipant, error) {
		item, err := requireItem(r, deps, deps.Access.CanEditWorkspace)
		if err != nil {
			return nil, err
		}
		user, err := principal(r)
		if err != nil {
			return nil, err
		}
		participants, created, err := deps.Participants.Add(r.Context(), user.ID, item.ID, services.ParticipantInput{
			Email:            input.Email,
			Name:             input.Name,
			PortalCustomerID: input.PortalCustomerID,
		})
		if err != nil {
			return nil, participantMutationError(err)
		}
		if created {
			logger.Record(r, logger.ActionItemParticipantAdd, logger.ResourceItem, &item.ID, item.Title, map[string]any{
				"email":              input.Email,
				"portal_customer_id": input.PortalCustomerID,
			})
		}
		return participants, nil
	}
}

func removeItemParticipant(deps Deps) commandOperation {
	return func(r *http.Request) error {
		item, err := requireItem(r, deps, deps.Access.CanEditWorkspace)
		if err != nil {
			return err
		}
		user, err := principal(r)
		if err != nil {
			return err
		}
		customerID, err := pathID(r, "customer_id")
		if err != nil {
			return err
		}
		if _, err := deps.Participants.Remove(r.Context(), user.ID, item.ID, customerID); err != nil {
			return participantMutationError(err)
		}
		logger.Record(r, logger.ActionItemParticipantRemove, logger.ResourceItem, &item.ID, item.Title, map[string]any{
			"portal_customer_id": customerID,
		})
		return nil
	}
}

func participantMutationError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, repository.ErrNotFound):
		return newError(http.StatusNotFound, "not_found", "Portal customer was not found")
	case errors.Is(err, services.ErrParticipantIdentityRequired), errors.Is(err, services.ErrParticipantEmailInvalid):
		return newError(http.StatusBadRequest, "invalid_request", err.Error())
	default:
		return internalError(err)
	}
}
