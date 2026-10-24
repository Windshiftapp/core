<script>
  import { BasePicker } from '.';
  import { createAsyncLoader } from '../composables';
  import { pages as pagesApi } from '../api/pages.js';
  import FileText from '@lucide/svelte/icons/file-text';
  import { onDestroy, untrack } from 'svelte';
  import { t } from '../stores/i18n.svelte.js';

  let {
    value = $bindable(null),
    workspaceId,
    id = 'page-picker',
    placeholder = '',
    disabled = false,
    allowClear = true,
    class: className = '',
    inputTestid = undefined,
    optionTestid = null,
    onSelect = () => {},
    onCancel = () => {},
  } = $props();

  const resolvedPlaceholder = $derived(placeholder || t('pickers.searchPages'));

  let searchQuery = $state('');

  // Server-side title-substring search. The picker leaves the menu empty
  // until the user types — workspaces with many pages would otherwise pay
  // the full tree-fetch on every open.
  const pages = createAsyncLoader(async () => {
    if (!workspaceId) return [];
    const q = (searchQuery || '').trim();
    if (q.length < 1) return [];
    const result = await pagesApi.searchPages(workspaceId, q, { limit: 20 });
    const items = result?.results ?? (Array.isArray(result) ? result : []);
    return items;
  });
  onDestroy(() => pages.dispose());

  $effect(() => {
    if (workspaceId) {
      const _ = [workspaceId, searchQuery];
      untrack(() => pages.load());
    }
  });

  function handleSearchChange(query) {
    searchQuery = query;
  }
</script>

<BasePicker
  {id}
  bind:value
  items={pages.data || []}
  loading={pages.loading}
  error={pages.error}
  placeholder={resolvedPlaceholder}
  {disabled}
  {allowClear}
  class={className}
  serverSearch
  {inputTestid}
  {optionTestid}
  onSearchChange={handleSearchChange}
  getValue={(page) => page?.id}
  getLabel={(page) => page?.title ?? ''}
  {onSelect}
  {onCancel}
>
  {#snippet itemSnippet({ item: page })}
    <div class="flex items-center gap-3 flex-1 min-w-0">
      <div
        class="w-6 h-6 rounded flex items-center justify-center flex-shrink-0"
        style="background: var(--ds-background-neutral);"
      >
        <FileText size={14} style="color: var(--ds-text-subtle);" />
      </div>
      <div class="flex flex-col min-w-0 flex-1">
        <span class="font-medium truncate">{page.title || t('pages.untitled')}</span>
      </div>
    </div>
  {/snippet}

  {#snippet noResultsSnippet({ searchQuery: sq })}
    <div class="px-4 py-4 text-center text-sm" style="color: var(--ds-text-subtle);">
      {#if !sq}
        {t('pickers.startTypingToSearch')}
      {:else}
        {t('pickers.noResultsFor', { query: sq })}
      {/if}
    </div>
  {/snippet}
</BasePicker>
