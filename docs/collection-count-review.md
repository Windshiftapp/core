# Collection item count review

Reviewed 2026-09-09. Implementation follow-up: `collection-counts.md`. This is a source review of the current working tree, not a browser reproduction. No production code was changed. Existing uncommitted Go changes were left intact.

The UI has no independent collection total. Shared headers mix query totals, loaded item counts, visible tree rows, and backlog totals. This explains disagreements even when the saved collection has not changed.

## Current count sources

Paths below are relative to `frontend/src/lib/`.

| Surface | Current source | Meaning |
| --- | --- | --- |
| Collection sidebar | `features/collections/CollectionNavigation.svelte:35`, store `itemsTotalCount` | Current main query total plus deferred board partition; hides zero |
| Board header | `features/collections/CollectionBoard.svelte:1469` | Same store total, including deferred statuses |
| Board columns | `CollectionBoard.svelte:866`, `getColumnTotal` | Usually loaded column items; substitutes deferred server total only without grouping, search, or iteration filtering and when the column covers the whole partition |
| Board swimlanes | `CollectionBoard.svelte:963` and `:982` | Loaded items assigned to the lane |
| Board footer | `CollectionBoard.svelte:1805` | Renderable cards after column caps and local filters, labelled “Total” |
| List header | `features/collections/CollectionList.svelte:336` | Server total after completion and sub-filters; loaded rows as fallback |
| List pagination/footer | `CollectionList.svelte:459` | Server pagination total, or local filtered row count in the fallback footer |
| Tree header | `features/collections/CollectionTree.svelte:337` | Loaded item array length |
| Tree pagination | `CollectionTree.svelte:198` | Roots in the loaded hierarchy, explicitly labelled as root items |
| Map header | `features/collections/CollectionMap.svelte:554` | Filtered server total; current backbone plus children as fallback |
| Map child badges | `CollectionMap.svelte:698` | Loaded children under the displayed parent |
| Roadmap header and row label | `features/collections/CollectionRoadmap.svelte:1105` and `:1333` | Flattened, expanded tree row count |
| Roadmap unscheduled hint | `CollectionRoadmap.svelte:512` | Loaded items without effective dates |
| Backlog header/footer | `features/collections/CollectionBacklog.svelte:244`, `:716`, `:844` | Backlog query total after sub-filters |
| Backlog section counts | Local backlog/iteration item arrays | Loaded section membership |
| Legacy board/backlog switcher badge | `features/collections/CollectionViewSwitcher.svelte:78` | Singleton `backlogStore.count`; switcher is rendered by board configuration |
| Collection query editor | `features/collections/Collections.svelte:510` | Search result pagination total, or loaded row fallback |

The collection directory's collection count counts saved collections, not work items. Filter sidebar `CollectionsSidebar.svelte` has no item total.

## Findings

### 1. High: the collection total changes with the active view's filters

`stores/collectionContext.svelte.js:129` adds `status_completed = false` unless completion visibility is enabled. `load` at line 217 forces completion visibility on for boards, while other views default to off and persist separate preferences per view. The main query at line 502 includes that completion predicate and the active sub-filter. `itemsTotalCount` at line 629 sums those query totals; it is not a saved-collection total.

For a collection containing 80 unfinished and 20 completed items, board can say 100 while list says 80 without any saved-query change. These are valid view result totals, but they cannot also serve as the stable collection count the user expects.

### 2. High: backlog sidebar count depends on navigation history

`loadsItems` excludes backlog views (`collectionContext.svelte.js:62`). Loading backlog sets only backlog results. The collection sidebar nevertheless reads `itemsTotalCount`, which reads main item pagination and board deferred state.

A fresh direct backlog load has no main total and suppresses the sidebar number. Switching from another view of the same collection retains its main pagination/deferred total. Backlog filter changes then refresh backlog data without refreshing the sidebar's source. The backlog header and sidebar can therefore disagree within the same page.

### 3. High: tree reports a partial load as the total and cannot reach remaining server pages

Tree requests 250 items (currently capped to 100 by the API adapter) (`collectionContext.svelte.js:34-59`) and copies the store array into `allItems`. Its header uses that array length. Its pagination slices only roots from this local array (`CollectionTree.svelte:191-204`). Tree does not consume `itemsHasMore` or invoke `loadMoreItems`.

With 300 matching items, a normal initial tree load can report 100 while the sidebar reports 300. Its root-page controls do not fetch the missing 200. Map and roadmap also lack server continuation controls, so their hierarchy and local counts can be incomplete even when map's header correctly reports the server total.

### 4. Medium: collapsing roadmap branches changes the collection header count

`CollectionRoadmap.svelte:495` recurses into children only for expanded parents. `treeData` at line 502 is this flattened visible hierarchy; the shared header uses `treeData.length`.

