# Contributing to Windshift

Thanks for your interest in contributing! See the [README](README.md) for a project overview.

## Prerequisites

- **Go** 1.25+
- **Node.js** 22+
- **Docker** (starts PostgreSQL and other services for local development)

## Development Setup

```bash
# Clone the repo
git clone <repo-url> && cd core

# Install frontend dependencies
cd frontend && npm install && cd ..

# Start PostgreSQL + dev server (SQLite for main app, PostgreSQL for logbook)
./dev.sh
```

The dev server runs on `localhost:7777`.

For frontend-only development with hot reload:

```bash
cd frontend && npm run dev
```

To run the design system viewer:

```bash
cd frontend && npm run ds:dev
```

For a standalone Go build that includes test utilities:

```bash
make dev-build
```

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
├── tests/             # Integration tests
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

### Frontend

The project uses [Biome](https://biomejs.dev/) (config: `frontend/biome.json`).

```bash
cd frontend
npm run lint        # check
npm run format      # auto-format
```

## Testing

### Go unit tests

```bash
make test           # runs with -tags="test" -race
```

### Go integration tests

Requires a running server (start with `./dev.sh` first):

```bash
make integration-test
```

### Frontend unit tests

```bash
cd frontend && npm test
```

### Frontend E2E (Playwright)

```bash
cd frontend && npx playwright test
```

Production builds exclude test code via build tags (`-tags="!test"`).

## Submitting a Pull Request

1. Push your branch and open a PR against `main`.
2. CI will run automatically:
   - **Go**: lint + unit tests
   - **Frontend**: lint, tests, bundle size check
   - **PR title lint** and **merge-conflict check**
3. Describe *what* changed and *why*. Reference related issues if applicable.

## Contributor License Agreement

By submitting a pull request you agree to the [CLA](CLA.md). The project is dual-licensed under the **AGPL v3.0** and the **Windshift Commercial License**.

## Code of Conduct

Be respectful, constructive, and collaborative.
