// Package customwidgets validates user-authored OpenUI dashboard widgets.
//
// A widget is an OpenUI Lang program plus at most three query slots. The
// program is untrusted model output, so validation here is a structural gate:
// size, statement count, and a component allow-list. CQL inside the slots is
// parsed with the real CQL parser rather than a re-implementation.
package customwidgets

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"windshift/internal/cql"
)

const (
	// MaxProgramBytes bounds the stored OpenUI program.
	MaxProgramBytes = 32 * 1024
	// MaxStatements bounds top-level assignments in a program.
	MaxStatements = 200
	// MaxQueryLength mirrors the search endpoint's ql cap.
	MaxQueryLength = 500
	// MaxLimit mirrors the v2 list page cap.
	MaxLimit = 1000
	// MaxLinkProjectionFields bounds the fields projected through a link.
	MaxLinkProjectionFields = 8
	// MaxEdges bounds the bipartite edge slot.
	MaxEdges = 10000
	// MaxNameRunes bounds the display name.
	MaxNameRunes = 100
	// MaxDescriptionRunes bounds the description.
	MaxDescriptionRunes = 500
)

// DefaultLimit is used when a slot omits limit.
const DefaultLimit = 100

// componentNames is the v1 rendering library. Keep in sync with
// frontend/src/lib/openui/library.spec.js; the prompt generator reads the same
// list from the frontend spec, and a mismatch is a rendering failure.
var componentNames = map[string]struct{}{
	"ActivityFeed":  {},
	"Badge":         {},
	"BarChart":      {},
	"BipartiteList": {},
	"Card":          {},
	"Divider":       {},
	"EmptyState":    {},
	"ItemList":      {},
	"ItemsTable":    {},
	"LineChart":     {},
	"LinkButton":    {},
	"Matrix":        {},
	"Metric":        {},
	"MetricRow":     {},
	"NetworkGraph":  {},
	"PieChart":      {},
	"ProgressList":  {},
	"Section":       {},
	"Stack":         {},
	"Text":          {},
	"Title":         {},
}