Collapsing a parent changes the header number without changing membership, filters, or persisted data. The row label can legitimately use this count if it says “visible rows”; the collection header cannot.

### 5. Medium: board uses “Total” for shown cards and mixes column count scopes

The header includes both server partitions. The footer sums displayable column cards after caps (`CollectionBoard.svelte:1010`) but the English translation says “Total: … work items across … columns” (`locales/en/workspace.js:677`). A capped completed column can therefore make the two apparent totals disagree.

Column totals switch between loaded items and a server partition total in `getColumnTotal`. Adding grouping, search, or an iteration filter disables the server-total substitution. Separate view counts are useful, but must state whether they count shown cards, loaded matches, or all matches. This review does not claim the partitions themselves are double-counted.

### 6. Medium: delta removals can leave paginated totals stale

`refreshDeltas` removes returned IDs, then returns immediately if there are no changed IDs (`collectionContext.svelte.js:1050-1060`). `#removeItemsById` decrements totals only for IDs found in loaded arrays (`:1132-1184`).

The backend emits deleted IDs without forcing a full reload in the ordinary deletion path (`internal/services/item_application_service.go:379-405`). Deleting a matching item on an unloaded page therefore need not decrement the displayed total. Blindly subtracting every returned ID would also be wrong: the delta stream may include items that never matched this collection. Authoritative count refresh is needed when membership is uncertain.

### 7. Medium: the legacy backlog badge lacks collection/filter identity

`stores/backlogStore.svelte.js` stores one workspace ID and one count. Board and backlog write their collection- and sub-filter-specific result totals into it (`CollectionBoard.svelte:465`, `CollectionBacklog.svelte:297`). The switcher reads the singleton without checking collection or filter identity. Global collections also share a null workspace identity.

This affects the switcher rendered by board configuration; it should not be confused with the main collection sidebar bug above. Counts should be keyed to their actual scope or supplied directly by the owning collection context.

## Backend and existing contract

`internal/services/item_crud_service.go:551-650` resolves saved collection queries and additional predicates for list and backlog. Backlog adds backlog status IDs. `internal/repository/item_list.go:525-549` constructs count and page queries from the same predicates; its special workspace count cache excludes collection/QL/filter requests.

The primary mismatch found is therefore in which result the UI calls the collection count, plus partial loading and refresh behavior. This is not proof that every backend count path is correct.

`docs/collection-query-contract.md` currently says completion/sub-filters narrow results before counting and that headers use `total_items`. That accurately describes filtered API totals but does not define a separate collection membership total; tree and roadmap also violate its header statement. Any implementation should update that document explicitly.

## Proposed count contract

1. **Collection total:** distinct accessible items matching the saved query, including completed items unless the saved query excludes them. Workspace-default views use their workspace scope. All shared view headers and the collection sidebar use this same value. View filters, collapsed branches, pagination, board caps, and backlog projection do not change it.
2. **View matches:** items matching the collection plus active view predicates. Label separately, for example “80 matching · 100 in collection.” Backlog, completion visibility, and sub-filters affect this value.
3. **Shown items/rows:** what the view currently renders. Label explicitly, for example “50 shown · 80 matching.” Roots, lane counts, column counts, child badges, and unscheduled hints need equally explicit scope where loading is incomplete.
4. Zero is a real count. Loading, unknown, and failed count reads must not masquerade as zero or retain another scope's number.

Keep server pagination totals for pagination. Simply replacing every header with today's `itemsTotalCount` would preserve filter-dependent counts and the backlog bug. Introduce an independently loaded membership total using the same authorized query resolution, without view predicates; it must load on direct entry to every view, including backlog. Refresh or invalidate it for item membership changes, saved-query changes, and permission changes, with stale-response guards. Do not sum backlog and main totals because their sets overlap.

## Regression work for implementation

Add tests in `../core-tests`, following its instructions, for:

- One fixed collection across board, list, tree, map, roadmap, and backlog: identical collection total despite different view match counts.
- Fresh backlog entry and navigation from board/list, including zero results and failed loads.
- More than 250 matching items: correct header total and accessible remaining hierarchy items.
- Roadmap collapse/expand and map drill-down: collection total unchanged.
- Completed visibility, saved completion predicates, sub-filters, board search, iteration filters, swimlanes, and capped columns: explicit count scopes.
- Deletion or membership change on an unloaded page, including unrelated delta IDs that must not reduce this collection total.
- Switching between collections/workspaces with requests resolving out of order.
- Inaccessible items excluded from totals, with an exact denial assertion for inaccessible collection reads.

Existing navigation tests mock `itemsTotalCount`; they verify rendering but cannot establish cross-view consistency. Existing collection context, completion, hierarchy, query, and browser tests are useful starting points.

Validation performed: source tracing and inspection of existing documentation/tests. No test suites, browser runs, or database reproductions were run for this documentation-only review.
