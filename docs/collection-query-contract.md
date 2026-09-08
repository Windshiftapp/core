# Collection query and pagination contract

A saved collection is a query over the caller's accessible workspaces. An empty
or whitespace-only query adds no restriction. A collection's workspace location
does not replace its query scope; explicit workspace restrictions belong in CQL.
Completion filters and other sub-filters narrow that same scope before counting
and pagination. Removing a sub-filter must not turn an unrestricted collection
into an empty result.

Item lists and backlog resolve saved queries through `resolveItemListQLContext`.
Delta membership checks use the item list service. Board metadata projects the
matching workspace IDs through the same CQL evaluator and permission scope,
including when the query is empty.

The v2 response contract uses `page`, `page_size`, `total_items`, and `total_pages`.
`collectionService.js` adapts `page_size` to the collection store's existing
`limit` option once, for both items and backlog. Continuations must use that
effective server size, especially when the server caps a requested size.
Headers, pagination controls, and remaining counts all use `total_items`.
The collection query editor opts into empty searches and consumes canonical v2
pagination directly. The general search page still waits for a query.

Regression coverage lives in the sibling `core-tests` repository:

- `tests/collection_empty_query_test.go`: empty and whitespace queries,
  completion sub-filters, page contents and totals, board metadata, and
  inaccessible workspace exclusion.
- `frontend/src/lib/features/collections/collectionService.test.js`: the v2
  pagination response adapter for item and backlog continuation.
- `e2e/tests/collection-empty-query.spec.ts`: navigation beyond fifty rows,
  both list and query-editor pagination, toggling completion visibility from
  page two, and preference persistence.
- `frontend/src/lib/stores/searchStore.test.js`: unrestricted empty collection
  searches and the general search page's initial idle state.

## Validation commands

Run from `core`:

```sh
../core-tests/overlay.sh . -- -tags=test -count=1 -run 'TestEmptyCollection|TestCollectionMetadata|TestItemsBatch' ./tests
TEST_DB_TYPE=postgres TEST_POSTGRES_DSN='postgresql://localhost:5432/postgres?sslmode=disable' ../core-tests/overlay.sh . -- -tags=test -count=1 -run 'TestEmptyCollection|TestCollectionMetadata|TestItemsBatch' ./tests
../core-tests/overlay.sh . -- -tags=test -count=1 -run 'TestItemCRUDService|TestCollection.*Board|TestBoardConfiguration' ./internal/services
../core-tests/run-overlay-script.sh . scripts/run-frontend-tests.sh src/lib/features/collections src/lib/stores/collectionContext.test.js src/lib/stores/collectionCompletion.test.js src/lib/stores/searchStore.test.js
E2E_KEEP_ARTIFACTS=1 ../core-tests/run-e2e.sh tests/collection-empty-query.spec.ts tests/collection-completed-visibility.spec.ts tests/collection-search-columns.spec.ts --retries=0
./scripts/run-golangci-lint.sh run --timeout=5m
```

Go files are formatted with `gofmt`. Changed JavaScript and TypeScript files are
checked with Biome using `frontend/biome.json`; Svelte files are excluded.
Full repository suites are outside this focused validation.
