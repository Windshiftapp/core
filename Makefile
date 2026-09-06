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
BINARY_WINDOWS_ARM64=$(BINARY_NAME)-windows-arm64.exe

# Build flags
LDFLAGS=-ldflags="-s -w"

# Directories
FRONTEND_DIR=frontend

.PHONY: all build build-linux build-windows build-windows-arm64 clean deps frontend help hooks lint dev-build release openapi openapi-v2 openapi-check openapi-v2-check openapi-v2-client-smoke coding-agent-image dev-tools install-golangci-lint install-govulncheck install-deadcode ci-tools-check ci-go ci-frontend ci

# Tooling. swag is a tool dependency tracked in go.mod (see `tool` directive),
# so the version is pinned and CI / dev installs always agree. `go tool swag`
# builds-and-runs from the pinned source.
SWAG := go tool swag
OPENAPI_DIR = api
GOLANGCI_LINT_VERSION := 2.13.1
GOVULNCHECK_VERSION := 1.3.0
DEADCODE_VERSION := 0.45.0
OAPI_CODEGEN_VERSION := 2.5.0
OAPI_RUNTIME_VERSION := 1.1.1
NODE_VERSION := 24.18.0
NPM_VERSION := 11.16.0

# Default target
all: clean frontend build

# Build production binary
build:
	@echo "Building production binary..."
	$(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME) -v

# Build for Linux
build-linux:
	@echo "Building for Linux..."
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_UNIX) -v

# Build for Windows
build-windows:
	@echo "Building for Windows..."
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_WINDOWS) -v

# Build for Windows on ARM
build-windows-arm64:
	@echo "Building for Windows on ARM64..."
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_WINDOWS_ARM64) -v

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
	@rm -f $(BINARY_WINDOWS_ARM64)

# Update dependencies
deps:
	@echo "Updating dependencies..."
	$(GOMOD) tidy
	$(GOMOD) download

# Install the same Go tools and versions used by .github/workflows/go.yml.
# swag is pinned via the `tool` directive in go.mod and runs through
# `go tool swag`, so it needs no separate install.
dev-tools: install-golangci-lint install-govulncheck install-deadcode
	@echo "Development tools match CI."

install-golangci-lint:
	@echo "Installing golangci-lint $(GOLANGCI_LINT_VERSION)..."
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v$(GOLANGCI_LINT_VERSION)

install-govulncheck:
	@echo "Installing govulncheck $(GOVULNCHECK_VERSION)..."
	go install golang.org/x/vuln/cmd/govulncheck@v$(GOVULNCHECK_VERSION)

install-deadcode:
	@echo "Installing deadcode $(DEADCODE_VERSION)..."
	go install golang.org/x/tools/cmd/deadcode@v$(DEADCODE_VERSION)

# Fail early when the local runtime/tool versions differ from CI. Use the
# repository .nvmrc (or mise) for Node, then run `make dev-tools` for Go tools.
ci-tools-check:
	@expected_go="go$$(awk '$$1 == "go" { print $$2; exit }' go.mod)"; \
		[ "$$(go env GOVERSION)" = "$$expected_go" ] || { echo "$$expected_go required (found $$(go env GOVERSION))."; exit 1; }
	@[ "$$(node --version)" = "v$(NODE_VERSION)" ] || { echo "Node $(NODE_VERSION) required (found $$(node --version)); run 'nvm use' or 'mise use'."; exit 1; }
	@[ "$$(npm --version)" = "$(NPM_VERSION)" ] || { echo "npm $(NPM_VERSION) required (found $$(npm --version))."; exit 1; }
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint missing; run 'make dev-tools'."; exit 1; }
	@golangci-lint version | grep -q "version $(GOLANGCI_LINT_VERSION)" || { echo "golangci-lint $(GOLANGCI_LINT_VERSION) required; run 'make dev-tools'."; exit 1; }
	@command -v govulncheck >/dev/null 2>&1 || { echo "govulncheck missing; run 'make dev-tools'."; exit 1; }
	@govulncheck -version | grep -q "Scanner: govulncheck@v$(GOVULNCHECK_VERSION)" || { echo "govulncheck $(GOVULNCHECK_VERSION) required; run 'make dev-tools'."; exit 1; }

# Local equivalents of the blocking GitHub workflows. These intentionally use
# npm ci so the lockfile and clean-install path are exercised exactly as in CI.
ci-go: ci-tools-check
	cd $(FRONTEND_DIR) && npm ci && npm run build
	$(MAKE) lint
	govulncheck ./...
	$(MAKE) openapi-check
	$(GOBUILD) $(LDFLAGS) -o /tmp/windshift-ci .

ci-frontend: ci-tools-check
	cd $(FRONTEND_DIR) && npm ci
	cd $(FRONTEND_DIR) && npm audit signatures --min-release-age=0
	cd $(FRONTEND_DIR) && npm run check
	cd $(FRONTEND_DIR) && npm run typecheck
	cd $(FRONTEND_DIR) && npm run build

ci: ci-go ci-frontend

