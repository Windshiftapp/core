<script>
  import { collectionEditorOptions } from '../stores/collectionEditorOptions.svelte.js';
  import { objectDisplayName } from '../utils/systemLabels.js';
  import ItemPicker from './ItemPicker.svelte';
  import Lozenge from '../components/Lozenge.svelte';
  import { createStatusPickerConfig } from './pickerConfigs.js';

  // Inline status editor shared by the list/tree cells and the item detail
  // link and child rows. Options are loaded lazily per owning workspace, so a
  // row from another workspace never offers this workspace's statuses.
  let {
    workspaceId,
    statusId,
    statusName = '',
    statusBuiltinKey = '',
    // Category color the row already carries, used until the options load.
    statusColor = null,
    statuses = [],
    statusCategories = [],
    canEdit = true,
    emptyText = '-',
    onselect = undefined,
  } = $props();

  let editorOptions = $derived(collectionEditorOptions.get(workspaceId));
  let selectedStatus = $derived(
    [...editorOptions.statuses, ...statuses].find((s) => s.id === statusId) ||
      (statusName ? { name: statusName, builtin_key: statusBuiltinKey, category_id: null } : null)
  );
  let color = $derived(
    statusCategories.find((sc) => sc.id === selectedStatus?.category_id)?.color ||
      selectedStatus?.category_color ||
      statusColor ||
      '#6b7280'
  );
  let label = $derived(selectedStatus ? objectDisplayName(selectedStatus, 'status') : null);
  const statusConfig = $derived(createStatusPickerConfig(statusCategories));
</script>

{#if canEdit}
  <ItemPicker
    value={statusId}
    items={editorOptions.statuses}
    loading={editorOptions.loading.statuses}
    config={statusConfig}
    placeholder="Set status"
    showUnassigned={false}
    allowClear={false}
    onOpen={() => collectionEditorOptions.load(workspaceId, 'statuses')}
    onSelect={async (selected) => {
      const nextStatusId = selected?.id;
      if (nextStatusId && nextStatusId !== statusId) {
        await onselect?.(nextStatusId);
      }
    }}
  >
    {#snippet children()}
      <span class="cursor-pointer">
        <Lozenge text={label ?? 'Set status'} customBg={color} />
      </span>
    {/snippet}
  </ItemPicker>
{:else}
  <Lozenge text={label ?? emptyText} customBg={color} />
{/if}
