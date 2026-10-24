// Package reporting resolves the declarative query slots of a custom widget
// into rows.
//
// It sits between the widget API and the existing item and asset list
// services, so every read goes through the same permission checks as the
// regular list endpoints. A slot carries no workspace, user, or asset-set ID
// chosen by the model: item scope is injected here, and asset sets are
// verified by the asset service.
package reporting

import (
	"fmt"
	"strings"
	"time"

	"windshift/internal/customwidgets"
)

// Entity is the kind of record a slot queries.
type Entity string

const (
	EntityItem  Entity = "item"
	EntityAsset Entity = "asset"
	// EntityLinks is the bipartite edge slot: an item seed set, an asset seed
	// set, and the links between them.
	EntityLinks Entity = "links"
)

const (
	// MaxSlots bounds the number of queries one dashboard may resolve.
	MaxSlots = 20
	// MaxLimit mirrors the v2 list page cap.
	MaxLimit = 1000
	// DefaultLimit is used when a slot omits its limit.
	DefaultLimit = customwidgets.DefaultLimit
	// MaxQueryLength mirrors the search endpoint's ql cap.
	MaxQueryLength = customwidgets.MaxQueryLength
	// MaxKeyLength bounds the opaque client-supplied result key.
	MaxKeyLength = 200
	// SlotTimeout bounds one query.
	SlotTimeout = 5 * time.Second
)

// Slot is one declarative query. Key is an opaque client-supplied identifier
// echoed back in the response so the client can match results to components.
type Slot struct {
	Key        string
	Entity     Entity
	QL         string
	Sort       string
	Limit      int
	AssetSetID int
	Projection *Projection
	// Links is set when Entity is EntityLinks.
	Links *LinksSpec
}

// QueryPart is one seed set of an edge slot.
type QueryPart struct {
	QL         string
	Sort       string
	Limit      int
	AssetSetID int
}

// LinksSpec is the bipartite edge request.
type LinksSpec struct {
	Item     *QueryPart
	Asset    *QueryPart
	MaxEdges int
}

// Projection requests fields from the entity linked to a row.
type Projection struct {
	Entity string
	Fields []string
	Mode   string
}

// ValidationError describes a rejected slot.
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

func invalid(format string, args ...any) error {
	return &ValidationError{Message: fmt.Sprintf(format, args...)}
}

// SourceError is returned to the client in place of rows.
type SourceError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Result is the resolved outcome for one slot. Row slots fill Rows/Total;
// edge slots fill Graph instead.
type Result struct {
	Status    string       `json:"status"`
	Rows      any          `json:"rows,omitempty"`
	Total     int          `json:"total,omitempty"`
	Graph     *GraphResult `json:"graph,omitempty"`
	Truncated bool         `json:"truncated,omitempty"`
	Error     *SourceError `json:"error,omitempty"`
}

