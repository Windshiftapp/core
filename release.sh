#!/bin/bash
set -euo pipefail

# =============================================================================
# Windshift Release Script
# Consolidates build-zig.sh and docker-build.sh into a unified release workflow
# =============================================================================

# Configuration
GHCR_REGISTRY="ghcr.io/windshiftapp/windshift"
GITHUB_REPO="Windshiftapp/windshift"
DOCKER_PLATFORMS="linux/amd64,linux/arm64"

# Build configurations: GOOS/GOARCH/ZIG_TARGET
NATIVE_PLATFORMS=(
    "linux/amd64/x86_64-linux-musl"
    "linux/arm64/aarch64-linux-musl"
    "windows/amd64/x86_64-windows-gnu"
)

# Darwin platforms (only buildable on macOS)
DARWIN_PLATFORMS=(
    "darwin/amd64/x86_64-macos"
    "darwin/arm64/aarch64-macos"
)

# State variables
VERSION=""
NOTES_FILE=""
DRY_RUN=false
SKIP_FRONTEND=false
CLEAN_FIRST=false
CONFIRM=true
TAG_CREATED=false

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# =============================================================================
# Utility Functions
# =============================================================================

log_info()    { echo -e "${BLUE}[INFO]${NC} $*"; }
log_success() { echo -e "${GREEN}[OK]${NC} $*"; }
log_warn()    { echo -e "${YELLOW}[WARN]${NC} $*"; }
log_error()   { echo -e "${RED}[ERROR]${NC} $*"; }
log_step()    { echo -e "${CYAN}[$1]${NC} $2"; }

die() { log_error "$*"; exit 1; }

dry_run_or_exec() {
    if [ "$DRY_RUN" = true ]; then
        log_info "[DRY-RUN] Would execute: $*"
        return 0
    else
        "$@"
    fi
}

# =============================================================================
# Version Management
# =============================================================================

get_git_tag() {
    git describe --tags --exact-match HEAD 2>/dev/null || echo ""
}

get_latest_tag() {
    git describe --tags --abbrev=0 2>/dev/null || echo ""
}

generate_next_version() {
    local latest=$(get_latest_tag)
    if [ -z "$latest" ]; then
        echo "v0.1.0"
    else
        local version="${latest#v}"
        local major minor patch
        IFS='.' read -r major minor patch <<< "$version"
        # Handle pre-release suffixes (e.g., v0.1.0-dev)
        patch="${patch%%-*}"
        patch=$((patch + 1))
        echo "v${major}.${minor}.${patch}"
    fi
}

validate_version() {
    local version="$1"
    if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.-]+)?$ ]]; then
        die "Invalid version format: $version (expected vX.Y.Z or vX.Y.Z-suffix)"
    fi
}

determine_version() {
    if [ -n "$VERSION" ]; then
        validate_version "$VERSION"
        log_info "Using specified version: $VERSION"
    else
        local current_tag=$(get_git_tag)
        if [ -n "$current_tag" ]; then
            VERSION="$current_tag"
            log_info "Using existing tag on HEAD: $VERSION"
        else
            VERSION=$(generate_next_version)
            log_info "Auto-generated version: $VERSION (bumping from $(get_latest_tag))"
        fi
    fi
}

tag_exists() {
    git rev-parse "$1" &>/dev/null
}

create_git_tag() {
    local tag="$1"

    if tag_exists "$tag"; then
        log_warn "Tag $tag already exists"
        return 0
    fi

    if [ "$DRY_RUN" = true ]; then
        log_info "[DRY-RUN] Would create git tag: $tag"
        log_info "[DRY-RUN] Would push tag to remote"
        return 0
    fi

    git tag -a "$tag" -m "Release $tag"
    log_success "Created git tag: $tag"
    git push origin "$tag"
    log_success "Pushed tag to remote"
    TAG_CREATED=true
}

# =============================================================================
# Pre-flight Checks
# =============================================================================

