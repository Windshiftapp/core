#!/bin/bash
set -euo pipefail

# =============================================================================
# Windshift Release Script
# =============================================================================

# Configuration
GHCR_REGISTRY="ghcr.io/windshiftapp/windshift"
GITHUB_REPO="Windshiftapp/windshift"
DOCKER_PLATFORMS="linux/amd64,linux/arm64"

# Build configurations: GOOS/GOARCH
PLATFORMS=(
    "linux/amd64"
    "linux/arm64"
    "windows/amd64"
    "darwin/amd64"
    "darwin/arm64"
)

# State variables
VERSION=""
NOTES_FILE=""
DRY_RUN=false
SKIP_FRONTEND=false
SKIP_DESKTOP=false
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

    log_success "Dependencies OK"
}

# Tools required only for build_desktop_mac. Called lazily by that step so a
# Linux release host without tauri-cli can still run --skip-desktop.
check_desktop_dependencies() {
    command -v jq >/dev/null 2>&1 \
        || die "jq required for desktop build (used to patch tauri.conf.json). Install with: brew install jq"
    cargo tauri --version >/dev/null 2>&1 \
        || die "cargo tauri not found. Install with: cargo install tauri-cli --version '^2.0' --locked"
    # rustup is only sometimes present (Homebrew rust installs don't ship it).
    # When available, verify the arm64 darwin target is installed; otherwise
    # trust that the rustc on PATH can target it and let `cargo tauri build`
    # surface a clear error if not.
    if command -v rustup >/dev/null 2>&1; then
        rustup target list --installed 2>/dev/null | grep -q '^aarch64-apple-darwin$' \
            || die "Rust target missing. Install with: rustup target add aarch64-apple-darwin"
    fi
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
    # Refresh the index so stat-only differences (touched timestamps from
    # builds, editor saves, etc.) don't get flagged as uncommitted work.
    git update-index --refresh >/dev/null 2>&1 || true

    if [ -n "$(git status --porcelain)" ]; then
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

    log_step "1/9" "Building frontend..."

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

    local output_path="dist/binaries/windshift-${goos}-${goarch}"
    [ "$goos" = "windows" ] && output_path="${output_path}.exe"

    log_info "  Building for ${goos}/${goarch}..."

    if [ "$DRY_RUN" = true ]; then
        log_info "  [DRY-RUN] Would build: $output_path"
        return 0
    fi

    export CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch"

    if go build -ldflags "-s -w" -o "$output_path" .; then
        local size=$(ls -lh "$output_path" | awk '{print $5}')
        log_success "  Built: $output_path ($size)"
    else
        log_error "  Failed to build for ${goos}/${goarch}"
        return 1
    fi
}

build_binaries() {
    log_step "2/9" "Building server binaries..."

    dry_run_or_exec mkdir -p dist/binaries

    for platform in "${PLATFORMS[@]}"; do
        IFS="/" read -r goos goarch <<< "$platform"
        build_binary "$goos" "$goarch" || true
    done

    log_success "Server binary builds complete"
}

build_ws_binary() {
    local goos="$1"
    local goarch="$2"

    local output_path="dist/binaries/ws-${goos}-${goarch}"
    [ "$goos" = "windows" ] && output_path="${output_path}.exe"

    log_info "  Building ws for ${goos}/${goarch}..."

    if [ "$DRY_RUN" = true ]; then
        log_info "  [DRY-RUN] Would build: $output_path"
        return 0
    fi

    export CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch"

    local version_clean="${VERSION#v}"
    local git_commit=$(git rev-parse --short HEAD)
    local build_date=$(date -u +'%Y-%m-%dT%H:%M:%SZ')

    if go build -ldflags "-s -w -X main.version=${version_clean} -X main.commit=${git_commit} -X main.date=${build_date}" -o "$output_path" ./cmd/ws; then
        local size=$(ls -lh "$output_path" | awk '{print $5}')
        log_success "  Built: $output_path ($size)"
    else
        log_error "  Failed to build ws for ${goos}/${goarch}"
        return 1
    fi
}

build_ws_binaries() {
    log_step "3/9" "Building ws CLI binaries..."

    dry_run_or_exec mkdir -p dist/binaries

    for platform in "${PLATFORMS[@]}"; do
        IFS="/" read -r goos goarch <<< "$platform"
        build_ws_binary "$goos" "$goarch" || true
    done

    log_success "ws CLI binary builds complete"
}

create_release_packages() {
    log_step "4/9" "Creating server release packages..."

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

        # Copy server binary
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
}

