# Windshift Work Management System - Build Configuration
# This Makefile ensures tests don't bloat the production binary

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Binary names
BINARY_NAME=windshift
BINARY_UNIX=$(BINARY_NAME)_unix
BINARY_WINDOWS=$(BINARY_NAME).exe

# Build flags
LDFLAGS=-ldflags="-s -w"
BUILD_TAGS=-tags="!test"

# Test flags
TEST_TAGS=-tags="test"
TEST_FLAGS=-v -race -coverprofile=coverage.out
TEST_TIMEOUT=-timeout=30s

# Directories
FRONTEND_DIR=frontend
COVERAGE_DIR=coverage

.PHONY: all build build-linux build-windows clean test test-coverage test-verbose deps frontend help

# Default target
all: clean frontend build

# Build production binary (excludes all test code)
build: 
	@echo "Building production binary..."
	@echo "Excluding test code with build tags..."
	$(GOBUILD) $(BUILD_TAGS) $(LDFLAGS) -o $(BINARY_NAME) -v

# Build for Linux
build-linux:
	@echo "Building for Linux..."
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) $(BUILD_TAGS) $(LDFLAGS) -o $(BINARY_UNIX) -v

# Build for Windows  
build-windows:
	@echo "Building for Windows..."
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GOBUILD) $(BUILD_TAGS) $(LDFLAGS) -o $(BINARY_WINDOWS) -v

# Build frontend
frontend:
	@echo "Building frontend..."
	@cd $(FRONTEND_DIR) && npm run build

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	$(GOCLEAN)
	@rm -f $(BINARY_NAME)
	@rm -f $(BINARY_UNIX)
	@rm -f $(BINARY_WINDOWS)
	@rm -f coverage.out
	@rm -rf $(COVERAGE_DIR)

# Run all tests
test:
	@echo "Running unit tests..."
	$(GOTEST) $(TEST_TAGS) $(TEST_FLAGS) $(TEST_TIMEOUT) ./...

# Run tests with detailed coverage
test-coverage: test
	@echo "Generating coverage report..."
	@mkdir -p $(COVERAGE_DIR)
	$(GOCMD) tool cover -html=coverage.out -o $(COVERAGE_DIR)/coverage.html
	$(GOCMD) tool cover -func=coverage.out | tail -1
	@echo "Coverage report generated: $(COVERAGE_DIR)/coverage.html"

# Run tests with verbose output
test-verbose:
	@echo "Running tests with verbose output..."
	$(GOTEST) $(TEST_TAGS) -v $(TEST_TIMEOUT) ./...

# Run specific test
test-setup:
	@echo "Running setup handler tests..."
	$(GOTEST) $(TEST_TAGS) -v $(TEST_TIMEOUT) ./internal/handlers -run TestSetupHandler

# Run database tests
test-db:
	@echo "Running database tests..."
	$(GOTEST) $(TEST_TAGS) -v $(TEST_TIMEOUT) ./internal/database

# Run model tests
test-models:
	@echo "Running model tests..."
	$(GOTEST) $(TEST_TAGS) -v $(TEST_TIMEOUT) ./internal/models

# Update dependencies
deps:
	@echo "Updating dependencies..."
	$(GOMOD) tidy
	$(GOMOD) download

# Verify binary size (production vs test build)
verify-size: build
	@echo "Verifying binary size..."
	@echo "Production binary size:"
	@ls -lh $(BINARY_NAME) | awk '{print $$5 " " $$9}'
	@echo ""
	@echo "Building with test code (for comparison)..."
	@$(GOBUILD) -o $(BINARY_NAME)_with_tests -v
	@echo "Binary with test code size:"
	@ls -lh $(BINARY_NAME)_with_tests | awk '{print $$5 " " $$9}'
	@echo ""
	@echo "Size difference:"
	@ls -l $(BINARY_NAME) $(BINARY_NAME)_with_tests | awk 'NR==1{prod=$$5} NR==2{test=$$5; diff=test-prod; print diff " bytes (" int(diff/prod*100) "% larger with tests)"}'
	@rm -f $(BINARY_NAME)_with_tests

# Install development tools
dev-tools:
	@echo "Installing development tools..."
	$(GOGET) golang.org/x/tools/cmd/cover
	$(GOGET) honnef.co/go/tools/cmd/staticcheck

# Run static analysis
lint:
	@echo "Running static analysis..."
	@if command -v staticcheck >/dev/null 2>&1; then \
		staticcheck ./...; \
	else \
		echo "staticcheck not installed, run 'make dev-tools' first"; \
	fi

# Quick development build (includes tests for development)
dev-build:
	@echo "Building development binary (includes test utilities)..."
	$(GOBUILD) -o $(BINARY_NAME)_dev -v

# Full development cycle
dev: clean frontend dev-build test

# Production release build
release: clean deps frontend build verify-size
	@echo "Production build complete!"
	@echo "Binary: $(BINARY_NAME)"
	@ls -lh $(BINARY_NAME)

# Integration test (requires running server)
integration-test:
	@echo "Running integration tests..."
	@cd tests && ./run-all-tests.sh

# Benchmark tests
benchmark:
	@echo "Running benchmark tests..."
	$(GOTEST) $(TEST_TAGS) -bench=. -benchmem ./...

# Show help
help:
	@echo "Windshift Build System"
	@echo "=================="
	@echo ""
	@echo "Production builds:"
	@echo "  make build          - Build production binary (excludes test code)"
	@echo "  make build-linux    - Cross-compile for Linux"
	@echo "  make build-windows  - Cross-compile for Windows"  
	@echo "  make release        - Full production release build"
	@echo ""
	@echo "Development builds:"
	@echo "  make dev-build      - Development binary (includes test utils)"
	@echo "  make dev            - Full development cycle"
	@echo ""
	@echo "Testing:"
	@echo "  make test           - Run all unit tests"
	@echo "  make test-coverage  - Run tests with coverage report"
	@echo "  make test-verbose   - Run tests with verbose output"
	@echo "  make test-setup     - Run setup handler tests only"
	@echo "  make test-db        - Run database tests only"
	@echo "  make test-models    - Run model tests only"
	@echo "  make integration-test - Run integration test suite"
	@echo "  make benchmark      - Run benchmark tests"
	@echo ""
	@echo "Utilities:"
	@echo "  make frontend       - Build frontend only"
	@echo "  make clean          - Clean build artifacts"
	@echo "  make deps           - Update dependencies"
	@echo "  make verify-size    - Compare binary sizes with/without tests"
	@echo "  make lint           - Run static analysis"
	@echo "  make dev-tools      - Install development tools"
	@echo "  make help           - Show this help message"

# Test that production build excludes test files
test-build-exclusion:
	@echo "Testing that production build excludes test code..."
	@$(GOBUILD) $(BUILD_TAGS) $(LDFLAGS) -o $(BINARY_NAME)_prod
	@if nm $(BINARY_NAME)_prod | grep -i test >/dev/null 2>&1; then \
		echo "ERROR: Test code found in production binary!"; \
		exit 1; \
	else \
		echo "SUCCESS: Production binary excludes test code"; \
	fi
	@rm -f $(BINARY_NAME)_prod