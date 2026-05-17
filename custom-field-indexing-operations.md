# Custom Field Indexing — Operator Manual

A guide for administrators deciding whether and when to flip the **Index this
field** toggle on a custom field, and what to expect operationally on SQLite
deployments.

## TL;DR

| Workspace item count | Recommendation |
|---|---|
| **< 100k** | Don't bother. Both indexed and unindexed queries return in well under 100ms. |
| **100k – 1M** | Enable for fields that are filtered or sorted frequently. ~1600× – 4300× speedup. |
| **> 1M** | Strongly recommended for any field used in user-facing filters. Unindexed queries on these fields become a UX problem. |

Cap is **20 indexed custom fields per target table** (items, assets). This is
not a soft cap — exceeding it returns a 400 error from the API.

## What the toggle actually does (SQLite)

The "Index this field" checkbox is **not** instant on SQLite:

1. Checking the box inserts a tracking row in `custom_field_indexes`. **No
   `CREATE INDEX` runs yet.** The toggle reflects the *intent*; the physical
   index does not exist.
2. The physical index is built **on the next server restart**, during
   `Initialize()`. For each tracked entry the server runs
   `CREATE INDEX idx_cf_<table>_<fieldID> ON <table>(...)` while booting.
3. Unchecking the box runs `DROP INDEX IF EXISTS` immediately and removes the
   tracking row. If the index never made it past step 1 (admin toggled on and
   off without an intervening restart), the drop is a silent no-op.

**Implication**: enabling an index on a workspace with millions of rows will
**not slow down the running server**. It will delay **the next startup**.
Plan deploys/restarts with this in mind — see "Restart impact" below.

(On PostgreSQL the same toggle runs `CREATE INDEX` inline. Postgres can build
indexes concurrently; SQLite cannot, so the deferred approach was chosen.)

## Measured performance (1M rows, SQLite WAL, commodity laptop)

Source: `core-tests/stresstest/run-index-stress.sh --rows 1000000`. Numbers
are median of 3 runs of `SELECT COUNT(*) ... WHERE <cf-extract> = ?`.

| Rows | Unindexed (ms) | Indexed (ms) | Speedup |
|---:|---:|---:|---:|
| 10,000 | 4.6 | 0.02 | 220× |
| 50,000 | 23.3 | 0.02 | 970× |
| 100,000 | 47.3 | 0.03 | 1,600× |
| 500,000 | 238.6 | 0.07 | 3,500× |
| 1,000,000 | 461.2 | 0.11 | 4,300× |

**Reading the curve**:

- **Unindexed query cost grows linearly** at ~0.46ms per 1000 rows. Project
  forward: at 10M rows, ~4.6s. At 100M rows, ~46s. This is what a "no index"
  custom field filter does on every request.
- **Indexed query cost grows logarithmically** (sub-ms up to 1M). SQLite
  serves these from a covering index — the table itself is never touched.

## Cost of enabling indexing

Three costs to weigh against the query-time win.

### 1. Restart impact

After enabling an index, the **next** `Initialize()` does the
`CREATE INDEX`. Measured: **~5.6 seconds per 1M rows per index** for a
single-column JSON-extract index on a number field. With the per-table cap
of 20 indexes, an extreme worst case is ~110s of additional startup delay
on a 1M-row workspace if every slot is filled and every index is freshly
queued. Realistic deployments will be far less.

**Action**: when enabling several indexes in a row, expect the first
restart after the change to be slower than usual. Watch the
`creating deferred custom field index` log lines to see progress.

### 2. Insert / update throughput

Every additional index adds write overhead. Items currently have ~20
schema indexes plus whatever custom field indexes are enabled. In our
test, insert rate dropped from ~7,500 rows/sec at small table size to
~6,100 rows/sec at 1M rows — that's the cost of maintaining all
secondary indexes on every `INSERT`.

**Action**: for workspaces with heavy bulk imports (e.g. Jira migrations,
CSV uploads), consider enabling custom field indexes **after** the bulk
load completes, not before. Bulk delete is similarly affected: measured
~75µs per row for `DELETE FROM items WHERE id > ?` on the indexed table.

### 3. Storage

Each JSON-extract index materialises the extracted values in a B-tree.
For a 1M-row, number-typed field, the index file fragment is on the order
of 10–30 MB depending on cardinality. Negligible for a single field;
worth budgeting if you cap-out on a self-hosted disk.

## Field type rules

Only three types can be indexed:

- `number` — index uses `CAST(... AS NUMERIC)`.
- `text` — no cast; UTF-8 ordering.
- `date` — index uses `CAST(... AS TEXT)`; effectively lex sort on
  `YYYY-MM-DD` strings.

Attempting to enable indexing on any other type (linking, mirror, etc.)
returns **400 Bad Request**.

## What is safe to do

- **Toggle on, then off, with no intervening restart**: safe. No row leaks
  in `custom_field_indexes`, no orphan in `sqlite_master`. Verified by the
  Phase D check in the stress test.
- **Delete a custom field while it is indexed**: safe. The delete handler
  explicitly drops every tracked index for the field, then the tracking
  rows cascade via FK.
- **Multiple indexes on the same field across both tables (items + assets)**:
  supported. Counts independently against each table's cap of 20.

## Known sharp edges

- **No pre-toggle warning**: enabling on a multi-million-row workspace gives
  no UI feedback about the upcoming startup delay. If you operate at that
  scale, communicate planned restarts in advance.
- **Audit log is thin**: `custom_field_update` audit rows record the new
  `indexed` map but not whether the build has happened. Use the
  `creating deferred custom field index` server log lines for the
  authoritative timeline.
- **Asymmetric data semantics elsewhere**: when a *milestone* is deleted,
  linked test plans get `milestone_id` set to NULL while linked items
  lose their join-table row. Custom field index toggles are unrelated but
  share the same pattern of "delete cascades through different mechanisms
  per relation type" — keep this in mind when reasoning about side effects.

## Reproducing the measurements

```bash
cd core-tests/stresstest
./run-index-stress.sh --rows 1000000
```

Full run takes ~5 minutes (2m46s inserts, ~80s DELETE-descent during the
indexed sweep, plus measurement and bootstrap). For a faster sanity check:

```bash
./run-index-stress.sh --rows 50000 --checkpoints 10000,25000,50000
```

See `core-tests/stresstest/README.md` for the rest of the flags.
