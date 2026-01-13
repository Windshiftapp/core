# Testing Overview

This document provides an overview of all testing infrastructure in the Windshift project.

## Test Suites

### 1. Go Integration Tests (`tests/`)

Black-box HTTP integration tests that start real server instances with isolated databases.

```bash
# Run all Go tests
cd tests && go test -v

# Run with HTML report
./tests/run-tests.sh

# Run specific test category
go test -v -run "Permission" ./tests/
go test -v -run "Workflow" ./tests/
go test -v -run "VQL" ./tests/
```

**Coverage:**
- API endpoint validation (workspaces, items, custom fields)
- Workflow system (statuses, workflows, screens, configuration sets)
- Permission system (global permissions, workspace roles, isolation)
- VQL queries (filtering, relationships)
- Link type restrictions
- Bearer token authentication

See `tests/README.md` for detailed test documentation.

### 2. Node.js Stress Tests (`stresstest/`)

Load and capacity testing using Node.js scripts.

```bash
# Basic load test
./stresstest/run-load-test.sh

# Mixed operations load test
./stresstest/run-mixed-load-test.sh

# PostgreSQL-specific tests
./stresstest/run-postgres-load-test.sh
./stresstest/run-postgres-capacity-test.sh
```

**Available Tests:**
- `load-test.js` - Basic concurrent request test
- `update-load-test.js` - Update operation stress test
- `mixed-load-test.js` - Mixed read/write operations
- `realistic-load-test.js` - Simulated real-world usage patterns

### 3. Database Schema Verification (`tests/`)

```bash
# Verify PostgreSQL schema
./tests/verify-postgres-schema.sh
```

## Test Infrastructure

### Test Database Management

- Go tests create isolated SQLite databases in the system temp directory
- Database files: `${TMPDIR}/windshift-tests/test_<timestamp>_<pid>.db`
- Automatic cleanup on test completion
- Stale database cleanup on test start (files older than 5 minutes)

### Test Server Management

- Each test starts its own server instance
- Random port allocation (8000-8999) to prevent conflicts
- Graceful shutdown with SIGTERM
- Output capture for debugging failed tests

### Authentication in Tests

Tests use bearer token authentication:
1. Complete initial setup (create admin user)
2. Login to get session cookie
3. Create API bearer token
4. Use bearer token for all API calls

## Running Tests

### Quick Start

```bash
# Run all integration tests with report
./tests/run-tests.sh

# Run specific test
cd tests && go test -v -run TestWorkspaceOperations

# Run with coverage
cd tests && go test -cover -coverprofile=coverage.out
```

### CI/CD Integration

```bash
# Recommended CI command
go test ./tests/... -v -cover -timeout 10m
```

## Test Documentation

- `tests/README.md` - Complete Go test documentation
- `tests/PERMISSION_TEST_DOCUMENTATION.md` - Permission system testing notes
- `stresstest/README.md` - Stress test documentation
