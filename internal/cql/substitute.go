package cql

import (
	"regexp"
	"strconv"
	"strings"
)

// FunctionContext carries the values used to resolve CQL context functions
// (currentUser, currentCustomer, currentOrganisation) before SQL generation.
// A nil pointer means "no value available in this context"; the matching
// function call is left unresolved and will fall through to the generator,
// where it returns a sentinel that won't match real data.
type FunctionContext struct {
	UserID         *int
	CustomerID     *int
	OrganisationID *int
}

// UserContext returns a FunctionContext containing only the authenticated user ID.
// Most non-portal call sites use this.
func UserContext(userID int) FunctionContext {
	return FunctionContext{UserID: &userID}
}

// Function names are matched case-insensitively, with optional whitespace
// inside the parens, mirroring the tokenizer's behavior.
var (
	currentUserRe         = regexp.MustCompile(`(?i)\bcurrentUser\s*\(\s*\)`)
	currentCustomerRe     = regexp.MustCompile(`(?i)\bcurrentCustomer\s*\(\s*\)`)
	currentOrganisationRe = regexp.MustCompile(`(?i)\bcurrentOrganisation\s*\(\s*\)`)
)

// SubstituteFunctions replaces context-dependent CQL function calls with
// their resolved values before tokenization. Each function is only replaced
// when the corresponding context value is set.
func SubstituteFunctions(query string, ctx FunctionContext) string {
	if strings.TrimSpace(query) == "" {
		return query
	}
	if ctx.UserID != nil {
		query = currentUserRe.ReplaceAllString(query, strconv.Itoa(*ctx.UserID))
	}
	if ctx.CustomerID != nil {
		query = currentCustomerRe.ReplaceAllString(query, strconv.Itoa(*ctx.CustomerID))
	}
	if ctx.OrganisationID != nil {
		query = currentOrganisationRe.ReplaceAllString(query, strconv.Itoa(*ctx.OrganisationID))
	}
	return query
}
