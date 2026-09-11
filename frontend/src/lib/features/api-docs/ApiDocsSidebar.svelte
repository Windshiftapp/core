<script>
  import MethodBadge from './MethodBadge.svelte';
  import Input from '../../components/Input.svelte';
  import NativeSelect from '../../components/NativeSelect.svelte';
  import { filterGroups } from './openapi-store.svelte.js';
  import ScrollableSidebar from '../../layout/ScrollableSidebar.svelte';

  /** @typedef {import('./openapi-store.svelte.js').OperationGroup} OperationGroup */
  /** @typedef {import('./openapi-store.svelte.js').OperationEntry} OperationEntry */
  /** @type {{ groups: OperationGroup[], query?: string, selectedId?: string | null, version?: string, versions?: Array<{ value: string, label: string }>, onselect?: (entry: OperationEntry) => void, onversionchange?: (version: string) => void }} */
  let {
    groups,
    query = $bindable(''),
    selectedId = null,
    version = 'v2',
    versions = [],
    onselect = () => {},
    onversionchange = () => {},
  } = $props();

  let collapsedTags = $state(new Set());
  const visibleGroups = $derived(filterGroups(groups, query));
  const visibleCount = $derived(countOperations(visibleGroups));
  const isFiltering = $derived(query.trim().length > 0);

  /** @param {OperationGroup[]} value */
  function countOperations(value) {
    return value.reduce((total, group) => total + group.operations.length, 0);
  }

  /** @param {KeyboardEvent} e @param {OperationEntry} entry */
  function handleKey(e, entry) {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      onselect(entry);
    }
  }

  /** @param {string} tag */
  function toggleGroup(tag) {
    const next = new Set(collapsedTags);
    if (next.has(tag)) next.delete(tag);
    else next.add(tag);
    collapsedTags = next;
  }

  /** @param {string} tag */
  function groupIsCollapsed(tag) {
    return !isFiltering && collapsedTags.has(tag);
  }

  function collapseAll() {
    collapsedTags = new Set(groups.map((group) => group.tag));
  }

  function expandAll() {
    collapsedTags = new Set();
  }
</script>

