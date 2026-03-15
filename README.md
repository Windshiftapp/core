<p align="center">
  <img src="frontend/public/windshift-3.svg" alt="Windshift" width="120" height="120">
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

## Getting started

Download the Windshift binaries from https://windshift.sh/download - you can find the quick start guide [here](https://windshift.sh/docs/01-getting-started/02-quick-start).

## Help wanted

If you would like to contribute to this project, we are looking for help in the following areas:

#### Early bug reports 
Let us know if you encounter any bug or uncertainties about a feature via Github Issues

#### OIDC Providers
If you can connect Windshift to an OIDC Provider, let us know how it goes via Discussion. Both positive and negative feedback helps us here. We have tested OIDC with PocketID from our side.


## Tech Stack

- **Backend**: Go 1.25+
- **Frontend**: Svelte 5, Vite, Tailwind CSS
- **Database**: SQLite (default) or PostgreSQL
- **Authentication**: Sessions, JWT, WebAuthn, OIDC

## Quick Start

### Prerequisites

- Go 1.25.7 or later
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

**Server**

| Variable / Flag | Description | Default |
|---|---|---|
| `PORT` / `-port`, `-p` | HTTP server port | `8080` |
| `BASE_URL` | Public URL (used for email links, SSO redirects) | - |
| `LOG_LEVEL` / `-log-level` | `debug`, `info`, `warn`, `error` | `info` |
| `LOG_FORMAT` / `-log-format` | `text`, `json`, `logfmt` | `text` |
| `USE_PROXY` / `-use-proxy` | Trust `X-Forwarded-Proto` from private IPs | `false` |
| `ADDITIONAL_PROXIES` / `-additional-proxies` | Extra trusted proxy IPs (requires proxy mode) | - |
| `-allowed-hosts` | Comma-separated hostnames for CSRF | - |
| `-allowed-port` | Port for CORS/WebAuthn origins (useful behind reverse proxy) | server port |
| `-no-csrf` | Disable CSRF protection (development only) | `false` |
| `-enable-fallback` | Admin password fallback for restrictive auth policies | `false` |

**Database**

| Variable / Flag | Description | Default |
|---|---|---|
| `DB_TYPE` | `sqlite` or `postgres` | `sqlite` |
| `DB_PATH` / `-db` | SQLite database file path | `windshift.db` |
| `POSTGRES_CONNECTION_STRING` / `-pg-conn` | Full PostgreSQL connection string | - |
| `POSTGRES_HOST` | PostgreSQL hostname (when `DB_TYPE=postgres`) | `postgres` |
| `POSTGRES_PORT` | PostgreSQL port | `5432` |
| `POSTGRES_USER` | PostgreSQL username | `windshift` |
| `POSTGRES_PASSWORD` | PostgreSQL password | - |
| `POSTGRES_DB` | PostgreSQL database name | `windshift` |
| `MAX_READ_CONNS` / `-max-read-conns` | Max read DB connections | `120` |
| `MAX_WRITE_CONNS` / `-max-write-conns` | Max write DB connections | `1` |

**Storage & TLS**

| Variable / Flag | Description | Default |
|---|---|---|
| `ATTACHMENT_PATH` / `-attachment-path` | Directory for file attachments | - |
| `-tls-cert` | Path to TLS certificate (enables HTTPS) | - |
| `-tls-key` | Path to TLS key (enables HTTPS) | - |

**Authentication**

| Variable / Flag | Description | Default |
|---|---|---|
| `SSO_SECRET` | Secret for SSO and credential encryption | - |
| `SESSION_SECRET` | Session encryption secret (falls back to `SSO_SECRET`) | - |
| `WEBAUTHN_RP_ID` | WebAuthn Relying Party ID | auto-detected |
| `WEBAUTHN_RP_NAME` | WebAuthn Relying Party display name | `Windshift` |

**SSH Server**

| Variable / Flag | Description | Default |
|---|---|---|
| `SSH_ENABLED` / `-ssh` | Enable SSH TUI server | `false` |
| `SSH_PORT` / `-ssh-port` | SSH server port | `23234` |
| `SSH_HOST` / `-ssh-host` | SSH server host | `localhost` |
| `-ssh-key` | Path to SSH host key file | `.ssh/windshift_host_key` |

**Services & Plugins**

| Variable / Flag | Description | Default |
|---|---|---|
| `LLM_ENDPOINT` | LLM inference service URL | - |
| `LOGBOOK_ENDPOINT` | Logbook sidecar service URL | - |
| `LLM_PROVIDERS_FILE` / `-llm-providers` | Custom LLM providers JSON file | - |
| `DISABLE_PLUGINS` / `-disable-plugins` | Disable the plugin system | `false` |
| `PLUGIN_DIRS` | Additional plugin directories (comma-separated) | - |

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
