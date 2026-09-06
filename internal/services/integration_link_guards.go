package services

import (
	"windshift/internal/database"
	"windshift/internal/repository"
)

// IntegrationLinkGuard protects provider-managed links whose lifecycle needs
// more than the generic item_integration_links cascade can provide.
type IntegrationLinkGuard interface {
	HasLinksForItemsTx(tx database.Tx, itemIDs []int) (bool, error)
	HasLinksForWorkspaceTx(tx database.Tx, workspaceID int) (bool, error)
	HasLinkUnavailableInWorkspaceTx(tx database.Tx, itemID, workspaceID int) (bool, error)
}

// IntegrationLinkGuards composes all provider lifecycle extensions. Core item
// and workspace services depend only on this interface set, not on Zammad.
type IntegrationLinkGuards struct {
	guards []IntegrationLinkGuard
}

func NewIntegrationLinkGuards(db database.Database) *IntegrationLinkGuards {
	return &IntegrationLinkGuards{guards: []IntegrationLinkGuard{
		zammadIntegrationLinkGuard{repo: repository.NewZammadRepository(db)},
	}}
}

func (g *IntegrationLinkGuards) HasLinksForItemsTx(tx database.Tx, itemIDs []int) (bool, error) {
	for _, guard := range g.guards {
		blocked, err := guard.HasLinksForItemsTx(tx, itemIDs)
		if err != nil || blocked {
			return blocked, err
		}
	}
	return false, nil
}

func (g *IntegrationLinkGuards) HasLinksForWorkspaceTx(tx database.Tx, workspaceID int) (bool, error) {
	for _, guard := range g.guards {
		blocked, err := guard.HasLinksForWorkspaceTx(tx, workspaceID)
		if err != nil || blocked {
			return blocked, err
		}
	}
	return false, nil
}

func (g *IntegrationLinkGuards) HasLinkUnavailableInWorkspaceTx(tx database.Tx, itemID, workspaceID int) (bool, error) {
	for _, guard := range g.guards {
		blocked, err := guard.HasLinkUnavailableInWorkspaceTx(tx, itemID, workspaceID)
		if err != nil || blocked {
			return blocked, err
		}
	}
	return false, nil
}

type zammadIntegrationLinkGuard struct {
	repo *repository.ZammadRepository
}

func (g zammadIntegrationLinkGuard) HasLinksForItemsTx(tx database.Tx, itemIDs []int) (bool, error) {
	return g.repo.HasTicketLinksForItemsTx(tx, itemIDs)
}

func (g zammadIntegrationLinkGuard) HasLinksForWorkspaceTx(tx database.Tx, workspaceID int) (bool, error) {
	return g.repo.HasTicketLinksForWorkspaceTx(tx, workspaceID)
}

func (g zammadIntegrationLinkGuard) HasLinkUnavailableInWorkspaceTx(tx database.Tx, itemID, workspaceID int) (bool, error) {
	return g.repo.HasTicketLinkUnavailableInWorkspaceTx(tx, itemID, workspaceID)
}
