<script>
  import { onMount } from 'svelte';
  import { ChevronRight, Command as CommandIcon, House, Search } from '@lucide/svelte';
  import { navigate } from '../router.js';
  import { workspacesStore } from '../stores';
  import { formatRelativeCompact } from '../utils/dateFormatter.js';
  import MobileHeader from './MobileHeader.svelte';
  import MobileListState from './MobileListState.svelte';
  import { fetchWorkspacePageSections } from './mobilePagesData.js';
  import { mobilePalette } from './mobilePalette.svelte.js';

  // Knowledge-pages list across every workspace the user belongs to. The
  // desktop page tree is workspace-scoped; on the phone one flat, grouped
  // list plus the filter (and the command palette) is the primary navigation.
  let sections = $state([]);
  let loading = $state(true);
  let errored = $state(false);
  let filter = $state('');
  let loadSeq = 0;

  const trimmedFilter = $derived(filter.trim().toLowerCase());

  const visibleSections = $derived(
    !trimmedFilter
      ? sections
      : sections
          .map((s) => ({
            ...s,
            pages: s.pages.filter((p) => p.title?.toLowerCase().includes(trimmedFilter)),
          }))
          .filter((s) => s.pages.length > 0),
  );

  function openPage(workspaceId, pageId) {
    navigate(`/m/pages/${workspaceId}/${pageId}`);
  }

  async function load() {
    const seq = ++loadSeq;
    loading = true;
    errored = false;
    try {
      // MobileShell loads the workspace list eagerly; the personal workspace
      // is on-demand and pages live there too.
      if (!$workspacesStore.personalWorkspace) await workspacesStore.loadPersonalWorkspace?.();
      const workspaces = [
        ...($workspacesStore.personalWorkspace ? [$workspacesStore.personalWorkspace] : []),
        ...$workspacesStore.regularWorkspaces,
      ];
      const loaded = await fetchWorkspacePageSections(workspaces);
      if (seq !== loadSeq) return;
      // fetchWorkspacePageSections swallows per-workspace failures (permission
      // or deletion); only a hard throw above surfaces the error state.
      sections = loaded;
    } catch (err) {
      console.error('Failed to load pages:', err);
      if (seq === loadSeq) errored = true;
    } finally {
      if (seq === loadSeq) loading = false;
    }
  }

  onMount(load);
</script>

<MobileHeader title="Pages">
  {#snippet right()}
    <button
      class="hdr-btn"
      onclick={() => mobilePalette.open()}
      data-testid="mobile-palette-open"
      aria-label="Command palette"
      type="button"
    >
      <CommandIcon size={20} />
    </button>
  {/snippet}
  {#snippet children()}
    <div class="field">
      <Search size={16} class="f-icon" />
      <input
        bind:value={filter}
        data-testid="mobile-pages-filter"
        type="search"
        enterkeyhint="search"
        autocomplete="off"
        placeholder="Filter pages…"
      />
    </div>
  {/snippet}
</MobileHeader>

<div class="pages-list" data-testid="mobile-pages-view">
  <MobileListState
    loading={loading}
    errored={errored}
    rowCount={visibleSections.reduce((n, s) => n + s.pages.length, 0)}
    errorMessage="Couldn't load pages."
    emptyMessage={trimmedFilter ? 'No pages match your filter.' : 'No pages yet.'}
    onretry={load}
  >
    {#each visibleSections as section (section.workspace.id)}
      <h2 class="ws-name" data-testid={`mobile-pages-section-${section.workspace.id}`}>
        {section.workspace.name}
      </h2>
      <div class="rows">
        {#each section.pages as page (page.id)}
          <button
            class="row"
            style:padding-left={`calc(0.875rem + ${Math.min(page.depth ?? 0, 6) * 0.75}rem)`}
            onclick={() => openPage(section.workspace.id, page.id)}
            data-testid="mobile-page-row"
            data-page-id={page.id}
            type="button"
          >
            <span class="row-main">
              <span class="row-title">
                {#if page.is_home}<House size={13} class="home-icon" />{/if}
                {page.title}
              </span>
              {#if page.excerpt}
                <span class="row-excerpt">{page.excerpt}</span>
              {/if}
            </span>
            <span class="row-side">
              {#if page.updated_at}
                <time class="row-time">{formatRelativeCompact(new Date(page.updated_at))}</time>
              {/if}
              <ChevronRight size={16} class="chev" />
            </span>
          </button>
        {/each}
      </div>
    {/each}
  </MobileListState>
</div>

<style>
  .hdr-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 36px;
    height: 36px;
    border: none;
    background: transparent;
    color: var(--ds-text);
    cursor: pointer;
  }

  .field {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    min-height: 38px;
    margin: 0 0.75rem 0.5rem;
    padding: 0 0.625rem;
    border: 1px solid var(--ds-border);
    border-radius: var(--radius-lg, 8px);
    background-color: var(--ds-surface-raised);
  }
  .field :global(.f-icon) {
    color: var(--ds-text-subtle);
    flex-shrink: 0;
  }
  .field input {
    flex: 1;
    min-width: 0;
    border: none;
    outline: none;
    background: transparent;
    font-size: 0.9375rem;
    color: var(--ds-text);
  }

  .pages-list { padding: 0.25rem 0 1rem; }

  .ws-name {
    padding: 0.75rem 0.875rem 0.3rem;
    font-size: 0.75rem;
    font-weight: var(--font-semibold, 600);
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--ds-text-subtle);
    margin: 0;
  }

  .rows {
    display: flex;
    flex-direction: column;
  }

  .row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.75rem;
    width: 100%;
    min-height: 52px;
    padding: 0.4rem 0.875rem;
    border: none;
    border-bottom: 1px solid var(--ds-border);
    background: transparent;
    text-align: left;
    cursor: pointer;
  }
  .row:active {
    background-color: var(--ds-background-neutral-hovered);
  }
  .row-main {
    display: flex;
    flex-direction: column;
    gap: 1px;
    min-width: 0;
  }
  .row-title {
    display: inline-flex;
    align-items: center;
    gap: 0.3rem;
    font-size: 0.9375rem;
    font-weight: var(--font-medium, 500);
    color: var(--ds-text);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .row-title :global(.home-icon) {
    color: var(--ds-icon-subtle, var(--ds-text-subtle));
    flex-shrink: 0;
  }
  .row-excerpt {
    font-size: 0.78125rem;
    color: var(--ds-text-subtle);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .row-side {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    flex-shrink: 0;
  }
  .row-time {
    font-size: 0.75rem;
    color: var(--ds-text-subtlest, var(--ds-text-subtle));
  }
  .row-side :global(.chev) {
    color: var(--ds-icon-subtle, var(--ds-text-subtle));
  }
</style>
