# plane-import: Plane → Windshift migration tooling

Two pieces, one contract:

| file | runs where | does |
|---|---|---|
| `dump.sql` | against the **Plane** Postgres (psql 10+) | emits the JSONL import contract: meta, users, issues (full sequence range incl. soft-deletes, each with its `state_group` and `label_ids`), comments, plus the project's full `states` and `labels` vocabulary |
| `cmd/plane-import` | against the **Windshift** Postgres | loads the contract into a workspace: numbers, authors, and timestamps from the source, and the source's state/priority/label vocabulary materialised where the destination lacks it |

## Why psql and not a Go fetcher

Plane's own HTTP export (`/api/workspaces/<slug>/export-issues/`) is
session-auth only — an API key gets 401 — and it is async (job + MinIO
artifact). The database the product itself writes is the faithful, reachable
source, and `COPY ... TO STDOUT` is streaming. Windshift's side stays pure Go.

## Runbook

```bash
# 1. Dump from Plane (project id from projects.id). The optional cap freezes
#    the number range (e.g. a cutover sequence) so post-dump source edits
#    cannot reach the destination mid-migration; omit it for the full corpus:
psql "$PLANE_DATABASE_URL" -q -v ON_ERROR_STOP=1 \
     -v project_id='8bcd9a90-64b9-4283-9b1f-dfd3755b1ac4' \
     -v max_sequence='' \
     -t -A -f dump.sql > contract.jsonl

# 2. Dry-run against Windshift (full report; the vocabulary pass DOES write,
#    then rolls back — schema problems surface here, not only on the real run):
go run ./cmd/plane-import \
  --connection "$WINDSHIFT_POSTGRES_CONNECTION_STRING" \
  --workspace VECTIS --workspace-slug solum-designum \
  --contract contract.jsonl \
  --alias hakerx1@gmail.com=oskars_germovs@inbox.lv \
  --dry-run

# 3. Import (idempotent and converging — see "Reruns" below):
go run ./cmd/plane-import \
  --connection "$WINDSHIFT_POSTGRES_CONNECTION_STRING" \
  --workspace VECTIS --workspace-slug solum-designum \
  --contract contract.jsonl \
  --alias hakerx1@gmail.com=oskars_germovs@inbox.lv
```

The report (stdout, JSON) states exactly what happened: rows imported,
existing, converged, left alone because edited after import, soft-deletes held
as number gaps, comments, unmapped authors, states whose category had to be
guessed, unmapped priorities, everything created (statuses, categories,
priorities, labels), transitions added, label links added, collisions, and the
preserved gap list. `--alias` maps a Plane email to a different Windshift
email — without it, an author whose emails differ across the two systems is
reported unmapped and imports with `creator_id` NULL rather than inventing a
user.

## Vocabulary: the source's, not the destination's shortcut

The loader does NOT collapse Plane's states onto whatever Windshift seeded.
Statuses and categories are mutable global catalogs the product itself edits
(`POST /statuses`, `POST /status-categories`), and importers materialising
vocabulary into them is the in-repo precedent — the Jira importer builds whole
workflows the same way. So:

- **states 1:1 by name.** An existing status of the same name is adopted
  (statuses are global; whoever named that row first owns it); a missing one is
  created. A state with zero issues still lands — the vocabulary belongs to the
  project, not to current traffic.
- **placement is Plane's own `group` field, never a name guess.**
  `backlog/unstarted/triage → to_do`, `started → in_progress`,
  `completed → done`, `cancelled → cancelled`. The name map
  (`DefaultPlaneStatusMap`) exists only for contracts predating
  `state_group`. This is load-bearing: the live VECTIS project has a `Blocked`
  state whose group is `started` — name-mapping files it under to-do, the group
  puts it where the work actually sits.
- **the `cancelled` category is created when the source needs it**
  (`is_completed=true`: abandoned work is finished work; burndown must stop
  counting it). Windshift seeds to_do/in_progress/done only.
- **`urgent` gets a priority row** (sort_order 0, above the seeded Critical)
  but only when an issue actually carries it — the importer does not decorate a
  deployment with vocabulary its sources never used. Dropping urgent to NULL
  loses a stated fact; folding it into Critical asserts a severity nobody
  chose.
- **labels** are created in the global catalog (source colour, normalised to
  `#RRGGBB`) and linked per item. Existing links are never removed.
- **new statuses get workflow edges** in the workflow the workspace actually
  resolves (configuration set first, then the global default): a
  `from_all_statuses` entry so anything may move onto them, and explicit exits
  to every other source-vocabulary status. Without edges a created status is
  an island — items land there and nothing may move them off. Edges are
  existence-checked before insert (the table has no unique key, so a rerun
  converges instead of accumulating).
- a state whose GROUP neither map knows is still created — under `to_do`, on a
  guess, and that guess is reported by name in `unmapped_state_names`.

## Reruns converge

A second run is not a no-op and must not be: an earlier build of this tool (or
an interrupted run) can leave rows on the collapsed vocabulary. For every
mapped row the loader checks the proof `items.updated_at == contract.updated_at`
— the import wrote the contract value, and every product write moves
`updated_at`. Untouched since import → **converged** onto the contract's
status/priority/labels (the UPDATE does not stamp time, so convergence is
itself idempotent). Edited since import → **left alone**, counted in
`issues_left_alone_edited_since_import`: the operator's later decision outranks
the import.

## Schema: two supported paths

- **Default** — the command runs the Windshift migration catalog (`Initialize`),
  which is how `plane_import_mappings` appears. Use this on a fresh install or a
  deployment already at this binary's version. Against a deployment that lags the
  catalog it moves the shared database forward to this binary's version, and the
  older app container will then refuse to restart (catalog checksums) until it is
  updated too. That is a deployment decision, not a detail.
- **`--skip-migrate`** — the app manages its own schema; the operator applies any
  missing pieces first. For a deployment measured at `20260918` (42 migrations)
  the complete set this importer needs is `legacy-schema.sql`: three nullable
  `items` columns (`team_id`, `intake_id`, `portal_org_shared`) plus
  `plane_import_mappings`. Every statement is copied from the upstream migration
  that owns it and is `IF NOT EXISTS`, so a later app upgrade finds them present,
  stamps the catalog itself, and changes nothing else. Nothing here touches
  `schema_migrations`, so the running app's checksums are untouched.

## Gates this tooling enforces

- meta must be the first contract line, and its `schema_version` must equal
  this build's (`ContractSchemaVersion`) — a v1 contract read by a v2 loader
  (or vice versa) refuses instead of silently mis-parsing rows;
- a number already owned by a row the importer does not own aborts the run
  (parity is not negotiable);
- soft-deleted source issues import as gaps, not as reused numbers;
- the vocabulary pass is one transaction; on `--dry-run` it commits, then
  rolls back — so a dry run proves the writes against the live schema and
  reports what the real run would create, persisting nothing;
- an issue referencing a label id the contract never carried is reported in
  `collisions`, not silently dropped;
- post-import item writes go through the product's own transition rules:
  the edges above are what make `POST /items/{id}/transition` accept moves
  onto and off the created statuses.
