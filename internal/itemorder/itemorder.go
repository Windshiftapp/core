// Package itemorder defines the persisted, ordered item sort contract shared by
// the Jira importer, the collection API, and the item-list repository. The
// field identifiers are stable storage and API values; they are deliberately
// not SQL expressions, so a stored default cannot execute arbitrary SQL.
package itemorder

import (
	"fmt"
	"strings"
)

// Direction is the semantic sort direction for one key.
type Direction string

const (
	Asc  Direction = "asc"
	Desc Direction = "desc"
)

// Term is one ordered sort key.
type Term struct {
	Field     string    `json:"field"`
	Direction Direction `json:"direction"`
}

// Sort is an ordered list of sort keys. Nil or empty means the caller has no
// saved default and the native order stays in effect.
type Sort []Term

// Supported target fields. Keep these identifiers stable: they are persisted
// in collections.default_sort and returned by the v2 collection contract.
const (
	FieldPriority  = "priority"
	FieldCreatedAt = "created_at"
	FieldUpdatedAt = "updated_at"
	FieldRank      = "rank"
)

// Supported reports whether field is a comparable target field.
func Supported(field string) bool {
	switch strings.TrimSpace(field) {
	case FieldPriority, FieldCreatedAt, FieldUpdatedAt, FieldRank:
		return true
	default:
		return false
	}
}

// Validate rejects a vector that names an unknown field or direction.
func (s Sort) Validate() error {
	for i, term := range s {
		if !Supported(term.Field) {
			return fmt.Errorf("unsupported sort field %q at position %d", term.Field, i)
		}
		if term.Direction != Asc && term.Direction != Desc {
			return fmt.Errorf("invalid sort direction %q for field %q", term.Direction, term.Field)
		}
	}
	return nil
}
