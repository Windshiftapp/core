# Windshift Demo

Self-contained Docker deployment with pre-seeded demo data and automatic TLS via Caddy.

## Quick Start

```bash
cd frontend/e2e/demo

# Local testing (self-signed certificate)
./setup-demo.sh localhost

# Production deployment with Let's Encrypt
./setup-demo.sh demo.example.com
```

**Note:** The setup script automatically handles the Docker build context. If running docker compose manually, run from the project root:

```bash
# From project root
HOSTNAME=localhost docker compose -f frontend/e2e/demo/docker-compose.yml up --build
```

## What's Included

The demo comes pre-loaded with:

- **3 Workspaces**: Software Development, Customer Support, Marketing
- **Multiple Projects** across workspaces
- **Hierarchical Work Items**: Epics, Stories, Tasks with realistic data
- **Custom Fields and Screens**
- **Test Management**: Test cases, test sets, and test runs
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

## Customization

### Using a Different Port
Edit `docker-compose.yml` to change the host port mappings:
```yaml
ports:
  - "8080:80"    # Map host 8080 to container 80
  - "8443:443"   # Map host 8443 to container 443
```

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

## Troubleshooting

### Certificate Issues
If Let's Encrypt fails to provision a certificate:
1. Verify DNS points to your server: `dig +short your-domain.com`
2. Check port 80 is accessible: `curl -I http://your-domain.com`
3. View Caddy logs: `docker compose logs demo | grep -i cert`

### Demo Data Not Loading
1. Check seeder stage in build: `docker compose build --no-cache`
2. View startup logs: `docker compose logs demo`

### Browser Security Warning (localhost)
This is expected when using `localhost`. Caddy generates a self-signed certificate for local development. Click "Advanced" and proceed to the site.
