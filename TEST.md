# Testing Guide for Windshift

This document describes the testing strategy and practices for the Windshift work management system.

## Test Architecture

### Test Organization
```
internal/
├── handlers/
│   ├── setup_test.go         # Unit tests for setup handlers
│   └── testutils/            # Test utilities (build tag: test)
│       ├── database.go       # Database testing helpers
│       └── helpers.go        # HTTP testing helpers
├── database/
│   ├── database_test.go      # Database initialization tests
│   └── integrity_test.go     # Database integrity tests
└── models/
    └── models_test.go        # Model serialization tests
```

### Build Tags
All test utilities use the `//go:build test` build tag to ensure they are excluded from production builds.

**Test Files:**
- `*_test.go` - Automatically excluded from production builds
- `testutils/*` - Explicitly tagged with `//go:build test`

## Test Types

### 1. Unit Tests (`*_test.go`)

#### Setup Handler Tests (`internal/handlers/setup_test.go`)
- **Test Coverage:**
  - `GetSetupStatus` - Fresh vs configured database states
  - `CompleteInitialSetup` - Valid and invalid setup requests
  - `GetModuleSettings` / `UpdateModuleSettings` - CRUD operations
  - Validation errors and edge cases
  - Transaction rollback on failures
  - Password hashing security

#### Database Tests (`internal/database/database_test.go`)
- **Test Coverage:**
  - Fresh database initialization
  - Schema creation and migration
  - Default data population
  - Foreign key constraints
  - Index creation

#### Database Integrity Tests (`internal/database/integrity_test.go`)
- **Test Coverage:**
  - Foreign key constraint enforcement
  - Cascade deletion behavior
  - Unique constraint validation
  - Transaction safety and rollback
  - Hierarchical data integrity
  - Index performance validation

#### Model Tests (`internal/models/models_test.go`)
- **Test Coverage:**
  - JSON serialization/deserialization
  - Field validation
  - Password hash exclusion (security)
  - Default values
  - API contract compliance

### 2. Integration Tests (`tests/`)
- Node.js-based API integration tests
- 61 comprehensive test cases
- Full HTTP request/response validation
- Real database operations

## Test Utilities

### Database Helpers (`testutils/database.go`)

```go
// Create test database
tdb := testutils.CreateTestDB(t, true) // in-memory
defer tdb.Close()

// Assertions
tdb.AssertTableExists(t, "workspaces")
tdb.AssertForeignKeyEnabled(t)
tdb.AssertColumnExists(t, "items", "workspace_id")
tdb.AssertIndexExists(t, "idx_items_workspace_id")

// Test data management
data := tdb.SeedTestData(t)
tdb.ClearAllTables(t)

// Transaction testing
tdb.ExecuteInTransaction(t, func(tx *sql.Tx) error {
    // Test operations
    return nil
})
```

### HTTP Helpers (`testutils/helpers.go`)

```go
// Create test requests
req := testutils.CreateJSONRequest(t, "POST", "/api/setup/complete", data)

// Execute requests
rr := testutils.ExecuteRequest(t, handler.CompleteSetup, req)

// Assertions
rr.AssertStatusCode(http.StatusOK)
  .AssertContentType("application/json")
  .AssertJSONResponse(&response)

// Validation helpers
testutils.AssertValidationError(t, rr, "email is required")
testutils.AssertSuccessResponse(t, rr)
```

## Running Tests

### Using Make (Recommended)

```bash
# Run all tests
make test

# Run with coverage report
make test-coverage

# Run specific test suites
make test-setup    # Setup handler tests
make test-db       # Database tests  
make test-models   # Model tests

# Integration tests
make integration-test
```

### Using Go Commands

```bash
# All unit tests
go test -tags="test" -v ./...

# With coverage
go test -tags="test" -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Specific packages
go test -tags="test" -v ./internal/handlers
go test -tags="test" -v ./internal/database
go test -tags="test" -v ./internal/models
```

### Test Environment Variables

```bash
# For integration tests
export API_BASE=http://localhost:8080
```

## Build Configuration

### Production Builds (Exclude Tests)
```bash
# Production binary (no test code)
make build
# or
go build -tags="!test" -ldflags="-s -w" -o windshift

# Verify test exclusion
make verify-size
make test-build-exclusion
```

