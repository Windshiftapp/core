// Package customfields resolves custom-field value keys shared by the
// portal-customer and customer-organisation JSON columns.
//
// Those columns historically keyed values by the human-readable field name.
// The canonical form is the numeric field id (matching items/assets); the
// legacy name key is kept as a mirror for one downgrade window. This package
// owns the name↔id mapping so the repository, handlers, and migration logic
// agree on the resolution rules.
package customfields

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"windshift/internal/database"
)

// AppliesTo selects which custom-field applicability flag a caller cares about.
type AppliesTo string

const (
	// AppliesToPortalCustomers covers fields with applies_to_portal_customers.
	AppliesToPortalCustomers AppliesTo = "portal_customers"
	// AppliesToCustomerOrganisations covers fields with
	// applies_to_customer_organisations.
	AppliesToCustomerOrganisations AppliesTo = "customer_organisations"
)

// AmbiguousNameError reports a legacy name key that matches more than one
// applicable custom field. The caller must use the numeric field id instead.
type AmbiguousNameError struct {
	Name string
}

func (e *AmbiguousNameError) Error() string {
	return fmt.Sprintf("ambiguous custom field name %q; use the field id", e.Name)
}

// CustomFieldNameIndex resolves legacy name keys to numeric custom-field ids
// for one applicability scope. Load it once per request and reuse it across
// rows; a single row must not trigger a query per key.
type CustomFieldNameIndex struct {
	idsByName    map[string][]int
	originalName map[int]string
}

// LoadCustomFieldNameIndex builds the name index for the given applicability.
func LoadCustomFieldNameIndex(db database.Database, appliesTo AppliesTo) (*CustomFieldNameIndex, error) {
	switch appliesTo {
	case AppliesToPortalCustomers, AppliesToCustomerOrganisations:
	default:
		return nil, fmt.Errorf("unknown custom field applicability %q", appliesTo)
	}

	rows, err := db.Query(`
		SELECT id, name, applies_to_portal_customers, applies_to_customer_organisations
		FROM custom_field_definitions`)
	if err != nil {
		return nil, fmt.Errorf("load custom field names: %w", err)
	}
	defer func() { _ = rows.Close() }()

	index := &CustomFieldNameIndex{
		idsByName:    map[string][]int{},
		originalName: map[int]string{},
	}
	for rows.Next() {
		var id int
		var name string
		var portal, org sql.NullBool
		if err := rows.Scan(&id, &name, &portal, &org); err != nil {
			return nil, fmt.Errorf("scan custom field name: %w", err)
		}
		applies := (appliesTo == AppliesToPortalCustomers && portal.Valid && portal.Bool) ||
			(appliesTo == AppliesToCustomerOrganisations && org.Valid && org.Bool)
		if !applies {
			continue
		}
		lower := strings.ToLower(name)
		index.idsByName[lower] = append(index.idsByName[lower], id)
		index.originalName[id] = name
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate custom field names: %w", err)
	}
	return index, nil
}

// Name returns the original field name for an id in this scope.
func (idx *CustomFieldNameIndex) Name(id int) (string, bool) {
	name, ok := idx.originalName[id]
	return name, ok
}

// Resolve rewrites legacy name keys in cfv to numeric id keys. A name that
// matches exactly one applicable field is moved to its id key and removed from
// the map; the stored mirror is rebuilt separately on write. Names matching
// more than one field are left untouched and returned as ambiguous; unknown
// names are left untouched. The map is mutated in place.
func (idx *CustomFieldNameIndex) Resolve(cfv map[string]any) (changed bool, ambiguous []string) {
	if len(cfv) == 0 {
		return false, nil
	}
	for key, value := range cfv {
		if _, err := strconv.Atoi(key); err == nil {
			continue
		}
		ids := idx.idsByName[strings.ToLower(key)]
		switch len(ids) {
		case 0:
			// Unknown/orphan name: leave it for a later cleanup.
		case 1:
			idKey := strconv.Itoa(ids[0])
			if _, exists := cfv[idKey]; !exists {
				cfv[idKey] = value
			}
			delete(cfv, key)
			changed = true
		default:
			ambiguous = append(ambiguous, key)
		}
	}
	return changed, ambiguous
}

// Validate normalises legacy name keys for a write. Unlike Resolve it rejects
// ambiguous names (the caller must use the field id) and drops unknown names.
func (idx *CustomFieldNameIndex) Validate(cfv map[string]any) error {
	if len(cfv) == 0 {
		return nil
	}
	for key, value := range cfv {
		if _, err := strconv.Atoi(key); err == nil {
			continue
		}
		ids := idx.idsByName[strings.ToLower(key)]
		switch len(ids) {
		case 0:
			delete(cfv, key)
		case 1:
			idKey := strconv.Itoa(ids[0])
			if _, exists := cfv[idKey]; !exists {
				cfv[idKey] = value
			}
			delete(cfv, key)
		default:
			return &AmbiguousNameError{Name: key}
		}
	}
	return nil
}

// RebuildMirrors keeps the legacy name key in sync with the authoritative id
// key for fields whose name is unique in this scope. Duplicate-named fields get
// no mirror. Call after Validate and value validation/normalisation.
func (idx *CustomFieldNameIndex) RebuildMirrors(cfv map[string]any) {
	if len(cfv) == 0 {
		return
	}
	for _, ids := range idx.idsByName {
		if len(ids) != 1 {
			continue
		}
		name := idx.originalName[ids[0]]
		value, ok := cfv[strconv.Itoa(ids[0])]
		if !ok {
			// A cleared field must not leave a stale mirror.
			delete(cfv, name)
			continue
		}
		cfv[name] = value
	}
}
