# Windshift v0.4.6

---

> **Not recommended for production use.**
>
> Windshift is an early release that is still undergoing internal testing. APIs, data formats, and configuration may change between releases without migration paths. We publish this release to invite early exploration, testing, and feedback - not to support production workloads.
>
> If you encounter issues or have ideas, please open an issue. Your feedback at this stage is incredibly valuable.

---

## Highlights

### Fixed CLI Status Filtering
Status filtering in the CLI (`ws task list -s done`, `ws task mine -s ~done`) was silently broken because aliases stored status names instead of numeric IDs. The server's `status_id` parameter requires an integer, so name-based values were quietly ignored. Aliases now store numeric status IDs, and a fallback mechanism queries the server's completed-statuses endpoint when aliases are stale or missing.

### Status Exclusion Filter (`~done`)
The CLI's negation syntax (`-s ~done`) now works end-to-end. A new `status_id_not` query parameter has been added to the items API, enabling server-side exclusion of a specific status.

### Completed Statuses Endpoint
A new `GET /rest/api/v1/workspaces/{id}/statuses/completed` endpoint returns only statuses where the category is marked as completed. This powers the CLI's fallback resolution and is available for any integration that needs to identify "done" statuses programmatically.

### `ws config refresh` Command
A new `ws config refresh` subcommand re-fetches workspace statuses from the server and regenerates status aliases with numeric IDs in `ws.toml`. Use this after renaming statuses on the server to keep your local aliases in sync.

---

## Bug Fixes

- **Status filter silently ignored:** `ws task list -s done` now correctly sends numeric status IDs to the server instead of status names that fail `strconv.Atoi` silently
- **Negation filter no-op:** `ws task list -s ~done` now excludes the specified status via the new `status_id_not` server-side filter
- **Stale alias resilience:** If a status is renamed after `ws init`, the CLI falls back to the completed-statuses endpoint to resolve "done" dynamically

## API Changes

- Added `status_id_not` query parameter to `GET /rest/api/v1/items` for excluding items by status
- Added `GET /rest/api/v1/workspaces/{id}/statuses/completed` endpoint
