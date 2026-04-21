// Package config centralizes environment-variable and command-line flag
// resolution for every windshift entrypoint. Nothing outside this package
// should call os.Getenv or flag.* directly.
// last review: ser, 210426
package config

import (
	"os"
	"strconv"
	"time"
)

// firstNonEmpty returns the first non-empty string in the provided list.
// Used to implement env-var fallback chains (e.g. SSO_SECRET → SESSION_SECRET).
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// parseBoolEnv reads the named env var and returns true for "true" / "1" / "yes"
// (case-insensitive). Any other value — including unset — returns false.
func parseBoolEnv(name string) bool {
	switch os.Getenv(name) {
	case "true", "TRUE", "True", "1", "yes", "YES", "Yes":
		return true
	}
	return false
}

// parseIntEnv reads the named env var and returns its parsed int value, or
// fallback when the var is unset or unparseable.
func parseIntEnv(name string, fallback int) int {
	if v := os.Getenv(name); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			return parsed
		}
	}
	return fallback
}

// parseDurationEnv reads the named env var and returns its parsed duration,
// or fallback when the var is unset or unparseable.
func parseDurationEnv(name string, fallback time.Duration) time.Duration {
	if v := os.Getenv(name); v != "" {
		if parsed, err := time.ParseDuration(v); err == nil {
			return parsed
		}
	}
	return fallback
}
