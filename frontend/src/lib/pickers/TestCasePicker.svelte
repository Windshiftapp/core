<script>
  import { BasePicker } from '.';
  import { createEventDispatcher, untrack } from 'svelte';
  import { api } from '../api.js';
  import { FileText } from 'lucide-svelte';

  const dispatch = createEventDispatcher();

  let {
    workspaceId,
    value = $bindable(null),
    excludeIds = [],
    placeholder = 'Search test cases...',
    label = '',
    disabled = false,
    autoOpen = false,
    class: className = ''
  } = $props();

  let testCases = $state([]);
  let loading = $state(false);
  let error = $state(null);

  // Load test cases when workspaceId is available
  $effect(() => {
    if (workspaceId) {
      untrack(() => loadTestCases());
    }
  });

  async function loadTestCases() {
    if (loading || !workspaceId) return;

    try {
      loading = true;
      error = null;
      testCases = await api.tests.testCases.getAll(workspaceId, { all: true }) || [];
    } catch (err) {
      console.error('Failed to load test cases:', err);
      error = err.message || 'Failed to load test cases';
      testCases = [];
    } finally {
      loading = false;
    }
  }

  // Filter out excluded IDs from the items
  const filteredTestCases = $derived.by(() => {
    const excludeSet = new Set(excludeIds);
    return testCases.filter(tc => !excludeSet.has(tc.id));
  });

  function handleSelect(item) {
    dispatch('select', item);
  }

  function handleCancel() {
    dispatch('cancel');
  }
</script>

<BasePicker
  bind:value
  items={filteredTestCases}
  {loading}
  {error}
  {placeholder}
  {label}
  {disabled}
  class={className}
  searchFields={['title', 'folder_name']}
  getValue={(tc) => tc?.id}
  getLabel={(tc) => tc?.title ?? ''}
  onSelect={handleSelect}
  onCancel={handleCancel}
>
  {#snippet itemSnippet({ item: testCase, isSelected })}
    <div class="flex items-center gap-3 flex-1 min-w-0">
      <FileText size={16} style="color: var(--ds-text-subtle); flex-shrink: 0;" />
      <div class="flex-1 min-w-0">
        <div class="font-medium truncate">{testCase.title}</div>
        <div class="text-xs truncate" style="color: var(--ds-text-subtle);">
          {testCase.folder_name || 'Root'}
        </div>
      </div>
    </div>
  {/snippet}
</BasePicker>
