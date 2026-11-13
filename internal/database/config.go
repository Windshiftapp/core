package database

import (
	"net"
	"net/url"
)

// DefaultPostgresSSLMode is the sslmode used when the caller leaves
// PostgresEnv.SSLMode empty. "disable" preserves the behavior the
// split-variable configuration path has always had; operators reaching a
// remote or managed PostgreSQL should set POSTGRES_SSLMODE=require or
// stricter. The connection-string path (POSTGRES_CONNECTION_STRING) carries
// its own sslmode in the URL and never passes through here.
const DefaultPostgresSSLMode = "disable"

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
	// SSLMode is the lib/pq sslmode. Empty means DefaultPostgresSSLMode;
	// internal/config validates the value before it gets here.
	SSLMode string
}

// BuildPostgresConnString constructs a PostgreSQL connection string from
// already-resolved parameters.
//
// Built via net/url rather than fmt.Sprintf so credentials containing URL
// metacharacters (@ : / ? #) are percent-encoded instead of corrupting the
// DSN, and so IPv6 hosts get their brackets.
func BuildPostgresConnString(p PostgresEnv) string {
	sslMode := p.SSLMode
	if sslMode == "" {
		sslMode = DefaultPostgresSSLMode
	}

	u := &url.URL{
		Scheme:   "postgresql",
		User:     url.UserPassword(p.User, p.Password),
		Host:     net.JoinHostPort(p.Host, p.Port),
		Path:     "/" + p.Database,
		RawQuery: url.Values{"sslmode": {sslMode}}.Encode(),
	}
	return u.String()
}
