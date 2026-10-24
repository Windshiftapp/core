#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# Source local .env if present (for APPLE_SIGNING_IDENTITY, APPLE_PASSWORD_OP_REF, etc.)
for env_file in "$SCRIPT_DIR/.env" "$SCRIPT_DIR/../desktop/.env"; do
    if [ -f "$env_file" ]; then
        set -a; source "$env_file"; set +a
    fi
done

# =============================================================================
# Windshift Release Script
# =============================================================================

# Configuration
GHCR_REGISTRY="ghcr.io/windshiftapp/windshift"
WS_CARRIER_GHCR_REGISTRY="ghcr.io/windshiftapp/ws-carrier"
AGENT_GHCR_REGISTRY="ghcr.io/windshiftapp/windshift-agent"
RUNNER_GHCR_REGISTRY="ghcr.io/windshiftapp/windshift-runner"
GITHUB_REPO="Windshiftapp/windshift"
DOCKER_PLATFORMS="linux/amd64,linux/arm64"
# The thin no-node windshift-agent lives in a sibling repo; its image is built
# from that checkout, lifting `ws` from the ws-carrier image this
# release just built. Override the path if your layout differs; the build is
# skipped (with a warning) when the checkout is absent.
WINDSHIFT_AGENT_DIR="${WINDSHIFT_AGENT_DIR:-../windshift-agent}"

# Build configurations: GOOS/GOARCH
PLATFORMS=(
    "linux/amd64"
    "linux/arm64"
    "windows/amd64"
    "windows/arm64"
    "darwin/amd64"
    "darwin/arm64"
)

# State variables
VERSION=""
RELEASE_NAME=""
NOTES_FILE=""
DRY_RUN=false
SKIP_FRONTEND=false
SKIP_DESKTOP=false
CONFIRM=true
TAG_CREATED=false
SKIP_SECURITY_CHECKS=false
# Set by cmd_release: official releases must not ship an unsigned/un-notarized
# DMG, so missing signing config becomes a hard failure instead of a warning.
REQUIRE_SIGNED_DMG=false

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

