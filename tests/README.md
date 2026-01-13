# Windshift Integration Tests

This directory contains comprehensive integration tests for the Windshift API using **Go-based black box HTTP testing**.

## Overview

The test suite validates all core API endpoints using true black box testing - real HTTP requests over the network with full authentication and middleware validation.

## Quick Start

```bash
# From tests directory
cd tests
go test -v

# Run specific test
go test -v -run TestWorkspaceOperations

# From project root
go test ./tests/... -v -cover
```

## Architecture

These tests use **true black box HTTP testing**:

- ✅ Start real Windshift server with isolated database
- ✅ Make actual HTTP requests over network
- ✅ Test all middleware layers (CORS, CSRF, auth)
- ✅ Full authentication flow (setup → login → bearer token)
- ✅ Automatic cleanup of databases and processes
- ✅ Reusable test helpers and utilities

## Test Files

- `helpers.go` - Reusable test utilities (server management, auth flow, HTTP helpers)
- `integration_test.go` - Workspace, custom field, and work item tests (24 subtests)
- `workflow_test.go` - Workflow system tests (19 subtests)

## Prerequisites

- **Go 1.24+** installed
- **Windshift binary** built: `go build -o windshift` (from project root)

## Running Tests

```bash
# All tests
go test -v

# Specific test suite
go test -v -run TestWorkspaceOperations

# With coverage
go test -cover -coverprofile=coverage.out
go tool cover -html=coverage.out

# Parallel execution
go test -v -parallel 4

# Verbose with timing
go test -v -timeout 5m
```

## Test Categories

### Integration Tests (`integration_test.go`)

1. **TestWorkspaceOperations** (5 subtests)
   - Create workspace with valid data
   - Get all workspaces
   - Get workspace by ID
   - Update workspace
   - Delete workspace

2. **TestCustomFieldOperations** (5 subtests)
   - Create text custom field
   - Create select custom field with options
   - Get all custom fields
   - Update custom field
   - Delete custom fields

3. **TestWorkItemOperations** (5 subtests)
   - Create basic work item
   - Get all items
   - Get items by workspace
   - Update item status
   - Delete item

4. **TestWorkItemHierarchy** (6 subtests)
   - Create parent item (Epic)
   - Create child item (Story)
   - Create grandchild item (Task)
   - Get children of parent
   - Get all descendants
   - Get tree structure

5. **TestErrorHandling** (3 subtests)
   - Invalid workspace creation
   - Non-existent resource requests
   - Invalid parent ID relationships

### Workflow Tests (`workflow_test.go`)

1. **TestStatusCategoryOperations** (4 subtests)
   - Create status category
   - Get all status categories
   - Get status category by ID
   - Update status category

2. **TestStatusOperations** (4 subtests)
   - Create statuses with category links
   - Get all statuses
   - Get status by ID
   - Update status

3. **TestWorkflowOperations** (4 subtests)
   - Create workflow
   - Update workflow transitions
   - Get workflow with transitions
   - Update workflow metadata

4. **TestScreenOperations** (4 subtests)
   - Create screen
   - Add fields to screen
   - Get screen fields
   - Update screen field order

5. **TestConfigurationSetOperations** (3 subtests)
   - Create configuration set with workflow/screens
   - Get configuration sets with details
   - Update configuration set

### Permission Tests

#### Global Permissions (`permission_global_test.go`)

1. **TestGlobalPermissions_SystemAdmin** - System admin has full access
2. **TestGlobalPermissions_WorkspaceCreate** - Workspace creation permission
3. **TestGlobalPermissions_UserManage** - User management permission
4. **TestGlobalPermissions_IterationManage** - Iteration management permission
5. **TestGlobalPermissions_NonAdminCannotAccessAdminEndpoints** - Permission denial tests

#### Workspace Roles (`permission_workspace_test.go`)

