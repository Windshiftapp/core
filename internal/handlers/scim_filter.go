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
func ParseSCIMFilter(filter, resourceType string) (*SCIMFilterResult, error) {
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
		if attr == "active" {
			boolVal, err := parseSCIMBool(value)
			if err != nil {
				return nil, err
			}
			whereClause = fmt.Sprintf("%s = ?", sqlCol)
			args = []interface{}{boolVal}
		} else {
			whereClause = fmt.Sprintf("LOWER(%s) = LOWER(?)", sqlCol)
			args = []interface{}{value}
		}
	case FilterOpNe:
		if attr == "active" {
			boolVal, err := parseSCIMBool(value)
			if err != nil {
				return nil, err
			}
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

// parseSCIMBool parses a SCIM boolean filter value. Only "true" and "false"
// (case-insensitive) are accepted; any other input — including "yes", "1", or
// "banana" — must return an "invalid filter" error so the caller can map it
// to a SCIM 400 invalidFilter response. Silently treating non-boolean inputs
// as false produces a successful 200 with the wrong rows and hides client
// bugs.
func parseSCIMBool(value string) (bool, error) {
	switch strings.ToLower(value) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("invalid filter: boolean value must be 'true' or 'false', got %q", value)
	}
}

// splitTopLevelAnd splits a SCIM filter on case-insensitive " and " when the
// match occurs at parenthesis depth zero and outside any double-quoted string
// literal. The previous implementation used a plain regex which happily split
// inside quoted values, breaking filters like
//
//	userName eq "Research and Development"
//	meta.resourceType eq "Group" and displayName eq "A and B"
//
// This helper is a one-pass lexer over bytes: it tracks paren depth, whether
// we are inside a quoted string, and honors `\"` / `\\` escapes inside the
// quotes per RFC 7644 §3.4.2.2. It is intentionally narrow — splitting on
// `and` at the top level is enough to feed the existing per-term parser; we
// do not (yet) need a full SCIM grammar.
func splitTopLevelAnd(filter string) []string {
	if filter == "" {
		return nil
	}
	var parts []string
	depth := 0
	inQuote := false
	start := 0
	i := 0
	for i < len(filter) {
		c := filter[i]
		if inQuote {
			switch c {
			case '\\':
				if i+1 < len(filter) {
					i += 2
					continue
				}
			case '"':
				inQuote = false
			}
			i++
			continue
		}
		switch c {
		case '"':
			inQuote = true
			i++
			continue
		case '(':
			depth++
			i++
			continue
		case ')':
			if depth > 0 {
				depth--
			}
			i++
			continue
		}
		if depth == 0 && c == ' ' && hasFoldedAndAt(filter, i) {
			parts = append(parts, filter[start:i])
			i += len(" and ")
			start = i
			continue
		}
		i++
	}
	parts = append(parts, filter[start:])
	return parts
}

// hasFoldedAndAt reports whether s[i:] begins with the 5-byte sequence
// " and " under ASCII case-folding (the `a` and `d` characters are
// matched case-insensitively).
func hasFoldedAndAt(s string, i int) bool {
	if i+5 > len(s) {
		return false
	}
	if s[i] != ' ' || s[i+4] != ' ' {
		return false
	}
	a, n, d := s[i+1], s[i+2], s[i+3]
	return (a == 'a' || a == 'A') && (n == 'n' || n == 'N') && (d == 'd' || d == 'D')
}

// stripParens removes wrapping parentheses from a filter term.
// e.g. "(userName eq \"john\")" -> "userName eq \"john\""
func stripParens(s string) string {
	s = strings.TrimSpace(s)
	for strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		// Verify the parens are balanced (the opening paren matches the closing one)
		depth := 0
		matched := true
		for i, ch := range s {
			switch ch {
			case '(':
				depth++
			case ')':
				depth--
			}
			if depth == 0 && i < len(s)-1 {
				matched = false
				break
			}
		}
		if !matched {
			break
		}
		s = s[1 : len(s)-1]
		s = strings.TrimSpace(s)
	}
	return s
}

// ExtractResourceTypeFilter extracts a meta.resourceType filter from a SCIM filter string.
// It returns the resource type value and the remaining filter with the resourceType term removed.
// If no meta.resourceType filter is found, it returns empty string and the original filter.
func ExtractResourceTypeFilter(filter string) (resourceType, remainingFilter string) {
	if filter == "" {
		return "", ""
	}

	parts := splitTopLevelAnd(filter)

	var remaining []string
	for _, part := range parts {
		stripped := stripParens(strings.TrimSpace(part))
		// Check if this is a meta.resourceType filter
		rtPattern := regexp.MustCompile(`^meta\.resourceType\s+eq\s+(?:"([^"]*)"|(\S+))$`)
		if matches := rtPattern.FindStringSubmatch(stripped); matches != nil {
			resourceType = matches[1]
			if resourceType == "" {
				resourceType = matches[2]
			}
			continue
		}
		remaining = append(remaining, strings.TrimSpace(part))
	}

	return resourceType, strings.Join(remaining, " and ")
}

// ParseSCIMFilterWithAnd parses multiple SCIM filters joined by "and"
// Example: userName eq "john" and active eq true
func ParseSCIMFilterWithAnd(filter, resourceType string) (*SCIMFilterResult, error) {
	if filter == "" {
		return &SCIMFilterResult{WhereClause: "", Args: nil}, nil
	}

	parts := splitTopLevelAnd(filter)

	var whereClauses []string
	var allArgs []interface{}

	for _, part := range parts {
		result, err := ParseSCIMFilter(stripParens(strings.TrimSpace(part)), resourceType)
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
