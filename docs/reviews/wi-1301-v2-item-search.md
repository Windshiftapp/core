# WI-1301: move UI item search to v2

Implemented on 2026-09-09. The shared frontend `api.search.items` adapter now
uses `GET /api/v2/items/search`; the bearer mount exposes the same operation at
`GET /rest/api/v2/items/search` with `items:read`. The old `/api/items/search`
route, handler, and unused supporting methods were removed.

The endpoint supports case-insensitive title/description search, exact item
keys, explicit CQL (`ql`) and CQL detection in `q`. Empty queries list visible
items. Repeated `workspace_id`, `status`, and `priority` filters intersect with
workspace permissions before pagination and totals. `exclude_personal` is also
supported. Results use updated-at descending and ID ascending as the tie-breaker.
The frontend maps its existing `limit` argument to canonical `page_size` (up to
100) and unwraps the v2 data envelope, preserving its callers' array contract.

The configured search rate limit and database request timeout remain enforced.
Rate-limit denials use the v2 JSON error envelope. OpenAPI documents the new
operation, query parameters, response shape, errors, and bearer scope.

## Regression evidence

Before implementation, `TestV2ItemSearch` failed because `/items/search` was
interpreted as an item reference and returned 400. After implementation, the
search, filtering, pagination, private-workspace exclusion, invalid-input, and
missing-scope cases pass on SQLite and PostgreSQL.

The Playwright test `command-palette-search-v2.spec.ts` uses a restricted user,
searches through the UI, synchronizes on the v2 response, verifies text/key
results and hidden-workspace exclusion, and opens the visible item. The final
run passed with zero retries (one workflow plus authentication setup). Its
initial run passed UI assertions but failed cleanup on an incorrect user-delete
route; the cleanup now uses the existing production endpoint and passes.
No frontend unit tests were added.

## Validation

Commands run from core unless otherwise stated:

```sh
# Original-code regression reproduction (expected failure).
../core-tests/overlay.sh . -- -tags=test -count=1 -run '^TestV2ItemSearch$' ./tests

# Final focused SQLite coverage: passed.
../core-tests/overlay.sh . -- -tags=test -count=1 -run 'TestV2ItemSearch|TestSearchPickerIsolationHTTPContracts|TestRouteClassification|TestItemCRUD' ./tests ./internal/services

# Final focused PostgreSQL coverage: passed; each test uses a disposable database.
TEST_DB_TYPE=postgres TEST_POSTGRES_DSN='host=127.0.0.1 port=5432 user=stefanernst dbname=postgres sslmode=disable' ../core-tests/overlay.sh . -- -tags=test -count=1 -run 'TestV2ItemSearch|TestSearchPickerIsolationHTTPContracts' ./tests

# Final browser workflow: passed.
E2E_KEEP_ARTIFACTS=1 ../core-tests/run-e2e.sh tests/command-palette-search-v2.spec.ts --retries=0

# Scoped Go lint: zero issues.
./scripts/run-golangci-lint.sh run --timeout=5m ./internal/restapi/v2/... ./internal/services/... ./internal/routes/... ./internal/server/... ./internal/handlers/... ./internal/middleware/...

# From frontend: passed; Svelte files excluded.
bun --bun x biome check src/lib/api/misc.js ../../core-tests/e2e/tests/command-palette-search-v2.spec.ts

bun scripts/check-frontend-v2-fields.mjs
git diff --check
git -C ../core-tests diff --check
```

The broader command below passed middleware coverage but failed v2 assertions
outside this change: the existing `/items` query-parameter expectation omits
`exclude_personal`, and concurrent workspace milestone/iteration changes had
incomplete OpenAPI metadata at the time of the run.

```sh
../core-tests/overlay.sh . -- -tags=test -count=1 ./internal/restapi/v2 ./internal/middleware
```

An isolated production copy at `/private/tmp/wi1301-verify-j5sr7s9p` retained the
search change and restored the committed planning implementation and its
contract metadata. These checks passed there:

```sh
go run ./scripts/openapi-v2-metadata
make openapi-v2
make openapi-v2-check
```

The generated client compiled, OpenAPI parity/reproducibility passed, and the
frontend payload guard passed. The following transport checks also passed:

```sh
/Users/stefanernst/realigned/windshift-core/core-tests/overlay.sh /private/tmp/wi1301-verify-j5sr7s9p -- -tags=test -count=1 -run 'TestItemSearchRateLimitUsesV2Error|TestInventoryMatchesOpenAPI' ./internal/restapi/v2
```

Not run: full Go suite, full browser suite, full frontend suite, or race detector.
Existing collection-visibility and concurrent planning changes were preserved.