create_ws_release_packages() {
    log_step "5/9" "Creating ws CLI release packages..."

    dry_run_or_exec mkdir -p dist/releases

    if [ "$DRY_RUN" = true ]; then
        log_info "[DRY-RUN] Would create ws release packages for all built ws binaries"
        return 0
    fi

    for binary in dist/binaries/ws-*; do
        [ -f "$binary" ] || continue

        local basename=$(basename "$binary")
        local platform="${basename#ws-}"
        platform="${platform%.exe}"

        local package_name="ws-${VERSION}-${platform}"
        local package_dir="dist/releases/${package_name}"

        mkdir -p "$package_dir"

        # Copy ws binary
        if [[ "$platform" == *windows* ]]; then
            cp "$binary" "$package_dir/ws.exe"
        else
            cp "$binary" "$package_dir/ws"
        fi

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
}

# Build the macOS desktop wrapper as a signed (if env vars set) arm64 DMG.
# Reuses the darwin/arm64 server + ws binaries already produced by
# build_binaries / build_ws_binaries — modernc.org/sqlite is pure-Go, so the
# CGO_ENABLED=0 binaries work fine as Tauri sidecars.
#
# Signing/notarization is opt-in via environment:
#   APPLE_SIGNING_IDENTITY   — Developer ID Application cert name (keychain)
#   APPLE_ID / APPLE_PASSWORD / APPLE_TEAM_ID  — for notarytool submission
# When any of these are missing the build still produces an unsigned DMG and
# warns the user.
build_desktop_mac() {
    log_step "6/9" "Building macOS desktop app (arm64)..."

    if [ "$SKIP_DESKTOP" = true ]; then
        log_info "Skipping desktop build (--skip-desktop)"
        return 0
    fi
    if [ "$(uname)" != "Darwin" ]; then
        log_info "Skipping desktop build (host is not macOS)"
        return 0
    fi

    check_desktop_dependencies

    # Surface the signing posture so a silent unsigned build doesn't surprise anyone.
    # Logged before the dry-run guard so dry-run reflects the actual outcome.
    if [ -z "${APPLE_SIGNING_IDENTITY:-}" ]; then
        log_warn "APPLE_SIGNING_IDENTITY not set — DMG will be UNSIGNED."
        log_warn "  Users will see \"App is damaged\" on first open; they'll need to right-click → Open."
    elif [ -z "${APPLE_ID:-}" ] || [ -z "${APPLE_PASSWORD:-}" ] || [ -z "${APPLE_TEAM_ID:-}" ]; then
        log_warn "APPLE_ID / APPLE_PASSWORD / APPLE_TEAM_ID not all set — DMG will be SIGNED but NOT notarized."
    else
        log_info "Signing identity: $APPLE_SIGNING_IDENTITY (will notarize via notarytool)"
    fi

    if [ "$DRY_RUN" = true ]; then
        log_info "[DRY-RUN] Would copy dist/binaries/{windshift,ws}-darwin-arm64 into desktop sidecars"
        log_info "[DRY-RUN] Would patch tauri.conf.json version to ${VERSION#v}"
        log_info "[DRY-RUN] Would run: cargo tauri build --target aarch64-apple-darwin"
        log_info "[DRY-RUN] Would copy DMG to dist/releases/Windshift-${VERSION}-macos-arm64.dmg"
        return 0
    fi

    # Inputs are produced by earlier steps; bail loudly if they vanished.
    local server_bin="dist/binaries/windshift-darwin-arm64"
    local ws_bin="dist/binaries/ws-darwin-arm64"
    [ -f "$server_bin" ] || die "Missing $server_bin — server build must run first"
    [ -f "$ws_bin" ]     || die "Missing $ws_bin — ws build must run first"

    local desktop_dir
    desktop_dir="$(cd .. && pwd)/desktop"
    [ -d "$desktop_dir" ] || die "Cannot find ../desktop (expected sibling of core/)"

    # Stage sidecars with Tauri's expected triple-suffixed names.
    mkdir -p "$desktop_dir/src-tauri/binaries"
    cp "$server_bin" "$desktop_dir/src-tauri/binaries/windshift-aarch64-apple-darwin"
    cp "$ws_bin"     "$desktop_dir/src-tauri/binaries/ws-aarch64-apple-darwin"

    # Patch tauri.conf.json with the release version. Install an EXIT trap
    # FIRST (not after the copy) so a Ctrl-C between cp and jq still restores
    # the original file. The trap is unset on success.
    local conf="$desktop_dir/src-tauri/tauri.conf.json"
    local backup="$conf.release-backup"
    cp "$conf" "$backup"
    trap 'if [ -f "$backup" ]; then mv -f "$backup" "$conf"; fi' EXIT
    jq --arg v "${VERSION#v}" '.version = $v' "$conf" > "$conf.new" && mv "$conf.new" "$conf"

    (cd "$desktop_dir" && cargo tauri build --target aarch64-apple-darwin)

    # Restore tauri.conf.json before the gh release step touches the working tree.
    mv -f "$backup" "$conf"
    trap - EXIT

    local v="${VERSION#v}"
    local src_dmg="$desktop_dir/src-tauri/target/aarch64-apple-darwin/release/bundle/dmg/Windshift_${v}_aarch64.dmg"
    [ -f "$src_dmg" ] || die "Expected DMG not produced: $src_dmg"

    mkdir -p dist/releases
    local dst_dmg="dist/releases/Windshift-${VERSION}-macos-arm64.dmg"
    cp "$src_dmg" "$dst_dmg"
    local size=$(ls -lh "$dst_dmg" | awk '{print $5}')
    log_success "Created $(basename "$dst_dmg") ($size)"
}

