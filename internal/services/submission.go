package services

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"windshift/internal/database"
	"windshift/internal/itemevents"
	"windshift/internal/repository"
)

// defaultSubmittedItemStatus is the fallback when an item type has no
// configured initial status.
const defaultSubmittedItemStatus = "open"

// SubmittedItemInput carries the inputs shared by the portal and public form
// submission paths after each has resolved its workspace and actor. The caller
// keeps its own HTTP response, visibility checks and attachment contract; only
// item creation and successful side-effect dispatch are shared.
type SubmittedItemInput struct {
	WorkspaceID       int
	Title             string
	Description       string
	ValidationResult  *RequestTypeValidationResult
	CreatorID         *int
	CreatorCustomerID *int
	ChannelID         int
	RequestTypeID     *int
	PortalOrgShared   bool
	// Source names the event metadata origin ("portal" or "form").
	Source string
	// LogComponent is the slog component for this surface ("portal" or "forms").
	LogComponent string
}

// CreateSubmittedItem determines the initial status, serializes the validated
// fields, and creates the item with the correct actor metadata. Callers handle
// the error and their own attachment timing.
func CreateSubmittedItem(db database.Database, in SubmittedItemInput) (int64, error) {
	initialStatus := defaultSubmittedItemStatus
	if in.ValidationResult.ItemTypeID != nil {
		status, err := GetInitialStatusForItemType(db, *in.ValidationResult.ItemTypeID)
		if err != nil {
			slog.Warn("could not determine initial status for item type", slog.String("component", in.LogComponent), slog.Int("item_type_id", *in.ValidationResult.ItemTypeID), slog.Any("error", err))
		} else {
			initialStatus = status
		}
	}
	customFieldsJSON, err := json.Marshal(in.ValidationResult.CustomFieldValues)
	if err != nil {
		return 0, err
	}
	virtualFieldsJSON, err := json.Marshal(in.ValidationResult.VirtualFieldValues)
	if err != nil {
		return 0, err
	}

	eventMetadata := itemevents.System(in.Source)
	if in.CreatorCustomerID != nil {
		eventMetadata = itemevents.PortalCustomer(*in.CreatorCustomerID, in.Source)
	} else if in.CreatorID != nil {
		eventMetadata = itemevents.User(*in.CreatorID, in.Source)
	}
	channelID := in.ChannelID
	return CreateItem(db, ItemCreationParams{
		WorkspaceID:             in.WorkspaceID,
		Title:                   in.Title,
		Description:             in.Description,
		Status:                  initialStatus,
		ItemTypeID:              in.ValidationResult.ItemTypeID,
		Priority:                "medium",
		CreatorID:               in.CreatorID,
		CreatorPortalCustomerID: in.CreatorCustomerID,
		ChannelID:               &channelID,
		RequestTypeID:           in.RequestTypeID,
		PortalOrgShared:         in.PortalOrgShared,
		CustomFieldValuesJSON:   string(customFieldsJSON),
		VirtualFieldDataJSON:    string(virtualFieldsJSON),
		EventMetadata:           eventMetadata,
	})
}

// DispatchSubmittedItemSideEffects emits the item-created event and touches the
// channel's last activity after a successful submission. Failures are logged,
// never returned: the item already exists and the submission already succeeded.
func DispatchSubmittedItemSideEffects(ctx context.Context, db database.Database, coordinator *EventCoordinator, itemID int64, actorID *int, channelID int, source, logComponent string) {
	if coordinator != nil {
		fullItem, fetchErr := repository.NewItemRepository(db).FindByIDWithDetailsContext(ctx, int(itemID))
		if fetchErr != nil {
			slog.Error("failed to hydrate "+source+"-created item for side effects", slog.Int64("item_id", itemID), slog.Any("error", fetchErr))
		} else {
			id := 0
			if actorID != nil {
				id = *actorID
			}
			coordinator.EmitItemCreated(fullItem, id)
		}
	}
	if _, err := db.ExecWriteContext(ctx, `UPDATE channels SET last_activity = ? WHERE id = ?`, time.Now(), channelID); err != nil {
		slog.Warn("failed to update channel last_activity", slog.String("component", logComponent), slog.Int("channel_id", channelID), slog.Any("error", err))
	}
}
