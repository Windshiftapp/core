// Package itemevents defines and records canonical work-item domain facts.
package itemevents

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"windshift/internal/database"
	"windshift/internal/events"
	"windshift/internal/models"
)

const (
	Created        = "item.created"
	Updated        = "item.updated"
	StatusChanged  = "item.status_changed"
	Deleted        = "item.deleted"
	CommentCreated = "item.comment_created"
	Linked         = "item.linked"
	Unlinked       = "item.unlinked"

	PayloadVersion = 1
)

// Metadata preserves the principal, entry path, and causal chain of a fact.
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

// AutomationContext preserves action cascade information across restarts.
type AutomationContext struct {
	TriggeredByAction bool   `json:"triggered_by_action"`
	ExecutionChainID  string `json:"execution_chain_id,omitempty"`
	CascadeDepth      int    `json:"cascade_depth"`
	SourceApplication string `json:"source_application,omitempty"`
}

// User records an internal user as the actor.
func User(userID int, sourceKind string) Metadata {
	return Metadata{ActorKind: "user", ActorRef: strconv.Itoa(userID), SourceKind: sourceKind}
}

// PortalCustomer records an unlinked portal customer as the actor.
func PortalCustomer(customerID int, sourceKind string) Metadata {
	return Metadata{ActorKind: "portal_customer", ActorRef: strconv.Itoa(customerID), SourceKind: sourceKind}
}

// Integration records a named external system as the actor.
func Integration(ref, sourceKind string) Metadata {
	return Metadata{ActorKind: "integration", ActorRef: ref, SourceKind: sourceKind}
}

// Agent records an autonomous agent as the actor.
func Agent(ref, sourceKind string) Metadata {
	return Metadata{ActorKind: "agent", ActorRef: ref, SourceKind: sourceKind}
}

// Import records an import job as the actor.
func Import(ref string) Metadata {
	return Metadata{ActorKind: "import", ActorRef: ref, SourceKind: "import"}
}

// System records a system-owned mutation.
func System(sourceKind string) Metadata {
	return Metadata{ActorKind: "system", SourceKind: sourceKind}
}

// ItemSnapshot is the stable item state carried by version-one events.
type ItemSnapshot struct {
	ID                      int            `json:"id"`
	WorkspaceID             int            `json:"workspace_id"`
	WorkspaceItemNumber     int            `json:"workspace_item_number"`
	ItemTypeID              *int           `json:"item_type_id,omitempty"`
	Title                   string         `json:"title"`
	Description             string         `json:"description"`
	StatusID                *int           `json:"status_id,omitempty"`
	PriorityID              *int           `json:"priority_id,omitempty"`
	AssigneeID              *int           `json:"assignee_id,omitempty"`
	CreatorID               *int           `json:"creator_id,omitempty"`
	CreatorPortalCustomerID *int           `json:"creator_portal_customer_id,omitempty"`
	ParentID                *int           `json:"parent_id,omitempty"`
	IterationID             *int           `json:"iteration_id,omitempty"`
	ProjectID               *int           `json:"project_id,omitempty"`
	TimeProjectID           *int           `json:"time_project_id,omitempty"`
	DueDate                 *time.Time     `json:"due_date,omitempty"`
	StartDate               *time.Time     `json:"start_date,omitempty"`
	EndDate                 *time.Time     `json:"end_date,omitempty"`
	IsTask                  bool           `json:"is_task"`
	InheritProject          bool           `json:"inherit_project"`
	ReporterID              *int           `json:"reporter_id,omitempty"`
	ChannelID               *int           `json:"channel_id,omitempty"`
	RequestTypeID           *int           `json:"request_type_id,omitempty"`
	RelatedWorkItemID       *int           `json:"related_work_item_id,omitempty"`
	StoryPoints             *float64       `json:"story_points,omitempty"`
	EstimateMinutes         *int           `json:"estimate_minutes,omitempty"`
	FracIndex               *string        `json:"frac_index,omitempty"`
	CustomFieldValues       map[string]any `json:"custom_field_values,omitempty"`
	VirtualFieldData        map[string]any `json:"virtual_field_data,omitempty"`
}