# Generate SHA256SUMS.txt over everything in dist/releases. Called after the
# desktop DMG is in place so the checksum file covers it too.
generate_checksums() {
    log_step "7/9" "Generating checksums..."

    if [ "$DRY_RUN" = true ]; then
        log_info "[DRY-RUN] Would generate SHA256SUMS.txt"
        return 0
    fi

    if ls dist/releases/*.tar.gz dist/releases/*.zip dist/releases/*.dmg 2>/dev/null | head -1 >/dev/null; then
        (cd dist/releases && sha256sum *.tar.gz *.zip *.dmg 2>/dev/null > SHA256SUMS.txt || true)
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
    log_step "8/9" "Building Docker images..."

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
    log_step "9/9" "Creating GitHub release..."

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
    for file in dist/releases/*.tar.gz dist/releases/*.zip dist/releases/*.dmg dist/releases/SHA256SUMS.txt; do
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

    rm -rf dist/

    build_frontend
    build_binaries
    build_ws_binaries
    create_release_packages
    create_ws_release_packages
    build_desktop_mac
    generate_checksums

    echo ""
    log_success "Build complete! Artifacts in dist/"
    echo ""
    echo "Release packages:"
    ls -1 dist/releases/*.tar.gz dist/releases/*.zip dist/releases/*.dmg 2>/dev/null | sed 's/^/  /' || echo "  (none)"
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

    rm -rf dist/

    build_frontend
    build_docker

    echo ""
    log_success "Push complete!"
    echo ""
    echo "Docker image: ${GHCR_REGISTRY}:${VERSION}"
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
        echo "  - Build server binaries for multiple platforms"
        echo "  - Build ws CLI binaries for multiple platforms"
        echo "  - Create release packages with checksums"
        if [ "$SKIP_DESKTOP" != true ] && [ "$(uname)" = "Darwin" ]; then
            echo "  - Build macOS desktop DMG (arm64)"
        fi
        echo "  - Build and push Docker image"
        echo "  - Create git tag and push"
        echo "  - Create GitHub release with assets"
        echo ""
        echo "Release notes: $NOTES_FILE"
        echo ""
        read -p "Continue? [y/N] " -n 1 -r
        echo
        [[ $REPLY =~ ^[Yy]$ ]] || exit 1
    fi

    rm -rf dist/

    build_frontend
    build_binaries
    build_ws_binaries
    create_release_packages
    create_ws_release_packages
    build_desktop_mac
    generate_checksums
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
  --skip-desktop          Skip macOS desktop app build (auto-skipped on non-Mac hosts)
  -y, --yes               Skip confirmation prompts
  -h, --help              Show this help

Desktop signing (optional, only consulted when running on macOS):
  APPLE_SIGNING_IDENTITY  Developer ID Application cert name in your keychain
  APPLE_ID                Apple ID email (for notarization)
  APPLE_PASSWORD          App-specific password (for notarization)
  APPLE_TEAM_ID           Apple Developer team ID (for notarization)
  When unset, the DMG is produced unsigned and unnotarized — Gatekeeper will
  block double-click on download, users must right-click → Open.

Examples:
  # Quick Docker push for testing
  ./release.sh push -v v0.1.8-dev

  # Full official release with release notes
  ./release.sh release -v v1.0.0 -n releases/v1.0.0.md

  # Preview what would happen
  ./release.sh release -v v1.0.0 -n releases/v1.0.0.md --dry-run

  # Just build binaries locally
  ./release.sh build

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
            --skip-desktop)
                SKIP_DESKTOP=true
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
