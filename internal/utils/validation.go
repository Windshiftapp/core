// Package utils provides common utility functions for the Windshift application.
package utils

import (
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
)

// Global validator instance
var validate *validator.Validate

func init() {
	validate = validator.New()
}

// Validate validates a struct and returns a user-friendly error message
// Returns nil if validation passes, otherwise returns an error with a descriptive message
func Validate(s interface{}) error {
	err := validate.Struct(s)
	if err == nil {
		return nil
	}

	// Convert validation errors to user-friendly messages
	validationErrors, ok := err.(validator.ValidationErrors)
	if !ok {
		return fmt.Errorf("validation failed")
	}

	// Return the first validation error with a user-friendly message
	for _, fieldError := range validationErrors {
		return formatValidationError(fieldError)
	}

	return fmt.Errorf("validation failed")
}

// formatValidationError converts a validator.FieldError to a user-friendly message
func formatValidationError(fieldError validator.FieldError) error {
	field := fieldError.Field()
	tag := fieldError.Tag()
	param := fieldError.Param()

	// Convert field name to a user-friendly format (e.g., "FirstName" -> "First name")
	friendlyField := toFriendlyFieldName(field)

	switch tag {
	case "required":
		return fmt.Errorf("%s is required", friendlyField)
	case "email":
		return fmt.Errorf("%s must be a valid email address", friendlyField)
	case "min":
		return fmt.Errorf("%s must be at least %s characters", friendlyField, param)
	case "max":
		return fmt.Errorf("%s must not exceed %s characters", friendlyField, param)
	case "alphanum":
		return fmt.Errorf("%s must contain only alphanumeric characters", friendlyField)
	case "oneof":
		return fmt.Errorf("%s must be one of: %s", friendlyField, param)
	case "len":
		return fmt.Errorf("%s must be exactly %s characters", friendlyField, param)
	case "gt":
		return fmt.Errorf("%s must be greater than %s", friendlyField, param)
	case "gte":
		return fmt.Errorf("%s must be greater than or equal to %s", friendlyField, param)
	case "lt":
		return fmt.Errorf("%s must be less than %s", friendlyField, param)
	case "lte":
		return fmt.Errorf("%s must be less than or equal to %s", friendlyField, param)
	default:
		return fmt.Errorf("%s failed validation (%s)", friendlyField, tag)
	}
}

// toFriendlyFieldName converts a field name like "FirstName" to "First name"
func toFriendlyFieldName(field string) string {
	// Convert camelCase/PascalCase to space-separated words
	var result strings.Builder
	for i, r := range field {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteRune(' ')
		}
		if i == 0 {
			result.WriteRune(r)
		} else {
			result.WriteRune(rune(strings.ToLower(string(r))[0]))
		}
	}
	return result.String()
}
