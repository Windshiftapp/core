<script>
  import { BasePicker } from '.';
  import { createAsyncLoader } from '../composables';
  import { api } from '../api.js';
  import { untrack } from 'svelte';
  import { FileText } from 'lucide-svelte';
  import { t } from '../stores/i18n.svelte.js';

  let {
    value = $bindable(null),
    bucketId,
    placeholder = '',
    disabled = false,
    allowClear = true,
    autoOpen = false,
    class: className = '',
    onSelect = () => {},
    onCancel = () => {}
  } = $props();

  const resolvedPlaceholder = $derived(placeholder || 'Select a document...');

  let searchQuery = $state('');

  const documents = createAsyncLoader(async () => {
    if (!bucketId) return [];
    if (searchQuery) {
      const result = await api.logbook.keywordSearch(searchQuery, { bucket_id: bucketId });
      const items = result?.data ?? (Array.isArray(result) ? result : []);
      return items.map(r => ({ ...r, id: r.document_id || r.id }));
    }
    const result = await api.logbook.listDocuments(bucketId, { limit: 50 });
    return result?.data ?? (Array.isArray(result) ? result : []);
  });

  // Reload when bucketId or searchQuery changes
  $effect(() => {
    if (bucketId) {
      const _ = [bucketId, searchQuery];
      untrack(() => documents.load());
    }
  });

  function handleSearchChange(query) {
    searchQuery = query;
  }
</script>

<BasePicker
  bind:value
  items={documents.data || []}
  loading={documents.loading}
  error={documents.error}
  placeholder={resolvedPlaceholder}
  {disabled}
  {allowClear}
  class={className}
  serverSearch
  onSearchChange={handleSearchChange}
  getValue={(doc) => doc?.id}
  getLabel={(doc) => {
    if (!doc) return '';
    return doc.title || doc.id;
  }}
  onSelect={onSelect}
  onCancel={onCancel}
>
  {#snippet itemSnippet({ item: doc, isSelected })}
    <div class="flex items-center gap-3 flex-1 min-w-0">
      <div class="w-8 h-8 rounded flex items-center justify-center flex-shrink-0"
           style="background: var(--ds-background-neutral);">
        <FileText size={16} style="color: var(--ds-text-subtle);" />
      </div>
      <div class="flex flex-col min-w-0 flex-1">
        <span class="font-medium truncate">{doc.title || 'Untitled'}</span>
        <span class="text-xs truncate" style="color: var(--ds-text-subtle);">
          {doc.source_type || ''}
          {#if doc.content_type} · {doc.content_type}{/if}
        </span>
      </div>
    </div>
  {/snippet}

  {#snippet noResultsSnippet({ searchQuery: sq })}
    <div class="px-4 py-4 text-center text-sm" style="color: var(--ds-text-subtle);">
      {t('pickers.noResultsFor', { query: sq })}
    </div>
  {/snippet}
</BasePicker>
