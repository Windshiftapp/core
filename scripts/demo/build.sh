#!/bin/bash
#
# Build the Windshift demo Docker image
#
# Usage:
#   ./build.sh              # Build with default tag 'windshift-demo'
#   ./build.sh my-tag       # Build with custom tag
#

set -e

TAG="${1:-windshift-demo}"

# Get project root (2 levels up from this script)
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

echo "Building windshift-demo image..."
echo "  Project root: $PROJECT_ROOT"
echo "  Tag: $TAG"
echo ""

cd "$PROJECT_ROOT"
docker build -f scripts/demo/Dockerfile -t "$TAG" .

echo ""
echo "Build complete: $TAG"
echo ""
echo "Run with:"
echo "  docker run -p 443:443 -e HOSTNAME=localhost $TAG"
