# Contributing to Windshift

Thanks for your interest in contributing! See the [README](README.md) for a project overview.

> **Note:** Development and contributions happen here on GitHub. Please fork, clone, and open pull requests against this repository.

## Prerequisites

- **Go**: the exact version declared in `go.mod`
- **Node.js** and **npm**: the exact versions pinned by `.nvmrc` and
  `frontend/package.json`
- **Docker** (starts PostgreSQL and other services for local development)

## Development Setup

```bash
# Clone the repo
git clone https://github.com/Windshiftapp/core.git && cd core

# Select the CI Node version and install frontend dependencies
nvm use # or configure mise/asdf from .nvmrc
cd frontend && npm ci && cd ..

# Install the pinned Go analysis tools used by CI
make dev-tools

# Install git hooks
make hooks

# Start PostgreSQL + dev server (SQLite for main app, PostgreSQL for logbook)
./dev.sh
```

The dev server runs on `localhost:7777`.

### Git Hooks

The project includes a pre-commit hook that runs the Go linter and architecture
guards, enforces fresh-schema/upgrade-migration pairing, runs Biome on
non-Svelte files, checks Svelte types, and validates OpenAPI generation. Install
it with:

```bash
make hooks
```

To bypass the hook for a quick commit, use `git commit --no-verify`.

For frontend-only development with hot reload:

```bash
cd frontend && npm run dev
```

To run the design system viewer:

```bash
cd frontend && npm run ds:dev
```

For a standalone development build:

```bash
make dev-build
```

### PostgreSQL integration tests (optional)

Requirements numbering and promote tests can run against a live PostgreSQL
instance. Tests skip automatically when no database is configured or reachable.

```bash
# Start PostgreSQL via ./dev.sh or deploy/docker-compose-main.yml, then:
export POSTGRES_CONNECTION_STRING="postgresql://windshift:YOUR_PASSWORD@localhost:5432/windshift?sslmode=disable"
make test-postgres
```

Use a dedicated test database when possible; tests insert workspace and page
fixtures with unique keys.

## Project Structure

```
.
├── internal/          # Go backend
│   ├── handlers/      # HTTP request handlers
│   ├── models/        # Data models
│   ├── services/      # Business logic
│   ├── repository/    # Data access layer
│   └── database/      # Database setup and migrations
├── frontend/          # Svelte 5 / Vite / Tailwind CSS
├── cmd/ws/            # CLI client
└── .github/workflows/ # CI pipelines
```

## Making Changes

1. Create a feature branch from `main`.
2. Keep commits focused and descriptive.

## Code Style

### Go

The project uses `gofmt`, `goimports`, and `staticcheck`. Lint configuration lives in `.golangci.yml`.

```bash
make lint
```

To reproduce the blocking public-repository workflows locally, including clean
dependency installation, vulnerability/signature checks, and production builds:

```bash
make ci             # both workflows
make ci-go          # Go CI only
make ci-frontend    # frontend CI only
```

### Frontend

The project uses [Biome](https://biomejs.dev/) (config: `frontend/biome.json`).

```bash
cd frontend
npm run lint        # check
npm run format      # auto-format
```
## Submitting a Pull Request

Pull requests are welcome here on [GitHub](https://github.com/Windshiftapp/core). Fork the repository, open a PR against `main`, and describe *what* changed and *why*. Reference related issues if applicable.

1. Push your branch and open a PR against `main`.
2. CI will run automatically:
   - **Go**: lint, vulnerability checks, and production build
   - **Frontend**: lint, type checks, and bundle size check
   - **Private tests**: maintained and run from the adjacent `core-tests`
     repository, which overlays its suites onto a core checkout
   - **PR title lint** and **merge-conflict check**

### AI-assisted contributions

PRs written with the help of AI tools are welcome, with two expectations:

- **Understand the code you submit.** You are responsible for every line in your
  PR. Be ready to explain what the change does, why it is written that way, and
  how you tested it. We may ask follow-up questions during review.
- **Respect the architecture.** Windshift follows a layered design — handlers
  call services, services call repositories, repositories own database access —
  and a layering guard in CI enforces the import boundaries (see
  `scripts/check-layering.sh`). Match the existing layering rather than taking
  shortcuts (for example, handlers must not import `internal/database`, and the
  REST v1 API must go through shared services, not the legacy cookie-auth
  handlers). PRs that break these boundaries will be asked to rework before
  merge.

### Out of scope: Pro features

Windshift ships an open-source core alongside commercial Pro features. **PRs
that attempt to merge Pro features into the open-source core will not be
accepted.** This includes, but is not limited to, SCIM user provisioning and
SAML single sign-on. These features are maintained separately and deliberately
kept out of the core repository.

## Contributor License Agreement

By submitting a pull request you agree to the [CLA](CLA.md). The project is dual-licensed under the **AGPL v3.0** and the **Windshift Commercial License**.

## Code of Conduct

Be respectful, constructive, and collaborative.