check_dependencies() {
    log_info "Checking dependencies..."

    local missing=()

    command -v go >/dev/null 2>&1 || missing+=("go")
    command -v npm >/dev/null 2>&1 || missing+=("npm")

    if [ ${#missing[@]} -gt 0 ]; then
        die "Missing required tools: ${missing[*]}"
    fi

    # Zig is optional - only needed for cross-compilation
    if ! command -v zig >/dev/null 2>&1; then
        log_warn "Zig not found - only native platform builds available"
    fi

    log_success "Dependencies OK"
}

check_docker() {
    if ! command -v docker >/dev/null 2>&1; then
        die "Docker not found - required for Docker builds"
    fi

    if ! docker buildx version &>/dev/null; then
        die "Docker Buildx not available - required for multi-arch builds"
    fi
}

check_gh_cli() {
    if ! command -v gh >/dev/null 2>&1; then
        die "GitHub CLI (gh) not found - required for GitHub releases"
    fi

    if ! gh auth status &>/dev/null; then
        die "GitHub CLI not authenticated - run 'gh auth login' first"
    fi
}

check_git_state() {
    if ! git diff-index --quiet HEAD --; then
        die "Uncommitted changes detected. Commit or stash before releasing."
    fi

    local branch=$(git branch --show-current)
    if [ "$branch" != "main" ] && [ "$branch" != "master" ]; then
        log_warn "Not on main/master branch (currently on: $branch)"
    fi
}

# =============================================================================
# Build Functions
# =============================================================================

build_frontend() {
    if [ "$SKIP_FRONTEND" = true ]; then
        log_info "Skipping frontend build"
        return 0
    fi

    log_step "1/6" "Building frontend..."

    if [ "$DRY_RUN" = true ]; then
        log_info "[DRY-RUN] Would run: cd frontend && npm install && npm run build"
        return 0
    fi

    (cd frontend && npm install --silent && npm run build)

    if [ ! -d "frontend/dist" ]; then
        die "Frontend build failed: dist/ not created"
    fi

    log_success "Frontend built"
}

build_binary() {
    local goos="$1"
    local goarch="$2"
    local zig_target="$3"

    local output_name="windshift"
    [ "$goos" = "windows" ] && output_name="windshift.exe"

    local output_path="dist/binaries/windshift-${goos}-${goarch}"
    [ "$goos" = "windows" ] && output_path="${output_path}.exe"

    log_info "  Building for ${goos}/${goarch}..."

    if [ "$DRY_RUN" = true ]; then
        log_info "  [DRY-RUN] Would build: $output_path"
        return 0
    fi

    # Darwin: use native toolchain (requires macOS)
    if [ "$goos" = "darwin" ]; then
        local current_os=$(uname -s | tr '[:upper:]' '[:lower:]')
        if [ "$current_os" != "darwin" ]; then
            log_warn "  Skipping darwin/${goarch} - requires macOS host"
            return 0
        fi
        export CGO_ENABLED=1 GOOS="$goos" GOARCH="$goarch"
        unset CC CXX
    else
        # Use Zig for cross-compilation (Linux, Windows)
        if ! command -v zig >/dev/null 2>&1; then
            log_warn "  Skipping ${goos}/${goarch} - Zig not installed"
            return 0
        fi
        export CGO_ENABLED=1 GOOS="$goos" GOARCH="$goarch"
        export CC="zig cc -target ${zig_target}"
        export CXX="zig c++ -target ${zig_target}"
    fi

    if go build -ldflags "-s -w" -o "$output_path" .; then
        local size=$(ls -lh "$output_path" | awk '{print $5}')
        log_success "  Built: $output_path ($size)"
    else
        log_error "  Failed to build for ${goos}/${goarch}"
        return 1
    fi
}

build_binaries() {
    log_step "2/6" "Building native binaries..."

    dry_run_or_exec mkdir -p dist/binaries

    # Build for standard platforms (Zig CC)
    for platform in "${NATIVE_PLATFORMS[@]}"; do
        IFS="/" read -r goos goarch zig_target <<< "$platform"
        build_binary "$goos" "$goarch" "$zig_target" || true
    done

    # Build for Darwin if on macOS
    if [ "$(uname -s | tr '[:upper:]' '[:lower:]')" = "darwin" ]; then
        for platform in "${DARWIN_PLATFORMS[@]}"; do
            IFS="/" read -r goos goarch zig_target <<< "$platform"
            build_binary "$goos" "$goarch" "$zig_target" || true
        done
    fi

    log_success "Binary builds complete"
}

create_release_packages() {
    log_step "3/6" "Creating release packages..."

    dry_run_or_exec mkdir -p dist/releases

    if [ "$DRY_RUN" = true ]; then
        log_info "[DRY-RUN] Would create release packages for all built binaries"
        return 0
    fi

    for binary in dist/binaries/windshift-*; do
        [ -f "$binary" ] || continue

        local basename=$(basename "$binary")
        local platform="${basename#windshift-}"
        platform="${platform%.exe}"

        local package_name="windshift-${VERSION}-${platform}"
        local package_dir="dist/releases/${package_name}"

        mkdir -p "$package_dir"

        # Copy binary
        if [[ "$platform" == *windows* ]]; then
            cp "$binary" "$package_dir/windshift.exe"
        else
            cp "$binary" "$package_dir/windshift"
        fi

        # Copy documentation
        [ -f "README.md" ] && cp README.md "$package_dir/" || true

        # Create sample config
        cat > "$package_dir/config.example.env" << 'CONFIGEOF'
# Windshift Configuration
PORT=8080

# Database - Choose one:
# SQLite (default)
DATABASE_PATH=windshift.db

# PostgreSQL (uncomment to use)
# POSTGRES_CONNECTION_STRING=postgresql://user:password@localhost:5432/windshift?sslmode=disable
CONFIGEOF

        # Create archive
        if [[ "$platform" == *windows* ]]; then
            (cd dist/releases && zip -q -r "${package_name}.zip" "${package_name}")
            log_success "Created ${package_name}.zip"
        else
            (cd dist/releases && tar -czf "${package_name}.tar.gz" "${package_name}")
            log_success "Created ${package_name}.tar.gz"
        fi

        rm -rf "$package_dir"
    done

    # Generate checksums
    if ls dist/releases/*.tar.gz dist/releases/*.zip 2>/dev/null | head -1 >/dev/null; then
        (cd dist/releases && sha256sum *.tar.gz *.zip 2>/dev/null > SHA256SUMS.txt || true)
        log_success "Generated SHA256SUMS.txt"
    fi
}

ensure_buildx() {
    if ! docker buildx inspect windshift-builder &>/dev/null; then
        log_info "Creating buildx builder..."
        dry_run_or_exec docker buildx create --name windshift-builder --use
    else
        dry_run_or_exec docker buildx use windshift-builder
    fi
}

build_docker() {
    log_step "4/6" "Building Docker images..."

    check_docker
    ensure_buildx

    local tags="-t ${GHCR_REGISTRY}:${VERSION}"

    # Only tag as latest for official releases (not dev/test versions)
    if [[ ! "$VERSION" =~ -dev|-test|-rc ]]; then
        tags="$tags -t ${GHCR_REGISTRY}:latest"
    fi

    log_info "Platforms: ${DOCKER_PLATFORMS}"
    log_info "Tags: ${GHCR_REGISTRY}:${VERSION}"

    if [ "$DRY_RUN" = true ]; then
        log_info "[DRY-RUN] Would build and push Docker images"
        return 0
    fi

    docker buildx build \
        --platform "$DOCKER_PLATFORMS" \
        $tags \
        --push \
        .

    log_success "Docker images pushed to ${GHCR_REGISTRY}"
}

create_github_release() {
    log_step "5/6" "Creating GitHub release..."

    check_gh_cli

    # Create git tag if needed
    local current_tag=$(get_git_tag)
    if [ -z "$current_tag" ]; then
        create_git_tag "$VERSION"
    fi

    if [ "$DRY_RUN" = true ]; then
        log_info "[DRY-RUN] Would create GitHub release ${VERSION}"
        log_info "[DRY-RUN] Would upload assets from dist/releases/"
        return 0
    fi

    # Collect assets
    local assets=()
    for file in dist/releases/*.tar.gz dist/releases/*.zip dist/releases/SHA256SUMS.txt; do
        [ -f "$file" ] && assets+=("$file")
    done

    if [ ${#assets[@]} -eq 0 ]; then
        log_warn "No release assets found"
    fi

    # Create release with notes file
    gh release create "$VERSION" \
        --repo "$GITHUB_REPO" \
        --title "Windshift $VERSION" \
        --notes-file "$NOTES_FILE" \
        "${assets[@]}"

    log_success "GitHub release created: https://github.com/${GITHUB_REPO}/releases/tag/${VERSION}"
}

# =============================================================================
# Commands
# =============================================================================

cmd_build() {
    check_dependencies
    determine_version

    [ "$CLEAN_FIRST" = true ] && rm -rf dist/

    build_frontend
    build_binaries
    create_release_packages

    echo ""
    log_success "Build complete! Artifacts in dist/"
    echo ""
    echo "Release packages:"
    ls -1 dist/releases/*.tar.gz dist/releases/*.zip 2>/dev/null | sed 's/^/  /' || echo "  (none)"
}

cmd_push() {
    check_dependencies
    check_docker
    determine_version

    if [ "$CONFIRM" = true ] && [ "$DRY_RUN" = false ]; then
        echo ""
        echo "Windshift Docker Push: $VERSION"
        echo "=============================="
        echo "This will:"
        echo "  - Build frontend"
        echo "  - Build and push Docker images to ${GHCR_REGISTRY}"
        echo ""
        echo "Note: This does NOT create a GitHub release."
        echo ""
        read -p "Continue? [y/N] " -n 1 -r
        echo
        [[ $REPLY =~ ^[Yy]$ ]] || exit 1
    fi

    [ "$CLEAN_FIRST" = true ] && rm -rf dist/

    build_frontend
    build_docker

    echo ""
    log_success "Push complete!"
    echo ""
    echo "Docker image: ${GHCR_REGISTRY}:${VERSION}"
    echo "Pull with: docker pull ${GHCR_REGISTRY}:${VERSION}"
}

cmd_release() {
    # Validate release notes file
    if [ -z "$NOTES_FILE" ]; then
        die "Release notes file required. Use: ./release.sh release -v VERSION -n NOTES_FILE"
    fi

    if [ ! -f "$NOTES_FILE" ]; then
        die "Release notes file not found: $NOTES_FILE"
    fi

    check_dependencies
    check_docker
    check_gh_cli
    check_git_state
    determine_version

    if [ "$CONFIRM" = true ] && [ "$DRY_RUN" = false ]; then
        echo ""
        echo "Windshift Release: $VERSION"
        echo "=========================="
        echo "This will:"
        echo "  - Build frontend"
        echo "  - Build binaries for multiple platforms"
        echo "  - Create release packages with checksums"
        echo "  - Build and push Docker images"
        echo "  - Create git tag and push"
        echo "  - Create GitHub release with assets"
        echo ""
        echo "Release notes: $NOTES_FILE"
        echo ""
        read -p "Continue? [y/N] " -n 1 -r
        echo
        [[ $REPLY =~ ^[Yy]$ ]] || exit 1
    fi

    [ "$CLEAN_FIRST" = true ] && rm -rf dist/

    build_frontend
    build_binaries
    create_release_packages
    build_docker
    create_github_release

    echo ""
    log_success "Release $VERSION complete!"
    echo ""
    echo "GitHub: https://github.com/${GITHUB_REPO}/releases/tag/${VERSION}"
    echo "Docker: docker pull ${GHCR_REGISTRY}:${VERSION}"
}

# =============================================================================
# Help
# =============================================================================

show_help() {
    cat << 'EOF'
Windshift Release Script

Usage: ./release.sh <command> [options]

Commands:
  build       Build binaries and packages locally (no publish)
  push        Build and push Docker images only (no GitHub release)
  release     Full release: binaries + Docker + GitHub release

Options:
  -v, --version VERSION   Specify version (e.g., v1.2.0)
  -n, --notes FILE        Release notes markdown file (required for 'release')
  --dry-run               Preview without executing
  --skip-frontend         Skip frontend build (use existing dist/)
  --clean                 Clean build directories first
  -y, --yes               Skip confirmation prompts
  -h, --help              Show this help

Examples:
  # Quick Docker push for testing
  ./release.sh push -v v0.1.8-dev

  # Full official release with release notes
  ./release.sh release -v v1.0.0 -n releases/v1.0.0.md

  # Preview what would happen
  ./release.sh release -v v1.0.0 -n releases/v1.0.0.md --dry-run

  # Just build binaries locally
  ./release.sh build --clean

Release Notes:
  For official releases, create a markdown file with your release notes:

    releases/v1.0.0.md:
    ## What's New
    - Feature X

    ## Bug Fixes
    - Fixed issue #123
EOF
}

# =============================================================================
# Argument Parsing
# =============================================================================

parse_args() {
    COMMAND="${1:-help}"
    shift || true

    while [[ $# -gt 0 ]]; do
        case $1 in
            -v|--version)
                VERSION="$2"
                shift 2
                ;;
            -n|--notes)
                NOTES_FILE="$2"
                shift 2
                ;;
            --dry-run)
                DRY_RUN=true
                shift
                ;;
            --skip-frontend)
                SKIP_FRONTEND=true
                shift
                ;;
            --clean)
                CLEAN_FIRST=true
                shift
                ;;
            -y|--yes)
                CONFIRM=false
                shift
                ;;
            -h|--help)
                show_help
                exit 0
                ;;
            *)
                die "Unknown option: $1"
                ;;
        esac
    done
}

main() {
    parse_args "$@"

    # Check we're in the right directory
    if [ ! -f "main.go" ]; then
        die "This script must be run from the project root directory"
    fi

    case "$COMMAND" in
        build)   cmd_build ;;
        push)    cmd_push ;;
        release) cmd_release ;;
        help|-h|--help) show_help ;;
        *)       die "Unknown command: $COMMAND (use --help for usage)" ;;
    esac
}

main "$@"
