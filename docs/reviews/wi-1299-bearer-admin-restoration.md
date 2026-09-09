# WI-1299: restore bearer admin endpoints

The v2 migration accidentally removed bearer access to audit logs, API token
administration, and object translations. The user confirmed on 2026-09-09 that
these restrictions were unintentional.

## Restored contract

Eleven operations are available under `/rest/api/v2/admin` and the canonical
session mount `/api/v2/admin`. Each requires system administrator permission.
Bearer callers additionally need the exact operation scope:

| Operations | Scope |
| --- | --- |
| GET audit-logs and audit-logs/since | admin:audit-logs:read |
| GET api-tokens | admin:api-tokens:read |
| DELETE api-tokens/{token_id} | admin:api-tokens:write |
| GET translation definitions, orphans, canonical-differences, and object translations; POST resolve | admin:object-translations:read |
| PUT and DELETE an instance translation | admin:object-translations:write |

Responses use v2 envelopes and errors. Audit-log and token lists use canonical
`page`/`page_size` pagination. Audit filters retain action type, resource type,
user ID, and inclusive RFC3339 timestamp bounds. Audit streaming preserves
`after_id`, ascending ID order, `next_after_id`, and `has_more`, with a default
limit of 500 and a maximum of 1000 (larger values are clamped).

The handlers reuse the existing audit repository, token manager, auditor, and
translation service. Revocation invalidates the token cache and records the
existing admin-revoke audit event. Translation mutations affect instance rows
and retain the service's shipped-translation protection and cache invalidation.
Unexpected token-manager failures produce a sanitized 500; a missing token
produces 404. OpenAPI includes typed payloads, explicit bearer security, and all
five scope names.

## Verification

Tests live in core-tests. Before implementation, all 33 cases in
`TestAdminBearerEndpointDenials` failed with 404 because the eleven operations
were absent. After implementation they pass with exact 401/403 error contracts
for anonymous callers, admins lacking the scope, and scoped non-admins.

HTTP regression tests create fixtures through production APIs. They cover
translation persistence, resolution and fallback after deletion, read-only
mutation denials, diagnostic reads, invalid object inputs, user-filtered token
listing, denied revocation, revocation of another user's cached token, audit
record creation, incremental audit reads, and invalid query values.

Commands run from core:

```sh
# Expected original-code failures, then passed after implementation.
../core-tests/overlay.sh . -- -tags=test -count=1 -run TestAdminBearerEndpointDenials ./internal/restapi/v2/...

# SQLite HTTP regressions: passed.
../core-tests/overlay.sh . -- -tags=test -count=1 -run TestV2AdminBearer ./tests/...

# Isolated PostgreSQL HTTP regressions: passed.
env TEST_DB_TYPE=postgres TEST_POSTGRES_DSN='postgresql://postgres@127.0.0.1:15439/postgres?sslmode=disable' ../core-tests/overlay.sh . -- -tags=test -count=1 -run TestV2AdminBearer ./tests/...

# Complete v2 unit suite: passed.
../core-tests/overlay.sh . -- -tags=test -count=1 ./internal/restapi/v2/...

# Related HTTP regressions and route classification: passed.
../core-tests/overlay.sh . -- -tags=test -count=1 -run 'TestV2AdminBearer|TestObjectTranslation|TestAPITokenScopes|TestRouteClassification' ./tests/...

# Production Go lint: zero issues. Changed Go files were also gofmt formatted.
./scripts/run-golangci-lint.sh run ./internal/restapi/v2/... ./internal/server/...

go run ./scripts/openapi-v2-metadata
make openapi-v2
make openapi-v2-check

git diff --check
git -C ../core-tests diff --check
```

OpenAPI checks include inventory parity, reproducibility, generated Go client
compilation, and the frontend payload guard. An intermediate check caught a
missing tag declaration and missing bearer security metadata; both were fixed.
The complete v2 suite then passed. Some Go runs required escalation for build
cache access. The default PostgreSQL address was unavailable; the successful
run used a disposable local cluster instead.

Not run: full repository Go suite, browser tests, frontend test suites, race
detector, release builds, or vulnerability scans. No frontend production code
was changed for this item. Implementation and regression tests are recorded in the WI-1299 commits in core and core-tests.
