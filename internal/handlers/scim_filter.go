package handlers

import (
	"fmt"
	"regexp"
	"strings"
)

// SCIM filter operators
const (
	FilterOpEq = "eq" // equals
	FilterOpNe = "ne" // not equals
	FilterOpCo = "co" // contains
	FilterOpSw = "sw" // starts with
	FilterOpEw = "ew" // ends with
	FilterOpPr = "pr" // present (has value)
)

// likeEscaper escapes SQL LIKE special characters to prevent pattern injection.
// This ensures %, _, and \ are treated as literal characters, not wildcards.
var likeEscaper = strings.NewReplacer(
	`\`, `\\`,
	`%`, `\%`,
	`_`, `\_`,
)

func escapeLikePattern(s string) string {
	return likeEscaper.Replace(s)
}

// Supported filter attributes for Users (SCIM attr -> SQL column)
var userFilterAttrs = map[string]string{
	"userName":        "username",
	"username":        "username", // case-insensitive alias
	"email":           "email",
	"emails.value":    "email",
	"displayName":     "first_name || ' ' || last_name",
	"name.givenName":  "first_name",
	"name.familyName": "last_name",
	"externalId":      "scim_external_id",
	"active":          "is_active",
}

// Supported filter attributes for Groups (SCIM attr -> SQL column)
var groupFilterAttrs = map[string]string{
	"displayName": "name",
	"externalId":  "scim_external_id",
}

// SCIMFilterResult holds parsed filter data
type SCIMFilterResult struct {
	WhereClause string
	Args        []interface{}
}

// ParseSCIMFilter parses a SCIM filter string and returns SQL WHERE clause and args
// Supports basic filters like: userName eq "john", email co "@example.com"
func ParseSCIMFilter(filter string, resourceType string) (*SCIMFilterResult, error) {
	if filter == "" {
		return &SCIMFilterResult{WhereClause: "", Args: nil}, nil
	}

	// Select attribute mapping based on resource type
	var attrMap map[string]string
	switch resourceType {
	case "User":
		attrMap = userFilterAttrs
	case "Group":
		attrMap = groupFilterAttrs
	default:
		return nil, fmt.Errorf("unsupported resource type: %s", resourceType)
	}

	// Parse the filter expression
	// Basic pattern: attribute op "value" or attribute op value
	// Examples: userName eq "john", active eq true, email co "@example"

	// Handle "pr" (present) operator separately: attribute pr
	prPattern := regexp.MustCompile(`^(\S+)\s+pr$`)
	if matches := prPattern.FindStringSubmatch(strings.TrimSpace(filter)); matches != nil {
		attr := matches[1]
		sqlCol, ok := attrMap[attr]
		if !ok {
			return nil, fmt.Errorf("unsupported filter attribute: %s", attr)
		}
		return &SCIMFilterResult{
			WhereClause: fmt.Sprintf("%s IS NOT NULL AND %s != ''", sqlCol, sqlCol),
			Args:        nil,
		}, nil
	}

	// Pattern for comparison operators: attribute op "value" or attribute op value
	// Captures: 1=attribute, 2=operator, 3=value (with or without quotes)
	pattern := regexp.MustCompile(`^(\S+)\s+(eq|ne|co|sw|ew)\s+(?:"([^"]*)"|(\S+))$`)
	matches := pattern.FindStringSubmatch(strings.TrimSpace(filter))
	if matches == nil {
		return nil, fmt.Errorf("invalid filter syntax: %s", filter)
	}

	attr := matches[1]
	op := matches[2]
	// Value is either in group 3 (quoted) or group 4 (unquoted)
	value := matches[3]
	if value == "" {
		value = matches[4]
	}

	// Get SQL column name
	sqlCol, ok := attrMap[attr]
	if !ok {
		return nil, fmt.Errorf("unsupported filter attribute: %s", attr)
	}

	// Build WHERE clause based on operator
	var whereClause string
	var args []interface{}

	switch op {
	case FilterOpEq:
		// Handle boolean values for "active"
		if attr == "active" {
			boolVal := strings.ToLower(value) == "true"
			whereClause = fmt.Sprintf("%s = ?", sqlCol)
			args = []interface{}{boolVal}
		} else {
			whereClause = fmt.Sprintf("LOWER(%s) = LOWER(?)", sqlCol)
			args = []interface{}{value}
		}
	case FilterOpNe:
		if attr == "active" {
			boolVal := strings.ToLower(value) == "true"
			whereClause = fmt.Sprintf("%s != ?", sqlCol)
			args = []interface{}{boolVal}
		} else {
			whereClause = fmt.Sprintf("LOWER(%s) != LOWER(?)", sqlCol)
			args = []interface{}{value}
		}
	case FilterOpCo:
		// Security: Escape LIKE wildcards to prevent pattern injection
		whereClause = fmt.Sprintf("LOWER(%s) LIKE LOWER(?) ESCAPE '\\'", sqlCol)
		args = []interface{}{"%" + escapeLikePattern(value) + "%"}
	case FilterOpSw:
		// Security: Escape LIKE wildcards to prevent pattern injection
		whereClause = fmt.Sprintf("LOWER(%s) LIKE LOWER(?) ESCAPE '\\'", sqlCol)
		args = []interface{}{escapeLikePattern(value) + "%"}
	case FilterOpEw:
		// Security: Escape LIKE wildcards to prevent pattern injection
		whereClause = fmt.Sprintf("LOWER(%s) LIKE LOWER(?) ESCAPE '\\'", sqlCol)
		args = []interface{}{"%" + escapeLikePattern(value)}
	default:
		return nil, fmt.Errorf("unsupported filter operator: %s", op)
	}

	return &SCIMFilterResult{
		WhereClause: whereClause,
		Args:        args,
	}, nil
}

// ParseSCIMFilterWithAnd parses multiple SCIM filters joined by "and"
// Example: userName eq "john" and active eq true
func ParseSCIMFilterWithAnd(filter string, resourceType string) (*SCIMFilterResult, error) {
	if filter == "" {
		return &SCIMFilterResult{WhereClause: "", Args: nil}, nil
	}

	// Split by " and " (case-insensitive)
	parts := regexp.MustCompile(`(?i)\s+and\s+`).Split(filter, -1)

	var whereClauses []string
	var allArgs []interface{}

	for _, part := range parts {
		result, err := ParseSCIMFilter(strings.TrimSpace(part), resourceType)
		if err != nil {
			return nil, err
		}
		if result.WhereClause != "" {
			whereClauses = append(whereClauses, "("+result.WhereClause+")")
			allArgs = append(allArgs, result.Args...)
		}
	}

	if len(whereClauses) == 0 {
		return &SCIMFilterResult{WhereClause: "", Args: nil}, nil
	}

	return &SCIMFilterResult{
		WhereClause: strings.Join(whereClauses, " AND "),
		Args:        allArgs,
	}, nil
}