// Snapshot copies the canonical fields consumers may need after later writes.
func Snapshot(item *models.Item) ItemSnapshot {
	return ItemSnapshot{
		ID:                      item.ID,
		WorkspaceID:             item.WorkspaceID,
		WorkspaceItemNumber:     item.WorkspaceItemNumber,
		ItemTypeID:              item.ItemTypeID,
		Title:                   item.Title,
		Description:             item.Description,
		StatusID:                item.StatusID,
		PriorityID:              item.PriorityID,
		AssigneeID:              item.AssigneeID,
		CreatorID:               item.CreatorID,
		CreatorPortalCustomerID: item.CreatorPortalCustomerID,
		ParentID:                item.ParentID,
		IterationID:             item.IterationID,
		ProjectID:               item.ProjectID,
		TimeProjectID:           item.TimeProjectID,
		DueDate:                 item.DueDate,
		StartDate:               item.StartDate,
		EndDate:                 item.EndDate,
		IsTask:                  item.IsTask,
		InheritProject:          item.InheritProject,
		ReporterID:              item.ReporterID,
		ChannelID:               item.ChannelID,
		RequestTypeID:           item.RequestTypeID,
		RelatedWorkItemID:       item.RelatedWorkItemID,
		StoryPoints:             item.StoryPoints,
		EstimateMinutes:         item.EstimateMinutes,
		FracIndex:               item.FracIndex,
		CustomFieldValues:       item.CustomFieldValues,
		VirtualFieldData:        item.VirtualFieldData,
	}
}

type CreatedV1 struct {
	Item         ItemSnapshot       `json:"item"`
	MilestoneIDs []int              `json:"milestone_ids,omitempty"`
	Automation   *AutomationContext `json:"automation,omitempty"`
}

type FieldChange struct {
	Field    string `json:"field"`
	OldValue any    `json:"old_value"`
	NewValue any    `json:"new_value"`
}

// Changes returns stable, typed before-and-after facts for canonical item
// fields. Joined display fields are intentionally excluded.
func Changes(before, after *models.Item) []FieldChange {
	changes := make([]FieldChange, 0)
	add := func(field string, oldValue, newValue any) {
		if !reflect.DeepEqual(oldValue, newValue) {
			changes = append(changes, FieldChange{Field: field, OldValue: oldValue, NewValue: newValue})
		}
	}
	add("workspace_id", before.WorkspaceID, after.WorkspaceID)
	add("workspace_item_number", before.WorkspaceItemNumber, after.WorkspaceItemNumber)
	add("item_type_id", pointerValue(before.ItemTypeID), pointerValue(after.ItemTypeID))
	add("title", before.Title, after.Title)
	add("description", before.Description, after.Description)
	add("status_id", pointerValue(before.StatusID), pointerValue(after.StatusID))
	add("priority_id", pointerValue(before.PriorityID), pointerValue(after.PriorityID))
	add("assignee_id", pointerValue(before.AssigneeID), pointerValue(after.AssigneeID))
	add("creator_id", pointerValue(before.CreatorID), pointerValue(after.CreatorID))
	add("creator_portal_customer_id", pointerValue(before.CreatorPortalCustomerID), pointerValue(after.CreatorPortalCustomerID))
	add("reporter_id", pointerValue(before.ReporterID), pointerValue(after.ReporterID))
	add("channel_id", pointerValue(before.ChannelID), pointerValue(after.ChannelID))
	add("request_type_id", pointerValue(before.RequestTypeID), pointerValue(after.RequestTypeID))
	add("parent_id", pointerValue(before.ParentID), pointerValue(after.ParentID))
	add("iteration_id", pointerValue(before.IterationID), pointerValue(after.IterationID))
	add("project_id", pointerValue(before.ProjectID), pointerValue(after.ProjectID))
	add("inherit_project", before.InheritProject, after.InheritProject)
	add("time_project_id", pointerValue(before.TimeProjectID), pointerValue(after.TimeProjectID))
	add("related_work_item_id", pointerValue(before.RelatedWorkItemID), pointerValue(after.RelatedWorkItemID))
	add("due_date", pointerValue(before.DueDate), pointerValue(after.DueDate))
	add("start_date", pointerValue(before.StartDate), pointerValue(after.StartDate))
	add("end_date", pointerValue(before.EndDate), pointerValue(after.EndDate))
	add("is_task", before.IsTask, after.IsTask)
	add("story_points", pointerValue(before.StoryPoints), pointerValue(after.StoryPoints))
	add("estimate_minutes", pointerValue(before.EstimateMinutes), pointerValue(after.EstimateMinutes))
	add("frac_index", pointerValue(before.FracIndex), pointerValue(after.FracIndex))
	addMapChanges(&changes, "cf_", before.CustomFieldValues, after.CustomFieldValues)
	addMapChanges(&changes, "virtual_", before.VirtualFieldData, after.VirtualFieldData)
	return changes
}

