package repository

import (
	"database/sql"
	"errors"
	"fmt"

	"windshift/internal/database"
)

// FracIndexRepository inspects the items.frac_index column for the admin
// diagnostics panel. Separates the data-layer access (driver-aware queries
// with explicit COLLATE "C" on Postgres) from the in-memory cache stats
// that live alongside the generator in the services package.
type FracIndexRepository struct {
	db database.Database
}

// NewFracIndexRepository constructs a new repository.
func NewFracIndexRepository(db database.Database) *FracIndexRepository {
	return &FracIndexRepository{db: db}
}

// FracIndexDBStats describes the persisted frac_index state.
// CollationMismatch is the smoking gun for a column that was created without
// COLLATE "C" — ORDER BY then returns a value that is not the byte-wise max,
// so the generator hands out successors that already exist.
// PredictedCollision is non-nil when the generator's next predicted key
// already exists in the table — i.e. the next INSERT will fail.
type FracIndexDBStats struct {
	ColumnCollation    *string  `json:"column_collation"`            // NULL if default DB collation
	DefaultCollation   string   `json:"default_collation,omitempty"` // datcollate of the current DB (Postgres only)
	LinguisticMax      *string  `json:"linguistic_max"`              // ORDER BY frac_index DESC LIMIT 1
	ByteMax            *string  `json:"byte_max"`                    // ORDER BY frac_index COLLATE "C" DESC LIMIT 1
	Top10ByByte        []string `json:"top_10_by_byte"`
	NotNullCount       int64    `json:"not_null_count"`
	CollationMismatch  bool     `json:"collation_mismatch"`
	PredictedCollision *string  `json:"predicted_collision,omitempty"`
}

// GetDBStats inspects items for the diagnostics panel.
// cacheNext (the generator's predicted next key, owned by the services
// package) is verified against the DB to surface in-flight collisions.
//
// Postgres applies COLLATE "C" at query time to compute the byte-wise max
// independent of the column's stored collation. SQLite stores TEXT with
// binary comparison by default; the linguistic vs byte distinction collapses
// and CollationMismatch will always be false.
func (r *FracIndexRepository) GetDBStats(cacheNext *string) (FracIndexDBStats, error) {
	out := FracIndexDBStats{Top10ByByte: []string{}}
	driver := r.db.GetDriverName()
	isPostgres := driver == "postgres" || driver == "postgresql"

	if isPostgres {
		var collation sql.NullString
		err := r.db.QueryRow(`
			SELECT collation_name FROM information_schema.columns
			WHERE table_name = 'items' AND column_name = 'frac_index'
		`).Scan(&collation)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return out, fmt.Errorf("read column collation: %w", err)
		}
		if collation.Valid {
			c := collation.String
			out.ColumnCollation = &c
		}

		var defaultCollation sql.NullString
		if err := r.db.QueryRow(`SELECT datcollate FROM pg_database WHERE datname = current_database()`).Scan(&defaultCollation); err == nil && defaultCollation.Valid {
			out.DefaultCollation = defaultCollation.String
		}
	}

	// Linguistic max (uses column collation as-stored)
	var lingMax sql.NullString
	if err := r.db.QueryRow(`
		SELECT frac_index FROM items
		WHERE frac_index IS NOT NULL
		ORDER BY frac_index DESC
		LIMIT 1
	`).Scan(&lingMax); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return out, fmt.Errorf("read linguistic max: %w", err)
	}
	if lingMax.Valid {
		v := lingMax.String
		out.LinguisticMax = &v
	}

	// Byte-wise max — Postgres needs COLLATE "C" applied at query time;
	// SQLite is already binary so the same value falls out.
	byteQuery := `
		SELECT frac_index FROM items
		WHERE frac_index IS NOT NULL
		ORDER BY frac_index DESC
		LIMIT 1
	`
	if isPostgres {
		byteQuery = `
			SELECT frac_index FROM items
			WHERE frac_index IS NOT NULL
			ORDER BY frac_index COLLATE "C" DESC
			LIMIT 1
		`
	}
	var byteMax sql.NullString
	if err := r.db.QueryRow(byteQuery).Scan(&byteMax); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return out, fmt.Errorf("read byte max: %w", err)
	}
	if byteMax.Valid {
		v := byteMax.String
		out.ByteMax = &v
	}

	if out.LinguisticMax != nil && out.ByteMax != nil && *out.LinguisticMax != *out.ByteMax {
		out.CollationMismatch = true
	}

	// Top 10 by byte order
	top10Query := `
		SELECT frac_index FROM items
		WHERE frac_index IS NOT NULL
		ORDER BY frac_index DESC
		LIMIT 10
	`
	if isPostgres {
		top10Query = `
			SELECT frac_index FROM items
			WHERE frac_index IS NOT NULL
			ORDER BY frac_index COLLATE "C" DESC
			LIMIT 10
		`
	}
	rows, err := r.db.Query(top10Query)
	if err != nil {
		return out, fmt.Errorf("read top 10: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return out, fmt.Errorf("scan top 10: %w", err)
		}
		out.Top10ByByte = append(out.Top10ByByte, v)
	}
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("iterate top 10: %w", err)
	}

	if err := r.db.QueryRow(`SELECT COUNT(*) FROM items WHERE frac_index IS NOT NULL`).Scan(&out.NotNullCount); err != nil {
		return out, fmt.Errorf("count: %w", err)
	}

	// Verify the generator's predicted next key is not already in the table.
	// Cheap one-row probe; equality uses the column collation, which is the
	// same comparison the UNIQUE index uses — so a positive result here
	// matches what a real INSERT would hit.
	if cacheNext != nil {
		var exists string
		err := r.db.QueryRow(`SELECT frac_index FROM items WHERE frac_index = ? LIMIT 1`, *cacheNext).Scan(&exists)
		if err == nil {
			c := exists
			out.PredictedCollision = &c
		} else if !errors.Is(err, sql.ErrNoRows) {
			return out, fmt.Errorf("probe predicted collision: %w", err)
		}
	}

	return out, nil
}
