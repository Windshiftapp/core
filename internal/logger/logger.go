package logger

import (
	"log/slog"
	"os"
	"strings"

	charmlog "github.com/charmbracelet/log"
)

var defaultLogger *slog.Logger

// Init initializes the global logger with the specified level and format
func Init(levelStr, format string) {
	// Create charmbracelet log handler
	handler := charmlog.NewWithOptions(os.Stderr, charmlog.Options{
		ReportTimestamp: true,
		ReportCaller:    false,
	})

	// Parse and set log level
	var level charmlog.Level
	switch strings.ToLower(levelStr) {
	case "debug":
		level = charmlog.DebugLevel
	case "info":
		level = charmlog.InfoLevel
	case "warn", "warning":
		level = charmlog.WarnLevel
	case "error":
		level = charmlog.ErrorLevel
	default:
		level = charmlog.InfoLevel
	}
	handler.SetLevel(level)

	// Set format
	switch strings.ToLower(format) {
	case "json":
		handler.SetFormatter(charmlog.JSONFormatter)
	case "logfmt":
		handler.SetFormatter(charmlog.LogfmtFormatter)
	case "text":
		handler.SetFormatter(charmlog.TextFormatter)
	default:
		// Default: use text formatter which has nice colors
		handler.SetFormatter(charmlog.TextFormatter)
	}

	// Create slog logger from charm handler
	defaultLogger = slog.New(handler)

	// Set as default slog logger
	slog.SetDefault(defaultLogger)
}

// Get returns the global logger instance
func Get() *slog.Logger {
	if defaultLogger == nil {
		// Fallback: initialize with defaults if not yet initialized
		Init("info", "text")
	}
	return defaultLogger
}
