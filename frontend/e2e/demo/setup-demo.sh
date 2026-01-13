#!/bin/bash
#
# Windshift Demo Setup Script
#
# Starts a self-contained demo instance with:
# - Pre-seeded demo data (workspaces, items, users)
# - Caddy reverse proxy with automatic Let's Encrypt TLS
#
# Usage:
#   ./setup-demo.sh <hostname>
#
# Examples:
#   ./setup-demo.sh demo.example.com    # Production with Let's Encrypt
#   ./setup-demo.sh localhost           # Local testing (self-signed cert)
#

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

print_banner() {
    echo -e "${BLUE}"
    echo "================================"
    echo "  Windshift Demo Setup"
    echo "================================"
    echo -e "${NC}"
}

print_success() {
    echo -e "${GREEN}$1${NC}"
}

print_warning() {
    echo -e "${YELLOW}$1${NC}"
}

print_error() {
    echo -e "${RED}$1${NC}"
}

# Check if hostname is provided
if [ -z "$1" ]; then
    print_banner
    echo "Usage: $0 <hostname>"
    echo ""
    echo "Examples:"
    echo "  $0 demo.example.com    # Production deployment with Let's Encrypt"
    echo "  $0 localhost           # Local testing"
    echo ""
    echo "Requirements for Let's Encrypt (production):"
    echo "  - DNS A record pointing to this server"
    echo "  - Port 80 accessible from the internet (ACME challenge)"
    echo "  - Ports 443 and/or 8443 for HTTPS access"
    exit 1
fi

HOSTNAME="$1"
export HOSTNAME

print_banner

echo "Configuration:"
echo "  Hostname: ${HOSTNAME}"
echo ""

# Check Docker
if ! command -v docker &> /dev/null; then
    print_error "Error: Docker is not installed or not in PATH"
    exit 1
fi

if ! docker info &> /dev/null; then
    print_error "Error: Docker is not running"
    exit 1
fi

# Check docker compose
if ! docker compose version &> /dev/null; then
    print_error "Error: Docker Compose is not available"
    exit 1
fi

# Requirements info
if [ "$HOSTNAME" != "localhost" ]; then
    echo -e "${YELLOW}Requirements for Let's Encrypt:${NC}"
    echo "  1. DNS A record for ${HOSTNAME} pointing to this server"
    echo "  2. Port 80 accessible from internet (ACME HTTP challenge)"
    echo "  3. Ports 443 and/or 8443 open for HTTPS traffic"
    echo ""
    read -p "Continue? [y/N] " -n 1 -r
    echo ""
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Aborted."
        exit 1
    fi
fi

echo ""
echo "Building and starting demo..."
echo ""

# Get script directory and project root
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"

# Build from project root to ensure correct context
cd "$PROJECT_ROOT"

# Build and start using the demo docker-compose file
docker compose -f frontend/e2e/demo/docker-compose.yml up --build -d

echo ""
print_success "Demo is starting!"
echo ""
echo "URLs:"
echo "  Main:        https://${HOSTNAME}"
echo "  Alternative: https://${HOSTNAME}:8443"
echo ""
echo "Login credentials:"
echo "  Username: admin"
echo "  Password: admin"
echo ""
echo "Demo users:"
echo "  john / john   (Developer)"
echo "  jane / jane   (Support Agent)"
echo "  mike / mike   (Marketing Manager)"
echo ""
echo "Commands:"
echo "  View logs:    docker compose logs -f"
echo "  Stop demo:    docker compose down"
echo "  Restart:      docker compose restart"
echo ""

if [ "$HOSTNAME" = "localhost" ]; then
    print_warning "Note: Using 'localhost' will generate a self-signed certificate."
    print_warning "Your browser will show a security warning - this is normal for local testing."
fi
