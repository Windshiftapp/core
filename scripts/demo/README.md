# Windshift Demo

Self-contained Docker deployment with pre-seeded demo data and automatic TLS via Caddy.

## Quick Start

```bash
cd scripts/demo

# Local testing (self-signed certificate)
./setup-demo.sh localhost

# Production deployment with Let's Encrypt
./setup-demo.sh demo.example.com
```

**Note:** The setup script automatically handles the Docker build context. If running docker compose manually, run from the project root:

```bash
# From project root (core/)
HOSTNAME=localhost docker compose -f scripts/demo/docker-compose.yml up --build
```

## What's Included

The demo comes pre-loaded with:

- **3 Workspaces**: Software Development, Customer Support, Marketing
- **Multiple Projects** across workspaces
- **Hierarchical Work Items**: Epics, Stories, Tasks with realistic data
- **Custom Fields and Screens**
- **Test Management**: Test cases, test plans, and test runs
- **Time Tracking**: Projects, customers, and work logs

## Login Credentials

| User  | Password | Role              |
|-------|----------|-------------------|
| admin | admin    | Administrator     |
| john  | john     | Developer         |
| jane  | jane     | Support Agent     |
| mike  | mike     | Marketing Manager |

## Ports

| Port | Purpose |
|------|---------|
| 80   | HTTP (Let's Encrypt ACME challenge) |
| 443  | HTTPS (main access) |
| 8443 | HTTPS (alternative, for restricted firewalls) |

## Requirements

### For Local Testing
- Docker and Docker Compose
- Browser will show certificate warning (expected for localhost)

### For Production (Let's Encrypt)
1. **DNS**: A record pointing your hostname to the server
2. **Port 80**: Must be accessible from internet (ACME HTTP-01 challenge)
3. **Ports 443/8443**: At least one must be open for HTTPS access

## Commands

```bash
# View logs
docker compose logs -f

# Stop the demo
docker compose down

# Restart
docker compose restart

# Rebuild (after code changes)
docker compose up --build -d

# Complete reset (removes all data including certificates)
docker compose down -v
```

## Architecture

```
[Internet] --> [Caddy :80/:443/:8443] --> [Windshift :8080]
                     |
                     v
               [Let's Encrypt]
```

- **Caddy**: Reverse proxy with automatic TLS certificate provisioning
- **Windshift**: Application server with SQLite database
- **Data**: Pre-seeded demo database, persisted in Docker volume

---

# Demo Content Generator

For development and testing, you can run the demo content generator directly without Docker.

> **Note**: This tool is configured via environment variables `APP_NAME` (default: WINDSHIFT) and `BINARY_NAME` (default: windshift) for easy rebranding.

## Prerequisites

1. **Node.js** (v22 or higher; matches `frontend/package.json`)
2. **Go** toolchain

`generate-demo.js` builds the frontend first, then builds the Go backend so the binary embeds the current `frontend/dist/index.html`.

## Quick Start

```bash
cd scripts/demo
node generate-demo.js
```

This will:
1. Build the frontend (`npm run build`)
2. Build the backend binary with the freshly embedded frontend
3. Create a new `demo.db` database
4. Start server on port 8080
5. Complete initial setup
6. Generate all demo content
7. Keep the server running

## Usage Options

```bash
# Custom port
node generate-demo.js --port 3000

# Custom database
node generate-demo.js --db my-demo.db

# Use existing server (for CI/testing)
node generate-demo.js --no-server --base-url http://localhost:8080

# Stop server after generation
node generate-demo.js --stop-server

# Custom binary output path (the script builds this binary before running it)
node generate-demo.js --binary /path/to/windshift

# Use an already-built binary without rebuilding
node generate-demo.js --no-build --binary /path/to/windshift

# Help
node generate-demo.js --help
```

## Generated Content

### Workspaces

1. **Software Development (SOFT)**
   - Projects: Mobile App Rewrite, API v2 Development
   - Work items: Epics with stories and tasks
   - Custom fields: Story Points, Estimated Hours, Environment, Browser

2. **Customer Support (SUPP)**
   - Projects: Customer Onboarding, Technical Support
   - Work items: Support categories with tickets and sub-tasks
   - Custom fields: Customer Tier, Request Type, Severity

3. **Marketing (MKTG)**
   - Project: Q1 2025 Campaign
   - Work items: Campaigns with tasks
   - Custom fields: Campaign Type, Due Date, Release Date

---

## Customization

### Environment Variables
- `HOSTNAME`: Domain name for TLS certificate (default: localhost)
- `WINDSHIFT_PORT`: Internal port for windshift (default: 8080)

### Persistent Data
Demo data is stored in Docker volumes:
- `demo-data`: Windshift database and attachments
- `caddy-data`: Let's Encrypt certificates
- `caddy-config`: Caddy configuration

To reset demo data but keep certificates:
```bash
docker compose down
docker volume rm windshift-demo_demo-data
docker compose up -d
```

### Modify Demo Content

Edit `demo-data.js` to customize workspaces, users, and work items.

---

## Troubleshooting

### Certificate Issues (Production)
1. Verify DNS points to your server: `dig +short your-domain.com`
2. Check port 80 is accessible: `curl -I http://your-domain.com`
3. View Caddy logs: `docker compose logs demo | grep -i cert`

### Browser Security Warning (localhost)
This is expected. Caddy generates a self-signed certificate for local development. Click "Advanced" and proceed.

### Binary Not Found
By default the generator builds the binary before starting the server. If you passed `--no-build`, either remove it or build manually after building the frontend:
```bash
cd frontend && npm run build
cd .. && go build -tags '!test' -ldflags '-s -w' -o windshift .
```

### Server Startup Timeout
- Check if port is already in use
- Verify database permissions
- Try a different port: `--port 3000`

### Database Locked
Ensure no other server instances are running:
```bash
rm demo.db demo.db-shm demo.db-wal
```

## File Structure

```
scripts/demo/
├── setup-demo.sh       # Quick start script for Docker deployment
├── build.sh            # Manual Docker image build script
├── docker-compose.yml  # Docker Compose configuration
├── Dockerfile          # Multi-stage Docker build
├── Caddyfile.template  # Caddy reverse proxy config
├── entrypoint.sh       # Container entrypoint
├── generate-demo.js    # Demo content generator
├── demo-data.js        # Demo data definitions
└── README.md           # This file
```
