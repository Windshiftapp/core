# Contextual collection counts

Implemented 2026-09-09 following the collection count review.

The existing muted header subtitle now shows `100 items · 50 shown`. The first
number counts accessible items matching the saved collection query (or the
workspace for its default views). The second counts work items currently shown
in that view, including local filters, pagination, collapsed branches, and board
caps. It is omitted when equal to the collection total. Tree test-case rows do
not count as work items. Backlog collapsed sections do not count as shown.

The collection sidebar uses the same membership total. Zero remains visible;
a failed count read omits the total and leaves any available shown count. No
additional banner, counter badge, or tooltip is introduced. The configuration
switcher's old backlog badge was removed because it inherited another view's
collection/filter scope. Board footer copy says “Showing”, rather than “Total”.

`fetchCollectionTotal` requests one item summary through the existing authorized
item endpoint and reads `pagination.total_items`. It passes collection/workspace
identity, without temporary view filters or board partition/retention predicates.
Saved-query predicates and server authorization still apply. Count failures do
not discard successfully loaded view data. Request generation and load identity
guards prevent old results from overwriting newer scopes. The count response's
watermark also participates in the oldest-snapshot tracking, so a mutation
between count and view reads is replayed by delta polling.

Counts refresh with loads, explicit refreshes, list pagination, and local/delta
updates. Delta removals refresh authoritative results rather than decrementing
only loaded rows or guessing whether an unloaded ID belonged to the collection.
No additional polling interval was added. Existing view refresh triggers remain
in effect; this does not introduce live polling into passive hierarchy views.

Filtered pagination totals still belong to pagination. Column, lane, root,
child, and backlog section counts remain scoped to their labelled containers.
This change does not implement missing server continuation controls for tree,
map, or roadmap: the header now describes their partial display accurately.

## Validation

Passed: 88 frontend tests across 18 files; both browser workflows plus their
shared setup; production frontend build; Biome on the changed non-Svelte code;
and `git diff --check` in both repositories. The final concurrency regression
first reproduced polling from the newer item watermark instead of the older
count snapshot, then passed with the snapshot tracking fix.

Regression tests first failed against the old code for missing independent
totals, absent shown context, and global headers' leading separator. Final
validation commands (run from `core`, except Biome as noted):

```sh
../core-tests/run-overlay-script.sh . scripts/run-frontend-tests.sh src/lib/features/collections src/lib/stores/collectionContext.test.js src/lib/stores/collectionCompletion.test.js src/lib/stores/collectionCounts.test.js src/lib/layout/ViewHeader.test.js
E2E_KEEP_ARTIFACTS=1 ../core-tests/run-e2e.sh tests/collection-counts.spec.ts tests/collection-completed-visibility.spec.ts --retries=0
```

Biome from `core/frontend` (Svelte excluded; locale files follow the repository's
existing exclusion):

```sh
bunx biome check --write src/lib/stores/collectionContext.svelte.js src/lib/features/collections/collectionService.js
bun --bun ./node_modules/@biomejs/biome/bin/biome check --write --config-path=biome.json ../../core-tests/frontend/src/lib/stores/collectionCounts.test.js ../../core-tests/frontend/src/lib/layout/ViewHeader.test.js ../../core-tests/frontend/src/lib/features/collections/CollectionNavigation.test.js ../../core-tests/frontend/src/lib/stores/collectionContext.test.js ../../core-tests/frontend/src/lib/stores/collectionCompletion.test.js ../../core-tests/e2e/tests/collection-counts.spec.ts
```

The browser runner builds the frontend and a disposable Go server. Full Go,
PostgreSQL, and full frontend/browser suites are outside this frontend change.
