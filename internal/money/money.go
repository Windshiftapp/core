// Package money centralizes ISO 4217 currency handling shared by the instance
// setting, time projects, and monetary custom-field values.
package money

import (
	"errors"
	"strings"

	"golang.org/x/text/currency"
)

// Default is the ISO 4217 code used when no currency is configured or a stored
// code is no longer recognized.
const Default = "USD"

// ErrInvalidCurrency reports a code that is not a recognized ISO 4217 currency.
var ErrInvalidCurrency = errors.New("invalid ISO 4217 currency code")

// Normalize trims and uppercases an ISO 4217 code, returning an error when it is
// not recognized. An empty string is invalid; callers decide what empty means
// in their context.
func Normalize(code string) (string, error) {
	unit, err := currency.ParseISO(strings.TrimSpace(code))
	if err != nil {
		return "", ErrInvalidCurrency
	}
	return unit.String(), nil
}

// IsValid reports whether code is a recognized ISO 4217 currency.
func IsValid(code string) bool {
	_, err := Normalize(code)
	return err == nil
}
