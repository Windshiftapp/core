# V2 transition bug fixes

The fixes cover batch-link endpoint permissions, asset picker query translation,
workspace planning records in unscoped frontend lists, empty batch-link query
pages, and merge-patch validation in the frontend contract guard.

Regression tests live in `../core-tests`. Before the production fixes, focused
tests reproduced the link disclosure, empty-query error, missing asset filter,
missing workspace planning records, and undetected PATCH field. The guard test
also exposed leading whitespace causing the first payload field to be skipped.

## Passing validation

Commands run from `core`, unless otherwise noted:

```sh
../core-tests/overlay.sh . -- -tags=test -count=1 ./internal/restapi/v2/... ./internal/services/...
../core-tests/overlay.sh . -- -tags=test -count=1 -run 'TestBatchLinks|TestListOneHopItemLinksPage' ./internal/services/...
env TEST_DB_TYPE=postgres TEST_POSTGRES_DSN='host=/tmp user=stefanernst dbname=postgres sslmode=disable' ../core-tests/overlay.sh . -- -tags=test -count=1 -run 'TestBatchLinks' ./internal/services/...
../core-tests/overlay.sh . -- -tags=test -count=1 -run 'TestV2BatchLinks|TestV2LinksBatchIncludesTestCaseLinks' ./tests/...
../core-tests/run-overlay-script.sh . scripts/run-frontend-tests.sh src/lib/api
bun run scripts/check-frontend-v2-fields.mjs
./scripts/run-golangci-lint.sh run ./internal/services/...
git diff --check
```

The frontend API suite passed 381 tests. The field guard checked 31 literals.
HTTP tests verify the hidden-link response on both session and bearer mounts,
the successful visible-link path, and the exact empty-query page envelope.
The PostgreSQL regressions cover non-item visibility and filtered pagination;
the broader Go suites ran on SQLite.

From `core/frontend`:

```sh
bun --bun node_modules/.bin/biome check --write ../scripts/check-frontend-v2-fields.mjs ../../core-tests/frontend/src/lib/api/v2Bughunt.test.js ../../core-tests/frontend/src/lib/api/v2FieldGuard.test.js src/lib/api/milestones.js src/lib/api/assets.js
```

Go changes and new Go tests were formatted with `gofmt -w`. Go lint and Biome
completed without findings. Sandbox failures accessing the Go and npm caches
were rerun outside the sandbox. PostgreSQL validation caught and resolved a
CASE-expression parameter type issue before passing.

Browser tests, the full frontend suite, the full HTTP suite, and a full
PostgreSQL suite were not run. Unscoped planning lists now enumerate accessible
workspaces, so they make additional requests proportional to workspace count.
