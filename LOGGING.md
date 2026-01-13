# Logging Guidelines

This codebase uses Go's structured logging (`log/slog`) with `charmbracelet/log` as the handler backend for human-friendly formatted output.

## Configuration

### Command-line Flags

```
--log-level   Log level: debug, info, warn, error (default: "info")
--log-format  Log format: text, json, logfmt (default: "text")
```

### Environment Variables

Environment variables override command-line flags:

```
LOG_LEVEL   Overrides --log-level
LOG_FORMAT  Overrides --log-format
```

### Examples

```bash
# Run with debug logging
./windshift --log-level=debug

# Run with JSON output (useful for log aggregation)
./windshift --log-format=json

# Using environment variables
LOG_LEVEL=debug LOG_FORMAT=json ./windshift
```

## Log Levels

| Level | When to Use |
|-------|-------------|
| **debug** | Detailed traces, performance metrics, request/response details. Only shown with `--log-level=debug`. |
| **info** | Normal operational events: startup, shutdown, significant state changes. Default visible level. |
| **warn** | Recoverable errors, deprecation notices, unexpected but handled conditions. |
| **error** | Failures requiring attention, unrecoverable errors, permission denials. |

## Structured Logging

All log messages use structured attributes for better filtering and analysis:

```go
slog.Debug("item create request received")
slog.Info("service started", slog.String("version", version))
slog.Warn("cache initialization failed", slog.Any("error", err))
slog.Error("permission check failed",
    slog.Int("user_id", userID),
    slog.Int("workspace_id", workspaceID),
    slog.Any("error", err))
```

### Common Attributes

| Attribute | Type | Description |
|-----------|------|-------------|
| `component` | string | Subsystem identifier (e.g., "sso", "jira", "scm") |
| `error` | any | Error value for failures |
| `user_id` | int | User ID for user-related operations |
| `workspace_id` | int | Workspace ID for workspace operations |
| `item_id` | int | Item ID for item operations |

### Component Tags

Use `slog.String("component", "name")` to identify subsystems:

| Component | Description |
|-----------|-------------|
| `sso` | SSO/OIDC authentication |
| `jira` | Jira integration |
| `scm` | SCM providers (GitHub, GitLab, etc.) |
| `notifications` | Notification service |
| `permissions` | Permission cache and checks |
| `attachments` | File attachments |
| `activity` | Activity tracking |
| `database` | Database operations |

## Output Formats

### Text (Default)

Human-readable colored output, ideal for development:

```
INFO  service started                       version=1.0.0
DEBUG creating session                      component=sso user_id=123
WARN  cache initialization failed           error="connection refused"
```

### JSON

Machine-readable format for log aggregation systems:

```json
{"level":"INFO","msg":"service started","version":"1.0.0"}
{"level":"DEBUG","msg":"creating session","component":"sso","user_id":123}
```

### Logfmt

Key-value format, compatible with many log analysis tools:

```
level=INFO msg="service started" version=1.0.0
level=DEBUG msg="creating session" component=sso user_id=123
```

## Performance Logging

Performance-critical sections use grouped timing attributes:

```go
slog.Debug("item creation performance",
    slog.Int("item_id", itemID),
    slog.Group("timings_ms",
        slog.Float64("validation", 1.23),
        slog.Float64("transaction", 4.56),
        slog.Float64("total", 7.89),
    ))
```

## Test Files

In test files, use `t.Logf()` instead of slog:

```go
func TestSomething(t *testing.T) {
    t.Logf("Debug info: %v", data)
}
```

## Best Practices

1. **Use appropriate log levels**: Debug for development details, Info for operational events, Warn for recoverable issues, Error for failures.

2. **Include context**: Always include relevant IDs (user_id, workspace_id, item_id) to aid debugging.

3. **Use component tags**: Add `slog.String("component", "name")` to identify the subsystem.

4. **Avoid sensitive data**: Never log passwords, tokens, or other secrets.

5. **Be concise**: Log messages should be lowercase and descriptive without prefixes like "Error:".

## Logger Initialization

The logger is initialized in `internal/logger/logger.go` and set as the default `slog` logger during application startup. All code can use `slog.*` functions directly without needing a logger instance.
