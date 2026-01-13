package services

import (
	"database/sql"
	"encoding/json"
	"regexp"
	"time"
)

// validColorNames contains Tailwind CSS color names that are valid
var validColorNames = map[string]bool{
	"red": true, "orange": true, "amber": true, "yellow": true,
	"lime": true, "green": true, "emerald": true, "teal": true,
	"cyan": true, "sky": true, "blue": true, "indigo": true,
	"violet": true, "purple": true, "fuchsia": true, "pink": true,
	"rose": true, "zinc": true, "grey": true, "gray": true,
}

// hexColorPattern matches valid 6-digit hex colors
var hexColorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// ValidateColor checks if the color is either a valid hex code or a valid Tailwind color name
func ValidateColor(color string) bool {
	// Check if it's a valid color name
	if validColorNames[color] {
		return true
	}

	// Check if it's a valid hex color
	return hexColorPattern.MatchString(color)
}

// ParseTimestamp parses a timestamp string from the database
// Handles both ISO 8601 format and SQLite datetime format
func ParseTimestamp(s string) (time.Time, error) {
	// Try common formats
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return t, nil
		}
	}

	return time.Time{}, nil // Return zero time if parsing fails
}

// ParseCustomFieldValues parses a nullable JSON string into a map
func ParseCustomFieldValues(s sql.NullString) map[string]interface{} {
	if !s.Valid || s.String == "" {
		return nil
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(s.String), &result); err != nil {
		return nil
	}
	return result
}

// MarshalCustomFieldValues converts a map to JSON bytes for storage
func MarshalCustomFieldValues(values map[string]interface{}) ([]byte, error) {
	if values == nil || len(values) == 0 {
		return nil, nil
	}
	return json.Marshal(values)
}
