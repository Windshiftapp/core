// Package assetevents defines versioned asset facts and records them in the
// shared durable domain-event journal.
package assetevents

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"windshift/internal/database"
	"windshift/internal/events"
)

const (
	PayloadVersion = 1

	Created       = "asset.created.v1"
	Updated       = "asset.updated.v1"
	StatusChanged = "asset.status_changed.v1"
	Deleted       = "asset.deleted.v1"
)

// AutomationContext carries loop-prevention state across asset automations.
type AutomationContext struct {
	TriggeredByAction bool   `json:"triggered_by_action,omitempty"`
	ExecutionChainID  string `json:"execution_chain_id,omitempty"`
	CascadeDepth      int    `json:"cascade_depth,omitempty"`
	SourceApplication string `json:"source_application,omitempty"`
}

// Metadata describes who caused a fact and where it originated.
type Metadata struct {
	OccurredAt        time.Time
	ActorKind         string
	ActorRef          string
	SourceKind        string
	SourceRef         string
	CorrelationID     string
	CausationEventKey string
	Automation        *AutomationContext
}

func User(userID int, sourceKind string) Metadata {
	return Metadata{ActorKind: "user", ActorRef: strconv.Itoa(userID), SourceKind: sourceKind}
}

func System(sourceKind string) Metadata {
	return Metadata{ActorKind: "system", SourceKind: sourceKind}
}

// Import records an import job as the actor. Imported facts are historical and
// must not drive live side effects.
func Import(ref string) Metadata {
	return Metadata{ActorKind: "import", ActorRef: ref, SourceKind: "import"}
}

// AssetSnapshot is the stable asset state carried by version-one facts.
type AssetSnapshot struct {
	ID          int    `json:"id"`
	SetID       int    `json:"set_id"`
	AssetTypeID int    `json:"asset_type_id"`
	CategoryID  *int   `json:"category_id,omitempty"`
	StatusID    *int   `json:"status_id,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	AssetTag    string `json:"asset_tag,omitempty"`
}

type CreatedV1 struct {
	Asset      AssetSnapshot      `json:"asset"`
	NewValues  map[string]any     `json:"new_values,omitempty"`
	Automation *AutomationContext `json:"automation,omitempty"`
}

type UpdatedV1 struct {
	Asset      AssetSnapshot      `json:"asset"`
	OldValues  map[string]any     `json:"old_values,omitempty"`
	NewValues  map[string]any     `json:"new_values,omitempty"`
	Automation *AutomationContext `json:"automation,omitempty"`
}

type StatusChangedV1 struct {
	Asset       AssetSnapshot      `json:"asset"`
	OldStatusID *int               `json:"old_status_id,omitempty"`
	NewStatusID *int               `json:"new_status_id,omitempty"`
	OldValues   map[string]any     `json:"old_values,omitempty"`
	NewValues   map[string]any     `json:"new_values,omitempty"`
	Automation  *AutomationContext `json:"automation,omitempty"`
}

type DeletedV1 struct {
	Asset      AssetSnapshot      `json:"asset"`
	OldValues  map[string]any     `json:"old_values,omitempty"`
	Automation *AutomationContext `json:"automation,omitempty"`
}

// Recorder appends asset facts through the shared event store.
type Recorder struct {
	store *events.Store
}

func NewRecorder(db database.Database) *Recorder {
	return &Recorder{store: events.NewStore(db)}
}

func (r *Recorder) Created(ctx context.Context, tx database.Tx, asset AssetSnapshot, newValues map[string]any, metadata Metadata) (*events.Event, error) {
	return r.append(ctx, tx, Created, asset, metadata, CreatedV1{Asset: asset, NewValues: newValues, Automation: metadata.Automation})
}

func (r *Recorder) Updated(ctx context.Context, tx database.Tx, asset AssetSnapshot, oldValues, newValues map[string]any, metadata Metadata) (*events.Event, error) {
	return r.append(ctx, tx, Updated, asset, metadata, UpdatedV1{Asset: asset, OldValues: oldValues, NewValues: newValues, Automation: metadata.Automation})
}

func (r *Recorder) StatusChanged(ctx context.Context, tx database.Tx, asset AssetSnapshot, oldStatusID, newStatusID *int, oldValues, newValues map[string]any, metadata Metadata) (*events.Event, error) {
	return r.append(ctx, tx, StatusChanged, asset, metadata, StatusChangedV1{
		Asset: asset, OldStatusID: oldStatusID, NewStatusID: newStatusID,
		OldValues: oldValues, NewValues: newValues, Automation: metadata.Automation,
	})
}

func (r *Recorder) Deleted(ctx context.Context, tx database.Tx, asset AssetSnapshot, oldValues map[string]any, metadata Metadata) (*events.Event, error) {
	return r.append(ctx, tx, Deleted, asset, metadata, DeletedV1{Asset: asset, OldValues: oldValues, Automation: metadata.Automation})
}

func (r *Recorder) append(ctx context.Context, tx database.Tx, eventType string, asset AssetSnapshot, metadata Metadata, payload any) (*events.Event, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode %s payload: %w", eventType, err)
	}
	input := events.NewEvent{
		AggregateType: "asset", AggregateID: strconv.Itoa(asset.ID),
		Type: eventType, PayloadVersion: PayloadVersion, OccurredAt: metadata.OccurredAt,
		ActorKind: metadata.ActorKind, ActorRef: metadata.ActorRef,
		SourceKind: metadata.SourceKind, SourceRef: metadata.SourceRef,
		CorrelationID: metadata.CorrelationID, CausationEventKey: metadata.CausationEventKey,
		Payload: encoded,
	}
	event, err := r.store.Append(ctx, tx, input)
	if err != nil {
		return nil, fmt.Errorf("append %s for asset %d: %w", eventType, asset.ID, err)
	}
	return event, nil
}
