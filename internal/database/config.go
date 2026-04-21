package database

import "fmt"

// PostgresEnv holds the individual POSTGRES_* parameters that compose a
// connection string. Callers (internal/config) resolve defaults from env
// before invoking BuildPostgresConnString.
// last review: ser, 210426
type PostgresEnv struct {
	Host     string
	Port     string
	User     string
	Password string
	Database string
}

// BuildPostgresConnString constructs a PostgreSQL connection string from
// already-resolved parameters.
func BuildPostgresConnString(p PostgresEnv) string {
	return fmt.Sprintf(
		"postgresql://%s:%s@%s:%s/%s?sslmode=disable",
		p.User, p.Password, p.Host, p.Port, p.Database,
	)
}