1. **TestWorkspaceRoles_Viewer** - Read-only access validation
2. **TestWorkspaceRoles_Editor** - Read/write access validation
3. **TestWorkspaceRoles_Administrator** - Full workspace access
4. **TestWorkspaceRoles_EveryoneRole** - Default role for all users
5. **TestWorkspaceRoles_NoRole** - Access denial without role

#### Isolation Tests (`permission_isolation_test.go`)

1. **TestCrossWorkspaceIsolation** - Users cannot access other workspaces
2. **TestCrossWorkspaceIsolation_DifferentRolesPerWorkspace** - Role scoping
3. **TestWorkspaceListFiltering** - Users only see accessible workspaces
4. **TestItemListFiltering** - Items filtered by workspace access
5. **TestSystemAdminBypassesIsolation** - System admin override

### VQL Tests

1. **TestVQLChildrenOf** (`vql_relationships_test.go`) - VQL childrenOf relationship query
2. **TestVQLLinkedOf** (`vql_relationships_test.go`) - VQL linkedOf relationship query
3. **TestVQLFiltering** (`vql_filtering_test.go`) - VQL field filtering

### Link Tests (`link_restrictions_test.go`)

1. **TestLinkTypeRestrictions** - Item type restrictions on links
2. **TestTestsLinkTypeIsSystemProtected** - System link types protected

### Other Tests

1. **TestBearerTokenCSRFBypass** (`integration_test.go`) - Bearer token auth bypasses CSRF
2. **TestPortalWorkflow** (`portal_test.go`) - Portal request workflow
3. **TestMiniStressTest** (`stress_test_mini_test.go`) - Lightweight stress test
4. **TestSimpleRankingTest** (`simple_rank_test.go`) - Basic ranking operations
5. **TestQuickRankingTest** (`quick_rank_test.go`) - Quick ranking validation
6. **TestRankingDemo** (`ranking_demo_test.go`) - Ranking demonstration

## Test Coverage Summary

**Total:** 39 test functions across multiple test files

### Core API Tests
- ✅ TestWorkspaceOperations (5 subtests)
- ✅ TestCustomFieldOperations (5 subtests)
- ✅ TestWorkItemOperations (5 subtests)
- ✅ TestWorkItemHierarchy (6 subtests)
- ✅ TestErrorHandling (3 subtests)
- ✅ TestBearerTokenCSRFBypass

### Workflow Tests
- ✅ TestStatusCategoryOperations (4 subtests)
- ✅ TestStatusOperations (4 subtests)
- ✅ TestWorkflowOperations (4 subtests)
- ✅ TestScreenOperations (4 subtests)
- ✅ TestConfigurationSetOperations (3 subtests)

### Permission Tests
- ✅ TestGlobalPermissions_* (5 tests)
- ✅ TestWorkspaceRoles_* (5 tests)
- ✅ TestCrossWorkspaceIsolation* (2 tests)
- ✅ TestWorkspaceListFiltering
- ✅ TestItemListFiltering
- ✅ TestSystemAdminBypassesIsolation

### VQL & Link Tests
- ✅ TestVQLChildrenOf
- ✅ TestVQLLinkedOf
- ✅ TestVQLFiltering
- ✅ TestLinkTypeRestrictions
- ✅ TestTestsLinkTypeIsSystemProtected

### Other Tests
- ✅ TestPortalWorkflow
- ✅ TestMiniStressTest
- ✅ TestSimpleRankingTest
- ✅ TestQuickRankingTest
- ✅ TestRankingDemo

## Writing New Tests

Use the helper functions from `helpers.go` to write new tests:

