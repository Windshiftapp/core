package repository

import (
	"database/sql"
	"errors"
	"fmt"
)

// Common repository errors
var (
	// ErrNotFound is returned when a requested entity does not exist
	ErrNotFound = errors.New("entity not found")

	// ErrInvalidInput is returned when input validation fails
	ErrInvalidInput = errors.New("invalid input")

	// ErrDuplicateEntry is returned when a unique constraint is violated
	ErrDuplicateEntry = errors.New("duplicate entry")

	// ErrConcurrentUpdate is returned when an optimistic write loses a race.
	ErrConcurrentUpdate = errors.New("concurrent update")

	// ErrDefaultChannel is returned when a delete is rejected because the
	// channel became (or remained) the default for its route.
	ErrDefaultChannel = errors.New("cannot delete default channel")

	// ErrChannelSlugConflict is returned when a concurrent portal/form write
	// claims the same normalized public slug.
	ErrChannelSlugConflict = errors.New("channel public slug is already in use")
)

// notFoundOrWrap maps missing rows to ErrNotFound and wraps other errors.
// Use only when absence is an error, not a soft nil/zero result.
func notFoundOrWrap(err error, context string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return fmt.Errorf("%s: %w", context, err)
}
