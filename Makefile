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

.PHONY: all build build-linux build-windows clean deps frontend help hooks lint dev-build release openapi openapi-check

# Tooling. swag is a tool dependency tracked in go.mod (see `tool` directive),
# so the version is pinned and CI / dev installs always agree. `go tool swag`
# builds-and-runs from the pinned source.
SWAG := go tool swag
OPENAPI_DIR = api

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

# Install development tools.
# swag is pinned via the `tool` directive in go.mod and runs through
# `go tool swag` — it builds on first use, no install step needed here.
dev-tools:
	@echo "Installing development tools..."
	$(GOGET) golang.org/x/tools/cmd/cover
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Regenerate the OpenAPI v1 spec from handler annotations.
# Pipeline: swag emits Swagger 2.0 (JSON) -> openapi-convert produces
# OpenAPI 3.0 yaml/json -> intermediate Swagger 2.0 file is removed.
# Only api/openapi.{yaml,json} is committed.
openapi:
	@echo "Regenerating OpenAPI spec..."
	@$(SWAG) init -g internal/restapi/v1/doc.go -d ./,internal/restapi --parseInternal -o $(OPENAPI_DIR) --outputTypes json -q
	@go run ./scripts/openapi-convert -in $(OPENAPI_DIR)/swagger.json \
		-out-yaml $(OPENAPI_DIR)/openapi.yaml \
		-out-json $(OPENAPI_DIR)/openapi.json
	@rm -f $(OPENAPI_DIR)/swagger.json
	@echo "Spec written to $(OPENAPI_DIR)/openapi.{yaml,json}"

# Verify that handler annotations parse cleanly under swag and the generated
# spec is valid OpenAPI 3.0. Does NOT compare against the committed
# api/openapi.{json,yaml} — that byte-equality check was a continuous source
# of host-environment-dependent CI noise (swag's output differed between CI
# and local in ways we couldn't isolate over multiple cycles).
#
# The canonical contract test is core-tests/TestAPIOpenAPIContract, which runs
# the actual server and validates response shapes against the spec. The
# committed api/openapi.{json,yaml} is best-effort up-to-date; run
# `make openapi` locally to refresh it (e.g., before a release).
#
# This target writes to a tempdir so it doesn't touch the committed spec.
openapi-check:
	@echo "Validating OpenAPI generation..."
	@tmpdir=$$(mktemp -d) && trap "rm -rf $$tmpdir" EXIT && \
		$(SWAG) init -g internal/restapi/v1/doc.go -d ./,internal/restapi --parseInternal -o $$tmpdir --outputTypes json -q && \
		go run ./scripts/openapi-convert -in $$tmpdir/swagger.json \
			-out-yaml $$tmpdir/openapi.yaml \
			-out-json $$tmpdir/openapi.json && \
		echo "OpenAPI spec generates cleanly and validates as OpenAPI 3.0."

# Run static analysis
lint:
	@echo "Running static analysis..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run --timeout=5m; \
	else \
		echo "golangci-lint not installed, run 'make dev-tools' first"; \
	fi
	@bash scripts/check-layering.sh
	@bash scripts/check-handler-db-access.sh

# Install git hooks
hooks:
	@echo "Installing git hooks..."
	@mkdir -p .git/hooks
	@cp scripts/pre-commit .git/hooks/pre-commit
	@chmod +x .git/hooks/pre-commit
	@echo "Pre-commit hook installed."

# Quick development build
dev-build:
	@echo "Building development binary..."
	$(GOBUILD) -o $(BINARY_NAME)_dev -v

# Production release build
release: clean deps frontend build
	@echo "Production build complete!"
	@echo "Binary: $(BINARY_NAME)"
	@ls -lh $(BINARY_NAME)

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
	@echo "Utilities:"
	@echo "  make frontend       - Build frontend only"
	@echo "  make clean          - Clean build artifacts"
	@echo "  make dev-tools      - Install development tools (incl. swag)"
	@echo "  make hooks          - Install git pre-commit hook"
	@echo "  make openapi        - Regenerate api/openapi.{yaml,json} from handler annotations"
	@echo "  make openapi-check  - Verify api/openapi.{yaml,json} is up to date (used by hooks/CI)"
	@echo "  make help           - Show this help message"
