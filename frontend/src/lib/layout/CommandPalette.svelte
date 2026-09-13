<script>
  import { createCombobox, melt } from '@melt-ui/svelte';
  import { scale } from 'svelte/transition';
  import { backOut } from 'svelte/easing';
  import { api } from '../api.js';
  import { contextCommands } from '../utils/contextCommands.js';
  import { currentRoute } from '../router.js';
  import { permissionStore, workspacesStore, isSystemAdmin, currentWorkspace } from '../stores';
  import { moduleSettings } from '../stores/moduleSettings.js';
  import { timerStore } from '../stores/timerStore.svelte.js';
  import { t } from '../stores/i18n.svelte.js';
  import ModalBackdrop from '../components/ModalBackdrop.svelte';

  import { scoreCommand } from '../commands/score.js';
  import { BUCKET, BUCKET_LABELS } from '../commands/buckets.js';
  import { rankCommands } from '../commands/rank.js';
  import { buildContext } from '../commands/context.js';
  import { buildCommands } from '../commands/buildCommands.js';
  import { executeCommand as runCommand } from '../commands/executor.js';
  import {
    adminProvider,
    createProvider,
    globalNavigationProvider,
    makeExternalProvider,
    recentlyViewedProvider,
    searchProvider,
    systemProvider,
    timeProvider,
    workspaceActionsProvider,
    workspaceNavigationProvider,
    workspacesProvider,
  } from '../commands/providers/index.js';

  let {
    isOpen = $bindable(false),
    onclose,
  } = $props();

  let workspaces = $state([]);
  let workItems = $state([]);
  let searchTimeout;

  // Sub-palette state. `mode` is 'commands' (the default command list) or
  // 'recent' (the recently-viewed work-item picker reached from the launcher
  // entry). recentItems holds the last 20 viewed items for that mode.
  let mode = $state('commands');
  let recentItems = $state([]);
  let recentLoading = $state(false);

  async function loadData() {
    try {
      const tasks = [];
      if (!$workspacesStore.loaded) tasks.push(workspacesStore.load());
      if (!$workspacesStore.personalWorkspace) tasks.push(workspacesStore.loadPersonalWorkspace());
      if (tasks.length) await Promise.all(tasks);
      workspaces = [
        ...($workspacesStore.personalWorkspace ? [$workspacesStore.personalWorkspace] : []),
        ...$workspacesStore.regularWorkspaces,
      ];
    } catch (err) {
      console.error('Failed to load data for command palette:', err);
    }
  }

  async function searchWorkItems(query) {
    if (!query || query.length < 2) {
      workItems = [];
      return;
    }
    try {
      const results = await api.search.items({ query: query.trim(), limit: 6 });
      workItems = results || [];
    } catch (err) {
      console.error('Failed to search work items:', err);
      workItems = [];
    }
  }

  function debouncedSearchWorkItems(query) {
    clearTimeout(searchTimeout);
    searchTimeout = setTimeout(() => searchWorkItems(query), 300);
  }

  const {
    elements: { menu, input, option },
    states: { open, inputValue, selected },
  } = createCombobox({
    forceVisible: true,
    portal: null,
  });

  $effect(() => {
    if (isOpen !== $open) open.set(isOpen);
  });

  $effect(() => {
    if (mode !== 'commands') return;
    if ($inputValue && $inputValue.length >= 2) {
      debouncedSearchWorkItems($inputValue);
    } else if ($inputValue.length < 2) {
      workItems = [];
    }
  });

  // Provider order: focused-entity first, then workspace-context, then
  // global. Within-bucket ordering matches plan's bucket display order.
  // The external provider wraps registerContextCommands consumers (ItemDetail
  // today) so component-pushed commands fall into the item-actions bucket
  // by default.
  const PROVIDERS = [
    recentlyViewedProvider,
    makeExternalProvider(() => $contextCommands),
    workspaceActionsProvider,
    workspaceNavigationProvider,
    globalNavigationProvider,
    workspacesProvider,
    createProvider,
    adminProvider,
    timeProvider,
    searchProvider,
    systemProvider,
  ];

  const commands = $derived(buildCommands(
    buildContext({
      route: $currentRoute,
      permissions: $permissionStore,
      isSystemAdmin: $isSystemAdmin,
      modules: $moduleSettings,
      workspaces,
      currentWorkspace: $currentWorkspace,
      workItems,
      activeTimer: timerStore.activeTimer,
      t,
      query: $inputValue,
    }),
    PROVIDERS,
  ));

  // Recently-viewed items mapped to command-shaped entries so the existing
  // render loop + keyboard handling drive navigation. No per-bucket cap is
  // applied here — the backend already bounds the list to the last 20.
  const recentCommands = $derived(
    recentItems.map((it) => {
      const key = `${it.workspace_key || 'WORK'}-${it.workspace_item_number || it.item_id}`;
      return {
        id: `recent-item-${it.item_id}`,
        label: `${key}: ${it.title}`,
        description: it.status || '',
        bucket: BUCKET.RECENT,
        keywords: [key.toLowerCase(), it.title?.toLowerCase()].filter(Boolean),
        url: `/workspaces/${it.workspace_id}/items/${it.item_id}`,
      };
    }),
  );

  function filterRecent(query, list) {
    const q = query.trim();
    if (!q) return list;
    return list
      .map((c) => ({
        c,
        s: scoreCommand(q, { label: c.label, description: c.description, keywords: c.keywords }),
      }))
      .filter((x) => x.s > 0)
      .sort((a, b) => b.s - a.s)
      .map((x) => x.c);
  }

  let filteredCommands = $derived(
    mode === 'recent' ? filterRecent($inputValue, recentCommands) : rankCommands($inputValue, commands),
  );

  let userInteracted = $state(false);

  // Auto-select the first entry so Enter works immediately. In recent mode the
  // list is meaningful with an empty query, so select even before the user
  // types.
  $effect(() => {
    if (filteredCommands.length > 0 && !userInteracted && (mode === 'recent' || $inputValue.trim())) {
      const first = filteredCommands[0];
      selected.set({ value: first.id, label: first.label });
    }
  });

  async function loadRecentItems() {
    recentLoading = true;
    try {
      const data = await api.homepage.get();
      const list = (data?.recently_viewed ?? [])
        .filter((a) => a && a.item_id)
        .map((a) => ({ ...a, lastActivityDate: a.last_activity ? new Date(a.last_activity) : null }));
      list.sort((a, b) => (b.lastActivityDate?.getTime() ?? 0) - (a.lastActivityDate?.getTime() ?? 0));
      recentItems = list.slice(0, 20);
    } catch (err) {
      console.error('Failed to load recently viewed items:', err);
      recentItems = [];
    } finally {
      recentLoading = false;
    }
  }

  async function enterRecentMode() {
    mode = 'recent';
    userInteracted = false;
    inputValue.set('');
    await loadRecentItems();
  }

  function exitRecentMode() {
    mode = 'commands';
    userInteracted = false;
    inputValue.set('');
  }

  async function executeAndClose(cmd) {
    // The recently-viewed launcher opens a sub-palette instead of executing.
    if (cmd?.submenu === 'recent') {
      await enterRecentMode();
      return;
    }
    try {
      await runCommand(cmd);
    } catch (err) {
      console.error('[command-palette] execute failed:', err);
    } finally {
      close();
    }
  }

  function close() {
    isOpen = false;
    open.set(false);
    inputValue.set('');
    mode = 'commands';
    onclose?.();
  }

  function handleKeydown(e) {
    if (!isOpen) return;
    if (e.key === 'Enter' && $selected) {
      e.preventDefault();
      const cmd = filteredCommands.find((c) => c.id === $selected.value);
      if (cmd) executeAndClose(cmd);
    } else if (e.key === 'Backspace' && mode === 'recent' && $inputValue === '') {
      // Backspace on an empty query steps back out of the sub-palette.
      e.preventDefault();
      exitRecentMode();
    } else if (e.key === 'ArrowUp' || e.key === 'ArrowDown') {
      userInteracted = true;
    }
  }

  let searchInputRef = $state(null);

  $effect(() => {
    if (isOpen) {
      loadData();
    }
  });

  $effect(() => {
    if (isOpen && searchInputRef) {
      setTimeout(() => {
        searchInputRef.focus();
        searchInputRef.select();
      }, 50);
    }
  });