ensure_node_runtime() {
    local version_file="$SCRIPT_DIR/.nvmrc"
    [ -f "$version_file" ] || return 0

    local required_version current_version
    required_version=$(tr -d '[:space:]' < "$version_file")
    [ -n "$required_version" ] || die "Node version file is empty: $version_file"

    current_version=""
    if command -v node >/dev/null 2>&1; then
        current_version=$(node --version 2>/dev/null || true)
        current_version="${current_version#v}"
    fi

    if [ "$current_version" = "$required_version" ]; then
        return 0
    fi

    if ! command -v mise >/dev/null 2>&1; then
        die "Node $required_version is required (active: ${current_version:-unavailable}). Install mise or activate the version from .nvmrc."
    fi

    # Prepend the pinned install ourselves. `mise exec` resolves the direct
    # command but leaves its node path behind earlier entries such as
    # Homebrew's, so every later PATH lookup would still pick the wrong node.
    local node_bin
    node_bin="$(mise where "node@$required_version" 2>/dev/null)/bin"
    if [ ! -x "$node_bin/node" ]; then
        die "mise could not provide Node $required_version (active: ${current_version:-unavailable}). Run 'mise install node@$required_version'."
    fi

    log_info "Switching Node from ${current_version:-unavailable} to $required_version via mise..."
    export PATH="$node_bin:$PATH"
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
        log_info "[DRY-RUN] Would create signed git tag: $tag"
        log_info "[DRY-RUN] Would push tag to remote"
        return 0
    fi

    if [ -n "${RELEASE_GPG_KEY:-}" ]; then
        git -c gpg.format=openpgp -c user.signingkey="$RELEASE_GPG_KEY" \
            tag -s "$tag" -m "Release $tag"
    else
        local ssh_key
        ssh_key=$(release_ssh_signing_key) ||
            die "Release signing requires RELEASE_GPG_KEY or RELEASE_SSH_KEY (or an SSH user.signingkey in Git config)"
        git -c gpg.format=ssh -c user.signingkey="$ssh_key" \
            tag -s "$tag" -m "Release $tag"
    fi
    log_success "Created signed git tag: $tag"
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

# =============================================================================
# Preflight credential checks
#
# Credential failures used to surface only at the step that consumed them: an
# expired 1Password session after a full multi-platform build, or a GHCR push
# denial after every Docker layer had already been built. Both cost a complete
# rebuild. Everything credential-shaped is therefore verified — and, for
# 1Password, refreshed — before the first build step runs.
# =============================================================================

# The DMG build is the only consumer of the 1Password-backed APPLE_PASSWORD.
desktop_build_is_active() {
    [ "$SKIP_DESKTOP" != true ] && [ "$(uname)" = "Darwin" ]
}

preflight_1password() {
    # $1: whether this command builds the DMG at all (push does not).
    [ "$1" = true ] || return 0
    desktop_build_is_active || return 0

    # Only the op-backed path needs a session: an inline APPLE_PASSWORD, or a
    # config without APPLE_PASSWORD_OP_REF, never touches 1Password.
    if [ -n "${APPLE_PASSWORD:-}" ] || [ -z "${APPLE_PASSWORD_OP_REF:-}" ]; then
        return 0
    fi

    command -v op >/dev/null 2>&1 \
        || die "APPLE_PASSWORD_OP_REF is set but 1Password CLI (op) is not installed."

    if ! op whoami >/dev/null 2>&1; then
        log_warn "1Password session expired or not signed in."
        [ -t 0 ] || die "No TTY available to run 'op signin'. Sign in first, or export APPLE_PASSWORD directly."

        log_info "Running 'op signin'..."
        local signin_env
        signin_env=$(op signin) \
            || die "1Password sign-in failed — run 'op signin' manually and retry."
        eval "$signin_env"
    fi

    # Resolve the secret now instead of at notarization time, so a wrong or
    # renamed item ID also fails here rather than after the build.
    APPLE_PASSWORD=$(op item get "$APPLE_PASSWORD_OP_REF" --fields label=password --reveal) \
        || die "Failed to read APPLE_PASSWORD from 1Password (item: $APPLE_PASSWORD_OP_REF)."
    [ -n "$APPLE_PASSWORD" ] \
        || die "1Password returned an empty password for item: $APPLE_PASSWORD_OP_REF"
    export APPLE_PASSWORD

    log_success "1Password session OK (APPLE_PASSWORD resolved)"
}

# Emits the ghcr.io username and secret on two lines, straight from the store
# `docker push` itself reads, so the probe tests the real credential rather
# than assuming it was seeded from `gh auth token`.
read_ghcr_credential() {
    local config="${DOCKER_CONFIG:-$HOME/.docker}/config.json"
    [ -f "$config" ] || return 1

    local helper
    helper=$(jq -r '.credHelpers["ghcr.io"] // .credsStore // empty' "$config") || return 1

    if [ -n "$helper" ]; then
        command -v "docker-credential-$helper" >/dev/null 2>&1 || return 1
        local out
        out=$(echo "ghcr.io" | "docker-credential-$helper" get 2>/dev/null) || return 1
        jq -r '.Username // empty, .Secret // empty' <<<"$out"
        return 0
    fi

    # No helper: credentials sit inline as base64("user:token").
    local b64 decoded
    b64=$(jq -r '.auths["ghcr.io"].auth // empty' "$config") || return 1
    [ -n "$b64" ] || return 1
    decoded=$(printf '%s' "$b64" | base64 -d 2>/dev/null) || return 1
    printf '%s\n%s\n' "${decoded%%:*}" "${decoded#*:}"
}

# 0 = push allowed, 1 = authenticated but not authorized, 2 = indeterminate.
# Opens a blob upload session and immediately cancels it; nothing is published.
ghcr_push_probe() {
    local repo="$1" user="$2" secret="$3"

    local token
    token=$(curl -s -u "$user:$secret" \
        "https://ghcr.io/token?service=ghcr.io&scope=repository:${repo}:pull,push" \
        | jq -r '.token // empty') || return 2
    [ -n "$token" ] || return 2

    local headers location
    headers=$(curl -s -D - -o /dev/null -X POST \
        -H "Authorization: Bearer $token" \
        "https://ghcr.io/v2/${repo}/blobs/uploads/") || return 2

    location=$(printf '%s' "$headers" | grep -i '^location:' | tr -d '\r' | cut -d' ' -f2-)
    [ -n "$location" ] || return 1

    # Release the session we just opened. GHCR expires abandoned uploads anyway,
    # so a failure here must not fail the release.
    curl -s -o /dev/null -X DELETE -H "Authorization: Bearer $token" "$location" || true
    return 0
}

# Fallback when the registry cannot be probed: inspect the gh token's scopes.
# Only conclusive when the Docker login was seeded from `gh auth token`, so
# this warns rather than aborts.
preflight_ghcr_scope_fallback() {
    if ! command -v gh >/dev/null 2>&1; then
        log_warn "Cannot verify GHCR push access (need jq, curl or gh) — continuing."
        return 0
    fi

    if gh auth status 2>&1 | grep -q "write:packages"; then
        log_success "GitHub token lists the write:packages scope"
        return 0
    fi

    log_warn "GitHub token does not list the 'write:packages' scope."
    log_warn "  If the ghcr.io Docker login was seeded from 'gh auth token', the push WILL fail."
    log_warn "  Fix: gh auth refresh -h github.com -s write:packages"
    log_warn "       gh auth token | docker login ghcr.io -u <github-username> --password-stdin"
}

preflight_ghcr() {
    local repo="${GHCR_REGISTRY#ghcr.io/}"

    if ! command -v jq >/dev/null 2>&1 || ! command -v curl >/dev/null 2>&1; then
        preflight_ghcr_scope_fallback
        return 0
    fi

    local creds
    if ! creds=$(read_ghcr_credential); then
        die "No ghcr.io credential found in Docker's credential store. Log in with:
    gh auth refresh -h github.com -s write:packages
    gh auth token | docker login ghcr.io -u <github-username> --password-stdin"
    fi

    local user secret
    { read -r user; read -r secret; } <<<"$creds"
    if [ -z "$user" ] || [ -z "$secret" ]; then
        preflight_ghcr_scope_fallback
        return 0
    fi

    local rc=0
    ghcr_push_probe "$repo" "$user" "$secret" || rc=$?
    case "$rc" in
        0) log_success "GHCR push access OK (as $user)" ;;
        1) die "GHCR denied push to ${GHCR_REGISTRY} for user '$user'.
   The credential is valid but is missing the 'write:packages' scope. Fix with:
     gh auth refresh -h github.com -s write:packages
     gh auth token | docker login ghcr.io -u $user --password-stdin" ;;
        *) log_warn "Could not reach the GHCR token API to verify push access — continuing." ;;
    esac
}

