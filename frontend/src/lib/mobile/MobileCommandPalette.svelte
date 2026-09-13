<script>
  import { Loader, Search } from '@lucide/svelte';
  import { api } from '../api.js';
  import { navigate } from '../router.js';
  import { workspacesStore } from '../stores';
  import { BUCKET_LABELS } from '../commands/buckets.js';
  import { buildContext } from '../commands/context.js';
  import { buildCommands } from '../commands/buildCommands.js';
  import { rankCommands } from '../commands/rank.js';
  import { executeCommand as runDesktopCommand } from '../commands/executor.js';
  import { mobileNavigationProvider } from '../commands/providers/mobileNavigationProvider.js';
  import { pageSearchProvider } from '../commands/providers/pageSearchProvider.js';
  import { searchProvider } from '../commands/providers/searchProvider.js';
  import { mobilePalette } from './mobilePalette.svelte.js';
  import { toMobileUrl } from './mobileUrls.js';
  import { searchPagesAcrossWorkspaces } from './mobilePagesData.js';
  import { timerStore } from '../stores/timerStore.svelte.js';
  import { t } from '../stores/i18n.svelte.js';

  // Phone-shaped command sheet. Reuses the desktop command providers and
  // ranking; only the shell (bottom sheet, touch targets) and the executor
  // (desktop URLs rewritten to /m routes) are mobile-specific.
  const PROVIDERS = [mobileNavigationProvider, searchProvider, pageSearchProvider];

  const SEARCH_DEBOUNCE_MS = 300;
  const MIN_QUERY_LEN = 2;

  let query = $state('');
  let workItems = $state([]);
  let pageResults = $state([]);
  let searching = $state(false);
  let highlightIndex = $state(0);
  let inputEl = $state(null);
  let debounceTimer = null;
  let searchSeq = 0;

  const isOpen = $derived(mobilePalette.isOpen);

  const workspacesForSearch = $derived([
    ...($workspacesStore.personalWorkspace ? [$workspacesStore.personalWorkspace] : []),
    ...$workspacesStore.regularWorkspaces,
  ]);

  // Same shape the desktop palette feeds buildContext; pageResults is the
  // only mobile addition (the sheet surfaces page hits inline).
  const commands = $derived(
    buildCommands(
      buildContext({
        route: { path: '/m', view: null, params: {}, query: {} },
        permissions: {},
        isSystemAdmin: false,
        modules: {},
        workspaces: workspacesForSearch,
        currentWorkspace: null,
        workItems,
        pageResults,
        activeTimer: timerStore.activeTimer,
        t,
        query,
      }),
      PROVIDERS,
    ),
  );

  const filtered = $derived(rankCommands(query, commands));

  async function runSearch(q) {
    const trimmed = q.trim();
    if (trimmed.length < MIN_QUERY_LEN) {
      workItems = [];
      pageResults = [];
      searching = false;
      return;
    }
    const seq = ++searchSeq;
    searching = true;
    try {
      const [itemsRes, pagesRes] = await Promise.allSettled([
        api.search.items({ query: trimmed, limit: 6 }),
        searchPagesAcrossWorkspaces(workspacesForSearch, trimmed),
      ]);
      if (seq !== searchSeq) return;
      workItems = itemsRes.status === 'fulfilled' ? (itemsRes.value ?? []) : [];
      pageResults = pagesRes.status === 'fulfilled' ? (pagesRes.value ?? []) : [];
    } finally {
      if (seq === searchSeq) searching = false;
    }
  }

  function onInput() {
    clearTimeout(debounceTimer);
    debounceTimer = setTimeout(() => runSearch(query), SEARCH_DEBOUNCE_MS);
    highlightIndex = 0;
  }

  function executeAndClose(cmd) {
    if (!cmd) return;
    mobilePalette.close();
    // Providers authored for the desktop palette may navigate with desktop
    // URLs; rewrite known ones so the phone stays in the mobile shell.
    const mobileCmd = cmd.url ? { ...cmd, url: toMobileUrl(cmd.url) } : cmd;
    runDesktopCommand(mobileCmd).catch((err) => console.error('[mobile-palette] execute failed:', err));
  }

  function onKeydown(e) {
    if (!isOpen) return;
    if (e.key === 'Escape') {
      e.preventDefault();
      mobilePalette.close();
    } else if (e.key === 'ArrowDown') {
      e.preventDefault();
      highlightIndex = Math.min(highlightIndex + 1, filtered.length - 1);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      highlightIndex = Math.max(highlightIndex - 1, 0);
    } else if (e.key === 'Enter') {
      e.preventDefault();
      executeAndClose(filtered[highlightIndex]);
    }
  }

  $effect(() => {
    if (isOpen) {
      query = '';
      workItems = [];
      pageResults = [];
      highlightIndex = 0;
      setTimeout(() => inputEl?.focus(), 50);
    }
  });
