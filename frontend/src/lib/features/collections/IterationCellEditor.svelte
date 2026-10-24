<script>
  import Calendar from '@lucide/svelte/icons/calendar';
  import ItemPicker from '../../pickers/ItemPicker.svelte';
  import { iterationPickerConfig } from '../../pickers/pickerConfigs.js';
  import IterationCellValue from './IterationCellValue.svelte';

  let {
    canEdit = false,
    value = null,
    iteration = null,
    items = [],
    loading = false,
    placeholder = 'Set iteration',
    unassignedLabel = 'No iteration',
    selectPrompt = placeholder,
    onOpen = () => {},
    onSelect = () => {}
  } = $props();
</script>

{#if canEdit}
  <ItemPicker
    {value}
    {items}
    {loading}
    config={iterationPickerConfig}
    {placeholder}
    showUnassigned={true}
    {unassignedLabel}
    allowClear={true}
    onOpen={() => onOpen()}
    onSelect={(selected) => onSelect(selected)}
  >
    {#snippet children()}
      {#if iteration}
        <IterationCellValue {iteration} interactive />
      {:else}
        <span
          class="flex items-center gap-2 text-sm cursor-pointer whitespace-nowrap"
          style="color: var(--ds-text-subtle);"
          data-testid="iteration-cell-display"
        >
          <Calendar class="w-4 h-4 flex-shrink-0" />
          <span data-testid="iteration-cell-value">{selectPrompt}</span>
        </span>
      {/if}
    {/snippet}
  </ItemPicker>
{:else if iteration}
  <IterationCellValue {iteration} />
{:else}
  <span class="text-sm" style="color: var(--ds-text-subtle);">-</span>
{/if}
