package services

import (
	"context"
	"database/sql"

	"windshift/internal/database"
)

const assetGraphMaxQueries = 256

// Graph repositories share one read budget and cancellation context per request.
type assetGraphReadBudget struct {
	database.Database
	ctx       context.Context
	cancel    context.CancelFunc
	queries   int
	truncated bool
}

func (b *assetGraphReadBudget) reserve() bool {
	if b.queries >= assetGraphMaxQueries {
		b.truncated = true
		b.cancel()
	}
	if b.ctx.Err() != nil {
		return false
	}
	b.queries++
	return true
}

func (b *assetGraphReadBudget) Query(query string, args ...any) (*sql.Rows, error) {
	if !b.reserve() {
		return nil, b.ctx.Err()
	}
	return b.QueryContext(b.ctx, query, args...)
}

func (b *assetGraphReadBudget) QueryRow(query string, args ...any) *sql.Row {
	b.reserve()
	return b.QueryRowContext(b.ctx, query, args...)
}

type assetGraphCandidates struct {
	remaining int
	truncated bool
}

func (b *assetGraphCandidates) take() bool {
	if b.remaining == 0 {
		b.truncated = true
		return false
	}
	b.remaining--
	return true
}
