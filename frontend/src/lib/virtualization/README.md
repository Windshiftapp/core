# Shared row virtualization

`components/VirtualRows.svelte` is the standard renderer. It delegates geometry,
measurement, sparse pins and scroll operations to `VirtualRowsController`, backed
by pinned `@tanstack/virtual-core`. The base has no domain stores or fetching.

## Consumer contract

- Supply the complete **loaded** logical sequence in `items`, an occurrence key
  through `getKey`, and a positive `estimateSize` (default 48px).
- Bind `listElement` to the actual flow container, below any nonvirtual header.
  Supply the actual vertical `scrollElement`, or inherit the shell's
  `VIRTUAL_SCROLL_CONTEXT`. Missing browser APIs/root fall back to normal rows.
- Apply `use:virtual.attach` to each row's actual outer element. Its measured
  border box must include spacing; do not add unmeasured vertical margins.
- Use an identity containing audience-independent workspace/collection/view,
  query, sort and page scope. Keep the identity stable for patches and expansion.
  Changing it remounts row-local state and resets/restores that query's anchor.
- Identities are also the restoration namespace across remounts. Two independent
  scroll surfaces must use distinct prefixes (for example `primary:records:...`
  and `secondary:records:...`), even when they show the same query. Root DOM
  objects are deliberately not the restoration key, since they change on remount.
  The 20-entry LRU is audience-scoped, not automatically root-scoped.
- Pass a `layoutRevision` when columns or other row geometry inputs change.
- Keep selection, permissions, actions, pagination, rank logic and data caches
  in the consumer. Virtualization never loads additional data.

```svelte
<div bind:this={root} style="height: 480px; overflow: auto;">
  <div bind:this={list} style="overflow-anchor: none;">
    <VirtualRows items={records} getKey={(record) => record.key}
      scrollElement={root} listElement={list} identity="records:query-1">
      {#snippet row(record, virtual)}
        <div use:virtual.attach>{record.label}</div>
      {/snippet}
    </VirtualRows>
  </div>
</div>
```

For native tables, bind the list to `<tbody>`, attach each `<tr>`, and supply a
`spacer(size)` snippet rendering an `aria-hidden` row with a spanning cell whose
height is `size`, padding/border zero. Do not insert divs beneath `<tbody>`.
The design-system viewer includes an independent 10,000-record table example.

## Lifecycle and interactions

The Svelte adapter owns controller teardown and print/input listeners. The
viewport registry shares one scroll listener/resize observation per root.
Normal rendering applies through 100 rows, or when `enabled` is false. Print
temporarily renders all loaded rows and returns to the previous anchor.
Full loaded-row mounting is intentional in normal/disabled/print mode, including
consumer cells: restoring `LazyRender` there would prevent browser Find, copying
and accessible/print output from seeing those rows. The 50-row performance gate
remains unqualified; the full-render escape hatch has a synchronous O(N) mount.

The queue identity includes the accepted search, effective queue QL, collection,
workspace and page/size. It changes only when results arrive, not on each draft
keystroke. Tree has no free-text search input; its hierarchy level and effective
sub-filter define its query scope. Backlog uses its effective backlog sub-filter.
Collapse/expansion intentionally preserve identity and a surviving row anchor;
they are structural changes, not new queries.

`useRowRetention(() => active, reason)` retains an editor, picker or portalled
preview's row while active, with idempotent reference-counted releases. It is a
no-op outside a virtual row context. Acquisition is imperative and untracked:
subscribing to the descriptors invalidated by pinning creates a reactive loop.
Focus retention is attached to the row; Tab traversal can materialize its next
logical neighbor. Pins add distant rows sparsely, not the intervening range.

`dragSources` opt-in dynamically loads the Pragmatic drag session bridge, pins
the source, increases overscan from 8 to 16, and owns root auto-scroll. A
`dragRevision` invalidates the gesture when relevant domain data changes.
Consumers still enforce domain permissions, drop eligibility and mutations.
Drop outcomes survive the complete native drop dispatch but are reset at every
new drag start; returning a stable drag-data object is supported.

Paired surfaces construct one `useVirtualRows` model and pass `sharedModel` to
both renderers, with distinct `pane` names. Model ownership is fixed for the
renderer lifetime. Roadmap uses one 40px fixed-size model, not two engines.
DOM registrations are pane-qualified; nonvirtual anchor/scroll geometry uses the
node inside the model's canonical list, regardless of pane attachment order.

Duplicate/empty occurrence keys are repaired at the controller boundary with
collision-safe positional keys and one warning per controller. This keeps a
malformed response visible, but cannot provide reorder-stable editor identity
for those malformed occurrences. `indexRows` remains a strict validation utility.

Mounted row DOM is bounded by viewport/overscan plus live pins. Loaded records,
indexes and engine measurements remain O(N); this is not an O(viewport) heap
claim. Qualification gaps are tracked in `QUALIFICATION.md` beside this file.