// ItemRow is the normalized shape a widget component sees for an item. It
// deliberately excludes descriptions, comments, and attachments.
type ItemRow struct {
	ID        int    `json:"id"`
	Key       string `json:"key,omitempty"`
	Title     string `json:"title"`
	Status    string `json:"status,omitempty"`
	Type      string `json:"type,omitempty"`
	Priority  string `json:"priority,omitempty"`
	Assignee  string `json:"assignee,omitempty"`
	Workspace string `json:"workspace,omitempty"`
	// WorkspaceID lets the renderer build a deep link to the item.
	WorkspaceID int        `json:"workspace_id,omitempty"`
	CreatedAt   *time.Time `json:"created_at,omitempty"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
	DueDate     *time.Time `json:"due_date,omitempty"`
	Completed   *time.Time `json:"completed_at,omitempty"`
	StoryPoint  *float64   `json:"story_points,omitempty"`
}

// AssetRow is the normalized shape for an asset. LinkedItems is present only
// when the slot requested a projection.
type AssetRow struct {
	ID          int              `json:"id"`
	Key         string           `json:"key,omitempty"`
	Title       string           `json:"title"`
	Status      string           `json:"status,omitempty"`
	Type        string           `json:"type,omitempty"`
	Category    string           `json:"category,omitempty"`
	Set         string           `json:"set,omitempty"`
	CreatedAt   *time.Time       `json:"created_at,omitempty"`
	UpdatedAt   *time.Time       `json:"updated_at,omitempty"`
	LinkedItems []map[string]any `json:"linked_items,omitempty"`
}

// NormalizeSlot validates a slot and applies defaults.
func NormalizeSlot(slot Slot) (Slot, error) {
	if strings.TrimSpace(slot.Key) == "" {
		return slot, invalid("slot key is required")
	}
	if len(slot.Key) > MaxKeyLength {
		return slot, invalid("slot key exceeds %d characters", MaxKeyLength)
	}

	querySlot := customwidgets.QuerySlot{QL: slot.QL, Sort: slot.Sort, Limit: slot.Limit, AssetSetID: slot.AssetSetID}
	if slot.Projection != nil {
		querySlot.LinkProjection = &customwidgets.LinkProjection{
			Entity: slot.Projection.Entity,
			Fields: slot.Projection.Fields,
			Mode:   slot.Projection.Mode,
		}
	}

	switch slot.Entity {
	case EntityItem:
		if err := customwidgets.ValidateSlot("item", &querySlot, false); err != nil {
			return slot, err
		}
	case EntityAsset:
		if err := customwidgets.ValidateSlot("asset", &querySlot, true); err != nil {
			return slot, err
		}
	case EntityLinks:
		if _, err := normalizeLinks(slot.Links); err != nil {
			return slot, err
		}
		return slot, nil
	default:
		return slot, invalid("slot entity must be item, asset, or links")
	}

	slot.QL = querySlot.QL
	slot.Sort = querySlot.Sort
	slot.Limit = querySlot.Limit
	slot.AssetSetID = querySlot.AssetSetID
	if querySlot.LinkProjection != nil {
		slot.Projection = &Projection{
			Entity: querySlot.LinkProjection.Entity,
			Fields: querySlot.LinkProjection.Fields,
			Mode:   querySlot.LinkProjection.Mode,
		}
	}
	return slot, nil
}

// normalizeLinks validates an edge slot and applies its defaults.
func normalizeLinks(links *LinksSpec) (*LinksSpec, error) {
	if links == nil || links.Item == nil || links.Asset == nil {
		return nil, invalid("links requires both an item and an asset seed")
	}
	if err := validateSeed("links.item", links.Item, false); err != nil {
		return nil, err
	}
	if err := validateSeed("links.asset", links.Asset, true); err != nil {
		return nil, err
	}
	if links.MaxEdges < 0 || links.MaxEdges > MaxEdges {
		return nil, invalid("links.max_edges must be between 0 and %d", MaxEdges)
	}
	if links.MaxEdges == 0 {
		links.MaxEdges = MaxEdges
	}
	return links, nil
}

func validateSeed(name string, part *QueryPart, asset bool) error {
	// Seed limits default to and are capped by the node cap, not by the row cap
	// the other slots use.
	if part.Limit <= 0 || part.Limit > MaxNodesPerSide {
		part.Limit = MaxNodesPerSide
	}
	slot := customwidgets.QuerySlot{QL: part.QL, Sort: part.Sort, Limit: part.Limit}
	if asset {
		slot.AssetSetID = part.AssetSetID
	}
	if err := customwidgets.ValidateSlot(name, &slot, asset); err != nil {
		return err
	}
	part.QL, part.Sort, part.Limit = slot.QL, slot.Sort, slot.Limit
	return nil
}

// SplitSort turns a "-field" sort into its field and direction.
func SplitSort(sort string) (field string, ascending bool) {
	sort = strings.TrimSpace(sort)
	if sort == "" {
		return "", false
	}
	if strings.HasPrefix(sort, "-") {
		return strings.TrimPrefix(sort, "-"), false
	}
	return sort, true
}
