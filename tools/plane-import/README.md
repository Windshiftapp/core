# plane-import: Plane → Windshift migration tooling

Two pieces, one contract:

| file | runs where | does |
|---|---|---|
| `dump.sql` | against the **Plane** Postgres (psql 10+) | emits the JSONL import contract: meta, users, issues (full sequence range incl. soft-deletes), comments |
| `cmd/plane-import` | against the **Windshift** Postgres | loads the contract into a workspace, preserving item numbers, authors, and timestamps |

## Why psql and not a Go fetcher

Plane's own HTTP export (`/api/workspaces/<slug>/export-issues/`) is
session-auth only — an API key gets 401 — and it is async (job + MinIO
artifact). The database the product itself writes is the faithful, reachable
source, and `COPY ... TO STDOUT` is streaming. Windshift's side stays pure Go.

## Runbook

```bash
# 1. Dump from Plane (project id from projects.id):
psql "$PLANE_DATABASE_URL" -q -v ON_ERROR_STOP=1 \
     -v project_id='8bcd9a90-64b9-4283-9b1f-dfd3755b1ac4' \
     -t -A -f dump.sql > contract.jsonl

# 2. Dry-run against Windshift (no writes, full report):
go run ./cmd/plane-import \
  --connection "$WINDSHIFT_POSTGRES_CONNECTION_STRING" \
  --workspace VECTIS --workspace-slug solum-designum \
  --contract contract.jsonl \
  --alias hakerx1@gmail.com=oskars_germovs@inbox.lv \
  --dry-run

# 3. Import (idempotent — rerun skips everything already mapped):
go run ./cmd/plane-import \
  --connection "$WINDSHIFT_POSTGRES_CONNECTION_STRING" \
  --workspace VECTIS --workspace-slug solum-designum \
  --contract contract.jsonl \
  --alias hakerx1@gmail.com=oskars_germovs@inbox.lv
```

The report (stdout, JSON) states exactly what happened: rows imported,
skipped-existing, soft-deletes held as number gaps, comments, unmapped
authors, unmapped states/priorities, collisions, and the preserved gap list.
`--alias` maps a Plane email to a different Windshift email — without it, an
author whose emails differ across the two systems is reported unmapped and
imports with `creator_id` NULL rather than inventing a user.

## Gates this tooling enforces

- meta must be the first contract line (loader refuses otherwise) — so a
  misordered dump fails loudly instead of importing mislabeled rows;
- a number already owned by a row the importer does not own aborts the run
  (parity is not negotiable);
- a workspace missing one of the mapped statuses aborts before any write;
- soft-deleted source issues import as gaps, not as reused numbers.
