<script>
  import { BasePicker } from '.';
  import { untrack } from 'svelte';
  import { api } from '../api.js';
  import { FileText } from '@lucide/svelte';
  import { t } from '../stores/i18n.svelte.js';

  let {
    workspaceId,
    value = $bindable(null),
    excludeIds = [],
    placeholder = '',
    label = '',
    disabled = false,
    autoOpen = false,
    class: className = '',
    onSelect = () => {},
    onCancel = () => {}
  } = $props();

  const resolvedPlaceholder = $derived(placeholder || t('pickers.searchTestCases'));

  let testCases = $state([]);
  let loading = $state(false);
  let error = $state(null);

  // Load test cases when workspaceId is available
  $effect(() => {
    if (workspaceId) {
      untrack(() => loadTestCases());
    }
  });

  // Server-backed directory read (WI-1448): first page of cases, then
  // search-as-you-type against the backend (title/folder_name) with a capped
  // page. No fetch-all.
  const PICKER_PAGE_SIZE = 50;
  let searchToken = 0;

  async function loadTestCases(query = '') {
    if (!workspaceId) return;
    const token = ++searchToken;
    const trimmed = query.trim();

    try {
      loading = true;
      error = null;
      const rows =
        (await api.tests.testCases.getAll(
          workspaceId,
          { limit: PICKER_PAGE_SIZE, ...(trimmed ? { q: trimmed } : {}) }
        )) || [];
      if (token !== searchToken) return;
      testCases = rows;
    } catch (err) {
      console.error('Failed to load test cases:', err);
      if (token !== searchToken) return;
      error = err.message || 'Failed to load test cases';
      testCases = [];
    } finally {
      if (token === searchToken) loading = false;
    }
  }

  // Filter out excluded IDs from the items
  const filteredTestCases = $derived.by(() => {
    const excludeSet = new Set(excludeIds);
    return testCases.filter(tc => !excludeSet.has(tc.id));
  });

  function handleSearchChange(query) {
    loadTestCases(query);
  }

  function handleSelect(item) {
    onSelect(item);
  }

  function handleCancel() {
    onCancel();
  }
</script>

<BasePicker
  bind:value
  id="test-case-picker"
  items={filteredTestCases}
  {loading}
  {error}
  placeholder={resolvedPlaceholder}
  {label}
  {disabled}
  class={className}
  searchFields={['title', 'folder_name']}
  getValue={(tc) => tc?.id}
  getLabel={(tc) => tc?.title ?? ''}
  onSearchChange={handleSearchChange}
  onSelect={handleSelect}
  onCancel={handleCancel}
  optionTestid={(option) => `test-case-picker-option-${option.value}`}
>
  {#snippet itemSnippet({ item: testCase, isSelected })}
    <div class="flex items-center gap-3 flex-1 min-w-0">
      <FileText size={16} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
      <div class="flex-1 min-w-0">
        <div class="font-medium truncate">{testCase.title}</div>
        <div class="text-xs truncate" style="color: var(--ds-text-subtle);">
          {testCase.folder_name || t('common.root')}
        </div>
      </div>
    </div>
  {/snippet}
</BasePicker>
