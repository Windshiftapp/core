package repository

import "errors"

// Common repository errors
var (
	// ErrNotFound is returned when a requested entity does not exist
	ErrNotFound = errors.New("entity not found")

	// ErrInvalidInput is returned when input validation fails
	ErrInvalidInput = errors.New("invalid input")

	// ErrDuplicateEntry is returned when a unique constraint is violated
	ErrDuplicateEntry = errors.New("duplicate entry")
)