# need_ghcr:    whether this command pushes Docker images.
# need_desktop: whether this command builds (and notarizes) the DMG.
preflight_auth() {
    local need_ghcr="$1" need_desktop="$2"

    log_step "0/9" "Preflight: verifying credentials..."
    preflight_1password "$need_desktop"
    if [ "$need_ghcr" = true ]; then
        preflight_ghcr
    fi
    log_success "Preflight checks passed"
}

run_frontend_supply_chain_checks() {
    if [ "$SKIP_SECURITY_CHECKS" = true ]; then
        log_warn "Skipping frontend supply-chain checks (--skip-security-checks)"
        return 0
    fi

    log_info "Verifying npm package signatures..."
    npm audit signatures --min-release-age=0

    # Audit production dependencies only. Dev-only tools (vite, vitest, ...)
    # never ship in the release artifact, so their advisories must not block it.
    log_info "Running npm audit (high severity and above, production deps only)..."
    npm audit --audit-level=high --omit=dev
}

run_go_vulnerability_check() {
    if [ "$SKIP_SECURITY_CHECKS" = true ]; then
        log_warn "Skipping Go vulnerability check (--skip-security-checks)"
        return 0
    fi

    if ! command -v govulncheck >/dev/null 2>&1; then
        log_warn "govulncheck not found; skipping Go vulnerability check. Install a pinned version of golang.org/x/vuln/cmd/govulncheck before official releases."
        return 0
    fi

    log_info "Running govulncheck..."
    govulncheck ./...
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
        log_info "[DRY-RUN] Would run: cd frontend && npm ci && npm run build"
        return 0
    fi

    # Inject version metadata at build time via Vite env vars. version.js
    # prefers these over the version.json fallback (which only ships "dev"
    # values for local `npm run dev`).
    (
        cd frontend
        export VITE_APP_VERSION_CODE="$VERSION"
        export VITE_APP_VERSION_NAME="$RELEASE_NAME"
        npm ci
        run_frontend_supply_chain_checks
        npm run build
    )

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

    local version_clean="${VERSION#v}"
    local git_commit=$(git rev-parse --short HEAD)
    local build_date=$(date -u +'%Y-%m-%dT%H:%M:%SZ')
    local pkg="windshift/internal/version"
    local ldflags="-s -w -X ${pkg}.Version=${version_clean} -X ${pkg}.Commit=${git_commit} -X ${pkg}.Date=${build_date} -X '${pkg}.ReleaseName=${RELEASE_NAME}'"

    if go build -ldflags "$ldflags" -o "$output_path" .; then
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

    local failed_platforms=()
    for platform in "${PLATFORMS[@]}"; do
        IFS="/" read -r goos goarch <<< "$platform"
        build_binary "$goos" "$goarch" || failed_platforms+=("$platform")
    done

    if [ ${#failed_platforms[@]} -gt 0 ]; then
        for platform in "${failed_platforms[@]}"; do
            log_error "Server binary build failed for ${platform}"
        done
        die "Server binary builds failed for ${#failed_platforms[@]} platform(s)"
    fi

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

    local failed_platforms=()
    for platform in "${PLATFORMS[@]}"; do
        IFS="/" read -r goos goarch <<< "$platform"
        build_ws_binary "$goos" "$goarch" || failed_platforms+=("$platform")
    done

    if [ ${#failed_platforms[@]} -gt 0 ]; then
        for platform in "${failed_platforms[@]}"; do
            log_error "ws CLI binary build failed for ${platform}"
        done
        die "ws CLI binary builds failed for ${#failed_platforms[@]} platform(s)"
    fi

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
DB_PATH=windshift.db

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

    # Resolve Apple app-specific password from 1Password if configured.
    # APPLE_PASSWORD_OP_REF is the 1Password item ID. preflight_1password
    # normally resolves and exports this before any build runs; this stays as a
    # fallback for callers that reach this step without preflight.
    if [ -z "${APPLE_PASSWORD:-}" ] && [ -n "${APPLE_PASSWORD_OP_REF:-}" ]; then
        if ! command -v op >/dev/null 2>&1; then
            die "APPLE_PASSWORD_OP_REF is set but 1Password CLI (op) is not installed."
        fi
        APPLE_PASSWORD=$(op item get "$APPLE_PASSWORD_OP_REF" --fields label=password --reveal) || {
            die "Failed to read APPLE_PASSWORD from 1Password (item: $APPLE_PASSWORD_OP_REF). Is 'op' signed in?"
        }
        export APPLE_PASSWORD
    fi

    # Surface the signing posture so a silent unsigned build doesn't surprise anyone.
    # Checked before the dry-run guard so dry-run reflects the actual outcome.
    # For 'release' (REQUIRE_SIGNED_DMG) an unsigned or un-notarized DMG is a
    # hard failure — use --skip-desktop to release without a DMG instead.
    if [ -z "${APPLE_SIGNING_IDENTITY:-}" ]; then
        if [ "$REQUIRE_SIGNED_DMG" = true ]; then
            die "APPLE_SIGNING_IDENTITY not set — refusing to publish an UNSIGNED DMG in an official release. Set the signing env vars or pass --skip-desktop."
        fi
        log_warn "APPLE_SIGNING_IDENTITY not set — DMG will be UNSIGNED."
        log_warn "  Users will see \"App is damaged\" on first open; they'll need to right-click → Open."
    elif [ -z "${APPLE_ID:-}" ] || [ -z "${APPLE_PASSWORD:-}" ] || [ -z "${APPLE_TEAM_ID:-}" ]; then
        if [ "$REQUIRE_SIGNED_DMG" = true ]; then
            die "APPLE_ID / APPLE_PASSWORD / APPLE_TEAM_ID not all set — refusing to publish a non-notarized DMG in an official release. Set the notarization env vars or pass --skip-desktop."
        fi
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

    if ls dist/releases/*.tar.gz dist/releases/*.zip dist/releases/*.dmg dist/releases/PROVENANCE.txt 2>/dev/null | head -1 >/dev/null; then
        if command -v sha256sum >/dev/null 2>&1; then
            (cd dist/releases && sha256sum *.tar.gz *.zip *.dmg PROVENANCE.txt 2>/dev/null > SHA256SUMS.txt || true)
        else
            (cd dist/releases && shasum -a 256 *.tar.gz *.zip *.dmg PROVENANCE.txt 2>/dev/null > SHA256SUMS.txt || true)
        fi
        log_success "Generated SHA256SUMS.txt"
    fi
}

write_release_provenance() {
    log_step "7b/9" "Writing release provenance..."

    if [ "$DRY_RUN" = true ]; then
        log_info "[DRY-RUN] Would write dist/releases/PROVENANCE.txt"
        return 0
    fi

    mkdir -p dist/releases

    local provenance="dist/releases/PROVENANCE.txt"
    local git_commit git_tree git_branch build_date
    git_commit=$(git rev-parse HEAD)
    git_tree=$(git rev-parse HEAD^{tree})
    git_branch=$(git branch --show-current)
    build_date=$(date -u +'%Y-%m-%dT%H:%M:%SZ')

    {
        echo "Windshift release provenance"
        echo "============================"
        echo "version: ${VERSION}"
        echo "release_name: ${RELEASE_NAME}"
        echo "build_date_utc: ${build_date}"
        echo "builder_host: $(hostname)"
        echo "builder_os: $(uname -a)"
        echo "git_commit: ${git_commit}"
        echo "git_tree: ${git_tree}"
        echo "git_branch: ${git_branch}"
        echo "git_status: clean"
        echo ""
        echo "Tool versions"
        echo "-------------"
        echo "go: $(go version 2>/dev/null || echo unavailable)"
        echo "node: $(node --version 2>/dev/null || echo unavailable)"
        echo "npm: $(npm --version 2>/dev/null || echo unavailable)"
        echo "docker: $(docker --version 2>/dev/null || echo unavailable)"
        echo "docker_buildx: $(docker buildx version 2>/dev/null || echo unavailable)"
        echo "gh: $(gh --version 2>/dev/null | head -1 || echo unavailable)"
        echo "govulncheck: $(govulncheck -version 2>/dev/null | head -1 || echo unavailable)"
        echo ""
        echo "npm security config"
        echo "-------------------"
        (cd frontend && npm config get min-release-age 2>/dev/null | sed 's/^/min-release-age: /') || true
        (cd frontend && npm config get ignore-scripts 2>/dev/null | sed 's/^/ignore-scripts: /') || true
        (cd frontend && npm config get engine-strict 2>/dev/null | sed 's/^/engine-strict: /') || true
        echo ""
        echo "Docker base image references"
        echo "----------------------------"
        echo "Dockerfile:"
        grep '^FROM ' Dockerfile || true
        echo ""
        echo "deploy/coding-agent/Dockerfile:"
        grep '^FROM ' deploy/coding-agent/Dockerfile || true
        if command -v docker >/dev/null 2>&1; then
            echo ""
            echo "Resolved base image digests at build time"
            echo "-----------------------------------------"
            for image in node:24.18.0-alpine golang:1.27.3-alpine golang:1.27.3-bookworm node:lts-slim; do
                docker buildx imagetools inspect "$image" 2>/dev/null | grep -E 'Name:|Digest:' || true
            done
        fi
    } > "$provenance"

    log_success "Wrote PROVENANCE.txt"
}

release_ssh_signing_key() {
    if [ -n "${RELEASE_SSH_KEY:-}" ]; then
        printf '%s\n' "$RELEASE_SSH_KEY"
        return 0
    fi

    [ "$(git config --get gpg.format 2>/dev/null || true)" = "ssh" ] || return 1
    git config --get user.signingkey 2>/dev/null
}

release_signing_identity() {
    if [ -n "${RELEASE_SIGNING_IDENTITY:-}" ]; then
        printf '%s\n' "$RELEASE_SIGNING_IDENTITY"
        return 0
    fi

    git config --get user.email 2>/dev/null
}

preflight_release_signing() {
    if [ "$DRY_RUN" = true ]; then
        log_info "[DRY-RUN] Would verify the release signing key"
        return 0
    fi

    if [ -n "${RELEASE_GPG_KEY:-}" ]; then
        command -v gpg >/dev/null 2>&1 ||
            die "gpg is required when RELEASE_GPG_KEY is set"
        gpg --batch --list-secret-keys "$RELEASE_GPG_KEY" >/dev/null 2>&1 ||
            die "No GPG secret key found for RELEASE_GPG_KEY=$RELEASE_GPG_KEY"
        log_success "GPG release signing key is available"
        return 0
    fi

    local ssh_key identity
    ssh_key=$(release_ssh_signing_key) ||
        die "Release signing requires RELEASE_GPG_KEY or RELEASE_SSH_KEY (or an SSH user.signingkey in Git config)"
    identity=$(release_signing_identity) ||
        die "SSH release signing requires RELEASE_SIGNING_IDENTITY or user.email in Git config"
    [ -n "$identity" ] ||
        die "SSH release signing requires RELEASE_SIGNING_IDENTITY or user.email in Git config"
    [ -f "$ssh_key" ] ||
        die "SSH release signing key not found: $ssh_key"
    command -v ssh-keygen >/dev/null 2>&1 ||
        die "ssh-keygen is required for SSH release signing"
    log_success "SSH release signing key is available"
}

sign_release_checksums() {
    if [ "$DRY_RUN" = true ]; then
        log_info "[DRY-RUN] Would sign and verify SHA256SUMS.txt"
        return 0
    fi

    local checksums="dist/releases/SHA256SUMS.txt"
    [ -s "$checksums" ] || die "Missing or empty $checksums"

    if [ -n "${RELEASE_GPG_KEY:-}" ]; then
        local gpg_signature="${checksums}.asc"
        gpg --batch --yes --armor --detach-sign --local-user "$RELEASE_GPG_KEY" \
            -o "$gpg_signature" "$checksums" ||
            die "Failed to sign SHA256SUMS.txt with GPG"
        [ -s "$gpg_signature" ] || die "GPG did not create $gpg_signature"
        gpg --batch --verify "$gpg_signature" "$checksums" >/dev/null 2>&1 ||
            die "Generated GPG release signature could not be verified"
        log_success "Signed and verified SHA256SUMS.txt -> SHA256SUMS.txt.asc"
        return 0
    fi

    local ssh_key identity ssh_signature public_key allowed_signers
    ssh_key=$(release_ssh_signing_key) ||
        die "Release signing requires RELEASE_GPG_KEY or RELEASE_SSH_KEY (or an SSH user.signingkey in Git config)"
    identity=$(release_signing_identity) ||
        die "SSH release signing requires RELEASE_SIGNING_IDENTITY or user.email in Git config"
    ssh_signature="${checksums}.sig"

    ssh-keygen -Y sign -f "$ssh_key" -n windshift-release "$checksums" >/dev/null ||
        die "Failed to sign SHA256SUMS.txt with SSH"
    [ -s "$ssh_signature" ] || die "SSH signing did not create $ssh_signature"

    if [[ "$ssh_key" == *.pub ]]; then
        public_key=$(awk 'NR == 1 { print $1 " " $2 }' "$ssh_key")
    else
        public_key=$(ssh-keygen -y -f "$ssh_key") ||
            die "Could not derive the public key for release signature verification"
    fi
    [ -n "$public_key" ] || die "Could not derive the public release signing key"

    allowed_signers="dist/releases/.allowed_signers.tmp"
    printf '%s %s\n' "$identity" "$public_key" > "$allowed_signers"
    if ! ssh-keygen -Y verify \
        -f "$allowed_signers" \
        -I "$identity" \
        -n windshift-release \
        -s "$ssh_signature" \
        < "$checksums" >/dev/null 2>&1; then
        rm -f "$allowed_signers"
        die "Generated SSH release signature could not be verified"
    fi
    rm -f "$allowed_signers"
    log_success "Signed and verified SHA256SUMS.txt -> SHA256SUMS.txt.sig"
}

ensure_buildx() {
    if ! docker buildx inspect windshift-builder &>/dev/null; then
        log_info "Creating buildx builder..."
        dry_run_or_exec docker buildx create --name windshift-builder --use
    else
        dry_run_or_exec docker buildx use windshift-builder
    fi
}

build_docker_image() {
    local image="$1"
    local dockerfile="$2"
    local label="$3"
    local include_version_args="$4"

    local tags=("-t" "${image}:${VERSION}")

    # Only tag as latest for official releases (not dev/test versions)
    if [[ ! "$VERSION" =~ -dev|-test|-rc ]]; then
        tags+=("-t" "${image}:latest")
    fi

    log_info "Building ${label}: ${image}:${VERSION}"
    log_info "  Dockerfile: ${dockerfile}"

    if [ "$DRY_RUN" = true ]; then
        log_info "[DRY-RUN] Would build and push ${label} Docker image (${image}:${VERSION})"
        return 0
    fi

    local args=(
        "buildx" "build"
        "--platform" "$DOCKER_PLATFORMS"
        "-f" "$dockerfile"
    )

    if [ "$include_version_args" = true ]; then
        local git_commit=$(git rev-parse --short HEAD)
        local build_date=$(date -u +'%Y-%m-%dT%H:%M:%SZ')
        args+=(
            "--build-arg" "VERSION=${VERSION}"
            "--build-arg" "RELEASE_NAME=${RELEASE_NAME}"
            "--build-arg" "COMMIT=${git_commit}"
            "--build-arg" "BUILD_DATE=${build_date}"
        )
    fi

    docker "${args[@]}" "${tags[@]}" --push .

    log_success "${label} Docker image pushed to ${image}"
}

# build_agent_image builds the thin no-node windshift-agent image from the
# sibling repo (WINDSHIFT_AGENT_DIR), lifting `ws` from the ws-carrier
# image this release just built (so the agent and runner ship matched). Skips
# with a warning when the checkout is absent so a server-only release still
# completes.
build_agent_image() {
    local image="$AGENT_GHCR_REGISTRY"
    local ws_image="${WS_CARRIER_GHCR_REGISTRY}:${VERSION}"
    local ctx="$WINDSHIFT_AGENT_DIR"

    if [ ! -d "$ctx" ]; then
        log_warn "windshift-agent checkout not found at ${ctx}; skipping agent image (set WINDSHIFT_AGENT_DIR)"
        return 0
    fi

    local tags=("-t" "${image}:${VERSION}")
    if [[ ! "$VERSION" =~ -dev|-test|-rc ]]; then
        tags+=("-t" "${image}:latest")
    fi

    log_info "Building windshift-agent: ${image}:${VERSION}"
    log_info "  Context: ${ctx}  (WS_IMAGE=${ws_image})"

    if [ "$DRY_RUN" = true ]; then
        log_info "[DRY-RUN] Would build and push windshift-agent image (${image}:${VERSION}) from ${ctx}"
        return 0
    fi

    docker buildx build \
        --platform "$DOCKER_PLATFORMS" \
        --build-arg "WS_IMAGE=${ws_image}" \
        "${tags[@]}" --push "$ctx"

    log_success "windshift-agent Docker image pushed to ${image}"
}

build_docker() {
    log_step "8/9" "Building Docker images..."

    check_docker
    ensure_buildx

    log_info "Platforms: ${DOCKER_PLATFORMS}"
    log_info "Server tags: ${GHCR_REGISTRY}:${VERSION}"
    log_info "ws-carrier tags: ${WS_CARRIER_GHCR_REGISTRY}:${VERSION}"
    log_info "Runner tags: ${RUNNER_GHCR_REGISTRY}:${VERSION}"
    log_info "Agent tags: ${AGENT_GHCR_REGISTRY}:${VERSION}"

    build_docker_image "$GHCR_REGISTRY" "Dockerfile" "Windshift server" true
    build_docker_image "$WS_CARRIER_GHCR_REGISTRY" "deploy/coding-agent/Dockerfile" "ws-carrier (WS_IMAGE for windshift-agent)" false
    build_docker_image "$RUNNER_GHCR_REGISTRY" "deploy/windshift-runner/Dockerfile" "windshift-runner" false
    # Built last: it lifts ws from the ws-carrier image pushed above.
    build_agent_image
}

create_github_release() {
    log_step "9/9" "Creating GitHub release..."

    check_gh_cli

    if [ "$DRY_RUN" = false ]; then
        local signature_found=false
        for signature in dist/releases/SHA256SUMS.txt.asc dist/releases/SHA256SUMS.txt.sig; do
            if [ -s "$signature" ]; then
                signature_found=true
                break
            fi
        done
        [ "$signature_found" = true ] ||
            die "Refusing to publish a release without a SHA256SUMS.txt signature"
    fi

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
    for file in dist/releases/*.tar.gz dist/releases/*.zip dist/releases/*.dmg dist/releases/SHA256SUMS.txt dist/releases/SHA256SUMS.txt.asc dist/releases/SHA256SUMS.txt.sig dist/releases/PROVENANCE.txt; do
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
    preflight_auth false true

    rm -rf dist/

    run_go_vulnerability_check
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
        echo "  - Build and push Docker images to ${GHCR_REGISTRY}, ${WS_CARRIER_GHCR_REGISTRY}, ${RUNNER_GHCR_REGISTRY}, and ${AGENT_GHCR_REGISTRY}"
        echo ""
        echo "Note: This does NOT create a GitHub release."
        echo ""
        read -p "Continue? [y/N] " -n 1 -r
        echo
        [[ $REPLY =~ ^[Yy]$ ]] || exit 1
    fi

    preflight_auth true false

    rm -rf dist/

    run_go_vulnerability_check
    build_frontend
    build_docker

    echo ""
    log_success "Push complete!"
    echo ""
    echo "Docker images:"
    echo "  ${GHCR_REGISTRY}:${VERSION}"
    echo "  ${WS_CARRIER_GHCR_REGISTRY}:${VERSION}"
    echo "  ${RUNNER_GHCR_REGISTRY}:${VERSION}"
    echo "  ${AGENT_GHCR_REGISTRY}:${VERSION}"
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

    REQUIRE_SIGNED_DMG=true
    preflight_release_signing

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
        echo "  - Build and push Docker images (server + coding-agent runner + windshift-runner + windshift-agent)"
        echo "  - Create git tag and push"
        echo "  - Create GitHub release with assets"
        echo ""
        echo "Release notes: $NOTES_FILE"
        echo ""
        read -p "Continue? [y/N] " -n 1 -r
        echo
        [[ $REPLY =~ ^[Yy]$ ]] || exit 1
    fi

    preflight_auth true true

    rm -rf dist/

    run_go_vulnerability_check
    build_frontend
    build_binaries
    build_ws_binaries
    create_release_packages
    create_ws_release_packages
    build_desktop_mac
    write_release_provenance
    generate_checksums
    sign_release_checksums
    build_docker
    create_github_release

    echo ""
    log_success "Release $VERSION complete!"
    echo ""
    echo "GitHub: https://github.com/${GITHUB_REPO}/releases/tag/${VERSION}"
    echo "Docker:"
    echo "  docker pull ${GHCR_REGISTRY}:${VERSION}"
    echo "  docker pull ${WS_CARRIER_GHCR_REGISTRY}:${VERSION}"
    echo "  docker pull ${RUNNER_GHCR_REGISTRY}:${VERSION}"
    echo "  docker pull ${AGENT_GHCR_REGISTRY}:${VERSION}"
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
  --release-name NAME     Human-readable release name (e.g., "Formation").
                          Injected into the server binary and UI footer.
  --dry-run               Preview without executing
  --skip-frontend         Skip frontend build (use existing dist/)
  --skip-desktop          Skip macOS desktop app build (auto-skipped on non-Mac hosts)
  --skip-security-checks  Skip npm signature/audit and govulncheck release checks
  -y, --yes               Skip confirmation prompts
  -h, --help              Show this help

Every command runs a credential preflight (step 0/9) before the first build:
GHCR push access is probed for 'push' and 'release', and an expired 1Password
session is refreshed via 'op signin' whenever the DMG will be built. Bad
credentials therefore fail in seconds rather than after a full build.

Desktop signing (required for an official macOS release):
  APPLE_SIGNING_IDENTITY  Developer ID Application cert name in your keychain
  APPLE_ID                Apple ID email (for notarization)
  APPLE_PASSWORD          App-specific password (for notarization)
  APPLE_PASSWORD_OP_REF   1Password item ID — alternative to APPLE_PASSWORD
  APPLE_TEAM_ID           Apple Developer team ID (for notarization)

Release signing (required for 'release'):
  RELEASE_GPG_KEY         GPG key id/email used to create SHA256SUMS.txt.asc
  RELEASE_SSH_KEY         SSH private/public key path used to create SHA256SUMS.txt.sig
                          Defaults to Git's SSH user.signingkey when configured.
  RELEASE_SIGNING_IDENTITY
                          SSH signer identity; defaults to Git's user.email.

Desktop behavior:
  For 'build' and 'push': when unset, the DMG is produced unsigned and
  unnotarized — Gatekeeper will block double-click on download, users must
  right-click → Open.
  For 'release': all signing/notarization vars are REQUIRED (the release
  aborts rather than publish an unsigned DMG); pass --skip-desktop to
  release without a DMG.

Examples:
  # Quick Docker push for testing
  ./release.sh push -v v0.1.8-dev

  # Full official release with release notes
  ./release.sh release -v v1.0.0 -n releases/v1.0.0.md --release-name "Formation"

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

Node Runtime:
  The script reads .nvmrc and activates that Node version through mise when a
  different one is active. Install mise if the required version is not already
  available.
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
            --release-name)
                RELEASE_NAME="$2"
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
            --skip-security-checks)
                SKIP_SECURITY_CHECKS=true
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
        build|push|release) ensure_node_runtime ;;
    esac

    case "$COMMAND" in
        build)   cmd_build ;;
        push)    cmd_push ;;
        release) cmd_release ;;
        help|-h|--help) show_help ;;
        *)       die "Unknown command: $COMMAND (use --help for usage)" ;;
    esac
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
    main "$@"
fi
