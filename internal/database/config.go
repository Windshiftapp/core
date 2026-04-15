package database

import (
	"fmt"
	"os"
)

// BuildPostgresConnString constructs a PostgreSQL connection string from
// individual POSTGRES_* environment variables, using sensible defaults.
func BuildPostgresConnString() string {
	pgHost := os.Getenv("POSTGRES_HOST")
	if pgHost == "" {
		pgHost = "postgres"
	}
	pgPort := os.Getenv("POSTGRES_PORT")
	if pgPort == "" {
		pgPort = "5432"
	}
	pgUser := os.Getenv("POSTGRES_USER")
	if pgUser == "" {
		pgUser = "windshift"
	}
	pgPassword := os.Getenv("POSTGRES_PASSWORD")
	pgDB := os.Getenv("POSTGRES_DB")
	if pgDB == "" {
		pgDB = "windshift"
	}
	return fmt.Sprintf("postgresql://%s:%s@%s:%s/%s?sslmode=disable", pgUser, pgPassword, pgHost, pgPort, pgDB)
}