### Development Builds (Include Tests)
```bash
# Development binary (with test utilities)
make dev-build
# or
go build -o windshift_dev
```

## Test Coverage Goals

### Current Coverage Targets
- **Setup Handlers**: 95%+ (critical path)
- **Database Operations**: 90%+ (data integrity)
- **Models**: 85%+ (serialization)
- **Overall Project**: 80%+

### Coverage Reports
- HTML Report: `coverage/coverage.html`
- Terminal: `make test-coverage`

## Test Database Strategy

### In-Memory Testing
- **Pros**: Fast, isolated, parallel-safe
- **Cons**: SQLite-specific behavior only
- **Usage**: Unit tests, CI/CD pipelines

```go
tdb := testutils.CreateTestDB(t, true) // in-memory
```

### File-Based Testing
- **Pros**: Persistent, real file I/O testing
- **Cons**: Slower, requires cleanup
- **Usage**: Integration tests, file system testing

```go
tdb := testutils.CreateTestDB(t, false) // temp file
```

## Test Data Management

### Test Data Isolation
- Each test gets a fresh database
- Automatic cleanup after tests
- No shared state between tests

### Seed Data
```go
data := tdb.SeedTestData(t)
// Provides: WorkspaceID, UserID, StatusCategoryID, StatusID
```

### Custom Test Data
```go
// Clear and rebuild
tdb.ClearAllTables(t)
// Add custom test data as needed
```

## Continuous Integration

### GitHub Actions Integration
```yaml
- name: Run Go Unit Tests
  run: make test-coverage

- name: Verify Binary Size
  run: make verify-size

- name: Integration Tests
  run: make integration-test
```

### Pre-commit Hooks
```bash
# Add to .git/hooks/pre-commit
make test
make lint
make verify-size
```

## Best Practices

### 1. Test Structure
```go
func TestHandler_Function_Scenario(t *testing.T) {
    // Arrange
    tdb := testutils.CreateTestDB(t, true)
    defer tdb.Close()
    
    // Act
    result := functionUnderTest(params)
    
    // Assert
    if result != expected {
        t.Errorf("Expected %v, got %v", expected, result)
    }
}
```

### 2. Table-Driven Tests
```go
tests := []struct {
    name     string
    input    InputType
    expected OutputType
    shouldFail bool
}{
    {"valid case", validInput, expectedOutput, false},
    {"error case", invalidInput, nil, true},
}

for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        // Test implementation
    })
}
```

### 3. Error Testing
- Always test both success and failure paths
- Verify proper error messages
- Test transaction rollback scenarios
- Validate security measures (password hashing)

### 4. Performance Considerations
- Use in-memory databases for unit tests
- Parallel test execution where safe
- Benchmark critical paths
- Monitor test execution time

## Security Testing

### Password Security
- Verify passwords are hashed, not stored plain
- Test bcrypt hash verification
- Ensure password fields are excluded from JSON

### SQL Injection Prevention
- Test parameterized queries
- Validate input sanitization
- Test constraint violations

### Transaction Safety
- Test rollback scenarios
- Verify atomic operations
- Test concurrent access patterns

## Troubleshooting

### Common Issues

1. **Tests fail in CI but pass locally**
   - Check for race conditions
   - Verify test isolation
   - Check environment differences

2. **Binary size increased unexpectedly**
   - Run `make verify-size`
   - Check for missing build tags
   - Verify production build flags

3. **Database tests are slow**
   - Use in-memory databases for unit tests
   - Check for proper cleanup
   - Optimize test data setup

### Debugging Tips

```bash
# Run single test with verbose output
go test -tags="test" -v -run TestSpecificTest ./internal/handlers

# Run with race detection
go test -tags="test" -race ./...

# Profile test execution
go test -tags="test" -cpuprofile=cpu.prof -memprofile=mem.prof ./...
```

## Future Improvements

### Planned Enhancements
1. **Mutation Testing** - Verify test quality
2. **Property-Based Testing** - Generate test cases
3. **Performance Benchmarks** - Track regression
4. **API Contract Testing** - Validate OpenAPI compliance

### Test Metrics
- Coverage trends over time
- Test execution performance
- Flaky test detection
- Binary size monitoring