```go
package tests

import (
    "net/http"
    "testing"
)

func TestMyFeature(t *testing.T) {
    // Setup: Start server and authenticate
    server, _ := StartTestServer(t, "sqlite")
    CreateBearerToken(t, server)

    t.Run("MySubTest", func(t *testing.T) {
        // Make authenticated request
        resp := MakeAuthRequest(t, server, http.MethodGet, "/my-endpoint", nil)
        defer resp.Body.Close()

        // Assert response
        AssertStatusCode(t, resp, http.StatusOK)

        // Decode and verify JSON
        var result map[string]interface{}
        DecodeJSON(t, resp, &result)
        AssertJSONField(t, result, "field", "expected value")
    })
}
```

## Helper Functions

### Server Management

- `StartTestServer(t, dbType)` - Starts isolated Windshift server
  - Creates unique database file
  - Uses random port (8000-8999)
  - Returns `*TestServer` and cleanup function
  - Cleanup registered with `t.Cleanup()` for automatic teardown

### Authentication

- `CreateBearerToken(t, server)` - Complete auth flow
  - Completes initial setup (admin user, modules)
  - Logs in with session cookie
  - Creates API bearer token
  - Stores token in `server.BearerToken`

### HTTP Requests

- `MakeAuthRequest(t, server, method, endpoint, body)` - Make authenticated request
- `makeRequest(t, method, url, bearerToken, body, headers)` - Low-level request helper

### Assertions

- `AssertStatusCode(t, resp, expected)` - Verify HTTP status code
- `DecodeJSON(t, resp, v)` - Decode JSON response
- `AssertJSONField(t, data, field, expected)` - Verify JSON field value

## Test Isolation

Each test gets:
- **Unique database file** - `test_<timestamp>_<pid>.db`
- **Random port** - Prevents conflicts
- **Separate server process** - True integration testing
- **Automatic cleanup** - Database and process cleaned up via `t.Cleanup()`

## Troubleshooting

### Build Errors

```bash
# Ensure windshift binary is built
cd /path/to/project/root
go build -o windshift
```

### Server Startup Timeout

If tests timeout during server startup:
- Check if port range (8000-8999) is available
- Verify windshift binary exists and is executable
- Check for database permission issues

### Test Failures

```bash
# Run with verbose output
go test -v

# Run specific failing test
go test -v -run TestWorkflowOperations/CreateWorkflow

# Check for leftover processes
lsof -ti:8080 | xargs kill  # Replace 8080 with stuck port
```

### Database Cleanup

Test databases are automatically cleaned up, but if you see leftover files:

```bash
# From tests directory
rm -f test_*.db test_*.db-shm test_*.db-wal
```

## Extending the Test Suite

### Adding New Test Suites

1. Create test function in appropriate file (`integration_test.go` or `workflow_test.go`)
2. Use `StartTestServer()` and `CreateBearerToken()` for setup
3. Organize subtests with `t.Run()`
4. Use helper assertions for validation

### Adding New Helpers

Add reusable helpers to `helpers.go`:

```go
// Example: Helper for creating a workspace
func CreateTestWorkspace(t *testing.T, server *TestServer, name string) int {
    t.Helper()

    data := map[string]interface{}{
        "name": name,
        "key":  fmt.Sprintf("TST%d", time.Now().Unix()),
    }

    resp := MakeAuthRequest(t, server, http.MethodPost, "/workspaces", data)
    defer resp.Body.Close()

    AssertStatusCode(t, resp, http.StatusCreated)

    var result map[string]interface{}
    DecodeJSON(t, resp, &result)

    return int(result["id"].(float64))
}
```

## CI/CD Integration

```bash
# Run in CI pipeline
go test ./tests/... -v -cover -timeout 10m

# Generate coverage report
go test ./tests/... -coverprofile=coverage.out
go tool cover -func=coverage.out

# HTML coverage report
go tool cover -html=coverage.out -o coverage.html
```

## Notes

- Tests use **real HTTP requests** over the network
- All test data is **isolated** per test run
- Tests are **idempotent** and can run multiple times
- Both **success and error scenarios** are validated
- Cleanup is **automatic** via Go's testing framework
- Tests validate **entire middleware stack** (CORS, CSRF, auth)
