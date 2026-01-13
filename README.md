<p align="center">
  <img src="frontend/public/cmicon-2.svg" alt="Windshift" width="120" height="120">
</p>

<h1 align="center">Windshift</h1>

<p align="center">
  A self-hosted work management platform for teams
</p>

---

## Overview

Windshift is a comprehensive work management platform that combines task tracking, workflow automation, and team collaboration in a single self-hosted application. Built with Go and Svelte, it offers enterprise-grade features while remaining easy to deploy and maintain.

## Features

**Work Management**
- Tasks and projects with custom fields and workflows
- Configurable statuses, priorities, and item types
- Rich text descriptions with mentions and attachments
- Recurring tasks with flexible scheduling

**Collaboration**
- Comments with activity tracking
- Multi-channel notifications (email, webhooks)
- Customer portal for external submissions
- Team workspaces with role-based access

**Integrations**
- SSO/OIDC authentication (Keycloak, Authentik, etc.)
- WebAuthn/FIDO2 passwordless login
- SCM integration (GitHub, GitLab, Gitea, Bitbucket)
- SCIM 2.0 user provisioning
- Jira project import

**Additional Modules**
- Test management (cases, runs, results)
- Time tracking with project billing
- Asset management
- Collections and saved searches

## Tech Stack

- **Backend**: Go 1.24+
- **Frontend**: Svelte 5, Vite, Tailwind CSS
- **Database**: SQLite (default) or PostgreSQL
- **Authentication**: Sessions, JWT, WebAuthn, OIDC

## Quick Start

### Prerequisites

- Go 1.24.5 or later
- Node.js 18 or later
- npm

### Build

```bash
# Build frontend
cd frontend
npm install
npm run build
cd ..

# Build backend
go build -o windshift main.go

# Run
./windshift --port 8080
```

### Using Make

```bash
make build    # Production build
make dev      # Development build
make test     # Run tests
```

### Docker

```bash
docker build -t windshift .
docker run -p 8080:8080 windshift
```

## Configuration

Set configuration via environment variables or command-line flags.

| Variable | Description | Default |
|----------|-------------|---------|
| `PORT` | HTTP server port | 8080 |
| `BASE_URL` | Public URL for the application | - |
| `DB_TYPE` | Database type (`sqlite` or `postgres`) | sqlite |
| `DB_PATH` | SQLite database file path | windshift.db |
| `LOG_LEVEL` | Log level (debug, info, warn, error) | info |
| `ATTACHMENT_PATH` | Directory for file attachments | - |

For PostgreSQL:
```bash
POSTGRES_CONNECTION_STRING=postgresql://user:pass@host:5432/windshift
```

See `.env.example` for all available options.

## Project Structure

```
.
├── main.go              # Application entry point
├── Makefile             # Build targets
├── Dockerfile           # Container build
├── frontend/            # Svelte frontend application
│   ├── src/
│   └── dist/            # Built assets (embedded in binary)
├── internal/            # Go internal packages
│   ├── handlers/        # HTTP request handlers
│   ├── routes/          # Route registration
│   ├── models/          # Data models
│   ├── services/        # Business logic
│   └── database/        # Database layer
└── tests/               # Integration tests
```

## Documentation

- [BUILD.md](BUILD.md) - Build instructions
- [LOGGING.md](LOGGING.md) - Logging configuration
- [TEST.md](TEST.md) - Testing guide

## License

See LICENSE file for details.