{#snippet sidebarHeader()}
  <header class="sidebar-head">
    <h1 class="sidebar-title">API reference</h1>
    <div class="sidebar-meta">
      <span>{countOperations(groups)} operations</span>
      <span class="group-controls">
        <button type="button" onclick={collapseAll}>Collapse all</button>
        <span aria-hidden="true">·</span>
        <button type="button" onclick={expandAll}>Expand all</button>
      </span>
    </div>
    <NativeSelect
      value={version}
      options={versions}
      size="small"
      dataTestid="api-docs-version"
      ariaLabel="API version"
      onchange={onversionchange}
    />
  </header>

  <div class="filter">
    <Input
      type="search"
      bind:value={query}
      placeholder="Search method, path, tag, or summary…"
      class="filter-input"
      dataTestid="api-docs-filter"
      ariaLabel="Filter operations"
      size="small"
    />
    {#if query}
      <span class="filter-count">{visibleCount}</span>
    {/if}
  </div>
{/snippet}

<ScrollableSidebar
  as="aside"
  class="sidebar"
  data-testid="api-docs-sidebar"
  aria-label="API reference"
  header={sidebarHeader}
  scrollTestid="api-docs-navigation-scroll"
>
  <nav class="groups">
    {#each visibleGroups as group (group.tag)}
      {@const collapsed = groupIsCollapsed(group.tag)}
      <section class="group">
        <h2 class="group-tag">
          <button
            type="button"
            aria-expanded={!collapsed}
            data-testid="api-docs-tag-toggle"
            onclick={() => toggleGroup(group.tag)}
          >
            <span class="group-chevron" class:group-chevron--collapsed={collapsed} aria-hidden="true">⌄</span>
            <span>{group.tag}</span>
            <span class="group-count">{group.operations.length}</span>
          </button>
        </h2>
        {#if !collapsed}
          <ul class="group-ops">
            {#each group.operations as entry (entry.id)}
              <li>
                <a
                  id={`api-docs-op-${entry.id}`}
                  href={`#${entry.id}`}
                  class="op-row"
                  class:op-row--active={selectedId === entry.id}
                  aria-current={selectedId === entry.id ? 'page' : undefined}
                  data-testid="api-docs-op-link"
                  data-op-id={entry.id}
                  onclick={(e) => { e.preventDefault(); onselect(entry); }}
                  onkeydown={(e) => handleKey(e, entry)}
                >
                  <MethodBadge method={entry.method} />
                  <span class="op-copy">
                    <span class="op-path">{entry.path}</span>
                    {#if entry.operation.summary}
                      <span class="op-summary">{entry.operation.summary}</span>
                    {/if}
                  </span>
                </a>
              </li>
            {/each}
          </ul>
        {/if}
      </section>
    {/each}
    {#if visibleGroups.length === 0}
      <p class="empty">No operations match “{query}”.</p>
    {/if}
  </nav>
</ScrollableSidebar>

<style>
  :global(.sidebar) {
    width: 100%;
    flex-shrink: 0;
    border-right: 1px solid var(--ds-border);
    background: var(--ds-surface);
    height: 100%;
  }
  .sidebar-head {
    padding: 18px 18px 8px;
    border-bottom: 1px solid var(--ds-border);
  }
  .sidebar-title {
    font-size: 14px;
    font-weight: 600;
    color: var(--ds-text);
    margin: 0;
  }
  .sidebar-meta {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    font-size: 12px;
    color: var(--ds-text-subtle);
    margin: 2px 0 10px;
  }
  .group-controls {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    white-space: nowrap;
  }
  .group-controls button {
    border: 0;
    padding: 0;
    background: transparent;
    color: var(--ds-text-link);
    font-size: 11px;
    cursor: pointer;
  }
  .group-controls button:hover {
    text-decoration: underline;
  }
  .filter {
    padding: 10px 18px;
    border-bottom: 1px solid var(--ds-border);
    position: relative;
  }
  :global(.filter-input) {
    width: 100%;
    padding: 6px 10px;
    font-size: 12.5px;
    background: var(--ds-surface);
    border: 1px solid var(--ds-border);
    border-radius: 4px;
    color: var(--ds-text);
  }
  :global(.filter-input):focus {
    outline: none;
    border-color: var(--ds-border-focused);
  }
  .filter-count {
    position: absolute;
    right: 26px;
    top: 50%;
    transform: translateY(-50%);
    font-size: 11px;
    color: var(--ds-text-subtle);
    pointer-events: none;
  }
  .groups {
    padding: 4px 0 24px;
  }
  .group {
    margin-top: 14px;
  }
  .group-tag {
    margin: 0;
  }
  .group-tag button {
    display: grid;
    grid-template-columns: 12px minmax(0, 1fr) auto;
    align-items: center;
    gap: 5px;
    width: 100%;
    border: 0;
    padding: 4px 18px;
    background: transparent;
    color: var(--ds-text-subtle);
    font-size: 10.5px;
    font-weight: 600;
    letter-spacing: 0.06em;
    text-align: left;
    text-transform: uppercase;
    cursor: pointer;
  }
  .group-tag button:hover {
    color: var(--ds-text);
  }
  .group-chevron {
    display: inline-block;
    font-size: 14px;
    line-height: 1;
    transform: rotate(0deg);
  }
  .group-chevron--collapsed {
    transform: rotate(-90deg);
  }
  .group-count {
    font-variant-numeric: tabular-nums;
    letter-spacing: 0;
  }
  .group-ops {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  .op-row {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr);
    align-items: start;
    gap: 8px;
    padding: 5px 18px;
    text-decoration: none;
    color: var(--ds-text);
    font-size: 12.5px;
    line-height: 1.3;
    cursor: pointer;
  }
  .op-row:hover {
    background: var(--ds-surface-hovered);
  }
  .op-row--active {
    background: var(--ds-surface-selected);
  }
  .op-path {
    display: block;
    font-family: var(--ds-font-mono, ui-monospace, monospace);
    color: var(--ds-text);
    word-break: break-all;
  }
  .op-copy {
    min-width: 0;
  }
  .op-summary {
    display: block;
    margin-top: 2px;
    overflow: hidden;
    color: var(--ds-text-subtle);
    font-size: 11.5px;
    line-height: 1.35;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .empty {
    padding: 16px 18px;
    color: var(--ds-text-subtle);
    font-size: 12.5px;
  }
</style>
