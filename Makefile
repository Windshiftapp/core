# Windshift Work Management System - Build Configuration

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Binary names
BINARY_NAME=windshift
BINARY_UNIX=$(BINARY_NAME)_unix
BINARY_WINDOWS=$(BINARY_NAME).exe

# Build flags
LDFLAGS=-ldflags="-s -w"
BUILD_TAGS=-tags="!test"

# Directories
FRONTEND_DIR=frontend

.PHONY: all build build-linux build-windows clean deps frontend help lint dev-build release test-setup

# Default target
all: clean frontend build

# Build production binary (excludes all test code)
build:
	@echo "Building production binary..."
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

# Update dependencies
deps:
	@echo "Updating dependencies..."
	$(GOMOD) tidy
	$(GOMOD) download

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

# Quick development build
dev-build:
	@echo "Building development binary..."
	$(GOBUILD) -o $(BINARY_NAME)_dev -v

# Production release build
release: clean deps frontend build
	@echo "Production build complete!"
	@echo "Binary: $(BINARY_NAME)"
	@ls -lh $(BINARY_NAME)

# Clone test repo and run tests locally
test-setup:
	@echo "Tests live in the private Windshiftapp/core-tests repo."
	@echo ""
	@echo "To run tests locally:"
	@echo "  git clone git@github.com:Windshiftapp/core-tests.git /tmp/core-tests"
	@echo "  /tmp/core-tests/overlay.sh ."
	@echo "  go test -tags=\"test\" -race -timeout=15m ./..."

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
	@echo "Development:"
	@echo "  make dev-build      - Development binary"
	@echo "  make lint           - Run static analysis"
	@echo "  make deps           - Update dependencies"
	@echo ""
	@echo "Testing:"
	@echo "  make test-setup     - Instructions for running tests (separate repo)"
	@echo ""
	@echo "Utilities:"
	@echo "  make frontend       - Build frontend only"
	@echo "  make clean          - Clean build artifacts"
	@echo "  make dev-tools      - Install development tools"
	@echo "  make help           - Show this help message"
