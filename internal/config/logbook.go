package config

import (
	"os"

	"windshift/internal/database"
)

// LoadLogbookSidecar resolves configuration for the standalone logbook sidecar
// (cmd/logbook/main.go). No CLI flags are parsed — the sidecar is env-only.
// last review: ser, 210426
func LoadLogbookSidecar() LogbookSidecarConfig {
	// Database: LOGBOOK_DATABASE_URL preferred, fall back to POSTGRES_CONNECTION_STRING,
	// then to individual POSTGRES_* env vars.
	pgConn := firstNonEmpty(
		os.Getenv("LOGBOOK_DATABASE_URL"),
		os.Getenv("POSTGRES_CONNECTION_STRING"),
	)
	if pgConn == "" {
		pgConn = database.BuildPostgresConnString(postgresEnv())
	}

	return LogbookSidecarConfig{
		PostgresConn:     pgConn,
		Port:             firstNonEmpty(os.Getenv("LOGBOOK_PORT"), "8090"),
		StoragePath:      firstNonEmpty(os.Getenv("LOGBOOK_STORAGE_PATH"), "/data/logbook"),
		LLMEndpoint:      os.Getenv("LLM_ENDPOINT"),
		ArticleEndpoint:  os.Getenv("LOGBOOK_ARTICLE_ENDPOINT"),
		MainServerURL:    os.Getenv("WINDSHIFT_URL"),
		MainServerSecret: os.Getenv("SSO_SECRET"),
		BaseURL:          os.Getenv("BASE_URL"),
		Logging: LoggingConfig{
			Level:  firstNonEmpty(os.Getenv("LOG_LEVEL"), "info"),
			Format: firstNonEmpty(os.Getenv("LOG_FORMAT"), "text"),
		},
	}
}