# Regenerate the OpenAPI v1 spec from handler annotations.
# Pipeline: swag emits Swagger 2.0 (JSON) -> openapi-convert produces
# OpenAPI 3.0 JSON -> intermediate Swagger 2.0 file is removed.
# Only api/openapi.json is committed.
openapi:
	@echo "Regenerating OpenAPI spec..."
	@$(SWAG) init -g internal/restapi/v1/doc.go -d ./,internal/restapi --parseInternal -o $(OPENAPI_DIR) --outputTypes json -q
	@go run ./scripts/openapi-convert -in $(OPENAPI_DIR)/swagger.json \
		-out-json $(OPENAPI_DIR)/openapi.json
	@rm -f $(OPENAPI_DIR)/swagger.json
	@echo "Spec written to $(OPENAPI_DIR)/openapi.json"

# Regenerate the public v2 schemas and shared responses from canonical route
# request, response, media-type, envelope, and success-status metadata.
openapi-v2:
	@go run ./scripts/openapi-v2-generate -spec $(OPENAPI_DIR)/openapi-v2.json

# Verify that handler annotations parse cleanly under swag and the generated
# spec is valid OpenAPI 3.0. Does NOT compare against the committed
# api/openapi.json — that byte-equality check was a continuous source
# of host-environment-dependent CI noise (swag's output differed between CI
# and local in ways we couldn't isolate over multiple cycles).
#
# The canonical contract test is core-tests/TestAPIOpenAPIContract, which runs
# the actual server and validates response shapes against the spec. The
# committed api/openapi.json is best-effort up-to-date; run
# `make openapi` locally to refresh it (e.g., before a release).
#
# This target writes to a tempdir so it doesn't touch the committed spec.
coding-agent-image:
	@echo "Building ws-carrier image (WS_IMAGE source for windshift-agent)..."
	docker build -f deploy/coding-agent/Dockerfile -t windshift/ws-carrier:local .

openapi-check:
	@echo "Validating OpenAPI generation..."
	@tmpdir=$$(mktemp -d) && trap "rm -rf $$tmpdir" EXIT && \
		$(SWAG) init -g internal/restapi/v1/doc.go -d ./,internal/restapi --parseInternal -o $$tmpdir --outputTypes json -q && \
		go run ./scripts/openapi-convert -in $$tmpdir/swagger.json \
			-out-json $$tmpdir/openapi.json && \
		echo "OpenAPI spec generates cleanly and validates as OpenAPI 3.0."
	@$(MAKE) --no-print-directory openapi-v2-check

openapi-v2-check:
	@go run ./scripts/openapi-v2-metadata -check
	@go run ./scripts/openapi-v2-generate -spec api/openapi-v2.json -check
	@go run ./scripts/openapi-v2-check -spec api/openapi-v2.json
	@$(MAKE) --no-print-directory openapi-v2-client-smoke

openapi-v2-client-smoke:
	@tmpdir=$$(mktemp -d /tmp/windshift-v2-client.XXXXXX) && trap 'rm -rf "$$tmpdir"' EXIT && \
		go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v$(OAPI_CODEGEN_VERSION) \
			-generate types,client -package v2client -o "$$tmpdir/client.gen.go" api/openapi-v2.json && \
		cd "$$tmpdir" && go mod init windshift-v2-client-smoke >/dev/null && \
		go get github.com/oapi-codegen/runtime@v$(OAPI_RUNTIME_VERSION) >/dev/null && \
		go test ./...

# Run static analysis
lint:
	@echo "Running static analysis..."
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not installed; run 'make dev-tools' first"; exit 1; }
	@scripts/run-golangci-lint.sh run --timeout=5m
	@node scripts/check-sql-boolean-literals.mjs
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
	@echo "  make build          - Build production binary"
	@echo "  make build-linux    - Cross-compile for Linux"
	@echo "  make build-windows  - Cross-compile for Windows"
	@echo "  make build-windows-arm64 - Cross-compile for Windows on ARM64"
	@echo "  make release        - Full production release build"
	@echo ""
	@echo "Development:"
	@echo "  make dev-build      - Development binary"
	@echo "  make lint           - Run static analysis"
	@echo "  make deps           - Update dependencies"
	@echo "  make ci             - Run the blocking Go + frontend CI checks locally"
	@echo "  make ci-go          - Run the blocking Go CI checks locally"
	@echo "  make ci-frontend    - Run the blocking frontend CI checks locally"
	@echo "  make ci-tools-check - Verify local Node/npm/Go tool versions match CI"
	@echo ""
	@echo "Utilities:"
	@echo "  make frontend       - Build frontend only"
	@echo "  make clean          - Clean build artifacts"
	@echo "  make dev-tools      - Install pinned Go tools used by CI"
	@echo "  make hooks          - Install git pre-commit hook"
	@echo "  make openapi        - Regenerate api/openapi.json from handler annotations"
	@echo "  make openapi-v2     - Regenerate typed v2 schemas from canonical route metadata"
	@echo "  make openapi-v2-client-smoke - Generate and compile a pinned Go v2 client"
	@echo "  make openapi-check  - Validate v1 generation and v2 route/spec parity (used by hooks/CI)"
	@echo "  make coding-agent-image - Build the thin ws-carrier image (WS_IMAGE for windshift-agent)"
	@echo "  make help           - Show this help message"