</script>

<svelte:window onkeydown={onKeydown} />

{#if isOpen}
  <div class="palette-root" data-testid="mobile-command-palette">
    <button
      class="backdrop"
      onclick={() => mobilePalette.close()}
      aria-label="Close command palette"
      type="button"
    ></button>
    <div class="sheet" role="dialog" aria-label="Command palette">
      <div class="grabber" aria-hidden="true"></div>
      <div class="input-row">
        <Search size={18} class="search-icon" />
        <input
          bind:this={inputEl}
          bind:value={query}
          oninput={onInput}
          data-testid="mobile-command-palette-input"
          type="text"
          enterkeyhint="go"
          autocomplete="off"
          placeholder="Search commands, items, pages…"
        />
        {#if searching}
          <Loader size={16} class="spin" />
        {/if}
      </div>

      <div class="list" data-testid="mobile-command-palette-list">
        {#if filtered.length === 0}
          <p class="empty" data-testid="mobile-command-palette-empty">
            {query.trim() ? 'Nothing matches.' : 'Type to search, or pick a destination.'}
          </p>
        {:else}
          {#each filtered as cmd, i (cmd.id)}
            {#if i === 0 || filtered[i - 1].bucket !== cmd.bucket}
              <div class="bucket" data-testid={`mobile-command-palette-bucket-${cmd.bucket}`}>
                {BUCKET_LABELS[cmd.bucket] || ''}
              </div>
            {/if}
            <button
              class="cmd"
              class:active={i === highlightIndex}
              onclick={() => executeAndClose(cmd)}
              data-testid={`mobile-command-palette-option-${cmd.id}`}
              type="button"
            >
              <span class="cmd-label">{cmd.label}</span>
              {#if cmd.description}
                <span class="cmd-desc">{cmd.description}</span>
              {/if}
            </button>
          {/each}
        {/if}
      </div>
    </div>
  </div>
{/if}

<style>
  .palette-root {
    position: fixed;
    inset: 0;
    /* Above the bottom nav (z-200): the sheet is modal and its list must
       stay tappable all the way to the bottom of the screen. */
    z-index: 300;
    display: flex;
    flex-direction: column;
    justify-content: flex-end;
  }

  .backdrop {
    position: absolute;
    inset: 0;
    border: none;
    background-color: rgba(0, 0, 0, 0.4);
    cursor: default;
  }

  .sheet {
    position: relative;
    display: flex;
    flex-direction: column;
    max-height: 78dvh;
    background-color: var(--ds-surface-raised);
    border-top: 1px solid var(--ds-border);
    border-radius: var(--radius-xl, 14px) var(--radius-xl, 14px) 0 0;
    box-shadow: var(--shadow-float, 0 12px 40px rgba(0, 0, 0, 0.3));
    padding-bottom: env(safe-area-inset-bottom, 0px);
  }

  .grabber {
    align-self: center;
    width: 36px;
    height: 4px;
    margin: 0.5rem 0 0.25rem;
    border-radius: var(--radius-full, 9999px);
    background-color: var(--ds-border-bold, var(--ds-border));
  }

  .input-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.5rem 0.875rem;
    border-bottom: 1px solid var(--ds-border);
  }
  .input-row :global(.search-icon) {
    color: var(--ds-text-subtle);
    flex-shrink: 0;
  }
  .input-row :global(.spin) {
    animation: spin 1s linear infinite;
    color: var(--ds-text-subtle);
    flex-shrink: 0;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
  input {
    flex: 1;
    min-width: 0;
    border: none;
    outline: none;
    background: transparent;
    font-size: 1.0625rem;
    color: var(--ds-text);
  }

  .list {
    overflow-y: auto;
    -webkit-overflow-scrolling: touch;
    overscroll-behavior: contain;
    padding-bottom: 0.5rem;
  }

  .empty {
    padding: 1.5rem 1rem;
    text-align: center;
    color: var(--ds-text-subtle);
    font-size: 0.875rem;
  }

  .bucket {
    padding: 0.6rem 0.875rem 0.25rem;
    font-size: 0.7rem;
    font-weight: var(--font-semibold, 600);
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--ds-text-subtle);
  }

  .cmd {
    display: flex;
    flex-direction: column;
    align-items: stretch;
    gap: 1px;
    width: 100%;
    min-height: 48px;
    padding: 0.45rem 0.875rem;
    border: none;
    background: transparent;
    text-align: left;
    cursor: pointer;
  }
  .cmd.active,
  .cmd:active {
    background-color: var(--ds-background-neutral-hovered);
  }
  .cmd-label {
    font-size: 0.9375rem;
    font-weight: var(--font-medium, 500);
    color: var(--ds-text);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .cmd-desc {
    font-size: 0.78125rem;
    color: var(--ds-text-subtle);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