func pointerValue[T any](value *T) any {
	if value == nil {
		return nil
	}
	return *value
}

func addMapChanges(changes *[]FieldChange, prefix string, before, after map[string]any) {
	keys := make(map[string]struct{}, len(before)+len(after))
	for key := range before {
		keys[key] = struct{}{}
	}
	for key := range after {
		keys[key] = struct{}{}
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	for _, key := range ordered {
		oldValue, oldPresent := before[key]
		newValue, newPresent := after[key]
		if !oldPresent {
			oldValue = nil
		}
		if !newPresent {
			newValue = nil
		}
		if !reflect.DeepEqual(oldValue, newValue) {
			*changes = append(*changes, FieldChange{Field: prefix + key, OldValue: oldValue, NewValue: newValue})
		}
	}
}

type UpdatedV1 struct {
	Item       ItemSnapshot       `json:"item"`
	Changes    []FieldChange      `json:"changes"`
	Automation *AutomationContext `json:"automation,omitempty"`
}

type StatusChangedV1 struct {
	Item        ItemSnapshot       `json:"item"`
	OldStatusID *int               `json:"old_status_id,omitempty"`
	NewStatusID *int               `json:"new_status_id,omitempty"`
	Changes     []FieldChange      `json:"changes"`
	Automation  *AutomationContext `json:"automation,omitempty"`
}

type DeletedV1 struct {
	Item            ItemSnapshot       `json:"item"`
	DescendantCount int                `json:"descendant_count"`
	Automation      *AutomationContext `json:"automation,omitempty"`
}

type CommentCreatedV1 struct {
	ItemID              int                `json:"item_id"`
	CommentID           int64              `json:"comment_id"`
	AuthorID            *int               `json:"author_id,omitempty"`
	PortalCustomerID    *int               `json:"portal_customer_id,omitempty"`
	IsPrivate           bool               `json:"is_private"`
	SuppressSideEffects bool               `json:"suppress_side_effects,omitempty"`
	Automation          *AutomationContext `json:"automation,omitempty"`
}

type LinkChangedV1 struct {
	ItemID     int                `json:"item_id"`
	LinkID     int                `json:"link_id"`
	LinkTypeID int                `json:"link_type_id"`
	Direction  string             `json:"direction"`
	OtherType  string             `json:"other_type"`
	OtherID    int                `json:"other_id"`
	SourceType string             `json:"source_type"`
	SourceID   int                `json:"source_id"`
	TargetType string             `json:"target_type"`
	TargetID   int                `json:"target_id"`
	Automation *AutomationContext `json:"automation,omitempty"`
}

// UpdateRecord describes one item update in a set-based source transaction.
type UpdateRecord struct {
	Item          *models.Item
	Changes       []FieldChange
	OldStatusID   *int
	NewStatusID   *int
	StatusChanged bool
	Metadata      Metadata
}

// CreateRecord describes one item created by a set-based source transaction.
type CreateRecord struct {
	Item         *models.Item
	MilestoneIDs []int
	Metadata     Metadata
}

// Recorder appends item facts through the shared durable event store.
type Recorder struct {
	store *events.Store
}

func NewRecorder(db database.Database) *Recorder {
	return &Recorder{store: events.NewStore(db)}
}

func (r *Recorder) Created(ctx context.Context, tx database.Tx, item *models.Item, milestoneIDs []int, metadata Metadata) (*events.Event, error) {
	return r.append(ctx, tx, Created, item.WorkspaceID, item.ID, metadata, CreatedV1{
		Item: Snapshot(item), MilestoneIDs: milestoneIDs, Automation: metadata.Automation,
	})
}

// CreatedBatch appends one created fact for each distinct item.
func (r *Recorder) CreatedBatch(ctx context.Context, tx database.Tx, records []CreateRecord) ([]*events.Event, error) {
	inputs := make([]events.NewEvent, len(records))
	for i, record := range records {
		if record.Item == nil {
			return nil, fmt.Errorf("item create record %d has no item", i)
		}
		input, err := newEvent(Created, record.Item.WorkspaceID, record.Item.ID, record.Metadata, CreatedV1{
			Item: Snapshot(record.Item), MilestoneIDs: record.MilestoneIDs, Automation: record.Metadata.Automation,
		})
		if err != nil {
			return nil, err
		}
		inputs[i] = input
	}
	return r.store.AppendBatch(ctx, tx, inputs)
}

func (r *Recorder) Updated(ctx context.Context, tx database.Tx, item *models.Item, changes []FieldChange, metadata Metadata) (*events.Event, error) {
	return r.append(ctx, tx, Updated, item.WorkspaceID, item.ID, metadata, UpdatedV1{
		Item: Snapshot(item), Changes: changes, Automation: metadata.Automation,
	})
}

// UpdatedBatch appends one update fact for each distinct item in two
// set-based event-store statements.
func (r *Recorder) UpdatedBatch(ctx context.Context, tx database.Tx, records []UpdateRecord) ([]*events.Event, error) {
	inputs := make([]events.NewEvent, len(records))
	for i, record := range records {
		if record.Item == nil {
			return nil, fmt.Errorf("item update record %d has no item", i)
		}
		eventType := Updated
		payload := any(UpdatedV1{
			Item: Snapshot(record.Item), Changes: record.Changes, Automation: record.Metadata.Automation,
		})
		if record.StatusChanged {
			eventType = StatusChanged
			payload = StatusChangedV1{
				Item: Snapshot(record.Item), OldStatusID: record.OldStatusID,
				NewStatusID: record.NewStatusID, Changes: record.Changes,
				Automation: record.Metadata.Automation,
			}
		}
		input, err := newEvent(eventType, record.Item.WorkspaceID, record.Item.ID, record.Metadata, payload)
		if err != nil {
			return nil, err
		}
		inputs[i] = input
	}
	return r.store.AppendBatch(ctx, tx, inputs)
}

func (r *Recorder) StatusChanged(ctx context.Context, tx database.Tx, item *models.Item, oldStatusID, newStatusID *int, changes []FieldChange, metadata Metadata) (*events.Event, error) {
	return r.append(ctx, tx, StatusChanged, item.WorkspaceID, item.ID, metadata, StatusChangedV1{
		Item: Snapshot(item), OldStatusID: oldStatusID, NewStatusID: newStatusID, Changes: changes, Automation: metadata.Automation,
	})
}

func (r *Recorder) Deleted(ctx context.Context, tx database.Tx, item *models.Item, descendantCount int, metadata Metadata) (*events.Event, error) {
	return r.append(ctx, tx, Deleted, item.WorkspaceID, item.ID, metadata, DeletedV1{
		Item: Snapshot(item), DescendantCount: descendantCount, Automation: metadata.Automation,
	})
}

func (r *Recorder) CommentCreated(ctx context.Context, tx database.Tx, workspaceID int, payload CommentCreatedV1, metadata Metadata) (*events.Event, error) {
	payload.Automation = metadata.Automation
	return r.append(ctx, tx, CommentCreated, workspaceID, payload.ItemID, metadata, payload)
}

// LinkChanged appends one fact to every item aggregate touched by the link.
func (r *Recorder) LinkChanged(ctx context.Context, tx database.Tx, eventType string, link models.ItemLink, metadata Metadata) ([]*events.Event, error) {
	return r.linkChanged(ctx, tx, eventType, link, metadata, false)
}

func (r *Recorder) linkChanged(ctx context.Context, tx database.Tx, eventType string, link models.ItemLink, metadata Metadata, skipMissingItems bool) ([]*events.Event, error) {
	if eventType != Linked && eventType != Unlinked {
		return nil, fmt.Errorf("unsupported item link event type %q", eventType)
	}
	type endpoint struct {
		itemID, otherID      int
		direction, otherType string
	}
	var endpoints []endpoint
	if link.SourceType == "item" {
		endpoints = append(endpoints, endpoint{link.SourceID, link.TargetID, "outgoing", link.TargetType})
	}
	if link.TargetType == "item" {
		endpoints = append(endpoints, endpoint{link.TargetID, link.SourceID, "incoming", link.SourceType})
	}
	eventsOut := make([]*events.Event, 0, len(endpoints))
	for _, endpoint := range endpoints {
		var workspaceID int
		if err := tx.QueryRowContext(ctx, "SELECT workspace_id FROM items WHERE id = ?", endpoint.itemID).Scan(&workspaceID); err != nil {
			if skipMissingItems && errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return nil, fmt.Errorf("load linked item %d workspace: %w", endpoint.itemID, err)
		}
		event, err := r.append(ctx, tx, eventType, workspaceID, endpoint.itemID, metadata, LinkChangedV1{
			ItemID: endpoint.itemID, LinkID: link.ID, LinkTypeID: link.LinkTypeID,
			Direction: endpoint.direction, OtherType: endpoint.otherType, OtherID: endpoint.otherID,
			SourceType: link.SourceType, SourceID: link.SourceID,
			TargetType: link.TargetType, TargetID: link.TargetID,
			Automation: metadata.Automation,
		})
		if err != nil {
			return nil, err
		}
		eventsOut = append(eventsOut, event)
	}
	return eventsOut, nil
}

// RemovedLinks records item.unlinked facts for every link touching the
// supplied polymorphic entity IDs. Call it before deleting the links or their
// endpoints in the same transaction.
func (r *Recorder) RemovedLinks(ctx context.Context, tx database.Tx, entityType string, entityIDs []int, metadata Metadata) error {
	if len(entityIDs) == 0 {
		return nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(entityIDs)), ",")
	args := make([]any, 0, len(entityIDs)*2+2)
	args = append(args, entityType)
	for _, id := range entityIDs {
		args = append(args, id)
	}
	args = append(args, entityType)
	for _, id := range entityIDs {
		args = append(args, id)
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id, link_type_id, source_type, source_id, target_type, target_id
		FROM item_links
		WHERE (source_type = ? AND source_id IN (`+placeholders+`))
		   OR (target_type = ? AND target_id IN (`+placeholders+`))
		ORDER BY id
	`, args...)
	if err != nil {
		return fmt.Errorf("load removed %s links: %w", entityType, err)
	}
	var links []models.ItemLink
	for rows.Next() {
		var link models.ItemLink
		if err := rows.Scan(&link.ID, &link.LinkTypeID, &link.SourceType, &link.SourceID, &link.TargetType, &link.TargetID); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan removed %s link: %w", entityType, err)
		}
		links = append(links, link)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close removed %s links: %w", entityType, err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate removed %s links: %w", entityType, err)
	}
	for _, link := range links {
		if _, err := r.linkChanged(ctx, tx, Unlinked, link, metadata, true); err != nil {
			return err
		}
	}
	return nil
}

func (r *Recorder) append(ctx context.Context, tx database.Tx, eventType string, workspaceID, itemID int, metadata Metadata, payload any) (*events.Event, error) {
	input, err := newEvent(eventType, workspaceID, itemID, metadata, payload)
	if err != nil {
		return nil, err
	}
	event, err := r.store.Append(ctx, tx, input)
	if err != nil {
		return nil, fmt.Errorf("append %s for item %d: %w", eventType, itemID, err)
	}
	return event, nil
}

func newEvent(eventType string, workspaceID, itemID int, metadata Metadata, payload any) (events.NewEvent, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return events.NewEvent{}, fmt.Errorf("encode %s payload: %w", eventType, err)
	}
	if metadata.ActorKind == "" {
		metadata.ActorKind = "system"
	}
	if metadata.SourceKind == "" {
		metadata.SourceKind = "application"
	}
	workspace := workspaceID
	return events.NewEvent{
		WorkspaceID:       &workspace,
		AggregateType:     "item",
		AggregateID:       strconv.Itoa(itemID),
		Type:              eventType,
		PayloadVersion:    PayloadVersion,
		OccurredAt:        metadata.OccurredAt,
		ActorKind:         metadata.ActorKind,
		ActorRef:          metadata.ActorRef,
		SourceKind:        metadata.SourceKind,
		SourceRef:         metadata.SourceRef,
		CorrelationID:     metadata.CorrelationID,
		CausationEventKey: metadata.CausationEventKey,
		Payload:           encoded,
	}, nil
}
