# Building Windshift

This document explains how to build Windshift for multiple platforms.

## Quick Build

### Build Everything (Recommended)
```bash
# Build both server and ws client for all platforms
./build-all.sh

# Clean and build everything
./build-all.sh --clean

# Build only the server
./build-all.sh --server-only

# Build only the ws client
./build-all.sh --client-only

# Get help
./build-all.sh --help
```

### Manual Build
```bash
# Build frontend
cd frontend
npm install
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

The build script creates the following structure:

```
dist/
├── server/
│   ├── windshift-linux-amd64
│   ├── windshift-linux-arm64
│   ├── windshift-windows-amd64.exe
│   ├── windshift-darwin-amd64
│   └── windshift-darwin-arm64
└── client/
    ├── ws-linux-amd64
    ├── ws-linux-arm64
    ├── ws-windows-amd64.exe
    ├── ws-darwin-amd64
    └── ws-darwin-arm64
```

## Supported Platforms

| Platform | Server | WS Client |
|----------|--------|-----------|
| Linux (x64) | ✅ | ✅ |
| Linux (ARM64) | ✅ | ✅ |
| Windows (x64) | ✅ | ✅ |
| macOS (Intel) | ✅ | ✅ |
| macOS (Apple Silicon) | ✅ | ✅ |

## Cross-Compilation

Go makes cross-compilation easy. You can build for any platform from any platform:

```bash
# Build server for Linux from macOS/Windows
GOOS=linux GOARCH=amd64 go build -o windshift-linux main.go

# Build ws client for Windows from Linux/macOS
cd cmd/ws
GOOS=windows GOARCH=amd64 go build -o ws-windows.exe
cd ../..

# Build for ARM64 (Apple Silicon, ARM Linux)
GOOS=darwin GOARCH=arm64 go build -o windshift-darwin-arm64 main.go
GOOS=linux GOARCH=arm64 go build -o windshift-linux-arm64 main.go
```

## Build Requirements

### For Server + Frontend
- **Go 1.21+** - Backend compilation
- **Node.js 18+** - Frontend build
- **npm** - Package management

### For WS Client Only
- **Go 1.21+** - Client compilation only

## Usage Examples

### Linux
```bash
# Extract and run
tar -xzf windshift-linux-amd64.tar.gz
./dist/server/windshift-linux-amd64 &
./dist/client/ws-linux-amd64 workspace list
```

### Windows
```bash
# Extract and run
dist\server\windshift-windows-amd64.exe
dist\client\ws-windows-amd64.exe workspace list
```

### macOS
```bash
# Extract and run
./dist/server/windshift-darwin-arm64 &
./dist/client/ws-darwin-arm64 workspace list
```

## Build Optimization

The build script uses these Go build flags for production:

- `-ldflags "-s -w"` - Strip debug information and reduce binary size
- Version information embedded at build time
- Static linking for standalone binaries

## Troubleshooting

### Frontend Build Issues
```bash
# Clear npm cache and reinstall
cd frontend
rm -rf node_modules package-lock.json  
npm install
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

**macOS**: If you get permission errors, make sure the build script is executable:
```bash
chmod +x build-all.sh
```

**Linux**: Ensure you have sufficient disk space. Cross-compilation creates multiple large binaries.

## Build Script Features

- 🎯 **Smart Platform Detection** - Automatically builds for all supported platforms
- 🧹 **Clean Builds** - Optional cleanup of previous builds  
- 📦 **Modular Building** - Build server-only or client-only
- 🎨 **Colorized Output** - Clear progress indication
- 📊 **Build Summary** - Shows file sizes and locations
- 🔧 **Error Handling** - Stops on any build failure
- 📋 **Documentation** - Generates usage examples

The build script is production-ready and handles edge cases like missing dependencies, wrong directories, and cross-platform differences.