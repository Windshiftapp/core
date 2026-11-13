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

Paired surfaces construct one `useVirtualRows` model and pass `sharedModel` to
both renderers, with distinct `pane` names. Model ownership is fixed for the
renderer lifetime. Roadmap uses one 40px fixed-size model, not two engines.

Mounted row DOM is bounded by viewport/overscan plus live pins. Loaded records,
indexes and engine measurements remain O(N); this is not an O(viewport) heap
claim. Qualification gaps are tracked in `QUALIFICATION.md` beside this file.
