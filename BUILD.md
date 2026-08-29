# Building Windshift

This document explains how to build Windshift for multiple platforms.

## Quick Build

### Using the Makefile (Recommended)

```bash
# Build everything (frontend + server)
make all

# Clean and rebuild
make clean && make all

# Build server only
make build

# Build frontend only
make frontend

# Show all available make targets
make help
```

### Manual Build

```bash
# Build frontend
cd frontend
npm ci
npm run build
cd ..

# Build server (current platform)
go build -o windshift main.go

# Build ws client (current platform)
cd cmd/ws
go build -o ws
cd ../..
```

## Build Output

### Development Builds (Makefile)

The Makefile produces binaries in the project root:

```
windshift           # Main server binary (current platform)
windshift_unix      # Linux binary (from make build-linux)
windshift.exe       # Windows binary (from make build-windows)
windshift-windows-arm64.exe  # Windows on ARM64 binary (from make build-windows-arm64)
```

### Release Builds (release.sh)

The release script creates a `dist/` structure:

```
dist/
├── binaries/
│   ├── windshift-linux-amd64
│   ├── windshift-linux-arm64
│   ├── windshift-windows-amd64.exe
│   ├── windshift-windows-arm64.exe
│   ├── windshift-darwin-amd64
│   └── windshift-darwin-arm64
└── releases/
    ├── windshift-v1.0.0-linux-amd64.tar.gz
    ├── windshift-v1.0.0-linux-arm64.tar.gz
    ├── windshift-v1.0.0-windows-amd64.zip
    ├── windshift-v1.0.0-windows-arm64.zip
    ├── windshift-v1.0.0-darwin-amd64.tar.gz
    ├── windshift-v1.0.0-darwin-arm64.tar.gz
    └── SHA256SUMS.txt
```

## Cross-Platform/Release Builds

### Using the Makefile

```bash
# Cross-compile for Linux
make build-linux

# Cross-compile for Windows
make build-windows

# Cross-compile for Windows on ARM64
make build-windows-arm64
```

### Using release.sh (Full Release)

```bash
# Full release with binaries + Docker + GitHub release
./release.sh release -v v1.0.0 -n releases/v1.0.0.md

# Build binaries and packages locally (no publish)
./release.sh build -v v1.0.0

# Build and push Docker images only
./release.sh push -v v1.0.0-dev

# Dry run to preview actions
./release.sh release -v v1.0.0 -n releases/v1.0.0.md --dry-run

# Skip frontend build (use existing dist/)
./release.sh build --skip-frontend

# Show help
./release.sh --help
```

## Supported Platforms

| Platform | Server | WS Client |
|----------|--------|-----------|
| Linux (x64) | ✅ | ✅ |
| Linux (ARM64) | ✅ | ✅ |
| Windows (x64) | ✅ | ✅ |
| macOS (Intel) | ✅ | ✅ |
| macOS (Apple Silicon) | ✅ | ✅ |

## Build Requirements

### For Server + Frontend
- **Go 1.27.0** - Exact version declared in `go.mod`
- **Node.js 24.18.0** - Exact version pinned in `.nvmrc` and `frontend/package.json`
- **npm 11.16.0** - Exact version pinned in `frontend/package.json`

### For WS Client Only
- **Go 1.27.0** - Exact version declared in `go.mod`

### For Cross-Compilation (release.sh)
- **Docker + Buildx** - Multi-arch Docker images

## Usage Examples

### Linux
```bash
# After building
./windshift &
./cmd/ws/ws workspace list
```

### Windows
```bash
# After building
windshift.exe
cmd\ws\ws.exe workspace list
```

### macOS
```bash
# After building
./windshift &
./cmd/ws/ws workspace list
```

## Build Optimization

Production builds use these Go build flags:

- `-ldflags "-s -w"` - Strip debug information and reduce binary size
- Version information embedded at build time

## Troubleshooting

### Frontend Build Issues
```bash
# Reinstall exactly from the lockfile
cd frontend
rm -rf node_modules
npm cache verify
npm ci
npm run build
```

### Go Build Issues
```bash
# Update dependencies
go mod download
go mod tidy

# Clean Go module cache
go clean -modcache
```

### Platform-Specific Issues

**Windows**: If you don't have `zip` command available, Windows archives won't be created but binaries will still build.

**Linux**: Ensure you have sufficient disk space. Cross-compilation creates multiple large binaries.

## Makefile Targets

Run `make help` to see all available targets:

**Production builds:**
- `make build` - Build production binary (excludes test code)
- `make build-linux` - Cross-compile for Linux
- `make build-windows` - Cross-compile for Windows
- `make build-windows-arm64` - Cross-compile for Windows on ARM64
- `make release` - Full production release build

**Development builds:**
- `make dev-build` - Development binary
- `make dev` - Full development cycle

**Testing:**
- Test sources and runners live in the adjacent private `core-tests`
  repository. Use `../core-tests/overlay.sh .` for the Go suites.

**Utilities:**
- `make frontend` - Build frontend only
- `make clean` - Clean build artifacts
- `make deps` - Update dependencies
- `make verify-size` - Compare binary sizes with/without tests