var (
	statementRe = regexp.MustCompile(`(?m)^\s*[A-Za-z_][A-Za-z0-9_]*\s*=`)
	callRe      = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	sortRe      = regexp.MustCompile(`^-?[A-Za-z0-9_.,]+$`)
	fieldRe     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.]*$`)
)

// ComponentNames returns the allow-listed component names, sorted. Callers use
// it to keep the prompt and the renderer library in step with this list.
func ComponentNames() []string {
	names := make([]string, 0, len(componentNames))
	for name := range componentNames {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ValidationError is a user-facing validation failure.
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

func invalid(format string, args ...any) error {
	return &ValidationError{Message: fmt.Sprintf(format, args...)}
}

// ValidateProgram enforces the structural contract of an OpenUI Lang program.
func ValidateProgram(program string) error {
	if strings.TrimSpace(program) == "" {
		return invalid("program is required")
	}
	if len(program) > MaxProgramBytes {
		return invalid("program exceeds %d bytes", MaxProgramBytes)
	}

	statements := statementRe.FindAllStringIndex(program, -1)
	if len(statements) == 0 {
		return invalid("program has no statements")
	}
	if len(statements) > MaxStatements {
		return invalid("program has %d statements, max is %d", len(statements), MaxStatements)
	}
	if !hasRootStatement(program) {
		return invalid("program must define a root statement")
	}

	for _, name := range componentCalls(program) {
		if _, ok := componentNames[name]; !ok {
			return invalid("unknown component %q", name)
		}
	}
	return nil
}

func hasRootStatement(program string) bool {
	for _, line := range strings.Split(program, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "root") {
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "root"))
			if strings.HasPrefix(rest, "=") && !strings.HasPrefix(rest, "==") {
				return true
			}
		}
	}
	return false
}

// componentCalls returns identifiers used as call targets, ignoring string
// literals so a label containing "Card(" is not treated as a component.
func componentCalls(program string) []string {
	stripped := stripStrings(program)
	matches := callRe.FindAllStringSubmatch(stripped, -1)
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		names = append(names, match[1])
	}
	return names
}

func stripStrings(program string) string {
	var out strings.Builder
	out.Grow(len(program))
	for i := 0; i < len(program); i++ {
		switch program[i] {
		case '"', '\'':
			quote := program[i]
			i++
			for i < len(program) {
				if program[i] == '\\' {
					i++
				} else if program[i] == quote {
					break
				}
				i++
			}
			out.WriteString(`""`)
		default:
			out.WriteByte(program[i])
		}
	}
	return out.String()
}

// QuerySlot is one declarative data request. It carries no user or workspace
// IDs: scope is injected by the data endpoint from the viewer's session.
type QuerySlot struct {
	QL         string `json:"ql"`
	Sort       string `json:"sort,omitempty"`
	Limit      int    `json:"limit,omitempty"`
	AssetSetID int    `json:"asset_set_id,omitempty"`
	// LinkProjection requests fields from the linked entity for grouping.
	LinkProjection *LinkProjection `json:"link_projection,omitempty"`
}

// LinkProjection projects fields of the opposite entity through item_links.
type LinkProjection struct {
	Entity string   `json:"entity"`
	Fields []string `json:"fields"`
	Mode   string   `json:"mode,omitempty"`
}

// LinksSlot describes a bipartite edge request between an item and an asset
// seed set.
type LinksSlot struct {
	Item     *QuerySlot `json:"item"`
	Asset    *QuerySlot `json:"asset"`
	MaxEdges int        `json:"max_edges,omitempty"`
}

// Queries is the stored query specification of a widget.
type Queries struct {
	Item  *QuerySlot `json:"item,omitempty"`
	Asset *QuerySlot `json:"asset,omitempty"`
	Links *LinksSlot `json:"links,omitempty"`
}

// NormalizeQueries decodes, validates, and re-encodes a query specification so
// stored JSON has a single canonical shape.
func NormalizeQueries(raw json.RawMessage) (json.RawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return json.RawMessage(`{}`), nil
	}

	var queries Queries
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&queries); err != nil {
		return nil, invalid("queries: %v", err)
	}
	if decoder.More() {
		return nil, invalid("queries: trailing data")
	}

	if queries.Item != nil {
		if err := validateSlot("item", queries.Item, false); err != nil {
			return nil, err
		}
	}
	if queries.Asset != nil {
		if err := validateSlot("asset", queries.Asset, true); err != nil {
			return nil, err
		}
	}
	if queries.Links != nil {
		if err := validateLinks(queries.Links); err != nil {
			return nil, err
		}
	}

	normalized, err := json.Marshal(queries)
	if err != nil {
		return nil, invalid("queries: %v", err)
	}
	return normalized, nil
}

// ValidateSlot validates one query slot in place, applying limit and mode
// defaults. asset marks slots that must carry an asset set.
func ValidateSlot(name string, slot *QuerySlot, asset bool) error {
	return validateSlot(name, slot, asset)
}

func validateSlot(name string, slot *QuerySlot, asset bool) error {
	if err := validateQL(name, slot.QL); err != nil {
		return err
	}
	if slot.Limit < 0 || slot.Limit > MaxLimit {
		return invalid("%s.limit must be between 0 and %d", name, MaxLimit)
	}
	if slot.Limit == 0 {
		slot.Limit = DefaultLimit
	}
	if slot.Sort != "" && (!sortRe.MatchString(slot.Sort) || len(slot.Sort) > 64) {
		return invalid("%s.sort is invalid", name)
	}
	if asset {
		if slot.AssetSetID <= 0 {
			return invalid("%s.asset_set_id is required", name)
		}
	} else if slot.AssetSetID != 0 {
		return invalid("%s.asset_set_id is only valid for asset slots", name)
	}
	if slot.LinkProjection != nil {
		if err := validateLinkProjection(name, slot.LinkProjection); err != nil {
			return err
		}
	}
	return nil
}

func validateLinkProjection(slot string, projection *LinkProjection) error {
	switch projection.Entity {
	case "item", "asset":
	default:
		return invalid("%s.link_projection.entity must be item or asset", slot)
	}
	if len(projection.Fields) == 0 {
		return invalid("%s.link_projection.fields is required", slot)
	}
	if len(projection.Fields) > MaxLinkProjectionFields {
		return invalid("%s.link_projection.fields exceeds %d", slot, MaxLinkProjectionFields)
	}
	for _, field := range projection.Fields {
		if !fieldRe.MatchString(field) || len(field) > 100 {
			return invalid("%s.link_projection.fields contains an invalid field", slot)
		}
	}
	switch projection.Mode {
	case "", "fanout", "primary":
	default:
		return invalid("%s.link_projection.mode must be fanout or primary", slot)
	}
	if projection.Mode == "" {
		projection.Mode = "fanout"
	}
	return nil
}

func validateLinks(links *LinksSlot) error {
	if links.Item == nil || links.Asset == nil {
		return invalid("links requires both an item and an asset slot")
	}
	if err := validateSlot("links.item", links.Item, false); err != nil {
		return err
	}
	if err := validateSlot("links.asset", links.Asset, true); err != nil {
		return err
	}
	// The edge slot always resolves the full seed sets.
	links.Item.Limit = MaxLimit
	links.Asset.Limit = MaxLimit
	if links.MaxEdges < 0 || links.MaxEdges > MaxEdges {
		return invalid("links.max_edges must be between 0 and %d", MaxEdges)
	}
	if links.MaxEdges == 0 {
		links.MaxEdges = MaxEdges
	}
	return nil
}

func validateQL(slot, ql string) error {
	if len(ql) > MaxQueryLength {
		return invalid("%s.ql exceeds %d characters", slot, MaxQueryLength)
	}
	if strings.TrimSpace(ql) == "" {
		return nil
	}
	tokens, err := cql.NewTokenizer(strings.TrimSpace(ql)).Tokenize()
	if err != nil {
		return invalid("%s.ql: %v", slot, err)
	}
	if _, err := cql.NewParser(tokens).Parse(); err != nil {
		return invalid("%s.ql: %v", slot, err)
	}
	return nil
}