</script>

<style>
  .command-palette-container {
    animation: scale-in var(--duration-normal, 200ms) var(--ease-spring, cubic-bezier(0.34, 1.56, 0.64, 1)) forwards;
  }

  [data-highlighted] {
    background-color: var(--ds-background-neutral-hovered) !important;
  }

  [data-melt-combobox-menu] {
    position: static !important;
    width: 100% !important;
    transform: none !important;
    top: auto !important;
    left: auto !important;
  }

  .command-option {
    transition: background-color var(--duration-fast, 100ms) ease;
  }

  .command-option:hover {
    background-color: var(--ds-background-neutral-hovered);
  }

  .bucket-header {
    padding: 0.5rem 1rem 0.25rem;
    font-size: 0.7rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    border-top: 1px solid;
  }
  .bucket-header:first-child {
    border-top: none;
  }

  .kbd {
    background-color: var(--ds-surface);
    color: var(--ds-text-subtle);
    transition: background-color var(--duration-fast, 100ms) ease;
  }

  .kbd:hover {
    background-color: var(--ds-background-neutral-hovered);
  }

  @media (prefers-reduced-motion: reduce) {
    .command-palette-container {
      animation: none;
    }
  }
</style>

<svelte:window onkeydown={handleKeydown} />

<ModalBackdrop bind:show={isOpen} opacity={0.4} blur={8} extraFilter="saturate(120%)" zIndex={60} align="top" paddingTop="pt-[20vh]" onclose={close}>
  <div
    class="relative w-full max-w-2xl mx-4"
    transition:scale={{ duration: 200, start: 0.95, easing: backOut }}
  >
    <div class="command-palette-container rounded-lg overflow-hidden" style="background-color: var(--ds-glass-bg, var(--ds-surface-raised)); backdrop-filter: blur(12px) saturate(180%); -webkit-backdrop-filter: blur(12px) saturate(180%); border: 1px solid var(--ds-glass-border, var(--ds-border)); box-shadow: var(--shadow-float, 0 20px 50px rgba(0, 0, 0, 0.18));">
      <div class="p-4 border-b" style="background-color: var(--ds-surface); border-color: var(--ds-border);">
        <input
          bind:this={searchInputRef}
          use:melt={$input}
          data-testid="command-palette-input"
          type="text"
          placeholder={mode === 'recent' ? t('commandPalette.recentlyViewed.searchPlaceholder') : t('commandPalette.searchPlaceholder')}
          class="w-full text-lg border-none outline-none bg-transparent"
          style="color: var(--ds-text);"
        />
      </div>

      {#if $open}
        <div
          use:melt={$menu}
          class="w-full"
          style="background-color: var(--ds-surface-raised);"
        >
          {#if mode === 'recent'}
            <button
              type="button"
              onclick={exitRecentMode}
              data-testid="command-palette-recent-back"
              class="w-full flex items-center gap-2 px-4 py-2 text-left text-sm font-medium command-option"
              style="color: var(--ds-text-subtle); border-bottom: 1px solid var(--ds-border);"
            >
              <span aria-hidden="true">←</span>
              {t('commandPalette.recentlyViewed.header')}
            </button>
          {/if}

          {#if filteredCommands.length === 0}
            <div class="p-4 text-center" style="color: var(--ds-text-subtle);">
              {#if mode === 'recent' && recentLoading}
                {t('commandPalette.recentlyViewed.loading')}
              {:else if mode === 'recent'}
                {t('commandPalette.recentlyViewed.empty')}
              {:else}
                {t('commandPalette.noCommandsFound')}
              {/if}
            </div>
          {:else}
            <div class="max-h-96 overflow-y-auto">
              {#each filteredCommands as command, i}
                {#if mode !== 'recent' && (i === 0 || filteredCommands[i - 1].bucket !== command.bucket)}
                  <div class="bucket-header" style="color: var(--ds-text-subtle); background-color: var(--ds-surface); border-color: var(--ds-border);">
                    {BUCKET_LABELS[command.bucket] || ''}
                  </div>
                {/if}
                <div
                  use:melt={$option({ value: command.id, label: command.label })}
                  onclick={() => executeAndClose(command)}
                  data-testid={`command-palette-option-${command.id}`}
                  class="w-full text-left px-4 py-2.5 transition-colors cursor-pointer command-option"
                >
                  <div class="flex items-center gap-2">
                    <div class="font-medium" style="color: var(--ds-text);">{command.label}</div>
                    {#if command._isContextCommand}
                      <span class="px-1.5 py-0.5 text-xs rounded font-medium" style="background-color: var(--ds-accent-blue-subtler); color: var(--ds-accent-blue);">
                        {t('commandPalette.context')}
                      </span>
                    {/if}
                  </div>
                  {#if command.description}
                    <div class="text-sm mt-0.5" style="color: var(--ds-text-subtle);">{command.description}</div>
                  {/if}
                </div>
              {/each}
            </div>
          {/if}

          <div class="p-3 border-t" style="background-color: var(--ds-surface); border-color: var(--ds-border);">
            <div class="flex justify-between text-xs mb-2" style="color: var(--ds-text-subtle);">
              <div>
                <kbd class="kbd px-1 py-0.5 rounded text-xs">↵</kbd> {t('commandPalette.toSelect')}
                <kbd class="kbd px-1 py-0.5 rounded text-xs ml-2">↑↓</kbd> {t('commandPalette.toNavigate')}
                {#if mode === 'recent'}
                  <kbd class="kbd px-1 py-0.5 rounded text-xs ml-2">⌫</kbd> {t('commandPalette.recentlyViewed.backHint')}
                {/if}
              </div>
              <div>
                <kbd class="kbd px-1 py-0.5 rounded text-xs">ESC</kbd> {t('commandPalette.toClose')}
              </div>
            </div>
            <div class="flex justify-between items-center">
              <button
                onclick={() => executeAndClose({ url: '/search' })}
                class="text-xs underline"
                style="color: var(--ds-interactive);"
              >
                {t('commandPalette.advancedSearch')}
              </button>
              <div class="text-xs" style="color: var(--ds-text-subtlest);">
                {t('commandPalette.pressToOpen', { shortcut: '⎵⎵' })}
              </div>
            </div>
          </div>
        </div>
      {/if}
    </div>
  </div>
</ModalBackdrop>
